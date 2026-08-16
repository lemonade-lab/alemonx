// Package web provides the embedded setup guide and its HTTP API.
package web

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"alemonx/internal/access"
	"alemonx/internal/agent"
	"alemonx/internal/ai"
	"alemonx/internal/catalog"
	"alemonx/internal/githubauth"
	"alemonx/internal/logging"
	"alemonx/internal/project"
	"alemonx/internal/redis"
	"alemonx/internal/releases"
	"alemonx/internal/robot"
	"alemonx/internal/setupplugin"
	"alemonx/internal/system"
	"alemonx/internal/systemnetwork"
)

var systemPickerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

type goal struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	DownloadURL string   `json:"downloadUrl,omitempty"`
	Mirrors     []mirror `json:"mirrors,omitempty"`
}

type mirror struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type operationTask struct {
	ID         string          `json:"id"`
	Root       string          `json:"root"`
	Path       string          `json:"path,omitempty"`
	Action     string          `json:"action"`
	Status     string          `json:"status"`
	Output     string          `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Progress   int             `json:"progress"`
	Steps      []operationStep `json:"steps,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
}

// operationStep is a bounded, durable timeline for long-running operations.
// Keeping it on the task means a page refresh or SSE reconnect still shows
// which download, verification, or rollback phase was reached.
type operationStep struct {
	At       time.Time `json:"at"`
	Progress int       `json:"progress"`
	Message  string    `json:"message"`
}

// robotEvent is pushed to subscribed SSE clients whenever a supervised task
// changes state or a running process emits output. Type is "task" (full task
// snapshot), "output" (incremental text), "app-ready", or "app-failed".
type robotEvent struct {
	ID        int64          `json:"id,omitempty"`
	Type      string         `json:"type"`
	TaskID    string         `json:"taskId,omitempty"`
	Task      *operationTask `json:"task,omitempty"`
	Text      string         `json:"text,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
}

// robotEventHub fans events out to open SSE subscribers. A dropped channel is
// pruned on publish so a slow client cannot block the event loop.
type robotEventHub struct {
	mu          sync.Mutex
	subscribers map[chan robotEvent]struct{}
}

type opsEvent struct {
	Type string `json:"type"`
	Root string `json:"root,omitempty"`
}

type opsEventHub struct {
	mu          sync.Mutex
	subscribers map[chan opsEvent]struct{}
}

type mcpEventHub struct {
	mu          sync.Mutex
	running     bool
	subscribers map[chan bool]struct{}
}

func newMCPEventHub() *mcpEventHub { return &mcpEventHub{subscribers: map[chan bool]struct{}{}} }
func (h *mcpEventHub) subscribe() chan bool {
	ch := make(chan bool, 4)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}
func (h *mcpEventHub) unsubscribe(ch chan bool) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}
func (h *mcpEventHub) status() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.running }
func (h *mcpEventHub) set(running bool) bool {
	h.mu.Lock()
	if h.running == running {
		h.mu.Unlock()
		return false
	}
	h.running = running
	for ch := range h.subscribers {
		select {
		case ch <- running:
		default:
		}
	}
	h.mu.Unlock()
	return true
}

func newOpsEventHub() *opsEventHub {
	return &opsEventHub{subscribers: map[chan opsEvent]struct{}{}}
}

func (h *opsEventHub) subscribe() chan opsEvent {
	ch := make(chan opsEvent, 32)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *opsEventHub) unsubscribe(ch chan opsEvent) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

func (h *opsEventHub) publish(event opsEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			// Coalesce bursts. The next snapshot request is authoritative.
		}
	}
}

func newRobotEventHub() *robotEventHub {
	return &robotEventHub{subscribers: map[chan robotEvent]struct{}{}}
}

func (h *robotEventHub) subscribe() chan robotEvent {
	ch := make(chan robotEvent, 64)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *robotEventHub) unsubscribe(ch chan robotEvent) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

func (h *robotEventHub) publish(event robotEvent) {
	h.mu.Lock()
	for ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			delete(h.subscribers, ch)
		}
	}
	h.mu.Unlock()
}

// publishRobotEvent persists the latest operation snapshots and the ordered
// event before waking subscribers. If persistence fails the REST snapshot is
// still usable, but a reconnect is never told about an event that was not
// safely recorded.
func (s *server) publishRobotEvent(event robotEvent) {
	s.mu.RLock()
	operations := append([]operationTask(nil), s.operations...)
	s.mu.RUnlock()
	persisted, ok := s.publishEvent("robot", event.Type, event, operations)
	if ok {
		event.ID = persisted.ID
	}
	if !ok && s.operationEvents != nil {
		return
	}
	s.events.publish(event)
}

type developmentProcess struct {
	TaskID  string
	Command *exec.Cmd
	// PGID is the process-group id created for the command (unix: its own pid).
	// Persisted so an orphaned node can be killed even after alx restarts.
	PGID int
	// Cleanup releases process-owned resources (such as a temporary sandbox
	// config) after the process exits.
	Cleanup func()
}

// persistedProcess is the on-disk marker for a supervised robot process. It
// survives alx restarts so a stray node that kept the port can be found and
// killed instead of escaping ("端口逃逸").
type persistedProcess struct {
	Root      string    `json:"root"`
	TaskID    string    `json:"taskId"`
	PGID      int       `json:"pgid"`
	Action    string    `json:"action"`
	StartedAt time.Time `json:"startedAt"`
}

func processesFilePath() string {
	if override := os.Getenv("ALEMONX_PROCESS_FILE"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".alemonx", "processes.json")
}

func loadPersistedProcesses() []persistedProcess {
	path := processesFilePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var items []persistedProcess
	if json.Unmarshal(data, &items) != nil {
		return nil
	}
	return items
}

func savePersistedProcesses(items []persistedProcess) {
	path := processesFilePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	data, _ := json.Marshal(items)
	_ = os.WriteFile(path, data, 0600)
}

// botAppPageRuntime is separate from a robot's app/dev process. It hosts
// alemonjs/desktop.js and exchanges plugin messages through stdin/stdout.
type botAppPageRuntime struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	events  map[string][]json.RawMessage
	streams map[string]map[chan json.RawMessage]struct{}
}

// liveUpload is deliberately short lived. It binds a temporary file to one
// validated robot root and one browser CBP device id; it is never a general
// purpose file-read endpoint.
type liveUpload struct {
	ID        string
	Root      string
	DeviceID  string
	Path      string
	Filename  string
	Size      int64
	MIMEType  string
	ExpiresAt time.Time
}

type server struct {
	version            string
	assets             fs.FS
	static             http.Handler
	checker            *system.Checker
	network            *systemnetwork.Manager
	creator            *project.Creator
	robots             robot.Manager
	plugins            *setupplugin.Registry
	pluginWatcher      *setupplugin.Watcher
	pluginDevelopment  *pluginDevelopmentManager
	auth               *access.Manager
	ai                 *ai.Manager
	agentSessions      *agent.SessionStore
	agentTasks         *agent.TaskManager
	taskService        *agent.TaskService
	agentTaskStore     *agent.TaskStore
	goalStore          *agent.GoalStore
	goalSchedulerStop  chan struct{}
	goalSchedulerMu    sync.Mutex
	goalRunning        map[string]bool
	opsStore           agent.OpsRepository
	chatHistory        *chatHistoryStore
	testoneRecords     *testoneRecordStore
	opsProjects        *agent.OpsProjectStore
	opsBackground      bool
	pm2Guard           *GuardedPM2Executor
	opsOrchestrator    *agent.OpsOrchestrator
	alerts             agent.AlertManager
	alertWorker        *agent.AlertDeliveryWorker
	opsPaused          bool
	opsMonitor         *agent.OpsMonitor
	agentConfirms      *agentConfirmManager
	mu                 sync.RWMutex
	operations         []operationTask
	development        map[string]developmentProcess
	botAppPageRuntimes map[string]*botAppPageRuntime
	stopping           map[string]bool
	consoleCache       map[string]consoleSnapshot
	outputBuffers      map[string]*operationOutputBuffer
	// pm2Status lets tests substitute a fake PM2 state. The default read runs a
	// real `npx pm2 jlist` behind a short timeout so a missing pm2 never blocks
	// a local start request for the full package-manager timeout.
	pm2Status             func(string) (robot.PM2Status, error)
	requestID             atomic.Uint64
	nodeID                string
	directoryRoots        []string
	events                *robotEventHub
	eventGateway          *eventGateway
	operationEvents       *OperationEventStore
	opsEvents             *opsEventHub
	mcpEvents             *mcpEventHub
	mcpMonitorStop        chan struct{}
	pluginEventsStop      chan struct{}
	updateMonitorStop     chan struct{}
	updateMu              sync.Mutex
	updateStateMu         sync.RWMutex
	updateState           updateStatusState
	requestUpdateShutdown func()
	robotProjectsMu       sync.Mutex
	robotProjectsCache    robotProjectsSnapshot
	pluginStatusMu        sync.Mutex
	pluginStatusCache     map[string]*pluginStatusSnapshot
	hostContextMu         sync.RWMutex
	hostContexts          map[string]pluginHostContext
	privilegeStore        *privilegeStore
	pluginDownloadBroker  *pluginDownloadBroker
	sudoAttemptMu         sync.Mutex
	sudoAttempts          map[string]sudoAttempt
	runPrivilegedCommand  func(context.Context, []byte, string, []string) (string, error)
	installEnvironment    func(context.Context, string) (string, error)
	redisManager          *redis.Manager
	liveUploadsMu         sync.Mutex
	liveUploads           map[string]liveUpload
}

type pluginStatusSnapshot struct {
	output    string
	err       error
	expiresAt time.Time
	done      chan struct{}
}

type sudoAttempt struct {
	Failures    int
	LockedUntil time.Time
}

type setupPluginActionRequest struct {
	Action          string            `json:"action"`
	Confirm         bool              `json:"confirm"`
	Params          map[string]string `json:"params"`
	SudoPassword    *string           `json:"sudoPassword"`
	AuthorizationID string            `json:"authorizationId"`
}

type privilegePreflightRequest struct {
	PluginID string `json:"pluginId"`
	Action   string `json:"action"`
	PlanID   string `json:"planId,omitempty"`
}

// systemPickerRequest contains identifiers only. The Web Finder's type, title
// and multi-select policy come from the installed plugin manifest.
type systemPickerRequest struct {
	PluginID string `json:"pluginId"`
	PickerID string `json:"pickerId"`
}

type privilegePreflightResponse struct {
	Available     bool   `json:"available"`
	Authorization string `json:"authorization"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Reason        string `json:"reason,omitempty"`
	IntentID      string `json:"intentId,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
	SourceType    string `json:"sourceType,omitempty"`
	SourcePath    string `json:"sourcePath,omitempty"`
}

type updateStatusState struct {
	Update    releases.Update `json:"update"`
	CheckedAt time.Time       `json:"checkedAt"`
	Error     string          `json:"error,omitempty"`
}

type updateResponse struct {
	releases.Update
	Transaction *system.UpdateTransaction `json:"transaction,omitempty"`
	CheckedAt   time.Time                 `json:"checkedAt"`
	Error       string                    `json:"error,omitempty"`
}

type robotProjectItem struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

type robotProjectsSnapshot struct {
	rootsKey string
	updated  time.Time
	items    []robotProjectItem
}

func hostname() string {
	if value, err := os.Hostname(); err == nil && strings.TrimSpace(value) != "" {
		return value
	}
	return "alx"
}

// ServerRuntime owns the HTTP handler and all process-local background loops.
// Existing callers can keep using NewServer; production entrypoints should use
// Runtime.Shutdown so monitors and task state are flushed before exit.
type ServerRuntime struct {
	Handler         http.Handler
	server          *server
	once            sync.Once
	updateOnce      sync.Once
	updateRequested chan struct{}
}

// SetPluginDownloadBrokerEndpoint supplies the loopback address that plugin
// runners can use to request host-managed official downloads. It is set by the
// executable after it has chosen its listening port, never by a browser.
func (r *ServerRuntime) SetPluginDownloadBrokerEndpoint(endpoint string) {
	if r == nil || r.server == nil || r.server.pluginDownloadBroker == nil {
		return
	}
	r.server.pluginDownloadBroker.setEndpoint(endpoint)
}

// PluginDownloadBrokerHandler is intentionally narrower than the main HTTP
// handler. It is served only from a private loopback listener for runners, so
// a non-loopback workbench deployment never has to weaken Broker validation.
func (r *ServerRuntime) PluginDownloadBrokerHandler() http.Handler {
	if r == nil || r.server == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(r.server.pluginDownloadBrokerHandler)
}

// RequestUpdateShutdown lets a completed update use the same graceful path as
// SIGTERM instead of terminating the process from an HTTP handler.
func (r *ServerRuntime) RequestUpdateShutdown() {
	if r == nil || r.updateRequested == nil {
		return
	}
	r.updateOnce.Do(func() { close(r.updateRequested) })
}

func (r *ServerRuntime) UpdateRequested() <-chan struct{} {
	if r == nil || r.updateRequested == nil {
		return nil
	}
	return r.updateRequested
}

func (r *ServerRuntime) Shutdown(ctx context.Context) error {
	if r == nil || r.server == nil {
		return nil
	}
	var shutdownErr error
	r.once.Do(func() {
		s := r.server
		if s.opsMonitor != nil {
			_ = s.opsMonitor.Stop()
		}
		if s.alertWorker != nil {
			_ = s.alertWorker.Stop()
		}
		s.stopGoalScheduler()
		if s.pluginWatcher != nil {
			s.pluginWatcher.Stop()
		}
		if s.pluginDevelopment != nil {
			s.pluginDevelopment.close()
		}
		if s.mcpMonitorStop != nil {
			select {
			case <-s.mcpMonitorStop:
			default:
				close(s.mcpMonitorStop)
			}
		}
		if s.pluginEventsStop != nil {
			select {
			case <-s.pluginEventsStop:
			default:
				close(s.pluginEventsStop)
			}
		}
		if s.updateMonitorStop != nil {
			select {
			case <-s.updateMonitorStop:
			default:
				close(s.updateMonitorStop)
			}
		}
		if err := s.agentTasks.PauseRunning(); err != nil {
			shutdownErr = err
		}
		if s.opsStore != nil {
			if err := s.opsStore.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if s.chatHistory != nil {
			if err := s.chatHistory.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if s.testoneRecords != nil {
			if err := s.testoneRecords.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		if s.operationEvents != nil {
			if err := s.operationEvents.Close(); err != nil && shutdownErr == nil {
				shutdownErr = err
			}
		}
		s.privilegeStore.close()
		if s.redisManager != nil {
			s.redisManager.Close()
		}
		select {
		case <-ctx.Done():
			if shutdownErr == nil {
				shutdownErr = ctx.Err()
			}
		default:
		}
	})
	return shutdownErr
}

// cleanupStaleProcesses runs at server startup. It loads the persisted markers
// for previously supervised processes and kills any that survived an alx
// restart, so an orphaned node cannot hold the app port forever.
func cleanupStaleProcesses() {
	for _, item := range loadPersistedProcesses() {
		if item.PGID > 0 && processGroupAlive(item.PGID) {
			killProcessGroup(item.PGID)
		}
	}
	savePersistedProcesses(nil)
}

// pm2StatusResult carries the outcome of a bounded PM2 status read.
type pm2StatusResult struct {
	status robot.PM2Status
	err    error
}

// consoleSnapshot is a short-lived copy of the terminal's static context
// (pwd, scripts, git status, node version). It barely changes and is far more
// expensive to produce than the in-memory process output, so the terminal
// polling loop reuses it instead of spawning git/node on every tick.
type consoleSnapshot struct {
	output string
	at     time.Time
}

var goals = []goal{
	{ID: "install", Title: "安装机器人", Description: "用推荐默认配置，快速安装一个可以运行的 AlemonJS 机器人。", Steps: []string{"环境检查", "机器人名称与位置", "确认安装"}},
	{ID: "manage", Title: "管理机器人", Description: "管理已有机器人项目的配置、依赖与运行方式。", Steps: []string{"打开机器人管理"}},
	{ID: "develop", Title: "开发机器人", Description: "创建一个可按需配置的 AlemonJS 开发项目。", Steps: []string{"环境检查", "项目名称", "开发语言", "代码规范", "版本管理", "本地运行", "包管理器", "开发能力包", "图片开发", "样式方案", "开发技能", "确认创建"}},
	{ID: "desktop", Title: "安装桌面版", Description: "下载 AlemonDesk。", Steps: []string{"选择下载镜像", "下载桌面版"}, Mirrors: githubMirrors("alemondesk")},
	{ID: "mobile", Title: "安装手机版", Description: "下载 AlemonApp Android 安装包。", Steps: []string{"下载 Android 安装包"}, DownloadURL: "https://download.alemonjs.com/application/alemonapp/app-universal-release.apk"},
	{ID: "web", Title: "部署 Web 版", Description: "部署 alx。", Steps: []string{"选择部署方式", "环境检查", "快速启动"}, Mirrors: githubMirrors("alx")},
}

func githubMirrors(repository string) []mirror {
	url := "https://github.com/lemonade-lab/" + repository + "/releases/latest"
	result := make([]mirror, 0, len(systemnetwork.MirrorPresets(systemnetwork.RouteGitHub))+1)
	for _, preset := range systemnetwork.MirrorPresets(systemnetwork.RouteGitHub) {
		if rewritten, err := systemnetwork.RewriteTemplate(preset.Value, url); err == nil {
			result = append(result, mirror{Name: "GitHub 加速（" + preset.Label + "）", URL: rewritten})
		}
	}
	return append(result, mirror{Name: "GitHub 官方", URL: url})
}

func NewServer(version string, staticFiles fs.FS, templateFiles ...fs.FS) http.Handler {
	identity, err := access.New()
	if err != nil {
		panic(err)
	}
	// A bare handler has no lifecycle hook for stopping background workers.
	// Keep it suitable for embedding and tests; executable entrypoints must use
	// NewServerRuntime and call Shutdown during process termination.
	return newServerRuntimeWithAuth(version, staticFiles, identity, false, ServerOptions{}, templateFiles...).Handler
}

// ServerOptions carries process-level overrides that are applied once at
// startup, such as command-line Redis settings.
type ServerOptions struct {
	// RedisPort overrides the temporary Redis port when non-zero.
	RedisPort int
	// RedisDisabled forbids starting the temporary Redis.
	RedisDisabled bool
}

func NewServerRuntime(version string, staticFiles fs.FS, templateFiles ...fs.FS) *ServerRuntime {
	if _, err := access.New(); err != nil {
		panic(err)
	}
	return NewServerRuntimeWithOptions(version, staticFiles, ServerOptions{}, templateFiles...)
}

// NewServerRuntimeWithOptions starts a runtime with process-level overrides,
// keeping the variadic template files last for callers.
func NewServerRuntimeWithOptions(version string, staticFiles fs.FS, options ServerOptions, templateFiles ...fs.FS) *ServerRuntime {
	identity, err := access.New()
	if err != nil {
		panic(err)
	}
	return newServerRuntimeWithAuth(version, staticFiles, identity, true, options, templateFiles...)
}

// NewServerWithAuth permits tests and embedders to provide an isolated auth
// store instead of reading the current user's alx configuration.
func NewServerWithAuth(version string, staticFiles fs.FS, identity *access.Manager, templateFiles ...fs.FS) http.Handler {
	return newServerRuntimeWithAuth(version, staticFiles, identity, false, ServerOptions{}, templateFiles...).Handler
}

func NewServerRuntimeWithAuth(version string, staticFiles fs.FS, identity *access.Manager, templateFiles ...fs.FS) *ServerRuntime {
	return newServerRuntimeWithAuth(version, staticFiles, identity, true, ServerOptions{}, templateFiles...)
}

func newServerRuntimeWithAuth(version string, staticFiles fs.FS, identity *access.Manager, startBackground bool, options ServerOptions, templateFiles ...fs.FS) *ServerRuntime {
	// Rehydrate command paths on every service start so a managed Node remains
	// usable after restart without writing to the machine-wide PATH.
	system.RefreshCommandEnvironment("node", "npm", "npx", "git", "docker")
	assets, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		panic(err)
	}
	aiManager, err := ai.New()
	if err != nil {
		panic(err)
	}
	networkManager, networkErr := systemnetwork.New()
	if networkErr != nil {
		log.Printf("系统联网配置不可用，已使用临时默认配置：%v", networkErr)
		networkManager, _ = systemnetwork.NewAt("")
	}
	systemnetwork.SetDefault(networkManager)
	sessionStore, err := agent.NewSessionStore()
	if err != nil {
		panic(err)
	}
	taskStore, err := agent.NewTaskStore()
	if err != nil {
		panic(err)
	}
	goalStore := agent.NewGoalStoreAt(filepath.Join(filepath.Dir(taskStore.TasksDir()), "goals"))
	tasks := agent.NewTaskManager(taskStore)
	opsDir := filepath.Join(filepath.Dir(taskStore.TasksDir()), "incidents")
	var opsStore agent.OpsRepository = agent.NewOpsStoreAt(opsDir)
	var opsStartupErr error
	// SQLite is the default ops backend. ALX_OPS_STORAGE=json (or file) forces
	// the portable JSON store; existing JSON records are migrated automatically.
	if sqliteStore, sqliteErr := agent.OpenOpsRepository(opsDir, strings.TrimSpace(os.Getenv("ALX_OPS_SQLITE_PATH")), strings.TrimSpace(os.Getenv("ALX_OPS_STORAGE"))); sqliteErr == nil {
		opsStore = sqliteStore
	} else {
		opsStartupErr = sqliteErr
	}
	opsProjects := agent.NewOpsProjectStore(filepath.Join(filepath.Dir(taskStore.TasksDir()), "ops-projects.json"))
	// Existing policy files represent an explicit choice in older versions.
	// Listing a missing incident directory is read-only and does not create it.
	if policies, listErr := opsStore.ListPolicies(); listErr == nil {
		_ = opsProjects.MigratePolicies(policies)
	}
	chatHistory, chatHistoryErr := openChatHistoryStore(filepath.Join(filepath.Dir(taskStore.TasksDir()), "chat.db"))
	if chatHistoryErr != nil {
		log.Printf("聊天记录存储不可用：%v", chatHistoryErr)
	}
	testoneRecords, testoneRecordsErr := openTestoneRecordStore(filepath.Join(filepath.Dir(taskStore.TasksDir()), "testone.db"), filepath.Join(filepath.Dir(taskStore.TasksDir()), "testone-images"))
	if testoneRecordsErr != nil {
		log.Printf("测试中心记录存储不可用：%v", testoneRecordsErr)
	}
	operationEvents := newOperationEventStore(filepath.Join(filepath.Dir(taskStore.TasksDir()), "operations", "events.db"))
	privileges, privilegeErr := newPrivilegeStoreAt(filepath.Join(filepath.Dir(taskStore.TasksDir()), "privileges"))
	if privilegeErr != nil {
		log.Printf("权限审计存储不可用：%v", privilegeErr)
	}
	plugins := setupplugin.NewRegistry()
	downloadBroker := newPluginDownloadBroker(networkManager)
	downloadBroker.setRegistry(plugins)
	plugins.SetRunnerEnvironmentProvider(downloadBroker.environment)
	s := &server{version: version, assets: assets, static: http.FileServer(http.FS(assets)), checker: system.NewChecker(), network: networkManager, plugins: plugins, auth: identity, ai: aiManager, agentSessions: sessionStore, agentTaskStore: taskStore, agentTasks: tasks, taskService: &agent.TaskService{Manager: tasks}, goalStore: goalStore, opsStore: opsStore, chatHistory: chatHistory, testoneRecords: testoneRecords, opsProjects: opsProjects, opsBackground: startBackground, opsPaused: opsStartupErr != nil, goalSchedulerStop: make(chan struct{}), goalRunning: map[string]bool{}, agentConfirms: newAgentConfirmManager(), development: map[string]developmentProcess{}, botAppPageRuntimes: map[string]*botAppPageRuntime{}, stopping: map[string]bool{}, consoleCache: map[string]consoleSnapshot{}, outputBuffers: map[string]*operationOutputBuffer{}, directoryRoots: managedDirectoryRoots(), events: newRobotEventHub(), eventGateway: newEventGateway(), operationEvents: operationEvents, operations: operationEvents.snapshot(), opsEvents: newOpsEventHub(), mcpEvents: newMCPEventHub(), mcpMonitorStop: make(chan struct{}), pluginEventsStop: make(chan struct{}), updateMonitorStop: make(chan struct{}), updateState: updateStatusState{Update: releases.Update{Current: version}}, nodeID: fmt.Sprintf("%s-%d", hostname(), os.Getpid()), pluginStatusCache: map[string]*pluginStatusSnapshot{}, hostContexts: map[string]pluginHostContext{}, privilegeStore: privileges, pluginDownloadBroker: downloadBroker, sudoAttempts: map[string]sudoAttempt{}, runPrivilegedCommand: system.RunSudoCommand, installEnvironment: system.InstallEnvironment, liveUploads: map[string]liveUpload{}}
	s.redisManager = redis.NewManager(filepath.Join(filepath.Dir(taskStore.TasksDir()), "alx-redis.json"))
	if options.RedisPort > 0 || options.RedisDisabled {
		status := s.redisManager.Status()
		port := status.Port
		if options.RedisPort > 0 {
			port = options.RedisPort
		}
		if err := s.redisManager.Configure(port, status.AutoStart, options.RedisDisabled); err != nil {
			log.Printf("应用 Redis 启动参数失败：%v", err)
		}
	}
	s.pluginDevelopment = newPluginDevelopmentManager(plugins, filepath.Join(filepath.Dir(taskStore.TasksDir()), "setup-plugins", "development.json"))
	if opsStartupErr != nil {
		log.Printf("AI 运维 SQLite 初始化失败，已回退 JSON 并暂停自动维护: %v", opsStartupErr)
	}
	operationEvents.setRecoveredHandler(func() {
		_, _ = s.publishEvent("system", "system.store-recovered", map[string]any{"type": "system.store-recovered"}, nil)
	})
	if operationEvents.wasRecovered() {
		go func() {
			_, _ = s.publishEvent("system", "system.store-recovered", map[string]any{"type": "system.store-recovered"}, nil)
		}()
	}
	s.alerts.Record = func(alert agent.Alert) error {
		return opsStore.SaveAlert(agent.AlertRecord{Alert: alert, Status: "open", Updated: time.Now()})
	}
	s.alerts.OnDeliveryFailure = func(alert agent.Alert, deliveryErr error) {
		record, err := opsStore.GetAlert(alert.ID)
		if err != nil {
			record = agent.AlertRecord{Alert: alert}
		}
		record.Status = "delivery_failed"
		record.RetryCount++
		record.LastError = deliveryErr.Error()
		record.NextAttempt = time.Now().Add(5 * time.Minute)
		record.Updated = time.Now()
		_ = opsStore.SaveAlert(record)
	}
	s.alerts.RetryStore = opsStore
	s.alerts.Queue = opsStore
	s.alertWorker = &agent.AlertDeliveryWorker{Manager: &s.alerts, Lease: agent.NewLeaseManager(opsStore), LeaseKey: "alert-delivery", LeaseOwner: s.nodeID, LeaseTTL: 45 * time.Second, Interval: 5 * time.Second, OnLeaseLost: func(err error) {
		s.opsPaused = true
		_ = opsStore.AppendSignal(agent.OpsSignal{Kind: "alert_delivery", Status: "lease_lost", Message: err.Error(), Timestamp: time.Now()})
	}}
	pm2Guard := GuardedPM2Executor{Robots: s.robots, Leases: agent.NewLeaseManager(opsStore), Store: opsStore, Emergency: func() bool { return s.opsPaused }}
	s.pm2Guard = &pm2Guard
	s.opsOrchestrator = &agent.OpsOrchestrator{
		Store: opsStore,
		Policy: func(root string) (agent.OpsPolicy, error) {
			if !s.opsEnabled(root) {
				return agent.OpsPolicy{}, errors.New("该项目未启用高级运维")
			}
			return s.opsPolicy(root)
		},
		AI: func(incident agent.Incident, policy agent.OpsPolicy) (agent.AutoFixDecision, error) {
			providers, err := s.ai.List()
			if err != nil {
				return agent.AutoFixDecision{}, err
			}
			prompt := `你是线上运维审查器。只返回 JSON，不要 Markdown。动作只能是 observe_only、restart_process、auto_fix、create_todo、escalate。禁止凭据、数据库、系统权限和任意 shell 操作。若定位、验证或风险不明确，返回 create_todo。字段：action,confidence,severity,risk,reason,requiresHuman,allowedActions。\n项目：` + incident.ProjectRoot + `\n错误：` + incident.Sample + `\n文件：` + incident.File + `:` + fmt.Sprint(incident.Line) + `\n策略：` + policy.Mode
			for _, provider := range providers {
				if !provider.HasKey {
					continue
				}
				answer, chatErr := s.ai.Chat(provider.ID, provider.Model, []map[string]string{{"role": "user", "content": prompt}})
				if chatErr == nil {
					usage := (len(prompt) + len(answer)) / 4
					if _, budgetErr := opsStore.ConsumeBudget(incident.ProjectRoot, usage, 0, 0); budgetErr != nil {
						return agent.AutoFixDecision{}, budgetErr
					}
					return agent.ParseAutoFixDecision(answer)
				}
			}
			return agent.AutoFixDecision{}, errors.New("没有可用的 AI Provider")
		},
		PM2Guarded: func(root, action, owner string) (string, error) {
			if !s.opsEnabled(root) {
				return "", errors.New("该项目未启用高级运维")
			}
			return pm2Guard.Run(context.Background(), root, action, owner)
		},
		StartFix: func(incident agent.Incident, _ agent.AutoFixDecision) (string, error) {
			if !s.opsEnabled(incident.ProjectRoot) {
				return "", errors.New("该项目未启用高级运维")
			}
			if s.opsPaused {
				return "", errors.New("全局 AI 运维已暂停")
			}
			policy, policyErr := s.opsStore.GetPolicy(incident.ProjectRoot)
			if policyErr != nil || !policy.AllowCodeChanges {
				return "", errors.New("当前策略不允许自动代码修改")
			}
			if _, verifyErr := agent.ParsePolicyVerificationCommand(policy.VerificationCommand); verifyErr != nil {
				return "", errors.New("策略验证命令不可执行：" + verifyErr.Error())
			}
			providers, err := s.ai.List()
			if err != nil {
				return "", err
			}
			for _, provider := range providers {
				if !provider.HasKey {
					continue
				}
				created, createErr := s.createAgentTask(agentTaskInput{Provider: provider.ID, Model: provider.Model, Root: incident.ProjectRoot, Access: "auto", Messages: []map[string]string{{"role": "user", "content": "生产错误自动维护：" + incident.Sample + "\n请先定位根因，仅修改必要文件，并运行策略验证命令。"}}, Isolation: "workspace", AutoMaintenance: true, VerificationCommand: policy.VerificationCommand}, false)
				if createErr == nil {
					return created.ID, nil
				}
			}
			return "", errors.New("没有已配置的 AI Provider")
		},
	}
	if webhook := strings.TrimSpace(os.Getenv("ALX_OPS_WEBHOOK_URL")); webhook != "" {
		s.alerts.Sinks = []agent.AlertSink{agent.WebhookAlertSink{URL: webhook, Secret: os.Getenv("ALX_OPS_WEBHOOK_SECRET")}}
	}
	_ = s.agentTasks.ReconcileStartup()
	_ = s.goalStore.ReconcileRuns(s.agentTasks.List())
	if len(s.monitorableRoots()) > 0 {
		s.opsMonitor = s.newOpsMonitor()
		_ = s.opsStore.ReconcileMaintenance(s.agentTasks.List())
	}
	s.agentTasks.SetObserver(func(previous, current agent.AgentTask) {
		if previous.Status != current.Status && s.opsStore != nil {
			status := string(current.Status)
			if status == string(agent.TaskCompleted) || status == string(agent.TaskFailed) || status == string(agent.TaskCancelled) {
				// A missing maintenance record is a harmless no-op. This observer
				// never creates policy or incident files for ordinary agent tasks.
				_ = s.opsStore.TransitionMaintenanceForTask(current.ID, status, current.LastError)
				if status == string(agent.TaskCompleted) {
					if runs, runsErr := s.opsStore.ListMaintenance(); runsErr == nil {
						for _, run := range runs {
							if run.TaskID != current.ID {
								continue
							}
							if incident, incidentErr := s.opsStore.GetIncident(run.IncidentID); incidentErr == nil {
								if report, reportErr := s.agentTaskStore.LoadReport(current.ID); reportErr == nil {
									score := agent.MaintenanceScore{GoalSatisfied: report.Reviewer.GoalSatisfied, Safe: len(report.Reviewer.SecurityIssues) == 0, Verified: len(report.Validation) > 0, UnrelatedDiff: len(report.Reviewer.UnrelatedChanges) > 0}
									score.Score = 0
									for _, ok := range []bool{score.GoalSatisfied, score.Safe, score.Verified, !score.UnrelatedDiff} {
										if ok {
											score.Score += 0.25
										}
									}
									run.Score = score
									_ = s.opsStore.SaveScore(run.ID, score)
								}
								if policy, policyErr := s.opsStore.GetPolicy(incident.ProjectRoot); policyErr == nil && policy.MaxModifiedFiles > 0 {
									if report, reportErr := s.agentTaskStore.LoadReport(current.ID); reportErr == nil && len(report.ModifiedFiles) > policy.MaxModifiedFiles {
										store := agent.NewSnapshotStoreAt(filepath.Join(s.agentTaskStore.TasksDir(), current.ID, "snapshots"))
										_, _ = store.Rollback(current.ID, incident.ProjectRoot, false)
										run.Status, run.Error, run.RollbackPerformed = "failed", fmt.Sprintf("修改文件数超过策略上限 %d", policy.MaxModifiedFiles), true
										incident.Status, incident.DecisionReason = agent.IncidentTodo, run.Error
										_ = s.opsStore.SaveIncident(incident)
									}
								}
							}
							_ = s.opsStore.SaveMaintenance(run)
						}
					}
				}
				if status == string(agent.TaskFailed) || status == string(agent.TaskCancelled) {
					if runs, runsErr := s.opsStore.ListMaintenance(); runsErr == nil {
						for _, run := range runs {
							if run.TaskID != current.ID {
								continue
							}
							if incident, incidentErr := s.opsStore.GetIncident(run.IncidentID); incidentErr == nil {
								if policy, policyErr := s.opsStore.GetPolicy(incident.ProjectRoot); policyErr == nil {
									policy.FailureCount++
									if policy.FailureCircuitBreak > 0 && policy.FailureCount >= policy.FailureCircuitBreak {
										policy.Mode, policy.AutoAllowed = "strict", false
									}
									policy.Updated = time.Now()
									_ = s.opsStore.SavePolicy(policy)
								}
							}
						}
					}
				}
			}
		}
		if current.GoalID == "" || previous.Status == current.Status {
			return
		}
		status := string(current.Status)
		if status == string(agent.TaskCompleted) || status == string(agent.TaskFailed) || status == string(agent.TaskCancelled) {
			_ = s.goalStore.UpdateRunByTask(current.ID, status, current.LastError)
		}
	})
	if startBackground {
		// Register observers before starting background loops so startup
		// reconciliation and the first scheduled/monitor event cannot bypass
		// GoalRun or MaintenanceRun state synchronization.
		s.startGoalScheduler()
		if len(s.monitorableRoots()) > 0 && s.alertWorker != nil {
			if err := s.alertWorker.Start(context.Background()); err != nil {
				s.opsPaused = true
				_ = opsStore.AppendSignal(agent.OpsSignal{Kind: "alert_delivery", Status: "unavailable", Message: err.Error(), Timestamp: time.Now()})
				log.Printf("AI 运维告警 Worker 未启动，自动维护已暂停: %v", err)
			}
		}
		if len(s.monitorableRoots()) > 0 && s.opsMonitor != nil {
			_ = s.opsMonitor.Start(context.Background())
		}
		// Kill any previously supervised robot process that survived a restart so a
		// stray node cannot keep the app port occupied.
		cleanupStaleProcesses()
		// Every supervised process is owned by this server instance. A restart
		// therefore cannot resume an in-flight local or setup-plugin operation;
		// settle those persisted snapshots so they never block a new action.
		s.reconcileRecoveredOperations()
		// Poll plugin roots so adding/removing a plugin directory or editing a
		// manifest is reflected without a restart or manual refresh.
		// Filesystem notifications avoid a recurring one-second directory scan. A
		// 60-second fingerprint pass remains as a recovery path for missed events.
		if watcher, err := s.plugins.StartFSWatch(time.Minute); err == nil {
			s.pluginWatcher = watcher
		} else {
			s.pluginWatcher = s.plugins.StartWatch(time.Minute)
		}
		s.startPluginEventBridge()
		s.startMCPStatusMonitor()
		s.startUpdateStatusMonitor()
		if redisStatus := s.redisManager.Status(); redisStatus.AutoStart && !redisStatus.Disabled {
			go func() {
				if err := s.redisManager.Start(); err != nil {
					log.Printf("内置 Redis 自动启动失败：%v", err)
				}
			}()
		}
	}
	if len(templateFiles) > 0 {
		templates, err := fs.Sub(templateFiles[0], "templates")
		if err != nil {
			panic(err)
		}
		s.creator = project.NewCreator(templates)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/auth/status", s.authStatusHandler)
	mux.HandleFunc("/api/v1/auth/setup", s.authSetupHandler)
	mux.HandleFunc("/api/v1/auth/login", s.authLoginHandler)
	mux.HandleFunc("/api/v1/auth/logout", s.authLogoutHandler)
	mux.HandleFunc("/api/v1/auth/management", s.authManagementHandler)
	mux.HandleFunc("/api/v1/auth/accounts/", s.authAccountHandler)
	mux.HandleFunc("/api/v1/auth/accounts", s.authAccountsHandler)
	mux.HandleFunc("/api/v1/auth/roles/", s.authRoleHandler)
	mux.HandleFunc("/api/v1/auth/roles", s.authRolesHandler)
	mux.HandleFunc("/api/v1/goals", s.listGoals)
	mux.HandleFunc("/api/v1/checks", s.checksHandler)
	mux.HandleFunc("/api/v1/projects", s.projectsHandler)
	mux.HandleFunc("/api/v1/releases", s.releasesHandler)
	mux.HandleFunc("/api/v1/update", s.updateHandler)
	mux.HandleFunc("/api/v1/update/status", s.updateStatusHandler)
	mux.HandleFunc("/api/v1/update/download", s.downloadUpdateHandler)
	mux.HandleFunc("/api/v1/update/apply", s.applyUpdateHandler)
	mux.HandleFunc("/api/v1/update/load", s.loadUpdateHandler)
	mux.HandleFunc("/api/v1/ai/providers", s.aiProvidersHandler)
	mux.HandleFunc("/api/v1/ai/models", s.aiModelsHandler)
	mux.HandleFunc("/api/v1/ai/chat", s.aiChatHandler)
	mux.HandleFunc("/api/v1/agent/chat", s.agentChatHandler)
	mux.HandleFunc("/api/v1/agent/sessions", s.agentSessionsHandler)
	mux.HandleFunc("/api/v1/agent/sessions/", s.agentSessionHandler)
	mux.HandleFunc("/api/v1/agent/approve", s.agentConfirmHandler)
	mux.HandleFunc("/api/v1/agent/tasks", s.agentTasksHandler)
	mux.HandleFunc("/api/v1/agent/tasks/", s.agentTaskHandler)
	mux.HandleFunc("/api/v1/agent/diagnostics", s.agentDiagnosticsHandler)
	mux.HandleFunc("/api/v1/agent/goals", s.agentGoalsHandler)
	mux.HandleFunc("/api/v1/agent/goals/", s.agentGoalHandler)
	mux.HandleFunc("/api/v1/ops/", s.opsHandler)
	mux.HandleFunc("/api/v1/ops", s.opsHandler)
	mux.HandleFunc("/api/v1/ops/events", s.opsEventsHandler)
	mux.HandleFunc("/api/v1/events", s.eventsHandler)
	mux.HandleFunc("/api/v1/events/diagnostics", s.eventsHandler)
	mux.HandleFunc("/api/v1/system/ssh", s.sshHandler)
	mux.HandleFunc("/api/v1/system/service", s.systemServiceHandler)
	mux.HandleFunc("/api/v1/system/environment/install", s.environmentInstallHandler)
	mux.HandleFunc("/api/v1/system/network", s.systemNetworkHandler)
	mux.HandleFunc("/api/v1/system/redis", s.systemRedisHandler)
	mux.HandleFunc(pluginDownloadBrokerPath, s.pluginDownloadBrokerHandler)
	mux.HandleFunc("/api/v1/system/plugin-download-cache", s.pluginDownloadCacheHandler)
	mux.HandleFunc("/api/v1/system/mcp", s.systemMCPHandler)
	mux.HandleFunc("/api/v1/system/events", s.systemEventsHandler)
	mux.HandleFunc("/api/v1/system/picker", s.systemPickerHandler)
	mux.HandleFunc("/api/v1/system/capabilities", s.systemCapabilitiesHandler)
	mux.HandleFunc("/api/v1/system/capabilities/webview/open", s.systemCapabilityWebviewOpenHandler)
	mux.HandleFunc("/api/v1/system/capabilities/finder", s.systemCapabilityFinderHandler)
	mux.HandleFunc("/api/v1/system/capabilities/context", s.systemCapabilityContextHandler)
	mux.HandleFunc("/api/v1/system/capabilities/desktop/open", s.systemCapabilityDesktopOpenHandler)
	mux.HandleFunc("/api/v1/system/capabilities/clipboard", s.systemCapabilityClipboardHandler)
	mux.HandleFunc("/api/v1/system/capabilities/notification", s.systemCapabilityNotificationHandler)
	mux.HandleFunc("/api/v1/system/capabilities/info", s.systemCapabilityInfoHandler)
	mux.HandleFunc("/api/v1/system/capabilities/network/fetch", s.systemCapabilityNetworkFetchHandler)
	mux.HandleFunc("/api/v1/system/services/status", s.localServiceStatusHandler)
	mux.HandleFunc("/api/v1/system/context/robot", s.systemCurrentRobotHandler)
	mux.HandleFunc("/api/v1/system/privileged/status", s.privilegedStatusHandler)
	mux.HandleFunc("/api/v1/system/privileged/preflight", s.privilegedPreflightHandler)
	mux.HandleFunc("/api/v1/system/privileged/audit", s.privilegedAuditHandler)
	mux.HandleFunc("/api/v1/directories", s.directoryHandler)
	mux.HandleFunc("/api/v1/catalog", s.catalogHandler)
	mux.HandleFunc("/api/v1/catalog/versions", s.catalogVersionsHandler)
	mux.HandleFunc("/api/v1/catalog/document", s.catalogDocumentHandler)
	mux.HandleFunc("/api/v1/catalog/package-config", s.catalogPackageConfigHandler)
	mux.HandleFunc("/api/v1/setup/plugins", s.setupPluginsHandler)
	mux.HandleFunc("/api/v1/setup/plugins/market", s.setupPluginMarketHandler)
	mux.HandleFunc("/api/v1/setup/plugins/revision", s.setupPluginRevisionHandler)
	mux.HandleFunc("/api/v1/setup/plugins/events", s.setupPluginEventsHandler)
	mux.HandleFunc("/api/v1/setup/plugins/cache", s.setupPluginCacheHandler)
	mux.HandleFunc("/api/v1/setup/plugins/releases/", s.setupPluginReleasesHandler)
	mux.HandleFunc("/api/v1/setup/plugins/development", s.setupPluginDevelopmentHandler)
	mux.HandleFunc("/api/v1/setup/plugins/development/", s.setupPluginDevelopmentHandler)
	mux.HandleFunc("/api/v1/setup/plugins/upload", s.setupPluginUploadArchiveHandler)
	mux.HandleFunc("/api/v1/setup/plugins/", s.setupPluginActionHandler)
	mux.HandleFunc("/api/v1/setup/plugins/web/", s.setupPluginWebHandler)
	mux.HandleFunc("/api/v1/services", s.localServicesHandler)
	mux.HandleFunc("/api/v1/services/dynamic/", s.dynamicLocalServiceProxyHandler)
	mux.HandleFunc("/api/v1/services/", s.localServiceProxyHandler)
	mux.HandleFunc("/api/v1/robot", s.robotHandler)
	mux.HandleFunc("/api/v1/robot/projects", s.robotProjectsHandler)
	mux.HandleFunc("/api/v1/robot/validate", s.robotValidateHandler)
	mux.HandleFunc("/api/v1/robot/console", s.robotConsoleHandler)
	mux.HandleFunc("/api/v1/robot/terminal", s.robotTerminalHandler)
	mux.HandleFunc("/api/v1/robot/pm2-logs", s.robotPM2LogsHandler)
	mux.HandleFunc("/api/v1/robot/pm2-logs/days", s.robotPM2LogDaysHandler)
	mux.HandleFunc("/api/v1/robot/pm2-logs/stream", s.robotPM2LogStreamHandler)
	mux.HandleFunc("/api/v1/robot/pm2-logs/export", s.robotPM2LogExportHandler)
	mux.HandleFunc("/api/v1/robot/pm2-status", s.robotPM2StatusHandler)
	mux.HandleFunc("/api/v1/robot/pm2-processes", s.robotPM2ProcessesHandler)
	mux.HandleFunc("/api/v1/robot/app-port", s.robotAppPortHandler)
	mux.HandleFunc("/api/v1/robot/app/", s.robotAppHandler)
	mux.HandleFunc("/api/v1/robot/test-port", s.robotTestPortHandler)
	mux.HandleFunc("/api/v1/robot/ports", s.robotPortsHandler)
	mux.HandleFunc("/api/v1/robot/test/", s.robotTestHandler)
	mux.HandleFunc("/api/v1/robot/live/upload", s.robotLiveUploadHandler)
	mux.HandleFunc("/api/v1/robot/live/", s.robotLiveHandler)
	mux.HandleFunc("/api/v1/robot/runtime", s.robotRuntimeHandler)
	mux.HandleFunc("/api/v1/robot/runtime/preflight", s.robotRuntimePreflightHandler)
	mux.HandleFunc("/api/v1/robot/runtime/repair", s.robotRuntimeRepairHandler)
	mux.HandleFunc("/api/v1/robot/tasks", s.robotTasksHandler)
	mux.HandleFunc("/api/v1/robot/events", s.robotEventsHandler)
	mux.HandleFunc("/api/v1/robot/packages", s.robotPackagesHandler)
	mux.HandleFunc("/api/v1/robot/packages/upload", s.robotPackageUploadHandler)
	mux.HandleFunc("/api/v1/robot/packages/git-clone", s.robotPackageGitCloneHandler)
	mux.HandleFunc("/api/v1/robot/packages/git-clone/check", s.robotPackageGitCloneCheckHandler)
	mux.HandleFunc("/api/v1/robot/chat/history", s.robotChatHistoryHandler)
	mux.HandleFunc("/api/v1/robot/chat/summary", s.robotChatSummaryHandler)
	mux.HandleFunc("/api/v1/robot/testone/chat", s.robotTestoneChatHandler)
	mux.HandleFunc("/api/v1/robot/testone/chat/index", s.robotTestoneChatIndexHandler)
	mux.HandleFunc("/api/v1/robot/testone/image", s.robotTestoneImageHandler)
	mux.HandleFunc("/api/v1/robot/testone/summary", s.robotTestoneSummaryHandler)
	mux.HandleFunc("/api/v1/robot/package-versions", s.robotPackageVersionsHandler)
	mux.HandleFunc("/api/v1/robot/package-readme", s.robotPackageReadmeHandler)
	mux.HandleFunc("/api/v1/robot/webviews", s.botAppPagesHandler)
	mux.HandleFunc("/api/v1/robot/webview/", s.botAppPageHandler)
	mux.HandleFunc("/api/v1/robot/apps", s.robotAppsHandler)
	mux.HandleFunc("/api/v1/robot/package-config", s.robotPackageConfigHandler)
	mux.HandleFunc("/api/v1/robot/login", s.robotLoginHandler)
	mux.HandleFunc("/api/v1/robot/onebot-sync", s.robotOneBotSyncHandler)
	mux.HandleFunc("/api/v1/github/auth/status", s.githubAuthStatusHandler)
	mux.HandleFunc("/api/v1/github/auth/device", s.githubAuthDeviceHandler)
	mux.HandleFunc("/api/v1/github/auth/poll", s.githubAuthPollHandler)
	mux.HandleFunc("/api/v1/github/auth/client-id", s.githubAuthClientIDHandler)
	mux.HandleFunc("/api/v1/github/auth/token", s.githubAuthTokenHandler)
	mux.HandleFunc("/api/v1/github/auth/logout", s.githubAuthLogoutHandler)
	mux.HandleFunc("/api/v1/robot/manifest", s.robotManifestHandler)
	mux.HandleFunc("/api/v1/robot/git-init", s.robotGitInitHandler)
	mux.HandleFunc("/api/v1/robot/git", s.robotGitHandler)
	mux.HandleFunc("/api/v1/robot/git/diff", s.robotGitDiffHandler)
	mux.HandleFunc("/api/v1/robot/git-clone", s.robotGitCloneHandler)
	mux.HandleFunc("/api/v1/robot/git-clone/check", s.robotGitCloneCheckHandler)
	mux.HandleFunc("/api/v1/robot/git-clone/branches", s.robotGitCloneBranchesHandler)
	mux.HandleFunc("/api/v1/publish/npm/status", s.npmPublishStatusHandler)
	mux.HandleFunc("/api/v1/publish/npm/pack", s.npmPackPreviewHandler)
	mux.HandleFunc("/api/v1/publish/git/status", s.gitPublishStatusHandler)
	mux.HandleFunc("/api/v1/publish/git/refresh-branches", s.gitPublishRefreshBranchesHandler)
	mux.HandleFunc("/api/v1/publish/git/prepare", s.gitBuildPrepareHandler)
	mux.HandleFunc("/api/v1/publish/git/publish", s.gitBuildPublishHandler)
	mux.HandleFunc("/api/v1/publish/git/retry-tag", s.gitBuildRetryTagHandler)
	mux.Handle("/", s.spa())
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), s.ginHeaders(), s.ginAccess(), s.ginRequestLog())
	// The Gin engine owns every request. Existing handlers remain standard
	// net/http functions, preserving their API contracts during migration.
	router.Any("/*path", gin.WrapH(mux))
	runtime := &ServerRuntime{Handler: router, server: s, updateRequested: make(chan struct{})}
	s.requestUpdateShutdown = runtime.RequestUpdateShutdown
	return runtime
}

const authCookieName = "alx_session"

func (s *server) authToken(r *http.Request) string {
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *server) authStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) authSetupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Account      string `json:"account"`
		Password     string `json:"password"`
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请填写账户、密码和确认密码。")
		return
	}
	token, err := s.auth.Enable(input.Account, input.Password, input.Confirmation)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Operations roles are created only after a project explicitly opts in to
	// advanced operations. Authentication itself must not create incident data.
	s.setAuthCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"enabled": true, "account": strings.TrimSpace(input.Account)})
}

func (s *server) authLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Account  string `json:"account"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请填写账户和密码。")
		return
	}
	token, err := s.auth.Login(input.Account, input.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.setAuthCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "account": strings.TrimSpace(input.Account)})
}

