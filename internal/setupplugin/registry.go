// Package setupplugin discovers setup plugins that add system controls to
// alx. A plugin is a static web UI (web.root) plus an optional executor that
// runs system operations the web UI requests through a generic forward. The
// declarative pages/actions manifest model has been removed: the web UI is the
// plugin's interface. Discovery never executes plugin code.
package setupplugin

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"alemonx/internal/system"

	"github.com/fsnotify/fsnotify"
)

const manifestName = "alx.json"
const installMetadataName = ".alx-install.json"
const maxManifestSize = 64 * 1024
const onlineIndexURL = "https://raw.githubusercontent.com/lemonade-lab/alemonjs.dev/main/docs/apps-x.md"
const installTimeout = 3 * time.Minute

var validID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
var onlineRepository = regexp.MustCompile(`(?m)^\s*\[[^\]]+\]:\s*(https://github\.com/lemonade-lab/([A-Za-z0-9_.-]+))\s*$`)
var onlineSource = regexp.MustCompile(`^https://github\.com/lemonade-lab/[A-Za-z0-9_.-]+$`)

// Navigation controls where the plugin appears in the global function rail.
type Navigation struct {
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Order int    `json:"order,omitempty"`
}

// Plugin is intentionally declarative. It is safe to list and render because
// no file from its directory is executed during discovery. A plugin is usable
// only when it declares a web root (its UI) and has an executor.
type Plugin struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Description    string            `json:"description,omitempty"`
	Platforms      []string          `json:"platforms,omitempty"`
	Navigation     Navigation        `json:"navigation"`
	Runtime        string            `json:"runtime,omitempty"`
	Entry          map[string]string `json:"entry,omitempty"`
	Development    *RuntimeSpec      `json:"development,omitempty"`
	Web            *WebSpec          `json:"web,omitempty"`
	Services       []ServiceSpec     `json:"services,omitempty"`
	Permissions    Permissions       `json:"permissions,omitempty"`
	Runnable       bool              `json:"runnable"`
	Enabled        bool              `json:"enabled"`
	Online         bool              `json:"online,omitempty"`
	Source         string            `json:"source,omitempty"`
	InstalledTag   string            `json:"installedTag,omitempty"`
	InstalledAsset string            `json:"installedAsset,omitempty"`
	ArchiveSHA256  string            `json:"archiveSha256,omitempty"`
	Fingerprint    string            `json:"fingerprint,omitempty"`
}

// Permissions explicitly opts a plugin into individually elevated actions.
// The browser never controls elevation: Registry consults this allowlist.
type Permissions struct {
	ElevatedActions []string `json:"elevatedActions,omitempty"`
}

func (p Plugin) RequiresElevation(action string) bool {
	for _, candidate := range p.Permissions.ElevatedActions {
		if candidate == action {
			return true
		}
	}
	return false
}

type installMetadata struct {
	ID            string `json:"id"`
	Tag           string `json:"tag"`
	Asset         string `json:"asset"`
	ArchiveSHA256 string `json:"archiveSha256"`
	Fingerprint   string `json:"fingerprint"`
	CachePath     string `json:"cachePath,omitempty"`
	InstalledAt   string `json:"installedAt"`
	LastUsedAt    string `json:"lastUsedAt,omitempty"`
}

// RuntimeSpec is an optional development fallback. Release plugins should use
// a compiled binary. A source runner may be kept here so contributors can run
// a plugin from a checkout without first producing every platform binary.
type RuntimeSpec struct {
	Runtime string            `json:"runtime"`
	Entry   map[string]string `json:"entry"`
}

// WebSpec declares the plugin's web UI directory inside the plugin folder. alx
// serves it same-origin so the UI can call the plugin's action forward API
// directly. Only installed, enabled plugins have their web root served.
type WebSpec struct {
	Root string `json:"root"`
}

// ServiceSpec is an allowlisted loopback HTTP service contributed by a setup
// plugin. The browser never supplies its host or port; alx proxies only these
// manifest-declared destinations through the authenticated management origin.
type ServiceSpec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	BasePath    string `json:"basePath,omitempty"`
	HealthPath  string `json:"healthPath,omitempty"`
	Embed       bool   `json:"embed,omitempty"`
	RewriteHTML bool   `json:"rewriteHtml,omitempty"`
	SSE         bool   `json:"sse,omitempty"`
	WebSocket   bool   `json:"websocket,omitempty"`
}

// Progress is an optional, structured stderr event emitted while a plugin
// action runs. Stdout remains reserved for the terminal alx/v1 response.
type Progress struct {
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
}

// Registry scans immediate child directories in order. Earlier roots win on
// duplicate IDs, allowing a user-installed plugin to override a bundled one.
// Results are cached and refreshed by Rescan/StartWatch so hot-plugging a
// plugin directory is reflected without restarting alx.
type Registry struct {
	mu                sync.RWMutex
	roots             []string
	statePath         string
	cacheRoot         string
	onlineIndexURL    string
	httpClient        *http.Client
	onlineManifestURL func(string) string
	releaseURL        func(string) string
	cached            []Plugin
	revision          uint64
	loaded            bool
	lastFingerprint   string
	listeners         map[chan struct{}]struct{}
}

// Subscribe returns a channel that receives a signal whenever the cached plugin
// set changes (revision bumps). Consumers that render the plugin list use this
// to refresh without polling the whole list. The signal is coalesced: a slow
// consumer may miss intermediate changes but is always woken for the latest.
func (r *Registry) Subscribe() chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listeners == nil {
		r.listeners = map[chan struct{}]struct{}{}
	}
	ch := make(chan struct{}, 1)
	r.listeners[ch] = struct{}{}
	return ch
}

// Unsubscribe stops delivering change signals to a channel from Subscribe.
func (r *Registry) Unsubscribe(ch chan struct{}) {
	r.mu.Lock()
	delete(r.listeners, ch)
	r.mu.Unlock()
}

func NewRegistry(roots ...string) *Registry {
	if len(roots) == 0 {
		roots = defaultRoots()
		return &Registry{
			roots:             uniqueRoots(roots),
			statePath:         defaultStatePath(),
			cacheRoot:         defaultCacheRoot(),
			onlineIndexURL:    onlineIndexURL,
			httpClient:        &http.Client{Timeout: 5 * time.Second},
			onlineManifestURL: defaultOnlineManifestURL,
			releaseURL:        defaultReleaseURL,
		}
	}
	return &Registry{roots: uniqueRoots(roots)}
}

func defaultRoots() []string {
	roots := make([]string, 0, 3)
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(executable), "plugins"))
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(cwd, "plugins"))
	}
	if config, err := os.UserConfigDir(); err == nil {
		roots = append(roots, filepath.Join(config, "alx", "plugins"))
	}
	return roots
}

