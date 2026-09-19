package systemnetwork

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/net/proxy"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ApplyCommand scopes network changes to an application-owned command. Robot
// runtime launchers deliberately do not call this function.
func ManagedCommand(cmd *exec.Cmd) bool {
	name := strings.ToLower(filepath.Base(cmd.Path))
	if strings.TrimSuffix(name, ".exe") == "git" {
		return true
	}
	if strings.TrimSuffix(name, ".exe") == "node" && len(cmd.Args) > 1 {
		name = strings.ToLower(filepath.Base(cmd.Args[1]))
	}
	if !strings.Contains(name, "npm") && !strings.Contains(name, "yarn") && !strings.Contains(name, "pip") && !strings.Contains(name, "npx") {
		return false
	}
	for _, arg := range cmd.Args[1:] {
		switch arg {
		case "install", "add", "ci", "view", "info", "publish", "whoami", "pack", "create", "init", "update", "upgrade":
			return true
		}
	}
	return strings.Contains(name, "npx")
}

func UsesGlobalPolicy() bool {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultManager.ConfigSnapshot() != nil
}

func CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	cleanup, err := ApplyCommand(cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return cmd.CombinedOutput()
}

func ApplyCommand(cmd *exec.Cmd) (func(), error) {
	defaultMu.RLock()
	m := defaultManager
	defaultMu.RUnlock()
	c := m.ConfigSnapshot()
	if c == nil {
		return func() {}, nil
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	env = append([]string(nil), env...)
	values := map[string]string{"HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "NO_PROXY": "localhost,127.0.0.1,::1", "http_proxy": "", "https_proxy": "", "all_proxy": "", "no_proxy": "localhost,127.0.0.1,::1", "npm_config_proxy": "", "npm_config_https_proxy": "", "npm_config_registry": "https://registry.npmjs.org/", "YARN_REGISTRY": "https://registry.npmjs.org/", "PIP_INDEX_URL": "https://pypi.org/simple", "PIP_EXTRA_INDEX_URL": "", "PIP_PROXY": "", "PIP_CONFIG_FILE": os.DevNull, "PYTHON_BUILD_MIRROR_URL": ""}
	cleanup := func() {}
	proxyURL := ""
	values["NVM_NODEJS_ORG_MIRROR"] = "https://nodejs.org/dist"
	values["NVM_IOJS_ORG_MIRROR"] = "https://iojs.org/dist"
	values["npm_config_replace_registry_host"] = "always"
	if c.Mode == "proxy" {
		var err error
		proxyURL, cleanup, err = commandGateway(*c)
		if err != nil {
			return func() {}, err
		}
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "npm_config_proxy", "npm_config_https_proxy", "PIP_PROXY"} {
			values[key] = proxyURL
		}
	}
	name := strings.ToLower(filepath.Base(cmd.Path))
	if name == "sudo" {
		cmd.Args = append([]string{cmd.Args[0], "--preserve-env=HTTP_PROXY,HTTPS_PROXY,ALL_PROXY,NO_PROXY,http_proxy,https_proxy,all_proxy,no_proxy"}, cmd.Args[1:]...)
	}
	if strings.TrimSuffix(name, ".exe") == "winget" {
		if c.Mode == "proxy" {
			cleanup()
			return func() {}, errors.New("WinGet 不支持此带认证的临时代理通道；请使用受管下载或离线安装，未回退直连")
		}
		cmd.Args = append(cmd.Args, "--no-proxy")
	}
	if strings.TrimSuffix(name, ".exe") == "choco" {
		if proxyURL == "" {
			var err error
			proxyURL, cleanup, err = commandGateway(*c)
			if err != nil {
				return func() {}, err
			}
		}
		u, _ := url.Parse(proxyURL)
		password, _ := u.User.Password()
		user := u.User.Username()
		u.User = nil
		cmd.Args = append(cmd.Args, "--proxy="+u.String(), "--proxy-user="+user, "--proxy-password="+password, "--proxy-bypass-list=^$")
	}
	tool := name
	if strings.TrimSuffix(name, ".exe") == "node" && len(cmd.Args) > 1 {
		tool = strings.ToLower(filepath.Base(cmd.Args[1]))
	}
	packageTool := strings.Contains(tool, "npm") || strings.Contains(tool, "yarn") || strings.Contains(tool, "pnpm") || strings.Contains(tool, "npx")
	if strings.Contains(tool, "python") {
		for _, arg := range cmd.Args {
			if arg == "pip" {
				tool = "pip"
			}
		}
	}
	networkRead := false
	for _, arg := range cmd.Args[1:] {
		if arg == "install" || arg == "add" || arg == "ci" || arg == "view" || arg == "info" {
			networkRead = true
		}
	}
	if strings.Contains(tool, "npx") {
		networkRead = true
	}
	if c.Mode == "auto" && strings.Contains(strings.Join(cmd.Args, " "), "alemonx-nvm") && strings.Contains(strings.Join(cmd.Args, " "), "install") {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		state, err := m.selected(ctx, *c, groupConfig(*c, "node"))
		if err != nil {
			cleanup()
			return func() {}, err
		}
		if state.Current != "official" {
			u, err := rewriteMirrorURL(state.Current, mustURL("https://nodejs.org/dist/"))
			if err != nil {
				cleanup()
				return func() {}, err
			}
			values["NVM_NODEJS_ORG_MIRROR"] = strings.TrimSuffix(u.String(), "/")
		}
	}
	if c.Mode == "auto" && packageTool && networkRead && !commandHasCredentials(cmd, env) {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		state, err := m.selected(ctx, *c, groupConfig(*c, "npm"))
		if err != nil {
			cleanup()
			return func() {}, err
		}
		if state.Current != "official" {
			target, err := rewriteMirrorURL(state.Current, mustURL("https://registry.npmjs.org/"))
			if err != nil {
				cleanup()
				return func() {}, err
			}
			values["npm_config_registry"] = target.String()
			values["YARN_REGISTRY"] = target.String()
		}
	}
	if c.Mode == "auto" && (strings.Contains(tool, "pip") || strings.Contains(tool, "pyenv")) && networkRead && !commandHasCredentials(cmd, env) {
		group, origin := "pypi", "https://pypi.org/simple/"
		if strings.Contains(tool, "pyenv") {
			group, origin = "python", "https://www.python.org/ftp/python/"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		state, err := m.selected(ctx, *c, groupConfig(*c, group))
		if err != nil {
			cleanup()
			return func() {}, err
		}
		if state.Current != "official" {
			u, err := rewriteMirrorURL(state.Current, mustURL(origin))
			if err != nil {
				cleanup()
				return func() {}, err
			}
			if group == "pypi" {
				values["PIP_INDEX_URL"] = u.String()
			} else {
				values["PYTHON_BUILD_MIRROR_URL"] = strings.TrimSuffix(u.String(), "/")
			}
		}
	}
	// CLI flags outrank project/user registry and proxy settings, without editing them.
	if packageTool && networkRead {
		cmd.Args = append(cmd.Args, "--registry="+values["npm_config_registry"], "--proxy="+proxyURL, "--https-proxy="+proxyURL)
	}
	if tool == "pip" || strings.HasPrefix(tool, "pip3") {
		if networkRead {
			cmd.Args = append(cmd.Args, "--index-url="+values["PIP_INDEX_URL"], "--proxy="+proxyURL)
		}
	}
	for _, arg := range cmd.Args {
		switch filepath.Base(arg) {
		case "apt-get":
			value := proxyURL
			if value == "" {
				value = "DIRECT"
			}
			cmd.Args = append(cmd.Args, "-o", "Acquire::http::Proxy="+value, "-o", "Acquire::https::Proxy="+value)
		case "dnf", "yum":
			value := proxyURL
			if value == "" {
				value = "_none_"
			}
			cmd.Args = append(cmd.Args, "--setopt=proxy="+value)
		}
	}
	// Command-scoped Git configuration overrides global proxy defaults without
	// rewriting .gitconfig or touching authentication helpers.
	if strings.TrimSuffix(name, ".exe") == "git" {
		pairs := [][2]string{{"http.proxy", proxyURL}, {"https.proxy", proxyURL}}
		// URL-scoped proxies outrank http.proxy, including configuration from a
		// repository and includeIf files. Read keys locally, then override only
		// this process; authentication helpers and headers remain intact.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		query := exec.CommandContext(ctx, cmd.Path, "config", "--get-regexp", `^(http\..*\.proxy|https\..*\.proxy|url\..*\.insteadof|remote\..*\.url)$`)
		query.Dir, query.Env = cmd.Dir, cmd.Env
		if raw, err := query.Output(); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				key, value, found := strings.Cut(line, " ")
				if !found {
					continue
				}
				lower := strings.ToLower(key)
				if strings.HasSuffix(lower, ".proxy") {
					pairs = append(pairs, [2]string{key, proxyURL})
				}
				if strings.HasPrefix(lower, "url.") {
					pairs = append(pairs, [2]string{key, "alemonx-disabled-rewrite://"})
				}
				if c.Mode == "proxy" && strings.HasPrefix(lower, "remote.") && (strings.HasPrefix(value, "git@") || strings.HasPrefix(value, "ssh://")) {
					cleanup()
					return func() {}, errors.New("当前全局代理要求使用 HTTPS Git 远端")
				}
			}
		}
		for _, arg := range cmd.Args {
			u, err := url.Parse(arg)
			if err == nil && u.Host != "" {
				pairs = append(pairs, [2]string{"http." + u.String() + ".proxy", proxyURL})
			}
		}
		count := 0
		for _, entry := range env {
			if strings.HasPrefix(entry, "GIT_CONFIG_COUNT=") {
				count, _ = strconv.Atoi(strings.TrimPrefix(entry, "GIT_CONFIG_COUNT="))
			}
		}
		if count < 0 || count > 1024 {
			cleanup()
			return func() {}, errors.New("Git 命令配置无效")
		}
		if c.Mode == "proxy" {
			for _, arg := range cmd.Args {
				if strings.HasPrefix(arg, "git@") || strings.HasPrefix(arg, "ssh://") {
					cleanup()
					return func() {}, errors.New("当前全局代理要求使用 HTTPS Git 地址")
				}
			}
		}
		values["GIT_CONFIG_COUNT"] = strconv.Itoa(count + len(pairs))
		for i, p := range pairs {
			values[fmt.Sprintf("GIT_CONFIG_KEY_%d", count+i)] = p[0]
			values[fmt.Sprintf("GIT_CONFIG_VALUE_%d", count+i)] = p[1]
		}
	}
	for k, v := range values {
		filtered := env[:0]
		for _, entry := range env {
			key, _, _ := strings.Cut(entry, "=")
			if key != k {
				filtered = append(filtered, entry)
			}
		}
		env = append(filtered, k+"="+v)
	}
	cmd.Env = env
	return cleanup, nil
}

