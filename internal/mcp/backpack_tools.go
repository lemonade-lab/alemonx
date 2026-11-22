package mcp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alemonx/internal/robot"
)

func backpackTools() []map[string]any {
	root := stringSchema("从当前账户机器人列表取得的目录。")
	pkg := stringSchema("从背包列表取得的插件名称。")
	confirm := boolSchema("用户已明确确认此次操作。")
	choices := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	batch := map[string]any{"type": "array", "items": pkg, "minItems": 1, "maxItems": 100, "uniqueItems": true}
	return []map[string]any{
		tool("alemonjs_get_backpack_plugin_details", "读取背包插件详情", "读取插件状态、README、本地改动、单文件差异或可恢复的 stash。", objectSchema(map[string]any{
			"root": root, "package": pkg, "view": choices("status", "readme", "changes", "diff", "stashes"), "path": stringSchema("仅 diff 使用，来自该插件改动列表。"),
		}, "root", "package", "view"), true, false),
		toolExternal("alemonjs_list_backpack_plugin_versions", "查询背包插件版本", "查询当前版本、候选版本、本地修改和领先提交。Git 使用本地已拉取的远程引用，npm 版本来自包仓库。", objectSchema(map[string]any{
			"root": root, "package": pkg, "page": map[string]any{"type": "integer", "minimum": 1, "maximum": 100000},
		}, "root", "package")),
		tool("alemonjs_update_backpack_plugins", "安全更新背包插件", "检查最新版本，只更新安全的插件，跳过有本地改动或领先提交的 Git 插件，返回任务 ID。", objectSchema(map[string]any{
			"root": root, "packages": batch, "confirm": confirm,
		}, "root", "packages", "confirm"), false, true),
		tool("alemonjs_manage_backpack_plugin_version", "切换或恢复背包插件", "刷新 Git 历史、配置 Git 来源、切换版本、保存改动后切换、强制切换、恢复或删除 stash。强制切换会丢弃本地改动或领先提交。返回任务 ID。", objectSchema(map[string]any{
			"root": root, "package": pkg, "action": choices("refresh-versions", "refresh-history", "configure-git", "switch", "force-switch", "stash-switch", "restore-stash", "drop-stash"),
			"version": stringSchema("切换时必填，使用查询取得的 Git 提交或 npm 版本。"), "stash": stringSchema("恢复或删除时必填，来自插件 stash 列表。"),
			"repository": stringSchema("仅配置 Git 来源使用，HTTPS 或 SSH 仓库地址。"), "branch": stringSchema("仅配置 Git 来源使用，目标分支。"), "confirm": confirm,
		}, "root", "package", "action", "confirm"), false, true),
		tool("alemonjs_import_backpack_plugin", "本地导入背包插件", "导入服务器上指定机器人目录内的插件文件夹或 ZIP/TGZ/TAR.GZ，返回任务 ID。同名插件不会被覆盖。", objectSchema(map[string]any{
			"root": root, "path": stringSchema("相对机器人目录的路径，例如 imports/example.zip 或 imports/example。与 archiveBase64 二选一。"),
			"archiveBase64": stringSchema("由客户端提供的 ZIP/TGZ/TAR.GZ 压缩包，Base64 编码前最多 700 KiB。与 path 二选一。"), "confirm": confirm,
		}, "root", "confirm"), false, true),
		tool("alemonjs_clone_backpack_plugin", "从 Git 克隆背包插件", "按仓库、分支、目录名、镜像和深度克隆插件，返回任务 ID。私有仓库可使用服务器已有的 SSH 凭据。", objectSchema(map[string]any{
			"root": root, "repository": stringSchema("HTTPS 或 SSH Git 地址，不含凭据。"), "branch": stringSchema("要克隆的分支。"),
			"name": stringSchema("可选，自定义背包目录名。"), "mirror": choices("official", "gh-proxy", "ghproxy-net"), "depth": map[string]any{"type": "integer", "minimum": 0}, "confirm": confirm,
		}, "root", "repository", "branch", "confirm"), false, true),
		tool("alemonjs_set_backpack_plugins_enabled", "批量启停背包插件", "一次写入修改多个背包插件的启用状态，下次机器人启动时生效。", objectSchema(map[string]any{
			"root": root, "packages": batch, "enabled": boolSchema("true 启用，false 停用。"), "confirm": confirm,
		}, "root", "packages", "enabled", "confirm"), false, true),
		tool("alemonjs_get_backpack_plugin_order", "读取背包插件加载顺序", "按实际加载顺序列出已启用的背包插件名称。", objectSchema(map[string]any{"root": root}, "root"), true, false),
		tool("alemonjs_move_backpack_plugin", "调整背包插件加载顺序", "将已启用的背包插件移动到另一个已启用插件之前或之后。", objectSchema(map[string]any{
			"root": root, "source": pkg, "target": pkg, "after": boolSchema("true 放在目标之后，false 放在目标之前。"), "confirm": confirm,
		}, "root", "source", "target", "after", "confirm"), false, true),
		tool("alemonjs_get_backpack_workspace", "读取背包工作区开关", "查看 packages/* 工作区是否已开启。", objectSchema(map[string]any{"root": root}, "root"), true, false),
		tool("alemonjs_set_backpack_workspace", "设置背包工作区开关", "启用或停用 packages/* 工作区，返回任务 ID。", objectSchema(map[string]any{
			"root": root, "enabled": boolSchema("true 开启，false 关闭。"), "confirm": confirm,
		}, "root", "enabled", "confirm"), false, true),
	}
}

