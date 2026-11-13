package main

import (
	_ "embed"
	"flag"
	"log"
	"os"
	"runtime"

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
	var workbenchURL string
	var auxiliary auxiliaryWindows
	app := application.New(application.Options{
		Name:        "AlemonX Desktop",
		Description: "AlemonX 工作台",
		RawMessageHandler: func(source application.Window, message string, origin *application.OriginInfo) {
			if !isWorkbenchMessage(origin, workbenchURL) {
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
		Title:     "AlemonX Desktop",
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
	menu := app.NewMenu()
	menu.Add("打开工作台").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("退出 AlemonX Desktop").OnClick(func(*application.Context) {
		app.Quit()
	})
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("AlemonX Desktop")
	tray.SetMenu(menu)

	app.OnShutdown(backend.stop)
	if err := app.Run(); err != nil {
		log.Print(err)
	}
}
