package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"alemonx/internal/packageschema"
)

type PackageManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Repository  string `json:"repository"`
	License     string `json:"license"`
	Private     bool   `json:"private"`
	Access      string `json:"access"`

	PackageManager    string   `json:"packageManager"`
	ModuleType        string   `json:"moduleType"`
	WorkspacesEnabled bool     `json:"workspacesEnabled"`
	Workspaces        []string `json:"workspaces"`

	AlemonjsConfig               json.RawMessage `json:"alemonjsConfig"`
	AlemonjsConfigSourceReadme   string          `json:"alemonjsConfigSourceReadme"`
	AlemonjsConfigSourceOfficial string          `json:"alemonjsConfigSourceOfficial"`
	AlemonjsConfigSourcePlatform string          `json:"alemonjsConfigSourcePlatform"`
	AlemonjsDesktopLogo          string          `json:"alemonjsDesktopLogo"`
	AlemonjsWebRoot              string          `json:"alemonjsWebRoot"`
	AlemonjsWebServerPort        bool            `json:"alemonjsWebServerPort"`
}

var npmNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)

func (Manager) PackageManifest(root string) (PackageManifest, error) {
	path, err := projectPath(root)
	if err != nil {
		return PackageManifest{}, err
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return PackageManifest{}, fmt.Errorf("无法读取 package.json：%w", err)
	}
	var source map[string]json.RawMessage
	if err := json.Unmarshal(data, &source); err != nil {
		return PackageManifest{}, errors.New("package.json 格式无法识别")
	}
	result := PackageManifest{}
	readString := func(key string) string { var value string; _ = json.Unmarshal(source[key], &value); return value }
	result.Name, result.Version, result.Description, result.Homepage, result.License = readString("name"), readString("version"), readString("description"), readString("homepage"), readString("license")
	result.PackageManager, result.ModuleType = readString("packageManager"), readString("type")
	_ = json.Unmarshal(source["private"], &result.Private)
	if !result.Private {
		var privateText string
		if json.Unmarshal(source["private"], &privateText) == nil {
			result.Private = strings.EqualFold(privateText, "true")
		}
	}
	if raw := source["repository"]; len(raw) > 0 {
		if json.Unmarshal(raw, &result.Repository) != nil {
			var repository struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(raw, &repository) == nil {
				result.Repository = repository.URL
			}
		}
	}
	if raw := source["publishConfig"]; len(raw) > 0 {
		var config struct {
			Access string `json:"access"`
		}
		if json.Unmarshal(raw, &config) == nil {
			result.Access = config.Access
		}
	}
	if raw := source["workspaces"]; len(raw) > 0 {
		var patterns []string
		if json.Unmarshal(raw, &patterns) == nil {
			result.WorkspacesEnabled, result.Workspaces = true, patterns
		} else {
			var config struct {
				Packages []string `json:"packages"`
			}
			if json.Unmarshal(raw, &config) == nil {
				result.WorkspacesEnabled, result.Workspaces = true, config.Packages
			}
		}
	}
	if raw := source["alemonjs"]; len(raw) > 0 {
		var alemonjs struct {
			Config       json.RawMessage `json:"config"`
			ConfigSource struct {
				Readme   string `json:"readme"`
				Official string `json:"official"`
				Platform string `json:"platform"`
			} `json:"config-source"`
			Desktop struct {
				Logo string `json:"logo"`
			} `json:"desktop"`
			Web struct {
				Root       string `json:"root"`
				ServerPort bool   `json:"serverPort"`
			} `json:"web"`
		}
		if json.Unmarshal(raw, &alemonjs) == nil {
			if len(alemonjs.Config) > 0 && string(alemonjs.Config) != "null" {
				result.AlemonjsConfig = append(json.RawMessage(nil), alemonjs.Config...)
			}
			result.AlemonjsConfigSourceReadme = alemonjs.ConfigSource.Readme
			result.AlemonjsConfigSourceOfficial = alemonjs.ConfigSource.Official
			result.AlemonjsConfigSourcePlatform = alemonjs.ConfigSource.Platform
			result.AlemonjsDesktopLogo = alemonjs.Desktop.Logo
			result.AlemonjsWebRoot, result.AlemonjsWebServerPort = alemonjs.Web.Root, alemonjs.Web.ServerPort
		}
	}
	return result, nil
}

