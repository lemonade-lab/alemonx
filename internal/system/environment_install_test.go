package system

import "testing"

func TestBrowserInstallPlanForDNFIncludesPuppeteerDependencies(t *testing.T) {
	plan, err := environmentInstallPlan("browser", "dnf")
	if err != nil {
		t.Fatal(err)
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

func TestBrowserInstallPlanForWindowsUsesChrome(t *testing.T) {
	plan, err := environmentInstallPlan("browser", "winget")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Packages) != 1 || plan.Packages[0] != "Google.Chrome" {
		t.Fatalf("Windows browser plan = %#v", plan)
	}
}
