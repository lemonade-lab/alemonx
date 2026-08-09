package web

import (
	"context"
	"testing"

	"alemonx/internal/agent"
	"alemonx/internal/robot"
)

func TestGuardedPM2ExecutorRejectsUnsafeActions(t *testing.T) {
	guard := GuardedPM2Executor{Robots: robot.Manager{}, Emergency: func() bool { return true }}
	if _, err := guard.Run(context.Background(), t.TempDir(), "delete", "owner"); err == nil {
		t.Fatal("unsafe PM2 action should be rejected")
	}
	if _, err := guard.Run(context.Background(), t.TempDir(), "restart", "owner"); err == nil {
		t.Fatal("emergency-stop should block PM2 writes")
	}
}

func TestGuardedPM2ExecutorRequiresLeaseForWrites(t *testing.T) {
	guard := GuardedPM2Executor{Robots: robot.Manager{}, Leases: agent.NewLeaseManager(agent.NewOpsStoreAt(t.TempDir()))}
	if _, err := guard.Run(context.Background(), t.TempDir(), "reload", "owner"); err == nil {
		t.Fatal("PM2 write without project lease should fail")
	}
}
