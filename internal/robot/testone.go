package robot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defaultTestPort is the AlemonJS CBP/sandbox port used when alemon.config.yaml
// declares no top-level port. The testone sandbox endpoint (沙盒测试平台) is
// served by the robot's CBP server on the same port at /testone.
const defaultTestPort = 17117

// TestPortInfo describes the robot's test (CBP/sandbox) port: the configured
// top-level port in alemon.config.yaml and whether it was explicitly declared.
type TestPortInfo struct {
	Port       int  `json:"port"`
	Configured bool `json:"configured"`
}

// TestPort returns the robot's test port from alemon.config.yaml (top-level
// `port`) and whether it was explicitly configured.
func (Manager) TestPort(root string) (TestPortInfo, error) {
	project, err := projectPath(root)
	if err != nil {
		return TestPortInfo{}, err
	}
	data, err := os.ReadFile(filepath.Join(project, "alemon.config.yaml"))
	if err != nil {
		return TestPortInfo{Port: defaultTestPort}, nil
	}
	// Only a YAML top-level port is the robot's CBP/test port.
	if match := regexp.MustCompile(`(?m)^port\s*:\s*['\"]?(\d+)`).FindStringSubmatch(string(data)); len(match) == 2 {
		if configured, parseErr := strconv.Atoi(match[1]); parseErr == nil && configured > 0 && configured < 65536 {
			return TestPortInfo{Port: configured, Configured: true}, nil
		}
	}
	return TestPortInfo{Port: defaultTestPort}, nil
}

// SaveTestPort writes the top-level port into alemon.config.yaml, replacing an
// existing value or appending a new one.
func (Manager) SaveTestPort(root string, port int) (Result, error) {
	if port < 1 || port > 65535 {
		return Result{}, errors.New("测试端口应在 1-65535 之间")
	}
	project, err := projectPath(root)
	if err != nil {
		return Result{}, err
	}
	configFile := filepath.Join(project, "alemon.config.yaml")
	content, err := os.ReadFile(configFile)
	if err != nil && !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("无法读取运行配置：%w", err)
	}
	text := string(content)
	value := "port: " + strconv.Itoa(port)
	pattern := regexp.MustCompile(`(?m)^port\s*:\s*['\"]?\d+['\"]?\s*$`)
	if pattern.MatchString(text) {
		text = pattern.ReplaceAllString(text, value)
	} else {
		text = strings.TrimRight(text, "\n")
		if text != "" {
			text += "\n"
		}
		text += value + "\n"
	}
	if err := os.WriteFile(configFile, []byte(text), 0644); err != nil {
		if permissionError(err) {
			return Result{}, permissionAdvice("保存测试端口")
		}
		return Result{}, fmt.Errorf("无法保存测试端口：%w", err)
	}
	return Result{Path: configFile, Output: "测试端口已设置为 " + strconv.Itoa(port) + "。"}, nil
}

// TestSandboxAvailable reports whether the robot is configured to start in
// AlemonJS sandbox mode (no platform/login), which the /testone endpoint
// requires. login/platform can also arrive via CLI or env at run time, so this
// is a configuration heuristic, not a runtime guarantee.
func (Manager) TestSandboxAvailable(root string) (bool, error) {
	project, err := projectPath(root)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(project, "alemon.config.yaml"))
	if err != nil {
		// 没有配置时框架默认进入沙盒模式。
		return true, nil
	}
	if match := regexp.MustCompile(`(?m)^\s*(login|platform)\s*:\s*['\"]?[^\s'\"]+`).FindStringSubmatch(string(data)); len(match) == 2 {
		return false, nil
	}
	return true, nil
}

// TestPortReachable probes whether the robot's CBP/sandbox server is actually
// listening on its configured test port. The browser connects through the
// backend WebSocket proxy (same origin), so the backend performs the health
// check against the loopback port before the test center opens.
func (m Manager) TestPortReachable(root string) (bool, int, error) {
	info, err := m.TestPort(root)
	if err != nil {
		return false, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(info.Port)+"/api/online", nil)
	if err != nil {
		return false, info.Port, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, info.Port, nil
	}
	defer response.Body.Close()
	// Any HTTP response (even an error status) means a server is listening on
	// the port; the CBP server answers /api/online with a JSON health payload.
	return true, info.Port, nil
}
