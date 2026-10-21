// Package packagemanager describes package-manager differences shared by the
// project creator, robot actions and command-scoped network policy.
package packagemanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	YarnVersion       = "1.22.22"
	ModernYarnVersion = "4.9.2"
	PNPMVersion       = "9.15.0"
)

var versionPattern = regexp.MustCompile(`^\d+(?:\.\d+){0,2}(?:-[0-9A-Za-z.-]+)?$`)

func DeclaredVersion(root, manager string) string {
	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil {
		return ""
	}
	name, version, _ := strings.Cut(strings.TrimSpace(manifest.PackageManager), "@")
	// Corepack may append an integrity hash to the version.
	version, _, _ = strings.Cut(version, "+")
	if strings.EqualFold(name, manager) && versionPattern.MatchString(version) {
		return version
	}
	return ""
}

func ModernYarn(root string) bool {
	if version := DeclaredVersion(root, "yarn"); version != "" {
		major, _ := strconv.Atoi(strings.Split(version, ".")[0])
		return major >= 2
	}
	_, err := os.Stat(filepath.Join(root, ".yarnrc.yml"))
	return err == nil
}

func requestedVersion(root, manager string) string {
	version := DeclaredVersion(root, manager)
	if manager == "yarn" {
		if version == "" {
			version = YarnVersion
			if ModernYarn(root) {
				version = ModernYarnVersion
			}
		}
	} else if manager == "pnpm" && version == "" {
		version = PNPMVersion
	}
	return version
}

func Spec(root, manager string) string { return manager + "@" + requestedVersion(root, manager) }

// NPXArgs never uses latest: a fallback must not silently switch CLI dialects.
// Yarn 2+ is distributed as @yarnpkg/cli-dist, not yarn on the npm registry.
func NPXArgs(root, manager string) []string {
	version := requestedVersion(root, manager)
	major, _ := strconv.Atoi(strings.Split(version, ".")[0])
	if manager == "yarn" && major >= 2 {
		return []string{"--yes", "--package=@yarnpkg/cli-dist@" + version, "yarn"}
	}
	return []string{"--yes", Spec(root, manager)}
}
