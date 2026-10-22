package robot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackageDiagnosticsRetainsStageAcrossWarningTail(t *testing.T) {
	started := time.Now().Add(-time.Minute)
	output := &packageCommandOutput{started: started}
	for _, chunk := range []string{"yarn install v1.22.", "22\n\x1b[32m[3/4] Linking dep", "endencies...\x1b[0m\r\n", strings.Repeat("warning peer dependency\n", 1000)} {
		_, _ = output.Write([]byte(chunk))
	}
	cmd := exec.Command("node", "/tools/yarn.js", "install")
	cmd.Env = []string{"npm_config_registry=https://user:secret@registry.example/private-token?auth=secret", "HTTPS_PROXY=http://user:proxy-secret@127.0.0.1:1234"}
	summary := output.summary(cmd, time.Now())
	for _, want := range []string{"Yarn 版本：1.22.22", "[3/4] Linking dependencies", "https://registry.example", "已启用受管代理通道", "距最近输出："} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %q in %s", want, summary)
		}
	}
	for _, secret := range []string{"secret", "private-token", "user:", "1234"} {
		if strings.Contains(summary, secret) {
			t.Fatal("diagnostic exposed credentials or private registry path")
		}
	}
	err := buildDependencyError("安装失败", output.String()+"\n"+summary, packageTimeoutError(20*time.Minute, output.stage))
	if !strings.Contains(err.Error(), "[3/4] Linking dependencies") || !strings.Contains(err.Error(), "文件占用") {
		t.Fatal("warning-heavy error lost the stage or timeout advice")
	}
}

func TestPackageCommandFailureIncludesDiagnostics(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	root := t.TempDir()
	script := filepath.Join(root, "yarn.js")
	if err := os.WriteFile(script, []byte(`console.error('yarn install v1.22.22\n[2/4] Fetching packages...'); process.exit(Number(process.env.ALX_TEST_EXIT));`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"0", "1"} {
		// A fixture auth variable prevents automatic mirror probing; no real
		// credentials or network calls are needed by this fake package CLI.
		output, runErr := runWithEnv(root, map[string]string{"ALX_TEST_EXIT": code, "ALX_TEST_AUTH_TOKEN": "fixture"}, node, script, "install")
		if (runErr != nil) != (code == "1") {
			t.Fatalf("unexpected execution result: %v", runErr)
		}
		if strings.Contains(output, "依赖操作诊断") != (code == "1") {
			t.Fatalf("diagnostics must be added only on failure: %s", output)
		}
		if code == "1" && (!strings.Contains(output, "最后识别阶段：[2/4] Fetching packages") || !strings.Contains(output, "Yarn 版本：1.22.22")) {
			t.Fatalf("subprocess pipe bypassed progress tracking: %s", output)
		}
		if !strings.Contains(output, "[2/4] Fetching packages") {
			t.Fatal("command output was lost")
		}
	}
}

func TestPackageTimeoutAdviceUsesObservedStage(t *testing.T) {
	for _, item := range []struct{ stage, want string }{
		{"", "不能确定是网络或代理故障"},
		{"[1/4] Resolving packages", "依赖解析或下载"},
		{"[2/4] Fetching packages", "依赖解析或下载"},
		{"[3/4] Linking dependencies", "文件占用"},
		{"[4/4] Building fresh packages", "原生模块构建"},
	} {
		if err := packageTimeoutError(20*time.Minute, item.stage); !strings.Contains(err.Error(), item.want) {
			t.Fatalf("stage %q: %v", item.stage, err)
		}
	}
}
