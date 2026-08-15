package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"

	"alemonx/internal/access"
	"alemonx/internal/robot"
	"alemonx/internal/setupplugin"
	"alemonx/internal/system"
	"alemonx/internal/systemnetwork"

	"golang.org/x/net/websocket"
)

var errInternalTest = errors.New("internal test failure")

// newIPv4TestServer keeps tests portable in sandboxes that disallow the Go
// httptest default IPv6 loopback listener. TEST_LISTEN_ADDR remains overridable
// for CI environments with dedicated network namespaces.
func newIPv4TestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	addr := os.Getenv("TEST_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("test listener %s: %v", addr, err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func TestUpdateStatusReportsIdleAndPersistedTransaction(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	handler := newTestServer()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/update/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"phase":"idle"`) {
		t.Fatalf("idle update status = %d %s", response.Code, response.Body.String())
	}
	if err := system.SaveUpdateTransaction(system.UpdateTransaction{Phase: system.UpdatePhaseStaged, TargetVersion: "v1.2.3"}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/update/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"phase":"staged"`) || !strings.Contains(response.Body.String(), `"targetVersion":"v1.2.3"`) {
		t.Fatalf("staged update status = %d %s", response.Code, response.Body.String())
	}
}

func TestSystemNetworkSettingsSaveWithoutLeakingProxyCredentials(t *testing.T) {
	manager, err := systemnetwork.NewAt(filepath.Join(t.TempDir(), "network.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &server{network: manager}

	update := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/system/network",
		strings.NewReader(`{"routes":{"github":{"mode":"manual","proxyUrl":"http://name:secret@127.0.0.1:7890"}}}`),
	)
	recorder := httptest.NewRecorder()
	s.systemNetworkHandler(recorder, update)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save network settings = %d %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("proxy credential leaked from API: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"hasCredentials":true`) {
		t.Fatalf("credential presence missing from API: %s", recorder.Body.String())
	}

	read := httptest.NewRecorder()
	s.systemNetworkHandler(read, httptest.NewRequest(http.MethodGet, "/api/v1/system/network", nil))
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "secret") || !strings.Contains(read.Body.String(), `"proxyUrl":"http://127.0.0.1:7890"`) {
		t.Fatalf("read network settings = %d %s", read.Code, read.Body.String())
	}
}

func TestGoalsApplyOfficialDownloadMirror(t *testing.T) {
	manager, err := systemnetwork.NewAt(filepath.Join(t.TempDir(), "network.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(systemnetwork.Settings{Routes: map[systemnetwork.Route]systemnetwork.RouteSettings{
		systemnetwork.RouteOfficial: {Mode: systemnetwork.ModeCustomMirror, MirrorURL: "https://download-mirror.example/{url}"},
	}}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	(&server{network: manager}).listGoals(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("goals status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "https://download-mirror.example/https://download.alemonjs.com/application/alemonapp/app-universal-release.apk") {
		t.Fatalf("official mirror missing from goals: %s", recorder.Body.String())
	}
}

func TestGitHubDownloadGuideUsesSystemMirrorPresets(t *testing.T) {
	presets := systemnetwork.MirrorPresets(systemnetwork.RouteGitHub)
	mirrors := githubMirrors("alx")
	if len(mirrors) != len(presets)+1 {
		t.Fatalf("guide mirrors = %#v, presets = %#v", mirrors, presets)
	}
	if mirrors[0].Name != "GitHub 加速（"+presets[0].Label+"）" || !strings.Contains(mirrors[0].URL, "ghfast.top/") {
		t.Fatalf("recommended guide mirror = %#v", mirrors[0])
	}
	if mirrors[len(mirrors)-1].Name != "GitHub 官方" {
		t.Fatalf("official guide mirror = %#v", mirrors[len(mirrors)-1])
	}
}

func newTestServer() http.Handler {
	return NewServer("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}})
}

// TestLoggableRequestBodyRedactsSecretsAndRestoresStream ensures the request
// logger prints the action for the console while redacting tokens/passwords
// and keeping the body readable by the downstream handler.
func TestLoggableRequestBodyRedactsSecretsAndRestoresStream(t *testing.T) {
	s := newStatefulTestServer()
	payload := `{"action":"npm-publish","token":"sekrit","sudoPassword":"also-sekrit","package":"@alemonjs/onebot","values":{"password":"nested-sekrit"}}`
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/robot", bytes.NewBufferString(payload))
	logged := s.loggableRequestBody(ginCtx)
	if strings.Contains(logged, "sekrit") {
		t.Fatalf("secret leaked into log: %s", logged)
	}
	if !strings.Contains(logged, `"action":"npm-publish"`) {
		t.Fatalf("action missing from log: %s", logged)
	}
	if !strings.Contains(logged, `"[REDACTED]"`) {
		t.Fatalf("secrets not redacted: %s", logged)
	}
	// The body must still be readable by the handler after logging.
	after, err := io.ReadAll(ginCtx.Request.Body)
	if err != nil || string(after) != payload {
		t.Fatalf("body not restored: %q, %v", after, err)
	}
}

// Large uploads must not be consumed or truncated by the request logger: a
// previous implementation capped the read at 16 KiB and then replaced the body
// with only those bytes, silently breaking every multipart upload bigger than
// that (ParseMultipartForm would fail on the truncated stream).
func TestLoggableRequestBodyLeavesLargeBodyUntouched(t *testing.T) {
	s := newStatefulTestServer()
	large := bytes.Repeat([]byte("x"), 256*1024)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/update/load", bytes.NewBuffer(large))
	ginCtx.Request.ContentLength = int64(len(large))
	if logged := s.loggableRequestBody(ginCtx); logged != "" {
		t.Fatalf("large body should not be logged: %q", logged)
	}
	after, err := io.ReadAll(ginCtx.Request.Body)
	if err != nil || !bytes.Equal(after, large) {
		t.Fatalf("large body truncated: got %d bytes, want %d (err %v)", len(after), len(large), err)
	}
}

func TestLoggableQueryRedactsCredentials(t *testing.T) {
	values := url.Values{
		"root":  {"/tmp/example"},
		"token": {"sekrit"},
		"tag":   {"stable", "latest"},
	}
	logged := loggableQuery(values)
	if logged["token"] != "[REDACTED]" {
		t.Fatalf("token = %#v", logged["token"])
	}
	if logged["root"] != "/tmp/example" {
		t.Fatalf("root = %#v", logged["root"])
	}
	tags, ok := logged["tag"].([]string)
	if !ok || len(tags) != 2 {
		t.Fatalf("tag = %#v", logged["tag"])
	}
}

func TestCaptureWriterBuffersFailedResponseBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	captured := &captureWriter{ResponseWriter: ginCtx.Writer}
	// Pass through a small error payload and confirm the buffer captures it.
	if _, err := captured.Write([]byte(`{"error":"无法读取发布校验文件，请检查网络后重试"}`)); err != nil {
		t.Fatal(err)
	}
	if got := captured.message(); !strings.Contains(got, "请检查网络后重试") {
		t.Fatalf("captured message = %q", got)
	}
	// A very large payload must be truncated so the console stays readable.
	ginCtx2, _ := gin.CreateTestContext(httptest.NewRecorder())
	captured2 := &captureWriter{ResponseWriter: ginCtx2.Writer}
	big := bytes.Repeat([]byte("x"), 64*1024)
	if _, err := captured2.Write(big); err != nil {
		t.Fatal(err)
	}
	if len(captured2.message()) > 400 {
		t.Fatalf("captured message not bounded: %d bytes", len(captured2.message()))
	}
}

func newSudoActionTestServer(t *testing.T, run func(context.Context, []byte, string, []string) (string, error)) (*server, string) {
	t.Helper()
	if err := system.ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "fixture-system")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture-system","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"},"privilegedOperations":[{"action":"prepare-runtime","runnerAction":"prepare-runtime-runner","title":"准备系统运行环境","description":"安装插件需要的系统运行环境。","authorization":"password","platforms":["` + runtime.GOOS + `"],"commands":[{"program":"go","args":["version"]}]}]}`
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := identity.Enable("root", "test-password", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.close)
	return &server{
		plugins:              setupplugin.NewRegistry(root),
		auth:                 identity,
		operations:           []operationTask{},
		events:               newRobotEventHub(),
		sudoAttempts:         map[string]sudoAttempt{},
		privilegeStore:       store,
		runPrivilegedCommand: run,
	}, token
}

