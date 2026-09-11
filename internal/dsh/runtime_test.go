package dsh

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRuntimeRejectsIncompleteConfig(t *testing.T) {
	runtime := New(t.TempDir(), nil)
	if err := runtime.Start(context.Background(), Config{}); err == nil {
		t.Fatal("空配置应被拒绝")
	}
}

func TestRuntimeOutlivesInitializationContext(t *testing.T) {
	runtime := New(t.TempDir(), func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `read line; echo '{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"deepseek-harness-sdk-runtime"}}}'; sleep 30`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := runtime.Start(ctx, Config{Provider: "deepseek-official", Model: "test", Root: t.TempDir(), APIKey: "test-key"}); err != nil {
		t.Fatal(err)
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
	if !runtime.Status().Running {
		t.Fatal("初始化请求结束后 sidecar 应继续运行")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStartsAndInitializesSDK(t *testing.T) {
	runtime := New(t.TempDir(), func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `read line; echo '{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"deepseek-harness-sdk-runtime","version":"test"}}}'; sleep 30`)
	})
	if err := runtime.Start(context.Background(), Config{Provider: "deepseek-official", Model: "test", Root: t.TempDir(), APIKey: "test-key"}); err != nil {
		t.Fatal(err)
	}
	if !runtime.Status().Ready {
		t.Fatal("初始化后 runtime 应就绪")
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultCommandInitializesProfileOnlyOnce(t *testing.T) {
	t.Setenv("ALX_DSH_BIN", "dsh-test")
	home := t.TempDir()
	if !slices.Contains(defaultCommand(context.Background(), home).Args, "--from-default-profile") {
		t.Fatal("首次启动应初始化受管 profile")
	}
	profile := filepath.Join(home, "profiles", "alemonx")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "package.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(defaultCommand(context.Background(), home).Args, "--from-default-profile") {
		t.Fatal("已有 profile 时不应再次初始化")
	}
}

func TestDefaultCommandAppliesOwnedApprovalPatch(t *testing.T) {
	t.Setenv("ALX_DSH_BIN", "dsh-test")
	home := t.TempDir()
	if err := writeRuntimePatch(home); err != nil {
		t.Fatal(err)
	}
	args := defaultCommand(context.Background(), home).Args
	if !slices.Contains(args, "--patch") || !slices.Contains(args, filepath.Join(home, "alemonx.patch.yml")) {
		t.Fatalf("未加载受管 approval patch：%#v", args)
	}
	raw, err := os.ReadFile(filepath.Join(home, "alemonx.patch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "@alemonx/dsh-approval-bridge") || !strings.Contains(string(raw), "ALX_DSH_BRIDGE_TOKEN") {
		t.Fatalf("approval patch 不完整：%s", raw)
	}
}

func TestRuntimePatchLinksBundledApprovalBridge(t *testing.T) {
	packageRoot := t.TempDir()
	bridge := filepath.Join(packageRoot, "node_modules", "@alemonx", "dsh-approval-bridge")
	if err := os.MkdirAll(bridge, 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(packageRoot, "node_modules", ".bin", "dsh")
	if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALX_DSH_BIN", bin)
	home := t.TempDir()
	if err := writeRuntimePatch(home); err != nil {
		t.Fatal(err)
	}
	linked, err := os.Readlink(filepath.Join(home, "node_modules", "@alemonx", "dsh-approval-bridge"))
	if err != nil || filepath.Clean(linked) != filepath.Clean(bridge) {
		t.Fatalf("approval bridge link=%q err=%v", linked, err)
	}
}

func TestConfigNeverPersistsCredential(t *testing.T) {
	runtime := New(t.TempDir(), nil)
	want := Config{Provider: "deepseek-official", Model: "deepseek-flash", Root: t.TempDir(), APIKey: "new-dsh-key"}
	if err := runtime.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(runtime.dir, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), want.APIKey) {
		t.Fatalf("配置泄露了 API key：%s", raw)
	}
	if _, err := runtime.LoadConfig(); err == nil {
		t.Fatal("缺少凭据的持久化配置不得被用于自动启动")
	}
}

func TestManagedEnvironmentReplacesInheritedDSHCredentials(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "old-key")
	t.Setenv("DSH_HOME", "old-home")
	t.Setenv("DSH_PERMISSION_MODE", "danger-full-access")
	env := managedEnvironment("new-home", "bridge-token", Config{APIKey: "new-key"})
	values := map[string]string{}
	for _, pair := range env {
		key, value, _ := strings.Cut(pair, "=")
		values[key] = value
	}
	if values["DEEPSEEK_API_KEY"] != "new-key" || values["DSH_HOME"] != "new-home" {
		t.Fatalf("DSH 凭据未被替换：%#v", values)
	}
	if _, exists := values["DSH_PERMISSION_MODE"]; exists {
		t.Fatal("不应继承用户 DSH 配置")
	}
	if values["DSH_TELEMETRY_MODE"] != "DISABLED" {
		t.Fatalf("受管 runtime 必须禁用 telemetry：%#v", values)
	}
}

func TestRuntimeRegistrySeparatesRobotRoots(t *testing.T) {
	registry := NewRegistry(t.TempDir(), nil)
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := registry.Runtime(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	again, err := registry.Runtime(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.Runtime(secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first != again {
		t.Fatal("相同机器人目录必须复用同一 runtime")
	}
	if first == second || first.dir == second.dir {
		t.Fatal("不同机器人目录不得共享 DSH runtime 或 profile")
	}
}

func TestRuntimeRegistryRestoreUsesPersistedConfigAndResolver(t *testing.T) {
	base := t.TempDir()
	root := t.TempDir()
	factory := func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `read line; echo '{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"deepseek-harness-sdk-runtime"}}}'; sleep 30`)
	}
	first := NewRegistry(base, factory)
	runtime, err := first.Runtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveConfig(Config{Provider: "deepseek-official", Model: "test", Root: root, APIKey: "not-persisted"}); err != nil {
		t.Fatal(err)
	}

	restored := NewRegistry(base, factory)
	failures := restored.Restore(context.Background(), func(gotRoot string) (string, error) {
		if gotRoot != root {
			t.Fatalf("恢复了错误的根目录：%q", gotRoot)
		}
		return "restored-secret", nil
	})
	if len(failures) != 0 {
		t.Fatalf("恢复失败：%v", failures)
	}
	active, err := restored.Runtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if !active.Status().Ready {
		t.Fatal("恢复后 runtime 应就绪")
	}
	if err := restored.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCatalogPersistsAndRejectsUnknownSession(t *testing.T) {
	dir := t.TempDir()
	runtime := New(dir, nil)
	id, err := runtime.CreateSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "alx-") {
		t.Fatalf("会话 ID = %q", id)
	}
	// A new Runtime models a process restart: the catalog remains available
	// without inventing an unsupported DSH session/list RPC.
	restarted := New(dir, nil)
	raw, err := restarted.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), id) {
		t.Fatalf("持久化会话目录 = %s，缺少 %q", raw, id)
	}
	if _, err := restarted.Prompt(context.Background(), "not-owned", "hello"); err == nil {
		t.Fatal("未知会话不得被转发到 DSH")
	}
	if err := restarted.CancelSession(context.Background(), id); !errors.Is(err, ErrSessionCancellationUnsupported) {
		t.Fatalf("取消语义 = %v", err)
	}
}

func TestBridgeTokenIsRuntimeScoped(t *testing.T) {
	registry := NewRegistry(t.TempDir(), nil)
	root := t.TempDir()
	runtime, err := registry.Runtime(root)
	if err != nil {
		t.Fatal(err)
	}
	runtime.bridge = "fresh-token"
	if runtime.AcceptsBridgeToken("wrong-token") {
		t.Fatal("错误 bridge token 不得通过")
	}
	got, gotRoot, ok := registry.RuntimeForBridgeToken("fresh-token")
	if !ok || got != runtime || gotRoot != root {
		t.Fatalf("bridge 路由 = %p %q %t", got, gotRoot, ok)
	}
}

func TestSubscribeAfterReplaysOnlyNewEvents(t *testing.T) {
	runtime := New(t.TempDir(), nil)
	runtime.mu.Lock()
	runtime.history = []Event{{Seq: 1, Method: "old"}, {Seq: 2, Method: "new"}}
	runtime.eventSeq = 2
	runtime.mu.Unlock()
	replay, _, unsubscribe := runtime.SubscribeAfter(1)
	defer unsubscribe()
	if len(replay) != 1 || replay[0].Seq != 2 {
		t.Fatalf("回放 = %#v", replay)
	}
}
