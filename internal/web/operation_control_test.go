package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationControlCancellationAndRetryGuard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &server{events: newRobotEventHub(), operations: []operationTask{{ID: "task", Status: "running", CanCancel: true, CanRetry: true}}, taskControls: map[string]*operationControl{"task": {cancel: cancel}}}
	request := func(action string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.operationControlHandler(w, httptest.NewRequest(http.MethodPost, "/api/v1/robot/tasks/control", strings.NewReader(`{"id":"task","action":"`+action+`"}`)))
		return w
	}
	if w := request("retry"); w.Code != 409 {
		t.Fatalf("retry running task: %d", w.Code)
	}
	if w := request("cancel"); w.Code != 202 {
		t.Fatalf("cancel task: %d %s", w.Code, w.Body.String())
	}
	if ctx.Err() == nil {
		t.Fatal("cancellation never reached worker")
	}
	if s.operations[0].Status != "running" {
		t.Fatal("marked stopped before worker exit")
	}
	s.updateOperation("task", 100, "worker exited", "killed", true)
	if s.operations[0].Status != "cancelled" || s.operations[0].CanCancel {
		t.Fatalf("wrong settlement: %#v", s.operations[0])
	}
}

func TestOperationControlRetryReusesOriginalRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"root": root, "action": "unsupported-test-action", "package": "fixture-plugin", "version": "fixture-version"})
	s := &server{events: newRobotEventHub(), operations: []operationTask{{ID: "old", Root: root, Action: "unsupported-test-action", Status: "failed", CanRetry: true}}, taskControls: map[string]*operationControl{"old": {request: payload}}}
	w := httptest.NewRecorder()
	s.operationControlHandler(w, httptest.NewRequest(http.MethodPost, "/api/v1/robot/tasks/control", strings.NewReader(`{"id":"old","action":"retry"}`)))
	if w.Code != 202 {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	var task operationTask
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "old" || task.Root != root || task.Action != "unsupported-test-action" {
		t.Fatalf("wrong new task: %#v", task)
	}
	s.mu.RLock()
	replay := append([]byte(nil), s.taskControls[task.ID].request...)
	s.mu.RUnlock()
	var input map[string]string
	if err := json.Unmarshal(replay, &input); err != nil {
		t.Fatal(err)
	}
	if input["package"] != "fixture-plugin" || input["version"] != "fixture-version" {
		t.Fatalf("lost original arguments: %s", replay)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		status := s.operations[0].Status
		s.mu.RUnlock()
		if status != "running" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("retried worker did not finish")
}
