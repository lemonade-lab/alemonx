// Package robot provides guarded local project management operations.
package robot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"alemonx/internal/catalog"
	"alemonx/internal/resources"
	"alemonx/internal/system"
)

type Manager struct{}
type Result struct {
	Output string `json:"output"`
	Path   string `json:"path"`
}

// PM2LogPage is a read-only slice of PM2 output. Page 1 is always the newest
// output, so users see the current problem before choosing to inspect history.
type PM2LogPage struct {
	Output   string `json:"output"`
	Page     int    `json:"page"`
	HasOlder bool   `json:"hasOlder"`
}

// PM2Status describes the PM2 process associated with one robot directory.
// It deliberately matches by PM2's working directory so unrelated services
// managed by the same local PM2 daemon never affect this project's controls.
type PM2Status struct {
	Configured bool   `json:"configured"`
	Managed    bool   `json:"managed"`
	Running    bool   `json:"running"`
	Status     string `json:"status,omitempty"`
}

// StreamPM2Logs follows PM2's raw log output until ctx is cancelled or the
// child exits. Callers own retry/backoff policy; keeping it here makes the
// process invocation testable and avoids a PM2 Node SDK dependency.
func (Manager) StreamPM2Logs(ctx context.Context, root string, onLine func(string)) error {
	path, err := projectPath(root)
	if err != nil {
		return err
	}
	name, args := pm2Launcher(path)
	command := exec.CommandContext(ctx, name, append(args, "logs", "--raw", "--lines", "0")...)
	command.Dir = path
	applyManagedNodeEnvironment(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan struct{}, 2)
	read := func(scanner *bufio.Scanner) {
		defer func() { done <- struct{}{} }()
		scanner.Buffer(make([]byte, 16*1024), 1024*1024)
		for scanner.Scan() {
			if onLine != nil {
				onLine(scanner.Text())
			}
		}
	}
	go read(bufio.NewScanner(stdout))
	go read(bufio.NewScanner(stderr))
	err = command.Wait()
	<-done
	<-done
	return err
}

// Validate confirms that a saved robot directory still exists and is an
// eligible Node.js project. It is intentionally side-effect free.
func (Manager) Validate(root string) (Result, error) {
	path, err := projectPath(root)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: path, Output: "机器人目录可用。"}, nil
}

// LocalPackage is a bundled plugin found in the robot's packages directory.
type LocalPackage struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Valid       bool   `json:"valid"`
}

// LocalPackageVersions identifies one authoritative version source for a
// backpack entry. A checked-out Git package always wins over npm metadata,
// matching the framework's source-first plugin convention.
type LocalPackageVersions struct {
	Source   string   `json:"source"`
	Current  string   `json:"current"`
	Latest   string   `json:"latest,omitempty"`
	Versions []string `json:"versions"`
}

func (m Manager) LocalPackageVersions(root, packageName string) (LocalPackageVersions, error) {
	items, err := m.LocalPackages(root)
	if err != nil {
		return LocalPackageVersions{}, err
	}
	for _, item := range items {
		if item.Name != packageName || !item.Valid {
			continue
		}
		if output, gitErr := gitRun(item.Path, "rev-parse", "--is-inside-work-tree"); gitErr == nil && strings.TrimSpace(output) == "true" {
			current, _ := gitRun(item.Path, "describe", "--tags", "--always", "--dirty")
			versions := packageGitVersions(item.Path)
			latest := ""
			if len(versions) > 0 {
				latest = versions[0]
			}
			return LocalPackageVersions{Source: "git", Current: strings.TrimSpace(current), Latest: latest, Versions: versions}, nil
		}
		versions, loadErr := catalog.LoadPackageVersions(item.Name)
		if loadErr != nil {
			return LocalPackageVersions{}, loadErr
		}
		return LocalPackageVersions{Source: "npm", Current: item.Version, Latest: versions.Latest, Versions: versions.Versions}, nil
	}
	return LocalPackageVersions{}, errors.New("背包中没有这个本地插件包")
}

// packageGitVersions enumerates published tags from origin without changing
// the checkout. A shallow clone often has no local tags, so only reading
// `git tag` would incorrectly make the version selector look empty.
func packageGitVersions(root string) []string {
	all := map[string]bool{}
	for _, tag := range gitLines(root, "tag", "--list", "v*", "--sort=-v:refname") {
		if gitVersionPattern.MatchString(tag) {
			all[tag] = true
		}
	}
	if remote, err := gitRun(root, "ls-remote", "--tags", "--refs", "origin"); err == nil {
		for _, line := range strings.Split(remote, "\n") {
			parts := strings.Fields(line)
			if len(parts) != 2 {
				continue
			}
			tag := strings.TrimPrefix(parts[1], "refs/tags/")
			if gitVersionPattern.MatchString(tag) {
				all[tag] = true
			}
		}
	}
	versions := make([]string, 0, len(all))
	for tag := range all {
		versions = append(versions, tag)
	}
	sort.Slice(versions, func(i, j int) bool { return newerGitTag(versions[i], versions[j]) })
	return versions
}

func newerGitTag(left, right string) bool {
	return semver.Compare(left, right) > 0
}

// LocalPackageReadme reads only the README belonging to a discovered backpack
// entry. It never accepts a caller-provided path, so the endpoint cannot be
// used to browse arbitrary project files.
func (m Manager) LocalPackageReadme(root, packageName string) (Result, error) {
	items, err := m.LocalPackages(root)
	if err != nil {
		return Result{}, err
	}
	for _, item := range items {
		if item.Name != packageName {
			continue
		}
		path := filepath.Join(item.Path, "README.md")
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Result{}, errors.New("这个本地包没有 README.md")
		}
		if info.Size() > maxMCPFileSize {
			return Result{}, errors.New("README.md 过大，无法显示")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return Result{}, errors.New("无法读取 README.md")
		}
		return Result{Path: path, Output: string(data)}, nil
	}
	return Result{}, errors.New("背包中没有这个本地包")
}

// RuntimeOverview is the small, stable set of project facts needed by the
// dashboard's run page. Packages are checked on disk, not only in
// package.json: declaring a platform is not the same as having installed it.
type RuntimeOverview struct {
	Name                 string           `json:"name"`
	Version              string           `json:"version"`
	PackageManager       string           `json:"packageManager"`
	HasAppScript         bool             `json:"hasAppScript"`
	HasDevScript         bool             `json:"hasDevScript"`
	HasBuildScript       bool             `json:"hasBuildScript"`
	HasStartScript       bool             `json:"hasStartScript"`
	PM2Configured        bool             `json:"pm2Configured"`
	DependenciesComplete bool             `json:"dependenciesComplete"`
	Platforms            []RuntimePackage `json:"platforms"`
}

