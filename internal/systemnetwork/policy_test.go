package systemnetwork

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func responseFor(r *http.Request, status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}
}
func testConfig(mode string) Config {
	c := *defaultConfig()
	c.Mode = mode
	for i := range c.Automatic.Groups {
		c.Automatic.Groups[i].Candidates = []string{}
	}
	return c
}
func goodProbe(_ GroupDefinition, entry string) CandidateState {
	return CandidateState{URL: entry, OK: true, LatencyMS: 10, Message: "可用"}
}

func TestV2ModesCredentialsRevisionAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	m, _ := NewAt(path)
	c := testConfig("proxy")
	c.Proxy = ProxyConfig{URL: "socks5://localhost:1080", Credentials: "replace", Username: "alice", Password: "secret"}
	saved, err := m.Save(Settings{Config: &c})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(saved.Config)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "alice") || !saved.Proxy.HasCredentials {
		t.Fatal("public credential leak", string(raw))
	}
	if _, err = m.Save(Settings{Config: &c}); !errors.Is(err, ErrRevision) {
		t.Fatal("stale save accepted", err)
	}
	next := *saved.Config
	next.Mode = "direct"
	saved, err = m.Save(Settings{Config: &next})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := NewAt(path)
	if err != nil || loaded.ConfigSnapshot().Proxy.URL != "socks5://alice:secret@localhost:1080" {
		t.Fatal("credential preservation", err)
	}
	next = *saved.Config
	next.Proxy.Credentials = "remove"
	saved, err = m.Save(Settings{Config: &next})
	if err != nil || saved.Proxy.HasCredentials {
		t.Fatal("remove credentials", err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm()&0077 != 0 {
		t.Fatal("unsafe config permissions")
	}
	for _, mode := range []string{"manual", "system", "mirror", ""} {
		next = *saved.Config
		next.Mode = mode
		if _, err = m.Save(Settings{Config: &next}); err == nil {
			t.Fatal("accepted old mode", mode)
		}
	}
}

func TestV2MigrationDoesNotWriteUntilConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	legacy := []byte(`{"routes":{"github":{"mode":"manual","proxyUrl":"http://alice:secret@localhost:8080"},"npm":{"mode":"manual","proxyUrl":"http://other:password@localhost:9090"}}}`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	suggested := m.Settings()
	if suggested.Migration == nil || !suggested.Migration.Pending || len(suggested.Migration.Proxies) != 2 || suggested.Mode != "auto" {
		t.Fatal("bad migration", suggested)
	}
	if _, err = m.Save(Settings{Config: suggested.Config}); err == nil {
		t.Fatal("migration not confirmed")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != string(legacy) {
		t.Fatal("legacy overwritten")
	}
	c := *suggested.Config
	c.ConfirmMigration = true
	c.Mode = "proxy"
	c.Proxy.URL = "http://localhost:8080"
	if _, err = m.Save(Settings{Config: &c}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.ConfigSnapshot().Proxy.URL, "alice:secret@") {
		t.Fatal("legacy credentials lost")
	}
	backup, _ := os.ReadFile(path + ".v1.bak")
	if string(backup) != string(legacy) {
		t.Fatal("backup mismatch")
	}
	if info, _ := os.Stat(path + ".v1.bak"); info.Mode().Perm()&0077 != 0 {
		t.Fatal("unsafe backup")
	}
}

func TestV2DetectionSingleflightCacheStabilityCooldown(t *testing.T) {
	c := testConfig("auto")
	g := GroupConfig{ID: "npm", Candidates: []string{"https://one.test", "https://two.test", "https://three.test"}}
	var active, maximum, calls atomic.Int32
	m := &Manager{config: &c, probe: func(_ GroupDefinition, entry string) CandidateState {
		n := active.Add(1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		calls.Add(1)
		time.Sleep(5 * time.Millisecond)
		latency := int64(100)
		if entry == "official" {
			latency = 109
		}
		return CandidateState{URL: entry, OK: true, LatencyMS: latency}
	}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, err := m.selected(context.Background(), c, g)
			if err != nil || state.Current != "official" {
				t.Error("official tie preference", state, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 4 || maximum.Load() > 3 {
		t.Fatal("duplicate/unbounded probes", calls.Load(), maximum.Load())
	}
	m.failCandidate(c, g, "official")
	state, err := m.selected(context.Background(), c, g)
	if err != nil || state.Current == "official" {
		t.Fatal("cooldown ignored", state, err)
	}
	stable := state.Current
	m.launchDetection(c, g, true)
	state, err = m.selected(context.Background(), c, g)
	if err != nil || state.Current != stable {
		t.Fatal("selection flapped")
	}
}

func TestV2ChangedGroupIsolatedFromOldDetection(t *testing.T) {
	c := testConfig("auto")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	m := &Manager{config: &c, probe: func(d GroupDefinition, e string) CandidateState {
		once.Do(func() { close(started) })
		<-release
		return goodProbe(d, e)
	}}
	old := m.launchDetection(c, groupConfig(c, "npm"), false)
	<-started
	next := cloneConfig(c)
	for i := range next.Automatic.Groups {
		if next.Automatic.Groups[i].ID == "npm" {
			next.Automatic.Groups[i].Candidates = []string{"https://new.test/{path}"}
		}
	}
	saved, err := m.Save(Settings{Config: &next})
	if err != nil {
		t.Fatal(err)
	}
	latest := m.selection(*saved.Config, groupConfig(*saved.Config, "npm"))
	if latest == old {
		t.Fatal("changed group reused task")
	}
	close(release)
	old.mu.Lock()
	done := old.done
	old.mu.Unlock()
	if done != nil {
		<-done
	}
	latest.mu.Lock()
	defer latest.mu.Unlock()
	if latest.value.State != "idle" {
		t.Fatal("old result overwrote new state")
	}
}

func TestV2AutoFailsOverButNeverMirrorsPrivateOrWrites(t *testing.T) {
	c := testConfig("auto")
	for i := range c.Automatic.Groups {
		if c.Automatic.Groups[i].ID == "npm" {
			c.Automatic.Groups[i].Candidates = []string{"https://mirror.test/{path}"}
		}
	}
	m := &Manager{config: &c, probe: goodProbe}
	var hosts []string
	transport := &snapshotTransport{manager: m, config: c, base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Hostname())
		if r.URL.Hostname() == "registry.npmjs.org" {
			return responseFor(r, 503), nil
		}
		return responseFor(r, 200), nil
	})}
	req, _ := http.NewRequest("GET", "https://registry.npmjs.org/npm", nil)
	response, err := transport.RoundTrip(req)
	if err != nil || response.StatusCode != 200 || strings.Join(hosts, ",") != "registry.npmjs.org,mirror.test" {
		t.Fatal(hosts, err)
	}
	response.Body.Close()
	for _, kind := range []string{"authorization", "cookie", "custom", "query", "write"} {
		hosts = nil
		private := req.Clone(context.Background())
		switch kind {
		case "authorization":
			private.Header.Set("Authorization", "Bearer secret")
		case "cookie":
			private.Header.Set("Cookie", "s=secret")
		case "custom":
			private.Header.Set("X-Api-Key", "secret")
		case "query":
			private.URL.RawQuery = "token=secret"
		case "write":
			private.Method = "POST"
		}
		response, err = transport.RoundTrip(private)
		if err != nil || len(hosts) != 1 || hosts[0] != "registry.npmjs.org" {
			t.Fatal("private replay", kind, hosts, err)
		}
		response.Body.Close()
	}
	for _, g := range c.Automatic.Groups {
		s := m.selection(c, g)
		s.mu.Lock()
		s.expires = time.Time{}
		s.mu.Unlock()
	}
	broken := &Manager{config: &c, probe: func(_ GroupDefinition, e string) CandidateState { return CandidateState{URL: e} }}
	if _, err = broken.selected(context.Background(), c, groupConfig(c, "npm")); err == nil {
		t.Fatal("all failure silently succeeded")
	}
}

func TestV2DirectAndProxyIgnoreEnvironmentAndBypassLoopback(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("missing authentication")
		}
		w.Write([]byte("proxy"))
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("local")) }))
	defer local.Close()
	for _, mode := range []string{"direct", "proxy"} {
		c := testConfig(mode)
		c.Proxy.URL = strings.Replace(proxy.URL, "http://", "http://user:secret@", 1)
		m := &Manager{config: &c}
		response, err := m.Client(time.Second).Get(local.URL)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if proxyCalls.Load() != 0 {
			t.Fatal("local proxied")
		}
		if mode == "proxy" {
			response, err = m.Client(time.Second).Get("http://remote.invalid/resource")
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if proxyCalls.Load() != 1 {
				t.Fatal("did not proxy unknown host")
			}
		}
	}
	c := testConfig("proxy")
	c.Proxy.URL = "http://127.0.0.1:1"
	m := &Manager{config: &c}
	if _, err := m.Client(time.Second).Get("http://remote.invalid"); err == nil {
		t.Fatal("proxy failure silently succeeded")
	}
}