func (Manager) SavePackageManifest(root string, input PackageManifest) (Result, error) {
	path, err := projectPath(root)
	if err != nil {
		return Result{}, err
	}
	if err := validateManifest(input); err != nil {
		return Result{}, err
	}
	file := filepath.Join(path, "package.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return Result{}, fmt.Errorf("无法读取 package.json：%w", err)
	}
	var source map[string]any
	if err := json.Unmarshal(data, &source); err != nil {
		return Result{}, errors.New("package.json 格式无法识别")
	}
	source["name"], source["version"], source["description"], source["homepage"], source["license"], source["private"] = input.Name, input.Version, input.Description, input.Homepage, input.License, input.Private
	if input.Repository == "" {
		delete(source, "repository")
	} else {
		source["repository"] = input.Repository
	}
	if input.Access == "" {
		if config, ok := source["publishConfig"].(map[string]any); ok {
			delete(config, "access")
			if len(config) == 0 {
				delete(source, "publishConfig")
			}
		}
	} else {
		config, _ := source["publishConfig"].(map[string]any)
		if config == nil {
			config = map[string]any{}
		}
		config["access"] = input.Access
		source["publishConfig"] = config
	}
	if input.PackageManager == "" {
		delete(source, "packageManager")
	} else {
		source["packageManager"] = input.PackageManager
	}
	if input.ModuleType == "" {
		delete(source, "type")
	} else {
		source["type"] = input.ModuleType
	}
	if !input.WorkspacesEnabled {
		delete(source, "workspaces")
	} else if existing, ok := source["workspaces"].(map[string]any); ok {
		existing["packages"] = input.Workspaces
		source["workspaces"] = existing
	} else {
		source["workspaces"] = input.Workspaces
	}
	if err := mergeAlemonjsManifest(source, input); err != nil {
		return Result{}, err
	}
	updated, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(file, append(updated, '\n'), 0644); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("保存 package.json")
		}
		return Result{}, fmt.Errorf("无法保存 package.json：%w", err)
	}
	return Result{Path: file, Output: "发布信息已保存。"}, nil
}

