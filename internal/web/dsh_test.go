package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"alemonx/internal/dsh"
)

type forbiddenDSHKeyring struct{ t *testing.T }

func (f forbiddenDSHKeyring) Get(string) (string, error) {
	f.t.Fatal("Docker accessed keyring")
	return "", nil
}
func (f forbiddenDSHKeyring) Set(string, string) error { f.t.Fatal("Docker wrote keyring"); return nil }

func TestDockerDSHSecretContract(t *testing.T) {
	t.Setenv("ALX_CONTAINER", "1")
	for _, mode := range []string{"disabled", "missing", "empty", "ready"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if mode == "empty" || mode == "ready" {
				value := "  \n"
				if mode == "ready" {
					value = "test-private-key\n"
				}
				if err := os.WriteFile(path, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "disabled" {
				path = ""
			}
			t.Setenv("ALX_DSH_SECRET_FILE", path)
			s := &server{dshSecrets: forbiddenDSHKeyring{t}}
			root := t.TempDir()
			rt := dsh.New(t.TempDir(), nil)
			response := httptest.NewRecorder()
			s.dshConfiguration(response, rt, root)
			var config map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
				t.Fatal(err)
			}
			if config["credentialConfigured"] != (mode == "ready") || config["configured"] != false {
				t.Fatalf("unexpected config: %v", config)
			}
			if strings.Contains(response.Body.String(), "test-private-key") {
				t.Fatal("key leaked")
			}
			response = httptest.NewRecorder()
			s.configureDSH(response, httptest.NewRequest("POST", "/", strings.NewReader(`{"apiKey":"browser-key"}`)), rt, root)
			if response.Code != 400 {
				t.Fatalf("browser override accepted: %d", response.Code)
			}
			if mode != "ready" {
				response = httptest.NewRecorder()
				s.configureDSH(response, httptest.NewRequest("POST", "/", strings.NewReader(`{}`)), rt, root)
				if response.Code != 503 {
					t.Fatalf("missing secret accepted: %d", response.Code)
				}
			}
		})
	}
}

func TestDockerDSHConnectUsesSecretWithoutWritingKeyring(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("shell fixture unavailable")
	}
	t.Setenv("ALX_CONTAINER", "1")
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("test-only-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALX_DSH_SECRET_FILE", secret)
	home, root := t.TempDir(), t.TempDir()
	rt := dsh.New(home, func(ctx context.Context, _ string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `read line; echo '{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"deepseek-harness-sdk-runtime"}}}'; while read line; do :; done`)
	})
	defer rt.Stop(context.Background())
	s := &server{dshSecrets: forbiddenDSHKeyring{t}}
	response := httptest.NewRecorder()
	s.configureDSH(response, httptest.NewRequest("POST", "/", strings.NewReader(`{"provider":"deepseek-official","model":"deepseek-chat"}`)), rt, root)
	if response.Code != 200 || !rt.Status().Ready {
		t.Fatalf("secret connection failed: %d %s", response.Code, response.Body.String())
	}
	config, err := os.ReadFile(filepath.Join(home, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config)+response.Body.String(), "test-only-secret") {
		t.Fatal("secret leaked into persisted/public configuration")
	}
}

func TestDSHStatusIsExposedThroughFacade(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dsh/status", nil)
	response := httptest.NewRecorder()
	newTestServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，应为 200：%s", response.Code, response.Body.String())
	}
}

func TestPublicDSHEventDoesNotForwardToolPayload(t *testing.T) {
	event := publicDSHEvent(dsh.Event{
		Method: "session.event",
		Params: json.RawMessage(`{"sessionId":"s1","status":"running","toolOutput":"secret"}`),
	}, "runtime")
	if event.SessionID != "s1" || event.Status != "running" {
		t.Fatalf("公开事件 = %#v", event)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("公开事件不应携带工具输出：%s", raw)
	}
}

func TestPublicDSHEventPublishesNestedAssistantTextOnly(t *testing.T) {
	event := publicDSHEvent(dsh.Event{Method: "session/event", Params: json.RawMessage(`{"sessionId":"s1","event":{"type":"assistant/message","data":{"message":{"content":[{"type":"text","text":"安全回答"}]},"secret":"nope"}}}`)}, "runtime")
	if event.Type != "assistant/message" || event.Text != "安全回答" || event.SessionID != "s1" {
		t.Fatalf("公开事件 = %#v", event)
	}
	raw, _ := json.Marshal(event)
	if strings.Contains(string(raw), "nope") {
		t.Fatalf("公开事件泄露了嵌套 payload：%s", raw)
	}
}

func TestDSHLifecycleIsNotAToolAndFailuresAreSafe(t *testing.T) {
	for _, kind := range []string{"turn/start", "step/end", "session.status"} {
		e := publicDSHEvent(dsh.Event{Method: kind, Params: json.RawMessage(`{"sessionId":"s1","status":"idle"}`)}, "runtime")
		if e.Tool != "" {
			t.Fatalf("lifecycle became tool: %#v", e)
		}
	}
	e := publicDSHEvent(dsh.Event{Method: "session.event", Params: json.RawMessage(`{"sessionId":"s1","event":{"type":"turn/end","data":{"reason":{"kind":"error","error":{"message":"private-provider-detail","code":"REQUEST_EXTENSION"}}}}}`)}, "runtime")
	if e.Error == "" || e.Tool != "" {
		t.Fatalf("missing safe failure: %#v", e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "private-provider-detail") {
		t.Fatal("provider detail leaked")
	}
}

func TestPublicDSHEventPublishesOnlyApprovalPreview(t *testing.T) {
	event := publicDSHEvent(dsh.Event{Method: "alemonx.approval", Params: json.RawMessage(`{"sessionId":"s1","approval":{"id":"approval-1","sessionId":"s1","action":"file.write","summary":"更新配置","expiresAt":"2026-01-01T00:00:00Z"},"command":"rm -rf /","secret":"nope"}`)}, "runtime")
	if event.Approval == nil || event.Approval.ID != "approval-1" || event.Approval.Summary != "更新配置" {
		t.Fatalf("审批事件 = %#v", event)
	}
	raw, _ := json.Marshal(event)
	if strings.Contains(string(raw), "rm -rf") || strings.Contains(string(raw), "nope") {
		t.Fatalf("审批事件泄露了受保护细节：%s", raw)
	}
}

func TestLegacyAgentRouteIsNotRegistered(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/chat", nil)
	response := httptest.NewRecorder()
	newTestServer().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("旧 Agent 路由应为 404，实际 %d", response.Code)
	}
}
