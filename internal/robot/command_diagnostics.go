package robot

import (
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	commandANSI     = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	yarnVersionLine = regexp.MustCompile(`\byarn(?: [a-z-]+)? v([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`)
	yarnStageLine   = regexp.MustCompile(`\[([0-9]+/[0-9]+)\][ \t]*([^\r\n]*)`)
)

// os/exec serializes writes when stdout and stderr use the same writer.
// Keep the original output while tracking progress separately so warning-heavy
// tails cannot hide the last stage when the error response is truncated.
type packageCommandOutput struct {
	buffer     bytes.Buffer
	pending    string
	version    string
	stage      string
	started    time.Time
	lastOutput time.Time
}

func (o *packageCommandOutput) Write(p []byte) (int, error) {
	o.lastOutput = time.Now()
	line := o.pending + commandANSI.ReplaceAllString(string(p), "")
	if matches := yarnVersionLine.FindAllStringSubmatch(line, -1); len(matches) > 0 {
		o.version = matches[len(matches)-1][1]
	}
	if matches := yarnStageLine.FindAllStringSubmatch(line, -1); len(matches) > 0 {
		match := matches[len(matches)-1]
		// Only retain Yarn's fixed phase label, never arbitrary script output
		// or request URLs that might contain credentials.
		phase := strings.TrimSpace(match[2])
		for _, known := range []string{"Validating package.json", "Resolving packages", "Fetching packages", "Linking dependencies", "Building fresh packages"} {
			if strings.HasPrefix(phase, known) {
				o.stage = "[" + match[1] + "] " + known
			}
		}
	}
	o.pending = line
	if index := strings.LastIndexAny(line, "\r\n"); index >= 0 {
		o.pending = line[index+1:]
	}
	if len(o.pending) > 4096 {
		o.pending = o.pending[len(o.pending)-4096:]
	}
	return o.buffer.Write(p)
}

func (o *packageCommandOutput) Bytes() []byte  { return o.buffer.Bytes() }
func (o *packageCommandOutput) String() string { return o.buffer.String() }

func packageCommandEntry(cmd *exec.Cmd) string {
	base := strings.ToLower(filepath.Base(cmd.Path))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".exe"), ".cmd")
	if base == "node" && len(cmd.Args) > 1 {
		base = strings.ToLower(filepath.Base(cmd.Args[1]))
		base = strings.TrimSuffix(strings.TrimSuffix(base, ".js"), ".cjs")
	}
	switch base {
	case "npm", "npm-cli", "npx", "npx-cli", "yarn", "pnpm", "corepack":
		return base
	}
	return ""
}

func (o *packageCommandOutput) summary(cmd *exec.Cmd, now time.Time) string {
	lines := []string{"依赖操作诊断：", "执行入口：" + packageCommandEntry(cmd), "已运行：" + now.Sub(o.started).Round(time.Second).String()}
	if o.version != "" {
		lines = append(lines, "Yarn 版本："+o.version)
	}
	if o.stage != "" {
		lines = append(lines, "最后识别阶段："+o.stage)
	} else {
		lines = append(lines, "最后识别阶段：未识别，请结合完整安装日志判断")
	}
	if !o.lastOutput.IsZero() {
		lines = append(lines, "距最近输出："+now.Sub(o.lastOutput).Round(time.Second).String())
	} else {
		lines = append(lines, "子进程尚无输出")
	}
	env := make(map[string]string)
	for _, entry := range cmd.Env {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	if registry, err := url.Parse(env["npm_config_registry"]); err == nil && registry.Host != "" {
		// Report only the registry origin; user info, paths and queries can
		// contain private credentials. Never print the temporary proxy URL.
		lines = append(lines, "本次仓库入口："+registry.Scheme+"://"+registry.Host)
	}
	if proxy, ok := env["HTTPS_PROXY"]; ok {
		state := "关闭（忽略环境代理）"
		if proxy != "" {
			state = "已启用受管代理通道"
		}
		lines = append(lines, "本次代理："+state)
	}
	return strings.Join(lines, "\n")
}

func packageTimeoutError(timeout time.Duration, stage string) error {
	advice := "请查看最后识别阶段和完整日志；超时本身不能确定是网络或代理故障。"
	switch {
	case strings.Contains(stage, "Resolving"), strings.Contains(stage, "Fetching"):
		advice += " 当前阶段涉及依赖解析或下载，请核对实际请求地址及本次网络策略。"
	case strings.Contains(stage, "Linking"):
		advice += " 当前阶段涉及文件写入和链接，请检查磁盘空间、文件占用及安全软件扫描。"
	case strings.Contains(stage, "Building"):
		advice += " 当前阶段涉及依赖安装脚本或原生模块构建，请查看具体脚本输出。"
	}
	return fmt.Errorf("依赖操作超时（%s）；%s", timeout.Round(time.Second), advice)
}
