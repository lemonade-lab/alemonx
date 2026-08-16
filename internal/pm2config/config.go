// Package pm2config builds isolated PM2 ecosystem files for local projects.
package pm2config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	namespace  = "alemonx"
	idFileName = ".alemonx-id"
)

var unsafeName = regexp.MustCompile(`[^a-z0-9]+`)

// EnsureID creates the stable per-project identity file (.alemonx-id) when it
// is missing. The identity travels with the robot directory, so the PM2 app
// name no longer changes when the directory is moved. Existing identity files
// are never overwritten.
func EnsureID(root string) error {
	path := filepath.Join(root, idFileName)
	if data, err := os.ReadFile(path); err == nil {
		if len(strings.TrimSpace(string(data))) >= 16 {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("生成项目身份失败：%w", err)
	}
	return os.WriteFile(path, []byte(hex.EncodeToString(raw)), 0o644)
}

func readID(root string) string {
	data, err := os.ReadFile(filepath.Join(root, idFileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// projectName returns a sanitized readable name from package.json, falling
// back to the directory base name. It is used for the stable identity branch.
func projectName(root string) string {
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var manifest struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			if base := sanitize(manifest.Name); base != "" {
				return base
			}
		}
	}
	return sanitize(filepath.Base(filepath.Clean(root)))
}

func sanitize(value string) string {
	value = strings.ToLower(value)
	value = strings.Trim(unsafeName.ReplaceAllString(value, "-"), "-")
	if value == "" {
		return "robot"
	}
	if len(value) > 40 {
		value = value[:40]
	}
	return value
}

// Name returns the PM2 application name for a project. When the project has a
// stable identity (.alemonx-id) the name is derived from the package.json name
// plus that identity, so moving the directory does not change the name.
// Projects without an identity keep the legacy path-digest name for
// compatibility with existing registrations.
func Name(root string) string {
	path, err := filepath.Abs(root)
	if err != nil {
		path = filepath.Clean(root)
	}
	if id := readID(path); len(id) >= 16 {
		sum := sha256.Sum256([]byte(id))
		return fmt.Sprintf("%s-%s-%s", namespace, projectName(path), hex.EncodeToString(sum[:])[:8])
	}
	sum := sha256.Sum256([]byte(filepath.ToSlash(path)))
	return fmt.Sprintf("%s-%s-%s", namespace, sanitize(filepath.Base(path)), hex.EncodeToString(sum[:])[:8])
}

// Config returns a complete PM2 ecosystem config. cwd derives from the config
// file's own location (__dirname), so moving or renaming the robot directory
// does not leave PM2 pointing at a stale absolute path. The restart backoff
// fields match the bundled project templates.
func Config(root string) string {
	path, err := filepath.Abs(root)
	if err != nil {
		path = filepath.Clean(root)
	}
	return "module.exports = {\n" +
		"  apps: [\n" +
		"    {\n" +
		"      name: " + strconv.Quote(Name(path)) + ",\n" +
		"      namespace: " + strconv.Quote(namespace) + ",\n" +
		"      cwd: __dirname,\n" +
		"      script: './index.js',\n" +
		"      autorestart: true,\n" +
		"      min_uptime: '10s',\n" +
		"      max_restarts: 10,\n" +
		"      restart_delay: 3000,\n" +
		"      exp_backoff_restart_delay: 1000,\n" +
		"      env: {\n" +
		"        NODE_ENV: 'production'\n" +
		"      }\n" +
		"    }\n" +
		"  ]\n" +
		"};\n"
}