func sudoActionRequest(t *testing.T, s *server, token, remote, password string, confirm bool) *http.Request {
	t.Helper()
	host, _, _ := net.SplitHostPort(remote)
	intent, err := s.privilegeStore.createIntent("fixture-system", "prepare-runtime", "", "root", host, "password")
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"action":"prepare-runtime","confirm":%t,"sudoPassword":%q,"authorizationId":%q}`, confirm, password, intent.ID)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/fixture-system/actions", strings.NewReader(body))
	request.RemoteAddr = remote
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	return request
}

func waitForSetupOperation(t *testing.T, s *server) operationTask {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		if len(s.operations) != 0 && s.operations[0].Status != "running" {
			operation := s.operations[0]
			s.mu.RUnlock()
			return operation
		}
		s.mu.RUnlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("sudo setup operation did not finish")
	return operationTask{}
}

func TestAppendOperationStepKeepsBoundedDistinctTimeline(t *testing.T) {
	task := operationTask{}
	appendOperationStep(&task, 20, "下载官方运行时")
	appendOperationStep(&task, 20, "下载官方运行时")
	if len(task.Steps) != 1 {
		t.Fatalf("duplicate step count = %d, want 1", len(task.Steps))
	}
	for index := 0; index < operationStepLimit+3; index++ {
		appendOperationStep(&task, index, fmt.Sprintf("阶段 %d", index))
	}
	if len(task.Steps) != operationStepLimit {
		t.Fatalf("step count = %d, want %d", len(task.Steps), operationStepLimit)
	}
	if task.Steps[len(task.Steps)-1].Message != fmt.Sprintf("阶段 %d", operationStepLimit+2) {
		t.Fatalf("last step = %#v", task.Steps[len(task.Steps)-1])
	}
}

func TestAppendOperationStepUpdatesActiveDownloadLine(t *testing.T) {
	task := operationTask{}
	appendOperationStep(&task, 20, "下载官方运行时（1 MB / 10 MB）")
	appendOperationStep(&task, 25, "下载官方运行时（2 MB / 10 MB）")
	if len(task.Steps) != 1 || task.Steps[0].Progress != 25 || task.Steps[0].Message != "下载官方运行时（2 MB / 10 MB）" {
		t.Fatalf("download updates must replace one line: %#v", task.Steps)
	}
}

func TestActiveSetupPluginOperationUsesPluginScope(t *testing.T) {
	operations := []operationTask{
		{ID: "finished", Action: "setup:alemonx-qq:luckylillia-install", Status: "completed"},
		{ID: "other", Action: "setup:alemonx-network:plan", Status: "running"},
		{ID: "active", Action: "setup:alemonx-qq:luckylillia-install", Status: "running"},
	}
	active := activeSetupPluginOperation(operations, "alemonx-qq")
	if active == nil || active.ID != "active" {
		t.Fatalf("active QQ operation = %#v", active)
	}
	if active := activeSetupPluginOperation(operations, "missing"); active != nil {
		t.Fatalf("missing plugin operation = %#v", active)
	}
}

func TestManifestSudoActionRequiresLocalSuperAdminAndConfirmation(t *testing.T) {
	s, token := newSudoActionTestServer(t, func(context.Context, []byte, string, []string) (string, error) {
		t.Fatal("sudo executor must not run when the request is rejected")
		return "", nil
	})
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := system.ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t.Setenv("ALX_PRIVILEGED_MODE", "disabled")
		_ = system.ConfigurePrivilegedMode("127.0.0.1", false)
	})
	if _, err := s.auth.CreateAccount("operator", "operator-password", "operator-password", nil); err != nil {
		t.Fatal(err)
	}
	operatorToken, err := s.auth.Login("operator", "operator-password")
	if err != nil {
		t.Fatal(err)
	}
	for name, request := range map[string]*http.Request{
		"remote":           sudoActionRequest(t, s, token, "203.0.113.10:4242", "password", true),
		"unconfirmed":      sudoActionRequest(t, s, token, "127.0.0.1:4242", "password", false),
		"unauthenticated":  sudoActionRequest(t, s, "", "127.0.0.1:4242", "password", true),
		"ordinary-account": sudoActionRequest(t, s, operatorToken, "127.0.0.1:4242", "password", true),
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			s.setupPluginActionHandler(recorder, request)
			if recorder.Code != http.StatusForbidden && recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestManifestSudoActionAllowsLocalOperationWhenAuthenticationIsUnset(t *testing.T) {
	s, _ := newSudoActionTestServer(t, func(context.Context, []byte, string, []string) (string, error) {
		return "", nil
	})
	// Replace the configured test account with an untouched manager: this is
	// the first-run state, where no ALX account system has been configured.
	unauthenticated, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.auth = unauthenticated
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := system.ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t.Setenv("ALX_PRIVILEGED_MODE", "disabled")
		_ = system.ConfigurePrivilegedMode("127.0.0.1", false)
	})

	intent, err := s.privilegeStore.createIntent("fixture-system", "prepare-runtime", "", "local", "127.0.0.1", "password")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/fixture-system/actions", strings.NewReader(fmt.Sprintf(`{"action":"prepare-runtime","confirm":true,"sudoPassword":"password","authorizationId":%q}`, intent.ID)))
	request.RemoteAddr = "127.0.0.1:4242"
	recorder := httptest.NewRecorder()
	s.setupPluginActionHandler(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unset authentication must allow local system operation, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestManifestSudoActionUsesTransientPasswordAndLocksAfterThreeFailures(t *testing.T) {
	var received []byte
	s, token := newSudoActionTestServer(t, func(_ context.Context, password []byte, program string, args []string) (string, error) {
		if program != "go" || !reflect.DeepEqual(args, []string{"version"}) {
			t.Fatalf("command = %s %#v", program, args)
		}
		received = password
		return "", system.ErrSudoPasswordInvalid
	})
	for attempt := 0; attempt < 3; attempt++ {
		recorder := httptest.NewRecorder()
		s.setupPluginActionHandler(recorder, sudoActionRequest(t, s, token, "127.0.0.1:4242", "only-once", true))
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("attempt %d status = %d, body=%s", attempt+1, recorder.Code, recorder.Body.String())
		}
		operation := waitForSetupOperation(t, s)
		if operation.Error != system.ErrSudoPasswordInvalid.Error() || strings.Contains(operation.Error, "only-once") || strings.Contains(operation.Output, "only-once") {
			t.Fatalf("unsafe sudo operation result: %#v", operation)
		}
	}
	if len(received) == 0 {
		t.Fatal("sudo executor was not called")
	}
	for _, value := range received {
		if value != 0 {
			t.Fatal("host retained transient sudo password bytes")
		}
	}
	recorder := httptest.NewRecorder()
	s.setupPluginActionHandler(recorder, sudoActionRequest(t, s, token, "127.0.0.1:4242", "only-once", true))
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("locked attempt status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestManifestSudoActionRequiresAuthorizationIntent(t *testing.T) {
	s, token := newSudoActionTestServer(t, func(context.Context, []byte, string, []string) (string, error) {
		t.Fatal("sudo executor must not run without a preflight intent")
		return "", nil
	})
	body := `{"action":"prepare-runtime","confirm":true,"sudoPassword":"not-forwarded"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/fixture-system/actions", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:4242"
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	recorder := httptest.NewRecorder()
	s.setupPluginActionHandler(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "工作台确认") {
		t.Fatalf("missing intent = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPrivilegePreflightExplainsRemoteRestriction(t *testing.T) {
	s, token := newSudoActionTestServer(t, func(context.Context, []byte, string, []string) (string, error) { return "", nil })
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := system.ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t.Setenv("ALX_PRIVILEGED_MODE", "disabled")
		_ = system.ConfigurePrivilegedMode("127.0.0.1", false)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/privileged/preflight", strings.NewReader(`{"pluginId":"fixture-system","action":"prepare-runtime"}`))
	request.RemoteAddr = "203.0.113.10:4242"
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	recorder := httptest.NewRecorder()
	s.privilegedPreflightHandler(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"available":false`) || !strings.Contains(recorder.Body.String(), "ALX_PRIVILEGED_MODE") {
		t.Fatalf("remote preflight = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestNormalPluginActionRejectsSudoPassword(t *testing.T) {
	s := newStatefulTestServer()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/setup/plugins/example/actions", strings.NewReader(`{"action":"status","sudoPassword":"never-forward"}`))
	s.setupPluginActionHandler(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "不接受系统管理员密码") {
		t.Fatalf("normal action password rejection = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestLocalPrivilegeRequestRejectsProxyHeaders(t *testing.T) {
	t.Setenv("ALX_PRIVILEGED_MODE", "local")
	if err := system.ConfigurePrivilegedMode("127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	s := newStatefulTestServer()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/privileged/preflight", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	if !s.privilegedRequestAllowed(request) {
		t.Fatal("direct loopback request must be local")
	}
	request.Header.Set("X-Forwarded-For", "203.0.113.8")
	if s.privilegedRequestAllowed(request) {
		t.Fatal("proxied request must not be treated as a local privilege request")
	}
}

func TestEnabledPrivilegeModeAllowsRemoteAdministratorRequest(t *testing.T) {
	s, _ := newSudoActionTestServer(t, func(context.Context, []byte, string, []string) (string, error) { return "", nil })
	t.Setenv("ALX_PRIVILEGED_MODE", "enabled")
	if err := system.ConfigurePrivilegedMode("0.0.0.0", true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		t.Setenv("ALX_PRIVILEGED_MODE", "disabled")
		_ = system.ConfigurePrivilegedMode("127.0.0.1", false)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/privileged/preflight", nil)
	request.RemoteAddr = "203.0.113.10:4242"
	if !s.privilegedRequestAllowed(request) {
		t.Fatal("enabled mode must allow a remote administrator request")
	}
}

// TestOperationWriterBuffersPartialLines ensures chunked writes that split a
// newline are only appended as complete lines, matching how a supervised
// process may emit output in arbitrary chunk sizes.
func TestOperationWriterBuffersPartialLines(t *testing.T) {
	s := newStatefulTestServer()
	s.operations = []operationTask{{ID: "dev-1", Output: ""}}
	writer := newOperationWriter("dev-1", s)

	// A chunk that splits "hello\n" across writes.
	if _, err := writer.Write([]byte("hel")); err != nil {
		t.Fatal(err)
	}
	if got := s.operations[0].Output; got != "" {
		t.Fatalf("partial line leaked before newline: %q", got)
	}
	if _, err := writer.Write([]byte("lo\n")); err != nil {
		t.Fatal(err)
	}
	if got := s.operations[0].Output; got != "hello\n" {
		t.Fatalf("output = %q, want hello\n", got)
	}
	// Multiple complete lines in one write.
	if _, err := writer.Write([]byte("a\nb\n")); err != nil {
		t.Fatal(err)
	}
	if got := s.operations[0].Output; got != "hello\na\nb\n" {
		t.Fatalf("output = %q, want three lines", got)
	}
}

// newStatefulTestServer builds a server whose internal maps are populated so
// tests can exercise the console, stop and mutual-exclusion paths directly
// without a real PM2 daemon or a listening listener.
func newStatefulTestServer() *server {
	return &server{
		robots:         robot.Manager{},
		operations:     []operationTask{},
		development:    map[string]developmentProcess{},
		stopping:       map[string]bool{},
		consoleCache:   map[string]consoleSnapshot{},
		pm2Status:      func(string) (robot.PM2Status, error) { return robot.PM2Status{}, nil },
		directoryRoots: managedDirectoryRoots(),
		events:         newRobotEventHub(),
		liveUploads:    map[string]liveUpload{},
	}
}

func TestSystemPickerUsesOnlyDeclaredWebFinderPicker(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "alemonx-qq")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"systemPickers":[{"id":"napcat-directory","kind":"directory","title":"选择现有 NapCat 安装目录"}]}`
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := &server{
		plugins: setupplugin.NewRegistry(root),
		auth:    identity,
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/capabilities/finder", strings.NewReader(`{"pluginId":"alemonx-qq","pickerId":"napcat-directory"}`))
	response := httptest.NewRecorder()
	s.systemCapabilityFinderHandler(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kind":"directory"`) || !strings.Contains(response.Body.String(), "选择现有 NapCat 安装目录") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}

	remote := httptest.NewRequest(http.MethodPost, "/api/v1/system/picker", strings.NewReader(`{"pluginId":"alemonx-qq","pickerId":"napcat-directory"}`))
	response = httptest.NewRecorder()
	s.systemPickerHandler(response, remote)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pickerId":"napcat-directory"`) {
		t.Fatalf("compatibility picker = %d %s", response.Code, response.Body.String())
	}

	unknown := httptest.NewRequest(http.MethodPost, "/api/v1/system/picker", strings.NewReader(`{"pluginId":"alemonx-qq","pickerId":"anything-else"}`))
	response = httptest.NewRecorder()
	s.systemCapabilityFinderHandler(response, unknown)
	if response.Code != http.StatusForbidden {
		t.Fatalf("undeclared picker = %d %s", response.Code, response.Body.String())
	}
}

func TestEnvironmentInstallRequiresConfirmationAndUsesFixedCheckID(t *testing.T) {
	called := ""
	s := &server{
		installEnvironment: func(_ context.Context, checkID string) (string, error) {
			called = checkID
			return "installed", nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/environment/install", strings.NewReader(`{"checkId":"node","confirm":true}`))
	response := httptest.NewRecorder()
	s.environmentInstallHandler(response, request)
	if response.Code != http.StatusOK || called != "node" || !strings.Contains(response.Body.String(), "installed") {
		t.Fatalf("response=%d %s called=%q", response.Code, response.Body.String(), called)
	}

	called = ""
	request = httptest.NewRequest(http.MethodPost, "/api/v1/system/environment/install", strings.NewReader(`{"checkId":"git","confirm":false}`))
	response = httptest.NewRecorder()
	s.environmentInstallHandler(response, request)
	if response.Code != http.StatusBadRequest || called != "" {
		t.Fatalf("unconfirmed response=%d %s called=%q", response.Code, response.Body.String(), called)
	}
}

func TestPluginContextCapabilityIsBuiltInAndSanitized(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "context-plugin")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"context-plugin","name":"Context","version":"1.0.0","web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	network, err := systemnetwork.NewAt(filepath.Join(t.TempDir(), "network.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := network.Save(systemnetwork.Settings{Routes: map[systemnetwork.Route]systemnetwork.RouteSettings{systemnetwork.RouteGitHub: {Mode: systemnetwork.ModeManual, ProxyURL: "http://name:secret@127.0.0.1:7890"}}}); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(root), network: network, hostContexts: map[string]pluginHostContext{"local": {RobotRoot: "/robots/demo"}}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/capabilities/context?pluginId=context-plugin&keys=robot,network", nil)
	response := httptest.NewRecorder()
	s.systemCapabilityContextHandler(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"root":"/robots/demo"`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("context = %d %s", response.Code, response.Body.String())
	}

	denied := httptest.NewRecorder()
	s.systemCapabilityContextHandler(denied, httptest.NewRequest(http.MethodGet, "/api/v1/system/capabilities/context?pluginId=context-plugin&keys=finder", nil))
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("unsupported context = %d %s", denied.Code, denied.Body.String())
	}
}

func TestSystemCapabilityCatalogAndPluginIdentity(t *testing.T) {
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "capability-plugin")
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(`{"id":"capability-plugin","name":"Capability","version":"1.0.0","web":{"root":"web"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(root)}
	response := httptest.NewRecorder()
	s.systemCapabilitiesHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/capabilities", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "network.fetch") || !strings.Contains(response.Body.String(), "finder.pick") {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.systemCapabilityInfoHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/capabilities/info?pluginId=missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown plugin capability = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	s.systemCapabilityInfoHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/capabilities/info?pluginId=capability-plugin", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "architecture") {
		t.Fatalf("system info = %d %s", response.Code, response.Body.String())
	}
}

func TestPluginWebInjectsHostCapabilityBridge(t *testing.T) {
	content := rewriteSetupPluginWebHTML("<html><head></head><body>ok</body></html>")
	if !strings.Contains(content, `host-bridge.js`) || strings.Contains(content, `finder-bridge.js`) {
		t.Fatalf("plugin bridge injection = %q", content)
	}
	bridge := setupPluginHostBridge()
	for _, want := range []string{"window.ALXHost", "desktop:{open", "clipboard:{read", "network:{fetch", "finder-request"} {
		if !strings.Contains(bridge, want) {
			t.Fatalf("host bridge missing %q", want)
		}
	}
}

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRobotLiveUploadIsBoundToRobotAndDeviceAndCleansUp(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("root", root); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("deviceId", "alemonx-live-test-upload"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello QQ")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/robot/live/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	s.robotLiveUploadHandler(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", response.Code, response.Body.String())
	}
	var uploaded struct {
		UploadID string `json:"uploadId"`
		Path     string `json:"path"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.UploadID == "" || uploaded.Filename != "hello.txt" || !strings.HasPrefix(uploaded.Path, filepath.Join(root, ".alemonx-live-uploads")) {
		t.Fatalf("unexpected upload response: %+v", uploaded)
	}
	if _, err := os.Stat(uploaded.Path); err != nil {
		t.Fatalf("staged file missing: %v", err)
	}
	wrongBody := strings.NewReader(`{"root":` + strconv.Quote(root) + `,"deviceId":"alemonx-live-other-device","uploadId":` + strconv.Quote(uploaded.UploadID) + `}`)
	wrong := httptest.NewRecorder()
	s.robotLiveUploadHandler(wrong, httptest.NewRequest(http.MethodDelete, "/api/v1/robot/live/upload", wrongBody))
	if wrong.Code != http.StatusNotFound {
		t.Fatalf("cross-device cleanup = %d %s", wrong.Code, wrong.Body.String())
	}
	if _, err := os.Stat(uploaded.Path); err != nil {
		t.Fatalf("cross-device cleanup removed file: %v", err)
	}
	deleteBody := strings.NewReader(`{"root":` + strconv.Quote(root) + `,"deviceId":"alemonx-live-test-upload","uploadId":` + strconv.Quote(uploaded.UploadID) + `}`)
	deleted := httptest.NewRecorder()
	s.robotLiveUploadHandler(deleted, httptest.NewRequest(http.MethodDelete, "/api/v1/robot/live/upload", deleteBody))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("cleanup = %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(uploaded.Path); !os.IsNotExist(err) {
		t.Fatalf("staged file still exists: %v", err)
	}
}

func TestRobotLiveUploadRejectsEmptyFile(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("root", root); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("deviceId", "alemonx-live-empty-upload"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.CreateFormFile("file", "empty.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/robot/live/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	s.robotLiveUploadHandler(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "文件为空") {
		t.Fatalf("empty upload = %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".alemonx-live-uploads")); err == nil {
		entries, readErr := os.ReadDir(filepath.Join(root, ".alemonx-live-uploads"))
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("empty upload left a staged file: %v", entries)
		}
	}
}

// TestRuntimeProcessOutputPrefersRunningProcess covers the case where both a
// dev and an app run have history: the currently running one must win, and a
// fresh dev run after an older foreground run must not be hidden.
func TestRuntimeProcessOutputPrefersRunningProcess(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	finished := time.Now()
	s.operations = []operationTask{
		{ID: "dev-new", Root: root, Action: "dev", Status: "running", Output: "dev live"},
		{ID: "app-old", Root: root, Action: "app", Status: "completed", Output: "app old", FinishedAt: &finished},
	}
	output, status, _, mode := s.runtimeProcessOutput(root)
	if status != "running" || output != "dev live" || mode != "开发模式" {
		t.Fatalf("running process not preferred: output=%q status=%q mode=%q", output, status, mode)
	}

	// No running process: fall back to the newest history (newest-first list).
	s.operations = []operationTask{
		{ID: "app-new", Root: root, Action: "app", Status: "completed", Output: "app new", FinishedAt: &finished},
		{ID: "dev-old", Root: root, Action: "dev", Status: "completed", Output: "dev old", FinishedAt: &finished},
	}
	output, status, _, mode = s.runtimeProcessOutput(root)
	if status != "completed" || output != "app new" || mode != "前台运行" {
		t.Fatalf("newest history not returned: output=%q status=%q mode=%q", output, status, mode)
	}
}

func TestRobotConsoleSeparatesSnapshotAndOutput(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"bot","version":"1.0.0","scripts":{"dev":"node index.js"}}`)
	writeFixture(t, root, "index.js", "console.log('hi')\n")
	s := newStatefulTestServer()
	s.operations = []operationTask{
		{ID: "dev-1", Root: root, Action: "dev", Status: "running", Output: "ready line"},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/robot/console?root="+url.QueryEscape(root), nil)
	s.robotConsoleHandler(recorder, request)

	var payload consolePayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !payload.Running {
		t.Fatalf("running = false, want true for a live dev task")
	}
	if !strings.Contains(payload.Output, "开发模式实时输出") || !strings.Contains(payload.Output, "ready line") {
		t.Fatalf("output = %q, want live dev output", payload.Output)
	}
	if !strings.Contains(payload.Snapshot, "$ pwd") || !strings.Contains(payload.Snapshot, "$ package.json") {
		t.Fatalf("snapshot = %q, want static project context", payload.Snapshot)
	}
	if payload.Mode != "开发模式" {
		t.Fatalf("mode = %q, want 开发模式", payload.Mode)
	}
}

func TestRobotConsoleRefreshBypassesSnapshotCache(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"bot","version":"1.0.0"}`)
	s := newStatefulTestServer()
	// Prime the cache with stale content; a non-refresh poll must return it.
	s.consoleCache[root] = consoleSnapshot{output: "STALE-SNAPSHOT", at: time.Now()}

	plain := httptest.NewRecorder()
	s.robotConsoleHandler(plain, httptest.NewRequest(http.MethodGet, "/api/v1/robot/console?root="+url.QueryEscape(root), nil))
	var stalePayload consolePayload
	if err := json.Unmarshal(plain.Body.Bytes(), &stalePayload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stalePayload.Snapshot, "STALE-SNAPSHOT") {
		t.Fatalf("non-refresh poll did not reuse cache: %q", stalePayload.Snapshot)
	}

	fresh := httptest.NewRecorder()
	s.robotConsoleHandler(fresh, httptest.NewRequest(http.MethodGet, "/api/v1/robot/console?root="+url.QueryEscape(root)+"&refresh=1", nil))
	var freshPayload consolePayload
	if err := json.Unmarshal(fresh.Body.Bytes(), &freshPayload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(freshPayload.Snapshot, "STALE-SNAPSHOT") {
		t.Fatalf("refresh=1 reused stale snapshot: %q", freshPayload.Snapshot)
	}
	if !strings.Contains(freshPayload.Snapshot, "$ pwd") {
		t.Fatalf("refresh did not regenerate snapshot: %q", freshPayload.Snapshot)
	}
}

func TestRobotConsoleShowsExitReasonOnFailure(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"bot","version":"1.0.0"}`)
	s := newStatefulTestServer()
	finished := time.Now()
	s.operations = []operationTask{
		{ID: "dev-1", Root: root, Action: "dev", Status: "failed", Output: "boom\n", Error: "开发进程已退出：exit status 1", FinishedAt: &finished},
	}

	recorder := httptest.NewRecorder()
	s.robotConsoleHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/robot/console?root="+url.QueryEscape(root), nil))
	var payload consolePayload
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload.Output, "开发进程已退出：exit status 1") {
		t.Fatalf("output misses exit reason: %q", payload.Output)
	}
	if payload.Running {
		t.Fatalf("failed task must not be reported as running")
	}
}

func TestLocalStartBlockedByPM2Running(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.pm2Status = func(string) (robot.PM2Status, error) {
		return robot.PM2Status{Configured: true, Managed: true, Running: true, Status: "online"}, nil
	}
	message, blocked := s.localStartBlockedByPM2(root)
	if !blocked || !strings.Contains(message, "后台（PM2）运行") {
		t.Fatalf("blocked = %v, message = %q", blocked, message)
	}
}

func TestLocalStartNotBlockedWhenPM2Idle(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	if _, blocked := s.localStartBlockedByPM2(root); blocked {
		t.Fatalf("idle PM2 must not block local start")
	}
}

// TestLocalStartBlockedWhenPM2StatusUnreadable is the strict behaviour: a PM2
// state that cannot be read (daemon mismatch, timeout) must still block a local
// start, otherwise a running PM2 service would silently share the app port.
func TestLocalStartBlockedWhenPM2StatusUnreadable(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.pm2Status = func(string) (robot.PM2Status, error) {
		return robot.PM2Status{}, errInternalTest
	}
	message, blocked := s.localStartBlockedByPM2(root)
	if !blocked || !strings.Contains(message, "无法读取后台（PM2）服务状态") {
		t.Fatalf("unreadable PM2 state must block local start, blocked=%v msg=%q", blocked, message)
	}
}

func TestPM2StartBlockedByLocalRunning(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.development[root] = developmentProcess{TaskID: "dev-1"}
	message, blocked := s.pm2StartBlockedByLocal(root)
	if !blocked || !strings.Contains(message, "本机（开发/前台）运行") {
		t.Fatalf("blocked = %v, message = %q", blocked, message)
	}
	delete(s.development, root)
	if _, blocked := s.pm2StartBlockedByLocal(root); blocked {
		t.Fatalf("no local process must not block PM2 start")
	}
}

func TestStopDevelopmentWithoutProcessFinishesImmediately(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	if s.stopDevelopment(root, "开发模式") {
		t.Fatalf("stopDevelopment reported a running process that does not exist")
	}
}

func TestWaitForManagedProcessExitTimesOutWhileStopping(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.mu.Lock()
	s.stopping[root] = true
	s.mu.Unlock()

	original := managedProcessStopTimeout
	managedProcessStopTimeout = 150 * time.Millisecond
	defer func() { managedProcessStopTimeout = original }()

	// The channel never receives: this simulates a Windows descendant that
	// inherited the output pipe and keeps command.Wait() blocked.
	blocked := make(chan error)
	started := time.Now()
	err, timedOut := s.waitForManagedProcessExit(root, blocked)
	if !timedOut || err == nil {
		t.Fatalf("waitForManagedProcessExit = (%v, %v), want a timeout error", err, timedOut)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("stop wait took %v, expected the bounded deadline", elapsed)
	}
}

func TestWaitForManagedProcessExitReturnsWhenProcessExits(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.mu.Lock()
	s.stopping[root] = true
	s.mu.Unlock()

	finished := make(chan error, 1)
	finished <- errors.New("process exited")
	err, timedOut := s.waitForManagedProcessExit(root, finished)
	if timedOut || err == nil || err.Error() != "process exited" {
		t.Fatalf("waitForManagedProcessExit = (%v, %v), want the exit error", err, timedOut)
	}
}

func TestCompletePendingStopTasks(t *testing.T) {
	root := t.TempDir()
	finished := time.Now()
	s := newStatefulTestServer()
	s.operations = []operationTask{
		{ID: "stop-1", Root: root, Action: "dev-stop", Status: "running"},
		{ID: "stop-2", Root: root, Action: "app-stop", Status: "running"},
		{ID: "other-root", Root: "/elsewhere", Action: "dev-stop", Status: "running"},
		{ID: "install-1", Root: root, Action: "install", Status: "running"},
	}
	s.completePendingStopTasks(root, finished)
	byID := map[string]operationTask{}
	for _, item := range s.operations {
		byID[item.ID] = item
	}
	if byID["stop-1"].Status != "completed" || !strings.Contains(byID["stop-1"].Output, "已停止开发模式") {
		t.Fatalf("stop-1 = %+v, want completed 开发模式", byID["stop-1"])
	}
	if byID["stop-2"].Status != "completed" || !strings.Contains(byID["stop-2"].Output, "已停止前台运行") {
		t.Fatalf("stop-2 = %+v, want completed 前台运行", byID["stop-2"])
	}
	if byID["other-root"].Status != "running" {
		t.Fatalf("a stop task on another root must stay running")
	}
	if byID["install-1"].Status != "running" {
		t.Fatalf("a non-stop task must stay running")
	}
}

func TestSettleUnmanagedLocalOperationsClearsForegroundAndDevelopmentState(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.operations = []operationTask{
		{ID: "app", Root: root, Action: "app", Status: "running"},
		{ID: "dev", Root: root, Action: "dev", Status: "running"},
		{ID: "app-stop", Root: root, Action: "app-stop", Status: "running"},
		{ID: "install", Root: root, Action: "install", Status: "running"},
		{ID: "other", Root: "/other", Action: "app", Status: "running"},
	}
	s.settleUnmanagedLocalOperations(root, "已停止")
	byID := map[string]operationTask{}
	for _, item := range s.operations {
		byID[item.ID] = item
	}
	for _, id := range []string{"app", "dev", "app-stop"} {
		if byID[id].Status != "completed" || byID[id].FinishedAt == nil {
			t.Fatalf("%s = %+v, want completed local lifecycle task", id, byID[id])
		}
	}
	if byID["install"].Status != "running" || byID["other"].Status != "running" {
		t.Fatalf("unrelated tasks must remain running: install=%+v other=%+v", byID["install"], byID["other"])
	}
}

func TestWebViewHTMLRewriteAndRestrictedBridge(t *testing.T) {
	html := rewriteWebViewHTML(`<!doctype html><head><link href="/favicon.ico"><link href="/assets/app.css"></head><body><script src="/assets/app.js"></script></body>`)
	for _, expected := range []string{`href="favicon.ico"`, `href="assets/app.css"`, `src="assets/app.js"`, `<script src="bridge.js"></script></head>`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rewritten HTML misses %q: %s", expected, html)
		}
	}
	bridge := webViewBridge(robot.WebViewEntry{Package: `plugin"name`, Name: "页面"})
	for _, expected := range []string{`window.__alxWebview`, `./api/`, `plugin\"name`, `window.appDesktopAPI`, `'message'`, `'events'`, `'api-error'`, `response.clone().json()`} {
		if !strings.Contains(bridge, expected) {
			t.Fatalf("bridge misses %q", expected)
		}
	}
	if strings.Contains(bridge, "WailsInvoke") || strings.Contains(bridge, "Shell") {
		t.Fatalf("bridge must not expose desktop privileges: %s", bridge)
	}
	for _, expected := range []string{"@alemonjs/process", "events[data.type]", "process.stdin.on"} {
		if !strings.Contains(defaultWebViewDesktopScript, expected) {
			t.Fatalf("desktop script misses %q", expected)
		}
	}
}

func TestDirectoryLocationNamesWindowsDrives(t *testing.T) {
	name, kind := directoryLocation(`C:\\`, `C:\\Users\\tester`, "windows")
	if name != "本地磁盘（C:）" || kind != "volume" {
		t.Fatalf("Windows root label = (%q, %q), want local C drive", name, kind)
	}
}

func TestHealth(t *testing.T) {
	response := httptest.NewRecorder()
	newTestServer().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"version":"test"`)) {
		t.Fatalf("response does not include version: %s", response.Body.String())
	}
}

func TestGoals(t *testing.T) {
	handler := newTestServer()
	goals := httptest.NewRecorder()
	handler.ServeHTTP(goals, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))
	if goals.Code != http.StatusOK || !bytes.Contains(goals.Body.Bytes(), []byte(`"id":"develop"`)) {
		t.Fatalf("goals response = %d %s", goals.Code, goals.Body.String())
	}
}

func TestRobotTasksStartsAsJSONArray(t *testing.T) {
	response := httptest.NewRecorder()
	newTestServer().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/tasks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("robot tasks status = %d, want 200", response.Code)
	}
	var tasks []operationTask
	if err := json.Unmarshal(response.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("robot tasks = %q, want JSON array: %v", response.Body.String(), err)
	}
}

func TestChecks(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewBufferString(`{"goalId":"desktop"}`))
	newTestServer().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"goalId":"desktop"`)) {
		t.Fatalf("checks response = %s", response.Body.String())
	}
}

