package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func newChatHistoryTestStore(t *testing.T) *chatHistoryStore {
	t.Helper()
	t.Setenv("ALX_CHAT_RETENTION_DAYS", "")
	store, err := openChatHistoryStore(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRetentionDaysFromEnv(t *testing.T) {
	t.Setenv("ALX_CHAT_RETENTION_DAYS", "3")
	if got := retentionDaysFromEnv("ALX_CHAT_RETENTION_DAYS", defaultChatHistoryRetention); got != 3*24*time.Hour {
		t.Fatalf("retention = %v, want 3d", got)
	}
	t.Setenv("ALX_CHAT_RETENTION_DAYS", "0")
	if got := retentionDaysFromEnv("ALX_CHAT_RETENTION_DAYS", defaultChatHistoryRetention); got != defaultChatHistoryRetention {
		t.Fatalf("invalid retention fallback = %v", got)
	}
	t.Setenv("ALX_CHAT_RETENTION_DAYS", "500")
	if got := retentionDaysFromEnv("ALX_CHAT_RETENTION_DAYS", defaultChatHistoryRetention); got != 365*24*time.Hour {
		t.Fatalf("clamped retention = %v, want 365d", got)
	}
	t.Setenv("ALX_CHAT_RETENTION_DAYS", "abc")
	if got := retentionDaysFromEnv("ALX_CHAT_RETENTION_DAYS", defaultChatHistoryRetention); got != defaultChatHistoryRetention {
		t.Fatalf("bad retention fallback = %v", got)
	}
}

func TestChatHistoryStoreRoundTripAndSummary(t *testing.T) {
	store := newChatHistoryTestStore(t)
	root := "/robots/demo"
	snapshot := chatHistorySnapshot{
		SavedAt: 1700000000000,
		Events: []json.RawMessage{
			json.RawMessage(`{"MessageId":"m1","MessageText":"hi","CreateAt":1699999999000}`),
			json.RawMessage(`{"MessageId":"m2","MessageText":"hello","CreateAt":1700000000000}`),
		},
		Tools:                 []json.RawMessage{json.RawMessage(`{"id":"t1","at":1700000000000,"action":"send","target":"x","state":"success","summary":"ok"}`)},
		Drafts:                map[string]string{"conv-1": "draft"},
		Favorites:             []json.RawMessage{json.RawMessage(`{"id":"f1"}`)},
		Contacts:              []json.RawMessage{json.RawMessage(`{"id":"u1"}`)},
		Spaces:                []json.RawMessage{json.RawMessage(`{"id":"g1"}`)},
		OpenedConversationIDs: []string{"conv-1"},
		Preferences:           json.RawMessage(`{"density":"comfortable"}`),
	}
	if err := store.Save(root, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || len(loaded.Events) != 2 || len(loaded.Tools) != 1 || loaded.Drafts["conv-1"] != "draft" || len(loaded.OpenedConversationIDs) != 1 || loaded.SavedAt != 1700000000000 {
		t.Fatalf("loaded = %#v", loaded)
	}
	if string(loaded.Events[0]) != `{"MessageId":"m1","MessageText":"hi","CreateAt":1699999999000}` {
		t.Fatalf("event order = %s", loaded.Events[0])
	}
	summary, err := store.Summary()
	if err != nil || len(summary) != 1 {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
	if summary[0].Root != root || summary[0].Messages != 2 || summary[0].Tools != 1 || summary[0].LastActivity != 1700000000000 || summary[0].Bytes == 0 {
		t.Fatalf("summary = %#v", summary[0])
	}
	if err := store.Delete(root); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(root)
	if err != nil || loaded != nil {
		t.Fatalf("load after delete = %#v, %v", loaded, err)
	}
	if err := store.Save(root, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAll(); err != nil {
		t.Fatal(err)
	}
	summary, err = store.Summary()
	if err != nil || len(summary) != 0 {
		t.Fatalf("summary after clear = %#v, %v", summary, err)
	}
}

func TestChatHistoryHandlersSaveLoadAndDelete(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	s.chatHistory = newChatHistoryTestStore(t)

	body, err := json.Marshal(map[string]any{
		"root": root,
		"snapshot": map[string]any{
			"savedAt": 1700000000000,
			"events": []any{
				map[string]any{"MessageId": "m1", "MessageText": "hi", "CreateAt": 1699999999000},
			},
			"tools":                 []any{},
			"drafts":                map[string]string{},
			"favorites":             []any{},
			"contacts":              []any{},
			"spaces":                []any{},
			"openedConversationIds": []string{},
			"preferences":           map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.robotChatHistoryHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/robot/chat/history", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotChatHistoryHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/chat/history?root="+url.QueryEscape(root), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("load = %d %s", response.Code, response.Body.String())
	}
	var loaded struct {
		Snapshot *chatHistorySnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Snapshot == nil || len(loaded.Snapshot.Events) != 1 {
		t.Fatalf("loaded snapshot = %#v", loaded.Snapshot)
	}
	response = httptest.NewRecorder()
	s.robotChatSummaryHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/chat/summary", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotChatHistoryHandler(response, httptest.NewRequest(http.MethodDelete, "/api/v1/robot/chat/history?root="+url.QueryEscape(root), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
	// Deleting a robot whose directory no longer exists must still work.
	response = httptest.NewRecorder()
	s.robotChatHistoryHandler(response, httptest.NewRequest(http.MethodDelete, "/api/v1/robot/chat/history?root="+url.QueryEscape(filepath.Join(t.TempDir(), "gone")), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete removed robot = %d %s", response.Code, response.Body.String())
	}
}

func TestChatHistoryStorePrunesExpired(t *testing.T) {
	store := newChatHistoryTestStore(t)
	root := "/robots/demo"
	snapshot := chatHistorySnapshot{
		SavedAt: time.Now().UnixMilli(),
		Events: []json.RawMessage{
			json.RawMessage(`{"MessageId":"m1","MessageText":"hi","CreateAt":1699999999000}`),
		},
		Tools: []json.RawMessage{
			json.RawMessage(`{"id":"t1","at":1699999999000,"action":"send"}`),
		},
	}
	if err := store.Save(root, snapshot); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-defaultChatHistoryRetention - time.Hour).UnixMilli()
	if _, err := store.db.Exec(`UPDATE chat_events SET created_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE chat_tools SET created_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE chat_state SET saved_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneExpired(time.Now()); err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.Load(root); err != nil || loaded != nil {
		t.Fatalf("expired chat survived = %#v, %v", loaded, err)
	}
	// Fresh snapshots survive pruning.
	now := time.Now().UnixMilli()
	fresh := chatHistorySnapshot{
		SavedAt: now,
		Events: []json.RawMessage{
			json.RawMessage(fmt.Sprintf(`{"MessageId":"m2","MessageText":"new","CreateAt":%d}`, now)),
		},
	}
	if err := store.Save(root, fresh); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneExpired(time.Now()); err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.Load(root); err != nil || loaded == nil || len(loaded.Events) != 1 {
		t.Fatalf("fresh chat pruned = %#v, %v", loaded, err)
	}
}
