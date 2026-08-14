// Package githubauth implements GitHub OAuth Device Flow for the workbench.
// The flow needs only a public OAuth App client_id (no client_secret), which
// makes it safe for a local desktop application. The resulting token is stored
// in the shared github-token file so every api.github.com request can attach
// it automatically and raise the rate limit from 60 to 5000 requests/hour.
package githubauth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"alemonx/internal/httpcache"
)

// Endpoints are package variables so tests can point them at local servers.
var (
	deviceCodeURL = "https://github.com/login/device/code"
	tokenURL      = "https://github.com/login/oauth/access_token"
	userURL       = "https://api.github.com/user"
)

// defaultClientID is the project's registered GitHub OAuth App identifier.
// 注册完成后（Settings → Developer settings → OAuth Apps，并启用 Device
// flow）把这里替换成真实的 Client ID，用户即可开箱即用；仍可在设置页覆盖。
const defaultClientID = "Ov23liJVFfF4UAhCWTfM"

type AuthStatus struct {
	LoggedIn           bool   `json:"loggedIn"`
	Login              string `json:"login,omitempty"`
	ClientIDConfigured bool   `json:"clientIdConfigured"`
}

type DeviceFlow struct {
	FlowID          string `json:"flowId"`
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
}

type PollResult struct {
	Status   string `json:"status"`
	Login    string `json:"login,omitempty"`
	Interval int    `json:"interval,omitempty"`
	Message  string `json:"message,omitempty"`
}

type deviceFlowState struct {
	deviceCode string
	interval   int
	expiresAt  time.Time
}

var (
	flowsMu sync.Mutex
	flows   = map[string]*deviceFlowState{}

	loginMu    sync.Mutex
	loginCache string
	loginAt    time.Time
)

// ClientID returns the registered OAuth App client id from
// ALEMONX_GITHUB_CLIENT_ID, the github-oauth-client-id file, or the built-in
// project default. An empty value means Device Flow cannot start.
func ClientID() string {
	if id := strings.TrimSpace(os.Getenv("ALEMONX_GITHUB_CLIENT_ID")); id != "" {
		return id
	}
	if id := readClientIDFile(); id != "" {
		return id
	}
	return strings.TrimSpace(defaultClientID)
}

// ClientIDSource describes where the effective Client ID comes from:
// env, file, builtin, or an empty string when nothing is configured.
func ClientIDSource() (value, source string) {
	if id := strings.TrimSpace(os.Getenv("ALEMONX_GITHUB_CLIENT_ID")); id != "" {
		return id, "env"
	}
	if id := readClientIDFile(); id != "" {
		return id, "file"
	}
	if id := strings.TrimSpace(defaultClientID); id != "" {
		return id, "builtin"
	}
	return "", ""
}