func defaultStatePath() string {
	if config, err := os.UserConfigDir(); err == nil {
		return filepath.Join(config, "alx", "setup-plugins.json")
	}
	return ""
}

func defaultCacheRoot() string {
	if config, err := os.UserConfigDir(); err == nil {
		return filepath.Join(config, "alx", "plugin-cache")
	}
	return ""
}

func uniqueRoots(roots []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(root)
		if root == "." || seen[root] {
			continue
		}
		seen[root] = true
		result = append(result, root)
	}
	return result
}

// List returns valid, enabled plugins from the cached snapshot.
func (r *Registry) List() []Plugin {
	r.ensureLoaded()
	items := r.snapshot()
	enabled := items[:0]
	for _, plugin := range items {
		if plugin.Enabled {
			enabled = append(enabled, plugin)
		}
	}
	return enabled
}

// All includes disabled plugins so the manager can offer a deliberate
// re-enable action, while List remains the source for the live navigation.
func (r *Registry) All() []Plugin {
	r.ensureLoaded()
	return r.snapshot()
}

// Find returns one currently discoverable plugin from the cache.
func (r *Registry) Find(id string) (Plugin, error) {
	for _, plugin := range r.All() {
		if plugin.ID == id {
			return plugin, nil
		}
	}
	return Plugin{}, errors.New("未找到已加载的 Setup 插件")
}

// Revision returns the cache revision, bumped whenever a rescan changes the
// plugin set. Poll it (or the plugin list) for hot-plug detection.
func (r *Registry) Revision() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.revision
}

func (r *Registry) ensureLoaded() {
	r.mu.RLock()
	loaded := r.loaded
	r.mu.RUnlock()
	if loaded {
		return
	}
	r.Rescan()
}

func (r *Registry) snapshot() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Plugin, len(r.cached))
	copy(items, r.cached)
	return items
}

// Rescan recomputes the full plugin set and replaces the cache. It bumps the
// revision only when the set actually changed, so watchers can cheaply detect
// real changes.
func (r *Registry) Rescan() {
	next := r.compute()
	r.mu.Lock()
	changed := !pluginSetsEqual(r.cached, next)
	r.cached = next
	r.loaded = true
	if changed {
		r.revision++
		// Wake subscribers non-blockingly so a full rescan never blocks on a
		// slow SSE consumer. The 1-buffer channel coalesces bursts.
		for ch := range r.listeners {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}
	r.mu.Unlock()
}

func pluginSetsEqual(a, b []Plugin) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !pluginsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func pluginsEqual(a, b Plugin) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Version == b.Version &&
		a.Description == b.Description && a.Runnable == b.Runnable &&
		a.Enabled == b.Enabled && a.Online == b.Online && a.InstalledTag == b.InstalledTag && a.InstalledAsset == b.InstalledAsset && a.ArchiveSHA256 == b.ArchiveSHA256 && a.Fingerprint == b.Fingerprint &&
		strings.Join(a.Platforms, ",") == strings.Join(b.Platforms, ",") &&
		a.Navigation.Label == b.Navigation.Label && a.Navigation.Icon == b.Navigation.Icon &&
		a.Navigation.Order == b.Navigation.Order && webRootEqual(a.Web, b.Web) &&
		entryEqual(a.Entry, b.Entry) && entryEqual(a.DevelopmentEntry(), b.DevelopmentEntry())
}

func (p Plugin) DevelopmentEntry() map[string]string {
	if p.Development != nil {
		return p.Development.Entry
	}
	return nil
}

func webRootEqual(a, b *WebSpec) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Root == b.Root
}

func entryEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (r *Registry) compute() []Plugin {
	items := make([]Plugin, 0)
	seen := map[string]bool{}
	disabled := r.disabled()
	for _, root := range r.roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			plugin, err := load(filepath.Join(root, entry.Name()))
			if err != nil || seen[plugin.ID] || !supportsCurrentPlatform(plugin.Platforms) {
				continue
			}
			plugin.Enabled = !disabled[plugin.ID]
			seen[plugin.ID] = true
			items = append(items, plugin)
		}
	}
	for _, plugin := range r.onlinePlugins() {
		if seen[plugin.ID] || !supportsCurrentPlatform(plugin.Platforms) {
			continue
		}
		plugin.Enabled = !disabled[plugin.ID]
		seen[plugin.ID] = true
		items = append(items, plugin)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Navigation.Order != items[j].Navigation.Order {
			return items[i].Navigation.Order < items[j].Navigation.Order
		}
		return items[i].Navigation.Label < items[j].Navigation.Label
	})
	return items
}

type disabledState struct {
	Disabled []string `json:"disabled"`
}

func (r *Registry) disabled() map[string]bool {
	items := map[string]bool{}
	if r.statePath == "" {
		return items
	}
	data, err := os.ReadFile(r.statePath)
	if err != nil || len(data) > maxManifestSize {
		return items
	}
	var state disabledState
	if json.Unmarshal(data, &state) != nil {
		return items
	}
	for _, id := range state.Disabled {
		if validID.MatchString(id) {
			items[id] = true
		}
	}
	return items
}

// SetEnabled is a reversible disable: the plugin's files are left intact,
// but it disappears from the active function rail and its web UI is no longer
// served. The cache is refreshed immediately.
func (r *Registry) SetEnabled(id string, enabled bool) error {
	if !validID.MatchString(id) {
		return errors.New("无效的 Setup 插件标识")
	}
	found := false
	for _, plugin := range r.All() {
		if plugin.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("未找到 Setup 插件")
	}
	if r.statePath == "" {
		return errors.New("当前运行环境不支持保存插件启用状态")
	}
	disabled := r.disabled()
	if enabled {
		delete(disabled, id)
	} else {
		disabled[id] = true
	}
	ids := make([]string, 0, len(disabled))
	for item := range disabled {
		ids = append(ids, item)
	}
	sort.Strings(ids)
	data, err := json.Marshal(disabledState{Disabled: ids})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.statePath), 0755); err != nil {
		return err
	}
	temporary := r.statePath + ".new"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, r.statePath); err != nil {
		return err
	}
	r.Rescan()
	return nil
}

