package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"alemonx/internal/logging"
	"alemonx/internal/robot"
)

const (
	mcpTaskAccepted = "mcp.task.accepted"
	mcpTaskResult   = "mcp.task.result"
)

func mcpTaskRoot(root string) string {
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		return strings.ToLower(root)
	}
	return root
}

// The task record and its audit event commit together. The record survives
// the event journal's retention window and the workbench's recent task limit.
func (s *OperationEventStore) recordMCPTask(task operationTask, kind string) (bool, error) {
	if s == nil {
		return false, os.ErrInvalid
	}
	task.Output = robot.BoundedDiagnosticText(task.Output, 32*1024)
	task.Error = robot.BoundedDiagnosticText(task.Error, 4096)
	task.Data = nil
	task.CanCancel, task.CanRetry = false, false
	task.Steps = append([]operationStep(nil), task.Steps...)
	for i := range task.Steps {
		task.Steps[i].Message = robot.BoundedDiagnosticText(task.Steps[i].Message, 1024)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return false, err
	}
	fields := logging.Fields{"task_id": task.ID, "account": task.Account, "root": task.Root, "action": task.Action, "status": task.Status}
	audit, err := json.Marshal(fields)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		if isStructuralSQLiteError(err) {
			s.fail(err)
		}
	}()
	if s.db == nil || s.health == "degraded" || s.health == "recovering" || s.health == "read-only" {
		return false, os.ErrInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if kind == mcpTaskAccepted {
		_, err = tx.Exec(`INSERT INTO mcp_operation_tasks(id,root,status,payload,created_at) VALUES(?,?,?,?,?)`,
			task.ID, mcpTaskRoot(task.Root), "running", payload, task.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	} else {
		var updated sql.Result
		updated, err = tx.Exec(`UPDATE mcp_operation_tasks SET status=?,payload=? WHERE id=? AND status='running'`, task.Status, payload, task.ID)
		if err == nil {
			var count int64
			count, err = updated.RowsAffected()
			if err == nil && count == 0 {
				return false, nil
			}
		}
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(`INSERT INTO event_journal(topic,type,payload,created_at) VALUES('mcp',?,?,?)`, kind, audit, now.Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	s.writes++
	s.maintenanceLocked(now, false)
	logging.InfoEvent(kind, fields)
	return true, nil
}

func (s *OperationEventStore) mcpTasks(ctx context.Context, query string, args ...any) ([]operationTask, error) {
	if s == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []operationTask
	for rows.Next() {
		var raw []byte
		var task operationTask
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *OperationEventStore) mcpTaskByID(ctx context.Context, id string) (operationTask, bool) {
	tasks, _ := s.mcpTasks(ctx, `SELECT payload FROM mcp_operation_tasks WHERE id=?`, id)
	if len(tasks) == 0 {
		return operationTask{}, false
	}
	return tasks[0], true
}

func (s *OperationEventStore) mcpTasksByRoot(ctx context.Context, root string, limit int) []operationTask {
	tasks, _ := s.mcpTasks(ctx, `SELECT payload FROM mcp_operation_tasks WHERE root=? ORDER BY created_at DESC LIMIT ?`, mcpTaskRoot(root), limit)
	return tasks
}

func (s *server) mcpProjectTasks(ctx context.Context, roots []string) []robot.DiagnosticOperation {
	keys := make([]string, 0, len(roots))
	for _, root := range roots {
		keys = append(keys, mcpTaskRoot(root))
	}
	encoded, _ := json.Marshal(keys)
	tasks, _ := s.operationEvents.mcpTasks(ctx, `SELECT payload FROM mcp_operation_tasks
		WHERE root IN (SELECT value FROM json_each(?)) ORDER BY created_at DESC LIMIT ?`, string(encoded), operationHistoryLimit)
	result := make([]robot.DiagnosticOperation, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, summarizeDiagnosticOperation(task))
	}
	return result
}

func (s *server) finishMCPTask(task operationTask) bool {
	_, err := s.operationEvents.recordMCPTask(task, mcpTaskResult)
	if err != nil {
		logging.WarnEvent("mcp.task.persistence_failed", logging.Fields{"task_id": task.ID, "status": task.Status})
		return false
	}
	return true
}

func (s *server) settleRecoveredMCPTasks() error {
	const message = "工作台重启，执行结果未知。"
	ctx := context.Background()
	for {
		tasks, err := s.operationEvents.mcpTasks(ctx, `SELECT payload FROM mcp_operation_tasks WHERE status='running' LIMIT 100`)
		if err != nil || len(tasks) == 0 {
			return err
		}
		for _, task := range tasks {
			now := time.Now()
			task.Status, task.Error, task.FinishedAt = "interrupted", message, &now
			if !s.finishMCPTask(task) {
				return errors.New("无法保存重启前的 MCP 任务状态")
			}
			s.mu.Lock()
			for i := range s.operations {
				if s.operations[i].ID == task.ID {
					s.operations[i] = task
					break
				}
			}
			s.mu.Unlock()
		}
	}
}
