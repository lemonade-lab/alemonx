package robot

import (
	"alemonx/internal/systemnetwork"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// BotAppPage is a web UI contributed by an AlemonJS plugin belonging to the
// selected robot. A desktop.sidebar is used only as its registration point;
// setup never runs its desktop command.
type BotAppPage struct {
	ID                 string `json:"id"`
	Package            string `json:"package"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	Logo               string `json:"logo,omitempty"`
	RequiresServerPort bool   `json:"requiresServerPort,omitempty"`
}

type appPageManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Alemonjs    struct {
		Web struct {
			Root       string `json:"root"`
			ServerPort bool   `json:"serverPort"`
		} `json:"web"`
		Desktop struct {
			Logo     string `json:"logo"`
			Sidebars []struct {
				Name string `json:"name"`
			} `json:"sidebars"`
		} `json:"desktop"`
	} `json:"alemonjs"`
}

type resolvedAppPage struct {
	BotAppPage
	root string
}

func (m Manager) AppPages(root string) ([]BotAppPage, error) {
	items, err := resolveAppPages(root)
	if err != nil {
		return nil, err
	}
	result := make([]BotAppPage, len(items))
	for i, item := range items {
		result[i] = item.BotAppPage
	}
	return result, nil
}

// AppPageFile resolves only files inside the registered web.root directory.
// It rejects traversal and symlink escapes before the web handler serves data.
func (m Manager) AppPageFile(root, id, requestPath string) (string, error) {
	items, err := resolveAppPages(root)
	if err != nil {
		return "", err
	}
	var entry *resolvedAppPage
	for i := range items {
		if items[i].ID == id {
			entry = &items[i]
			break
		}
	}
	if entry == nil {
		return "", errors.New("未找到该机器人插件 Web 页面")
	}
	requestPath = strings.TrimPrefix(filepath.ToSlash(requestPath), "/")
	if requestPath == "" {
		requestPath = "index.html"
	}
	clean := filepath.Clean(requestPath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("插件 Web 资源路径无效")
	}
	candidate := filepath.Join(entry.root, clean)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		// A SPA route has no file extension. Serve its entry document only.
		if filepath.Ext(clean) == "" {
			resolved = filepath.Join(entry.root, "index.html")
		} else {
			return "", errors.New("插件 Web 资源不存在")
		}
	}
	rootResolved, err := filepath.EvalSymlinks(entry.root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("插件 Web 资源路径无效")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("插件 Web 资源不存在")
	}
	return resolved, nil
}

func resolveAppPages(root string) ([]resolvedAppPage, error) {
	project, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	candidates := pluginManifestPaths(project)
	items := []resolvedAppPage{}
	seen := map[string]bool{}
	for _, manifestPath := range candidates {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest appPageManifest
		if json.Unmarshal(data, &manifest) != nil || manifest.Name == "" {
			continue
		}
		webRoot := strings.TrimSpace(manifest.Alemonjs.Web.Root)
		if webRoot == "" || len(manifest.Alemonjs.Desktop.Sidebars) == 0 {
			continue
		}
		packageDir := filepath.Dir(manifestPath)
		if filepath.IsAbs(webRoot) {
			continue
		}
		absoluteRoot := filepath.Join(packageDir, filepath.Clean(webRoot))
		resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
		if err != nil {
			continue
		}
		packageResolved, err := filepath.EvalSymlinks(packageDir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(packageResolved, resolvedRoot)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(filepath.Join(resolvedRoot, "index.html")); err != nil || !info.Mode().IsRegular() {
			continue
		}
		for _, sidebar := range manifest.Alemonjs.Desktop.Sidebars {
			label := strings.TrimSpace(sidebar.Name)
			if label == "" {
				continue
			}
			id := appPageID(manifest.Name, label, packageResolved)
			if seen[id] {
				continue
			}
			seen[id] = true
			items = append(items, resolvedAppPage{BotAppPage: BotAppPage{ID: id, Package: manifest.Name, Name: label, Description: manifest.Description, Logo: manifest.Alemonjs.Desktop.Logo, RequiresServerPort: manifest.Alemonjs.Web.ServerPort}, root: resolvedRoot})
		}
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

func pluginManifestPaths(project string) []string {
	paths := []string{}
	if entries, err := os.ReadDir(filepath.Join(project, "packages")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, filepath.Join(project, "packages", entry.Name(), "package.json"))
				// Local packages may use the standard npm scope layout:
				// packages/@scope/package. Keep the scan deliberately shallow so
				// arbitrary nested directories never become 应用页 candidates.
				if strings.HasPrefix(entry.Name(), "@") {
					if scoped, readErr := os.ReadDir(filepath.Join(project, "packages", entry.Name())); readErr == nil {
						for _, child := range scoped {
							if child.IsDir() {
								paths = append(paths, filepath.Join(project, "packages", entry.Name(), child.Name(), "package.json"))
							}
						}
					}
				}
			}
		}
	}
	// Only packages explicitly declared by this robot are considered. This
	// avoids exposing arbitrary transitive npm dependencies as applications.
	data, err := os.ReadFile(filepath.Join(project, "package.json"))
	if err == nil {
		var manifest struct {
			Dependencies         map[string]string `json:"dependencies"`
			DevDependencies      map[string]string `json:"devDependencies"`
			OptionalDependencies map[string]string `json:"optionalDependencies"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			declared := map[string]bool{}
			for _, group := range []map[string]string{manifest.Dependencies, manifest.DevDependencies, manifest.OptionalDependencies} {
				for name := range group {
					declared[name] = true
				}
			}
			for name := range declared {
				paths = append(paths, filepath.Join(project, "node_modules", filepath.FromSlash(name), "package.json"))
			}
		}
	}
	return paths
}