func TestCleanWebChecksNodeAndGit(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewBufferString(`{"goalId":"web","variant":"clean"}`))
	NewServer("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"id":"node"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"git"`)) {
		t.Fatalf("clean web checks should include node and git: %s", response.Body.String())
	}
}

func TestIdentityProtectionRequiresLoginAfterEnable(t *testing.T) {
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)
	if _, err := identity.Enable("lemonade", "secret", "secret"); err != nil {
		t.Fatal(err)
	}
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil))
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", blocked.Code)
	}
	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"lemonade","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(login, request)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	allowed := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil)
	request.AddCookie(login.Result().Cookies()[0])
	handler.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", allowed.Code)
	}
}

func TestAccountRolesControlManagementAndWorkbenchAccess(t *testing.T) {
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Enable("root", "secret", "secret"); err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)

	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"root","password":"secret"}`))
	handler.ServeHTTP(login, request)
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatalf("super login = %d %s", login.Code, login.Body.String())
	}
	superCookie := login.Result().Cookies()[0]
	management := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/management", nil)
	request.AddCookie(superCookie)
	handler.ServeHTTP(management, request)
	if management.Code != http.StatusOK {
		t.Fatalf("initial management = %d %s", management.Code, management.Body.String())
	}
	if !bytes.Contains(management.Body.Bytes(), []byte(`"roles":[]`)) {
		t.Fatalf("initial management must return an empty roles array: %s", management.Body.String())
	}

	createRole := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/roles", bytes.NewBufferString(`{"id":"reader","name":"只读","permissions":["workbench.view"]}`))
	request.AddCookie(superCookie)
	handler.ServeHTTP(createRole, request)
	if createRole.Code != http.StatusCreated {
		t.Fatalf("create role = %d %s", createRole.Code, createRole.Body.String())
	}
	createAccount := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/accounts", bytes.NewBufferString(`{"account":"alice","password":"password","confirmation":"password","roles":["reader"]}`))
	request.AddCookie(superCookie)
	handler.ServeHTTP(createAccount, request)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("create account = %d %s", createAccount.Code, createAccount.Body.String())
	}

	aliceLogin := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"account":"alice","password":"password"}`))
	handler.ServeHTTP(aliceLogin, request)
	if aliceLogin.Code != http.StatusOK || len(aliceLogin.Result().Cookies()) != 1 {
		t.Fatalf("ordinary login = %d %s", aliceLogin.Code, aliceLogin.Body.String())
	}
	aliceCookie := aliceLogin.Result().Cookies()[0]

	readOnly := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil)
	request.AddCookie(aliceCookie)
	handler.ServeHTTP(readOnly, request)
	if readOnly.Code != http.StatusOK {
		t.Fatalf("reader GET = %d %s", readOnly.Code, readOnly.Body.String())
	}
	blockedWrite := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewBufferString(`{"goalId":"desktop"}`))
	request.AddCookie(aliceCookie)
	handler.ServeHTTP(blockedWrite, request)
	if blockedWrite.Code != http.StatusForbidden {
		t.Fatalf("reader write = %d %s", blockedWrite.Code, blockedWrite.Body.String())
	}
	blockedManagement := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/management", nil)
	request.AddCookie(aliceCookie)
	handler.ServeHTTP(blockedManagement, request)
	if blockedManagement.Code != http.StatusForbidden {
		t.Fatalf("reader management = %d %s", blockedManagement.Code, blockedManagement.Body.String())
	}
}

