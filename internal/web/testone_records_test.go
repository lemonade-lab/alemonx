package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestoneTestStore(t *testing.T) *testoneRecordStore {
	t.Helper()
	t.Setenv("ALX_TESTONE_RETENTION_DAYS", "")
	dataDir := t.TempDir()
	store, err := openTestoneRecordStore(filepath.Join(dataDir, "testone.db"), filepath.Join(dataDir, "testone-images"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestTestoneRecordStoreRoundTripAndCleanup(t *testing.T) {
	store := newTestoneTestStore(t)
	root := "/robots/demo"
	chatKey := "127.0.0.1:8080:private:bot"
	if err := store.SaveChat(root, chatKey, []byte(`[{"type":"Text","value":"hi"}]`)); err != nil {
		t.Fatal(err)
	}
	record, err := store.LoadChat(root, chatKey)
	if err != nil || record == nil || string(record.Payload) != `[{"type":"Text","value":"hi"}]` || record.UpdatedAt == 0 {
		t.Fatalf("loaded = %#v, %v", record, err)
	}
	index, err := store.ChatIndex(root)
	if err != nil || len(index) != 1 || index[0].ChatKey != chatKey {
		t.Fatalf("index = %#v, %v", index, err)
	}
	imagePath, mime, err := store.LoadImagePath(root, "abc")
	if err == nil {
		t.Fatalf("missing image should error, got %q %q", imagePath, mime)
	}
	if err := store.SaveImage(root, "abc123", []byte("png-bytes"), "image/png"); err != nil {
		t.Fatal(err)
	}
	imagePath, mime, err = store.LoadImagePath(root, "abc123")
	if err != nil || mime != "image/png" {
		t.Fatalf("image = %q %q, %v", imagePath, mime, err)
	}
	if _, err := os.Stat(imagePath); err != nil {
		t.Fatalf("image file missing: %v", err)
	}
	summary, err := store.Summary()
	if err != nil || len(summary) != 1 {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
	if summary[0].Root != root || summary[0].Chats != 1 || summary[0].Images != 1 || summary[0].Bytes == 0 {
		t.Fatalf("summary = %#v", summary[0])
	}
	if err := store.DeleteRoot(root); err != nil {
		t.Fatal(err)
	}
	if record, err := store.LoadChat(root, chatKey); err != nil || record != nil {
		t.Fatalf("chat after delete = %#v, %v", record, err)
	}
	if _, _, err := store.LoadImagePath(root, "abc123"); err == nil {
		t.Fatal("image after delete should be missing")
	}
	if err := store.SaveChat(root, chatKey, []byte(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveImage(root, "def456", []byte("x"), "image/jpeg"); err != nil {
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

func TestTestoneRecordStoreRejectsOversizePayload(t *testing.T) {
	store := newTestoneTestStore(t)
	payload := bytes.Repeat([]byte("x"), maxTestoneChatPayload+1)
	if err := store.SaveChat("/robots/demo", "k", payload); err == nil {
		t.Fatal("oversize payload must be rejected")
	}
	if err := store.SaveImage("/robots/demo", "abc", bytes.Repeat([]byte("y"), maxTestoneImageBytes+1), "image/png"); err == nil {
		t.Fatal("oversize image must be rejected")
	}
}

func TestTestoneRecordStorePrunesExpired(t *testing.T) {
	store := newTestoneTestStore(t)
	root := "/robots/demo"
	if err := store.SaveChat(root, "k1", []byte(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveImage(root, "abc123", []byte("png"), "image/png"); err != nil {
		t.Fatal(err)
	}
	imagePath, _, err := store.LoadImagePath(root, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-defaultTestoneRetention - time.Hour).UnixMilli()
	if _, err := store.db.Exec(`UPDATE testone_chats SET updated_at = ? WHERE chat_key = ?`, old, "k1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE testone_images SET created_at = ? WHERE hash = ?`, old, "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneExpired(time.Now()); err != nil {
		t.Fatal(err)
	}
	if record, err := store.LoadChat(root, "k1"); err != nil || record != nil {
		t.Fatalf("expired chat survived = %#v, %v", record, err)
	}
	if _, _, err := store.LoadImagePath(root, "abc123"); err == nil {
		t.Fatal("expired image survived")
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("expired image file survived, stat err = %v", err)
	}
	if err := store.SaveChat(root, "k2", []byte(`[]`)); err != nil {
		t.Fatal(err)
	}
	if err := store.PruneExpired(time.Now()); err != nil {
		t.Fatal(err)
	}
	if record, err := store.LoadChat(root, "k2"); err != nil || record == nil {
		t.Fatalf("fresh chat pruned = %#v, %v", record, err)
	}
}

func TestTestoneChatHandlersSaveLoadIndexAndDelete(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	s.testoneRecords = newTestoneTestStore(t)
	chatKey := "127.0.0.1:9000:private:bot"

	body, err := json.Marshal(map[string]any{
		"root":    root,
		"chatKey": chatKey,
		"payload": []any{map[string]any{"type": "Text", "value": "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.robotTestoneChatHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/robot/testone/chat", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("save = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotTestoneChatHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/testone/chat?root="+url.QueryEscape(root)+"&chatKey="+url.QueryEscape(chatKey), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("load = %d %s", response.Code, response.Body.String())
	}
	var loaded struct {
		Chat *testoneChatRecord `json:"chat"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Chat == nil || string(loaded.Chat.Payload) != `[{"type":"Text","value":"hi"}]` {
		t.Fatalf("loaded chat = %#v", loaded.Chat)
	}
	response = httptest.NewRecorder()
	s.robotTestoneChatIndexHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/testone/chat/index?root="+url.QueryEscape(root), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("index = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotTestoneChatHandler(response, httptest.NewRequest(http.MethodDelete, "/api/v1/robot/testone/chat?root="+url.QueryEscape(root), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotTestoneSummaryHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/testone/summary", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", response.Code, response.Body.String())
	}
	var summary struct {
		Items []testoneRecordSummary `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Items) != 0 {
		t.Fatalf("summary after delete = %#v", summary.Items)
	}
}

func TestTestoneImageHandlersUploadAndServe(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	s.testoneRecords = newTestoneTestStore(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("root", root); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("hash", "abc123"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "img.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("png-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/robot/testone/image", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	s.robotTestoneImageHandler(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.robotTestoneImageHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/testone/image?root="+url.QueryEscape(root)+"&hash=abc123", nil))
	if response.Code != http.StatusOK || response.Body.String() != "png-bytes" {
		t.Fatalf("serve = %d %s", response.Code, response.Body.String())
	}
	if mime := response.Header().Get("Content-Type"); mime != "image/png" {
		t.Fatalf("mime = %q", mime)
	}
	// Invalid hash must be rejected.
	response = httptest.NewRecorder()
	s.robotTestoneImageHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/testone/image?root="+url.QueryEscape(root)+"&hash=../escape", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid hash = %d", response.Code)
	}
}
