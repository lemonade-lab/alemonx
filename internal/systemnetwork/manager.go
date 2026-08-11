// Package systemnetwork owns AlemonX's application-level outbound network
// configuration. It is intentionally separate from project commands (git,
// npm, robot runtimes and plugin webviews), so a workbench setting can never
// silently rewrite a user's project networking.
package systemnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Mode string

const (
	// ModeSystem follows the proxy environment inherited by AlemonX.
	ModeSystem       Mode = "system"
	ModeMirror       Mode = "mirror"
	ModeCustomMirror Mode = "custom-mirror"
	// ModeManual is retained only for configurations saved by the initial HTTP
	// proxy implementation. New UI never creates it.
	ModeManual Mode = "manual"
	ModeDirect Mode = "direct"
)

// Route is a bounded group of AlemonX-owned content hosts. Project Git/NPM,
// robot traffic and third-party plugin pages deliberately do not belong here.
type Route string

const (
	RouteGitHub   Route = "github"
	RouteGitee    Route = "gitee"
	RouteNPM      Route = "npm"
	RouteNode     Route = "node"
	RouteCDN      Route = "cdn"
	RouteOfficial Route = "official"
)

var allRoutes = []Route{RouteGitHub, RouteGitee, RouteNPM, RouteNode, RouteCDN, RouteOfficial}

const (
	defaultGitHubMirror = "https://ghfast.top/{url}"
	defaultNPMMirror    = "https://registry.npmmirror.com{path}"
	defaultNodeMirror   = "https://npmmirror.com/mirrors/node{nodepath}"
)

// MirrorPreset is a host-maintained template available for a system route.
// The list is returned to the settings UI, used by the safe fallback chain,
// and reused by the download guide so those surfaces cannot drift apart.
type MirrorPreset struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// mirrorPresets are deliberately small, known URL templates. They are used
// for the recommended default and a best-effort fallback for safe, read-only
// system-content requests. Custom mirrors are always used alone: AlemonX must
// not silently send a user's custom route to an unrelated service.
var mirrorPresets = map[Route][]MirrorPreset{
	RouteGitHub: {
		{Label: "GHFast（推荐）", Value: defaultGitHubMirror},
		{Label: "gh-proxy", Value: "https://gh-proxy.com/{url}"},
		{Label: "v6 gh-proxy", Value: "https://v6.gh-proxy.org/{url}"},
		{Label: "ghproxy.net", Value: "https://ghproxy.net/{url}"},
		{Label: "ui.ghproxy.cc", Value: "https://ui.ghproxy.cc/{url}"},
		{Label: "github.akams.cn", Value: "https://github.akams.cn/{url}"},
		{Label: "gh.jasonzeng.dev", Value: "https://gh.jasonzeng.dev/{url}"},
	},
	RouteNPM:  {{Label: "npmmirror", Value: defaultNPMMirror}},
	RouteNode: {{Label: "npmmirror Node.js", Value: defaultNodeMirror}},
}

// RouteSettings is a self-contained answer for one resource category. A
// GitHub mirror never accidentally becomes the source for Gitee or NPM.
type RouteSettings struct {
	Mode           Mode   `json:"mode"`
	MirrorURL      string `json:"mirrorUrl,omitempty"`
	ProxyURL       string `json:"proxyUrl,omitempty"`
	HasCredentials bool   `json:"hasCredentials,omitempty"`
}

type Settings struct {
	Routes        map[Route]RouteSettings  `json:"routes"`
	MirrorPresets map[Route][]MirrorPreset `json:"mirrorPresets,omitempty"`
}

type storedRouteSettings struct {
	Mode      Mode   `json:"mode"`
	MirrorURL string `json:"mirrorUrl,omitempty"`
	ProxyURL  string `json:"proxyUrl,omitempty"`
}

type storedSettings struct {
	// Mode and ProxyURL migrate the original all-or-nothing setting. New files
	// only store per-route objects under Routes.
	Mode     Mode                          `json:"mode,omitempty"`
	ProxyURL string                        `json:"proxyUrl,omitempty"`
	Routes   map[Route]storedRouteSettings `json:"routes,omitempty"`
}

