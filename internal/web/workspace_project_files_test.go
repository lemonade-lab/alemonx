package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alemonx/internal/access"
	"alemonx/internal/workspace"
)

func TestWorkspaceProjectFilesAreScopedToAccount(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "robot")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"package.json": "{}", "index.ts": "export {}", ".env": "PRIVATE=fixture", "binary.dat": "\x00",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"src", "assets"} {
		if err := os.Mkdir(filepath.Join(project, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := access.NewAt(filepath.Join(root, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	password := "test-resource-explorer-password"
	owner, err := identity.Enable("owner", password, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.CreateAccount("other", password, password); err != nil {
		t.Fatal(err)
	}
	other, err := identity.Login("other", password)
	if err != nil {
		t.Fatal(err)
	}
	status, err := identity.Status(owner)
	if err != nil {
		t.Fatal(err)
	}
	list, err := workspace.LoadRobots(root, status.RobotScope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.UpdateRobots(root, status.RobotScope, workspace.RobotListChange{
		Scope: list.Scope, Action: "add", Projects: []workspace.RobotEntry{{Path: project}},
	}); err != nil {
		t.Fatal(err)
	}

	s := &server{auth: identity, workspaceRequested: root}
	call := func(method, token, filename string, body []byte, code int) string {
		t.Helper()
		query := url.Values{"root": {project}}
		if filename != "" {
			query.Set("file", filename)
		}
		request := httptest.NewRequest(method, "/api/v1/workspace/project-files?"+query.Encode(), bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		s.workspaceProjectFilesHandler(response, request)
		if response.Code != code {
			t.Fatalf("%v returned %d instead of %d: %s", query, response.Code, code, response.Body.String())
		}
		return response.Body.String()
	}

	get := func(token, filename string, code int) string {
		t.Helper()
		return call(http.MethodGet, token, filename, nil, code)
	}
	save := func(token, filename, revision, content string, code int) {
		t.Helper()
		body, err := json.Marshal(map[string]string{"revision": revision, "content": content})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodPut, token, filename, body, code)
	}

	var listing struct {
		Entries []struct{ Name string }
	}
	if err := json.Unmarshal([]byte(get(owner, "", http.StatusOK)), &listing); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		names = append(names, entry.Name)
	}
	want := []string{"assets", "src", "binary.dat", "index.ts", "package.json"}
	if !slices.Equal(names, want) {
		t.Fatalf("project entry order = %v, want %v", names, want)
	}
	var preview struct{ Content, Revision string }
	if err := json.Unmarshal([]byte(get(owner, "index.ts", http.StatusOK)), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Content != "export {}" || preview.Revision == "" {
		t.Fatalf("unexpected workspace document: %+v", preview)
	}
	save(other, "index.ts", preview.Revision, "blocked", http.StatusForbidden)
	save(owner, "index.ts", "", "missing revision", http.StatusBadRequest)
	save(owner, "binary.dat", preview.Revision, "blocked", http.StatusUnsupportedMediaType)
	save(owner, "src/missing.ts", preview.Revision, "blocked", http.StatusBadRequest)
	save(owner, "index.ts", preview.Revision, "export const value = 1", http.StatusOK)
	save(owner, "index.ts", preview.Revision, "stale draft", http.StatusConflict)
	if content := get(owner, "index.ts", http.StatusOK); !strings.Contains(content, "export const value = 1") {
		t.Fatalf("revision conflict overwrote the saved file: %s", content)
	}
	save(owner, "index.ts", preview.Revision, "export const value = 1", http.StatusOK)

	get(other, "index.ts", http.StatusForbidden)
	get(owner, "../auth.json", http.StatusBadRequest)
	get(owner, ".env", http.StatusBadRequest)
	get(owner, "binary.dat", http.StatusUnsupportedMediaType)
	create := func(token, name string, directory bool, code int) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"name": name, "directory": directory})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodPost, token, "", body, code)
	}
	create(other, "new.ts", false, http.StatusForbidden)
	create(owner, "../outside.ts", false, http.StatusBadRequest)
	create(owner, ".env", false, http.StatusBadRequest)
	create(owner, "index.ts", false, http.StatusConflict)
	create(owner, "index.ts", true, http.StatusConflict)
	create(owner, "new.ts", false, http.StatusCreated)
	create(owner, "new-dir", true, http.StatusCreated)
	if content, err := os.ReadFile(filepath.Join(project, "new.ts")); err != nil || len(content) != 0 {
		t.Fatalf("new file contents = %q, error = %v", content, err)
	}
	if info, err := os.Stat(filepath.Join(project, "new-dir")); err != nil || !info.IsDir() {
		t.Fatalf("new directory info = %v, error = %v", info, err)
	}

	if err := os.Symlink(filepath.Join(root, "auth.json"), filepath.Join(project, "linked.ts")); err == nil {
		get(owner, "linked.ts", http.StatusBadRequest)
	}
}
