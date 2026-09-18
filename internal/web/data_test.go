package web

import (
	"alemonx/internal/access"
	"alemonx/internal/workspace"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestDataDefaultSQLiteEndpoint(t *testing.T) {
	s := &server{workspace: workspace.Layout{Root: t.TempDir()}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/data/sqlite/default", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	s.dataHandler(w, r)
	if w.Code != 200 {
		t.Fatalf("default open: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(result.Path) != "default.sqlite" || s.isSystemSQLite(result.Path) {
		t.Fatal("default database not separate", result.Path)
	}
	body, _ := json.Marshal(map[string]any{"path": result.Path, "sql": "CREATE TABLE user_data(id INTEGER)", "write": true, "confirmed": true})
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/api/v1/data/sql", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	s.dataHandler(w, r)
	if w.Code != 200 {
		t.Fatalf("default database is not manageable: %d %s", w.Code, w.Body.String())
	}
}

func TestDataHTTPBoundary(t *testing.T) {
	s := &server{}
	for _, test := range []struct {
		method, path, body, content string
		code                        int
	}{
		{"GET", "/api/v1/data/catalog", "", "", 200},
		{"GET", "/api/v1/data/sql", "", "", 405},
		{"POST", "/api/v1/data/sql", `{}`, "text/plain", 415},
		{"POST", "/api/v1/data/sql", `{`, "application/json", 400},
		{"POST", "/api/v1/data/redis", `{"args":["FLUSHALL"],"confirmed":true}`, "application/json", 400},
		{"POST", "/api/v1/data/unknown", `{}`, "application/json", 404},
	} {
		r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		r.Header.Set("Content-Type", test.content)
		w := httptest.NewRecorder()
		s.dataHandler(w, r)
		if w.Code != test.code {
			t.Fatalf("%s: got %d: %s", test.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("data response is cacheable")
		}
	}
}

func TestDataRedisConflictStatus(t *testing.T) {
	redis := miniredis.RunT(t)
	if err := redis.Set("key", "changed"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"address": redis.Addr(), "args": []string{"SET", "key", "new", "KEEPTTL"}, "expected": "original", "confirmed": true})
	r := httptest.NewRequest("POST", "/api/v1/data/redis", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	(&server{}).dataHandler(w, r)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected conflict: %d %s", w.Code, w.Body.String())
	}
	value, _ := redis.Get("key")
	if value != "changed" {
		t.Fatal("overwrote concurrent change")
	}
}

func TestDataProtectsSystemSQLiteAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ops.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE demo(id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Setenv("ALX_OPS_SQLITE_PATH", path)
	s := &server{}
	paths := []string{path}
	alias := filepath.Join(t.TempDir(), "alias.db")
	if err := os.Link(path, alias); err == nil {
		paths = append(paths, alias)
	}
	for _, selected := range paths {
		body, _ := json.Marshal(map[string]any{"path": selected, "sql": "DROP TABLE demo", "write": true, "confirmed": true})
		r := httptest.NewRequest("POST", "/api/v1/data/sql", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.dataHandler(w, r)
		if w.Code != 403 {
			t.Fatalf("system write allowed: %d %s", w.Code, w.Body.String())
		}
	}
	body, _ := json.Marshal(map[string]any{"path": path, "sql": "SELECT * FROM demo"})
	r := httptest.NewRequest("POST", "/api/v1/data/sql", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.dataHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"systemDatabase":true`) {
		t.Fatalf("system read failed: %d %s", w.Code, w.Body.String())
	}
}

func TestDataRequiresSuperAdmin(t *testing.T) {
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	admin, err := identity.Enable("root", "secret", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.SaveRole(access.Role{ID: "reader", Name: "只读", Permissions: []string{"workbench.view"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.CreateAccount("reader", "secret", "secret", []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	reader, err := identity.Login("reader", "secret")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{auth: identity}
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 403}, {reader, 403}, {admin, 200}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/data/catalog", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		s.dataHandler(w, r)
		if w.Code != tc.status {
			t.Fatalf("got %d want %d", w.Code, tc.status)
		}
	}
}

func TestDataBodiesNeverLogged(t *testing.T) {
	s := &server{}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/data/sql", strings.NewReader(`{"sql":"select 'private customer value'"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	if body := s.loggableRequestBody(c); body != "" {
		t.Fatal("SQL logged:", body)
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/data/connect", strings.NewReader(`{"connection":{"password":"secret"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	if body := s.loggableRequestBody(c); body != "" {
		t.Fatal("connection logged", body)
	}
}
