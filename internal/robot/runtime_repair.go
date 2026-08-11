package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"alemonx/internal/pm2config"
	"alemonx/internal/system"
)

// RuntimeRepairPlan is a safe, inspectable description of the changes needed
// to make a recognised AlemonJS project runnable. It intentionally never
// exposes connection values or other credentials.
type RuntimeRepairPlan struct {
	Phase                string   `json:"phase"`
	Profile              string   `json:"profile"`
	Mode                 string   `json:"mode"`
	Automatic            []string `json:"automatic"`
	RequiresConfirmation []string `json:"requiresConfirmation"`
	Blocked              []string `json:"blocked"`
	Diagnostics          []string `json:"diagnostics"`
}

type RuntimeRepairResult struct {
	RuntimeRepairPlan
	BackupPath string `json:"backupPath,omitempty"`
	Output     string `json:"output,omitempty"`
}

type runtimeRepairManifest struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

func (m Manager) RuntimeRepairPlan(root, mode string) (RuntimeRepairPlan, error) {
	path, err := projectPath(root)
	if err != nil {
		return RuntimeRepairPlan{}, err
	}
	if mode != "app" && mode != "dev" && mode != "pm2" && mode != "all" {
		return RuntimeRepairPlan{}, errors.New("修复模式无效")
	}
	manifest, err := readRuntimeRepairManifest(path)
	if err != nil {
		return RuntimeRepairPlan{}, err
	}
	plan := RuntimeRepairPlan{Phase: "planned", Mode: mode, Automatic: []string{}, RequiresConfirmation: []string{}, Blocked: []string{}, Diagnostics: []string{}}
	framework := manifest.Dependencies["alemonjs"] != "" || manifest.DevDependencies["alemonjs"] != ""
	isTS := exists(filepath.Join(path, "app.ts")) || exists(filepath.Join(path, "lvy.config.ts"))
	isJS := exists(filepath.Join(path, "app.js")) || exists(filepath.Join(path, "lvy.config.js"))
	if !framework && !isTS && !isJS {
		plan.Profile = "node"
		plan.Blocked = append(plan.Blocked, "未识别为 AlemonJS 项目；不会自动改写通用 Node.js 项目的启动脚本。")
		return plan, nil
	}
	if isTS {
		plan.Profile = "alemonjs-ts"
	} else {
		plan.Profile = "alemonjs-js"
	}
	if !framework {
		plan.Automatic = append(plan.Automatic, "补齐 AlemonJS 框架依赖")
	}
	if _, err := system.ResolveCommand("node"); err != nil {
		plan.Blocked = append(plan.Blocked, "未检测到 Node.js；请先安装 Node.js LTS。")
	}
	missing, dependencyErr := m.RuntimeDependencies(root)
	if dependencyErr != nil {
		plan.Blocked = append(plan.Blocked, dependencyErr.Error())
	} else if len(missing) > 0 {
		plan.Automatic = append(plan.Automatic, "同步缺失依赖："+strings.Join(missing, "、"))
	}
	entry := filepath.Join(path, "index.js")
	if !exists(entry) && (mode == "app" || mode == "pm2" || mode == "all") {
		plan.Automatic = append(plan.Automatic, "创建标准生产入口 index.js")
	}
	if mode == "dev" || mode == "all" {
		desired := "lvy app.js"
		if isTS {
			desired = "lvy app.ts"
		}
		planScriptChange(&plan, manifest.Scripts["dev"], desired, "dev")
	}
	if mode == "app" || mode == "dev" || mode == "all" {
		planScriptChange(&plan, manifest.Scripts["app"], "node index.js", "app")
	}
	if mode == "pm2" || mode == "all" {
		planScriptChange(&plan, manifest.Scripts["start"], "npx --yes pm2 startOrRestart pm2.config.cjs", "start")
		planScriptChange(&plan, manifest.Scripts["stop"], "npx --yes pm2 stop pm2.config.cjs", "stop")
		config := filepath.Join(path, "pm2.config.cjs")
		if !exists(config) {
			plan.Automatic = append(plan.Automatic, "创建默认 PM2 配置")
		} else if data, readErr := os.ReadFile(config); readErr == nil && !isManagedPM2Config(path, string(data)) {
			plan.RequiresConfirmation = append(plan.RequiresConfirmation, "覆盖自定义 pm2.config.cjs（将先备份）")
		} else if data, readErr := os.ReadFile(config); readErr == nil && string(data) != defaultPM2Config(path) {
			plan.Automatic = append(plan.Automatic, "升级默认 PM2 配置，隔离同名项目进程")
		}
	}
	if len(plan.Blocked) > 0 {
		plan.Phase = "blocked"
	} else if len(plan.Automatic) == 0 && len(plan.RequiresConfirmation) == 0 {
		plan.Diagnostics = append(plan.Diagnostics, "运行配置已完整，无需修复。")
	}
	return plan, nil
}