func TestV2CommandScopePreservesGitConfigAndClearsInheritedProxy(t *testing.T) {
	previous := defaultManager
	defer SetDefault(previous)
	c := testConfig("direct")
	SetDefault(&Manager{config: &c})
	cmd := exec.Command("git", "clone", "https://example.com/repo.git")
	cmd.Env = []string{"HTTPS_PROXY=http://wrong:1", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: secret"}
	cleanup, err := ApplyCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	env := strings.Join(cmd.Env, "\n")
	if strings.Contains(env, "wrong:1") || !strings.Contains(env, "GIT_CONFIG_KEY_0=http.extraHeader") || !strings.Contains(env, "GIT_CONFIG_COUNT=4") {
		t.Fatal("bad scoped env", env)
	}
	if ManagedCommand(exec.Command("npm", "start")) {
		t.Fatal("robot launcher classified as owned networking")
	}
}

func TestV2SOCKS5AuthenticationAndHTTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		header := make([]byte, 2)
		if _, e = io.ReadFull(conn, header); e != nil {
			done <- e
			return
		}
		methods := make([]byte, int(header[1]))
		io.ReadFull(conn, methods)
		conn.Write([]byte{5, 2})
		io.ReadFull(conn, header)
		user := make([]byte, int(header[1]))
		io.ReadFull(conn, user)
		io.ReadFull(conn, header[:1])
		pass := make([]byte, int(header[0]))
		io.ReadFull(conn, pass)
		if string(user) != "user" || string(pass) != "secret" {
			done <- errors.New("bad socks auth")
			return
		}
		conn.Write([]byte{1, 0})
		greeting := make([]byte, 4)
		io.ReadFull(conn, greeting)
		if greeting[3] != 3 {
			done <- errors.New("expected remote DNS")
			return
		}
		io.ReadFull(conn, header[:1])
		address := make([]byte, int(header[0])+2)
		io.ReadFull(conn, address)
		conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
		buffer := make([]byte, 4096)
		conn.Read(buffer)
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
		done <- nil
	}()
	c := testConfig("proxy")
	c.Proxy.URL = "socks5://user:secret@" + listener.Addr().String()
	m := &Manager{config: &c}
	response, err := m.Client(3 * time.Second).Get("http://remote.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestV2ProbeRejectsWrongContentAndDomainBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>login</html>")) }))
	defer server.Close()
	result := probeCandidate(GroupDefinition{Probe: server.URL, Kind: "npm"}, "official")
	if result.OK {
		t.Fatal("HTML accepted as registry")
	}
	for _, raw := range []string{"https://api.github.com.evil.test/", "https://evilgithub.com/", "https://github.com/login", "https://pypi.org/account/"} {
		u, _ := url.Parse(raw)
		if groupFor(u) != "" {
			t.Fatal("broad domain match", raw)
		}
	}
}

