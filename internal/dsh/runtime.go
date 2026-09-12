// Package dsh owns the application-managed DeepSeek Harness sidecar.
//
// The harness is deliberately kept out of the HTTP surface: it speaks
// newline-delimited JSON-RPC only to this package over stdio.  Callers expose
// product-specific DTOs instead of leaking a preview runtime protocol to the
// browser.
package dsh

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"alemonx/internal/resources"
	"alemonx/internal/system"
)

const Version = "0.1.5-rc.1"

type Config struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Root     string `json:"root"`
	APIKey   string `json:"apiKey,omitempty"`
}

// PersistedConfig deliberately has no credential field. Credentials are an
// input to Start and must be sourced from a platform secret store, never from
// the normal ALemonX configuration directory.
type PersistedConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Root     string `json:"root"`
}

func (c PersistedConfig) Valid() error {
	if c.Provider != "deepseek-official" {
		return errors.New("当前仅支持 DeepSeek Official Provider")
	}
	if c.Model == "" {
		return errors.New("请选择 DSH 模型")
	}
	if c.Root == "" || !filepath.IsAbs(c.Root) {
		return errors.New("请选择有效的机器人目录")
	}
	return nil
}

func (c Config) Valid() error {
	if c.Provider != "deepseek-official" {
		return errors.New("当前仅支持 DeepSeek Official Provider")
	}
	if c.Model == "" {
		return errors.New("请选择 DSH 模型")
	}
	if c.Root == "" || !filepath.IsAbs(c.Root) {
		return errors.New("请选择有效的机器人目录")
	}
	if c.APIKey == "" {
		return errors.New("请输入 DeepSeek API Key")
	}
	return nil
}

type Status struct {
	Version   string `json:"version"`
	Running   bool   `json:"running"`
	Ready     bool   `json:"ready"`
	Profile   string `json:"profile"`
	LastError string `json:"lastError,omitempty"`
}

