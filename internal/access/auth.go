// Package access implements local accounts and role-based access control for alx.
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

// Permissions is the complete, stable list of permissions that can be granted
// to a role. Permission checks are server-side; the UI list is only an editor.
var Permissions = []string{
	"workbench.view",
	"workbench.manage",
	"system.manage",
	"operations.view",
	"operations.manage",
}

type Account struct {
	Account      string    `json:"account"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	Roles        []string  `json:"roles"`
	SuperAdmin   bool      `json:"superAdmin,omitempty"`
	Enabled      bool      `json:"enabled"`
	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
}

type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// Status deliberately contains no password material.
type Status struct {
	Enabled       bool     `json:"enabled"`
	Authenticated bool     `json:"authenticated"`
	Account       string   `json:"account,omitempty"`
	Roles         []string `json:"roles,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	SuperAdmin    bool     `json:"superAdmin,omitempty"`
}

type config struct {
	Enabled bool `json:"enabled"`
	// Account and PasswordHash are retained solely to migrate the old
	// single-account file without locking an existing installation out.
	Account      string    `json:"account,omitempty"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	SessionKey   string    `json:"sessionKey,omitempty"`
	Accounts     []Account `json:"accounts,omitempty"`
	Roles        []Role    `json:"roles,omitempty"`
}

type Manager struct {
	path string
	mu   sync.RWMutex
	data config
}

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
		next.Accounts = []Account{{Account: next.Account, PasswordHash: next.PasswordHash, SuperAdmin: true, Enabled: true, Created: now, Updated: now}}
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

func normalizeRole(role Role) (Role, error) {
	role.ID = strings.TrimSpace(role.ID)
	role.Name = strings.TrimSpace(role.Name)
	if role.ID == "" || len(role.ID) > 64 || strings.ContainsAny(role.ID, "\r\n/") {
		return Role{}, errors.New("角色标识需为 1 到 64 个非换行字符")
	}
	if role.Name == "" || len(role.Name) > 64 || strings.ContainsAny(role.Name, "\r\n") {
		return Role{}, errors.New("请填写角色名称")
	}
	seen := map[string]bool{}
	permissions := make([]string, 0, len(role.Permissions))
	for _, value := range role.Permissions {
		if !isKnownPermission(value) {
			return Role{}, fmt.Errorf("未知权限：%s", value)
		}
		if !seen[value] {
			seen[value] = true
			permissions = append(permissions, value)
		}
	}
	sort.Strings(permissions)
	role.Permissions = permissions
	return role, nil
}

func isKnownPermission(permission string) bool {
	for _, value := range Permissions {
		if value == permission {
			return true
		}
	}
	return false
}

func accountIndex(items []Account, account string) int {
	for index, item := range items {
		if item.Account == account {
			return index
		}
	}
	return -1
}

func roleIndex(items []Role, id string) int {
	for index, item := range items {
		if item.ID == id {
			return index
		}
	}
	return -1
}

func permissionsFor(data config, account Account) []string {
	if account.SuperAdmin {
		return append([]string(nil), Permissions...)
	}
	allowed := map[string]bool{}
	for _, roleID := range account.Roles {
		if index := roleIndex(data.Roles, roleID); index >= 0 {
			for _, permission := range data.Roles[index].Permissions {
				allowed[permission] = true
			}
		}
	}
	items := make([]string, 0, len(allowed))
	for permission := range allowed {
		items = append(items, permission)
	}
	sort.Strings(items)
	return items
}

func (m *Manager) Status(token string) (Status, error) {
	data, err := m.current()
	if err != nil {
		return Status{}, err
	}
	status := Status{Enabled: data.Enabled}
	if !data.Enabled {
		return status, nil
	}
	account, ok := m.tokenAccount(data, token)
	if !ok {
		return status, nil
	}
	status.Authenticated, status.Account = true, account.Account
	status.Roles = append([]string(nil), account.Roles...)
	status.Permissions, status.SuperAdmin = permissionsFor(data, account), account.SuperAdmin
	return status, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.data = config{Enabled: true, SessionKey: base64.RawURLEncoding.EncodeToString(key), Accounts: []Account{{Account: account, PasswordHash: string(hash), SuperAdmin: true, Enabled: true, Created: now, Updated: now}}}
	if err := m.persist(); err != nil {
		return "", err
	}
	return m.issueToken(m.data, account)
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = config{}
	return m.persist()
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

func (m *Manager) Authorize(token, permission string) bool {
	data, err := m.current()
	if err != nil || !data.Enabled {
		return err == nil
	}
	account, ok := m.tokenAccount(data, token)
	if !ok {
		return false
	}
	if account.SuperAdmin {
		return true
	}
	for _, value := range permissionsFor(data, account) {
		if value == permission {
			return true
		}
	}
	return false
}

func (m *Manager) ListAccounts() ([]Account, error) {
	data, err := m.current()
	if err != nil {
		return nil, err
	}
	items := append([]Account(nil), data.Accounts...)
	for index := range items {
		items[index].PasswordHash = ""
		sort.Strings(items[index].Roles)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Account < items[j].Account })
	return items, nil
}

func (m *Manager) ListRoles() ([]Role, error) {
	data, err := m.current()
	if err != nil {
		return nil, err
	}
	items := append([]Role(nil), data.Roles...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Manager) CreateAccount(account, password, confirmation string, roles []string) (Account, error) {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reload(); err != nil {
		return Account{}, err
	}
	if accountIndex(m.data.Accounts, account) >= 0 {
		return Account{}, errors.New("账户已存在")
	}
	if err := m.validateRoles(roles); err != nil {
		return Account{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, fmt.Errorf("无法保护密码：%w", err)
	}
	now := time.Now()
	value := Account{Account: account, PasswordHash: string(hash), Roles: uniqueRoles(roles), Enabled: true, Created: now, Updated: now}
	m.data.Accounts = append(m.data.Accounts, value)
	if err := m.persist(); err != nil {
		return Account{}, err
	}
	value.PasswordHash = ""
	return value, nil
}

func (m *Manager) UpdateAccount(account string, roles []string, password, confirmation *string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	if roles != nil {
		if err := m.validateRoles(roles); err != nil {
			return Account{}, err
		}
		m.data.Accounts[index].Roles = uniqueRoles(roles)
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.data.Accounts = append(m.data.Accounts[:index], m.data.Accounts[index+1:]...)
	return m.persist()
}

func (m *Manager) SaveRole(role Role) (Role, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return Role{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reload(); err != nil {
		return Role{}, err
	}
	if index := roleIndex(m.data.Roles, role.ID); index >= 0 {
		m.data.Roles[index] = role
	} else {
		m.data.Roles = append(m.data.Roles, role)
	}
	if err := m.persist(); err != nil {
		return Role{}, err
	}
	return role, nil
}

// CreateRole makes accidental replacement impossible. Updating an existing
// role is intentionally a separate operation because its permissions apply to
// every bound account immediately.
func (m *Manager) CreateRole(role Role) (Role, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return Role{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reload(); err != nil {
		return Role{}, err
	}
	if roleIndex(m.data.Roles, role.ID) >= 0 {
		return Role{}, errors.New("角色标识已存在")
	}
	m.data.Roles = append(m.data.Roles, role)
	if err := m.persist(); err != nil {
		return Role{}, err
	}
	return role, nil
}

func (m *Manager) DeleteRole(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reload(); err != nil {
		return err
	}
	index := roleIndex(m.data.Roles, strings.TrimSpace(id))
	if index < 0 {
		return errors.New("角色不存在")
	}
	m.data.Roles = append(m.data.Roles[:index], m.data.Roles[index+1:]...)
	for index := range m.data.Accounts {
		m.data.Accounts[index].Roles = withoutRole(m.data.Accounts[index].Roles, id)
	}
	return m.persist()
}

func (m *Manager) validateRoles(roles []string) error {
	for _, id := range uniqueRoles(roles) {
		if roleIndex(m.data.Roles, id) < 0 {
			return fmt.Errorf("角色不存在：%s", id)
		}
	}
	return nil
}
func uniqueRoles(roles []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role != "" && !seen[role] {
			seen[role] = true
			out = append(out, role)
		}
	}
	sort.Strings(out)
	return out
}
func withoutRole(roles []string, deleted string) []string {
	out := roles[:0]
	for _, role := range roles {
		if role != deleted {
			out = append(out, role)
		}
	}
	return out
}

func (m *Manager) persist() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return fmt.Errorf("无法创建身份认证配置目录：%w", err)
	}
	encoded, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, append(encoded, '\n'), 0600)
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
