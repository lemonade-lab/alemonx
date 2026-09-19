package systemnetwork

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func initialSettings() storedSettings {
	next, connection, _, _ := expandConnection(Settings{Connection: &RouteSettings{Mode: ModeAuto}}, storedSettings{})
	routes := make(map[Route]storedRouteSettings, len(next.Routes))
	for route, item := range next.Routes {
		routes[route] = storedRouteSettings{Mode: item.Mode, MirrorURL: item.MirrorURL}
	}
	return storedSettings{Connection: connection, Routes: routes}
}

type mirrorRequestKey struct{}

// Compile defaults and exceptions into the existing route table so older clients
// can still submit route-only settings without losing their configuration.
func expandConnection(next Settings, previous storedSettings) (Settings, *storedRouteSettings, []Route, error) {
	input := *next.Connection
	if input.Mode != ModeAuto && input.Mode != ModeDirect && input.Mode != ModeManual {
		return Settings{}, nil, nil, errors.New("默认连接方式无效")
	}
	connection := storedRouteSettings{Mode: input.Mode}
	if input.Mode == ModeManual {
		connection.ProxyURL = strings.TrimSpace(input.ProxyURL)
		if !input.ClearCredentials && previous.Connection != nil && publicRouteSettings(*previous.Connection).ProxyURL == connection.ProxyURL {
			connection.ProxyURL = previous.Connection.ProxyURL
		}
		if err := validate(storedSettings{Routes: map[Route]storedRouteSettings{RouteGitHub: connection}}); err != nil {
			return Settings{}, nil, nil, err
		}
	}
	next.Routes = make(map[Route]RouteSettings, len(allRoutes))
	for route, recommended := range defaultRoutes() {
		item := RouteSettings{Mode: connection.Mode, ProxyURL: connection.ProxyURL, ClearCredentials: true}
		if connection.Mode == ModeAuto {
			item.MirrorURL = recommended.MirrorURL
		}
		next.Routes[route] = item
	}
	var overrides []Route
	for _, route := range allRoutes {
		if item, ok := next.Overrides[route]; ok {
			next.Routes[route] = item
			overrides = append(overrides, route)
		}
	}
	if len(overrides) != len(next.Overrides) {
		return Settings{}, nil, nil, errors.New("未知的资源例外")
	}
	return next, &connection, overrides, nil
}

// Preview operates on a detached snapshot, never the active config or file.
func (m *Manager) Preview(ctx context.Context, route Route, next Settings) (CheckResult, error) {
	m.mu.RLock()
	snapshot := m.settings
	m.mu.RUnlock()
	draft := &Manager{settings: snapshot}
	if _, err := draft.Save(next); err != nil {
		return CheckResult{}, err
	}
	return draft.Test(ctx, route), nil
}

func (m *Manager) allowsOfficialFallback(target *url.URL) bool {
	route, known := routeForURL(target)
	if !known {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	mode := m.settings.Routes[route].Mode
	return mode == ModeMirror || mode == ModeAuto
}

func resolveAuto(item storedRouteSettings, target string) storedRouteSettings {
	if item.Mode != ModeAuto {
		return item
	}
	request, _ := http.NewRequest(http.MethodGet, target, nil)
	proxy, _ := http.ProxyFromEnvironment(request)
	if proxy != nil {
		return storedRouteSettings{Mode: ModeSystem}
	}
	if item.MirrorURL != "" {
		item.Mode = ModeMirror
	} else {
		item.Mode = ModeDirect
	}
	return item
}