type Event struct {
	Seq    int64           `json:"seq"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Session is the host-owned index for SDK session identities. The SDK lazily
// creates the durable agent on the first prompt; its wire protocol deliberately
// has no create/list/history RPCs.
type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Access is a per-session approval preference owned by ALemonX, rather
	// than a DSH process environment flag. Empty values from older catalogs
	// retain the safe ask-on-every-write default.
	Access    string    `json:"access,omitempty"`
}

var ErrSessionCancellationUnsupported = errors.New("当前 DSH SDK 不支持逐会话取消")

type CommandFactory func(context.Context, string) *exec.Cmd

// Runtime starts one SDK-profile process.  The process has an application
// owned DSH_HOME and a fresh bridge token; user DSH profiles are never read.
type Runtime struct {
	mu         sync.Mutex
	dir        string
	command    CommandFactory
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	pending    map[uint64]chan response
	nextID     atomic.Uint64
	events     chan Event
	subs       map[chan Event]struct{}
	history    []Event
	eventSeq   int64
	done       chan struct{}
	ready      bool
	desired    bool
	restarting bool
	lastErr    error
	bridge     string
	bridgeURL  string
	config     Config
	sessions   map[string]Session
	lockFile   *os.File
	managed    bool
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func New(dir string, factory CommandFactory) *Runtime {
	managed := factory == nil
	if factory == nil {
		factory = defaultCommand
	}
	return &Runtime{dir: dir, command: factory, managed: managed, pending: map[uint64]chan response{}, events: make(chan Event, 128), subs: map[chan Event]struct{}{}, sessions: map[string]Session{}}
}

func DefaultHome() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "alemonjs", "dsh"), nil
}

func (r *Runtime) SaveConfig(cfg Config) error {
	if err := cfg.Valid(); err != nil {
		return err
	}
	return r.savePersistedConfig(PersistedConfig{Provider: cfg.Provider, Model: cfg.Model, Root: cfg.Root})
}

func (r *Runtime) savePersistedConfig(cfg PersistedConfig) error {
	if err := cfg.Valid(); err != nil {
		return err
	}
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lockFile == nil {
		file, err := acquireRuntimeLock(r.dir)
		if err != nil {
			return err
		}
		defer unlockRuntime(file)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	temporary := filepath.Join(r.dir, "runtime.json.tmp")
	if err := os.WriteFile(temporary, append(raw, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(r.dir, "runtime.json"))
}

func (r *Runtime) LoadConfig() (Config, error) {
	persisted, err := r.LoadPersistedConfig()
	if err != nil {
		return Config{}, err
	}
	return Config{Provider: persisted.Provider, Model: persisted.Model, Root: persisted.Root}, errors.New("DSH 凭据必须从安全存储读取")
}

func (r *Runtime) LoadPersistedConfig() (PersistedConfig, error) {
	raw, err := os.ReadFile(filepath.Join(r.dir, "runtime.json"))
	if err != nil {
		return PersistedConfig{}, err
	}
	var cfg PersistedConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return PersistedConfig{}, fmt.Errorf("DSH 配置无效：%w", err)
	}
	return cfg, cfg.Valid()
}

func defaultCommand(ctx context.Context, home string) *exec.Cmd {
	// DSH is installed as an application dependency. ALX_DSH_BIN is only for
	// packaging and tests; it is never an HTTP/user controlled value.
	bin := os.Getenv("ALX_DSH_BIN")
	var prefix []string
	if bin == "" {
		entry, packageErr := resources.DSHProgram()
		node, nodeErr := system.CurrentNodeRuntime()
		if packageErr == nil && nodeErr == nil {
			bin, prefix = node.Path, []string{entry}
		} else {
			bin = filepath.Join(home, "missing-managed-dsh-runtime")
		}
	}
	args := append(prefix, "--profile", "alemonx")
	if patch := filepath.Join(home, "alemonx.patch.yml"); fileExists(patch) {
		args = append(args, "--patch", patch)
	}
	// DSH creates a custom profile only once. Passing the initializer again is
	// a hard error, so subsequent sidecar restarts boot the owned profile.
	if _, err := os.Stat(filepath.Join(home, "profiles", "alemonx", "package.json")); errors.Is(err, os.ErrNotExist) {
		args = append(args, "--from-default-profile", "sdk")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "DSH_HOME="+home)
	system.HideWindow(cmd)
	return cmd
}

func (r *Runtime) Start(ctx context.Context, cfg Config) error {
	if err := cfg.Valid(); err != nil {
		return err
	}
	if r.managed && os.Getenv("ALX_DSH_BIN") == "" {
		if _, err := resources.DSHProgram(); err != nil {
			return err
		}
		if _, err := system.CurrentNodeRuntime(); err != nil {
			return fmt.Errorf("DSH 需要可用的 Node.js：%w", err)
		}
	}
	r.mu.Lock()
	if r.cmd != nil {
		r.mu.Unlock()
		return nil
	}
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		r.mu.Unlock()
		return err
	}
	lockFile, err := acquireRuntimeLock(r.dir)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	started := false
	defer func() {
		if !started {
			unlockRuntime(lockFile)
		}
	}()
	if err := writeRuntimePatch(r.dir); err != nil {
		r.mu.Unlock()
		return err
	}
	bridge, err := randomToken()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	// The sidecar must outlive the HTTP request used to configure it. The
	// caller's context only bounds the initialize RPC below, never the process.
	cmd := r.command(context.Background(), r.dir)
	cmd.Dir = cfg.Root
	cmd.Env = append(managedEnvironment(r.dir, bridge, cfg), "ALX_DSH_BRIDGE_URL="+r.bridgeURL)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return fmt.Errorf("启动 DSH sidecar: %w", err)
	}
	r.lockFile = lockFile
	started = true
	r.cmd, r.stdin, r.done, r.bridge, r.config, r.lastErr = cmd, stdin, make(chan struct{}), bridge, cfg, nil
	r.mu.Unlock()
	go r.read(stdout)
	go r.readStderr(stderr)
	go r.wait(cmd)

	var initialized struct {
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := r.Call(ctx, "initialize", map[string]any{"cwd": cfg.Root, "provider": cfg.Provider, "model": cfg.Model}, &initialized); err != nil {
		_ = r.stop(context.Background(), false)
		return err
	}
	if initialized.ServerInfo.Name != "deepseek-harness-sdk-runtime" {
		_ = r.stop(context.Background(), false)
		return errors.New("DSH sidecar 返回了未知 SDK runtime")
	}
	r.mu.Lock()
	r.ready = true
	r.desired = true
	r.mu.Unlock()
	return nil
}

func (r *Runtime) Call(ctx context.Context, method string, params any, out any) error {
	id := r.nextID.Add(1)
	ch := make(chan response, 1)
	r.mu.Lock()
	if r.stdin == nil {
		r.mu.Unlock()
		return errors.New("DSH runtime 未运行")
	}
	r.pending[id] = ch
	raw, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err == nil {
		_, err = r.stdin.Write(append(raw, '\n'))
	}
	if err != nil {
		delete(r.pending, id)
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	select {
	case reply := <-ch:
		if reply.Error != nil {
			return fmt.Errorf("DSH RPC %s: %s", method, reply.Error.Message)
		}
		if out != nil && len(reply.Result) > 0 {
			return json.Unmarshal(reply.Result, out)
		}
		return nil
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
		return ctx.Err()
	}
}

func (r *Runtime) Prompt(ctx context.Context, sessionID, text string) (string, error) {
	if !r.touchSession(sessionID) {
		return "", errors.New("DSH 会话不存在或不属于当前机器人目录")
	}
	var result struct {
		MessageID string `json:"messageId"`
	}
	err := r.Call(ctx, "session/prompt", map[string]any{"sessionId": sessionID, "contentBlocks": []map[string]any{{"type": "text", "text": text}}}, &result)
	return result.MessageID, err
}

// CreateSession mints a host-owned session identifier. The official SDK creates
// the matching agent lazily when Prompt first uses it.
func (r *Runtime) CreateSession(ctx context.Context) (string, error) {
	_ = ctx
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	item := Session{ID: "alx-" + token, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Access: "ask"}
	r.mu.Lock()
	if r.sessions == nil {
		r.sessions = map[string]Session{}
	}
	r.sessions[item.ID] = item
	err = r.saveSessionsLocked()
	r.mu.Unlock()
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

func (r *Runtime) ListSessions(ctx context.Context) (json.RawMessage, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadSessionsLocked(); err != nil {
		return nil, err
	}
	items := make([]Session, 0, len(r.sessions))
	for _, item := range r.sessions {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return json.Marshal(items)
}

func (r *Runtime) CancelSession(ctx context.Context, sessionID string) error {
	_ = ctx
	_ = sessionID
	return ErrSessionCancellationUnsupported
}

// AcceptsBridgeToken authenticates an application-owned loopback plugin. The
// token is regenerated for every sidecar start and is never exposed through
// browser DTOs or persisted configuration.
func (r *Runtime) AcceptsBridgeToken(token string) bool {
	r.mu.Lock()
	bridge := r.bridge
	r.mu.Unlock()
	if bridge == "" || token == "" || len(bridge) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(bridge), []byte(token)) == 1
}

func (r *Runtime) HasSession(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadSessionsLocked(); err != nil {
		return false
	}
	_, ok := r.sessions[sessionID]
	return ok
}

// SessionAccess returns the persisted approval preference for a runtime-owned
// session. Unknown and historical sessions default to ask, never auto-approve.
func (r *Runtime) SessionAccess(sessionID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadSessionsLocked(); err != nil {
		return "ask"
	}
	access := r.sessions[sessionID].Access
	if access != "auto" && access != "full" {
		return "ask"
	}
	return access
}

// SetSessionAccess persists a user-selected approval preference. It is scoped
// to one runtime-owned session and cannot create an arbitrary session record.
func (r *Runtime) SetSessionAccess(sessionID, access string) error {
	if access != "ask" && access != "auto" && access != "full" {
		return errors.New("DSH 权限模式无效")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadSessionsLocked(); err != nil {
		return err
	}
	item, ok := r.sessions[sessionID]
	if !ok {
		return errors.New("DSH 会话不存在或不属于当前机器人目录")
	}
	item.Access = access
	item.UpdatedAt = time.Now().UTC()
	r.sessions[sessionID] = item
	return r.saveSessionsLocked()
}

func (r *Runtime) touchSession(sessionID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.loadSessionsLocked(); err != nil {
		return false
	}
	item, ok := r.sessions[sessionID]
	if !ok {
		return false
	}
	item.UpdatedAt = time.Now().UTC()
	r.sessions[sessionID] = item
	return r.saveSessionsLocked() == nil
}

func (r *Runtime) loadSessionsLocked() error {
	if r.sessions == nil {
		r.sessions = map[string]Session{}
	}
	raw, err := os.ReadFile(filepath.Join(r.dir, "sessions.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var items []Session
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("DSH 会话目录无效：%w", err)
	}
	for _, item := range items {
		if item.ID != "" {
			r.sessions[item.ID] = item
		}
	}
	return nil
}

func (r *Runtime) saveSessionsLocked() error {
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		return err
	}
	items := make([]Session, 0, len(r.sessions))
	for _, item := range r.sessions {
		items = append(items, item)
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	temporary := filepath.Join(r.dir, "sessions.json.tmp")
	if err := os.WriteFile(temporary, append(raw, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(r.dir, "sessions.json"))
}

// RuntimeRegistry prevents a selected project from silently sharing a DSH
// process or profile with another project. Each root has its own owned home.
type RuntimeRegistry struct {
	mu        sync.Mutex
	baseDir   string
	factory   CommandFactory
	runtimes  map[string]*Runtime
	bridgeURL string
	legacyDir string
	initErr   error
}

func NewRegistry(baseDir string, factory CommandFactory) *RuntimeRegistry {
	return &RuntimeRegistry{baseDir: baseDir, factory: factory, runtimes: map[string]*Runtime{}}
}

func (r *RuntimeRegistry) Runtime(root string) (*Runtime, error) {
	if r.initErr != nil {
		return nil, r.initErr
	}
	root = canonicalRoot(root)
	if !filepath.IsAbs(root) {
		return nil, errors.New("请选择有效的机器人目录")
	}
	sum := sha256.Sum256([]byte(root))
	key := hex.EncodeToString(sum[:16])
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.MigrateLegacy(); err != nil {
		return nil, fmt.Errorf("迁移 DSH 数据失败：%w", err)
	}
	if runtime := r.runtimes[root]; runtime != nil {
		return runtime, nil
	}
	dir := filepath.Join(r.baseDir, "runtimes", key)
	// Keep the original storage identity after a project relocation or path
	// canonicalization change. The persisted root is the authoritative binding.
	if entries, err := os.ReadDir(filepath.Join(r.baseDir, "runtimes")); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			candidate := filepath.Join(r.baseDir, "runtimes", entry.Name())
			if cfg, err := New(candidate, r.factory).LoadPersistedConfig(); err == nil && canonicalRoot(cfg.Root) == root {
				dir = candidate
				break
			}
		}
	}
	runtime := New(dir, r.factory)
	if cfg, err := runtime.LoadPersistedConfig(); err == nil && canonicalRoot(cfg.Root) != root {
		return nil, errors.New("该 DSH 数据已关联到其他机器人目录，请使用迁移后的目录")
	}
	runtime.bridgeURL = r.bridgeURL
	r.runtimes[root] = runtime
	return runtime, nil
}

// SetBridgeEndpoint updates the private loopback endpoint inherited by new
// and subsequently restarted sidecars. It is never derived from an HTTP
// request or exposed in browser-facing status.
func (r *RuntimeRegistry) SetBridgeEndpoint(endpoint string) {
	r.mu.Lock()
	r.bridgeURL = endpoint
	for _, runtime := range r.runtimes {
		runtime.mu.Lock()
		runtime.bridgeURL = endpoint
		runtime.mu.Unlock()
	}
	r.mu.Unlock()
}

func (r *RuntimeRegistry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	runtimes := make([]*Runtime, 0, len(r.runtimes))
	for _, runtime := range r.runtimes {
		runtimes = append(runtimes, runtime)
	}
	r.mu.Unlock()
	var first error
	for _, runtime := range runtimes {
		if err := runtime.Stop(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// RuntimeForBridgeToken resolves a freshly minted sidecar credential without
// accepting a browser-provided root. Callers must also require a loopback peer.
func (r *RuntimeRegistry) RuntimeForBridgeToken(token string) (*Runtime, string, bool) {
	r.mu.Lock()
	items := make(map[string]*Runtime, len(r.runtimes))
	for root, runtime := range r.runtimes {
		items[root] = runtime
	}
	r.mu.Unlock()
	for root, runtime := range items {
		if runtime.AcceptsBridgeToken(token) {
			return runtime, root, true
		}
	}
	return nil, "", false
}

// Restore starts every previously configured project whose key can be resolved
// by the caller. Missing or unavailable credentials never create a process.
func (r *RuntimeRegistry) Restore(ctx context.Context, resolve func(string) (string, error)) []error {
	if err := r.MigrateLegacy(); err != nil {
		return []error{err}
	}
	entries, err := os.ReadDir(filepath.Join(r.baseDir, "runtimes"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []error{err}
	}
	var failures []error
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		runtime := New(filepath.Join(r.baseDir, "runtimes", entry.Name()), r.factory)
		r.mu.Lock()
		runtime.bridgeURL = r.bridgeURL
		r.mu.Unlock()
		persisted, err := runtime.LoadPersistedConfig()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		registered, err := r.Runtime(persisted.Root)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if registered.dir != runtime.dir {
			failures = append(failures, fmt.Errorf("%s: 存在重复 DSH 目录，未启动 %s", persisted.Root, runtime.dir))
			continue
		}
		runtime = registered
		key, err := resolve(persisted.Root)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", persisted.Root, err))
			continue
		}
		cfg := Config{Provider: persisted.Provider, Model: persisted.Model, Root: persisted.Root, APIKey: key}
		if err := runtime.Start(ctx, cfg); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", persisted.Root, err))
			continue
		}
		r.mu.Lock()
		r.runtimes[canonicalRoot(persisted.Root)] = runtime
		r.mu.Unlock()
	}
	return failures
}

func (r *Runtime) Events() <-chan Event { return r.events }

// Subscribe fans SDK session and subagent events out to one product consumer.
// It never exposes the sidecar's stdin/stdout transport to the browser layer.
func (r *Runtime) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	r.mu.Lock()
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		delete(r.subs, ch)
		r.mu.Unlock()
	}
}

// SubscribeAfter provides a bounded replay window before live notifications.
// It closes the normal reconnect race without exposing DSH's raw transport.
func (r *Runtime) SubscribeAfter(after int64) ([]Event, <-chan Event, func()) {
	ch := make(chan Event, 64)
	r.mu.Lock()
	replay := make([]Event, 0)
	for _, event := range r.history {
		if event.Seq > after {
			replay = append(replay, event)
		}
	}
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return replay, ch, func() { r.mu.Lock(); delete(r.subs, ch); r.mu.Unlock() }
}

// PublishEvent appends an application-generated, already-sanitized lifecycle
// event to the same ordered stream as SDK notifications. It is intentionally
// not an RPC transport and never writes to the sidecar.
func (r *Runtime) PublishEvent(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	r.publish(Event{Method: method, Params: raw})
	return nil
}

func (r *Runtime) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := Status{Version: Version, Running: r.cmd != nil, Ready: r.ready, Profile: "alemonx"}
	if r.lastErr != nil {
		status.LastError = r.lastErr.Error()
	}
	return status
}

func (r *Runtime) Stop(ctx context.Context) error {
	return r.stop(ctx, true)
}

func (r *Runtime) stop(ctx context.Context, disableRestart bool) error {
	r.mu.Lock()
	cmd, done := r.cmd, r.done
	if disableRestart {
		r.desired = false
	}
	if cmd == nil {
		r.mu.Unlock()
		return nil
	}
	r.ready = false
	r.mu.Unlock()
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
	_, _ = r.callWithTimeout(shutdownCtx, "shutdown", nil)
	cancel()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) callWithTimeout(ctx context.Context, method string, params any) ([]byte, error) {
	var out json.RawMessage
	err := r.Call(ctx, method, params, &out)
	return out, err
}

func (r *Runtime) read(source io.Reader) {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var frame struct {
			ID     *uint64         `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			continue
		}
		if frame.ID != nil && frame.Method == "" {
			r.mu.Lock()
			ch := r.pending[*frame.ID]
			delete(r.pending, *frame.ID)
			r.mu.Unlock()
			if ch != nil {
				ch <- response{ID: *frame.ID, Result: frame.Result, Error: frame.Error}
			}
			continue
		}
		if frame.Method != "" {
			r.publish(Event{Method: frame.Method, Params: frame.Params})
		}
	}
	if err := scanner.Err(); err != nil {
		r.fail(err)
	}
}