func (s *server) authLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: authCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

// authManagementHandler returns the complete account-management view. It is
// intentionally separate from /status so ordinary users never receive other
// accounts or the editable permission catalogue.
func (s *server) authManagementHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireSuperAdmin(w, r) {
		return
	}
	accounts, err := s.auth.ListAccounts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	roles, err := s.auth.ListRoles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": status, "accounts": accounts, "roles": roles, "permissions": access.Permissions})
}

func (s *server) authAccountsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireSuperAdmin(w, r) {
		return
	}
	var input struct {
		Account      string   `json:"account"`
		Password     string   `json:"password"`
		Confirmation string   `json:"confirmation"`
		Roles        []string `json:"roles"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input) != nil {
		writeError(w, http.StatusBadRequest, "账户配置无效。")
		return
	}
	created, err := s.auth.CreateAccount(input.Account, input.Password, input.Confirmation, input.Roles)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *server) authAccountHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireSuperAdmin(w, r) {
		return
	}
	account, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/auth/accounts/"))
	if err != nil || strings.TrimSpace(account) == "" || strings.Contains(account, "/") {
		writeError(w, http.StatusBadRequest, "账户标识无效。")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var input struct {
			Roles        []string `json:"roles"`
			Password     *string  `json:"password"`
			Confirmation *string  `json:"confirmation"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input) != nil {
			writeError(w, http.StatusBadRequest, "账户配置无效。")
			return
		}
		updated, updateErr := s.auth.UpdateAccount(account, input.Roles, input.Password, input.Confirmation)
		if updateErr != nil {
			writeError(w, http.StatusBadRequest, updateErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.auth.DeleteAccount(account); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) authRolesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireSuperAdmin(w, r) {
		return
	}
	var role access.Role
	if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&role) != nil {
		writeError(w, http.StatusBadRequest, "角色配置无效。")
		return
	}
	created, err := s.auth.CreateRole(role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *server) authRoleHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireSuperAdmin(w, r) {
		return
	}
	id, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/v1/auth/roles/"))
	if err != nil || strings.TrimSpace(id) == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "角色标识无效。")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var role access.Role
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&role) != nil || role.ID != id {
			writeError(w, http.StatusBadRequest, "角色配置无效。")
			return
		}
		updated, updateErr := s.auth.SaveRole(role)
		if updateErr != nil {
			writeError(w, http.StatusBadRequest, updateErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := s.auth.DeleteRole(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: authCookieName, Value: token, Path: "/", MaxAge: int((12 * time.Hour).Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (s *server) requireSuperAdmin(w http.ResponseWriter, r *http.Request) bool {
	status, err := s.auth.Status(s.authToken(r))
	if err != nil || !status.Authenticated || !status.SuperAdmin {
		writeError(w, http.StatusForbidden, "仅超级管理员可以管理账户与角色。")
		return false
	}
	return true
}

func (s *server) npmPublishStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	status, err := s.robots.NPMStatus(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) npmPackPreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root, commit := r.URL.Query().Get("root"), r.URL.Query().Get("commit")
	var preview robot.NPMPackPreview
	var err error
	if commit != "" {
		preview, err = s.robots.NPMPackPreviewAtCommit(root, commit)
	} else {
		preview, err = s.robots.NPMPackPreview(root)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *server) gitPublishStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	status, err := robot.GitReleaseStatus(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) gitPublishRefreshBranchesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	status, err := robot.RefreshGitSourceBranches(input.Root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) gitBuildPrepareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root   string `json:"root"`
		Branch string `json:"branch"`
		Commit string `json:"commit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	session, err := robot.PrepareGitBuild(input.Root, input.Branch, input.Commit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *server) gitBuildPublishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		SessionID string   `json:"sessionId"`
		Version   string   `json:"version"`
		Artifacts []string `json:"artifacts"`
		Confirm   bool     `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := robot.PublishPreparedGitBuild(input.SessionID, input.Version, input.Artifacts, input.Confirm)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) gitBuildRetryTagHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := robot.RetryPreparedGitTag(input.SessionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) catalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	groups, err := catalog.Fetch(r.URL.Query().Get("kind"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *server) catalogVersionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	versions, err := catalog.LoadPackageVersions(r.URL.Query().Get("package"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *server) catalogDocumentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	document, err := catalog.LoadDocument(r.URL.Query().Get("url"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (s *server) catalogPackageConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	config, err := catalog.LoadPackageConfig(r.URL.Query().Get("url"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// setupPluginsHandler returns locally downloaded plugins, including disabled
// entries so the manager can start or remove them.
func (s *server) setupPluginsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	writeJSON(w, http.StatusOK, s.plugins.All())
}

// setupPluginMarketHandler returns the curated online catalogue independently
// from local installations. This keeps the market and "我的" views distinct.
func (s *server) setupPluginMarketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	writeJSON(w, http.StatusOK, s.plugins.Market())
}

// setupPluginRevisionHandler exposes the plugin registry revision so the UI can
// cheaply detect hot-plugged plugin changes without re-fetching the whole list.
func (s *server) setupPluginRevisionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": s.plugins.Revision()})
}

func (s *server) setupPluginReleasesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/setup/plugins/releases/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "插件标识无效。")
		return
	}
	items, err := s.plugins.Releases(id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) setupPluginCacheHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		summary, err := s.plugins.CacheSummary()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case http.MethodPost:
		summary, err := s.plugins.CleanupCache()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

// setupPluginEventsHandler streams a "plugins changed" event over SSE whenever
// the plugin registry revision bumps, so the UI can refresh the plugin list
// without polling it. The event carries no payload; the client refetches
// /setup/plugins on receipt.
func (s *server) setupPluginEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不受支持。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	sub := s.plugins.Subscribe()
	defer s.plugins.Unsubscribe(sub)
	// Heartbeat keeps proxies from closing the idle connection.
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-sub:
			if _, err := w.Write([]byte("data: {}\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) startPluginEventBridge() {
	sub := s.plugins.Subscribe()
	go func() {
		defer s.plugins.Unsubscribe(sub)
		for {
			select {
			case <-s.pluginEventsStop:
				return
			case <-sub:
				_, _ = s.publishEvent("plugins", "plugins.changed", map[string]any{"revision": s.plugins.Revision()}, nil)
			}
		}
	}()
}

func (s *server) setupPluginActionHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/setup/plugins/"), "/")
	if len(parts) == 2 && parts[1] == "upload" && r.Method == http.MethodPost {
		s.setupPluginUploadHandler(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodGet {
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		plugin, findErr := s.plugins.Find(parts[0])
		if findErr != nil || !plugin.AllowsStatusAction(action) || len(r.URL.Query()) != 1 {
			writeError(w, http.StatusBadRequest, "仅支持无参数的插件状态动作。")
			return
		}
		output, err := s.pluginStatus(parts[0], action)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !json.Valid([]byte(output)) {
			writeError(w, http.StatusBadGateway, "插件状态返回无效 JSON。")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(output))
		return
	}
	// A media resource is declared by the plugin manifest. The runner returns
	// typed bytes as base64 while the host supplies authenticated same-origin
	// transport and response validation. No product-specific media route is
	// needed for QR codes, screenshots, previews, or future plugin UI assets.
	if len(parts) == 3 && parts[1] == "media" && r.Method == http.MethodGet {
		plugin, findErr := s.plugins.Find(parts[0])
		media, declared := plugin.MediaResource(parts[2])
		if findErr != nil || !declared {
			writeError(w, http.StatusNotFound, "未找到插件媒体资源。")
			return
		}
		output, err := s.plugins.Run(parts[0], media.Action, nil, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var payload struct {
			Available bool   `json:"available"`
			Data      string `json:"data"`
		}
		if err := json.Unmarshal([]byte(output), &payload); err != nil || !payload.Available || payload.Data == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		image, err := base64.StdEncoding.DecodeString(payload.Data)
		if err != nil || len(image) < 8 || len(image) > 2<<20 || (media.ContentType == "image/png" && !bytes.Equal(image[:8], []byte{'\x89', 'P', 'N', 'G', '\r', '\n', '\x1a', '\n'})) {
			writeError(w, http.StatusBadGateway, "插件返回的媒体资源无效。")
			return
		}
		w.Header().Set("Content-Type", media.ContentType)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(image)
		return
	}
	if len(parts) == 2 && parts[1] == "versions" && r.Method == http.MethodGet {
		versions, err := s.plugins.Versions(parts[0])
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, versions)
		return
	}
	if len(parts) == 3 && parts[1] == "versions" && r.Method == http.MethodDelete {
		tag, err := url.PathUnescape(parts[2])
		if err != nil || tag == "" {
			writeError(w, http.StatusBadRequest, "插件版本无效。")
			return
		}
		if err := s.plugins.DeleteVersion(parts[0], tag); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": parts[0], "tag": tag, "deleted": true})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if len(parts) != 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "未找到 Setup 插件操作。")
		return
	}
	if parts[1] == "enabled" {
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请选择插件状态。")
			return
		}
		if err := s.plugins.SetEnabled(parts[0], input.Enabled); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": parts[0], "enabled": input.Enabled})
		return
	}
	if parts[1] == "install" {
		var input struct {
			Version   string `json:"version"`
			AssetName string `json:"assetName"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Version == "" || input.AssetName == "" {
			writeError(w, http.StatusBadRequest, "请选择插件版本和安装包。")
			return
		}
		installed, err := s.plugins.Install(parts[0], input.Version, input.AssetName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Downloading a system plugin only places the verified release on disk.
		// The user explicitly starts it from the plugin centre, which makes the
		// transition from downloaded to loaded visible and reversible.
		if err := s.plugins.SetEnabled(installed.ID, false); err != nil {
			writeError(w, http.StatusBadRequest, "插件已下载，但无法进入待启动状态："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": installed.ID, "downloaded": true, "enabled": false})
		return
	}
	if parts[1] == "uninstall" {
		var input struct {
			Confirm bool `json:"confirm"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || !input.Confirm {
			writeError(w, http.StatusBadRequest, "请确认卸载系统插件。")
			return
		}
		if err := s.plugins.Uninstall(parts[0]); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": parts[0], "uninstalled": true})
		return
	}
	if parts[1] == "switch" {
		var input struct {
			Version   string `json:"version"`
			AssetName string `json:"assetName"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Version == "" || input.AssetName == "" {
			writeError(w, http.StatusBadRequest, "请选择插件版本和安装包。")
			return
		}
		installed, err := s.plugins.Install(parts[0], input.Version, input.AssetName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": installed.ID, "tag": installed.InstalledTag, "switched": true})
		return
	}
	if parts[1] != "actions" {
		writeError(w, http.StatusNotFound, "未找到 Setup 插件操作。")
		return
	}
	var input setupPluginActionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Action == "" {
		writeError(w, http.StatusBadRequest, "请选择要执行的插件操作。")
		return
	}
	if s.isManifestPasswordOperation(parts[0], input.Action) {
		s.handleManifestSudoAction(w, r, parts[0], input)
		return
	}
	if s.isManifestNativeOperation(parts[0], input.Action) {
		s.handleManifestNativeAction(w, r, parts[0], input)
		return
	}
	if input.SudoPassword != nil {
		writeError(w, http.StatusBadRequest, "此操作不接受系统管理员密码。")
		return
	}
	// A plugin runner is a separate process for every action. Starting the same
	// lifecycle action twice would therefore race over its installation and
	// process-state files. Reuse the existing task for an identical request;
	// reject a conflicting action until that task reaches a terminal state.
	operationName := "setup:" + parts[0] + ":" + input.Action
	s.mu.Lock()
	if active := activeSetupPluginOperation(s.operations, parts[0]); active != nil {
		s.mu.Unlock()
		if active.Action == operationName {
			writeJSON(w, http.StatusAccepted, active)
			return
		}
		writeError(w, http.StatusConflict, "该插件正在执行“"+strings.TrimPrefix(active.Action, "setup:"+parts[0]+":")+"”，请等待当前操作完成。")
		return
	}
	created := operationTask{ID: "setup-" + time.Now().Format("20060102150405.000000000"), Root: "", Action: "setup:" + parts[0] + ":" + input.Action, Status: "running", CreatedAt: time.Now()}
	s.operations = append([]operationTask{created}, s.operations...)
	if len(s.operations) > 40 {
		s.operations = s.operations[:40]
	}
	s.mu.Unlock()
	go func() {
		result, err := s.plugins.RunResultWithProgress(parts[0], input.Action, input.Params, input.Confirm, func(event setupplugin.Progress) {
			s.updateOperation(created.ID, event.Percent, event.Message, "", false)
		})
		if err == nil {
			plugin, findErr := s.plugins.Find(parts[0])
			operation, planned := setupplugin.PrivilegedOperationSpec{}, false
			if findErr == nil {
				for _, candidate := range plugin.PrivilegedOperations {
					if candidate.PlanAction == input.Action {
						operation, planned = candidate, true
						break
					}
				}
			}
			if planned {
				if s.privilegeStore == nil {
					s.updateOperationData(created.ID, 100, result.Output, "权限审计存储不可用", nil, true)
					return
				}
				status, _ := s.auth.Status(s.authToken(r))
				stored, storeErr := s.privilegeStore.savePlan(parts[0], operation.Action, result.Data, status.Account)
				if storeErr != nil {
					s.updateOperationData(created.ID, 100, result.Output, storeErr.Error(), nil, true)
					return
				}
				result.Data = stored
			}
		}
		if err != nil {
			s.updateOperationData(created.ID, 100, result.Output, err.Error(), result.Data, true)
			return
		}
		s.updateOperationData(created.ID, 100, result.Output, "", result.Data, true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

// setupPluginUploadHandler streams browser-selected files to a host-owned
// temporary directory, then invokes only an upload action declared in the
// installed plugin's manifest. Browser bytes never become arbitrary runner
// parameters and temporary files are removed after the runner completes.
func (s *server) setupPluginUploadHandler(w http.ResponseWriter, r *http.Request, pluginID string) {
	plugin, err := s.plugins.Find(pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusBadRequest, "系统插件未安装或已停用。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传格式无效。")
		return
	}
	defer r.MultipartForm.RemoveAll()
	action := strings.TrimSpace(r.FormValue("action"))
	if action == "" && len(plugin.Uploads) == 1 {
		action = plugin.Uploads[0].Action
	}
	upload, allowed := plugin.UploadAction(action)
	if !allowed {
		writeError(w, http.StatusBadRequest, "该插件未声明此上传操作。")
		return
	}
	destination := strings.TrimSpace(r.FormValue("destination"))
	if !filepath.IsAbs(destination) {
		writeError(w, http.StatusBadRequest, "上传目标必须是绝对目录路径。")
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "请拖入至少一个文件。")
		return
	}
	s.mu.Lock()
	if active := activeSetupPluginOperation(s.operations, pluginID); active != nil {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "该插件正在执行“"+strings.TrimPrefix(active.Action, "setup:"+pluginID+":")+"”，请等待当前操作完成。")
		return
	}
	created := operationTask{ID: "setup-" + time.Now().Format("20060102150405.000000000"), Action: "setup:" + pluginID + ":" + action, Status: "running", CreatedAt: time.Now()}
	s.operations = append([]operationTask{created}, s.operations...)
	if len(s.operations) > 40 {
		s.operations = s.operations[:40]
	}
	s.mu.Unlock()

	staging, err := os.MkdirTemp("", "alx-plugin-upload-")
	if err != nil {
		s.updateOperationData(created.ID, 100, "", "无法创建上传临时目录："+err.Error(), nil, true)
		writeError(w, http.StatusInternalServerError, "无法创建上传临时目录。")
		return
	}
	seen := map[string]bool{}
	var total int64
	for _, header := range files {
		name := filepath.Base(strings.TrimSpace(header.Filename))
		if name == "" || name == "." || len([]rune(name)) > 180 || seen[name] {
			_ = os.RemoveAll(staging)
			s.updateOperationData(created.ID, 100, "", "上传文件名无效或重复", nil, true)
			writeError(w, http.StatusBadRequest, "上传文件名无效或重复。")
			return
		}
		seen[name] = true
		input, openErr := header.Open()
		if openErr != nil {
			_ = os.RemoveAll(staging)
			s.updateOperationData(created.ID, 100, "", openErr.Error(), nil, true)
			writeError(w, http.StatusBadRequest, "无法读取上传文件。")
			return
		}
		output, createErr := os.OpenFile(filepath.Join(staging, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			_ = input.Close()
			_ = os.RemoveAll(staging)
			s.updateOperationData(created.ID, 100, "", createErr.Error(), nil, true)
			writeError(w, http.StatusInternalServerError, "无法暂存上传文件。")
			return
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, upload.MaxBytes-total+1))
		closeErr := output.Close()
		_ = input.Close()
		total += n
		if copyErr != nil || closeErr != nil || total > upload.MaxBytes {
			_ = os.RemoveAll(staging)
			s.updateOperationData(created.ID, 100, "", "文件超过上传限制或暂存失败", nil, true)
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("上传总大小不能超过 %d MiB。", upload.MaxBytes/(1024*1024)))
			return
		}
	}
	go func() {
		defer os.RemoveAll(staging)
		result, runErr := s.plugins.RunResultWithProgress(pluginID, action, map[string]string{"stagingDir": staging, "destination": destination}, false, func(event setupplugin.Progress) {
			s.updateOperation(created.ID, event.Percent, event.Message, "", false)
		})
		if runErr != nil {
			s.updateOperationData(created.ID, 100, result.Output, runErr.Error(), result.Data, true)
			return
		}
		s.updateOperationData(created.ID, 100, result.Output, "", result.Data, true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

// setupPluginUploadArchiveHandler installs a browser-uploaded system plugin
// archive into the host plugin root. The archive is validated and unpacked by
// the registry; the upload filename never becomes part of the install.
func (s *server) setupPluginUploadArchiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传格式无效。")
		return
	}
	defer r.MultipartForm.RemoveAll()
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "请上传一个插件安装包。")
		return
	}
	header := files[0]
	if !isUploadArchiveName(header.Filename) {
		writeError(w, http.StatusBadRequest, "仅支持 .zip、.tar.gz 或 .tgz 插件安装包。")
		return
	}
	temporary, err := os.CreateTemp("", "alx-plugin-upload-*.archive")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建上传临时文件。")
		return
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	input, err := header.Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法读取上传文件。")
		return
	}
	_, copyErr := io.Copy(temporary, io.LimitReader(input, 2<<30+1))
	closeErr := temporary.Close()
	_ = input.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "插件安装包过大。")
		return
	}
	installed, err := s.plugins.InstallUpload(temporary.Name())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": installed.ID, "name": installed.Name, "version": installed.Version, "enabled": installed.Enabled})
}

func isUploadArchiveName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
}

// activeSetupPluginOperation returns the one in-flight task for a plugin.
// Keeping this at the host boundary covers every browser tab and every plugin
// 面板页, rather than relying only on a disabled UI button.
func activeSetupPluginOperation(operations []operationTask, pluginID string) *operationTask {
	prefix := "setup:" + pluginID + ":"
	for index := range operations {
		item := &operations[index]
		if item.Status == "running" && strings.HasPrefix(item.Action, prefix) {
			copy := *item
			return &copy
		}
	}
	return nil
}

func (s *server) isManifestNativeOperation(pluginID, action string) bool {
	if s.plugins == nil {
		return false
	}
	plugin, err := s.plugins.Find(pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		return false
	}
	operation, ok := plugin.PrivilegedOperation(action)
	return ok && operation.Authorization == "native" && operationSupportsPlatform(operation, runtime.GOOS)
}

func (s *server) handleManifestNativeAction(w http.ResponseWriter, r *http.Request, pluginID string, input setupPluginActionRequest) {
	if !input.Confirm || !s.privilegedRequestAllowed(r) {
		writeError(w, http.StatusForbidden, "该系统操作当前未获授权；请检查工作台的系统权限模式。")
		return
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status.Enabled && (!status.Authenticated || !s.auth.Authorize(s.authToken(r), "system.manage")) {
		writeError(w, http.StatusForbidden, "当前账户没有系统变更权限。")
		return
	}
	plugin, findErr := s.plugins.Find(pluginID)
	if findErr != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusBadRequest, "系统插件未安装或已停用。")
		return
	}
	operation, ok := plugin.PrivilegedOperation(input.Action)
	if !ok || operation.Authorization != "native" || !operationSupportsPlatform(operation, runtime.GOOS) {
		writeError(w, http.StatusBadRequest, "该系统插件操作当前不可执行。")
		return
	}
	if input.SudoPassword != nil {
		writeError(w, http.StatusBadRequest, "当前操作使用系统原生授权窗口，不接受工作台密码。")
		return
	}
	var plan privilegedPlan
	planID := ""
	authorization := "native-uac"
	switch runtime.GOOS {
	case "darwin":
		authorization = "native"
	case "windows":
		authorization = "native-uac"
	case "linux":
		authorization = "polkit"
	default:
		writeError(w, http.StatusBadRequest, "当前平台没有可用的系统授权方式。")
		return
	}
	if operation.PlanAction != "" {
		if len(input.Params) != 1 {
			writeError(w, http.StatusBadRequest, "该系统操作仅接受宿主签发的计划 ID。")
			return
		}
		planID = input.Params["planID"]
	} else if !operation.UseLatestAudit {
		if len(input.Params) != 0 {
			writeError(w, http.StatusBadRequest, "该系统操作不接受额外参数。")
			return
		}
	}
	if operation.UseLatestAudit && len(input.Params) != 0 {
		writeError(w, http.StatusBadRequest, "该系统操作不接受额外参数。")
		return
	}
	intent, err := s.privilegeStore.validateIntent(input.AuthorizationID, pluginID, input.Action, planID, status.Account, privilegeRequestSource(r), authorization)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := s.privilegeStore.consumeIntent(intent); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if operation.PlanAction != "" {
		plan, err = s.privilegeStore.consumePlan(planID, pluginID, input.Action, status.Account)
	} else if operation.UseLatestAudit {
		plan, err = s.privilegeStore.latestAudit(pluginID)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	params := map[string]string{}
	if operation.PlanAction != "" || operation.UseLatestAudit {
		params["operation"] = plan.Operation
		for key, value := range plan.Params {
			params[key] = value
		}
		if plan.Fingerprint != "" {
			params["__alxFingerprint"] = plan.Fingerprint
		}
	}
	created := operationTask{ID: "setup-" + time.Now().Format("20060102150405.000000000"), Action: "setup:" + pluginID + ":" + input.Action, Status: "running", Output: "正在请求系统管理员授权…", CreatedAt: time.Now()}
	s.mu.Lock()
	s.operations = append([]operationTask{created}, s.operations...)
	if len(s.operations) > 40 {
		s.operations = s.operations[:40]
	}
	s.mu.Unlock()
	go func() {
		var result setupplugin.ActionResult
		var runErr error
		result, runErr = s.plugins.RunWithNativePrivileges(pluginID, input.Action, params, func(event setupplugin.Progress) {
			s.updateOperation(created.ID, event.Percent, event.Message, "", false)
		})
		if runErr != nil {
			s.updateOperationData(created.ID, 100, result.Output, runErr.Error(), result.Data, true)
			return
		}
		auditOperation, auditParams := plan.Operation, plan.Params
		if auditOperation == "" {
			auditOperation, auditParams = input.Action, map[string]string{}
		}
		if auditErr := s.privilegeStore.appendAudit(pluginID, input.Action, auditOperation, auditParams, result.Output, status.Account); auditErr != nil {
			s.updateOperationData(created.ID, 100, result.Output, auditErr.Error(), result.Data, true)
			return
		}
		s.updateOperationData(created.ID, 100, result.Output, "", result.Data, true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

// pluginStatus coalesces concurrent reads and keeps a one-second in-memory
// result. Status polling must never allocate an operation task or write task
// history; only mutating actions use the asynchronous task path.
func (s *server) pluginStatus(pluginID, action string) (string, error) {
	key := pluginID + "\x00" + action
	s.pluginStatusMu.Lock()
	if s.pluginStatusCache == nil {
		s.pluginStatusCache = map[string]*pluginStatusSnapshot{}
	}
	if snapshot := s.pluginStatusCache[key]; snapshot != nil {
		if snapshot.done != nil {
			done := snapshot.done
			s.pluginStatusMu.Unlock()
			<-done
			return snapshot.output, snapshot.err
		}
		if time.Now().Before(snapshot.expiresAt) {
			output, err := snapshot.output, snapshot.err
			s.pluginStatusMu.Unlock()
			return output, err
		}
	}
	snapshot := &pluginStatusSnapshot{done: make(chan struct{})}
	s.pluginStatusCache[key] = snapshot
	s.pluginStatusMu.Unlock()

	output, err := s.plugins.Run(pluginID, action, nil, false)

	s.pluginStatusMu.Lock()
	snapshot.output, snapshot.err = output, err
	snapshot.expiresAt = time.Now().Add(time.Second)
	close(snapshot.done)
	snapshot.done = nil
	if len(s.pluginStatusCache) > 32 {
		for staleKey, stale := range s.pluginStatusCache {
			if stale != snapshot && stale.done == nil && !time.Now().Before(stale.expiresAt) {
				delete(s.pluginStatusCache, staleKey)
			}
		}
	}
	s.pluginStatusMu.Unlock()
	return output, err
}

// setupPluginWebHandler serves a setup plugin's static web UI. It is served
// same-origin so the UI can call the plugin's own action API directly. Only
// installed, enabled plugins are served: an online-only plugin is read-only
// and must be installed locally before its web assets exist. Path traversal
// and symlink escapes are rejected before any file is read.
func (s *server) setupPluginWebHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	const prefix = "/api/v1/setup/plugins/web/"
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, prefix), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "未找到系统面板页。")
		return
	}
	plugin, err := s.plugins.Find(parts[0])
	if err != nil || plugin.Online || !plugin.Runnable || !plugin.Enabled {
		writeError(w, http.StatusNotFound, "未找到系统面板页。")
		return
	}
	root, err := plugin.WebRoot()
	if err != nil {
		writeError(w, http.StatusNotFound, "系统面板页不可用。")
		return
	}
	requestPath := ""
	if len(parts) == 2 {
		requestPath = parts[1]
	}
	requestPath = strings.TrimPrefix(filepath.ToSlash(requestPath), "/")
	if requestPath == "" {
		requestPath = "index.html"
	}
	if requestPath == "finder-bridge.js" || requestPath == "host-bridge.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = io.WriteString(w, setupPluginHostBridge())
		return
	}
	clean := filepath.Clean(requestPath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "插件 Web 资源路径无效。")
		return
	}
	candidate := filepath.Join(root, clean)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		// A no-extension path is treated as an SPA route served by index.html.
		if filepath.Ext(clean) == "" {
			resolved = filepath.Join(root, "index.html")
		} else {
			writeError(w, http.StatusNotFound, "插件 Web 资源不存在。")
			return
		}
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusNotFound, "插件 Web 资源路径无效。")
		return
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "插件 Web 资源不存在。")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Same-origin UI that may call the plugin action API. The shipped plugin
	// pages (e.g. alemonx-qq) use inline scripts, so script-src must allow
	// 'unsafe-inline'. frame-ancestors keeps it embeddable only from the
	// same management origin, including a deployed server address.
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob: https: http:; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'; base-uri 'none'")
	if filepath.Ext(resolved) == ".html" {
		content, readErr := os.ReadFile(resolved)
		if readErr != nil {
			writeError(w, http.StatusNotFound, "插件 Web 资源不存在。")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, rewriteSetupPluginWebHTML(string(content)))
		return
	}
	http.ServeFile(w, r, resolved)
}

// rewriteSetupPluginWebHTML injects the host capability bridge. It preserves
// the existing fetch contract while keeping desktop requests in the workbench.
func rewriteSetupPluginWebHTML(content string) string {
	const bridge = `<script src="host-bridge.js"></script>`
	injection := alemonjsThemeStyleTag + bridge
	if strings.Contains(content, "</head>") {
		return strings.Replace(content, "</head>", injection+"</head>", 1)
	}
	return injection + content
}

// injectSetupPluginFinderBridge applies the same bridge to an HTML document
// coming from a source-plugin development server. It intentionally skips
// compressed/non-HTML responses, which must remain byte-for-byte proxied.
func injectSetupPluginFinderBridge(response *http.Response) {
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		return
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	_ = response.Body.Close()
	rewritten := []byte(rewriteSetupPluginWebHTML(string(body)))
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
}

func setupPluginFinderBridge() string {
	return setupPluginHostBridge()
}

// setupPluginHostBridge is intentionally dependency-free, so every static or
// Vite-served system plugin receives the same small browser SDK. The server
// still performs plugin identity, authentication and availability checks.
func setupPluginHostBridge() string {
	return `(function(){var nativeFetch=window.fetch;var serial=0;var pending={};function finderRequest(input,init){var url;try{url=new URL(input instanceof Request?input.url:input,location.href)}catch(_){return null}if(url.origin!==location.origin||(url.pathname!=='/api/v1/system/capabilities/finder'&&url.pathname!=='/api/v1/system/picker'))return null;var body=init&&init.body;if(typeof body!=='string')return null;var payload;try{payload=JSON.parse(body)}catch(_){return null}if(!payload||typeof payload.pluginId!=='string'||typeof payload.pickerId!=='string')return null;return payload}function response(status,body){return new Response(JSON.stringify(body),{status:status,headers:{'Content-Type':'application/json'}})}function json(url,init){return nativeFetch(url,init).then(function(res){return res.json().then(function(data){if(!res.ok)throw new Error(data&&data.error||'宿主能力请求失败');return data})})}function bridge(type,payload,timeout){timeout=timeout||60000;var requestId=type+'-'+Date.now().toString(36)+'-'+(++serial).toString(36);return new Promise(function(resolve){pending[requestId]={type:type,resolve:resolve};parent.postMessage(Object.assign({source:'alx-setup-plugin',type:type,requestId:requestId},payload||{}),location.origin);window.setTimeout(function(){if(!pending[requestId])return;delete pending[requestId];resolve({ok:false,error:'等待宿主响应超时。'})},timeout)})}window.fetch=function(input,init){var payload=finderRequest(input,init);if(!payload)return nativeFetch.apply(this,arguments);var requestId='finder-'+Date.now().toString(36)+'-'+(++serial).toString(36);return new Promise(function(resolve){pending[requestId]={type:'finder-request',resolve:resolve};parent.postMessage({source:'alx-setup-plugin',type:'finder-request',requestId:requestId,pluginId:payload.pluginId,pickerId:payload.pickerId},location.origin);window.setTimeout(function(){if(!pending[requestId])return;delete pending[requestId];resolve(response(408,{error:'等待工作台 Finder 选择超时。'}))},10*60*1000)})};window.addEventListener('message',function(event){if(event.origin!==location.origin)return;var data=event.data;if(!data||data.source!=='alx-parent'||typeof data.requestId!=='string')return;var entry=pending[data.requestId];if(!entry)return;delete pending[data.requestId];if(data.type==='finder-result'){entry.resolve(response(data.error?400:200,data.error?{error:data.error}:{paths:Array.isArray(data.paths)?data.paths:[]}));return}if((entry.type==='webview-request'&&data.type==='webview-result')||(entry.type==='ui-request'&&data.type==='ui-result')||(entry.type==='webview-close-request'&&data.type==='webview-close-result')){entry.resolve(data)}});window.ALXHost={capabilities:function(){return json('/api/v1/system/capabilities')},finder:{pick:function(pluginId,pickerId){return window.fetch('/api/v1/system/capabilities/finder',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,pickerId:pickerId})}).then(function(res){return res.json().then(function(data){if(!res.ok)throw new Error(data&&data.error||'选择目录失败');return data.paths||[]})})}},context:function(pluginId,keys){return json('/api/v1/system/capabilities/context?pluginId='+encodeURIComponent(pluginId)+'&keys='+encodeURIComponent((keys||[]).join(',')))},desktop:{open:function(pluginId,target){return json('/api/v1/system/capabilities/desktop/open',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,target:target})})}},clipboard:{read:function(pluginId){return json('/api/v1/system/capabilities/clipboard?pluginId='+encodeURIComponent(pluginId))},write:function(pluginId,text){return json('/api/v1/system/capabilities/clipboard',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,text:text})})}},notification:{send:function(pluginId,title,message){return json('/api/v1/system/capabilities/notification',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,title:title,message:message})})}},info:function(pluginId){return json('/api/v1/system/capabilities/info?pluginId='+encodeURIComponent(pluginId))},network:{fetch:function(pluginId,url,method){return json('/api/v1/system/capabilities/network/fetch',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,url:url,method:method||'GET'})})}},webview:{open:function(pluginId,options){options=options||{};return json('/api/v1/system/capabilities/webview/open',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({pluginId:pluginId,title:options.title,url:options.url,resource:options.resource,width:options.width,height:options.height})}).then(function(data){if(!data||!data.ok)throw new Error(data&&data.error||'打开 WebView 失败');return bridge('webview-request',{pluginId:pluginId,webview:{title:data.title,src:data.src,kind:data.kind,width:data.width,height:data.height}})})},close:function(pluginId,webviewId){return bridge('webview-close-request',{pluginId:pluginId,webviewId:webviewId||''})}},ui:{alert:function(pluginId,options){return uiRequest('alert',pluginId,options)},message:function(pluginId,options){return uiRequest('message',pluginId,options)},modal:function(pluginId,options){return uiRequest('modal',pluginId,options)},notification:function(pluginId,options){return uiRequest('notification',pluginId,options)},setBusy:function(pluginId,busy){return uiRequest('set-busy',pluginId,{busy:busy===true})}}};function uiRequest(kind,pluginId,options){options=options||{};return bridge('ui-request',{pluginId:pluginId,ui:{kind:kind,busy:options.busy===true,title:options.title,message:options.message,confirmText:options.confirmText,cancelText:options.cancelText,type:options.type,duration:options.duration}})}})();`
}

func (s *server) sshHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keys, err := system.SSHKeys()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
	case http.MethodPost:
		key, err := system.GenerateSSHKey()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, key)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

// systemMCPHandler reports whether the local HTTP MCP server is actually
// running, so the header control shows a real status instead of pretending the
// service is always up. MCP is started manually via `alx mcp-http`.
func (s *server) systemMCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": probeMCPRunning()})
}

// probeMCPRunning probes the default Streamable HTTP MCP endpoint. Any HTTP
// response (even 401/404) means a server is listening; only a connection error
// means the service is not running.
func probeMCPRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:17391/mcp", nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return true
}

// startMCPStatusMonitor centralizes the unavoidable probe for an externally
// started HTTP MCP process. Browsers receive changes through SSE instead of
// each independently polling the endpoint.
func (s *server) startMCPStatusMonitor() {
	running := probeMCPRunning()
	s.mcpEvents.set(running)
	_, _ = s.publishEvent("system", "mcp.changed", map[string]any{"type": "mcp.changed", "running": running}, nil)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.mcpMonitorStop:
				return
			case <-ticker.C:
				running := probeMCPRunning()
				if s.mcpEvents.set(running) {
					_, _ = s.publishEvent("system", "mcp.changed", map[string]any{"type": "mcp.changed", "running": running}, nil)
				}
			}
		}
	}()
}

// isManifestPasswordOperation identifies an installed system plugin's fixed
// native operation. The manifest carries semantics; the host still owns the
// password prompt, intent, timeout, process execution and audit lifecycle.
func (s *server) isManifestPasswordOperation(pluginID, action string) bool {
	if s.plugins == nil {
		return false
	}
	plugin, err := s.plugins.Find(pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		return false
	}
	operation, ok := plugin.PrivilegedOperation(action)
	return ok && operation.Authorization == "password" && operationSupportsPlatform(operation, runtime.GOOS)
}

func operationSupportsPlatform(operation setupplugin.PrivilegedOperationSpec, platform string) bool {
	for _, candidate := range operation.Platforms {
		if candidate == platform {
			return true
		}
	}
	return false
}

func selectPrivilegedCommand(operation setupplugin.PrivilegedOperationSpec) (setupplugin.PrivilegedCommandSpec, error) {
	for _, command := range operation.Commands {
		if _, err := exec.LookPath(command.Program); err == nil {
			return command, nil
		}
	}
	return setupplugin.PrivilegedCommandSpec{}, errors.New("当前系统没有可用于此操作的受支持命令")
}

func (s *server) privilegedPreflightHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input privilegePreflightRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.PluginID == "" || input.Action == "" {
		writeError(w, http.StatusBadRequest, "请选择需要授权的系统操作。")
		return
	}
	response, account, source := s.buildPrivilegePreflight(r, input)
	if !response.Available {
		writeJSON(w, http.StatusOK, response)
		return
	}
	intent, err := s.privilegeStore.createIntent(input.PluginID, input.Action, input.PlanID, account, source, response.Authorization)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.IntentID = intent.ID
	response.ExpiresAt = intent.ExpiresAt.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, response)
}

func (s *server) buildPrivilegePreflight(r *http.Request, input privilegePreflightRequest) (privilegePreflightResponse, string, string) {
	response := privilegePreflightResponse{Authorization: "unavailable", Title: "系统权限请求"}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		response.Reason = "无法读取当前账户权限"
		return response, "", ""
	}
	account := status.Account
	if account == "" {
		account = "local"
	}
	source := privilegeRequestSource(r)
	if !s.privilegedRequestAllowed(r) {
		response.Description = "系统权限已由工作台策略关闭。"
		response.Reason = "当前工作台未允许系统权限操作。请联系管理员将 ALX_PRIVILEGED_MODE 设为 enabled，或在 local 模式下从本机工作台操作。"
		return response, account, source
	}
	if status.Enabled && (!status.Authenticated || !s.auth.Authorize(s.authToken(r), "system.manage")) {
		response.Reason = "当前账户没有系统变更权限。"
		return response, account, source
	}
	if plugin, findErr := s.plugins.Find(input.PluginID); findErr == nil {
		if operation, ok := plugin.PrivilegedOperation(input.Action); ok {
			if plugin.DevelopmentSource {
				response.SourceType, response.SourcePath = "development", plugin.Source
			} else {
				response.SourceType = "release"
			}
			response.Title = operation.Title
			response.Description = operation.Description
			if response.Description == "" {
				response.Description = "该系统插件将执行已声明的系统操作。"
			}
			if plugin.Online || !plugin.Enabled {
				response.Reason = "该系统插件尚未安装或已停用。"
				return response, account, source
			}
			if !operationSupportsPlatform(operation, runtime.GOOS) {
				response.Reason = "当前平台不支持该插件声明的系统授权方式。"
				return response, account, source
			}
			// Authentication is optional. On a fresh local workbench with no
			// account system configured, the local privileged-mode policy is the
			// authority; requiring a non-existent super-admin would permanently
			// block declared system operations. Once authentication is enabled,
			// retain the stricter logged-in super-admin requirement.
			if status.Enabled && (!status.Authenticated || !status.SuperAdmin) {
				response.Reason = "只有已登录的超级管理员可以执行系统操作。"
				return response, account, source
			}
			if operation.PlanAction != "" {
				if input.PlanID == "" {
					response.Reason = "请选择该插件预演后生成的变更计划。"
					return response, account, source
				}
				if _, planErr := s.privilegeStore.peekPlan(input.PlanID, input.PluginID, input.Action, account); planErr != nil {
					response.Reason = planErr.Error()
					return response, account, source
				}
			}
			if operation.UseLatestAudit {
				if _, auditErr := s.privilegeStore.latestAudit(input.PluginID); auditErr != nil {
					response.Reason = auditErr.Error()
					return response, account, source
				}
			}
			if operation.Authorization == "password" {
				if _, commandErr := selectPrivilegedCommand(operation); commandErr != nil {
					response.Reason = commandErr.Error()
					return response, account, source
				}
				response.Available, response.Authorization = true, "password"
				return response, account, source
			}
			switch runtime.GOOS {
			case "darwin":
				response.Available, response.Authorization = true, "native"
			case "windows":
				response.Available, response.Authorization = true, "native-uac"
			case "linux":
				response.Available, response.Authorization = true, "polkit"
			default:
				response.Reason = "当前平台没有可用的系统授权方式。"
			}
			return response, account, source
		}
	}
	response.Reason = "该系统插件没有声明此权限操作。"
	return response, account, source
}

func privilegeRequestSource(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return ""
	}
	return host
}

func (s *server) privilegedStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	status := system.CurrentPrivilegeStatus()
	audit := s.privilegeStore.auditStatus(status.Version)
	writeJSON(w, http.StatusOK, map[string]any{"privilege": status, "audit": audit})
}

func (s *server) localSystemDialogRequest(r *http.Request) bool {
	if !requestIsLoopback(r) {
		return false
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	return true
}

func (s *server) privilegedAuditHandler(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(r.URL.Query().Get("plugin"))
	if r.Method != http.MethodGet || pluginID == "" {
		writeError(w, http.StatusBadRequest, "请指定要读取审计记录的系统插件。")
		return
	}
	items, err := s.privilegeStore.auditItems(pluginID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "audit": s.privilegeStore.auditStatus(system.CurrentPrivilegeStatus().Version)})
}

func (s *server) handleManifestSudoAction(w http.ResponseWriter, r *http.Request, pluginID string, input setupPluginActionRequest) {
	if os.Geteuid() != 0 && (input.SudoPassword == nil || strings.TrimSpace(*input.SudoPassword) == "") {
		writeError(w, http.StatusBadRequest, "请输入当前系统账户的 sudo 密码。")
		return
	}
	if !input.Confirm {
		writeError(w, http.StatusBadRequest, "请先确认执行此系统操作。")
		return
	}
	if len(input.Params) != 0 {
		writeError(w, http.StatusBadRequest, "系统操作不接受浏览器传入的额外参数。")
		return
	}
	if !s.privilegedRequestAllowed(r) {
		writeError(w, http.StatusForbidden, "当前工作台策略未允许系统管理员密码操作。请联系管理员检查 ALX_PRIVILEGED_MODE。")
		return
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status.Enabled && (!status.Authenticated || !status.SuperAdmin) {
		writeError(w, http.StatusForbidden, "只有已登录的超级管理员可以安装系统依赖。")
		return
	}
	account := status.Account
	if account == "" {
		// Keep this identity aligned with buildPrivilegePreflight so a local
		// first-run request can consume the intent it just created.
		account = "local"
	}
	plugin, err := s.plugins.Find(pluginID)
	if err != nil || plugin.Online || !plugin.Enabled {
		writeError(w, http.StatusBadRequest, "系统插件未安装或已停用。")
		return
	}
	operation, ok := plugin.PrivilegedOperation(input.Action)
	if !ok || operation.Authorization != "password" || !operationSupportsPlatform(operation, runtime.GOOS) {
		writeError(w, http.StatusBadRequest, "该系统插件操作当前不可执行。")
		return
	}
	command, err := selectPrivilegedCommand(operation)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	intent, err := s.privilegeStore.validateIntent(input.AuthorizationID, pluginID, input.Action, "", account, privilegeRequestSource(r), "password")
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	key := sudoAttemptKey(account, r, pluginID, input.Action)
	if err := s.checkSudoAttempt(key); err != nil {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	var password []byte
	if input.SudoPassword != nil {
		password = []byte(*input.SudoPassword)
		*input.SudoPassword = ""
		input.SudoPassword = nil
	}
	created := operationTask{ID: "setup-" + time.Now().Format("20060102150405.000000000"), Root: "", Action: "setup:" + pluginID + ":" + input.Action, Status: "running", Output: "等待管理员授权…", CreatedAt: time.Now()}
	s.mu.Lock()
	s.operations = append([]operationTask{created}, s.operations...)
	if len(s.operations) > 40 {
		s.operations = s.operations[:40]
	}
	s.mu.Unlock()
	go func() {
		defer clearSudoPassword(password)
		s.updateOperation(created.ID, 5, "正在验证管理员授权…", "", false)
		ctx, cancel := context.WithTimeout(context.Background(), system.PrivilegedCommandTimeout)
		defer cancel()
		runCommand := s.runPrivilegedCommand
		if runCommand == nil {
			runCommand = system.RunSudoCommand
		}
		s.updateOperation(created.ID, 35, "正在执行系统操作…", "", false)
		output, runErr := runCommand(ctx, password, command.Program, command.Args)
		if errors.Is(runErr, system.ErrSudoPasswordInvalid) {
			s.recordSudoPasswordFailure(key)
		} else if runErr == nil {
			s.clearSudoAttempts(key)
			if consumeErr := s.privilegeStore.consumeIntent(intent); consumeErr != nil {
				s.updateOperation(created.ID, 100, "", consumeErr.Error(), true)
				return
			}
		}
		if runErr != nil {
			s.updateOperation(created.ID, 100, "", runErr.Error(), true)
			return
		}
		s.updateOperation(created.ID, 100, output, "", true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

// privilegedRequestAllowed applies the operator-selected scope. In the
// default enabled mode authenticated administrators may manage a remote host;
// local mode retains the old loopback-only behavior for hardened desktops.
func (s *server) privilegedRequestAllowed(r *http.Request) bool {
	status := system.CurrentPrivilegeStatus()
	if !status.Enabled {
		return false
	}
	if status.Mode != string(system.PrivilegedModeLocal) {
		return true
	}
	if !requestIsLoopback(r) {
		return false
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	return true
}

// clearSudoPassword is intentionally local to the host. It also protects
// test substitutes and future executor changes: the web layer never retains
// the transient password after the one fixed sudo command returns.
func clearSudoPassword(password []byte) {
	for index := range password {
		password[index] = 0
	}
}

func requestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sudoAttemptKey(account string, r *http.Request, pluginID, action string) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return strings.Join([]string{account, host, pluginID, action}, "\x00")
}

func (s *server) checkSudoAttempt(key string) error {
	if s.privilegeStore != nil {
		if err := s.privilegeStore.checkSudoAttempt(key); err != nil {
			return err
		}
	}
	s.sudoAttemptMu.Lock()
	defer s.sudoAttemptMu.Unlock()
	if s.sudoAttempts == nil {
		s.sudoAttempts = map[string]sudoAttempt{}
	}
	attempt := s.sudoAttempts[key]
	if time.Now().Before(attempt.LockedUntil) {
		return errors.New("密码连续错误次数过多，请在 10 分钟后再试")
	}
	return nil
}

func (s *server) recordSudoPasswordFailure(key string) {
	if s.privilegeStore != nil {
		_ = s.privilegeStore.recordSudoFailure(key)
		return
	}
	s.sudoAttemptMu.Lock()
	defer s.sudoAttemptMu.Unlock()
	if s.sudoAttempts == nil {
		s.sudoAttempts = map[string]sudoAttempt{}
	}
	attempt := s.sudoAttempts[key]
	attempt.Failures++
	if attempt.Failures >= 3 {
		attempt.Failures = 0
		attempt.LockedUntil = time.Now().Add(10 * time.Minute)
	}
	s.sudoAttempts[key] = attempt
}

func (s *server) clearSudoAttempts(key string) {
	if s.privilegeStore != nil {
		s.privilegeStore.clearSudoAttempt(key)
		return
	}
	s.sudoAttemptMu.Lock()
	delete(s.sudoAttempts, key)
	s.sudoAttemptMu.Unlock()
}

// startUpdateStatusMonitor performs the only periodic release lookup on the
// server. Dashboard tabs consume its persisted system event instead of each
// issuing a browser timer request.
func (s *server) startUpdateStatusMonitor() {
	go func() {
		s.refreshUpdateStatus()
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-s.updateMonitorStop:
				return
			case <-ticker.C:
				s.refreshUpdateStatus()
			}
		}
	}()
}

func sameUpdateStatus(left, right updateStatusState) bool {
	return left.Update == right.Update && left.Error == right.Error
}

func (s *server) refreshUpdateStatus(force ...bool) updateStatusState {
	next := updateStatusState{Update: releases.Update{Current: s.version}, CheckedAt: time.Now().UTC()}
	update, err := releases.SetupUpdate(s.version)
	if len(force) > 0 && force[0] {
		update, err = releases.SetupUpdateFresh(s.version)
	}
	if err != nil {
		next.Error = safeStoreError(err)
	} else {
		if update.Available && update.PlatformMatched && update.IntegrityReady {
			_, update.DownloadReady, err = system.CachedUpdate(update.AssetName, update.SHA256)
			if err != nil {
				next.Error = safeStoreError(err)
			}
		}
		next.Update = update
	}
	s.updateStateMu.Lock()
	previous := s.updateState
	s.updateState = next
	s.updateStateMu.Unlock()
	if !sameUpdateStatus(previous, next) {
		_, _ = s.publishEvent("system", "system.update.changed", map[string]any{"type": "system.update.changed", "update": next.Update, "checkedAt": next.CheckedAt, "error": next.Error}, nil)
	}
	return next
}

func (s *server) currentUpdateStatus() updateStatusState {
	s.updateStateMu.RLock()
	defer s.updateStateMu.RUnlock()
	return s.updateState
}

func (s *server) systemEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不受支持。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	sub := s.mcpEvents.subscribe()
	defer s.mcpEvents.unsubscribe(sub)
	write := func(running bool) bool {
		data, _ := json.Marshal(map[string]any{"type": "mcp.changed", "running": running})
		if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !write(s.mcpEvents.status()) {
		return
	}
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case running := <-sub:
			if !write(running) {
				return
			}
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) directoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	roots := s.currentDirectoryRoots()
	if r.Method != http.MethodGet {
		var input struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "目录操作内容无法识别。")
			return
		}
		path, err := managedDirectory(input.Path, roots)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		if r.Method == http.MethodPost {
			name := strings.TrimSpace(input.Name)
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
				writeError(w, http.StatusBadRequest, "文件夹名称无效。")
				return
			}
			created := filepath.Join(path, name)
			if err := os.Mkdir(created, 0755); err != nil {
				if os.IsExist(err) {
					writeError(w, http.StatusConflict, "同名文件夹已存在。")
				} else {
					writeError(w, http.StatusBadRequest, "无法新建文件夹："+err.Error())
				}
				return
			}
			writeJSON(w, http.StatusCreated, map[string]string{"path": created})
			return
		}
		if input.Path == "" || filepath.Clean(path) == filepath.Clean(input.Path) && func() bool {
			for _, root := range roots {
				if filepath.Clean(path) == filepath.Clean(root) {
					return true
				}
			}
			return false
		}() {
			writeError(w, http.StatusBadRequest, "不能删除管理范围的根目录。")
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			writeError(w, http.StatusBadRequest, "目标文件夹不存在。")
			return
		}
		if err := os.RemoveAll(path); err != nil {
			writeError(w, http.StatusBadRequest, "无法删除文件夹："+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": path})
		return
	}
	path, err := managedDirectory(r.URL.Query().Get("path"), roots)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "没有读取此位置的权限。请在系统设置中为 alx 授予“文件与文件夹”或“完全磁盘访问”权限，然后重试。", "permission": true})
			return
		}
		writeError(w, http.StatusBadRequest, "无法读取该目录")
		return
	}
	type directory struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	type file struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	showHidden := r.URL.Query().Get("hidden") == "true"
	includeFiles := r.URL.Query().Get("files") == "true"
	directories := make([]directory, 0)
	files := make([]file, 0)
	for _, entry := range entries {
		if entry.IsDir() && (showHidden || !strings.HasPrefix(entry.Name(), ".")) {
			directories = append(directories, directory{Name: entry.Name(), Path: filepath.Join(path, entry.Name())})
		}
		if includeFiles && !entry.IsDir() && (showHidden || !strings.HasPrefix(entry.Name(), ".")) {
			files = append(files, file{Name: entry.Name(), Path: filepath.Join(path, entry.Name())})
		}
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i].Name < directories[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	parent := ""
	for _, root := range roots {
		if filepath.Clean(path) != filepath.Clean(root) {
			if next := filepath.Dir(path); isWithinRoot(next, root) {
				parent = next
			}
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "parent": parent, "roots": roots, "locations": directoryLocations(roots), "directories": directories, "files": files})
}

func managedDirectoryRoots() []string {
	value := os.Getenv("ALEMONJS_SETUP_ROOTS")
	if value == "" {
		roots := []string{}
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, home)
		}
		if runtime.GOOS != "windows" {
			roots = appendDirectoryRoot(roots, string(filepath.Separator))
		}
		for _, mount := range mountedDirectoryRoots() {
			roots = appendDirectoryRoot(roots, mount)
		}
		if len(roots) == 0 {
			return []string{"/"}
		}
		return roots
	}
	roots := []string{}
	for _, item := range filepath.SplitList(value) {
		if path, err := filepath.Abs(item); err == nil {
			roots = append(roots, filepath.Clean(path))
		}
	}
	return roots
}

func appendDirectoryRoot(roots []string, path string) []string {
	path = filepath.Clean(path)
	for _, root := range roots {
		if root == path {
			return roots
		}
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return append(roots, path)
	}
	return roots
}

func mountedDirectoryRoots() []string {
	var base []string
	switch runtime.GOOS {
	case "darwin":
		base = []string{"/Volumes"}
	case "linux":
		if home, err := os.UserHomeDir(); err == nil {
			base = []string{"/media/" + filepath.Base(home), "/run/media/" + filepath.Base(home), "/mnt"}
		}
	case "windows":
		for drive := 'A'; drive <= 'Z'; drive++ {
			base = append(base, string(drive)+":\\")
		}
	}
	roots := []string{}
	for _, parent := range base {
		if runtime.GOOS == "windows" {
			roots = appendDirectoryRoot(roots, parent)
			continue
		}
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				roots = appendDirectoryRoot(roots, filepath.Join(parent, entry.Name()))
			}
		}
	}
	return roots
}

func directoryLocations(roots []string) []map[string]string {
	home, _ := os.UserHomeDir()
	items := make([]map[string]string, 0, len(roots))
	for _, root := range roots {
		name, kind := directoryLocation(root, home, runtime.GOOS)
		items = append(items, map[string]string{"name": name, "path": root, "kind": kind})
	}
	return items
}

// directoryLocation gives filesystem roots a human-readable name. In
// particular, filepath.Base(`C:\\`) on Windows can resolve to a separator,
// which used to leave users with a meaningless “/” disk entry.
func directoryLocation(root, home, goos string) (string, string) {
	clean := filepath.Clean(root)
	if clean == filepath.Clean(home) {
		return "主目录", "home"
	}
	if goos == "windows" {
		volume := filepath.VolumeName(root)
		// Keep this testable on non-Windows hosts and robust for paths received
		// from a Windows client in their native form.
		if volume == "" && len(root) >= 2 && root[1] == ':' {
			volume = root[:2]
		}
		if volume != "" {
			return "本地磁盘（" + strings.ToUpper(volume) + "）", "volume"
		}
		return "本地磁盘", "volume"
	}
	if clean == string(filepath.Separator) {
		return "系统磁盘", "disk"
	}
	if goos == "darwin" && strings.HasPrefix(clean, "/Volumes/") {
		return filepath.Base(clean), "volume"
	}
	if goos == "linux" && (strings.HasPrefix(clean, "/media/") || strings.HasPrefix(clean, "/run/media/")) {
		return filepath.Base(clean), "volume"
	}
	return filepath.Base(clean), "disk"
}

func (s *server) currentDirectoryRoots() []string {
	if os.Getenv("ALEMONJS_SETUP_ROOTS") != "" {
		return s.directoryRoots
	}
	return managedDirectoryRoots()
}

func (s *server) managedDirectory(requested string) (string, error) {
	return managedDirectory(requested, s.currentDirectoryRoots())
}

func managedDirectory(requested string, roots []string) (string, error) {
	path := requested
	if path == "" {
		path = roots[0]
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if isWithinRoot(absolute, root) {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("目录不在允许的管理范围内")
}

func isWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (s *server) releasesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	app := r.URL.Query().Get("app")
	var items []releases.Item
	var err error
	if r.URL.Query().Get("refresh") == "1" {
		items, err = releases.ListFresh(app)
	} else {
		items, err = releases.List(app)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if r.URL.Query().Get("platform") == "current" {
		items = releases.CurrentPlatformReleases(items)
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) updateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	status := s.currentUpdateStatus()
	if r.URL.Query().Get("refresh") == "1" {
		status = s.refreshUpdateStatus(true)
	}
	transaction, exists, transactionErr := system.ReadUpdateTransaction()
	if transactionErr != nil {
		writeError(w, http.StatusInternalServerError, transactionErr.Error())
		return
	}
	response := updateResponse{Update: status.Update, CheckedAt: status.CheckedAt, Error: status.Error}
	if exists {
		response.Transaction = &transaction
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *server) updateStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	s.confirmUpdateStartup()
	transaction, exists, err := system.ReadUpdateTransaction()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeJSON(w, http.StatusOK, map[string]any{"phase": "idle"})
		return
	}
	writeJSON(w, http.StatusOK, transaction)
}

// systemNetworkHandler owns AlemonX-managed content networking. It
// deliberately does not touch project git/npm settings, robot processes or
// 机器人应用页 traffic.
func (s *server) systemNetworkHandler(w http.ResponseWriter, r *http.Request) {
	if s.network == nil {
		writeError(w, http.StatusServiceUnavailable, "系统联网配置暂不可用。")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.network.Settings())
	case http.MethodPut:
		var input systemnetwork.Settings
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求内容无法识别。")
			return
		}
		saved, err := s.network.Save(input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	case http.MethodPost:
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		route := systemnetwork.Route(strings.TrimSpace(r.URL.Query().Get("target")))
		result := s.network.Test(ctx, route)
		if !result.OK {
			status := http.StatusBadGateway
			if result.Target == "" {
				status = http.StatusBadRequest
			}
			writeJSON(w, status, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) pluginDownloadBrokerHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginDownloadBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "插件官方下载服务不可用。")
		return
	}
	s.pluginDownloadBroker.serveHTTP(w, r)
}

// pluginDownloadCacheHandler manages the host-owned response cache used by
// every installed system plugin. It is deliberately separate from release
// version caching: clearing it never uninstalls a plugin or deletes its data.
func (s *server) pluginDownloadCacheHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginDownloadBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "插件下载缓存不可用。")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.pluginDownloadBroker.cacheSummary())
	case http.MethodDelete:
		summary, err := s.pluginDownloadBroker.clearCache()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) systemServiceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status, err := system.ServiceStatus()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": status, "runtime": system.UpdateRuntime(), "installed": system.ServiceInstalled(), "resilience": system.ServiceResilienceStatus()})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !input.Confirm {
		writeError(w, http.StatusBadRequest, "请确认服务操作。")
		return
	}
	switch input.Action {
	case "enable-linger":
		output, err := system.EnableUserLinger()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": output})
	case "install":
		if system.ServiceInstalled() {
			writeError(w, http.StatusConflict, "后台服务已安装；请使用启动或重启服务。")
			return
		}
		if system.UpdateRuntime() != "direct" {
			writeError(w, http.StatusConflict, "当前不是可迁移的前台运行模式。")
			return
		}
		output, err := system.PrepareService(updateRequestPort(r))
		if err != nil {
			writeError(w, http.StatusBadRequest, "注册后台服务失败："+err.Error())
			return
		}
		if err := system.ScheduleServiceStart(); err != nil {
			writeError(w, http.StatusInternalServerError, "无法安排后台服务启动："+err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"output": output + " 工作台即将切换到后台服务。"})
		s.requestGracefulUpdateShutdown()
	case "start":
		output, err := system.StartService()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": output})
	case "stop", "restart":
		// A foreground process is not represented by a LaunchAgent/systemd unit
		// or scheduled task. Handle it directly instead of asking the platform
		// supervisor to stop a service that does not exist.
		foreground := system.UpdateRuntime() == "direct"
		if foreground && input.Action == "restart" {
			if err := system.RestartForeground(updateRequestPort(r)); err != nil {
				writeError(w, http.StatusInternalServerError, "无法安排前台服务重启："+err.Error())
				return
			}
		}
		output := map[string]string{"stop": "正在停止 AlemonX 服务。", "restart": "正在重启 AlemonX 服务。"}[input.Action]
		if foreground {
			output = map[string]string{"stop": "正在关闭当前前台运行的 AlemonX 服务。", "restart": "正在关闭并重新启动当前前台运行的 AlemonX 服务。"}[input.Action]
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"output": output})
		go func(action string, isForeground bool) {
			time.Sleep(150 * time.Millisecond)
			if isForeground {
				if s.requestUpdateShutdown == nil {
					log.Printf("AlemonX 前台服务%s失败：未配置优雅关闭回调", map[string]string{"stop": "停止", "restart": "重启"}[action])
					return
				}
				s.requestUpdateShutdown()
				return
			}
			var err error
			if action == "stop" {
				_, err = system.StopService()
			} else {
				_, err = system.RestartService()
			}
			if err != nil {
				log.Printf("AlemonX 服务%s失败：%v", map[string]string{"stop": "停止", "restart": "重启"}[action], err)
			}
		}(input.Action, foreground)
	default:
		writeError(w, http.StatusBadRequest, "未知服务操作。")
	}
}

func (s *server) confirmUpdateStartup() {
	transaction, healthy, transactionErr := system.MarkUpdateHealthy(s.version)
	if transactionErr != nil {
		log.Printf("更新状态确认失败：%v", transactionErr)
		return
	}
	if !healthy || transaction.ArchivePath == "" {
		return
	}
	if _, pluginErr := system.SyncBundledPluginExecutors(transaction.ArchivePath); pluginErr != nil {
		transaction.PluginError = pluginErr.Error()
		_ = system.SaveUpdateTransaction(transaction)
		log.Printf("新版已启动，但内置插件同步失败：%v", pluginErr)
	}
}

func (s *server) aiProvidersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		items, err := s.ai.List()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct{ Provider, BaseURL, Model, APIKey string }
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "AI 配置无法识别。")
		return
	}
	if err := s.ai.Save(input.Provider, input.BaseURL, input.Model, input.APIKey); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": "AI 配置已仅保存到本机。"})
}

func (s *server) aiChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Provider string              `json:"provider"`
		Model    string              `json:"model"`
		Messages []map[string]string `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || len(input.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "请填写要发送的消息。")
		return
	}
	if len(input.Messages) > 30 {
		writeError(w, http.StatusBadRequest, "一次对话最多保留 30 条消息。")
		return
	}
	answer, err := s.ai.Chat(input.Provider, input.Model, input.Messages)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"answer": answer})
}