// Uninstall removes only a release installation owned by the Setup-plugin
// installer. Development, bundled, and manually copied directories do not
// carry a valid .alx-install.json and therefore cannot be deleted through the
// workbench. Cached archives are intentionally retained so a later reinstall
// can reuse a verified download without fetching it again.
func (r *Registry) Uninstall(id string) error {
	if !validID.MatchString(id) {
		return errors.New("无效的 Setup 插件标识")
	}
	plugin, err := r.Find(id)
	if err != nil || plugin.Online {
		return errors.New("在线系统插件尚未安装")
	}
	metadata, err := r.readActiveMetadata(id)
	if err != nil || metadata.ID != id || metadata.Fingerprint == "" || metadata.Fingerprint != installFingerprint(metadata.ID, metadata.Tag, metadata.Asset, metadata.ArchiveSHA256) {
		return errors.New("该插件不是工作台安装的 Release，不能从这里删除")
	}
	source := filepath.Clean(plugin.Source)
	if source == "." || filepath.Base(source) == "." || filepath.Base(source) == ".." {
		return errors.New("插件安装目录无效")
	}
	if info, statErr := os.Lstat(source); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("插件安装目录无效或不是普通目录")
	}
	ownedRoot := false
	for _, root := range r.roots {
		if filepath.Clean(filepath.Dir(source)) == filepath.Clean(root) {
			ownedRoot = true
			break
		}
	}
	if !ownedRoot {
		return errors.New("插件安装目录不受工作台管理，已拒绝删除")
	}
	if err := os.RemoveAll(source); err != nil {
		return errors.New("删除插件文件失败：" + err.Error())
	}
	// A previous reversible disable must not hide the online entry after the
	// local release directory is gone.
	if r.statePath != "" {
		disabled := r.disabled()
		if disabled[id] {
			delete(disabled, id)
			ids := make([]string, 0, len(disabled))
			for item := range disabled {
				ids = append(ids, item)
			}
			sort.Strings(ids)
			data, marshalErr := json.Marshal(disabledState{Disabled: ids})
			if marshalErr != nil {
				return marshalErr
			}
			temporary := r.statePath + ".new"
			if writeErr := os.WriteFile(temporary, data, 0o600); writeErr != nil {
				return writeErr
			}
			if renameErr := os.Rename(temporary, r.statePath); renameErr != nil {
				return renameErr
			}
		}
	}
	r.Rescan()
	return nil
}

type request struct {
	Protocol string            `json:"protocol"`
	Method   string            `json:"method"`
	Action   string            `json:"action"`
	Params   map[string]string `json:"params,omitempty"`
	Confirm  bool              `json:"confirm"`
	StateDir string            `json:"stateDir,omitempty"`
}

type response struct {
	Output string          `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// ActionResult is the structured response of an alx/v1 plugin action. Output
// remains for existing plugins and concise human-facing task summaries.
type ActionResult struct {
	Output string          `json:"output,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Run forwards a web UI request to the plugin executor using the stable JSON
// stdin/stdout contract. The plugin receives no shell and no inherited user
// input. The executor itself whitelists the action names it supports and
// validates every parameter; there is no manifest-level action declaration
// anymore. "confirmed" is accepted for API compatibility but the web UI owns
// its own confirmation UX.
func (r *Registry) Run(id, actionID string, params map[string]string, confirmed bool) (string, error) {
	result, err := r.RunResultWithProgress(id, actionID, params, confirmed, nil)
	return result.Output, err
}

// RunWithProgress executes a plugin while forwarding its optional stderr
// progress frames. Unrecognised stderr remains diagnostic output on failure.
func (r *Registry) RunWithProgress(id, actionID string, params map[string]string, confirmed bool, progress func(Progress)) (string, error) {
	result, err := r.RunResultWithProgress(id, actionID, params, confirmed, progress)
	return result.Output, err
}

// RunResultWithProgress keeps stdout reserved for the single plugin response
// while exposing optional structured data to the authenticated web UI.
func (r *Registry) RunResultWithProgress(id, actionID string, params map[string]string, confirmed bool, progress func(Progress)) (ActionResult, error) {
	plugin, err := r.Find(id)
	if err != nil {
		return ActionResult{}, err
	}
	if plugin.Online {
		return ActionResult{}, errors.New("在线系统插件尚未安装，不能执行远程代码")
	}
	if !plugin.Enabled {
		return ActionResult{}, errors.New("该 Setup 插件已停用")
	}
	if !plugin.Runnable {
		return ActionResult{}, errors.New("该 Setup 插件缺少可用的执行器")
	}
	entry, err := plugin.entryPath()
	if err != nil {
		return ActionResult{}, err
	}
	if plugin.RequiresElevation(actionID) && !confirmed {
		return ActionResult{}, errors.New("此操作需要在界面确认后才会请求系统管理员权限")
	}
	if plugin.RequiresElevation(actionID) {
		return ActionResult{}, errors.New("系统权限操作必须经宿主权限代理执行")
	}
	requestPayload := request{Protocol: "alx/v1", Method: "run", Action: actionID, Params: params, Confirm: confirmed}
	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return ActionResult{}, err
	}
	command := exec.Command(entry.name, entry.args...)
	command.Dir = plugin.Source
	command.Stdin = strings.NewReader(string(payload))
	stdout, err := command.StdoutPipe()
	if err != nil {
		return ActionResult{}, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return ActionResult{}, err
	}
	if err := command.Start(); err != nil {
		return ActionResult{}, err
	}
	var stderrText strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 1024), 256<<10)
		for scanner.Scan() {
			line := scanner.Text()
			if event, ok := parseProgress(line); ok && progress != nil {
				progress(event)
				continue
			}
			stderrText.WriteString(line + "\n")
		}
	}()
	output, readErr := io.ReadAll(stdout)
	err = command.Wait()
	<-done
	if readErr != nil {
		return ActionResult{}, readErr
	}
	if err != nil {
		message := strings.TrimSpace(stderrText.String())
		if message == "" {
			message = strings.TrimSpace(string(output))
		}
		if errors.Is(err, exec.ErrNotFound) {
			return ActionResult{}, fmt.Errorf("插件入口无法启动：未检测到 %s。请先安装插件所需运行环境后重试", entry.name)
		}
		if message == "" {
			message = err.Error()
		}
		return ActionResult{}, fmt.Errorf("插件执行失败：%s", message)
	}
	return parseActionResult(output, nil)
}

