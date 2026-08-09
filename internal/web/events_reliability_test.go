package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newReliabilityServer(t *testing.T) *server {
	t.Helper()
	s := newStatefulTestServer()
	s.eventGateway = newEventGateway()
	s.operationEvents = newOperationEventStore(filepath.Join(t.TempDir(), "operations", "events.db"))
	s.outputBuffers = map[string]*operationOutputBuffer{}
	t.Cleanup(func() { _ = s.operationEvents.Close() })
	return s
}

func cancelledRequest(path string) *http.Request {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
}

func TestEventsGatewayContractReplayTopicAndCursor(t *testing.T) {
	s := newReliabilityServer(t)
	robot, ok := s.publishEvent("robot", "task", map[string]any{"taskId": "r-1"}, nil)
	if !ok || robot.ID == 0 {
		t.Fatal("robot event was not persisted")
	}
	ops, ok := s.publishEvent("ops", "incident.changed", map[string]any{"id": "i-1"}, nil)
	if !ok || ops.ID <= robot.ID {
		t.Fatalf("global event ids are not ordered: robot=%d ops=%d", robot.ID, ops.ID)
	}

	recorder := httptest.NewRecorder()
	s.eventsHandler(recorder, cancelledRequest("/api/v1/events?topics=robot&lastEventId=0"))
	body := recorder.Body.String()
	if !strings.Contains(body, "id: "+stringID(robot.ID)) || !strings.Contains(body, `"topic":"robot"`) {
		t.Fatalf("robot replay envelope missing: %s", body)
	}
	if strings.Contains(body, "incident.changed") {
		t.Fatalf("topic filter leaked ops event: %s", body)
	}

	s.operationEvents.mu.Lock()
	_, _ = s.operationEvents.db.Exec(`DELETE FROM event_journal WHERE id=?`, robot.ID)
	s.operationEvents.mu.Unlock()
	recorder = httptest.NewRecorder()
	s.eventsHandler(recorder, cancelledRequest("/api/v1/events?topics=ops&lastEventId="+stringID(robot.ID)))
	if !strings.Contains(recorder.Body.String(), "system.cursor-expired") {
		t.Fatalf("expired cursor was not signaled: %s", recorder.Body.String())
	}

	// EventSource reconnects with this header. It must take precedence over an
	// older query cursor so a stale URL cannot replay an already confirmed item.
	recorder = httptest.NewRecorder()
	request := cancelledRequest("/api/v1/events?topics=ops&lastEventId=0")
	request.Header.Set("Last-Event-ID", stringID(ops.ID))
	s.eventsHandler(recorder, request)
	if strings.Contains(recorder.Body.String(), "incident.changed") {
		t.Fatalf("Last-Event-ID replayed an acknowledged item: %s", recorder.Body.String())
	}
}

func stringID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func TestOperationEventStoreRuntimeRecoveryAndNoUnpersistedEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations", "events.db")
	store := newOperationEventStore(path)
	defer store.Close()
	if _, err := store.append("robot", "task", map[string]string{"taskId": "before"}, nil); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.backupLocked(time.Now().UTC())
	store.mu.Unlock()
	store.fail(errors.New("database disk image is malformed"))
	deadline := time.Now().Add(2 * time.Second)
	for store.wasRecovered() == false && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !store.wasRecovered() {
		t.Fatalf("store did not recover: %#v", store.diagnostics())
	}
	if _, err := store.append("robot", "task", map[string]string{"taskId": "after"}, nil); err != nil {
		t.Fatalf("recovered store refused write: %v", err)
	}
	if strings.Contains(store.diagnostics()["degradedReason"].(string), "malformed") {
		t.Fatal("structural error leaked through recovered diagnostics")
	}
}

func TestOperationOutputBatchFlushPersistsOnce(t *testing.T) {
	s := newReliabilityServer(t)
	s.operations = []operationTask{{ID: "task-1", Status: "running"}}
	for i := 0; i < 100; i++ {
		s.appendOperationOutput("task-1", "line\n")
	}
	s.flushOperationOutput("task-1")
	events := s.operationEvents.after(0, map[string]bool{"robot": true})
	outputs := 0
	for _, event := range events {
		if event.Type == "output" {
			outputs++
		}
	}
	if outputs != 1 {
		t.Fatalf("output batches=%d, want 1", outputs)
	}
	if !strings.Contains(s.operations[0].Output, "line") {
		t.Fatal("batched output did not update task snapshot")
	}
}

func TestEventSanitizationKeepsDiagnosticsWithoutCredentials(t *testing.T) {
	clean := sanitizeEventData(map[string]any{
		"token": "secret-value",
		"text":  "connecting with Authorization: Bearer abc.def.ghi",
	})
	encoded := string(mustJSON(t, clean))
	if strings.Contains(encoded, "secret-value") || strings.Contains(encoded, "abc.def.ghi") {
		t.Fatalf("credential leaked into durable event: %s", encoded)
	}
	if !strings.Contains(encoded, "[REDACTED]") {
		t.Fatalf("sanitized event lost redaction marker: %s", encoded)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func BenchmarkOperationOutputBatching(b *testing.B) {
	path := filepath.Join(b.TempDir(), "operations", "events.db")
	s := &server{operations: []operationTask{{ID: "bench", Status: "running"}}, outputBuffers: map[string]*operationOutputBuffer{}, events: newRobotEventHub(), eventGateway: newEventGateway(), operationEvents: newOperationEventStore(path)}
	b.Cleanup(func() { _ = s.operationEvents.Close() })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.appendOperationOutput("bench", "0123456789abcdef\n")
		if i%100 == 99 {
			s.flushOperationOutput("bench")
		}
	}
	s.flushOperationOutput("bench")
}
