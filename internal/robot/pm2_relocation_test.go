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
