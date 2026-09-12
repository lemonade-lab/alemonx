package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"alemonx/internal/dsh"
	"alemonx/internal/robot"
)

// DSHBridgeHandler is served only on a separate loopback listener. It is not
// mounted below the public workbench router because a browser must never be
// able to present a sidecar bridge token.
func (r *ServerRuntime) DSHBridgeHandler() http.Handler {
	if r == nil || r.server == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(r.server.dshBridgeHandler)
}

func (s *server) dshBridgeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/approval" && r.URL.Path != "/pm2" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "该 bridge 操作暂不支持。")
		return
	}
	if !requestIsLoopback(r) {
		writeError(w, http.StatusForbidden, "DSH bridge 仅接受本机 sidecar。")
		return
	}
	if s.dshRuntimes == nil {
		writeError(w, http.StatusServiceUnavailable, "DSH runtime 不可用。")
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-ALX-DSH-Bridge"))
	runtime, root, ok := s.dshRuntimes.RuntimeForBridgeToken(token)
	if !ok {
		writeError(w, http.StatusUnauthorized, "DSH bridge 凭据无效或已过期。")
		return
	}
	var input struct {
		SessionID string `json:"sessionId"`
		ToolName  string `json:"toolName"`
		Reason    string `json:"reason"`
		Action    string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "DSH bridge 请求格式无效。")
		return
	}
	if !validDSHSessionID(input.SessionID) || !runtime.HasSession(input.SessionID) {
		writeError(w, http.StatusForbidden, "DSH bridge 会话不属于当前 runtime。")
		return
	}
	if r.URL.Path == "/pm2" {
		s.dshBridgePM2(w, r, runtime, root, input.SessionID, input.Action)
		return
	}
	if len(input.ToolName) == 0 || len(input.ToolName) > 120 {
		writeError(w, http.StatusBadRequest, "DSH bridge 工具无效。")
		return
	}
	if err := s.awaitDSHBridgeApproval(r.Context(), runtime, root, input.SessionID, input.ToolName, safeApprovalSummary(input.ToolName)); err != nil {
		outcome := "rejected"
		if strings.Contains(err.Error(), "取消") {
			outcome = "cancelled"
		}
		writeJSON(w, http.StatusOK, map[string]string{"outcome": outcome})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"outcome": "allowed-once"})
}

func (s *server) dshBridgePM2(w http.ResponseWriter, r *http.Request, runtime *dsh.Runtime, root, sessionID, action string) {
	if s.pm2Guard == nil || !s.opsEnabled(root) {
		writeError(w, http.StatusForbidden, "当前机器人目录未启用受管 PM2 运维。")
		return
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "status":
		status, err := s.robots.PM2Status(root)
		if err != nil {
			writeError(w, http.StatusBadGateway, "无法读取 PM2 状态。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"summary": safePM2StatusSummary(status)})
		return
	case "logs":
		page, err := s.robots.PM2AuditLogs(root, robot.PM2AuditQuery{Page: 1, PerPage: 40})
		if err != nil {
			writeError(w, http.StatusBadGateway, "无法读取 PM2 日志。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"summary": truncateBridgeText(page.Output, 12000)})
		return
	case "restart", "reload":
		if err := s.awaitDSHBridgeApproval(r.Context(), runtime, root, sessionID, "pm2."+action, "请求"+map[string]string{"restart": "重启", "reload": "热重载"}[action]+"项目 PM2 进程"); err != nil {
			writeError(w, http.StatusForbidden, "PM2 操作未获批准。")
			return
		}
		output, err := s.pm2Guard.Run(r.Context(), root, action, "dsh:"+sessionID)
		if err != nil {
			writeError(w, http.StatusConflict, "受管 PM2 操作失败。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"summary": truncateBridgeText(output, 12000)})
		return
	default:
		writeError(w, http.StatusBadRequest, "PM2 bridge 操作不在白名单。")
	}
}

// awaitDSHBridgeApproval applies the selected session preference without
// widening the bridge's capability whitelist. "auto" is deliberately narrow:
// only project-file edits skip a prompt; commands and PM2 writes still need a
// user decision. "full" is an explicit session-local override for callers
// who have selected it in the composer.
func (s *server) awaitDSHBridgeApproval(ctx context.Context, runtime *dsh.Runtime, root, sessionID, action, summary string) error {
	access := runtime.SessionAccess(sessionID)
	if access == "full" || (access == "auto" && isDSHFileEdit(action)) {
		return nil
	}
	return s.awaitDSHApproval(ctx, runtime, robotAppToken(root), root, sessionID, action, summary)
}

func isDSHFileEdit(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "write", "edit", "str_replace_editor", "file.write", "file.edit":
		return true
	default:
		return false
	}
}

func safePM2StatusSummary(status robot.PM2Status) string {
	if !status.Configured {
		return "当前项目尚未配置 PM2。"
	}
	if status.Running {
		return "当前项目 PM2 进程正在运行。"
	}
	return "当前项目 PM2 已配置但未运行。"
}

func truncateBridgeText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[输出已截断]"
}

func safeApprovalSummary(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "write", "edit", "str_replace_editor":
		return "请求修改机器人项目文件"
	case "bash", "pwsh":
		return "请求执行受控项目命令"
	default:
		return "请求执行一次受限工具操作"
	}
}
