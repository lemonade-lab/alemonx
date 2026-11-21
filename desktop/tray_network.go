package main

import (
	"encoding/json"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const trayNetworkMessagePrefix = "alx:tray-network:"

type trayNetworkState struct {
	Mode            string `json:"mode"`
	ProxyConfigured bool   `json:"proxyConfigured"`
	Error           string `json:"error"`
}

type trayNetworkMenu struct {
	app   *application.App
	items map[string]*application.MenuItem
	state trayNetworkState
}

func newTrayNetworkMenu(app *application.App, menu *application.Menu, window *application.WebviewWindow, openSettings func()) *trayNetworkMenu {
	network := &trayNetworkMenu{app: app, items: make(map[string]*application.MenuItem)}
	submenu := menu.AddSubmenu("网络模式")
	for _, item := range []struct{ mode, label string }{
		{"auto", "自动"}, {"direct", "直连"}, {"proxy", "代理"},
	} {
		mode := item.mode
		detail, _ := json.Marshal(mode)
		network.items[mode] = submenu.AddRadio(item.label, false).OnClick(func(*application.Context) {
			application.InvokeAsync(func() {
				// Wails checks radio items on click. Keep the confirmed mode checked until the save succeeds.
				network.updateItems()
				if network.state.Mode != mode && network.items[mode].Enabled() {
					window.ExecJS("window.dispatchEvent(new CustomEvent('alx:tray-network-select', {detail:" + string(detail) + "}))")
				}
			})
		})
	}
	submenu.AddSeparator()
	submenu.Add("网络设置…").OnClick(func(*application.Context) { openSettings() })
	network.updateItems()
	return network
}

func (m *trayNetworkMenu) updateItems() {
	for mode, item := range m.items {
		item.SetChecked(mode == m.state.Mode)
		item.SetEnabled(m.state.Mode != "" && (mode != "proxy" || m.state.ProxyConfigured))
	}
	proxyLabel := "代理"
	if m.state.Mode != "" && !m.state.ProxyConfigured {
		proxyLabel = "代理（未配置）"
	}
	m.items["proxy"].SetLabel(proxyLabel)
}

func (m *trayNetworkMenu) receive(payload string) {
	var state trayNetworkState
	if len(payload) > 4096 || json.Unmarshal([]byte(payload), &state) != nil {
		return
	}
	if _, valid := m.items[state.Mode]; !valid {
		state.Mode = ""
	}
	application.InvokeAsync(func() {
		m.state = state
		m.updateItems()
		if state.Error != "" {
			dialog := m.app.Dialog.Error().SetTitle("无法切换网络模式").SetMessage(state.Error)
			dialog.AddButton("关闭")
			dialog.Show()
		}
	})
}