type RuntimePackage struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Package   string `json:"package"`
	Declared  bool   `json:"declared"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	Source    string `json:"source,omitempty"`
	Logo      string `json:"logo,omitempty"`
}

type RuntimePreflight struct {
	Login                string   `json:"login"`
	Package              string   `json:"package,omitempty"`
	Missing              []string `json:"missing"`
	Summary              []string `json:"summary"`
	DependenciesComplete bool     `json:"dependenciesComplete"`
}

type platformCandidate struct{ id, label, pkg string }

// builtinPlatforms are the well-known connection packages shown as installable
// candidates. Discovered desktop.platform declarations always override them.
var builtinPlatforms = []platformCandidate{
	{"onebot", "OneBot", "@alemonjs/onebot"},
	{"qq-bot", "QQ Bot", "@alemonjs/qq-bot"},
	{"discord", "Discord", "@alemonjs/discord"},
	{"bubble", "Bubble", "@alemonjs/bubble"},
	{"kook", "KOOK", "@alemonjs/kook"},
	{"telegram", "Telegram", "@alemonjs/telegram"},
}

// resolveRuntimePlatforms merges the builtin candidates with platform
// connections declared by installed dependencies and backpack packages.
// Dynamic declarations win for the same login identifier.
func resolveRuntimePlatforms(project string) ([]RuntimePackage, error) {
	data, err := os.ReadFile(filepath.Join(project, "package.json"))
	if err != nil {
		return nil, errors.New("无法读取 package.json")
	}
	var manifest struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return nil, errors.New("package.json 无法识别")
	}
	declared := map[string]bool{}
	for name := range manifest.Dependencies {
		declared[name] = true
	}
	for name := range manifest.DevDependencies {
		declared[name] = true
	}
	for name := range manifest.OptionalDependencies {
		declared[name] = true
	}
	merged := map[string]RuntimePackage{}
	for _, candidate := range builtinPlatforms {
		item := RuntimePackage{ID: candidate.id, Label: candidate.label, Package: candidate.pkg, Source: "builtin"}
		item.Declared = declared[item.Package]
		if packageData, readErr := os.ReadFile(filepath.Join(project, "node_modules", filepath.FromSlash(item.Package), "package.json")); readErr == nil {
			var installed struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(packageData, &installed) == nil {
				item.Installed = true
				item.Version = installed.Version
			}
		}
		merged[item.ID] = item
	}
	for name := range declared {
		packageFile := filepath.Join(project, "node_modules", filepath.FromSlash(name), "package.json")
		if data, readErr := os.ReadFile(packageFile); readErr == nil {
			mergeDeclaredPlatform(merged, data)
		}
	}
	backpack := filepath.Join(project, "packages")
	if entries, readErr := os.ReadDir(backpack); readErr == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			packageFile := filepath.Join(backpack, entry.Name(), "package.json")
			if data, readErr := os.ReadFile(packageFile); readErr == nil {
				mergeDeclaredPlatform(merged, data)
			}
			if strings.HasPrefix(entry.Name(), "@") {
				if scoped, scopedErr := os.ReadDir(filepath.Join(backpack, entry.Name())); scopedErr == nil {
					for _, child := range scoped {
						if child.IsDir() {
							if data, readErr := os.ReadFile(filepath.Join(backpack, entry.Name(), child.Name(), "package.json")); readErr == nil {
								mergeDeclaredPlatform(merged, data)
							}
						}
					}
				}
			}
		}
	}
	result := make([]RuntimePackage, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Label), strings.ToLower(result[j].Label)
		if left == right {
			return result[i].ID < result[j].ID
		}
		return left < right
	})
	return result, nil
}

// mergeDeclaredPlatform registers every desktop.platform entry of an installed
// package. Dynamic entries override builtin candidates with the same ID.
func mergeDeclaredPlatform(merged map[string]RuntimePackage, data []byte) {
	var manifest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		Alemonjs    struct {
			Desktop struct {
				Logo     string `json:"logo"`
				Platform []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"platform"`
			} `json:"desktop"`
		} `json:"alemonjs"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name == "" {
		return
	}
	for _, platform := range manifest.Alemonjs.Desktop.Platform {
		if !yamlNamePattern.MatchString(platform.Name) {
			continue
		}
		packageName := strings.TrimSpace(platform.Value)
		if packageName == "" {
			packageName = manifest.Name
		}
		label := strings.TrimSpace(manifest.Description)
		if label == "" {
			label = manifest.Name
			if slash := strings.LastIndex(label, "/"); slash >= 0 {
				label = label[slash+1:]
			}
		}
		merged[platform.Name] = RuntimePackage{
			ID:        platform.Name,
			Label:     label,
			Package:   packageName,
			Declared:  true,
			Installed: true,
			Version:   manifest.Version,
			Source:    "declared",
			Logo:      manifest.Alemonjs.Desktop.Logo,
		}
	}
}

const maxMCPFileSize = 1024 * 1024

// ListProjectFiles returns source and configuration files that an MCP client
// may inspect. Dependency trees, Git metadata, secrets, and symlinks are
// intentionally excluded even though they may live under the project root.
func (Manager) ListProjectFiles(root string) ([]string, error) {
	path, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == path {
			return nil
		}
		relative, err := filepath.Rel(path, current)
		if err != nil {
			return err
		}
		if blockedProjectPath(relative) || entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		files = append(files, filepath.ToSlash(relative))
		if len(files) > 1000 {
			return errors.New("项目文件过多；请缩小项目目录后重试")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("读取项目文件列表失败：%w", err)
	}
	sort.Strings(files)
	return files, nil
}

// ReadProjectFile reads a non-sensitive, regular file below a managed robot
// project. It is separate from Read so the UI's narrow configuration-file
// contract remains unchanged.
func (Manager) ReadProjectFile(root, name string) (Result, error) {
	path, err := managedProjectFile(root, name)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("读取失败：%w", err)
	}
	if info.Size() > maxMCPFileSize {
		return Result{}, errors.New("文件超过 1 MiB，MCP 不会读取")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("读取失败：%w", err)
	}
	return Result{Path: path, Output: string(data)}, nil
}

// WriteProjectFile writes a non-sensitive source/configuration file within a
// managed project. It does not create directories and rejects symlinks.
func (Manager) WriteProjectFile(root, name, content string) (Result, error) {
	if len(content) > maxMCPFileSize {
		return Result{}, errors.New("文件内容超过 1 MiB，MCP 不会写入")
	}
	path, err := managedProjectFile(root, name)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("访问 " + filepath.Dir(name))
		}
		return Result{}, errors.New("目标目录不存在；MCP 不会自动创建目录")
	}
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return Result{}, errors.New("只能写入普通项目文件")
	} else if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("无法检查目标文件：%w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("保存 " + filepath.Base(path))
		}
		return Result{}, fmt.Errorf("保存失败：%w", err)
	}
	return Result{Path: path, Output: "已保存。"}, nil
}

// CreateProjectFile creates a new source file within a managed project. It
// rejects files that already exist and inherits the same path, size and
// sensitivity checks as WriteProjectFile.
func (Manager) CreateProjectFile(root, name, content string) (Result, error) {
	if len(content) > maxMCPFileSize {
		return Result{}, errors.New("文件内容超过 1 MiB，MCP 不会写入")
	}
	path, err := managedProjectFile(root, name)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("访问 " + filepath.Dir(name))
		}
		return Result{}, errors.New("目标目录不存在；MCP 不会自动创建目录")
	}
	if _, err := os.Lstat(path); err == nil {
		return Result{}, errors.New("目标文件已存在；如需修改请用 edit 模式")
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("无法检查目标文件：%w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("创建 " + filepath.Base(path))
		}
		return Result{}, fmt.Errorf("创建失败：%w", err)
	}
	return Result{Path: path, Output: "已创建。"}, nil
}

// DeleteProjectFile removes a non-sensitive source file within a managed
// project. It rejects symlinks and refuses to delete directories.
func (Manager) DeleteProjectFile(root, name string) (Result, error) {
	path, err := managedProjectFile(root, name)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Result{}, errors.New("目标文件不存在")
	}
	if err != nil {
		return Result{}, fmt.Errorf("无法检查目标文件：%w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Result{}, errors.New("只能删除普通项目文件")
	}
	if err := os.Remove(path); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("删除 " + filepath.Base(path))
		}
		return Result{}, fmt.Errorf("删除失败：%w", err)
	}
	return Result{Path: path, Output: "已删除。"}, nil
}

func (Manager) LocalPackages(root string) ([]LocalPackage, error) {
	path, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(path, "packages")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []LocalPackage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("无法读取 packages 目录：%w", err)
	}
	items := make([]LocalPackage, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			item := LocalPackage{Name: entry.Name(), Path: filepath.Join(directory, entry.Name())}
			data, readErr := os.ReadFile(filepath.Join(item.Path, "package.json"))
			if readErr == nil {
				var manifest struct {
					Name        string `json:"name"`
					Version     string `json:"version"`
					Description string `json:"description"`
				}
				if json.Unmarshal(data, &manifest) == nil && manifest.Name != "" {
					item.Name, item.Version, item.Description, item.Valid = manifest.Name, manifest.Version, manifest.Description, true
				}
			}
			items = append(items, item)
		}
	}
	return items, nil
}

func (Manager) RuntimeOverview(root string) (RuntimeOverview, error) {
	path, err := projectPath(root)
	if err != nil {
		return RuntimeOverview{}, err
	}
	var manifest struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		PackageManager  string            `json:"packageManager"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		return RuntimeOverview{}, errors.New("无法读取 package.json")
	}
	overview := RuntimeOverview{Name: manifest.Name, Version: manifest.Version, PackageManager: projectPackageManager(path), HasAppScript: manifest.Scripts["app"] != "", HasDevScript: manifest.Scripts["dev"] != "", HasBuildScript: manifest.Scripts["build"] != "", HasStartScript: manifest.Scripts["start"] != ""}
	if manifest.PackageManager != "" {
		overview.PackageManager = strings.Split(manifest.PackageManager, "@")[0]
	}
	if missing, err := (Manager{}).RuntimeDependencies(root); err == nil && len(missing) == 0 {
		overview.DependenciesComplete = true
	}
	if _, err := os.Stat(filepath.Join(path, "pm2.config.cjs")); err == nil {
		overview.PM2Configured = true
	}
	platforms, platformErr := resolveRuntimePlatforms(path)
	if platformErr != nil {
		return RuntimeOverview{}, platformErr
	}
	overview.Platforms = platforms
	return overview, nil
}

// RuntimePreflight describes the effective, non-secret connection settings
// before a command is allowed to start. Values are deliberately reduced to
// “configured / missing” so tokens never enter the browser response.
func (m Manager) RuntimePreflight(root string) (RuntimePreflight, error) {
	path, err := projectPath(root)
	if err != nil {
		return RuntimePreflight{}, err
	}
	content, err := os.ReadFile(filepath.Join(path, "alemon.config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return RuntimePreflight{}, fmt.Errorf("无法读取机器人运行配置：%w", err)
	}
	content = []byte(stripYAMLBOM(string(content)))
	preflight := RuntimePreflight{Missing: []string{}, Summary: []string{}}
	dependencies, dependencyErr := m.RuntimeDependencies(root)
	if dependencyErr != nil {
		return RuntimePreflight{}, dependencyErr
	}
	preflight.DependenciesComplete = len(dependencies) == 0
	if preflight.DependenciesComplete {
		preflight.Summary = append(preflight.Summary, "项目依赖：完整")
	} else {
		preflight.Missing = append(preflight.Missing, dependencies...)
		preflight.Summary = append(preflight.Summary, "项目依赖：需要重载")
	}
	match := regexp.MustCompile(`(?m)^login:\s*['\"]?([^'\"\r\n#]+)`).FindStringSubmatch(string(content))
	if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
		preflight.Summary = append(preflight.Summary, "登录连接：未配置（可选择无 login 启动）")
		return preflight, nil
	}
	preflight.Login = strings.TrimSpace(match[1])
	preflight.Summary = append(preflight.Summary, "登录连接："+preflight.Login)
	platforms, platformErr := resolveRuntimePlatforms(path)
	if platformErr != nil {
		return RuntimePreflight{}, platformErr
	}
	for _, platform := range platforms {
		if platform.ID != preflight.Login {
			continue
		}
		preflight.Package = platform.Package
		if _, _, installedErr := installedPackageManifest(path, platform.Package); installedErr != nil {
			preflight.Missing = append(preflight.Missing, "连接包 "+platform.Package+" 未安装")
			return preflight, nil
		}
		definition, configErr := m.PackageConfig(root, platform.Package)
		if configErr != nil {
			preflight.Missing = append(preflight.Missing, "连接包 "+platform.Package+" 配置无法读取")
			return preflight, nil
		}
		if len(definition.Fields) == 0 {
			preflight.Summary = append(preflight.Summary, "连接包 "+platform.Package+"：已安装，无额外配置")
			return preflight, nil
		}
		for _, field := range definition.Fields {
			label := field.Description
			if label == "" {
				label = field.Name
			}
			configured := !isConfigEmpty(definition.Values[field.Name])
			if field.Required && !configured && !field.DefaultConfigured() {
				preflight.Missing = append(preflight.Missing, label)
			}
			preflight.Summary = append(preflight.Summary, label+map[bool]string{true: "：已填写", false: "：未填写"}[configured])
		}
		return preflight, nil
	}
	preflight.Summary = append(preflight.Summary, "自定义登录对象：未声明可校验字段")
	return preflight, nil
}

