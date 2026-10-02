package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWebLaunchURL(t *testing.T) {
	for _, line := range []string{
		"dsh web: https://127.0.0.1:42/?token=a",
		"dsh web: http://example.com:42/?token=a",
		"dsh web: http://127.0.0.1:0/?token=a",
		"dsh web: http://127.0.0.1:42/",
		"dsh web: opening the default browser",
		"dsh web: http://user@127.0.0.1:42/?token=a",
	} {
		if webLaunchURL(line) != "" {
			t.Fatalf("accepted invalid URL: %s", line)
		}
	}
	if got := webLaunchURL("dsh web: http://127.0.0.1:42/?token=a (LAN: other)"); got != "http://127.0.0.1:42/?token=a" {
		t.Fatal(got)
	}
}

func TestDefaultWebCommand(t *testing.T) {
	t.Setenv("ALX_DSH_BIN", "test-dsh")
	cmd, err := defaultWebCommand(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"test-dsh", "web", "--host", "127.0.0.1", "--port", "0", "--no-open"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("%v", cmd.Args)
	}
}

func helperWebCommand(mode string) CommandFactory {
	return func(ctx context.Context, _ string) *exec.Cmd {
		// StartWeb deliberately replaces Env; pass the test mode through argv.
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestWebRuntimeChild", "--", mode)
	}
}

func TestWebRuntimeChild(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "fail" {
		os.Exit(1)
	}
	if mode == "ready" {
		if os.Getenv("DSH_HOME") == "" || os.Getenv("DEEPSEEK_API_KEY") != "test-key" {
			os.Exit(2)
		}
		fmt.Println("dsh web: http://127.0.0.1:4242/?token=private-launch-token")
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestWebRuntimeLifecycle(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	old := filepath.Join(home, "sessions.json")
	if err := os.WriteFile(old, []byte("old-session-data"), 0600); err != nil {
		t.Fatal(err)
	}
	rt := New(home, nil)
	rt.webCommand = helperWebCommand("ready")
	ctx, cancel := context.WithCancel(context.Background())
	address, err := rt.StartWeb(ctx, root, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop(context.Background())
	cancel()
	again, err := rt.StartWeb(context.Background(), root, "test-key")
	if err != nil || address != again {
		t.Fatalf("must reuse live process: %s %v", again, err)
	}
	other := New(home, nil)
	other.webCommand = helperWebCommand("ready")
	if _, err := other.StartWeb(context.Background(), root, "test-key"); err == nil {
		t.Fatal("second owner accepted")
	}
	if err := rt.Start(context.Background(), Config{Provider: "deepseek-official", Model: "test", Root: root, APIKey: "test-key"}); err == nil {
		t.Fatal("SDK accepted alongside Web")
	}
	if err := rt.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := other.StartWeb(context.Background(), root, "test-key"); err != nil {
		t.Fatal(err)
	}
	defer other.Stop(context.Background())
	raw, err := os.ReadFile(old)
	if err != nil || string(raw) != "old-session-data" {
		t.Fatal("old data changed")
	}
}

func TestWebRuntimeFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"fail", "wait"} {
		t.Run(mode, func(t *testing.T) {
			rt := New(t.TempDir(), nil)
			rt.webCommand = helperWebCommand(mode)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			if _, err := rt.StartWeb(ctx, t.TempDir(), ""); err == nil {
				t.Fatal("expected failure")
			}
			lock, err := acquireRuntimeLock(rt.dir)
			if err != nil {
				t.Fatal("lock was not released", err)
			}
			unlockRuntime(lock)
			rt.mu.Lock()
			defer rt.mu.Unlock()
			if rt.webCmd != nil || strings.Contains(rt.webURL, "token") {
				t.Fatal("process/token retained")
			}
		})
	}
}

func TestWebWorkspacePatchUsesBundledPlugin(t *testing.T) {
	packageRoot := filepath.Join(t.TempDir(), "程序包 'quoted'")
	entry := filepath.Join(packageRoot, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	home := t.TempDir()
	if _, err := writeWebWorkspacePatch(home, entry); err == nil {
		t.Fatal("missing plugin must fail")
	}
	plugin := filepath.Join(packageRoot, "plugins", "workspace-entry", "index.js")
	if err := os.MkdirAll(filepath.Dir(plugin), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plugin, []byte("export {}"), 0600); err != nil {
		t.Fatal(err)
	}
	patch, err := writeWebWorkspacePatch(home, entry)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(patch)
	if err != nil {
		t.Fatal(err)
	}
	quoted := strings.TrimSpace(strings.SplitN(string(data), "name:", 2)[1])
	var resolved string
	if err := json.Unmarshal([]byte(quoted), &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.ToSlash(plugin) {
		t.Fatalf("wrong bundled plugin: %s", resolved)
	}
	if strings.Contains(string(data), "approval-bridge") {
		t.Fatal("Web entry must not enable the SDK bridge")
	}
}
