package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"alemonx/internal/access"
)

// Discover the actual registration sites so newly added endpoints join the
// security matrix automatically instead of silently escaping a static list.
func registeredAPIRoutes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "HandleFunc" {
			return true
		}
		path := ""
		switch value := call.Args[0].(type) {
		case *ast.BasicLit:
			path, _ = strconv.Unquote(value.Value)
		case *ast.Ident:
			if value.Name == "pluginDownloadBrokerPath" {
				path = pluginDownloadBrokerPath
			} else {
				t.Errorf("unresolved route constant: %s", value.Name)
			}
		default:
			t.Error("route needs explicit inventory support")
		}
		if strings.HasPrefix(path, "/api/v1/") {
			paths[path] = true
		}
		return true
	})
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	if len(result) == 0 {
		t.Fatal("empty API inventory")
	}
	return result
}

func TestAllRegisteredAPIsRequireAuthentication(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = identity.Enable("api-test-admin", "test-password", "test-password"); err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)
	public := map[string]bool{
		"/api/v1/auth/status": true, "/api/v1/auth/setup": true,
		"/api/v1/auth/login": true, "/api/v1/auth/logout": true,
		pluginDownloadBrokerPath: true,
	}
	routes := registeredAPIRoutes(t)
	for _, path := range routes {
		if public[path] {
			continue
		} // exercised by their dedicated authentication tests
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
			t.Run(method+" "+path, func(t *testing.T) {
				request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusUnauthorized {
					t.Fatalf("anonymous %s %s returned %d", method, path, recorder.Code)
				}
				if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
					t.Fatalf("API returned non-JSON: %s", recorder.Header().Get("Content-Type"))
				}
			})
		}
	}
	t.Logf("registered routes=%d; protected routes=%d; request matrix=%d", len(routes), len(routes)-len(public), (len(routes)-len(public))*6)
}

// Exercise each registered handler's protocol boundary without installing,
// deleting, launching processes, or calling an external provider. Success-path
// fixtures and external-service behavior are covered by the package tests.
func TestAllRegisteredAPIsRejectUnsupportedMethods(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := identity.Enable("api-test-admin", "test-password", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)
	for _, path := range registeredAPIRoutes(t) {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodTrace, path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusMethodNotAllowed && recorder.Code != http.StatusNotFound && recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusForbidden {
				t.Errorf("unsupported method returned %d: %.160s", recorder.Code, recorder.Body.String())
			}
			if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
				t.Errorf("API returned non-JSON: %s", recorder.Header().Get("Content-Type"))
			}
		})
	}
}

func TestUnknownAPIDoesNotFallBackToHTML(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/unknown-api-contract", nil))
		if recorder.Code != http.StatusNotFound || !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("unknown API: %d %s", recorder.Code, recorder.Body.String())
		}
	}
}
