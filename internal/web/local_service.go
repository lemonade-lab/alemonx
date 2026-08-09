package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"alemonx/internal/setupplugin"
)

const localServicePrefix = "/api/v1/services/"

type localServiceView struct {
	PluginID  string `json:"pluginId"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	ProxyURL  string `json:"proxyUrl"`
	Embed     bool   `json:"embed"`
}

func (s *server) localServicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	wanted := r.URL.Query().Get("plugin")
	items := make([]localServiceView, 0)
	for _, plugin := range s.plugins.List() {
		if wanted != "" && wanted != plugin.ID || plugin.Online {
			continue
		}
		for _, service := range plugin.Services {
			items = append(items, localServiceView{PluginID: plugin.ID, ID: service.ID, Name: service.Name, Reachable: localServiceReachable(r.Context(), service), ProxyURL: localServiceURL(plugin.ID, service.ID), Embed: service.Embed})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) localServiceProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "本地服务不支持该请求方式。")
		return
	}
	if isUpgradeRequest(r) {
		writeError(w, http.StatusNotImplemented, "本地服务网关暂不支持 WebSocket。")
		return
	}
	if rest := strings.TrimPrefix(r.URL.Path, localServicePrefix); strings.Count(rest, "/") == 1 && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
		return
	}
	plugin, service, requestPath, err := s.resolveLocalService(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if !localServiceReachable(r.Context(), service) {
		writeError(w, http.StatusBadGateway, "本地服务尚未启动或无法连接。")
		return
	}
	target, err := localServiceTarget(service, requestPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mount := localServiceURL(plugin.ID, service.ID)
	if !strings.HasSuffix(mount, "/") {
		mount += "/"
	}
	cookiePrefix := localServiceCookiePrefix(plugin.ID, service.ID)
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme, request.URL.Host, request.URL.Path, request.URL.RawPath = target.Scheme, target.Host, target.Path, ""
			request.URL.RawQuery = r.URL.RawQuery
			request.Host = target.Host
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			for _, cookie := range r.Cookies() {
				if strings.HasPrefix(cookie.Name, cookiePrefix) {
					request.AddCookie(&http.Cookie{Name: strings.TrimPrefix(cookie.Name, cookiePrefix), Value: cookie.Value})
				}
			}
		},
		Transport:     localServiceTransport(service.SSE),
		FlushInterval: 100 * time.Millisecond,
		ModifyResponse: func(response *http.Response) error {
			if location := response.Header.Get("Location"); location != "" && !localServiceLocationAllowed(location, target) {
				return errors.New("本地服务重定向到了不受信任的地址")
			}
			if service.RewriteHTML {
				modifyRobotAppResponse(response, target, mount, r.URL.Path, r)
			}
			isolateLocalServiceCookies(response, cookiePrefix, mount)
			if service.Embed {
				response.Header.Del("X-Frame-Options")
				response.Header.Set("Content-Security-Policy", localServiceFramePolicy(response.Header.Get("Content-Security-Policy")))
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "本地服务代理失败："+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}

func (s *server) resolveLocalService(requestURL string) (setupplugin.Plugin, setupplugin.ServiceSpec, string, error) {
	rest := strings.TrimPrefix(requestURL, localServicePrefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return setupplugin.Plugin{}, setupplugin.ServiceSpec{}, "", errors.New("缺少本地服务标识。")
	}
	plugin, err := s.plugins.Find(parts[0])
	if err != nil || plugin.Online || !plugin.Enabled {
		return setupplugin.Plugin{}, setupplugin.ServiceSpec{}, "", errors.New("未找到已启用的本地服务。")
	}
	for _, service := range plugin.Services {
		if service.ID == parts[1] {
			requestPath := "/"
			if len(parts) == 3 {
				requestPath += parts[2]
			}
			if strings.Contains(requestPath, "\\") || strings.Contains(requestPath, "..") {
				return setupplugin.Plugin{}, setupplugin.ServiceSpec{}, "", errors.New("本地服务路径无效。")
			}
			return plugin, service, requestPath, nil
		}
	}
	return setupplugin.Plugin{}, setupplugin.ServiceSpec{}, "", errors.New("未声明该本地服务。")
}

func localServiceURL(pluginID, serviceID string) string {
	return localServicePrefix + pluginID + "/" + serviceID + "/"
}

func localServiceTarget(service setupplugin.ServiceSpec, requestPath string) (*url.URL, error) {
	clean := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	if clean == "/.." || strings.HasPrefix(clean, "/../") {
		return nil, errors.New("本地服务路径无效。")
	}
	base := strings.TrimSuffix(service.BasePath, "/")
	if base == "" {
		base = "/"
	}
	targetPath := path.Join(base, clean)
	if strings.HasSuffix(requestPath, "/") && !strings.HasSuffix(targetPath, "/") {
		targetPath += "/"
	}
	return &url.URL{Scheme: "http", Host: net.JoinHostPort(service.Host, strconv.Itoa(service.Port)), Path: targetPath}, nil
}

func localServiceReachable(parent context.Context, service setupplugin.ServiceSpec) bool {
	target, err := localServiceTarget(service, service.HealthPath)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false
	}
	response, err := (&http.Client{Transport: localServiceTransport(false), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(request)
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return true
}

func localServiceTransport(sse bool) *http.Transport {
	headTimeout := 15 * time.Second
	if sse {
		headTimeout = 30 * time.Second
	}
	return &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: false, MaxIdleConns: 16, IdleConnTimeout: 45 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: headTimeout, ExpectContinueTimeout: time.Second}
}

func isUpgradeRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func localServiceLocationAllowed(location string, target *url.URL) bool {
	parsed, err := url.Parse(location)
	if err != nil {
		return false
	}
	return !parsed.IsAbs() || parsed.Host == target.Host
}

func localServiceCookiePrefix(pluginID, serviceID string) string {
	return "alxsvc_" + base64.RawURLEncoding.EncodeToString([]byte(pluginID+":"+serviceID)) + "_"
}

func isolateLocalServiceCookies(response *http.Response, prefix, mount string) {
	cookies := response.Cookies()
	response.Header.Del("Set-Cookie")
	for _, cookie := range cookies {
		cookie.Name = prefix + cookie.Name
		cookie.Path = mount
		response.Header.Add("Set-Cookie", cookie.String())
	}
}

var frameAncestorsPattern = regexp.MustCompile(`(?i)(^|;)\s*frame-ancestors\s+[^;]*`)

func localServiceFramePolicy(policy string) string {
	if frameAncestorsPattern.MatchString(policy) {
		return frameAncestorsPattern.ReplaceAllString(policy, "$1 frame-ancestors 'self'")
	}
	if strings.TrimSpace(policy) == "" {
		return "frame-ancestors 'self'"
	}
	return policy + "; frame-ancestors 'self'"
}
