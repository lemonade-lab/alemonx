package web

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	maxTestoneChatPayload = 3 << 20
	maxTestoneChatKeyLen  = 512
	maxTestoneImageBytes  = 5 << 20
	// defaultTestoneRetention bounds how long sandbox records are kept. The sandbox
	// is a temporary test environment, so chats and images older than this
	// window are pruned automatically on startup and on writes.
	defaultTestoneRetention = 7 * 24 * time.Hour
)

var testoneImageHashPattern = regexp.MustCompile(`^[0-9a-fA-F]{1,32}$`)

// testoneChatRecord is one persisted testone chat session. Payload is the
// opaque chat array produced by the sandbox UI (message data with image refs).
type testoneChatRecord struct {
	ChatKey   string          `json:"chatKey"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt int64           `json:"updatedAt"`
}

// testoneRecordSummary aggregates one robot's persisted testone records for
// the management view.
type testoneRecordSummary struct {
	Root   string `json:"root"`
	Chats  int64  `json:"chats"`
	Images int64  `json:"images"`
	Bytes  int64  `json:"bytes"`
}

// testoneRecordStore persists sandbox chat records and image blobs. Chat JSON
// lives in SQLite; image bytes live under imageRoot with SQL metadata.
type testoneRecordStore struct {
	path      string
	imageRoot string
	db        *sql.DB
	mu        sync.Mutex
	retention time.Duration
}

func openTestoneRecordStore(path, imageRoot string) (*testoneRecordStore, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(imageRoot) == "" {
		return nil, errors.New("测试中心记录存储路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(imageRoot, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &testoneRecordStore{path: path, imageRoot: imageRoot, db: db, retention: retentionDaysFromEnv("ALX_TESTONE_RETENTION_DAYS", defaultTestoneRetention)}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS testone_chats(root TEXT NOT NULL, chat_key TEXT NOT NULL, payload TEXT NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY(root, chat_key));
CREATE TABLE IF NOT EXISTS testone_images(root TEXT NOT NULL, hash TEXT NOT NULL, path TEXT NOT NULL, size INTEGER NOT NULL, mime TEXT NOT NULL, created_at INTEGER NOT NULL, PRIMARY KEY(root, hash));
CREATE INDEX IF NOT EXISTS testone_chats_root_updated ON testone_chats(root, updated_at DESC);
CREATE INDEX IF NOT EXISTS testone_images_root_hash ON testone_images(root, hash);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Best-effort cleanup of records that expired while the service was off.
	_ = store.PruneExpired(time.Now())
	return store, nil
}

func (s *testoneRecordStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func (s *testoneRecordStore) SaveChat(root, chatKey string, payload []byte) error {
	if len(payload) > maxTestoneChatPayload {
		return errors.New("聊天记录内容过大")
	}
	_ = s.PruneExpired(time.Now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`INSERT INTO testone_chats(root, chat_key, payload, updated_at) VALUES(?,?,?,?)
ON CONFLICT(root, chat_key) DO UPDATE SET payload=excluded.payload, updated_at=excluded.updated_at`,
		root, chatKey, string(payload), time.Now().UnixMilli()); err != nil {
		return err
	}
	return nil
}

func (s *testoneRecordStore) LoadChat(root, chatKey string) (*testoneChatRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	var updatedAt int64
	if err := s.db.QueryRow(`SELECT payload, updated_at FROM testone_chats WHERE root = ? AND chat_key = ?`, root, chatKey).Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &testoneChatRecord{ChatKey: chatKey, Payload: json.RawMessage(payload), UpdatedAt: updatedAt}, nil
}

func (s *testoneRecordStore) ChatIndex(root string) ([]testoneChatRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT chat_key, updated_at FROM testone_chats WHERE root = ? ORDER BY updated_at DESC`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]testoneChatRecord, 0)
	for rows.Next() {
		var chatKey string
		var updatedAt int64
		if err := rows.Scan(&chatKey, &updatedAt); err != nil {
			return nil, err
		}
		items = append(items, testoneChatRecord{ChatKey: chatKey, UpdatedAt: updatedAt})
	}
	return items, rows.Err()
}