// UnmarshalJSON accepts both the current object shape and the short-lived
// first route shape (`{"github":"manual"}`) released before per-route
// addresses existed.
func (s *storedSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Mode     Mode                      `json:"mode"`
		ProxyURL string                    `json:"proxyUrl"`
		Routes   map[Route]json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Mode, s.ProxyURL = raw.Mode, raw.ProxyURL
	s.Routes = make(map[Route]storedRouteSettings, len(raw.Routes))
	for route, value := range raw.Routes {
		var item storedRouteSettings
		if err := json.Unmarshal(value, &item); err == nil && item.Mode != "" {
			s.Routes[route] = item
			continue
		}
		var mode Mode
		if err := json.Unmarshal(value, &mode); err != nil {
			return fmt.Errorf("路由 %s 配置无效", route)
		}
		s.Routes[route] = storedRouteSettings{Mode: mode, ProxyURL: raw.ProxyURL}
	}
	return nil
}

type CheckResult struct {
	OK        bool   `json:"ok"`
	Target    string `json:"target"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latencyMs,omitempty"`
	Message   string `json:"message"`
}

// Manager persists a user-level setting and manufactures isolated HTTP
// clients for AlemonX-managed content. It does not alter http.DefaultClient
// or process environment variables.
type Manager struct {
	mu       sync.RWMutex
	path     string
	settings storedSettings
}

var (
	defaultMu      sync.RWMutex
	defaultManager = &Manager{settings: storedSettings{Routes: defaultRoutes()}}
	testEndpoints  = map[Route]string{
		RouteGitHub:   "https://api.github.com/",
		RouteGitee:    "https://gitee.com/api/v5/version",
		RouteNPM:      "https://registry.npmjs.org/",
		RouteNode:     "https://nodejs.org/dist/index.json",
		RouteCDN:      "https://cdn.jsdelivr.net/",
		RouteOfficial: "https://download.alemonjs.com/",
	}
)

func New() (*Manager, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return NewAt(filepath.Join(directory, "alemonx", "network.json"))
}

func NewAt(path string) (*Manager, error) {
	manager := &Manager{path: path, settings: storedSettings{Routes: defaultRoutes()}}
	if strings.TrimSpace(path) == "" {
		return manager, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manager, nil
	}
	if err != nil {
		return nil, err
	}
	var saved storedSettings
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, fmt.Errorf("系统联网配置无效：%w", err)
	}
	saved.Routes = normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)
	if err := validate(saved); err != nil {
		return nil, fmt.Errorf("系统联网配置无效：%w", err)
	}
	manager.settings = saved
	return manager, nil
}

// SetDefault makes subsequently-created system-content clients follow this
// manager. Existing clients remain safe: their Proxy callback reads the
// manager's current value for every request.
func SetDefault(manager *Manager) {
	if manager == nil {
		return
	}
	defaultMu.Lock()
	defaultManager = manager
	defaultMu.Unlock()
}

func DefaultClient(timeout time.Duration) *http.Client {
	defaultMu.RLock()
	manager := defaultManager
	defaultMu.RUnlock()
	return manager.Client(timeout)
}

func (m *Manager) Settings() Settings {
	m.mu.RLock()
	saved := m.settings
	m.mu.RUnlock()
	return publicSettings(saved)
}

// MirrorPresets returns a copy, keeping the host-maintained catalog immutable
// for callers. Only routes with verified presets are included.
func MirrorPresets(route Route) []MirrorPreset {
	presets := mirrorPresets[route]
	return append([]MirrorPreset(nil), presets...)
}

func publicSettings(saved storedSettings) Settings {
	routes := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)
	result := Settings{Routes: make(map[Route]RouteSettings, len(routes)), MirrorPresets: make(map[Route][]MirrorPreset, len(mirrorPresets))}
	for route, item := range routes {
		result.Routes[route] = publicRouteSettings(item)
	}
	for route := range mirrorPresets {
		result.MirrorPresets[route] = MirrorPresets(route)
	}
	return result
}

func publicRouteSettings(saved storedRouteSettings) RouteSettings {
	result := RouteSettings{Mode: saved.Mode, MirrorURL: saved.MirrorURL}
	parsed, err := url.Parse(saved.ProxyURL)
	if err != nil || parsed == nil {
		return result
	}
	result.HasCredentials = parsed.User != nil
	parsed.User = nil
	result.ProxyURL = parsed.String()
	return result
}

