package systemnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GroupDefinition struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Probe   string `json:"probe"`
	Kind    string `json:"-"`
	Legacy  Route  `json:"-"`
	Mirrors bool   `json:"mirrors"`
}

var ErrNoCandidate = errors.New("此资源没有可用镜像")

var groupDefinitions = []GroupDefinition{
	{"github-api", "GitHub API", "https://api.github.com/repos/lemonade-lab/alemonx/releases?per_page=30", "github", RouteGitHubAPI, true},
	{"github-download", "GitHub 发布下载", "https://github.com/lemonade-lab/alemonx/releases/latest/download/redis-runtime-index.json", "runtime-index", RouteGitHub, true},
	{"github-raw", "GitHub 原始文件", "https://raw.githubusercontent.com/nodejs/node/main/LICENSE", "license", RouteGitHub, true},
	{"github-archive", "GitHub 源码归档", "https://codeload.github.com/octocat/Hello-World/tar.gz/refs/heads/master", "gzip", RouteGitHub, true},
	{"gitee", "Gitee API", "https://gitee.com/api/v5/version", "json", RouteGitee, true},
	{"npm", "NPM", "https://registry.npmjs.org/npm/latest", "npm", RouteNPM, true},
	{"node", "Node.js", "https://nodejs.org/dist/index.json", "node", RouteNode, true},
	{"python", "Python 安装包", "https://www.python.org/ftp/python/", "python", RoutePython, true},
	{"pypi", "PyPI", "https://pypi.org/simple/pip/", "pypi", "", true},
	{"cdn", "jsDelivr CDN", "https://cdn.jsdelivr.net/npm/npm/package.json", "npm", RouteCDN, true},
	{"official", "ALemonX 下载", "https://download.alemonjs.com/application/alemonapp/app-universal-release.apk", "zip", RouteOfficial, true},
}