func (r *Runtime) publish(event Event) {
	r.mu.Lock()
	r.eventSeq++
	event.Seq = r.eventSeq
	r.history = append(r.history, event)
	if len(r.history) > 512 {
		r.history = append([]Event(nil), r.history[len(r.history)-512:]...)
	}
	for subscriber := range r.subs {
		select {
		case subscriber <- event:
		default:
		}
	}
	r.mu.Unlock()
	select {
	case r.events <- event:
	default:
	}
}

func (r *Runtime) readStderr(source io.Reader) {
	// DSH reserves stdout for JSON-RPC. Keep stderr out of browser-facing
	// status because a provider may include sensitive request context there;
	// process failure is reported as a generic runtime error instead.
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 8*1024), 64*1024)
	for scanner.Scan() {
		// Draining stderr prevents a noisy plugin from blocking the sidecar.
	}
}

func (r *Runtime) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	r.mu.Lock()
	if r.cmd == cmd {
		if r.lockFile != nil {
			unlockRuntime(r.lockFile)
			r.lockFile = nil
		}
		r.cmd, r.stdin, r.ready = nil, nil, false
		failure := &rpcError{Code: -32000, Message: "DSH runtime 已停止"}
		for id, pending := range r.pending {
			delete(r.pending, id)
			pending <- response{ID: id, Error: failure}
		}
		close(r.done)
		if err != nil {
			r.lastErr = errors.New("DSH sidecar 已退出")
		}
		shouldRestart := err != nil && r.desired && !r.restarting
		if shouldRestart {
			r.restarting = true
			go r.restart()
		}
	}
	r.mu.Unlock()
}