// RunPrivilegedApproved is intentionally separate from RunResultWithProgress:
// only the host broker may choose the policy action and the runner action. A
// downloaded manifest can describe a button but can never elevate itself.
func (r *Registry) RunPrivilegedApproved(id, policyAction, runnerAction string, params map[string]string, progress func(Progress)) (ActionResult, error) {
	plugin, err := r.Find(id)
	if err != nil {
		return ActionResult{}, err
	}
	if plugin.Online || !plugin.Enabled || !plugin.Runnable {
		return ActionResult{}, errors.New("系统插件未安装、已停用或没有可用执行器")
	}
	if !plugin.RequiresElevation(policyAction) {
		return ActionResult{}, errors.New("该操作未被插件声明为系统变更")
	}
	entry, err := plugin.entryPath()
	if err != nil {
		return ActionResult{}, err
	}
	if err := system.AuthorizePluginPrivilege(system.PluginPrivilegeIdentity{PluginID: plugin.ID, Action: policyAction, Tag: plugin.InstalledTag, Asset: plugin.InstalledAsset, ArchiveSHA256: plugin.ArchiveSHA256, RunnerPath: entry.name, RunnerArgs: entry.args, DeclaredActions: plugin.Permissions.ElevatedActions}); err != nil {
		return ActionResult{}, err
	}
	payload, err := json.Marshal(request{Protocol: "alx/v1", Method: "run", Action: runnerAction, Params: params, Confirm: true})
	if err != nil {
		return ActionResult{}, err
	}
	output, runErr := system.RunWithPrivilegesInput(plugin.Source, entry.name, entry.args, payload)
	return parseActionResult(output, runErr)
}

// RunPrivilegedApprovedWithPassword is the macOS variant of the host broker.
// It intentionally remains separate so only a preflight-selected, policy-bound
// operation can receive a one-time password.
func (r *Registry) RunPrivilegedApprovedWithPassword(id, policyAction, runnerAction string, params map[string]string, password []byte) (ActionResult, error) {
	plugin, err := r.Find(id)
	if err != nil {
		return ActionResult{}, err
	}
	if plugin.Online || !plugin.Enabled || !plugin.Runnable || !plugin.RequiresElevation(policyAction) {
		return ActionResult{}, errors.New("当前插件没有可用的受控系统权限操作")
	}
	entry, err := plugin.entryPath()
	if err != nil {
		return ActionResult{}, err
	}
	identity := system.PluginPrivilegeIdentity{PluginID: plugin.ID, Action: policyAction, Tag: plugin.InstalledTag, Asset: plugin.InstalledAsset, ArchiveSHA256: plugin.ArchiveSHA256, RunnerPath: entry.name, RunnerArgs: entry.args, DeclaredActions: plugin.Permissions.ElevatedActions}
	if err := system.AuthorizePluginPrivilege(identity); err != nil {
		return ActionResult{}, err
	}
	payload, err := json.Marshal(request{Protocol: "alx/v1", Method: "run", Action: runnerAction, Params: params, Confirm: true})
	if err != nil {
		return ActionResult{}, err
	}
	output, runErr := system.RunWithSudoInput(plugin.Source, entry.name, entry.args, payload, password)
	return parseActionResult(output, runErr)
}

// PrivilegeAvailability exposes only a safe readiness result for the host UI.
// It lets a plugin show preview-only mode before a user reaches an OS prompt.
func (r *Registry) PrivilegeAvailability(id, action string) error {
	plugin, err := r.Find(id)
	if err != nil {
		return err
	}
	if plugin.Online || !plugin.Enabled || !plugin.Runnable || !plugin.RequiresElevation(action) {
		return errors.New("当前插件没有可用的受控系统权限操作")
	}
	entry, err := plugin.entryPath()
	if err != nil {
		return err
	}
	return system.CheckPluginPrivilege(system.PluginPrivilegeIdentity{PluginID: plugin.ID, Action: action, Tag: plugin.InstalledTag, Asset: plugin.InstalledAsset, ArchiveSHA256: plugin.ArchiveSHA256, RunnerPath: entry.name, RunnerArgs: entry.args, DeclaredActions: plugin.Permissions.ElevatedActions})
}

func parseActionResult(output []byte, runErr error) (ActionResult, error) {
	var result response
	if err := json.Unmarshal(output, &result); err != nil {
		if runErr != nil {
			return ActionResult{}, runErr
		}
		return ActionResult{}, errors.New("插件返回格式无效；请使用 alx/v1 JSON 协议")
	}
	action := ActionResult{Output: result.Output, Data: result.Data}
	if result.Error != "" {
		return action, errors.New(result.Error)
	}
	if runErr != nil {
		return action, runErr
	}
	if action.Output == "" {
		action.Output = "插件操作已完成。"
	}
	return action, nil
}

func parseProgress(line string) (Progress, bool) {
	const prefix = "@alx-progress "
	if !strings.HasPrefix(line, prefix) {
		return Progress{}, false
	}
	var event Progress
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &event); err != nil || event.Percent < 0 || event.Percent > 100 || event.Stage == "" {
		return Progress{}, false
	}
	return event, true
}

type executable struct {
	name string
	args []string
}

func (p Plugin) entryPath() (executable, error) {
	entry, err := p.runtimeEntry(p.Runtime, p.Entry)
	if err == nil {
		return entry, nil
	}
	if p.Development != nil {
		if fallback, fallbackErr := p.runtimeEntry(p.Development.Runtime, p.Development.Entry); fallbackErr == nil {
			return fallback, nil
		}
	}
	return executable{}, err
}

func (p Plugin) runtimeEntry(runtimeName string, entries map[string]string) (executable, error) {
	if runtimeName == "" {
		runtimeName = "binary"
	}
	if runtimeName == "go" {
		relative := entries["go"]
		if relative == "" || filepath.IsAbs(relative) || strings.HasPrefix(filepath.Clean(relative), ".."+string(filepath.Separator)) {
			return executable{}, errors.New("Go 插件缺少位于插件目录内的入口文件")
		}
		return executable{name: "go", args: []string{"run", filepath.Join(p.Source, relative)}}, nil
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	relative := entries[key]
	if relative == "" {
		relative = entries[runtime.GOOS]
	}
	if relative == "" {
		return executable{}, errors.New("此插件没有适用于当前系统的执行器")
	}
	if filepath.IsAbs(relative) || strings.HasPrefix(filepath.Clean(relative), ".."+string(filepath.Separator)) {
		return executable{}, errors.New("插件执行器路径必须位于插件目录内")
	}
	path := filepath.Join(p.Source, relative)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return executable{}, errors.New("插件执行器不可用")
	}
	if runtimeName == "node" {
		return executable{name: "node", args: []string{path}}, nil
	}
	return executable{name: path}, nil
}

// WebRoot resolves the plugin's static web directory and verifies it stays
// inside the plugin directory even if intermediate components are symlinks.
func (p Plugin) WebRoot() (string, error) {
	if p.Web == nil || strings.TrimSpace(p.Web.Root) == "" {
		return "", errors.New("此插件未提供静态 Web 界面")
	}
	root := filepath.Join(p.Source, filepath.FromSlash(p.Web.Root))
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("插件 Web 目录不存在或不可访问")
	}
	sourceResolved, err := filepath.EvalSymlinks(p.Source)
	if err != nil {
		return "", errors.New("插件目录不可访问")
	}
	rel, err := filepath.Rel(sourceResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("插件 Web 目录越界")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("插件 Web 目录不可用")
	}
	return resolved, nil
}