func appPageID(pkg, name, directory string) string {
	sum := sha256.Sum256([]byte(pkg + "\x00" + name + "\x00" + directory))
	return hex.EncodeToString(sum[:8])
}

func (m Manager) BotAppPage(root, id string) (BotAppPage, error) {
	items, err := resolveAppPages(root)
	if err != nil {
		return BotAppPage{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item.BotAppPage, nil
		}
	}
	return BotAppPage{}, fmt.Errorf("未找到该机器人应用页")
}

// defaultAppPort is the AlemonJS application port used when alemon.config.yaml
// declares no serverPort.
const defaultAppPort = 18110

// AppPortInfo describes the robot's application port: the configured value
// (serverPort in alemon.config.yaml) and whether it was explicitly declared.
type AppPortInfo struct {
	Port           int    `json:"port"`
	ConfiguredPort int    `json:"configuredPort"`
	ActualPort     int    `json:"actualPort"`
	Configured     bool   `json:"configured"`
	Drifted        bool   `json:"drifted"`
	Source         string `json:"source,omitempty"`
}

// AutoPortEnabled reports whether AlemonJS may advance a requested port when
// it is occupied. The configured port remains the durable user preference.
func (Manager) AutoPortEnabled(root string) (bool, error) {
	project, err := projectPath(root)
	if err != nil {
		return false, err
	}
	data, err := readRuntimeConfigFile(filepath.Join(project, "alemon.config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("无法解析运行配置：%w", err)
	}
	switch value := config["autoPort"].(type) {
	case bool:
		return value, nil
	case string:
		enabled, parseErr := strconv.ParseBool(strings.TrimSpace(value))
		return enabled && parseErr == nil, nil
	default:
		return false, nil
	}
}

// AppPort returns the robot's application port from alemon.config.yaml
// (serverPort) and whether it was explicitly configured.
func (Manager) AppPort(root string) (AppPortInfo, error) {
	project, err := projectPath(root)
	if err != nil {
		return AppPortInfo{}, err
	}
	data, err := readRuntimeConfigFile(filepath.Join(project, "alemon.config.yaml"))
	if err != nil {
		return AppPortInfo{Port: defaultAppPort}, nil
	}
	if match := regexp.MustCompile(`(?m)^serverPort\s*:\s*['\"]?(\d+)`).FindStringSubmatch(string(data)); len(match) == 2 {
		if configured, parseErr := strconv.Atoi(match[1]); parseErr == nil && configured > 0 && configured < 65536 {
			return AppPortInfo{Port: configured, ConfiguredPort: configured, ActualPort: configured, Configured: true, Source: "config"}, nil
		}
	}
	return AppPortInfo{Port: defaultAppPort, ConfiguredPort: defaultAppPort, ActualPort: defaultAppPort, Source: "default"}, nil
}

// SaveAppPort writes serverPort into alemon.config.yaml, replacing an existing
// value or appending a new one.
func (m Manager) SaveAppPort(root string, port int) (Result, error) {
	if port < 1 || port > 65535 {
		return Result{}, errors.New("应用端口应在 1-65535 之间")
	}
	result, err := m.UpdateRuntimeConfig(root, "", func(content string) (string, error) {
		return setTopLevelScalar(content, "serverPort", strconv.Itoa(port)), nil
	})
	if err != nil {
		return Result{}, err
	}
	result.Output = "应用端口已设置为 " + strconv.Itoa(port) + "。"
	return result, nil
}

// EnabledApps returns enabled packages in AlemonJS loading order.
func (Manager) EnabledApps(root string) ([]string, error) {
	project, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	content, err := readRuntimeConfigFile(filepath.Join(project, "alemon.config.yaml"))
	if err != nil {
		return []string{}, nil
	}
	_, apps, err := readAppsConfig(string(content))
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(apps))
	for _, item := range orderedApps(apps) {
		if item.enabled {
			result = append(result, item.name)
		}
	}
	return result, nil
}

