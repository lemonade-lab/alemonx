package main

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"
)

const workbenchCredentialService = "com.alemonjs.xiuxian.alemonx.desktop.workbench"
const gitHubDeviceURL = "https://github.com/login/device"

var credentialRequestID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func handleWorkbenchAuthMessage(app *application.App, source application.Window, workbenchURL, message string) bool {
	if message == "alx:github-device:open" {
		_ = app.Browser.OpenURL(gitHubDeviceURL)
		return true
	}
	payload, ok := strings.CutPrefix(message, "alx:workbench-credential:")
	if !ok {
		return false
	}
	var request struct {
		ID, Action, Account, Password string
	}
	if len(payload) > 4096 || json.Unmarshal([]byte(payload), &request) != nil ||
		!credentialRequestID.MatchString(request.ID) || request.Account == "" || len(request.Account) > 256 {
		return true
	}
	go func() {
		var result struct {
			ID       string `json:"id"`
			Password string `json:"password,omitempty"`
			Error    string `json:"error,omitempty"`
		}
		result.ID = request.ID
		var err error
		switch request.Action {
		case "get":
			result.Password, err = keyring.Get(workbenchCredentialService, request.Account)
		case "set":
			err = keyring.Set(workbenchCredentialService, request.Account, request.Password)
		case "delete":
			err = keyring.Delete(workbenchCredentialService, request.Account)
		default:
			return
		}
		if err != nil && !errors.Is(err, keyring.ErrNotFound) {
			result.Error = "无法访问系统密码管理器。"
		}
		encoded, _ := json.Marshal(result)
		origin, _ := json.Marshal(workbenchURL)
		source.ExecJS("if (location.origin === " + string(origin) + " && !location.pathname.startsWith('/api/')) window.dispatchEvent(new CustomEvent('alx:workbench-credential', {detail:" + string(encoded) + "}))")
	}()
	return true
}