func load(directory string) (Plugin, error) {
	path := filepath.Join(directory, manifestName)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxManifestSize {
		return Plugin{}, errors.New("invalid setup plugin manifest")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Plugin{}, err
	}
	plugin, err := decodeManifest(data, directory)
	if err != nil {
		return Plugin{}, err
	}
	metadataPath := filepath.Join(directory, installMetadataName)
	metadataInfo, metadataErr := os.Lstat(metadataPath)
	if metadataErr == nil && metadataInfo.Mode().IsRegular() && metadataInfo.Size() <= maxManifestSize {
		metadataBytes, readErr := os.ReadFile(metadataPath)
		var metadata installMetadata
		if readErr == nil && json.Unmarshal(metadataBytes, &metadata) == nil && metadata.ID == plugin.ID && validReleaseTag(metadata.Tag) && len(metadata.ArchiveSHA256) == 64 && metadata.Fingerprint == installFingerprint(metadata.ID, metadata.Tag, metadata.Asset, metadata.ArchiveSHA256) {
			plugin.Version = strings.TrimPrefix(metadata.Tag, "v")
			plugin.InstalledTag = metadata.Tag
			plugin.InstalledAsset = metadata.Asset
			plugin.ArchiveSHA256 = metadata.ArchiveSHA256
			plugin.Fingerprint = metadata.Fingerprint
		}
	}
	return plugin, nil
}

func validReleaseTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if strings.HasPrefix(tag, "v") {
		tag = strings.TrimPrefix(tag, "v")
	}
	return tag != "" && !strings.ContainsAny(tag, "\\/\t\r\n")
}

func decodeManifest(data []byte, source string) (Plugin, error) {
	var plugin Plugin
	if err := json.Unmarshal(data, &plugin); err != nil {
		return Plugin{}, err
	}
	if !validID.MatchString(plugin.ID) || strings.TrimSpace(plugin.Name) == "" || strings.TrimSpace(plugin.Version) == "" {
		return Plugin{}, errors.New("setup plugin requires id, name and version")
	}
	if plugin.Navigation.Label == "" {
		plugin.Navigation.Label = plugin.Name
	}
	if plugin.Navigation.Icon == "" {
		plugin.Navigation.Icon = "◈"
	}
	// Web UI is the plugin's interface; a manifest without a web root is not a
	// usable setup plugin.
	if plugin.Web == nil {
		return Plugin{}, errors.New("setup plugin requires a web root")
	}
	root := filepath.ToSlash(strings.TrimSpace(plugin.Web.Root))
	if root == "" || filepath.IsAbs(root) || root == ".." {
		return Plugin{}, errors.New("setup plugin web root must be a directory inside the plugin")
	}
	// Reject any ".." path component before cleaning hides it.
	for _, component := range strings.Split(root, "/") {
		if component == ".." {
			return Plugin{}, errors.New("setup plugin web root must be a directory inside the plugin")
		}
	}
	clean := filepath.Clean(root)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return Plugin{}, errors.New("setup plugin web root must be a directory inside the plugin")
	}
	plugin.Web.Root = root
	seenElevated := map[string]bool{}
	for index, action := range plugin.Permissions.ElevatedActions {
		action = strings.TrimSpace(action)
		if action == "" || len(action) > 96 || seenElevated[action] {
			return Plugin{}, errors.New("setup plugin elevatedActions must contain unique action names")
		}
		plugin.Permissions.ElevatedActions[index] = action
		seenElevated[action] = true
	}
	seenServices := map[string]bool{}
	for index := range plugin.Services {
		service := &plugin.Services[index]
		service.ID = strings.TrimSpace(service.ID)
		service.Name = strings.TrimSpace(service.Name)
		service.Host = strings.TrimSpace(service.Host)
		if !validID.MatchString(service.ID) || service.Name == "" || seenServices[service.ID] {
			return Plugin{}, errors.New("setup plugin service requires a unique id and name")
		}
		if service.Host != "127.0.0.1" && service.Host != "localhost" || service.Port < 1 || service.Port > 65535 {
			return Plugin{}, errors.New("setup plugin service must target a valid loopback port")
		}
		for _, value := range []*string{&service.BasePath, &service.HealthPath} {
			path := strings.TrimSpace(*value)
			if path == "" {
				*value = "/"
				continue
			}
			if !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "..") {
				return Plugin{}, errors.New("setup plugin service path must stay within its loopback service")
			}
			*value = "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "/")
		}
		seenServices[service.ID] = true
	}
	if !validRuntime(plugin.Runtime) {
		return Plugin{}, errors.New("setup plugin runtime must be binary, node or go")
	}
	if plugin.Development != nil && (!validRuntime(plugin.Development.Runtime) || len(plugin.Development.Entry) == 0) {
		return Plugin{}, errors.New("setup plugin development runner is invalid")
	}
	plugin.Runnable = len(plugin.Entry) > 0 || plugin.Development != nil
	plugin.Source = source
	return plugin, nil
}

func defaultOnlineManifestURL(repository string) string {
	name := strings.TrimPrefix(repository, "https://github.com/lemonade-lab/")
	return "https://raw.githubusercontent.com/lemonade-lab/" + name + "/main/" + manifestName
}

// onlinePlugins reads the curated Apps-X index. Only repositories owned by
// lemonade-lab are accepted, so a documentation edit cannot turn discovery
// into an arbitrary URL fetch. Online manifests are deliberately read-only:
// they render in the manager but must be installed locally before execution.
func (r *Registry) onlinePlugins() []Plugin {
	if r.onlineIndexURL == "" || r.httpClient == nil || r.onlineManifestURL == nil {
		return nil
	}
	index, err := r.readOnlineFile(r.onlineIndexURL)
	if err != nil {
		return nil
	}
	items := make([]Plugin, 0)
	seen := map[string]bool{}
	for _, match := range onlineRepository.FindAllStringSubmatch(string(index), -1) {
		repository := match[1]
		manifest, err := r.readOnlineFile(r.onlineManifestURL(repository))
		if err != nil {
			continue
		}
		plugin, err := decodeManifest(manifest, repository)
		if err != nil || seen[plugin.ID] {
			continue
		}
		plugin.Online = true
		plugin.Runnable = false
		seen[plugin.ID] = true
		items = append(items, plugin)
	}
	return items
}

type ReleaseAsset struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256,omitempty"`
	Compatible bool   `json:"compatible"`
}