// restart makes transient DSH crashes recoverable without turning the BFF into
// a second agent runtime. It is deliberately bounded: persistent faults remain
// visible to the user instead of causing an unbounded restart loop.
func (r *Runtime) restart() {
	defer func() {
		r.mu.Lock()
		r.restarting = false
		r.mu.Unlock()
	}()
	for _, delay := range []time.Duration{time.Second, 2 * time.Second, 5 * time.Second} {
		time.Sleep(delay)
		r.mu.Lock()
		if !r.desired || r.cmd != nil {
			r.mu.Unlock()
			return
		}
		cfg := r.config
		r.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := r.Start(ctx, cfg)
		cancel()
		if err == nil {
			return
		}
	}
	r.mu.Lock()
	if r.desired && r.cmd == nil {
		r.lastErr = errors.New("DSH sidecar 重启失败")
	}
	r.mu.Unlock()
}

func (r *Runtime) fail(err error) {
	r.mu.Lock()
	r.lastErr = err
	r.mu.Unlock()
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func writeRuntimePatch(home string) error {
	if err := installApprovalBridgeModule(home); err != nil {
		return err
	}
	const patch = `- insert:
    - id: alemonx-approval-bridge
      name: '@alemonx/dsh-approval-bridge'
      config:
        url: !!js process.env.ALX_DSH_BRIDGE_URL
        token: !!js process.env.ALX_DSH_BRIDGE_TOKEN
`
	temporary := filepath.Join(home, "alemonx.patch.yml.tmp")
	if err := os.WriteFile(temporary, []byte(patch), 0600); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(home, "alemonx.patch.yml"))
}

