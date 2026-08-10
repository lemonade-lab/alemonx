package httpcache

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetCachesSuccessfulResponses(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("cached-body"))
	}))
	defer server.Close()

	first, err := Get(server.Client(), server.URL, time.Minute)
	if err != nil || string(first.Body) != "cached-body" || first.FromCache {
		t.Fatalf("first = %#v, %v", first, err)
	}
	second, err := Get(server.Client(), server.URL, time.Minute)
	if err != nil || !second.FromCache || string(second.Body) != "cached-body" {
		t.Fatalf("second = %#v, %v", second, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hits = %d, want 1", hits.Load())
	}
	Evict(server.URL)
}

func TestGetFallsBackToStaleOnFailure(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			_, _ = w.Write([]byte("cached-body"))
			return
		}
		// Simulate GitHub rate limiting on the second request.
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	if _, err := Get(server.Client(), server.URL, 20*time.Millisecond); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	response, err := Get(server.Client(), server.URL, time.Minute)
	if err != nil {
		t.Fatalf("stale fetch: %v", err)
	}
	if !response.Stale || string(response.Body) != "cached-body" {
		t.Fatalf("stale response = %#v", response)
	}
	Evict(server.URL)
}

func TestGetReturnsErrorWithoutCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	if _, err := Get(server.Client(), server.URL, time.Minute); err == nil {
		t.Fatal("uncached failure must return an error")
	}
}