func TestRobotWebViewUsesItsOwnFramePolicyAndBypassesManagementLogin(t *testing.T) {
	identity, err := access.NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Enable("lemonade", "secret", "secret"); err != nil {
		t.Fatal(err)
	}
	handler := NewServerWithAuth("test", fstest.MapFS{"dist/index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}, identity)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/robot/webview/not-a-root/plugin/", nil))

	if response.Code == http.StatusUnauthorized {
		t.Fatalf("WebView route must not require the management cookie")
	}
	if got := response.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("WebView X-Frame-Options = %q, want empty for cross-loopback frame", got)
	}
}

// TestStopPM2ForStartSkipsWhenIdle verifies the "谁最后启动谁为准" helper only
// attempts a PM2 stop when the service is running or unreadable, not when it is
// confirmed idle.
func TestStopPM2ForStartSkipsWhenIdle(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	s.pm2Status = func(string) (robot.PM2Status, error) {
		return robot.PM2Status{Configured: true, Managed: true, Running: false}, nil
	}
	if err := s.stopPM2ForStart(root); err != nil {
		t.Fatalf("idle PM2 should be skipped, got %v", err)
	}
}

// TestStopLocalForStartSkipsWhenNothingRunning verifies a local stop is a no-op
// when no supervised process exists.
func TestStopLocalForStartSkipsWhenNothingRunning(t *testing.T) {
	root := t.TempDir()
	s := newStatefulTestServer()
	if err := s.stopLocalForStart(root); err != nil {
		t.Fatalf("no local process should be a no-op, got %v", err)
	}
}

