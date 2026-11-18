package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"alemonx/internal/mcp"
	"alemonx/internal/robot"
	"alemonx/internal/systemnetwork"
	"alemonx/internal/workspace"
)

func (s *server) robotDiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	identity, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取工作台账户。")
		return
	}
	if identity.Enabled && !identity.Authenticated {
		writeError(w, http.StatusUnauthorized, "请先登录工作台。")
		return
	}
	list, err := workspace.LoadRobots(s.robotListWorkspace(), identity.RobotScope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取账户机器人列表。")
		return
	}
	root := r.URL.Query().Get("root")
	for _, entry := range list.Projects {
		if root == "" || !workspace.SameRobotPath(root, entry.Path) {
			continue
		}
		if r.URL.Query().Get("view") == "pm2-logs" {
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			page, err := s.robots.DiagnosticPM2Logs(ctx, entry.Path, robot.PM2AuditQuery{
				Source: r.URL.Query().Get("source"), PerPage: 40,
			})
			if err != nil {
				writeError(w, http.StatusBadRequest, robot.BoundedDiagnosticText(err.Error(), 1024))
				return
			}
			writeJSON(w, http.StatusOK, page)
			return
		}
		if taskID := r.URL.Query().Get("taskId"); taskID != "" {
			taskRoot, data, ok := s.mcpRobotTask(r.Context(), taskID)
			if !ok || !workspace.SameRobotPath(taskRoot, entry.Path) {
				writeError(w, http.StatusNotFound, "该机器人没有这条操作记录。")
				return
			}
			writeJSON(w, http.StatusOK, json.RawMessage(data))
			return
		}
		report := s.robotDiagnosticSnapshot(r.Context(), entry.Path)
		report.SetRecentOperations(robot.RecentDiagnosticOperations(s.robotDiagnosticOperations(r.Context(), entry.Path), "", 10))
		writeJSON(w, http.StatusOK, report)
		return
	}
	writeError(w, http.StatusForbidden, "请从当前账户的机器人列表选择机器人。")
}

func (s *server) robotDiagnosticSnapshot(ctx context.Context, root string) robot.RobotDiagnostics {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	report := s.robots.DiagnoseRobot(ctx, root, func(ctx context.Context, pm2 []robot.DiagnosticPM2Process) ([]robot.DiagnosticPort, error) {
		return s.effectiveDiagnosticPorts(ctx, root, pm2)
	})
	// Like the workbench terminal, prefer the active dev/app run, otherwise
	// report the most recent run. These are supervised process observations.
	var latest *robot.DiagnosticOperation
	for _, task := range s.currentRobotOperations(root) {
		if task.Action != "dev" && task.Action != "app" {
			continue
		}
		if latest == nil || task.Status == "running" {
			latest = &task
		}
		if task.Status == "running" {
			break
		}
	}
	report.LocalRuntime = latest
	return report
}

func (s *server) currentRobotOperations(root string) []robot.DiagnosticOperation {
	s.mu.RLock()
	snapshot := append([]operationTask(nil), s.operations...)
	s.mu.RUnlock()
	operations := make([]robot.DiagnosticOperation, 0, len(snapshot))
	for _, task := range snapshot {
		if task.Root != "" && workspace.SameRobotPath(task.Root, root) {
			operations = append(operations, summarizeDiagnosticOperation(task))
		}
	}
	return operations
}

func (s *server) robotDiagnosticOperations(ctx context.Context, root string) []robot.DiagnosticOperation {
	operations := s.currentRobotOperations(root)
	seen := make(map[string]bool, len(operations))
	for _, task := range operations {
		seen[task.ID] = true
	}
	for _, history := range [][]operationTask{
		s.operationEvents.mcpTasksByRoot(ctx, root, operationHistoryLimit),
		s.operationEvents.diagnosticTasks(ctx, "$.task.root", root, operationHistoryLimit),
	} {
		for _, task := range history {
			if !seen[task.ID] {
				seen[task.ID] = true
				operations = append(operations, summarizeDiagnosticOperation(task))
			}
		}
	}
	return robot.RecentDiagnosticOperations(operations, "", operationHistoryLimit)
}

func summarizeDiagnosticOperation(task operationTask) robot.DiagnosticOperation {
	return robot.DiagnosticOperation{
		ID: task.ID, Root: task.Root, Account: task.Account, Source: task.Source, Action: robot.BoundedDiagnosticText(task.Action, 256), Status: task.Status, CreatedAt: task.CreatedAt,
		FinishedAt: task.FinishedAt, Progress: task.Progress, Error: robot.BoundedDiagnosticText(task.Error, 4096),
	}
}

