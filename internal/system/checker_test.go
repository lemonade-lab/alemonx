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
		if got := nodeVersionAtLeast(test.version, MinimumNodeVersion); got != test.want {
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

func TestEnvironmentReportSeparatesBrowserAndCommonDependencies(t *testing.T) {
	report := NewChecker().CheckGoal("install", "")
	ids := map[string]bool{}
	for _, check := range report.Checks {
		ids[check.ID] = true
	}
	for _, id := range []string{
		"node",
		"git",
		"fonts",
		"browser",
		"browser-dependencies",
		"common-dependencies",
	} {
		if !ids[id] {
			t.Fatalf("environment checks lack %q: %#v", id, report.Checks)
		}
	}
}

func TestEnvironmentReportAlwaysIncludesSixCoreChecks(t *testing.T) {
	for _, test := range []struct {
		goal, variant string
		nodeOptional  bool
		gitOptional   bool
	}{
		{"install", "", false, false},
		{"develop", "", false, false},
		{"web", "clean", false, false},
		{"web", "docker", true, true},
		{"mobile", "", true, true},
		{"build", "git", false, false},
		{"build", "npm", false, true},
	} {
		report := NewChecker().CheckGoal(test.goal, test.variant)
		checks := map[string]Check{}
		for _, check := range report.Checks {
			checks[check.ID] = check
		}
		for _, id := range []string{"node", "git", "fonts", "browser", "browser-dependencies", "common-dependencies"} {
			if _, ok := checks[id]; !ok {
				t.Fatalf("%s/%s lacks %s: %#v", test.goal, test.variant, id, report.Checks)
			}
		}
		if checks["node"].Optional != test.nodeOptional || checks["git"].Optional != test.gitOptional {
			t.Fatalf("%s/%s optional node=%t git=%t", test.goal, test.variant, checks["node"].Optional, checks["git"].Optional)
		}
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
