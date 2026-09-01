package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"alemonx/internal/robot"
)

type scriptProcess struct {
	TaskID  string
	Root    string
	Script  string
	Command *exec.Cmd
}

type scriptControlItem struct {
	robot.PackageScript
	Running bool   `json:"running"`
	TaskID  string `json:"taskId,omitempty"`
	Record  string `json:"record,omitempty"`
}

func scriptProcessKey(root, script string) string { return root + "\x00" + script }

func (s *server) robotScriptsHandler(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("root"))
	if r.Method == http.MethodPut {
		var input struct {
			Root         string `json:"root"`
			PreviousName string `json:"previousName"`
			Name         string `json:"name"`
			Command      string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求内容无法识别。")
			return
		}
		if input.PreviousName != input.Name {
			s.mu.RLock()
			_, running := s.scriptProcesses[scriptProcessKey(input.Root, input.PreviousName)]
			s.mu.RUnlock()
			if running {
				writeError(w, http.StatusConflict, "脚本正在运行；请先停止后再修改名称")
				return
			}
		}
		if err := s.robots.UpdatePackageScript(input.Root, input.PreviousName, input.Name, input.Command); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Root   string `json:"root"`
			Script string `json:"script"`
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求内容无法识别。")
			return
		}
		root, input.Script = strings.TrimSpace(input.Root), strings.TrimSpace(input.Script)
		if root == "" || input.Script == "" {
			writeError(w, http.StatusBadRequest, "请选择机器人目录和脚本。")
			return
		}
		if input.Action == "run" {
			task, err := s.startRobotScript(root, input.Script)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, task)
			return
		}
		if input.Action == "stop" {
			task, err := s.stopRobotScript(root, input.Script)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, task)
			return
		}
		writeError(w, http.StatusBadRequest, "未知的脚本操作。")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	items, err := s.robots.PackageScripts(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	includeRecord := r.URL.Query().Get("record") == "1"
	response := make([]scriptControlItem, 0, len(items))
	s.mu.RLock()
	for _, item := range items {
		entry := scriptControlItem{PackageScript: item}
		if process, ok := s.scriptProcesses[scriptProcessKey(root, item.Name)]; ok {
			entry.Running, entry.TaskID = true, process.TaskID
		}
		if includeRecord {
			for _, task := range s.operations {
				if task.Root == root && task.Action == "script:"+item.Name {
					entry.Record = task.Output
					break
				}
			}
		}
		response = append(response, entry)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"items": response})
}

func (s *server) startRobotScript(root, script string) (operationTask, error) {
	if _, err := s.robots.Validate(root); err != nil {
		return operationTask{}, err
	}
	if _, err := s.robots.EnsureRuntimeDependencies(root); err != nil {
		return operationTask{}, err
	}
	key := scriptProcessKey(root, script)
	s.mu.RLock()
	_, running := s.scriptProcesses[key]
	s.mu.RUnlock()
	if running {
		return operationTask{}, errors.New("该脚本已在运行。")
	}
	command, err := s.robots.ScriptCommand(root, script)
	if err != nil {
		return operationTask{}, err
	}
	configureManagedProcess(command)
	configureRobotCBPFileTransport(&command.Env, root)
	created := operationTask{ID: "script-" + time.Now().Format("20060102150405.000000000"), Root: root, Action: "script:" + script, Status: "running", Output: "正在执行脚本 " + script + "…\n", CreatedAt: time.Now()}
	command.Stdout = newOperationWriter(created.ID, s)
	command.Stderr = newOperationWriter(created.ID, s)
	if err := command.Start(); err != nil {
		return operationTask{}, err
	}
	s.mu.Lock()
	if _, exists := s.scriptProcesses[key]; exists {
		s.mu.Unlock()
		_ = forceStopManagedProcess(command)
		return operationTask{}, errors.New("该脚本已在运行。")
	}
	s.scriptProcesses[key] = scriptProcess{TaskID: created.ID, Root: root, Script: script, Command: command}
	s.mu.Unlock()
	s.addOperation(created)
	s.recordProcess(root, created.ID, processGroupID(command), created.Action)
	go s.watchRobotScript(key, created, command)
	return created, nil
}

func (s *server) stopRobotScript(root, script string) (operationTask, error) {
	key := scriptProcessKey(root, script)
	s.mu.RLock()
	process, running := s.scriptProcesses[key]
	s.mu.RUnlock()
	if !running {
		finished := time.Now()
		task := operationTask{ID: "script-stop-" + finished.Format("20060102150405.000000000"), Root: root, Action: "script-stop:" + script, Status: "completed", Output: "脚本当前未运行。", CreatedAt: finished, FinishedAt: &finished}
		s.addOperation(task)
		return task, nil
	}
	_ = interruptManagedProcess(process.Command)
	time.AfterFunc(3*time.Second, func() {
		s.mu.RLock()
		current, active := s.scriptProcesses[key]
		s.mu.RUnlock()
		if active && current.TaskID == process.TaskID {
			_ = forceStopManagedProcess(current.Command)
		}
	})
	finished := time.Now()
	task := operationTask{ID: "script-stop-" + finished.Format("20060102150405.000000000"), Root: root, Action: "script-stop:" + script, Status: "completed", Output: "已请求停止脚本 " + script + "。", CreatedAt: finished, FinishedAt: &finished}
	s.addOperation(task)
	return task, nil
}

func (s *server) watchRobotScript(key string, task operationTask, command *exec.Cmd) {
	err := command.Wait()
	s.flushOperationOutput(task.ID)
	finished := time.Now()
	s.mu.Lock()
	if current, active := s.scriptProcesses[key]; active && current.TaskID == task.ID {
		delete(s.scriptProcesses, key)
	}
	var snapshot operationTask
	for index := range s.operations {
		if s.operations[index].ID != task.ID {
			continue
		}
		s.operations[index].FinishedAt = &finished
		if err != nil {
			s.operations[index].Status = "failed"
			s.operations[index].Error = "脚本已退出：" + err.Error()
		} else {
			s.operations[index].Status = "completed"
			s.operations[index].Output += "脚本执行完成。\n"
		}
		snapshot = s.operations[index]
		break
	}
	s.mu.Unlock()
	s.forgetProcess(task.Root, task.ID)
	if snapshot.ID != "" {
		s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
	}
}
