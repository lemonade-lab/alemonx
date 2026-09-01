package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"alemonx/internal/robot"
	"github.com/creack/pty"
)

type robotTerminalSession struct {
	id   string
	root string
	pty  *os.File
	cmd  *exec.Cmd
}

var terminalSessionMu sync.Mutex
var terminalSessions = map[string]*robotTerminalSession{}

type terminalSessionRequest struct {
	Root string `json:"root"`
}

type terminalInputRequest struct {
	Root      string `json:"root"`
	SessionID string `json:"sessionId"`
	Input     string `json:"input"`
}

type terminalResizeRequest struct {
	Root      string `json:"root"`
	SessionID string `json:"sessionId"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

func (s *server) robotTerminalSessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if runtime.GOOS == "windows" {
		writeError(w, http.StatusNotImplemented, "Windows 暂不支持交互式终端。")
		return
	}
	var input terminalSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "终端会话格式无效。")
		return
	}
	validation, err := (robot.Manager{}).Validate(input.Root)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-i")
	cmd.Dir = filepath.Clean(validation.Path)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PS1=$ ")
	terminal, err := pty.Start(cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, "交互式终端启动失败："+err.Error())
		return
	}
	id := fmt.Sprintf("terminal-%d", time.Now().UnixNano())
	session := &robotTerminalSession{id: id, root: validation.Path, pty: terminal, cmd: cmd}
	terminalSessionMu.Lock()
	terminalSessions[id] = session
	terminalSessionMu.Unlock()
	go s.readRobotTerminalSession(session)
	writeJSON(w, http.StatusOK, map[string]string{"sessionId": id})
}

func (s *server) readRobotTerminalSession(session *robotTerminalSession) {
	buffer := make([]byte, 32*1024)
	for {
		n, err := session.pty.Read(buffer)
		if n > 0 {
			s.publishEvent("robot", "terminal.output", map[string]string{
				"root":      session.root,
				"sessionId": session.id,
				"text":      string(buffer[:n]),
			}, nil)
		}
		if err != nil {
			terminalSessionMu.Lock()
			delete(terminalSessions, session.id)
			terminalSessionMu.Unlock()
			_ = session.pty.Close()
			return
		}
	}
}

func (s *server) robotTerminalInputHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input terminalInputRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.SessionID == "" {
		writeError(w, http.StatusBadRequest, "终端输入格式无效。")
		return
	}
	terminalSessionMu.Lock()
	session := terminalSessions[input.SessionID]
	terminalSessionMu.Unlock()
	if session == nil || strings.TrimSpace(input.Root) != session.root {
		writeError(w, http.StatusNotFound, "终端会话不存在。")
		return
	}
	if _, err := session.pty.Write([]byte(input.Input)); err != nil {
		writeError(w, http.StatusGone, "终端会话已结束。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotTerminalCloseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input terminalInputRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.SessionID == "" {
		writeError(w, http.StatusBadRequest, "终端会话格式无效。")
		return
	}
	terminalSessionMu.Lock()
	session := terminalSessions[input.SessionID]
	delete(terminalSessions, input.SessionID)
	terminalSessionMu.Unlock()
	if session != nil {
		_ = session.cmd.Process.Kill()
		_ = session.pty.Close()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) robotTerminalResizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input terminalResizeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.SessionID == "" || input.Cols == 0 || input.Rows == 0 {
		writeError(w, http.StatusBadRequest, "终端尺寸格式无效。")
		return
	}
	terminalSessionMu.Lock()
	session := terminalSessions[input.SessionID]
	terminalSessionMu.Unlock()
	if session == nil || strings.TrimSpace(input.Root) != session.root {
		writeError(w, http.StatusNotFound, "终端会话不存在。")
		return
	}
	if err := pty.Setsize(session.pty, &pty.Winsize{Cols: input.Cols, Rows: input.Rows}); err != nil {
		writeError(w, http.StatusBadRequest, "终端尺寸更新失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
