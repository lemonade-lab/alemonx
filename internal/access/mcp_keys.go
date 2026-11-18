package access

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const mcpKeyPrefix = "alx_mcp_"

var (
	ErrMCPKeyExists      = errors.New("已有连接密钥，请选择重新生成")
	ErrMCPAccountChanged = errors.New("工作台账户已变更，请重新登录")
)

// UpdateMCPKey updates only the account from the authenticated workbench request.
// The plaintext is returned on creation or rotation and is never written to disk.
func (m *Manager) UpdateMCPKey(session string, identity Status, action string, diagnostic bool) (string, error) {
	unlock, err := m.lockWrite()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := m.reload(); err != nil {
		return "", err
	}
	status := m.sessionStatus(m.data, session)
	if status.Enabled != identity.Enabled || status.RobotScope != identity.RobotScope ||
		(status.Enabled && !status.Authenticated) {
		return "", ErrMCPAccountChanged
	}
	scope := status.RobotScope
	keys := &m.data.MCPKeys
	if diagnostic {
		keys = &m.data.MCPDiagnosticKeys
	}
	exists := (*keys)[scope] != ""
	switch action {
	case "revoke":
		if !exists {
			return "", nil
		}
		delete(*keys, scope)
		return "", m.persist()
	case "token":
		if exists {
			return "", ErrMCPKeyExists
		}
	case "rotate":
	default:
		return "", errors.New("未知 MCP 密钥操作")
	}

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := mcpKeyPrefix + base64.RawURLEncoding.EncodeToString(secret[:])
	if *keys == nil {
		*keys = make(map[string]string)
	}
	(*keys)[scope] = mcpKeyDigest(token)
	if err := m.persist(); err != nil {
		return "", err
	}
	return token, nil
}

func mcpKeyDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (m *Manager) MCPKeyStatus(token string) (Status, bool, error) {
	if !strings.HasPrefix(token, mcpKeyPrefix) || len(token) != len(mcpKeyPrefix)+base64.RawURLEncoding.EncodedLen(32) {
		return Status{}, false, nil
	}
	data, err := m.current()
	if err != nil {
		return Status{}, false, err
	}
	digest := mcpKeyDigest(token)
	match := func(scope string) (bool, bool) {
		if hmac.Equal([]byte(data.MCPDiagnosticKeys[scope]), []byte(digest)) {
			return true, true
		}
		return hmac.Equal([]byte(data.MCPKeys[scope]), []byte(digest)), false
	}
	if !data.Enabled {
		ok, diagnostic := match("")
		return Status{MCPDiagnostic: diagnostic}, ok, nil
	}
	for _, account := range data.Accounts {
		if account.Enabled {
			if ok, diagnostic := match(robotScope(account)); ok {
				status := authenticatedStatus(data, account)
				status.MCPDiagnostic = diagnostic
				return status, true, nil
			}
		}
	}
	return Status{}, false, nil
}
