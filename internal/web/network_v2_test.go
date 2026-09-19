package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"alemonx/internal/systemnetwork"
	"github.com/gin-gonic/gin"
)

func TestNetworkV2RejectsLegacyAndRevisionConflict(t *testing.T) {
	m, _ := systemnetwork.NewAt(filepath.Join(t.TempDir(), "network.json"))
	s := &server{network: m}
	for _, body := range []string{`{"routes":{}}`, `{"connection":{"mode":"auto"}}`, `{"version":2,"overrides":{}}`} {
		out := httptest.NewRecorder()
		s.systemNetworkHandler(out, httptest.NewRequest("PUT", "/api/v1/system/network", strings.NewReader(body)))
		if out.Code != 400 || !strings.Contains(out.Body.String(), "升级") {
			t.Fatal("old contract not rejected", out.Code, out.Body.String())
		}
	}
	c := m.Settings().Config
	c.Mode = "direct"
	raw, _ := json.Marshal(c)
	first := httptest.NewRecorder()
	s.systemNetworkHandler(first, httptest.NewRequest("PUT", "/api/v1/system/network", bytes.NewReader(raw)))
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	second := httptest.NewRecorder()
	s.systemNetworkHandler(second, httptest.NewRequest("PUT", "/api/v1/system/network", bytes.NewReader(raw)))
	if second.Code != 409 {
		t.Fatal("revision overwritten", second.Body.String())
	}
	read := httptest.NewRecorder()
	s.systemNetworkHandler(read, httptest.NewRequest("GET", "/api/v1/system/network", nil))
	if strings.Contains(read.Body.String(), "overrides") || strings.Contains(read.Body.String(), "routes") {
		t.Fatal("hidden rules exposed", read.Body.String())
	}
}

func TestNetworkRequestBodiesNeverLogged(t *testing.T) {
	for _, body := range []string{`{"proxy":{"url":"http://alice:secret@localhost:1080","username":"alice","password":"secret"}}`, "invalid raw secret"} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("PUT", "/api/v1/system/network", strings.NewReader(body))
		if result := (&server{}).loggableRequestBody(ctx); result != "" {
			t.Fatal("network payload logged")
		}
		remaining, _ := io.ReadAll(ctx.Request.Body)
		if string(remaining) != body {
			t.Fatal("logger consumed payload")
		}
	}
}

func TestDSHNetworkBridgeRejectsInvalidOrigins(t *testing.T) {
	for _, address := range []string{"http://api.deepseek.com/chat/completions", "https://api.deepseek.com.evil.test/", "https://user:secret@api.deepseek.com/", "https://127.0.0.1/"} {
		request := httptest.NewRequest("POST", "http://localhost/network", strings.NewReader("{}"))
		request.Header.Set("X-ALX-Network-URL", address)
		out := httptest.NewRecorder()
		(&server{}).dshBridgeNetwork(out, request)
		if out.Code != http.StatusBadRequest {
			t.Fatal("unsafe AI origin accepted", address)
		}
	}
	out := httptest.NewRecorder()
	(&server{}).dshBridgeHandler(out, httptest.NewRequest("POST", "http://localhost/network", nil))
	if out.Code == 200 {
		t.Fatal("unauthenticated provider relay")
	}
}

func TestGoalDownloadsUseAuthenticatedHost(t *testing.T) {
	m, _ := systemnetwork.NewAt("")
	out := httptest.NewRecorder()
	(&server{network: m}).listGoals(out, httptest.NewRequest("GET", "/api/v1/goals", nil))
	if !strings.Contains(out.Body.String(), "/api/v1/goals/download?id=mobile") {
		t.Fatal("browser bypasses network policy", out.Body.String())
	}
	out = httptest.NewRecorder()
	(&server{}).downloadGoal(out, httptest.NewRequest("GET", "/api/v1/goals/download?id=unknown", nil))
	if out.Code != 404 {
		t.Fatal("download accepts arbitrary target")
	}
}