// TestRobotEventHubPublishesOutput verifies appendOperationOutput fans out an
// incremental output event to SSE subscribers.
func TestRobotEventHubPublishesOutput(t *testing.T) {
	s := newStatefulTestServer()
	s.operations = []operationTask{{ID: "dev-1", Output: ""}}
	sub := s.events.subscribe()
	defer s.events.unsubscribe(sub)
	s.appendOperationOutput("dev-1", "hello line\n")
	select {
	case event := <-sub:
		if event.Type != "output" || event.TaskID != "dev-1" || event.Text != "hello line\n" {
			t.Fatalf("event = %+v, want output for dev-1", event)
		}
	default:
		t.Fatal("expected an output event")
	}
}

// TestIsNumeric validates the PID filter used by forceFreePort.
func TestIsNumeric(t *testing.T) {
	for _, ok := range []string{"1234", "0", "99999"} {
		if !isNumeric(ok) {
			t.Fatalf("%q should be numeric", ok)
		}
	}
	for _, bad := range []string{"", "12a", "-1", "1.5", "abc"} {
		if isNumeric(bad) {
			t.Fatalf("%q should not be numeric", bad)
		}
	}
}

// TestRecordAndForgetProcessPersistsMarker verifies the process marker is
// written and removed from the persisted store.
func TestRecordAndForgetProcessPersistsMarker(t *testing.T) {
	t.Setenv("ALEMONX_PROCESS_FILE", filepath.Join(t.TempDir(), "processes.json"))
	s := newStatefulTestServer()
	root := "/robots/bot"
	s.recordProcess(root, "task-1", 12345, "dev")
	items := loadPersistedProcesses()
	found := false
	for _, item := range items {
		if item.Root == root && item.TaskID == "task-1" && item.PGID == 12345 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("persisted marker not found after record")
	}
	s.forgetProcess(root, "task-1")
	items = loadPersistedProcesses()
	for _, item := range items {
		if item.Root == root && item.TaskID == "task-1" {
			t.Fatal("marker not removed after forget")
		}
	}
	savePersistedProcesses(nil)
}