type orderedApp struct {
	name    string
	order   float64
	enabled bool
	index   int
}

func orderedApps(apps yaml.MapSlice) []orderedApp {
	ordered := make([]orderedApp, 0, len(apps))
	seen := make(map[string]bool, len(apps))
	for index, item := range apps {
		name, _ := item.Key.(string)
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		order := float64(index)
		enabled := false
		switch value := item.Value.(type) {
		case bool:
			enabled = value
		case yaml.MapSlice:
			enabled = true
			valid := true
			for _, option := range value {
				switch option.Key {
				case "enabled":
					enabled, valid = option.Value.(bool)
				case "order":
					order, valid = appOrder(option.Value)
				}
				if !valid {
					break
				}
			}
			if !valid {
				continue
			}
		default:
			continue
		}
		seen[name] = true
		ordered = append(ordered, orderedApp{name, order, enabled, index})
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].order < ordered[j].order })
	return ordered
}

func (m Manager) MoveApp(root, source, target string, after bool) (Result, error) {
	if source == "" || target == "" || source == target {
		return Result{}, errors.New("请选择两个不同的已启用插件。")
	}
	return m.UpdateRuntimeConfig(root, "", func(content string) (string, error) {
		return updateAppsConfig(content, func(apps yaml.MapSlice) (yaml.MapSlice, error) {
			ordered := orderedApps(apps)
			from, to := -1, -1
			for index, item := range ordered {
				if item.enabled && item.name == source {
					from = index
				}
				if item.enabled && item.name == target {
					to = index
				}
			}
			if from < 0 || to < 0 {
				return nil, errors.New("插件启用状态已变化，请刷新背包后重试。")
			}
			if after {
				to++
			}
			if from < to {
				to--
			}
			if from == to {
				return apps, nil
			}
			moved := ordered[from]
			ordered = slices.Delete(ordered, from, from+1)
			ordered = slices.Insert(ordered, to, moved)
			for index, item := range ordered {
				entry := &apps[item.index]
				options, ok := entry.Value.(yaml.MapSlice)
				if !ok {
					options = yaml.MapSlice{{Key: "enabled", Value: item.enabled}}
				}
				entry.Value = setAppOption(options, "order", index)
			}
			return apps, nil
		})
	})
}

func setAppOption(options yaml.MapSlice, key string, value any) yaml.MapSlice {
	for index := range options {
		if options[index].Key == key {
			options[index].Value = value
			return options
		}
	}
	return append(options, yaml.MapItem{Key: key, Value: value})
}

