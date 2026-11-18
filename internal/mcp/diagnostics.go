package mcp

import (
	"context"
	"fmt"

	"alemonx/internal/robot"
)

func (s *Server) diagnosticOperations(ctx context.Context, root string) []robot.DiagnosticOperation {
	operations := make([]robot.DiagnosticOperation, 0)
	if s.policy.WorkbenchOperations != nil {
		operations = append(operations, s.policy.WorkbenchOperations(ctx, root)...)
	}
	for _, task := range s.listTasks(root) {
		operations = append(operations, robot.DiagnosticOperation{
			ID: task.ID, Root: task.Root, Action: task.Action, Status: task.Status, CreatedAt: task.CreatedAt,
			FinishedAt: task.FinishedAt, Error: robot.BoundedDiagnosticText(task.Error, 4096),
		})
	}
	return operations
}

func (s *Server) robotOperationList(ctx context.Context, root, status string, limit int) ([]robot.DiagnosticOperation, error) {
	switch status {
	case "", "running", "completed", "failed", "stopped", "cancelled", "interrupted":
	default:
		return nil, fmt.Errorf("操作任务状态无效")
	}
	if limit < 0 || limit > 40 {
		return nil, fmt.Errorf("操作任务数量限制为 1 到 40")
	}
	if limit == 0 {
		limit = 10
	}
	return robot.RecentDiagnosticOperations(s.diagnosticOperations(ctx, root), status, limit), nil
}

func (s *Server) diagnoseRobot(ctx context.Context, root string) robot.RobotDiagnostics {
	var report robot.RobotDiagnostics
	if s.policy.Diagnostics != nil {
		report = s.policy.Diagnostics(ctx, root)
	} else {
		report = s.robots.DiagnoseRobot(ctx, root, nil)
	}
	operations, _ := s.robotOperationList(ctx, root, "", 10)
	if s.policy.Diagnostics == nil {
		for _, task := range robot.RecentDiagnosticOperations(s.diagnosticOperations(ctx, root), "", 40) {
			if task.Action != "dev" {
				continue
			}
			if report.LocalRuntime == nil || task.Status == "running" {
				report.LocalRuntime = &task
			}
			if task.Status == "running" {
				break
			}
		}
	}
	report.SetRecentOperations(operations)
	return report
}
