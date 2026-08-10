package system

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestConfigurePrivilegedModeRejectsNonLoopbackLocal(t *testing.T) {
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := ConfigurePrivilegedMode("0.0.0.0", false); err == nil {
		t.Fatal("non-loopback local privilege mode must be rejected")
	}
}

func TestPluginPrivilegeBindsExactInstalledRunner(t *testing.T) {
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { t.Setenv("ALX_PRIVILEGED_MODE", "disabled"); _ = ConfigurePrivilegedMode("127.0.0.1", false) })
	runner := t.TempDir() + "/runner"
	if err := os.WriteFile(runner, []byte("reviewed"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(runner)
	if err != nil {
		t.Fatal(err)
	}
	privilegeRuntime.Lock()
	previous := privilegeRuntime.policy
	privilegeRuntime.policy = privilegePolicyFile{Version: "test", Operations: []privilegeOperation{{PluginID: "alemonx-network", Action: "apply-plan", Platform: runtime.GOOS + "-" + runtime.GOARCH, Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerSHA256: digest, Prompt: "native"}}}
	privilegeRuntime.Unlock()
	t.Cleanup(func() { privilegeRuntime.Lock(); privilegeRuntime.policy = previous; privilegeRuntime.Unlock() })
	identity := PluginPrivilegeIdentity{PluginID: "alemonx-network", Action: "apply-plan", Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerPath: runner, DeclaredActions: []string{"apply-plan"}}
	if err := AuthorizePluginPrivilege(identity); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runner, []byte("changed"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizePluginPrivilege(identity); err == nil {
		t.Fatal("changed runner must not retain privilege")
	}
}

func TestPluginPrivilegeRejectsManifestActionMismatch(t *testing.T) {
	runner := t.TempDir() + "/runner"
	if err := os.WriteFile(runner, []byte("reviewed"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := fileSHA256(runner)
	if err != nil {
		t.Fatal(err)
	}
	privilegeRuntime.Lock()
	previous := privilegeRuntime.policy
	privilegeRuntime.policy = privilegePolicyFile{Version: "test", Operations: []privilegeOperation{
		{PluginID: "alemonx-network", Action: "apply-plan", Platform: runtime.GOOS + "-" + runtime.GOARCH, Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerSHA256: digest},
		{PluginID: "alemonx-network", Action: "undo-last", Platform: runtime.GOOS + "-" + runtime.GOARCH, Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerSHA256: digest},
	}}
	privilegeRuntime.Unlock()
	t.Cleanup(func() { privilegeRuntime.Lock(); privilegeRuntime.policy = previous; privilegeRuntime.Unlock() })
	identity := PluginPrivilegeIdentity{PluginID: "alemonx-network", Action: "apply-plan", Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerPath: runner, DeclaredActions: []string{"apply-plan"}}
	if err := CheckPluginPrivilege(identity); err == nil {
		t.Fatal("partial manifest declaration must not receive privilege")
	}
}

func TestPrivilegePolicyRejectsIncompleteOrDuplicateEntries(t *testing.T) {
	if err := validatePrivilegePolicy(privilegePolicyFile{Version: "1", Operations: []privilegeOperation{{PluginID: "network"}}}); err == nil {
		t.Fatal("incomplete policy entry must be rejected")
	}
	entry := privilegeOperation{PluginID: "network", Action: "apply", Platform: "linux-amd64", Tag: "v1.0.0", Asset: "network.zip", ArchiveSHA256: strings.Repeat("a", 64), RunnerSHA256: strings.Repeat("b", 64)}
	if err := validatePrivilegePolicy(privilegePolicyFile{Version: "1", Operations: []privilegeOperation{entry, entry}}); err == nil {
		t.Fatal("duplicate policy entry must be rejected")
	}
}
