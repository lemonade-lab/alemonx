package web

import (
	"bytes"
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

func TestGitManagementHTTPStagingCommitAndDiffViews(t *testing.T) {
	root := t.TempDir()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is unavailable")
	}
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = root
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, output, err)
		}
		return strings.TrimSpace(string(output))
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "中文.txt"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "HTTP Test")
	git("config", "user.email", "http@example.invalid")
	write("old\n")
	git("add", ".")
	git("commit", "-m", "initial")
	write("staged\n")
	s := &server{}
	post := func(body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body["root"] = root
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		s.robotGitHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/robot/git", bytes.NewReader(encoded)))
		return response
	}
	response := post(map[string]any{"action": "stage", "paths": []string{"中文.txt"}})
	if response.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", response.Code, response.Body.String())
	}
	write("unstaged\n")
	response = httptest.NewRecorder()
	s.robotGitDiffHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/git/diff?"+url.Values{"root": {root}, "path": {"中文.txt"}, "view": {"staged"}}.Encode(), nil))
	var diff robot.GitDiffResult
	if err := json.Unmarshal(response.Body.Bytes(), &diff); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !strings.Contains(diff.Diff, "+staged") || strings.Contains(diff.Diff, "+unstaged") {
		t.Fatalf("staged HTTP diff: %d %#v", response.Code, diff)
	}
	response = httptest.NewRecorder()
	s.robotGitHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/git?"+url.Values{"root": {root}, "view": {"commit"}}.Encode(), nil))
	var status robot.GitWorkspaceStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.IndexRevision == "" || !status.AtRepositoryRoot {
		t.Fatalf("missing index confirmation data: %#v", status)
	}
	response = post(map[string]any{"action": "commit", "message": "HTTP commit", "indexRevision": status.IndexRevision})
	if response.Code != http.StatusOK || git("show", "HEAD:中文.txt") != "staged" {
		t.Fatalf("commit: %d %s", response.Code, response.Body.String())
	}
	response = post(map[string]any{"action": "stage", "paths": []string{"../outside"}})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "路径无效") {
		t.Fatalf("unsafe path not rejected: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotGitCommitHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/git/commit?"+url.Values{"root": {root}, "sha": {git("rev-parse", "HEAD")}}.Encode(), nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "HTTP commit") {
		t.Fatalf("commit details: %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotGitHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/git?"+url.Values{"root": {root}, "view": {"history"}, "offset": {"-1"}}.Encode(), nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid history offset accepted: %d", response.Code)
	}
}
