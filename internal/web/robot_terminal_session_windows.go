//go:build windows

package web

// Windows keeps the same HTTP/session contract as Unix. The process adapter is
// intentionally isolated here so a future ConPTY implementation can replace
// the pipes without changing callers or exposing a second API.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const terminalOutputLimit = 1024 * 1024
const terminalLease = 2 * time.Minute
const terminalSessionLimit = 12

type terminalSession struct {
	id, owner, key, cwd, shell string
	input                      io.WriteCloser
	cmd                        *exec.Cmd
	mu                         sync.Mutex
	output                     string
	truncated                  bool
	lastSeen                   time.Time
	subscribers                map[chan string]struct{}
}
type terminalSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*terminalSession
}
type terminalCreateRequest struct {
	CWD   string `json:"cwd"`
	Shell string `json:"shell"`
}
type terminalInputRequest struct {
	Input string `json:"input"`
}
type terminalResizeRequest struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func newTerminalSessionStore() *terminalSessionStore {
	return &terminalSessionStore{sessions: map[string]*terminalSession{}}
}
func terminalRandomID() string {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("terminal-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}
func terminalShell(requested string) (string, error) {
	if strings.TrimSpace(requested) != "" {
		return requested, nil
	}
	if value, err := exec.LookPath("pwsh.exe"); err == nil {
		return value, nil
	}
	if value, err := exec.LookPath("powershell.exe"); err == nil {
		return value, nil
	}
	return exec.LookPath("cmd.exe")
}
func terminalDirectory(cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		cwd, _ = os.Getwd()
	}
	resolved, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", fmt.Errorf("目录不存在或不可访问：%s", resolved)
	}
	return resolved, nil
}
func (s *server) terminalOwner(r *http.Request) (string, bool) {
	if s.auth == nil {
		return "local", false
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil || !status.Enabled {
		return "local", false
	}
	return status.Account, true
}
func (s *server) terminalSessionFor(r *http.Request, id string) (*terminalSession, bool) {
	s.terminalSessions.mu.Lock()
	session := s.terminalSessions.sessions[id]
	s.terminalSessions.mu.Unlock()
	if session == nil {
		return nil, false
	}
	owner, authenticated := s.terminalOwner(r)
	if authenticated {
		if session.owner != owner {
			return nil, false
		}
	} else if r.Header.Get("X-ALX-Terminal-Key") != session.key && r.URL.Query().Get("key") != session.key {
		return nil, false
	}
	session.mu.Lock()
	session.lastSeen = time.Now()
	session.mu.Unlock()
	return session, true
}
func (s *server) terminalSessionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.listTerminalSessions(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input terminalCreateRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeError(w, http.StatusBadRequest, "终端会话格式无效。")
		return
	}
	cwd, err := terminalDirectory(input.CWD)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	shell, err := terminalShell(input.Shell)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	owner, _ := s.terminalOwner(r)
	s.terminalSessions.mu.Lock()
	count := 0
	for _, item := range s.terminalSessions.sessions {
		if item.owner == owner {
			count++
		}
	}
	s.terminalSessions.mu.Unlock()
	if count >= terminalSessionLimit {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("每个账户最多同时打开 %d 个终端。", terminalSessionLimit))
		return
	}
	cmd := exec.Command(shell)
	cmd.Dir = cwd
	inputPipe, err := cmd.StdinPipe()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	errors, _ := cmd.StderrPipe()
	if err = cmd.Start(); err != nil {
		writeError(w, http.StatusBadRequest, "交互式终端启动失败："+err.Error())
		return
	}
	session := &terminalSession{id: terminalRandomID(), owner: owner, key: terminalRandomID(), cwd: cwd, shell: shell, input: inputPipe, cmd: cmd, lastSeen: time.Now(), subscribers: map[chan string]struct{}{}}
	s.terminalSessions.mu.Lock()
	s.terminalSessions.sessions[session.id] = session
	s.terminalSessions.mu.Unlock()
	go s.readWindowsTerminal(session, output)
	if errors != nil {
		go s.readWindowsTerminal(session, errors)
	}
	go func() { _ = cmd.Wait(); s.closeTerminalSession(session.id) }()
	writeJSON(w, http.StatusOK, map[string]any{"id": session.id, "key": session.key, "cwd": cwd, "shell": shell, "outputLimit": terminalOutputLimit})
}
func (s *server) readWindowsTerminal(session *terminalSession, reader io.Reader) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			s.appendTerminalOutput(session, string(buffer[:n]))
		}
		if err != nil {
			return
		}
	}
}
func (s *server) listTerminalSessions(w http.ResponseWriter, r *http.Request) {
	owner, authenticated := s.terminalOwner(r)
	keys := map[string]bool{}
	for _, key := range strings.Split(r.Header.Get("X-ALX-Terminal-Keys"), ",") {
		keys[strings.TrimSpace(key)] = true
	}
	s.terminalSessions.mu.Lock()
	defer s.terminalSessions.mu.Unlock()
	items := []map[string]any{}
	for _, item := range s.terminalSessions.sessions {
		if (authenticated && item.owner == owner) || (!authenticated && keys[item.key]) {
			items = append(items, terminalSummary(item))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
}
func terminalSummary(session *terminalSession) map[string]any {
	session.mu.Lock()
	defer session.mu.Unlock()
	return map[string]any{"id": session.id, "cwd": session.cwd, "shell": session.shell, "output": session.output, "truncated": session.truncated}
}
func (s *server) appendTerminalOutput(session *terminalSession, text string) {
	session.mu.Lock()
	session.output += text
	if len(session.output) > terminalOutputLimit {
		session.output = session.output[len(session.output)-terminalOutputLimit:]
		session.truncated = true
	}
	for subscriber := range session.subscribers {
		select {
		case subscriber <- text:
		default:
			delete(session.subscribers, subscriber)
		}
	}
	session.mu.Unlock()
}
func (s *server) closeTerminalSession(id string) {
	s.terminalSessions.mu.Lock()
	session := s.terminalSessions.sessions[id]
	delete(s.terminalSessions.sessions, id)
	s.terminalSessions.mu.Unlock()
	if session == nil {
		return
	}
	_ = session.cmd.Process.Kill()
	_ = session.input.Close()
	session.mu.Lock()
	for subscriber := range session.subscribers {
		close(subscriber)
		delete(session.subscribers, subscriber)
	}
	session.mu.Unlock()
}
func (s *server) terminalSessionHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/terminal/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "终端会话不存在。")
		return
	}
	session, ok := s.terminalSessionFor(r, parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, "终端会话不存在。")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch action {
	case "":
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, terminalSummary(session))
			return
		}
		if r.Method == http.MethodDelete {
			s.closeTerminalSession(session.id)
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	case "input":
		if r.Method == http.MethodPost {
			var input terminalInputRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Input == "" {
				writeError(w, http.StatusBadRequest, "终端输入格式无效。")
				return
			}
			if _, err := session.input.Write([]byte(input.Input)); err != nil {
				writeError(w, http.StatusGone, "终端会话已结束。")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	case "resize":
		if r.Method == http.MethodPost {
			var input terminalResizeRequest
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.Cols == 0 || input.Rows == 0 {
				writeError(w, http.StatusBadRequest, "终端尺寸格式无效。")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	case "heartbeat":
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	case "stream":
		if r.Method == http.MethodGet {
			s.streamTerminalSession(w, r, session)
			return
		}
	}
	writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
}
func (s *server) streamTerminalSession(w http.ResponseWriter, r *http.Request, session *terminalSession) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "终端流不受支持。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch := make(chan string, 32)
	session.mu.Lock()
	snapshot, truncated := session.output, session.truncated
	session.subscribers[ch] = struct{}{}
	session.mu.Unlock()
	defer func() { session.mu.Lock(); delete(session.subscribers, ch); session.mu.Unlock() }()
	if truncated {
		fmt.Fprint(w, "data: {\"truncated\":true}\n\n")
	}
	if snapshot != "" {
		encoded, _ := json.Marshal(map[string]any{"text": snapshot, "snapshot": true})
		fmt.Fprintf(w, "data: %s\n\n", encoded)
	}
	flusher.Flush()
	for {
		select {
		case text, ok := <-ch:
			if !ok {
				return
			}
			encoded, _ := json.Marshal(map[string]string{"text": text})
			fmt.Fprintf(w, "data: %s\n\n", encoded)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
func (s *server) reapTerminalSessions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		var stale []string
		s.terminalSessions.mu.Lock()
		for id, item := range s.terminalSessions.sessions {
			item.mu.Lock()
			expired := time.Since(item.lastSeen) > terminalLease
			item.mu.Unlock()
			if expired {
				stale = append(stale, id)
			}
		}
		s.terminalSessions.mu.Unlock()
		for _, id := range stale {
			s.closeTerminalSession(id)
		}
	}
}
