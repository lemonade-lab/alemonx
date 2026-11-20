// Package access implements local accounts and authentication for alx.
package access

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionDuration = 12 * time.Hour

type Account struct {
	GitHubID       int64     `json:"githubId,omitempty"`
	GitHubLogin    string    `json:"githubLogin,omitempty"`
	Account        string    `json:"account"`
	PasswordHash   string    `json:"passwordHash,omitempty"`
	SuperAdmin     bool      `json:"superAdmin,omitempty"`
	Enabled        bool      `json:"enabled"`
	Created        time.Time `json:"created"`
	Updated        time.Time `json:"updated"`
	LegacyRegistry bool      `json:"legacyRegistry,omitempty"`
}

// Status deliberately contains no password material.
type Status struct {
	GitHubLogin   string `json:"githubLogin,omitempty"`
	RobotScope    string `json:"-"`
	Enabled       bool   `json:"enabled"`
	Authenticated bool   `json:"authenticated"`
	Account       string `json:"account,omitempty"`
	SuperAdmin    bool   `json:"superAdmin,omitempty"`
	HasMCPKey     bool   `json:"-"`
}

type config struct {
	Enabled bool `json:"enabled"`
	// Account and PasswordHash are retained solely to migrate the old
	// single-account file without locking an existing installation out.
	Account      string            `json:"account,omitempty"`
	PasswordHash string            `json:"passwordHash,omitempty"`
	SessionKey   string            `json:"sessionKey,omitempty"`
	Accounts     []Account         `json:"accounts,omitempty"`
	MCPKeys      map[string]string `json:"mcpKeys,omitempty"`
	MCPAutoStart bool              `json:"mcpAutoStart,omitempty"`
}

type Manager struct {
	path string
	mu   sync.RWMutex
	data config
}

// Path returns the exact file used by this manager. It is exposed for local
// diagnostics so operators do not accidentally reset a different user's (or
// the host's instead of a container's) authentication store.
func (m *Manager) Path() string { return m.path }

func DefaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户配置目录：%w", err)
	}
	return filepath.Join(directory, "alemonjs", "alx-auth.json"), nil
}

func New() (*Manager, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return NewAt(path)
}

func NewAt(path string) (*Manager, error) {
	manager := &Manager{path: path}
	if err := manager.reload(); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) reload() error {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		m.data = config{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法读取身份认证配置：%w", err)
	}
	var next config
	if err := json.Unmarshal(data, &next); err != nil {
		return fmt.Errorf("身份认证配置无效：%w", err)
	}
	if next.Enabled && (next.SessionKey == "" || (len(next.Accounts) == 0 && (next.Account == "" || next.PasswordHash == ""))) {
		return errors.New("身份认证配置不完整")
	}
	if next.Enabled && len(next.Accounts) == 0 {
		now := time.Now()
		next.Accounts = []Account{{Account: next.Account, PasswordHash: next.PasswordHash, SuperAdmin: true, Enabled: true, Created: now, Updated: now, LegacyRegistry: true}}
		// Persisting on the next write removes neither field, making migration
		// safe even if an older binary is used once more.
	}
	m.data = next
	return nil
}

func (m *Manager) current() (config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reload(); err != nil {
		return config{}, err
	}
	return m.data, nil
}

func normalizeAccount(account string) (string, error) {
	account = strings.TrimSpace(account)
	if account == "" || len(account) > 64 || strings.ContainsAny(account, "\r\n/") {
		return "", errors.New("账户需为 1 到 64 个非换行字符")
	}
	return account, nil
}

func accountIndex(items []Account, account string) int {
	for index, item := range items {
		if item.Account == account {
			return index
		}
	}
	return -1
}

func (m *Manager) Status(token string) (Status, error) {
	data, err := m.current()
	if err != nil {
		return Status{}, err
	}
	return m.sessionStatus(data, token), nil
}

func (m *Manager) sessionStatus(data config, token string) Status {
	if !data.Enabled {
		return Status{HasMCPKey: data.MCPKeys[""] != ""}
	}
	account, ok := m.tokenAccount(data, token)
	if !ok {
		return Status{Enabled: true}
	}
	return authenticatedStatus(data, account)
}

func authenticatedStatus(data config, account Account) Status {
	scope := robotScope(account)
	return Status{Enabled: true, Authenticated: true, Account: account.Account,
		GitHubLogin: account.GitHubLogin, RobotScope: scope, SuperAdmin: account.SuperAdmin,
		HasMCPKey: data.MCPKeys[scope] != "",
	}
}

