package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentArchiveRejectsWrites(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-archive/sessions", nil)
	response := httptest.NewRecorder()
	newTestServer().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("状态码 = %d", response.Code)
	}
}
