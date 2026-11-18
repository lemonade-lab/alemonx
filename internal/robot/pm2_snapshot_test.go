//go:build !windows

package robot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func pm2SnapshotFixture(t *testing.T, body string) (root, launcher, calls string) {
	t.Helper()
	root = t.TempDir()
	bin := t.TempDir()
	launcher = filepath.Join(bin, "pm2")
	calls = filepath.Join(bin, "calls")
	t.Setenv("PM2_HOME", bin)
	t.Setenv("PM2_DAEMON_RPC_PORT", "")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_PM2_CALLS", calls)
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\n[ \"$1\" = jlist ] || exit 99\nprintf 'call\\n' >> \"$TEST_PM2_CALLS\"\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"package.json": "{}", "pm2.config.cjs": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, launcher, calls
}

func waitForPM2SnapshotCondition(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("PM2 test condition did not complete")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPM2SnapshotSharesReadersAndFreshStatusBypassesCache(t *testing.T) {
	root, launcher, calls := pm2SnapshotFixture(t, `
while [ ! -e released ]; do /bin/sleep 0.01; done
if [ -e stopped ]; then
  printf '[]\n'
else
  printf '[{"pm2_env":{"pm_cwd":"%s","status":"online"}}]\n' "$PWD"
fi
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstCtx, cancelFirst := context.WithCancel(ctx)
	defer cancelFirst()
	manager := Manager{}
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		_, err := manager.PM2ProjectProcessesContext(firstCtx, root)
		first <- err
	}()
	go func() {
		status, err := manager.PM2StatusContext(ctx, root)
		if err == nil && !status.Running {
			err = fmt.Errorf("expected online process, got %+v", status)
		}
		second <- err
	}()
	waitForPM2SnapshotCondition(t, func() bool {
		key := pm2SnapshotKey(root) + "\x00" + launcher + "\x00"
		pm2Snapshots.Lock()
		defer pm2Snapshots.Unlock()
		return pm2Snapshots.entries[key] != nil && pm2Snapshots.entries[key].waiters == 2
	})
	cancelFirst()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("first caller: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "released"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stopped"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if status, err := manager.PM2StatusContext(ctx, root); err != nil || !status.Running {
		t.Fatalf("cached status: %+v, %v", status, err)
	}
	if status, err := manager.FreshPM2Status(ctx, root); err != nil || status.Running {
		t.Fatalf("fresh status: %+v, %v", status, err)
	}
	data, err := os.ReadFile(calls)
	if err != nil || strings.Count(string(data), "call\n") != 2 {
		t.Fatalf("expected one shared and one fresh invocation, got %q, %v", data, err)
	}
}

func TestPM2SnapshotStopsCommandWhenLastReaderCancels(t *testing.T) {
	root, launcher, calls := pm2SnapshotFixture(t, "while true; do /bin/sleep 1; done\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := (Manager{}).PM2ProcessesContext(ctx, root)
		result <- err
	}()
	waitForPM2SnapshotCondition(t, func() bool {
		select {
		case err := <-result:
			t.Fatalf("PM2 call exited before cancellation: %v", err)
		default:
		}
		_, err := os.Stat(calls)
		return err == nil
	})
	key := pm2SnapshotKey(root) + "\x00" + launcher + "\x00"
	pm2Snapshots.Lock()
	entry := pm2Snapshots.entries[key]
	pm2Snapshots.Unlock()
	if entry == nil {
		t.Fatal("PM2 command was not tracked")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller: %v", err)
	}
	select {
	case <-entry.done:
		if !errors.Is(entry.err, context.Canceled) {
			t.Fatalf("command: %v", entry.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PM2 command remained running after the last cancellation")
	}
}
