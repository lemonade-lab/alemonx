package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
	"golang.org/x/net/websocket"
)

const localServicePrefix = "/api/v1/services/"

// dynamicServicePrefix mounts a same-origin proxy for a loopback port chosen
// by an installed plugin at runtime (Docker-published container ports cannot
// be declared statically in alx.json). The target is always forced to
// 127.0.0.1 and the route is validated the same way as manifest services.
const dynamicServicePrefix = localServicePrefix + "dynamic/"

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

type localServiceView struct {
	PluginID  string `json:"pluginId"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	ProxyURL  string `json:"proxyUrl"`
	Embed     bool   `json:"embed"`
	WebSocket bool   `json:"websocket"`
	Error     string `json:"error,omitempty"`
}

// localWebSocketFrame keeps the original WebSocket frame type. The standard
// websocket.Message codec only preserves the type chosen by the caller; using
// string here would silently turn binary WebUI frames into text frames.
type localWebSocketFrame struct {
	data        []byte
	payloadType byte
}

var localWebSocketFrameCodec = websocket.Codec{
	Marshal: func(value interface{}) ([]byte, byte, error) {
		frame, ok := value.(localWebSocketFrame)
		if !ok {
			return nil, websocket.UnknownFrame, errors.New("invalid local WebSocket frame")
		}
		return frame.data, frame.payloadType, nil
	},
	Unmarshal: func(data []byte, payloadType byte, value interface{}) error {
		frame, ok := value.(*localWebSocketFrame)
		if !ok {
			return errors.New("invalid local WebSocket frame destination")
		}
		frame.data = append(frame.data[:0], data...)
		frame.payloadType = payloadType
		return nil
	},
}

var (
	htmlHeadPattern = regexp.MustCompile(`(?i)<head[^>]*>`)
	scriptSrcPolicy = regexp.MustCompile(`(?i)(script-src[^;]*)`)
)

// embeddedAPICompatScript rebases root-relative API and WebSocket requests
// inside an embedded service page to the ALX service mount. LLBot reports its
// WebUI port to its frontend; that port is rewritten to the workbench port, so
// its WebSocket must also be mapped back under the service mount rather than
// attempting an upgrade at the workbench root.
const embeddedAPICompatScript = `<script>(function(){var m=%q,h=%q;if(!m||window.__alxApiCompat)return;window.__alxApiCompat=true;var nf=window.fetch?window.fetch.bind(window):null,z=new URLSearchParams(window.location.hash.slice(1)).get("alx-napcat-token"),cred="",boot=Promise.resolve("");if(z&&nf&&window.crypto&&crypto.subtle){boot=crypto.subtle.digest("SHA-256",new TextEncoder().encode(z+".napcat")).then(function(b){return Array.from(new Uint8Array(b)).map(function(v){return v.toString(16).padStart(2,"0")}).join("")}).then(function(hash){return nf(m+"api/auth/login",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({hash:hash})})}).then(function(r){return r.json()}).then(function(v){cred=v&&v.data&&v.data.Credential;if(!cred)throw new Error("NapCat WebUI login failed");localStorage.setItem("token",JSON.stringify(cred));history.replaceState(null,"",window.location.pathname+window.location.search);return cred}).catch(function(e){console.warn("NapCat embedded WebUI login failed",e);return ""})}function p(u){if(typeof u==="string"&&u.charAt(0)==="/"&&u.charAt(1)!=="/")return m+u.slice(1);return u}function w(u){try{var x=new URL(String(u),window.location.href),same=x.host===window.location.host||x.host===h||x.hostname==="::",root=x.pathname.charAt(0)==="/"?x.pathname.slice(1):x.pathname;if((same||typeof u==="string"&&u.charAt(0)==="/")&&(x.protocol==="ws:"||x.protocol==="wss:"||x.protocol==="http:"||x.protocol==="https:")){return (window.location.protocol==="https:"?"wss://":"ws://")+window.location.host+m+root+x.search+x.hash}}catch(e){}return u}var f=nf;if(f){window.fetch=function(i,n){var u=null,x=i;if(typeof i==="string"){u=i;x=p(i)}else if(i&&typeof i.url==="string"){u=i.url;x=new Request(p(i.url),i)}var q=f(x,n);if(!u||u.indexOf("/api/login-info")===-1)return q;return q.then(function(r){return r.clone().text().then(function(t){try{var d=JSON.parse(t),port=Number(window.location.port)||(window.location.protocol==="https:"?443:80);if(d&&d.data&&d.data.webui&&d.data.webui.port){d.data.webui.port=port;var h=new Headers(r.headers);h.set("Content-Length",String(JSON.stringify(d).length));return new Response(JSON.stringify(d),{status:r.status,statusText:r.statusText,headers:h})}}catch(e){}return r})})}}var o=XMLHttpRequest.prototype.open,oh=XMLHttpRequest.prototype.setRequestHeader,os=XMLHttpRequest.prototype.send;XMLHttpRequest.prototype.open=function(a,u){this.__alxURL=String(u);arguments[1]=p(u);return o.apply(this,arguments)};XMLHttpRequest.prototype.setRequestHeader=function(k,v){if(String(k).toLowerCase()==="authorization"&&(!v||String(v).trim()==="Bearer")){this.__alxAuthPending=true;return}return oh.call(this,k,v)};XMLHttpRequest.prototype.send=function(b){var q=this;if(q.__alxAuthPending&&z){boot.then(function(c){if(c)oh.call(q,"Authorization","Bearer "+c);os.call(q,b)});return}return os.call(q,b)};var E=window.EventSource;if(E){function P(u,c){return new E(p(u),c)}P.prototype=E.prototype;P.CONNECTING=E.CONNECTING;P.OPEN=E.OPEN;P.CLOSED=E.CLOSED;window.EventSource=P}var W=window.WebSocket;if(W){function S(u,c){return c===undefined?new W(w(u)):new W(w(u),c)}S.prototype=W.prototype;S.CONNECTING=W.CONNECTING;S.OPEN=W.OPEN;S.CLOSING=W.CLOSING;S.CLOSED=W.CLOSED;window.WebSocket=S}if(navigator.serviceWorker){navigator.serviceWorker.register=function(){return Promise.reject(new Error("embedded service worker disabled"))}}})();</script>`

// This runs before the third-party page's module scripts. It exchanges the
// fragment-only token first, then holds the first API calls until NapCat's
// short-lived credential exists. Axios chooses fetch in some Chromium builds
// and XHR in others, hence the matching post-bootstrap XHR gate below.
const embeddedAPIAuthGate = `<script>(function(){var m=%q,z=new URLSearchParams(window.location.hash.slice(1)).get("alx-napcat-token");if(!z||!window.fetch||!window.crypto||!crypto.subtle)return;history.replaceState(null,"",window.location.pathname+window.location.search);var f=window.fetch.bind(window),c="",b=crypto.subtle.digest("SHA-256",new TextEncoder().encode(z+".napcat")).then(function(x){return Array.from(new Uint8Array(x)).map(function(v){return v.toString(16).padStart(2,"0")}).join("")}).then(function(h){return f(m+"api/auth/login",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({hash:h})})}).then(function(r){return r.json()}).then(function(v){c=v&&v.data&&v.data.Credential;if(!c)throw new Error("NapCat WebUI login failed");localStorage.setItem("token",JSON.stringify(c));return c}).catch(function(e){console.warn("NapCat embedded WebUI login failed",e);return ""});window.__alxNapcatCredentialBoot=b;window.fetch=function(i,n){return b.then(function(k){if(k){n=Object.assign({},n||{});var h=new Headers(n.headers||(i&&i.headers));h.delete("Authorization");h.set("X-ALX-Upstream-Authorization","Bearer "+k);n.headers=h}return f(i,n)})}})();</script>`

// embeddedAPIAuthXHRGate keeps NapCat's credential out of the ordinary
// Authorization header.  ALX authenticates the outer request with its own
// session cookie; a service credential in Authorization would otherwise be
// parsed by ginAccess as an ALX credential and rejected before proxying.
//
// This also covers a later manual visit to the WebUI URL, where NapCat has a
// token in localStorage but no one-click fragment and therefore no bootstrap
// promise to wait for.
const embeddedAPIAuthXHRGate = `<script>(function(){function saved(){var raw=localStorage.getItem("token")||"";try{var v=JSON.parse(raw);return typeof v==="string"?v:raw}catch(_){return raw}}var b=window.__alxNapcatCredentialBoot||Promise.resolve(saved());var h=XMLHttpRequest.prototype.setRequestHeader,s=XMLHttpRequest.prototype.send;XMLHttpRequest.prototype.setRequestHeader=function(k,v){if(String(k).toLowerCase()==="authorization"){this.__alxOriginalAuthorization=String(v);return}return h.call(this,k,v)};XMLHttpRequest.prototype.send=function(v){var x=this,original=x.__alxOriginalAuthorization;b.then(function(c){var value=c?"Bearer "+c:original;if(/^Bearer\\s+\\S+/i.test(value))h.call(x,"X-ALX-Upstream-Authorization",value);s.call(x,v)});return}})();</script>`

// Fetch has no setRequestHeader hook.  Install this final wrapper after the
// compatibility runtime so every existing and future NapCat fetch call (not
// only calls made during the first one-click login) uses the namespaced header.
const embeddedAPIAuthFetchNamespace = `<script>(function(){if(!window.fetch||window.__alxNapcatFetchNamespace)return;function saved(){var raw=localStorage.getItem("token")||"";try{var v=JSON.parse(raw);return typeof v==="string"?v:raw}catch(_){return raw}}window.__alxNapcatFetchNamespace=true;var f=window.fetch.bind(window);window.fetch=function(i,n){var h=new Headers((n&&n.headers)||(i&&i.headers)),a=h.get("Authorization");if(!a){var c=saved();if(c)a="Bearer "+c}if(a&&/^Bearer\s+\S+/i.test(a)){h.delete("Authorization");h.set("X-ALX-Upstream-Authorization",a);n=Object.assign({},n||{},{headers:h})}return f(i,n)}})();</script>`

// injectEmbeddedAPIBootstrap adds the compatibility script to a rewritten
// HTML document. Compressed bodies are left untouched, mirroring the HTML
// rewrite path.
func injectEmbeddedAPIBootstrap(response *http.Response, mount, upstreamHost string) {
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		return
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	_ = response.Body.Close()
	script := fmt.Sprintf(embeddedAPIAuthGate, mount) + fmt.Sprintf(embeddedAPICompatScript, mount, upstreamHost) + embeddedAPIAuthXHRGate + embeddedAPIAuthFetchNamespace
	document := string(body)
	if head := htmlHeadPattern.FindString(document); head != "" {
		document = strings.Replace(document, head, head+script, 1)
	} else {
		document = script + document
	}
	rewritten := []byte(document)
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	// This body is no longer the upstream's HTML: it contains a service-specific
	// base URL and WebSocket shim. Never let the browser reuse the upstream's
	// public cache entry, otherwise a later WebUI open can silently load the
	// original page and bypass the proxy (which surfaces as “not websocket
	// protocol” or a blank WebUI).
	response.Header.Set("Cache-Control", "no-store")
	if policy := response.Header.Get("Content-Security-Policy"); policy != "" && scriptSrcPolicy.MatchString(policy) {
		response.Header.Set("Content-Security-Policy", scriptSrcPolicy.ReplaceAllString(policy, "$1 'unsafe-inline'"))
	}
}

// rewriteNapcatWebUIAssets makes NapCat's compiled BrowserRouter and dynamic
// imports work beneath an authenticated ALX service mount. NapCat hard-codes
// both basename:"/webui/" and /webui/ asset URLs; changing browser history
// loses the proxy on refresh and module loads, so rewrite the served assets
// instead. Other local services are unchanged because they do not contain the
// NapCat-specific marker.
func rewriteNapcatWebUIAssets(response *http.Response, mount string) {
	contentType := response.Header.Get("Content-Type")
	if response.StatusCode != http.StatusOK || (!strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "text/css")) {
		return
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	_ = response.Body.Close()
	document := string(body)
	if !strings.Contains(document, "/webui/") {
		response.Body = io.NopCloser(bytes.NewReader(body))
		return
	}
	assetPrefix := mount + "webui/"
	if strings.Contains(contentType, "javascript") {
		// Preserve the Router mount separately; its basename must be the ALX
		// proxy root, whereas all assets remain under upstream /webui/.
		document = strings.ReplaceAll(document, `basename:"/webui/"`, `basename:"`+mount+`"`)
	}
	document = strings.ReplaceAll(document, "/webui/", assetPrefix)
	rewritten := []byte(document)
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	response.Header.Set("Cache-Control", "no-store")
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
			reachable := localServiceReachable(r.Context(), service)
			item := localServiceView{PluginID: plugin.ID, ID: service.ID, Name: service.Name, Reachable: reachable, ProxyURL: localServiceURL(plugin.ID, service.ID), Embed: service.Embed, WebSocket: service.WebSocket}
			if !reachable {
				item.Error = "服务未启动或无法连接"
			}
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) localServiceProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "本地服务不支持该请求方式。")
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
	if isUpgradeRequest(r) {
		if !service.WebSocket {
			writeError(w, http.StatusNotImplemented, "该本地服务未声明 WebSocket 支持。 ")
			return
		}
		s.localServiceWebSocketHandler(w, r, plugin, service, requestPath)
		return
	}
	s.localServiceProxyWith(w, r, plugin, service, requestPath, localServiceURL(plugin.ID, service.ID))
}

// dynamicLocalServiceProxyHandler proxies a loopback port supplied by the
// plugin (e.g. a Docker-published port) through the authenticated management
// origin, so the plugin's iframe can embed container web UIs the same way
// alemonx-qq embeds its manifest-declared services. Only 127.0.0.1 is ever
// reachable, and an installed plugin is already trusted with host execution.
func (s *server) dynamicLocalServiceProxyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "本地服务不支持该请求方式。")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, dynamicServicePrefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || !pluginIDPattern.MatchString(parts[0]) {
		writeError(w, http.StatusNotFound, "缺少动态本地服务标识。")
		return
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		writeError(w, http.StatusBadRequest, "动态本地服务端口无效。")
		return
	}
	plugin, err := s.plugins.Find(parts[0])
	if err != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusNotFound, "未找到已启用的插件。")
		return
	}
	requestPath := "/"
	if len(parts) == 3 {
		requestPath += parts[2]
	}
	if strings.Contains(requestPath, "\\") || strings.Contains(requestPath, "..") {
		writeError(w, http.StatusBadRequest, "动态本地服务路径无效。")
		return
	}
	service := setupplugin.ServiceSpec{
		ID:             "dynamic-" + strconv.Itoa(port),
		Name:           "动态回环服务",
		Host:           "127.0.0.1",
		Port:           port,
		HealthPath:     "/",
		Embed:          true,
		RewriteHTML:    true,
		RewriteAPIBase: true,
		WebSocket:      true,
		SSE:            true,
	}
	if !localServiceReachable(r.Context(), service) {
		writeError(w, http.StatusBadGateway, "本地服务尚未启动或无法连接。")
		return
	}
	if isUpgradeRequest(r) {
		s.localServiceWebSocketHandler(w, r, plugin, service, requestPath)
		return
	}
	s.localServiceProxyWith(w, r, plugin, service, requestPath, dynamicServiceURL(plugin.ID, port))
}

func dynamicServiceURL(pluginID string, port int) string {
	return dynamicServicePrefix + pluginID + "/" + strconv.Itoa(port) + "/"
}

func (s *server) localServiceProxyWith(w http.ResponseWriter, r *http.Request, plugin setupplugin.Plugin, service setupplugin.ServiceSpec, requestPath, mount string) {
	target, err := localServiceTarget(service, requestPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !strings.HasSuffix(mount, "/") {
		mount += "/"
	}
	cookiePrefix := localServiceCookiePrefix(plugin.ID, service.ID)
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme, request.URL.Host, request.URL.Path, request.URL.RawPath = target.Scheme, target.Host, target.Path, ""
			request.URL.RawQuery = r.URL.RawQuery
			request.Host = target.Host
			// A rewritten embedded WebUI (NapCat in particular) authenticates its
			// own API calls with a short-lived Bearer credential. Do not confuse
			// that service-local credential with an ALX management credential and
			// strip it: doing so makes login succeed in the browser but guarantees
			// every following upstream request is Unauthorized.
			authorization := request.Header.Get("X-ALX-Upstream-Authorization")
			request.Header.Del("Authorization")
			request.Header.Del("X-ALX-Upstream-Authorization")
			if service.RewriteAPIBase && strings.HasPrefix(strings.TrimSpace(authorization), "Bearer ") {
				request.Header.Set("Authorization", authorization)
			}
			request.Header.Del("Cookie")
			// HTML rewritten below contains a service-specific base URL and API/
			// WebSocket shim. A browser normally requests gzip; rewriting only an
			// uncompressed body would otherwise make this work with curl but fail
			// in the embedded WebUI. Ask the loopback service for identity here so
			// every response that needs rewriting follows the same path.
			if service.RewriteHTML {
				request.Header.Set("Accept-Encoding", "identity")
			}
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
			if service.RewriteHTML && service.RewriteAPIBase {
				injectEmbeddedAPIBootstrap(response, mount, target.Host)
				rewriteNapcatWebUIAssets(response, mount)
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

func (s *server) localServiceStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	pluginID := strings.TrimSpace(r.URL.Query().Get("plugin"))
	serviceID := strings.TrimSpace(r.URL.Query().Get("service"))
	if pluginID == "" || serviceID == "" {
		writeError(w, http.StatusBadRequest, "请指定插件和服务。")
		return
	}
	plugin, service, _, err := s.resolveLocalService(localServiceURL(pluginID, serviceID))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	reachable := localServiceReachable(r.Context(), service)
	result := localServiceView{PluginID: plugin.ID, ID: service.ID, Name: service.Name, Reachable: reachable, ProxyURL: localServiceURL(plugin.ID, service.ID), Embed: service.Embed, WebSocket: service.WebSocket}
	if !reachable {
		result.Error = "服务未启动或无法连接"
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) localServiceWebSocketHandler(w http.ResponseWriter, r *http.Request, plugin setupplugin.Plugin, service setupplugin.ServiceSpec, requestPath string) {
	target, err := localServiceTarget(service, requestPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target.Scheme = "ws"
	cookiePrefix := localServiceCookiePrefix(plugin.ID, service.ID)
	header := http.Header{}
	for _, cookie := range r.Cookies() {
		if strings.HasPrefix(cookie.Name, cookiePrefix) {
			header.Add("Cookie", strings.TrimPrefix(cookie.Name, cookiePrefix)+"="+cookie.Value)
		}
	}
	websocket.Handler(func(client *websocket.Conn) {
		// The upstream sees a same-origin loopback request. Forwarding the
		// workbench origin would make WebUI origin checks reject a safe proxy.
		upstream, dialErr := websocket.DialConfig(&websocket.Config{Location: target, Origin: &url.URL{Scheme: "http", Host: target.Host}, Version: websocket.ProtocolVersionHybi13, Header: header})
		if dialErr != nil {
			_ = client.Close()
			return
		}
		defer upstream.Close()
		defer client.Close()
		// Relay complete frames rather than raw io.Copy. This retains text vs.
		// binary semantics while closing the opposite side when either peer ends.
		go func() {
			for {
				var frame localWebSocketFrame
				if err := localWebSocketFrameCodec.Receive(client, &frame); err != nil {
					return
				}
				if localWebSocketFrameCodec.Send(upstream, frame) != nil {
					return
				}
			}
		}()
		for {
			var frame localWebSocketFrame
			if err := localWebSocketFrameCodec.Receive(upstream, &frame); err != nil {
				return
			}
			if localWebSocketFrameCodec.Send(client, frame) != nil {
				return
			}
		}
	}).ServeHTTP(w, r)
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
	// Web apps such as NapCat can emit absolute assets under their own base
	// path (for example /webui/assets/app.js). The service mount already maps
	// to that base path, so appending it again would request
	// /webui/webui/assets/app.js and usually return the HTML fallback.
	targetPath := ""
	if service.RewriteAPIBase && (strings.HasPrefix(clean, "/api/") || strings.HasPrefix(clean, "/files/")) {
		// NapCat serves its UI under /webui but exposes its control API and
		// theme files at the server root. The injected browser shim deliberately
		// preserves those root-relative calls under the service mount.
		targetPath = clean
	} else if base != "/" && (clean == base || strings.HasPrefix(clean, base+"/")) {
		targetPath = clean
	} else {
		targetPath = path.Join(base, clean)
	}
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
	// Connection: Upgrade alone is not a WebSocket handshake. Some WebUI
	// navigation and streaming requests retain that hop-by-hop token; routing
	// those ordinary HTTP requests to websocket.Handler makes x/net reject them
	// with “not websocket protocol”. The Upgrade header must explicitly name
	// websocket before the proxy takes the WebSocket path.
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
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