func planScriptChange(plan *RuntimeRepairPlan, current, desired, name string) {
	if current == desired {
		return
	}
	if current == "" || current == "node index.js" || current == "lvy app.js" || current == "lvy app.ts" {
		plan.Automatic = append(plan.Automatic, "补齐 "+name+" 运行脚本："+desired)
		return
	}
	plan.RequiresConfirmation = append(plan.RequiresConfirmation, "替换自定义 "+name+" 脚本："+current)
}

// ApplyRuntimeRepair backs up each changed file before writing. Dependency
// installs are intentionally not rolled back; package managers own their lock
// files and caches, while all authored configuration is recoverable.
func (m Manager) ApplyRuntimeRepair(root, mode string, confirmOverrides bool) (RuntimeRepairResult, error) {
	plan, err := m.RuntimeRepairPlan(root, mode)
	if err != nil {
		return RuntimeRepairResult{}, err
	}
	result := RuntimeRepairResult{RuntimeRepairPlan: plan}
	if plan.Phase == "blocked" {
		return result, errors.New(strings.Join(plan.Blocked, "\n"))
	}
	if len(plan.RequiresConfirmation) > 0 && !confirmOverrides {
		return result, errors.New("检测到自定义运行配置，请确认覆盖后继续")
	}
	path, _ := projectPath(root)
	manifest, err := readRuntimeRepairManifest(path)
	if err != nil {
		return result, err
	}
	backup, err := backupRuntimeRepairFiles(path)
	if err != nil {
		return result, err
	}
	result.BackupPath = backup
	if manifest.Scripts == nil {
		manifest.Scripts = map[string]string{}
	}
	if manifest.DevDependencies == nil {
		manifest.DevDependencies = map[string]string{}
	}
	if manifest.Dependencies["alemonjs"] == "" && manifest.DevDependencies["alemonjs"] == "" {
		manifest.DevDependencies["alemonjs"] = "^2.1.94"
	}
	isTS := exists(filepath.Join(path, "app.ts")) || exists(filepath.Join(path, "lvy.config.ts"))
	isJS := exists(filepath.Join(path, "app.js")) || exists(filepath.Join(path, "lvy.config.js"))
	if mode == "dev" || mode == "all" {
		if isTS {
			manifest.Scripts["dev"] = "lvy app.ts"
		} else if isJS {
			manifest.Scripts["dev"] = "lvy app.js"
		} else {
			manifest.Scripts["dev"] = "node index.js"
		}
	}
	if mode == "app" || mode == "dev" || mode == "all" {
		manifest.Scripts["app"] = "node index.js"
	}
	if mode == "pm2" || mode == "all" {
		manifest.Scripts["start"] = "npx --yes pm2 startOrRestart pm2.config.cjs"
		manifest.Scripts["stop"] = "npx --yes pm2 stop pm2.config.cjs"
		manifest.DevDependencies["pm2"] = "^5"
		manifest.DevDependencies["yaml"] = "^2.6.0"
		configPath := filepath.Join(path, "pm2.config.cjs")
		data, readErr := os.ReadFile(configPath)
		if os.IsNotExist(readErr) || readErr == nil && string(data) != defaultPM2Config(path) {
			if err := os.WriteFile(configPath, []byte(defaultPM2Config(path)), 0644); err != nil {
				return result, err
			}
		} else if readErr != nil {
			return result, readErr
		}
	}
	if !exists(filepath.Join(path, "index.js")) && (mode == "app" || mode == "dev" || mode == "pm2" || mode == "all") {
		if err := os.WriteFile(filepath.Join(path, "index.js"), []byte("import { start } from 'alemonjs';\n\nstart();\n"), 0644); err != nil {
			return result, err
		}
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return result, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return result, errors.New("无法读取 package.json")
	}
	raw["scripts"] = manifest.Scripts
	raw["devDependencies"] = manifest.DevDependencies
	encoded, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(filepath.Join(path, "package.json"), append(encoded, '\n'), 0644); err != nil {
		return result, err
	}
	result.Phase = "installing"
	if output, installErr := m.EnsureRuntimeDependencies(root); installErr != nil {
		_ = restoreRuntimeRepairFiles(path, backup)
		result.Phase, result.Output = "rolled_back", output
		return result, installErr
	} else {
		result.Output = output
	}
	if (mode == "dev" || mode == "all") && manifest.Scripts["build"] != "" {
		result.Phase = "building"
		if _, buildErr := runPackageManager(root, "run", "build"); buildErr != nil {
			_ = restoreRuntimeRepairFiles(path, backup)
			result.Phase = "rolled_back"
			return result, fmt.Errorf("构建验证失败，已恢复运行配置：%w", buildErr)
		}
	}
	if missing, checkErr := m.RuntimeDependencies(root); checkErr != nil || len(missing) > 0 {
		_ = restoreRuntimeRepairFiles(path, backup)
		result.Phase = "rolled_back"
		if checkErr != nil {
			return result, checkErr
		}
		return result, errors.New("依赖验证失败，已恢复运行配置：" + strings.Join(missing, "、"))
	}
	result.Phase = "healthy"
	result.Output = strings.TrimSpace(result.Output + "\n运行配置已修复并通过依赖/构建验证。备份位置：" + backup)
	return result, nil
}

