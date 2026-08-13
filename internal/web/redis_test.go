package web

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alemonx/internal/redis"
)

func TestSystemRedisLifecycle(t *testing.T) {
	manager := redis.NewManager(filepath.Join(t.TempDir(), "alx-redis.json"))
	s := &server{redisManager: manager}
	port := testFreePort(t)

	response := httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/redis", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("initial status = %d %s", response.Code, response.Body.String())
	}
	var status redis.Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Port != redis.DefaultPort {
		t.Fatalf("initial status = %+v", status)
	}

	response = httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPut, "/api/v1/system/redis", strings.NewReader(`{"port":`+strconv.Itoa(port)+`,"autoStart":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("configure = %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Port != port || !status.AutoStart || status.Running {
		t.Fatalf("configured status = %+v", status)
	}

	response = httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/redis", strings.NewReader(`{"action":"start"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("start = %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Running || !status.Managed {
		t.Fatalf("started status = %+v", status)
	}

	response = httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/redis", strings.NewReader(`{"action":"restart"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("restart = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/redis", strings.NewReader(`{"action":"stop"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("stop = %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Managed {
		t.Fatalf("stopped status = %+v", status)
	}
}

func TestSystemRedisRejectsUnknownActionAndMissingManager(t *testing.T) {
	manager := redis.NewManager(filepath.Join(t.TempDir(), "alx-redis.json"))
	s := &server{redisManager: manager}
	response := httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/redis", strings.NewReader(`{"action":"explode"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown action = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	(&server{}).systemRedisHandler(response, httptest.NewRequest(http.MethodGet, "/api/v1/system/redis", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing manager = %d %s", response.Code, response.Body.String())
	}
}

func TestSystemRedisConfigCanDisableStart(t *testing.T) {
	manager := redis.NewManager(filepath.Join(t.TempDir(), "alx-redis.json"))
	s := &server{redisManager: manager}

	response := httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPut, "/api/v1/system/redis", strings.NewReader(`{"port":`+strconv.Itoa(testFreePort(t))+`,"autoStart":true,"disabled":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("disable config = %d %s", response.Code, response.Body.String())
	}
	var status redis.Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Disabled || status.Running {
		t.Fatalf("disabled status = %+v", status)
	}

	response = httptest.NewRecorder()
	s.systemRedisHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/redis", strings.NewReader(`{"action":"start"}`)))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "禁用") {
		t.Fatalf("start while disabled = %d %s", response.Code, response.Body.String())
	}
}

// testFreePort mirrors the redis package helper for the web package tests.
func testFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}
