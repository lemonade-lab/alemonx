package mcp

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"alemonx/internal/catalog"
	"alemonx/internal/robot"
	"alemonx/internal/workspace"
)

func robotTools() []map[string]any {
	root := stringSchema("从机器人列表取得的机器人目录。")
	confirm := boolSchema("用户已确认本次操作。")
	return []map[string]any{
		tool("alemonjs_list_robots", "我的机器人", "列出当前账户保存的机器人，以及最近选中的机器人。", objectSchema(map[string]any{}), true, false),
		tool("alemonjs_list_robot_backpack", "查看机器人背包", "列出指定机器人的背包包、版本、有效性和启用状态。", objectSchema(map[string]any{"root": root}, "root"), true, false),
		tool("alemonjs_get_robot_run_options", "查看机器人启动方式", "查看已启用插件、项目可用的启动脚本和此机器人的后台进程。", objectSchema(map[string]any{"root": root}, "root"), true, false),
		tool("alemonjs_manage_robot_plugin", "安装或卸载机器人插件", "从官方资源目录安装 Git 插件到指定机器人的背包，或卸载背包内指定插件。返回操作任务。", objectSchema(map[string]any{"root": root, "action": map[string]any{"type": "string", "enum": []string{"install", "uninstall"}}, "resourceId": stringSchema("安装时填写官方 Git 插件资源 ID。"), "package": stringSchema("卸载时填写背包包名。"), "confirm": confirm}, "root", "action", "confirm"), false, true),
		tool("alemonjs_set_robot_plugin_enabled", "启用或停用背包插件", "调整指定机器人下次启动时加载的背包插件。", objectSchema(map[string]any{"root": root, "package": stringSchema("背包中的包名。"), "enabled": boolSchema("true 为启用，false 为停用。"), "confirm": confirm}, "root", "package", "enabled", "confirm"), false, true),
		tool("alemonjs_control_robot_runtime", "启动或停止机器人", "按开发模式或 PM2 后台模式启动或停止指定机器人。工作台连接还支持前台模式。", objectSchema(map[string]any{"root": root, "mode": map[string]any{"type": "string", "enum": []string{"development", "foreground", "background"}}, "action": map[string]any{"type": "string", "enum": []string{"start", "stop"}}, "confirm": confirm}, "root", "mode", "action", "confirm"), false, true),
		tool("alemonjs_get_robot_logs", "查看机器人日志", "读取指定机器人的最近日志。后台日志支持关键词与页码。工作台连接可读取本机运行日志。", objectSchema(map[string]any{"root": root, "mode": map[string]any{"type": "string", "enum": []string{"local", "background"}}, "query": stringSchema("可选日志关键词。"), "page": map[string]any{"type": "integer", "minimum": 1}}, "root", "mode"), true, false),
	}
}

func (s *Server) registeredRobot(root string) (string, error) {
	if s.policy.RobotList == nil {
		return "", fmt.Errorf("请使用工作台 MCP 连接，或在 STDIO 配置中提供工作台账户认证")
	}
	list, err := s.policy.RobotList()
	if err != nil {
		return "", err
	}
	for _, item := range list.Projects {
		if workspace.SameRobotPath(item.Path, root) {
			if err := s.authorizePath(item.Path); err != nil {
				return "", err
			}
			return item.Path, nil
		}
	}
	return "", fmt.Errorf("请从当前账户的机器人列表选择机器人")
}

func (s *Server) listAccountRobots() (string, error) {
	if s.policy.RobotList == nil {
		return "", fmt.Errorf("请使用工作台 MCP 连接，或在 STDIO 配置中提供工作台账户认证")
	}
	list, err := s.policy.RobotList()
	if err != nil {
		return "", err
	}
	robots := make([]workspace.RobotEntry, 0, len(list.Projects))
	active := ""
	for _, item := range list.Projects {
		if s.authorizePath(item.Path) == nil {
			robots = append(robots, item)
			if item.ID == list.ActiveProjectID {
				active = item.ID
			}
		}
	}
	return encodeResult(map[string]any{"robots": robots, "activeRobotId": active})
}

