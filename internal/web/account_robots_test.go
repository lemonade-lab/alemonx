package web

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/mcp"
	"alemonx/internal/workspace"
)

func TestAccountRobotsAndMCPUseIsolatedLists(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PM2_HOME", t.TempDir())
	t.Setenv("PM2_DAEMON_RPC_PORT", "")
	identity, err := access.NewAt(filepath.Join(root, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	password := "test-robot-list-password"
	alice, err := identity.Enable("alice", password, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.CreateAccount("bob", password, password); err != nil {
		t.Fatal(err)
	}
	bob, err := identity.Login("bob", password)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	_ = listener.Close()
	if err != nil {
		t.Fatal(err)
	}
	newServer := func(identity *access.Manager) *server {
		s := &server{auth: identity, workspaceRequested: root, mcpEvents: newMCPEventHub(), mcpMonitorStop: make(chan struct{}), mcpHTTP: managedMCP{port: port}}
		s.mcpHTTP.accountHandler = func(token string, identity access.Status) http.Handler {
			policy := s.accountMCPPolicy(mcp.Policy{}, root, identity, token)
			return mcp.NewServerWithPolicyAndWorkspace("test", nil, policy, root).HTTPHandler(token)
		}
		t.Cleanup(func() {
			s.shutdownMCP()
			close(s.mcpMonitorStop)
		})
		return s
	}
	s := newServer(identity)
	request := func(handler http.HandlerFunc, method, token string, input any, status int) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "http://localhost/api/v1/workspace/robots", bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+token)
		out := httptest.NewRecorder()
		handler(out, r)
		if out.Code != status {
			t.Fatalf("%s got %d want %d: %s", method, out.Code, status, out.Body.String())
		}
		return out
	}
	read := func(token string) workspace.RobotList {
		t.Helper()
		out := request(s.robotListHandler, http.MethodGet, token, nil, 200)
		var list workspace.RobotList
		if err := json.Unmarshal(out.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		return list
	}
	robotPath := filepath.Join(root, "alice-robot")
	if err := os.MkdirAll(filepath.Join(robotPath, "packages", "demo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(robotPath, "package.json"), []byte(`{"name":"alice-robot"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(robotPath, "packages", "demo", "package.json"), []byte(`{"name":"demo","version":"1.0.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	change := workspace.RobotListChange{Scope: read(alice).Scope, Action: "add", Projects: []workspace.RobotEntry{{Path: robotPath}}}
	request(s.robotListHandler, http.MethodPost, alice, change, 200)
	request(s.robotListHandler, http.MethodPost, bob, change, 409)
	if list := read(bob); len(list.Projects) != 0 {
		t.Fatalf("Bob received Alice's robots: %+v", list.Projects)
	}

	post := func(token, action string, status int) *httptest.ResponseRecorder {
		t.Helper()
		return request(s.systemMCPHandler, http.MethodPost, token, map[string]string{"action": action}, status)
	}
	issueToken := func(session, action string) string {
		t.Helper()
		var connection struct{ Token string }
		out := post(session, action, http.StatusOK)
		if err := json.Unmarshal(out.Body.Bytes(), &connection); err != nil || connection.Token == "" {
			t.Fatal("missing MCP key")
		}
		return connection.Token
	}
	post(bob, "start", http.StatusForbidden)
	readMCPStatus := func(session string) mcpHTTPStatus {
		t.Helper()
		response := request(s.systemMCPHandler, http.MethodGet, session, nil, http.StatusOK)
		var status mcpHTTPStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	start := post(alice, "start", http.StatusOK)
	var service mcpHTTPStatus
	if err := json.Unmarshal(start.Body.Bytes(), &service); err != nil || !service.Managed || !service.AutoStart || service.HasToken {
		t.Fatalf("starting the listener should not issue a key: %+v (%v)", service, err)
	}
	endpoint := service.URL
	aliceMCP := issueToken(alice, "token")
	ordinaryStatus := request(s.systemMCPHandler, http.MethodGet, bob, nil, http.StatusOK)
	var status mcpHTTPStatus
	if err := json.Unmarshal(ordinaryStatus.Body.Bytes(), &status); err != nil || status.CanControl ||
		!status.CanIssueToken || !status.Running || !status.Managed || status.HasToken || status.Account != "bob" {
		t.Fatalf("ordinary account MCP status: %+v (%v)", status, err)
	}
	bobMCP := issueToken(bob, "token")
	if bobMCP == aliceMCP {
		t.Fatal("keys must be distinct across accounts")
	}
	post(bob, "token", http.StatusConflict)
	post(bobMCP, "rotate", http.StatusUnauthorized)
	raw, err := os.ReadFile(identity.Path())
	if err != nil || bytes.Contains(raw, []byte(aliceMCP)) || bytes.Contains(raw, []byte(bobMCP)) {
		t.Fatal("only key digests should be stored")
	}
	post(bob, "stop", http.StatusForbidden)
	client := &http.Client{Timeout: 2 * time.Second}
	rpc := func(token, method string, params any, status int) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result json.RawMessage
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != status {
			t.Fatalf("MCP status %d want %d (%v): %s", response.StatusCode, status, err, result)
		}
		return string(result)
	}
	call := func(token, name string, args map[string]any, status int) string {
		t.Helper()
		return rpc(token, "tools/call", map[string]any{"name": name, "arguments": args}, status)
	}
	diagnosticKey := func(action string, expected int) string {
		t.Helper()
		response := request(s.systemMCPHandler, http.MethodPost, alice, map[string]string{
			"action": action, "keyType": "diagnostic",
		}, expected)
		if expected != http.StatusOK {
			return ""
		}
		var result struct{ Token string }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Token == "" {
			t.Fatal("missing diagnostic key")
		}
		return result.Token
	}
	readKey := diagnosticKey("token", http.StatusOK)
	saved, err := os.ReadFile(identity.Path())
	if err != nil || bytes.Contains(saved, []byte(readKey)) {
		t.Fatal("only the diagnostic key digest should be stored")
	}
	if status := readMCPStatus(alice); !status.HasToken || !status.HasDiagnosticToken {
		t.Fatal("operator and diagnostic keys must coexist")
	}
	var listed struct {
		Result struct{ Tools []struct{ Name string } }
	}
	if err := json.Unmarshal([]byte(rpc(readKey, "tools/list", nil, 200)), &listed); err != nil || len(listed.Result.Tools) != 5 {
		t.Fatalf("diagnostic discovery: %+v (%v)", listed, err)
	}
	for _, tool := range []string{
		"alemonjs_read_project_file", "alemonjs_get_package_runtime_config", "alemonjs_control_robot_runtime",
		"alemonjs_start_project_action", "alemonjs_write_project_file",
	} {
		args := map[string]any{"root": robotPath, "path": "package.json", "confirm": true}
		if output := call(readKey, tool, args, 200); !strings.Contains(output, "诊断密钥无权调用此工具") {
			t.Fatalf("diagnostic key was not rejected for %s", tool)
		}
	}
	if output := call(bobMCP, "alemonjs_list_robots", nil, 200); strings.Contains(output, "alice-robot") {
		t.Fatal("ordinary account inherited the server owner's robot list")
	}
	if output := call(bobMCP, "alemonjs_list_robot_backpack", map[string]any{"root": robotPath}, 200); !strings.Contains(output, `"isError":true`) {
		t.Fatal("ordinary account accessed another account's robot")
	}
	if output := call(aliceMCP, "alemonjs_list_robots", nil, 200); !strings.Contains(output, "alice-robot") {
		t.Fatal(output)
	}
	if output := call(aliceMCP, "alemonjs_list_robot_backpack", map[string]any{"root": robotPath}, 200); !strings.Contains(output, `"version":"1.0.0"`) {
		t.Fatal(output)
	}
	if output := call(aliceMCP, "alemonjs_set_robot_plugin_enabled", map[string]any{"root": robotPath, "package": "demo", "enabled": true}, 200); !strings.Contains(output, `"isError":true`) {
		t.Fatal(output)
	}
	if output := call(aliceMCP, "alemonjs_set_robot_plugin_enabled", map[string]any{"root": robotPath, "package": "demo", "enabled": true, "confirm": true}, 200); !strings.Contains(output, `"enabled":true`) {
		t.Fatal(output)
	}
	s.mu.Lock()
	s.operations = []operationTask{{ID: "robot-test-log", Root: robotPath, Action: "app", Status: "running", Output: "robot ready\npassword=demo-secret\n"}}
	s.mu.Unlock()
	logArgs := map[string]any{"root": robotPath, "mode": "local"}
	if output := call(aliceMCP, "alemonjs_get_robot_logs", logArgs, 200); !strings.Contains(output, "robot ready") || strings.Contains(output, "demo-secret") {
		t.Fatal("MCP did not return redacted workbench logs")
	}
	// The failed build has already aged out of the live task snapshot.
	store := newOperationEventStore(filepath.Join(root, "diagnostic-events.db"))
	t.Cleanup(func() { _ = store.Close() })
	s.operationEvents = store
	failed := operationTask{
		ID: "evicted-build", Root: robotPath, Action: "build", Status: "failed",
		CreatedAt: time.Now().Add(-time.Hour), Progress: 70,
		Error:  "build failed password=fixture-secret",
		Output: "compiler: cannot resolve dependency\ntoken=fixture-secret",
		Steps:  []operationStep{{Progress: 70, Message: "compile stage"}},
	}
	if _, err := store.append("robot", "task", robotEvent{Type: "task", Task: &failed}, nil); err != nil {
		t.Fatal(err)
	}
	operations := call(readKey, "alemonjs_list_robot_operations", map[string]any{"root": robotPath, "status": "failed"}, 200)
	if !strings.Contains(operations, "evicted-build") || strings.Contains(operations, "fixture-secret") {
		t.Fatalf("failed operation history was missing or unsafe: %s", operations)
	}
	detail := call(readKey, "alemonjs_get_project_task", map[string]any{"taskId": "evicted-build"}, 200)
	if !strings.Contains(detail, "compile stage") || !strings.Contains(detail, "cannot resolve dependency") || strings.Contains(detail, "fixture-secret") {
		t.Fatalf("failed operation detail was missing or unsafe: %s", detail)
	}
	if output := call(bobMCP, "alemonjs_get_project_task", map[string]any{"taskId": "evicted-build"}, 200); !strings.Contains(output, `"isError":true`) || strings.Contains(output, "compile stage") {
		t.Fatal("task details escaped the account robot scope")
	}
	diagnostics := call(readKey, "alemonjs_get_robot_diagnostics", map[string]any{"root": robotPath}, 200)
	if !strings.Contains(diagnostics, "evicted-build") || !strings.Contains(diagnostics, "recent_operation_failed") ||
		!strings.Contains(diagnostics, "robot-test-log") || strings.Contains(diagnostics, "fixture-secret") {
		t.Fatalf("production diagnostic report incomplete: %s", diagnostics)
	}
	for _, tc := range []struct {
		session string
		status  int
	}{{alice, 200}, {bob, 403}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/robot/diagnostics?root="+url.QueryEscape(robotPath), nil)
		r.Header.Set("Authorization", "Bearer "+tc.session)
		response := httptest.NewRecorder()
		s.robotDiagnosticsHandler(response, r)
		if response.Code != tc.status || (tc.status == 200 && !strings.Contains(response.Body.String(), "evicted-build")) {
			t.Fatalf("workbench diagnostic status %d: %s", response.Code, response.Body.String())
		}
	}
	call(aliceMCP, "alemonjs_list_robots", nil, 200)
	request(s.authLogoutHandler, http.MethodPost, bob, nil, http.StatusOK)
	call(bobMCP, "alemonjs_list_robots", nil, http.StatusOK)

	s.shutdownMCP()
	reopened, err := access.NewAt(identity.Path())
	if err != nil {
		t.Fatal(err)
	}
	// A fresh workbench uses the saved preference even when the port is initially busy.
	occupied, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = occupied.Close() })
	s = newServer(reopened)
	s.startMCPStatusMonitor()
	if status := readMCPStatus(alice); !status.AutoStart || status.Managed {
		t.Fatalf("expected automatic restore to wait for port: %+v", status)
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	s.refreshMCPStatus()
	if status := readMCPStatus(alice); !status.AutoStart || !status.Managed {
		t.Fatalf("expected automatic restore to open MCP listener: %+v", status)
	}
	call(bobMCP, "alemonjs_list_robots", nil, http.StatusOK)
	call(aliceMCP, "alemonjs_list_robots", nil, http.StatusOK)

	call(readKey, "alemonjs_list_robots", nil, http.StatusOK)
	rotatedReadKey := diagnosticKey("rotate", http.StatusOK)
	call(readKey, "alemonjs_list_robots", nil, http.StatusUnauthorized)
	call(rotatedReadKey, "alemonjs_list_robots", nil, http.StatusOK)
	diagnosticKey("revoke", http.StatusNoContent)
	call(rotatedReadKey, "alemonjs_list_robots", nil, http.StatusUnauthorized)
	call(aliceMCP, "alemonjs_list_robots", nil, http.StatusOK)

	newKey := issueToken(bob, "rotate")
	call(bobMCP, "alemonjs_list_robots", nil, http.StatusUnauthorized)
	call(newKey, "alemonjs_list_robots", nil, http.StatusOK)
	post(bob, "revoke", http.StatusNoContent)
	call(newKey, "alemonjs_list_robots", nil, http.StatusUnauthorized)
	call(aliceMCP, "alemonjs_list_robots", nil, http.StatusOK)
	post(alice, "stop", http.StatusOK)
	s.shutdownMCP()
	stopped, err := access.NewAt(identity.Path())
	if err != nil {
		t.Fatal(err)
	}
	s = newServer(stopped)
	s.startMCPStatusMonitor()
	if status := readMCPStatus(alice); status.AutoStart || status.Managed || status.Running {
		t.Fatalf("manual stop must survive workbench restart: %+v", status)
	}
	offlineKey := issueToken(bob, "token")
	post(alice, "start", http.StatusOK)
	call(offlineKey, "alemonjs_list_robots", nil, http.StatusOK)
	if err := reopened.DeleteAccount("bob"); err != nil {
		t.Fatal(err)
	}
	call(offlineKey, "alemonjs_list_robots", nil, http.StatusUnauthorized)
}
