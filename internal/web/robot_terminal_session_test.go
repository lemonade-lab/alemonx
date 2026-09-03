//go:build !windows

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTerminalSessionsRequireTheirLocalKey(t *testing.T) {
	s := &server{terminalSessions: newTerminalSessionStore()}
	create := httptest.NewRequest(http.MethodPost, "/api/v1/terminal/sessions", bytes.NewBufferString(`{"cwd":`+jsonString(t, t.TempDir())+`}`))
	created := httptest.NewRecorder()
	s.terminalSessionsHandler(created, create)
	if created.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var session struct{ ID, Key string }
	if err := json.Unmarshal(created.Body.Bytes(), &session); err != nil || session.ID == "" || session.Key == "" {
		t.Fatalf("create response=%s err=%v", created.Body.String(), err)
	}
	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/terminal/sessions/"+session.ID, nil)
	blocked := httptest.NewRecorder()
	s.terminalSessionHandler(blocked, unauthorized)
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("missing key status=%d, want 404", blocked.Code)
	}
	close := httptest.NewRequest(http.MethodDelete, "/api/v1/terminal/sessions/"+session.ID, nil)
	close.Header.Set("X-ALX-Terminal-Key", session.Key)
	closed := httptest.NewRecorder()
	s.terminalSessionHandler(closed, close)
	if closed.Code != http.StatusOK {
		t.Fatalf("close status=%d body=%s", closed.Code, closed.Body.String())
	}
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
