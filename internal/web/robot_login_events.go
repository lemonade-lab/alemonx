package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"alemonx/internal/robot"
)

const robotCBPDirectory = ".alemon/cbp"

// configureRobotCBPFileTransport lets recent AlemonJS adapters expose their
// login lifecycle without requiring ALemonX to own the Node process IPC.
func configureRobotCBPFileTransport(commandEnv *[]string, root string) {
	if *commandEnv == nil {
		*commandEnv = os.Environ()
	}
	*commandEnv = append(*commandEnv,
		"ALEMON_CBP_FILE_TRANSPORT=1",
		"ALEMON_CBP_FILE_DIR="+filepath.Join(root, robotCBPDirectory),
	)
}

type robotCBPFileRecord struct {
	ID   string                 `json:"id"`
	At   int64                  `json:"at"`
	Name string                 `json:"name"`
	Data map[string]interface{} `json:"data"`
}

type robotCBPStatus struct {
	UpdatedAt int64                  `json:"updatedAt"`
	Login     map[string]interface{} `json:"login"`
}

type robotLoginWatcher struct {
	cancel    context.CancelFunc
	startedAt time.Time
}

func (s *server) startRobotLoginWatcher(root string, startedAt time.Time, background bool) {
	key := filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(key); err == nil {
		key = resolved
	}
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	s.mu.Lock()
	previous := s.loginWatchers[key]
	if previous != nil && previous.startedAt.After(startedAt) {
		s.mu.Unlock()
		return
	}
	if previous != nil {
		previous.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	watcher := &robotLoginWatcher{cancel: cancel, startedAt: startedAt}
	if s.loginWatchers == nil {
		s.loginWatchers = make(map[string]*robotLoginWatcher)
	}
	s.loginWatchers[key] = watcher
	s.mu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			s.mu.Lock()
			if s.loginWatchers[key] == watcher {
				delete(s.loginWatchers, key)
			}
			s.mu.Unlock()
		}()
		s.watchRobotLoginEvents(ctx, root, startedAt, background)
	}()
}

func (s *server) watchRobotLoginEvents(ctx context.Context, root string, startedAt time.Time, background bool) {
	directory := filepath.Join(root, robotCBPDirectory)
	eventsPath := filepath.Join(directory, "events.jsonl")
	var journal robotLoginJournal
	pending := map[string]bool{}
	var readyAt time.Time
	publish := func(name string, data map[string]interface{}) {
		if ctx.Err() != nil || data == nil {
			return
		}
		platform, _ := data["Platform"].(string)
		switch name {
		case "login.qrcode":
			pending[platform] = true
		case "login.success", "connection.ready":
			delete(pending, platform)
			readyAt = time.Now()
		default:
			return
		}
		data["root"] = root
		s.publishEvent("robot", name, data, nil)
	}
	if content, err := os.ReadFile(filepath.Join(directory, "status.json")); err == nil {
		var status robotCBPStatus
		if json.Unmarshal(content, &status) == nil && status.UpdatedAt >= startedAt.UnixMilli()-1000 {
			if _, ok := status.Login["QRCode"]; ok {
				publish("login.qrcode", status.Login)
			}
		}
	}
	readEvents := func() {
		journal.read(ctx, eventsPath, func(record robotCBPFileRecord) {
			if record.At >= startedAt.UnixMilli()-1000 {
				publish(record.Name, record.Data)
			}
		})
	}
	readEvents()
	events := time.NewTicker(time.Second)
	defer events.Stop()
	liveness := time.NewTicker(15 * time.Second)
	defer liveness.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-events.C:
			readEvents()
			if len(pending) == 0 && !readyAt.IsZero() && time.Since(readyAt) >= 30*time.Second {
				return
			}
			if !background && !s.developmentRunning(root) {
				return
			}
		case <-liveness.C:
			if background && (readyAt.IsZero() || len(pending) > 0) {
				processes, err := s.robots.PM2ProjectProcessesContext(ctx, root)
				if err == nil && !robotLoginProcessActive(processes) {
					return
				}
			}
		}
	}
}

func robotLoginProcessActive(processes []robot.PM2Process) bool {
	for _, process := range processes {
		switch process.Status {
		case "online", "launching", "waiting restart":
			return true
		}
	}
	return false
}

type robotLoginJournal struct {
	file   os.FileInfo
	offset int64
}

func (journal *robotLoginJournal) read(ctx context.Context, path string, consume func(robotCBPFileRecord)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return
	}
	if journal.file == nil || !os.SameFile(journal.file, info) || info.Size() < journal.offset {
		journal.offset = 0
	}
	journal.file = info
	if info.Size() == journal.offset {
		return
	}
	if _, err := file.Seek(journal.offset, io.SeekStart); err != nil {
		return
	}
	reader := bufio.NewReader(io.LimitReader(file, info.Size()-journal.offset))
	const maxRecordSize = 2 << 20
	var line []byte
	var size int
	for ctx.Err() == nil {
		part, err := reader.ReadSlice('\n')
		size += len(part)
		if size <= maxRecordSize {
			line = append(line, part...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			// Retry an incomplete final record when its newline arrives.
			return
		}
		journal.offset += int64(size)
		var record robotCBPFileRecord
		if size <= maxRecordSize && json.Unmarshal(line, &record) == nil && record.ID != "" {
			consume(record)
		}
		line, size = line[:0], 0
	}
}

func (s *server) robotQRCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	loginID := strings.TrimSpace(r.URL.Query().Get("loginId"))
	if root == "" || loginID == "" || strings.ContainsAny(loginID, `/\\`) || loginID == "." || loginID == ".." {
		writeError(w, http.StatusBadRequest, "二维码参数无效。")
		return
	}
	validated, err := s.robots.Validate(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path := filepath.Join(validated.Path, robotCBPDirectory, "qrcode", loginID+".png")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "二维码暂不可用。")
			return
		}
		writeError(w, http.StatusBadRequest, "读取二维码失败。")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