func (m *Manager) Save(next Settings) (Settings, error) {
	saved := storedSettings{Routes: make(map[Route]storedRouteSettings, len(next.Routes))}
	for route, item := range next.Routes {
		saved.Routes[route] = storedRouteSettings{Mode: item.Mode, MirrorURL: strings.TrimSpace(item.MirrorURL), ProxyURL: strings.TrimSpace(item.ProxyURL)}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	saved.Routes = normalizedRoutes(saved.Routes, "", "")
	current := normalizedRoutes(m.settings.Routes, m.settings.Mode, m.settings.ProxyURL)
	for _, route := range allRoutes {
		candidate := saved.Routes[route]
		previous := current[route]
		if candidate.Mode == ModeManual && publicRouteSettings(previous).ProxyURL == candidate.ProxyURL {
			old, oldErr := url.Parse(previous.ProxyURL)
			input, inputErr := url.Parse(candidate.ProxyURL)
			if oldErr == nil && inputErr == nil && old.User != nil && input.User == nil {
				candidate.ProxyURL = previous.ProxyURL
				saved.Routes[route] = candidate
			}
		}
	}
	if err := validate(saved); err != nil {
		return Settings{}, err
	}
	if m.path != "" {
		raw, err := json.MarshalIndent(saved, "", "  ")
		if err != nil {
			return Settings{}, err
		}
		if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
			return Settings{}, err
		}
		temporary := m.path + ".tmp"
		if err := os.WriteFile(temporary, append(raw, '\n'), 0o600); err != nil {
			return Settings{}, err
		}
		if err := os.Rename(temporary, m.path); err != nil {
			_ = os.Remove(temporary)
			return Settings{}, err
		}
	}
	m.settings = saved
	return publicSettings(saved), nil
}

func validate(saved storedSettings) error {
	routes := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)
	for _, route := range allRoutes {
		item := routes[route]
		switch item.Mode {
		case ModeSystem, ModeDirect:
			continue
		case ModeMirror, ModeCustomMirror:
			if err := validateMirrorURL(item.MirrorURL); err != nil {
				return fmt.Errorf("%s 镜像地址无效：%w", route, err)
			}
		case ModeManual:
			parsed, err := url.Parse(item.ProxyURL)
			if err != nil || parsed == nil || parsed.Host == "" {
				return fmt.Errorf("请为 %s 填写有效代理地址", route)
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return fmt.Errorf("%s 代理地址仅支持 HTTP 或 HTTPS", route)
			}
			if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
				return fmt.Errorf("%s 代理地址不能包含路径、参数或片段", route)
			}
		default:
			return errors.New("存在未知的系统联网路由模式")
		}
	}
	return nil
}

func (m *Manager) Client(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = m.proxyFor
	return &http.Client{Timeout: timeout, Transport: &mirrorTransport{base: transport, manager: m}}
}

type mirrorTransport struct {
	base    http.RoundTripper
	manager *Manager
}

func (t *mirrorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || t.manager == nil {
		return t.base.RoundTrip(request)
	}
	mirrors := t.manager.mirrorCandidates(request.URL)
	if len(mirrors) == 0 {
		return t.base.RoundTrip(request)
	}
	for index, mirror := range mirrors {
		target, err := rewriteMirrorURL(mirror, request.URL)
		if err != nil {
			return nil, err
		}
		copy := request.Clone(request.Context())
		copy.URL = target
		copy.Host = ""
		response, requestErr := t.base.RoundTrip(copy)
		if shouldFallbackToOfficialAPI(request, response, requestErr) {
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			return t.base.RoundTrip(request)
		}
		if index == len(mirrors)-1 || !retryMirrorRequest(request, response, requestErr) {
			return response, requestErr
		}
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
	}
	return nil, errors.New("镜像请求没有可用的访问入口")
}

func (m *Manager) proxyFor(request *http.Request) (*url.URL, error) {
	if request == nil || bypassProxy(request.URL) {
		return nil, nil
	}
	route, known := routeForURL(request.URL)
	if !known {
		return nil, nil
	}
	m.mu.RLock()
	saved := m.settings
	m.mu.RUnlock()
	item := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)[route]
	switch item.Mode {
	case ModeDirect, ModeMirror, ModeCustomMirror:
		return nil, nil
	case ModeManual:
		return url.Parse(item.ProxyURL)
	default:
		return http.ProxyFromEnvironment(request)
	}
}