// RuntimeDependencies checks every direct project dependency on disk. A lock
// file alone is not enough: a partially completed install can leave the bot
// unable to start even though package.json looks correct.
func (Manager) RuntimeDependencies(root string) ([]string, error) {
	path, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		return nil, errors.New("无法读取 package.json")
	}
	if _, err := os.Stat(filepath.Join(path, "node_modules")); err != nil {
		if os.IsNotExist(err) {
			return []string{"未发现 node_modules，请先重载依赖"}, nil
		}
		return nil, fmt.Errorf("无法检查 node_modules：%w", err)
	}
	packages := map[string]bool{}
	for name := range manifest.Dependencies {
		packages[name] = true
	}
	for name := range manifest.DevDependencies {
		packages[name] = true
	}
	missing := []string{}
	for name := range packages {
		if _, err := os.Stat(filepath.Join(path, "node_modules", filepath.FromSlash(name), "package.json")); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, name+" 未安装")
				continue
			}
			return nil, fmt.Errorf("无法检查依赖 %s：%w", name, err)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// EnsureRuntimeDependencies makes a robot runnable without asking its user to
// understand node_modules or lock files. It only invokes the project's own
// package manager when the on-disk dependency check finds something missing.
func (m Manager) EnsureRuntimeDependencies(root string) (string, error) {
	missing, err := m.RuntimeDependencies(root)
	if err != nil {
		return "", err
	}
	if len(missing) == 0 {
		return "", nil
	}
	prefix := "检测到依赖不完整（" + strings.Join(missing, "、") + "），正在自动同步依赖。"
	output, installErr := m.installRuntimeDependencies(root, "自动同步依赖")
	return prefix + "\n" + output, installErr
}

// installRuntimeDependencies verifies the package manager's result on disk so
// an install command that exits successfully cannot be reported as complete
// while direct project dependencies are still missing.
func (m Manager) installRuntimeDependencies(root, action string) (string, error) {
	removeYarnIntegrity(root)
	output, installErr := runPackageManager(root, "install")
	if installErr != nil {
		return output, buildDependencyError(action+"失败", output, installErr)
	}
	remaining, checkErr := m.RuntimeDependencies(root)
	if checkErr != nil {
		return output, checkErr
	}
	if len(remaining) > 0 {
		return output, errors.New(action + "后依赖仍不完整：" + strings.Join(remaining, "、"))
	}
	return strings.TrimSpace(output + "\n依赖已同步。"), nil
}

// SyncWorkspaceDependencies is used after changing packages/*, which is a
// workspace topology change even if every root dependency was already present.
func (Manager) SyncWorkspaceDependencies(root string) (string, error) {
	removeYarnIntegrity(root)
	output, err := runPackageManager(root, "install")
	prefix := "本地插件工作区已变更，正在同步依赖。"
	if err != nil {
		return prefix + "\n" + output, buildDependencyError("同步本地插件依赖失败", output, err)
	}
	return prefix + "\n" + output + "\n本地插件依赖已同步。", nil
}

// removeYarnIntegrity clears node_modules/.yarn-integrity before an install.
// Yarn v1 trusts that marker and answers "Already up-to-date" without checking
// whether the declared packages actually exist on disk, so a partially missing
// node_modules would otherwise never be repaired by a subsequent install.
func removeYarnIntegrity(root string) {
	path, err := projectPath(root)
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(path, "node_modules", ".yarn-integrity"))
}

type alemonUpgradePlan struct {
	Dependencies    []string
	DevDependencies []string
	Workspace       bool
}

// UpgradeAlemonDependencies upgrades only the framework and its official
// scoped packages declared directly by this robot. Business dependencies stay
// untouched, so the one-click action is safe to use as framework maintenance.
func (m Manager) UpgradeAlemonDependencies(root string) (Result, error) {
	path, err := projectPath(root)
	if err != nil {
		return Result{}, err
	}
	plan, err := readAlemonUpgradePlan(path)
	if err != nil {
		return Result{}, err
	}
	packages := append(append([]string{}, plan.Dependencies...), plan.DevDependencies...)
	if len(packages) == 0 {
		return Result{Path: path, Output: "package.json 中未声明 alemonjs 或 @alemonjs/ 依赖，无需升级。"}, nil
	}
	manager := projectPackageManager(path)
	latest := func(items []string) []string {
		result := make([]string, 0, len(items))
		for _, item := range items {
			result = append(result, item+"@latest")
		}
		return result
	}
	var outputs []string
	runUpgrade := func(args ...string) error {
		output, runErr := runNamedPackageManager(path, manager, args...)
		if output != "" {
			outputs = append(outputs, output)
		}
		return runErr
	}
	switch manager {
	case "npm":
		if len(plan.Dependencies) > 0 {
			if err := runUpgrade(append([]string{"install", "--save"}, latest(plan.Dependencies)...)...); err != nil {
				return Result{Path: path, Output: strings.Join(outputs, "\n")}, err
			}
		}
		if len(plan.DevDependencies) > 0 {
			if err := runUpgrade(append([]string{"install", "--save-dev"}, latest(plan.DevDependencies)...)...); err != nil {
				return Result{Path: path, Output: strings.Join(outputs, "\n")}, err
			}
		}
	case "yarn":
		// Yarn 1's `upgrade --latest` refuses a stale lockfile and a fallback
		// `yarn install` would reconcile every project dependency. Use targeted
		// `add` commands instead: only the allow-listed AlemonJS packages are
		// changed in package.json, with their necessary lock entries updated.
		// In a Yarn workspace, -W explicitly confirms that these are root
		// dependencies; without it Yarn aborts before resolving any package.
		if len(plan.Dependencies) > 0 {
			if err := runUpgrade(append([]string{"add", "-W"}, latest(plan.Dependencies)...)...); err != nil {
				return Result{Path: path, Output: strings.Join(outputs, "\n")}, err
			}
		}
		if len(plan.DevDependencies) > 0 {
			if err := runUpgrade(append([]string{"add", "--dev", "-W"}, latest(plan.DevDependencies)...)...); err != nil {
				return Result{Path: path, Output: strings.Join(outputs, "\n")}, err
			}
		}
	default:
		args := []string{"upgrade", "--latest"}
		if manager == "pnpm" {
			args[0] = "update"
		}
		if plan.Workspace {
			if manager == "pnpm" {
				args = append(args, "-w")
			}
		}
		if err := runUpgrade(append(args, packages...)...); err != nil {
			return Result{Path: path, Output: strings.Join(outputs, "\n")}, err
		}
	}
	return Result{Path: path, Output: "已升级 AlemonJS 依赖：" + strings.Join(packages, "、") + ".\n" + strings.Join(outputs, "\n")}, nil
}

