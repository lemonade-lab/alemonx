package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxChatEventsPerRoot    = 500
	maxChatToolsPerRoot     = 100
	maxChatItemBytes        = 1 << 20
	maxChatSnapshotBytes    = 8 << 20
	maxChatStateCollections = 500
	// defaultChatHistoryRetention bounds server-side chat records. The UI already
	// keeps 7/30 days of history; this prunes anything the browser stops
	// refreshing (inactive robots, abandoned sessions) so chat.db self-clears.
	defaultChatHistoryRetention = 30 * 24 * time.Hour
)

// chatHistorySnapshot mirrors the browser chat store: message events and tool
// records are stored per row, and the remaining UI state is one JSON blob per
// robot root. Payloads are treated as opaque JSON produced by the chat UI.
type chatHistorySnapshot struct {
	SavedAt               int64             `json:"savedAt"`
	Events                []json.RawMessage `json:"events"`
	Tools                 []json.RawMessage `json:"tools"`
	Drafts                map[string]string `json:"drafts"`
	Favorites             []json.RawMessage `json:"favorites"`
	Contacts              []json.RawMessage `json:"contacts"`
	Spaces                []json.RawMessage `json:"spaces"`
	OpenedConversationIDs []string          `json:"openedConversationIds"`
	Preferences           json.RawMessage   `json:"preferences"`
}

// chatRecordSummary is one robot's persisted record totals, used by the
// management view to show how much chat data is stored and to clean it up.
type chatRecordSummary struct {
	Root         string `json:"root"`
	Messages     int64  `json:"messages"`
	Tools        int64  `json:"tools"`
	LastActivity int64  `json:"lastActivity"`
	Bytes        int64  `json:"bytes"`
}

// chatHistoryStore is a pure-Go SQLite store for robot chat snapshots. It uses
// the same modernc.org/sqlite driver as the ops store, so it works in minimal
// production images without a system sqlite3 binary.
type chatHistoryStore struct {
	path      string
	mediaDir  string
	db        *sql.DB
	mu        sync.Mutex
	retention time.Duration
}

