package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alemonx/internal/setupplugin"
	"alemonx/internal/systemnetwork"
)

func TestPluginDownloadBrokerIssuesCredentialFreeLoopbackGrant(t *testing.T) {
	network, err := systemnetwork.NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	broker := newPluginDownloadBroker(network)
	broker.setEndpoint("http://127.0.0.1:17100")
	plugin := downloadFixturePlugin()
	environment := strings.Join(broker.environment(plugin, "install"), "\n")
	if !strings.Contains(environment, "ALX_PLUGIN_DOWNLOAD_BROKER=http://127.0.0.1:17100"+pluginDownloadBrokerPath) || !strings.Contains(environment, "ALX_PLUGIN_DOWNLOAD_TOKEN=") || !strings.Contains(environment, "ALX_PLUGIN_INSTALLED_TAG=v1.2.3") || strings.Contains(environment, "proxy") || strings.Contains(environment, "secret") {
		t.Fatalf("broker environment = %q", environment)
	}
	development := strings.Join(broker.environment(plugin, "install"), "\n")
	if !strings.Contains(development, "ALX_PLUGIN_DOWNLOAD_BROKER=http://127.0.0.1:17100"+pluginDownloadBrokerPath) || !strings.Contains(development, "ALX_PLUGIN_DOWNLOAD_TOKEN=") {
		t.Fatalf("source session did not receive bounded download broker: %q", development)
	}
	localBuild := strings.Join(broker.environment(plugin, "install"), "\n")
	if !strings.Contains(localBuild, "ALX_PLUGIN_DOWNLOAD_BROKER=http://127.0.0.1:17100"+pluginDownloadBrokerPath) || !strings.Contains(localBuild, "ALX_PLUGIN_DOWNLOAD_TOKEN=") {
		t.Fatalf("local system plugin did not receive bounded download broker: %q", localBuild)
	}
	if got := broker.environment(plugin, "status"); len(got) == 0 {
		t.Fatal("any runner action may use the generic download transport")
	}
}

func TestPluginDownloadBrokerRejectsRemoteAndInvalidURLs(t *testing.T) {
	broker := newPluginDownloadBroker(nil)
	broker.setEndpoint("http://127.0.0.1:17100")
	environment := broker.environment(downloadFixturePlugin(), "install")
	var token string
	for _, value := range environment {
		if strings.HasPrefix(value, "ALX_PLUGIN_DOWNLOAD_TOKEN=") {
			token = strings.TrimPrefix(value, "ALX_PLUGIN_DOWNLOAD_TOKEN=")
		}
	}
	if token == "" {
		t.Fatal("missing grant token")
	}
	remote := httptest.NewRequest(http.MethodGet, pluginDownloadBrokerPath+"?url=https://downloads.example.test/releases/latest", nil)
	remote.RemoteAddr = "203.0.113.4:1234"
	remote.Header.Set("Authorization", "Bearer "+token)
	if _, _, ok := broker.grant(remote); ok {
		t.Fatal("remote request must not use a broker token")
	}
	invalid := httptest.NewRequest(http.MethodGet, pluginDownloadBrokerPath+"?url=file:///tmp/archive.zip", nil)
	invalid.RemoteAddr = "127.0.0.1:1234"
	invalid.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	broker.serveHTTP(response, invalid)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL = %d %s", response.Code, response.Body.String())
	}
}

func TestPluginDownloadBrokerGrantHasBoundedUses(t *testing.T) {
	broker := newPluginDownloadBroker(nil)
	broker.mu.Lock()
	broker.grants["token"] = pluginDownloadGrant{pluginID: "fixture", remaining: 1, expiresAt: time.Now().Add(time.Minute)}
	broker.mu.Unlock()
	if !broker.consume("token") {
		t.Fatal("first authorized download must consume the grant")
	}
	if broker.consume("token") {
		t.Fatal("exhausted grant must not authorize another download")
	}
}

func TestPluginDownloadBrokerClearCacheLeavesNoDownloadEntries(t *testing.T) {
	broker := newPluginDownloadBroker(nil)
	broker.cacheDir = t.TempDir()
	directory := filepath.Join(broker.cacheDir, "cached-resource")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "body"), []byte("cached QQ archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePluginDownloadMeta(filepath.Join(directory, "meta.json"), pluginDownloadCacheMeta{URL: "https://example.test/qq.zip", Size: 17, LastAccess: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}
	if summary := broker.cacheSummary(); summary.Entries != 1 || summary.Bytes != 17 {
		t.Fatalf("cache summary = %#v", summary)
	}
	summary, err := broker.clearCache()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Entries != 0 || summary.Bytes != 0 {
		t.Fatalf("cleared cache summary = %#v", summary)
	}
}

func downloadFixturePlugin() setupplugin.Plugin {
	return setupplugin.Plugin{
		ID: "fixture", Enabled: true, InstalledTag: "v1.2.3", InstalledAsset: "fixture.zip", ArchiveSHA256: strings.Repeat("a", 64), Fingerprint: "fingerprint",
	}
}
