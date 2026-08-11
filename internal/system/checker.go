// Package system contains safe, read-only checks for local prerequisites.
package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Check struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
}
type Report struct {
	GoalID    string  `json:"goalId"`
	Ready     bool    `json:"ready"`
	Platform  string  `json:"platform"`
	Checks    []Check `json:"checks"`
	CheckedAt string  `json:"checkedAt"`
}
type Checker struct{ timeout time.Duration }

func NewChecker() *Checker { return &Checker{timeout: 5 * time.Second} }

func (c *Checker) CheckGoal(goalID, variant string) Report {
	checks := []Check{c.platform()}
	switch goalID {
	case "install", "develop":
		checks = append(checks, c.command("node", "Node.js", "--version", "请安装 Node.js LTS 版本后重新检查。"), c.command("git", "Git", "--version", "请安装 Git 后重新检查。"))
	case "web":
		if variant == "clean" {
			checks = append(checks, c.command("node", "Node.js", "--version", "请安装 Node.js LTS 版本后重新检查。"), c.command("git", "Git", "--version", "请安装 Git 后重新检查。"))
		} else {
			checks = append(checks, c.command("docker", "Docker", "--version", "请安装并启动 Docker Desktop 后重新检查。"))
		}
	case "mobile":
		// Mobile installation is completed on the phone; the desktop app only checks host support.
	case "build":
		if variant == "git" {
			// Git 发布实际只需要 Node.js 与 Git。Yarn/PNPM 会在执行时
			// 通过 npx 临时运行，jq 也没有参与当前发布链路，不能把它们
			// 误报为新用户必须全局安装的环境。
			checks = append(checks, c.command("node", "Node.js", "--version", "请安装 Node.js LTS 版本后重新检查。"), c.command("git", "Git", "--version", "请安装 Git 后重新检查。"))
		} else {
			checks = append(checks, c.command("node", "Node.js", "--version", "请安装 Node.js LTS 版本后重新检查。"), c.command("npm", "npm", "--version", "请随 Node.js 一并安装 npm 后重新检查。"))
		}
	}
	ready := true
	for _, check := range checks {
		if check.Status != "ready" {
			ready = false
			break
		}
	}
	return Report{goalID, ready, runtime.GOOS + "/" + runtime.GOARCH, checks, time.Now().Format(time.RFC3339)}
}

func (c *Checker) platform() Check {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		return Check{ID: "platform", Name: "当前系统", Status: "ready", Detail: runtime.GOOS + "（" + runtime.GOARCH + "）"}
	default:
		return Check{ID: "platform", Name: "当前系统", Status: "missing", Detail: "暂未支持 " + runtime.GOOS, Suggestion: "请在 Windows、macOS 或 Linux 上运行此工具。"}
	}
}

func (c *Checker) command(id, name, argument, suggestion string) Check {
	previousPath, previousErr := exec.LookPath(id)
	RefreshCommandEnvironment(id)
	path, err := ResolveCommand(id)
	if err != nil {
		return Check{ID: id, Name: name, Status: "missing", Detail: "未检测到", Suggestion: suggestion}
	}
	repaired := previousErr != nil || filepath.Clean(previousPath) != filepath.Clean(path)
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, argument).CombinedOutput()
	if ctx.Err() != nil {
		return Check{ID: id, Name: name, Status: "warning", Detail: "检测超时", Suggestion: "请确认程序可以正常启动后重试。"}
	}
	if err != nil {
		return Check{ID: id, Name: name, Status: "warning", Detail: "已找到，但无法正常运行", Suggestion: "请重新安装或修复 " + name + " 后重试。"}
	}
	version := strings.TrimSpace(strings.Split(string(output), "\n")[0])
	if repaired {
		if version != "" {
			version += " · "
		}
		version += "已自动修复当前服务 PATH"
	}
	return Check{ID: id, Name: name, Status: "ready", Detail: version}
}

