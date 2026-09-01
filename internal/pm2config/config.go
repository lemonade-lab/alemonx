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
	"sort"
	"strconv"
	"strings"
)

const (
	namespace  = "alemonx"
	idFileName = ".alemonx-id"
)

// Options is the small, safe subset of PM2 settings exposed by the workbench.
// Low-level ecosystem fields remain generated defaults.
type Options struct {
	Name        string            `json:"name"`
	Script      string            `json:"script"`
	Autorestart bool              `json:"autorestart"`
	MaxRestarts int               `json:"maxRestarts"`
	MaxMemory   string            `json:"maxMemory,omitempty"`
	Env         map[string]string `json:"env"`
}

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
	return ConfigWithOptions(root, DefaultOptions(root))
}

// DefaultOptions describes the conventional production registration.
func DefaultOptions(root string) Options {
	return Options{
		Name:        Name(root),
		Script:      "./index.js",
		Autorestart: true,
		MaxRestarts: 10,
		Env:         map[string]string{"NODE_ENV": "production"},
	}
}

// ConfigWithOptions renders a managed ecosystem file from regular settings.
func ConfigWithOptions(root string, options Options) string {
	path, err := filepath.Abs(root)
	if err != nil {
		path = filepath.Clean(root)
	}
	defaults := DefaultOptions(path)
	if strings.TrimSpace(options.Name) == "" {
		options.Name = defaults.Name
	}
	if strings.TrimSpace(options.Script) == "" {
		options.Script = defaults.Script
	}
	if options.MaxRestarts < 0 {
		options.MaxRestarts = defaults.MaxRestarts
	}
	if options.Env == nil {
		options.Env = defaults.Env
	}
	metadata, _ := json.Marshal(options)
	var fields strings.Builder
	fields.WriteString("// alemonx-pm2: " + string(metadata) + "\n")
	fields.WriteString("const path = require('node:path')\n\nmodule.exports = {\n  apps: [\n    {\n")
	fields.WriteString("      name: " + strconv.Quote(options.Name) + ",\n")
	fields.WriteString("      namespace: " + strconv.Quote(namespace) + ",\n")
	fields.WriteString("      cwd: __dirname,\n")
	fields.WriteString("      script: " + jsSingleQuote(options.Script) + ",\n")
	fields.WriteString("      autorestart: " + strconv.FormatBool(options.Autorestart) + ",\n")
	fields.WriteString("      min_uptime: '10s',\n")
	fields.WriteString("      max_restarts: " + strconv.Itoa(options.MaxRestarts) + ",\n")
	if strings.TrimSpace(options.MaxMemory) != "" {
		fields.WriteString("      max_memory_restart: " + strconv.Quote(options.MaxMemory) + ",\n")
	}
	fields.WriteString("      restart_delay: 3000,\n")
	fields.WriteString("      exp_backoff_restart_delay: 1000,\n")
	fields.WriteString("      env: {\n")
	// PM2 launches outside the workbench process, so it must receive the same
	// file transport used by foreground robots for login/QR lifecycle events.
	fields.WriteString("        ALEMON_CBP_FILE_TRANSPORT: '1',\n")
	fields.WriteString("        ALEMON_CBP_FILE_DIR: path.join(__dirname, '.alemon', 'cbp'),\n")
	keys := make([]string, 0, len(options.Env))
	for key := range options.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fields.WriteString("        " + key + ": " + jsSingleQuote(options.Env[key]) + ",\n")
	}
	fields.WriteString("      }\n")
	fields.WriteString("    }\n  ]\n};\n")
	return fields.String()
}

func jsSingleQuote(value string) string {
	return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'", "\n", "\\n", "\r", "\\r").Replace(value) + "'"
}