func readAlemonUpgradePlan(root string) (alemonUpgradePlan, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return alemonUpgradePlan{}, errors.New("无法读取 package.json")
	}
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Workspaces      json.RawMessage   `json:"workspaces"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return alemonUpgradePlan{}, errors.New("package.json 格式无法识别")
	}
	filter := func(values map[string]string) []string {
		result := []string{}
		for name := range values {
			if name == "alemonjs" || strings.HasPrefix(name, "@alemonjs/") {
				result = append(result, name)
			}
		}
		sort.Strings(result)
		return result
	}
	return alemonUpgradePlan{
		Dependencies:    filter(manifest.Dependencies),
		DevDependencies: filter(manifest.DevDependencies),
		Workspace:       len(manifest.Workspaces) > 0 && string(manifest.Workspaces) != "null",
	}, nil
}

// Console returns a fixed, read-only project snapshot. There is deliberately
// no command argument: the web UI can present terminal-like context without
// exposing a browser shell that accepts arbitrary input.
func (Manager) Console(root string) (Result, error) {
	path, err := projectPath(root)
	if err != nil {
		return Result{}, err
	}
	lines := []string{"$ pwd", path}
	var manifest struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Scripts map[string]string `json:"scripts"`
	}
	if data, readErr := os.ReadFile(filepath.Join(path, "package.json")); readErr == nil && json.Unmarshal(data, &manifest) == nil {
		lines = append(lines, "", "$ package.json", fmt.Sprintf("%s@%s", manifest.Name, manifest.Version))
		keys := make([]string, 0, len(manifest.Scripts))
		for key := range manifest.Scripts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 0 {
			lines = append(lines, "", "$ scripts")
			for _, key := range keys {
				lines = append(lines, key+" · "+manifest.Scripts[key])
			}
		}
	}
	lines = append(lines, "", "$ git status --short")
	if output, gitErr := run(path, "git", "status", "--short"); gitErr == nil {
		// A project without a .gitignore (or one missing node_modules) reports
		// thousands of dependency entries. The terminal snapshot is about the
		// project, so filter noisy generated/ignored paths out of the display.
		clean := []string{}
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			entry := trimmed
			if len(entry) >= 4 {
				entry = entry[3:]
			}
			entry = strings.TrimSpace(entry)
			if entry == "" || isConsoleNoisePath(entry) {
				continue
			}
			clean = append(clean, line)
		}
		if len(clean) == 0 {
			lines = append(lines, "工作区干净")
		} else {
			lines = append(lines, clean...)
		}
	} else {
		lines = append(lines, "当前目录尚未初始化 Git，或 Git 不可用。")
	}
	lines = append(lines, "", "$ node --version")
	if output, nodeErr := run(path, "node", "--version"); nodeErr == nil {
		lines = append(lines, output)
	} else {
		lines = append(lines, "未检测到 Node.js。")
	}
	return Result{Path: path, Output: strings.Join(lines, "\n")}, nil
}

// isConsoleNoisePath reports whether a git-status path should be hidden from
// the terminal snapshot. Dependency/build/environment directories add nothing
// to a project overview and dominate the output when untracked.
func isConsoleNoisePath(path string) bool {
	lower := strings.ToLower(path)
	for _, prefix := range []string{"node_modules/", "dist/", "lib/", "build/", ".git/", "logs/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, name := range []string{"node_modules", "dist", "lib", "build", ".env", ".DS_Store"} {
		if lower == name {
			return true
		}
	}
	return false
}

func (Manager) PM2Logs(root string, page int) (PM2LogPage, error) {
	if _, err := projectPath(root); err != nil {
		return PM2LogPage{}, err
	}
	if page < 1 {
		page = 1
	}
	const pageSize = 120
	requested := page * pageSize
	pm2Name, pm2Args := pm2Launcher(root)
	output, err := run(root, pm2Name, append(pm2Args, "logs", "--lines", strconv.Itoa(requested), "--nostream")...)
	if err != nil {
		return PM2LogPage{}, pm2UnavailableHint(fmt.Errorf("无法读取 PM2 日志：%w", err))
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	return paginatePM2LogLines(lines, page), nil
}

// PM2Process is one process from the local PM2 daemon, surfaced to the UI so
// the PM2 status panel can render a real table instead of raw CLI text.
type PM2Process struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Status    string  `json:"status"`
	PID       int     `json:"pid"`
	CWD       string  `json:"cwd"`
	Memory    int64   `json:"memory"`
	CPU       float64 `json:"cpu"`
	Uptime    int64   `json:"uptime"`
	Restarts  int     `json:"restarts"`
	Script    string  `json:"script"`
	ErrorLog  string  `json:"errorLog,omitempty"`
	OutputLog string  `json:"outputLog,omitempty"`
}

// PackageScript is one declared package.json script safe to present to the
// workbench. The command is descriptive only; execution remains server-side.
type PackageScript struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

func (Manager) PackageScripts(root string) ([]PackageScript, error) {
	path, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return nil, errors.New("无法读取 package.json")
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, errors.New("package.json 格式无法识别")
	}
	items := make([]PackageScript, 0, len(manifest.Scripts))
	for name, command := range manifest.Scripts {
		if name = strings.TrimSpace(name); name != "" && strings.TrimSpace(command) != "" {
			items = append(items, PackageScript{Name: name, Command: command})
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left].Name < items[right].Name })
	return items, nil
}

// UpdatePackageScript changes one declared script without accepting arbitrary
// package.json content from the browser.
func (Manager) UpdatePackageScript(root, previousName, name, command string) error {
	path, err := projectPath(root)
	if err != nil {
		return err
	}
	previousName, name, command = strings.TrimSpace(previousName), strings.TrimSpace(name), strings.TrimSpace(command)
	if !regexp.MustCompile(`^[A-Za-z0-9:_-]{1,100}$`).MatchString(name) {
		return errors.New("脚本名称只能包含字母、数字、冒号、下划线和连字符")
	}
	if command == "" || len(command) > 4096 {
		return errors.New("脚本命令不能为空，且不能超过 4096 个字符")
	}
	file := filepath.Join(path, "package.json")
	data, err := os.ReadFile(file)
	if err != nil {
		return errors.New("无法读取 package.json")
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		return errors.New("package.json 格式无法识别")
	}
	scripts := map[string]string{}
	if raw := manifest["scripts"]; len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &scripts); err != nil {
			return errors.New("package.json 的 scripts 格式无法识别")
		}
	}
	if previousName != "" && previousName != name {
		if _, ok := scripts[previousName]; !ok {
			return errors.New("要修改的脚本不存在")
		}
		if _, exists := scripts[name]; exists {
			return errors.New("目标脚本名称已存在")
		}
		delete(scripts, previousName)
	}
	scripts[name] = command
	encoded, _ := json.Marshal(scripts)
	manifest["scripts"] = encoded
	output, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("保存 package.json 失败：%w", err)
	}
	if err := os.WriteFile(file, append(output, '\n'), 0o644); err != nil {
		return fmt.Errorf("保存 package.json 失败：%w", err)
	}
	return nil
}

// PM2Processes lists every process managed by the local PM2 daemon. Unlike
// PM2Status it does not filter by this robot's directory, so the panel can
// show the full picture; the UI highlights processes for the current root.
func (Manager) PM2Processes(root string) ([]PM2Process, error) {
	if _, err := projectPath(root); err != nil {
		return nil, err
	}
	output, err := pm2JList(root)
	if err != nil {
		return nil, fmt.Errorf("无法读取 PM2 进程：%w", err)
	}
	return parsePM2Processes(output)
}

// PM2ProjectProcesses returns only the PM2 entries whose working directory is
// the selected robot. The global list remains available for the system-wide
// PM2 window, while runtime cards must not mix in other projects.
func (m Manager) PM2ProjectProcesses(root string) ([]PM2Process, error) {
	items, err := m.PM2Processes(root)
	if err != nil {
		return nil, err
	}
	result := make([]PM2Process, 0, len(items))
	for _, item := range items {
		if sameWorkspacePath(item.CWD, root) {
			result = append(result, item)
		}
	}
	return result, nil
}

// pm2JList reads `pm2 jlist` and tolerates a non-zero exit code: a PM2 daemon
// version mismatch prints warnings and may exit non-zero even though stdout
// still carries a valid JSON array. Those banners can be written to stdout as
// well, so before returning we strip everything before the JSON array so the
// caller always sees a parseable payload.
func pm2JList(root string) (string, error) {
	name, args := pm2Launcher(root)
	timeout := commandTimeout(name, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, append(args, "jlist")...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	applyManagedNodeEnvironment(cmd)
	HideWindow(cmd)
	output, err := cmd.Output()
	text := strings.TrimSpace(string(output))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("操作超时（%s）；请检查网络、登录状态或代理后重试", timeout.Round(time.Second))
	}
	if err != nil && text == "" {
		if commandNotFound(err, text) {
			return text, pm2UnavailableHint(missingCommandAdvice("pm2"))
		}
		return text, pm2UnavailableHint(fmt.Errorf("pm2 jlist 失败：%w", err))
	}
	// Cut any banner/notice that PM2 wrote to stdout ahead of the JSON array
	// (for example ">>>> In-memory PM2 is out-of-date ...").
	return stripPM2Banner(text), nil
}

// pm2Launcher resolves how to run PM2. Project-local installations are
// preferred because their version matches the project lockfile; otherwise the
// bundled PM2 keeps the workbench usable offline; npx remains the last resort
// and downloads only when nothing else is available.
func pm2Launcher(root string) (string, []string) {
	if localPM2(root) {
		return nodeToolPath("npx"), []string{"--no-install", "pm2"}
	}
	if command, args, ok := resources.ToolCommand("pm2"); ok {
		return command, args
	}
	return nodeToolPath("npx"), []string{"--yes", "pm2"}
}

// pm2UnavailableHint attaches the recorded reason when the bundled PM2
// provisioning failed, so offline failures are not reported as a bare npx
// download error.
func pm2UnavailableHint(err error) error {
	if reason := resources.LastProvisionError("pm2"); reason != "" {
		return fmt.Errorf("%w（内置 PM2 安装失败：%s；可能未联网，请联网后重试）", err, reason)
	}
	return err
}

func localPM2(root string) bool {
	_, err := os.Stat(filepath.Join(root, "node_modules", "pm2"))
	return err == nil
}

var pm2ConfigNamePattern = regexp.MustCompile("name:\\s*[`\"]([^`\"]+)[`\"]")

// pm2ConfigAppName extracts the application name declared in a generated
// pm2.config.cjs. The name is the stable identity the workbench uses to match
// a project's PM2 registration. Custom configs that cannot be parsed are left
// alone (empty result).
func pm2ConfigAppName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "pm2.config.cjs"))
	if err != nil {
		return ""
	}
	match := pm2ConfigNamePattern.FindSubmatch(data)
	if len(match) < 2 {
		return ""
	}
	return string(match[1])
}