func (s *testoneRecordStore) SaveImage(root, hash string, data []byte, mime string) error {
	if len(data) > maxTestoneImageBytes {
		return errors.New("图片超过 5 MiB 限制")
	}
	_ = s.PruneExpired(time.Now())
	directory := filepath.Join(s.imageRoot, testoneImageRootDir(root))
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	target := filepath.Join(directory, hash)
	temporary, err := os.CreateTemp(directory, ".img-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`INSERT INTO testone_images(root, hash, path, size, mime, created_at) VALUES(?,?,?,?,?,?)
ON CONFLICT(root, hash) DO UPDATE SET path=excluded.path, size=excluded.size, mime=excluded.mime`,
		root, hash, target, len(data), mime, time.Now().UnixMilli()); err != nil {
		_ = os.Remove(target)
		return err
	}
	return nil
}

// PruneExpired removes chats and images older than the retention window and
// cleans up the image files and empty per-root directories they referenced.
func (s *testoneRecordStore) PruneExpired(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	retention := s.retention
	if retention <= 0 {
		retention = defaultTestoneRetention
	}
	cutoff := now.Add(-retention).UnixMilli()
	rows, err := s.db.Query(`SELECT path FROM testone_images WHERE created_at < ?`, cutoff)
	if err != nil {
		return err
	}
	var expiredPaths []string
	for rows.Next() {
		var imagePath string
		if err := rows.Scan(&imagePath); err != nil {
			rows.Close()
			return err
		}
		expiredPaths = append(expiredPaths, imagePath)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, imagePath := range expiredPaths {
		_ = os.Remove(imagePath)
	}
	for _, statement := range []string{
		`DELETE FROM testone_images WHERE created_at < ?`,
		`DELETE FROM testone_chats WHERE updated_at < ?`,
	} {
		if _, err := s.db.Exec(statement, cutoff); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(s.imageRoot)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := filepath.Join(s.imageRoot, entry.Name())
		children, err := os.ReadDir(directory)
		if err == nil && len(children) == 0 {
			_ = os.Remove(directory)
		}
	}
	return nil
}

func (s *testoneRecordStore) LoadImagePath(root, hash string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var imagePath, mime string
	if err := s.db.QueryRow(`SELECT path, mime FROM testone_images WHERE root = ? AND hash = ?`, root, hash).Scan(&imagePath, &mime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", os.ErrNotExist
		}
		return "", "", err
	}
	return imagePath, mime, nil
}

func (s *testoneRecordStore) Summary() ([]testoneRecordSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	merged := map[string]*testoneRecordSummary{}
	merge := func(root string, chats, images, bytes int64) {
		item := merged[root]
		if item == nil {
			item = &testoneRecordSummary{Root: root}
			merged[root] = item
		}
		item.Chats += chats
		item.Images += images
		item.Bytes += bytes
	}
	rows, err := s.db.Query(`SELECT root, COUNT(*), COALESCE(SUM(length(payload)),0) FROM testone_chats GROUP BY root`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var root string
		var count, bytes int64
		if err := rows.Scan(&root, &count, &bytes); err != nil {
			rows.Close()
			return nil, err
		}
		merge(root, count, 0, bytes)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = s.db.Query(`SELECT root, COUNT(*), COALESCE(SUM(size),0) FROM testone_images GROUP BY root`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var root string
		var count, bytes int64
		if err := rows.Scan(&root, &count, &bytes); err != nil {
			rows.Close()
			return nil, err
		}
		merge(root, 0, count, bytes)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]testoneRecordSummary, 0, len(merged))
	for _, item := range merged {
		result = append(result, *item)
	}
	return result, nil
}

func (s *testoneRecordStore) DeleteChat(root, chatKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM testone_chats WHERE root = ? AND chat_key = ?`, root, chatKey)
	return err
}

