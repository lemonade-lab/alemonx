package system

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestBrowserInstallPlanForDNFIncludesPuppeteerDependencies(t *testing.T) {
	original := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "chromium" }
	defer func() { rpmBrowserPackageAvailable = original }()

	plan, err := environmentInstallPlan("browser", "dnf")
	if err != nil {
		t.Fatal(err)
	}
	if plan.BrowserPackage != "chromium" {
		t.Fatalf("DNF browser plan browser = %q, want chromium", plan.BrowserPackage)
	}
	packages := map[string]bool{}
	for _, pkg := range plan.Packages {
		packages[pkg] = true
	}
	for _, pkg := range []string{"chromium", "alsa-lib", "gtk3", "mesa-libgbm"} {
		if !packages[pkg] {
			t.Fatalf("DNF browser plan lacks %q: %#v", pkg, plan.Packages)
		}
	}
	if packages["wqy-microhei-fonts"] {
		t.Fatalf("browser plan must not include fonts: %#v", plan.Packages)
	}
}

func TestBrowserInstallPlanForRPMWithoutBrowserPackageSkipsBrowser(t *testing.T) {
	original := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = original }()

	for _, manager := range []string{"dnf", "yum"} {
		plan, err := environmentInstallPlan("browser", manager)
		if err != nil {
			t.Fatal(err)
		}
		if plan.BrowserPackage != "" {
			t.Fatalf("%s browser plan browser = %q, want empty", manager, plan.BrowserPackage)
		}
		for _, pkg := range plan.Packages {
			if pkg == "chromium" {
				t.Fatalf("%s browser plan must not include a browser when repositories lack one: %#v", manager, plan.Packages)
			}
		}
		for _, pkg := range []string{"alsa-lib", "gtk3", "mesa-libgbm"} {
			found := false
			for _, candidate := range plan.Packages {
				if candidate == pkg {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s browser plan lacks dependency %q: %#v", manager, pkg, plan.Packages)
			}
		}
		if packages := map[string]bool{}; true {
			for _, pkg := range plan.Packages {
				packages[pkg] = true
			}
			if packages["wqy-microhei-fonts"] {
				t.Fatalf("%s browser plan must not include fonts: %#v", manager, plan.Packages)
			}
		}
	}
}

func TestBrowserInstallPlanForRPMUsesFirstAvailableBrowser(t *testing.T) {
	original := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "google-chrome-stable" }
	defer func() { rpmBrowserPackageAvailable = original }()

	plan, err := environmentInstallPlan("browser", "dnf")
	if err != nil {
		t.Fatal(err)
	}
	if plan.BrowserPackage != "google-chrome-stable" || len(plan.Packages) == 0 || plan.Packages[0] != "google-chrome-stable" {
		t.Fatalf("DNF browser plan = %#v", plan)
	}
}

func TestRPMBrowserInstallReportsUnavailableBrowserPackage(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if anyBrowserPackage(args) {
			return "Error: Problem installing chromium", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf", BrowserPackage: "chromium"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	// dnf preparation: dnf-plugins-core, CRB, epel-release, update; then
	// makecache, every dependency group and the browser binary.
	wantCalls := 5 + len(browserDependencyPackageCandidates("dnf")) + len(rpmBrowserPackages)
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "Chromium") || !strings.Contains(message, "自带") {
		t.Fatalf("fallback message = %q", message)
	}
}

func TestRPMBrowserInstallSucceedsWithBrowserPackage(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "Installed", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf", BrowserPackage: "google-chrome-stable"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := 5 + len(browserDependencyPackageCandidates("dnf")) + 1
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "google-chrome-stable") {
		t.Fatalf("success message = %q", message)
	}
}

func TestRPMBrowserInstallEnablesEPELWhenBrowserMissing(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	probeCalls := 0
	rpmBrowserPackageAvailable = func(string) string {
		probeCalls++
		return "chromium"
	}
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "chromium") {
		t.Fatalf("browser should be installed after EPEL fallback: %q", message)
	}
	sawEPEL := false
	for _, args := range calls {
		if slices.Contains(args, "epel-release") {
			sawEPEL = true
		}
	}
	if !sawEPEL {
		t.Fatalf("EPEL fallback was never attempted: %#v", calls)
	}
	if probeCalls != 1 {
		t.Fatalf("browser availability should be probed once after EPEL: %d", probeCalls)
	}
}