func TestV2HTTPSProxyAndRedirectPolicySnapshot(t *testing.T) {
	var firstCalls, secondCalls atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { secondCalls.Add(1); w.Write([]byte("wrong")) }))
	defer second.Close()
	c := testConfig("proxy")
	m := &Manager{config: &c}
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstCalls.Add(1)
		if r.Header.Get("Proxy-Authorization") == "" {
			t.Error("missing TLS proxy credentials")
		}
		if r.URL.Path == "/start" {
			next := *m.Settings().Config
			next.Proxy = ProxyConfig{URL: second.URL, Credentials: "remove"}
			if _, err := m.Save(Settings{Config: &next}); err != nil {
				t.Error(err)
			}
			w.Header().Set("Location", "http://other.invalid/end")
			w.WriteHeader(302)
			return
		}
		w.Write([]byte("pinned"))
	}))
	defer first.Close()
	pool := x509.NewCertPool()
	pool.AddCert(first.Certificate())
	original := http.DefaultTransport
	trusted := original.(*http.Transport).Clone()
	trusted.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	http.DefaultTransport = trusted
	defer func() { http.DefaultTransport = original; trusted.CloseIdleConnections() }()
	c.Proxy.URL = strings.Replace(first.URL, "https://", "https://user:secret@", 1)
	response, err := m.Client(3 * time.Second).Get("http://remote.invalid/start")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "pinned" || firstCalls.Load() != 2 || secondCalls.Load() != 0 {
		t.Fatal("in-flight policy changed", string(body), firstCalls.Load(), secondCalls.Load())
	}
	response, err = m.Client(time.Second).Get("http://remote.invalid/new")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if secondCalls.Load() != 1 {
		t.Fatal("new request kept old policy")
	}
}

