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
	if status.Running || status.Managed || status.Port != DefaultPort || !status.AutoStart {
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
	if !ping(t, status.Address, manager.config.Password) {
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

func TestPublicProxyRequiresInstancePassword(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.Configure(freePort(t), false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	connection, err := net.DialTimeout("tcp", manager.Status().Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 128)
	count, err := connection.Read(reply)
	if err != nil || !strings.HasPrefix(string(reply[:count]), "-NOAUTH") {
		t.Fatalf("unauthenticated proxy reply = %q, %v", reply[:count], err)
	}
	if !ping(t, manager.Status().Address, manager.config.Password) {
		t.Fatal("authenticated proxy did not forward PING")
	}
}

func TestManagerRestoresPersistedDataAfterRestart(t *testing.T) {
	manager := newTestManager(t)
	port := freePort(t)
	if err := manager.Configure(port, true, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	database := manager.server.DB(0)
	if err := database.Set("string", "value"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Push("list", "first", "second"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SetAdd("set", "one", "two"); err != nil {
		t.Fatal(err)
	}
	database.HSet("hash", "field", "value")
	if _, err := database.ZAdd("zset", 2.5, "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.XAdd("stream", "1-0", []string{"field", "value"}); err != nil {
		t.Fatal(err)
	}
	database.SetTTL("string", time.Minute)
	manager.Close()

	restarted := NewManager(manager.path)
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restored := restarted.server.DB(0)
	if value, err := restored.Get("string"); err != nil || value != "value" {
		t.Fatalf("restored string = %q, %v", value, err)
	}
	if values, err := restored.List("list"); err != nil || strings.Join(values, ",") != "first,second" {
		t.Fatalf("restored list = %#v, %v", values, err)
	}
	if values, err := restored.Members("set"); err != nil || strings.Join(values, ",") != "one,two" {
		t.Fatalf("restored set = %#v, %v", values, err)
	}
	if value := restored.HGet("hash", "field"); value != "value" {
		t.Fatalf("restored hash = %q", value)
	}
	if score, err := restored.ZScore("zset", "member"); err != nil || score != 2.5 {
		t.Fatalf("restored zset = %v, %v", score, err)
	}
	if entries, err := restored.Stream("stream"); err != nil || len(entries) != 1 || entries[0].ID != "1-0" || strings.Join(entries[0].Values, ",") != "field,value" {
		t.Fatalf("restored stream = %#v, %v", entries, err)
	}
	if ttl := restored.TTL("string"); ttl <= 0 {
		t.Fatalf("restored string TTL = %s", ttl)
	}
	if status := restarted.Status(); !status.Persistent || status.LastSaved == "" {
		t.Fatalf("persistent status = %+v", status)
	}
}

func TestSnapshotImportsIntoRESPRedis(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.Configure(freePort(t), false, false); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(); err != nil {
		t.Fatal(err)
	}
	database := manager.server.DB(0)
	if err := database.Set("string", "value"); err != nil {
		t.Fatal(err)
	}
	database.HSet("hash", "field", "value")
	if _, err := database.Push("list", "one", "two"); err != nil {
		t.Fatal(err)
	}
	database.SetTTL("string", time.Minute)
	if err := manager.server.DB(1).Set("db1", "value"); err != nil {
		t.Fatal(err)
	}
	if err := manager.saveSnapshotLocked(); err != nil {
		t.Fatal(err)
	}
	manager.Close()

	destination := miniredis.NewMiniRedis()
	if err := destination.StartAddr("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err := restoreSnapshotToPrivateRedis(destination.Addr(), manager.snapshotPath); err != nil {
		t.Fatal(err)
	}
	restored := destination.DB(0)
	if value, err := restored.Get("string"); err != nil || value != "value" {
		t.Fatalf("imported string = %q, %v", value, err)
	}
	if value := restored.HGet("hash", "field"); value != "value" {
		t.Fatalf("imported hash = %q", value)
	}
	if values, err := restored.List("list"); err != nil || strings.Join(values, ",") != "one,two" {
		t.Fatalf("imported list = %#v, %v", values, err)
	}
	if ttl := restored.TTL("string"); ttl <= 0 {
		t.Fatalf("imported ttl = %s", ttl)
	}
	if value, err := destination.DB(1).Get("db1"); err != nil || value != "value" {
		t.Fatalf("imported db1 value = %q, %v", value, err)
	}
}

func TestPrivateInitializationStatePersists(t *testing.T) {
	manager := newTestManager(t)
	manager.config.PrivateInitialized = true
	if err := manager.saveLocked(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewManager(manager.path)
	if !reloaded.config.PrivateInitialized {
		t.Fatal("private runtime initialization state was not persisted")
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
	if !ping(t, status.Address, manager.config.Password) {
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

func ping(t *testing.T, address string, password ...string) bool {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	if len(password) > 0 {
		if err := sendRESPCommand(connection, [][]byte{[]byte("AUTH"), []byte(password[0])}); err != nil {
			return false
		}
		buffer := make([]byte, 64)
		if count, err := connection.Read(buffer); err != nil || !strings.HasPrefix(string(buffer[:count]), "+OK") {
			return false
		}
	}
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