func TestRPMBrowserInstallNotesFailedEPELFallback(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		if anyBrowserPackage(args) {
			return "Error: No match for browser package", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "EPEL") {
		t.Fatalf("deps-only message should mention the EPEL attempt: %q", message)
	}
}

func TestRPMBrowserInstallOnlyInstallsDepsWithoutBrowserPackage(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if anyBrowserPackage(args) {
			return "Error: No match for browser package", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "yum"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	// makecache + epel-release + makecache + every core candidate group.
	wantCalls := 3 + len(browserDependencyPackageCandidates("yum")) + len(rpmBrowserPackages)
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "未找到") {
		t.Fatalf("deps-only message = %q", message)
	}
}

func TestRPMBrowserInstallPreparesRepositoriesFirst(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	if _, err := installRPMBrowserEnvironment(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(calls) < 4 || !slices.Contains(calls[0], "dnf-plugins-core") {
		t.Fatalf("first call must install dnf-plugins-core, got %#v", calls)
	}
	sawCRB, sawEPEL, sawUpdate := false, false, false
	for _, args := range calls {
		switch {
		case slices.Contains(args, "crb"):
			sawCRB = true
		case slices.Contains(args, "epel-release"):
			sawEPEL = true
		case slices.Contains(args, "update"):
			sawUpdate = true
		}
	}
	if !sawCRB || !sawEPEL || !sawUpdate {
		t.Fatalf("preparation must enable CRB/EPEL and update, got %#v", calls)
	}
}

func TestRPMBrowserInstallTriesAlternativeCandidate(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if slices.Contains(args, "atk") && !slices.Contains(args, "at-spi2-atk") && !slices.Contains(args, "at-spi2-core") {
			return "Error: No match for argument: atk", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message, "未能安装") {
		t.Fatalf("alternative candidate should have succeeded: %q", message)
	}
	seenAlternative := false
	for _, args := range calls {
		if slices.Contains(args, "at-spi2-atk") {
			seenAlternative = true
		}
	}
	if !seenAlternative {
		t.Fatalf("alternative candidate was never attempted: %#v", calls)
	}
}

func TestRPMBrowserInstallSkipsUnavailableDependency(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if slices.Contains(args, "alsa-lib") {
			return "Error: No match for argument: alsa-lib", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatalf("partial failure should not abort: %v", err)
	}
	if !strings.Contains(message, "alsa-lib") || !strings.Contains(message, "未能安装") {
		t.Fatalf("partial failure message = %q", message)
	}
	// Every dnf invocation installs exactly one package.
	for _, args := range calls {
		if len(args) != 4 || args[0] != "install" {
			continue
		}
		if args[1] != "-y" || args[2] != "--allowerasing" {
			t.Fatalf("per-package install args = %#v", args)
		}
	}
}

func TestRPMBrowserInstallFailsWhenNothingInstalls(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "Error: Could not find package", errors.New("exit status 1")
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	_, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "均未安装") {
		t.Fatalf("all-failed error = %v", err)
	}
}

func TestBrowserInstallPlanForAPTUsesCandidates(t *testing.T) {
	plan, err := environmentInstallPlan("browser", "apt-get")
	if err != nil {
		t.Fatal(err)
	}
	packages := map[string]bool{}
	for _, pkg := range plan.Packages {
		packages[pkg] = true
	}
	for _, pkg := range []string{"chromium", "chromium-browser", "libnss3", "libgbm1", "libxkbcommon0"} {
		if !packages[pkg] {
			t.Fatalf("apt browser plan lacks %q: %#v", pkg, plan.Packages)
		}
	}
	if packages["wqy-microhei-fonts"] {
		t.Fatalf("browser plan must not include fonts: %#v", plan.Packages)
	}
}

func TestLinuxBrowserInstallInstallsBrowserCandidates(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "Installed", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "apt-get"}
	message, err := installLinuxBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "chromium") {
		t.Fatalf("browser install message = %q", message)
	}
	sawChromium := false
	for _, args := range calls {
		if slices.Contains(args, "chromium") && !slices.Contains(args, "chromium-browser") {
			sawChromium = true
		}
	}
	if !sawChromium {
		t.Fatalf("chromium was never attempted: %#v", calls)
	}
}

