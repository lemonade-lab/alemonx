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

func isolateGitHubConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

func TestStartDeviceFlowUsesConfiguredClientID(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "")
	isolateGitHubConfig(t)
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
	isolateGitHubConfig(t)

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
		if r.Header.Get("Authorization") != "Bearer gho_test_token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
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
	// Start without the in-memory login name or cached /user response.
	cacheLogin("")
	httpcache.Evict(userServer.URL)
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
	isolateGitHubConfig(t)
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
	isolateGitHubConfig(t)
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

func TestIdentityFlowUsesFreshGitHubIDWithoutStoringToken(t *testing.T) {
	t.Setenv("ALEMONX_GITHUB_CLIENT_ID", "identity-test-client")
	isolateGitHubConfig(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device":
			_, _ = w.Write([]byte(`{"device_code":"test-device","user_code":"TEST-CODE","verification_uri":"https://github.com/login/device","expires_in":900,"interval":5}`))
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"identity-test-token"}`))
		case "/user":
			if r.Header.Get("Authorization") != "Bearer identity-test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":123456,"login":"renamed-account"}`))
		}
	}))
	defer upstream.Close()
	previousDevice, previousToken, previousUser := deviceCodeURL, tokenURL, userURL
	deviceCodeURL, tokenURL, userURL = upstream.URL+"/device", upstream.URL+"/token", upstream.URL+"/user"
	t.Cleanup(func() { deviceCodeURL, tokenURL, userURL = previousDevice, previousToken, previousUser })

	flow, err := StartIdentityFlow("binding:account-a")
	if err != nil {
		t.Fatal(err)
	}
	if result, _ := PollDeviceFlow(flow.FlowID); result.Status != "expired" {
		t.Fatal("shared token flow accepted an identity flow ID")
	}
	if result, _ := PollIdentityFlow(flow.FlowID, "binding:account-b"); result.Status != "expired" {
		t.Fatal("a different session accessed the binding flow")
	}
	if result, _ := PollIdentityFlow(flow.FlowID, "binding:account-a"); result.Status != "pending" {
		t.Fatal("early request bypassed the polling interval")
	}
	flowsMu.Lock()
	flows[flow.FlowID].nextPoll = time.Now().Add(-time.Second)
	flowsMu.Unlock()
	result, err := PollIdentityFlow(flow.FlowID, "binding:account-a")
	if err != nil || result.Status != "ok" || result.UserID != 123456 || result.Login != "renamed-account" {
		t.Fatalf("poll = %#v, %v", result, err)
	}
	if _, err := os.Stat(httpcache.TokenPath()); !os.IsNotExist(err) {
		t.Fatal("identity flow persisted the shared GitHub token")
	}
	if result, _ := PollIdentityFlow(flow.FlowID, "binding:account-a"); result.Status != "expired" {
		t.Fatal("completed flow was reusable")
	}
}
