package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"alemonx/internal/robot"
)

func TestServeInitializesAndListsTools(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/list\"}\n")
	var output bytes.Buffer
	if err := NewServer("test", fstest.MapFS{}).Serve(input, &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("response count = %d, want 2: %s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize response = %s", lines[0])
	}
	if !strings.Contains(lines[1], `"name":"alemonjs_write_project_file"`) || !strings.Contains(lines[1], `"name":"alemonjs_start_project_action"`) {
		t.Fatalf("tools/list response = %s", lines[1])
	}
}

func TestToolSchemasNeverEmitNullRequired(t *testing.T) {
	encoded, err := json.Marshal(tools())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"required":null`) {
		t.Fatalf("tool schema contains invalid null required: %s", encoded)
	}
}

func TestRobotLogsReportStreamFilter(t *testing.T) {
	server, root := backpackFixture(t)
	home := t.TempDir()
	t.Setenv("PM2_HOME", home)
	t.Setenv("PM2_DAEMON_RPC_PORT", "")

	stdout := filepath.Join(root, "out.log")
	stderr := filepath.Join(root, "err.log")
	backpackWrite(t, stdout, "[ERROR] framework request failed\n")
	backpackWrite(t, stderr, "runtime diagnostic\n")
	dump, err := json.Marshal([]map[string]string{{
		"pm_cwd": root, "pm_out_log_path": stdout, "pm_err_log_path": stderr,
	}})
	if err != nil {
		t.Fatal(err)
	}
	backpackWrite(t, filepath.Join(home, "dump.pm2"), string(dump))

	for _, tc := range []struct {
		name   string
		source string
		filter string
		total  int
	}{
		{name: "omitted source", filter: "all", total: 1},
		{name: "stdout", source: "out", filter: "out", total: 1},
		{name: "stderr", source: "err", filter: "err", total: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"root": root, "mode": "background", "query": "framework request failed"}
			if tc.source != "" {
				args["source"] = tc.source
			}
			raw := backpackOK(t, server, "alemonjs_get_robot_logs", args)
			var result struct {
				robot.PM2AuditPage
				SourceFilter string `json:"sourceFilter"`
			}
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Fatal(err)
			}
			if result.SourceFilter != tc.filter || result.Total != tc.total || len(result.Lines) != tc.total {
				t.Fatalf("unexpected filtered result: %s", raw)
			}
			if tc.total > 0 && result.Lines[0].Source != "out" {
				t.Fatalf("framework error should retain its stdout source: %s", raw)
			}
		})
	}
}

func TestRobotLogHistoryReportsPM2ArchiveAndTimeCoverage(t *testing.T) {
	server, root := backpackFixture(t)
	home := t.TempDir()
	t.Setenv("PM2_HOME", home)
	t.Setenv("PM2_DAEMON_RPC_PORT", "")
	stderr := filepath.Join(root, "err.log")
	backpackWrite(t, stderr, "failure without time\n")
	backpackWrite(t, filepath.Join(root, "err__2026-11-22_01-00-00.log"), "2026-11-22T00:00:00+08:00: archived failure\n")
	backpackWrite(t, filepath.Join(root, "err.log.1"), "2026-11-22T00:20:00+08:00: numbered failure\n")
	excluded := filepath.Join(root, "err__2026-11-22_02-00-00.log")
	backpackWrite(t, excluded, "2026-11-22T00:00:00+08:00: foreign failure\n")
	dump, err := json.Marshal([]map[string]string{
		{"pm_cwd": root, "pm_err_log_path": stderr},
		{"pm_cwd": t.TempDir(), "pm_err_log_path": excluded},
	})
	if err != nil {
		t.Fatal(err)
	}
	backpackWrite(t, filepath.Join(home, "dump.pm2"), string(dump))
	raw := backpackOK(t, server, "alemonjs_get_robot_logs", map[string]any{
		"root": root, "mode": "background", "source": "err", "query": "failure",
		"since": "2026-11-22T00:00:00+08:00", "until": "2026-11-22T01:00:00+08:00",
	})
	var result robot.PM2AuditPage
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.UndatedExcluded != 1 || len(result.Lines) != 2 ||
		result.Lines[0].Timestamp == nil || !strings.Contains(result.Output, "archived failure") || !strings.Contains(result.Output, "numbered failure") ||
		result.ReadScope == nil || result.ReadScope.ArchiveFiles != 2 || result.ReadScope.ActiveFiles != 1 ||
		result.ReadScope.ScannedLines != 3 || result.ReadScope.OldestTimestamp == nil || result.ReadScope.NewestTimestamp == nil {
		t.Fatalf("unexpected history coverage: %s", raw)
	}
}

func TestServeAcceptsInitializedNotificationAndReadsCapabilities(t *testing.T) {
	input := strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{},\"clientInfo\":{\"name\":\"Codex\",\"version\":\"test\"}}}\n{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"resources/list\"}\n{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"resources/read\",\"params\":{\"uri\":\"alemonjs://mcp/capabilities\"}}\n")
	var output bytes.Buffer
	if err := NewServer("test", fstest.MapFS{}).Serve(input, &output); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("response count = %d, want 3: %s", len(lines), output.String())
	}
	if !strings.Contains(lines[1], "alemonjs://mcp/capabilities") || !strings.Contains(lines[2], "protected local Streamable HTTP") {
		t.Fatalf("resource responses = %q / %q", lines[1], lines[2])
	}
}

func TestWriteToolRequiresConfirmation(t *testing.T) {
	arguments, err := json.Marshal(map[string]any{"root": "/not-a-project", "action": "build", "confirm": false})
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewServer("test", nil).execute(context.Background(), "alemonjs_start_project_action", arguments)
	if err == nil || result != "" || !strings.Contains(err.Error(), "confirm=true") {
		t.Fatalf("execute() = %q, %v; want confirmation error", result, err)
	}
}

func TestUnknownToolReturnsMCPToolError(t *testing.T) {
	response := NewServer("test", nil).handle(context.Background(), rpcRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: json.RawMessage(`{"name":"unknown","arguments":{}}`)})
	encoded, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	if response.Error != nil || !strings.Contains(string(encoded), `"isError":true`) {
		t.Fatalf("response = %+v", response)
	}
}

func TestProjectActionRunsAsPollableTask(t *testing.T) {
	server := NewServer("test", nil)
	arguments, err := json.Marshal(map[string]any{"root": "/not-a-project", "action": "build", "confirm": true})
	if err != nil {
		t.Fatal(err)
	}
	text, err := server.execute(context.Background(), "alemonjs_start_project_action", arguments)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	var started operationTask
	if err := json.Unmarshal([]byte(text), &started); err != nil || started.ID == "" || started.Status != "running" {
		t.Fatalf("started task = %s, %v", text, err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		current, ok := server.getTask(started.ID)
		if ok && current.Status == "failed" {
			return
		}
	}
	t.Fatalf("task %s did not complete with expected failure", started.ID)
}

func TestHTTPHandlerRequiresBearerTokenAndServesMCP(t *testing.T) {
	server := NewServer("test", nil)
	unauthorized := httptest.NewRecorder()
	server.HTTPHandler("secret").ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	request.Header.Set("Authorization", "Bearer secret")
	server.HTTPHandler("secret").ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), `"protocolVersion":"2025-06-18"`) {
		t.Fatalf("initialize HTTP response = %d %s", authorized.Code, authorized.Body.String())
	}
	if got := authorized.Header().Get("MCP-Protocol-Version"); got != protocolVersion {
		t.Fatalf("response protocol version = %q", got)
	}

	unsupportedVersion := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("MCP-Protocol-Version", "1999-01-01")
	server.HTTPHandler("secret").ServeHTTP(unsupportedVersion, request)
	if unsupportedVersion.Code != http.StatusBadRequest {
		t.Fatalf("unsupported protocol status = %d", unsupportedVersion.Code)
	}

	foreignOrigin := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Origin", "https://untrusted.example")
	server.HTTPHandler("secret").ServeHTTP(foreignOrigin, request)
	if foreignOrigin.Code != http.StatusForbidden {
		t.Fatalf("foreign Origin status = %d", foreignOrigin.Code)
	}

	get := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer secret")
	server.HTTPHandler("secret").ServeHTTP(get, request)
	if get.Code != http.StatusMethodNotAllowed || get.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("GET stream response = %d Allow=%q", get.Code, get.Header().Get("Allow"))
	}

	notification := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	server.HTTPHandler("secret").ServeHTTP(notification, request)
	if notification.Code != http.StatusAccepted || notification.Body.Len() != 0 {
		t.Fatalf("notification HTTP response = %d %q", notification.Code, notification.Body.String())
	}
}

func TestPolicyRejectsProjectOutsideAllowedRoots(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	server := NewServerWithPolicy("test", nil, Policy{AllowedRoots: []string{allowed}})
	arguments, err := json.Marshal(map[string]any{"root": outside})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.execute(context.Background(), "alemonjs_project_status", arguments)
	if err == nil || !strings.Contains(err.Error(), "MCP_ALLOWED_ROOTS") {
		t.Fatalf("policy error = %v", err)
	}
}

func TestPolicyRejectsSymlinkEscapingAllowedRoot(t *testing.T) {
	allowed, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "package.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	server := NewServerWithPolicy("test", nil, Policy{AllowedRoots: []string{allowed}})
	arguments, err := json.Marshal(map[string]any{"root": link})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.execute(context.Background(), "alemonjs_project_status", arguments)
	if err == nil || !strings.Contains(err.Error(), "MCP_ALLOWED_ROOTS") {
		t.Fatalf("symlink policy error = %v", err)
	}
}

func TestOfficialResourceToolsAreDiscoverableAndValidateInput(t *testing.T) {
	encoded, err := json.Marshal(tools())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alemonjs_search_resources", "alemonjs_get_resource", "alemonjs_resource_taxonomy", "alemonjs_package_inventory"} {
		if !strings.Contains(string(encoded), name) {
			t.Fatalf("missing tool %s", name)
		}
	}
	server := NewServer("test", nil)
	for _, test := range []struct{ name, args string }{
		{"alemonjs_search_resources", `{"page":-1}`},
		{"alemonjs_search_resources", `{"pageSize":101}`},
		{"alemonjs_get_resource", `{"id":"https://github.com/example/repo"}`},
	} {
		if _, err := server.execute(context.Background(), test.name, json.RawMessage(test.args)); err == nil {
			t.Fatalf("invalid input accepted: %s", test.args)
		}
	}
}