type Release struct {
	Tag         string         `json:"tag"`
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	PublishedAt string         `json:"publishedAt"`
	Assets      []ReleaseAsset `json:"assets"`
}

// Releases returns formal GitHub releases for an online plugin. Source trees
// and branches are intentionally not exposed as install options.
func (r *Registry) Releases(id string) ([]Release, error) {
	online := r.onlinePlugin(id)
	if online == nil || !onlineSource.MatchString(online.Source) {
		return nil, errors.New("未找到可安装的在线 Setup 插件")
	}
	url := r.releaseURL
	if url == nil {
		url = defaultReleaseURL
	}
	request, err := http.NewRequest(http.MethodGet, url(online.Source), nil)
	if err != nil {
		return nil, errors.New("插件版本地址无效")
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, errors.New("无法获取插件版本，请检查网络后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("插件版本暂时无法获取")
	}
	var data []struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&data); err != nil {
		return nil, errors.New("插件版本内容无法识别")
	}
	result := make([]Release, 0, len(data))
	for _, item := range data {
		if item.Draft || item.Prerelease || item.TagName == "" {
			continue
		}
		release := Release{Tag: item.TagName, Name: item.Name, URL: item.HTMLURL, PublishedAt: item.PublishedAt.Format(time.RFC3339)}
		for _, asset := range item.Assets {
			release.Assets = append(release.Assets, ReleaseAsset{Name: asset.Name, URL: asset.URL, Size: asset.Size, Compatible: compatibleAsset(asset.Name)})
		}
		for index := range release.Assets {
			if checksum := checksumAsset(release.Assets, release.Assets[index].Name); checksum != "" {
				release.Assets[index].SHA256 = checksum
			}
		}
		result = append(result, release)
	}
	if len(result) == 0 {
		return nil, errors.New("暂未找到可用的插件正式版本")
	}
	return result, nil
}

// Install downloads or reuses a selected formal release asset and activates it.
// It never checks out source code.
func (r *Registry) Install(id, version, assetName string) (Plugin, error) {
	if !validID.MatchString(id) {
		return Plugin{}, errors.New("无效的 Setup 插件标识")
	}
	name := id
	local, localErr := r.Find(id)
	if localErr == nil && !local.Online {
		name = filepath.Base(local.Source)
	} else {
		online := r.onlinePlugin(id)
		if online == nil {
			return Plugin{}, errors.New("未找到可安装的在线 Setup 插件")
		}
		if online.Source == "" || !onlineSource.MatchString(online.Source) {
			return Plugin{}, errors.New("在线插件仓库来源不受支持")
		}
		name = strings.TrimPrefix(online.Source, "https://github.com/lemonade-lab/")
	}
	if !validID.MatchString(name) {
		return Plugin{}, errors.New("在线插件仓库名无效")
	}
	cached, err := r.ensureCached(id, version, assetName)
	if err != nil {
		return Plugin{}, err
	}
	root, err := r.installRoot()
	if err != nil {
		return Plugin{}, err
	}
	if err := r.activateCached(root, name, cached); err != nil {
		return Plugin{}, err
	}
	_, _ = r.cleanupCache()
	r.Rescan()
	installed, err := r.Find(id)
	if err != nil {
		return Plugin{}, errors.New("插件已安装，但加载失败；请检查插件目录 " + filepath.Join(root, name))
	}
	return installed, nil
}

func (r *Registry) selectedAsset(id, version, assetName string) (ReleaseAsset, error) {
	releases, err := r.Releases(id)
	if err != nil {
		return ReleaseAsset{}, err
	}
	for _, release := range releases {
		if release.Tag != version {
			continue
		}
		for _, asset := range release.Assets {
			if asset.Name == assetName && asset.Compatible && isArchiveName(asset.Name) && strings.HasPrefix(asset.URL, "https://github.com/") {
				return asset, nil
			}
		}
		return ReleaseAsset{}, errors.New("所选插件安装包无效")
	}
	return ReleaseAsset{}, errors.New("未找到所选插件版本")
}

