// Package httpcache keeps a small in-memory cache for remote metadata that the
// workbench reads repeatedly (catalog markdown, README files, package.json,
// release lists). It is the first line of defense against GitHub rate limits:
// successful responses are cached with a TTL, and when a fresh fetch fails
// (including 429/403 rate-limit responses), the most recent cached body is
// returned so the UI keeps working instead of showing a network error.
package httpcache

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultUserAgent = "alemonx"
	maxBodyBytes     = 8 << 20
	maxEntries       = 512
)

type entry struct {
	body    []byte
	status  int
	expires time.Time
}

var (
	mu            sync.Mutex
	store         = map[string]*entry{}
	tokenFileOnce sync.Once
	tokenFile     string
)

// Response is the result of a cached fetch. Stale is true when the fresh
// request failed and the previous cached body was served instead.
type Response struct {
	Body      []byte
	Status    int
	FromCache bool
	Stale     bool
}

// Get fetches url with the default client, caching successful responses for
// ttl and falling back to the last cached body when a fresh fetch fails.
func Get(client *http.Client, url string, ttl time.Duration) (Response, error) {
	return GetWithHeaders(client, url, ttl, nil)
}

// GetWithHeaders behaves like Get with extra request headers.
func GetWithHeaders(client *http.Client, url string, ttl time.Duration, headers map[string]string) (Response, error) {
	now := time.Now()
	mu.Lock()
	cached := store[url]
	mu.Unlock()
	if cached != nil && now.Before(cached.expires) {
		return Response{Body: cached.body, Status: cached.status, FromCache: true}, nil
	}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Response{}, err
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if strings.HasPrefix(url, "https://api.github.com/") {
		if header, value := GitHubTokenHeader(); header != "" {
			request.Header.Set(header, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		if cached != nil {
			return Response{Body: cached.body, Status: cached.status, FromCache: true, Stale: true}, nil
		}
		return Response{}, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if response.StatusCode >= 200 && response.StatusCode < 300 && readErr == nil {
		mu.Lock()
		if len(store) >= maxEntries {
			for key, item := range store {
				if time.Now().After(item.expires) {
					delete(store, key)
				}
			}
		}
		if len(store) < maxEntries {
			store[url] = &entry{body: body, status: response.StatusCode, expires: now.Add(ttl)}
		}
		mu.Unlock()
		return Response{Body: body, Status: response.StatusCode}, nil
	}
	if cached != nil {
		return Response{Body: cached.body, Status: cached.status, FromCache: true, Stale: true}, nil
	}
	if readErr != nil {
		return Response{}, readErr
	}
	return Response{Body: body, Status: response.StatusCode}, fmt.Errorf("HTTP %d", response.StatusCode)
}

// GitHubTokenHeader returns an Authorization header for GitHub API requests
// when a token is configured, or empty strings otherwise. The token comes
// from GITHUB_TOKEN/GH_TOKEN or the optional token file
// <user-config>/alemonjs/github-token. Authenticated API calls raise the
// GitHub rate limit from 60 to 5000 requests per hour.
func GitHubTokenHeader() (string, string) {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(readTokenFile())
	}
	if token == "" {
		return "", ""
	}
	return "Authorization", "Bearer " + token
}

func readTokenFile() string {
	tokenFileOnce.Do(func() {
		tokenFile = TokenPath()
	})
	if tokenFile == "" {
		return ""
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// TokenPath returns the GitHub token file location. It mirrors the lookup
// used by readTokenFile so writers and readers always agree.
func TokenPath() string {
	directory, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "alemonjs", "github-token")
}

// SaveToken writes the GitHub token with owner-only permissions so a PAT or
// OAuth token never stays world-readable on disk.
func SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("GitHub Token 无效")
	}
	path := TokenPath()
	if path == "" {
		return fmt.Errorf("无法定位用户配置目录")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("无法创建配置目录：%w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("无法保存 GitHub Token：%w", err)
	}
	return nil
}

// RemoveToken deletes the GitHub token file. A missing file is not an error.
func RemoveToken() error {
	path := TokenPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法移除 GitHub Token：%w", err)
	}
	return nil
}

// Evict removes a cached URL. It exists for tests and for callers that know
// the remote content changed (for example after publishing a release).
func Evict(url string) {
	mu.Lock()
	defer mu.Unlock()
	delete(store, url)
}
