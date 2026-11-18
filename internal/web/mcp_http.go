package web

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/logging"
	"alemonx/internal/systemnetwork"
)

type managedMCP struct {
	mu             sync.Mutex
	port           string
	accountHandler func(string, access.Status) http.Handler
	handlers       map[mcpHandlerKey]mcpAccountHandler
	server         *http.Server
	listener       net.Listener
	closed         bool
	autoStart      bool
	restoreFailed  bool
}

type mcpHandlerKey struct {
	scope      string
	diagnostic bool
}

type mcpAccountHandler struct {
	token   string
	handler http.Handler
}

type mcpHTTPStatus struct {
	CanControl         bool   `json:"canControl"`
	CanIssueToken      bool   `json:"canIssueToken"`
	HasDiagnosticToken bool   `json:"hasDiagnosticToken"`
	HasToken           bool   `json:"hasToken"`
	Account            string `json:"account,omitempty"`
	Running            bool   `json:"running"`
	AutoStart          bool   `json:"autoStart"`
	Managed            bool   `json:"managed"`
	URL                string `json:"url"`
}

func (m *managedMCP) address() string {
	return net.JoinHostPort("127.0.0.1", m.port)
}

func (m *managedMCP) status() mcpHTTPStatus {
	status := mcpHTTPStatus{URL: "http://" + m.address() + "/mcp", Managed: m.server != nil, AutoStart: m.autoStart}
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
	authToken := s.authToken(r)
	identity, err := s.auth.Status(authToken)
	if err != nil || (identity.Enabled && !identity.Authenticated) {
		writeError(w, http.StatusUnauthorized, "请先登录工作台。")
		return
	}
	canIssueToken := allowMCPRequest(r, identity)
	canControl := canIssueToken && (!identity.Enabled || identity.SuperAdmin)
	m := &s.mcpHTTP
	if r.Method == http.MethodGet {
		m.mu.Lock()
		status := m.accountStatus(identity, canControl, canIssueToken)
		m.mu.Unlock()
		writeJSON(w, http.StatusOK, status)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !canIssueToken {
		writeError(w, http.StatusForbidden, "请在当前工作台操作 MCP。未开启认证时请使用本机工作台。")
		return
	}
	var input struct {
		Action  string `json:"action"`
		KeyType string `json:"keyType"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "无法读取 MCP 服务操作。")
		return
	}
	if (input.Action == "start" || input.Action == "stop") && !canControl {
		writeError(w, http.StatusForbidden, "只有超级管理员可以启动或停止 MCP。")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.accountHandler == nil {
		writeError(w, http.StatusServiceUnavailable, "MCP HTTP 服务当前不可用。")
		return
	}
	switch input.Action {
	case "start", "stop":
		enable := input.Action == "start"
		opened := enable && m.server == nil
		if opened {
			if err := s.openMCPListener(); err != nil {
				code := http.StatusConflict
				if errors.Is(err, errMCPPort) {
					code = http.StatusBadRequest
				}
				writeError(w, code, err.Error())
				return
			}
		}
		if err := s.auth.SetMCPAutoStart(authToken, identity, enable); err != nil {
			if opened {
				m.stop()
			}
			switch {
			case errors.Is(err, access.ErrMCPAccountChanged):
				writeError(w, http.StatusUnauthorized, err.Error())
			case errors.Is(err, access.ErrMCPServiceControl):
				writeError(w, http.StatusForbidden, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, "无法保存 MCP 服务启停设置。")
			}
			return
		}
		m.autoStart = enable
		m.restoreFailed = false
		if !enable {
			m.stop()
		}
	case "token", "rotate", "revoke":
		if input.KeyType != "" && input.KeyType != "operator" && input.KeyType != "diagnostic" {
			writeError(w, http.StatusBadRequest, "无效的 MCP 密钥类型。")
			return
		}
		diagnostic := input.KeyType == "diagnostic"
		token, err := s.auth.UpdateMCPKey(authToken, identity, input.Action, diagnostic)
		if err != nil {
			switch {
			case errors.Is(err, access.ErrMCPKeyExists):
				writeError(w, http.StatusConflict, err.Error())
			case errors.Is(err, access.ErrMCPAccountChanged):
				writeError(w, http.StatusUnauthorized, err.Error())
			default:
				writeError(w, http.StatusInternalServerError, "无法保存 MCP 连接密钥。")
			}
			return
		}
		delete(m.handlers, mcpHandlerKey{scope: identity.RobotScope, diagnostic: diagnostic})
		if input.Action == "revoke" {
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeJSON(w, http.StatusOK, map[string]string{"token": token})
		}
		return
	default:
		writeError(w, http.StatusBadRequest, "MCP 服务操作无效。")
		return
	}
	status := m.accountStatus(identity, canControl, canIssueToken)
	s.publishMCPStatus(status)
	writeJSON(w, http.StatusOK, status)
}

func (m *managedMCP) accountStatus(identity access.Status, canControl, canIssueToken bool) mcpHTTPStatus {
	status := m.status()
	status.CanControl = canControl
	status.CanIssueToken = canIssueToken
	status.HasToken = canIssueToken && identity.HasMCPKey
	status.HasDiagnosticToken = canIssueToken && identity.HasMCPDiagnosticKey
	status.Account = identity.Account
	return status
}

func (s *server) serveMCPAccount(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	m := &s.mcpHTTP
	m.mu.Lock()
	identity, valid, err := s.auth.MCPKeyStatus(token)
	var handler http.Handler
	if err == nil && valid && m.server != nil {
		key := mcpHandlerKey{scope: identity.RobotScope, diagnostic: identity.MCPDiagnostic}
		cached := m.handlers[key]
		if cached.token != token {
			cached = mcpAccountHandler{token: token, handler: m.accountHandler(token, identity)}
			m.handlers[key] = cached
		}
		handler = cached.handler
	}
	m.mu.Unlock()
	if handler == nil {
		w.Header().Set("Cache-Control", "no-store")
		writeError(w, http.StatusUnauthorized, "MCP HTTP 认证失败")
		return
	}
	handler.ServeHTTP(w, r)
}

func allowMCPRequest(r *http.Request, status access.Status) bool {
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
	if status.Enabled {
		return status.Authenticated
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

var errMCPPort = errors.New("MCP 端口必须在 1 到 65535 之间。")

// The caller holds mcpHTTP.mu and has checked that no listener is running.
func (s *server) openMCPListener() error {
	m := &s.mcpHTTP
	port, err := strconv.Atoi(m.port)
	if err != nil || port < 1 || port > 65535 {
		return errMCPPort
	}
	listener, err := net.Listen("tcp4", m.address())
	if err != nil {
		return errors.New("无法监听 MCP 端口，请检查端口是否已被占用。")
	}
	server := &http.Server{Handler: http.HandlerFunc(s.serveMCPAccount), ReadHeaderTimeout: 10 * time.Second}
	m.server, m.listener = server, listener
	m.handlers = make(map[mcpHandlerKey]mcpAccountHandler)
	go s.serveManagedMCP(server, listener)
	return nil
}

func (m *managedMCP) stop() {
	if m.server != nil {
		_ = m.server.Close()
		_ = m.listener.Close()
		m.server, m.listener, m.handlers = nil, nil, nil
	}
}

func (s *server) serveManagedMCP(server *http.Server, listener net.Listener) {
	_ = server.Serve(listener)
	m := &s.mcpHTTP
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server == server {
		m.stop()
		s.publishMCPStatus(m.status())
	}
}

func (s *server) shutdownMCP() {
	m := &s.mcpHTTP
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stop()
}

func (s *server) publishMCPStatus(status mcpHTTPStatus) {
	if s.mcpEvents.set(status.Running, status.Managed, status.AutoStart) {
		_, _ = s.publishEvent("system", "mcp.changed", map[string]any{
			"type": "mcp.changed", "running": status.Running, "managed": status.Managed, "autoStart": status.AutoStart,
		}, nil)
	}
}

func (s *server) refreshMCPStatus() {
	m := &s.mcpHTTP
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if m.autoStart && m.server == nil {
		if err := s.openMCPListener(); err != nil {
			if !m.restoreFailed {
				logging.WarnEvent("mcp.restore.failed", logging.Fields{"address": m.address(), "error": err.Error()})
			}
			m.restoreFailed = true
		} else {
			m.restoreFailed = false
		}
	}
	s.publishMCPStatus(m.status())
}

func (s *server) startMCPStatusMonitor() {
	autoStart, err := s.auth.MCPAutoStart()
	if err != nil {
		logging.WarnEvent("mcp.preference.load_failed", logging.Fields{"error": err.Error()})
	} else {
		s.mcpHTTP.mu.Lock()
		s.mcpHTTP.autoStart = autoStart
		s.mcpHTTP.mu.Unlock()
	}
	s.refreshMCPStatus()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.mcpMonitorStop:
				return
			case <-ticker.C:
				s.refreshMCPStatus()
			}
		}
	}()
}
