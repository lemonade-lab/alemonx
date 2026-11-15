package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"alemonx/internal/githubauth"
)

func (s *server) authGitHubHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	binding := r.URL.Path == authGitHubBindingPath
	if r.Method != http.MethodPost && !(binding && r.Method == http.MethodDelete) {
		writeError(w, http.StatusMethodNotAllowed, "该操作暂不支持。")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "请提交 JSON 请求。")
		return
	}

	token := s.authToken(r)
	status, err := s.auth.Status(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法读取身份认证状态。")
		return
	}
	if !status.Enabled || (binding && !status.Authenticated) {
		writeError(w, http.StatusUnauthorized, "请先开启身份认证并使用账户密码登录。")
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.auth.SetGitHubBinding(token, "", 0, ""); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	var input struct {
		FlowID   string `json:"flowId"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "GitHub 授权请求无效。")
		return
	}
	owner := "workbench-login"
	var bindingKey string
	if binding {
		var password *string
		if input.FlowID == "" {
			password = &input.Password
		}
		bindingKey, err = s.auth.GitHubBindingKey(token, password)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		owner = "workbench-binding:" + bindingKey
	}
	if input.FlowID == "" {
		flow, err := githubauth.StartIdentityFlow(owner)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, flow)
		return
	}
	result, err := githubauth.PollIdentityFlow(input.FlowID, owner)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if result.Status != "ok" {
		writeJSON(w, http.StatusOK, result)
		return
	}
	if binding {
		if err := s.auth.SetGitHubBinding(token, bindingKey, result.UserID, result.Login); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	session, account, err := s.auth.LoginWithGitHub(result.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.setAuthCookie(w, session)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "account": account})
}
