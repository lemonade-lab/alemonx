package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"alemonx/internal/setupplugin"
	"alemonx/internal/system"
)

// safeWebviewResourcePattern allows only ordinary relative paths inside a
// plugin's web root. Webviews must never accept absolute paths, traversal or
// backslashes: the host serves the resolved resource itself.
var safeWebviewResourcePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

type systemCapabilityItem struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type webviewOpenRequest struct {
	PluginID string `json:"pluginId"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Resource string `json:"resource"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// systemCapabilitiesHandler publishes the versioned, host-owned API catalog.
// Plugins call typed endpoints below; this document is for capability
// discovery and clear UI fallbacks, never a per-plugin permission whitelist.
func (s *server) systemCapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	items := []systemCapabilityItem{
		{Name: "finder.pick", Version: "v1", Available: true},
		{Name: "webview.open", Version: "v1", Available: true},
		{Name: "ui.alert", Version: "v1", Available: true},
		{Name: "ui.message", Version: "v1", Available: true},
		{Name: "ui.modal", Version: "v1", Available: true},
		{Name: "ui.notification", Version: "v1", Available: true},
		{Name: "context.current-robot", Version: "v1", Available: true},
		{Name: "context.network-settings", Version: "v1", Available: s.network != nil, Reason: unavailableReason(s.network != nil, "当前网络配置不可用")},
		{Name: "desktop.open", Version: "v1", Available: true},
		{Name: "clipboard.read", Version: "v1", Available: system.DesktopClipboardAvailable(), Reason: unavailableReason(system.DesktopClipboardAvailable(), "当前桌面没有可用剪贴板服务")},
		{Name: "clipboard.write", Version: "v1", Available: system.DesktopClipboardAvailable(), Reason: unavailableReason(system.DesktopClipboardAvailable(), "当前桌面没有可用剪贴板服务")},
		{Name: "notification.send", Version: "v1", Available: runtime.GOOS == "darwin" || runtime.GOOS == "linux", Reason: unavailableReason(runtime.GOOS == "darwin" || runtime.GOOS == "linux", "当前平台暂未提供系统通知服务")},
		{Name: "system.info", Version: "v1", Available: true},
		{Name: "network.fetch", Version: "v1", Available: s.network != nil, Reason: unavailableReason(s.network != nil, "当前网络配置不可用")},
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// systemCapabilityWebviewOpenHandler lets an installed plugin open a
// host-managed WebView window. The plugin chooses either a same-origin static
// resource inside its own web root or an external http(s) URL; the host
// validates identity and target, and the workbench frontend renders the
// window. Opening a URL never grants the target page any host privilege.
func (s *server) systemCapabilityWebviewOpenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input webviewOpenRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "打开 WebView 请求无效。")
		return
	}
	plugin, ok := s.hostCapabilityPlugin(w, r, input.PluginID)
	if !ok {
		return
	}
	rawURL := strings.TrimSpace(input.URL)
	resource := strings.TrimSpace(input.Resource)
	hasURL := rawURL != ""
	hasResource := resource != ""
	if hasURL == hasResource {
		writeError(w, http.StatusBadRequest, "请指定 url 或 resource 其中之一。")
		return
	}
	src := ""
	kind := "url"
	if hasURL {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			writeError(w, http.StatusBadRequest, "WebView 地址必须是有效的 HTTP 或 HTTPS 地址。")
			return
		}
		src = parsed.String()
	} else {
		resourcePath := resource
		query := ""
		if idx := strings.IndexByte(resource, '?'); idx >= 0 {
			resourcePath, query = resource[:idx], resource[idx:]
		}
		resourcePath = strings.TrimPrefix(filepath.ToSlash(resourcePath), "/")
		if resourcePath == "" || resourcePath == "." || strings.Contains(resourcePath, "\\") || !safeWebviewResourcePattern.MatchString(resourcePath) {
			writeError(w, http.StatusBadRequest, "WebView 静态资源路径无效。")
			return
		}
		for _, segment := range strings.Split(resourcePath, "/") {
			if segment == ".." {
				writeError(w, http.StatusBadRequest, "WebView 静态资源路径无效。")
				return
			}
		}
		base := "/api/v1/setup/plugins/web/" + plugin.ID + "/"
		if plugin.DevelopmentWebProxy {
			base = "/api/v1/setup/plugins/development/" + plugin.ID + "/web/"
		} else {
			root, rootErr := plugin.WebRoot()
			if rootErr != nil {
				writeError(w, http.StatusNotFound, "插件 Web 界面不可用。")
				return
			}
			candidate := filepath.Join(root, resourcePath)
			resolved, resolveErr := filepath.EvalSymlinks(candidate)
			if resolveErr != nil && filepath.Ext(resourcePath) == "" {
				// Mirror the static web handler's SPA fallback: an
				// extensionless route is served by index.html.
				resolved = filepath.Join(root, "index.html")
			} else if resolveErr != nil {
				writeError(w, http.StatusNotFound, "WebView 静态资源不存在。")
				return
			}
			rel, relErr := filepath.Rel(root, resolved)
			if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				writeError(w, http.StatusBadRequest, "WebView 静态资源路径无效。")
				return
			}
			info, statErr := os.Stat(resolved)
			if statErr != nil || !info.Mode().IsRegular() {
				writeError(w, http.StatusNotFound, "WebView 静态资源不存在。")
				return
			}
		}
		src = base + resourcePath + query
		kind = "static"
	}
	width := input.Width
	if width < 480 || width > 1600 {
		width = 960
	}
	height := input.Height
	if height < 420 || height > 1200 {
		height = 680
	}
	title := strings.TrimSpace(input.Title)
	if runes := []rune(title); len(runes) > 80 {
		title = string(runes[:80])
	}
	if title == "" {
		title = plugin.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"pluginId": plugin.ID,
		"title":    title,
		"src":      src,
		"kind":     kind,
		"width":    width,
		"height":   height,
	})
}

