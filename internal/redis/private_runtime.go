package redis

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// privateRuntime is an ALemonX-distributed Redis executable. The downloader
// activates a verified release by placing it at this stable location. The
// environment override is deliberately useful to release CI and developers.
func (m *Manager) privateRuntime() string {
	if override := strings.TrimSpace(os.Getenv("ALX_REDIS_RUNTIME_BIN")); override != "" {
		return override
	}
	name := "redis-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(m.path), "redis-runtime", "current", name)
}

func (m *Manager) privateDataDir() string { return filepath.Join(filepath.Dir(m.path), "redis-data") }
func (m *Manager) privateOwnerPath() string {
	return filepath.Join(filepath.Dir(m.path), "alx-redis-owner.json")
}

type privateOwner struct {
	InstanceID string `json:"instanceId"`
	PID        int    `json:"pid"`
	Port       int    `json:"port"`
	Binary     string `json:"binary"`
	SHA256     string `json:"sha256"`
	StartedAt  string `json:"startedAt"`
}

func (m *Manager) privateInstalledLocked() bool {
	info, err := os.Stat(m.privateRuntime())
	return err == nil && !info.IsDir()
}

func (m *Manager) startPrivateLocked(address string) error {
	binary := m.privateRuntime()
	if !m.privateInstalledLocked() {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(m.privateDataDir(), 0o700); err != nil {
		return fmt.Errorf("创建私有 Redis 数据目录失败：%w", err)
	}
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("私有 Redis 后端地址无效：%w", err)
	}
	if portText == "0" {
		listener, listenErr := net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
		if listenErr != nil {
			return listenErr
		}
		portText = strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
		_ = listener.Close()
		address = net.JoinHostPort(bindHost, portText)
	}
	config := filepath.Join(m.privateDataDir(), "redis.conf")
	contents := fmt.Sprintf("bind 127.0.0.1\nport %s\ndir %s\ndbfilename dump.rdb\nappendonly yes\nappendfsync everysec\ndaemonize no\nprotected-mode yes\nlogfile %s\n", portText, redisConfigPath(m.privateDataDir()), redisConfigPath(filepath.Join(m.privateDataDir(), "redis.log")))
	if err := os.WriteFile(config, []byte(contents), 0o600); err != nil {
		return fmt.Errorf("写入私有 Redis 配置失败：%w", err)
	}
	command := exec.Command(binary, config)
	command.Dir = m.privateDataDir()
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动私有 Redis 失败：%w", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := probeRedis(address, 300*time.Millisecond); ok {
			m.privateProcess = command
			m.private = true
			m.backendAddress = address
			m.external = false
			m.skipped = false
			m.writePrivateOwnerLocked(command.Process.Pid, binary)
			m.message = "应用私有 Redis 已启动，已启用 AOF 与 RDB 持久化。"
			return nil
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_, _ = command.Process.Wait()
	return errors.New("私有 Redis 未在限定时间内通过健康检查")
}

func redisConfigPath(path string) string { return strings.ReplaceAll(path, "\\", "/") }

