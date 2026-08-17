package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidChatMediaSource(t *testing.T) {
	for _, raw := range []string{
		"https://multimedia.nt.qq.com.cn/download?id=1",
		"https://gchat.qpic.cn/gchatpic_new/file/0",
	} {
		parsed, err := url.Parse(raw)
		if err != nil || !validChatMediaSource(parsed) {
			t.Fatalf("expected allowed source: %s", raw)
		}
	}
	for _, raw := range []string{
		"http://multimedia.nt.qq.com.cn/download?id=1",
		"https://example.com/image.png",
		"file:///tmp/image.png",
	} {
		parsed, _ := url.Parse(raw)
		if validChatMediaSource(parsed) {
			t.Fatalf("unexpected allowed source: %s", raw)
		}
	}
}

func TestCacheChatMediaAndServeIt(t *testing.T) {
	image := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(image)),
		}, nil
	})}

	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	store := newChatHistoryTestStore(t)
	id, err := cacheChatMedia(
		context.Background(),
		store,
		root,
		"https://fixture.example/temporary.png",
		client,
		func(*url.URL) bool { return true },
	)
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.mediaPath(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || !bytes.Equal(content, image) {
		t.Fatalf("cached image mismatch: %v", err)
	}

	s := newStatefulTestServer()
	s.chatHistory = store
	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/robot/chat/media?"+url.Values{"root": {root}, "id": {id}}.Encode(),
		nil,
	)
	s.robotChatMediaHandler(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), image) {
		t.Fatalf("served image = %d %q", response.Code, response.Body.Bytes())
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("content type = %q", got)
	}

	if err := store.Delete(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("media directory still exists: %v", err)
	}
}

func TestRobotChatMediaCacheRejectsNonQQSource(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"name":"demo","version":"1.0.0"}`)
	s := newStatefulTestServer()
	s.chatHistory = newChatHistoryTestStore(t)
	body, err := json.Marshal(map[string]string{
		"root": root,
		"url":  "https://example.com/image.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.robotChatMediaHandler(
		response,
		httptest.NewRequest(http.MethodPost, "/api/v1/robot/chat/media", bytes.NewReader(body)),
	)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "来源不受支持") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
