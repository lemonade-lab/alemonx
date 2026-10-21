package systemnetwork

import (
	"os/exec"
	"path"
	"strings"

	"alemonx/internal/packagemanager"
)

type packageCommand struct {
	tool       string
	start      int
	operation  string
	bootstrap  int // insertion point for npx/npm exec options; -1 if absent
	modernYarn bool
}

// commandBase also accepts Windows command paths when inspecting a command on
// another host. Exact tool names keep arbitrary node scripts out of this path.
func commandBase(value string) string {
	name := strings.ToLower(path.Base(strings.ReplaceAll(value, `\`, "/")))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".js", ".cjs", ".mjs"} {
		name = strings.TrimSuffix(name, suffix)
	}
	switch name {
	case "npm-cli":
		return "npm"
	case "npx-cli":
		return "npx"
	}
	if strings.HasPrefix(name, "yarn-") && len(name) > 5 && name[5] >= '0' && name[5] <= '9' {
		return "yarn"
	}
	return name
}

func packageToolName(value string) string {
	name, _, _ := strings.Cut(value, "@")
	switch name {
	case "yarn", "npm", "pnpm", "npx", "corepack":
		return name
	}
	return ""
}

// firstCommand skips only known value-taking launcher/global options, then
// stops at the command. It must never search script arguments for "install".
func firstCommand(args []string, start int) int {
	for i := start; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return i + 1
		}
		if !strings.HasPrefix(arg, "-") {
			return i
		}
		switch arg {
		case "--cwd", "--dir", "-C", "--prefix", "--registry", "--proxy", "--https-proxy", "--userconfig", "--package", "-p", "--cache", "--config.proxy", "--config.https-proxy":
			i++
		}
	}
	return len(args)
}

func inspectPackageCommand(cmd *exec.Cmd) packageCommand {
	p := packageCommand{bootstrap: -1, start: 1}
	tool := commandBase(cmd.Path)
	if tool == "node" && len(cmd.Args) > 1 {
		tool, p.start = commandBase(cmd.Args[1]), 2
	}
	if packageToolName(tool) == "" {
		return p
	}
	for depth := 0; depth < 3; depth++ {
		index := firstCommand(cmd.Args, p.start)
		if tool == "corepack" {
			if index >= len(cmd.Args) {
				return p
			}
			tool, p.start = packageToolName(cmd.Args[index]), index+1
			continue
		}
		if tool == "npx" || tool == "npm" && index < len(cmd.Args) && (cmd.Args[index] == "exec" || cmd.Args[index] == "x") {
			p.bootstrap = p.start
			if tool == "npm" {
				index = firstCommand(cmd.Args, index+1)
			}
			if index < len(cmd.Args) {
				tool, p.start = packageToolName(cmd.Args[index]), index+1
				continue
			}
			tool = ""
		}
		p.tool = tool
		if index < len(cmd.Args) {
			p.operation = cmd.Args[index]
		}
		// A workspace invocation owns the nested subcommand, not the name.
		if (p.tool == "yarn" || p.tool == "pnpm") && p.operation == "workspace" && index+2 < len(cmd.Args) {
			p.operation = cmd.Args[index+2]
			p.start = index + 2
		}
		p.modernYarn = p.tool == "yarn" && packagemanager.ModernYarn(cmd.Dir)
		return p
	}
	return p
}

func (p packageCommand) networkOperation() bool {
	switch p.operation {
	case "install", "i", "add", "ci", "view", "info", "show", "publish", "whoami", "pack", "create", "init", "update", "upgrade", "up", "remove", "uninstall", "rm", "link", "unlink", "rebuild", "dlx", "import", "outdated", "audit":
		return p.tool == "npm" || p.tool == "yarn" || p.tool == "pnpm"
	}
	return false
}

func (p packageCommand) mirrorRead() bool {
	// Writes and authentication always stay on the official registry.
	if p.operation == "publish" || p.operation == "whoami" || p.operation == "login" || p.operation == "logout" {
		return false
	}
	return p.bootstrap >= 0 || p.networkOperation() && p.operation != "pack"
}

func insertCommandOptions(cmd *exec.Cmd, index int, options []string) {
	args := append([]string(nil), cmd.Args[:index]...)
	args = append(args, options...)
	cmd.Args = append(args, cmd.Args[index:]...)
}

func (p packageCommand) applyNetworkOptions(cmd *exec.Cmd, values map[string]string, proxyURL string) {
	if p.tool == "yarn" && !p.modernYarn {
		values["YARN_REGISTRY"] = values["npm_config_registry"]
	}
	if p.modernYarn {
		// Berry rejects Classic's CLI flags and YARN_REGISTRY environment key.
		delete(values, "YARN_REGISTRY")
		values["YARN_NPM_REGISTRY_SERVER"] = values["npm_config_registry"]
		values["YARN_HTTP_PROXY"], values["YARN_HTTPS_PROXY"] = proxyURL, proxyURL
	}
	if p.networkOperation() && !p.modernYarn {
		prefix := "--"
		if p.tool == "pnpm" {
			prefix = "--config."
		}
		proxyValue := proxyURL
		if p.tool == "npm" && proxyValue == "" {
			proxyValue = "false"
		}
		index := len(cmd.Args)
		for i := p.start; i < len(cmd.Args); i++ {
			if cmd.Args[i] == "--" {
				index = i
				break
			}
		}
		insertCommandOptions(cmd, index, []string{prefix + "registry=" + values["npm_config_registry"], prefix + "proxy=" + proxyValue, prefix + "https-proxy=" + proxyValue})
	}
	if p.bootstrap >= 0 {
		// npx stops parsing options at the first positional argument. Appending
		// here would pass npm flags to lvy, PM2, or a project's build script.
		proxyValue := proxyURL
		if proxyValue == "" {
			proxyValue = "false"
		}
		insertCommandOptions(cmd, p.bootstrap, []string{"--registry=" + values["npm_config_registry"], "--proxy=" + proxyValue, "--https-proxy=" + proxyValue})
	}
}