func (m *Manager) mirrorFor(target *url.URL) (string, bool) {
	if target == nil {
		return "", false
	}
	route, known := routeForURL(target)
	if !known {
		return "", false
	}
	m.mu.RLock()
	saved := m.settings
	m.mu.RUnlock()
	item := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)[route]
	return item.MirrorURL, (item.Mode == ModeMirror || item.Mode == ModeCustomMirror) && item.MirrorURL != ""
}

// RewriteURL resolves the URL that an AlemonX-owned browser hand-off should
// open. It keeps browser downloads consistent with the same official-content
// setting used by backend requests, while returning unrelated URLs untouched.
func (m *Manager) RewriteURL(raw string) (string, error) {
	target, err := url.Parse(raw)
	if err != nil || target == nil || target.Host == "" {
		return raw, fmt.Errorf("资源地址无效")
	}
	mirror, ok := m.mirrorFor(target)
	if !ok {
		return raw, nil
	}
	rewritten, err := rewriteMirrorURL(mirror, target)
	if err != nil {
		return raw, err
	}
	return rewritten.String(), nil
}

func (m *Manager) mirrorCandidates(target *url.URL) []string {
	mirror, ok := m.mirrorFor(target)
	if !ok {
		return nil
	}
	route, known := routeForURL(target)
	if !known {
		return nil
	}
	m.mu.RLock()
	saved := m.settings
	m.mu.RUnlock()
	item := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)[route]
	if item.Mode != ModeMirror {
		return []string{mirror}
	}
	presets := MirrorPresets(route)
	if len(presets) < 2 || !containsMirror(presets, mirror) {
		return []string{mirror}
	}
	result := make([]string, 0, len(presets))
	result = append(result, mirror)
	for _, candidate := range presets {
		if candidate.Value != mirror {
			result = append(result, candidate.Value)
		}
	}
	return result
}

func containsMirror(candidates []MirrorPreset, value string) bool {
	for _, candidate := range candidates {
		if candidate.Value == value {
			return true
		}
	}
	return false
}

func retryMirrorRequest(request *http.Request, response *http.Response, requestErr error) bool {
	if request == nil || (request.Method != http.MethodGet && request.Method != http.MethodHead) || request.Context().Err() != nil {
		return false
	}
	if requestErr != nil {
		return true
	}
	return response != nil && response.StatusCode >= http.StatusInternalServerError
}

// shouldFallbackToOfficialAPI covers a practical limitation of URL-prefix
// mirrors: many deliberately reject GitHub's REST API even though they work
// for release assets. A public metadata request is safe to retry directly,
// and doing it immediately avoids walking every mirror after a predictable
// 403/429. Repository downloads and custom mirrors keep their normal policy.
func shouldFallbackToOfficialAPI(request *http.Request, response *http.Response, requestErr error) bool {
	if request == nil || (request.Method != http.MethodGet && request.Method != http.MethodHead) || request.Context().Err() != nil || !strings.EqualFold(request.URL.Hostname(), "api.github.com") {
		return false
	}
	if requestErr != nil {
		return true
	}
	return response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError)
}

func validateMirrorURL(value string) error {
	urlCount, pathCount, nodePathCount := strings.Count(value, "{url}"), strings.Count(value, "{path}"), strings.Count(value, "{nodepath}")
	if (urlCount == 1 && pathCount == 0 && nodePathCount == 0) || (urlCount == 0 && pathCount == 1 && nodePathCount == 0) || (urlCount == 0 && pathCount == 0 && nodePathCount == 1) {
		// A mirror can either proxy the complete URL (GitHub accelerators) or
		// replace only the host while preserving a path (NPM registries).
	} else {
		return errors.New("请使用包含 {url}、{path} 或 {nodepath} 的完整镜像模板")
	}
	marker := "{url}"
	if pathCount == 1 {
		marker = "{path}"
	} else if nodePathCount == 1 {
		marker = "{nodepath}"
	}
	prefix := strings.Split(value, marker)[0]
	parsed, err := url.Parse(prefix)
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("镜像模板必须以 HTTP 或 HTTPS 地址开头")
	}
	return nil
}