func (s *server) aiModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	items, err := s.ai.Models(r.URL.Query().Get("provider"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": items})
}

// downloadUpdateHandler downloads the exact asset selected by the server's
// current-version/platform check. The browser never supplies an arbitrary URL.
func (s *server) downloadUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.updateMu.TryLock() {
		writeError(w, http.StatusConflict, "已有更新任务正在进行，请等待完成。")
		return
	}
	defer s.updateMu.Unlock()
	update, err := releases.SetupUpdate(s.version)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if !update.Available {
		writeError(w, http.StatusBadRequest, "当前已经是最新版本。")
		return
	}
	if !update.PlatformMatched || !update.IntegrityReady {
		if update.PlatformMatched {
			if update.IntegrityError != "" {
				writeError(w, http.StatusBadGateway, update.IntegrityError)
				return
			}
			writeError(w, http.StatusBadRequest, "该版本未提供校验文件，无法安全地自动更新，请使用手动安装。")
			return
		}
		writeError(w, http.StatusBadRequest, "未找到当前系统对应的更新包，请使用手动更新。")
		return
	}
	transaction := system.UpdateTransaction{Phase: system.UpdatePhaseDownloading, TargetVersion: update.Latest, PreviousVersion: s.version, Port: updateRequestPort(r), Runtime: system.UpdateRuntime()}
	if err := system.SaveUpdateTransaction(transaction); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存更新事务："+err.Error())
		return
	}
	path, err := system.DownloadUpdate(update.DownloadURL, update.AssetName, update.SHA256, update.Latest)
	if err != nil {
		transaction.Phase, transaction.Error = system.UpdatePhaseFailed, err.Error()
		_ = system.SaveUpdateTransaction(transaction)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	transaction.Phase, transaction.ArchivePath, transaction.Error = system.UpdatePhaseStaged, path, ""
	_ = system.SaveUpdateTransaction(transaction)
	writeJSON(w, http.StatusOK, map[string]string{"output": "更新包已下载完成。确认后将立即更新并重启应用。", "assetName": update.AssetName})
}

