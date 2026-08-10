// Package pm2config builds isolated PM2 ecosystem files for local projects.
package pm2config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const namespace = "alemonx"

var unsafeName = regexp.MustCompile(`[^a-z0-9]+`)

// Name returns a stable PM2 application name for one project directory. PM2's
// daemon is shared by all local projects, so the human-readable directory name
// alone is insufficient: the path digest prevents same-name projects colliding.
func Name(root string) string {
	path, err := filepath.Abs(root)
	if err != nil {
		path = filepath.Clean(root)
	}
	base := strings.ToLower(filepath.Base(path))
	base = strings.Trim(unsafeName.ReplaceAllString(base, "-"), "-")
	if base == "" {
		base = "robot"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	sum := sha256.Sum256([]byte(filepath.ToSlash(path)))
	return fmt.Sprintf("%s-%s-%s", namespace, base, hex.EncodeToString(sum[:])[:8])
}

// Config returns a complete PM2 ecosystem config. cwd is explicit so PM2
// status and lifecycle actions remain tied to this exact project directory.
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
		"      cwd: " + strconv.Quote(path) + ",\n" +
		"      script: './index.js',\n" +
		"      env: {\n" +
		"        NODE_ENV: 'production'\n" +
		"      }\n" +
		"    }\n" +
		"  ]\n" +
		"};\n"
}