// TestAppProxyFramePolicy verifies /api/v1/robot/app/ responses are allowed in
// frames (no X-Frame-Options DENY) so the application service can render inside
// the workspace iframe, and are exempt from management auth like webviews.
func TestAppProxyFramePolicy(t *testing.T) {
	router := gin.New()
	router.Use((&server{}).ginHeaders())
	router.Any("/api/v1/robot/app/", func(c *gin.Context) { c.Status(http.StatusOK) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/robot/app/?root=%2Frobots%2Fbot", nil)
	router.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("app proxy X-Frame-Options = %q, want empty so it can be framed", got)
	}
}

func TestLocalServiceProxyKeepsManagementCookieIsolated(t *testing.T) {
	var upstreamCookie string
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCookie = r.Header.Get("Cookie")
		if strings.TrimSuffix(r.URL.Path, "/") == "/webui" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "upstream", Path: "/"})
		w.Header().Set("Location", "/next")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(rawPort)
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"napcat-webui","name":"NapCat","host":"127.0.0.1","port":%d,"basePath":"/webui","healthPath":"/","embed":true,"rewriteHtml":true}]}`, port)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services/alemonx-qq/napcat-webui/page?x=1", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: "management-secret"})
	s.localServiceProxyHandler(response, request)
	if upstreamCookie != "" {
		t.Fatalf("management cookie reached upstream: %q", upstreamCookie)
	}
	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.Code)
	}
	if got := response.Header().Get("Location"); got != "/api/v1/services/alemonx-qq/napcat-webui/next" {
		t.Fatalf("location = %q", got)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !strings.HasPrefix(cookies[0].Name, "alxsvc_") || cookies[0].Path != "/api/v1/services/alemonx-qq/napcat-webui/" {
		t.Fatalf("service cookie was not isolated: %+v", cookies)
	}
}

func TestLocalServiceStatusReportsGatewayCapability(t *testing.T) {
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "service-status")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"service-status","name":"Service","version":"1.0.0","web":{"root":"web"},"services":[{"id":"ui","name":"UI","host":"127.0.0.1","port":%s,"websocket":true}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	response := httptest.NewRecorder()
	s.localServiceStatusHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/services/status?plugin=service-status&service=ui", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"websocket":true`) || !strings.Contains(response.Body.String(), `"reachable":true`) {
		t.Fatalf("service status = %d %s", response.Code, response.Body.String())
	}
}

func TestLocalServiceAPIBootstrapInjectedWhenDeclared(t *testing.T) {
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "<!doctype html><html><head><title>LLBot</title></head><body>login</body></html>")
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = io.WriteString(w, `fetch("/api/login-qrcode");`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"luckylillia-webui","name":"LLBot","host":"127.0.0.1","port":%s,"basePath":"/","healthPath":"/","embed":true,"rewriteHtml":true,"rewriteApiBase":true}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	response := httptest.NewRecorder()
	s.localServiceProxyHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/services/alemonx-qq/luckylillia-webui/", nil))
	body := response.Body.String()
	if !strings.Contains(body, "__alxApiCompat") || !strings.Contains(body, "/api/v1/services/alemonx-qq/luckylillia-webui/") {
		t.Fatalf("compat bootstrap missing from proxied HTML: %s", body)
	}
	if !strings.Contains(body, "EventSource") {
		t.Fatalf("compat bootstrap must also rebase EventSource URLs: %s", body)
	}
	if !strings.Contains(body, "<base href=") {
		t.Fatalf("rewritten HTML lacks base href: %s", body)
	}

	// Non-HTML assets pass through unmodified.
	response = httptest.NewRecorder()
	s.localServiceProxyHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/services/alemonx-qq/luckylillia-webui/app.js", nil))
	if got := response.Body.String(); got != `fetch("/api/login-qrcode");` {
		t.Fatalf("javascript asset was modified: %q", got)
	}
}