func (s *server) mcpRobotTask(ctx context.Context, id string) (string, string, bool) {
	s.mu.RLock()
	var task operationTask
	activeDevelopment := false
	for _, item := range s.operations {
		if item.ID == id && item.Root != "" {
			task = item
			activeDevelopment = task.Action == "dev" && task.Status == "running" && s.development[task.Root].TaskID == task.ID
			// Operation step slices may be reused by a concurrent progress update.
			task.Steps = append([]operationStep(nil), item.Steps...)
			break
		}
	}
	s.mu.RUnlock()
	if task.ID == "" || task.Source == "mcp" {
		if stored, ok := s.operationEvents.mcpTaskByID(ctx, id); ok && (task.ID == "" || stored.FinishedAt != nil) {
			task = stored
			activeDevelopment = false
		}
	}
	if task.ID == "" {
		history := s.operationEvents.diagnosticTasks(ctx, "$.task.id", id, 1)
		if len(history) == 0 {
			return "", "", false
		}
		task = history[0]
	}
	for i := range task.Steps {
		task.Steps[i].Message = robot.BoundedDiagnosticText(task.Steps[i].Message, 1024)
	}
	payload := struct {
		robot.DiagnosticOperation
		TaskID            string          `json:"taskId"`
		ActiveDevelopment bool            `json:"activeDevelopment,omitempty"`
		Mode              string          `json:"mode"`
		Lines             []string        `json:"lines"`
		Output            string          `json:"output,omitempty"`
		Steps             []operationStep `json:"steps,omitempty"`
	}{
		DiagnosticOperation: summarizeDiagnosticOperation(task), TaskID: task.ID, ActiveDevelopment: activeDevelopment,
		Mode:  robot.BoundedDiagnosticText(task.Action, 256),
		Lines: mcp.RuntimeLogLines(task.Output, ""), Steps: task.Steps,
	}
	if task.Source == "mcp" {
		payload.Output = robot.BoundedDiagnosticText(task.Output, 32*1024)
	}
	raw, err := json.Marshal(payload)
	return task.Root, string(raw), err == nil
}

func (s *server) effectiveDiagnosticPorts(ctx context.Context, root string, pm2 []robot.DiagnosticPM2Process) ([]robot.DiagnosticPort, error) {
	app, appErr := s.effectiveAppPort(root)
	test, testErr := s.effectiveTestPort(root)
	ports := make([]robot.DiagnosticPort, 0, 2)
	for _, entry := range []struct {
		kind, label, source, endpoint string
		configured                    bool
		port, configuredPort          int
	}{
		{"app", "应用端口", app.Source, "/", app.Configured, app.Port, app.ConfiguredPort},
		{"test", "服务端口", test.Source, "/api/online", test.Configured, test.Port, test.ConfiguredPort},
	} {
		if entry.port < 1 || entry.port > 65535 || (!entry.configured && entry.source != "runtime") {
			continue
		}
		report := robot.DiagnosticPort{
			RobotPort: robot.RobotPort{Kind: entry.kind, Label: entry.label, Port: entry.port, Configured: entry.configured},
			Source:    entry.source,
		}
		if entry.configured {
			report.ConfiguredPort = entry.configuredPort
		}
		if entry.source == "runtime" {
			report.ActualPort = entry.port
		}
		report.ObserveTCP(ctx)
		if report.TCP.State == "reachable" {
			report.Ownership = s.diagnosticPortOwnership(ctx, root, entry.port, pm2)
			if report.Ownership.State == "project" {
				report.ApplicationReadiness, report.HTTPStatus = diagnosticHTTPReadiness(ctx, entry.port, entry.endpoint)
			}
		}
		ports = append(ports, report)
	}
	return ports, errors.Join(appErr, testErr)
}

func (s *server) diagnosticPortOwnership(ctx context.Context, root string, port int, pm2 []robot.DiagnosticPM2Process) robot.DiagnosticSignal {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	pids := portListenerPIDsContext(ctx, port, true)
	unknown := robot.DiagnosticSignal{State: "unknown", Evidence: "listener_owner_unverified"}
	if len(pids) == 0 {
		return unknown
	}
	s.mu.RLock()
	managed := s.development[root]
	s.mu.RUnlock()
	known := make(map[int]bool)
	if managed.Command != nil && managed.Command.Process != nil {
		known[managed.Command.Process.Pid] = true
	}
	for _, process := range pm2 {
		if process.PID > 0 && process.Status == "online" {
			known[process.PID] = true
		}
	}
	owned, other := 0, 0
	for _, pid := range pids {
		if known[pid] || (managed.PGID > 0 && processPGID(pid) == managed.PGID) {
			owned++
			continue
		}
		cwd := processWorkingDirectoryContext(ctx, pid)
		if cwd == "" {
			continue
		}
		if workspace.SameRobotPath(cwd, root) {
			owned++
		} else if filepath.IsAbs(cwd) {
			other++
		}
	}
	if owned == len(pids) {
		return robot.DiagnosticSignal{State: "project", Evidence: "listener_process_matches_project"}
	}
	if other == len(pids) {
		return robot.DiagnosticSignal{State: "other", Evidence: "listener_working_directory_differs"}
	}
	return unknown
}

func diagnosticHTTPReadiness(ctx context.Context, port int, endpoint string) (robot.DiagnosticSignal, int) {
	unknown := robot.DiagnosticSignal{State: "unknown", Evidence: "application_response_unverified"}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(port))+endpoint, nil)
	if err != nil {
		return unknown, 0
	}
	client := systemnetwork.LocalClient(700 * time.Millisecond)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return unknown, 0
	}
	_ = response.Body.Close()
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return robot.DiagnosticSignal{State: "ready", Evidence: "owned_application_http_2xx"}, response.StatusCode
	case response.StatusCode >= 500:
		return robot.DiagnosticSignal{State: "not_ready", Evidence: "owned_application_http_5xx"}, response.StatusCode
	default:
		return unknown, response.StatusCode
	}
}
