package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// testListenerPort binds an ephemeral loopback port for occupancy tests and
// returns the listener plus its port. Callers close the listener explicitly to
// flip the port back to free.
func testListenerPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("test listener: %v", err)
	}
	return listener, listener.Addr().(*net.TCPAddr).Port
}

func TestSniffPortReportsOccupancy(t *testing.T) {
	listener, port := testListenerPort(t)
	occupied, occupants := sniffPort(port)
	if !occupied {
		t.Fatalf("sniffPort(%d) = free, want occupied", port)
	}
	if len(occupants) == 0 {
		// The bind probe is authoritative; occupant identity depends on
		// platform tooling (lsof/netstat) and is allowed to be empty.
		t.Log("occupant identity unavailable, occupancy still reported")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	occupied, _ = sniffPort(port)
	if occupied {
		t.Fatalf("sniffPort(%d) = occupied after close, want free", port)
	}
}

func TestRobotPortsHandlerReportsOccupancy(t *testing.T) {
	listener, port := testListenerPort(t)
	defer listener.Close()
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	writeFixture(t, root, "alemon.config.yaml", "serverPort: "+strconv.Itoa(port)+"\nport: "+strconv.Itoa(port)+"\n")
	s := newStatefulTestServer()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/robot/ports?"+url.Values{"root": []string{root}}.Encode(),
		nil,
	)
	recorder := httptest.NewRecorder()
	s.robotPortsHandler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ports status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Items []struct {
			Kind     string `json:"kind"`
			Port     int    `json:"port"`
			Occupied bool   `json:"occupied"`
			PID      int    `json:"pid"`
			Owned    bool   `json:"owned"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode ports payload: %v", err)
	}
	found := false
	for _, item := range payload.Items {
		if item.Port != port || item.Kind != "app" {
			continue
		}
		found = true
		if !item.Occupied {
			t.Fatalf("app port %d reported free while listener is bound", port)
		}
		if item.PID <= 0 {
			t.Logf("occupant PID not reported for port %d (tooling unavailable)", port)
		}
		if item.Owned {
			t.Fatalf("foreign test listener on port %d reported as owned", port)
		}
	}
	if !found {
		t.Fatalf("app port %d missing from items: %s", port, recorder.Body.String())
	}
}

func TestRobotTasksStartBlockedByForeignPort(t *testing.T) {
	listener, port := testListenerPort(t)
	defer listener.Close()
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0","scripts":{"dev":"node index.js"}}`)
	writeFixture(t, root, "alemon.config.yaml", "serverPort: "+strconv.Itoa(port)+"\n")
	s := newStatefulTestServer()
	body := strings.NewReader(`{"root":` + strconv.Quote(root) + `,"action":"dev","ready":"app"}`)
	recorder := httptest.NewRecorder()
	s.robotTasksHandler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/robot/tasks", body))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("start status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "启动前端口检查未通过") {
		t.Fatalf("missing proactive port error: %s", recorder.Body.String())
	}
}

func TestRobotStartPortBlockersAllowFreePorts(t *testing.T) {
	appListener, appPort := testListenerPort(t)
	_ = appListener.Close()
	testListener, testPort := testListenerPort(t)
	_ = testListener.Close()
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	writeFixture(t, root, "alemon.config.yaml", "serverPort: "+strconv.Itoa(appPort)+"\nport: "+strconv.Itoa(testPort)+"\n")
	s := newStatefulTestServer()
	if blockers := s.robotStartPortBlockers(root, "app"); len(blockers) != 0 {
		t.Fatalf("free ports reported blocked: %v", blockers)
	}
}

func TestRobotProxiesRejectUnconfiguredPorts(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	query := url.Values{"root": []string{root}}.Encode()

	app := httptest.NewRecorder()
	s.robotAppHandler(app, httptest.NewRequest(http.MethodGet, "/api/v1/robot/app/?"+query, nil))
	if app.Code != http.StatusConflict || !strings.Contains(app.Body.String(), "serverPort") {
		t.Fatalf("unconfigured app proxy = %d, %s", app.Code, app.Body.String())
	}

	test := httptest.NewRecorder()
	s.robotTestHandler(test, httptest.NewRequest(http.MethodGet, "/api/v1/robot/test/?"+query, nil))
	if test.Code != http.StatusConflict || !strings.Contains(test.Body.String(), "服务端口") {
		t.Fatalf("unconfigured test proxy = %d, %s", test.Code, test.Body.String())
	}
}

func TestWaitPortFreeOnDetectsNonHTTPListener(t *testing.T) {
	listener, port := testListenerPort(t)
	s := newStatefulTestServer()
	if s.waitPortFreeOn(port, 1) {
		t.Fatalf("TCP listener on %d reported free", port)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if !s.waitPortFreeOn(port, 1) {
		t.Fatalf("closed port %d reported occupied", port)
	}
}