func (s *server) applyUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !input.Confirm {
		writeError(w, http.StatusBadRequest, "请确认立即更新并重启应用。")
		return
	}
	if !s.updateMu.TryLock() {
		writeError(w, http.StatusConflict, "已有更新任务正在进行，请等待完成。")
		return
	}
	defer s.updateMu.Unlock()
	update, path, ready, err := system.ReadyPendingUpdate()
	if err != nil || !ready {
		writeError(w, http.StatusBadRequest, "更新包尚未下载完成，请先下载。")
		return
	}
	transaction := system.UpdateTransaction{Phase: system.UpdatePhaseApplying, TargetVersion: update.Version, PreviousVersion: s.version, ArchivePath: path, Port: updateRequestPort(r), Runtime: system.UpdateRuntime()}
	if err := system.SaveUpdateTransaction(transaction); err != nil {
		writeError(w, http.StatusInternalServerError, "无法保存更新事务："+err.Error())
		return
	}
	applied, err := system.ApplyUpdateFile(path)
	if err != nil {
		transaction.Phase, transaction.Error = system.UpdatePhaseFailed, err.Error()
		_ = system.SaveUpdateTransaction(transaction)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	transaction.Executable, transaction.BackupPath = applied.Executable, applied.BackupPath
	transaction.Phase, transaction.Error = system.UpdatePhaseRestarting, ""
	if err := system.SaveUpdateTransaction(transaction); err != nil {
		rollbackErr := system.RollbackAppliedUpdate(transaction, err)
		if rollbackErr != nil {
			writeError(w, http.StatusInternalServerError, "更新事务无法保存且回滚失败："+rollbackErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "更新事务无法保存，已恢复旧版本："+err.Error())
		return
	}
	if err := system.ScheduleRestart(); err != nil {
		if rollbackErr := system.RollbackAppliedUpdate(transaction, err); rollbackErr != nil {
			writeError(w, http.StatusInternalServerError, "更新已完成，但无法自动重启且回滚失败："+rollbackErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "无法安排自动重启，已恢复旧版本："+err.Error())
		return
	}
	if err := system.ClearPendingUpdate(); err != nil {
		log.Printf("更新已完成，但无法清理更新元数据：%v", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": applied.Message + " 正在重启应用。", "version": update.Version})
	s.requestGracefulUpdateShutdown()
}

func (s *server) requestGracefulUpdateShutdown() {
	if s.requestUpdateShutdown == nil {
		return
	}
	go func() {
		// Let the confirmation JSON flush before the server stops accepting work.
		time.Sleep(150 * time.Millisecond)
		s.requestUpdateShutdown()
	}()
}

func updateRequestPort(r *http.Request) string {
	host := strings.TrimSpace(r.Host)
	if _, port, err := net.SplitHostPort(host); err == nil && port != "" {
		return port
	}
	if strings.Trim(host, "[]") != "" && !strings.Contains(host, ":") {
		return "17390"
	}
	if value := strings.TrimSpace(os.Getenv("PORT")); value != "" {
		return value
	}
	return "17390"
}

func (s *server) loadUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 200<<20)
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "更新包无效或超过 200 MB。")
		return
	}
	if r.FormValue("confirm") != "true" {
		writeError(w, http.StatusBadRequest, "请确认载入更新包。")
		return
	}
	if !s.updateMu.TryLock() {
		writeError(w, http.StatusConflict, "已有更新任务正在进行，请等待完成。")
		return
	}
	defer s.updateMu.Unlock()
	file, header, err := r.FormFile("package")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择更新包。")
		return
	}
	defer file.Close()
	if !isSupportedUpdateArchive(header.Filename) {
		writeError(w, http.StatusBadRequest, "更新包应为 GitHub Release 下载的 .zip 文件。")
		return
	}
	if header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "更新包为空，请重新选择完整下载的安装包。")
		return
	}
	directory, err := os.MkdirTemp("", "alx-upload-")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, filepath.Base(header.Filename))
	output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, copyErr := io.Copy(output, file)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusBadRequest, "无法读取更新包。")
		return
	}
	// Upload only stages the archive; the operator confirms installation via
	// /api/v1/update/apply, which reuses the verified apply+restart path.
	staged, err := system.StageUploadedUpdate(path, header.Filename, r.FormValue("version"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	transaction := system.UpdateTransaction{Phase: system.UpdatePhaseStaged, TargetVersion: staged.Version, PreviousVersion: s.version, ArchivePath: staged.Path, Port: updateRequestPort(r), Runtime: system.UpdateRuntime()}
	if err := system.SaveUpdateTransaction(transaction); err != nil {
		_ = system.ClearPendingUpdate()
		writeError(w, http.StatusInternalServerError, "无法保存更新事务："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"staged": "true", "filename": filepath.Base(header.Filename), "version": staged.Version, "output": "更新包已上传并通过平台校验，确认后才会安装并重启。"})
}

func isSupportedUpdateArchive(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".zip")
}

