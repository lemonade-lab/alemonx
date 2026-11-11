package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"alemonx/internal/systemnetwork"
)

type managedMCP struct {
	mu       sync.Mutex
	port     string
	handler  func(token string) http.Handler
	server   *http.Server
	listener net.Listener
	token    string
	closed   bool
}

type mcpHTTPStatus struct {
	Running bool   `json:"running"`
	Managed bool   `json:"managed"`
	URL     string `json:"url"`
}

func (m *managedMCP) address() string {
	return net.JoinHostPort("127.0.0.1", m.port)
}

func (m *managedMCP) status() mcpHTTPStatus {
	status := mcpHTTPStatus{URL: "http://" + m.address() + "/mcp", Managed: m.server != nil}
	status.Running = status.Managed
	if !status.Managed && !m.closed {
		client := systemnetwork.LocalClient(500 * time.Millisecond)
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		response, err := client.Get(status.URL)
		if err == nil {
			response.Body.Close()
			status.Running = response.StatusCode == http.StatusUnauthorized &&
				response.Header.Get("MCP-Protocol-Version") != ""
		}
	}
	return status
}

func (s *server) systemMCPHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	m := &s.mcpHTTP
	if r.Method == http.MethodGet {
		m.mu.Lock()
		status := m.status()
		m.mu.Unlock()
		writeJSON(w, http.StatusOK, status)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.allowMCPControl(r) {
		writeError(w, http.StatusForbidden, "请从本机工作台操作，或登录具有系统管理权限的账户。")
		return
	}

	var input struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "无法读取 MCP 服务操作。")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.handler == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP HTTP 服务当前不可用。")
		return
	}
	switch input.Action {
	case "start":
		if m.server == nil {
			port, err := strconv.Atoi(m.port)
			if err != nil || port < 1 || port > 65535 {
				writeError(w, http.StatusBadRequest, "MCP 端口必须在 1 到 65535 之间。")
				return
			}
			var secret [32]byte
			if _, err := rand.Read(secret[:]); err != nil {
				writeError(w, http.StatusInternalServerError, "无法生成 MCP 连接令牌。")
				return
			}
			listener, err := net.Listen("tcp4", m.address())
			if err != nil {
				writeError(w, http.StatusConflict, "无法监听 MCP 端口，请检查端口是否已被占用。")
				return
			}
			m.token = base64.RawURLEncoding.EncodeToString(secret[:])
			server := &http.Server{Handler: m.handler(m.token), ReadHeaderTimeout: 10 * time.Second}
			m.server, m.listener = server, listener
			go s.serveManagedMCP(server, listener)
		}
	case "stop":
		m.stop()
	case "token":
		if m.server == nil {
			writeError(w, http.StatusConflict, "请先在工作台启动 MCP HTTP 服务。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": m.token})
		return
	default:
		writeError(w, http.StatusBadRequest, "MCP 服务操作无效。")
		return
	}
	status := m.status()
	s.publishMCPStatus(status.Running)
	writeJSON(w, http.StatusOK, status)
}

func (s *server) allowMCPControl(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(parsed.Host, r.Host) ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			return false
		}
	}
	status, err := s.auth.Status("")
	if err != nil {
		return false
	}
	if status.Enabled {
		return s.auth.Authorize(s.authToken(r), "system.manage")
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return requestIsLoopback(r) && (strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())) &&
		r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Forwarded-Host") == ""
}

func (m *managedMCP) stop() {
	if m.server != nil {
		_ = m.server.Close()
		_ = m.listener.Close()
		m.server, m.listener, m.token = nil, nil, ""
	}
}

func (s *server) serveManagedMCP(server *http.Server, listener net.Listener) {
	_ = server.Serve(listener)
	m := &s.mcpHTTP
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == server {
		m.stop()
		s.publishMCPStatus(m.status().Running)
	}
}

func (s *server) shutdownMCP() {
	m := &s.mcpHTTP
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stop()
}

func (s *server) publishMCPStatus(running bool) {
	if s.mcpEvents.set(running) {
		_, _ = s.publishEvent("system", "mcp.changed", map[string]any{"type": "mcp.changed", "running": running}, nil)
	}
}

func (s *server) startMCPStatusMonitor() {
	update := func() {
		m := &s.mcpHTTP
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.closed {
			s.publishMCPStatus(m.status().Running)
		}
	}
	update()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.mcpMonitorStop:
				return
			case <-ticker.C:
				update()
			}
		}
	}()
}
