package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"alemonx/internal/datamanager"
)

func TestDataConnectSystemProtection(t *testing.T) {
	path, err := datamanager.EnsureDefaultSQLite(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALX_OPS_SQLITE_PATH", path)
	for _, write := range []bool{false, true} {
		input := datamanager.ConnectRequest{Connection: datamanager.Connection{Engine: "sqlite", Path: path}, Action: "query", SQL: "SELECT 1", Write: write, Confirmed: write}
		if write {
			input.SQL = "CREATE TABLE forbidden(id INTEGER)"
		}
		body, _ := json.Marshal(input)
		request := httptest.NewRequest("POST", "/api/v1/data/connect", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		(&server{}).dataHandler(response, request)
		if write && response.Code != 403 {
			t.Fatal("protected write accepted", response.Code)
		}
		if !write && (response.Code != 200 || !strings.Contains(response.Body.String(), `"systemDatabase":true`)) {
			t.Fatal(response.Code, response.Body.String())
		}
	}
}