func GroupDefinitions() []GroupDefinition { return append([]GroupDefinition(nil), groupDefinitions...) }
func groupDefinition(id string) (GroupDefinition, bool) {
	for _, d := range groupDefinitions {
		if d.ID == id {
			return d, true
		}
	}
	return GroupDefinition{}, false
}
func mustURL(value string) *url.URL { u, _ := url.Parse(value); return u }
func groupFor(u *url.URL) string {
	if u.Port() != "" && !(u.Scheme == "https" && u.Port() == "443") && !(u.Scheme == "http" && u.Port() == "80") {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	switch host {
	case "api.github.com":
		return "github-api"
	case "raw.githubusercontent.com":
		return "github-raw"
	case "codeload.github.com":
		return "github-archive"
	case "github.com":
		if strings.Contains(u.Path, "/releases/") {
			return "github-download"
		}
		return ""
	case "registry.npmjs.org":
		return "npm"
	case "nodejs.org":
		if strings.HasPrefix(u.Path, "/dist/") {
			return "node"
		}
	case "python.org", "www.python.org":
		if strings.HasPrefix(u.Path, "/ftp/python/") {
			return "python"
		}
	case "pypi.org":
		if strings.HasPrefix(u.Path, "/simple/") {
			return "pypi"
		}
	case "gitee.com":
		if strings.HasPrefix(u.Path, "/api/") {
			return "gitee"
		}
	case "cdn.jsdelivr.net":
		return "cdn"
	case "download.alemonjs.com":
		return "official"
	}
	return ""
}

type CandidateState struct {
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latencyMs"`
	Message   string `json:"message"`
}
type GroupState struct {
	ID         string           `json:"id"`
	State      string           `json:"state"`
	Current    string           `json:"current"`
	LatencyMS  int64            `json:"latencyMs"`
	CheckedAt  time.Time        `json:"checkedAt"`
	Candidates []CandidateState `json:"candidates"`
}
type selection struct {
	mu       sync.Mutex
	done     chan struct{}
	value    GroupState
	expires  time.Time
	cooldown map[string]time.Time
}

func (m *Manager) selection(c Config, g GroupConfig) *selection {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()
	if m.engines == nil {
		m.engines = map[string]*selection{}
	}
	key := configKey(c, g)
	s := m.engines[key]
	if s == nil {
		s = &selection{value: GroupState{ID: g.ID, State: "idle", Candidates: []CandidateState{}}, cooldown: map[string]time.Time{}}
		m.engines[key] = s
	}
	return s
}
func copyState(s GroupState) GroupState {
	s.Candidates = append([]CandidateState(nil), s.Candidates...)
	return s
}
func (m *Manager) States(c Config) []GroupState {
	out := []GroupState{}
	for _, g := range c.Automatic.Groups {
		s := m.selection(c, g)
		s.mu.Lock()
		out = append(out, copyState(s.value))
		s.mu.Unlock()
	}
	return out
}
func (m *Manager) StartDetection(c Config, id string) error {
	if c.Mode != "auto" {
		return errors.New("仅自动模式支持分组检测")
	}
	if id != "" {
		if _, ok := groupDefinition(id); !ok {
			return errors.New("未知分组")
		}
	}
	for _, g := range c.Automatic.Groups {
		if id == "" || id == g.ID {
			m.launchDetection(c, g, true)
		}
	}
	return nil
}
func (m *Manager) launchDetection(c Config, g GroupConfig, force bool) *selection {
	s := m.selection(c, g)
	s.mu.Lock()
	if s.done != nil || (!force && time.Now().Before(s.expires)) {
		s.mu.Unlock()
		return s
	}
	done := make(chan struct{})
	s.done = done
	s.value.State = "checking"
	cooldown := map[string]time.Time{}
	for k, v := range s.cooldown {
		cooldown[k] = v
	}
	s.mu.Unlock()
	go func() {
		state := probeGroupWith(g, cooldown, m.probe)
		s.mu.Lock()
		s.value = state
		s.expires = time.Now().Add(10 * time.Minute)
		if state.State == "unavailable" || state.Current == "official" && len(g.Candidates) > 0 {
			s.expires = time.Now().Add(60 * time.Second)
		}
		s.done = nil
		close(done)
		s.mu.Unlock()
	}()
	return s
}
func (m *Manager) selected(ctx context.Context, c Config, g GroupConfig) (GroupState, error) {
	s := m.launchDetection(c, g, false)
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return GroupState{}, ctx.Err()
		}
	}
	s.mu.Lock()
	state := copyState(s.value)
	s.mu.Unlock()
	if state.State != "ready" {
		return state, ErrNoCandidate
	}
	return state, nil
}
func (m *Manager) failCandidate(c Config, g GroupConfig, entry string) {
	s := m.selection(c, g)
	s.mu.Lock()
	s.cooldown[entry] = time.Now().Add(time.Minute)
	s.expires = time.Time{}
	s.mu.Unlock()
}
func probeGroupWith(g GroupConfig, cooldown map[string]time.Time, probe func(GroupDefinition, string) CandidateState) GroupState {
	if probe == nil {
		probe = probeCandidate
	}
	d, _ := groupDefinition(g.ID)
	entries := append(append([]string(nil), g.Candidates...), "official")
	state := GroupState{ID: g.ID, State: "unavailable"}
	for _, entry := range entries {
		if time.Now().Before(cooldown[entry]) {
			state.Candidates = append(state.Candidates, CandidateState{URL: entry, Message: "冷却中"})
			continue
		}
		result := probe(d, entry)
		state.Candidates = append(state.Candidates, result)
		if result.OK {
			state.State, state.Current, state.LatencyMS = "ready", entry, result.LatencyMS
			break
		}
	}
	state.CheckedAt = time.Now()
	return state
}
func directTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
}
func probeCandidate(d GroupDefinition, entry string) CandidateState {
	return probeCandidateContext(context.Background(), d, entry)
}
func probeCandidateContext(ctx context.Context, d GroupDefinition, entry string) CandidateState {
	result := CandidateState{URL: entry, Message: "不可用"}
	target := mustURL(d.Probe)
	if entry != "official" {
		var e error
		target, e = rewriteMirrorURL(entry, target)
		if e != nil {
			return result
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if d.Kind == "github" {
		request.Header.Set("Accept", "application/vnd.github+json")
	}
	if d.Kind == "zip" || d.Kind == "gzip" {
		request.Header.Set("Range", "bytes=0-1023")
	}
	t := directTransport()
	defer t.CloseIdleConnections()
	client := &http.Client{Transport: t, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("重定向次数过多")
		}
		if entry != "official" && groupFor(request.URL) == d.ID {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		result.Message = "连接失败"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Message = "连接超时"
		}
		return result
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Message = "HTTP " + strconv.Itoa(response.StatusCode)
		return result
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		result.Message = "读取测试响应失败"
		return result
	}
	if !validProbe(d.Kind, body) {
		result.Message = "返回内容与测试资源不匹配"
		return result
	}
	result.OK = true
	result.Message = "可用"
	result.LatencyMS = time.Since(started).Milliseconds()
	return result
}
func validProbe(kind string, body []byte) bool {
	switch kind {
	case "github":
		var releases []struct {
			Tag string `json:"tag_name"`
		}
		return json.Unmarshal(body, &releases) == nil && len(releases) > 0 && releases[0].Tag != ""
	case "npm":
		var v struct {
			Name    string
			Version string
		}
		return json.Unmarshal(body, &v) == nil && v.Name != "" && v.Version != ""
	case "node":
		var v []struct{ Version string }
		return json.Unmarshal(body, &v) == nil && len(v) > 0 && strings.HasPrefix(v[0].Version, "v")
	case "license":
		return strings.Contains(string(body), "Copyright") && strings.Contains(string(body), "Permission")
	case "python":
		return strings.Contains(string(body), "href=\"3.")
	case "json":
		return json.Valid(body)
	case "pypi":
		return strings.Contains(string(body), "href=") && strings.Contains(string(body), "pip-")
	case "gzip":
		return len(body) > 10 && body[0] == 0x1f && body[1] == 0x8b && body[2] == 8
	case "zip":
		return len(body) >= 4 && string(body[:4]) == "PK\x03\x04"
	case "runtime-index":
		var v struct {
			Version string
			Assets  []json.RawMessage
		}
		return json.Unmarshal(body, &v) == nil && v.Version != "" && len(v.Assets) > 0
	default:
		return len(body) > 0
	}
}
