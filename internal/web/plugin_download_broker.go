package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"alemonx/internal/setupplugin"
	"alemonx/internal/systemnetwork"
)

const (
	pluginDownloadBrokerPath = "/api/v1/system/plugin-download"
	pluginDownloadCacheLimit = int64(1 << 30)
)

type pluginDownloadGrant struct {
	pluginID  string
	remaining int
	expiresAt time.Time
}

type pluginDownloadCacheMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	Disposition  string `json:"disposition,omitempty"`
	Size         int64  `json:"size"`
	LastAccess   string `json:"lastAccess"`
}

// pluginDownloadBroker is a generic HTTP(S) transport for installed system
// plugins. It deliberately knows neither release providers nor plugin asset
// names: a capability token establishes the calling plugin, while the host
// contributes proxy settings, retry, cancellation and a bounded local cache.
type pluginDownloadBroker struct {
	mu       sync.Mutex
	endpoint string
	grants   map[string]pluginDownloadGrant
	network  *systemnetwork.Manager
	plugins  *setupplugin.Registry
	cacheDir string
}

func newPluginDownloadBroker(network *systemnetwork.Manager) *pluginDownloadBroker {
	directory, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(directory) == "" {
		directory = os.TempDir()
	}
	directory = filepath.Join(directory, "alx", "plugin-downloads")
	_ = os.MkdirAll(directory, 0o700)
	return &pluginDownloadBroker{grants: map[string]pluginDownloadGrant{}, network: network, cacheDir: directory}
}

func (b *pluginDownloadBroker) setEndpoint(endpoint string) {
	b.mu.Lock()
	b.endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	b.mu.Unlock()
}

func (b *pluginDownloadBroker) setRegistry(registry *setupplugin.Registry) {
	b.mu.Lock()
	b.plugins = registry
	b.mu.Unlock()
}

func (b *pluginDownloadBroker) environment(plugin setupplugin.Plugin, _ string) []string {
	if b == nil || plugin.Online || !plugin.Enabled {
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
	b.grants[token] = pluginDownloadGrant{pluginID: plugin.ID, remaining: 24, expiresAt: now.Add(90 * time.Minute)}
	environment := []string{"ALX_PLUGIN_DOWNLOAD_BROKER=" + b.endpoint + pluginDownloadBrokerPath, "ALX_PLUGIN_DOWNLOAD_TOKEN=" + token, "ALX_PLUGIN_PROGRESS_MODE=structured"}
	if tag := strings.TrimSpace(plugin.InstalledTag); tag != "" {
		environment = append(environment, "ALX_PLUGIN_INSTALLED_TAG="+tag)
	}
	return environment
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
	return token, grant, ok && grant.remaining > 0
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
	} else {
		b.grants[token] = grant
	}
	return true
}

func (b *pluginDownloadBroker) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "下载代理仅支持 HTTP(S) 读取。")
		return
	}
	token, grant, ok := b.grant(r)
	if !ok || !b.consume(token) {
		writeError(w, http.StatusForbidden, "插件下载授权无效或已过期。")
		return
	}
	target, err := url.Parse(strings.TrimSpace(r.URL.Query().Get("url")))
	if err != nil || target == nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		writeError(w, http.StatusBadRequest, "下载地址必须是有效的 HTTP 或 HTTPS 地址。")
		return
	}
	plugin, err := b.plugin(grant.pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusForbidden, "系统插件未安装或已停用。")
		return
	}
	if b.network == nil {
		writeError(w, http.StatusServiceUnavailable, "主应用网络配置不可用。")
		return
	}
	b.fetch(w, r, target)
}

func (b *pluginDownloadBroker) fetch(w http.ResponseWriter, incoming *http.Request, target *url.URL) {
	cachePath, metaPath := b.cachePaths(target.String())
	meta, cached := readPluginDownloadMeta(metaPath)
	requestHeaders := make(http.Header)
	if cached && incoming.Method == http.MethodGet {
		if meta.ETag != "" {
			requestHeaders.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			requestHeaders.Set("If-Modified-Since", meta.LastModified)
		}
	}
	response, err := b.downloadRequest(incoming.Context(), incoming.Method, target.String(), requestHeaders)
	if err != nil {
		if cached && incoming.Method == http.MethodGet {
			if body, openErr := os.Open(cachePath); openErr == nil {
				defer body.Close()
				meta.LastAccess = time.Now().UTC().Format(time.RFC3339Nano)
				_ = writePluginDownloadMeta(metaPath, meta)
				serveCachedPluginDownload(w, meta, body)
				return
			}
		}
		writeError(w, http.StatusBadGateway, "主应用下载资源失败，请检查网络配置后重试。")
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && cached && incoming.Method == http.MethodGet {
		body, err := os.Open(cachePath)
		if err == nil {
			defer body.Close()
			meta.LastAccess = time.Now().UTC().Format(time.RFC3339Nano)
			_ = writePluginDownloadMeta(metaPath, meta)
			serveCachedPluginDownload(w, meta, body)
			return
		}
	}
	copyPluginDownloadHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	if incoming.Method == http.MethodHead || response.StatusCode < 200 || response.StatusCode >= 300 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		_, _ = io.Copy(w, response.Body)
		return
	}
	temporary, err := os.CreateTemp(filepath.Dir(cachePath), "body-*.tmp")
	if err != nil {
		_, _ = io.Copy(w, response.Body)
		return
	}
	temporaryPath := temporary.Name()
	written, copyErr := io.Copy(io.MultiWriter(w, temporary), response.Body)
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(temporaryPath)
		return
	}
	if err := os.Rename(temporaryPath, cachePath); err != nil {
		_ = os.Remove(temporaryPath)
		return
	}
	meta = pluginDownloadCacheMeta{URL: target.String(), ETag: response.Header.Get("ETag"), LastModified: response.Header.Get("Last-Modified"), ContentType: response.Header.Get("Content-Type"), Disposition: response.Header.Get("Content-Disposition"), Size: written, LastAccess: time.Now().UTC().Format(time.RFC3339Nano)}
	_ = writePluginDownloadMeta(metaPath, meta)
	b.cleanupCache()
}

