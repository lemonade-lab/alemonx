package web

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
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
	identity, err := access.NewAt(filepath.Join(root, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	password := "test-robot-list-password"
	alice, err := identity.Enable("alice", password, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.SaveRole(access.Role{ID: "robot-manager", Name: "Robot Manager", Permissions: []string{"workbench.view", "workbench.manage", "system.manage"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.CreateAccount("bob", password, password, []string{"robot-manager"}); err != nil {
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
	s := &server{auth: identity, workspaceRequested: root, mcpEvents: newMCPEventHub(), mcpHTTP: managedMCP{port: port}}
	s.mcpHTTP.accountHandler = func(token string, identity access.Status, authToken string) http.Handler {
		policy := s.accountMCPPolicy(mcp.Policy{}, root, identity, authToken)
		return mcp.NewServerWithPolicyAndWorkspace("test", nil, policy, root).HTTPHandler(token)
	}
	t.Cleanup(s.shutdownMCP)
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

	start := func(token string) (url, key string) {
		t.Helper()
		out := request(s.systemMCPHandler, http.MethodPost, token, map[string]string{"action": "start"}, 200)
		var status mcpHTTPStatus
		if err := json.Unmarshal(out.Body.Bytes(), &status); err != nil || !status.Managed {
			t.Fatalf("invalid status: %+v (%v)", status, err)
		}
		out = request(s.systemMCPHandler, http.MethodPost, token, map[string]string{"action": "token"}, 200)
		var connection struct{ Token string }
		if err := json.Unmarshal(out.Body.Bytes(), &connection); err != nil || connection.Token == "" {
			t.Fatal("missing MCP token")
		}
		return status.URL, connection.Token
	}
	endpoint, aliceMCP := start(alice)
	bobEndpoint, bobMCP := start(bob)
	if aliceMCP == bobMCP || endpoint != bobEndpoint {
		t.Fatal("accounts must share the listener with different credentials")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	call := func(token, name string, args map[string]any, status int) string {
		t.Helper()
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
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
	if output := call(aliceMCP, "alemonjs_list_robots", nil, 200); !strings.Contains(output, "alice-robot") {
		t.Fatal(output)
	}
	if output := call(bobMCP, "alemonjs_list_robots", nil, 200); strings.Contains(output, "alice-robot") {
		t.Fatal(output)
	}
	if output := call(bobMCP, "alemonjs_list_robot_backpack", map[string]any{"root": robotPath}, 200); !strings.Contains(output, `"isError":true`) {
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
	request(s.systemMCPHandler, http.MethodPost, bob, map[string]string{"action": "stop"}, 200)
	call(bobMCP, "alemonjs_list_robots", nil, 401)
	call(aliceMCP, "alemonjs_list_robots", nil, 200)
}
