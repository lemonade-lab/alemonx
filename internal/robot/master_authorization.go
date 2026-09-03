package robot

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

var masterAuthorizationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,160}$`)

// SetMasterAuthorization adds or removes one platform user from master_id.
// It only rewrites that top-level section, keeping the rest of the user's
// AlemonJS configuration intact.
func (m Manager) SetMasterAuthorization(root, userID string, enabled bool) (Result, error) {
	userID = strings.TrimSpace(userID)
	if !masterAuthorizationIDPattern.MatchString(userID) {
		return Result{}, errors.New("主人用户 ID 无效")
	}
	return m.UpdateRuntimeConfig(root, "", func(content string) (string, error) {
		content = stripYAMLBOM(content)
		parsed := map[string]any{}
		if strings.TrimSpace(content) != "" {
			if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
				return "", errors.New("alemon.config.yaml 无法解析，不能安全更新主人权限")
			}
		}
		ids := masterAuthorizationIDs(parsed["master_id"])
		if enabled {
			ids[userID] = true
		} else {
			delete(ids, userID)
		}
		return replaceMasterAuthorizationSection(content, ids), nil
	})
}

func masterAuthorizationIDs(value any) map[string]bool {
	ids := map[string]bool{}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if id := strings.TrimSpace(strings.TrimSpace(toConfigString(item))); masterAuthorizationIDPattern.MatchString(id) {
				ids[id] = true
			}
		}
	case map[string]any:
		for id, enabled := range typed {
			if active, ok := enabled.(bool); ok && active && masterAuthorizationIDPattern.MatchString(id) {
				ids[id] = true
			}
		}
	}
	return ids
}

func toConfigString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(typed)
	default:
		return ""
	}
}

func replaceMasterAuthorizationSection(content string, ids map[string]bool) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	lines = removeYAMLSections(lines, "master_id:")
	if len(ids) == 0 {
		return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, "master_id:")
	for _, id := range keys {
		lines = append(lines, "  '"+strings.ReplaceAll(id, "'", "''")+"': true")
	}
	return strings.Join(lines, "\n") + "\n"
}