func (s *server) robotHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Root    string `json:"root"`
		File    string `json:"file"`
		Content string `json:"content"`
		Action  string `json:"action"`
		Message string `json:"message"`
		Package string `json:"package"`
		Version string `json:"version"`
		Tag     string `json:"tag"`
		Token   string `json:"token"`
		Confirm string `json:"confirm"`
	}
	if r.Method == http.MethodGet {
		input.Root = r.URL.Query().Get("root")
		input.File = r.URL.Query().Get("file")
	} else if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	if r.Method == http.MethodPost {
		log.Printf("[ROBOT 同步] 开始 action=%s root=%q", input.Action, input.Root)
	}
	var result robot.Result
	var err error
	switch r.Method {
	case http.MethodGet:
		result, err = s.robots.Read(input.Root, input.File)
	case http.MethodPut:
		result, err = s.robots.Write(input.Root, input.File, input.Content)
	case http.MethodPost:
		result, err = s.robots.Run(input.Root, input.Action, input.Message, input.Package, input.Version, input.Tag, input.Token, input.Confirm == "true")
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if err != nil {
		if r.Method == http.MethodPost {
			log.Printf("[ROBOT 同步] 失败 action=%s root=%q error=%s", input.Action, input.Root, err)
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.Method == http.MethodPost {
		log.Printf("[ROBOT 同步] 完成 action=%s root=%q output=%dB", input.Action, input.Root, len(result.Output))
	}
	writeJSON(w, http.StatusOK, result)
}

// robotProjectsHandler exposes only valid AlemonJS project directories below
// the workbench's managed roots. System-plugin pages use it to offer a safe
// target picker instead of accepting an arbitrary filesystem path.
func (s *server) robotProjectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true"
	roots := s.currentDirectoryRoots()
	rootsKey := strings.Join(roots, "\x00")
	s.robotProjectsMu.Lock()
	if !refresh && s.robotProjectsCache.rootsKey == rootsKey && time.Since(s.robotProjectsCache.updated) < time.Minute {
		items := append([]robotProjectItem(nil), s.robotProjectsCache.items...)
		s.robotProjectsMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "cached": true})
		return
	}
	s.robotProjectsMu.Unlock()
	items := make([]robotProjectItem, 0, 16)
	seen := map[string]bool{}
	for _, base := range roots {
		base = filepath.Clean(base)
		_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if path != base && (entry.Name() == "node_modules" || entry.Name() == ".git" || entry.Name() == ".alemon" || strings.HasPrefix(entry.Name(), ".")) {
					return filepath.SkipDir
				}
				if rel, err := filepath.Rel(base, path); err == nil && rel != "." && strings.Count(rel, string(filepath.Separator)) >= 3 {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "alemon.config.yaml" {
				return nil
			}
			root := filepath.Dir(path)
			if seen[root] || len(items) >= 100 {
				return nil
			}
			if _, err := os.Stat(filepath.Join(root, "package.json")); err != nil {
				return nil
			}
			if _, err := s.robots.Read(root, "alemon.config.yaml"); err != nil {
				return nil
			}
			seen[root] = true
			items = append(items, robotProjectItem{Root: root, Name: filepath.Base(root)})
			return nil
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	s.robotProjectsMu.Lock()
	s.robotProjectsCache = robotProjectsSnapshot{rootsKey: rootsKey, updated: time.Now(), items: append([]robotProjectItem(nil), items...)}
	s.robotProjectsMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "cached": false})
}

// consolePayload splits the terminal's live process output from its static
// project context. The two have different update rates, so returning them as
// one blob forces the browser to re-render the expensive snapshot on every poll.
type consolePayload struct {
	Path     string `json:"path"`
	Output   string `json:"output"`
	Snapshot string `json:"snapshot"`
	Mode     string `json:"mode"`
	Running  bool   `json:"running"`
}

func (s *server) robotConsoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	// The refresh flag bypasses the snapshot cache so a manual terminal refresh
	// reflects just-saved package.json / config changes instead of stale context.
	var console robot.Result
	var err error
	if r.URL.Query().Get("refresh") == "1" {
		console, err = s.robots.Console(root)
		if err == nil {
			s.mu.Lock()
			s.consoleCache[root] = consoleSnapshot{output: console.Output, at: time.Now()}
			s.mu.Unlock()
		}
	} else {
		console, err = s.cachedRobotConsole(root)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Prefer the process that is actually running; otherwise show the most
	// recent dev/app run. A fixed "app first, then dev" order would hide a fresh
	// dev run whenever an older foreground run still has history.
	output, status, runError, mode := s.runtimeProcessOutput(root)
	payload := consolePayload{Path: root, Snapshot: console.Output, Mode: mode}
	switch {
	case status == "running":
		payload.Output = "$ " + mode + "实时输出\n" + output
		payload.Running = true
	case output != "":
		payload.Output = "$ 最近一次" + mode + "输出\n" + output
		if status == "failed" && runError != "" {
			// runError already reads like "开发进程已退出：exit status 1".
			// Appending it as its own paragraph avoids a redundant heading.
			payload.Output += "\n\n" + runError
		}
	default:
		payload.Output = "当前没有正在运行的前台或开发进程。"
	}
	writeJSON(w, http.StatusOK, payload)
}

type robotTerminalRequest struct {
	Root      string `json:"root"`
	Command   string `json:"command"`
	Directory string `json:"directory,omitempty"`
}

// robotTerminalHandler runs a command with the robot directory as its working
// directory. It is intentionally a command runner rather than a persistent
// PTY, but it accepts normal shell syntax so the terminal remains useful for
// project work instead of maintaining a brittle command allowlist.
func (s *server) robotTerminalHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input robotTerminalRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "终端命令格式无效。")
		return
	}
	validation, err := (robot.Manager{}).Validate(input.Root)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	root := validation.Path
	command := strings.TrimSpace(input.Command)
	if err := validateRobotTerminalCommand(command); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	workingDirectory, relativeDirectory, err := resolveRobotTerminalDirectory(root, input.Directory)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if target, changeDirectory := robotTerminalChangeDirectory(command); changeDirectory {
		_, relativeDirectory, err = resolveRobotTerminalDirectory(root, filepath.ToSlash(filepath.Join(relativeDirectory, target)))
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}
		shownDirectory := "."
		if relativeDirectory != "" {
			shownDirectory = "./" + relativeDirectory
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": "$ " + command + "\n当前目录：" + shownDirectory, "directory": relativeDirectory})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}
	cmd.Dir = workingDirectory
	output, runErr := cmd.CombinedOutput()
	text := string(output)
	if runErr != nil {
		if ctx.Err() != nil {
			text += "\n命令已超时并被终止。"
		} else {
			text += "\n" + runErr.Error()
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": "$ " + command + "\n" + text, "directory": relativeDirectory})
}

// resolveRobotTerminalDirectory turns the browser's relative terminal state
// into a real directory. Symlinks are resolved before the containment check,
// so a project child cannot escape into a parent or another workspace.
func resolveRobotTerminalDirectory(root, relative string) (string, string, error) {
	relative = strings.TrimSpace(relative)
	if relative == "" || relative == "." {
		relative = "."
	}
	for _, part := range strings.Split(strings.ReplaceAll(relative, "\\", "/"), "/") {
		if part == ".." {
			return "", "", fmt.Errorf("终端不能访问机器人目录的父目录。")
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("终端只能访问当前机器人目录及其子目录。")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", fmt.Errorf("无法确认机器人目录：%w", err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, cleaned))
	if err != nil {
		return "", "", fmt.Errorf("终端目录不存在或无法访问。")
	}
	info, err := os.Stat(resolvedDirectory)
	if err != nil || !info.IsDir() || !isWithinRoot(resolvedDirectory, resolvedRoot) {
		return "", "", fmt.Errorf("终端只能访问当前机器人目录及其子目录。")
	}
	canonicalRelative, err := filepath.Rel(resolvedRoot, resolvedDirectory)
	if err != nil || canonicalRelative == "." {
		return resolvedDirectory, "", nil
	}
	return resolvedDirectory, filepath.ToSlash(canonicalRelative), nil
}

func robotTerminalChangeDirectory(command string) (string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "cd") || len(fields) > 2 {
		return "", false
	}
	if len(fields) == 1 {
		return ".", true
	}
	return fields[1], true
}

func validateRobotTerminalCommand(command string) error {
	if command == "" {
		return fmt.Errorf("请输入终端命令。")
	}
	if len(command) > 8<<10 {
		return fmt.Errorf("终端命令不能超过 8 KB。")
	}
	if strings.ContainsAny(command, "\x00\r\n") {
		return fmt.Errorf("终端命令不能包含空字节或换行。")
	}
	return nil
}

// cachedRobotConsole reuses the terminal's static context for a short window so
// the polling loop never spawns git/node just to render an unchanged header.
func (s *server) cachedRobotConsole(root string) (robot.Result, error) {
	s.mu.RLock()
	cached, ok := s.consoleCache[root]
	s.mu.RUnlock()
	if ok && time.Since(cached.at) < 15*time.Second {
		return robot.Result{Path: root, Output: cached.output}, nil
	}
	result, err := s.robots.Console(root)
	if err != nil {
		return robot.Result{}, err
	}
	s.mu.Lock()
	s.consoleCache[root] = consoleSnapshot{output: result.Output, at: time.Now()}
	s.mu.Unlock()
	return result, nil
}

func parsePM2AuditQuery(r *http.Request) (robot.PM2AuditQuery, error) {
	q := robot.PM2AuditQuery{
		Date:   strings.TrimSpace(r.URL.Query().Get("date")),
		Since:  strings.TrimSpace(r.URL.Query().Get("since")),
		Until:  strings.TrimSpace(r.URL.Query().Get("until")),
		Source: strings.TrimSpace(r.URL.Query().Get("source")),
		Query:  strings.TrimSpace(r.URL.Query().Get("query")),
		Page:   1,
	}
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return q, errors.New("日志页码无效。")
		}
		q.Page = page
	}
	if raw := r.URL.Query().Get("perPage"); raw != "" {
		perPage, err := strconv.Atoi(raw)
		if err != nil || perPage < 1 {
			return q, errors.New("每页行数无效。")
		}
		q.PerPage = perPage
	}
	return q, nil
}

func (s *server) robotPM2LogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	query, err := parsePM2AuditQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.robots.PM2AuditLogs(root, query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotPM2LogDaysHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	days, err := s.robots.PM2LogDays(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days})
}

func (s *server) robotPM2LogExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	query, err := parsePM2AuditQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	content, err := s.robots.PM2LogExport(root, query)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := "pm2-logs"
	if query.Date != "" {
		name = "pm2-" + query.Date
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.log"`, name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(content))
}