func (b *pluginDownloadBroker) downloadRequest(ctx context.Context, method, rawURL string, headers http.Header) (*http.Response, error) {
	// Every retry creates a pristine request, so no workbench cookie or
	// Authorization header can ever reach the target.
	for attempt := 0; attempt < 2; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
		if err != nil {
			return nil, err
		}
		request.Header = headers.Clone()
		client := b.network.Client(0)
		client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
			if next.URL.Scheme != "http" && next.URL.Scheme != "https" {
				return errors.New("下载重定向协议无效")
			}
			return nil
		}
		response, err := client.Do(request)
		if err == nil && response.StatusCode < 500 {
			return response, nil
		}
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			err = fmt.Errorf("HTTP %d", response.StatusCode)
		}
		if attempt == 1 {
			return nil, err
		}
	}
	return nil, errors.New("下载失败")
}

func (b *pluginDownloadBroker) cachePaths(rawURL string) (string, string) {
	digest := sha256.Sum256([]byte(rawURL))
	directory := filepath.Join(b.cacheDir, hex.EncodeToString(digest[:]))
	return filepath.Join(directory, "body"), filepath.Join(directory, "meta.json")
}

func readPluginDownloadMeta(path string) (pluginDownloadCacheMeta, bool) {
	var meta pluginDownloadCacheMeta
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &meta) != nil || meta.URL == "" {
		return pluginDownloadCacheMeta{}, false
	}
	return meta, true
}

func writePluginDownloadMeta(path string, meta pluginDownloadCacheMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func copyPluginDownloadHeaders(target, source http.Header) {
	for _, key := range []string{"Content-Length", "Content-Type", "Content-Disposition", "ETag", "Last-Modified"} {
		if value := source.Get(key); value != "" {
			target.Set(key, value)
		}
	}
	target.Set("Cache-Control", "no-store")
}

func serveCachedPluginDownload(w http.ResponseWriter, meta pluginDownloadCacheMeta, body io.Reader) {
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	if meta.Disposition != "" {
		w.Header().Set("Content-Disposition", meta.Disposition)
	}
	if meta.ETag != "" {
		w.Header().Set("ETag", meta.ETag)
	}
	if meta.LastModified != "" {
		w.Header().Set("Last-Modified", meta.LastModified)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

func (b *pluginDownloadBroker) cleanupCache() {
	entries := []struct {
		body   string
		meta   string
		size   int64
		access time.Time
	}{}
	var total int64
	_ = filepath.WalkDir(b.cacheDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "meta.json" {
			return nil
		}
		meta, ok := readPluginDownloadMeta(path)
		if !ok {
			return nil
		}
		body := filepath.Join(filepath.Dir(path), "body")
		info, err := os.Stat(body)
		if err != nil {
			return nil
		}
		access, _ := time.Parse(time.RFC3339Nano, meta.LastAccess)
		entries = append(entries, struct {
			body   string
			meta   string
			size   int64
			access time.Time
		}{body, path, info.Size(), access})
		total += info.Size()
		return nil
	})
	if total <= pluginDownloadCacheLimit {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].access.Before(entries[j].access) })
	for _, entry := range entries {
		if total <= pluginDownloadCacheLimit {
			break
		}
		_ = os.Remove(entry.body)
		_ = os.Remove(entry.meta)
		_ = os.Remove(filepath.Dir(entry.body))
		total -= entry.size
	}
}

func (b *pluginDownloadBroker) plugin(id string) (setupplugin.Plugin, error) {
	if b == nil {
		return setupplugin.Plugin{}, errors.New("系统插件注册表不可用")
	}
	b.mu.Lock()
	registry := b.plugins
	b.mu.Unlock()
	if registry == nil {
		return setupplugin.Plugin{}, errors.New("系统插件注册表不可用")
	}
	return registry.Find(id)
}
