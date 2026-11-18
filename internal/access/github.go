package access

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// GitHubBindingKey scopes a pending authorization to the current session and
// password, so changing the password also invalidates unfinished bindings.
func (m *Manager) GitHubBindingKey(token string, password *string) (string, error) {
	data, err := m.current()
	if err != nil {
		return "", err
	}
	account, ok := m.tokenAccount(data, token)
	if !data.Enabled || !ok {
		return "", errors.New("请先登录身份认证账户")
	}
	if password != nil && bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(*password)) != nil {
		return "", errors.New("工作台密码错误")
	}
	return githubBindingKey(token, account.PasswordHash), nil
}

func githubBindingKey(token, hash string) string {
	digest := sha256.Sum256([]byte(token + "\x00" + hash))
	return hex.EncodeToString(digest[:])
}

func (m *Manager) SetGitHubBinding(token, bindingKey string, id int64, login string) error {
	unlock, err := m.lockWrite()
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return err
	}
	account, ok := m.tokenAccount(m.data, token)
	if !m.data.Enabled || !ok {
		return errors.New("请先登录身份认证账户")
	}
	index := accountIndex(m.data.Accounts, account.Account)
	if id != 0 {
		if bindingKey != githubBindingKey(token, account.PasswordHash) {
			return errors.New("工作台密码已更改，请重新授权")
		}
		if account.GitHubID != 0 && account.GitHubID != id {
			return errors.New("请先解除当前 GitHub 绑定")
		}
		for i, existing := range m.data.Accounts {
			if i != index && existing.GitHubID == id {
				return errors.New("此 GitHub 账号已绑定其他工作台账户")
			}
		}
	}
	m.data.Accounts[index].GitHubID = id
	m.data.Accounts[index].GitHubLogin = login
	return m.persist()
}

func (m *Manager) LoginWithGitHub(id int64) (string, string, error) {
	data, err := m.current()
	if err != nil {
		return "", "", err
	}
	if data.Enabled && id > 0 {
		for _, account := range data.Accounts {
			if account.GitHubID == id && account.Enabled {
				token, err := m.issueToken(data, account.Account)
				return token, account.Account, err
			}
		}
	}
	return "", "", errors.New("此 GitHub 账号未绑定可登录的工作台账户")
}
