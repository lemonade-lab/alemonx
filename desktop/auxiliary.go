package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type auxiliaryWindows struct {
	app        *application.App
	baseURL    string
	mainWindow *application.WebviewWindow
	pendingGit sync.Map
}

func (a *auxiliaryWindows) openFromMessage(message string) {
	request, ok := strings.CutPrefix(message, "alx:open-window:")
	if !ok {
		return
	}
	name, section, _ := strings.Cut(request, ":")
	if len(section) > 16000 {
		return
	}
	var browserCommand string
	var browserClose bool
	focusExisting := true
	var title string
	var width int
	windowKey := name
	suffix := ""
	switch name {
	case "settings":
		title, width = "设置", 820
		switch section {
		case "", "general", "auth", "accounts", "github", "ssh", "mcp", "network", "update", "service", "dependency-sources":
		default:
			return
		}
	case "data":
		title, width = "数据管理", 1080
		if section != "" {
			return
		}
	case "operation-log":
		title, width = "操作日志", 740
		if section != "" && section != "update" {
			return
		}
		focusExisting = section != "update"
	case "browser":
		title, width = "浏览器", 1180
		if section != "" {
			var command struct {
				ID    string `json:"id"`
				Kind  string `json:"kind"`
				URL   string `json:"url"`
				TabID string `json:"tabID"`
			}
			if json.Unmarshal([]byte(section), &command) != nil || command.ID == "" {
				return
			}
			switch command.Kind {
			case "open":
				if command.URL == "" {
					return
				}
			case "close":
				if command.TabID == "" {
					return
				}
				browserClose = true
			default:
				return
			}
			encoded, _ := json.Marshal(section)
			browserCommand = "window.dispatchEvent(new CustomEvent('alx:native-browser-command', {detail: JSON.parse(" + string(encoded) + ")}))"
		}
	case "main-project":
		params, err := url.ParseQuery(section)
		if err != nil || params.Get("id") == "" {
			return
		}
		encoded, _ := json.Marshal(params.Get("id"))
		application.InvokeAsync(func() {
			a.mainWindow.ExecJS("window.dispatchEvent(new CustomEvent('alx:native-open-project', {detail: " + string(encoded) + "}))")
			revealWindow(a.mainWindow)
		})
		return
	case "terminal", "foreground-logs", "pm2-logs", "robot-app", "git", "pm2-processes", "test", "live", "ops", "ops-overview", "tasks", "environment", "plugins", "plugin":
		params, err := url.ParseQuery(section)
		if err != nil {
			return
		}
		switch name {
		case "terminal":
			title, width = "终端", 820
		case "foreground-logs":
			title, width = "前台日志", 900
		case "pm2-logs":
			title, width = "PM2 运行日志", 1000
		case "git":
			root := params.Get("root")
			if root == "" {
				return
			}
			title, width = "Git", 1100
			// Display metadata must not change the window identity.
			windowKey = fmt.Sprintf("git-%x", sha256.Sum256([]byte(root)))
		case "pm2-processes":
			title, width = "PM2 进程", 1050
		case "test":
			title, width = "机器人测试", 1180
		case "live":
			title, width = "在线聊天", 1180
		case "ops":
			title, width = "运维工具", 1080
		case "ops-overview":
			title, width = "运维总览", 1080
		case "tasks":
			title, width = "任务记录", 1080
		case "environment":
			title, width = "环境检查", 1080
		case "plugins":
			title, width = "插件", 1080
		case "plugin":
			title, width = "插件页面", 1080
			if params.Get("id") == "" {
				return
			}
		case "robot-app":
			title, width = "机器人应用", 1080
			target, err := url.Parse(params.Get("path"))
			if err != nil || target.IsAbs() || target.Host != "" || !strings.HasPrefix(target.Path, "/") {
				return
			}
			clean := path.Clean(target.Path)
			if !strings.HasPrefix(clean, "/api/v1/robot/app/") && !strings.HasPrefix(clean, "/api/v1/robot/webview/") {
				return
			}
		}
		if name != "git" {
			identity := params.Get("root") + "\x00" + params.Get("taskId") + "\x00" + params.Get("path") + "\x00" + params.Get("id")
			windowKey = fmt.Sprintf("%s-%x", name, sha256.Sum256([]byte(identity)))
		}
		suffix = "?" + params.Encode()
	default:
		return
	}
	target := a.baseURL + "/desktop-window/" + name + suffix
	if name == "settings" && section != "" {
		target += "#section=" + url.QueryEscape(section)
	}
	if name == "git" {
		// Wails delivers raw messages concurrently. Keep one pending open per
		// repository, from queueing through completion on the UI thread.
		if _, pending := a.pendingGit.LoadOrStore(windowKey, struct{}{}); pending {
			return
		}
	}
	application.InvokeAsync(func() {
		if name == "git" {
			defer a.pendingGit.Delete(windowKey)
		}
		if existing, found := a.app.Window.GetByName(windowKey); found {
			if name == "settings" && section != "" {
				existing.SetURL(target)
			}
			if browserCommand != "" {
				existing.ExecJS(browserCommand)
			}
			if !browserClose && focusExisting {
				revealWindow(existing)
			}
			return
		}
		if browserClose {
			return
		}
		if browserCommand != "" {
			target += "#command=" + url.QueryEscape(section)
		}
		window := a.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: windowKey, Title: title, URL: target, Width: width, Height: 640,
			MinWidth: 640, MinHeight: 480,
			Frameless: runtime.GOOS == "windows",
			Mac:       application.MacWindow{TitleBar: application.MacTitleBarHiddenInset},
		})
		if name == "terminal" || name == "browser" || name == "test" || name == "live" {
			window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
				event.Cancel()
				window.Hide()
			})
		}
	})
}

func revealWindow(window application.Window) {
	if window.IsMinimised() {
		window.UnMinimise()
	}
	window.Show()
	window.Focus()
}