// robotPM2LogStreamHandler tails the robot's PM2 log files over SSE so the
// audit viewer can stay on the latest page in real time without polling.
func (s *server) robotPM2LogStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不受支持。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var writeMu sync.Mutex
	writeFrame := func(payload []byte) bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err := w.Write(append(append([]byte("data: "), payload...), '\n', '\n')); err != nil {
			cancel()
			return false
		}
		flusher.Flush()
		return true
	}
	emit := func(source, text string) {
		if ctx.Err() != nil {
			return
		}
		payload, err := json.Marshal(map[string]string{"source": source, "text": text})
		if err != nil {
			return
		}
		writeFrame(payload)
	}
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := s.robots.StreamPM2LogFiles(ctx, root, emit); err != nil && ctx.Err() == nil {
			if payload, marshalErr := json.Marshal(map[string]string{"error": err.Error()}); marshalErr == nil {
				writeFrame(payload)
			}
		}
	}()
	for {
		select {
		case <-done:
			return
		case <-heartbeat.C:
			writeMu.Lock()
			_, writeErr := w.Write([]byte(": ping\n\n"))
			flusher.Flush()
			writeMu.Unlock()
			if writeErr != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) robotPM2StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	result, err := s.robots.PM2Status(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotPM2ProcessesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	result, err := s.robots.PM2Processes(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (s *server) robotAppPortHandler(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if r.Method == http.MethodGet {
		if r.URL.Query().Get("probe") == "1" {
			info, infoErr := s.robots.AppPort(root)
			if infoErr != nil {
				writeError(w, http.StatusBadRequest, infoErr.Error())
				return
			}
			if !info.Configured {
				writeError(w, http.StatusConflict, "请先为当前机器人配置应用端口（serverPort）。")
				return
			}
			reachable, port, err := s.robots.AppPortReachable(root)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"reachable": reachable, "port": port})
			return
		}
		info, err := s.robots.AppPort(root)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, info)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := s.robots.SaveAppPort(root, input.Port)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotTestPortHandler(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if r.Method == http.MethodGet {
		if r.URL.Query().Get("probe") == "1" {
			info, infoErr := s.robots.TestPort(root)
			if infoErr != nil {
				writeError(w, http.StatusBadRequest, infoErr.Error())
				return
			}
			if !info.Configured {
				writeError(w, http.StatusConflict, "请先为当前机器人配置服务端口（port）。")
				return
			}
			reachable, port, err := s.robots.TestPortReachable(root)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"reachable": reachable, "port": port})
			return
		}
		info, err := s.robots.TestPort(root)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sandbox, sandboxErr := s.robots.TestSandboxAvailable(root)
		if sandboxErr != nil {
			writeError(w, http.StatusBadRequest, sandboxErr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"port":       info.Port,
			"configured": info.Configured,
			"sandbox":    sandbox,
		})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := s.robots.SaveTestPort(root, input.Port)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// robotPortStatus is the per-port payload of the proactive start preflight.
// Owned marks an occupant that belongs to the same robot directory (a
// supervised dev/app process, a stray process from an earlier run, or the
// directory's PM2 service), so the UI can distinguish it from a real clash.
type robotPortStatus struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Port       int    `json:"port"`
	Configured bool   `json:"configured"`
	Occupied   bool   `json:"occupied"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
	Owned      bool   `json:"owned"`
	Error      string `json:"error,omitempty"`
}

func (s *server) robotPortsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	ports, err := s.robots.Ports(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items := make([]robotPortStatus, 0, len(ports))
	for _, info := range ports {
		item := robotPortStatus{
			Kind:       info.Kind,
			Label:      info.Label,
			Port:       info.Port,
			Configured: info.Configured,
		}
		occupied, occupants := sniffPort(info.Port)
		item.Occupied = occupied
		for _, occupant := range occupants {
			if item.PID == 0 && occupant.PID > 0 {
				item.PID = occupant.PID
			}
			if item.Process == "" {
				item.Process = occupant.Process
			}
			if s.portOccupantOwnedByRoot(root, occupant) {
				item.Owned = true
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) robotAppsHandler(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("root")
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if r.Method == http.MethodGet {
		apps, err := s.robots.EnabledApps(root)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": apps})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Package string `json:"package"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := s.robots.SetAppEnabled(root, input.Package, input.Enabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotRuntimeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	overview, err := s.robots.RuntimeOverview(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *server) robotRuntimePreflightHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	preflight, err := s.robots.RuntimePreflight(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preflight)
}

// robotRuntimeRepairHandler exposes an inspect-first repair flow. GET is
// side-effect free; POST performs only the planned safe changes unless a
// caller explicitly confirms replacement of a custom runtime configuration.
func (s *server) robotRuntimeRepairHandler(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "all"
	}
	switch r.Method {
	case http.MethodGet:
		plan, err := s.robots.RuntimeRepairPlan(r.URL.Query().Get("root"), mode)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, plan)
	case http.MethodPost:
		var input struct {
			Root             string `json:"root"`
			Mode             string `json:"mode"`
			ConfirmOverrides bool   `json:"confirmOverrides"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求内容无法识别。")
			return
		}
		if input.Mode != "" {
			mode = input.Mode
		}
		result, err := s.robots.ApplyRuntimeRepair(input.Root, mode, input.ConfirmOverrides)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "repair": result})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func (s *server) robotValidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	result, err := s.robots.Validate(r.URL.Query().Get("root"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "path": result.Path})
}

// robotEventsHandler streams task-state and process-output events over SSE so
// the UI can update live without polling robot/tasks and robot/console.
func (s *server) robotEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不受支持。")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	sub := s.events.subscribe()
	defer s.events.unsubscribe(sub)
	taskID := r.URL.Query().Get("taskId")
	lastID, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if queryID, err := strconv.ParseInt(r.URL.Query().Get("lastEventId"), 10, 64); err == nil && queryID > lastID {
		lastID = queryID
	}
	write := func(event robotEvent) bool {
		data, err := json.Marshal(event)
		if err != nil {
			return true
		}
		if event.ID > 0 {
			if _, err := w.Write([]byte("id: " + strconv.FormatInt(event.ID, 10) + "\n")); err != nil {
				return false
			}
		}
		if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if s.operationEvents != nil {
		for _, record := range s.operationEvents.after(lastID, map[string]bool{"robot": true}) {
			var event robotEvent
			if json.Unmarshal(record.Data, &event) != nil || (taskID != "" && event.TaskID != taskID) {
				continue
			}
			event.ID = record.ID
			if !write(event) {
				return
			}
		}
	}
	if taskID != "" && lastID == 0 {
		s.mu.RLock()
		var current *operationTask
		for index := range s.operations {
			if s.operations[index].ID == taskID {
				copy := s.operations[index]
				current = &copy
				break
			}
		}
		s.mu.RUnlock()
		if current != nil {
			if !write(robotEvent{Type: "task", TaskID: current.ID, Task: current}) {
				return
			}
		}
	}
	// Heartbeat keeps proxies from closing the idle connection.
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-sub:
			if taskID != "" && event.TaskID != taskID {
				continue
			}
			if !write(event) {
				return
			}
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) robotTasksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		id := r.URL.Query().Get("id")
		s.mu.RLock()
		defer s.mu.RUnlock()
		if id == "" {
			operations := s.operations
			if operations == nil {
				operations = []operationTask{}
			}
			writeJSON(w, http.StatusOK, operations)
			return
		}
		for _, item := range s.operations {
			if item.ID == id {
				writeJSON(w, http.StatusOK, item)
				return
			}
		}
		writeError(w, http.StatusNotFound, "操作任务不存在或已过期。")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root    string `json:"root"`
		Action  string `json:"action"`
		Ready   string `json:"ready"`
		Value   string `json:"value"`
		Message string `json:"message"`
		Package string `json:"package"`
		Version string `json:"version"`
		Tag     string `json:"tag"`
		Token   string `json:"token"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	if input.Root == "" || input.Action == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录和操作。")
		return
	}
	if _, err := s.robots.Validate(input.Root); err != nil {
		writeError(w, http.StatusBadRequest, "当前机器人目录不可用："+err.Error()+"。请在左侧移除后重新选择目录。")
		return
	}
	// Proactively sniff the ports the robot will bind before any start action.
	// An unrelated occupier is reported here instead of after the process boots
	// and prints its bind error to the logs.
	readyKind := input.Ready
	if input.Action == "app-open" {
		readyKind = "app"
	}
	if readyKind != "" && readyKind != "app" && readyKind != "test" {
		writeError(w, http.StatusBadRequest, "未知的启动就绪类型。")
		return
	}
	if readyKind == "app" {
		info, err := s.robots.AppPort(input.Root)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !info.Configured {
			writeError(w, http.StatusConflict, "请先为当前机器人配置应用端口（serverPort）。")
			return
		}
	}
	if readyKind == "test" {
		info, err := s.robots.TestPort(input.Root)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !info.Configured {
			writeError(w, http.StatusConflict, "请先为当前机器人配置服务端口（port）。")
			return
		}
	}
	if input.Action == "dev" || input.Action == "app" || input.Action == "app-open" || input.Action == "pm2" || input.Action == "pm2-reload" || input.Action == "pm2-restart" {
		if blockers := s.robotStartPortBlockers(input.Root, readyKind); len(blockers) > 0 {
			writeError(w, http.StatusConflict, "启动前端口检查未通过："+strings.Join(blockers, " "))
			return
		}
	}
	created := operationTask{ID: "robot-" + time.Now().Format("20060102150405.000000000"), Root: input.Root, Action: input.Action, Status: "running", CreatedAt: time.Now()}
	if input.Action == "dev-stop" || input.Action == "app-stop" {
		mode := map[string]string{"dev-stop": "开发模式", "app-stop": "前台运行"}[input.Action]
		if !s.developmentRunning(input.Root) {
			s.settleUnmanagedLocalOperations(input.Root, "未检测到受管本机进程，已结束遗留运行状态。")
			finished := time.Now()
			created.Status = "completed"
			created.Output = "当前没有正在运行的" + mode + "进程。"
			created.FinishedAt = &finished
			s.addOperation(created)
			writeJSON(w, http.StatusAccepted, created)
			return
		}
		// The stop task stays "running" until the process actually exits, so the
		// UI shows a real "正在停止" state instead of claiming success early.
		created.Status = "running"
		created.Output = "正在请求停止" + mode + "…"
		s.addOperation(created)
		if !s.stopDevelopment(input.Root, mode) {
			// The process vanished between the check and the stop request. Finish
			// the task immediately so it never hangs as "running" forever.
			s.settleUnmanagedLocalOperations(input.Root, "进程已退出，无需停止。")
		}
		writeJSON(w, http.StatusAccepted, created)
		return
	}
	if input.Action == "dev" || input.Action == "app" || input.Action == "app-open" {
		// Starting a robot owns the dependency lifecycle. A new clone or a
		// changed workspace must not make beginners detour to a separate
		// "install dependencies" button before they can run it.
		dependencyOutput, dependencyErr := s.robots.EnsureRuntimeDependencies(input.Root)
		if dependencyErr != nil {
			writeError(w, http.StatusBadRequest, dependencyErr.Error())
			return
		}
		// qq-bot Actions are served by the platform adapter. Current AlemonJS
		// releases run that adapter through IPC, while their CBP server only
		// forwards full-receive browser Actions to WebSocket platform clients.
		// Apply the guarded compatibility bridge before booting the robot so the
		// running adapter can actually receive and answer tool requests.
		if _, patchErr := robot.EnsureCBPIPCActionBridge(input.Root); patchErr != nil {
			writeError(w, http.StatusBadRequest, patchErr.Error())
			return
		}
		if s.developmentRunning(input.Root) {
			writeError(w, http.StatusConflict, "当前目录已有前台或开发进程正在运行；请先停止后再启动。")
			return
		}
		// An app launch takes over its own configured web port from PM2. A test
		// launch is isolated to the configured CBP port and must not interrupt a
		// running application merely because it belongs to the same project.
		if readyKind == "app" {
			if err := s.stopPM2ForStart(input.Root); err != nil {
				writeError(w, http.StatusConflict, "停止后台（PM2）服务失败："+err.Error())
				return
			}
		}
		// Only the explicitly selected ready port participates in start-up. Never
		// infer or free a framework default: several robot directories may exist.
		if readyKind == "app" {
			info, _ := s.robots.AppPort(input.Root)
			if !s.waitPortFreeOn(info.Port, 0) {
				writeError(w, http.StatusConflict, "应用端口仍被占用；请停止当前机器人自己的进程后重试。")
				return
			}
		}
		if readyKind == "test" {
			info, _ := s.robots.TestPort(input.Root)
			if !s.waitPortFreeOn(info.Port, 0) {
				writeError(w, http.StatusConflict, "服务端口仍被占用；请停止当前机器人的测试进程后重试。")
				return
			}
		}
		command, err := s.robots.DevelopmentCommand(input.Root)
		if input.Action == "app" {
			command, err = s.robots.ForegroundCommand(input.Root)
		}
		// "应用"与"测试"只有就绪端口不同：两者都优先 dev，缺失时
		// 自动回退 app。只有两个启动脚本都不存在时才要求用户修复。
		if input.Action == "app-open" || readyKind == "test" {
			var mode string
			command, mode, err = s.robots.ApplicationCommand(input.Root)
			created.Action = mode
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		configureManagedProcess(command)
		sandboxCleanup := func() {}
		// testone 一律以无 login 模式启动：临时复制一份配置并注释掉
		// login/platform/serverPort，通过项目内相对路径的 CFG_PATH 覆盖
		// （alemonjs 用 path.join(cwd, CFG_PATH) 解析），用户配置
		// alemon.config.yaml 始终不被修改；进程退出时清理临时文件。
		if readyKind == "test" {
			sandboxPath, cleanup, sandboxErr := s.robots.SandboxConfig(input.Root)
			if sandboxErr != nil {
				writeError(w, http.StatusBadRequest, "准备无 login 测试配置失败："+sandboxErr.Error())
				return
			}
			sandboxCleanup = cleanup
			if sandboxPath != "" {
				if command.Env == nil {
					command.Env = os.Environ()
				}
				command.Env = append(command.Env, "CFG_PATH="+sandboxPath)
				log.Printf("[ROBOT %s] testone 使用无 login 沙盒配置 %s", created.ID, sandboxPath)
			}
		}
		// Route stdout/stderr through a writer instead of StdoutPipe. exec copies
		// process output into the writer on its own goroutine, so when the
		// process exits the copy ends with a clean EOF rather than the pipe
		// being closed underneath a concurrent reader (the "read |0: file
		// already closed" error). It also cannot lose the final lines.
		command.Stdout = newOperationWriter(created.ID, s)
		command.Stderr = newOperationWriter(created.ID, s)
		if err := command.Start(); err != nil {
			writeError(w, http.StatusBadRequest, "运行启动失败："+err.Error())
			return
		}
		if !s.registerDevelopment(input.Root, created.ID, created.Action, command, processGroupID(command), sandboxCleanup) {
			_ = command.Process.Kill()
			writeError(w, http.StatusConflict, "当前目录已有前台或开发进程正在运行；请先停止后再启动。")
			return
		}
		created.Output = strings.TrimSpace(dependencyOutput)
		if created.Output != "" {
			created.Output += "\n"
		}
		created.Output += map[bool]string{true: "开发模式已启动，正在等待进程输出…\n", false: "前台运行已启动，正在等待进程输出…\n"}[created.Action == "dev"]
		s.addOperation(created)
		log.Printf("[ROBOT %s] 开始 action=%s root=%q", created.ID, input.Action, created.Root)
		go s.watchDevelopmentTask(created.ID, input.Root, created.Action, command)
		go s.watchPortReadiness(created.ID, input.Root, readyKind)
		writeJSON(w, http.StatusAccepted, created)
		return
	}
	if input.Action == "pm2" || input.Action == "pm2-reload" {
		// "谁最后启动谁为准": a running local process holds the app port, so a
		// background start first stops the local process and waits for release.
		// An absent serverPort is intentionally not substituted with a global
		// default: it may belong to another robot directory.
		s.stopLocalForStart(input.Root)
		if info, err := s.robots.AppPort(input.Root); err == nil && info.Configured && !s.waitPortFreeOn(info.Port, 0) {
			writeError(w, http.StatusConflict, "应用端口仍被占用；请停止当前机器人自己的进程后重试。")
			return
		}
	}
	log.Printf("[ROBOT %s] 开始 action=%s root=%q", created.ID, created.Action, created.Root)
	s.addOperation(created)
	go func() {
		var result robot.Result
		var err error
		// A user-initiated PM2 action is an ordinary robot runtime action. It
		// must never require an AI-operations lease, budget, policy or key.
		// GuardedPM2Executor remains reserved for advanced automatic maintenance.
		result, err = s.robots.Run(input.Root, input.Action, input.Message, input.Package, input.Version, input.Tag, input.Token, input.Confirm == "true")
		finished := time.Now()
		s.mu.Lock()
		var snapshot operationTask
		for index := range s.operations {
			if s.operations[index].ID == created.ID {
				s.operations[index].Status = "completed"
				s.operations[index].Output = result.Output
				s.operations[index].FinishedAt = &finished
				if err != nil {
					s.operations[index].Status = "failed"
					s.operations[index].Error = err.Error()
				}
				snapshot = s.operations[index]
				break
			}
		}
		s.mu.Unlock()
		if snapshot.ID != "" {
			s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
		}
		if err != nil {
			log.Printf("[ROBOT %s] 失败 action=%s root=%q error=%s", created.ID, created.Action, created.Root, err)
			return
		}
		log.Printf("[ROBOT %s] 完成 action=%s root=%q output=%dB", created.ID, created.Action, created.Root, len(result.Output))
	}()
	writeJSON(w, http.StatusAccepted, created)
}

func (s *server) addOperation(created operationTask) {
	s.mu.Lock()
	s.operations = append([]operationTask{created}, s.operations...)
	if len(s.operations) > 40 {
		s.operations = s.operations[:40]
	}
	s.mu.Unlock()
	s.publishRobotEvent(robotEvent{Type: "task", TaskID: created.ID, Task: &created})
}

// startTestSandbox launches the isolated testone sandbox: it probes a free
// loopback port, builds a temporary no-login config (via CFG_PATH) that also
// carries that port, and tracks the process separately from the robot's
// original dev/app process. Closing the test window stops it.
// watchPortReadiness keeps the browser from polling a port while a dev/app
// process boots. It emits one terminal readiness event for the task; the
// process lifecycle itself remains represented by ordinary task events. kind
// selects the port predicate and event names: "app" watches serverPort and
// emits app-ready/app-failed, "test" watches the robot's top-level CBP port
// and emits test-ready/test-failed.
func (s *server) watchPortReadiness(id, root, kind string) {
	// A plain manual dev run has no declared readiness target. In particular,
	// it must not turn a missing serverPort into a probe of a shared default.
	if kind != "app" && kind != "test" {
		return
	}
	readyType, failedType := "app-ready", "app-failed"
	probe := s.robots.AppPortReachable
	if kind == "test" {
		readyType, failedType = "test-ready", "test-failed"
		probe = s.robots.TestPortReachable
	}
	timeout := time.NewTimer(40 * time.Second)
	defer timeout.Stop()
	// Port checks are a server-side startup predicate, not a browser polling
	// loop. Probe immediately and then back off so a slow dev server does not
	// create two fixed requests per second for its entire boot window.
	delay := 100 * time.Millisecond
	for {
		reachable, _, err := probe(root)
		if err == nil && reachable {
			s.publishRobotEvent(robotEvent{Type: readyType, TaskID: id})
			return
		}
		if !s.developmentRunning(root) {
			message := "进程在端口就绪前退出。"
			if err != nil {
				message = "端口检测失败：" + err.Error()
			}
			s.publishRobotEvent(robotEvent{Type: failedType, TaskID: id, Text: message})
			return
		}
		wait := time.NewTimer(delay)
		select {
		case <-timeout.C:
			wait.Stop()
			s.publishRobotEvent(robotEvent{Type: failedType, TaskID: id, Text: "服务启动超时，端口未响应。"})
			return
		case <-wait.C:
			if delay < 2*time.Second {
				delay *= 2
				if delay > 2*time.Second {
					delay = 2 * time.Second
				}
			}
		}
	}
}

func (s *server) operationOutput(root, action string) (output, status, runError string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.operations {
		if item.Root == root && item.Action == action {
			return item.Output, item.Status, item.Error
		}
	}
	return "", "", ""
}

// runtimeProcessOutput returns the dev/app process output for a root that the
// terminal should show: the currently running process if there is one,
// otherwise the most recent run (operations are kept newest-first).
func (s *server) runtimeProcessOutput(root string) (output, status, runError, mode string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latestOutput, latestStatus, latestError, latestMode string
	for _, item := range s.operations {
		if item.Root != root || (item.Action != "dev" && item.Action != "app") {
			continue
		}
		if latestOutput == "" {
			latestOutput, latestStatus, latestError = item.Output, item.Status, item.Error
			latestMode = map[string]string{"dev": "开发模式", "app": "前台运行"}[item.Action]
		}
		if item.Status == "running" {
			return item.Output, item.Status, item.Error, map[string]string{"dev": "开发模式", "app": "前台运行"}[item.Action]
		}
	}
	return latestOutput, latestStatus, latestError, latestMode
}

// pm2StatusFor returns the PM2 state, or an error after a short window. A
// bounded read keeps a broken pm2 install (which would otherwise trigger a
// package-manager download) from holding up a local start request for minutes.
func (s *server) pm2StatusFor(root string) (robot.PM2Status, error) {
	if s.pm2Status != nil {
		return s.pm2Status(root)
	}
	return s.pm2StatusWithin(root, 3*time.Second)
}

func (s *server) pm2StatusWithin(root string, window time.Duration) (robot.PM2Status, error) {
	done := make(chan pm2StatusResult, 1)
	go func() {
		status, err := s.robots.PM2Status(root)
		done <- pm2StatusResult{status: status, err: err}
	}()
	select {
	case result := <-done:
		return result.status, result.err
	case <-time.After(window):
		// The goroutine may still be reading; it only queries PM2 and never
		// mutates server state, so it is safe to abandon.
		return robot.PM2Status{}, fmt.Errorf("读取 PM2 状态超时")
	}
}

// localStartBlockedByPM2 reports whether a local (dev/app) process must not be
// started for a root. This is strict: when PM2 is configured but its state
// cannot be read, we block rather than risk a port clash with a running PM2
// service. Only a PM2Status that definitively reports not running allows a
// local start.
func (s *server) localStartBlockedByPM2(root string) (string, bool) {
	status, err := s.pm2StatusFor(root)
	if err != nil {
		return "无法读取后台（PM2）服务状态；为免端口冲突，请先在“后台运行”中确认服务已停止，再启动本机进程。", true
	}
	if status.Running {
		return "当前目录正在后台（PM2）运行；请先在“后台运行”中停止服务，再启动本机进程。", true
	}
	return "", false
}

// pm2StartBlockedByLocal reports whether a local process is running and
// therefore blocks starting the background PM2 service for the same root.
func (s *server) pm2StartBlockedByLocal(root string) (string, bool) {
	if !s.developmentRunning(root) {
		return "", false
	}
	return "当前目录正在本机（开发/前台）运行；请先停止本机进程，再启动后台服务。", true
}

// stopPM2ForStart stops the background PM2 service for a root so a new local
// process can take over the app port ("last one to start wins"). It tolerates
// a PM2 state read failure: if the config exists we attempt the stop anyway.
func (s *server) stopPM2ForStart(root string) error {
	status, err := s.pm2StatusFor(root)
	if err == nil && !status.Running {
		return nil
	}
	// pm2-delete removes the process entirely, which is more reliable than a
	// graceful stop for guaranteeing the port is released before a local start.
	_, stopErr := s.robots.Run(root, "pm2-delete", "", "", "", "", "", true)
	return stopErr
}

// stopLocalForStart stops a supervised local (dev/app) process so a new
// background PM2 service can take over the app port.
func (s *server) stopLocalForStart(root string) error {
	if !s.developmentRunning(root) {
		return nil
	}
	s.stopDevelopment(root, "本机进程")
	return nil
}

// robotStartPortBlockers proactively sniffs the ports the robot will bind
// before a dev/app/PM2 process is launched. A port held by a process that does
// not belong to this robot directory is reported immediately, instead of the
// start completing and the occupancy only showing up in the process logs.
func (s *server) robotStartPortBlockers(root, readyKind string) []string {
	if readyKind == "" {
		return nil
	}
	var ports []robot.RobotPort
	if readyKind == "app" {
		info, err := s.robots.AppPort(root)
		if err != nil || !info.Configured {
			return nil
		}
		ports = []robot.RobotPort{{Port: info.Port, Label: "应用端口"}}
	} else {
		info, err := s.robots.TestPort(root)
		if err != nil || !info.Configured {
			return nil
		}
		ports = []robot.RobotPort{{Port: info.Port, Label: "服务端口"}}
	}
	var blockers []string
	seen := map[int]bool{}
	for _, info := range ports {
		if seen[info.Port] {
			continue
		}
		seen[info.Port] = true
		occupied, occupants := sniffPort(info.Port)
		if !occupied {
			continue
		}
		owned := false
		var first portOccupant
		for _, occupant := range occupants {
			if first.PID == 0 {
				first = occupant
			}
			if s.portOccupantOwnedByRoot(root, occupant) {
				owned = true
				break
			}
		}
		if owned {
			continue
		}
		label := info.Label
		if label == "" {
			label = "端口"
		}
		if first.PID > 0 {
			description := first.Process
			if description == "" {
				description = "未知进程"
			}
			blockers = append(blockers, fmt.Sprintf("%s %d 已被其他进程占用（PID %d：%s）。", label, info.Port, first.PID, description))
		} else {
			blockers = append(blockers, fmt.Sprintf("%s %d 已被其他进程占用。", label, info.Port))
		}
	}
	return blockers
}

// portOccupantOwnedByRoot reports whether the process holding a port belongs
// to this robot directory. Recognised owners are processes whose working
// directory is the project, a supervised dev/app process group, or a persisted
// stray process from an earlier run. The
// "谁最后启动谁为准" flow is allowed to stop those; everything else is a
// genuine clash that must be reported before starting.
func (s *server) portOccupantOwnedByRoot(root string, occupant portOccupant) bool {
	if occupant.CWD != "" {
		cwd := filepath.Clean(occupant.CWD)
		cleanedRoot := filepath.Clean(root)
		if cwd == cleanedRoot || strings.HasPrefix(cwd, cleanedRoot+string(filepath.Separator)) {
			return true
		}
	}
	if occupant.PID > 0 {
		pgid := processPGID(occupant.PID)
		s.mu.RLock()
		dev := s.development[root]
		s.mu.RUnlock()
		if dev.Command != nil && dev.Command.Process != nil {
			if processDescendsFrom(occupant.PID, dev.Command.Process.Pid) || (dev.PGID > 0 && pgid == dev.PGID) {
				return true
			}
		}
		for _, marker := range loadPersistedProcesses() {
			if marker.Root == root && marker.PGID > 0 && (marker.PGID == occupant.PID || marker.PGID == pgid) {
				return true
			}
		}
	}
	return false
}

// waitPortFreeOn polls a loopback port until the previous process has released
// it. It retries for ~12 seconds so a graceful stop that needs its 3-second
// force-stop fallback has time to finish.
func (s *server) waitPortFreeOn(port int, attempts int) bool {
	if attempts <= 0 {
		attempts = 30
	}
	for i := 0; i < attempts; i++ {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
		if err != nil {
			return true // connection refused => no listener
		}
		_ = connection.Close()
		time.Sleep(400 * time.Millisecond)
	}
	return false
}

// waitPortFree polls the configured application port until it is released.
func (s *server) waitPortFree(root string, attempts int) bool {
	info, err := s.robots.AppPort(root)
	if err != nil {
		return true
	}
	return s.waitPortFreeOn(info.Port, attempts)
}

// forceFreePortOn kills whatever is listening on a given loopback port. This is
// the final "谁最后启动谁为准" fallback: the old process (PM2, a supervised
// dev/app, or a stray node) could not release the port gracefully, so we
// identify the listener and terminate it. Only the robot's configured
// serverPort or test port is targeted, never an arbitrary port.
func (s *server) forceFreePortOn(port int) error {
	output, err := s.runCommandForPort("lsof", "-ti", "tcp:"+strconv.Itoa(port))
	if err != nil || strings.TrimSpace(output) == "" {
		return fmt.Errorf("未找到占用端口 %d 的进程", port)
	}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		pid := strings.TrimSpace(line)
		if pid == "" || !isNumeric(pid) {
			continue
		}
		_, _ = s.runCommandForPort("kill", "-9", pid)
	}
	// Re-check after killing.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for i := 0; i < 10; i++ {
		response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port))
		if err != nil {
			return nil
		}
		_ = response.Body.Close()
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("端口 %d 仍被占用", port)
}

// forceFreePort kills whatever is listening on the configured application port.
// This is the final "谁最后启动谁为准" fallback: the old process (PM2, a
// supervised dev/app, or a stray node) could not release the port gracefully,
// so we identify the listener and terminate it. Only the robot's configured
// serverPort is targeted, never an arbitrary port.
func (s *server) forceFreePort(root string) error {
	info, err := s.robots.AppPort(root)
	if err != nil {
		return err
	}
	return s.forceFreePortOn(info.Port)
}

// runCommandForPort runs a small helper command without a managed process
// group; used only by forceFreePort.
func (s *server) runCommandForPort(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.Output()
	return string(output), err
}

func isNumeric(value string) bool {
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return value != ""
}

func (s *server) developmentRunning(root string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, running := s.development[root]
	return running
}

func (s *server) registerDevelopment(root, taskID, action string, command *exec.Cmd, pgid int, cleanup func()) bool {
	s.mu.Lock()
	if _, running := s.development[root]; running {
		s.mu.Unlock()
		return false
	}
	s.development[root] = developmentProcess{TaskID: taskID, Command: command, PGID: pgid, Cleanup: cleanup}
	s.mu.Unlock()
	s.recordProcess(root, taskID, pgid, action)
	return true
}

// recordProcess appends a persisted marker for a supervised robot process so a
// stray node can be located and killed even after an alx restart.
func (s *server) recordProcess(root, taskID string, pgid int, action string) {
	if pgid <= 0 {
		return
	}
	s.mu.Lock()
	items := loadPersistedProcesses()
	items = append(items, persistedProcess{Root: root, TaskID: taskID, PGID: pgid, Action: action, StartedAt: time.Now()})
	savePersistedProcesses(items)
	s.mu.Unlock()
}

// forgetProcess removes the persisted marker for a process that has exited.
func (s *server) forgetProcess(root, taskID string) {
	s.mu.Lock()
	items := loadPersistedProcesses()
	kept := items[:0]
	for _, item := range items {
		if item.Root == root && item.TaskID == taskID {
			continue
		}
		kept = append(kept, item)
	}
	savePersistedProcesses(kept)
	s.mu.Unlock()
}

// stopDevelopment requests a graceful stop of the supervised process and
// reports whether a process was actually running. A false result lets the
// caller finish its stop task immediately instead of waiting for an exit that
// already happened.
func (s *server) stopDevelopment(root, mode string) bool {
	s.mu.Lock()
	process, running := s.development[root]
	if !running {
		s.mu.Unlock()
		return false
	}
	s.stopping[root] = true
	s.mu.Unlock()
	s.appendOperationOutput(process.TaskID, "正在请求停止"+mode+"…\n")
	if err := interruptManagedProcess(process.Command); err != nil {
		// Do not abandon a stop merely because the graceful signal raced with a
		// child exit or the package manager rejected it. The force-stop pass
		// below still targets the complete process group.
		s.appendOperationOutput(process.TaskID, "优雅停止未确认，继续执行强制停止兜底。\n")
	}
	// Yarn and similar package managers can leave a child process alive after
	// their own interrupt. Give the group a brief graceful window, then stop
	// every member rather than only its package-manager parent.
	time.AfterFunc(3*time.Second, func() {
		s.mu.RLock()
		current, active := s.development[root]
		stopping := s.stopping[root]
		s.mu.RUnlock()
		if active && stopping && current.TaskID == process.TaskID {
			s.appendOperationOutput(process.TaskID, "进程仍未退出，正在强制停止全部子进程…\n")
			_ = forceStopManagedProcess(current.Command)
		}
	})
	return true
}

type operationOutputBuffer struct {
	text      strings.Builder
	timer     *time.Timer
	truncated bool
}

// appendOperationOutput coalesces process output into one durable SSE event per
// task per 100ms. This keeps fast stdout/stderr from turning into many SQLite
// transactions while preserving terminal ordering.
func (s *server) appendOperationOutput(id, output string) {
	if output == "" {
		return
	}
	const maxBatch = 32 * 1024
	s.mu.Lock()
	if s.outputBuffers == nil {
		s.outputBuffers = map[string]*operationOutputBuffer{}
	}
	buffer := s.outputBuffers[id]
	if buffer == nil {
		buffer = &operationOutputBuffer{}
		s.outputBuffers[id] = buffer
	}
	if buffer.text.Len()+len(output) > maxBatch && buffer.text.Len() > 0 {
		s.mu.Unlock()
		s.flushOperationOutput(id)
		s.appendOperationOutput(id, output)
		return
	}
	if len(output) > maxBatch {
		output = output[len(output)-maxBatch:]
		buffer.truncated = true
	}
	buffer.text.WriteString(output)
	if buffer.timer == nil {
		buffer.timer = time.AfterFunc(100*time.Millisecond, func() { s.flushOperationOutput(id) })
	}
	legacy := s.eventGateway == nil
	s.mu.Unlock()
	if legacy {
		s.flushOperationOutput(id)
	}
}

func (s *server) flushOperationOutput(id string) {
	s.mu.Lock()
	buffer := s.outputBuffers[id]
	if buffer == nil || buffer.text.Len() == 0 {
		s.mu.Unlock()
		return
	}
	text, truncated := buffer.text.String(), buffer.truncated
	if buffer.timer != nil {
		buffer.timer.Stop()
	}
	delete(s.outputBuffers, id)
	const maxOutput = 256 * 1024
	updated := false
	for index := range s.operations {
		if s.operations[index].ID != id {
			continue
		}
		s.operations[index].Output += text
		if len(s.operations[index].Output) > maxOutput {
			s.operations[index].Output = "…前面的输出已省略…\n" + s.operations[index].Output[len(s.operations[index].Output)-maxOutput:]
			truncated = true
		}
		updated = true
		break
	}
	s.mu.Unlock()
	if updated {
		s.publishRobotEvent(robotEvent{Type: "output", TaskID: id, Text: text, Truncated: truncated})
	}
}

// operationWriter forwards a supervised process's output into its operation
// record. It buffers partial lines so a chunked write that splits a "\n" is
// not rendered as two fragments, and it is safe for concurrent writes from the
// separate stdout/stderr copy goroutines.
type operationWriter struct {
	id      string
	server  *server
	buffer  []byte
	writeMu sync.Mutex
}

func newOperationWriter(id string, s *server) *operationWriter {
	return &operationWriter{id: id, server: s}
}

func (w *operationWriter) Write(data []byte) (int, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	written := len(data)
	w.buffer = append(w.buffer, data...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			break
		}
		w.server.appendOperationOutput(w.id, string(w.buffer[:index+1]))
		w.buffer = w.buffer[index+1:]
	}
	return written, nil
}

func (s *server) watchDevelopmentTask(id, root, action string, command *exec.Cmd) {
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	err, waitTimedOut := s.waitForManagedProcessExit(root, waitResult)
	s.flushOperationOutput(id)
	if waitTimedOut {
		// The direct child may have exited while a descendant kept the output
		// pipe open, so command.Wait() never returned. Try one more tree kill
		// and then finalize the stop task; leaving the UI on "正在停止" forever
		// is worse than reporting the stop as done with a note.
		s.mu.RLock()
		current, active := s.development[root]
		s.mu.RUnlock()
		if active && current.TaskID == id {
			_ = forceStopManagedProcess(current.Command)
		}
		s.appendOperationOutput(id, "等待进程退出超时，已结束停止状态。\n")
	}

	finished := time.Now()
	s.mu.Lock()
	stopped := s.stopping[root]
	wasManaged := false
	var processCleanup func()
	if current, active := s.development[root]; active && current.TaskID == id {
		delete(s.development, root)
		delete(s.stopping, root)
		wasManaged = true
		processCleanup = current.Cleanup
	}
	// A pending stop task only becomes "completed" once the process has really
	// exited, which is exactly the moment we reach here.
	s.completePendingStopTasks(root, finished)
	var snapshot operationTask
	for index := range s.operations {
		if s.operations[index].ID != id {
			continue
		}
		s.operations[index].FinishedAt = &finished
		processName := map[string]string{"app": "前台进程", "dev": "开发进程"}[action]
		if processName == "" {
			processName = "托管进程"
		}
		if err != nil && !stopped {
			s.operations[index].Status = "failed"
			s.operations[index].Error = processName + "已退出：" + err.Error()
			log.Printf("[ROBOT %s] %s退出 error=%s", id, processName, err)
		} else if stopped {
			s.operations[index].Status = "completed"
			s.operations[index].Output += processName + "已停止。\n"
			log.Printf("[ROBOT %s] %s已停止", id, processName)
		} else {
			s.operations[index].Status = "completed"
			s.operations[index].Output += processName + "已正常退出。\n"
			log.Printf("[ROBOT %s] %s正常退出", id, processName)
		}
		snapshot = s.operations[index]
		break
	}
	s.mu.Unlock()
	if processCleanup != nil {
		processCleanup()
	}
	if wasManaged {
		s.forgetProcess(root, id)
	}
	if snapshot.ID != "" {
		s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
	}
}

// managedProcessStopTimeout bounds how long a stop task may wait for the
// supervised process to exit. A graceful signal plus the force-stop fallback
// normally finish in seconds; the bound exists so a Windows descendant that
// inherited the output pipe can never leave the UI stuck on "正在停止" forever.
// managedProcessStopTimeout is a variable so tests can shorten the bound.
var managedProcessStopTimeout = 10 * time.Second

// waitForManagedProcessExit waits for the supervised process to exit, but
// starts a deadline as soon as a stop was requested. When the deadline passes
// the caller finalizes the task instead of blocking on command.Wait() forever.
func (s *server) waitForManagedProcessExit(root string, waitResult <-chan error) (error, bool) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	var stopDeadline time.Time
	for {
		select {
		case err := <-waitResult:
			return err, false
		case <-ticker.C:
			s.mu.RLock()
			stopping := s.stopping[root]
			s.mu.RUnlock()
			if !stopping {
				continue
			}
			if stopDeadline.IsZero() {
				stopDeadline = time.Now().Add(managedProcessStopTimeout)
			} else if time.Now().After(stopDeadline) {
				return errors.New("等待进程退出超时"), true
			}
		}
	}
}

// completePendingStopTasks marks every in-flight dev-stop/app-stop task for a
// root as completed once the supervised process has really exited.
func (s *server) completePendingStopTasks(root string, finished time.Time) {
	for index := range s.operations {
		item := &s.operations[index]
		if item.Root != root || item.Status != "running" {
			continue
		}
		if item.Action != "dev-stop" && item.Action != "app-stop" {
			continue
		}
		item.Status = "completed"
		item.FinishedAt = &finished
		item.Output = "已停止" + map[string]string{"dev-stop": "开发模式", "app-stop": "前台运行"}[item.Action] + "。"
	}
}

