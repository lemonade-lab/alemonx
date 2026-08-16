package robot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePM2ProcessesIncludesCWD(t *testing.T) {
	payload := `[{"pm_id":0,"name":"alemonx-bot-1234abcd","status":"online","pid":1,"pm2_env":{"script":"./index.js","namespace":"alemonx","pm_cwd":"/robots/old","pm_uptime":1},"monit":{"memory":1,"cpu":0}}]`
	processes, err := parsePM2Processes(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(processes) != 1 {
		t.Fatalf("processes = %#v", processes)
	}
	if processes[0].CWD != "/robots/old" {
		t.Fatalf("CWD = %q, want /robots/old", processes[0].CWD)
	}
}

func TestPM2StaleRegistration(t *testing.T) {
	processes := []PM2Process{
		{Name: "alemonx-bot-11111111", CWD: "/robots/current"},
		{Name: "alemonx-bot-22222222", CWD: "/robots/old-path"},
	}
	if pm2StaleRegistration(processes, "alemonx-bot-11111111", "/robots/current") {
		t.Error("same cwd must not be stale")
	}
	if !pm2StaleRegistration(processes, "alemonx-bot-22222222", "/robots/current") {
		t.Error("different cwd must be stale")
	}
	if pm2StaleRegistration(processes, "alemonx-bot-33333333", "/robots/current") {
		t.Error("unregistered app must not be stale")
	}
	if pm2StaleRegistration(nil, "alemonx-bot-22222222", "/robots/current") {
		t.Error("empty process list must not be stale")
	}
}

func TestPM2ConfigAppNameExtractsGeneratedName(t *testing.T) {
	root := t.TempDir()
	generated := "module.exports = {\n  apps: [\n    {\n      name: \"alemonx-bot-1234abcd\",\n      namespace: \"alemonx\",\n      cwd: __dirname,\n      script: './index.js'\n    }\n  ]\n};\n"
	if err := os.WriteFile(filepath.Join(root, "pm2.config.cjs"), []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	if name := pm2ConfigAppName(root); name != "alemonx-bot-1234abcd" {
		t.Fatalf("app name = %q, want alemonx-bot-1234abcd", name)
	}
	if name := pm2ConfigAppName(t.TempDir()); name != "" {
		t.Fatalf("missing config should yield empty name, got %q", name)
	}
}

func TestPM2ConfigAppNameToleratesTemplateStyleName(t *testing.T) {
	root := t.TempDir()
	template := "module.exports = {\n  apps: [\n    {\n      name: `alemonx-${project.slice(0, 40)}-${hash}`,\n      namespace: 'alemonx',\n      cwd,\n      script: './index.js'\n    }\n  ]\n};\n"
	if err := os.WriteFile(filepath.Join(root, "pm2.config.cjs"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if name := pm2ConfigAppName(root); name == "" {
		t.Fatal("template-style name should be parsed (even if it cannot match a registration)")
	}
}

func TestStalePM2SameProjectPicksSameCWDAndMovedLeftovers(t *testing.T) {
	current := t.TempDir()
	otherProject := t.TempDir()
	movedAway := filepath.Join(t.TempDir(), "moved-away")
	processes := []PM2Process{
		// Identity rewrite leftover at the same cwd.
		{Name: "alemonx-bot-11111111", Namespace: "alemonx", CWD: current, Status: "stopped"},
		// Running app at the same cwd must survive.
		{Name: "alemonx-bot-22222222", Namespace: "alemonx", CWD: current, Status: "online"},
		// Another existing project with the same readable name must survive.
		{Name: "alemonx-bot-33333333", Namespace: "alemonx", CWD: otherProject, Status: "stopped"},
		// Moved-directory leftover: recorded cwd no longer exists.
		{Name: "alemonx-bot-44444444", Namespace: "alemonx", CWD: movedAway, Status: "stopped"},
		// Different readable name, even with a missing cwd, is not ours.
		{Name: "alemonx-other-99999999", Namespace: "alemonx", CWD: movedAway, Status: "stopped"},
		// Other namespace is never touched.
		{Name: "other-app", Namespace: "default", CWD: current, Status: "stopped"},
	}
	stale := stalePM2SameProject(processes, "alemonx-bot-84673d56", current)
	want := []string{"alemonx-bot-11111111", "alemonx-bot-44444444"}
	if len(stale) != len(want) {
		t.Fatalf("stale = %#v, want %#v", stale, want)
	}
	for index := range want {
		if stale[index] != want[index] {
			t.Fatalf("stale = %#v, want %#v", stale, want)
		}
	}
}

func TestLooksLikeYAMLConfig(t *testing.T) {
	for _, good := range []string{
		"registry=https://registry.npmjs.org\npackage-lock=false\n",
		"//registry.npmjs.org/:_authToken=secret\n",
		"; comment\n# comment\n",
	} {
		if looksLikeYAMLConfig(good) {
			t.Fatalf("valid npmrc flagged as YAML: %q", good)
		}
	}
	for _, bad := range []string{
		"mysql:\n  host: db.example.com\n",
		"qq-bot:\n  app_id: \"123\"\n",
	} {
		if !looksLikeYAMLConfig(bad) {
			t.Fatalf("YAML config not detected: %q", bad)
		}
	}
}