func appOrder(value any) (float64, bool) {
	var order float64
	switch number := value.(type) {
	case int:
		order = float64(number)
	case int64:
		order = float64(number)
	case uint:
		order = float64(number)
	case uint64:
		order = float64(number)
	case float64:
		order = number
	default:
		return 0, false
	}
	const maxSafeInteger = 1<<53 - 1
	return order, order >= -maxSafeInteger && order <= maxSafeInteger && order == math.Trunc(order)
}

func readAppsConfig(content string) (yaml.MapSlice, yaml.MapSlice, error) {
	var config yaml.MapSlice
	if err := yaml.UnmarshalWithOptions([]byte(content), &config, yaml.UseOrderedMap()); err != nil {
		return nil, nil, fmt.Errorf("无法解析运行配置：%w", err)
	}
	for _, item := range config {
		if item.Key != "apps" {
			continue
		}
		switch values := item.Value.(type) {
		case yaml.MapSlice:
			return config, values, nil
		case []any:
			apps := make(yaml.MapSlice, 0, len(values))
			seen := make(map[string]bool, len(values))
			for _, entry := range values {
				if name, ok := entry.(string); ok && strings.TrimSpace(name) != "" && !seen[name] {
					seen[name] = true
					apps = append(apps, yaml.MapItem{Key: name, Value: true})
				}
			}
			return config, apps, nil
		}
	}
	return config, nil, nil
}

// SetAppEnabled updates the switch while preserving order and command options.
func (m Manager) SetAppEnabled(root, packageName string, enabled bool) (Result, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return Result{}, errors.New("请选择要启动的本地包")
	}
	result, err := m.UpdateRuntimeConfig(root, "", func(content string) (string, error) {
		return updateAppsConfig(content, func(apps yaml.MapSlice) (yaml.MapSlice, error) {
			for index, item := range apps {
				name, _ := item.Key.(string)
				if strings.TrimSpace(name) != packageName {
					continue
				}
				if options, ok := item.Value.(yaml.MapSlice); ok {
					apps[index].Value = setAppOption(options, "enabled", enabled)
				} else {
					apps[index].Value = enabled
				}
				return apps, nil
			}
			return append(apps, yaml.MapItem{Key: packageName, Value: enabled}), nil
		})
	})
	if err != nil {
		return Result{}, err
	}
	state := "已停用"
	if enabled {
		state = "已启用"
	}
	result.Output = packageName + " " + state + "。"
	return result, nil
}

// ValidateEnabledBackpackRelease is retained for runtime-entry compatibility.
// Running a backpack plugin only needs the package code already on disk; Git
// provenance belongs to optional version, sync, and recovery operations.
func (m Manager) ValidateEnabledBackpackRelease(root string) error {
	_, err := m.EnabledApps(root)
	return err
}

