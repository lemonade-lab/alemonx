package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

// systemRedisHandler reports the temporary Redis status (GET), runs a control
// action (POST), or persists manager settings (PUT).
func (s *server) systemRedisHandler(w http.ResponseWriter, r *http.Request) {
	if s.redisManager == nil {
		writeError(w, http.StatusServiceUnavailable, "内置 Redis 管理不可用。")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.redisManager.Status())
		return
	case http.MethodPost:
		var input struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil || strings.TrimSpace(input.Action) == "" {
			writeError(w, http.StatusBadRequest, "请指定 Redis 操作。")
			return
		}
		var err error
		switch input.Action {
		case "retry-runtime":
			s.redisManager.RetryRuntime()
		case "start":
			err = s.redisManager.Start()
		case "stop":
			_, err = s.redisManager.Stop()
		case "restart":
			_, err = s.redisManager.Restart()
		default:
			writeError(w, http.StatusBadRequest, "未知 Redis 操作。")
			return
		}
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.redisManager.Status())
		return
	case http.MethodPut:
		var input struct {
			Port      int  `json:"port"`
			AutoStart bool `json:"autoStart"`
			Disabled  bool `json:"disabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "Redis 配置无法识别。")
			return
		}
		if err := s.redisManager.Configure(input.Port, input.AutoStart, input.Disabled); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, s.redisManager.Status())
		return
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}
