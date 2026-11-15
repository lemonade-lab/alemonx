//go:build darwin

package robot

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPM2SocketEnvironmentReusesLegacyDarwinSocket(t *testing.T) {
	base, err := os.MkdirTemp("/tmp", "alx-pm2-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	// Leave room for the 'rpc' prefix on platforms where Node truncates.
	home := filepath.Join(base, strings.Repeat("a", pm2DarwinSocketLimit-len(base)-4))
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	rpc := filepath.Join(home, "rpc.sock")
	legacy := rpc[:pm2DarwinSocketLimit+1]
	shortHome := filepath.Join(base, "home")
	if err := os.Symlink(home, shortHome); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(shortHome, filepath.Base(legacy)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	cmd := exec.Command("pm2", "jlist")
	cmd.Env = []string{"PM2_HOME=" + home}
	if err := applyPM2SocketEnvironment(cmd); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, entry := range cmd.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		got[key] = value
	}
	alias := got["PM2_DAEMON_RPC_PORT"]
	t.Cleanup(func() { _ = os.Remove(filepath.Dir(alias)) })
	if got["PM2_HOME"] != home || len(alias) > pm2DarwinSocketLimit {
		t.Fatalf("unexpected home or socket length")
	}
	info, err := os.Stat(alias)
	if err != nil || info.Mode()&os.ModeSocket == 0 || filepath.Base(alias) != filepath.Base(legacy) {
		t.Fatalf("alias does not resolve to the existing socket: %q, %v", alias, err)
	}
}

func TestPM2SocketEnvironmentShortensFreshLongDarwinHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), strings.Repeat("a", 112))
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("pm2", "jlist")
	cmd.Env = []string{"PM2_HOME=" + home}
	if err := applyPM2SocketEnvironment(cmd); err != nil {
		t.Fatal(err)
	}
	for _, entry := range cmd.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if key != "PM2_DAEMON_RPC_PORT" {
			continue
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Dir(value)) })
		resolved, err := filepath.EvalSymlinks(filepath.Dir(value))
		if err != nil || !sameWorkspacePath(resolved, home) || len(value) > pm2DarwinSocketLimit || filepath.Base(value) != "rpc.sock" {
			t.Fatalf("invalid socket alias %q: %v", value, err)
		}
		return
	}
	t.Fatal("missing abbreviated RPC socket address")
}
