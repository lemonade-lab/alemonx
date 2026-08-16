package system

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"alemonx/internal/resources"
)

var (
	developmentUserHomeDir              = os.UserHomeDir
	developmentSystemCommandDirectories = unixCommandDirectories
)

// PrepareDevelopmentCommand resolves a source-plugin command in the same
// environment a developer normally uses. Service managers do not load shell
// profiles, so Node version managers and package-manager directories must be
// added explicitly instead of relying on an interactive SSH session.
//
// Yarn and pnpm have a portable fallback: Corepack from the selected Node
// runtime, then npx. Neither fallback installs a global package or modifies
// the user's shell configuration.
func PrepareDevelopmentCommand(program string, args []string) (string, []string, []string, string) {
	environment := DevelopmentCommandEnvironment()
	if path, err := ResolveDevelopmentCommand(program); err == nil {
		return path, append([]string(nil), args...), environment, ""
	}
	if program != "yarn" && program != "pnpm" {
		return program, append([]string(nil), args...), environment, ""
	}
	// The bundled Yarn works offline, so it outranks Corepack/npx which may
	// need the registry on first use.
	if program == "yarn" {
		if command, prefix, ok := resources.ToolCommand("yarn"); ok {
			return command, append(prefix, args...), environment, "未找到独立的 Yarn，已使用内置 Yarn。"
		}
	}
	if corepack, err := ResolveDevelopmentCommand("corepack"); err == nil {
		return corepack, append([]string{program}, args...), environment, "未找到独立的 " + program + "，已使用 Node.js 自带的 Corepack。"
	}
	if npx, err := ResolveDevelopmentCommand("npx"); err == nil {
		packageName := program + "@latest"
		if program == "yarn" {
			packageName = "yarn@1.22.22"
		}
		return npx, append([]string{"--yes", packageName}, args...), environment, "未找到独立的 " + program + "，本次将通过 Node.js 自动运行它。"
	}
	return program, append([]string(nil), args...), environment, ""
}

// DevelopmentCommandEnvironment returns a complete child environment for a
// source plugin. It never changes the machine-wide PATH.
func DevelopmentCommandEnvironment() []string {
	directories := developmentCommandDirectories()
	entries := filepath.SplitList(os.Getenv("PATH"))
	merged := make([]string, 0, len(directories)+len(entries))
	seen := map[string]bool{}
	for _, directory := range append(directories, entries...) {
		if directory == "" {
			continue
		}
		key := filepath.Clean(directory)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			merged = append(merged, directory)
		}
	}
	path := "PATH=" + strings.Join(merged, string(os.PathListSeparator))
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PATH=") {
			environment = append(environment, value)
		}
	}
	return append(environment, path)
}

func developmentCommandDirectories() []string {
	directories := []string{}
	if bin := ManagedNodeBin(); bin != "" {
		directories = append(directories, bin)
	}
	if runtime.GOOS == "windows" {
		return append(directories, windowsCommandDirectories("node")...)
	}
	home, _ := developmentUserHomeDir()
	if home != "" {
		directories = append(directories,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".yarn", "bin"),
			filepath.Join(home, ".config", "yarn", "global", "node_modules", ".bin"),
			filepath.Join(home, ".local", "share", "pnpm"),
			filepath.Join(home, ".volta", "bin"),
			filepath.Join(home, ".asdf", "shims"),
		)
		for _, pattern := range []string{
			filepath.Join(home, ".nvm", "versions", "node", "*", "bin"),
			filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "bin"),
		} {
			matches, _ := filepath.Glob(pattern)
			directories = append(directories, matches...)
		}
	}
	return append(directories, developmentSystemCommandDirectories("node")...)
}

// ResolveDevelopmentCommand is useful to callers that need an executable
// path before creating an exec.Cmd.
func ResolveDevelopmentCommand(name string) (string, error) {
	// Respect an explicit service PATH first. If it does not name the tool,
	// prefer the user's development runtime directories over system defaults.
	if path, ok := commandInDirectories(name, filepath.SplitList(os.Getenv("PATH"))); ok {
		return path, nil
	}
	if path, ok := commandInDirectories(name, developmentCommandDirectories()); ok {
		return path, nil
	}
	return "", exec.ErrNotFound
}

func commandInDirectories(name string, directories []string) (string, bool) {
	names := []string{name}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		names = append(names, name+".exe", name+".cmd", name+".bat")
	}
	for _, directory := range directories {
		for _, candidate := range names {
			path := filepath.Join(directory, candidate)
			if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return path, true
			}
		}
	}
	return "", false
}
