// Package redis manages a local in-process Redis for applications that do not
// have access to a dedicated Redis service. The server is a pure-Go Redis
// implementation (miniredis), binds only to loopback, and persists supported
// data types to a local snapshot.
package redis

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
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
	// snapshotInterval bounds the amount of data lost after an ungraceful exit.
	// Graceful stop and restart always write a snapshot before closing Redis.
	snapshotInterval = time.Second
)

// Config is the persisted manager configuration.
type Config struct {
	Port      int  `json:"port"`
	AutoStart bool `json:"autoStart"`
	// Disabled forbids starting the temporary Redis. It is set by the
	// --redis-off command-line flag and can be reversed from the settings page.
	Disabled bool `json:"disabled"`
	// Native selects the host Redis service. It is intentionally optional so
	// configurations written by older ALemonX versions remain compatible.
	Native bool `json:"native,omitempty"`
	// PrivateInitialized is written only after the miniredis snapshot has been
	// imported into an application-private runtime. Later starts trust the
	// runtime's own AOF/RDB rather than replaying an old fallback snapshot.
	PrivateInitialized bool `json:"privateInitialized,omitempty"`
	// Password is generated once per ALemonX instance. It protects the public
	// loopback proxy as well as the private Redis backend. It is deliberately
	// omitted from Status so a status refresh can never leak credentials.
	Password   string `json:"password,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
}

// Status is the manager state returned to the workbench.
type Status struct {
	Mode             string `json:"mode"`
	Phase            string `json:"phase,omitempty"`
	Ownership        string `json:"ownership"`
	Running          bool   `json:"running"`
	Managed          bool   `json:"managed"`
	External         bool   `json:"external"`
	Skipped          bool   `json:"skipped"`
	Port             int    `json:"port"`
	Address          string `json:"address"`
	Message          string `json:"message"`
	AutoStart        bool   `json:"autoStart"`
	Disabled         bool   `json:"disabled"`
	Persistent       bool   `json:"persistent"`
	LastSaved        string `json:"lastSaved,omitempty"`
	NativeSupported  bool   `json:"nativeSupported"`
	NativeInstalled  bool   `json:"nativeInstalled"`
	NativeRunning    bool   `json:"nativeRunning"`
	NativeEnabled    bool   `json:"nativeEnabled"`
	NativeService    string `json:"nativeService,omitempty"`
	PrivateInstalled bool   `json:"privateInstalled"`
	PrivateRunning   bool   `json:"privateRunning"`
	RuntimePath      string `json:"runtimePath,omitempty"`
	RuntimeVersion   string `json:"runtimeVersion,omitempty"`
	Retryable        bool   `json:"retryable"`
	TaskID           string `json:"taskId,omitempty"`
	ConnectionURI    string `json:"connectionUri,omitempty"`
}

// Manager owns the built-in Redis lifecycle and its persisted settings.
type Manager struct {
	path         string
	snapshotPath string
	mu           sync.Mutex

	config         Config
	server         *miniredis.Miniredis
	proxy          *redisProxy
	backendAddress string
	private        bool
	privateProcess *exec.Cmd
	external       bool
	skipped        bool
	message        string
	mode           string
	phase          string
	retryable      bool
	lastSaved      time.Time
	snapshotStop   chan struct{}
}

// NewManager returns a manager backed by the given configuration file. A
// missing or unreadable file falls back to the defaults; the failure is logged
// so the manager remains usable even when the user config is corrupt.
func NewManager(path string) *Manager {
	manager := &Manager{
		path:         path,
		snapshotPath: filepath.Join(filepath.Dir(path), "alx-redis-data.json"),
		config:       Config{Port: DefaultPort, AutoStart: true},
	}
	if err := manager.loadLocked(); err != nil {
		log.Printf("内置 Redis 配置不可用，已使用默认配置：%v", err)
	}
	if manager.config.Password == "" || manager.config.InstanceID == "" {
		// A random per-instance secret keeps a loopback Redis from being a
		// writable service for every process on the machine.
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Printf("生成 Redis 本地凭据失败：%v", err)
		} else {
			if manager.config.Password == "" {
				manager.config.Password = base64.RawURLEncoding.EncodeToString(secret)
			}
			if manager.config.InstanceID == "" {
				manager.config.InstanceID = base64.RawURLEncoding.EncodeToString(secret[:16])
			}
			if err := manager.saveLocked(); err != nil {
				log.Printf("保存 Redis 本地凭据失败：%v", err)
			}
		}
	}
	return manager
}

// Status returns a snapshot of the current manager state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.statusLocked()
}

// Start launches the built-in Redis. When the configured port is already
// occupied by an existing Redis server, startup is skipped and that server is
// reported as the active Redis. When the port is occupied by another program,
// an error is returned and nothing is started.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked()
}

// InstallNative installs Redis through the Linux package manager, enables its
// system service, and persists native mode. ALemonX never owns that process.
func (m *Manager) InstallNative() error {
	m.mu.Lock()
	port := m.config.Port
	wasManaged := m.server != nil
	if wasManaged {
		if err := m.saveSnapshotLocked(); err != nil {
			m.mu.Unlock()
			return err
		}
		m.stopSnapshotterLocked()
		m.server.Close()
		m.server = nil
		m.external = false
		m.skipped = false
	}
	m.mu.Unlock()
	if port != DefaultPort {
		if wasManaged {
			m.restoreManagedAfterNativeFailure()
		}
		return fmt.Errorf("独立 Redis 当前使用系统默认端口 6379，请先将 Redis 端口改为 6379")
	}
	if err := installNative(); err != nil {
		if wasManaged {
			m.restoreManagedAfterNativeFailure()
		}
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.Native = true
	m.config.Disabled = false
	m.config.AutoStart = true
	if err := m.saveLocked(); err != nil {
		return err
	}
	m.external = true
	m.skipped = true
	m.message = "独立 Redis 已安装并启用 systemd 开机自启；ALemonX 不会接管其生命周期。"
	return nil
}

func (m *Manager) restoreManagedAfterNativeFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.Native || m.config.Disabled || m.server != nil {
		return
	}
	if err := m.startLocked(); err != nil {
		log.Printf("独立 Redis 安装失败后恢复内置 Redis 失败：%v", err)
	}
}

// Stop shuts down the managed built-in Redis. Stopping is a no-op when the
// active Redis is external or nothing is running.
func (m *Manager) Stop() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.external {
		m.message = "当前连接的是外部 Redis，无需停止。"
		return m.message, nil
	}
	if m.proxy != nil {
		m.proxy.close()
		m.proxy = nil
	}
	if m.private {
		if err := m.stopPrivateLocked(); err != nil {
			return "", err
		}
		m.skipped = false
		m.message = "应用私有 Redis 已停止，数据已持久化。"
		return m.message, nil
	}
	if m.server == nil {
		m.message = "内置 Redis 未在运行。"
		return m.message, nil
	}
	if err := m.saveSnapshotLocked(); err != nil {
		log.Printf("保存内置 Redis 数据失败：%v", err)
	}
	m.stopSnapshotterLocked()
	m.server.Close()
	m.server = nil
	m.skipped = false
	m.message = "内置 Redis 已停止，数据已持久化。"
	return m.message, nil
}

// Restart stops the managed Redis and starts it again on the configured port.
// If the port is occupied by an external Redis, the restart skips and reuses
// that server.
func (m *Manager) Restart() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.external {
		m.message = "当前使用的是外部 Redis，ALemonX 不会重启它。"
		return m.message, nil
	}
	if m.proxy != nil {
		m.proxy.close()
		m.proxy = nil
	}
	if m.private {
		if err := m.stopPrivateLocked(); err != nil {
			return "", err
		}
	}
	if m.server != nil {
		if err := m.saveSnapshotLocked(); err != nil {
			log.Printf("保存内置 Redis 数据失败：%v", err)
		}
		m.stopSnapshotterLocked()
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
	if m.config.Native && port != m.config.Port {
		return fmt.Errorf("独立 Redis 使用系统默认端口 6379，不能修改为自定义端口")
	}
	portChanged := (m.server != nil || m.private) && port != m.config.Port
	m.config = Config{Port: port, AutoStart: autoStart, Disabled: disabled, Native: m.config.Native, PrivateInitialized: m.config.PrivateInitialized}
	if err := m.saveLocked(); err != nil {
		return err
	}
	if disabled || portChanged {
		if m.proxy != nil {
			m.proxy.close()
			m.proxy = nil
		}
		if m.private {
			if err := m.stopPrivateLocked(); err != nil {
				return err
			}
		}
		if m.server != nil {
			if err := m.saveSnapshotLocked(); err != nil {
				log.Printf("保存内置 Redis 数据失败：%v", err)
			}
			m.stopSnapshotterLocked()
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
		m.message = "内置 Redis 已禁用。"
	}
	return nil
}

// Close stops the managed Redis. It is safe to call multiple times and is
// intended for server shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proxy != nil {
		m.proxy.close()
		m.proxy = nil
	}
	if m.server != nil {
		if err := m.saveSnapshotLocked(); err != nil {
			log.Printf("保存内置 Redis 数据失败：%v", err)
		}
		m.stopSnapshotterLocked()
		m.server.Close()
		m.server = nil
	}
	if m.private {
		if err := m.stopPrivateLocked(); err != nil {
			log.Printf("关闭应用私有 Redis 失败：%v", err)
		}
	}
	m.skipped = false
}

func (m *Manager) startLocked() error {
	if m.config.Disabled {
		return errors.New("内置 Redis 已被禁用，无法启动；请在设置中重新启用后再试。")
	}
	// Native was a legacy setting. Never start or install a system service; an
	// existing process is simply treated as an external, read-only endpoint.
	if m.config.Native {
		m.config.Native = false
		_ = m.saveLocked()
	}
	if m.private || m.server != nil {
		m.message = fmt.Sprintf("ALemonX Redis 已在运行，地址 %s。", net.JoinHostPort(bindHost, strconv.Itoa(m.config.Port)))
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
	if m.privateInstalledLocked() {
		backend := net.JoinHostPort(bindHost, "0")
		if err := m.startPrivateLocked(backend); err == nil {
			if !m.config.PrivateInitialized {
				if err := restoreSnapshotToPrivateRedis(m.backendAddress, m.snapshotPath, m.config.Password); err != nil {
					_ = m.stopPrivateLocked()
					log.Printf("私有 Redis 首次恢复失败，回退 miniredis：%v", err)
				} else {
					m.config.PrivateInitialized = true
					if err := m.saveLocked(); err != nil {
						log.Printf("保存私有 Redis 初始化状态失败：%v", err)
					}
					if err := m.startProxyLocked(address); err != nil {
						_ = m.stopPrivateLocked()
						return err
					}
					m.mode, m.phase, m.message = "private-running", "", "ALemonX 私有 Redis 已准备好。"
					return nil
				}
			} else {
				if err := m.startProxyLocked(address); err != nil {
					_ = m.stopPrivateLocked()
					return err
				}
				m.mode, m.phase = "private-running", ""
				return nil
			}
		} else {
			log.Printf("私有 Redis 启动失败，回退 miniredis：%v", err)
		}
	}
	server := miniredis.NewMiniRedis()
	if err := server.StartAddr(net.JoinHostPort(bindHost, "0")); err != nil {
		m.external = false
		m.skipped = false
		return fmt.Errorf("启动内置 Redis 失败：%w", err)
	}
	server.RequireAuth(m.config.Password)
	m.server = server
	m.backendAddress = server.Addr()
	if err := m.restoreSnapshotLocked(); err != nil {
		log.Printf("恢复内置 Redis 数据失败：%v", err)
		m.message = "内置 Redis 已启动；历史数据恢复失败。"
	} else {
		m.message = "内置 Redis 已启动，已启用本地持久化。"
	}
	m.startSnapshotterLocked()
	if err := m.startProxyLocked(address); err != nil {
		m.stopSnapshotterLocked()
		m.server.Close()
		m.server = nil
		return fmt.Errorf("启动 Redis 本地代理失败：%w", err)
	}
	m.external = false
	m.skipped = false
	m.mode, m.phase, m.retryable = "fallback-running", "preparing-runtime", true
	return nil
}

func (m *Manager) startProxyLocked(address string) error {
	if m.proxy != nil {
		return nil
	}
	proxy, err := newRedisProxy(address, m.backendAddress, m.config.Password)
	if err != nil {
		return err
	}
	m.proxy = proxy
	return nil
}

func (m *Manager) statusLocked() Status {
	port := m.config.Port
	managed := m.server != nil || m.private
	external := m.external
	message := m.message
	if m.config.Disabled {
		message = "内置 Redis 已禁用；可在设置中重新启用。"
	} else if message == "" {
		switch {
		case m.private:
			message = "应用私有 Redis 正在运行，数据持久化到本机。"
		case managed:
			message = "内置 Redis 正在运行，数据会自动持久化到本机。"
		case external:
			message = "正在使用独立 Redis；服务由 systemd 管理，工作台退出不会影响它。"
		default:
			message = "内置 Redis 未运行。"
		}
	}
	lastSaved := ""
	if !m.lastSaved.IsZero() {
		lastSaved = m.lastSaved.Format(time.RFC3339)
	}
	return Status{
		Mode:             m.modeForStatusLocked(managed, external),
		Phase:            m.phase,
		Ownership:        ownershipFor(managed, external),
		Running:          managed || external,
		Managed:          managed,
		External:         external,
		Skipped:          m.skipped,
		Port:             port,
		Address:          net.JoinHostPort(bindHost, strconv.Itoa(port)),
		Message:          message,
		AutoStart:        m.config.AutoStart,
		Disabled:         m.config.Disabled,
		Persistent:       managed,
		LastSaved:        lastSaved,
		NativeSupported:  false,
		NativeInstalled:  false,
		NativeRunning:    false,
		NativeEnabled:    false,
		NativeService:    "",
		PrivateInstalled: m.privateInstalledLocked(),
		PrivateRunning:   m.private,
		RuntimePath:      m.privateRuntime(),
		Retryable:        m.retryable,
		ConnectionURI:    "redis://:" + m.config.Password + "@" + net.JoinHostPort(bindHost, strconv.Itoa(port)),
	}
}

// RetryRuntime records a user-requested retry. The downloader is deliberately
// asynchronous: fallback Redis remains available even when the release source
// is offline.
func (m *Manager) RetryRuntime() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil && !m.private {
		m.phase, m.retryable = "preparing-runtime", true
		m.message = "正在后台准备 ALemonX 私有 Redis。"
	}
}

func ownershipFor(managed, external bool) string {
	if external {
		return "external"
	}
	if managed {
		return "alemonx"
	}
	return "none"
}

func (m *Manager) modeForStatusLocked(managed, external bool) string {
	if m.config.Disabled {
		return "disabled"
	}
	if external {
		return "external-reused"
	}
	if m.mode != "" {
		return m.mode
	}
	if m.private {
		return "private-running"
	}
	if managed {
		return "fallback-running"
	}
	return "stopped"
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