func (r *Registry) ensureCached(id, version, assetName string) (cacheVersion, error) {
	directory := r.cacheDirectory(id, version, assetName)
	packagePath := filepath.Join(directory, "package"+archiveSuffix(assetName))
	extractedPath := filepath.Join(directory, "extracted")
	if item, readErr := readCacheVersion(directory); readErr == nil && item.ArchiveSHA256 != "" {
		if _, packageErr := os.Stat(item.Package); packageErr == nil {
			if checksum, checksumErr := fileSHA256(item.Package); checksumErr == nil && strings.EqualFold(checksum, item.ArchiveSHA256) {
				if _, extractedErr := os.Stat(item.Extracted); extractedErr == nil && validExtractedPlugin(item.Extracted, id) {
					item.LastUsedAt = time.Now().UTC().Format(time.RFC3339Nano)
					_ = writeCacheVersion(item)
					return item, nil
				}
			}
		}
		_ = os.RemoveAll(directory)
	}
	// A verified local cache is sufficient for switching back to a downloaded
	// version; do not require the GitHub API to be reachable in that case.
	asset, err := r.selectedAsset(id, version, assetName)
	if err != nil {
		return cacheVersion{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return cacheVersion{}, errors.New("无法创建插件缓存目录：" + err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	temporaryArchive, archiveSHA256, err := r.downloadAsset(ctx, asset)
	if err != nil {
		return cacheVersion{}, err
	}
	defer os.Remove(temporaryArchive)
	if err := os.Rename(temporaryArchive, packagePath); err != nil {
		return cacheVersion{}, errors.New("保存插件缓存失败：" + err.Error())
	}
	staging, err := os.MkdirTemp(directory, ".extract-")
	if err != nil {
		return cacheVersion{}, err
	}
	defer os.RemoveAll(staging)
	if err := extractArchive(packagePath, staging); err != nil {
		return cacheVersion{}, err
	}
	source, err := locatePluginRoot(staging)
	if err != nil {
		return cacheVersion{}, err
	}
	if err := os.Rename(source, extractedPath); err != nil {
		return cacheVersion{}, errors.New("保存插件解压缓存失败：" + err.Error())
	}
	if !validExtractedPlugin(extractedPath, id) {
		return cacheVersion{}, errors.New("插件缓存缺少有效的 alx.json、Web 或执行器")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := cacheVersion{ID: id, Tag: version, Asset: asset.Name, ArchiveSHA256: archiveSHA256, Fingerprint: installFingerprint(id, version, asset.Name, archiveSHA256), Size: archiveSize(packagePath), Package: packagePath, Extracted: extractedPath, CreatedAt: now, LastUsedAt: now}
	if err := writeCacheVersion(item); err != nil {
		return cacheVersion{}, err
	}
	return item, nil
}

func archiveSuffix(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return ".tar.gz"
	}
	if strings.HasSuffix(lower, ".tgz") {
		return ".tgz"
	}
	return ".zip"
}

func archiveSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func validExtractedPlugin(directory, id string) bool {
	plugin, err := load(directory)
	if err != nil || plugin.ID != id {
		return false
	}
	if _, err := plugin.WebRoot(); err != nil {
		return false
	}
	_, err = plugin.entryPath()
	return err == nil
}

func (r *Registry) activateCached(root, name string, cached cacheVersion) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return errors.New("无法创建插件安装目录：" + err.Error())
	}
	staging, err := os.MkdirTemp(root, ".plugin-switch-")
	if err != nil {
		return errors.New("无法创建插件切换目录：" + err.Error())
	}
	defer os.RemoveAll(staging)
	if err := copyTree(cached.Extracted, staging); err != nil {
		return errors.New("准备插件版本失败：" + err.Error())
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata := installMetadata{ID: cached.ID, Tag: cached.Tag, Asset: cached.Asset, ArchiveSHA256: cached.ArchiveSHA256, Fingerprint: cached.Fingerprint, CachePath: filepath.Dir(cached.Package), InstalledAt: now, LastUsedAt: now}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, installMetadataName), append(data, '\n'), 0o600); err != nil {
		return err
	}
	target := filepath.Join(root, name)
	backup := filepath.Join(root, ".plugin-backup-"+safeCacheComponent(name))
	_ = os.RemoveAll(backup)
	hadTarget := false
	if _, err := os.Lstat(target); err == nil {
		hadTarget = true
		if err := os.Rename(target, backup); err != nil {
			return errors.New("无法备份当前插件版本：" + err.Error())
		}
	}
	if err := os.Rename(staging, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		return errors.New("切换插件版本失败：" + err.Error())
	}
	if hadTarget {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func (r *Registry) downloadAsset(ctx context.Context, asset ReleaseAsset) (string, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", "", errors.New("插件安装包地址无效")
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return "", "", errors.New("下载插件安装包失败：" + err.Error())
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", errors.New("下载插件安装包失败")
	}
	file, err := os.CreateTemp("", "alx-plugin-*."+filepath.Ext(asset.Name))
	if err != nil {
		return "", "", err
	}
	path := file.Name()
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	limit := io.LimitReader(io.TeeReader(response.Body, hash), 300<<20)
	count, copyErr := io.Copy(file, limit)
	if copyErr != nil || count > 300<<20 {
		_ = os.Remove(path)
		return "", "", errors.New("插件安装包过大或下载失败")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	if asset.SHA256 != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(got, asset.SHA256) {
			_ = os.Remove(path)
			return "", "", errors.New("插件安装包校验失败")
		}
	}
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}

func installFingerprint(id, tag, asset, archiveSHA256 string) string {
	hash := sha256.Sum256([]byte(id + "\x00" + tag + "\x00" + asset + "\x00" + archiveSHA256))
	return hex.EncodeToString(hash[:])
}

func compatibleAsset(name string) bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}) {
		tokens[token] = true
	}
	system := (runtime.GOOS == "darwin" && (tokens["darwin"] || tokens["macos"] || tokens["mac"])) ||
		(runtime.GOOS == "windows" && (tokens["windows"] || tokens["win32"])) ||
		(runtime.GOOS == "linux" && tokens["linux"])
	architecture := (runtime.GOARCH == "arm64" && (tokens["arm64"] || tokens["aarch64"])) ||
		(runtime.GOARCH == "amd64" && (tokens["amd64"] || tokens["x64"] || tokens["x86_64"]))
	return system && architecture
}

func defaultReleaseURL(repository string) string {
	name := strings.TrimPrefix(repository, "https://github.com/lemonade-lab/")
	return "https://api.github.com/repos/lemonade-lab/" + name + "/releases?per_page=30"
}

func isArchiveName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

func checksumAsset(assets []ReleaseAsset, name string) string {
	for _, asset := range assets {
		upper := strings.ToUpper(asset.Name)
		if upper != "SHA256SUMS" && upper != "SHA256SUMS.TXT" && upper != "CHECKSUMS.TXT" {
			continue
		}
		// Checksums are fetched by Releases' caller only when a release exposes
		// them. The manifest response itself remains bounded and read-only.
		request, err := http.NewRequest(http.MethodGet, asset.URL, nil)
		if err != nil {
			continue
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(strings.TrimPrefix(fields[1], "*"), name) && len(fields[0]) == 64 {
				return strings.ToLower(fields[0])
			}
		}
	}
	return ""
}

const maxExtractedPluginSize int64 = 500 << 20

func extractArchive(source, destination string) error {
	lower := strings.ToLower(source)
	if strings.HasSuffix(lower, ".zip") {
		return extractZip(source, destination)
	}
	return extractTarGz(source, destination)
}

func safeArchivePath(root, name string) (string, error) {
	name = filepath.Clean(filepath.FromSlash(name))
	if name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", errors.New("插件安装包包含非法路径")
	}
	target := filepath.Join(root, name)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("插件安装包路径越界")
	}
	return target, nil
}

func extractZip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return errors.New("插件压缩包无法读取")
	}
	defer archive.Close()
	var total int64
	for _, entry := range archive.File {
		target, err := safeArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("插件安装包不允许包含符号链接")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if !entry.FileInfo().Mode().IsRegular() {
			return errors.New("插件安装包包含不支持的文件类型")
		}
		total += int64(entry.UncompressedSize64)
		if total > maxExtractedPluginSize {
			return errors.New("插件解压内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode().Perm()|0600)
		if err == nil {
			_, err = io.Copy(output, io.LimitReader(input, maxExtractedPluginSize+1))
			_ = output.Close()
		}
		_ = input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return errors.New("插件压缩包无法读取")
	}
	defer input.Close()
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return errors.New("插件压缩包无法读取")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("插件压缩包无法读取")
		}
		target, err := safeArchivePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New("插件安装包包含非法文件")
			}
			total += header.Size
			if total > maxExtractedPluginSize {
				return errors.New("插件解压内容过大")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0777|0600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, io.LimitReader(reader, maxExtractedPluginSize+1))
			_ = output.Close()
			if copyErr != nil {
				return copyErr
			}
		default:
			return errors.New("插件安装包不允许包含链接或特殊文件")
		}
	}
	return nil
}