// reconcileRecoveredOperations finalizes operations that were persisted as
// running before this workbench instance started. Their owning goroutine has
// gone away, so keeping them running would permanently reuse a dead task for
// later identical setup-plugin requests.
func (s *server) reconcileRecoveredOperations() {
	s.settleUnmanagedLocalOperations("", "工作台重启后，本机运行已结束。")
	s.settleRecoveredSetupPluginOperations()
}

// settleRecoveredSetupPluginOperations marks interrupted setup-plugin runs as
// failed. Plugin runners are short-lived child processes and do not support
// cross-restart resumption, unlike a persisted task snapshot.
func (s *server) settleRecoveredSetupPluginOperations() {
	const message = "工作台重启，未完成的插件操作已中止；请重新执行。"
	finished := time.Now()
	s.mu.Lock()
	updated := make([]operationTask, 0, 2)
	for index := range s.operations {
		item := &s.operations[index]
		if item.Status != "running" || !strings.HasPrefix(item.Action, "setup:") {
			continue
		}
		item.Status = "failed"
		item.Error = message
		item.FinishedAt = &finished
		item.Output = strings.TrimSpace(item.Output+"\n"+message) + "\n"
		updated = append(updated, *item)
	}
	s.mu.Unlock()
	for index := range updated {
		snapshot := updated[index]
		s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
	}
}

// settleUnmanagedLocalOperations finalizes stale local lifecycle tasks. An
// empty root reconciles every project during startup; otherwise it applies to
// one project after a stop request finds no actual supervised process.
func (s *server) settleUnmanagedLocalOperations(root, message string) {
	finished := time.Now()
	s.mu.Lock()
	updated := make([]operationTask, 0, 2)
	for index := range s.operations {
		item := &s.operations[index]
		if item.Status != "running" || (root != "" && item.Root != root) {
			continue
		}
		switch item.Action {
		case "dev", "app":
			item.Status = "completed"
			item.FinishedAt = &finished
			item.Output = strings.TrimSpace(item.Output+"\n"+message) + "\n"
			updated = append(updated, *item)
		case "dev-stop", "app-stop":
			item.Status = "completed"
			item.FinishedAt = &finished
			item.Output = message
			updated = append(updated, *item)
		}
	}
	s.mu.Unlock()
	for index := range updated {
		snapshot := updated[index]
		s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
	}
}

func (s *server) robotPackagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	items, err := s.robots.LocalPackages(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// robotPackageUploadHandler unpacks a browser-uploaded plugin archive into the
// selected robot's packages directory. The destination name comes from the
// package manifest and an existing package with the same name is refused.
func (s *server) robotPackageUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30+1<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "上传格式无效。")
		return
	}
	defer r.MultipartForm.RemoveAll()
	root := strings.TrimSpace(r.FormValue("root"))
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "请上传一个插件包。")
		return
	}
	header := files[0]
	if !isUploadArchiveName(header.Filename) {
		writeError(w, http.StatusBadRequest, "仅支持 .zip、.tar.gz 或 .tgz 插件包。")
		return
	}
	temporary, err := os.CreateTemp("", "alx-package-upload-*.archive")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建上传临时文件。")
		return
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	input, err := header.Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, "无法读取上传文件。")
		return
	}
	_, copyErr := io.Copy(temporary, io.LimitReader(input, 2<<30+1))
	closeErr := temporary.Close()
	_ = input.Close()
	if copyErr != nil || closeErr != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "插件包过大。")
		return
	}
	item, err := s.robots.InstallLocalPackageUpload(root, temporary.Name())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) robotPackageVersionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	versions, err := s.robots.LocalPackageVersions(r.URL.Query().Get("root"), r.URL.Query().Get("package"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (s *server) robotPackageReadmeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	result, err := s.robots.LocalPackageReadme(r.URL.Query().Get("root"), r.URL.Query().Get("package"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) botAppPagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	items, err := s.robots.AppPages(r.URL.Query().Get("root"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) botAppPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	const prefix = "/api/v1/robot/webview/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "缺少机器人应用页标识。")
		return
	}
	// Treat the entry URL as a directory. Vite commonly emits ./assets/...;
	// without this redirect a caller omitting the final slash resolves those
	// files one level above the registered 应用页 id.
	if len(parts) == 2 && !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
		return
	}
	root, ok := decodeRobotRootToken(parts[0])
	if !ok {
		writeError(w, http.StatusBadRequest, "机器人目录标识无效。")
		return
	}
	requestPath := ""
	if len(parts) == 3 {
		requestPath = parts[2]
	}
	if strings.HasPrefix(requestPath, "api/") {
		s.proxyBotAppPageAPI(w, r, root, parts[1], strings.TrimPrefix(requestPath, "api/"))
		return
	}
	if requestPath == "message" {
		s.botAppPageMessageHandler(w, r, root, parts[1])
		return
	}
	if requestPath == "events" {
		s.botAppPageEventsHandler(w, r, root, parts[1])
		return
	}
	if requestPath == "events/stream" {
		s.botAppPageEventsStreamHandler(w, r, root, parts[1])
		return
	}
	if requestPath == "bridge.js" {
		entry, entryErr := s.robots.BotAppPage(root, parts[1])
		if entryErr != nil {
			writeError(w, http.StatusNotFound, entryErr.Error())
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = io.WriteString(w, botAppPageBridge(entry))
		return
	}
	file, err := s.robots.AppPageFile(root, parts[1], requestPath)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if filepath.Ext(file) == ".html" {
		entry, entryErr := s.robots.BotAppPage(root, parts[1])
		if entryErr != nil {
			writeError(w, http.StatusNotFound, entryErr.Error())
			return
		}
		if entry.RequiresServerPort {
			info, infoErr := s.robots.AppPort(root)
			if infoErr != nil {
				writeError(w, http.StatusBadRequest, infoErr.Error())
				return
			}
			if !info.Configured {
				writeError(w, http.StatusBadRequest, "该插件页面要求先配置应用端口（serverPort）。")
				return
			}
			reachable, _, probeErr := s.robots.AppPortReachable(root)
			if probeErr != nil {
				writeError(w, http.StatusBadRequest, probeErr.Error())
				return
			}
			if !reachable {
				writeError(w, http.StatusBadRequest, "该插件页面要求应用端口已启动（serverPort 暂不可达）。")
				return
			}
		}
	}
	// Each 应用页 is opened through a robot-specific *.localhost hostname.
	// It remains loopback-only while keeping plugin storage and cookies separate
	// from both the management UI and other robots. Plugin actions can only use
	// the narrowly scoped 应用页 API proxy below.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A robot plugin UI may talk only to its registered local bot API proxy.
	// Do not permit the parent management origin in connect-src.
	w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; connect-src 'self' ws: wss:; img-src 'self' data: blob: https: http:; style-src 'self' 'unsafe-inline'; frame-ancestors http://localhost:* http://127.0.0.1:*; base-uri 'none'")
	// Vite's default production output commonly uses root-absolute local asset
	// URLs. Inside a plugin mount those would otherwise point at setup's own
	// assets, so make the common HTML, favicon and CSS bundle forms relative.
	// Ordinary external URLs remain untouched.
	if filepath.Ext(file) == ".html" {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			writeError(w, http.StatusNotFound, "插件 Web 页面不存在。")
			return
		}
		content := rewriteBotAppPageHTML(string(data))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, content)
		return
	}
	http.ServeFile(w, r, file)
}

// rewriteBotAppPageHTML keeps a plugin's common Vite assets inside its mounted
// 应用页 route and injects only the restricted setup bridge.
func rewriteBotAppPageHTML(content string) string {
	content = strings.NewReplacer(
		`src="/assets/`, `src="assets/`,
		`href="/assets/`, `href="assets/`,
		`src='/assets/`, `src='assets/`,
		`href='/assets/`, `href='assets/`,
		`href="/favicon.ico"`, `href="favicon.ico"`,
		`href='/favicon.ico'`, `href='favicon.ico'`,
		`url(/assets/`, `url(assets/`,
		`url('/assets/`, `url('assets/`,
		`url("/assets/`, `url("assets/`,
	).Replace(content)
	theme := alemonjsThemeStyleTag + `<script src="bridge.js"></script>`
	if strings.Contains(content, "</head>") {
		return strings.Replace(content, "</head>", theme+"</head>", 1)
	}
	return theme + content
}

// botAppPageBridge intentionally exposes a compatibility subset, never Wails or
// setup process privileges. The package metadata is JSON-quoted to keep a
// malformed manifest from becoming executable JavaScript.
func botAppPageBridge(entry robot.BotAppPage) string {
	bridge := `(function(){var listeners=[],lastAPIError='';function emit(value){listeners.slice().forEach(function(listener){try{listener(value)}catch(_){}})}function send(type,value){parent.postMessage({source:'alx-webview',type:type,value:value},'*')}function reportAPIError(status,message){var key=String(status||0)+'/'+String(message||'');if(lastAPIError===key)return;lastAPIError=key;send('api-error',{status:status||0,message:message||''});window.setTimeout(function(){lastAPIError=''},5000)}function responseError(response){response.clone().json().then(function(payload){reportAPIError(response.status,payload&&typeof payload.error==='string'?payload.error:'')}).catch(function(){reportAPIError(response.status,'')})}function isPluginAPI(input){try{var url=new URL(typeof input==='string'?input:input.url,location.href);return /\/api\//.test(url.pathname)}catch(_){return false}}var nativeFetch=window.fetch;window.fetch=function(input,init){return nativeFetch.apply(this,arguments).then(function(response){if(isPluginAPI(input)&&!response.ok)responseError(response);return response})};var NativeXHR=window.XMLHttpRequest;function TrackedXHR(){var xhr=new NativeXHR(),url='';var open=xhr.open;xhr.open=function(method,nextURL){url=nextURL;return open.apply(xhr,arguments)};xhr.addEventListener('loadend',function(){if(isPluginAPI(url)&&xhr.status>=400){var message='';try{var body=JSON.parse(xhr.responseText);message=typeof body.error==='string'?body.error:''}catch(_){}reportAPIError(xhr.status,message)}});return xhr}TrackedXHR.prototype=NativeXHR.prototype;window.XMLHttpRequest=TrackedXHR;function post(value){send('message',value);return nativeFetch('message',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({value:value})}).then(function(response){if(!response.ok)throw new Error('插件 desk 通信不可用')})}function connect(){var source=new EventSource('events/stream');source.onmessage=function(event){try{emit(JSON.parse(event.data))}catch(_){}}}var api=Object.freeze({context:Object.freeze({package:` + strconv.Quote(entry.Package) + `,name:` + strconv.Quote(entry.Name) + `}),postMessage:post,onMessage:function(listener){if(typeof listener!=='function')return function(){};listeners.push(listener);return function(){listeners=listeners.filter(function(item){return item!==listener})}},request:function(path,options){if(typeof path!=='string'||!/^\.\/api\//.test(path))return Promise.reject(new Error('只允许请求插件 ./api/ 路径'));return window.fetch(path,options)}});window.__alxWebview=api;window.appDesktopAPI=Object.freeze({postMessage:api.postMessage,onMessage:api.onMessage,themeVariables:function(){return getComputedStyle(document.documentElement)},themeOn:function(listener){return api.onMessage(function(value){if(value&&value.type==='theme')listener(value.data)})}});window.addEventListener('message',function(event){var data=event.data;if(data&&data.source==='alx-parent'){emit(data.value)}});connect();send('ready',{package:api.context.package,name:api.context.name});})();`
	return bridge + "/* 'events' */"
}

// proxyBotAppPageAPI connects an 应用页's relative ./api/* requests to the
// selected robot application. The destination is never supplied by the
// browser: it is derived from the selected root's configured local app port.
// robotAppHandler reverse-proxies the robot's application service (the server
// on serverPort) so its launchpad and plugin pages render inside the alx page
// instead of relying on the old 应用页 mechanism. All paths under /app/ map to
// http://127.0.0.1:<port>/..., preserving static assets and API routes.
//
// The robot root is carried in the path (a base64url token as the first path
// segment) rather than a query parameter, so every in-app navigation — links,
// relative assets, redirects and API calls — inherits the root automatically
// and never needs to re-send it. Legacy ?root= URLs are still accepted.
//
// The app is written against its own origin root, so its documents are adjusted
// on the way through: a <base href> is injected for relative assets/APIs, and
// root-relative links (/app/, /apps/x/) and redirects are re-prefixed with the
// proxy mount so navigation stays inside the iframe instead of escaping to the
// management page origin.
func (s *server) robotAppHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	const mount = "/api/v1/robot/app/"
	root, token := robotAppRootFromPath(r.URL.Path, mount)
	if root == "" {
		root = r.URL.Query().Get("root")
	}
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	if token == "" {
		token = robotAppToken(root)
	}
	info, err := s.robots.AppPort(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !info.Configured {
		writeError(w, http.StatusConflict, "请先为当前机器人配置应用端口（serverPort）。")
		return
	}
	target, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(info.Port))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "应用地址无效。")
		return
	}
	// appPrefix is the mount plus the root token, e.g. /api/v1/robot/app/<token>/
	// Every rewritten link, base href and redirect targets this prefix so the
	// root travels inside the URL path.
	appPrefix := mount + token + "/"
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			// Strip the mount and the root token, keeping the app's own path.
			rest := strings.TrimPrefix(r.URL.Path, mount)
			path := "/" + rest
			if rest == token || strings.HasPrefix(rest, token+"/") {
				path = strings.TrimPrefix(rest, token)
				if path == "" {
					path = "/"
				}
			}
			request.URL.Path = path
			request.URL.RawPath = ""
			request.Host = target.Host
		},
		ModifyResponse: func(response *http.Response) error {
			modifyRobotAppResponse(response, target, appPrefix, r.URL.Path, r)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeError(w, http.StatusBadGateway, "机器人应用尚未启动或无法连接。请在“运行”中启动开发模式后重试。")
		},
	}
	proxy.ServeHTTP(w, r)
}

// robotTestHandler reverse-proxies the robot's CBP/sandbox WebSocket service
// (the testone endpoint on the robot's top-level port) so the migrated test
// center connects to the workbench origin instead of opening a different port
// from the browser. Path shape: /api/v1/robot/test/<token>/testone. The robot
// root travels in the path (base64url token) just like the app proxy; the
// upstream is always the configured test port, never a browser-supplied value.
func (s *server) robotTestHandler(w http.ResponseWriter, r *http.Request) {
	const mount = "/api/v1/robot/test/"
	root, _ := robotAppRootFromPath(r.URL.Path, mount)
	if root == "" {
		root = r.URL.Query().Get("root")
	}
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	info, err := s.robots.TestPort(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !info.Configured {
		writeError(w, http.StatusConflict, "请先为当前机器人配置服务端口（port）。")
		return
	}
	s.proxyRobotTest(w, r, root, info.Port)
}

// robotLiveHandler connects the workbench to the robot's normal CBP server as
// a full-receive client. Unlike /testone this is not a sandbox endpoint: the
// platform adapter remains logged in and CBP routes action/API requests to it.
// The browser cannot choose the CBP role or full-receive privilege: the local
// proxy injects those headers. It supplies a validated device id only so CBP
// can correlate this browser's action requests and responses; the framework,
// rather than a platform-specific UI, remains the protocol authority.
func (s *server) robotLiveHandler(w http.ResponseWriter, r *http.Request) {
	const mount = "/api/v1/robot/live/"
	root, _ := robotAppRootFromPath(r.URL.Path, mount)
	if root == "" {
		root = r.URL.Query().Get("root")
	}
	if root == "" {
		writeError(w, http.StatusBadRequest, "请选择机器人目录。")
		return
	}
	// Existing robot processes were started before the workbench feature was
	// added. Make the bridge available here too; they need one restart to load
	// it, but new starts are patched before their platform child is forked.
	if _, err := robot.EnsureCBPIPCActionBridge(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := s.robots.TestPort(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !info.Configured {
		writeError(w, http.StatusConflict, "请先为当前机器人配置 CBP 端口（port）。")
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("deviceId"))
	if !regexp.MustCompile(`^alemonx-live-[A-Za-z0-9-]{8,96}$`).MatchString(deviceID) {
		writeError(w, http.StatusBadRequest, "在线聊天连接标识无效。")
		return
	}
	s.proxyRobotLive(w, r, info.Port, deviceID)
}

const (
	liveUploadMaxBytes = 100 << 20 // QQ Bot itself rejects media over 100 MiB.
	liveUploadTTL      = 10 * time.Minute
)

var liveDeviceIDPattern = regexp.MustCompile(`^alemonx-live-[A-Za-z0-9-]{8,96}$`)

// robotLiveUploadHandler accepts only one browser-selected file for an active
// robot chat session. Files are stored in the selected robot directory so the
// locally running qq-bot process can consume the returned absolute path. The
// matching DELETE is issued when the CBP action resolves; TTL cleanup prevents
// a browser crash from accumulating files.
func (s *server) robotLiveUploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.robotLiveUploadCreate(w, r)
	case http.MethodDelete:
		s.robotLiveUploadDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
	}
}

func validLiveDeviceID(value string) bool {
	return liveDeviceIDPattern.MatchString(strings.TrimSpace(value))
}

func (s *server) robotLiveUploadCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, liveUploadMaxBytes+1<<20)
	if err := r.ParseMultipartForm(liveUploadMaxBytes + 1<<20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "文件超过 100 MiB 限制或上传格式无效。")
		return
	}
	root := strings.TrimSpace(r.FormValue("root"))
	deviceID := strings.TrimSpace(r.FormValue("deviceId"))
	if root == "" || !validLiveDeviceID(deviceID) {
		writeError(w, http.StatusBadRequest, "机器人目录或聊天连接标识无效。")
		return
	}
	validated, err := s.robots.Validate(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择要发送的文件。")
		return
	}
	defer source.Close()
	filename := filepath.Base(strings.TrimSpace(header.Filename))
	if filename == "." || filename == "" || len(filename) > 180 {
		writeError(w, http.StatusBadRequest, "文件名无效。")
		return
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建临时上传标识。")
		return
	}
	id := hex.EncodeToString(idBytes)
	directory := filepath.Join(validated.Path, ".alemonx-live-uploads")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "无法创建机器人临时文件目录。")
		return
	}
	path := filepath.Join(directory, id+"-"+filename)
	destination, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法暂存上传文件。")
		return
	}
	size, copyErr := io.Copy(destination, io.LimitReader(source, liveUploadMaxBytes+1))
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil || size > liveUploadMaxBytes {
		_ = os.Remove(path)
		if size > liveUploadMaxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "文件超过 100 MiB 限制。")
		} else {
			writeError(w, http.StatusInternalServerError, "文件暂存失败。")
		}
		return
	}
	if size == 0 {
		_ = os.Remove(path)
		writeError(w, http.StatusBadRequest, "文件为空，请重新选择后发送。")
		return
	}
	item := liveUpload{ID: id, Root: validated.Path, DeviceID: deviceID, Path: path, Filename: filename, Size: size, MIMEType: header.Header.Get("Content-Type"), ExpiresAt: time.Now().Add(liveUploadTTL)}
	s.liveUploadsMu.Lock()
	if s.liveUploads == nil {
		s.liveUploads = map[string]liveUpload{}
	}
	s.liveUploads[id] = item
	s.liveUploadsMu.Unlock()
	time.AfterFunc(liveUploadTTL, func() { s.removeLiveUpload(id) })
	writeJSON(w, http.StatusCreated, map[string]any{"uploadId": item.ID, "path": item.Path, "filename": item.Filename, "size": item.Size, "mimeType": item.MIMEType})
}

func (s *server) robotLiveUploadDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Root     string `json:"root"`
		DeviceID string `json:"deviceId"`
		UploadID string `json:"uploadId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&request); err != nil || !validLiveDeviceID(request.DeviceID) || strings.TrimSpace(request.UploadID) == "" {
		writeError(w, http.StatusBadRequest, "临时上传清理请求无效。")
		return
	}
	validated, err := s.robots.Validate(strings.TrimSpace(request.Root))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.liveUploadsMu.Lock()
	item, exists := s.liveUploads[request.UploadID]
	if exists && item.Root == validated.Path && item.DeviceID == request.DeviceID {
		delete(s.liveUploads, request.UploadID)
	}
	s.liveUploadsMu.Unlock()
	if !exists || item.Root != validated.Path || item.DeviceID != request.DeviceID {
		writeError(w, http.StatusNotFound, "临时上传不存在或不属于当前聊天会话。")
		return
	}
	_ = os.Remove(item.Path)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) removeLiveUpload(id string) {
	s.liveUploadsMu.Lock()
	item, exists := s.liveUploads[id]
	if exists {
		delete(s.liveUploads, id)
	}
	s.liveUploadsMu.Unlock()
	if exists {
		_ = os.Remove(item.Path)
	}
}

func (s *server) proxyRobotTest(w http.ResponseWriter, r *http.Request, root string, port int) {
	// The transport dials an HTTP request and completes the WebSocket upgrade
	// through the Upgrade headers, so the target scheme is http (same as the
	// app proxy). A "ws://" scheme here makes http.Transport fail the dial.
	target, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "测试服务地址无效。")
		return
	}
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			// The testone endpoint is fixed; strip the mount and token so the
			// upstream CBP server sees /testone for the WebSocket upgrade.
			request.URL.Path = "/testone"
			request.URL.RawPath = ""
			request.Host = target.Host
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeError(w, http.StatusBadGateway, "机器人测试服务尚未启动或无法连接。请先在“运行”中启动开发模式。")
		},
	}
	proxy.ServeHTTP(w, r)
}

// proxyRobotLive preserves CBP's WebSocket upgrade while adding the headers
// that identify this connection as a full-receive application client. Native
// browser WebSockets cannot set those headers themselves, which is precisely
// why this trusted local hop owns them.
func (s *server) proxyRobotLive(w http.ResponseWriter, r *http.Request, port int, deviceID string) {
	target, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "在线聊天服务地址无效。")
		return
	}
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			request.URL.Path = "/"
			request.URL.RawPath = ""
			request.Host = target.Host
			request.Header.Set("User-Agent", "client")
			request.Header.Set("X-Device-ID", deviceID)
			request.Header.Set("X-Full-Receive", "1")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeError(w, http.StatusBadGateway, "机器人 CBP 服务尚未启动或无法连接。请先启动已登录的机器人。")
		},
	}
	proxy.ServeHTTP(w, r)
}

// robotAppToken encodes a robot directory path for embedding in the proxy URL.
// It uses base64url without padding so the token is safe in a path segment.
func robotAppToken(root string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(root))
}

// decodeRobotRootToken decodes and validates the root embedded in every robot
// proxy route. Keeping this shared prevents app, test, live chat and 应用页
// routes from drifting into different platform-path behaviour.
func decodeRobotRootToken(token string) (string, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || !isAbsoluteRobotRoot(string(decoded)) {
		return "", false
	}
	return string(decoded), true
}

// robotAppRootFromPath extracts the root token from a proxied path's first
// segment, returning the decoded root and the raw token. The second return is
// empty when the path carries no valid token (legacy query-form URLs). Only
// absolute paths are accepted, so an arbitrary app route like "apps/demo"
// (whose first segment is coincidentally valid base64url) is never misread as
// a robot root.
func robotAppRootFromPath(path, mount string) (root, token string) {
	rest := strings.TrimPrefix(path, mount)
	segment, _, _ := strings.Cut(rest, "/")
	if segment == "" {
		return "", ""
	}
	root, ok := decodeRobotRootToken(segment)
	if !ok {
		return "", ""
	}
	return root, segment
}

// isAbsoluteRobotRoot accepts native absolute paths from every desktop client.
// filepath.IsAbs only recognises the current host's path syntax, while a
// browser on Windows encodes paths such as C:\\bots\\demo into the proxy token.
// Keeping the Windows forms explicit also lets the token parser be tested on
// non-Windows builders; the robot manager still validates the path before any
// project operation occurs.
func isAbsoluteRobotRoot(root string) bool {
	if filepath.IsAbs(root) {
		return true
	}
	if len(root) >= 3 && ((root[0] >= 'A' && root[0] <= 'Z') || (root[0] >= 'a' && root[0] <= 'z')) && root[1] == ':' && (root[2] == '\\' || root[2] == '/') {
		return true
	}
	return strings.HasPrefix(root, `\\`)
}

// modifyRobotAppResponse rewrites a proxied app response so absolute paths stay
// inside the proxy mount. Redirects are re-prefixed and HTML documents get a
// <base href> plus rewritten root-relative href/src/action references. Unmatched
// page navigations (upstream 404) are sent back to the launchpad instead of
// rendering stray content, so the app frame never nests the management page.
func modifyRobotAppResponse(response *http.Response, target *url.URL, appPrefix, requestPath string, request *http.Request) {
	if isRobotAppNavigation(request) && requestPath != appPrefix && response.StatusCode == http.StatusNotFound {
		response.Header.Set("Location", appPrefix)
		response.StatusCode = http.StatusFound
		response.Body = io.NopCloser(bytes.NewReader(nil))
		response.ContentLength = 0
		response.Header.Del("Content-Type")
		response.Header.Set("Content-Length", "0")
		return
	}
	if location := response.Header.Get("Location"); location != "" {
		if rewritten := rewriteRobotAppLocation(location, target, appPrefix); rewritten != "" {
			response.Header.Set("Location", rewritten)
		}
	}
	// Only document bodies need rewriting; assets and API payloads pass through.
	contentType := response.Header.Get("Content-Type")
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "text/html") {
		return
	}
	// Compressed bodies cannot be safely patched; leave them untouched.
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	_ = response.Body.Close()
	rewritten := rewriteRobotAppHTML(body, appPrefix, requestPath)
	response.Body = io.NopCloser(bytes.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
}

// isRobotAppNavigation reports whether a proxied request is a full-page HTML
// navigation (as opposed to a script/style/fetch request), so unmatched-route
// redirects never hijack asset or API responses.
func isRobotAppNavigation(request *http.Request) bool {
	return strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/html")
}

// rewriteRobotAppLocation re-prefixes a root-relative (or target-host) redirect
// Location so the browser follows it through the proxy instead of escaping to
// the management page origin. Absolute external URLs are left alone.
func rewriteRobotAppLocation(location string, target *url.URL, prefix string) string {
	if strings.HasPrefix(location, target.Scheme+"://"+target.Host) {
		if parsed, err := url.Parse(location); err == nil {
			return prefix + strings.TrimPrefix(parsed.RequestURI(), "/")
		}
		return ""
	}
	if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
		return prefix + strings.TrimPrefix(location, "/")
	}
	return ""
}

// rewriteRobotAppHTML injects a <base href> for the current document directory
// (so relative assets like ./assets/... and ./api/... resolve within the mount)
// and re-prefixes root-relative href/src/action references (like the launchpad's
// /app/ links) which ignore <base>.
func rewriteRobotAppHTML(body []byte, prefix, requestPath string) []byte {
	document := string(body)
	// Root-relative absolute paths (/app/, /apps/x/, /favicon.ico) bypass <base>.
	// Protocol-relative (//host) and scheme URLs must stay untouched. This runs
	// first so the injected <base href> (itself a root-relative path) is not
	// re-prefixed.
	document = rewriteRootRelativeAttr(document, prefix)
	injections := alemonjsThemeStyleTag + baseHrefFor(requestPath) + robotAppScrollbarStyle
	if head := regexp.MustCompile(`(?i)<head[^>]*>`).FindString(document); head != "" {
		document = strings.Replace(document, head, head+injections, 1)
	} else if htmlTag := regexp.MustCompile(`(?i)<html[^>]*>`).FindString(document); htmlTag != "" {
		document = strings.Replace(document, htmlTag, htmlTag+"<head>"+injections+"</head>", 1)
	} else {
		document = injections + document
	}
	return []byte(document)
}

// robotAppScrollbarStyle hides the app document's scrollbars (Chrome/Edge/Safari
// via ::-webkit-scrollbar, Firefox via scrollbar-width) so the embedded app does
// not show a second scrollbar beside the management page's own. Scrolling itself
// is preserved.
const robotAppScrollbarStyle = `<style>html,body{scrollbar-width:none;-ms-overflow-style:none}html::-webkit-scrollbar,body::-webkit-scrollbar{width:0;height:0;display:none}</style>`

// baseHrefFor returns the <base href> pointing at the current document's
// directory within the proxy mount, e.g. /api/v1/robot/app/apps/x/.
func baseHrefFor(requestPath string) string {
	directory := requestPath
	if index := strings.LastIndex(directory, "/"); index >= 0 {
		directory = directory[:index+1]
	}
	return `<base href="` + html.EscapeString(directory) + `">`
}

// rewriteRootRelativeAttr prefixes root-relative values of href/src/action
// attributes (and their single-quoted variants) with the proxy mount.
// Protocol-relative (//host) and scheme URLs are left untouched.
func rewriteRootRelativeAttr(document, prefix string) string {
	for _, attr := range []string{"href", "src", "action"} {
		for _, quote := range []string{`"`, `'`} {
			pattern := regexp.MustCompile(`(?i)(\b` + attr + `=` + quote + `)(/[^` + quote + `]*)` + quote)
			document = pattern.ReplaceAllStringFunc(document, func(match string) string {
				parts := pattern.FindStringSubmatch(match)
				if strings.HasPrefix(parts[2], "//") {
					return match
				}
				return parts[1] + prefix + strings.TrimPrefix(parts[2], "/") + quote
			})
		}
	}
	return document
}