func (s *Server) robotTool(name string, arguments []byte) (string, error) {
	if name == "alemonjs_list_robots" {
		return s.listAccountRobots()
	}
	var input struct {
		ResourceID string `json:"resourceId"`
		Root       string `json:"root"`
		Package    string `json:"package"`
		Enabled    *bool  `json:"enabled"`
		Confirm    bool   `json:"confirm"`
		Mode       string `json:"mode"`
		Action     string `json:"action"`
		Query      string `json:"query"`
		Page       int    `json:"page"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	root, err := s.registeredRobot(input.Root)
	if err != nil {
		return "", err
	}
	switch name {
	case "alemonjs_list_robot_backpack":
		packages, err := s.robots.LocalPackages(root)
		if err != nil {
			return "", err
		}
		enabled, err := s.robots.EnabledApps(root)
		if err != nil {
			return "", err
		}
		type backpackEntry struct {
			robot.LocalPackage
			Enabled bool `json:"enabled"`
		}
		entries := make([]backpackEntry, 0, len(packages))
		for _, item := range packages {
			entries = append(entries, backpackEntry{LocalPackage: item, Enabled: slices.Contains(enabled, item.Name)})
		}
		return encodeResult(entries)
	case "alemonjs_get_robot_run_options":
		return s.robotRunOptions(root)
	case "alemonjs_manage_robot_plugin":
		if !input.Confirm {
			return "", fmt.Errorf("请在用户确认后传 confirm=true")
		}
		return s.manageRobotPlugin(root, input.Action, input.ResourceID, input.Package)
	case "alemonjs_set_robot_plugin_enabled":
		if !input.Confirm || input.Enabled == nil {
			return "", fmt.Errorf("请明确确认并指定插件的启用状态")
		}
		packages, err := s.robots.LocalPackages(root)
		if err != nil {
			return "", err
		}
		if !slices.ContainsFunc(packages, func(item robot.LocalPackage) bool { return item.Valid && item.Name == input.Package }) {
			return "", fmt.Errorf("请先从背包列表选择有效插件")
		}
		result, err := s.robots.SetAppEnabled(root, input.Package, *input.Enabled)
		if err != nil {
			return "", err
		}
		return encodeResult(map[string]any{"message": result.Output, "enabled": *input.Enabled, "appliesOnNextStart": true})
	case "alemonjs_control_robot_runtime":
		if !input.Confirm {
			return "", fmt.Errorf("请在用户确认后传 confirm=true")
		}
		return s.controlRobot(root, input.Mode, input.Action)
	case "alemonjs_get_robot_logs":
		if input.Mode == "background" {
			result, err := s.robots.PM2AuditLogs(root, robot.PM2AuditQuery{Page: input.Page, PerPage: 100, Query: strings.TrimSpace(input.Query)})
			if err != nil {
				return "", err
			}
			return encodeResult(result)
		}
		if input.Mode != "local" {
			return "", fmt.Errorf("请选择 local 或 background 日志")
		}
		if s.policy.LocalRobotLogs != nil {
			return s.policy.LocalRobotLogs(root, strings.TrimSpace(input.Query))
		}
		return s.developmentLogs(root, input.Query)
	}
	return "", fmt.Errorf("未知机器人操作")
}

func (s *Server) manageRobotPlugin(root, action, resourceID, name string) (string, error) {
	command := ""
	switch action {
	case "install":
		resource, err := catalog.Detail(resourceID)
		if err != nil {
			return "", err
		}
		target, err := catalog.InstallTarget(resource)
		if err != nil {
			return "", err
		}
		if resource.Type != "js-plugin" || !strings.HasPrefix(target, "git+") {
			return "", fmt.Errorf("请选择通过 Git 安装的机器人插件")
		}
		command, name = "install-package", target
	case "uninstall":
		packages, err := s.robots.LocalPackages(root)
		if err != nil {
			return "", err
		}
		if !slices.ContainsFunc(packages, func(item robot.LocalPackage) bool { return item.Name == name }) {
			return "", fmt.Errorf("请从此机器人的背包列表选择要卸载的插件")
		}
		command = "remove-local-package"
	default:
		return "", fmt.Errorf("请选择安装或卸载")
	}
	return encodeResult(s.startTask(root, command, func() (string, error) {
		result, err := s.robots.Run(root, command, "", name, "", "", "", true)
		return result.Output, err
	}))
}

func (s *Server) robotRunOptions(root string) (string, error) {
	enabled, err := s.robots.EnabledApps(root)
	if err != nil {
		return "", err
	}
	status, err := s.robots.RuntimeOverview(root)
	if err != nil {
		return "", err
	}
	background, pm2Err := s.robotBackgroundProcesses(root)
	available := make([]string, 0, 3)
	if status.HasDevScript {
		available = append(available, "development")
	}
	if status.HasAppScript && s.policy.ControlRobot != nil {
		available = append(available, "foreground")
	}
	if status.HasStartScript {
		available = append(available, "background")
	}
	return encodeResult(map[string]any{
		"robot": filepath.Base(root), "enabledPlugins": enabled, "project": status,
		"availableModes": available, "startTool": "alemonjs_control_robot_runtime",
		"backgroundProcesses": background, "backgroundStatusAvailable": pm2Err == nil,
	})
}

func (s *Server) robotBackgroundProcesses(root string) ([]map[string]any, error) {
	processes, err := s.robots.PM2ProjectProcesses(root)
	result := make([]map[string]any, 0, len(processes))
	for _, process := range processes {
		result = append(result, map[string]any{"name": process.Name, "status": process.Status, "restarts": process.Restarts})
	}
	return result, err
}

func (s *Server) controlRobot(root, mode, action string) (string, error) {
	if action != "start" && action != "stop" {
		return "", fmt.Errorf("请选择启动或停止")
	}
	if mode != "development" && mode != "foreground" && mode != "background" {
		return "", fmt.Errorf("请选择开发、前台或后台模式")
	}
	if s.policy.ControlRobot != nil {
		return s.policy.ControlRobot(root, mode, action)
	}
	if mode == "foreground" {
		return "", fmt.Errorf("前台模式请通过工作台 MCP 启动")
	}
	if mode == "development" {
		if action == "start" {
			return encodeResult(s.startDevelopment(root))
		}
		for _, task := range s.listTasks(root) {
			if task.Action == "dev" && task.Status == "running" {
				return s.stopDevelopment(task.ID)
			}
		}
		return encodeResult(map[string]string{"status": "stopped"})
	}
	command := "pm2"
	if action == "stop" {
		command = "pm2-stop-project"
	}
	return encodeResult(s.startTask(root, command, func() (string, error) {
		result, err := s.robots.Run(root, command, "", "", "", "", "", true)
		return result.Output, err
	}))
}
