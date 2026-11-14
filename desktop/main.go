package main

import (
	_ "embed"
	"flag"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets/tray.png
var trayIcon []byte

func main() {
	workspace := flag.String("workspace", "", "工作区目录，也可通过 ALX_WORKSPACE 指定")
	port := flag.Int("port", 17392, "本地工作台端口")
	flag.Parse()

	var window *application.WebviewWindow
	var trayWindow *application.WebviewWindow
	var workbenchURL string
	var auxiliary auxiliaryWindows
	var restartCommand atomic.Pointer[exec.Cmd]
	var openWorkbench, restartWorkbench, quitWorkbench func()
	app := application.New(application.Options{
		Name:        "ALemonX Desktop",
		Description: "AlemonX 工作台",
		PostShutdown: func() {
			// Wails releases the single-instance lock before running this hook.
			if cmd := restartCommand.Load(); cmd != nil {
				if err := cmd.Start(); err != nil {
					log.Printf("无法重启 ALemonX Desktop：%v", err)
					return
				}
				_ = cmd.Process.Release()
			}
		},
		RawMessageHandler: func(source application.Window, message string, origin *application.OriginInfo) {
			if !isWorkbenchMessage(origin, workbenchURL) {
				return
			}
			if trayWindow != nil && source == trayWindow {
				switch message {
				case "alx:tray:open":
					openWorkbench()
				case "alx:tray:restart":
					restartWorkbench()
				case "alx:tray:quit":
					quitWorkbench()
				case "alx:tray:terminal":
					trayWindow.Hide()
					window.ExecJS("window.dispatchEvent(new Event('alx:desktop-terminal-toggle'))")
				case "alx:tray:developer-mode":
					window.ExecJS("window.dispatchEvent(new Event('alx:desktop-developer-mode-toggle'))")
				}
				return
			}
			auxiliary.openFromMessage(message)
			if runtime.GOOS == "windows" || source != window && message == "alx:window:close" {
				handleCaptionAction(source, message)
			}
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.alemonjs.xiuxian.alemonx.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
	})
	backend, err := startWorkbench(*workspace, *port)
	if err != nil {
		log.Print(err)
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			dialog := app.Dialog.Error().SetTitle("无法启动 AlemonX").SetMessage(err.Error())
			dialog.AddButton("退出").OnClick(app.Quit)
			dialog.Show()
		})
		if runErr := app.Run(); runErr != nil {
			log.Fatal(runErr)
		}
		os.Exit(1)
	}
	defer backend.stop()
	workbenchURL = backend.url

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "ALemonX Desktop",
		Width:     1280,
		Height:    800,
		MinWidth:  640,
		MinHeight: 480,
		URL:       backend.url,
		Frameless: runtime.GOOS == "windows",
		Mac: application.MacWindow{
			TitleBar: application.MacTitleBarHiddenInset,
		},
	})
	auxiliary = auxiliaryWindows{app: app, baseURL: backend.url, mainWindow: window}
	// Linux desktops can lack a tray host. Keep normal window closing available.
	if runtime.GOOS != "linux" {
		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			window.Hide()
		})
	}
	openWorkbench = func() {
		if trayWindow != nil {
			trayWindow.Hide()
		}
		revealWindow(window)
	}
	restartWorkbench = func() {
		executable, err := os.Executable()
		if err != nil {
			dialog := app.Dialog.Error().SetTitle("无法重启 ALemonX").SetMessage(err.Error())
			dialog.AddButton("关闭")
			dialog.Show()
			return
		}
		cmd := exec.Command(executable, os.Args[1:]...)
		cmd.Stderr = os.Stderr
		if restartCommand.CompareAndSwap(nil, cmd) {
			app.Quit()
		}
	}
	quitWorkbench = app.Quit
	menu := app.NewMenu()
	menu.Add("打开").OnClick(func(*application.Context) { openWorkbench() })
	menu.Add("重启").OnClick(func(*application.Context) { restartWorkbench() })
	menu.Add("退出").OnClick(func(*application.Context) { quitWorkbench() })
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("ALemonX Desktop")
	tray.SetMenu(menu)
	if runtime.GOOS == "darwin" {
		applicationMenu := app.NewMenu()
		appMenu := applicationMenu.AddSubmenu("ALemonX")
		appMenu.Add("打开工作台").OnClick(func(*application.Context) { openWorkbench() })
		appMenu.Add("重启应用").OnClick(func(*application.Context) { restartWorkbench() })
		appMenu.AddSeparator()
		appMenu.Add("退出").SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) { quitWorkbench() })

		editMenu := applicationMenu.AddSubmenu("编辑")
		editMenu.AddRole(application.Undo)
		editMenu.AddRole(application.Redo)
		editMenu.AddSeparator()
		editMenu.AddRole(application.Cut)
		editMenu.AddRole(application.Copy)
		editMenu.AddRole(application.Paste)
		editMenu.AddRole(application.SelectAll)
		for _, item := range []struct {
			role  application.Role
			label string
		}{
			{application.Undo, "撤销"},
			{application.Redo, "重做"},
			{application.Cut, "剪切"},
			{application.Copy, "复制"},
			{application.Paste, "粘贴"},
			{application.SelectAll, "全选"},
		} {
			editMenu.FindByRole(item.role).SetLabel(item.label)
		}
		app.Menu.SetApplicationMenu(applicationMenu)

		trayWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: "tray-panel", Title: "ALemonX",
			Width: 316, Height: 211,
			URL:    backend.url + "/desktop-window/tray-panel",
			Hidden: true, Frameless: true, DisableResize: true, AlwaysOnTop: true,
			Mac:             application.MacWindow{Backdrop: application.MacBackdropTransparent},
			HideOnFocusLost: true, HideOnEscape: true,
			DefaultContextMenuDisabled: true,
		})
		tray.AttachWindow(trayWindow)
	}

	app.OnShutdown(backend.stop)
	if err := app.Run(); err != nil {
		log.Print(err)
	}
}
