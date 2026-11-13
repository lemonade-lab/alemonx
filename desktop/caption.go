package main

import (
	"net/url"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func isWorkbenchMessage(origin *application.OriginInfo, workbenchURL string) bool {
	if origin == nil || workbenchURL == "" {
		return false
	}
	if runtime.GOOS == "darwin" && !origin.IsMainFrame {
		return false
	}
	if runtime.GOOS == "windows" && origin.Origin != origin.TopOrigin {
		return false
	}
	page, err := url.Parse(origin.Origin)
	return err == nil && page.Scheme+"://"+page.Host == workbenchURL && !strings.HasPrefix(page.Path, "/api/")
}

func handleCaptionAction(window application.Window, message string) {
	switch message {
	case "alx:window:minimise":
		window.Minimise()
	case "alx:window:toggle-maximise":
		window.ToggleMaximise()
	case "alx:window:close":
		window.Close()
	}
}
