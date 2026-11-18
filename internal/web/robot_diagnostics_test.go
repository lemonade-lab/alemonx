package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"alemonx/internal/robot"
)

func TestDiagnosticPortUsesObservedPortAndSeparatesEvidence(t *testing.T) {
	var hits atomic.Int32
	var statusCode atomic.Int32
	statusCode.Store(http.StatusOK)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(int(statusCode.Load()))
	}))
	defer endpoint.Close()
	port := endpoint.Listener.Addr().(*net.TCPAddr).Port
	root := t.TempDir()
	t.Setenv("PM2_HOME", t.TempDir())
	t.Setenv("PM2_DAEMON_RPC_PORT", "")
	configured := 12345
	if configured == port {
		configured++
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"port-fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alemon.config.yaml"), []byte("serverPort: "+strconv.Itoa(configured)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &server{runtimePorts: map[string]runtimePortState{root: {App: port}}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	observe := func(pm2 []robot.DiagnosticPM2Process) robot.DiagnosticPort {
		t.Helper()
		ports, err := s.effectiveDiagnosticPorts(ctx, root, pm2)
		if err != nil || len(ports) != 1 {
			t.Fatalf("diagnostic ports: %+v (%v)", ports, err)
		}
		return ports[0]
	}
	unverified := observe(nil)
	if unverified.ConfiguredPort != configured || unverified.ActualPort != port ||
		unverified.TCP.State != "reachable" || unverified.Ownership.State == "project" ||
		unverified.ApplicationReadiness.State != "unknown" || hits.Load() != 0 {
		t.Fatalf("TCP reachability promoted to application readiness: %+v", unverified)
	}
	if len(portListenerPIDsContext(ctx, port, true)) == 0 {
		t.Skip("host could not verify listener PID")
	}
	known := []robot.DiagnosticPM2Process{{PID: os.Getpid(), Status: "online"}}
	if verified := observe(known); verified.Ownership.State != "project" ||
		verified.ApplicationReadiness.State != "ready" || verified.HTTPStatus != http.StatusOK {
		t.Fatalf("owned HTTP endpoint: %+v", verified)
	}
	statusCode.Store(http.StatusServiceUnavailable)
	if failing := observe(known); failing.TCP.State != "reachable" || failing.ApplicationReadiness.State != "not_ready" {
		t.Fatalf("HTTP failure was not distinguished from TCP reachability: %+v", failing)
	}
	if platform := s.robotDiagnosticSnapshot(ctx, root).PlatformConnection; platform.State != "unknown" {
		t.Fatalf("platform connection cannot be inferred from local TCP/HTTP: %+v (%s)", platform, runtime.GOOS)
	}
}