func robotScope(account Account) string {
	if account.LegacyRegistry {
		return account.Account + "\x00legacy"
	}
	// A recreated username must not inherit the previous account's data.
	return account.Account + "\x00" + account.Created.UTC().Format(time.RFC3339Nano)
}

func (m *Manager) Enable(account, password, confirmation string) (string, error) {
	account, err := normalizeAccount(account)
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("请填写密码")
	}
	if password != confirmation {
		return "", errors.New("两次输入的密码不一致")
	}
	unlock, err := m.lockWrite()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return "", err
	}
	if m.data.Enabled {
		return "", errors.New("身份认证已开启；请先使用现有账户登录")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("无法保护密码：%w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("无法生成登录密钥：%w", err)
	}
	now := time.Now()
	m.data = config{Enabled: true, MCPAutoStart: m.data.MCPAutoStart, SessionKey: base64.RawURLEncoding.EncodeToString(key), Accounts: []Account{{Account: account, PasswordHash: string(hash), SuperAdmin: true, Enabled: true, Created: now, Updated: now}}}
	if err := m.persist(); err != nil {
		return "", err
	}
	return m.issueToken(m.data, account)
}

func (m *Manager) Disable() error {
	unlock, err := m.lockWrite()
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return err
	}
	m.data = config{MCPAutoStart: m.data.MCPAutoStart}
	return m.persist()
}

// ResetSuperAdmin is the emergency recovery path for a lost or leaked
// super-administrator credential. It preserves ordinary accounts,
// but rotates the session key and disables every other super administrator so
// an old password cannot immediately regain control.
func (m *Manager) ResetSuperAdmin(account, password, confirmation string) (string, error) {
	account, err := normalizeAccount(account)
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("请填写密码")
	}
	if password != confirmation {
		return "", errors.New("两次输入的密码不一致")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("无法保护密码：%w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("无法生成登录密钥：%w", err)
	}
	unlock, err := m.lockWrite()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return "", err
	}
	if !m.data.Enabled {
		return "", errors.New("身份认证尚未开启；请使用 auth enable 创建首个管理员")
	}
	now := time.Now()
	index := accountIndex(m.data.Accounts, account)
	if index < 0 {
		m.data.Accounts = append(m.data.Accounts, Account{Account: account, PasswordHash: string(hash), SuperAdmin: true, Enabled: true, Created: now, Updated: now})
	} else {
		delete(m.data.MCPKeys, robotScope(m.data.Accounts[index]))
		m.data.Accounts[index].PasswordHash = string(hash)
		m.data.Accounts[index].GitHubID = 0
		m.data.Accounts[index].GitHubLogin = ""
		m.data.Accounts[index].SuperAdmin = true
		m.data.Accounts[index].Enabled = true
		m.data.Accounts[index].Updated = now
	}
	for index := range m.data.Accounts {
		if m.data.Accounts[index].Account != account && m.data.Accounts[index].SuperAdmin {
			delete(m.data.MCPKeys, robotScope(m.data.Accounts[index]))
			m.data.Accounts[index].SuperAdmin = false
			m.data.Accounts[index].Enabled = false
			m.data.Accounts[index].Updated = now
		}
	}
	m.data.SessionKey = base64.RawURLEncoding.EncodeToString(key)
	if err := m.persist(); err != nil {
		return "", err
	}
	return m.issueToken(m.data, account)
}

func (m *Manager) Login(account, password string) (string, error) {
	data, err := m.current()
	if err != nil {
		return "", err
	}
	if !data.Enabled {
		return "", errors.New("身份认证尚未开启")
	}
	index := accountIndex(data.Accounts, strings.TrimSpace(account))
	if index < 0 || !data.Accounts[index].Enabled || bcrypt.CompareHashAndPassword([]byte(data.Accounts[index].PasswordHash), []byte(password)) != nil {
		return "", errors.New("账户或密码错误")
	}
	return m.issueToken(data, data.Accounts[index].Account)
}

func (m *Manager) Authenticate(token string) bool {
	data, err := m.current()
	if err != nil {
		return false
	}
	return !data.Enabled || func() bool { _, ok := m.tokenAccount(data, token); return ok }()
}

func (m *Manager) ListAccounts() ([]Account, error) {
	data, err := m.current()
	if err != nil {
		return nil, err
	}
	// The management API represents collections as JSON arrays, including when
	// they are empty.  Starting with a non-nil slice avoids serializing an
	// unconfigured account store as `null`.
	items := append([]Account{}, data.Accounts...)
	for index := range items {
		items[index].PasswordHash = ""
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Account < items[j].Account })
	return items, nil
}

