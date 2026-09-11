package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alemonx/internal/dsh"
	"alemonx/internal/robot"
)

// dshEventDTO is the browser contract. Raw tool arguments, tool results and
// provider failures deliberately never cross this boundary.
type dshEventDTO struct {
	ID        int64           `json:"id"`
	RuntimeID string          `json:"runtimeId"`
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId,omitempty"`
	Status    string          `json:"status,omitempty"`
	Text      string          `json:"text,omitempty"`
	Tool      string          `json:"tool,omitempty"`
	Approval  *dshApprovalDTO `json:"approval,omitempty"`
	At        string          `json:"at"`
}

func newDSHRuntime() *dsh.Runtime {
	home, err := dsh.DefaultHome()
	if err != nil {
		home = filepath.Join(os.TempDir(), "alemonx-dsh")
	}
	return dsh.New(home, nil)
}

func newDSHRegistry() *dsh.RuntimeRegistry {
	home, err := dsh.DefaultHome()
	if err != nil {
		home = filepath.Join(os.TempDir(), "alemonx-dsh")
	}
	return dsh.NewRegistry(home, nil)
}

// restoreDSHRuntimes rehydrates only configurations that already have a
// credential in the platform secret store (or the Docker Secret mount). It
// intentionally skips unavailable credentials rather than leaving a sidecar
// running with an inherited environment key.
func (s *server) restoreDSHRuntimes() {
	if s.dshRuntimes == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, err := range s.dshRuntimes.Restore(ctx, s.dshCredential) {
		log.Printf("DSH runtime 未恢复：%v", err)
	}
}

func (s *server) dshCredential(root string) (string, error) {
	if secretFile := strings.TrimSpace(os.Getenv("ALX_DSH_SECRET_FILE")); secretFile != "" {
		raw, err := os.ReadFile(secretFile)
		if err != nil {
			return "", fmt.Errorf("无法读取 DSH Docker Secret: %w", err)
		}
		if key := strings.TrimSpace(string(raw)); key != "" {
			return key, nil
		}
		return "", fmt.Errorf("DSH Docker Secret 为空")
	}
	if s.dshSecrets == nil {
		return "", fmt.Errorf("没有可用的 DSH 凭据存储")
	}
	return s.dshSecrets.Get(root)
}

// dshHandler exposes runtime-scoped APIs only. A session cannot be routed to
// a process selected by a different robot directory.
func (s *server) dshHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/dsh"), "/")
	if path == "status" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"ready": false, "version": dsh.Version})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "runtimes" {
		writeError(w, 404, "DSH API 不存在。")
		return
	}
	root, ok := decodeRobotRootToken(parts[1])
	if !ok {
		writeError(w, 400, "机器人目录令牌无效。")
		return
	}
	if _, err := (robot.Manager{}).Validate(root); err != nil {
		writeError(w, 400, "请先选择一个有效的机器人目录。")
		return
	}
	runtime, err := s.dshRuntimes.Runtime(root)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if s.dshEvents != nil {
		s.dshEvents.ensureRelay(parts[1], runtime)
	}
	rest := parts[2:]
	if len(rest) == 1 && rest[0] == "status" && r.Method == http.MethodGet {
		writeJSON(w, 200, runtime.Status())
		return
	}
	if len(rest) == 1 && rest[0] == "config" && r.Method == http.MethodGet {
		s.dshConfiguration(w, runtime, root)
		return
	}
	if len(rest) == 1 && rest[0] == "config" && r.Method == http.MethodPost {
		s.configureDSH(w, r, runtime, root)
		return
	}
	if len(rest) == 1 && rest[0] == "sessions" && r.Method == http.MethodGet {
		s.listDSHSessions(w, r, runtime)
		return
	}
	if len(rest) == 1 && rest[0] == "sessions" && r.Method == http.MethodPost {
		s.createDSHSession(w, r, runtime)
		return
	}
	if len(rest) == 2 && rest[0] == "approvals" && r.Method == http.MethodPost {
		s.resolveDSHApproval(w, r, root, rest[1])
		return
	}
	if len(rest) == 3 && rest[0] == "sessions" && rest[2] == "events" && r.Method == http.MethodGet {
		dshEvents(w, r, runtime, s.dshEvents, parts[1], rest[1])
		return
	}
	if len(rest) == 3 && rest[0] == "sessions" && rest[2] == "prompt" && r.Method == http.MethodPost {
		s.promptDSH(w, r, runtime, rest[1])
		return
	}
	if len(rest) == 3 && rest[0] == "sessions" && rest[2] == "cancel" && r.Method == http.MethodPost {
		s.cancelDSH(w, r, runtime, rest[1])
		return
	}
	writeError(w, 404, "DSH API 不存在。")
}

