package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alemonx/internal/setupplugin"
)

func TestPluginDevelopmentRegistersAndStopsStaticSource(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "alx.json"), []byte(`{"id":"source-plugin","name":"Source","version":"1.0.0","web":{"root":"web"},"development":{"web":{"mode":"static"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	registry := setupplugin.NewRegistry(t.TempDir())
	manager := newPluginDevelopmentManager(registry, filepath.Join(t.TempDir(), "development.json"))
	registered, err := manager.registerPath(source)
	if err != nil || registered.Running {
		t.Fatalf("register = %#v, err=%v", registered, err)
	}
	started, err := manager.start("source-plugin")
	if err != nil || !started.Running {
		t.Fatalf("start = %#v, err=%v", started, err)
	}
	if started.State != "running" || started.Busy {
		t.Fatalf("unexpected running state: %#v", started)
	}
	plugin, err := registry.Find("source-plugin")
	if err != nil || !plugin.DevelopmentSource {
		t.Fatalf("development overlay = %#v, err=%v", plugin, err)
	}
	stopped, err := manager.stop("source-plugin")
	if err != nil || stopped.Running {
		t.Fatalf("stop = %#v, err=%v", stopped, err)
	}
	if stopped.State != "stopped" || stopped.Busy {
		t.Fatalf("unexpected stopped state: %#v", stopped)
	}
}

func TestPluginDevelopmentRemoveForgetsSourceWithoutDeletingIt(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(source, "alx.json")
	if err := os.WriteFile(manifest, []byte(`{"id":"remove-source-plugin","name":"Remove source","version":"1.0.0","web":{"root":"web"},"development":{"web":{"mode":"static"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "development.json")
	manager := newPluginDevelopmentManager(setupplugin.NewRegistry(t.TempDir()), statePath)
	if _, err := manager.registerPath(source); err != nil {
		t.Fatal(err)
	}
	if err := manager.remove("remove-source-plugin"); err != nil {
		t.Fatal(err)
	}
	if items := manager.list(); len(items) != 0 {
		t.Fatalf("remaining development sessions: %#v", items)
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("source manifest was removed: %v", err)
	}
	reloaded := newPluginDevelopmentManager(setupplugin.NewRegistry(t.TempDir()), statePath)
	if items := reloaded.list(); len(items) != 0 {
		t.Fatalf("removed source persisted: %#v", items)
	}
}

func TestPluginDevelopmentRemoveStopsRunningSource(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "alx.json"), []byte(`{"id":"remove-running-plugin","name":"Remove running","version":"1.0.0","web":{"root":"web"},"development":{"web":{"mode":"static"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	registry := setupplugin.NewRegistry(t.TempDir())
	manager := newPluginDevelopmentManager(registry, filepath.Join(t.TempDir(), "development.json"))
	if _, err := manager.registerPath(source); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.start("remove-running-plugin"); err != nil {
		t.Fatal(err)
	}
	if err := manager.remove("remove-running-plugin"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Find("remove-running-plugin"); err == nil {
		t.Fatal("development overlay remains active after removal")
	}
	if items := manager.list(); len(items) != 0 {
		t.Fatalf("remaining development sessions: %#v", items)
	}
}

func TestPluginDevelopmentRegistersFinderSelectedPath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source-plugin")
	if err := os.MkdirAll(filepath.Join(source, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "alx.json"), []byte(`{"id":"finder-source-plugin","name":"Finder source","version":"1.0.0","web":{"root":"web"},"development":{"web":{"mode":"static"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALEMONJS_SETUP_ROOTS", root)
	handler := newTestServer()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/development", bytes.NewBufferString(`{"path":"`+source+`"}`))
	request.RemoteAddr = "127.0.0.1:17390"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("Finder source registration = %d %s", response.Code, response.Body.String())
	}

	legacy := httptest.NewRecorder()
	legacyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/development/pick", nil)
	legacyRequest.RemoteAddr = "127.0.0.1:17390"
	handler.ServeHTTP(legacy, legacyRequest)
	if legacy.Code != http.StatusGone {
		t.Fatalf("legacy native picker = %d %s", legacy.Code, legacy.Body.String())
	}
}

func TestPluginDevelopmentWebSocketRespectsHMRDeclaration(t *testing.T) {
	registry := setupplugin.NewRegistry(t.TempDir())
	manager := newPluginDevelopmentManager(registry, "")
	for _, test := range []struct {
		name string
		hmr  bool
	}{
		{name: "disabled", hmr: false},
		{name: "enabled", hmr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager.mu.Lock()
			manager.sessions["source-plugin"] = &pluginDevelopmentSession{
				plugin:  setupplugin.Plugin{ID: "source-plugin", Development: &setupplugin.RuntimeSpec{Web: &setupplugin.DevelopmentWeb{Mode: "dev-server", HMR: test.hmr}}},
				running: true,
				web:     &pluginDevelopmentProcess{port: 4317},
			}
			manager.mu.Unlock()
			_, service, ok := manager.webTarget("source-plugin")
			if !ok || service.WebSocket != test.hmr {
				t.Fatalf("web target = %#v, ok=%v", service, ok)
			}
		})
	}
}

func TestDevelopmentWebHealthPathsPrefersUpstreamRoot(t *testing.T) {
	paths := developmentWebHealthPaths("source-plugin", "/ready")
	want := []string{"/ready", "/api/v1/setup/plugins/development/source-plugin/web/ready"}
	if !slices.Equal(paths, want) {
		t.Fatalf("health paths = %#v, want %#v", paths, want)
	}
	if paths := developmentWebHealthPaths("source-plugin", ""); !slices.Equal(paths, []string{"/", "/api/v1/setup/plugins/development/source-plugin/web/"}) {
		t.Fatalf("default health paths = %#v", paths)
	}
}

func TestRewriteDevelopmentModuleImportsKeepsViteModulesInProxyMount(t *testing.T) {
	response := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/javascript"}},
		Body: io.NopCloser(bytes.NewBufferString(`import React from "/node_modules/.vite/deps/react.js";
import "/src/styles.css";
const page = import("/src/page.tsx");
const asset = new URL("/src/logo.svg", import.meta.url);
const api = "/api/v1/setup/plugins";`)),
	}
	mount := "/api/v1/setup/plugins/development/source-plugin/web/"
	rewriteDevelopmentModuleImports(response, mount, true)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)
	for _, expected := range []string{
		`from "/api/v1/setup/plugins/development/source-plugin/web/node_modules/.vite/deps/react.js"`,
		`import "/api/v1/setup/plugins/development/source-plugin/web/src/styles.css"`,
		`import("/api/v1/setup/plugins/development/source-plugin/web/src/page.tsx")`,
		`new URL("/api/v1/setup/plugins/development/source-plugin/web/src/logo.svg"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing rewritten module URL %q in %s", expected, output)
		}
	}
	if !strings.Contains(output, `const api = "/api/v1/setup/plugins"`) {
		t.Fatalf("application URL should not be rewritten: %s", output)
	}
	if !strings.Contains(output, `target.pathname="/api/v1/setup/plugins/development/source-plugin/web/"`) {
		t.Fatalf("Vite HMR root path was not routed through the proxy: %s", output)
	}
}

func TestDevelopmentWebProxyDisablesCaching(t *testing.T) {
	registry := setupplugin.NewRegistry(t.TempDir())
	manager := newPluginDevelopmentManager(registry, "")
	server := &server{pluginDevelopment: manager}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/development/missing/web/", nil)
	recorder := httptest.NewRecorder()
	server.pluginDevelopmentWebProxy(recorder, request, "missing", "/")
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store, max-age=0" {
		t.Fatalf("development proxy Cache-Control = %q", cacheControl)
	}
}

func TestDevelopmentRetryTransportRetriesViteOptimizeTimeout(t *testing.T) {
	attempts := 0
	transport := developmentRetryTransport{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		status := http.StatusGatewayTimeout
		if attempts == 2 {
			status = http.StatusOK
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("response")), Header: http.Header{}}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/source", nil)
	response, err := transport.RoundTrip(request)
	if err != nil || response.StatusCode != http.StatusOK || attempts != 2 {
		t.Fatalf("response=%#v attempts=%d err=%v", response, attempts, err)
	}
	_ = response.Body.Close()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