func (m *Manager) writePrivateOwnerLocked(pid int, binary string) {
	hash, _ := fileSHA256(binary)
	port := m.config.Port
	if _, value, err := net.SplitHostPort(m.backendAddress); err == nil {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil {
			port = parsed
		}
	}
	owner := privateOwner{InstanceID: m.config.InstanceID, PID: pid, Port: port, Binary: binary, SHA256: hash, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(owner)
	if err == nil {
		_ = os.WriteFile(m.privateOwnerPath(), raw, 0o600)
	}
}

func (m *Manager) stopPrivateLocked() error {
	if !m.private {
		return nil
	}
	address := net.JoinHostPort(bindHost, strconv.Itoa(m.config.Port))
	if m.backendAddress != "" {
		address = m.backendAddress
	}
	_ = redisRESP(address, 3*time.Second, []byte("SHUTDOWN"), []byte("SAVE"))
	if m.privateProcess != nil && m.privateProcess.Process != nil {
		done := make(chan error, 1)
		go func() { done <- m.privateProcess.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = m.privateProcess.Process.Kill()
			<-done
		}
	}
	m.privateProcess = nil
	m.private = false
	_ = os.Remove(m.privateOwnerPath())
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func redisRESP(address string, timeout time.Duration, args ...[]byte) error {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if _, err := fmt.Fprintf(connection, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(connection, "$%d\r\n", len(arg)); err != nil {
			return err
		}
		if _, err := connection.Write(arg); err != nil {
			return err
		}
		if _, err := connection.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.HasPrefix(line, "-") {
		return errors.New(strings.TrimSpace(line))
	}
	return nil
}

// ActivatePrivateRuntime upgrades only while no authenticated proxy clients
// are active. That conservative cutover rule preserves every existing TCP
// session: callers retry later instead of disconnecting a client or risking a
// write split between the fallback and private runtime.
func (m *Manager) ActivatePrivateRuntime() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.Disabled {
		return errors.New("内置 Redis 已被禁用")
	}
	if !m.privateInstalledLocked() {
		return os.ErrNotExist
	}
	if m.private {
		return nil
	}
	if m.server == nil {
		return errors.New("当前没有可迁移的 miniredis 实例")
	}
	if m.proxy == nil {
		return errors.New("Redis 本地代理未运行")
	}
	if m.proxy.activeClients() != 0 {
		m.phase, m.retryable = "waiting-for-idle", true
		m.message = "正在等待现有 Redis 连接空闲后安全迁移。"
		return errors.New("仍有 Redis 客户端连接，迁移将在连接空闲后重试")
	}
	if err := m.saveSnapshotLocked(); err != nil {
		return err
	}
	m.phase, m.message = "migrating", "正在安全迁移到 ALemonX 私有 Redis。"
	if err := m.startPrivateLocked(net.JoinHostPort(bindHost, "0")); err == nil {
		if restoreErr := restoreSnapshotToPrivateRedis(m.backendAddress, m.snapshotPath); restoreErr == nil {
			m.proxy.setBackend(m.backendAddress)
			m.stopSnapshotterLocked()
			m.server.Close()
			m.server = nil
			m.config.PrivateInitialized = true
			if err := m.saveLocked(); err != nil {
				m.retryable = true
				return fmt.Errorf("私有 Redis 已切换，但保存初始化状态失败：%w", err)
			}
			m.mode, m.phase, m.retryable = "private-running", "", false
			m.message = "已迁移到 ALemonX 私有 Redis，数据持久化已启用。"
			return nil
		} else {
			_ = m.stopPrivateLocked()
			m.backendAddress = m.server.Addr()
			m.phase, m.retryable = "preparing-runtime", true
			m.message = "私有 Redis 数据迁移失败；仍在使用临时 Redis。"
			return fmt.Errorf("私有 Redis 数据迁移失败：%w", restoreErr)
		}
	} else {
		m.backendAddress = m.server.Addr()
		m.phase, m.retryable = "preparing-runtime", true
		m.message = "私有 Redis 启动失败；仍在使用临时 Redis。"
		return fmt.Errorf("私有 Redis 启动失败：%w", err)
	}
}

func (m *Manager) restoreEmbeddedLocked() {
	address := net.JoinHostPort(bindHost, strconv.Itoa(m.config.Port))
	server := miniredis.NewMiniRedis()
	if err := server.StartAddr(address); err != nil {
		m.message = fmt.Sprintf("miniredis 回退失败：%v", err)
		return
	}
	m.server = server
	if err := m.restoreSnapshotLocked(); err != nil {
		m.message = fmt.Sprintf("miniredis 已回退，但恢复快照失败：%v", err)
	} else {
		m.message = "私有 Redis 不可用，已回退 miniredis。"
	}
	m.startSnapshotterLocked()
}

func restoreSnapshotToPrivateRedis(address, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var data snapshot
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.Version != snapshotVersion {
		return fmt.Errorf("不支持的 Redis 数据快照版本 %d", data.Version)
	}
	connection, err := net.DialTimeout("tcp", address, 20*time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	reader := bufio.NewReader(connection)
	for _, database := range data.Databases {
		if err := redisRESPOn(connection, reader, []byte("SELECT"), []byte(strconv.Itoa(database.ID))); err != nil {
			return err
		}
		for _, entry := range database.Keys {
			if err := restoreSnapshotKeyToPrivateRedis(connection, reader, entry); err != nil {
				return fmt.Errorf("键 %q：%w", entry.Key, err)
			}
		}
	}
	return nil
}

func restoreSnapshotKeyToPrivateRedis(connection net.Conn, reader *bufio.Reader, entry snapshotKey) error {
	if err := redisRESPOn(connection, reader, []byte("DEL"), entry.Key); err != nil {
		return err
	}
	var command [][]byte
	switch entry.Type {
	case "string":
		command = [][]byte{[]byte("SET"), entry.Key, entry.Value}
	case "list":
		command = append([][]byte{[]byte("RPUSH"), entry.Key}, entry.Values...)
	case "set":
		command = append([][]byte{[]byte("SADD"), entry.Key}, entry.Values...)
	case "hash":
		command = [][]byte{[]byte("HSET"), entry.Key}
		for _, field := range entry.Fields {
			command = append(command, field.Key, field.Value)
		}
	case "zset":
		command = [][]byte{[]byte("ZADD"), entry.Key}
		for _, member := range entry.Members {
			command = append(command, []byte(strconv.FormatFloat(member.Score, 'g', -1, 64)), member.Value)
		}
	case "stream":
		for _, value := range entry.Entries {
			if err := redisRESPOn(connection, reader, append([][]byte{[]byte("XADD"), entry.Key, []byte(value.ID)}, value.Values...)...); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("不支持的 Redis 数据类型 %q", entry.Type)
	}
	if len(command) > 2 {
		if err := redisRESPOn(connection, reader, command...); err != nil {
			return err
		}
	}
	if entry.TTLMillis > 0 {
		return redisRESPOn(connection, reader, []byte("PEXPIRE"), entry.Key, []byte(strconv.FormatInt(entry.TTLMillis, 10)))
	}
	return nil
}

func redisRESPOn(connection net.Conn, reader *bufio.Reader, args ...[]byte) error {
	if _, err := fmt.Fprintf(connection, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(connection, "$%d\r\n", len(arg)); err != nil {
			return err
		}
		if _, err := connection.Write(arg); err != nil {
			return err
		}
		if _, err := connection.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if strings.HasPrefix(line, "-") {
		return errors.New(strings.TrimSpace(line))
	}
	return nil
}