// pm2StaleRegistration reports whether a registered PM2 app for the given
// config name points at a different working directory than the project root.
func pm2StaleRegistration(processes []PM2Process, appName, root string) bool {
	for _, process := range processes {
		if process.Name != appName {
			continue
		}
		return !sameWorkspacePath(process.CWD, root)
	}
	return false
}

// reconcilePM2Registration deletes a stale PM2 registration whose recorded
// working directory no longer matches the project root, so the next
// start/restart registers the process at the current location instead of
// resurrecting the old path. It reports whether a stale registration was
// removed. jlist failures are ignored so a broken daemon never blocks startup.
func reconcilePM2Registration(root string) (bool, error) {
	appName := pm2ConfigAppName(root)
	if appName == "" {
		return false, nil
	}
	output, err := pm2JList(root)
	if err != nil {
		return false, nil
	}
	processes, err := parsePM2Processes(output)
	if err != nil {
		return false, nil
	}
	// Drop stale registrations of this same project that were left behind by a
	// config rewrite (for example the legacy path-digest name -> stable
	// identity migration). Only stopped/errored apps are removed.
	command, args := pm2Launcher(root)
	removed := false
	for _, stale := range stalePM2SameProject(processes, appName, root) {
		if _, deleteErr := run(root, command, append(args, "delete", stale)...); deleteErr == nil {
			removed = true
		}
	}
	if !pm2StaleRegistration(processes, appName, root) {
		return removed, nil
	}
	args = append(args, "delete", appName)
	if _, deleteErr := run(root, command, args...); deleteErr != nil {
		return true, fmt.Errorf("清理旧 PM2 登记失败（目录已移动）：%w", deleteErr)
	}
	return true, nil
}

// stalePM2SameProject returns PM2 app names registered in the alemonx
// namespace that are leftovers of this project and safe to remove: either a
// stopped/errored registration at the current root with a different name
// (identity rewrite), or a stopped/errored registration whose recorded cwd no
// longer exists and whose readable name matches the current config (directory
// moved before the identity file existed).
func stalePM2SameProject(processes []PM2Process, appName, root string) []string {
	var names []string
	readablePrefix := ""
	if dash := strings.LastIndex(appName, "-"); dash > 0 {
		readablePrefix = appName[:dash]
	}
	for _, process := range processes {
		if process.Name == appName || process.Namespace != "alemonx" {
			continue
		}
		status := strings.ToLower(process.Status)
		if status == "online" || status == "launching" {
			continue
		}
		if sameWorkspacePath(process.CWD, root) {
			names = append(names, process.Name)
			continue
		}
		if readablePrefix != "" && strings.HasPrefix(process.Name, readablePrefix+"-") && !pathExists(process.CWD) {
			names = append(names, process.Name)
		}
	}
	return names
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stripPM2Banner removes any non-JSON prefix (PM2 banner/notice) so the caller
// parses only the JSON array or object that follows.
func stripPM2Banner(text string) string {
	// PM2 prints banners and daemon-spawn notices such as "[PM2] Spawning
	// PM2 daemon ..." to stdout on first run. Only a real JSON array start
	// ("[{...}]" or "[]") marks the payload.
	for index := 0; index < len(text); index++ {
		if text[index] != '[' {
			continue
		}
		next := byte(0)
		if index+1 < len(text) {
			next = text[index+1]
		}
		if next == '{' || next == ']' {
			return text[index:]
		}
	}
	if start := strings.IndexByte(text, '{'); start >= 0 {
		return text[start:]
	}
	return text
}

func parsePM2Processes(output string) ([]PM2Process, error) {
	var raw []struct {
		PID      int    `json:"pid"`
		Name     string `json:"name"`
		PMID     int    `json:"pm_id"`
		Status   string `json:"status"`
		Restarts int    `json:"restart_time"`
		PM2Env   struct {
			Script    string `json:"script"`
			Namespace string `json:"namespace"`
			Status    string `json:"status"`
			Uptime    int64  `json:"pm_uptime"`
			ErrorLog  string `json:"pm_err_log_path"`
			OutputLog string `json:"pm_out_log_path"`
			CWD       string `json:"pm_cwd"`
		} `json:"pm2_env"`
		Monit struct {
			Memory int64   `json:"memory"`
			CPU    float64 `json:"cpu"`
		} `json:"monit"`
	}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("无法解析 PM2 进程：%w", err)
	}
	processes := make([]PM2Process, 0, len(raw))
	for _, p := range raw {
		status := p.PM2Env.Status
		if status == "" {
			// Older PM2 releases exposed the state at the top level. Current
			// jlist payloads keep it in pm2_env, so accept both shapes.
			status = p.Status
		}
		processes = append(processes, PM2Process{
			ID:        p.PMID,
			Name:      p.Name,
			Namespace: p.PM2Env.Namespace,
			Status:    strings.ToLower(status),
			PID:       p.PID,
			CWD:       p.PM2Env.CWD,
			Memory:    p.Monit.Memory,
			CPU:       p.Monit.CPU,
			Uptime:    p.PM2Env.Uptime,
			Restarts:  p.Restarts,
			Script:    p.PM2Env.Script,
			ErrorLog:  p.PM2Env.ErrorLog,
			OutputLog: p.PM2Env.OutputLog,
		})
	}
	return processes, nil
}

func (Manager) PM2Status(root string) (PM2Status, error) {
	path, err := projectPath(root)
	if err != nil {
		return PM2Status{}, err
	}
	if _, err := os.Stat(filepath.Join(path, "pm2.config.cjs")); err != nil {
		if os.IsNotExist(err) {
			return PM2Status{}, nil
		}
		return PM2Status{}, fmt.Errorf("无法读取 PM2 配置：%w", err)
	}
	// pm2 jlist emits a JSON array on stdout; PM2's own warnings (for example
	// "In-memory PM2 is out-of-date") go to stderr. pm2JList reads stdout only
	// and tolerates a non-zero exit so banner text cannot corrupt the payload.
	output, err := pm2JList(path)
	if err != nil {
		return PM2Status{}, fmt.Errorf("无法读取 PM2 状态：%w", err)
	}
	return parsePM2Status(path, output)
}

func parsePM2Status(root, output string) (PM2Status, error) {
	var processes []struct {
		PM2Env struct {
			CWD    string `json:"pm_cwd"`
			Status string `json:"status"`
		} `json:"pm2_env"`
	}
	if err := json.Unmarshal([]byte(output), &processes); err != nil {
		return PM2Status{}, fmt.Errorf("无法解析 PM2 状态：%w", err)
	}
	for _, process := range processes {
		if !sameWorkspacePath(process.PM2Env.CWD, root) {
			continue
		}
		status := strings.ToLower(process.PM2Env.Status)
		return PM2Status{
			Configured: true,
			Managed:    true,
			Running:    status == "online" || status == "launching",
			Status:     status,
		}, nil
	}
	return PM2Status{Configured: true, Status: "not-found"}, nil
}

func paginatePM2LogLines(lines []string, page int) PM2LogPage {
	if page < 1 {
		page = 1
	}
	const pageSize = 120
	end := len(lines) - (page-1)*pageSize
	if end <= 0 {
		return PM2LogPage{Output: "没有更早的 PM2 日志。", Page: page}
	}
	start := end - pageSize
	if start < 0 {
		start = 0
	}
	result := strings.Join(lines[start:end], "\n")
	if result == "" {
		result = "PM2 暂无可读取的日志。"
	}
	return PM2LogPage{Output: result, Page: page, HasOlder: start > 0}
}

// DevelopmentCommand prepares the project's declared development command for
// the web server to supervise. Its stdout and stderr stay attached to the
// operation record, so the UI can show progress without exposing a shell.
func (Manager) DevelopmentCommand(root string) (*exec.Cmd, error) {
	return (Manager{}).scriptCommand(root, "dev")
}

// ScriptCommand creates a package-manager command for one declared script.
// It never accepts an arbitrary shell command.
func (Manager) ScriptCommand(root, script string) (*exec.Cmd, error) {
	return (Manager{}).scriptCommand(root, script)
}

// ApplicationCommand resolves the foreground app command for consumers that
// open a runnable application surface. Development mode is an explicit runtime
// choice and must never be selected merely because an application or test
// window was opened.
func (Manager) ApplicationCommand(root string) (*exec.Cmd, string, error) {
	if (Manager{}).HasScript(root, "app") {
		command, err := (Manager{}).ForegroundCommand(root)
		return command, "app", err
	}
	return nil, "", errors.New("未找到 app 前台启动脚本；请先在“运行”中诊断并修复")
}

// HasScript reports whether package.json declares a non-empty script. Keeping
// this check before process creation avoids treating a package-manager error as
// a successfully started development session.
func (Manager) HasScript(root, script string) bool {
	path, err := projectPath(root)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	return json.Unmarshal(data, &manifest) == nil && strings.TrimSpace(manifest.Scripts[script]) != ""
}

// ForegroundCommand runs the project's declared `app` script under the same
// supervised terminal used for development mode.
func (Manager) ForegroundCommand(root string) (*exec.Cmd, error) {
	return (Manager{}).scriptCommand(root, "app")
}

func (Manager) scriptCommand(root, script string) (*exec.Cmd, error) {
	if err := project(root); err != nil {
		return nil, err
	}
	if !(Manager{}).HasScript(root, script) {
		return nil, fmt.Errorf("package.json 未配置 %q 启动脚本", script)
	}
	if script == "dev" {
		if err := fixLegacyLvyScript(root); err != nil {
			return nil, err
		}
	}
	command, _ := PackageManagerCommand(root, "run", script)
	if filepath.Base(command.Path) == "npx" {
		if system.ManagedNodeCommand("npx") == "" {
			if _, err := exec.LookPath("npx"); err != nil {
				return nil, missingCommandAdvice("npx")
			}
		}
	}
	return command, nil
}

