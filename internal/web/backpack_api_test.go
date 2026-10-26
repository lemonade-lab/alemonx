package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"alemonx/internal/robot"
)

func TestBackpackListAndStatusExposeInstalledVersion(t *testing.T) {
	root := t.TempDir()
	plugin := filepath.Join(root, "packages", "fixture-plugin")
	if err := os.MkdirAll(plugin, 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "package.json"):       `{"name":"fixture","private":true,"workspaces":["packages/*"]}`,
		filepath.Join(root, "alemon.config.yaml"): "apps:\n  - fixture-plugin\n",
		filepath.Join(plugin, "package.json"):     `{"name":"fixture-plugin","version":"1.2.3","main":"index.js","alemonjs":{}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = plugin
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	git("init", "-b", "main")
	git("config", "user.email", "fixture@example.invalid")
	git("config", "user.name", "Fixture")
	git("add", "package.json")
	git("commit", "-m", "fixture")
	head := git("rev-parse", "--short", "HEAD")
	s := &server{robots: robot.Manager{}}
	query := "?" + url.Values{"root": {root}}.Encode()
	recorder := httptest.NewRecorder()
	s.robotPackagesHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/robot/packages"+query, nil))
	var packages struct {
		Items []robot.LocalPackage `json:"items"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &packages) != nil || len(packages.Items) != 1 || packages.Items[0].Version != "1.2.3" {
		t.Fatalf("list=%d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	s.robotPackageStatusHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/robot/packages/status"+query, nil))
	var statuses struct {
		Items []robot.LocalPackageStatus `json:"items"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &statuses) != nil || len(statuses.Items) != 1 || statuses.Items[0].Current != head || statuses.Items[0].Branch != "main" {
		t.Fatalf("status=%d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	s.robotPackageInventoryHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/robot/package-inventory"+query, nil))
	var inventory struct {
		Items []robot.PackageFact `json:"items"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &inventory) != nil || len(inventory.Items) != 1 || inventory.Items[0].Version != "1.2.3" {
		t.Fatalf("inventory=%d %s", recorder.Code, recorder.Body.String())
	}
}
