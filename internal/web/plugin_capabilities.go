package web

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alemonx/internal/setupplugin"
)

// pluginHostContext is host-owned, per signed-in workbench account. It never
// accepts arbitrary plugin data: the main app can only set a validated robot
// root, while plugins can only read the narrow context they declared.
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
	plugin, ok := s.hostCapabilityPlugin(w, r, pluginID, setupplugin.HostCapabilityFinder)
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
			if _, ok := s.hostCapabilityPlugin(w, r, pluginID, setupplugin.HostCapabilityRobotContext); !ok {
				return
			}
			response["robot"] = s.currentRobotContext(r)
		case "network":
			if _, ok := s.hostCapabilityPlugin(w, r, pluginID, setupplugin.HostCapabilityNetworkContext); !ok {
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

func (s *server) hostCapabilityPlugin(w http.ResponseWriter, r *http.Request, pluginID, capability string) (setupplugin.Plugin, bool) {
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
	if plugin.DevelopmentSource && !s.requirePluginDevelopment(w, r) {
		return setupplugin.Plugin{}, false
	}
	if !plugin.AllowsHostCapability(capability) {
		writeError(w, http.StatusForbidden, "该插件未声明此宿主能力。")
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
