package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopPreferencesSurviveOriginChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	call := func(method, body string, authorized bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, desktopPreferencesPath, strings.NewReader(body))
		if authorized {
			r.Header.Set("X-ALX-Desktop-Preferences", "1")
		}
		w := httptest.NewRecorder()
		(&desktopPreferences{path: path}).ServeHTTP(w, r)
		return w
	}
	for _, step := range []struct {
		body string
		code int
	}{
		{`{"key":"alemonjs-theme","value":"dark","import":true}`, http.StatusNoContent},
		{`{"key":"alemonjs-theme","value":"light","import":true}`, http.StatusNoContent},
		{`{"key":"access-token","value":"test"}`, http.StatusBadRequest},
	} {
		if got := call(http.MethodPut, step.body, true).Code; got != step.code {
			t.Fatalf("PUT returned %d, want %d", got, step.code)
		}
	}
	if got := call(http.MethodGet, "", false).Code; got != http.StatusForbidden {
		t.Fatalf("GET without desktop header returned %d", got)
	}
	var values map[string]*string
	if err := json.Unmarshal(call(http.MethodGet, "", true).Body.Bytes(), &values); err != nil || values["alemonjs-theme"] == nil || *values["alemonjs-theme"] != "dark" {
		t.Fatalf("theme was not restored: %v", err)
	}
	for _, body := range []string{
		`{"key":"alemonjs-theme","value":null}`,
		`{"key":"alemonjs-theme","value":"light","import":true}`,
	} {
		if got := call(http.MethodPut, body, true).Code; got != http.StatusNoContent {
			t.Fatalf("PUT returned %d", got)
		}
	}
	if got := strings.TrimSpace(call(http.MethodGet, "", true).Body.String()); got != `{"alemonjs-theme":null}` {
		t.Fatalf("deleted preference was restored: %s", got)
	}
}
