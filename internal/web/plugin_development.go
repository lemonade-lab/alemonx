package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"alemonx/internal/setupplugin"
)

const pluginDevelopmentLogLimit = 256 << 10

type pluginDevelopmentStore struct {
	Sources []string `json:"sources"`
}

type pluginDevelopmentProcess struct {
	command *exec.Cmd
	done    chan struct{}
	port    int
	log     *pluginDevelopmentLog
}

type pluginDevelopmentLog struct {
	mu   sync.Mutex
	text []byte
}

func (l *pluginDevelopmentLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	l.text = append(l.text, data...)
	if len(l.text) > pluginDevelopmentLogLimit {
		l.text = append([]byte("…前面的输出已省略…\n"), l.text[len(l.text)-pluginDevelopmentLogLimit:]...)
	}
	l.mu.Unlock()
	return len(data), nil
}

func (l *pluginDevelopmentLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return string(l.text)
}

type pluginDevelopmentSession struct {
	plugin         setupplugin.Plugin
	source         string
	running        bool
	state          string
	operation      string
	web            *pluginDevelopmentProcess
	services       map[string]*pluginDevelopmentProcess
	serviceRestart map[string]string
	buildLog       string
	lastError      string
	updatedAt      time.Time
}

type pluginDevelopmentView struct {
	ID             string                         `json:"id"`
	Name           string                         `json:"name"`
	Source         string                         `json:"source"`
	Registered     bool                           `json:"registered"`
	Running        bool                           `json:"running"`
	State          string                         `json:"state"`
	Busy           bool                           `json:"busy"`
	Runner         string                         `json:"runner,omitempty"`
	WebMode        string                         `json:"webMode,omitempty"`
	BuildAvailable bool                           `json:"buildAvailable"`
	WebURL         string                         `json:"webUrl,omitempty"`
	WebPort        int                            `json:"webPort,omitempty"`
	SourceType     string                         `json:"sourceType"`
	Privileges     []string                       `json:"privileges,omitempty"`
	Services       []pluginDevelopmentServiceView `json:"services,omitempty"`
	LastError      string                         `json:"lastError,omitempty"`
	UpdatedAt      time.Time                      `json:"updatedAt"`
}

type pluginDevelopmentServiceView struct {
	ID      string `json:"id"`
	Port    int    `json:"port,omitempty"`
	Running bool   `json:"running"`
	Restart string `json:"restart,omitempty"`
}

type pluginDevelopmentManager struct {
	mu        sync.RWMutex
	registry  *setupplugin.Registry
	statePath string
	sessions  map[string]*pluginDevelopmentSession
}

func newPluginDevelopmentManager(registry *setupplugin.Registry, statePath string) *pluginDevelopmentManager {
	m := &pluginDevelopmentManager{registry: registry, statePath: statePath, sessions: map[string]*pluginDevelopmentSession{}}
	var state pluginDevelopmentStore
	data, err := os.ReadFile(statePath)
	if err != nil || json.Unmarshal(data, &state) != nil {
		return m
	}
	for _, source := range state.Sources {
		if plugin, loadErr := setupplugin.LoadDevelopmentSource(source); loadErr == nil {
			m.sessions[plugin.ID] = &pluginDevelopmentSession{plugin: plugin, source: plugin.Source, state: "registered", services: map[string]*pluginDevelopmentProcess{}, serviceRestart: map[string]string{}, updatedAt: time.Now()}
		}
	}
	return m
}

// registerPath receives a path only after the workbench Finder has constrained
// it to an approved local directory root. The manager still re-parses the
// manifest before it is allowed to become an executable development session.
func (m *pluginDevelopmentManager) registerPath(source string) (pluginDevelopmentView, error) {
	plugin, err := setupplugin.LoadDevelopmentSource(source)
	if err != nil {
		return pluginDevelopmentView{}, err
	}
	m.mu.Lock()
	existing := m.sessions[plugin.ID]
	if existing != nil && existing.operation != "" {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("源码插件正在" + existing.operation + "，请等待当前操作完成")
	}
	m.mu.Unlock()
	if existing != nil && existing.running {
		if _, err := m.stop(plugin.ID); err != nil {
			return pluginDevelopmentView{}, err
		}
	}
	m.mu.Lock()
	m.sessions[plugin.ID] = &pluginDevelopmentSession{plugin: plugin, source: plugin.Source, state: "registered", services: map[string]*pluginDevelopmentProcess{}, serviceRestart: map[string]string{}, updatedAt: time.Now()}
	m.saveLocked()
	view := m.viewLocked(m.sessions[plugin.ID])
	m.mu.Unlock()
	return view, nil
}

