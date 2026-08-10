package robot

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// TestTestPortReadsAndSavesTopLevelPort covers the "测试" flow: reading the
// configured top-level port from alemon.config.yaml, the default fallback, and
// writing a new port (replacing an existing value or appending one). The
// serverPort key must never be mistaken for the test port.
func TestTestPortReadsAndSavesTopLevelPort(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// No config file yet: default port, not configured.
	info, err := (Manager{}).TestPort(root)
	if err != nil || info.Port != defaultTestPort || info.Configured {
		t.Fatalf("default port = %+v, %v", info, err)
	}
	// Save a port then read it back.
	if _, err := (Manager{}).SaveTestPort(root, 19192); err != nil {
		t.Fatalf("SaveTestPort: %v", err)
	}
	info, _ = (Manager{}).TestPort(root)
	if info.Port != 19192 || !info.Configured {
		t.Fatalf("port after save = %+v, want 19192 configured", info)
	}
	// Replace an existing port.
	if _, err := (Manager{}).SaveTestPort(root, 20001); err != nil {
		t.Fatalf("SaveTestPort replace: %v", err)
	}
	info, _ = (Manager{}).TestPort(root)
	if info.Port != 20001 || !info.Configured {
		t.Fatalf("port after replace = %+v, want 20001 configured", info)
	}
	// Invalid port is rejected.
	if _, err := (Manager{}).SaveTestPort(root, 70000); err == nil {
		t.Fatal("invalid port should be rejected")
	}
	// serverPort must not be read as the test port.
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "serverPort: 18110\nport: 20002\n")
	info, _ = (Manager{}).TestPort(root)
	if info.Port != 20002 || !info.Configured {
		t.Fatalf("port with serverPort present = %+v, want 20002 configured", info)
	}
}

// TestTestPortReachableProbesOnlineEndpoint verifies the probe reports a live
// listener as reachable and a closed port as unreachable.
func TestTestPortReachableProbesOnlineEndpoint(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// Closed port is unreachable.
	if _, err := (Manager{}).SaveTestPort(root, 65531); err != nil {
		t.Fatal(err)
	}
	reachable, port, err := (Manager{}).TestPortReachable(root)
	if err != nil || reachable || port != 65531 {
		t.Fatalf("closed port probe = reachable %v port %d err %v", reachable, port, err)
	}
	// A live server answering /api/online is reachable.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind listener: %v", err)
	}
	defer listener.Close()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"service online"}`))
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	livePort := listener.Addr().(*net.TCPAddr).Port
	if _, err := (Manager{}).SaveTestPort(root, livePort); err != nil {
		t.Fatal(err)
	}
	reachable, port, err = (Manager{}).TestPortReachable(root)
	if err != nil || !reachable || port != livePort {
		t.Fatalf("live port probe = reachable %v port %d err %v", reachable, port, err)
	}
}

// TestTestSandboxAvailableRequiresNoLogin ensures the sandbox-mode heuristic
// treats an empty config as sandbox, and a configured login/platform as not.
func TestTestSandboxAvailableRequiresNoLogin(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	// No alemon.config.yaml: sandbox mode by default.
	available, err := (Manager{}).TestSandboxAvailable(root)
	if err != nil || !available {
		t.Fatalf("default sandbox = %v, %v", available, err)
	}
	// A port-only config stays in sandbox mode.
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "port: 17117\n")
	available, _ = (Manager{}).TestSandboxAvailable(root)
	if !available {
		t.Fatal("port-only config should stay sandbox")
	}
	// login disables sandbox mode.
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "port: 17117\nlogin: discord\n")
	available, _ = (Manager{}).TestSandboxAvailable(root)
	if available {
		t.Fatal("login config must not be sandbox")
	}
	// platform disables sandbox mode; empty value does not.
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "platform: ''\n")
	available, _ = (Manager{}).TestSandboxAvailable(root)
	if !available {
		t.Fatal("empty platform should stay sandbox")
	}
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "platform: discord\n")
	available, _ = (Manager{}).TestSandboxAvailable(root)
	if available {
		t.Fatal("platform config must not be sandbox")
	}
	// A commented-out login must not count.
	writeWebViewFixture(t, filepath.Join(root, "alemon.config.yaml"), "# login: discord\nport: 17117\n")
	available, _ = (Manager{}).TestSandboxAvailable(root)
	if !available {
		t.Fatal("commented login should stay sandbox")
	}
}
