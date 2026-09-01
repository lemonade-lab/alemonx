package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func (s *server) watchRobotLoginEvents(root string, startedAt time.Time) {
	directory := filepath.Join(root, robotCBPDirectory)
	eventsPath := filepath.Join(directory, "events.jsonl")
	statusPath := filepath.Join(directory, "status.json")
	seen := map[string]bool{}
	var lastStatus int64
	var lastEventsSize int64

	publish := func(name string, data map[string]interface{}, at int64) {
		if name == "" || data == nil {
			return
		}
		data["root"] = root
		s.publishEvent("robot", name, data, nil)
	}

	readStatus := func() {
		content, err := os.ReadFile(statusPath)
		if err != nil {
			return
		}
		var status robotCBPStatus
		if json.Unmarshal(content, &status) != nil || status.UpdatedAt <= lastStatus || status.UpdatedAt < startedAt.UnixMilli()-1000 {
			return
		}
		lastStatus = status.UpdatedAt
		if status.Login != nil {
			if _, ok := status.Login["QRCode"]; ok {
				publish("login.qrcode", status.Login, status.UpdatedAt)
			}
		}
	}

	readEvents := func() {
		file, err := os.Open(eventsPath)
		if err != nil {
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return
		}
		if info.Size() < lastEventsSize {
			lastEventsSize = 0
		}
		lastEventsSize = info.Size()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 2*1024*1024)
		for scanner.Scan() {
			var record robotCBPFileRecord
			if json.Unmarshal(scanner.Bytes(), &record) != nil || record.ID == "" || seen[record.ID] || record.At < startedAt.UnixMilli()-1000 {
				continue
			}
			seen[record.ID] = true
			if record.Name == "login.qrcode" || record.Name == "login.success" || record.Name == "connection.ready" {
				publish(record.Name, record.Data, record.At)
			}
		}
	}

	// Read the current snapshot first, then tail the journal. This handles a
	// QR challenge emitted before the dashboard's SSE connection was ready.
	readStatus()
	readEvents()
	// PM2 processes are not registered in s.development. Keep observing while
	// either a foreground process or this project's PM2 process is alive.
	active := func() bool {
		if s.developmentRunning(root) {
			return true
		}
		processes, err := s.robots.PM2ProjectProcesses(root)
		if err != nil {
			return false
		}
		for _, process := range processes {
			if process.Status == "online" || process.Status == "launching" {
				return true
			}
		}
		return false
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !active() {
			return
		}
		readEvents()
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