func (s *testoneRecordStore) DeleteRoot(root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT path FROM testone_images WHERE root = ?`, root)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var imagePath string
		if err := rows.Scan(&imagePath); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, imagePath)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, imagePath := range paths {
		_ = os.Remove(imagePath)
	}
	for _, statement := range []string{`DELETE FROM testone_chats WHERE root = ?`, `DELETE FROM testone_images WHERE root = ?`} {
		if _, err := s.db.Exec(statement, root); err != nil {
			return err
		}
	}
	_ = os.Remove(filepath.Join(s.imageRoot, testoneImageRootDir(root)))
	return nil
}

func (s *testoneRecordStore) DeleteAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, statement := range []string{`DELETE FROM testone_chats`, `DELETE FROM testone_images`} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(s.imageRoot)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			_ = os.RemoveAll(filepath.Join(s.imageRoot, entry.Name()))
		} else {
			_ = os.Remove(filepath.Join(s.imageRoot, entry.Name()))
		}
	}
	return nil
}

func testoneImageRootDir(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])[:16]
}

func validTestoneChatKey(key string) bool {
	if key == "" || len(key) > maxTestoneChatKeyLen {
		return false
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (s *server) requireTestoneRecords(w http.ResponseWriter) bool {
	if s.testoneRecords != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, "测试中心记录存储不可用。")
	return false
}

func (s *server) robotTestoneChatHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.robotTestoneChatLoad(w, r)
	case http.MethodPost:
		s.robotTestoneChatSave(w, r)
	case http.MethodDelete:
		s.robotTestoneChatDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) robotTestoneChatLoad(w http.ResponseWriter, r *http.Request) {
	if !s.requireTestoneRecords(w) {
		return
	}
	root, chatKey := strings.TrimSpace(r.URL.Query().Get("root")), strings.TrimSpace(r.URL.Query().Get("chatKey"))
	if root == "" || !validTestoneChatKey(chatKey) {
		writeError(w, http.StatusBadRequest, "机器人目录或会话键无效。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.testoneRecords.LoadChat(root, chatKey)
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取聊天记录失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": record})
}

func (s *server) robotTestoneChatSave(w http.ResponseWriter, r *http.Request) {
	if !s.requireTestoneRecords(w) {
		return
	}
	var input struct {
		Root    string          `json:"root"`
		ChatKey string          `json:"chatKey"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxTestoneChatPayload+1<<20)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "聊天记录数据无法识别。")
		return
	}
	root, chatKey := strings.TrimSpace(input.Root), strings.TrimSpace(input.ChatKey)
	if root == "" || !validTestoneChatKey(chatKey) {
		writeError(w, http.StatusBadRequest, "机器人目录或会话键无效。")
		return
	}
	if len(input.Payload) > maxTestoneChatPayload {
		writeError(w, http.StatusRequestEntityTooLarge, "聊天记录超过 3 MiB 限制。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.testoneRecords.SaveChat(root, chatKey, input.Payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotTestoneChatDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireTestoneRecords(w) {
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	chatKey := strings.TrimSpace(r.URL.Query().Get("chatKey"))
	var err error
	switch {
	case chatKey != "":
		err = s.testoneRecords.DeleteChat(root, chatKey)
	case root != "":
		err = s.testoneRecords.DeleteRoot(root)
	default:
		err = s.testoneRecords.DeleteAll()
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "清理测试中心记录失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotTestoneChatIndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireTestoneRecords(w) {
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
	items, err := s.testoneRecords.ChatIndex(root)
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取会话索引失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) robotTestoneImageHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.robotTestoneImageUpload(w, r)
	case http.MethodGet:
		s.robotTestoneImageServe(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) robotTestoneImageUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireTestoneRecords(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxTestoneImageBytes+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传格式无效。")
		return
	}
	defer r.MultipartForm.RemoveAll()
	root := strings.TrimSpace(r.FormValue("root"))
	hash := strings.TrimSpace(r.FormValue("hash"))
	if root == "" || !testoneImageHashPattern.MatchString(hash) {
		writeError(w, http.StatusBadRequest, "机器人目录或图片哈希无效。")
		return
	}
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择要上传的图片。")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTestoneImageBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法读取上传图片。")
		return
	}
	if len(data) > maxTestoneImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "图片超过 5 MiB 限制。")
		return
	}
	mime := header.Header.Get("Content-Type")
	if mime == "" || strings.HasPrefix(strings.ToLower(mime), "application/octet-stream") {
		mime = "image/png"
	}
	if err := s.testoneRecords.SaveImage(root, strings.ToLower(hash), data, mime); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "size": len(data)})
}

func (s *server) robotTestoneImageServe(w http.ResponseWriter, r *http.Request) {
	if !s.requireTestoneRecords(w) {
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	hash := strings.TrimSpace(r.URL.Query().Get("hash"))
	if root == "" || !testoneImageHashPattern.MatchString(hash) {
		writeError(w, http.StatusBadRequest, "机器人目录或图片哈希无效。")
		return
	}
	imagePath, mime, err := s.testoneRecords.LoadImagePath(root, strings.ToLower(hash))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "图片不存在。")
			return
		}
		writeError(w, http.StatusBadGateway, "读取图片失败。")
		return
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "图片文件不存在。")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = w.Write(data)
}

func (s *server) robotTestoneSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireTestoneRecords(w) {
		return
	}
	items, err := s.testoneRecords.Summary()
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取测试中心记录统计失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
