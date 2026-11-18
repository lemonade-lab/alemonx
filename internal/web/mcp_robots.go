package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"

	"alemonx/internal/access"
	"alemonx/internal/logging"
	"alemonx/internal/mcp"
	"alemonx/internal/robot"
	"alemonx/internal/workspace"
)

func (s *server) accountMCPPolicy(policy mcp.Policy, root string, identity access.Status, token string) mcp.Policy {
	policy = mcp.WithRobotAccount(policy, root, func() (string, error) {
		current, valid, err := s.auth.MCPKeyStatus(token)
		if err != nil || !valid || current.RobotScope != identity.RobotScope || current.Enabled != identity.Enabled || current.MCPDiagnostic != identity.MCPDiagnostic {
			return "", errors.New("MCP 连接密钥已失效。")
		}
		return identity.RobotScope, nil
	})
	account := identity.Account
	if account == "" {
		account = "local"
	}
	policy.DiagnosticOnly = identity.MCPDiagnostic
	policy.OnToolCall = func(tool string, success bool) {
		logging.InfoEvent("mcp.tool.called", logging.Fields{"account": account, "diagnostic_key": identity.MCPDiagnostic, "tool": tool, "success": success})
	}
	policy.ControlRobot = func(root, mode, action string) (string, error) {
		return s.controlMCPRobot(account, root, mode, action)
	}
	policy.SubmitProjectAction = func(action mcp.ProjectAction) (string, error) {
		return s.submitMCPRobotTask(account, robotTaskRequest{ProjectAction: action, Confirm: "true"})
	}
	policy.LocalRobotLogs = s.localMCPRobotLogs
	policy.WorkbenchTask = s.mcpRobotTask
	policy.Diagnostics = s.robotDiagnosticSnapshot
	policy.WorkbenchOperations = s.robotDiagnosticOperations
	policy.WorkbenchProjectTasks = s.mcpProjectTasks
	return policy
}

func (s *server) controlMCPRobot(account, root, mode, action string) (string, error) {
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
	return s.submitMCPRobotTask(account, robotTaskRequest{ProjectAction: mcp.ProjectAction{Root: root, Action: command}})
}

func (s *server) submitMCPRobotTask(account string, input robotTaskRequest) (string, error) {
	recorder := httptest.NewRecorder()
	s.submitRobotTask(recorder, context.Background(), input, account)
	if recorder.Code < 200 || recorder.Code >= 300 {
		var failure struct{ Error string }
		if json.Unmarshal(recorder.Body.Bytes(), &failure) == nil && failure.Error != "" {
			return "", errors.New(failure.Error)
		}
		return "", errors.New("机器人运行操作失败。")
	}
	var task operationTask
	if err := json.Unmarshal(recorder.Body.Bytes(), &task); err != nil {
		return "", errors.New("无法读取机器人任务状态。")
	}
	task.Output = robot.RedactRuntimeLog(task.Output)
	task.Error = robot.RedactRuntimeLog(task.Error)
	result, err := json.Marshal(task)
	return string(result), err
}

func (s *server) localMCPRobotLogs(root, query string) (string, error) {
	snapshot := s.runtimeProcess(root)
	return mcp.RuntimeLogResult(snapshot.id, snapshot.mode, snapshot.status, snapshot.runError, snapshot.output, query)
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