type backpackInput struct {
	Root       string   `json:"root"`
	Package    string   `json:"package"`
	Packages   []string `json:"packages"`
	Confirm    bool     `json:"confirm"`
	Enabled    *bool    `json:"enabled"`
	After      *bool    `json:"after"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	View       string   `json:"view"`
	Path       string   `json:"path"`
	Archive    string   `json:"archiveBase64"`
	Page       int      `json:"page"`
	Action     string   `json:"action"`
	Version    string   `json:"version"`
	Stash      string   `json:"stash"`
	Repository string   `json:"repository"`
	Branch     string   `json:"branch"`
	Name       string   `json:"name"`
	Mirror     string   `json:"mirror"`
	Depth      int      `json:"depth"`
}

func (s *Server) backpackTool(name string, arguments json.RawMessage) (string, error) {
	var input backpackInput
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	root, err := s.registeredRobot(input.Root)
	if err != nil {
		return "", err
	}
	if info, err := os.Lstat(filepath.Join(root, "packages")); err == nil && !info.IsDir() {
		return "", errors.New("packages 必须是普通目录")
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	switch name {
	case "alemonjs_get_backpack_plugin_details":
		return s.backpackDetails(root, input)
	case "alemonjs_list_backpack_plugin_versions":
		if input.Page < 0 || input.Page > 100000 {
			return "", errors.New("版本页码无效")
		}
		return backpackResult(s.robots.CachedLocalPackageVersions(root, input.Package, input.Page))
	case "alemonjs_get_backpack_plugin_order":
		apps, err := s.robots.EnabledApps(root)
		if err != nil {
			return "", err
		}
		packages, err := s.robots.LocalPackages(root)
		if err != nil {
			return "", err
		}
		order := make([]string, 0, len(apps))
		for _, app := range apps {
			if slices.ContainsFunc(packages, func(item robot.LocalPackage) bool { return item.Name == app && item.Valid }) {
				order = append(order, app)
			}
		}
		return encodeResult(order)
	case "alemonjs_get_backpack_workspace":
		manifest, err := s.robots.PackageManifest(root)
		if err != nil {
			return "", err
		}
		return encodeResult(map[string]bool{"enabled": manifest.Private && manifest.WorkspacesEnabled && slices.Contains(manifest.Workspaces, "packages/*")})
	}
	if !input.Confirm {
		return "", errors.New("请在用户确认后传 confirm=true")
	}
	action := ProjectAction{Root: root, Package: input.Package}
	switch name {
	case "alemonjs_update_backpack_plugins":
		if err := s.validateBackpackSelection(root, input.Packages, true); err != nil {
			return "", err
		}
		action.Action = "safe-update-backpack-plugins"
		action.Message, err = encodeResult(input.Packages)
	case "alemonjs_manage_backpack_plugin_version":
		if err := s.validateBackpackSelection(root, []string{input.Package}, true); err != nil {
			return "", err
		}
		if err := backpackVersionAction(&action, input); err != nil {
			return "", err
		}
	case "alemonjs_import_backpack_plugin":
		if (input.Archive == "") == (input.Path == "") {
			return "", errors.New("请在相对路径和内联压缩包之间选择一种导入方式")
		}
		if input.Archive != "" {
			if len(input.Archive) > base64.StdEncoding.EncodedLen(robot.MaxInlineBackpackArchiveSize) {
				return "", errors.New("内联插件压缩包不能超过 700 KiB")
			}
			action.Action, action.Message = "import-inline-backpack-archive", input.Archive
			break
		}
		if input.Path == "." || !filepath.IsLocal(input.Path) || strings.ContainsAny(input.Path, "\\:\x00") {
			return "", errors.New("请选择机器人目录内的插件文件夹或压缩包")
		}
		action.Action, action.Message = "import-local-backpack-plugin", input.Path
	case "alemonjs_clone_backpack_plugin":
		if !validBackpackRemote(input.Repository) || input.Branch == "" || input.Depth < 0 {
			return "", errors.New("请填写没有内嵌凭据的 HTTPS 或 SSH Git 地址、分支及有效的克隆深度")
		}
		action.Action = "clone-backpack-plugin"
		action.Message, err = encodeResult(map[string]any{"repository": input.Repository, "branch": input.Branch, "name": input.Name, "mirror": input.Mirror, "depth": input.Depth})
	case "alemonjs_set_backpack_plugins_enabled":
		if input.Enabled == nil {
			return "", errors.New("请明确指定启用状态")
		}
		if err := s.validateBackpackSelection(root, input.Packages, *input.Enabled); err != nil {
			return "", err
		}
		if _, err := s.robots.SetAppsEnabled(root, input.Packages, *input.Enabled); err != nil {
			return "", err
		}
		return encodeResult(map[string]any{"packages": input.Packages, "enabled": *input.Enabled, "appliesOnNextStart": true})
	case "alemonjs_move_backpack_plugin":
		if input.After == nil {
			return "", errors.New("请指定移动到目标之前或之后")
		}
		if err := s.validateBackpackSelection(root, []string{input.Source, input.Target}, true); err != nil {
			return "", err
		}
		return backpackResult(s.robots.MoveApp(root, input.Source, input.Target, *input.After))
	case "alemonjs_set_backpack_workspace":
		if input.Enabled == nil {
			return "", errors.New("请明确指定工作区开关状态")
		}
		action.Action = "disable-backpack-workspace"
		if *input.Enabled {
			action.Action = "enable-backpack-workspace"
		}
	default:
		return "", errors.New("未知背包操作")
	}
	if err != nil {
		return "", err
	}
	return s.submitProjectAction(action)
}

func (s *Server) validateBackpackSelection(root string, names []string, validOnly bool) error {
	if len(names) == 0 || len(names) > 100 {
		return errors.New("请选择 1 到 100 个背包插件")
	}
	packages, err := s.robots.LocalPackages(root)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] || !slices.ContainsFunc(packages, func(item robot.LocalPackage) bool {
			return item.Name == name && (!validOnly || item.Valid)
		}) {
			return fmt.Errorf("请选择不同的背包插件：%s", name)
		}
		seen[name] = true
	}
	return nil
}

func backpackVersionAction(action *ProjectAction, input backpackInput) error {
	switch input.Action {
	case "refresh-versions":
		action.Action = "refresh-backpack-plugin-versions"
	case "refresh-history":
		action.Action = "refresh-local-package-history"
	case "configure-git":
		if !validBackpackRemote(input.Repository) || input.Branch == "" {
			return errors.New("请填写没有内嵌凭据的 HTTPS 或 SSH Git 地址及分支")
		}
		action.Action = "configure-local-package-git"
		var err error
		action.Message, err = encodeResult(map[string]string{"repository": input.Repository, "branch": input.Branch})
		return err
	case "switch", "force-switch", "stash-switch":
		if strings.TrimSpace(input.Version) == "" {
			return errors.New("请选择要切换的版本或提交")
		}
		action.Action = map[string]string{"switch": "sync-local-package-release", "force-switch": "force-sync-local-package-release", "stash-switch": "stash-sync-local-package-release"}[input.Action]
		action.Version = input.Version
	case "restore-stash", "drop-stash":
		if strings.TrimSpace(input.Stash) == "" {
			return errors.New("请选择要恢复或删除的 stash")
		}
		action.Action = map[string]string{"restore-stash": "restore-local-package-stash", "drop-stash": "drop-local-package-stash"}[input.Action]
		action.Message = input.Stash
	default:
		return errors.New("未知插件版本操作")
	}
	return nil
}

func validBackpackRemote(repository string) bool {
	if strings.HasPrefix(repository, "git@") {
		return !strings.ContainsAny(repository, " \t\r\n") && strings.Contains(repository, ":")
	}
	remote, err := url.Parse(repository)
	if err != nil || remote.Hostname() == "" || remote.RawQuery != "" || remote.Fragment != "" {
		return false
	}
	if remote.Scheme == "https" {
		return remote.User == nil
	}
	if remote.Scheme != "ssh" {
		return false
	}
	_, password := remote.User.Password()
	return !password
}

func (s *Server) backpackDetails(root string, input backpackInput) (string, error) {
	switch input.View {
	case "status":
		status, err := s.robots.LocalPackageStatus(root, input.Package)
		status.Repository = robot.RedactRuntimeLog(status.Repository)
		return backpackResult(status, err)
	case "readme":
		return backpackResult(s.robots.LocalPackageReadme(root, input.Package))
	case "changes":
		return backpackResult(s.robots.LocalPackageChanges(root, input.Package))
	case "diff":
		return backpackResult(s.robots.ManagedLocalPackageDiff(root, input.Package, input.Path))
	case "stashes":
		return backpackResult(s.robots.LocalPackageStashes(root, input.Package))
	default:
		return "", errors.New("未知插件详情类型")
	}
}

func backpackResult(result any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return encodeResult(result)
}
