package web

import (
	"net/http"
	"strings"
)

// agentArchiveHandler intentionally exposes historical Agent data without any
// mutation endpoint. It lets users retain audit evidence while DSH is the
// only executable runtime.
func (s *server) agentArchiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "归档仅支持只读访问。")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-archive/"), "/")
	parts := strings.Split(path, "/")
	switch {
	case path == "sessions":
		items, err := s.agentSessions.List()
		if err != nil {
			writeError(w, 500, "无法读取会话归档。")
			return
		}
		writeJSON(w, 200, items)
	case len(parts) == 2 && parts[0] == "sessions":
		messages, err := s.agentSessions.Load(parts[1])
		if err != nil {
			writeError(w, 404, "归档会话不存在。")
			return
		}
		writeJSON(w, 200, messages)
	case path == "tasks":
		items, err := s.agentTaskStore.ListTasks()
		if err != nil {
			writeError(w, 500, "无法读取任务归档。")
			return
		}
		for index := range items {
			items[index] = publicTask(items[index])
		}
		writeJSON(w, 200, items)
	default:
		writeError(w, 404, "归档 API 不存在。")
	}
}
