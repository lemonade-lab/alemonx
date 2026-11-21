package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"alemonx/internal/robot"
	"alemonx/internal/workspace"
)

func (s *server) workspaceProjectFilesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	identity, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取工作台账户。")
		return
	}
	if identity.Enabled && !identity.Authenticated {
		writeError(w, http.StatusUnauthorized, "请先登录工作台。")
		return
	}
	projects, err := workspace.LoadRobots(s.robotListWorkspace(), identity.RobotScope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取账户机器人列表。")
		return
	}
	requested := r.URL.Query().Get("root")
	for _, project := range projects.Projects {
		if workspace.SameRobotPath(project.Path, requested) {
			switch r.Method {
			case http.MethodPut:
				s.saveWorkspaceProjectFile(w, r, project.Path)
			case http.MethodPost:
				s.createWorkspaceProjectEntry(w, r, project.Path)
			default:
				s.readWorkspaceProjectFiles(w, r, project.Path)
			}
			return
		}
	}
	writeError(w, http.StatusForbidden, "请从当前账户的机器人列表选择目录。")
}

func (s *server) createWorkspaceProjectEntry(w http.ResponseWriter, r *http.Request, root string) {
	var input struct {
		Name      string `json:"name"`
		Directory *bool  `json:"directory"`
	}
	body := http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(body).Decode(&input); err != nil || input.Directory == nil {
		writeError(w, http.StatusBadRequest, "项目文件创建请求无效。")
		return
	}
	entry, err := s.robots.CreateWorkspaceProjectEntry(root, input.Name, *input.Directory)
	if err != nil {
		writeWorkspaceProjectFileError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *server) readWorkspaceProjectFiles(w http.ResponseWriter, r *http.Request, root string) {
	if file := r.URL.Query().Get("file"); file != "" {
		result, err := s.robots.ReadWorkspaceProjectText(root, file)
		if err != nil {
			writeWorkspaceProjectFileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	entries, err := s.robots.ListProjectDirectory(root, r.URL.Query().Get("directory"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *server) saveWorkspaceProjectFile(w http.ResponseWriter, r *http.Request, root string) {
	var input struct {
		Content  *string `json:"content"`
		Revision string  `json:"revision"`
	}
	file := r.URL.Query().Get("file")
	// JSON escaping can expand a 1 MiB text document up to six times.
	body := http.MaxBytesReader(w, r.Body, (6<<20)+4096)
	if err := json.NewDecoder(body).Decode(&input); err != nil || file == "" || input.Content == nil {
		writeError(w, http.StatusBadRequest, "文件保存请求无效或超出大小限制。")
		return
	}
	result, err := s.robots.SaveWorkspaceProjectText(root, file, *input.Content, input.Revision)
	if err != nil {
		writeWorkspaceProjectFileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeWorkspaceProjectFileError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, robot.ErrProjectFileRevision), errors.Is(err, robot.ErrProjectEntryExists):
		status = http.StatusConflict
	case errors.Is(err, robot.ErrProjectFileNotText):
		status = http.StatusUnsupportedMediaType
	}
	writeError(w, status, err.Error())
}
