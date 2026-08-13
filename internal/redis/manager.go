// Package redis manages a temporary in-process Redis for applications that do
// not have access to a dedicated Redis service. The server is a pure-Go,
// in-memory Redis implementation (miniredis): it starts on demand, binds only
// to loopback, and clears all data when the workbench stops.
package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicebob/miniredis/v2"
)

const (
	// DefaultPort is the Redis port used when no configuration exists.
	DefaultPort = 6379
	// bindHost keeps the temporary Redis on loopback only; it is never exposed
	// to the network.
	bindHost = "127.0.0.1"
)

// Config is the persisted manager configuration.
type Config struct {
	Port      int  `json:"port"`
	AutoStart bool `json:"autoStart"`
	// Disabled forbids starting the temporary Redis. It is set by the
	// --redis-off command-line flag and can be reversed from the settings page.
	Disabled bool `json:"disabled"`
}

// Status is the manager state returned to the workbench.
type Status struct {
	Running   bool   `json:"running"`
	Managed   bool   `json:"managed"`
	External  bool   `json:"external"`
	Skipped   bool   `json:"skipped"`
	Port      int    `json:"port"`
	Address   string `json:"address"`
	Message   string `json:"message"`
	AutoStart bool   `json:"autoStart"`
	Disabled  bool   `json:"disabled"`
}

// Manager owns the temporary Redis lifecycle and its persisted settings.
type Manager struct {
	path string
	mu   sync.Mutex

	config   Config
	server   *miniredis.Miniredis
	external bool
	skipped  bool
	message  string
}

// NewManager returns a manager backed by the given configuration file. A
// missing or unreadable file falls back to the defaults; the failure is logged
// so the manager remains usable even when the user config is corrupt.
func NewManager(path string) *Manager {
	manager := &Manager{path: path, config: Config{Port: DefaultPort}}
	if err := manager.loadLocked(); err != nil {
		log.Printf("临时 Redis 配置不可用，已使用默认配置：%v", err)
	}
	return manager
}

// Status returns a snapshot of the current manager state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

// Start launches the temporary Redis. When the configured port is already
// occupied by an existing Redis server, startup is skipped and that server is
// reported as the active Redis. When the port is occupied by another program,
// an error is returned and nothing is started.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked()
}

// Stop shuts down the managed temporary Redis. Stopping is a no-op when the
// active Redis is external or nothing is running.
func (m *Manager) Stop() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.external {
		m.message = "当前连接的是外部 Redis，无需停止。"
		return m.message, nil
	}
	if m.server == nil {
		m.message = "临时 Redis 未在运行。"
		return m.message, nil
	}
	m.server.Close()
	m.server = nil
	m.skipped = false
	m.message = "临时 Redis 已停止，内存数据已清空。"
	return m.message, nil
}

// Restart stops the managed Redis and starts it again on the configured port.
// If the port is occupied by an external Redis, the restart skips and reuses
// that server.
func (m *Manager) Restart() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		m.server.Close()
		m.server = nil
		m.skipped = false
	}
	if err := m.startLocked(); err != nil {
		return "", err
	}
	return m.message, nil
}

// Configure validates and persists the manager settings. When a managed Redis
// is running and the port changes, it is restarted on the new port.
func (m *Manager) Configure(port int, autoStart, disabled bool) error {
	if port < 1 || port > 65535 {
		return errors.New("Redis 端口需要在 1-65535 之间")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	portChanged := m.server != nil && port != m.config.Port
	m.config = Config{Port: port, AutoStart: autoStart, Disabled: disabled}
	if err := m.saveLocked(); err != nil {
		return err
	}
	if disabled || portChanged {
		if m.server != nil {
			m.server.Close()
			m.server = nil
		}
		m.skipped = false
		if portChanged && !disabled {
			if err := m.startLocked(); err != nil {
				return err
			}
		}
	}
	if disabled {
		m.external = false
		m.message = "临时 Redis 已禁用。"
	}
	return nil
}

// Close stops the managed Redis. It is safe to call multiple times and is
// intended for server shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		m.server.Close()
		m.server = nil
	}
	m.skipped = false
}

