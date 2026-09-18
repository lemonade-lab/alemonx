package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alemonx/internal/datamanager"
)

func (s *server) systemSQLitePath() string {
	if path := strings.TrimSpace(os.Getenv("ALX_OPS_SQLITE_PATH")); path != "" {
		return path
	}
	if s.agentTaskStore != nil {
		return filepath.Join(filepath.Dir(s.agentTaskStore.TasksDir()), "ops.db")
	}
	return ""
}

func (s *server) isSystemSQLite(path string) bool {
	protected, err := os.Stat(s.systemSQLitePath())
	if err != nil {
		return false
	}
	selected, err := os.Stat(path)
	return err == nil && os.SameFile(protected, selected)
}

func (s *server) dataHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.requireSuperAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/v1/data/catalog" {
		paths := []string{}
		path := s.systemSQLitePath()
		if path != "" {
			if absolute, err := filepath.Abs(path); err == nil {
				if valid, err := datamanager.SQLitePath(absolute); err == nil {
					paths = append(paths, valid)
				}
			}
		}
		address := ""
		if s.redisManager != nil {
			address = s.redisManager.Status().Address
		}
		writeJSON(w, http.StatusOK, map[string]any{"sqlite": paths, "redisAddress": address})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "请使用 JSON 请求")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	decode := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	switch r.URL.Path {
	case "/api/v1/data/connect":
		var input datamanager.ConnectRequest
		if err := decode.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "数据库请求无效")
			return
		}
		protected := input.Connection.Engine == "sqlite" && s.isSystemSQLite(input.Connection.Path)
		if protected && input.Write {
			writeError(w, http.StatusForbidden, "系统运维数据库仅允许只读访问")
			return
		}
		result, err := datamanager.Connect(ctx, input)
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, datamanager.ErrSQLiteBusy) {
				code = http.StatusServiceUnavailable
				w.Header().Set("Retry-After", "1")
			}
			writeError(w, code, err.Error())
			return
		}
		result.SystemDatabase = protected
		writeJSON(w, http.StatusOK, result)
	case "/api/v1/data/sql":
		var input datamanager.SQLRequest
		if err := decode.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "SQL 请求无效")
			return
		}
		protected := s.isSystemSQLite(input.Path)
		if input.Write && protected {
			writeError(w, http.StatusForbidden, "系统运维数据库仅允许只读访问，请通过对应业务功能修改数据")
			return
		}
		result, err := datamanager.QuerySQLite(ctx, input)
		if err != nil {
			if errors.Is(err, datamanager.ErrSQLiteBusy) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result.SystemDatabase = protected
		writeJSON(w, http.StatusOK, result)
	case "/api/v1/data/sqlite/default":
		path, err := datamanager.EnsureDefaultSQLite(ctx, s.workspace.Root)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "默认 SQLite 准备失败，请检查工作区权限与数据库文件后重试")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
	case "/api/v1/data/redis":
		var input datamanager.RedisRequest
		if err := decode.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "Redis 请求无效")
			return
		}
		if input.Address == "" && s.redisManager != nil {
			input.Address = s.redisManager.Status().Address
		}
		result, err := datamanager.RedisCommand(ctx, input)
		if err != nil {
			if errors.Is(err, datamanager.ErrRedisConflict) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": result})
	default:
		writeError(w, http.StatusNotFound, "数据接口不存在")
	}
}