// SaveClientID persists a user-provided Client ID to the local config file.
// An empty value restores the built-in default by removing the override.
func SaveClientID(id string) error {
	path := clientIDPath()
	if path == "" {
		return errors.New("无法定位用户配置目录")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("无法移除 Client ID 配置：%w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("无法创建配置目录：%w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return fmt.Errorf("无法保存 Client ID：%w", err)
	}
	return nil
}

func readClientIDFile() string {
	path := clientIDPath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func clientIDPath() string {
	directory, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "alemonjs", "github-oauth-client-id")
}

// Status reports whether a token is configured and, when possible, which
// GitHub account it belongs to.
func Status() (AuthStatus, error) {
	_, token := httpcache.GitHubTokenHeader()
	status := AuthStatus{LoggedIn: token != ""}
	if status.LoggedIn {
		status.Login = cachedLogin()
		if status.Login == "" {
			if login, err := fetchLogin(); err == nil {
				status.Login = login
				cacheLogin(login)
			}
		}
	}
	status.ClientIDConfigured = ClientID() != ""
	return status, nil
}

// StartDeviceFlow begins the GitHub device authorization flow and stores the
// pending state under a flow id for later polling.
func StartDeviceFlow() (DeviceFlow, error) {
	clientID := ClientID()
	if clientID == "" {
		return DeviceFlow{}, errors.New("未配置 GitHub OAuth App 的 Client ID；请在环境中设置 ALEMONX_GITHUB_CLIENT_ID 或创建配置文件 github-oauth-client-id")
	}
	form := url.Values{"client_id": {clientID}}
	body, err := postForm(deviceCodeURL, form)
	if err != nil {
		return DeviceFlow{}, errors.New("无法启动 GitHub 授权，请检查网络后重试")
	}
	var response struct {
		DeviceCode       string `json:"device_code"`
		UserCode         string `json:"user_code"`
		VerificationURI  string `json:"verification_uri"`
		ExpiresIn        int    `json:"expires_in"`
		Interval         int    `json:"interval"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return DeviceFlow{}, errors.New("GitHub 授权响应无法识别")
	}
	if response.Error != "" {
		message := response.ErrorDescription
		if message == "" {
			message = response.Error
		}
		return DeviceFlow{}, fmt.Errorf("GitHub 授权失败：%s", message)
	}
	if response.DeviceCode == "" || response.UserCode == "" {
		return DeviceFlow{}, errors.New("GitHub 授权响应缺少设备码")
	}
	interval := response.Interval
	if interval < 5 {
		interval = 5
	}
	flowID := randomID()
	flowsMu.Lock()
	flows[flowID] = &deviceFlowState{
		deviceCode: response.DeviceCode,
		interval:   interval,
		expiresAt:  time.Now().Add(time.Duration(response.ExpiresIn) * time.Second),
	}
	flowsMu.Unlock()
	return DeviceFlow{
		FlowID:          flowID,
		UserCode:        response.UserCode,
		VerificationURI: response.VerificationURI,
		ExpiresIn:       response.ExpiresIn,
		Interval:        interval,
	}, nil
}

// PollDeviceFlow advances the pending device flow. On success the token is
// persisted and the flow is removed.
func PollDeviceFlow(flowID string) (PollResult, error) {
	flowsMu.Lock()
	state := flows[flowID]
	flowsMu.Unlock()
	if state == nil {
		return PollResult{Status: "expired"}, nil
	}
	if time.Now().After(state.expiresAt) {
		removeFlow(flowID)
		return PollResult{Status: "expired"}, nil
	}
	form := url.Values{
		"client_id":   {ClientID()},
		"device_code": {state.deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	body, err := postForm(tokenURL, form)
	if err != nil {
		return PollResult{}, errors.New("无法读取 GitHub 授权状态，请检查网络后重试")
	}
	var response struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return PollResult{}, errors.New("GitHub 授权状态无法识别")
	}
	switch response.Error {
	case "":
		if response.AccessToken == "" {
			removeFlow(flowID)
			return PollResult{}, errors.New("GitHub 授权响应缺少 Token")
		}
		removeFlow(flowID)
		if err := httpcache.SaveToken(response.AccessToken); err != nil {
			return PollResult{}, err
		}
		login := ""
		if value, loginErr := fetchLogin(); loginErr == nil {
			login = value
			cacheLogin(value)
		}
		return PollResult{Status: "ok", Login: login}, nil
	case "authorization_pending":
		return PollResult{Status: "pending", Interval: state.interval}, nil
	case "slow_down":
		state.interval += 5
		return PollResult{Status: "slow_down", Interval: state.interval}, nil
	case "expired_token":
		removeFlow(flowID)
		return PollResult{Status: "expired"}, nil
	case "access_denied":
		removeFlow(flowID)
		return PollResult{Status: "denied"}, nil
	default:
		removeFlow(flowID)
		message := response.ErrorDescription
		if message == "" {
			message = response.Error
		}
		return PollResult{Status: "error", Message: message}, nil
	}
}

// SaveManualToken persists a user-provided PAT, reusing the same storage as
// Device Flow so every GitHub API request benefits from it.
func SaveManualToken(token string) (AuthStatus, error) {
	if err := httpcache.SaveToken(token); err != nil {
		return AuthStatus{}, err
	}
	login := ""
	if value, err := fetchLogin(); err == nil {
		login = value
		cacheLogin(value)
	}
	return AuthStatus{LoggedIn: true, Login: login, ClientIDConfigured: ClientID() != ""}, nil
}

// Logout removes the stored token and any cached account identity.
func Logout() error {
	loginMu.Lock()
	loginCache = ""
	loginAt = time.Time{}
	loginMu.Unlock()
	return httpcache.RemoveToken()
}

func cachedLogin() string {
	loginMu.Lock()
	defer loginMu.Unlock()
	if time.Since(loginAt) < time.Hour {
		return loginCache
	}
	return ""
}

func cacheLogin(login string) {
	loginMu.Lock()
	loginCache = login
	loginAt = time.Now()
	loginMu.Unlock()
}

func fetchLogin() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := httpcache.GetWithHeaders(client, userURL, time.Hour, map[string]string{
		"Accept": "application/vnd.github+json",
	})
	if err != nil || response.Status != http.StatusOK {
		return "", errors.New("无法读取 GitHub 用户信息")
	}
	var data struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(response.Body, &data); err != nil || data.Login == "" {
		return "", errors.New("GitHub 用户信息无法识别")
	}
	return data.Login, nil
}

func postForm(endpoint string, form url.Values) ([]byte, error) {
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "alemonx")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 请求返回 %d", response.StatusCode)
	}
	return readBody(response)
}

func readBody(response *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func removeFlow(flowID string) {
	flowsMu.Lock()
	delete(flows, flowID)
	flowsMu.Unlock()
}

func randomID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