func locatePluginRoot(staging string) (string, error) {
	if _, err := os.Stat(filepath.Join(staging, manifestName)); err == nil {
		return staging, nil
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		candidate := filepath.Join(staging, entries[0].Name())
		if _, err := os.Stat(filepath.Join(candidate, manifestName)); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("插件安装包中未找到根目录 alx.json")
}

// onlinePlugin returns one currently discoverable online-only plugin, falling
// back to a fresh index fetch if the cache has not been populated.
func (r *Registry) onlinePlugin(id string) *Plugin {
	for _, plugin := range r.All() {
		if plugin.Online && plugin.ID == id {
			return &plugin
		}
	}
	// Once a plugin is installed, the local entry hides the online catalogue
	// entry during compute(). Refresh the catalogue on demand so versions and
	// switching remain available for installed plugins too.
	for _, plugin := range r.onlinePlugins() {
		if plugin.ID == id {
			return &plugin
		}
	}
	return nil
}

// installRoot picks where an online plugin release is unpacked. The user-level root (where
// enable state is stored) is preferred when it is one of the scan roots, so an
// install lands in a directory alx owns; otherwise the first root a directory
// can be created in is used.
func (r *Registry) installRoot() (string, error) {
	preferred := ""
	if config, err := os.UserConfigDir(); err == nil {
		preferred = filepath.Join(config, "alx", "plugins")
	}
	hasPreferred := false
	for _, root := range r.roots {
		if root == preferred {
			hasPreferred = true
			break
		}
	}
	ordered := make([]string, 0, len(r.roots))
	if hasPreferred {
		ordered = append(ordered, preferred)
	}
	for _, root := range r.roots {
		if root == preferred {
			continue
		}
		ordered = append(ordered, root)
	}
	for _, root := range ordered {
		if err := os.MkdirAll(root, 0o755); err == nil {
			return root, nil
		}
	}
	return "", errors.New("没有可写入的插件安装目录")
}

func (r *Registry) readOnlineFile(url string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("online plugin request returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxManifestSize+1))
	if err != nil || len(data) > maxManifestSize {
		return nil, errors.New("online plugin document is unavailable")
	}
	return data, nil
}

func validRuntime(value string) bool {
	return value == "" || value == "binary" || value == "node" || value == "go"
}

func supportsCurrentPlatform(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, platform := range platforms {
		if platform == runtime.GOOS {
			return true
		}
	}
	return false
}

// Watcher keeps the plugin cache current. StartWatch retains the old polling
// entry point; StartFSWatch adds filesystem notifications with a low-frequency
// fingerprint check as a safety net for dropped filesystem events.
type Watcher struct {
	registry *Registry
	stop     chan struct{}
	done     chan struct{}
	fs       *fsnotify.Watcher
}

// StartWatch begins polling at interval. Call Stop to end it. Interval 0
// disables the poller (the cache is still filled by the first List/All call).
func (r *Registry) StartWatch(interval time.Duration) *Watcher {
	r.ensureLoaded()
	watcher := &Watcher{registry: r, stop: make(chan struct{}), done: make(chan struct{})}
	if interval <= 0 {
		close(watcher.done)
		return watcher
	}
	go watcher.loop(interval)
	return watcher
}

// StartFSWatch uses filesystem events for immediate plugin refreshes. The
// fallback fingerprint scan should be deliberately infrequent (60 seconds in
// production) and only covers platforms/filesystems that miss notifications.
func (r *Registry) StartFSWatch(fallback time.Duration) (*Watcher, error) {
	r.ensureLoaded()
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{registry: r, stop: make(chan struct{}), done: make(chan struct{}), fs: fsWatcher}
	if err := w.refreshWatches(); err != nil {
		_ = fsWatcher.Close()
		return nil, err
	}
	go w.fsLoop(fallback)
	return w, nil
}

func (w *Watcher) loop(interval time.Duration) {
	defer close(w.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			if w.registry.fingerprintChanged() {
				w.registry.Rescan()
			}
		}
	}
}

func (w *Watcher) fsLoop(fallback time.Duration) {
	defer close(w.done)
	defer w.fs.Close()
	var fallbackTick <-chan time.Time
	var ticker *time.Ticker
	if fallback > 0 {
		ticker = time.NewTicker(fallback)
		defer ticker.Stop()
		fallbackTick = ticker.C
	}
	var debounce *time.Timer
	var debounceC <-chan time.Time
	queueRescan := func() {
		if debounce == nil {
			debounce = time.NewTimer(150 * time.Millisecond)
			debounceC = debounce.C
			return
		}
		if !debounce.Stop() {
			select {
			case <-debounce.C:
			default:
			}
		}
		debounce.Reset(150 * time.Millisecond)
	}
	for {
		select {
		case <-w.stop:
			if debounce != nil {
				debounce.Stop()
			}
			return
		case event, ok := <-w.fs.Events:
			if !ok {
				return
			}
			// Roots and immediate children are watched. A directory rename/create
			// changes that watch set; a regular file change only needs a rescan.
			if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				_ = w.refreshWatches()
			}
			queueRescan()
		case <-w.fs.Errors:
			// The fallback scan repairs any missed event; keep the watcher alive.
		case <-debounceC:
			debounceC = nil
			w.registry.Rescan()
		case <-fallbackTick:
			if w.registry.fingerprintChanged() {
				w.registry.Rescan()
			}
		}
	}
}

func (w *Watcher) refreshWatches() error {
	for _, root := range w.registry.roots {
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		if err := w.fs.Add(root); err != nil && !errors.Is(err, fsnotify.ErrEventOverflow) {
			return err
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			_ = w.fs.Add(filepath.Join(root, entry.Name()))
		}
	}
	return nil
}

// Stop ends the polling goroutine and waits for it to exit.
func (w *Watcher) Stop() {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	<-w.done
}

// fingerprintChanged stats the manifest files and directory listings. It
// returns true when the previous fingerprint differs.
func (r *Registry) fingerprintChanged() bool {
	next := r.fingerprint()
	r.mu.Lock()
	defer r.mu.Unlock()
	if next != r.lastFingerprint {
		r.lastFingerprint = next
		return true
	}
	return false
}

func (r *Registry) fingerprint() string {
	var builder strings.Builder
	for _, root := range r.roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			builder.WriteString(root)
			builder.WriteByte(0)
			builder.WriteString(entry.Name())
			builder.WriteByte(0)
			if info, statErr := os.Stat(filepath.Join(root, entry.Name(), manifestName)); statErr == nil {
				builder.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
				builder.WriteByte(':')
				builder.WriteString(strconv.FormatInt(info.Size(), 10))
			}
			builder.WriteByte('\n')
		}
	}
	if r.statePath != "" {
		if info, statErr := os.Stat(r.statePath); statErr == nil {
			builder.WriteString("state:")
			builder.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
		}
	}
	return builder.String()
}