func (m *pluginDevelopmentManager) list() []pluginDevelopmentView {
	m.mu.RLock()
	items := make([]pluginDevelopmentView, 0, len(m.sessions))
	for _, session := range m.sessions {
		items = append(items, m.viewLocked(session))
	}
	m.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (m *pluginDevelopmentManager) viewLocked(session *pluginDevelopmentSession) pluginDevelopmentView {
	state := session.state
	if state == "" {
		state = "registered"
	}
	view := pluginDevelopmentView{ID: session.plugin.ID, Name: session.plugin.Name, Source: session.source, SourceType: "source", Registered: true, Running: session.running, State: state, Busy: session.operation != "", LastError: session.lastError, UpdatedAt: session.updatedAt}
	for _, operation := range session.plugin.PrivilegedOperations {
		view.Privileges = append(view.Privileges, operation.Action)
	}
	if session.plugin.Development != nil {
		view.Runner = session.plugin.Development.Runtime
		if web := session.plugin.Development.Web; web != nil {
			view.WebMode = web.Mode
			view.BuildAvailable = web.Build != nil
		}
	}
	if session.web != nil {
		view.WebPort = session.web.port
		view.WebURL = developmentWebURL(session.plugin.ID)
	}
	for _, declaration := range session.plugin.Development.Services {
		service := pluginDevelopmentServiceView{ID: declaration.ID, Restart: declaration.Restart}
		for _, known := range session.plugin.Services {
			if known.ID == declaration.ID {
				service.Port = known.Port
				break
			}
		}
		service.Running = session.services[declaration.ID] != nil
		view.Services = append(view.Services, service)
	}
	return view
}

func (m *pluginDevelopmentManager) start(id string) (pluginDevelopmentView, error) {
	m.mu.Lock()
	session := m.sessions[id]
	if session == nil {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("未登记该源码插件")
	}
	if session.operation != "" {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("源码插件正在" + session.operation + "，请等待当前操作完成")
	}
	if session.running {
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, nil
	}
	plugin := session.plugin
	session.operation, session.state, session.updatedAt = "启动", "starting", time.Now()
	m.mu.Unlock()
	if plugin.Development == nil || plugin.Development.Web == nil || plugin.Development.Web.Mode != "dev-server" {
		if _, err := plugin.WebRoot(); err != nil {
			return m.failStart(id, session, err)
		}
	}

	started := make([]*pluginDevelopmentProcess, 0)
	startFailure := func(err error) (pluginDevelopmentView, error) {
		for _, process := range started {
			_ = stopPluginDevelopmentProcess(process)
		}
		m.mu.Lock()
		if m.sessions[id] == session && session.operation == "启动" {
			session.operation, session.state, session.lastError, session.updatedAt = "", "failed", err.Error(), time.Now()
		}
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, err
	}

	services := map[string]*pluginDevelopmentProcess{}
	serviceRestart := map[string]string{}
	if plugin.Development != nil {
		for _, service := range plugin.Development.Services {
			servicePort := 0
			var declared *setupplugin.ServiceSpec
			for index := range plugin.Services {
				if plugin.Services[index].ID == service.ID {
					declared = &plugin.Services[index]
					servicePort = declared.Port
					break
				}
			}
			process, err := startPluginDevelopmentCommand(plugin.Source, &service.Command, servicePort)
			if err != nil {
				return startFailure(fmtDevelopmentError("启动服务 "+service.ID, err))
			}
			started = append(started, process)
			if declared != nil {
				if err := waitDevelopmentHTTP(declared.Port, declared.HealthPath, 30*time.Second); err != nil {
					return startFailure(fmtDevelopmentError("等待服务 "+service.ID, err))
				}
			}
			services[service.ID] = process
			serviceRestart[service.ID] = service.Restart
		}
	}
	var web *pluginDevelopmentProcess
	if plugin.Development != nil && plugin.Development.Web != nil && plugin.Development.Web.Mode == "dev-server" {
		port, err := freeDevelopmentPort()
		if err != nil {
			return startFailure(err)
		}
		web, err = startPluginDevelopmentCommand(plugin.Source, plugin.Development.Web.Dev, port)
		if err != nil {
			return startFailure(fmtDevelopmentError("启动前端开发服务", err))
		}
		started = append(started, web)
		if err := waitDevelopmentHTTPAny(port, developmentWebHealthPaths(plugin.ID, plugin.Development.Web.HealthPath), 30*time.Second); err != nil {
			return startFailure(fmtDevelopmentError("等待前端开发服务", err))
		}
	}
	m.mu.Lock()
	if m.sessions[id] != session || session.operation != "启动" {
		m.mu.Unlock()
		for _, process := range started {
			_ = stopPluginDevelopmentProcess(process)
		}
		return pluginDevelopmentView{}, errors.New("源码插件启动已被新的操作取消")
	}
	session.plugin.DevelopmentWebProxy = web != nil
	session.services, session.serviceRestart, session.web, session.running = services, serviceRestart, web, true
	session.operation, session.state, session.lastError, session.updatedAt = "", "running", "", time.Now()
	m.registry.ActivateDevelopment(session.plugin)
	view := m.viewLocked(session)
	m.mu.Unlock()
	if plugin.Development != nil {
		for _, service := range plugin.Development.Services {
			if service.Restart == "on-failure" {
				m.superviseService(plugin.ID, service.ID, plugin.Source, service.Command, services[service.ID])
			}
		}
	}
	return view, nil
}

func (m *pluginDevelopmentManager) failStart(id string, session *pluginDevelopmentSession, err error) (pluginDevelopmentView, error) {
	m.mu.Lock()
	if m.sessions[id] == session && session.operation == "启动" {
		session.operation, session.state, session.lastError, session.updatedAt = "", "failed", err.Error(), time.Now()
	}
	view := m.viewLocked(session)
	m.mu.Unlock()
	return view, err
}

func (m *pluginDevelopmentManager) stop(id string) (pluginDevelopmentView, error) {
	m.mu.Lock()
	session := m.sessions[id]
	if session == nil {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("未登记该源码插件")
	}
	if session.operation != "" {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("源码插件正在" + session.operation + "，请等待当前操作完成")
	}
	if !session.running {
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, nil
	}
	web, services := session.web, session.services
	session.operation, session.state, session.updatedAt = "停止", "stopping", time.Now()
	m.mu.Unlock()
	var failures []string
	if web != nil {
		if err := stopPluginDevelopmentProcess(web); err != nil {
			failures = append(failures, "前端开发服务："+err.Error())
		}
	}
	for serviceID, process := range services {
		if err := stopPluginDevelopmentProcess(process); err != nil {
			failures = append(failures, "服务 "+serviceID+"："+err.Error())
		}
	}
	m.mu.Lock()
	if m.sessions[id] != session || session.operation != "停止" {
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, errors.New("源码插件停止状态已变化")
	}
	if len(failures) > 0 {
		session.operation, session.state, session.lastError, session.updatedAt = "", "failed", "未能完全停止源码进程："+strings.Join(failures, "；"), time.Now()
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, errors.New(session.lastError)
	}
	session.web, session.services, session.serviceRestart, session.running = nil, map[string]*pluginDevelopmentProcess{}, map[string]string{}, false
	session.operation, session.state, session.updatedAt = "", "stopped", time.Now()
	m.registry.DeactivateDevelopment(id)
	view := m.viewLocked(session)
	m.mu.Unlock()
	return view, nil
}

// remove forgets a development source registered with the workbench. It never
// touches the source directory itself. A running session must stop cleanly
// first so that its development overlay is removed before the record is gone.
func (m *pluginDevelopmentManager) remove(id string) error {
	m.mu.RLock()
	session := m.sessions[id]
	if session == nil {
		m.mu.RUnlock()
		return errors.New("未登记该源码插件")
	}
	running, operation := session.running, session.operation
	m.mu.RUnlock()
	if operation != "" {
		return errors.New("源码插件正在" + operation + "，请等待当前操作完成")
	}
	if running {
		if _, err := m.stop(id); err != nil {
			return err
		}
	}

	m.mu.Lock()
	session = m.sessions[id]
	if session == nil {
		m.mu.Unlock()
		return errors.New("未登记该源码插件")
	}
	if session.operation != "" || session.running {
		m.mu.Unlock()
		return errors.New("源码插件状态已变化，请稍后重试")
	}
	delete(m.sessions, id)
	m.saveLocked()
	m.mu.Unlock()
	// stop already deactivates a running source. This second call also covers
	// sessions that were registered but never started.
	m.registry.DeactivateDevelopment(id)
	return nil
}

func (m *pluginDevelopmentManager) restart(id string) (pluginDevelopmentView, error) {
	if _, err := m.stop(id); err != nil {
		return pluginDevelopmentView{}, err
	}
	return m.start(id)
}

func (m *pluginDevelopmentManager) build(id string) (pluginDevelopmentView, error) {
	m.mu.Lock()
	session := m.sessions[id]
	if session == nil || session.plugin.Development == nil || session.plugin.Development.Web == nil || session.plugin.Development.Web.Build == nil {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("该源码插件未声明前端构建命令")
	}
	if session.operation != "" {
		m.mu.Unlock()
		return pluginDevelopmentView{}, errors.New("源码插件正在" + session.operation + "，请等待当前操作完成")
	}
	source, command := session.source, session.plugin.Development.Web.Build
	previousState := session.state
	session.operation, session.state, session.updatedAt = "构建", "building", time.Now()
	m.mu.Unlock()
	log := &pluginDevelopmentLog{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	run := exec.CommandContext(ctx, command.Program, expandDevelopmentArgs(command.Args, 0)...)
	run.Dir, run.Stdout, run.Stderr = source, log, log
	err := run.Run()
	m.mu.Lock()
	if m.sessions[id] != session || session.operation != "构建" {
		view := m.viewLocked(session)
		m.mu.Unlock()
		return view, errors.New("源码插件构建状态已变化")
	}
	session.operation, session.state = "", previousState
	if err != nil {
		session.state, session.lastError = "failed", fmtDevelopmentError("前端构建失败", err).Error()+"\n"+log.String()
	} else {
		session.lastError = ""
	}
	session.buildLog = log.String()
	session.updatedAt = time.Now()
	view := m.viewLocked(session)
	m.mu.Unlock()
	return view, err
}

func (m *pluginDevelopmentManager) logs(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil {
		return "", errors.New("未登记该源码插件")
	}
	var text strings.Builder
	if session.web != nil {
		text.WriteString("[前端开发服务]\n")
		text.WriteString(session.web.log.String())
	}
	for id, service := range session.services {
		text.WriteString("\n[服务 " + id + "]\n")
		text.WriteString(service.log.String())
	}
	if session.lastError != "" {
		text.WriteString("\n[最近错误]\n")
		text.WriteString(session.lastError)
	}
	if session.buildLog != "" {
		text.WriteString("\n[最近构建]\n")
		text.WriteString(session.buildLog)
	}
	if text.Len() == 0 {
		return "尚无开发进程日志。", nil
	}
	return text.String(), nil
}

func (m *pluginDevelopmentManager) serviceLogs(id, serviceID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil {
		return "", errors.New("未登记该源码插件")
	}
	process := session.services[serviceID]
	if process == nil {
		return "", errors.New("该源码服务尚未启动")
	}
	if text := process.log.String(); text != "" {
		return text, nil
	}
	return "尚无服务日志。", nil
}

func (m *pluginDevelopmentManager) restartService(id, serviceID string) (pluginDevelopmentView, error) {
	m.mu.RLock()
	session := m.sessions[id]
	if session == nil {
		m.mu.RUnlock()
		return pluginDevelopmentView{}, errors.New("未登记该源码插件")
	}
	if !session.running {
		m.mu.RUnlock()
		return pluginDevelopmentView{}, errors.New("源码开发会话尚未启动")
	}
	found := false
	for _, declaration := range session.plugin.Development.Services {
		if declaration.ID == serviceID {
			found = true
			break
		}
	}
	m.mu.RUnlock()
	if !found {
		return pluginDevelopmentView{}, errors.New("未声明该源码服务")
	}
	// A source session owns one dependent process topology. Restarting the
	// session is safer than leaving another declared service with stale state.
	return m.restart(id)
}

func (m *pluginDevelopmentManager) webTarget(id string) (setupplugin.Plugin, setupplugin.ServiceSpec, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session := m.sessions[id]
	if session == nil || !session.running || session.web == nil || session.web.port < 1 {
		return setupplugin.Plugin{}, setupplugin.ServiceSpec{}, false
	}
	hmr := session.plugin.Development != nil && session.plugin.Development.Web != nil && session.plugin.Development.Web.HMR
	return session.plugin, setupplugin.ServiceSpec{ID: "development-web", Name: "源码前端", Host: "127.0.0.1", Port: session.web.port, BasePath: "/", HealthPath: "/", Embed: true, RewriteHTML: true, SSE: true, WebSocket: hmr}, true
}

func (m *pluginDevelopmentManager) close() {
	for _, session := range m.list() {
		_, _ = m.stop(session.ID)
	}
}

func (m *pluginDevelopmentManager) saveLocked() {
	if m.statePath == "" {
		return
	}
	sources := make([]string, 0, len(m.sessions))
	for _, session := range m.sessions {
		sources = append(sources, session.source)
	}
	sort.Strings(sources)
	data, err := json.Marshal(pluginDevelopmentStore{Sources: sources})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(m.statePath), 0700)
	_ = os.WriteFile(m.statePath+".new", data, 0600)
	_ = os.Rename(m.statePath+".new", m.statePath)
}

func startPluginDevelopmentCommand(source string, command *setupplugin.CommandSpec, port int) (*pluginDevelopmentProcess, error) {
	if command == nil {
		return nil, errors.New("缺少开发命令")
	}
	log := &pluginDevelopmentLog{}
	run := exec.Command(command.Program, expandDevelopmentArgs(command.Args, port)...)
	run.Dir, run.Stdout, run.Stderr = source, log, log
	run.Env = append(os.Environ(), "ALX_PLUGIN_DEV_PORT="+strconv.Itoa(port), "ALX_PLUGIN_SOURCE="+source)
	configureManagedProcess(run)
	if err := run.Start(); err != nil {
		return nil, err
	}
	process := &pluginDevelopmentProcess{command: run, done: make(chan struct{}), port: port, log: log}
	go func() { _ = run.Wait(); close(process.done) }()
	return process, nil
}

func expandDevelopmentArgs(args []string, port int) []string {
	result := make([]string, len(args))
	for index, arg := range args {
		result[index] = strings.ReplaceAll(arg, "${ALX_PLUGIN_DEV_PORT}", strconv.Itoa(port))
	}
	return result
}

func freeDevelopmentPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitDevelopmentHTTP(port int, healthPath string, timeout time.Duration) error {
	return waitDevelopmentHTTPAny(port, []string{healthPath}, timeout)
}

// developmentWebHealthPaths checks the development server directly. The host
// owns the browser-facing mount and rewrites browser resources through it;
// asking Vite for that mount can otherwise redirect back to a path it does not
// serve and make a healthy server look unavailable.
func developmentWebHealthPaths(pluginID, healthPath string) []string {
	if healthPath == "" {
		healthPath = "/"
	}
	mounted := developmentWebURL(pluginID) + strings.TrimPrefix(healthPath, "/")
	if mounted == healthPath {
		return []string{healthPath}
	}
	return []string{healthPath, mounted}
}

func waitDevelopmentHTTPAny(port int, healthPaths []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		for _, healthPath := range healthPaths {
			response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + healthPath)
			if err == nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusBadRequest {
				_ = response.Body.Close()
				return nil
			}
			if response != nil {
				_ = response.Body.Close()
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("服务在 30 秒内未就绪")
}

func stopPluginDevelopmentProcess(process *pluginDevelopmentProcess) error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return nil
	}
	select {
	case <-process.done:
		return nil
	default:
	}
	interruptErr := interruptManagedProcess(process.command)
	select {
	case <-process.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	forceErr := forceStopManagedProcess(process.command)
	select {
	case <-process.done:
		return nil
	case <-time.After(5 * time.Second):
		if forceErr != nil {
			return fmt.Errorf("强制终止失败：%w", forceErr)
		}
		if interruptErr != nil {
			return fmt.Errorf("进程未在超时内退出：%w", interruptErr)
		}
		return errors.New("进程未在强制终止后退出")
	}
}

func (m *pluginDevelopmentManager) superviseService(pluginID, serviceID, source string, command setupplugin.CommandSpec, process *pluginDevelopmentProcess) {
	go func() {
		<-process.done
		if process.command.ProcessState == nil || process.command.ProcessState.ExitCode() == 0 {
			return
		}
		m.mu.RLock()
		session := m.sessions[pluginID]
		shouldRestart := session != nil && session.running && session.operation == "" && session.services[serviceID] == process && session.serviceRestart[serviceID] == "on-failure"
		m.mu.RUnlock()
		if !shouldRestart {
			return
		}
		time.Sleep(time.Second)
		servicePort := 0
		var declared *setupplugin.ServiceSpec
		m.mu.RLock()
		if session := m.sessions[pluginID]; session != nil {
			for index := range session.plugin.Services {
				if session.plugin.Services[index].ID == serviceID {
					copy := session.plugin.Services[index]
					declared = &copy
					servicePort = copy.Port
					break
				}
			}
		}
		m.mu.RUnlock()
		restarted, err := startPluginDevelopmentCommand(source, &command, servicePort)
		if err == nil && declared != nil {
			err = waitDevelopmentHTTP(declared.Port, declared.HealthPath, 30*time.Second)
		}
		if err != nil && restarted != nil {
			_ = stopPluginDevelopmentProcess(restarted)
			restarted = nil
		}
		m.mu.Lock()
		session = m.sessions[pluginID]
		if err != nil || session == nil || !session.running || session.operation != "" || session.services[serviceID] != process {
			if session != nil && err != nil {
				session.lastError, session.updatedAt = fmtDevelopmentError("重启服务 "+serviceID, err).Error(), time.Now()
			}
			m.mu.Unlock()
			if restarted != nil {
				_ = stopPluginDevelopmentProcess(restarted)
			}
			return
		}
		session.services[serviceID], session.updatedAt = restarted, time.Now()
		m.mu.Unlock()
		m.superviseService(pluginID, serviceID, source, command, restarted)
	}()
}

func developmentWebURL(id string) string { return "/api/v1/setup/plugins/development/" + id + "/web/" }

func fmtDevelopmentError(prefix string, err error) error {
	if err == nil {
		return nil
	}
	return errors.New(prefix + "：" + err.Error())
}

func (s *server) setupPluginDevelopmentHandler(w http.ResponseWriter, r *http.Request) {
	if s.pluginDevelopment == nil {
		writeError(w, http.StatusServiceUnavailable, "源码开发功能不可用")
		return
	}
	const prefix = "/api/v1/setup/plugins/development"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		if r.Method == http.MethodGet {
			if !s.requirePluginDevelopment(w, r) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": s.pluginDevelopment.list()})
			return
		}
		if r.Method == http.MethodPost {
			if !s.requirePluginDevelopment(w, r) {
				return
			}
			var input struct {
				Path string `json:"path"`
			}
			if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input) != nil {
				writeError(w, http.StatusBadRequest, "请选择插件源码目录")
				return
			}
			var view pluginDevelopmentView
			var err error
			if strings.TrimSpace(input.Path) != "" {
				path, pathErr := s.managedDirectory(input.Path)
				if pathErr != nil {
					writeError(w, http.StatusForbidden, pathErr.Error())
					return
				}
				view, err = s.pluginDevelopment.registerPath(path)
			} else {
				writeError(w, http.StatusBadRequest, "请选择插件源码目录")
				return
			}
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusCreated, view)
			return
		}
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if rest == "pick" {
		writeError(w, http.StatusGone, "源码目录请选择已迁移到工作台 Finder。")
		return
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "未找到源码开发会话。")
		return
	}
	id, action := parts[0], parts[1]
	if action == "web" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "源码前端仅支持读取。")
			return
		}
		if !s.requirePluginDevelopment(w, r) {
			return
		}
		requestPath := "/"
		if len(parts) == 3 {
			requestPath += parts[2]
		}
		s.pluginDevelopmentWebProxy(w, r, id, requestPath)
		return
	}
	if !s.requirePluginDevelopment(w, r) {
		return
	}
	if action == "logs" && r.Method == http.MethodGet {
		output, err := s.pluginDevelopment.logs(id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"output": output})
		return
	}
	if action == "services" && len(parts) == 3 {
		serviceParts := strings.SplitN(parts[2], "/", 2)
		if len(serviceParts) == 2 && serviceParts[0] != "" && serviceParts[1] == "logs" && r.Method == http.MethodGet {
			output, err := s.pluginDevelopment.serviceLogs(id, serviceParts[0])
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"output": output})
			return
		}
		if len(serviceParts) == 2 && serviceParts[0] != "" && serviceParts[1] == "restart" && r.Method == http.MethodPost {
			var input struct {
				Confirm bool `json:"confirm"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input)
			if !input.Confirm {
				writeError(w, http.StatusBadRequest, "请确认重启源码服务。")
				return
			}
			view, err := s.pluginDevelopment.restartService(id, serviceParts[0])
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, view)
			return
		}
	}
	if action == "remove" && r.Method == http.MethodDelete {
		if err := s.pluginDevelopment.remove(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "removed": true})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input)
	if (action == "start" || action == "restart" || action == "build") && !input.Confirm {
		writeError(w, http.StatusBadRequest, "请确认执行源码开发命令。")
		return
	}
	var view pluginDevelopmentView
	var err error
	switch action {
	case "start":
		view, err = s.pluginDevelopment.start(id)
	case "stop":
		view, err = s.pluginDevelopment.stop(id)
	case "restart":
		view, err = s.pluginDevelopment.restart(id)
	case "build":
		view, err = s.pluginDevelopment.build(id)
	default:
		writeError(w, http.StatusNotFound, "未知源码开发操作。")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) requirePluginDevelopment(w http.ResponseWriter, r *http.Request) bool {
	if !s.localSystemDialogRequest(r) {
		writeError(w, http.StatusForbidden, "源码开发只能在本机桌面工作台中执行。")
		return false
	}
	status, err := s.auth.Status(s.authToken(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if status.Enabled && (!status.Authenticated || !status.SuperAdmin) {
		writeError(w, http.StatusForbidden, "只有已登录的超级管理员可以管理源码开发会话。")
		return false
	}
	return true
}

func (s *server) pluginDevelopmentWebProxy(w http.ResponseWriter, r *http.Request, id, requestPath string) {
	// A source session is intentionally live: neither the WebView nor an
	// intermediary may reuse a previous Vite HTML/module response after a
	// developer has stopped and started the session again. In particular this
	// prevents a stale entry module from presenting an older plugin UI while its
	// current runner has already changed.
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	if requestPath == "/finder-bridge.js" || requestPath == "/host-bridge.js" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = io.WriteString(w, setupPluginHostBridge())
		return
	}
	plugin, service, ok := s.pluginDevelopment.webTarget(id)
	if !ok {
		writeError(w, http.StatusNotFound, "源码前端尚未启动。")
		return
	}
	if isUpgradeRequest(r) {
		if !service.WebSocket {
			writeError(w, http.StatusNotImplemented, "该源码前端未声明 HMR WebSocket 支持。")
			return
		}
		s.localServiceWebSocketHandler(w, r, plugin, service, requestPath)
		return
	}
	target, err := localServiceTarget(service, requestPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	mount := developmentWebURL(id)
	cookiePrefix := localServiceCookiePrefix(plugin.ID, service.ID)
	proxy := &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			request.URL.Scheme, request.URL.Host, request.URL.Path, request.URL.RawPath = target.Scheme, target.Host, target.Path, ""
			request.URL.RawQuery, request.Host = r.URL.RawQuery, target.Host
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
			for _, cookie := range r.Cookies() {
				if strings.HasPrefix(cookie.Name, cookiePrefix) {
					request.AddCookie(&http.Cookie{Name: strings.TrimPrefix(cookie.Name, cookiePrefix), Value: cookie.Value})
				}
			}
		},
		Transport: developmentWebTransport(), FlushInterval: 100 * time.Millisecond,
		ModifyResponse: func(response *http.Response) error {
			response.Header.Set("Cache-Control", "no-store, max-age=0")
			if location := response.Header.Get("Location"); location != "" && !localServiceLocationAllowed(location, target) {
				return errors.New("源码前端重定向到了不受信任的地址")
			}
			modifyRobotAppResponse(response, target, mount, r.URL.Path, r)
			rewriteDevelopmentModuleImports(response, mount, requestPath == "/@vite/client")
			injectSetupPluginFinderBridge(response)
			isolateLocalServiceCookies(response, cookiePrefix, mount)
			response.Header.Del("X-Frame-Options")
			response.Header.Set("Content-Security-Policy", localServiceFramePolicy(response.Header.Get("Content-Security-Policy")))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "源码前端代理失败："+err.Error())
		},
	}
	proxy.ServeHTTP(w, r)
}

const developmentModuleRewriteLimit = 4 << 20

const developmentViteRetryCount = 8

type developmentRetryTransport struct{ base http.RoundTripper }

// RoundTrip hides Vite's short-lived 504 response while its optimized
// dependency cache is being rebuilt. Source frontends only use GET/HEAD here,
// so retrying the original request never repeats a state-changing operation.
func (t developmentRetryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := t.base.RoundTrip(request)
		if err != nil || response == nil || response.StatusCode != http.StatusGatewayTimeout || attempt >= developmentViteRetryCount {
			return response, err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func developmentWebTransport() http.RoundTripper {
	return developmentRetryTransport{base: localServiceTransport(true)}
}

var developmentModuleImportPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)(\bfrom\s*)(["'])/`),
	regexp.MustCompile(`(?m)(\bimport\s*)(["'])/`),
	regexp.MustCompile(`(?m)(\bimport\s*\(\s*)(["'])/`),
	regexp.MustCompile(`(?m)(\bnew\s+URL\s*\(\s*)(["'])/`),
}