func (m *Manager) CreateAccount(account, password, confirmation string) (Account, error) {
	account, err := normalizeAccount(account)
	if err != nil {
		return Account{}, err
	}
	if password == "" {
		return Account{}, errors.New("请填写密码")
	}
	if password != confirmation {
		return Account{}, errors.New("两次输入的密码不一致")
	}
	unlock, err := m.lockWrite()
	if err != nil {
		return Account{}, err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return Account{}, err
	}
	if accountIndex(m.data.Accounts, account) >= 0 {
		return Account{}, errors.New("账户已存在")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, fmt.Errorf("无法保护密码：%w", err)
	}
	now := time.Now()
	value := Account{Account: account, PasswordHash: string(hash), Enabled: true, Created: now, Updated: now}
	m.data.Accounts = append(m.data.Accounts, value)
	if err := m.persist(); err != nil {
		return Account{}, err
	}
	value.PasswordHash = ""
	return value, nil
}

func (m *Manager) UpdateAccount(account string, password, confirmation *string) (Account, error) {
	unlock, err := m.lockWrite()
	if err != nil {
		return Account{}, err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return Account{}, err
	}
	index := accountIndex(m.data.Accounts, strings.TrimSpace(account))
	if index < 0 {
		return Account{}, errors.New("账户不存在")
	}
	if m.data.Accounts[index].SuperAdmin {
		return Account{}, errors.New("超级管理员账户不能在此修改")
	}
	if password != nil {
		if *password == "" {
			return Account{}, errors.New("请填写密码")
		}
		if confirmation == nil || *password != *confirmation {
			return Account{}, errors.New("两次输入的密码不一致")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return Account{}, fmt.Errorf("无法保护密码：%w", err)
		}
		m.data.Accounts[index].PasswordHash = string(hash)
		m.data.Accounts[index].GitHubID = 0
		m.data.Accounts[index].GitHubLogin = ""
	}
	m.data.Accounts[index].Updated = time.Now()
	if err := m.persist(); err != nil {
		return Account{}, err
	}
	value := m.data.Accounts[index]
	value.PasswordHash = ""
	return value, nil
}

func (m *Manager) DeleteAccount(account string) error {
	unlock, err := m.lockWrite()
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return err
	}
	index := accountIndex(m.data.Accounts, strings.TrimSpace(account))
	if index < 0 {
		return errors.New("账户不存在")
	}
	if m.data.Accounts[index].SuperAdmin {
		return errors.New("不能删除超级管理员账户")
	}
	delete(m.data.MCPKeys, robotScope(m.data.Accounts[index]))
	m.data.Accounts = append(m.data.Accounts[:index], m.data.Accounts[index+1:]...)
	return m.persist()
}

func (m *Manager) lockWrite() (func(), error) {
	m.mu.Lock()
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("无法创建身份认证配置目录：%w", err)
	}
	lock, err := os.OpenFile(m.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if err := lockConfigFile(lock); err != nil {
		lock.Close()
		m.mu.Unlock()
		return nil, err
	}
	return func() {
		lock.Close()
		m.mu.Unlock()
	}, nil
}

func (m *Manager) persist() error {
	encoded, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(m.path), ".alx-auth-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(append(encoded, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), m.path)
}

func (m *Manager) issueToken(data config, account string) (string, error) {
	payload, err := json.Marshal(struct {
		Account string `json:"account"`
		Expires int64  `json:"expires"`
	}{Account: account, Expires: time.Now().Add(sessionDuration).Unix()})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	key, err := base64.RawURLEncoding.DecodeString(data.SessionKey)
	if err != nil {
		return "", errors.New("身份认证会话密钥无效")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (m *Manager) tokenAccount(data config, token string) (Account, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || token == "" {
		return Account{}, false
	}
	key, err := base64.RawURLEncoding.DecodeString(data.SessionKey)
	if err != nil {
		return Account{}, false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return Account{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Account{}, false
	}
	var value struct {
		Account string `json:"account"`
		Expires int64  `json:"expires"`
	}
	if json.Unmarshal(payload, &value) != nil || value.Expires <= time.Now().Unix() {
		return Account{}, false
	}
	index := accountIndex(data.Accounts, value.Account)
	if index < 0 || !data.Accounts[index].Enabled {
		return Account{}, false
	}
	return data.Accounts[index], true
}
