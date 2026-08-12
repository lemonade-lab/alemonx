package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"alemonx/internal/setupplugin"
	"alemonx/internal/systemnetwork"
)

const pluginDownloadBrokerPath = "/api/v1/system/plugin-download"

type pluginDownloadGrant struct {
	pluginID  string
	action    string
	remaining int
	expiresAt time.Time
}

// pluginDownloadBroker keeps credential-bearing networking inside the host.
// A runner gets only a short-lived loopback token and may fetch only the
// official domains declared in this small host policy.
type pluginDownloadBroker struct {
	mu       sync.Mutex
	endpoint string
	grants   map[string]pluginDownloadGrant
	network  *systemnetwork.Manager
}

func newPluginDownloadBroker(network *systemnetwork.Manager) *pluginDownloadBroker {
	return &pluginDownloadBroker{grants: map[string]pluginDownloadGrant{}, network: network}
}

func (b *pluginDownloadBroker) setEndpoint(endpoint string) {
	b.mu.Lock()
	b.endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	b.mu.Unlock()
}

func (b *pluginDownloadBroker) environment(plugin setupplugin.Plugin, action string) []string {
	// This is a host policy, not a manifest capability. The token has no proxy
	// credentials. The gateway is read-only and binds every request to one QQ
	// action plus a fixed official URL allowlist, so a local build can safely use
	// it too. Requiring a Release fingerprint here made local package testing
	// silently bypass the workbench's GitHub mirror/proxy setting and fall back
	// to direct GitHub. This is networking only: it never grants privilege.
	if b == nil || !qqDownloadAction(action) || plugin.ID != "alemonx-qq" || plugin.Online || !plugin.Enabled {
		return nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil
	}
	token := hex.EncodeToString(secret)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.endpoint == "" {
		return nil
	}
	now := time.Now()
	for value, grant := range b.grants {
		if !grant.expiresAt.After(now) {
			delete(b.grants, value)
		}
	}
	// A Linux NapCat install performs one metadata request and two archive
	// downloads. Eight requests leave room for retries without turning a runner
	// token into a long-lived generic download capability.
	b.grants[token] = pluginDownloadGrant{pluginID: plugin.ID, action: action, remaining: 8, expiresAt: now.Add(70 * time.Minute)}
	return []string{"ALX_PLUGIN_DOWNLOAD_BROKER=" + b.endpoint + pluginDownloadBrokerPath, "ALX_PLUGIN_DOWNLOAD_TOKEN=" + token}
}

func (b *pluginDownloadBroker) grant(r *http.Request) (string, pluginDownloadGrant, bool) {
	if b == nil || !requestIsLoopback(r) || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Forwarded-Host") != "" {
		return "", pluginDownloadGrant{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return "", pluginDownloadGrant{}, false
	}
	b.mu.Lock()
	grant, ok := b.grants[token]
	if ok && !grant.expiresAt.After(time.Now()) {
		delete(b.grants, token)
		ok = false
	}
	b.mu.Unlock()
	return token, grant, ok && grant.pluginID == "alemonx-qq" && grant.remaining > 0
}

func (b *pluginDownloadBroker) consume(token string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	grant, ok := b.grants[token]
	if !ok || !grant.expiresAt.After(time.Now()) || grant.remaining <= 0 {
		if ok {
			delete(b.grants, token)
		}
		return false
	}
	grant.remaining--
	if grant.remaining <= 0 {
		delete(b.grants, token)
		return true
	}
	b.grants[token] = grant
	return true
}

func (b *pluginDownloadBroker) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "下载代理仅支持读取官方资源。")
		return
	}
	token, grant, ok := b.grant(r)
	if !ok {
		writeError(w, http.StatusForbidden, "插件下载授权无效或已过期。")
		return
	}
	target, err := url.Parse(strings.TrimSpace(r.URL.Query().Get("url")))
	if err != nil || target == nil || !allowedQQDownloadURL(grant.action, target) {
		writeError(w, http.StatusBadRequest, "仅允许下载 QQ 官方发布资源。")
		return
	}
	if b.network == nil {
		writeError(w, http.StatusServiceUnavailable, "主应用网络配置不可用。")
		return
	}
	if !b.consume(token) {
		writeError(w, http.StatusForbidden, "插件下载授权无效、已过期或请求次数已用完。")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "官方资源地址无效。")
		return
	}
	client := b.network.Client(0)
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		if next.URL.Scheme != "https" || !allowedQQDownloadRedirect(next.URL) {
			return errors.New("官方资源重定向到了未允许的地址")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		// Transport errors can include implementation-specific proxy details.
		// The runner only needs a recovery hint, never the configured endpoint.
		writeError(w, http.StatusBadGateway, "主应用下载官方资源失败，请检查网络配置后重试。")
		return
	}
	defer response.Body.Close()
	for _, key := range []string{"Content-Length", "Content-Type", "Content-Disposition", "ETag", "Last-Modified"} {
		if value := response.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func qqDownloadAction(action string) bool {
	switch action {
	case "install", "update", "update-check", "napcat-macos-installer-download", "napcat-windows-installer-download", "luckylillia-install", "luckylillia-reinstall", "luckylillia-update", "luckylillia-update-check":
		return true
	default:
		return false
	}
}

func allowedQQDownloadURL(action string, target *url.URL) bool {
	if target == nil || target.Scheme != "https" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	path := target.EscapedPath()
	switch action {
	case "install", "update", "update-check":
		if host == "api.github.com" && path == "/repos/NapNeko/NapCatQQ/releases/latest" {
			return true
		}
		if host == "github.com" && strings.HasPrefix(path, "/NapNeko/NapCatQQ/releases/download/") {
			return true
		}
		return host == "qqdl.gtimg.cn" && strings.HasPrefix(path, "/qqfile/QQNT/")
	case "napcat-macos-installer-download":
		if host == "api.github.com" && path == "/repos/NapNeko/NapCat-Mac-Installer/releases/latest" {
			return true
		}
		return host == "github.com" && strings.HasPrefix(path, "/NapNeko/NapCat-Mac-Installer/releases/download/")
	case "napcat-windows-installer-download":
		if host == "api.github.com" && path == "/repos/NapNeko/NapCatQQ/releases/latest" {
			return true
		}
		return host == "github.com" && strings.HasPrefix(path, "/NapNeko/NapCatQQ/releases/download/")
	case "luckylillia-install", "luckylillia-reinstall", "luckylillia-update", "luckylillia-update-check":
		if host == "api.github.com" && path == "/repos/LLOneBot/LuckyLilliaBot/releases/latest" {
			return true
		}
		return host == "github.com" && strings.HasPrefix(path, "/LLOneBot/LuckyLilliaBot/releases/download/")
	default:
		return false
	}
}

func allowedQQDownloadRedirect(target *url.URL) bool {
	if target == nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(target.Hostname())), ".")
	return host == "githubusercontent.com" || strings.HasSuffix(host, ".githubusercontent.com") || host == "qqdl.gtimg.cn"
}
