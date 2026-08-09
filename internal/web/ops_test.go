package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alemonx/internal/agent"
	"alemonx/internal/robot"
)

func TestOpsHandlerIncidentEventsAndMetrics(t *testing.T) {
	store := agent.NewOpsStoreAt(t.TempDir())
	incident := agent.Incident{ID: "inc-test", ProjectRoot: t.TempDir(), ProcessName: "app", Fingerprint: "fp", Status: agent.IncidentDetected, Severity: "medium", Occurrences: 1, Updated: time.Now()}
	if err := store.SaveIncident(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(incident.ID, agent.ErrorEvent{ID: "evt-1", RawMessage: "Error: test"}); err != nil {
		t.Fatal(err)
	}
	s := &server{opsStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ops/incidents/inc-test/events", nil)
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("evt-1")) {
		t.Fatalf("events response: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ops/metrics", nil)
	rec = httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"incidents":1`)) {
		t.Fatalf("metrics response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOpsPolicyRequiresAutoWhitelist(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"ops-test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store := agent.NewOpsStoreAt(t.TempDir())
	s := &server{opsStore: store}
	body, _ := json.Marshal(agent.OpsPolicy{ProjectRoot: root, Mode: "auto", AllowCodeChanges: true})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/ops/policy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("auto without whitelist should fail, got %d", rec.Code)
	}
	body, _ = json.Marshal(agent.OpsPolicy{ProjectRoot: root, Mode: "observe"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/ops/policy", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("observe policy should save, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestMonitorableRootsFiltersNonProjects(t *testing.T) {
	valid := t.TempDir()
	if err := os.WriteFile(filepath.Join(valid, "package.json"), []byte(`{"name":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	notAProject := t.TempDir()
	s := &server{directoryRoots: []string{valid, notAProject, filepath.Join(notAProject, "does-not-exist")}, robots: robot.Manager{}}
	roots := s.monitorableRoots()
	if len(roots) != 1 || roots[0] != valid {
		t.Fatalf("monitorableRoots should keep only valid robot projects, got %v", roots)
	}
}

func TestOpsIncidentDelete(t *testing.T) {
	store := agent.NewOpsStoreAt(t.TempDir())
	incident := agent.Incident{ID: "inc-del", ProjectRoot: t.TempDir(), ProcessName: "app", Fingerprint: "fp", Status: agent.IncidentDetected, Updated: time.Now()}
	if err := store.SaveIncident(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(incident.ID, agent.ErrorEvent{ID: "e1", RawMessage: "x"}); err != nil {
		t.Fatal(err)
	}
	s := &server{opsStore: store}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/ops/incidents/inc-del", nil)
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("inc-del")) {
		t.Fatalf("delete response: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetIncident("inc-del"); err == nil {
		t.Fatal("incident should be gone after delete")
	}
	if items, _ := store.ListIncidents(); len(items) != 0 {
		t.Fatalf("expected no incidents after delete, got %d", len(items))
	}
}

func TestOpsTodoDelete(t *testing.T) {
	store := agent.NewOpsStoreAt(t.TempDir())
	todo := agent.OpsTodo{ID: "todo-del", IncidentID: "inc-1", Title: "处理：app", Status: "open", Created: time.Now(), Updated: time.Now()}
	if err := store.SaveTodo(todo); err != nil {
		t.Fatal(err)
	}
	s := &server{opsStore: store}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/ops/todos/todo-del", nil)
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("todo-del")) {
		t.Fatalf("todo delete response: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := store.GetTodo("todo-del"); err == nil {
		t.Fatal("todo should be gone after delete")
	}
}

func TestOpsTodoPatchStatus(t *testing.T) {
	store := agent.NewOpsStoreAt(t.TempDir())
	todo := agent.OpsTodo{ID: "todo-patch", IncidentID: "inc-2", Title: "处理：app", Status: "open", Created: time.Now(), Updated: time.Now()}
	if err := store.SaveTodo(todo); err != nil {
		t.Fatal(err)
	}
	s := &server{opsStore: store}
	body, _ := json.Marshal(map[string]string{"status": "done"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/ops/todos/todo-patch", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("done")) {
		t.Fatalf("todo patch response: %d %s", rec.Code, rec.Body.String())
	}
	updated, _ := store.GetTodo("todo-patch")
	if updated.Status != "done" {
		t.Fatalf("expected status done, got %q", updated.Status)
	}
}

func TestOpsPrometheusMetrics(t *testing.T) {
	store := agent.NewOpsStoreAt(t.TempDir())
	s := &server{opsStore: store}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ops/metrics/prometheus", nil)
	rec := httptest.NewRecorder()
	s.opsHandler(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("incident_total")) {
		t.Fatalf("prometheus response: %d %s", rec.Code, rec.Body.String())
	}
}
