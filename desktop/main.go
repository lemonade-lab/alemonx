package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets/tray.png
var trayIcon []byte

func main() {
	if len(os.Args) == 3 && os.Args[1] == desktopInstallFlag {
		if err := runDesktopInstaller(os.Args[2]); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}
	updater := desktopUpdater{}
	workspace := flag.String("workspace", "", "工作区目录，也可通过 ALX_WORKSPACE 指定")
	port := flag.Int("port", 17392, "本地工作台端口")
	flag.Parse()

	var window *application.WebviewWindow
	var workbenchURL string
	var auxiliary auxiliaryWindows
	var trayNetwork *trayNetworkMenu
	var restartCommand atomic.Pointer[exec.Cmd]
	var openWorkbench, restartWorkbench, quitWorkbench func()
	app := application.New(application.Options{
		Name:        "ALemonX Desktop",
		Description: "AlemonX 工作台",
		KeyBindings: windowKeyBindings(),
		PostShutdown: func() {
			// Wails releases the single-instance lock before running this hook.
			if cmd := restartCommand.Load(); cmd != nil {
				if err := cmd.Start(); err != nil {
					log.Printf("无法重启 ALemonX Desktop：%v", err)
					if len(cmd.Args) == 3 && cmd.Args[1] == desktopInstallFlag {
						_ = os.WriteFile(desktopUpdateResultPath(), []byte("无法启动桌面安装程序："+err.Error()), 0600)
						if executable, executableErr := os.Executable(); executableErr == nil {
							_ = startUpdatedDesktop(desktopInstallPlan{Executable: executable, Arguments: os.Args[1:]})
						}
					}
					return
				}
				_ = cmd.Process.Release()
			}
		},
		RawMessageHandler: func(source application.Window, message string, origin *application.OriginInfo) {
			if !isWorkbenchMessage(origin, workbenchURL) {
				return
			}
			if payload, found := strings.CutPrefix(message, trayNetworkMessagePrefix); found {
				if trayNetwork != nil && source == window {
					trayNetwork.receive(payload)
				}
				return
			}
			if action, found := strings.CutPrefix(message, "alx:desktop-update:"); found {
				settings, exists := appWindowSettings(auxiliary.app)
				if exists && source == settings {
					go func() {
						state := updater.handle(action, func(cmd *exec.Cmd) bool {
							if !restartCommand.CompareAndSwap(nil, cmd) {
								return false
							}
							application.InvokeAsync(auxiliary.app.Quit)
							return true
						})
						if action == "install" && state.Error == "" {
							return
						}
						result, _ := json.Marshal(struct {
							desktopUpdateState
							Action string `json:"action"`
						}{state, action})
						source.ExecJS("window.dispatchEvent(new CustomEvent('alx:desktop-update', {detail:" + string(result) + "}))")
					}()
				}
				return
			}
			if handleWorkbenchAuthMessage(auxiliary.app, source, workbenchURL, message) {
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
	if runtime.GOOS == "darwin" {
		menu.AddSectionHeader("工作台")
	}
	menu.Add("打开").OnClick(func(*application.Context) { openWorkbench() })
	if runtime.GOOS == "darwin" {
		trayNetwork = newTrayNetworkMenu(app, menu, window, func() {
			auxiliary.openFromMessage("alx:open-window:settings:network")
		})
		menu.AddSeparator()
	}
	menu.Add("重启").OnClick(func(*application.Context) { restartWorkbench() })
	menu.Add("退出").OnClick(func(*application.Context) { quitWorkbench() })
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("ALemonX Desktop")
	tray.SetMenu(menu)
	if runtime.GOOS == "darwin" {
		app.Event.RegisterApplicationEventHook(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
			// Wails otherwise shows every hidden window when the Dock reopens the app.
			event.Cancel()
			application.InvokeAsync(func() {
				if !window.IsVisible() || window.IsMinimised() {
					openWorkbench()
				}
			})
		})

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
		windowMenu := applicationMenu.AddSubmenu("窗口")
		windowMenu.AddRole(application.CloseWindow)
		windowMenu.FindByRole(application.CloseWindow).SetLabel("关闭窗口")
		windowMenu.AddRole(application.Minimise)
		windowMenu.FindByRole(application.Minimise).SetLabel("最小化窗口")
		windowMenu.Add("最大化或还原窗口").SetAccelerator("CmdOrCtrl+Shift+m").OnClick(func(*application.Context) {
			if focused := app.Window.Current(); focused != nil {
				focused.ToggleMaximise()
			}
		})
		app.Menu.SetApplicationMenu(applicationMenu)
	}

	app.OnShutdown(backend.stop)
	app.OnShutdown(updater.discardStagedUpdate)
	if err := app.Run(); err != nil {
		log.Print(err)
	}
}

func appWindowSettings(app *application.App) (application.Window, bool) {
	if app == nil {
		return nil, false
	}
	return app.Window.GetByName("settings")
}