func (Manager) RepairRuntime(root, mode string) (Result, error) {
	repair, err := (Manager{}).ApplyRuntimeRepair(root, mode, true)
	if err != nil {
		return Result{Output: repair.Output, Path: root}, err
	}
	return Result{Path: root, Output: repair.Output}, nil
}

func (m Manager) Read(root, name string) (Result, error) {
	path, err := file(root, name)
	if err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("读取机器人目录")
		}
		return Result{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// These two files are editable project configuration. A new robot
		// project legitimately may not have created either one yet; present an
		// empty document so the editor can create it on save.
		if errors.Is(err, os.ErrNotExist) && (name == "alemon.config.yaml" || name == ".npmrc" || name == ".env") {
			return Result{Path: path, Output: ""}, nil
		}
		if permissionError(err) {
			return Result{}, permissionAdvice("读取 " + filepath.Base(path))
		}
		return Result{}, fmt.Errorf("读取失败：%w", err)
	}
	return Result{Path: path, Output: string(data)}, nil
}
func (m Manager) Write(root, name, content string) (Result, error) {
	path, err := file(root, name)
	if err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("访问机器人目录")
		}
		return Result{}, err
	}
	if filepath.Base(path) == ".npmrc" && looksLikeYAMLConfig(content) {
		return Result{}, errors.New(".npmrc 内容看起来是 YAML 配置（例如 alemon.config.yaml 被误存到了 .npmrc）。npm 无法识别这类内容并会把其中的值打印到日志；请改存到 alemon.config.yaml")
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if !permissionError(err) {
			return Result{}, fmt.Errorf("保存 %s 失败：%w", filepath.Base(path), err)
		}
		return Result{}, permissionAdvice("保存 " + filepath.Base(path))
	}
	return Result{Path: path, Output: "已保存。"}, nil
}

var yamlKeyValuePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+\s*:\s`)

// looksLikeYAMLConfig detects lines such as `mysql:` or `host: value` that
// belong in alemon.config.yaml, not in an npm .npmrc file. npm would treat
// them as unknown config and echo the values into process logs.
func looksLikeYAMLConfig(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if yamlKeyValuePattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}
func (m Manager) Run(root, action, message, packageName, version, tag, token string, confirmed bool) (Result, error) {
	if action == "git-release" {
		if _, err := workspacePath(root); permissionError(err) {
			return Result{}, permissionAdvice("访问 Git 工作区")
		}
		// message carries the source commit selected in the Git publishing card.
		var request struct {
			Branch    string   `json:"branch"`
			Commit    string   `json:"commit"`
			Artifacts []string `json:"artifacts"`
		}
		if json.Unmarshal([]byte(message), &request) == nil && request.Commit != "" {
			if len(request.Artifacts) == 0 {
				return Result{}, errors.New("请至少选择一个最终发布产物")
			}
			return GitPublishWithOptions(root, version, request.Branch, request.Commit, request.Artifacts, confirmed)
		}
		return GitPublish(root, version, message, confirmed)
	}
	if err := project(root); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("访问机器人目录")
		}
		return Result{}, err
	}
	dependencyOutput := ""
	if map[string]bool{
		"build": true, "dev": true, "app": true, "pm2": true,
		"pm2-restart": true, "pm2-reload": true,
	}[action] {
		var dependencyErr error
		dependencyOutput, dependencyErr = m.EnsureRuntimeDependencies(root)
		if dependencyErr != nil {
			return Result{Path: root, Output: dependencyOutput}, dependencyErr
		}
	}
	// A robot directory may have been moved since the last PM2 start. The PM2
	// daemon keeps the old absolute cwd in its registration, so delete a stale
	// registration before start/restart/reload re-registers it here.
	if map[string]bool{"pm2": true, "pm2-restart": true, "pm2-reload": true}[action] {
		relocated, relocateErr := reconcilePM2Registration(root)
		if relocateErr != nil {
			return Result{Path: root, Output: dependencyOutput}, relocateErr
		}
		if relocated {
			dependencyOutput = strings.TrimSpace(dependencyOutput + "\n检测到机器人目录已移动，已重新登记 PM2 进程。")
		}
	}
	manager := projectPackageManager(root)
	var name string
	var args []string
	switch action {
	case "dependency-status":
		missing, err := (Manager{}).RuntimeDependencies(root)
		if err != nil {
			return Result{}, err
		}
		checks := []string{"依赖按 package.json 中声明的 " + strings.ToUpper(manager) + " 管理。"}
		if len(missing) == 0 {
			checks = append(checks, "已检查全部直接依赖，当前安装完整。")
		} else {
			checks = append(checks, "依赖不完整："+strings.Join(missing, "、"), "请执行“重新安装依赖”后再运行。")
		}
		return Result{Path: root, Output: strings.Join(checks, "\n")}, nil
	case "install":
		output, installErr := m.installRuntimeDependencies(root, "安装依赖")
		return Result{Path: root, Output: output}, installErr
	case "dependency-add", "dependency-add-dev", "dependency-link", "dependency-remove":
		source, sourceErr := dependencyControlSource(packageName, message, action == "dependency-link")
		if sourceErr != nil {
			return Result{}, sourceErr
		}
		verb := strings.TrimPrefix(action, "dependency-")
		if verb == "add-dev" {
			verb = "add"
		}
		var commandErr error
		name, args, commandErr = connectionPackageCommand(root, verb, source)
		if commandErr != nil {
			return Result{}, commandErr
		}
		if action == "dependency-add-dev" {
			switch manager {
			case "npm":
				args = append(args, "--save-dev")
			case "pnpm":
				args = append(args, "--save-dev")
			default:
				args = append(args, "--dev")
			}
		}
	case "upgrade-alemon":
		return (Manager{}).UpgradeAlemonDependencies(root)
	case "build":
		if err := fixLegacyLvyScript(root); err != nil {
			return Result{}, err
		}
		name, args = manager, []string{"run", "build"}
	case "npm-publish":
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(tag) {
			return Result{}, errors.New("npm 标签格式无效")
		}
		// message is the explicitly selected source commit, shared with the
		// Git release flow. NPM receives a tarball made from that revision.
		return (Manager{}).NPMPublish(root, message, tag, token)
	case "npm-version":
		if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
			return Result{}, errors.New("版本号应为 1.2.3")
		}
		name, args = manager, packageVersionArgs(manager, version)
	case "dev":
		if err := fixLegacyLvyScript(root); err != nil {
			return Result{}, err
		}
		name, args = manager, []string{"run", "dev"}
	case "app":
		name, args = manager, []string{"run", "app"}
	case "repair-dev":
		return (Manager{}).RepairRuntime(root, "dev")
	case "repair-app":
		return (Manager{}).RepairRuntime(root, "app")
	case "repair-pm2":
		result, repairErr := (Manager{}).RepairRuntime(root, "pm2")
		if repairErr != nil {
			return result, repairErr
		}
		output, dependencyErr := m.EnsureRuntimeDependencies(root)
		result.Output = strings.TrimSpace(result.Output + "\n" + output)
		return result, dependencyErr
	case "commit":
		if strings.TrimSpace(message) == "" {
			return Result{}, errors.New("请填写本次提交说明")
		}
		name, args = "git", []string{"add", "."}
		if _, err := run(root, name, args...); err != nil {
			return Result{}, err
		}
		name, args = "git", []string{"commit", "-m", message}
	case "pm2":
		name, args = manager, []string{"run", "start"}
	case "pm2-stop":
		name, args = manager, []string{"run", "stop"}
	case "pm2-stop-project":
		processes, processErr := m.PM2ProjectProcesses(root)
		if processErr != nil {
			return Result{}, processErr
		}
		var outputs []string
		for _, process := range processes {
			if process.Status != "online" && process.Status != "launching" {
				continue
			}
			launcher, launcherArgs := pm2Launcher(root)
			output, stopErr := run(root, launcher, append(launcherArgs, "stop", strconv.Itoa(process.ID))...)
			if output != "" {
				outputs = append(outputs, output)
			}
			if stopErr != nil {
				return Result{Path: root, Output: strings.Join(outputs, "\n")}, stopErr
			}
		}
		if len(outputs) == 0 {
			outputs = append(outputs, "当前没有需要停止的后台进程。")
		}
		return Result{Path: root, Output: strings.Join(outputs, "\n")}, nil
	case "pm2-restart":
		name, args = pm2Launcher(root)
		args = append(args, "restart", "pm2.config.cjs", "--update-env")
	case "pm2-reload":
		name, args = pm2Launcher(root)
		args = append(args, "reload", "pm2.config.cjs", "--update-env")
	case "pm2-delete":
		name, args = pm2Launcher(root)
		args = append(args, "delete", "pm2.config.cjs")
	case "pm2-status":
		name, args = pm2Launcher(root)
		args = append(args, "list")
	case "pm2-logs":
		name, args = pm2Launcher(root)
		args = append(args, "logs", "--lines", "120", "--nostream")
	case "pm2-process-start", "pm2-process-stop", "pm2-process-restart", "pm2-process-reload", "pm2-process-delete":
		target, targetErr := pm2ProcessTarget(message)
		if targetErr != nil {
			return Result{}, targetErr
		}
		command := strings.TrimPrefix(action, "pm2-process-")
		name, args = pm2Launcher(root)
		args = append(args, command, target)
		if command == "start" || command == "restart" || command == "reload" {
			args = append(args, "--update-env")
		}
	case "install-package":
		if !allowedInstallPackage(packageName) {
			return Result{}, errors.New("不支持的 AlemonJS 包")
		}
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return installLocalPackage(root, packageName)
		})
	case "uninstall-package":
		if !allowedPackage(packageName) {
			return Result{}, errors.New("不支持的 AlemonJS 包")
		}
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return removeLocalPackage(root, packageName)
		})
	case "remove-local-package":
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return removeLocalPackageByName(root, packageName)
		})
	case "replace-local-package":
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return replaceLocalPackage(root, packageName, version)
		})
	case "switch-local-package-version":
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return switchLocalPackageVersion(root, packageName, version, false)
		})
	case "force-switch-local-package-version":
		if !confirmed {
			return Result{}, errors.New("强制切换会丢弃该插件工作区的本地修改，请确认后继续")
		}
		return m.syncLocalPackageOperation(root, func() (Result, error) {
			return switchLocalPackageVersion(root, packageName, version, true)
		})
	case "enable-backpack-workspace":
		return setBackpackWorkspace(root, true)
	case "disable-backpack-workspace":
		return setBackpackWorkspace(root, false)
	case "install-connection":
		if !allowedInstallPackage(packageName) {
			return Result{}, errors.New("连接包名无效")
		}
		return installConnectionPackage(root, packageName)
	case "uninstall-connection":
		if !allowedPackage(packageName) {
			return Result{}, errors.New("连接包名无效")
		}
		return removeConnectionPackage(root, packageName)
	case "install-module":
		if !allowedInstallPackage(packageName) {
			return Result{}, errors.New("模块包名无效")
		}
		return installModulePackage(root, packageName)
	case "uninstall-module":
		if !allowedPackage(packageName) {
			return Result{}, errors.New("模块包名无效")
		}
		return removeModulePackage(root, packageName)
	case "git-init":
		if _, err := run(root, "git", "init"); err != nil {
			return Result{}, err
		}
		name, args = "git", []string{"branch", "-M", "main"}
	default:
		return Result{}, errors.New("未知的机器人操作")
	}
	var output string
	var runErr error
	if name == manager {
		output, runErr = runPackageManager(root, args...)
	} else {
		output, runErr = run(root, name, args...)
	}
	if runErr == nil && (action == "pm2" || action == "pm2-restart" || action == "pm2-reload") {
		saveName, saveArgs := pm2Launcher(root)
		saved, saveErr := run(root, saveName, append(saveArgs, "save")...)
		output = strings.TrimSpace(output + "\n" + saved)
		if saveErr != nil {
			return Result{Path: root, Output: output}, fmt.Errorf("PM2 已启动，但保存重启恢复清单失败：%w", saveErr)
		}
		output = strings.TrimSpace(output + "\nPM2 进程清单已保存；请在服务器上完成一次 PM2 startup 配置以支持主机重启恢复。")
	}
	return Result{Path: root, Output: strings.TrimSpace(dependencyOutput + "\n" + output)}, runErr
}

func dependencyControlSource(packageName, version string, link bool) (string, error) {
	packageName, version = strings.TrimSpace(packageName), strings.TrimSpace(version)
	if packageName == "" || strings.HasPrefix(packageName, "-") || strings.ContainsAny(packageName, "\r\n\t ") {
		return "", errors.New("请填写有效的包名或链接目标")
	}
	if link && version != "" {
		return "", errors.New("链接依赖不支持填写版本")
	}
	if version == "" {
		return packageName, nil
	}
	if strings.ContainsAny(version, "\r\n\t ") {
		return "", errors.New("版本不能包含空白字符")
	}
	return packageName + "@" + version, nil
}

// pm2ProcessTarget accepts only a PM2 numeric process id before it is used as
// a command argument. The processes endpoint exposes the daemon-wide list, so
// UI actions must never forward an arbitrary name or shell-like expression.
func pm2ProcessTarget(value string) (string, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id < 0 {
		return "", errors.New("PM2 进程 ID 无效")
	}
	return strconv.Itoa(id), nil
}

func (m Manager) syncLocalPackageOperation(root string, operation func() (Result, error)) (Result, error) {
	result, operationErr := operation()
	if operationErr != nil {
		return result, operationErr
	}
	output, dependencyErr := m.SyncWorkspaceDependencies(root)
	result.Output = strings.TrimSpace(result.Output + "\n" + output)
	return result, dependencyErr
}

func allowedInstallPackage(name string) bool {
	if allowedPackage(name) {
		return true
	}
	if at := strings.LastIndex(name, "@"); at > strings.LastIndex(name, "/") {
		return allowedPackage(name[:at])
	}
	return false
}

func allowedPackage(name string) bool {
	if strings.HasPrefix(name, "git+https://github.com/") || strings.HasPrefix(name, "git+https://gitee.com/") {
		return true
	}
	// Packages are executed through the selected package manager without a
	// shell. Accept a normal npm name so a user can register a custom platform,
	// but reject flags, paths and arbitrary command text.
	return regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`).MatchString(name)
}
func project(root string) error {
	_, err := projectPath(root)
	return err
}

