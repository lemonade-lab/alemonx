package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
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
		mcpHTTP:   managedMCP{port: port, handler: mcp.NewServer("test", nil).HTTPHandler},
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
	post("start", local, http.StatusConflict)
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
	post("stop", local, http.StatusOK)
	post("token", local, http.StatusConflict)
	post("start", local, http.StatusOK)
	requestMCP(token, http.StatusUnauthorized)
	requestMCP(readToken(), http.StatusOK)
	s.shutdownMCP()
	post("start", local, http.StatusServiceUnavailable)
}
