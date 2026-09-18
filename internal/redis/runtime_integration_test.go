package redis

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Opt-in: point ALX_TEST_REDIS_BIN at a real Redis build with redis-cli alongside it.
// No system services, default ports or application data are used.
func TestRealRuntimeInstallAndMigration(t *testing.T) {
	binary := os.Getenv("ALX_TEST_REDIS_BIN")
	if binary == "" {
		t.Skip("set ALX_TEST_REDIS_BIN to test with real Redis")
	}
	raw, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	entry, err := w.Create(runtimeBinaryName())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var index runtimeIndex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			_ = json.NewEncoder(w).Encode(index)
			return
		}
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	index = runtimeIndex{Version: "integration", Assets: []runtimeAsset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: server.URL + "/runtime.zip", Size: int64(archive.Len()), SHA256: fmt.Sprintf("%x", sha256.Sum256(archive.Bytes())), Archive: "zip", Binary: runtimeBinaryName()}}}
	t.Setenv("ALX_REDIS_RUNTIME_INDEX_URL", server.URL+"/index")
	t.Setenv("ALX_REDIS_RUNTIME_BIN", "")
	t.Run("automatic-install", func(t *testing.T) {
		m := NewManager(filepath.Join(t.TempDir(), "alx-redis.json"))
		if err := m.Configure(freePort(t), false, false); err != nil {
			t.Fatal(err)
		}
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			status := m.Status()
			if status.Implementation == "Redis" {
				if !ping(t, status.Address) {
					t.Fatal("installed Redis does not answer via public endpoint")
				}
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("automatic install did not complete: %+v", m.Status())
	})
	for _, directory := range []string{"plain", "Application Support"} {
		t.Run(directory, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), directory)
			m := NewManager(filepath.Join(base, "alx-redis.json"))
			if err := m.Configure(freePort(t), false, false); err != nil {
				t.Fatal(err)
			}
			// Control installation explicitly so Start's asynchronous downloader
			// does not race the fixture's snapshot population and activation.
			m.mu.Lock()
			startErr := m.startLocked()
			m.mu.Unlock()
			if err := startErr; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.Close)
			address := m.Status().Address
			db := m.server.DB(0)
			_ = db.Set("string", "before")
			db.SetTTL("string", time.Minute)
			db.HSet("hash", "field", "value")
			_, _ = db.Push("list", "one", "two")
			_, _ = db.SetAdd("set", "one", "two")
			_, _ = db.ZAdd("zset", 2.5, "member")
			_, _ = db.XAdd("stream", "1-0", []string{"field", "value"})
			_, _ = db.XAdd("stream", "2-0", []string{"field", "second"})
			_ = m.server.DB(1).Set("db1", "other")
			if _, err := downloadAndActivateRuntime(context.Background(), base); err != nil {
				t.Fatal("install:", err)
			}
			if directory == "plain" {
				client, err := net.DialTimeout("tcp", address, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = client.Write([]byte("*1\r\n$4\r\nPING\r\n"))
				_ = client.SetReadDeadline(time.Now().Add(time.Second))
				if line, err := bufio.NewReader(client).ReadString('\n'); err != nil || line != "+PONG\r\n" {
					_ = client.Close()
					t.Fatalf("client: %q %v", line, err)
				}
				if err := m.ActivatePrivateRuntime(); err == nil {
					_ = client.Close()
					t.Fatal("migration must wait for connected clients")
				}
				_ = client.Close()
				deadline := time.Now().Add(time.Second)
				for m.proxy.activeClients() != 0 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				t.Log("active client correctly blocks migration; explicitly retrying after disconnect")
			}
			if err := m.ActivatePrivateRuntime(); err != nil {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				out, _ := exec.CommandContext(ctx, m.privateRuntime(), filepath.Join(m.privateDataDir(), "redis.conf")).CombinedOutput()
				t.Logf("redis-server diagnostic: %s", out)
				t.Fatal("migration:", err)
			}
			if m.Status().Address != address || m.Status().Implementation != "Redis" {
				t.Fatal("public endpoint or implementation incorrect")
			}
			cli := filepath.Join(filepath.Dir(binary), "redis-cli")
			command := func(args ...string) string {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				out, err := exec.CommandContext(ctx, cli, append([]string{"-p", fmt.Sprint(m.config.Port), "--raw"}, args...)...).CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %v %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			for _, check := range []struct {
				args []string
				want string
			}{
				{[]string{"GET", "string"}, "before"}, {[]string{"HGET", "hash", "field"}, "value"},
				{[]string{"LRANGE", "list", "0", "-1"}, "one\ntwo"}, {[]string{"SCARD", "set"}, "2"},
				{[]string{"ZSCORE", "zset", "member"}, "2.5"}, {[]string{"XLEN", "stream"}, "2"},
				{[]string{"-n", "1", "GET", "db1"}, "other"},
			} {
				if got := command(check.args...); got != check.want {
					t.Errorf("%v = %q, want %q", check.args, got, check.want)
				}
			}
			if got := command("TTL", "string"); got == "-1" || got == "-2" {
				t.Errorf("TTL lost: %s", got)
			}
			if got := command("SET", "after-migration", "persistent"); got != "OK" {
				t.Fatal(got)
			}
			m.Close()
			m = NewManager(filepath.Join(base, "alx-redis.json"))
			if err := m.Start(); err != nil {
				t.Fatal("restart:", err)
			}
			defer m.Close()
			if got := command("GET", "after-migration"); got != "persistent" {
				t.Fatalf("restart lost data: %s", got)
			}
		})
	}
}
