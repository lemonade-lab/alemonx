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
	for _, pkg := range []string{"chromium", "alsa-lib", "gtk3", "mesa-libgbm", "wqy-microhei-fonts"} {
		if !packages[pkg] {
			t.Fatalf("DNF browser plan lacks %q: %#v", pkg, plan.Packages)
		}
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
		for _, pkg := range []string{"alsa-lib", "gtk3", "mesa-libgbm", "wqy-microhei-fonts"} {
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

func TestRPMBrowserInstallFallsBackWhenBrowserPackageFails(t *testing.T) {
	originalRun := runPackageCommand
	var calls [][]string
	runPackageCommand = func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		if len(calls) == 2 {
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
	if len(calls) != 2 {
		t.Fatalf("install calls = %d, want 2: %#v", len(calls), calls)
	}
	if !slices.Contains(calls[0], "alsa-lib") || slices.Contains(calls[0], "chromium") {
		t.Fatalf("dependency install args = %#v", calls[0])
	}
	if !slices.Contains(calls[1], "chromium") {
		t.Fatalf("browser install args = %#v", calls[1])
	}
	if !strings.Contains(message, "运行库") || !strings.Contains(message, "自带") {
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
	if len(calls) != 2 {
		t.Fatalf("install calls = %d, want 2: %#v", len(calls), calls)
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
	if len(calls) != 1 {
		t.Fatalf("install calls = %d, want 1: %#v", len(calls), calls)
	}
	if !strings.Contains(message, "未提供") {
		t.Fatalf("deps-only message = %q", message)
	}
}

func TestRPMBrowserInstallFailsWhenDependenciesFail(t *testing.T) {
	originalRun := runPackageCommand
	runPackageCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "Error: Could not find package", errors.New("exit status 1")
	}
	defer func() { runPackageCommand = originalRun }()

	plan := EnvironmentInstallPlan{CheckID: "browser", Name: "浏览器及依赖包", PackageManager: "dnf", BrowserPackage: "chromium"}
	_, err := installRPMBrowserEnvironment(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "失败") {
		t.Fatalf("dependency failure error = %v", err)
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
