package systemnetwork

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConnectionCredentialsOverridesAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	m, _ := NewAt(path)
	if m.Settings().Connection.Mode != ModeAuto || len(m.Settings().Overrides) != 0 {
		t.Fatal("new configs must inherit auto")
	}
	next, err := m.Save(Settings{Connection: &RouteSettings{Mode: ModeManual, ProxyURL: "http://user:secret@127.0.0.1:7890"}, Overrides: map[Route]RouteSettings{RouteNPM: {Mode: ModeDirect}}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Connection.ProxyURL != "http://127.0.0.1:7890" || !next.Connection.HasCredentials {
		t.Fatal("credentials not redacted")
	}
	if _, err = m.Save(next); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded := reloaded.Settings()
	if loaded.Migration == nil || !loaded.Migration.Pending {
		t.Fatal("legacy file requires confirmation")
	}
	loaded.Config, loaded.Migration = nil, nil
	if !reflect.DeepEqual(loaded, next) {
		t.Fatal("reload changed effective configuration")
	}
	request, _ := http.NewRequest("GET", "https://github.com/example", nil)
	proxy, _ := reloaded.proxyFor(request)
	if proxy.String() != "http://user:secret@127.0.0.1:7890" {
		t.Fatal("credentials lost")
	}
	if reloaded.Settings().Routes[RouteNPM].Mode != ModeDirect {
		t.Fatal("exception ignored")
	}
	next.Connection.ClearCredentials = true
	cleared, err := m.Save(next)
	if err != nil || cleared.Connection.HasCredentials || cleared.Routes[RouteGitHub].HasCredentials {
		t.Fatal("credential clearing failed", err)
	}
}

func TestDraftTestDoesNotSave(t *testing.T) {
	m, _ := NewAt(filepath.Join(t.TempDir(), "network.json"))
	before := m.Settings()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	old := testEndpoints[RouteGitHub]
	testEndpoints[RouteGitHub] = server.URL
	defer func() { testEndpoints[RouteGitHub] = old }()
	result, err := m.Preview(context.Background(), RouteGitHub, Settings{Connection: &RouteSettings{Mode: ModeDirect}})
	if err != nil || !result.OK {
		t.Fatal(result, err)
	}
	if !reflect.DeepEqual(before, m.Settings()) {
		t.Fatal("draft changed active settings")
	}
	if _, err := os.Stat(m.path); !os.IsNotExist(err) {
		t.Fatal("draft wrote config file")
	}
	_, err = m.Preview(context.Background(), RouteGitHub, Settings{Connection: &RouteSettings{Mode: ModeManual, ProxyURL: "socks5://user:secret@localhost:7890"}})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid proxy or credential leak", err)
	}
}

func TestCustomProxyTransportsRequestsWithoutMirror(t *testing.T) {
	requests := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.String() != "http://github.com/resource" {
			t.Errorf("unexpected proxy target %s", r.URL)
		}
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("proxy authentication missing")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	address, _ := url.Parse(proxy.URL)
	address.User = url.UserPassword("user", "secret")
	m, _ := NewAt("")
	if _, err := m.Save(Settings{Connection: &RouteSettings{Mode: ModeManual, ProxyURL: address.String()}}); err != nil {
		t.Fatal(err)
	}
	response, err := m.Client(0).Get("http://github.com/resource")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests != 1 || response.StatusCode != 200 {
		t.Fatal("request did not use configured proxy")
	}
}

func TestLegacyRoutesRetainedAndCustomMirrorNeverFallsBack(t *testing.T) {
	m, _ := NewAt("")
	next, err := m.Save(Settings{Routes: map[Route]RouteSettings{RouteGitHub: {Mode: ModeManual, ProxyURL: "http://localhost:7890"}, RouteNPM: {Mode: ModeSystem}}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Connection != nil || next.Overrides[RouteGitHub].Mode != ModeManual || next.Overrides[RouteNPM].Mode != ModeSystem {
		t.Fatal("legacy routing lost")
	}
	next.Connection = &RouteSettings{Mode: ModeDirect}
	if _, err := m.Save(next); err != nil {
		t.Fatal(err)
	}
	if m.Settings().Routes[RouteGitHub].Mode != ModeManual {
		t.Fatal("migration dropped override")
	}
	m.Save(Settings{Routes: map[Route]RouteSettings{RouteGitHub: {Mode: ModeCustomMirror, MirrorURL: "https://example.com/{url}"}}})
	target, _ := url.Parse("https://api.github.com/")
	if m.allowsOfficialFallback(target) {
		t.Fatal("custom mirror allows direct fallback")
	}
	count := 0
	transport := mirrorTransport{manager: m, base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 403, Body: http.NoBody}, nil
	})}
	req, _ := http.NewRequest("GET", target.String(), nil)
	transport.RoundTrip(req)
	if count != 1 {
		t.Fatal("custom mirror retried official API")
	}
}