// ResolveCommand resolves a prerequisite without relying solely on the PATH
// captured when AlemonX started. The managed Node runtime deliberately wins
// over an older system Node so installs immediately use the LTS chosen here.
func ResolveCommand(name string) (string, error) {
	if path := ManagedNodeCommand(name); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" {
		for _, directory := range windowsCommandDirectories(name) {
			path := filepath.Join(directory, name+".exe")
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
		return "", exec.ErrNotFound
	}
	for _, directory := range unixCommandDirectories(name) {
		path := filepath.Join(directory, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

// RefreshCommandEnvironment updates AlemonX's own process environment after
// an approved install. It does not alter the machine-wide PATH; it makes the
// current service and every child process immediately see the new tool.
func RefreshCommandEnvironment(names ...string) []string {
	directories := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		path, err := ResolveCommand(name)
		if err != nil {
			continue
		}
		directory := filepath.Dir(path)
		key := directory
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			directories = append(directories, directory)
		}
	}
	if len(directories) == 0 {
		return nil
	}
	entries := filepath.SplitList(os.Getenv("PATH"))
	merged := make([]string, 0, len(directories)+len(entries))
	seen = map[string]bool{}
	for _, directory := range append(directories, entries...) {
		key := directory
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if directory != "" && !seen[key] {
			seen[key] = true
			merged = append(merged, directory)
		}
	}
	_ = os.Setenv("PATH", strings.Join(merged, string(os.PathListSeparator)))
	return directories
}

func unixCommandDirectories(name string) []string {
	directories := []string{"/usr/local/bin", "/usr/bin", "/bin"}
	if runtime.GOOS == "darwin" {
		directories = append([]string{"/opt/homebrew/bin", "/Applications/Docker.app/Contents/Resources/bin"}, directories...)
	}
	return directories
}

func windowsCommandDirectories(name string) []string {
	directories := windowsRegistryPathDirectories()
	programFiles := []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles(x86)")}
	switch name {
	case "node", "npm", "npx":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "nodejs"))
			}
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			directories = append(directories, filepath.Join(localAppData, "Programs", "nodejs"))
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			directories = append(directories, filepath.Join(appData, "nvm"))
		}
	case "git":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "Git", "cmd"), filepath.Join(root, "Git", "bin"))
			}
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			directories = append(directories, filepath.Join(localAppData, "Programs", "Git", "cmd"), filepath.Join(localAppData, "Programs", "Git", "bin"))
		}
	case "docker":
		for _, root := range programFiles {
			if root != "" {
				directories = append(directories, filepath.Join(root, "Docker", "Docker", "resources", "bin"))
			}
		}
	}

	seen := make(map[string]bool, len(directories))
	result := make([]string, 0, len(directories))
	for _, directory := range directories {
		directory = strings.Trim(strings.TrimSpace(directory), `"`)
		if directory != "" && !seen[strings.ToLower(directory)] {
			seen[strings.ToLower(directory)] = true
			result = append(result, directory)
		}
	}
	return result
}

// windowsRegistryPathDirectories reads both persistent PATH values. It is kept
// best-effort: the standard install directories above still cover installations
// where access to one of the registry hives is unavailable.
func windowsRegistryPathDirectories() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	directories := []string{}
	for _, key := range []string{
		`HKCU\Environment`,
		`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
	} {
		output, err := exec.CommandContext(ctx, "reg.exe", "query", key, "/v", "Path").Output()
		if err != nil {
			continue
		}
		if value := windowsRegistryPathValue(string(output)); value != "" {
			directories = append(directories, filepath.SplitList(expandWindowsEnvironment(value))...)
		}
	}
	return directories
}

func windowsRegistryPathValue(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if typeIndex := strings.Index(line, "REG_"); typeIndex >= 0 {
			if valueIndex := strings.IndexAny(line[typeIndex:], " \t"); valueIndex >= 0 {
				return strings.TrimSpace(line[typeIndex+valueIndex:])
			}
		}
	}
	return ""
}

func expandWindowsEnvironment(value string) string {
	return os.Expand(expandWindowsPercentVariables(value), func(key string) string { return os.Getenv(key) })
}

func expandWindowsPercentVariables(value string) string {
	for start := strings.IndexByte(value, '%'); start >= 0; {
		endOffset := strings.IndexByte(value[start+1:], '%')
		if endOffset < 0 {
			break
		}
		end := start + endOffset + 1
		key := value[start+1 : end]
		value = value[:start] + os.Getenv(key) + value[end+1:]
		start = strings.IndexByte(value, '%')
	}
	return value
}