// installApprovalBridgeModule makes the bundled bridge resolvable from a DSH
// profile. DSH resolves external profile plugins from DSH_HOME, not from the
// launcher process's node_modules. Link only our own package, and refuse to
// replace an unexpected filesystem entry in the private runtime home.
func installApprovalBridgeModule(home string) error {
	source := managedApprovalBridgeModule()
	if source == "" {
		return nil // Command factories in unit tests do not carry a Node runtime.
	}
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("受管 DSH approval bridge 不可用")
	}
	destination := filepath.Join(home, "node_modules", "@alemonx", "dsh-approval-bridge")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	if existing, err := os.Lstat(destination); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			// Remove only the old host-owned link, never its destination.
			if err := os.Remove(destination); err != nil {
				return err
			}
		} else if !fileExists(filepath.Join(destination, ".alx-owned")) {
			return errors.New("DSH runtime bridge 目录非工作台托管，未覆盖")
		} else if data, err := os.ReadFile(filepath.Join(destination, ".alx-owned")); err == nil && string(data) == source {
			return nil
		} else {
			if err := os.RemoveAll(destination); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), ".bridge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	// A small module forwards to the versioned package so its dependencies
	// resolve beside the SDK. Copying the plugin alone loses that resolution.
	modulePath := filepath.ToSlash(filepath.Join(source, "index.js"))
	if !strings.HasPrefix(modulePath, "/") {
		modulePath = "/" + modulePath
	}
	moduleURL := (&url.URL{Scheme: "file", Path: modulePath}).String()
	quoted, _ := json.Marshal(moduleURL)
	if err := os.WriteFile(filepath.Join(staging, "index.js"), []byte("export * from "+string(quoted)+";\n"), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "package.json"), []byte(`{"name":"@alemonx/dsh-approval-bridge","type":"module","exports":"./index.js"}`), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, ".alx-owned"), []byte(source), 0600); err != nil {
		return err
	}
	return os.Rename(staging, destination)
}