func unavailableReason(available bool, reason string) string {
	if available {
		return ""
	}
	return reason
}

// pluginHostContext is host-owned, per signed-in workbench account. It never
// accepts arbitrary plugin data: the main app can only set a validated robot
// root, while installed system plugins read the narrow built-in context.
type pluginHostContext struct {
	RobotRoot string
	UpdatedAt time.Time
}

type hostCapabilityRequest struct {
	PluginID string `json:"pluginId"`
	PickerID string `json:"pickerId,omitempty"`
}

// systemCapabilityFinderHandler exposes a host-owned Finder definition. The
// setup-plugin page bridge asks the parent workbench to render the actual Web
// Finder, so this endpoint never opens an OS-native dialog.
func (s *server) systemCapabilityFinderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input hostCapabilityRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请选择有效的 Finder 请求。")
		return
	}
	s.servePluginFinderDefinition(w, r, input.PluginID, input.PickerID)
}

// systemPickerHandler remains as a compatibility alias. Static plugin pages
// intercept both paths and forward the request to the workbench Web Finder.
func (s *server) systemPickerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input hostCapabilityRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请选择有效的系统文件或目录选择器。")
		return
	}
	s.servePluginFinderDefinition(w, r, input.PluginID, input.PickerID)
}

func (s *server) servePluginFinderDefinition(w http.ResponseWriter, r *http.Request, pluginID, pickerID string) {
	plugin, ok := s.hostCapabilityPlugin(w, r, pluginID)
	if !ok {
		return
	}
	if !systemPickerIDPattern.MatchString(strings.TrimSpace(pickerID)) {
		writeError(w, http.StatusBadRequest, "请选择有效的 Finder 项目。")
		return
	}
	picker, ok := plugin.SystemPicker(pickerID)
	if !ok {
		writeError(w, http.StatusForbidden, "该插件未声明此 Finder 项目。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pluginId": plugin.ID,
		"pickerId": picker.ID,
		"kind":     picker.Kind,
		"title":    picker.Title,
		"multiple": picker.Multiple,
	})
}