func rewriteMirrorURL(template string, source *url.URL) (*url.URL, error) {
	if err := validateMirrorURL(template); err != nil {
		return nil, err
	}
	if strings.Contains(template, "{url}") {
		return url.Parse(strings.Replace(template, "{url}", source.String(), 1))
	}
	path := source.EscapedPath()
	if strings.Contains(template, "{nodepath}") {
		path = strings.TrimPrefix(path, "/dist")
		template = strings.Replace(template, "{nodepath}", path, 1)
	} else {
		template = strings.Replace(template, "{path}", path, 1)
	}
	target, err := url.Parse(template)
	if err != nil {
		return nil, err
	}
	if source.RawQuery != "" {
		target.RawQuery = source.RawQuery
	}
	return target, nil
}

// RewriteTemplate is the safe public form used by AlemonX-owned hand-off
// links. It applies exactly the same validation and placeholder semantics as
// the outbound transport.
func RewriteTemplate(template, raw string) (string, error) {
	source, err := url.Parse(raw)
	if err != nil || source == nil || source.Host == "" {
		return "", errors.New("资源地址无效")
	}
	target, err := rewriteMirrorURL(template, source)
	if err != nil {
		return "", err
	}
	return target.String(), nil
}

func defaultRoutes() map[Route]storedRouteSettings {
	return map[Route]storedRouteSettings{
		RouteGitHub:   {Mode: ModeMirror, MirrorURL: defaultGitHubMirror},
		RouteGitee:    {Mode: ModeDirect},
		RouteNPM:      {Mode: ModeMirror, MirrorURL: defaultNPMMirror},
		RouteNode:     {Mode: ModeMirror, MirrorURL: defaultNodeMirror},
		RouteCDN:      {Mode: ModeDirect},
		RouteOfficial: {Mode: ModeDirect},
	}
}

func normalizedRoutes(input map[Route]storedRouteSettings, legacyMode Mode, legacyProxyURL string) map[Route]storedRouteSettings {
	routes := defaultRoutes()
	if legacyMode == ModeSystem || legacyMode == ModeMirror || legacyMode == ModeCustomMirror || legacyMode == ModeManual || legacyMode == ModeDirect {
		for _, route := range allRoutes {
			routes[route] = storedRouteSettings{Mode: legacyMode, ProxyURL: legacyProxyURL}
		}
	}
	for route, item := range input {
		for _, allowed := range allRoutes {
			if route == allowed {
				if item.Mode == "" {
					item.Mode = routes[route].Mode
				}
				if item.ProxyURL == "" && item.Mode == ModeManual {
					item.ProxyURL = legacyProxyURL
				}
				if route == RouteNode && item.MirrorURL == "https://npmmirror.com/mirrors/node{path}" {
					item.MirrorURL = defaultNodeMirror
				}
				routes[route] = item
				break
			}
		}
	}
	return routes
}

func routeForURL(target *url.URL) (Route, bool) {
	if target == nil {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com") || host == "githubusercontent.com" || strings.HasSuffix(host, ".githubusercontent.com"):
		return RouteGitHub, true
	case host == "gitee.com" || strings.HasSuffix(host, ".gitee.com"):
		return RouteGitee, true
	case host == "registry.npmjs.org" || host == "registry.npmmirror.com":
		return RouteNPM, true
	case host == "nodejs.org" || host == "npmmirror.com":
		return RouteNode, true
	case host == "cdn.jsdelivr.net":
		return RouteCDN, true
	case host == "download.alemonjs.com":
		return RouteOfficial, true
	default:
		return "", false
	}
}

func bypassProxy(target *url.URL) bool {
	if target == nil {
		return true
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// Test makes a bounded, credential-free request through one active route.
func (m *Manager) Test(ctx context.Context, route Route) CheckResult {
	target, ok := testEndpoints[route]
	if !ok {
		return CheckResult{Message: "不支持测试该系统联网资源"}
	}
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return CheckResult{Target: target, Message: "无法创建测试请求"}
	}
	response, err := m.Client(8 * time.Second).Do(request)
	if err != nil {
		return CheckResult{Target: target, LatencyMS: time.Since(started).Milliseconds(), Message: "无法连接该资源，请检查对应代理地址"}
	}
	defer response.Body.Close()
	result := CheckResult{Target: target, Status: response.StatusCode, LatencyMS: time.Since(started).Milliseconds()}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result.OK, result.Message = true, "连接正常"
	} else {
		result.Message = "资源返回了异常状态，请检查对应网络策略"
	}
	return result
}
