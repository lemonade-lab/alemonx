package systemnetwork

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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
		case "Accept", "Accept-Encoding", "User-Agent", "Range", "If-None-Match", "If-Modified-Since":
		default:
			return true
		}
	}
	return r.Method != "GET" && r.Method != "HEAD" || r.Body != nil && r.Body != http.NoBody || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.User != nil || r.URL.RawQuery != ""
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
	for attempt := 0; attempt <= len(g.Candidates)+1; attempt++ {
		selected, err := t.manager.selected(r.Context(), t.config, g)
		if err != nil {
			return nil, err
		}
		copy := r.Clone(r.Context())
		if selected.Current != "official" {
			copy.URL, err = rewriteMirrorURL(selected.Current, r.URL)
			if err != nil {
				return nil, err
			}
			copy.Host = ""
		}
		response, err := t.base.RoundTrip(copy)
		if response != nil && selected.Current != "official" && response.Header.Get("Location") != "" {
			if location, e := copy.URL.Parse(response.Header.Get("Location")); e == nil {
				response.Header.Set("Location", location.String())
			}
		}
		if err == nil && response.StatusCode < 500 && response.StatusCode != 429 {
			return response, nil
		}
		if response != nil {
			response.Body.Close()
		}
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		t.manager.failCandidate(t.config, g, selected.Current)
	}
	return nil, ErrNoCandidate
}
func (m *Manager) PreviewConfig(ctx context.Context, input Config, target string) (CheckResult, error) {
	prepared, err := m.saveConfig(input, true)
	if err != nil {
		return CheckResult{}, err
	}
	draft := &Manager{config: prepared.Config}
	if d, ok := groupDefinition(target); ok {
		if prepared.Mode == "auto" {
			state := probeGroup(groupConfig(*prepared.Config, target), nil, "")
			return CheckResult{OK: state.State == "ready", Target: target, LatencyMS: state.LatencyMS, Message: map[bool]string{true: "连接正常", false: "没有可用入口"}[state.State == "ready"]}, nil
		}
		_ = d
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