// systemCapabilityContextHandler exposes sanitized, read-only workbench data.
// Query keys are deliberately finite: robot and network. Secrets, draft files
// and arbitrary workspace state never cross this boundary.
func (s *server) systemCapabilityContextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	pluginID := strings.TrimSpace(r.URL.Query().Get("pluginId"))
	keys := strings.Split(strings.TrimSpace(r.URL.Query().Get("keys")), ",")
	if pluginID == "" || len(keys) == 0 || keys[0] == "" {
		writeError(w, http.StatusBadRequest, "请指定插件和所需的上下文能力。")
		return
	}
	response := map[string]any{}
	for _, key := range keys {
		switch strings.TrimSpace(key) {
		case "robot":
			if _, ok := s.hostCapabilityPlugin(w, r, pluginID); !ok {
				return
			}
			response["robot"] = s.currentRobotContext(r)
		case "network":
			if _, ok := s.hostCapabilityPlugin(w, r, pluginID); !ok {
				return
			}
			if s.network == nil {
				writeError(w, http.StatusServiceUnavailable, "当前网络配置不可用。")
				return
			}
			response["network"] = s.network.Settings()
		default:
			writeError(w, http.StatusBadRequest, "不支持的工作台上下文能力。")
			return
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) systemCapabilityDesktopOpenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		PluginID string `json:"pluginId"`
		Target   string `json:"target"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&input) != nil {
		writeError(w, http.StatusBadRequest, "打开目标无效。")
		return
	}
	if _, ok := s.hostCapabilityPlugin(w, r, input.PluginID); !ok {
		return
	}
	if err := system.OpenDesktopTarget(input.Target); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": "已交给系统打开。"})
}

func (s *server) systemCapabilityClipboardHandler(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(r.URL.Query().Get("pluginId"))
	if r.Method == http.MethodPost {
		var input struct {
			PluginID string `json:"pluginId"`
			Text     string `json:"text"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
			writeError(w, http.StatusBadRequest, "剪贴板内容无效。")
			return
		}
		pluginID = input.PluginID
		if _, ok := s.hostCapabilityPlugin(w, r, pluginID); !ok {
			return
		}
		if err := system.WriteDesktopClipboard(input.Text); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": "已写入系统剪贴板。"})
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if _, ok := s.hostCapabilityPlugin(w, r, pluginID); !ok {
		return
	}
	value, err := system.ReadDesktopClipboard()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": value})
}

func (s *server) systemCapabilityNotificationHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		PluginID string `json:"pluginId"`
		Title    string `json:"title"`
		Message  string `json:"message"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input) != nil {
		writeError(w, http.StatusBadRequest, "通知内容无效。")
		return
	}
	if _, ok := s.hostCapabilityPlugin(w, r, input.PluginID); !ok {
		return
	}
	if err := system.SendDesktopNotification(input.Title, input.Message); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": "已发送系统通知。"})
}

func (s *server) systemCapabilityInfoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if _, ok := s.hostCapabilityPlugin(w, r, r.URL.Query().Get("pluginId")); !ok {
		return
	}
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]string{"platform": runtime.GOOS, "architecture": runtime.GOARCH, "hostname": host})
}

func (s *server) systemCapabilityNetworkFetchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		PluginID string `json:"pluginId"`
		URL      string `json:"url"`
		Method   string `json:"method"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input) != nil {
		writeError(w, http.StatusBadRequest, "网络请求无效。")
		return
	}
	if _, ok := s.hostCapabilityPlugin(w, r, input.PluginID); !ok {
		return
	}
	parsed, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "请求地址必须是 HTTP 或 HTTPS 地址。")
		return
	}
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead {
		writeError(w, http.StatusBadRequest, "network.fetch 仅支持 GET 和 HEAD。")
		return
	}
	if s.network == nil {
		writeError(w, http.StatusServiceUnavailable, "当前网络配置不可用。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, method, parsed.String(), nil)
	response, err := s.network.Client(0).Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "网络请求失败，请检查工作台网络配置。")
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	writeJSON(w, http.StatusOK, map[string]any{"status": response.StatusCode, "contentType": response.Header.Get("Content-Type"), "bodyBase64": base64.StdEncoding.EncodeToString(body), "truncated": response.ContentLength > int64(len(body))})
}

