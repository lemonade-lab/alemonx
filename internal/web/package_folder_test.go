package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackpackFolderInstall(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"robot"}`), 0644); err != nil {
		t.Fatal(err)
	}
	request := func(files map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for path, content := range files {
			part, err := writer.CreateFormFile("files:"+path, filepath.Base(path))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/robot/packages/folder?"+url.Values{"root": {root}}.Encode(), &body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		response := httptest.NewRecorder()
		(&server{}).robotPackageFolderHandler(response, r)
		return response
	}
	for _, invalid := range []string{"../escape", "/absolute", "C:/windows", "sub\\escape", "nested/../../escape", ".env", "node_modules/file"} {
		response := request(map[string]string{"package.json": `{"name":"invalid"}`, invalid: "bad"})
		if response.Code != 400 {
			t.Fatalf("accepted path %q: %d %s", invalid, response.Code, response.Body)
		}
	}
	if r := request(map[string]string{"index.js": "test"}); r.Code != 400 {
		t.Fatalf("missing manifest: %d", r.Code)
	}
	if r := request(map[string]string{"package.json": "invalid"}); r.Code != 400 {
		t.Fatalf("invalid manifest: %d", r.Code)
	}
	files := map[string]string{"package.json": `{"name":"folder-plugin","version":"1.0.0"}`, "lib/main.js": "original"}
	if r := request(files); r.Code != 200 {
		t.Fatalf("install failed: %d %s", r.Code, r.Body)
	}
	file := filepath.Join(root, "packages", "folder-plugin", "lib", "main.js")
	if data, err := os.ReadFile(file); err != nil || string(data) != "original" {
		t.Fatalf("nested file: %s %v", data, err)
	}
	files["lib/main.js"] = "replacement"
	if r := request(files); r.Code != 400 || !strings.Contains(r.Body.String(), "已存在") {
		t.Fatalf("expected conflict: %d %s", r.Code, r.Body)
	}
	if data, _ := os.ReadFile(file); string(data) != "original" {
		t.Fatal("existing package overwritten")
	}
}
