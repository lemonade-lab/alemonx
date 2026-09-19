package systemnetwork

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrRevision = errors.New("网络配置已更新，请重新打开编辑")

type Config struct {
	Version          int         `json:"version"`
	Revision         uint64      `json:"revision"`
	Mode             string      `json:"mode"`
	Automatic        Automatic   `json:"automatic"`
	Proxy            ProxyConfig `json:"proxy"`
	ConfirmMigration bool        `json:"confirmMigration,omitempty"`
}
type Automatic struct {
	Groups []GroupConfig `json:"groups"`
}
type GroupConfig struct {
	ID         string   `json:"id"`
	Candidates []string `json:"candidates"`
}
type ProxyConfig struct {
	LegacyID       string `json:"legacyId,omitempty"`
	URL            string `json:"url"`
	HasCredentials bool   `json:"hasCredentials,omitempty"`
	Credentials    string `json:"credentials,omitempty"` // preserve, replace, remove
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
}
type Migration struct {
	Pending bool          `json:"pending"`
	Proxies []string      `json:"proxies"`
	Choices []LegacyProxy `json:"choices"`
}
type LegacyProxy struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	HasCredentials bool   `json:"hasCredentials"`
}

func defaultConfig() *Config {
	c := &Config{Version: 2, Mode: "auto"}
	for _, def := range groupDefinitions {
		g := GroupConfig{ID: def.ID, Candidates: []string{}}
		for _, p := range MirrorPresets(def.Legacy) {
			if def.Mirrors {
				g.Candidates = append(g.Candidates, p.Value)
			}
		}
		c.Automatic.Groups = append(c.Automatic.Groups, g)
	}
	return c
}
func cloneConfig(c Config) Config {
	raw, _ := json.Marshal(c)
	var next Config
	_ = json.Unmarshal(raw, &next)
	return next
}
func publicConfig(c Config) *Config {
	next := cloneConfig(c)
	if u, e := url.Parse(next.Proxy.URL); e == nil {
		next.Proxy.HasCredentials = u.User != nil
		u.User = nil
		next.Proxy.URL = u.String()
	}
	next.Proxy.Username = ""
	next.Proxy.Password = ""
	next.Proxy.Credentials = ""
	next.ConfirmMigration = false
	return &next
}
func validateConfig(c Config) error {
	if c.Version != 2 {
		return errors.New("请升级客户端后重新配置网络")
	}
	if c.Mode != "auto" && c.Mode != "direct" && c.Mode != "proxy" {
		return errors.New("网络模式无效")
	}
	if c.Proxy.URL != "" || c.Mode == "proxy" {
		u, e := url.Parse(c.Proxy.URL)
		if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("请输入有效的 HTTP、HTTPS 或 SOCKS5 代理地址")
		}
		if u.Port() != "" {
			port, err := strconv.Atoi(u.Port())
			if err != nil || port < 1 || port > 65535 {
				return errors.New("代理端口范围为 1–65535")
			}
		}
	}
	seen := map[string]bool{}
	for _, g := range c.Automatic.Groups {
		d, ok := groupDefinition(g.ID)
		if !ok || seen[g.ID] {
			return errors.New("自动分组无效")
		}
		seen[g.ID] = true
		if len(g.Candidates) > 16 || (!d.Mirrors && len(g.Candidates) > 0) {
			return errors.New("此分组不支持这些候选入口")
		}
		urls := map[string]bool{}
		for _, candidate := range g.Candidates {
			if len(candidate) > 2048 || urls[candidate] {
				return errors.New("候选入口重复或过长")
			}
			urls[candidate] = true
			if err := validateMirrorURL(candidate); err != nil {
				return err
			}
			u, e := rewriteMirrorURL(candidate, mustURL(d.Probe))
			if e != nil || u.User != nil || u.Fragment != "" {
				return errors.New("候选入口不能包含认证或片段")
			}
			if strings.Contains(candidate, "{nodepath}") && g.ID != "node" || strings.Contains(candidate, "{pythonpath}") && g.ID != "python" {
				return errors.New("入口模板与分组用途不匹配")
			}
		}
	}
	if len(seen) != len(groupDefinitions) {
		return errors.New("自动分组不完整，请刷新")
	}
	return nil
}
func migrateConfig(saved storedSettings) (Config, Migration) {
	c := *defaultConfig()
	migration := Migration{Pending: true, Proxies: []string{}}
	routes := normalizedRoutes(saved.Routes, saved.Mode, saved.ProxyURL)
	for i, g := range c.Automatic.Groups {
		d, _ := groupDefinition(g.ID)
		old := routes[d.Legacy]
		if d.Mirrors && old.MirrorURL != "" {
			found := false
			for _, v := range g.Candidates {
				found = found || v == old.MirrorURL
			}
			if !found {
				c.Automatic.Groups[i].Candidates = append(g.Candidates, old.MirrorURL)
			}
		}
	}
	seen := map[string]bool{}
	privateSeen := map[string]bool{}
	for _, r := range allRoutes {
		p := routes[r]
		if p.Mode == ModeManual {
			if !privateSeen[p.ProxyURL] {
				public := publicRouteSettings(p)
				migration.Choices = append(migration.Choices, LegacyProxy{ID: string(r), URL: public.ProxyURL, HasCredentials: public.HasCredentials})
				privateSeen[p.ProxyURL] = true
			}
			value := publicRouteSettings(p).ProxyURL
			if !seen[value] {
				migration.Proxies = append(migration.Proxies, value)
				seen[value] = true
			}
		}
	}
	if saved.Connection != nil && saved.Connection.Mode == ModeDirect {
		c.Mode = "direct"
	}
	if saved.Connection != nil && saved.Connection.Mode == ModeManual {
		c.Mode = "proxy"
		c.Proxy.URL = saved.Connection.ProxyURL
		c.Proxy.LegacyID = "global"
	}
	return c, migration
}
func (m *Manager) prepareConfig(next Config) (Config, error) {
	next = cloneConfig(next)
	current := uint64(0)
	previous := ""
	if m.config != nil {
		current = m.config.Revision
		previous = m.config.Proxy.URL
	}
	if next.Revision != current {
		return Config{}, ErrRevision
	}
	if len(m.legacyRaw) > 0 && !next.ConfirmMigration {
		return Config{}, errors.New("请先确认网络配置迁移")
	}
	p, e := url.Parse(next.Proxy.URL)
	if e != nil {
		return Config{}, errors.New("代理地址无效")
	}
	if p.User != nil {
		return Config{}, errors.New("请在认证字段填写用户名和密码")
	}
	switch next.Proxy.Credentials {
	case "replace":
		p.User = url.UserPassword(next.Proxy.Username, next.Proxy.Password)
	case "remove":
		p.User = nil
	case "", "preserve":
		candidates := []string{previous}
		if len(m.legacyRaw) > 0 {
			if next.Proxy.LegacyID == "global" && m.settings.Connection != nil {
				candidates = append(candidates, m.settings.Connection.ProxyURL)
			} else if next.Proxy.LegacyID != "" {
				candidates = append(candidates, m.settings.Routes[Route(next.Proxy.LegacyID)].ProxyURL)
			} else {
				matches := map[string]bool{}
				for _, r := range allRoutes {
					old := m.settings.Routes[r].ProxyURL
					u, err := url.Parse(old)
					if err == nil {
						u.User = nil
						if sameProxyEndpoint(u, p) {
							matches[old] = true
						}
					}
				}
				if len(matches) > 1 {
					return Config{}, errors.New("此地址存在不同旧认证，请选择具体旧代理或重新填写认证")
				}
				for old := range matches {
					candidates = append(candidates, old)
				}
			}
		}
		for _, old := range candidates {
			u, err := url.Parse(old)
			if err == nil {
				user := u.User
				u.User = nil
				if sameProxyEndpoint(u, p) {
					p.User = user
					break
				}
			}
		}
	default:
		return Config{}, errors.New("认证操作无效")
	}
	next.Proxy = ProxyConfig{URL: strings.TrimSuffix(p.String(), "/")}
	next.ConfirmMigration = false
	if err := validateConfig(next); err != nil {
		return Config{}, err
	}
	return next, nil
}
func (m *Manager) saveConfig(input Config, preview bool) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := m.prepareConfig(input)
	if err != nil {
		return Settings{}, err
	}
	if preview {
		return Settings{Config: &next}, nil
	}
	next.Revision++
	if m.path != "" {
		if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
			return Settings{}, errors.New("无法保存网络配置")
		}
		if len(m.legacyRaw) > 0 {
			f, err := os.OpenFile(m.path+".v1.bak", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil && !os.IsExist(err) {
				return Settings{}, errors.New("无法备份旧网络配置")
			}
			if os.IsExist(err) {
				old, readErr := os.ReadFile(m.path + ".v1.bak")
				if readErr != nil || !bytes.Equal(old, m.legacyRaw) {
					return Settings{}, errors.New("旧配置备份已存在且内容不同，请先妥善保管备份")
				}
			}
			if err == nil {
				_, err = f.Write(m.legacyRaw)
				if err == nil {
					err = f.Sync()
				}
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					return Settings{}, errors.New("无法备份旧网络配置")
				}
			}
		}
		raw, _ := json.MarshalIndent(next, "", "  ")
		f, err := os.CreateTemp(filepath.Dir(m.path), ".network-*")
		if err != nil {
			return Settings{}, errors.New("无法保存网络配置")
		}
		name := f.Name()
		defer os.Remove(name)
		if _, err = f.Write(raw); err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, m.path)
		}
		if err != nil {
			return Settings{}, errors.New("无法保存网络配置")
		}
	}
	// Only unchanged groups retain their cached selection. In-flight requests
	// hold their old selection, which cannot publish into the new map.
	m.engineMu.Lock()
	retained := map[string]*selection{}
	if m.config != nil {
		for _, g := range next.Automatic.Groups {
			old := groupConfig(*m.config, g.ID)
			key := configKey(next, g)
			oldJSON, _ := json.Marshal(old)
			newJSON, _ := json.Marshal(g)
			oldKey := configKey(*m.config, old)
			if bytes.Equal(oldJSON, newJSON) && m.engines[oldKey] != nil {
				retained[key] = m.engines[oldKey]
			}
		}
	}
	m.engines = retained
	m.engineMu.Unlock()
	m.config = &next
	m.legacyRaw = nil
	return Settings{Config: publicConfig(next)}, nil
}
func (m *Manager) ConfigSnapshot() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return nil
	}
	c := cloneConfig(*m.config)
	return &c
}
func configKey(c Config, g GroupConfig) string {
	raw, _ := json.Marshal(g)
	return fmt.Sprintf("%d:%s:%s", c.Revision, g.ID, raw)
}
func groupConfig(c Config, id string) GroupConfig {
	for _, g := range c.Automatic.Groups {
		if g.ID == id {
			return g
		}
	}
	return GroupConfig{ID: id, Candidates: []string{}}
}

func sameProxyEndpoint(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if value := u.Port(); value != "" {
			return value
		}
		switch u.Scheme {
		case "https":
			return "443"
		case "socks5":
			return "1080"
		default:
			return "80"
		}
	}
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}