func TestLocalServiceAPIBootstrapNotInjectedWithoutFlag(t *testing.T) {
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><head><title>x</title></head><body>plain</body></html>")
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"webui","name":"WebUI","host":"127.0.0.1","port":%s,"rewriteHtml":true}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	response := httptest.NewRecorder()
	s.localServiceProxyHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/services/alemonx-qq/webui/", nil))
	if strings.Contains(response.Body.String(), "__alxApiCompat") {
		t.Fatalf("compat bootstrap injected without rewriteApiBase: %s", response.Body.String())
	}
}

func TestLocalServiceWebSocketRequiresManifestDeclaration(t *testing.T) {
	upstream := newIPv4TestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"webui","name":"WebUI","host":"127.0.0.1","port":%s}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/services/alemonx-qq/webui/", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	response := httptest.NewRecorder()
	s.localServiceProxyHandler(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNotImplemented, response.Body.String())
	}
}

func TestLocalServiceWebSocketProxyUsesDeclaredLoopbackTarget(t *testing.T) {
	upstream := newIPv4TestServer(t, websocket.Handler(func(connection *websocket.Conn) {
		defer connection.Close()
		var message string
		if err := websocket.Message.Receive(connection, &message); err == nil {
			_ = websocket.Message.Send(connection, "echo:"+message)
		}
	}))
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"webui","name":"WebUI","host":"127.0.0.1","port":%s,"websocket":true}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	proxy := newIPv4TestServer(t, http.HandlerFunc(s.localServiceProxyHandler))
	endpoint := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/api/v1/services/alemonx-qq/webui/"
	client, err := websocket.Dial(endpoint, "", proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := websocket.Message.Send(client, "hello"); err != nil {
		t.Fatal(err)
	}
	var response string
	if err := websocket.Message.Receive(client, &response); err != nil {
		t.Fatal(err)
	}
	if response != "echo:hello" {
		t.Fatalf("response = %q", response)
	}
}

func TestLocalServiceWebSocketProxyPreservesBinaryFrames(t *testing.T) {
	upstream := newIPv4TestServer(t, websocket.Handler(func(connection *websocket.Conn) {
		defer connection.Close()
		var payload []byte
		if err := websocket.Message.Receive(connection, &payload); err == nil {
			_ = websocket.Message.Send(connection, append([]byte("echo:"), payload...))
		}
	}))
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	pluginsRoot := t.TempDir()
	pluginRoot := filepath.Join(pluginsRoot, "alemonx-qq")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`{"id":"alemonx-qq","name":"QQ","version":"1.0.0","web":{"root":"web"},"services":[{"id":"webui","name":"WebUI","host":"127.0.0.1","port":%s,"websocket":true}]}`, rawPort)
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(pluginsRoot)}
	proxy := newIPv4TestServer(t, http.HandlerFunc(s.localServiceProxyHandler))
	endpoint := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/api/v1/services/alemonx-qq/webui/"
	client, err := websocket.Dial(endpoint, "", proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := websocket.Message.Send(client, []byte{0, 1, 2}); err != nil {
		t.Fatal(err)
	}
	var response []byte
	if err := websocket.Message.Receive(client, &response); err != nil {
		t.Fatal(err)
	}
	if got, want := string(response), "echo:\x00\x01\x02"; got != want {
		t.Fatalf("response = %q, want %q", got, want)
	}
}

func TestRobotLiveProxyUsesFullReceiveCBPClient(t *testing.T) {
	var receivedHeader http.Header
	upstream := newIPv4TestServer(t, websocket.Handler(func(connection *websocket.Conn) {
		receivedHeader = connection.Request().Header.Clone()
		defer connection.Close()
		var message string
		if err := websocket.Message.Receive(connection, &message); err == nil {
			_ = websocket.Message.Send(connection, "echo:"+message)
		}
	}))
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, rawPort, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	writeFixture(t, root, "alemon.config.yaml", "port: "+rawPort+"\n")
	s := newStatefulTestServer()
	proxy := newIPv4TestServer(t, http.HandlerFunc(s.robotLiveHandler))
	token := robotAppToken(root)
	endpoint := "ws" + strings.TrimPrefix(proxy.URL, "http") + "/api/v1/robot/live/" + token + "/?deviceId=alemonx-live-test-connection"
	client, err := websocket.Dial(endpoint, "", proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := websocket.Message.Send(client, "hello"); err != nil {
		t.Fatal(err)
	}
	var response string
	if err := websocket.Message.Receive(client, &response); err != nil {
		t.Fatal(err)
	}
	if response != "echo:hello" {
		t.Fatalf("response = %q", response)
	}
	if got := receivedHeader.Get("X-Full-Receive"); got != "1" {
		t.Fatalf("X-Full-Receive = %q, want 1", got)
	}
	if got := receivedHeader.Get("X-Device-Id"); got != "alemonx-live-test-connection" {
		t.Fatalf("X-Device-Id = %q", got)
	}
	if got := receivedHeader.Get("User-Agent"); got != "client" {
		t.Fatalf("User-Agent = %q, want client", got)
	}
}

// TestModifyRobotAppResponse verifies the /api/v1/robot/app/ proxy patches
// proxied documents so absolute app links (/app/, /apps/x/) and relative assets
// stay inside the proxy mount (which carries the robot root token) instead of
// escaping to the management page origin, and that redirects stay within the
// mount.
func TestModifyRobotAppResponse(t *testing.T) {
	target, err := url.Parse("http://127.0.0.1:18110")
	if err != nil {
		t.Fatal(err)
	}
	const mount = "/api/v1/robot/app/"
	appPrefix := mount + robotAppToken("/robots/bot") + "/"
	htmlResponse := func(body string) *http.Response {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body:          io.NopCloser(strings.NewReader(body)),
			ContentLength: int64(len(body)),
		}
	}

	navigation := func(path string) *http.Request {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "text/html")
		return request
	}

	t.Run("launchpad root", func(t *testing.T) {
		response := htmlResponse(`<html><head><title>launchpad</title></head><body>` +
			`<a href="/app/">Main</a><a href="/apps/demo/">Plugin</a>` +
			`<script src="./assets/app.js"></script>` +
			`<link rel="icon" href="/favicon.ico"></body></html>`)
		modifyRobotAppResponse(response, target, appPrefix, appPrefix, navigation(appPrefix))
		body, _ := io.ReadAll(response.Body)
		for _, want := range []string{
			`<base href="` + appPrefix + `">`,
			`href="` + appPrefix + `app/"`,
			`href="` + appPrefix + `apps/demo/"`,
			`href="` + appPrefix + `favicon.ico"`,
			`src="./assets/app.js"`,
			`scrollbar-width:none`,
		} {
			if !strings.Contains(string(body), want) {
				t.Errorf("launchpad body missing %q\nbody:\n%s", want, body)
			}
		}
		if strings.Contains(string(body), `href="/app/"`) {
			t.Errorf("launchpad body still contains un-prefixed /app/ link\nbody:\n%s", body)
		}
		if got := response.Header.Get("Content-Length"); got != strconv.Itoa(len(body)) {
			t.Errorf("Content-Length = %q, want %d", got, len(body))
		}
	})

	t.Run("plugin page base", func(t *testing.T) {
		path := appPrefix + "apps/demo/"
		response := htmlResponse(`<html><head><title>demo</title><script src="./assets/index.js"></script></head>` +
			`<body><a href="/config/qq">Config</a></body></html>`)
		modifyRobotAppResponse(response, target, appPrefix, path, navigation(path))
		body, _ := io.ReadAll(response.Body)
		for _, want := range []string{
			`<base href="` + path + `">`,
			`src="./assets/index.js"`,
			`href="` + appPrefix + `config/qq"`,
		} {
			if !strings.Contains(string(body), want) {
				t.Errorf("plugin body missing %q\nbody:\n%s", want, body)
			}
		}
	})

	t.Run("no head falls back to html tag", func(t *testing.T) {
		response := htmlResponse(`<html><body><a href="/app/">x</a></body></html>`)
		modifyRobotAppResponse(response, target, appPrefix, appPrefix, navigation(appPrefix))
		body, _ := io.ReadAll(response.Body)
		if !strings.Contains(string(body), `<base href="`+appPrefix+`">`) {
			t.Errorf("missing injected base\nbody:\n%s", body)
		}
	})

	t.Run("scheme and protocol-relative URLs untouched", func(t *testing.T) {
		response := htmlResponse(`<a href="https://example.com/x">a</a><a href="//cdn.example.com/x">b</a>`)
		modifyRobotAppResponse(response, target, appPrefix, appPrefix, navigation(appPrefix))
		body, _ := io.ReadAll(response.Body)
		for _, want := range []string{`href="https://example.com/x"`, `href="//cdn.example.com/x"`} {
			if !strings.Contains(string(body), want) {
				t.Errorf("external URL changed: missing %q\nbody:\n%s", want, body)
			}
		}
	})

	t.Run("redirect re-prefixed", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"/app/"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		modifyRobotAppResponse(response, target, appPrefix, appPrefix+"app", navigation(appPrefix+"app"))
		if got := response.Header.Get("Location"); got != appPrefix+"app/" {
			t.Errorf("Location = %q, want %q", got, appPrefix+"app/")
		}
	})

	t.Run("full target-host redirect re-prefixed", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://127.0.0.1:18110/app/?q=1"}},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		modifyRobotAppResponse(response, target, appPrefix, appPrefix+"app", navigation(appPrefix+"app"))
		if got := response.Header.Get("Location"); got != appPrefix+"app/?q=1" {
			t.Errorf("Location = %q, want %q", got, appPrefix+"app/?q=1")
		}
	})

	t.Run("unmatched page navigation returns to launchpad", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":404}`)),
		}
		path := appPrefix + "apps/demo/missing"
		modifyRobotAppResponse(response, target, appPrefix, path, navigation(path))
		if response.StatusCode != http.StatusFound {
			t.Fatalf("status = %d, want 302 redirect to launchpad", response.StatusCode)
		}
		if got := response.Header.Get("Location"); got != appPrefix {
			t.Errorf("Location = %q, want %q", got, appPrefix)
		}
		if body, _ := io.ReadAll(response.Body); len(body) != 0 {
			t.Errorf("redirect body should be empty, got %q", body)
		}
	})

	t.Run("unmatched asset request is not redirected", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":404}`)),
		}
		path := appPrefix + "apps/demo/assets/missing.js"
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "*/*")
		modifyRobotAppResponse(response, target, appPrefix, path, request)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404 (asset request must pass through)", response.StatusCode)
		}
		if got := response.Header.Get("Location"); got != "" {
			t.Errorf("asset request must not redirect, got Location %q", got)
		}
	})

	t.Run("launchpad 404 never redirects to itself", func(t *testing.T) {
		response := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}
		modifyRobotAppResponse(response, target, appPrefix, appPrefix, navigation(appPrefix))
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("launchpad 404 must stay 404, got %d", response.StatusCode)
		}
	})

	t.Run("assets and API payloads pass through", func(t *testing.T) {
		response := htmlResponse(`{"api":"/status"}`)
		response.Header.Set("Content-Type", "application/json")
		path := appPrefix + "api/status"
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Accept", "application/json")
		modifyRobotAppResponse(response, target, appPrefix, path, request)
		body, _ := io.ReadAll(response.Body)
		if string(body) != `{"api":"/status"}` {
			t.Errorf("JSON body changed: %q", body)
		}
	})
}

