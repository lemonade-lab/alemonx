package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"alemonx/internal/robot"
)

func (s *server) robotRuntimeSwitchHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		root := r.URL.Query().Get("root")
		plan, err := s.robots.RuntimeSwitchPlan(r.Context(), root, r.URL.Query().Get("target"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if reason := s.runtimeSwitchRunningReason(root); reason != "" {
			plan.Blocked = append(plan.Blocked, reason)
		}
		writeJSON(w, http.StatusOK, plan)
	case http.MethodPost:
		var input struct {
			Root     string `json:"root"`
			Target   string `json:"target"`
			Revision string `json:"revision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Root == "" || input.Revision == "" {
			writeError(w, http.StatusBadRequest, "请先检查并确认切换预览。")
			return
		}
		if _, err := s.robots.Validate(input.Root); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		release, err := robot.BeginOperation(r.Context(), input.Root)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		defer release()
		if reason := s.runtimeSwitchRunningReason(input.Root); reason != "" {
			writeError(w, http.StatusConflict, reason)
			return
		}
		s.clearIdleRuntimeSwitchBridges(input.Root)
		_, err = s.robots.ApplyRuntimeSwitch(r.Context(), input.Root, input.Target, input.Revision)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		overview, err := s.robots.RuntimeOverview(input.Root)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"runtime": input.Target})
			return
		}
		writeJSON(w, http.StatusOK, overview)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) runtimeSwitchRunningReason(root string) string {
	s.mu.RLock()
	active := false
	for path := range s.development {
		active = active || runtimeSwitchSameRoot(root, path)
	}
	for _, process := range s.scriptProcesses {
		active = active || runtimeSwitchSameRoot(root, process.Root)
	}
	for path, bridge := range s.botAppPageRuntimes {
		if runtimeSwitchSameRoot(root, path) {
			for _, streams := range bridge.streams {
				active = active || len(streams) > 0
			}
		}
	}
	s.mu.RUnlock()
	if active {
		return "请先手动停止前台、开发进程及项目脚本，并关闭机器人应用页后重新检查。"
	}
	if s.pm2Status == nil {
		home := os.Getenv("PM2_HOME")
		if home == "" {
			if userHome, err := os.UserHomeDir(); err == nil {
				home = filepath.Join(userHome, ".pm2")
			}
		}
		if home != "" {
			if _, err := os.Stat(filepath.Join(home, "pm2.pid")); os.IsNotExist(err) {
				return ""
			}
		}
	}
	status, err := s.pm2StatusFor(root)
	if err != nil {
		return "无法确认 PM2 状态，请先在后台运行面板确认项目已停止。"
	}
	if status.Running || status.Status == "stopping" || status.Status == "waiting restart" {
		return "请先手动停止该项目的 PM2 进程，再切换运行时。"
	}
	return ""
}

func runtimeSwitchSameRoot(a, b string) bool {
	normalize := func(path string) string {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return filepath.Clean(path)
	}
	a, b = normalize(a), normalize(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (s *server) clearIdleRuntimeSwitchBridges(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for path, bridge := range s.botAppPageRuntimes {
		if runtimeSwitchSameRoot(root, path) {
			_ = bridge.command.Process.Kill()
			delete(s.botAppPageRuntimes, path)
		}
	}
}
