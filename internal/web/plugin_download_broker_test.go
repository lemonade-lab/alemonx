package web

import (
	"net/http"
	"net/http/httptest"
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
	environment := strings.Join(broker.environment(setupplugin.Plugin{ID: "alemonx-qq", Enabled: true, InstalledTag: "v1.2.3", InstalledAsset: "alemonx-qq-linux-amd64.zip", ArchiveSHA256: strings.Repeat("a", 64), Fingerprint: "fingerprint"}, "install"), "\n")
	if !strings.Contains(environment, "ALX_PLUGIN_DOWNLOAD_BROKER=http://127.0.0.1:17100"+pluginDownloadBrokerPath) || !strings.Contains(environment, "ALX_PLUGIN_DOWNLOAD_TOKEN=") || strings.Contains(environment, "proxy") || strings.Contains(environment, "secret") {
		t.Fatalf("broker environment = %q", environment)
	}
	if got := broker.environment(setupplugin.Plugin{ID: "alemonx-qq", Enabled: true, DevelopmentSource: true, InstalledTag: "v1.2.3", InstalledAsset: "asset", ArchiveSHA256: strings.Repeat("a", 64), Fingerprint: "fingerprint"}, "install"); len(got) != 0 {
		t.Fatalf("source session received broker variables: %#v", got)
	}
	if got := broker.environment(setupplugin.Plugin{ID: "alemonx-qq", Enabled: true, InstalledTag: "v1.2.3", InstalledAsset: "asset", ArchiveSHA256: strings.Repeat("a", 64), Fingerprint: "fingerprint"}, "napcat-status"); len(got) != 0 {
		t.Fatalf("status action received broker variables: %#v", got)
	}
}

func TestPluginDownloadBrokerRejectsRemoteAndUnapprovedURLs(t *testing.T) {
	broker := newPluginDownloadBroker(nil)
	broker.setEndpoint("http://127.0.0.1:17100")
	environment := broker.environment(setupplugin.Plugin{ID: "alemonx-qq", Enabled: true, InstalledTag: "v1.2.3", InstalledAsset: "asset", ArchiveSHA256: strings.Repeat("a", 64), Fingerprint: "fingerprint"}, "install")
	var token string
	for _, value := range environment {
		if strings.HasPrefix(value, "ALX_PLUGIN_DOWNLOAD_TOKEN=") {
			token = strings.TrimPrefix(value, "ALX_PLUGIN_DOWNLOAD_TOKEN=")
		}
	}
	if token == "" {
		t.Fatal("missing grant token")
	}
	remote := httptest.NewRequest(http.MethodGet, pluginDownloadBrokerPath+"?url=https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest", nil)
	remote.RemoteAddr = "203.0.113.4:1234"
	remote.Header.Set("Authorization", "Bearer "+token)
	if _, _, ok := broker.grant(remote); ok {
		t.Fatal("remote request must not use a broker token")
	}
	unapproved := httptest.NewRequest(http.MethodGet, pluginDownloadBrokerPath+"?url=https://example.com/archive.zip", nil)
	unapproved.RemoteAddr = "127.0.0.1:1234"
	unapproved.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	broker.serveHTTP(response, unapproved)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unapproved URL = %d %s", response.Code, response.Body.String())
	}
}

func TestPluginDownloadBrokerGrantHasBoundedUses(t *testing.T) {
	broker := newPluginDownloadBroker(nil)
	broker.mu.Lock()
	broker.grants["token"] = pluginDownloadGrant{pluginID: "alemonx-qq", action: "install", remaining: 1, expiresAt: time.Now().Add(time.Minute)}
	broker.mu.Unlock()
	if !broker.consume("token") {
		t.Fatal("first authorized download must consume the grant")
	}
	if broker.consume("token") {
		t.Fatal("exhausted grant must not authorize another download")
	}
}

func TestAllowedQQDownloadURLIsBoundToAction(t *testing.T) {
	for _, value := range []string{
		"https://api.github.com/repos/NapNeko/NapCatQQ/releases/latest",
		"https://github.com/NapNeko/NapCatQQ/releases/download/v1/package.zip",
		"https://qqdl.gtimg.cn/qqfile/QQNT/9.9.32/release/file.deb",
	} {
		target, _ := http.NewRequest(http.MethodGet, value, nil)
		if !allowedQQDownloadURL("install", target.URL) {
			t.Fatalf("NapCat official URL %q rejected", value)
		}
	}
	for _, value := range []string{
		"https://api.github.com/repos/NapNeko/NapCat-Mac-Installer/releases/latest",
		"https://github.com/NapNeko/NapCat-Mac-Installer/releases/download/v1.5/NapCatInstaller.zip",
	} {
		target, _ := http.NewRequest(http.MethodGet, value, nil)
		if !allowedQQDownloadURL("napcat-macos-installer-download", target.URL) {
			t.Fatalf("NapCat macOS installer URL %q rejected", value)
		}
	}
	for _, value := range []string{
		"https://api.github.com/repos/LLOneBot/LuckyLilliaBot/releases/latest",
		"https://github.com/LLOneBot/LuckyLilliaBot/releases/download/v1/package.zip",
	} {
		target, _ := http.NewRequest(http.MethodGet, value, nil)
		if !allowedQQDownloadURL("luckylillia-install", target.URL) {
			t.Fatalf("Lucky official URL %q rejected", value)
		}
	}
	for _, value := range []string{
		"https://github.com/other/repository/releases/download/v1/package.zip",
		"https://objects.githubusercontent.com/unrelated",
		"https://qqdl.gtimg.cn/other/file.deb",
	} {
		target, _ := http.NewRequest(http.MethodGet, value, nil)
		if allowedQQDownloadURL("install", target.URL) {
			t.Fatalf("unapproved URL %q allowed", value)
		}
	}
}