// isGitBackpackPackage is deliberately narrower than releaseGitStatus: a
// package's local working tree is the code that will run, so only its Git
// checkout and branch determine whether it is eligible to run.
func isGitBackpackPackage(path string) bool {
	inside, err := gitRun(path, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return false
	}
	branch, err := gitRun(path, "symbolic-ref", "--quiet", "--short", "HEAD")
	return err == nil && isGitBranch(branch)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// updateAppsConfig rewrites only the apps section of a block-style config.
func updateAppsConfig(content string, update func(yaml.MapSlice) (yaml.MapSlice, error)) (string, error) {
	content = normalizeRuntimeConfigYAML(content)
	config, apps, err := readAppsConfig(content)
	if err != nil {
		return "", err
	}
	apps, err = update(apps)
	if err != nil {
		return "", err
	}
	if apps == nil {
		apps = yaml.MapSlice{}
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	start, end := -1, len(lines)
	pattern := regexp.MustCompile(`^(?:apps|"apps"|'apps')[ \t]*:`)
	for index, line := range lines {
		if start < 0 {
			if pattern.MatchString(line) {
				start = index
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed == line && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "- ") {
			end = index
			break
		}
	}
	for end > start+1 && (strings.TrimSpace(lines[end-1]) == "" || strings.HasPrefix(lines[end-1], "#")) {
		end--
	}
	section := yaml.MapSlice{{Key: "apps", Value: apps}}
	if start < 0 && strings.HasPrefix(strings.TrimSpace(content), "{") {
		for index := range config {
			if config[index].Key == "apps" {
				config[index].Value = apps
				data, err := yaml.Marshal(config)
				return string(data), err
			}
		}
		data, err := yaml.Marshal(append(config, section...))
		return string(data), err
	}
	data, err := yaml.Marshal(section)
	if err != nil {
		return "", err
	}
	if start < 0 {
		prefix := strings.TrimRight(content, "\n")
		if prefix != "" {
			prefix += "\n\n"
		}
		return prefix + string(data), nil
	}
	replacement := strings.TrimSuffix(string(data), "\n")
	updated := append(append(lines[:start:start], replacement), lines[end:]...)
	return strings.Join(updated, "\n") + "\n", nil
}

// normalizeEmptyYAMLRoot repairs the invalid shape produced by older writers
// when an empty YAML document (`{}`) was followed by a top-level section.
// `{}` is a complete YAML document, so it must never precede `apps:` or any
// other root key. Keeping this narrow repair lets existing broken projects
// recover on their next normal configuration update.
func normalizeEmptyYAMLRoot(content string) string {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == "{}" {
			return strings.Join(append(lines[:index], lines[index+1:]...), "\n")
		}
		break
	}
	return content
}

// normalizeRuntimeConfigYAML repairs narrowly-scoped legacy shapes before the
// generic YAML parser sees them. In particular, old app toggles were sometimes
// written without indentation directly below `apps:`, which turns a map into
// an invalid scalar sequence.
func normalizeRuntimeConfigYAML(content string) string {
	content = normalizeEmptyYAMLRoot(content)
	lines := strings.Split(content, "\n")
	inApps := false
	appEntry := regexp.MustCompile(`^[A-Za-z0-9@._/-]+\s*:\s*(?:true|false)\s*(?:#.*)?$`)
	for index, line := range lines {
		if strings.TrimSpace(line) == "apps:" && len(line) == len(strings.TrimLeft(line, " \t")) {
			inApps = true
			continue
		}
		if !inApps || strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if len(line) != len(strings.TrimLeft(line, " \t")) {
			continue
		}
		if appEntry.MatchString(line) {
			lines[index] = "  " + line
			continue
		}
		inApps = false
	}
	return strings.Join(lines, "\n")
}

// AppPortReachable probes whether the robot's application is actually serving
// on its configured port. The browser cannot reach 127.0.0.1 across the dev
// proxy/CSP, so the backend performs the health check before opening the app.
func (m Manager) AppPortReachable(root string) (bool, int, error) {
	info, err := m.AppPort(root)
	if err != nil {
		return false, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(info.Port), nil)
	if err != nil {
		return false, info.Port, err
	}
	response, err := systemnetwork.DefaultClient(0).Do(request)
	if err != nil {
		return false, info.Port, nil
	}
	defer response.Body.Close()
	// Any HTTP response (even 404/500) means a server is listening on the port.
	return true, info.Port, nil
}

// AppPageAPIURL resolves a plugin's relative ./api contract to the selected
// robot application. AlemonJS 应用页 are often not static-only: their UI
// expects the robot's Koa API on the configured application port.
func (m Manager) AppPageAPIURL(root, id, requestPath string) (string, error) {
	if _, err := m.BotAppPage(root, id); err != nil {
		return "", err
	}
	project, err := projectPath(root)
	if err != nil {
		return "", err
	}
	clean := filepath.ToSlash(filepath.Clean("/" + strings.TrimPrefix(requestPath, "/")))
	if clean == "/" || strings.HasPrefix(clean, "/../") {
		return "", errors.New("插件 API 路径无效")
	}
	info, err := m.AppPort(project)
	if err != nil {
		return "", err
	}
	if !info.Configured {
		return "", errors.New("插件 API 需要先配置应用端口（serverPort）")
	}
	return "http://127.0.0.1:" + strconv.Itoa(info.Port) + "/api" + clean, nil
}