func managedApprovalBridgeModule() string {
	bin := strings.TrimSpace(os.Getenv("ALX_DSH_BIN"))
	if filepath.IsAbs(bin) {
		candidate := filepath.Join(filepath.Dir(filepath.Dir(bin)), "@alemonx", "dsh-approval-bridge")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if bin == "" {
		entry, err := resources.DSHProgram()
		if err != nil {
			return ""
		}
		packageRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry)))))
		candidate := filepath.Join(packageRoot, "plugins", "approval-bridge")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func canonicalRoot(root string) string {
	root = filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

// managedEnvironment prevents a previously exported DSH configuration or
// provider key from silently becoming this runtime's credentials. Keep normal
// OS variables (PATH, locale, proxy settings) so the bundled Node process is
// still launched normally; replace only DSH and provider-owned inputs.
func managedEnvironment(home, bridge string, cfg Config) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		if key == "DEEPSEEK_API_KEY" || key == "ALX_DSH_BRIDGE_TOKEN" || key == "ALX_DSH_BRIDGE_URL" || strings.HasPrefix(key, "DSH_") {
			continue
		}
		env = append(env, pair)
	}
	return append(env,
		"DSH_HOME="+home,
		"ALX_DSH_BRIDGE_TOKEN="+bridge,
		"DEEPSEEK_API_KEY="+cfg.APIKey,
		"DSH_TELEMETRY_MODE=DISABLED",
	)
}
