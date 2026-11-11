package systemnetwork

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type snapshotKey struct{}
type requestSnapshot struct {
	manager *Manager
	config  *Config
}
type policyTransport struct {
	manager *Manager
	legacy  http.RoundTripper
}

func (t *policyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	manager := t.manager
	c := manager.ConfigSnapshot()
	pinned, ok := request.Context().Value(snapshotKey{}).(requestSnapshot)
	if !ok && request.Response != nil && request.Response.Request != nil {
		pinned, ok = request.Response.Request.Context().Value(snapshotKey{}).(requestSnapshot)
	}
	if ok {
		manager, c = pinned.manager, pinned.config
	}
	if c == nil {
		return t.legacy.RoundTrip(request)
	}
	base := directTransport()
	if c.Mode == "proxy" {
		proxy, _ := url.Parse(c.Proxy.URL)
		base.Proxy = func(r *http.Request) (*url.URL, error) {
			if bypassProxy(r.URL) {
				return nil, nil
			}
			return proxy, nil
		}
	}
	copy := request.Clone(context.WithValue(request.Context(), snapshotKey{}, requestSnapshot{manager: manager, config: c}))
	response, err := (&snapshotTransport{manager: manager, config: *c, base: base}).RoundTrip(copy)
	if response != nil {
		response.Request = copy
	}
	// Closing idle sockets does not interrupt active response bodies.
	base.CloseIdleConnections()
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		if errors.Is(err, ErrNoCandidate) {
			return nil, ErrNoCandidate
		}
		return nil, errors.New("网络连接失败，请检查当前网络模式")
	}
	return response, nil
}

// Keep redirect handling at http.Client level so callers' security checks
// still run. Response.Request carries the first-hop policy, not a secret URL.
func pinRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("重定向次数过多")
	}
	if request.Response != nil && request.Response.Request != nil {
		if pinned, ok := request.Response.Request.Context().Value(snapshotKey{}).(requestSnapshot); ok {
			*request = *request.WithContext(context.WithValue(request.Context(), snapshotKey{}, pinned))
		}
	}
	return nil
}
func (t *policyTransport) CloseIdleConnections() {
	if c, ok := t.legacy.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

type snapshotTransport struct {
	manager *Manager
	config  Config
	base    http.RoundTripper
}

func sensitiveRequest(r *http.Request) bool {
	for name := range r.Header {
		switch http.CanonicalHeaderKey(name) {
		case "Accept", "Accept-Encoding", "User-Agent", "Range", "If-None-Match", "If-Modified-Since", "Cache-Control", "Pragma":
		default:
			return true
		}
	}
	return r.Method != "GET" && r.Method != "HEAD" || r.Body != nil && r.Body != http.NoBody || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.User != nil || !publicQuery(r.URL)
}

// Only known numeric pagination on public GitHub API reads can leave the origin.
func publicQuery(target *url.URL) bool {
	if target.RawQuery == "" {
		return true
	}
	if groupFor(target) != "github-api" {
		return false
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return false
	}
	for key, values := range query {
		if (key != "page" && key != "per_page") || len(values) != 1 {
			return false
		}
		if value, err := strconv.ParseUint(values[0], 10, 32); err != nil || value == 0 {
			return false
		}
	}
	return true
}

type mirrorResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b mirrorResponseBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}

func roundTripMirror(base http.RoundTripper, request *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(request.Context())
	// Limit the wait for a mirror to begin responding, while allowing a
	// successful download to stream under the original caller deadline.
	timer := time.AfterFunc(6*time.Second, cancel)
	response, err := base.RoundTrip(request.WithContext(ctx))
	if !timer.Stop() || err != nil {
		cancel()
		if response != nil {
			response.Body.Close()
		}
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, err
	}
	response.Body = mirrorResponseBody{ReadCloser: response.Body, cancel: cancel}
	return response, nil
}

func (t *snapshotTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.config.Mode != "auto" || bypassProxy(r.URL) || sensitiveRequest(r) {
		return t.base.RoundTrip(r)
	}
	id := groupFor(r.URL)
	if id == "" {
		return t.base.RoundTrip(r)
	}
	g := groupConfig(t.config, id)
	// One bounded retry per eligible entry; writes and credentials never enter here.
	for attempt := 0; attempt <= len(g.Candidates); attempt++ {
		selected, err := t.manager.selected(r.Context(), t.config, g)
		if err != nil && !errors.Is(err, ErrNoCandidate) {
			return nil, err
		}
		if err != nil || selected.Current == "official" {
			return t.base.RoundTrip(r)
		}
		copy := r.Clone(r.Context())
		copy.URL, err = rewriteMirrorURL(selected.Current, r.URL)
		if err != nil {
			t.manager.failCandidate(t.config, g, selected.Current)
			continue
		}
		copy.Host = ""
		response, err := roundTripMirror(t.base, copy)
		if err == nil {
			code := response.StatusCode
			usable := code >= 200 && code < 300 || code == http.StatusNotModified
			if code >= 300 && code < 400 && code != http.StatusNotModified {
				if value := response.Header.Get("Location"); value != "" {
					if location, err := copy.URL.Parse(value); err == nil && groupFor(location) != id {
						response.Header.Set("Location", location.String())
						usable = true
					}
				}
			}
			if usable && !(id == "github-api" && strings.Contains(response.Header.Get("Content-Type"), "text/html")) {
				return response, nil
			}
		}
		if response != nil {
			response.Body.Close()
		}
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		t.manager.failCandidate(t.config, g, selected.Current)
	}
	return t.base.RoundTrip(r)
}
func (m *Manager) PreviewConfig(ctx context.Context, input Config, target string) (CheckResult, error) {
	prepared, err := m.saveConfig(input, true)
	if err != nil {
		return CheckResult{}, err
	}
	draft := &Manager{config: prepared.Config}
	if _, ok := groupDefinition(target); ok {
		if prepared.Mode == "auto" {
			state := probeGroupWith(groupConfig(*prepared.Config, target), nil, func(d GroupDefinition, entry string) CandidateState {
				return probeCandidateContext(ctx, d, entry)
			})
			return CheckResult{OK: state.State == "ready", Target: target, LatencyMS: state.LatencyMS, Message: map[bool]string{true: "连接正常", false: "没有可用镜像"}[state.State == "ready"]}, nil
		}
	}
	address := "https://api.github.com/"
	if d, ok := groupDefinition(target); ok {
		address = d.Probe
	}
	request, _ := http.NewRequestWithContext(ctx, "GET", address, nil)
	started := time.Now()
	client := draft.Client(8 * time.Second)
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	result := CheckResult{Target: target, LatencyMS: time.Since(started).Milliseconds(), Message: "连接失败"}
	if err != nil {
		return result, nil
	}
	defer response.Body.Close()
	result.Status = response.StatusCode
	result.OK = response.StatusCode >= 200 && response.StatusCode < 300
	if result.OK {
		result.Message = "连接正常"
	}
	return result, nil
}

// TestMirror checks exactly one draft mirror without changing routing state.
func (m *Manager) TestMirror(ctx context.Context, id, entry string) (CandidateState, error) {
	d, ok := groupDefinition(id)
	if !ok {
		return CandidateState{}, errors.New("请选择资源分类")
	}
	if entry != "official" {
		if len(entry) > 2048 {
			return CandidateState{}, errors.New("镜像地址过长")
		}
		var err error
		entry, err = normalizeGroupMirror(id, entry)
		if err != nil {
			return CandidateState{}, err
		}
	}
	if m.probe != nil {
		return m.probe(d, entry), nil
	}
	return probeCandidateContext(ctx, d, entry), nil
}
