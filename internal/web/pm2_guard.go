package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"alemonx/internal/agent"
	"alemonx/internal/robot"
)

// GuardedPM2Executor is the only path automatic maintenance should use for
// PM2 writes. It validates the project lease before and after the command.
type GuardedPM2Executor struct {
	Robots    robot.Manager
	Leases    agent.FencingLeaseManager
	Store     agent.OpsRepository
	Emergency func() bool
}

func (e GuardedPM2Executor) Run(ctx context.Context, root, action, owner string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(action))
	switch normalized {
	case "status", "pm2-status":
		normalized = "pm2-status"
	case "logs", "pm2-logs":
		normalized = "pm2-logs"
	case "restart", "pm2-restart":
		normalized = "pm2-restart"
	case "reload", "pm2-reload":
		normalized = "pm2-reload"
	default:
		return "", fmt.Errorf("PM2 操作不在白名单：%s", action)
	}
	if e.Emergency != nil && e.Emergency() && normalized != "pm2-status" && normalized != "pm2-logs" {
		return "", errors.New("AI 运维已紧急停止")
	}
	write := normalized == "pm2-restart" || normalized == "pm2-reload"
	var beforeToken uint64
	if write {
		if e.Leases == nil || strings.TrimSpace(owner) == "" {
			return "", errors.New("PM2 写操作缺少租约")
		}
		key := "project:" + root
		if err := e.Leases.Acquire(ctx, key, owner, 30*time.Minute); err != nil {
			return "", err
		}
		defer func() { _ = e.Leases.Release(context.Background(), key, owner) }()
		if fenced, ok := e.Leases.(agent.FencingLeaseManager); ok {
			var err error
			if beforeToken, err = fenced.Token(ctx, key, owner); err != nil {
				return "", err
			}
		}
		if e.Store != nil {
			if _, err := e.Store.ConsumeBudget(root, 0, 1, 0); err != nil {
				return "", err
			}
		}
	}
	result, err := e.Robots.Run(root, normalized, "", "", "", "", "", true)
	if err != nil {
		e.audit(root, owner, normalized, "failed", err.Error())
		return result.Output, err
	}
	if write {
		if e.Emergency != nil && e.Emergency() {
			return "", errors.New("PM2 操作后检测到紧急停止")
		}
		if fenced, ok := e.Leases.(agent.FencingLeaseManager); ok {
			after, tokenErr := fenced.Token(ctx, "project:"+root, owner)
			if tokenErr != nil {
				return "", tokenErr
			}
			if after != beforeToken {
				err := fmt.Errorf("PM2 租约 fencing token 已变化")
				e.audit(root, owner, normalized, "fenced", err.Error())
				return "", err
			}
		}
	}
	e.audit(root, owner, normalized, "success", "")
	return result.Output, nil
}

func (e GuardedPM2Executor) audit(root, owner, action, result, reason string) {
	if e.Store == nil {
		return
	}
	_ = e.Store.AppendAudit(agent.AuditEntry{
		Actor: owner, Action: action, Resource: root, Result: result, Reason: reason,
	})
}
