package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/mcp"
)

func TestWorkbenchManagedMCPHTTP(t *testing.T) {
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	_, port, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	s := &server{
		auth:      identity,
		mcpEvents: newMCPEventHub(),
		mcpHTTP: managedMCP{port: port, accountHandler: func(token string, _ access.Status) http.Handler {
			return mcp.NewServer("test", nil).HTTPHandler(token)
		}},
	}
	t.Cleanup(s.shutdownMCP)
	post := func(action, origin string, expected int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://localhost:17390/api/v1/system/mcp", strings.NewReader(`{"action":"`+action+`"}`))
		request.RemoteAddr = "127.0.0.1:50000"
		request.Header.Set("Origin", origin)
		result := httptest.NewRecorder()
		s.systemMCPHandler(result, request)
		if result.Code != expected {
			t.Fatalf("%s: status %d, expected %d", action, result.Code, expected)
		}
		return result
	}
	const local = "http://localhost:17390"
	post("start", "http://untrusted.example", http.StatusForbidden)
	post("token", "http://untrusted.example", http.StatusForbidden)
	post("start", local, http.StatusConflict)
	if autoStart, err := identity.MCPAutoStart(); err != nil || autoStart {
		t.Fatalf("a failed start must not enable automatic restore: %t (%v)", autoStart, err)
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	start := post("start", local, http.StatusOK)
	post("start", local, http.StatusOK)
	var status mcpHTTPStatus
	if err := json.Unmarshal(start.Body.Bytes(), &status); err != nil || !status.Running || !status.Managed {
		t.Fatalf("unexpected start status: %+v (%v)", status, err)
	}
	readToken := func() string {
		t.Helper()
		result := post("token", local, http.StatusOK)
		var connection struct{ Token string }
		if err := json.Unmarshal(result.Body.Bytes(), &connection); err != nil || len(connection.Token) < 40 {
			t.Fatal("missing connection token")
		}
		if result.Header().Get("Cache-Control") != "no-store" || strings.Contains(start.Body.String(), connection.Token) {
			t.Fatal("connection token leaked into cache or status")
		}
		return connection.Token
	}
	token := readToken()
	client := &http.Client{Timeout: time.Second}
	requestMCP := func(token string, expected int) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, status.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != expected {
			t.Fatalf("MCP initialize status %d, expected %d", response.StatusCode, expected)
		}
	}
	requestMCP("", http.StatusUnauthorized)
	requestMCP(token, http.StatusOK)
	workbench := httptest.NewRequest(http.MethodPost, "https://workbench.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	workbench.Header.Set("Authorization", "Bearer "+token)
	denied := httptest.NewRecorder()
	s.serveWorkbenchMCP(denied, workbench)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("local-only key was accepted by the workbench endpoint: %d", denied.Code)
	}
	post("stop", local, http.StatusOK)
	post("token", local, http.StatusConflict)
	post("start", local, http.StatusOK)
	requestMCP(token, http.StatusOK)
	post("revoke", local, http.StatusNoContent)
	requestMCP(token, http.StatusUnauthorized)
	requestMCP(readToken(), http.StatusOK)
	s.shutdownMCP()
	post("start", local, http.StatusServiceUnavailable)
}

func TestWorkbenchMCPThroughHTTPSProxy(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	runtime := newServerRuntimeWithAuth("test", fstest.MapFS{
		"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>fixture")},
	}, identity, false, ServerOptions{MCPPort: strconv.Itoa(port), WorkspaceRoot: t.TempDir(), RedisDisabled: true})
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	upstream := httptest.NewServer(runtime.Handler)
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	reverse := httputil.NewSingleHostReverseProxy(target)
	proxy := httptest.NewTLSServer(reverse)
	defer proxy.Close()
	client := proxy.Client()
	client.Timeout = 3 * time.Second

	post := func(path, token, jsonBody string, status int) (string, http.Header) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, proxy.URL+path, strings.NewReader(jsonBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		result, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer result.Body.Close()
		body, err := io.ReadAll(result.Body)
		if err != nil || result.StatusCode != status || (status != http.StatusNoContent && status != http.StatusAccepted && !strings.HasPrefix(result.Header.Get("Content-Type"), "application/json")) {
			t.Fatalf("%s: status %d want %d: %s (%v)", path, result.StatusCode, status, body, err)
		}
		return string(body), result.Header
	}

	const init = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"remote-client","version":"1"}}}`
	const list = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	post("/mcp", "", init, http.StatusServiceUnavailable)
	session, err := identity.Enable("proxy-test", "test-password", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	const settings = "/api/v1/system/mcp"
	post(settings, session, `{"action":"start"}`, http.StatusOK)
	_, headers := post("/mcp", "", init, http.StatusUnauthorized)
	if !strings.HasPrefix(headers.Get("WWW-Authenticate"), "Bearer") {
		t.Fatal("missing bearer authentication challenge")
	}
	post("/mcp", session, init, http.StatusUnauthorized)
	post(settings, session, `{"action":"token","keyType":"diagnostic"}`, http.StatusBadRequest)
	raw, _ := post(settings, session, `{"action":"token"}`, http.StatusOK)
	var credentials struct{ Token string }
	if err := json.Unmarshal([]byte(raw), &credentials); err != nil || credentials.Token == "" {
		t.Fatal("missing MCP key")
	}
	body, _ := post("/mcp", credentials.Token, init, http.StatusOK)
	if !strings.Contains(body, `"protocolVersion":"2025-06-18"`) || !strings.Contains(body, `"name":"alemonx"`) {
		t.Fatalf("invalid MCP initialize response: %s", body)
	}
	post("/mcp", credentials.Token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, http.StatusAccepted)
	body, _ = post("/mcp", credentials.Token, list, http.StatusOK)
	if !strings.Contains(body, "alemonjs_start_project_action") || !strings.Contains(body, "alemonjs_get_robot_diagnostics") {
		t.Fatal("MCP key did not discover the full tool list")
	}
	post(settings, session, `{"action":"stop"}`, http.StatusOK)
	post("/mcp", credentials.Token, list, http.StatusServiceUnavailable)
}