// systemCurrentRobotHandler receives the main workbench's current selection.
// It does not trust the browser path: the host revalidates that it is a
// managed AlemonJS project before exposing it to plugins.
func (s *server) systemCurrentRobotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root string `json:"root"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "当前机器人目录无效。")
		return
	}
	root := strings.TrimSpace(input.Root)
	if root != "" {
		var err error
		root, err = s.managedDirectory(root)
		if err != nil {
			writeError(w, http.StatusBadRequest, "当前机器人目录不受工作台管理。")
			return
		}
		if _, err := s.robots.Read(root, "alemon.config.yaml"); err != nil {
			writeError(w, http.StatusBadRequest, "当前目录不是有效的 AlemonJS 机器人。")
			return
		}
		if info, err := os.Stat(filepath.Join(root, "package.json")); err != nil || info.IsDir() {
			writeError(w, http.StatusBadRequest, "当前目录不是有效的 AlemonJS 机器人。")
			return
		}
	}
	key := s.hostContextKey(r)
	s.hostContextMu.Lock()
	if s.hostContexts == nil {
		s.hostContexts = map[string]pluginHostContext{}
	}
	s.hostContexts[key] = pluginHostContext{RobotRoot: root, UpdatedAt: time.Now()}
	s.hostContextMu.Unlock()
	writeJSON(w, http.StatusOK, s.currentRobotContext(r))
}

// hostCapabilityPlugin resolves an installed system plugin for a built-in host
// capability. Capabilities are supplied by the host and remain typed API
// endpoints; plugins do not need repetitive per-capability permission flags.
func (s *server) hostCapabilityPlugin(w http.ResponseWriter, r *http.Request, pluginID string) (setupplugin.Plugin, bool) {
	pluginID = strings.TrimSpace(pluginID)
	if !systemPickerIDPattern.MatchString(pluginID) || s.plugins == nil {
		writeError(w, http.StatusBadRequest, "插件能力请求无效。")
		return setupplugin.Plugin{}, false
	}
	if s.auth != nil {
		status, err := s.auth.Status(s.authToken(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return setupplugin.Plugin{}, false
		}
		if status.Enabled && !status.Authenticated {
			writeError(w, http.StatusUnauthorized, "请先登录工作台后再请求宿主能力。")
			return setupplugin.Plugin{}, false
		}
	}
	plugin, err := s.plugins.Find(pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusNotFound, "系统插件未安装或已停用。")
		return setupplugin.Plugin{}, false
	}
	return plugin, true
}

func (s *server) hostContextKey(r *http.Request) string {
	if s.auth == nil {
		return "local"
	}
	status, err := s.auth.Status(s.authToken(r))
	if err == nil && status.Enabled && status.Authenticated && strings.TrimSpace(status.Account) != "" {
		return "account:" + status.Account
	}
	return "local"
}

func (s *server) currentRobotContext(r *http.Request) any {
	s.hostContextMu.RLock()
	context := s.hostContexts[s.hostContextKey(r)]
	s.hostContextMu.RUnlock()
	if context.RobotRoot == "" {
		return nil
	}
	return map[string]string{"root": context.RobotRoot, "name": filepath.Base(context.RobotRoot)}
}