func commandHasCredentials(cmd *exec.Cmd, env []string) bool {
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		key = strings.ToLower(key)
		if value != "" && (strings.Contains(key, "token") || strings.Contains(key, "password") || strings.Contains(key, "auth")) {
			return true
		}
	}
	paths := []string{filepath.Join(cmd.Dir, ".npmrc"), filepath.Join(cmd.Dir, ".yarnrc")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".npmrc"), filepath.Join(home, ".yarnrc"))
	}
	for _, path := range paths {
		if raw, err := os.ReadFile(path); err == nil {
			lower := strings.ToLower(string(raw))
			if strings.Contains(lower, "auth") || strings.Contains(lower, "password") || strings.Contains(lower, "registry") {
				return true
			}
		}
	}
	return false
}
func commandGateway(c Config) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", func() {}, errors.New("无法创建命令网络通道")
	}
	token := make([]byte, 24)
	if _, err = rand.Read(token); err != nil {
		listener.Close()
		return "", func() {}, errors.New("无法创建命令网络通道")
	}
	secret := hex.EncodeToString(token)
	authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("alx:"+secret))
	pinned := &Manager{config: &c}
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	var tunnelsMu sync.Mutex
	tunnels := map[net.Conn]bool{}
	closed := false
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != authorization {
			w.WriteHeader(407)
			return
		}
		r.Header.Del("Proxy-Authorization")
		if r.Method == http.MethodConnect {
			var upstream net.Conn
			var err error
			if c.Mode == "proxy" {
				upstream, err = dialProxy(r.Context(), c.Proxy.URL, r.Host)
			} else {
				upstream, err = (&net.Dialer{Timeout: 10 * time.Second}).DialContext(r.Context(), "tcp", r.Host)
			}
			if err != nil {
				http.Error(w, "代理连接失败", 502)
				return
			}
			local, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				upstream.Close()
				return
			}
			tunnelsMu.Lock()
			if closed {
				tunnelsMu.Unlock()
				local.Close()
				upstream.Close()
				return
			}
			tunnels[local], tunnels[upstream] = true, true
			tunnelsMu.Unlock()
			_, _ = local.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
			go func() {
				defer func() { tunnelsMu.Lock(); delete(tunnels, local); delete(tunnels, upstream); tunnelsMu.Unlock() }()
				defer local.Close()
				defer upstream.Close()
				go func() { _, _ = io.Copy(upstream, buffer); upstream.Close() }()
				_, _ = io.Copy(local, upstream)
			}()
			return
		}
		copy := r.Clone(r.Context())
		copy.RequestURI = ""
		response, err := pinned.Client(0).Do(copy)
		if err != nil {
			http.Error(w, "代理连接失败", 502)
			return
		}
		defer response.Body.Close()
		for key, values := range response.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	})
	go server.Serve(listener)
	return "http://alx:" + secret + "@" + listener.Addr().String(), func() {
		_ = server.Close()
		tunnelsMu.Lock()
		defer tunnelsMu.Unlock()
		closed = true
		for conn := range tunnels {
			_ = conn.Close()
		}
	}, nil
}
func dialProxy(ctx context.Context, address, target string) (net.Conn, error) {
	if bypassProxy(&url.URL{Host: target}) {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", target)
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, errors.New("代理地址无效")
	}
	if u.Scheme == "socks5" {
		d, e := proxy.FromURL(u, &net.Dialer{Timeout: 10 * time.Second})
		if e != nil {
			return nil, e
		}
		return d.(proxy.ContextDialer).DialContext(ctx, "tcp", target)
	}
	host := u.Host
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "https" {
			port = "443"
		}
		host = net.JoinHostPort(u.Hostname(), port)
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	cancelConn := conn
	stopCancel := context.AfterFunc(ctx, func() { _ = cancelConn.Close() })
	defer stopCancel()
	if u.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = secure
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if u.User != nil {
		password, _ := u.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+password)))
	}
	if err = request.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil || response.StatusCode != 200 {
		conn.Close()
		return nil, errors.New("代理拒绝连接")
	}
	_ = conn.SetDeadline(time.Time{})
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
