package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"

	"alemonx/internal/access"
	"alemonx/internal/mcp"
	"alemonx/internal/workspace"
)

func (s *server) mcpAccountValid(identity access.Status, token string) bool {
	current, err := s.auth.Status(token)
	return err == nil && current.Enabled == identity.Enabled &&
		(!identity.Enabled || current.Authenticated && current.RobotScope == identity.RobotScope &&
			(current.SuperAdmin || (slices.Contains(current.Permissions, "system.manage") && slices.Contains(current.Permissions, "workbench.manage"))))
}

func (s *server) accountMCPPolicy(policy mcp.Policy, root string, identity access.Status, token string) mcp.Policy {
	policy = mcp.WithRobotAccount(policy, root, func() (string, error) {
		if !s.mcpAccountValid(identity, token) {
			return "", errors.New("MCP 工作台账户会话已失效，请重新启动 HTTP 服务。")
		}
		return identity.RobotScope, nil
	})
	policy.ControlRobot = s.controlMCPRobot
	policy.LocalRobotLogs = s.localMCPRobotLogs
	policy.WorkbenchTask = s.mcpRobotTask
	return policy
}

func (s *server) controlMCPRobot(root, mode, action string) (string, error) {
	actions := map[string][2]string{
		"development": {"dev", "dev-stop"},
		"foreground":  {"app", "app-stop"},
		"background":  {"pm2", "pm2-stop-project"},
	}
	commands, ok := actions[mode]
	if !ok {
		return "", errors.New("未知运行模式。")
	}
	command := commands[0]
	if action == "stop" {
		command = commands[1]
	}
	payload, _ := json.Marshal(map[string]string{"root": root, "action": command})
	// Reuse the workbench operation handler so MCP and the UI share startup
	// checks, process ownership, logs, and task state.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/robot/tasks", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	s.robotTasksHandler(recorder, request)
	if recorder.Code < 200 || recorder.Code >= 300 {
		var failure struct{ Error string }
		if json.Unmarshal(recorder.Body.Bytes(), &failure) == nil && failure.Error != "" {
			return "", errors.New(failure.Error)
		}
		return "", errors.New("机器人运行操作失败。")
	}
	return recorder.Body.String(), nil
}

func (s *server) localMCPRobotLogs(root, query string) (string, error) {
	snapshot := s.runtimeProcess(root)
	return mcp.RuntimeLogResult(snapshot.id, snapshot.mode, snapshot.status, snapshot.runError, snapshot.output, query)
}

func (s *server) mcpRobotTask(id string) (string, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, task := range s.operations {
		if task.ID == id && task.Root != "" {
			switch task.Action {
			case "dev", "dev-stop", "app", "app-stop", "pm2", "pm2-stop-project":
				result, err := mcp.RuntimeLogResult(task.ID, task.Action, task.Status, task.Error, task.Output, "")
				return task.Root, result, err == nil
			}
		}
	}
	return "", "", false
}

func (s *server) addCreatedRobot(root, account, path string) error {
	list, err := workspace.LoadRobots(root, account)
	if err != nil {
		return err
	}
	_, err = workspace.UpdateRobots(root, account, workspace.RobotListChange{
		Scope: list.Scope, Action: "add", Projects: []workspace.RobotEntry{{Path: path}},
	})
	return err
}
