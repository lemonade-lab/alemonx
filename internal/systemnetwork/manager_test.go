package systemnetwork

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestManualProxySettingsRedactCredentialsAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	manager, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := manager.Save(Settings{
		Routes: map[Route]RouteSettings{
			RouteGitHub: {Mode: ModeManual, ProxyURL: "http://name:secret@127.0.0.1:7890"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	github := saved.Routes[RouteGitHub]
	if github.ProxyURL != "http://127.0.0.1:7890" || !github.HasCredentials {
		t.Fatalf("public settings leaked or lost credentials: %#v", saved)
	}
	reloaded, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	reloadedGitHub := reloaded.Settings().Routes[RouteGitHub]
	if reloadedGitHub.ProxyURL != "http://127.0.0.1:7890" || !reloadedGitHub.HasCredentials {
		t.Fatalf("persisted settings = %#v", reloaded.Settings())
	}
}

func TestLegacyGlobalModeMigratesToEverySystemRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	if err := os.WriteFile(path, []byte(`{"mode":"manual","proxyUrl":"http://127.0.0.1:7890"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range allRoutes {
		if got := manager.Settings().Routes[route].Mode; got != ModeManual {
			t.Fatalf("legacy route %s = %q, want %q", route, got, ModeManual)
		}
	}
}

func TestDefaultRoutesPreferAvailableMirrors(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	routes := manager.Settings().Routes
	if github := routes[RouteGitHub]; github.Mode != ModeMirror || github.MirrorURL != defaultGitHubMirror {
		t.Fatalf("GitHub defaults = %#v", github)
	}
	if npm := routes[RouteNPM]; npm.Mode != ModeMirror || npm.MirrorURL != defaultNPMMirror {
		t.Fatalf("NPM defaults = %#v", npm)
	}
	if routes[RouteGitee].Mode != ModeDirect || routes[RouteOfficial].Mode != ModeDirect {
		t.Fatalf("unsupported mirror defaults = %#v", routes)
	}
	presets := manager.Settings().MirrorPresets[RouteGitHub]
	if len(presets) < 2 || presets[0].Label != "GHFast（推荐）" || presets[0].Value != defaultGitHubMirror {
		t.Fatalf("GitHub mirror presets = %#v", presets)
	}
}

func TestProxyModesAndLoopbackBypass(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(Settings{
		Routes: map[Route]RouteSettings{
			RouteGitHub: {Mode: ModeManual, ProxyURL: "http://127.0.0.1:7890"},
			RouteGitee:  {Mode: ModeManual, ProxyURL: "http://127.0.0.1:7891"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	external, _ := url.Parse("https://api.github.com/")
	proxy, err := manager.proxyFor(&http.Request{URL: external})
	if err != nil || proxy == nil || proxy.Host != "127.0.0.1:7890" {
		t.Fatalf("manual proxy = %v, %v", proxy, err)
	}
	gitee, _ := url.Parse("https://gitee.com/example/project")
	proxy, err = manager.proxyFor(&http.Request{URL: gitee})
	if err != nil || proxy == nil || proxy.Host != "127.0.0.1:7891" {
		t.Fatalf("Gitee must use its own proxy: %v, %v", proxy, err)
	}
	asset, _ := url.Parse("https://objects.githubusercontent.com/archive.zip")
	proxy, err = manager.proxyFor(&http.Request{URL: asset})
	if err != nil || proxy == nil || proxy.Host != "127.0.0.1:7890" {
		t.Fatalf("GitHub release asset must use GitHub route: %v, %v", proxy, err)
	}
	unknown, _ := url.Parse("https://example.com/resource")
	proxy, err = manager.proxyFor(&http.Request{URL: unknown})
	if err != nil || proxy != nil {
		t.Fatalf("unknown system host must remain direct: %v, %v", proxy, err)
	}
	local, _ := url.Parse("http://127.0.0.1:17390/healthz")
	proxy, err = manager.proxyFor(&http.Request{URL: local})
	if err != nil || proxy != nil {
		t.Fatalf("loopback must bypass proxy: %v, %v", proxy, err)
	}
}

func TestMirrorURLRewritesOnlyTheSelectedRoute(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(Settings{Routes: map[Route]RouteSettings{
		RouteGitHub: {Mode: ModeMirror, MirrorURL: "https://gh-proxy.com/{url}"},
	}}); err != nil {
		t.Fatal(err)
	}
	github, _ := url.Parse("https://api.github.com/repos/lemonade-lab/alx/releases")
	mirror, ok := manager.mirrorFor(github)
	if !ok || mirror != "https://gh-proxy.com/{url}" {
		t.Fatalf("GitHub mirror = %q, %t", mirror, ok)
	}
	rewritten, err := rewriteMirrorURL(mirror, github)
	if err != nil || rewritten.String() != "https://gh-proxy.com/https://api.github.com/repos/lemonade-lab/alx/releases" {
		t.Fatalf("rewritten GitHub URL = %v, %v", rewritten, err)
	}
	gitee, _ := url.Parse("https://gitee.com/example/project")
	if _, ok := manager.mirrorFor(gitee); ok {
		t.Fatal("Gitee must not inherit the GitHub mirror")
	}
	npm, _ := url.Parse("https://registry.npmjs.org/alemonjs?active=true")
	rewritten, err = rewriteMirrorURL(defaultNPMMirror, npm)
	if err != nil || rewritten.String() != "https://registry.npmmirror.com/alemonjs?active=true" {
		t.Fatalf("rewritten NPM URL = %v, %v", rewritten, err)
	}
	node, err := url.Parse("https://nodejs.org/dist/v24.19.0/SHASUMS256.txt")
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err = rewriteMirrorURL(defaultNodeMirror, node)
	if err != nil || rewritten.String() != "https://npmmirror.com/mirrors/node/v24.19.0/SHASUMS256.txt" {
		t.Fatalf("node mirror rewrite = %v, %v", rewritten, err)
	}
}

func TestMirrorTransportRewritesRequestsAndRetriesKnownPresets(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	var targets []string
	transport := &mirrorTransport{
		manager: manager,
		base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			targets = append(targets, request.URL.String())
			if len(targets) == 1 {
				return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("mirror unavailable")), Request: request}, nil
			}
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}),
	}
	request, err := http.NewRequest(http.MethodGet, "https://github.com/lemonade-lab/alx/releases/latest", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}
	want := []string{
		"https://ghfast.top/https://github.com/lemonade-lab/alx/releases/latest",
		"https://gh-proxy.com/https://github.com/lemonade-lab/alx/releases/latest",
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("mirror targets = %#v, want %#v", targets, want)
	}
}

func TestGitHubAPIUsesOfficialFallbackWhenMirrorRejectsIt(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	var targets []string
	transport := &mirrorTransport{
		manager: manager,
		base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			targets = append(targets, request.URL.String())
			if len(targets) == 1 {
				return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
			}
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}),
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/yunzaijs/alemonjs-load-yunzai/releases?per_page=100", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	want := []string{
		"https://ghfast.top/https://api.github.com/repos/yunzaijs/alemonjs-load-yunzai/releases?per_page=100",
		"https://api.github.com/repos/yunzaijs/alemonjs-load-yunzai/releases?per_page=100",
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("API fallback targets = %#v, want %#v", targets, want)
	}
}

func TestCustomMirrorNeverFallsBackToAnotherProvider(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(Settings{Routes: map[Route]RouteSettings{
		RouteGitHub: {Mode: ModeCustomMirror, MirrorURL: "https://custom.example/{url}"},
	}}); err != nil {
		t.Fatal(err)
	}
	candidates := manager.mirrorCandidates(&url.URL{Scheme: "https", Host: "api.github.com", Path: "/"})
	if len(candidates) != 1 || candidates[0] != "https://custom.example/{url}" {
		t.Fatalf("custom mirror candidates = %#v", candidates)
	}
}

func TestRewriteURLUsesTheConfiguredRouteOnly(t *testing.T) {
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(Settings{Routes: map[Route]RouteSettings{
		RouteOfficial: {Mode: ModeCustomMirror, MirrorURL: "https://download-mirror.example/{url}"},
	}}); err != nil {
		t.Fatal(err)
	}
	rewritten, err := manager.RewriteURL("https://download.alemonjs.com/application/alemonapp/app.apk")
	if err != nil || rewritten != "https://download-mirror.example/https://download.alemonjs.com/application/alemonapp/app.apk" {
		t.Fatalf("official rewritten URL = %q, %v", rewritten, err)
	}
	direct, err := manager.RewriteURL("https://example.com/asset")
	if err != nil || direct != "https://example.com/asset" {
		t.Fatalf("unmanaged URL = %q, %v", direct, err)
	}
}

func TestRewriteTemplateUsesTheSameRulesAsTransport(t *testing.T) {
	rewritten, err := RewriteTemplate(defaultGitHubMirror, "https://github.com/lemonade-lab/alx/releases/latest")
	if err != nil || rewritten != "https://ghfast.top/https://github.com/lemonade-lab/alx/releases/latest" {
		t.Fatalf("rewritten guide URL = %q, %v", rewritten, err)
	}
}

func TestConnectionTestUsesActiveClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	previous := testEndpoints[RouteGitHub]
	testEndpoints[RouteGitHub] = server.URL
	t.Cleanup(func() { testEndpoints[RouteGitHub] = previous })
	manager, err := NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	result := manager.Test(context.Background(), RouteGitHub)
	if !result.OK || result.Status != http.StatusNoContent {
		t.Fatalf("connection test = %#v", result)
	}
}
