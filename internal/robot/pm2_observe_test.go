//go:build !windows

package robot

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alemonx/internal/system"
)

func TestProductionPM2DiagnosticsArePassiveAndProjectScoped(t *testing.T) {
	if _, err := system.ResolveCommand("node"); err != nil {
		if _, err := system.ResolveCommand("bun"); err != nil {
			t.Skip("requires an installed Node.js or Bun")
		}
	}
	home, err := os.MkdirTemp("", "alx-pm2-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("PM2_HOME", home)
	t.Setenv("PM2_DAEMON_RPC_PORT", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	offline, err := manager.DiagnosticPM2Status(ctx, root)
	if err != nil || offline.State != "daemon_offline" {
		t.Fatalf("offline: %+v (%v)", offline, err)
	}
	socket := filepath.Join(home, "rpc.sock")
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Fatalf("a diagnostic read must not start PM2: %v", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var monitored = []map[string]any{
		{"name": "fixture", "pid": 321, "pm2_env": map[string]any{
			"pm_cwd": root, "status": "online", "restart_time": 7, "pm_uptime": 1760000000000,
		}, "monit": map[string]any{"memory": 123456, "cpu": 2.5}},
		{"name": "other", "pm2_env": map[string]any{"pm_cwd": filepath.Join(root, "other"), "status": "online"}},
	}
	reply, err := json.Marshal(map[string]any{"args": []any{monitored}})
	if err != nil {
		t.Fatal(err)
	}
	serverError := make(chan error, 1)
	go func() {
		serverError <- serveDiagnosticPM2Fixture(listener, reply)
	}()
	status, err := manager.DiagnosticPM2Status(ctx, root)
	if err != nil || status.State != "registered" || len(status.Processes) != 1 {
		t.Fatalf("project PM2: %+v (%v)", status, err)
	}
	if err := <-serverError; err != nil {
		t.Fatal(err)
	}
	if got := status.Processes[0]; got.PID != 321 || got.Restarts != 7 || got.Memory != 123456 || got.StartedAt == nil {
		t.Fatalf("incorrect PM2 metrics: %+v", got)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(root, "run.log")
	log := "[2026-11-19 11:00:00][ERROR] password=fixture-secret older\n[2026-11-19 12:00:00][ERROR] password=fixture-secret matched\n"
	if err := os.WriteFile(logPath, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	writeDump := func(entries []map[string]string) {
		t.Helper()
		body, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "dump.pm2"), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	owner := map[string]string{"pm_cwd": root, "pm_err_log_path": logPath}
	writeDump([]map[string]string{owner})
	page, err := manager.DiagnosticPM2Logs(ctx, root, PM2AuditQuery{Source: "err", Since: "2026-11-19 11:30:00"})
	if err != nil || len(page.Lines) != 1 || page.Lines[0].Timestamp == nil ||
		!strings.Contains(page.Output, "matched") || strings.Contains(page.Output, "fixture-secret") {
		t.Fatalf("filtered historical logs: %+v (%v)", page, err)
	}
	if filtered, err := manager.DiagnosticPM2Logs(ctx, root, PM2AuditQuery{Query: "fixture-secret"}); err != nil || filtered.Total != 0 {
		t.Fatalf("keyword filter matched a redacted value: %+v (%v)", filtered, err)
	}
	writeDump([]map[string]string{owner, {"pm_cwd": filepath.Join(root, "other"), "pm_err_log_path": logPath}})
	if _, err := manager.DiagnosticPM2Logs(ctx, root, PM2AuditQuery{}); err == nil {
		t.Fatal("diagnostics must reject log paths shared with another project")
	}
}

func serveDiagnosticPM2Fixture(listener *net.UnixListener, reply []byte) error {
	_ = listener.SetDeadline(time.Now().Add(3 * time.Second))
	connection, err := listener.AcceptUnix()
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	var header [1]byte
	if _, err := io.ReadFull(connection, header[:]); err != nil {
		return err
	}
	if header[0] != 0x12 {
		return fmt.Errorf("unexpected frame header %x", header[0])
	}
	readPart := func() ([]byte, error) {
		var size uint32
		if err := binary.Read(connection, binary.BigEndian, &size); err != nil {
			return nil, err
		}
		if size > 4096 {
			return nil, fmt.Errorf("unexpected PM2 request part length %d", size)
		}
		part := make([]byte, size)
		_, err := io.ReadFull(connection, part)
		return part, err
	}
	call, err := readPart()
	if err != nil {
		return err
	}
	if string(call) != `j:{"type":"call","method":"getMonitorData","args":[{}]}` {
		return fmt.Errorf("unexpected PM2 method: %s", call)
	}
	id, err := readPart()
	if err != nil {
		return err
	}
	frame := []byte{0x12}
	for _, part := range [][]byte{append([]byte("j:"), reply...), id} {
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(part)))
		frame = append(frame, part...)
	}
	if _, err := connection.Write(frame[:3]); err != nil {
		return err
	}
	_, err = connection.Write(frame[3:])
	return err
}