func openChatHistoryStore(path string) (*chatHistoryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("聊天记录存储路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &chatHistoryStore{
		path:      path,
		mediaDir:  filepath.Join(filepath.Dir(path), "chat-media"),
		db:        db,
		retention: retentionDaysFromEnv("ALX_CHAT_RETENTION_DAYS", defaultChatHistoryRetention),
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS chat_events(root TEXT NOT NULL, event_id TEXT NOT NULL, created_at INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(root, event_id));
CREATE TABLE IF NOT EXISTS chat_tools(root TEXT NOT NULL, tool_id TEXT NOT NULL, created_at INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(root, tool_id));
CREATE TABLE IF NOT EXISTS chat_state(root TEXT PRIMARY KEY, payload TEXT NOT NULL, saved_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS chat_events_root_created ON chat_events(root, created_at DESC);
CREATE INDEX IF NOT EXISTS chat_tools_root_created ON chat_tools(root, created_at DESC);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Best-effort cleanup of records that expired while the service was off.
	_ = store.PruneExpired(time.Now())
	return store, nil
}

// retentionDaysFromEnv reads an optional storage-valve environment variable
// (days) and clamps it to a sane range. Invalid or missing values fall back.
func retentionDaysFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 {
		return fallback
	}
	if days > 365 {
		days = 365
	}
	return time.Duration(days) * 24 * time.Hour
}

func (s *chatHistoryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// Save replaces one robot's persisted snapshot atomically. The browser is the
// authoritative client for chat state; the server keeps a bounded, queryable
// copy so records survive browser storage and can be managed centrally.
func (s *chatHistoryStore) Save(root string, snapshot chatHistorySnapshot) error {
	_ = s.PruneExpired(time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(snapshot.Events) > maxChatEventsPerRoot {
		snapshot.Events = snapshot.Events[len(snapshot.Events)-maxChatEventsPerRoot:]
	}
	if len(snapshot.Tools) > maxChatToolsPerRoot {
		snapshot.Tools = snapshot.Tools[:maxChatToolsPerRoot]
	}
	if len(snapshot.Favorites) > maxChatStateCollections {
		snapshot.Favorites = snapshot.Favorites[:maxChatStateCollections]
	}
	if len(snapshot.Contacts) > maxChatStateCollections {
		snapshot.Contacts = snapshot.Contacts[:maxChatStateCollections]
	}
	if len(snapshot.Spaces) > maxChatStateCollections {
		snapshot.Spaces = snapshot.Spaces[:maxChatStateCollections]
	}
	if len(snapshot.OpenedConversationIDs) > maxChatStateCollections {
		snapshot.OpenedConversationIDs = snapshot.OpenedConversationIDs[:maxChatStateCollections]
	}
	if snapshot.Drafts == nil {
		snapshot.Drafts = map[string]string{}
	}
	var total int64
	check := func(item []byte) error {
		if int64(len(item)) > maxChatItemBytes {
			return errors.New("聊天记录单条内容过大")
		}
		total += int64(len(item))
		if total > maxChatSnapshotBytes {
			return errors.New("聊天记录快照过大")
		}
		return nil
	}
	for _, item := range snapshot.Events {
		if err := check(item); err != nil {
			return err
		}
	}
	for _, item := range snapshot.Tools {
		if err := check(item); err != nil {
			return err
		}
	}
	for _, item := range snapshot.Favorites {
		if err := check(item); err != nil {
			return err
		}
	}
	for _, item := range snapshot.Contacts {
		if err := check(item); err != nil {
			return err
		}
	}
	for _, item := range snapshot.Spaces {
		if err := check(item); err != nil {
			return err
		}
	}
	for _, item := range snapshot.OpenedConversationIDs {
		if err := check([]byte(item)); err != nil {
			return err
		}
	}
	if err := check(snapshot.Preferences); err != nil {
		return err
	}
	for _, value := range snapshot.Drafts {
		if err := check([]byte(value)); err != nil {
			return err
		}
	}
	state, err := json.Marshal(map[string]any{
		"drafts":                snapshot.Drafts,
		"favorites":             snapshot.Favorites,
		"contacts":              snapshot.Contacts,
		"spaces":                snapshot.Spaces,
		"openedConversationIds": snapshot.OpenedConversationIDs,
		"preferences":           snapshot.Preferences,
	})
	if err != nil {
		return err
	}
	savedAt := snapshot.SavedAt
	if savedAt <= 0 {
		savedAt = time.Now().UnixMilli()
	}
	now := savedAt
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM chat_events WHERE root = ?`, root); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM chat_tools WHERE root = ?`, root); err != nil {
		return err
	}
	for index, item := range snapshot.Events {
		if _, err := tx.Exec(`INSERT INTO chat_events(root, event_id, created_at, payload) VALUES(?,?,?,?)`, root, fmt.Sprintf("e%d", index), eventCreatedAt(item, now), string(item)); err != nil {
			return err
		}
	}
	for index, item := range snapshot.Tools {
		if _, err := tx.Exec(`INSERT INTO chat_tools(root, tool_id, created_at, payload) VALUES(?,?,?,?)`, root, fmt.Sprintf("t%d", index), toolCreatedAt(item, now), string(item)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO chat_state(root, payload, saved_at) VALUES(?,?,?) ON CONFLICT(root) DO UPDATE SET payload=excluded.payload, saved_at=excluded.saved_at`, root, string(state), savedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// PruneExpired removes events, tool records and state snapshots older than the
// retention window, so inactive robots and abandoned sessions do not pile up
// in the database forever.
func (s *chatHistoryStore) PruneExpired(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	retention := s.retention
	if retention <= 0 {
		retention = defaultChatHistoryRetention
	}
	cutoff := now.Add(-retention).UnixMilli()
	for _, statement := range []string{
		`DELETE FROM chat_events WHERE created_at < ?`,
		`DELETE FROM chat_tools WHERE created_at < ?`,
		`DELETE FROM chat_state WHERE saved_at < ?`,
	} {
		if _, err := s.db.Exec(statement, cutoff); err != nil {
			return err
		}
	}
	return s.pruneMediaExpired(now.Add(-retention))
}

// Load returns the persisted snapshot for one robot, or nil when nothing has
// been stored yet. Events come back in chronological order; tools newest first.
func (s *chatHistoryStore) Load(root string) (*chatHistorySnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := &chatHistorySnapshot{Drafts: map[string]string{}}
	eventRows, err := s.db.Query(`SELECT payload FROM chat_events WHERE root = ? ORDER BY created_at ASC`, root)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var payload string
		if err := eventRows.Scan(&payload); err != nil {
			return nil, err
		}
		snapshot.Events = append(snapshot.Events, json.RawMessage(payload))
	}
	if err := eventRows.Err(); err != nil {
		return nil, err
	}
	toolRows, err := s.db.Query(`SELECT payload FROM chat_tools WHERE root = ? ORDER BY created_at DESC`, root)
	if err != nil {
		return nil, err
	}
	defer toolRows.Close()
	for toolRows.Next() {
		var payload string
		if err := toolRows.Scan(&payload); err != nil {
			return nil, err
		}
		snapshot.Tools = append(snapshot.Tools, json.RawMessage(payload))
	}
	if err := toolRows.Err(); err != nil {
		return nil, err
	}
	var statePayload string
	var savedAt int64
	if err := s.db.QueryRow(`SELECT payload, saved_at FROM chat_state WHERE root = ?`, root).Scan(&statePayload, &savedAt); err == nil {
		snapshot.SavedAt = savedAt
		var state struct {
			Drafts                map[string]string `json:"drafts"`
			Favorites             []json.RawMessage `json:"favorites"`
			Contacts              []json.RawMessage `json:"contacts"`
			Spaces                []json.RawMessage `json:"spaces"`
			OpenedConversationIDs []string          `json:"openedConversationIds"`
			Preferences           json.RawMessage   `json:"preferences"`
		}
		if json.Unmarshal([]byte(statePayload), &state) == nil {
			snapshot.Drafts = state.Drafts
			snapshot.Favorites = state.Favorites
			snapshot.Contacts = state.Contacts
			snapshot.Spaces = state.Spaces
			snapshot.OpenedConversationIDs = state.OpenedConversationIDs
			snapshot.Preferences = state.Preferences
		}
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	if len(snapshot.Events) == 0 && len(snapshot.Tools) == 0 && snapshot.SavedAt == 0 {
		return nil, nil
	}
	return snapshot, nil
}

// Summary aggregates persisted record totals across robots for management.
func (s *chatHistoryStore) Summary() ([]chatRecordSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	merged := map[string]*chatRecordSummary{}
	merge := func(root string, messages, tools, bytes, lastActivity int64) {
		item := merged[root]
		if item == nil {
			item = &chatRecordSummary{Root: root}
			merged[root] = item
		}
		item.Messages += messages
		item.Tools += tools
		item.Bytes += bytes
		if lastActivity > item.LastActivity {
			item.LastActivity = lastActivity
		}
	}
	rows, err := s.db.Query(`SELECT root, COUNT(*), COALESCE(MAX(created_at),0), COALESCE(SUM(length(payload)),0) FROM chat_events GROUP BY root`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var root string
		var count, lastActivity, bytes int64
		if err := rows.Scan(&root, &count, &lastActivity, &bytes); err != nil {
			rows.Close()
			return nil, err
		}
		merge(root, count, 0, bytes, lastActivity)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.db.Query(`SELECT root, COUNT(*), COALESCE(MAX(created_at),0), COALESCE(SUM(length(payload)),0) FROM chat_tools GROUP BY root`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var root string
		var count, lastActivity, bytes int64
		if err := rows.Scan(&root, &count, &lastActivity, &bytes); err != nil {
			rows.Close()
			return nil, err
		}
		merge(root, 0, count, bytes, lastActivity)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.db.Query(`SELECT root, saved_at, length(payload) FROM chat_state`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var root string
		var savedAt, bytes int64
		if err := rows.Scan(&root, &savedAt, &bytes); err != nil {
			rows.Close()
			return nil, err
		}
		merge(root, 0, 0, bytes, savedAt)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]chatRecordSummary, 0, len(merged))
	for _, item := range merged {
		result = append(result, *item)
	}
	return result, nil
}

func (s *chatHistoryStore) Delete(root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{`DELETE FROM chat_events WHERE root = ?`, `DELETE FROM chat_tools WHERE root = ?`, `DELETE FROM chat_state WHERE root = ?`} {
		if _, err := tx.Exec(statement, root); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return os.RemoveAll(s.mediaRootPath(root))
}

func (s *chatHistoryStore) DeleteAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, statement := range []string{`DELETE FROM chat_events`, `DELETE FROM chat_tools`, `DELETE FROM chat_state`} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return os.RemoveAll(s.mediaDir)
}

func eventCreatedAt(raw json.RawMessage, fallback int64) int64 {
	var envelope struct {
		CreateAt *int64 `json:"CreateAt"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.CreateAt != nil && *envelope.CreateAt > 0 {
		return *envelope.CreateAt
	}
	return fallback
}

func toolCreatedAt(raw json.RawMessage, fallback int64) int64 {
	var envelope struct {
		At *int64 `json:"at"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.At != nil && *envelope.At > 0 {
		return *envelope.At
	}
	return fallback
}

// robotChatHistoryHandler exposes one robot's persisted chat snapshot. GET
// loads it for the chat UI, POST replaces it, and DELETE clears one robot or
// (without a root) every robot.
func (s *server) robotChatHistoryHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.robotChatHistoryLoad(w, r)
	case http.MethodPost:
		s.robotChatHistorySave(w, r)
	case http.MethodDelete:
		s.robotChatHistoryDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) requireChatHistory(w http.ResponseWriter) bool {
	if s.chatHistory != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "聊天记录存储不可用。")
	return false
}

func (s *server) robotChatHistoryLoad(w http.ResponseWriter, r *http.Request) {
	if !s.requireChatHistory(w) {
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot, err := s.chatHistory.Load(root)
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取聊天记录失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": snapshot})
}

func (s *server) robotChatHistorySave(w http.ResponseWriter, r *http.Request) {
	if !s.requireChatHistory(w) {
		return
	}
	var input struct {
		Root     string              `json:"root"`
		Snapshot chatHistorySnapshot `json:"snapshot"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxChatSnapshotBytes+1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "聊天记录数据无法识别。")
		return
	}
	root := strings.TrimSpace(input.Root)
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.chatHistory.Save(root, input.Snapshot); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotChatHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireChatHistory(w) {
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	if root == "" {
		if err := s.chatHistory.DeleteAll(); err != nil {
			writeError(w, http.StatusBadGateway, "清理聊天记录失败。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	// Removal must work even when the robot directory no longer exists, so the
	// management view can clean records of deleted robots.
	if err := s.chatHistory.Delete(root); err != nil {
		writeError(w, http.StatusBadGateway, "清理聊天记录失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotChatSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireChatHistory(w) {
		return
	}
	items, err := s.chatHistory.Summary()
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取聊天记录统计失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
