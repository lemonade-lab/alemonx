package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type operationControl struct {
	cancel     context.CancelFunc
	path       string
	cancelling bool
	request    json.RawMessage // Memory only; never serialized into task history.
}

func (s *server) operationControlHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "该操作暂不支持。")
		return
	}
	var input struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeError(w, 400, "请求内容无法识别。")
		return
	}
	s.mu.RLock()
	var task operationTask
	for _, item := range s.operations {
		if item.ID == input.ID {
			task = item
			break
		}
	}
	control := s.taskControls[input.ID]
	s.mu.RUnlock()
	if task.ID == "" {
		writeError(w, 404, "任务不存在或已过期。")
		return
	}
	if input.Action == "cancel" {
		if task.Status != "running" {
			writeError(w, 409, "任务已经结束。")
			return
		}
		if control == nil || !task.CanCancel {
			writeError(w, 409, "此任务没有可用的取消句柄。")
			return
		}
		s.mu.Lock()
		control.cancelling = true
		s.mu.Unlock()
		control.cancel()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
		return
	}
	if input.Action == "retry" {
		if task.Status == "running" {
			writeError(w, 409, "请等待原任务退出后再重新执行。")
			return
		}
		if control == nil || !task.CanRetry {
			writeError(w, 409, "原任务参数已过期，请回到功能页面重新执行。")
			return
		}
		next := r.Clone(r.Context())
		next.Body = http.NoBody
		next.Body = io.NopCloser(bytes.NewReader(control.request))
		if control.path == "script" {
			task, err := s.startRobotScript(task.Root, strings.TrimPrefix(task.Action, "script:"))
			if err != nil {
				writeError(w, 409, err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, task)
		} else if control.path == "clone" {
			s.robotGitCloneHandler(w, next)
		} else if control.path == "clone-package" {
			s.robotPackageGitCloneHandler(w, next)
		} else if control.path != "" {
			next.URL.Path = control.path
			s.setupPluginActionHandler(w, next)
		} else {
			s.robotTasksHandler(w, next)
		}
		return
	}
	writeError(w, 400, "未知任务操作。")
}