func TestLinuxBrowserInstallTriesAlternativeBrowserCandidate(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		if slices.Contains(args, "chromium") && !slices.Contains(args, "chromium-browser") {
			return "Error: No match for argument: chromium", errors.New("exit status 1")
		}
		return "Installed", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "apt-get"}
	message, err := installLinuxBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "chromium-browser") {
		t.Fatalf("alternative browser candidate should be reported: %q", message)
	}
	if strings.Contains(message, "未能安装") {
		t.Fatalf("alternative candidate should have succeeded: %q", message)
	}
}

func TestLinuxBrowserInstallSkipsUnavailableDependency(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if slices.Contains(args, "libnss3") || slices.Contains(args, "libnss3t64") {
			return "Error: No match for argument: libnss3", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "apt-get"}
	message, err := installLinuxBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatalf("partial failure should not abort: %v", err)
	}
	if !strings.Contains(message, "libnss3") || !strings.Contains(message, "未能安装") {
		t.Fatalf("partial failure message = %q", message)
	}
	// Every apt invocation installs exactly one package.
	for _, args := range calls {
		if len(args) != 3 || args[0] != "install" {
			continue
		}
		if args[1] != "-y" {
			t.Fatalf("per-package install args = %#v", args)
		}
	}
}

func TestLinuxBrowserInstallFallsBackToDepsOnly(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		for _, arg := range args {
			if arg == "chromium" || arg == "chromium-browser" {
				return "Error: No match for argument: chromium", errors.New("exit status 1")
			}
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "apt-get"}
	message, err := installLinuxBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "自带") {
		t.Fatalf("deps-only fallback message = %q", message)
	}
}

func TestLinuxBrowserInstallFailsWhenNothingInstalls(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "Error: Could not find package", errors.New("exit status 1")
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "apt-get"}
	_, err := installLinuxBrowserEnvironment(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "均未安装") {
		t.Fatalf("all-failed error = %v", err)
	}
}

func TestRPMBrowserInstallTimeoutIncludesPartialProgress(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	ctx, cancel := context.WithCancel(context.Background())
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		for _, arg := range args {
			if arg == "atk" || arg == "at-spi2-atk" || arg == "at-spi2-core" {
				return "Error: No match for argument: atk", errors.New("exit status 1")
			}
		}
		if slices.Contains(args, "gtk3") {
			cancel()
			return "", errors.New("context canceled")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	_, err := installRPMBrowserEnvironment(ctx, plan)
	if err == nil || !strings.Contains(err.Error(), "服务器安装超时") || !strings.Contains(err.Error(), "atk") {
		t.Fatalf("timeout error should keep partial progress: %v", err)
	}
}

func TestEnableCRBRepositoryFallsBackToCrbHelper(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if slices.Contains(args, "config-manager") {
			return "Error: Unknown command", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	enableCRBRepository(context.Background(), "dnf")
	if len(calls) != 3 ||
		!slices.Contains(calls[0], "dnf-plugins-core") ||
		!slices.Contains(calls[1], "crb") ||
		!slices.Contains(calls[2], "enable") {
		t.Fatalf("CRB enable calls = %#v", calls)
	}
}

func TestFontInstallPlanPerManager(t *testing.T) {
	for _, test := range []struct {
		manager string
		want    []string
	}{
		{"apt-get", []string{"fonts-noto-cjk", "fonts-noto-color-emoji"}},
		{"dnf", []string{"google-noto-serif-cjk-fonts", "google-noto-emoji-fonts"}},
		{"yum", []string{"google-noto-serif-cjk-fonts", "google-noto-emoji-fonts"}},
		{"pacman", []string{"noto-fonts-cjk", "noto-fonts-emoji"}},
		{"apk", []string{"font-noto-cjk", "font-noto-emoji"}},
	} {
		plan, err := environmentInstallPlan("fonts", test.manager)
		if err != nil {
			t.Fatalf("%s fonts plan: %v", test.manager, err)
		}
		if !slices.Equal(plan.Packages, test.want) {
			t.Fatalf("%s fonts plan = %#v, want %#v", test.manager, plan.Packages, test.want)
		}
	}
	if _, err := environmentInstallPlan("fonts", "brew"); err == nil {
		t.Fatal("fonts install on brew must be rejected")
	}
}

func TestFontsInstallSucceeds(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "fonts", Name: "系统字体（CJK/Emoji）", PackageManager: "apt-get"}
	message, err := installFontsEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "已安装系统字体") {
		t.Fatalf("fonts success message = %q", message)
	}
}

