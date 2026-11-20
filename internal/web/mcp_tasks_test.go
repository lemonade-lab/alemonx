package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/mcp"
)

func TestMCPRobotTasksSurviveRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX shell scripts")
	}
	root := t.TempDir()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	write := func(path, text string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "node"), "#!/bin/sh\nprintf 'v22.23.5\\n'\n", 0700)
	write(filepath.Join(bin, "npm"), `#!/bin/sh
case "$1" in
  install) printf 'install\n' ;;
  *) printf '%s\n' "$*" ;;
esac >> calls
printf 'fixture %s token=fixture-secret-value\n' "$*"
`, 0700)
	write(filepath.Join(root, "package.json"), `{"name":"fixture","packageManager":"npm@10.9.0","scripts":{"build":"fixture"}}`, 0600)

	auth, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.Enable("fixture-admin", "local-test-password", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateAccount("fixture-operator", "local-test-password", "local-test-password"); err != nil {
		t.Fatal(err)
	}
	operator, err := auth.Login("fixture-operator", "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "events.db")
	open := func() *server {
		store := newOperationEventStore(path)
		t.Cleanup(func() { _ = store.Close() })
		return &server{auth: auth, operationEvents: store, operations: store.snapshot(), events: newRobotEventHub(), eventGateway: newEventGateway()}
	}
	s := open()
	issue := func(session string) string {
		t.Helper()
		identity, err := auth.Status(session)
		if err != nil {
			t.Fatal(err)
		}
		token, err := auth.UpdateMCPKey(session, identity, "token")
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	operatorIdentity, err := auth.Status(operator)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.addCreatedRobot(root, operatorIdentity.RobotScope, root); err != nil {
		t.Fatal(err)
	}
	operatorKey := issue(operator)
	adminKey := issue(session)
	call := func(token, name string, args map[string]any) (string, bool) {
		t.Helper()
		return callTaskMCP(t, s, root, token, name, args)
	}

	start := func(key, action, tag string) (string, bool) {
		return call(key, "alemonjs_start_project_action", map[string]any{
			"root": root, "action": action, "tag": tag, "confirm": true, "account": "spoofed",
		})
	}
	ctx := context.Background()
	taskIDs := map[string]string{}
	for _, action := range []string{"build", "install", "npm-publish"} {
		// An invalid tag exercises the production publish path and fails before
		// it can contact a registry or remote repository.
		tag := ""
		if action == "npm-publish" {
			tag = "!invalid-tag"
		}
		result, failed := start(operatorKey, action, tag)
		var task operationTask
		if failed || json.Unmarshal([]byte(result), &task) != nil || task.ID == "" || task.Status != "running" ||
			task.Account != "fixture-operator" || task.Root != root || task.Source != "mcp" {
			t.Fatalf("invalid acceptance for %s: %s", action, result)
		}
		taskIDs[action] = task.ID
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			task, _ = s.operationEvents.mcpTaskByID(ctx, task.ID)
			if task.FinishedAt != nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		expected := "completed"
		if action == "npm-publish" {
			expected = "failed"
		}
		if task.Status != expected || task.FinishedAt == nil || strings.Contains(task.Output+task.Error, "fixture-secret-value") {
			t.Fatalf("invalid final result for %s: %+v", action, task)
		}
	}

	pending := operationTask{ID: "crashed-publish", Source: "mcp", Account: "fixture-operator", Root: root, Action: "git-release", Status: "running", CreatedAt: time.Now()}
	if _, err := s.operationEvents.recordMCPTask(pending, mcpTaskAccepted); err != nil {
		t.Fatal(err)
	}
	s.addOperation(pending)
	if err := s.operationEvents.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	if err := s.settleRecoveredMCPTasks(); err != nil {
		t.Fatal(err)
	}
	for action, id := range taskIDs {
		result, failed := call(operatorKey, "alemonjs_get_project_task", map[string]any{"taskId": id})
		if failed || !strings.Contains(result, "fixture-operator") || !strings.Contains(result, action) ||
			!strings.Contains(result, "finishedAt") || !strings.Contains(result, root) {
			t.Fatalf("missing durable task for %s: %s", action, result)
		}
		if _, failed := call(adminKey, "alemonjs_get_project_task", map[string]any{"taskId": id}); !failed {
			t.Fatalf("another account read a task outside its robot list: %s", id)
		}
	}
	if result, failed := call(operatorKey, "alemonjs_list_robot_operations", map[string]any{"root": root, "status": "interrupted"}); failed ||
		!strings.Contains(result, pending.ID) || !strings.Contains(result, "执行结果未知") {
		t.Fatalf("interrupted task status: %s", result)
	}
	if result, failed := call(operatorKey, "alemonjs_list_project_tasks", map[string]any{}); failed || !strings.Contains(result, taskIDs["build"]) {
		t.Fatalf("project task list: %s", result)
	}

	for _, id := range append([]string{pending.ID}, taskIDs["build"], taskIDs["install"], taskIDs["npm-publish"]) {
		rows, err := s.operationEvents.db.Query(`SELECT type FROM event_journal WHERE topic='mcp'
			AND json_extract(CAST(payload AS TEXT), '$.task_id')=? ORDER BY id`, id)
		if err != nil {
			t.Fatal(err)
		}
		var kinds []string
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				t.Fatal(err)
			}
			kinds = append(kinds, kind)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if len(kinds) != 2 || kinds[0] != mcpTaskAccepted || kinds[1] != mcpTaskResult {
			t.Fatalf("audit for %s: %v", id, kinds)
		}
	}
	if _, err := s.operationEvents.db.Exec(`DELETE FROM event_journal; DELETE FROM operation_snapshots WHERE id != ?`, pending.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.operationEvents.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	if task, ok := s.operationEvents.mcpTaskByID(ctx, taskIDs["build"]); !ok || task.Status != "completed" {
		t.Fatalf("task lost after journal removal: %+v", task)
	}
	if len(s.operations) != 1 || s.operations[0].Status != "interrupted" {
		t.Fatalf("stale running snapshot after second restart: %+v", s.operations)
	}
	if err := s.operationEvents.Close(); err != nil {
		t.Fatal(err)
	}
	if result, failed := start(operatorKey, "build", ""); !failed || !strings.Contains(result, "操作未受理") {
		t.Fatalf("admission with closed task store: %s", result)
	}
	calls, err := os.ReadFile(filepath.Join(root, "calls"))
	if err != nil || string(calls) != "install\nrun build\ninstall\n" {
		t.Fatalf("unexpected fixture executions: %q (%v)", calls, err)
	}
}

func callTaskMCP(t *testing.T, s *server, root, token, name string, args map[string]any) (string, bool) {
	t.Helper()
	identity, valid, err := s.auth.MCPKeyStatus(token)
	if err != nil || !valid {
		t.Fatalf("invalid fixture key: %v", err)
	}
	policy := s.accountMCPPolicy(mcp.Policy{}, root, identity, token)
	handler := mcp.NewServerWithPolicyAndWorkspace("test", nil, policy, root).HTTPHandler(token)
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/mcp", bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload struct {
		Result struct {
			Content []struct{ Text string }
			IsError bool
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.Result.Content) != 1 {
		t.Fatalf("invalid MCP response %s (%v)", response.Body, err)
	}
	return payload.Result.Content[0].Text, payload.Result.IsError
}
