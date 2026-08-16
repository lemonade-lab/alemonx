package system

import "testing"

func TestNodeVersionAtLeast(t *testing.T) {
	for _, test := range []struct {
		version string
		want    bool
	}{
		{"v22.22.2", false},
		{"v22.22.3", true},
		{"v22.23.0", true},
		{"v23.0.0", true},
		{"v22.22.3-pre", false},
		{"not-a-version", false},
	} {
		if got := nodeVersionAtLeast(test.version, minimumNodeVersion); got != test.want {
			t.Errorf("nodeVersionAtLeast(%q) = %t, want %t", test.version, got, test.want)
		}
	}
}

func TestOutdatedNodeDoesNotBlockEnvironment(t *testing.T) {
	checks := []Check{{ID: "node", Status: "outdated"}, {ID: "git", Status: "ready"}}
	if !checksAreUsable(checks) {
		t.Fatal("outdated Node.js should be non-blocking")
	}
	checks[1].Status = "missing"
	if checksAreUsable(checks) {
		t.Fatal("missing prerequisite should remain blocking")
	}
}

func TestOptionalBrowserDoesNotBlockEnvironment(t *testing.T) {
	checks := []Check{{ID: "browser", Status: "missing", Optional: true}, {ID: "node", Status: "ready"}}
	if !checksAreUsable(checks) {
		t.Fatal("optional browser should not block the environment")
	}
}

func TestOptionalFontsDoesNotBlockEnvironment(t *testing.T) {
	checks := []Check{{ID: "fonts", Status: "missing", Optional: true}, {ID: "node", Status: "ready"}}
	if !checksAreUsable(checks) {
		t.Fatal("optional fonts should not block the environment")
	}
}

func TestGitBuildCheckDoesNotRequireGlobalPackageTools(t *testing.T) {
	report := NewChecker().CheckGoal("build", "git")
	ids := map[string]bool{}
	for _, check := range report.Checks {
		ids[check.ID] = true
	}
	if !ids["node"] || !ids["git"] {
		t.Fatalf("git build checks = %#v, want node and git", report.Checks)
	}
	for _, id := range []string{"yarn", "pnpm", "jq"} {
		if ids[id] {
			t.Fatalf("git build should not require global %s: %#v", id, report.Checks)
		}
	}
}

func TestWindowsRegistryPathValue(t *testing.T) {
	output := "\r\nHKEY_CURRENT_USER\\Environment\r\n    Path    REG_EXPAND_SZ    %LOCALAPPDATA%\\Programs\\nodejs;C:\\Tools\\Git\\cmd\r\n"
	got := windowsRegistryPathValue(output)
	want := "%LOCALAPPDATA%\\Programs\\nodejs;C:\\Tools\\Git\\cmd"
	if got != want {
		t.Fatalf("windowsRegistryPathValue() = %q, want %q", got, want)
	}
}

func TestExpandWindowsEnvironment(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\\Users\\alemon\\AppData\\Local`)
	got := expandWindowsEnvironment(`%LOCALAPPDATA%\\Programs\\nodejs`)
	want := `C:\\Users\\alemon\\AppData\\Local\\Programs\\nodejs`
	if got != want {
		t.Fatalf("expandWindowsEnvironment() = %q, want %q", got, want)
	}
}
