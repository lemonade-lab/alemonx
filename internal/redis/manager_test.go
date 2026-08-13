package redis

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// freePort reserves a loopback port and releases it so the manager can bind it
// without racing the test helper itself.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(filepath.Join(t.TempDir(), "alx-redis.json"))
}

func TestManagerStartsAndStopsTemporaryRedis(t *testing.T) {
	manager := newTestManager(t)
	status := manager.Status()
	if status.Running || status.Managed || status.Port != DefaultPort {
		t.Fatalf("default status = %+v", status)
	}
	if err := manager.Configure(freePort(t), false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	status = manager.Status()
	if !status.Running || !status.Managed || status.External {
		t.Fatalf("running status = %+v", status)
	}
	if status.Port != manager.config.Port {
		t.Fatalf("port mismatch: config %d, status %d", manager.config.Port, status.Port)
	}
	if !ping(t, status.Address) {
		t.Fatalf("embedded Redis did not answer PING at %s", status.Address)
	}
	message, err := manager.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "已停止") {
		t.Fatalf("stop message = %q", message)
	}
	status = manager.Status()
	if status.Running || status.Managed {
		t.Fatalf("stopped status = %+v", status)
	}
}

func TestManagerSkipsStartWhenExternalRedisOccupiesPort(t *testing.T) {
	external := miniredis.NewMiniRedis()
	if err := external.StartAddr("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer external.Close()
	port, err := strconv.Atoi(external.Port())
	if err != nil {
		t.Fatal(err)
	}

	manager := newTestManager(t)
	if err := manager.Configure(port, false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if !status.Running || !status.External || status.Managed || !status.Skipped {
		t.Fatalf("external status = %+v", status)
	}
	if !strings.Contains(status.Message, "已跳过启动") {
		t.Fatalf("external message = %q", status.Message)
	}
	if manager.server != nil {
		t.Fatal("manager should not own a server when reusing external Redis")
	}
	message, err := manager.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "外部 Redis") {
		t.Fatalf("external stop message = %q", message)
	}
}

func TestManagerRefusesForeignServiceOnPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}(connection)
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port

	manager := newTestManager(t)
	if err := manager.Configure(port, false, false); err != nil {
		t.Fatal(err)
	}
	// The foreign listener does not answer PING, so probing takes up to the
	// 1.5s deadline; the manager must reject rather than silently skip.
	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "已被其他程序占用") {
		t.Fatalf("start on foreign service = %v", err)
	}
	status := manager.Status()
	if status.Running || status.External || status.Skipped {
		t.Fatalf("status after failed start = %+v", status)
	}
}

func TestManagerConfigureValidatesPortAndPersists(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.Configure(0, true, false); err == nil {
		t.Fatal("port 0 should be rejected")
	}
	if err := manager.Configure(70000, true, false); err == nil {
		t.Fatal("port 70000 should be rejected")
	}
	port := freePort(t)
	if err := manager.Configure(port, true, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manager.path)
	if err != nil {
		t.Fatal(err)
	}
	var stored Config
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Port != port || !stored.AutoStart {
		t.Fatalf("persisted config = %+v", stored)
	}
	reloaded := NewManager(manager.path)
	if reloaded.config.Port != port || !reloaded.config.AutoStart {
		t.Fatalf("reloaded config = %+v", reloaded.config)
	}
}

func TestManagerConfigureRestartsRunningRedisOnPortChange(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.Configure(freePort(t), false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	oldPort := manager.config.Port
	nextPort := freePort(t)
	if err := manager.Configure(nextPort, false, false); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if !status.Running || !status.Managed || status.Port != nextPort {
		t.Fatalf("status after port change = %+v", status)
	}
	if status.Port == oldPort {
		t.Fatal("port did not change")
	}
	if !ping(t, status.Address) {
		t.Fatalf("restarted Redis did not answer PING at %s", status.Address)
	}
	manager.Close()
	if manager.Status().Running {
		t.Fatal("Close should stop the managed Redis")
	}
}

func TestManagerDisabledForbidsStartAndClosesRunningServer(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.Configure(freePort(t), false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Configure(manager.config.Port, true, true); err != nil {
		t.Fatal(err)
	}
	status := manager.Status()
	if !status.Disabled || status.Running || status.Managed {
		t.Fatalf("disabled status = %+v", status)
	}
	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "禁用") {
		t.Fatalf("start while disabled = %v", err)
	}
	if _, err := manager.Restart(); err == nil || !strings.Contains(err.Error(), "禁用") {
		t.Fatalf("restart while disabled = %v", err)
	}
	// Re-enabling keeps the stored port and lets the manager start again.
	if err := manager.Configure(manager.config.Port, true, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	status = manager.Status()
	if status.Disabled || !status.Running || !status.Managed {
		t.Fatalf("reenabled status = %+v", status)
	}
}

func ping(t *testing.T, address string) bool {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	if _, err := connection.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return false
	}
	buffer := make([]byte, 64)
	count, err := connection.Read(buffer)
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(buffer[:count]), "+PONG")
}