// TestRobotAppTokenRoundTrip verifies the root token survives a decode round
// trip and that the legacy query form still resolves from the handler.
func TestRobotAppTokenRoundTrip(t *testing.T) {
	const root = "/Users/lemonade/Desktop/alemonx/alemonb"
	token := robotAppToken(root)
	decoded, raw := robotAppRootFromPath("/api/v1/robot/app/"+token+"/apps/demo/", "/api/v1/robot/app/")
	if decoded != root || raw != token {
		t.Fatalf("round trip = (%q, %q), want (%q, %q)", decoded, raw, root, token)
	}
	if strings.ContainsAny(token, "+/=") {
		t.Fatalf("token must be base64url-safe, got %q", token)
	}
	// A path whose first segment is not valid base64url must yield no root.
	if root, _ := robotAppRootFromPath("/api/v1/robot/app/apps/demo/", "/api/v1/robot/app/"); root != "" {
		t.Fatalf("non-token path decoded root = %q, want empty", root)
	}
}

// flushWriter adapts httptest.ResponseRecorder to http.Flusher so the SSE
// handler can be driven without binding a real port (the test sandbox forbids
// listen).
type flushWriter struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (f *flushWriter) Flush() {}
func (f *flushWriter) Write(data []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(data)
}
func (f *flushWriter) Snapshot() string { f.mu.Lock(); defer f.mu.Unlock(); return f.Body.String() }

// TestSetupPluginEventsSSE verifies /setup/plugins/events emits a change event
// when the plugin registry changes and stays quiet otherwise.
func TestSetupPluginEventsSSE(t *testing.T) {
	root := t.TempDir()
	registry := setupplugin.NewRegistry(root)
	registry.Rescan()
	s := &server{plugins: registry}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/events", nil).WithContext(ctx)
	recorder := &flushWriter{ResponseRecorder: httptest.NewRecorder()}
	done := make(chan struct{})
	go func() {
		s.setupPluginEventsHandler(recorder, request)
		close(done)
	}()

	// No initial event is emitted.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(recorder.Snapshot()) > 0 {
			cancel()
			t.Fatalf("unexpected early SSE data: %q", recorder.Snapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Adding a plugin bumps the revision and must push a change event.
	directory := filepath.Join(root, "newone")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"newone","name":"New","version":"1.0.0","web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, "alx.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	registry.Rescan()

	waitFor := time.Now().Add(time.Second)
	for !strings.Contains(recorder.Snapshot(), "data: {}") && time.Now().Before(waitFor) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(recorder.Snapshot(), "data: {}") {
		t.Fatalf("SSE stream did not emit a change event, body: %q", recorder.Snapshot())
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
}

// TestSetupPluginWebEmbeddable verifies setup plugin pages are frame-embeddable
// (ginHeaders must not add X-Frame-Options DENY), so the management UI can
// render them in an iframe.
func TestSetupPluginWebEmbeddable(t *testing.T) {
	router := gin.New()
	router.Use((&server{}).ginHeaders())
	router.Any("/api/v1/setup/plugins/web/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/web/alemonx-qq/index.html", nil)
	router.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("setup plugin web X-Frame-Options = %q, want empty", got)
	}
}

func TestSetupPluginStatusIsReadOnlyAndCoalesced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	pluginRoot := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(pluginRoot, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"},"statusActions":["status"]}`
	if err := os.WriteFile(filepath.Join(pluginRoot, "alx.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "runner"), []byte(`#!/bin/sh
sleep 0.1
echo x >> status-count
printf '{"output":"{\\"engine\\":\\"fixture\\",\\"installed\\":true}"}'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &server{plugins: setupplugin.NewRegistry(root)}
	for range 2 {
		recorder := httptest.NewRecorder()
		s.setupPluginActionHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/fixture/status?action=status", nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"engine":"fixture"`) {
			t.Fatalf("status = %d %s", recorder.Code, recorder.Body.String())
		}
	}
	count, err := os.ReadFile(filepath.Join(pluginRoot, "status-count"))
	if err != nil || strings.Count(string(count), "x") != 1 {
		t.Fatalf("status runner executions = %q, %v", count, err)
	}
	if len(s.operations) != 0 {
		t.Fatalf("read-only status must not create operations: %#v", s.operations)
	}
	// After the one-second cached result expires, parallel readers must join the
	// same runner invocation rather than create a task (or two processes).
	time.Sleep(1100 * time.Millisecond)
	var readers sync.WaitGroup
	results := make(chan int, 2)
	for range 2 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			recorder := httptest.NewRecorder()
			s.setupPluginActionHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/fixture/status?action=status", nil))
			results <- recorder.Code
		}()
	}
	readers.Wait()
	close(results)
	for code := range results {
		if code != http.StatusOK {
			t.Fatalf("coalesced status = %d, want 200", code)
		}
	}
	count, err = os.ReadFile(filepath.Join(pluginRoot, "status-count"))
	if err != nil || strings.Count(string(count), "x") != 2 {
		t.Fatalf("concurrent status runner executions = %q, %v", count, err)
	}
	recorder := httptest.NewRecorder()
	s.setupPluginActionHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/setup/plugins/fixture/status?action=install", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("non-status action = %d, want 400", recorder.Code)
	}
}