func projectPath(root string) (string, error) {
	if root == "." {
		current, err := os.Getwd()
		if err != nil {
			return "", errors.New("无法读取当前运行目录")
		}
		root = current
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("请选择完整的机器人文件夹路径")
	}
	info, err := os.Stat(root)
	if err != nil {
		if permissionError(err) {
			return "", fmt.Errorf("无法访问机器人文件夹：%w", err)
		}
		return "", errors.New("机器人文件夹不存在")
	}
	if !info.IsDir() {
		return "", errors.New("机器人文件夹不存在")
	}
	if _, err := os.Stat(filepath.Join(root, "package.json")); err != nil {
		if permissionError(err) {
			return "", fmt.Errorf("无法读取机器人 package.json：%w", err)
		}
		return "", errors.New("该文件夹不是可管理的 Node.js 机器人项目（缺少 package.json）")
	}
	return root, nil
}

func managedProjectFile(root, name string) (string, error) {
	projectRoot, err := projectPath(root)
	if err != nil {
		return "", err
	}
	relative := filepath.Clean(name)
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || blockedProjectPath(relative) {
		return "", errors.New("不允许访问该项目文件")
	}
	target := filepath.Join(projectRoot, relative)
	directory := filepath.Dir(target)
	directoryRelative, err := filepath.Rel(projectRoot, directory)
	if err != nil {
		return "", errors.New("目标文件不在机器人项目中")
	}
	current := projectRoot
	for _, part := range strings.Split(filepath.Clean(directoryRelative), string(filepath.Separator)) {
		if part != "." {
			current = filepath.Join(current, part)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("无法检查项目路径：%w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("不允许通过符号链接访问项目文件")
		}
	}
	return target, nil
}

func blockedProjectPath(name string) bool {
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		lower := strings.ToLower(part)
		if lower == ".git" || lower == "node_modules" || lower == ".npmrc" || lower == ".env" || strings.HasPrefix(lower, ".env.") || strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") || strings.HasSuffix(lower, ".p12") || strings.HasSuffix(lower, ".pfx") {
			return true
		}
	}
	return false
}

func permissionError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return os.IsPermission(err) || strings.Contains(text, "eacces") || strings.Contains(text, "permission denied") || strings.Contains(text, "access is denied")
}

// permissionAdvice deliberately stays inside the web operation flow.  A
// background install or save must never surprise a beginner with an operating
// system administrator dialog.  The dashboard preserves this exact message in
// its operation record and toast, alongside the directory picker guidance.
func permissionAdvice(action string) error {
	if system.InContainer() {
		return fmt.Errorf("没有权限%s。当前运行在容器内：官方镜像以 root 运行，请确认宿主机挂载目录未被设为只读、Docker Desktop 已共享该目录；若你自定义为非 root 用户运行，请确保挂载目录对该用户（uid 1000）可写", action)
	}
	return fmt.Errorf("没有权限%s。请在系统设置中为 alx 授予该磁盘或文件夹的访问权限（macOS：\"文件与文件夹\"或\"完全磁盘访问\"），或选择当前登录账户可读写的目录后重试", action)
}
func file(root, name string) (string, error) {
	if err := project(root); err != nil {
		return "", err
	}
	if name != ".npmrc" && name != ".env" && name != "alemon.config.yaml" && name != "README.md" {
		return "", errors.New("不支持的文件")
	}
	return filepath.Join(root, name), nil
}

// fixLegacyLvyScript migrates old project templates in the robot project's
// own package.json. It deliberately never traverses node_modules or edits a
// dependency manifest.
func fixLegacyLvyScript(root string) error {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("无法读取 package.json：%w", err)
	}
	fixed := strings.ReplaceAll(string(data), "npx lvy ", "lvy ")
	if fixed == string(data) {
		return nil
	}
	if err := os.WriteFile(path, []byte(fixed), 0644); err != nil {
		if permissionError(err) {
			return permissionAdvice("更新开发脚本")
		}
		return fmt.Errorf("无法更新开发脚本：%w", err)
	}
	return nil
}

func run(root, name string, args ...string) (string, error) {
	return runWithEnv(root, nil, name, args...)
}

// nodeToolPath keeps the bundled runtime usable by a GUI or service process
// whose own PATH was established before Node.js was installed. Only bare
// node/npm/npx names are resolved so explicit user command paths stay intact.
func nodeToolPath(name string) string {
	if filepath.Base(name) != name || (name != "node" && name != "npm" && name != "npx") {
		return name
	}
	if _, err := system.ResolveCommand("node"); err != nil {
		return name
	}
	system.RefreshCommandEnvironment("node", "npm", "npx")
	if path, err := system.ResolveCommand(name); err == nil {
		return path
	}
	return name
}