// rewriteDevelopmentModuleImports keeps Vite's transformed imports inside the
// source-development proxy. Vite emits optimized dependencies as root-relative
// module URLs even when its HTML is served from a nested base path.
func rewriteDevelopmentModuleImports(response *http.Response, mount string, viteClient bool) {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "javascript") || response.ContentLength > developmentModuleRewriteLimit {
		return
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, developmentModuleRewriteLimit+1))
	if err != nil {
		return
	}
	if len(body) > developmentModuleRewriteLimit {
		response.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), response.Body))
		return
	}
	_ = response.Body.Close()
	rewritten := string(body)
	for _, pattern := range developmentModuleImportPatterns {
		rewritten = pattern.ReplaceAllString(rewritten, "${1}${2}"+mount)
	}
	if viteClient {
		rewritten = developmentViteHMRProxyPreamble(mount) + rewritten
	}
	response.Body = io.NopCloser(strings.NewReader(rewritten))
	response.ContentLength = int64(len(rewritten))
	response.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
}

func developmentViteHMRProxyPreamble(mount string) string {
	return `(function(){var NativeWebSocket=globalThis.WebSocket;if(typeof NativeWebSocket!=="function")return;globalThis.WebSocket=new Proxy(NativeWebSocket,{construct:function(Target,args){try{var target=new URL(String(args[0]),location.href);if(target.origin===location.origin&&target.pathname==="/"&&target.searchParams.has("token")){target.pathname=` + strconv.Quote(mount) + `;args[0]=target.toString()}}catch(_){ }return Reflect.construct(Target,args)}})})();` + "\n"
}

var _ io.Writer = (*pluginDevelopmentLog)(nil)