// dshConfiguration intentionally exposes only restart-safe, non-secret
// fields. The browser can tell that a key exists without ever receiving it.
func (s *server) dshConfiguration(w http.ResponseWriter, runtime *dsh.Runtime, root string) {
	persisted, err := runtime.LoadPersistedConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]any{"configured": false, "credentialConfigured": false})
			return
		}
		writeError(w, http.StatusInternalServerError, "无法读取 DSH 配置。")
		return
	}
	credentialConfigured := false
	if _, credentialErr := s.dshCredential(root); credentialErr == nil {
		credentialConfigured = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":           true,
		"provider":             persisted.Provider,
		"model":                persisted.Model,
		"root":                 persisted.Root,
		"credentialConfigured": credentialConfigured,
	})
}

func (s *server) resolveDSHApproval(w http.ResponseWriter, r *http.Request, root, approvalID string) {
	if approvalID == "" || len(approvalID) > 128 {
		writeError(w, http.StatusBadRequest, "审批 ID 无效。")
		return
	}
	var input struct {
		Approve bool `json:"approve"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "审批请求格式无效。")
		return
	}
	if s.dshApprovals == nil || !s.dshApprovals.resolve(root, approvalID, input.Approve) {
		writeError(w, http.StatusNotFound, "该审批已过期、不存在或不属于当前机器人目录。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"approved": input.Approve})
}

func (s *server) configureDSH(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime, root string) {
	var input struct{ Provider, Model, APIKey string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, 400, "DSH 配置格式无效。")
		return
	}
	key := strings.TrimSpace(input.APIKey)
	if key != "" && s.dshSecrets != nil {
		if err := s.dshSecrets.Set(root, key); err != nil {
			writeError(w, 503, "无法写入系统钥匙串："+err.Error())
			return
		}
	}
	if key == "" {
		stored, err := s.dshCredential(root)
		if err != nil {
			writeError(w, 503, "请提供 DeepSeek API Key 或配置 Docker Secret。")
			return
		}
		key = stored
	}
	cfg := dsh.Config{Provider: input.Provider, Model: input.Model, Root: root, APIKey: key}
	if err := cfg.Valid(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := runtime.SaveConfig(cfg); err != nil {
		writeError(w, 500, "无法保存 DSH 配置："+err.Error())
		return
	}
	_ = runtime.Stop(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := runtime.Start(ctx, cfg); err != nil {
		writeError(w, 503, "DSH 启动失败："+err.Error())
		return
	}
	writeJSON(w, 200, runtime.Status())
}

func (s *server) createDSHSession(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime) {
	if !runtime.Status().Ready {
		writeError(w, 503, "请先配置并启动模型。")
		return
	}
	id, err := runtime.CreateSession(r.Context())
	if err != nil {
		writeError(w, 502, "无法创建会话："+err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"sessionId": id})
}

func (s *server) listDSHSessions(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime) {
	if !runtime.Status().Ready {
		writeJSON(w, 200, []any{})
		return
	}
	items, err := runtime.ListSessions(r.Context())
	if err != nil {
		writeError(w, 502, "无法读取会话："+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(items)
}

func (s *server) promptDSH(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime, sessionID string) {
	if !validDSHSessionID(sessionID) {
		writeError(w, 400, "会话 ID 无效。")
		return
	}
	if !runtime.Status().Ready {
		writeError(w, 503, "请先配置并启动模型。")
		return
	}
	var input struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || strings.TrimSpace(input.Text) == "" {
		writeError(w, 400, "请输入消息内容。")
		return
	}
	id, err := runtime.Prompt(r.Context(), sessionID, input.Text)
	if err != nil {
		writeError(w, 502, "DSH 请求失败："+err.Error())
		return
	}
	writeJSON(w, 202, map[string]string{"sessionId": sessionID, "messageId": id})
}

func (s *server) cancelDSH(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime, sessionID string) {
	if !validDSHSessionID(sessionID) {
		writeError(w, 400, "会话 ID 无效。")
		return
	}
	if err := runtime.CancelSession(r.Context(), sessionID); err != nil {
		if errors.Is(err, dsh.ErrSessionCancellationUnsupported) {
			writeError(w, http.StatusNotImplemented, "当前固定版本的 DSH SDK 不支持逐会话取消；请等待当前回合结束，或停止该机器人 runtime。")
			return
		}
		writeError(w, 502, "无法取消会话："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"sessionId": sessionID, "status": "cancelled"})
}

func validDSHSessionID(id string) bool {
	return id != "" && len(id) <= 160 && !strings.ContainsAny(id, `\\/`)
}

func dshEvents(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime, store *dshEventStore, runtimeID, wantedSession string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "当前服务不支持事件流。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	var after int64
	_, _ = fmt.Sscan(r.Header.Get("Last-Event-ID"), &after)
	if store == nil {
		writeError(w, http.StatusServiceUnavailable, "DSH 事件存储不可用。")
		return
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	send := func(public dshEventDTO) {
		if public.SessionID != "" && public.SessionID != wantedSession {
			return
		}
		raw, _ := json.Marshal(public)
		_, _ = fmt.Fprintf(w, "id: %d\nevent: dsh\ndata: %s\n\n", public.ID, raw)
		flusher.Flush()
	}
	for _, event := range store.after(runtimeID, wantedSession, after) {
		after = event.ID
		send(event)
	}
	// The store owns the permanent relay. Polling its bounded safe index lets a
	// slow client reconnect by Last-Event-ID without sharing raw sidecar frames.
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": ping\n\n")
			flusher.Flush()
		case <-poll.C:
			for _, event := range store.after(runtimeID, wantedSession, after) {
				after = event.ID
				send(event)
			}
		}
	}
}

func publicDSHEvent(event dsh.Event, runtimeID string) dshEventDTO {
	var fields struct {
		SessionID string `json:"sessionId"`
		Status    string `json:"status"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		Text      string `json:"text"`
		Message   struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		Event struct {
			Type string `json:"type"`
			Data struct {
				Name    string `json:"name"`
				Text    string `json:"text"`
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"data"`
		} `json:"event"`
		Approval *dshApprovalDTO `json:"approval"`
	}
	_ = json.Unmarshal(event.Params, &fields)
	text := fields.Text
	if text == "" {
		for _, block := range fields.Message.Content {
			if block.Type == "text" {
				text += block.Text
			}
		}
	}
	if text == "" {
		text = fields.Event.Data.Text
		for _, block := range fields.Event.Data.Message.Content {
			if block.Type == "text" {
				text += block.Text
			}
		}
	}
	if len(text) > 12000 {
		text = text[:12000]
	}
	return dshEventDTO{ID: event.Seq, RuntimeID: runtimeID, Type: firstNonEmpty(fields.Event.Type, fields.Type, event.Method), SessionID: fields.SessionID, Status: fields.Status, Text: text, Tool: firstNonEmpty(fields.Event.Data.Name, fields.Name), Approval: fields.Approval, At: time.Now().UTC().Format(time.RFC3339Nano)}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "event"
}