func applyManagedNodeEnvironment(command *exec.Cmd) {
	bin := system.ManagedNodeBin()
	if bin == "" {
		return
	}
	if command.Env == nil {
		command.Env = os.Environ()
	}
	prefix := "PATH="
	value := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	for index := len(command.Env) - 1; index >= 0; index-- {
		if strings.HasPrefix(command.Env[index], prefix) {
			command.Env[index] = prefix + value
			return
		}
	}
	command.Env = append(command.Env, prefix+value)
}

// PackageManagerCommand keeps a robot usable when package.json requests a
// package manager that is not globally installed. npx uses the npm bundled
// with Node.js and needs no administrator write permission.
func PackageManagerCommand(root string, args ...string) (*exec.Cmd, string) {
	return packageManagerCommand(root, projectPackageManager(root), args...)
}

func packageManagerCommand(root, manager string, args ...string) (*exec.Cmd, string) {
	if path := nodeToolPath(manager); path != manager {
		command := exec.Command(path, args...)
		command.Dir = root
		applyManagedNodeEnvironment(command)
		HideWindow(command)
		return command, ""
	}
	if _, err := exec.LookPath(manager); err == nil {
		command := exec.Command(manager, args...)
		command.Dir = root
		applyManagedNodeEnvironment(command)
		HideWindow(command)
		return command, ""
	}
	if manager == "yarn" {
		if command, prefix, ok := resources.ToolCommand("yarn"); ok {
			executable := exec.Command(command, append(prefix, args...)...)
			executable.Dir = root
			applyManagedNodeEnvironment(executable)
			HideWindow(executable)
			return executable, "使用内置 Yarn 运行"
		}
	}
	packageName := ""
	switch manager {
	case "yarn":
		packageName = "yarn@1.22.22"
	case "pnpm":
		packageName = "pnpm@latest"
	default:
		command := exec.Command(manager, args...)
		command.Dir = root
		applyManagedNodeEnvironment(command)
		HideWindow(command)
		return command, ""
	}
	command := exec.Command(nodeToolPath("npx"), append([]string{"--yes", packageName}, args...)...)
	command.Dir = root
	applyManagedNodeEnvironment(command)
	HideWindow(command)
	return command, "未找到 " + strings.ToUpper(manager) + "，临时通过 npm 获取并执行；不会修改电脑的全局安装。"
}

func runPackageManager(root string, args ...string) (string, error) {
	return runNamedPackageManager(root, projectPackageManager(root), args...)
}

func packageVersionArgs(manager, version string) []string {
	if manager == "yarn" {
		return []string{"version", "--new-version", version, "--no-git-tag-version"}
	}
	return []string{"version", version, "--no-git-tag-version"}
}

func runNamedPackageManager(root, manager string, args ...string) (string, error) {
	command, notice := packageManagerCommand(root, manager, args...)
	values := packageManagerEnvironment(root)
	if filepath.Base(command.Path) == "npx" {
		// npm reads project .npmrc before it launches temporary Yarn/PNPM.
		// Settings owned by another package manager (for example node-linker)
		// are harmless, and must not bury the actual operation result.
		values["NPM_CONFIG_LOGLEVEL"] = "error"
	}
	output, err := runWithEnv(root, values, command.Path, command.Args[1:]...)
	if notice != "" {
		if output != "" {
			output = notice + "\n" + output
		} else {
			output = notice
		}
	}
	return output, err
}

// packageManagerEnvironment makes GUI-launched setup processes honour the
// user's Node version manager. On macOS a process launched before nvm is
// loaded often keeps Homebrew's non-LTS Node in PATH, while the terminal has
// the intended LTS version. Prefer .nvmrc, then nvm's default alias.
func packageManagerEnvironment(root string) map[string]string {
	values := map[string]string{}
	if bin := preferredNVMNodeBin(root); bin != "" {
		values["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	} else if bin := system.ManagedNodeBin(); bin != "" {
		values["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	return values
}

func preferredNVMNodeBin(root string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	wanted := ""
	if data, err := os.ReadFile(filepath.Join(root, ".nvmrc")); err == nil {
		wanted = strings.TrimPrefix(strings.TrimSpace(string(data)), "v")
	}
	if wanted == "" {
		if data, err := os.ReadFile(filepath.Join(home, ".nvm", "alias", "default")); err == nil {
			wanted = strings.TrimPrefix(strings.TrimSpace(string(data)), "v")
		}
	}
	if wanted == "" || strings.ContainsAny(wanted, "/ ") {
		return ""
	}
	versions, err := os.ReadDir(filepath.Join(home, ".nvm", "versions", "node"))
	if err != nil {
		return ""
	}
	candidates := []string{}
	for _, entry := range versions {
		name := strings.TrimPrefix(entry.Name(), "v")
		if entry.IsDir() && nodeVersionMatches(name, wanted) {
			if info, err := os.Stat(filepath.Join(home, ".nvm", "versions", "node", entry.Name(), "bin", "node")); err == nil && !info.IsDir() {
				candidates = append(candidates, entry.Name())
			}
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool { return nodeVersionGreater(candidates[i], candidates[j]) })
	return filepath.Join(home, ".nvm", "versions", "node", candidates[0], "bin")
}

func nodeVersionMatches(version, wanted string) bool {
	version, wanted = strings.TrimPrefix(version, "v"), strings.TrimPrefix(wanted, "v")
	if version == wanted {
		return true
	}
	return regexp.MustCompile(`^\d+$`).MatchString(wanted) && strings.HasPrefix(version, wanted+".")
}

func nodeVersionGreater(left, right string) bool {
	var l1, l2, l3, r1, r2, r3 int
	_, _ = fmt.Sscanf(strings.TrimPrefix(left, "v"), "%d.%d.%d", &l1, &l2, &l3)
	_, _ = fmt.Sscanf(strings.TrimPrefix(right, "v"), "%d.%d.%d", &r1, &r2, &r3)
	for _, pair := range [][2]int{{l1, r1}, {l2, r2}, {l3, r3}} {
		if pair[0] != pair[1] {
			return pair[0] > pair[1]
		}
	}
	return false
}

func runWithEnv(root string, values map[string]string, name string, args ...string) (string, error) {
	return runWithOutput(root, values, true, name, args...)
}

func runWithOutput(root string, values map[string]string, combined bool, name string, args ...string) (string, error) {
	timeout := commandTimeout(name, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, nodeToolPath(name), args...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	applyManagedNodeEnvironment(cmd)
	HideWindow(cmd)
	for key, value := range values {
		prefix := key + "="
		for index := len(cmd.Env) - 1; index >= 0; index-- {
			if strings.HasPrefix(cmd.Env[index], prefix) {
				cmd.Env[index] = prefix + value
				prefix = ""
				break
			}
		}
		if prefix != "" {
			cmd.Env = append(cmd.Env, prefix+value)
		}
	}
	var raw []byte
	var err error
	if combined {
		raw, err = cmd.CombinedOutput()
	} else {
		raw, err = cmd.Output()
	}
	text := strings.TrimSpace(string(raw))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("操作超时（%s）；请检查网络、登录状态或代理后重试", timeout.Round(time.Second))
	}
	if err != nil {
		if permissionError(err) || permissionError(errors.New(text)) {
			return text, permissionAdvice("执行 " + filepath.Base(name))
		}
		if commandNotFound(err, text) {
			return text, missingCommandAdvice(filepath.Base(name))
		}
		if text != "" {
			return text, fmt.Errorf("%s：%w", text, err)
		}
		return text, fmt.Errorf("执行 %s 失败：%w", strings.Join(append([]string{filepath.Base(name)}, args...), " "), err)
	}
	return text, nil
}

func commandTimeout(name string, args ...string) time.Duration {
	base := strings.ToLower(filepath.Base(name))
	if base == "git" && len(args) > 0 {
		switch args[0] {
		case "clone", "fetch", "pull", "push", "ls-remote":
			return 10 * time.Minute
		}
	}
	if base == "npm" || base == "yarn" || base == "pnpm" || base == "npx" {
		return 20 * time.Minute
	}
	// Bundled tools run through `node <workspace>/packages/<name>/...` and
	// should keep the generous package-manager timeout instead of the generic
	// command timeout.
	if base == "node" && len(args) > 0 && strings.Contains(filepath.ToSlash(args[0]), "/packages/") {
		return 20 * time.Minute
	}
	return 2 * time.Minute
}

func commandNotFound(err error, output string) bool {
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	text := strings.ToLower(err.Error() + "\n" + output)
	return strings.Contains(text, "executable file not found") || strings.Contains(text, "command not found") || strings.Contains(text, "is not recognized as an internal or external command")
}

func missingCommandAdvice(name string) error {
	switch strings.ToLower(name) {
	case "node", "npm", "npx", "yarn", "pnpm":
		return errors.New("未检测到 Node.js 运行环境（含 npm/npx）。请先在左上角“环境”中安装 Node.js LTS，完成后重新执行；Yarn 和 PNPM 无需全局安装，alx 会临时执行它们")
	case "git":
		return errors.New("未检测到 Git。请先在左上角“环境”中安装 Git，完成后重新执行")
	default:
		return fmt.Errorf("未检测到 %s 命令。请安装对应的系统工具后重新执行", name)
	}
}