func readRuntimeRepairManifest(root string) (runtimeRepairManifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return runtimeRepairManifest{}, err
	}
	var manifest runtimeRepairManifest
	if json.Unmarshal(data, &manifest) != nil {
		return runtimeRepairManifest{}, errors.New("无法读取 package.json")
	}
	return manifest, nil
}

func backupRuntimeRepairFiles(root string) (string, error) {
	dir := filepath.Join(root, ".alemon", "runtime-repairs", time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for _, name := range []string{"package.json", "pm2.config.cjs", "index.js"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func restoreRuntimeRepairFiles(root, backup string) error {
	for _, name := range []string{"package.json", "pm2.config.cjs", "index.js"} {
		data, err := os.ReadFile(filepath.Join(backup, name))
		if os.IsNotExist(err) {
			_ = os.Remove(filepath.Join(root, name))
			continue
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func defaultPM2Config(root string) string {
	return pm2config.Config(root)
}

func isManagedPM2Config(root, config string) bool {
	return config == defaultPM2Config(root) || config == legacyRepairPM2Config || config == legacyTemplatePM2Config || config == legacyDevTemplatePM2Config
}

const legacyRepairPM2Config = "const pm2 = globalThis.pm2;\n\nmodule.exports = pm2 || {\n  apps: [\n    {\n      name: 'alemonb',\n      script: './index.js',\n      env: {\n        NODE_ENV: 'production'\n      }\n    }\n  ]\n};\n"

const legacyTemplatePM2Config = "module.exports = {\n  apps: [\n    {\n      name: 'alemonb',\n      script: './index.js',\n      env: {\n        NODE_ENV: 'production'\n      }\n    }\n  ]\n};\n"

const legacyDevTemplatePM2Config = "/**\n * @type {{ apps: import(\"pm2\").StartOptions[] }}\n */\nmodule.exports = {\n  apps: [\n    {\n      name: 'alemonb',\n      script: './index.js',\n      env: {\n        NODE_ENV: 'production'\n      }\n    }\n  ]\n};\n"

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
