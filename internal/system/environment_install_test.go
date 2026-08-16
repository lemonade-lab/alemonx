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
		if slices.Contains(args, "chromium") {
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
	wantCalls := 1 + len(browserDependencyPackageCandidates()) + 1
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "chromium") || !strings.Contains(message, "自带") {
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
	wantCalls := 1 + len(browserDependencyPackageCandidates()) + 1
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "google-chrome-stable") {
		t.Fatalf("success message = %q", message)
	}
}

func TestRPMBrowserInstallOnlyInstallsDepsWithoutBrowserPackage(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "yum"}
	message, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := 1 + len(browserDependencyPackageCandidates())
	if len(calls) != wantCalls {
		t.Fatalf("install calls = %d, want %d: %#v", len(calls), wantCalls, calls)
	}
	if !strings.Contains(message, "未提供") {
		t.Fatalf("deps-only message = %q", message)
	}
}

func TestRPMBrowserInstallRunsMakecacheFirst(t *testing.T) {
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
	if len(calls) == 0 || len(calls[0]) != 1 || calls[0][0] != "makecache" {
		t.Fatalf("first call must be makecache, got %#v", calls)
	}
}

func TestRPMBrowserInstallTriesAlternativeCandidate(t *testing.T) {
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
		if args[0] == "makecache" {
			continue
		}
		if len(args) != 4 || args[0] != "install" || args[1] != "-y" || args[2] != "--allowerasing" {
			t.Fatalf("per-package install args = %#v", args)
		}
	}
}

func TestRPMBrowserInstallFailsWhenNothingInstalls(t *testing.T) {
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

func TestFontInstallPlanPerManager(t *testing.T) {
	for _, test := range []struct {
		manager string
		want    []string
	}{
		{"apt-get", []string{"fonts-noto-cjk", "fonts-noto-color-emoji"}},
		{"dnf", []string{"google-noto-serif-cjk-fonts"}},
		{"yum", []string{"google-noto-serif-cjk-fonts"}},
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

func TestRPMBrowserInstallAbortsOnSudoFailure(t *testing.T) {
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