func (m *Manager) startLocked() error {
	if m.config.Disabled {
		return errors.New("临时 Redis 已被禁用，无法启动；请在设置中重新启用后再试。")
	}
	if m.server != nil {
		m.message = fmt.Sprintf("临时 Redis 已在运行，地址 %s。", m.server.Addr())
		return nil
	}
	port := m.config.Port
	address := net.JoinHostPort(bindHost, strconv.Itoa(port))
	if !portFree(port) {
		if isRedis, _ := probeRedis(address, 1500*time.Millisecond); isRedis {
			m.external = true
			m.skipped = true
			m.message = fmt.Sprintf("端口 %d 已被现有 Redis 占用，已跳过启动；应用可直接连接 %s 使用。", port, address)
			return nil
		}
		m.external = false
		m.skipped = false
		return fmt.Errorf("端口 %d 已被其他程序占用，且不是可用的 Redis 服务", port)
	}
	server := miniredis.NewMiniRedis()
	if err := server.StartAddr(address); err != nil {
		m.external = false
		m.skipped = false
		return fmt.Errorf("启动临时 Redis 失败：%w", err)
	}
	m.server = server
	m.external = false
	m.skipped = false
	m.message = fmt.Sprintf("临时 Redis 已启动，地址 %s。", server.Addr())
	return nil
}

func (m *Manager) statusLocked() Status {
	port := m.config.Port
	if m.server != nil {
		if parsed, err := strconv.Atoi(m.server.Port()); err == nil {
			port = parsed
		}
	}
	managed := m.server != nil
	external := m.external
	message := m.message
	if m.config.Disabled {
		message = "临时 Redis 已禁用；可在设置中重新启用。"
	} else if message == "" {
		switch {
		case managed:
			message = "临时 Redis 正在运行，数据仅保存在内存中。"
		case external:
			message = "正在使用外部 Redis；工作台退出不会影响该服务。"
		default:
			message = "临时 Redis 未运行。"
		}
	}
	return Status{
		Running:   managed || external,
		Managed:   managed,
		External:  external,
		Skipped:   m.skipped,
		Port:      port,
		Address:   net.JoinHostPort(bindHost, strconv.Itoa(port)),
		Message:   message,
		AutoStart: m.config.AutoStart,
		Disabled:  m.config.Disabled,
	}
}

func (m *Manager) loadLocked() error {
	raw, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 Redis 配置失败：%w", err)
	}
	var config Config
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("Redis 配置无效：%w", err)
	}
	if config.Port < 1 || config.Port > 65535 {
		config.Port = DefaultPort
	}
	m.config = config
	return nil
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("无法创建 Redis 配置目录：%w", err)
	}
	raw, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 Redis 配置失败：%w", err)
	}
	if err := os.WriteFile(m.path, raw, 0o600); err != nil {
		return fmt.Errorf("保存 Redis 配置失败：%w", err)
	}
	return nil
}

// portFree reports whether the loopback port is not currently bound.
func portFree(port int) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(bindHost, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}

// probeRedis asks a RESP server on the address whether it is Redis. A +PONG
// reply, or a -NOAUTH error (Redis with a password), confirms Redis; other
// RESP replies are not treated as Redis to avoid hijacking foreign services.
func probeRedis(address string, timeout time.Duration) (bool, error) {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if _, err := connection.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return false, err
	}
	buffer := make([]byte, 128)
	count, err := connection.Read(buffer)
	if err != nil {
		return false, err
	}
	reply := strings.ToUpper(strings.TrimSpace(string(buffer[:count])))
	return strings.HasPrefix(reply, "+PONG") || strings.HasPrefix(reply, "-NOAUTH"), nil
}
