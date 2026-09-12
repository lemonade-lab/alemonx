package dsh

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

const keyringService = "alemonx.dsh"

// SecretStore isolates OS credential storage so normal configuration and
// diagnostics never need to carry a provider key.
type SecretStore interface {
	Set(root, value string) error
	Get(root string) (string, error)
}

type KeyringSecretStore struct{}

func secretAccount(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "deepseek:" + hex.EncodeToString(sum[:16])
}

func (KeyringSecretStore) Set(root, value string) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(value) == "" {
		return errors.New("密钥或机器人目录无效")
	}
	return keyring.Set(keyringService, secretAccount(canonicalRoot(root)), value)
}

func (KeyringSecretStore) Get(root string) (string, error) {
	value, err := keyring.Get(keyringService, secretAccount(canonicalRoot(root)))
	if err != nil && canonicalRoot(root) != root {
		value, err = keyring.Get(keyringService, secretAccount(root))
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("系统钥匙串中不存在 DSH 密钥")
	}
	return value, nil
}