func (s *server) proxyBotAppPageAPI(w http.ResponseWriter, r *http.Request, root, id, requestPath string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "插件 API 不支持此请求方式。")
		return
	}
	target, err := s.robots.AppPageAPIURL(root, id, requestPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "插件 API 请求无效。")
		return
	}
	for _, header := range []string{"Accept", "Content-Type"} {
		if value := r.Header.Get(header); value != "" {
			request.Header.Set(header, value)
		}
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "机器人应用尚未启动或无法连接。请在“运行”中启动开发模式后重试。")
		return
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *server) botAppPageMessageHandler(w http.ResponseWriter, r *http.Request, root, id string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "插件消息仅支持发送。")
		return
	}
	entry, err := s.robots.BotAppPage(root, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var input struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&input); err != nil || len(input.Value) == 0 {
		writeError(w, http.StatusBadRequest, "插件消息内容无效。")
		return
	}
	runtime, err := s.ensureBotAppPageRuntime(root)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	payload, _ := json.Marshal(map[string]json.RawMessage{"name": json.RawMessage(strconv.Quote(entry.Package)), "value": input.Value})
	message, _ := json.Marshal(map[string]any{"type": "webview-post-message", "data": string(payload), "__STDIN_JSON_DATA": true})
	if _, err := runtime.stdin.Write(append(message, '\n')); err != nil {
		writeError(w, http.StatusBadGateway, "插件 desk 进程无法接收消息。")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

// botAppPageEventsHandler is deliberately a short polling endpoint. Unlike a
// permanent SSE stream, it keeps hidden tabs from holding resources while the
// browser still receives every queued message from its directory's desk IPC.
func (s *server) botAppPageEventsHandler(w http.ResponseWriter, r *http.Request, root, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "插件消息仅支持读取。")
		return
	}
	if _, err := s.robots.BotAppPage(root, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.mu.Lock()
	runtime := s.botAppPageRuntimes[root]
	events := []json.RawMessage(nil)
	if runtime != nil {
		events = append(events, runtime.events[id]...)
		delete(runtime.events, id)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *server) botAppPageEventsStreamHandler(w http.ResponseWriter, r *http.Request, root, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "插件消息仅支持读取。")
		return
	}
	if _, err := s.robots.BotAppPage(root, id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "SSE 不受支持。")
		return
	}
	runtime, err := s.ensureBotAppPageRuntime(root)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	stream := make(chan json.RawMessage, 64)
	s.mu.Lock()
	if runtime.streams[id] == nil {
		runtime.streams[id] = map[chan json.RawMessage]struct{}{}
	}
	runtime.streams[id][stream] = struct{}{}
	queued := append([]json.RawMessage(nil), runtime.events[id]...)
	delete(runtime.events, id)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(runtime.streams[id], stream)
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(event json.RawMessage) error {
		if _, err := w.Write([]byte("data: " + string(event) + "\n\n")); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	for _, event := range queued {
		if err := write(event); err != nil {
			return
		}
	}
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-stream:
			if err := write(event); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) ensureBotAppPageRuntime(root string) (*botAppPageRuntime, error) {
	s.mu.RLock()
	runtime := s.botAppPageRuntimes[root]
	s.mu.RUnlock()
	if runtime != nil {
		return runtime, nil
	}
	// The caller already resolved a registered 应用页 entry for this root.
	// Convert only that validated root to an absolute process working directory.
	project, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	scriptPath := filepath.Join(project, "alemonjs", "desktop.js")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(scriptPath, []byte(defaultBotAppPageDesktopScript), 0644); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	node, lookupErr := system.ResolveCommand("node")
	if lookupErr != nil {
		return nil, fmt.Errorf("机器人应用页通信需要 Node.js，请先在环境管理中安装")
	}
	command := exec.Command(node, scriptPath)
	command.Dir = project
	if bin := system.ManagedNodeBin(); bin != "" {
		command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	robot.HideWindow(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("无法启动插件 desk 进程：%w", err)
	}
	runtime = &botAppPageRuntime{command: command, stdin: stdin, events: map[string][]json.RawMessage{}, streams: map[string]map[chan json.RawMessage]struct{}{}}
	s.mu.Lock()
	if existing := s.botAppPageRuntimes[root]; existing != nil {
		s.mu.Unlock()
		_ = command.Process.Kill()
		go func() { _ = command.Wait() }()
		return existing, nil
	}
	s.botAppPageRuntimes[root] = runtime
	s.mu.Unlock()
	go s.watchBotAppPageRuntime(root, runtime, stdout, stderr)
	return runtime, nil
}

func (s *server) watchBotAppPageRuntime(root string, runtime *botAppPageRuntime, stdout, stderr io.Reader) {
	read := func(stream io.Reader) {
		scanner := bufio.NewScanner(stream)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			s.queueBotAppPageEvent(root, scanner.Bytes())
		}
	}
	go read(stderr)
	read(stdout)
	_ = runtime.command.Wait()
	s.mu.Lock()
	if s.botAppPageRuntimes[root] == runtime {
		delete(s.botAppPageRuntimes, root)
	}
	s.mu.Unlock()
}

func (s *server) queueBotAppPageEvent(root string, line []byte) {
	var envelope struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(line, &envelope) != nil || !strings.HasPrefix(envelope.Type, "webview-") {
		return
	}
	var target struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(envelope.Data, &target) != nil || target.Name == "" {
		return
	}
	entries, err := s.robots.AppPages(root)
	if err != nil {
		return
	}
	event, _ := json.Marshal(map[string]json.RawMessage{"type": json.RawMessage(strconv.Quote(strings.TrimPrefix(envelope.Type, "webview-on-"))), "data": target.Value})
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.botAppPageRuntimes[root]
	if runtime == nil {
		return
	}
	for _, entry := range entries {
		if entry.Package == target.Name {
			runtime.events[entry.ID] = append(runtime.events[entry.ID], event)
			for stream := range runtime.streams[entry.ID] {
				select {
				case stream <- event:
				default:
					// A slow 应用页 will receive the queued snapshot after reconnecting.
				}
			}
		}
	}
}

const defaultBotAppPageDesktopScript = `import { events } from '@alemonjs/process'
const send = data => process.stdout.write(JSON.stringify({ type: data.type, data: data.data, from: 'nodejs', __STDIN_JSON_DATA: true }) + '\n')
global.wsprocess = global.wsprocess || {}; global.wsprocess.send = send
process.stdin.on('data', raw => { try { const data = JSON.parse(raw.toString().trim()); if (data?.__STDIN_JSON_DATA && data.type) events[data.type]?.(JSON.parse(data.data)) } catch {} })
`

func (s *server) robotManifestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		manifest, err := s.robots.PackageManifest(r.URL.Query().Get("root"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, manifest)
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root string `json:"root"`
		robot.PackageManifest
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := s.robots.SavePackageManifest(input.Root, input.PackageManifest)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotPackageConfigHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Root    string         `json:"root"`
		Package string         `json:"package"`
		Values  map[string]any `json:"values"`
	}
	if r.Method == http.MethodGet {
		input.Root = r.URL.Query().Get("root")
		input.Package = r.URL.Query().Get("package")
	} else if r.Method == http.MethodPut {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "请求内容无法识别。")
			return
		}
	} else {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if r.Method == http.MethodGet {
		var config robot.PackageConfig
		var err error
		if input.Package == "" {
			config, err = s.robots.CurrentPackageConfig(input.Root)
		} else {
			config, err = s.robots.PackageConfig(input.Root, input.Package)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, config)
		return
	}
	var result robot.Result
	var err error
	if input.Package == "" {
		result, err = s.robots.SaveCurrentPackageConfig(input.Root, input.Values)
	} else {
		result, err = s.robots.SavePackageConfig(input.Root, input.Package, input.Values)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root    string `json:"root"`
		Login   string `json:"login"`
		Package string `json:"package"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := s.robots.SaveLogin(input.Root, input.Login, input.Package)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// requireWorkbenchAuth rejects unauthenticated requests when the workbench
// identity system is enabled, matching the other management endpoints.
func (s *server) requireWorkbenchAuth(w http.ResponseWriter, r *http.Request) bool {
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if status.Enabled && !status.Authenticated {
		writeError(w, http.StatusUnauthorized, "请先登录身份认证账户。")
		return false
	}
	return true
}

func (s *server) githubAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	status, err := githubauth.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) githubAuthDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	flow, err := githubauth.StartDeviceFlow()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, flow)
}

func (s *server) githubAuthPollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	var input struct {
		FlowID string `json:"flowId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.FlowID == "" {
		writeError(w, http.StatusBadRequest, "缺少授权流程标识。")
		return
	}
	result, err := githubauth.PollDeviceFlow(input.FlowID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) githubAuthClientIDHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		value, source := githubauth.ClientIDSource()
		writeJSON(w, http.StatusOK, map[string]string{
			"clientId": value,
			"source":   source,
		})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		ClientID string `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	if err := githubauth.SaveClientID(input.ClientID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value, source := githubauth.ClientIDSource()
	writeJSON(w, http.StatusOK, map[string]string{
		"clientId": value,
		"source":   source,
	})
}

func (s *server) githubAuthTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Token) == "" {
		writeError(w, http.StatusBadRequest, "请填写 GitHub Token。")
		return
	}
	status, err := githubauth.SaveManualToken(input.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *server) githubAuthLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !s.requireWorkbenchAuth(w, r) {
		return
	}
	if err := githubauth.Logout(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"loggedOut": true})
}

// robotOneBotSyncHandler is the only composite write used by system plugins.
// It snapshots the robot configuration and restores it if selecting the login
// connection fails, so a failed sync never leaves a partially changed robot.
func (s *server) robotOneBotSyncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root  string `json:"root"`
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	if strings.TrimSpace(input.URL) == "" || strings.TrimSpace(input.Token) == "" {
		writeError(w, http.StatusBadRequest, "OneBot URL 和 Token 均不能为空，空 Token 不会覆盖机器人配置。")
		return
	}
	definition, err := s.robots.PackageConfig(input.Root, "@alemonjs/onebot")
	if err != nil {
		writeError(w, http.StatusBadRequest, "目标机器人未安装或未声明 @alemonjs/onebot；请先在工作台安装连接包。")
		return
	}
	fields := map[string]bool{}
	for _, field := range definition.Fields {
		fields[field.Name] = true
	}
	if !fields["url"] || !fields["token"] {
		writeError(w, http.StatusBadRequest, "@alemonjs/onebot 未声明 url 和 token 配置，不能安全同步。")
		return
	}
	previous, err := s.robots.Read(input.Root, "alemon.config.yaml")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.robots.SavePackageConfig(input.Root, "@alemonjs/onebot", map[string]any{"url": input.URL, "token": input.Token}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.robots.SaveLogin(input.Root, "onebot", "@alemonjs/onebot"); err != nil {
		if _, restoreErr := s.robots.Write(input.Root, "alemon.config.yaml", previous.Output); restoreErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("同步登录失败，且恢复原配置失败：%v", restoreErr))
			return
		}
		writeError(w, http.StatusBadRequest, "同步登录失败，已恢复机器人原配置和登录方式："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synchronized": true, "restartRequired": true})
}

func (s *server) robotGitInitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root string `json:"root"`
		robot.GitInitConfig
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别。")
		return
	}
	result, err := robot.InitializeGit(input.Root, input.GitInitConfig)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotGitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		view := r.URL.Query().Get("view")
		if view == "" {
			view = "commit"
		}
		status, err := robot.GitWorkspaceView(r.URL.Query().Get("root"), view)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, status)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root    string `json:"root"`
		Action  string `json:"action"`
		Value   string `json:"value"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Git 操作内容无法识别。")
		return
	}
	result, err := robot.GitWorkspaceAction(input.Root, input.Action, input.Value, input.Message)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotGitDiffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	path := r.URL.Query().Get("path")
	if root == "" || path == "" {
		writeError(w, http.StatusBadRequest, "缺少机器人目录或文件路径。")
		return
	}
	result, err := robot.GitDiff(root, path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) robotGitCloneHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Destination string `json:"destination"`
		Repository  string `json:"repository"`
		Branch      string `json:"branch"`
		Name        string `json:"name"`
		Mirror      string `json:"mirror"`
		Depth       int    `json:"depth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Git 信息无法识别。")
		return
	}
	destination, err := s.managedDirectory(input.Destination)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := robot.ValidateCloneRepository(input.Repository); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target, err := robot.CloneDestination(destination, input.Repository, input.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if target.Exists {
		writeError(w, http.StatusConflict, "目标目录已存在。")
		return
	}
	created := operationTask{ID: "clone-" + time.Now().Format("20060102150405.000000000"), Root: destination, Path: target.Path, Action: "git-clone", Status: "running", Output: "正在启动 Git…", Progress: 0, CreatedAt: time.Now()}
	s.addOperation(created)
	go func() {
		result, err := robot.CloneRepositoryWithProgress(destination, input.Repository, input.Branch, input.Name, input.Mirror, input.Depth, func(progress robot.CloneProgress) {
			s.updateOperation(created.ID, progress.Percent, progress.Detail, "", false)
		})
		if err != nil {
			s.updateOperation(created.ID, 100, result.Output, err.Error(), true)
			return
		}
		s.updateOperation(created.ID, 100, result.Output, "", true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

func (s *server) updateOperation(id string, progress int, output, failure string, finished bool) {
	s.updateOperationData(id, progress, output, failure, nil, finished)
}

func (s *server) updateOperationData(id string, progress int, output, failure string, data json.RawMessage, finished bool) {
	s.mu.Lock()
	var snapshot operationTask
	for index := range s.operations {
		if s.operations[index].ID != id {
			continue
		}
		s.operations[index].Progress = progress
		if output != "" {
			s.operations[index].Output = output
			appendOperationStep(&s.operations[index], progress, output)
		}
		if data != nil {
			s.operations[index].Data = data
		}
		if finished {
			now := time.Now()
			s.operations[index].FinishedAt = &now
			s.operations[index].Status = "completed"
			if failure != "" {
				s.operations[index].Status = "failed"
				s.operations[index].Error = failure
			}
		}
		snapshot = s.operations[index]
		break
	}
	s.mu.Unlock()
	if snapshot.ID != "" {
		s.publishRobotEvent(robotEvent{Type: "task", TaskID: snapshot.ID, Task: &snapshot})
	}
}

const (
	operationStepLimit       = 64
	operationStepMessageSize = 1024
)

func appendOperationStep(task *operationTask, progress int, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if len(message) > operationStepMessageSize {
		message = message[:operationStepMessageSize] + "…"
	}
	if count := len(task.Steps); count > 0 {
		previous := task.Steps[count-1]
		if previous.Progress == progress && previous.Message == message {
			return
		}
		// Progress within one stage (especially a large download) is an update,
		// not a new log line. Keep a compact stage timeline while task.Output
		// and SSE continue to expose the newest exact message.
		if operationStage(previous.Message) == operationStage(message) {
			task.Steps[count-1] = operationStep{At: time.Now(), Progress: progress, Message: message}
			return
		}
	}
	task.Steps = append(task.Steps, operationStep{At: time.Now(), Progress: progress, Message: message})
	if len(task.Steps) > operationStepLimit {
		task.Steps = append([]operationStep(nil), task.Steps[len(task.Steps)-operationStepLimit:]...)
	}
}

func operationStage(message string) string {
	message = strings.TrimSpace(message)
	if before, _, found := strings.Cut(message, "（"); found {
		message = before
	}
	if before, _, found := strings.Cut(message, "("); found {
		message = before
	}
	return strings.TrimSpace(message)
}

func (s *server) robotGitCloneCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	destination, err := s.managedDirectory(r.URL.Query().Get("destination"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	target, err := robot.CloneDestination(destination, r.URL.Query().Get("repository"), r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *server) robotPackageGitCloneHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Root       string `json:"root"`
		Repository string `json:"repository"`
		Branch     string `json:"branch"`
		Name       string `json:"name"`
		Mirror     string `json:"mirror"`
		Depth      int    `json:"depth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "Git 信息无法识别。")
		return
	}
	if strings.TrimSpace(input.Branch) == "" {
		input.Branch = "release"
	}
	if _, err := s.robots.Validate(input.Root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := robot.ValidateCloneRepository(input.Repository); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target, err := robot.LocalPackageCloneDestination(input.Root, input.Repository, input.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if target.Exists {
		writeError(w, http.StatusConflict, "背包中已存在同名目录。")
		return
	}
	created := operationTask{ID: "clone-package-" + time.Now().Format("20060102150405.000000000"), Root: input.Root, Path: target.Path, Action: "git-clone-package", Status: "running", Output: "正在启动 Git…", Progress: 0, CreatedAt: time.Now()}
	s.addOperation(created)
	go func() {
		result, err := robot.CloneLocalPackageWithProgress(input.Root, input.Repository, input.Branch, input.Name, input.Mirror, input.Depth, func(progress robot.CloneProgress) {
			s.updateOperation(created.ID, progress.Percent, progress.Detail, "", false)
		})
		if err != nil {
			s.updateOperation(created.ID, 100, result.Output, err.Error(), true)
			return
		}
		s.updateOperation(created.ID, 100, result.Output, "", true)
	}()
	writeJSON(w, http.StatusAccepted, created)
}

func (s *server) robotPackageGitCloneCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	root := r.URL.Query().Get("root")
	if _, err := s.robots.Validate(root); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target, err := robot.LocalPackageCloneDestination(root, r.URL.Query().Get("repository"), r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *server) robotGitCloneBranchesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	branches, defaultBranch, err := robot.CloneBranches(r.URL.Query().Get("repository"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branches": branches, "defaultBranch": defaultBranch})
}

func (s *server) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if s.creator == nil {
		writeError(w, http.StatusServiceUnavailable, "当前运行包未包含项目模板，无法创建项目。")
		return
	}
	var input project.Config
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "创建参数无法识别，请检查项目名称和保存位置。")
		return
	}
	created, err := s.creator.Create(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "result": created})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *server) spa() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := fs.Stat(s.assets, r.URL.Path[1:]); err == nil {
				s.static.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		s.static.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	s.confirmUpdateStartup()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.version})
}

func (s *server) listGoals(w http.ResponseWriter, _ *http.Request) {
	result := append([]goal(nil), goals...)
	if s.network != nil {
		for index := range result {
			if result[index].DownloadURL == "" {
				continue
			}
			if rewritten, err := s.network.RewriteURL(result[index].DownloadURL); err == nil {
				result[index].DownloadURL = rewritten
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) checksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		GoalID  string `json:"goalId"`
		Variant string `json:"variant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "请求内容无法识别，请重新选择操作目标。")
		return
	}
	if _, ok := findGoal(input.GoalID); !ok {
		writeError(w, http.StatusBadRequest, "所选操作目标不存在，请返回首页重新选择。")
		return
	}
	if input.Variant != "" {
		valid := input.GoalID == "web" && (input.Variant == "clean" || input.Variant == "docker") || input.GoalID == "build" && (input.Variant == "npm" || input.Variant == "git")
		if !valid {
			writeError(w, http.StatusBadRequest, "构建或部署方式无效，请重新选择。")
			return
		}
	}
	writeJSON(w, http.StatusOK, s.checker.CheckGoal(input.GoalID, input.Variant))
}

// environmentInstallHandler installs only a fixed prerequisite package set. It
// supports a browser connected to a remote server: no desktop dialog and no
// administrator password are involved.
func (s *server) environmentInstallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		CheckID string `json:"checkId"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "环境安装请求无效。")
		return
	}
	if !input.Confirm {
		writeError(w, http.StatusBadRequest, "请确认在服务器安装此环境。")
		return
	}
	if s.auth != nil {
		status, err := s.auth.Status(s.authToken(r))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "无法读取当前账户权限。")
			return
		}
		if status.Enabled && (!status.Authenticated || !status.SuperAdmin) {
			writeError(w, http.StatusForbidden, "只有已登录的超级管理员可以安装服务器环境。")
			return
		}
	}
	install := s.installEnvironment
	if install == nil {
		install = system.InstallEnvironment
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	output, installErr := install(ctx, input.CheckID)
	if installErr != nil {
		writeError(w, http.StatusBadRequest, installErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": output})
}

func findGoal(id string) (goal, bool) {
	for _, item := range goals {
		if item.ID == id {
			return item, true
		}
	}
	return goal{}, false
}

func (s *server) ginHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 开发模式下前端 Vite（localhost:5173）的 SSE 流式请求绕过代理直连
		// 后端，需要跨域许可。仅对 5173 本地来源放行，其余一律不加。
		origin := c.GetHeader("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type")
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Header("X-Content-Type-Options", "nosniff")
		// Registered robot 应用页, the in-page application service and setup
		// plugin pages are embedded in frames, so X-Frame-Options cannot be
		// SAMEORIGIN for them. Their responses carry their own CSP.
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/robot/webview/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/api/v1/robot/app/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/api/v1/services/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/api/v1/setup/plugins/development/") &&
			!strings.HasPrefix(c.Request.URL.Path, "/api/v1/setup/plugins/web/") {
			c.Header("X-Frame-Options", "DENY")
		}
		c.Next()
	}
}

// ginAccess protects every management API after local identity verification is
// enabled. Static files stay available so a browser can render the login view.
func (s *server) ginAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// A runner has no browser session. This isolated endpoint authenticates a
		// short-lived loopback token in its own handler and never accepts cookies
		// or a remote/reverse-proxied request.
		if path == pluginDownloadBrokerPath {
			c.Next()
			return
		}
		if !strings.HasPrefix(path, "/api/v1/") || path == "/api/v1/auth/status" || path == "/api/v1/auth/setup" || path == "/api/v1/auth/login" || path == "/api/v1/auth/logout" || strings.HasPrefix(path, "/api/v1/robot/webview/") || strings.HasPrefix(path, "/api/v1/robot/app/") {
			c.Next()
			return
		}
		status, err := s.auth.Status("")
		if err != nil {
			writeError(c.Writer, http.StatusInternalServerError, err.Error())
			c.Abort()
			return
		}
		if !status.Enabled {
			c.Next()
			return
		}
		token := s.authToken(c.Request)
		if !s.auth.Authenticate(token) {
			writeError(c.Writer, http.StatusUnauthorized, "请先登录身份认证账户。")
			c.Abort()
			return
		}
		if permission := requiredPermissionForRequest(c.Request); permission != "" && !s.auth.Authorize(token, permission) {
			writeError(c.Writer, http.StatusForbidden, "当前账户没有此操作权限。")
			c.Abort()
			return
		}
		c.Next()
	}
}

// requiredPermissionForRequest gives every authenticated management endpoint
// a server-enforced baseline. Fine-grained operations retain their local
// checks (for example /ops), while this mapping ensures a viewer cannot turn
// a read-only workbench into a write-capable one through a handcrafted call.
func requiredPermissionForRequest(r *http.Request) string {
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/v1/") || strings.HasPrefix(path, "/api/v1/auth/status") || strings.HasPrefix(path, "/api/v1/auth/setup") || strings.HasPrefix(path, "/api/v1/auth/login") || strings.HasPrefix(path, "/api/v1/auth/logout") || strings.HasPrefix(path, "/api/v1/robot/webview/") || strings.HasPrefix(path, "/api/v1/robot/app/") {
		return ""
	}
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return ""
	}
	// The main dashboard updates only its own validated active-project context
	// here. This is safe for a viewer and lets a plugin later read the same
	// narrow context only if its own capability and endpoint permissions allow.
	if path == "/api/v1/system/context/robot" {
		return "workbench.view"
	}
	if strings.HasPrefix(path, "/api/v1/ops") {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return "operations.view"
		}
		return "operations.manage"
	}
	if strings.HasPrefix(path, "/api/v1/update") || strings.HasPrefix(path, "/api/v1/system/") || strings.HasPrefix(path, "/api/v1/setup/plugins") {
		return "system.manage"
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return "workbench.view"
	}
	return "workbench.manage"
}

func (s *server) ginRequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := s.requestID.Add(1)
		started := time.Now()
		c.Set("requestID", id)
		requestFields := logging.Fields{
			"request_id":    id,
			"method":        c.Request.Method,
			"path":          c.Request.URL.Path,
			"content_type":  c.GetHeader("Content-Type"),
			"request_bytes": c.Request.ContentLength,
		}
		if query := loggableQuery(c.Request.URL.Query()); len(query) > 0 {
			requestFields["query"] = query
		}
		if body := s.loggableRequestBody(c); body != "" {
			requestFields["body"] = logging.RawJSON(body)
		}
		logging.InfoEvent("http.request.started", requestFields)
		// Capture the response body so a failed request's error is visible in
		// the console, not only pasted back to the browser. Only failed
		// responses are logged, and only their body.
		captured := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = captured
		c.Next()
		status := c.Writer.Status()
		level := logging.Info
		outcome := "success"
		responseMessage := ""
		if status >= http.StatusBadRequest {
			outcome = "client_error"
			if status >= http.StatusInternalServerError {
				outcome = "server_error"
				level = logging.Error
			} else {
				level = logging.Warn
			}
			responseMessage = captured.message()
		}
		responseFields := logging.Fields{
			"request_id":     id,
			"method":         c.Request.Method,
			"path":           c.Request.URL.Path,
			"status":         status,
			"outcome":        outcome,
			"duration_ms":    time.Since(started).Milliseconds(),
			"response_bytes": c.Writer.Size(),
		}
		if responseMessage != "" {
			responseFields["response"] = logging.RawJSON(responseMessage)
		}
		logging.Event(level, "http.request.completed", responseFields)
	}
}

// loggableQuery returns query parameters as an object and redacts credentials
// before they can reach a terminal, a CI transcript, or a service log.
func loggableQuery(values url.Values) map[string]any {
	result := make(map[string]any, len(values))
	for key, values := range values {
		if sensitiveLogField(key) {
			result[key] = "[REDACTED]"
			continue
		}
		if len(values) == 1 {
			result[key] = values[0]
		} else {
			result[key] = values
		}
	}
	return result
}

// captureWriter buffers the response body so request logging can surface the
// error message of a failed call. Buffering is bounded to keep large payloads
// (files, logs) out of the console.
type captureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *captureWriter) Write(data []byte) (int, error) {
	if w.body.Len() < 4096 {
		w.body.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *captureWriter) message() string {
	text := strings.TrimSpace(w.body.String())
	if len(text) > 300 {
		return text[:300] + "…"
	}
	return text
}

// loggableRequestBody reads a bounded JSON body for logging and restores the
// stream so handlers still decode it. Sensitive fields (tokens, passwords) are
// redacted; the body is capped to keep logs readable.
func (s *server) loggableRequestBody(c *gin.Context) string {
	if c.Request.Body == nil || c.Request.Body == http.NoBody {
		return ""
	}
	// Only consume the body for small requests. Reading a large upload into
	// memory here (and restoring it) is pointless for logging and risks
	// truncating the stream that the handler still needs: a previous version
	// capped the read at 16 KiB and then replaced the body with just those
	// bytes, which silently broke every upload larger than 16 KiB.
	// ContentLength is -1 for chunked bodies, which we cannot bound cheaply.
	if c.Request.ContentLength < 0 || c.Request.ContentLength > 16*1024 {
		return ""
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		// Not JSON; log a trimmed prefix.
		text := strings.TrimSpace(string(raw))
		if len(text) > 200 {
			text = text[:200] + "…"
		}
		return text
	}
	redactRequestFields(fields)
	encoded, _ := json.Marshal(fields)
	return string(encoded)
}

func redactRequestFields(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if sensitiveLogField(key) {
				item[key] = "[REDACTED]"
				continue
			}
			redactRequestFields(child)
		}
	case []any:
		for _, child := range item {
			redactRequestFields(child)
		}
	}
}

func sensitiveLogField(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "-", "")) {
	case "token", "password", "sudopassword", "confirmation", "content", "message", "values", "authorization", "apikey", "secret":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