func validateManifest(input PackageManifest) error {
	if !npmNamePattern.MatchString(input.Name) {
		return errors.New("包名只能使用小写字母、数字、短横线、下划线和一个可选的 @scope/")
	}
	if !isNpmVersion(input.Version) {
		return errors.New("版本号应为 1.2.3 格式")
	}
	if len(input.Description) > 512 || strings.ContainsAny(input.Description, "\r\n") {
		return errors.New("包描述应为单行且不超过 512 个字符")
	}
	if input.License != "" && (len(input.License) > 100 || strings.ContainsAny(input.License, "\r\n")) {
		return errors.New("许可证格式无效")
	}
	for _, value := range []string{input.Homepage, input.Repository} {
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return errors.New("主页和仓库地址必须是完整的 http(s) 地址")
		}
	}
	if input.Access != "" && input.Access != "public" && input.Access != "restricted" {
		return errors.New("发布权限只能选择 public 或 restricted")
	}
	if input.ModuleType != "" && input.ModuleType != "module" && input.ModuleType != "commonjs" {
		return errors.New("模块类型只能选择 module 或 commonjs")
	}
	if len(input.PackageManager) > 100 || strings.ContainsAny(input.PackageManager, "\r\n") {
		return errors.New("包管理器格式无效")
	}
	if input.WorkspacesEnabled && len(input.Workspaces) == 0 {
		return errors.New("已启用工作空间时，至少需要一个目录匹配规则")
	}
	for _, workspace := range input.Workspaces {
		workspace = strings.TrimSpace(workspace)
		if workspace == "" || len(workspace) > 200 || strings.ContainsAny(workspace, "\r\n") || filepath.IsAbs(workspace) || strings.Contains(workspace, "..") {
			return errors.New("工作空间目录规则无效")
		}
	}
	for _, value := range []string{input.AlemonjsConfigSourceReadme, input.AlemonjsConfigSourceOfficial, input.AlemonjsConfigSourcePlatform, input.AlemonjsDesktopLogo} {
		if len(value) > 512 || strings.ContainsAny(value, "\r\n") {
			return errors.New("AlemonJS 声明字段应为单行且不超过 512 个字符")
		}
	}
	if input.AlemonjsWebRoot != "" && (len(input.AlemonjsWebRoot) > 200 || strings.ContainsAny(input.AlemonjsWebRoot, "\r\n") || filepath.IsAbs(input.AlemonjsWebRoot) || strings.Contains(input.AlemonjsWebRoot, "..")) {
		return errors.New("AlemonJS Web 根目录必须是项目内的相对路径")
	}
	if len(input.AlemonjsConfig) > 0 && string(input.AlemonjsConfig) != "null" {
		var fields []packageschema.Field
		if err := json.Unmarshal(input.AlemonjsConfig, &fields); err != nil {
			return errors.New("AlemonJS 配置字段必须是 JSON 数组")
		}
		if err := (&packageschema.Declaration{Config: fields}).ValidateDeclaration(); err != nil {
			return fmt.Errorf("AlemonJS 配置字段无效：%w", err)
		}
	}
	return nil
}

func mergeAlemonjsManifest(source map[string]any, input PackageManifest) error {
	alemonjs, _ := source["alemonjs"].(map[string]any)
	if alemonjs == nil {
		alemonjs = map[string]any{}
	}
	configSource, _ := alemonjs["config-source"].(map[string]any)
	if configSource == nil {
		configSource = map[string]any{}
	}
	setManifestString(configSource, "readme", input.AlemonjsConfigSourceReadme)
	setManifestString(configSource, "official", input.AlemonjsConfigSourceOfficial)
	setManifestString(configSource, "platform", input.AlemonjsConfigSourcePlatform)
	setManifestObject(alemonjs, "config-source", configSource)

	desktop, _ := alemonjs["desktop"].(map[string]any)
	if desktop == nil {
		desktop = map[string]any{}
	}
	setManifestString(desktop, "logo", input.AlemonjsDesktopLogo)
	setManifestObject(alemonjs, "desktop", desktop)

	web, _ := alemonjs["web"].(map[string]any)
	if web == nil {
		web = map[string]any{}
	}
	setManifestString(web, "root", input.AlemonjsWebRoot)
	if input.AlemonjsWebServerPort {
		web["serverPort"] = true
	} else {
		delete(web, "serverPort")
	}
	setManifestObject(alemonjs, "web", web)

	if len(input.AlemonjsConfig) == 0 || string(input.AlemonjsConfig) == "null" || string(input.AlemonjsConfig) == "[]" {
		delete(alemonjs, "config")
	} else {
		var config any
		if err := json.Unmarshal(input.AlemonjsConfig, &config); err != nil {
			return errors.New("AlemonJS 配置字段必须是 JSON 数组")
		}
		alemonjs["config"] = config
	}
	if len(alemonjs) == 0 {
		delete(source, "alemonjs")
	} else {
		source["alemonjs"] = alemonjs
	}
	return nil
}

func setManifestString(object map[string]any, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(object, key)
		return
	}
	object[key] = value
}

func setManifestObject(object map[string]any, key string, value map[string]any) {
	if len(value) == 0 {
		delete(object, key)
		return
	}
	object[key] = value
}
