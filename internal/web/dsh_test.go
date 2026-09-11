package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alemonx/internal/dsh"
)

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
