package githubauth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alemonx/internal/httpcache"
)

func TestStartDeviceFlowUsesConfiguredClientID(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "")
	t.Setenv("HOME", t.TempDir())
	var receivedClientID string
	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		receivedClientID = r.PostForm.Get("client_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dc-1","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`))
	}))
	defer deviceServer.Close()
	deviceCodeURL = deviceServer.URL
	t.Cleanup(func() { deviceCodeURL = "https://github.com/login/device/code" })

	flow, err := StartDeviceFlow()
	if err != nil {
		t.Fatalf("StartDeviceFlow with builtin client id: %v", err)
	}
	if receivedClientID != defaultClientID {
		t.Fatalf("device flow client_id = %q, want builtin %q", receivedClientID, defaultClientID)
	}
	if flow.UserCode != "ABCD-1234" {
		t.Fatalf("flow = %#v", flow)
	}
}

func TestDeviceFlowEndToEnd(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "client-123")
	t.Setenv("HOME", t.TempDir())

	var polls atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if polls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"error":"authorization_pending","error_description":"waiting"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"gho_test_token"}`))
	}))
	defer tokenServer.Close()
	deviceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dc-1","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`))
	}))
	defer deviceServer.Close()
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer userServer.Close()
	deviceCodeURL = deviceServer.URL
	tokenURL = tokenServer.URL
	userURL = userServer.URL
	t.Cleanup(func() {
		deviceCodeURL = "https://github.com/login/device/code"
		tokenURL = "https://github.com/login/oauth/access_token"
		userURL = "https://api.github.com/user"
	})

	flow, err := StartDeviceFlow()
	if err != nil {
		t.Fatalf("StartDeviceFlow: %v", err)
	}
	if flow.UserCode != "ABCD-1234" || flow.FlowID == "" || flow.Interval < 5 {
		t.Fatalf("flow = %#v", flow)
	}
	first, err := PollDeviceFlow(flow.FlowID)
	if err != nil || first.Status != "pending" {
		t.Fatalf("first poll = %#v, %v", first, err)
	}
	second, err := PollDeviceFlow(flow.FlowID)
	if err != nil || second.Status != "ok" || second.Login != "octocat" {
		t.Fatalf("second poll = %#v, %v", second, err)
	}
	data, err := os.ReadFile(httpcache.TokenPath())
	if err != nil || strings.TrimSpace(string(data)) != "gho_test_token" {
		t.Fatalf("token file = %q, %v", data, err)
	}
	status, err := Status()
	if err != nil || !status.LoggedIn || status.Login != "octocat" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	if err := Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := os.Stat(httpcache.TokenPath()); !os.IsNotExist(err) {
		t.Fatalf("token file should be removed after logout")
	}
	after, _ := Status()
	if after.LoggedIn {
		t.Fatal("status must be logged out after logout")
	}
}

func TestPollExpiredFlowReturnsExpired(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "client-123")
	flowsMu.Lock()
	flows["gone"] = &deviceFlowState{expiresAt: time.Now().Add(-time.Hour)}
	flowsMu.Unlock()
	result, err := PollDeviceFlow("gone")
	if err != nil || result.Status != "expired" {
		t.Fatalf("expired poll = %#v, %v", result, err)
	}
	if result, _ := PollDeviceFlow("unknown"); result.Status != "expired" {
		t.Fatalf("unknown flow = %#v", result)
	}
}

func TestSaveManualTokenAndStatus(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "")
	t.Setenv("HOME", t.TempDir())
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"manual-user"}`))
	}))
	defer userServer.Close()
	userURL = userServer.URL
	t.Cleanup(func() { userURL = "https://api.github.com/user" })

	status, err := SaveManualToken("ghp_manual")
	if err != nil || !status.LoggedIn || status.Login != "manual-user" {
		t.Fatalf("SaveManualToken = %#v, %v", status, err)
	}
	data, err := os.ReadFile(httpcache.TokenPath())
	if err != nil || strings.TrimSpace(string(data)) != "ghp_manual" {
		t.Fatalf("token file = %q, %v", data, err)
	}
}

func TestClientIDSourceAndSaveOverride(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "")
	t.Setenv("HOME", t.TempDir())
	if value, source := ClientIDSource(); value != defaultClientID || source != "builtin" {
		t.Fatalf("builtin default expected, got %q/%q", value, source)
	}
	if err := SaveClientID("Iv1.custom"); err != nil {
		t.Fatalf("SaveClientID: %v", err)
	}
	if value, source := ClientIDSource(); value != "Iv1.custom" || source != "file" {
		t.Fatalf("file override = %q/%q", value, source)
	}
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "Iv1.env")
	if value, source := ClientIDSource(); value != "Iv1.env" || source != "env" {
		t.Fatalf("env must win over file, got %q/%q", value, source)
	}
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "")
	if err := SaveClientID(""); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	if value, source := ClientIDSource(); value != defaultClientID || source != "builtin" {
		t.Fatalf("cleared override should fall back to builtin, got %q/%q", value, source)
	}
}
