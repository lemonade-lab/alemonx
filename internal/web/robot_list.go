package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"alemonx/internal/robot"
	"alemonx/internal/workspace"
)

func (s *server) robotListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, 500, "无法读取工作台账户。")
		return
	}
	if identity.Enabled && !identity.Authenticated {
		writeError(w, 401, "请先登录工作台。")
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := workspace.LoadRobots(s.robotListWorkspace(), identity.RobotScope)
		if err != nil {
			writeError(w, 500, "无法读取账户机器人列表。")
			return
		}
		writeJSON(w, 200, list)
	case http.MethodPost:
		var change workspace.RobotListChange
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&change); err != nil {
			writeError(w, 400, "机器人列表操作无效。")
			return
		}
		if change.Action == "add" {
			for _, entry := range change.Projects {
				if _, err := (robot.Manager{}).Validate(filepath.Clean(entry.Path)); err != nil {
					writeError(w, 400, "请选取有效的机器人目录。")
					return
				}
			}
		}
		list, err := workspace.UpdateRobots(s.robotListWorkspace(), identity.RobotScope, change)
		if err != nil {
			code := 400
			if errors.Is(err, workspace.ErrRobotListScope) {
				code = 409
			}
			writeError(w, code, err.Error())
			return
		}
		writeJSON(w, 200, list)
	default:
		writeError(w, 405, "该操作暂不支持。")
	}
}

func (s *server) robotListWorkspace() string {
	if s.workspace.Root != "" {
		return s.workspace.Root
	}
	return s.workspaceRequested
}