func TestV2CallerRedirectSecurityIsNotBypassed(t *testing.T) {
	var landed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { landed.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer source.Close()
	c := testConfig("direct")
	client := (&Manager{config: &c}).Client(time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 302 || landed.Load() != 0 {
		t.Fatal("redirect security bypassed")
	}
}

func TestV2ProxyFailuresNeverFallback(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			c := testConfig("proxy")
			c.Proxy.URL = scheme + "://127.0.0.1:1"
			m := &Manager{config: &c}
			if _, err := m.Client(100 * time.Millisecond).Get("http://remote.invalid/"); err == nil {
				t.Fatal("failed proxy accepted")
			}
		})
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(407) }))
	defer denied.Close()
	c := testConfig("proxy")
	c.Proxy.URL = denied.URL
	m := &Manager{config: &c}
	response, err := m.Client(time.Second).Get("http://remote.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 407 {
		t.Fatal("authentication failure replayed")
	}
	untrusted := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("unsafe")) }))
	defer untrusted.Close()
	c.Proxy.URL = untrusted.URL
	if _, err = m.Client(time.Second).Get("http://remote.invalid/"); err == nil {
		t.Fatal("untrusted HTTPS proxy accepted")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer slow.Close()
	c.Proxy.URL = slow.URL
	started := time.Now()
	if _, err = m.Client(30 * time.Millisecond).Get("http://remote.invalid/"); err == nil || time.Since(started) > 500*time.Millisecond {
		t.Fatal("proxy timeout ignored", err)
	}
}

func TestV2TCPProxyAndLoopback(t *testing.T) {
	old := defaultManager
	defer SetDefault(old)
	c := testConfig("proxy")
	c.Proxy.URL = "http://127.0.0.1:1"
	SetDefault(&Manager{config: &c})
	if _, err := (Dialer{Timeout: 100 * time.Millisecond}).Dial("tcp", "remote.invalid:6379"); err == nil {
		t.Fatal("TCP proxy failure bypassed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	conn, err := (Dialer{Timeout: time.Second}).Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal("loopback sent to external proxy", err)
	}
	conn.Close()
}

func TestV2MigrationKeepsDistinctCredentialsOnSameAddress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "network.json")
	legacy := []byte(`{"routes":{"github":{"mode":"manual","proxyUrl":"http://first:secret@localhost:8080"},"npm":{"mode":"manual","proxyUrl":"http://second:password@localhost:8080"}}}`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewAt(path)
	if err != nil {
		t.Fatal(err)
	}
	suggested := m.Settings()
	if len(suggested.Migration.Choices) != 2 {
		t.Fatal("distinct proxies merged")
	}
	c := *suggested.Config
	c.ConfirmMigration = true
	c.Mode = "proxy"
	c.Proxy.URL = "http://localhost:8080"
	if _, err = m.Save(Settings{Config: &c}); err == nil {
		t.Fatal("ambiguous credentials automatically selected")
	}
	c.Proxy.LegacyID = "npm"
	if _, err = m.Save(Settings{Config: &c}); err != nil {
		t.Fatal(err)
	}
	if m.ConfigSnapshot().Proxy.URL != "http://second:password@localhost:8080" {
		t.Fatal("wrong selected credentials")
	}
}