func TestFontsInstallSkipsMissingPackage(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		if slices.Contains(args, "fonts-noto-cjk") {
			return "Error: No match for argument: fonts-noto-cjk", errors.New("exit status 1")
		}
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "fonts", Name: "系统字体（CJK/Emoji）", PackageManager: "apt-get"}
	message, err := installFontsEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatalf("partial fonts failure should not abort: %v", err)
	}
	if !strings.Contains(message, "fonts-noto-cjk") || !strings.Contains(message, "未找到") {
		t.Fatalf("fonts partial message = %q", message)
	}
}

func TestPrepareRPMRepositoriesEnablesEPELAndUpdates(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	// dnf additionally enables CRB before EPEL.
	if !prepareRPMRepositories(context.Background(), "dnf") {
		t.Fatal("RPM preparation should succeed")
	}
	if len(calls) != 4 ||
		!slices.Contains(calls[0], "dnf-plugins-core") ||
		!slices.Contains(calls[1], "crb") ||
		!slices.Contains(calls[2], "epel-release") ||
		!slices.Contains(calls[3], "update") {
		t.Fatalf("RPM preparation calls = %#v", calls)
	}
	calls = nil
	if !prepareRPMRepositories(context.Background(), "yum") {
		t.Fatal("RPM preparation should succeed")
	}
	if len(calls) != 2 || !slices.Contains(calls[0], "epel-release") || !slices.Contains(calls[1], "update") {
		t.Fatalf("yum preparation calls = %#v", calls)
	}
	if prepareRPMRepositories(context.Background(), "apt-get") {
		t.Fatal("prepareRPMRepositories must be a no-op for non-RPM managers")
	}
}

func TestPackageManagerPreconditionRefreshesMetadata(t *testing.T) {
	for _, test := range []struct {
		manager string
		want    string
	}{
		{"apt-get", "update"},
		{"pacman", "-Sy"},
		{"apk", "update"},
		{"brew", "update"},
	} {
		originalRun := runPackageCommand
		var got []string
		runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
			got = args
			return "", nil
		}
		if !packageManagerPrecondition(context.Background(), test.manager) {
			t.Fatalf("%s precondition failed", test.manager)
		}
		runPackageCommand = originalRun
		if len(got) != 1 || got[0] != test.want {
			t.Fatalf("%s precondition args = %#v, want %q", test.manager, got, test.want)
		}
	}
	if packageManagerPrecondition(context.Background(), "dnf") {
		t.Fatal("dnf precondition must be handled by prepareRPMRepositories")
	}
}

func TestRPMBrowserInstallAbortsOnSudoFailure(t *testing.T) {
	originalProbe := rpmBrowserPackageAvailable
	rpmBrowserPackageAvailable = func(string) string { return "" }
	defer func() { rpmBrowserPackageAvailable = originalProbe }()
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "sudo: a password is required", errors.New("exit status 1")
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf"}
	_, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "管理员授权") {
		t.Fatalf("sudo failure error = %v", err)
	}
}

func TestBrowserInstallPlanForWindowsUsesChrome(t *testing.T) {
	plan, err := environmentInstallPlan("browser", "winget")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Packages) != 1 || plan.Packages[0] != "Google.Chrome" {
		t.Fatalf("Windows browser plan = %#v", plan)
	}
}

func TestBrowserInstallPlanForFreeBSDUsesPkgChromium(t *testing.T) {
	plan, err := environmentInstallPlan("browser", "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Packages) != 1 || plan.Packages[0] != "chromium" {
		t.Fatalf("FreeBSD browser plan = %#v", plan)
	}
}

func anyBrowserPackage(args []string) bool {
	for _, candidate := range rpmBrowserPackages {
		if slices.Contains(args, candidate) {
			return true
		}
	}
	return false
}
