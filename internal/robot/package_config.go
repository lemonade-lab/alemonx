package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"alemonx/internal/packageschema"
)

// PackageConfig is the workbench-facing shape of a package's AlemonJS
// declaration plus its current values from alemon.config.yaml.
type PackageConfig struct {
	Package       string                     `json:"package"`
	Namespace     string                     `json:"namespace"`
	Fields        []packageschema.Field      `json:"fields"`
	Values        map[string]any             `json:"values"`
	ConfigSource  packageschema.ConfigSource `json:"configSource,omitempty"`
	Logo          string                     `json:"logo,omitempty"`
	Commands      []packageschema.Command    `json:"commands,omitempty"`
	Platforms     []packageschema.Platform   `json:"platforms,omitempty"`
	WebServerPort bool                       `json:"webServerPort,omitempty"`
}

var packageNamePattern = regexp.MustCompile(`^(?:@[a-zA-Z0-9][a-zA-Z0-9._-]*/)?[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var yamlNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func (m Manager) PackageConfig(root, name string) (PackageConfig, error) {
	path, err := projectPath(root)
	if err != nil {
		return PackageConfig{}, err
	}
	if !packageNamePattern.MatchString(name) {
		return PackageConfig{}, errors.New("包名无效")
	}
	data, subject, err := installedPackageManifest(path, name)
	if err != nil {
		return PackageConfig{}, err
	}
	return packageConfigFromManifest(path, data, subject)
}

// installedPackageManifest resolves both dependency packages and packages kept
// in the robot's backpack. Local packages are workspace packages, so looking
// only in node_modules made their declared AlemonJS configuration impossible
// to manage from the backpack page.
func installedPackageManifest(project, name string) ([]byte, string, error) {
	data, err := os.ReadFile(filepath.Join(project, "node_modules", filepath.FromSlash(name), "package.json"))
	if err == nil {
		return data, "该包", nil
	}
	if !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("无法读取包配置声明：%w", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(project, "packages"))
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, "", fmt.Errorf("无法读取背包：%w", readErr)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate, candidateErr := os.ReadFile(filepath.Join(project, "packages", entry.Name(), "package.json"))
		if candidateErr != nil {
			continue
		}
		var manifest struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(candidate, &manifest) == nil && manifest.Name == name {
			return candidate, "背包中的本地插件包", nil
		}
	}
	return nil, "", errors.New("该包尚未安装到依赖或背包中")
}

// CurrentPackageConfig reads the robot project's own package.json. It is not a
// node_modules lookup: a project can expose its own alemonjs.config extension
// and should configure it from the main robot configuration screen.
func (m Manager) CurrentPackageConfig(root string) (PackageConfig, error) {
	path, err := projectPath(root)
	if err != nil {
		return PackageConfig{}, err
	}
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return PackageConfig{}, fmt.Errorf("无法读取当前项目的 package.json：%w", err)
	}
	return packageConfigFromManifest(path, data, "当前项目")
}

func packageConfigFromManifest(path string, data []byte, subject string) (PackageConfig, error) {
	declaration, err := packageschema.Parse(data)
	if err != nil {
		return PackageConfig{}, errors.New(subject + "的 " + err.Error())
	}
	namespace := declaration.ResolveNamespace()
	config := PackageConfig{
		Package:       declaration.Name,
		Namespace:     namespace,
		Fields:        declaration.Config,
		Values:        map[string]any{},
		ConfigSource:  declaration.ConfigSource,
		Logo:          declaration.Desktop.Logo,
		Commands:      declaration.Desktop.Commands,
		Platforms:     declaration.Desktop.Platforms,
		WebServerPort: declaration.Web.ServerPort,
	}
	if config.Fields == nil {
		config.Fields = []packageschema.Field{}
	}
	if len(declaration.Config) == 0 {
		return config, nil
	}
	content, err := os.ReadFile(filepath.Join(path, "alemon.config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return PackageConfig{}, fmt.Errorf("无法读取机器人运行配置：%w", err)
	}
	// Prefer the short connection key but keep reading the legacy scoped-package
	// key ('@alemonjs/onebot') so existing values survive the migration.
	candidates := []string{namespace}
	if namespace != declaration.Name {
		candidates = append(candidates, declaration.Name)
	}
	config.Values = readConfigValues(string(content), candidates, declaration.Config)
	return config, nil
}

// PlatformValueForLogin returns the explicit --platform value that must be
// written next to login, or "" when the runtime can derive it from login.
func (c PackageConfig) PlatformValueForLogin(login string) string {
	for _, platform := range c.Platforms {
		if platform.Name != login {
			continue
		}
		value := strings.TrimSpace(platform.Value)
		if value == "" || value == "@alemonjs/"+login {
			return ""
		}
		return value
	}
	return ""
}

func (m Manager) SavePackageConfig(root, name string, values map[string]any) (Result, error) {
	definition, err := m.PackageConfig(root, name)
	if err != nil {
		return Result{}, err
	}
	return m.savePackageConfigDefinition(root, definition, values)
}

func (m Manager) savePackageConfigDefinition(root string, definition PackageConfig, values map[string]any) (Result, error) {
	allowed := make(map[string]bool, len(definition.Fields))
	for _, field := range definition.Fields {
		allowed[field.Name] = true
	}
	for key := range values {
		if !allowed[key] {
			return Result{}, fmt.Errorf("配置项 %s 不属于该包", key)
		}
	}
	current, err := m.Read(root, "alemon.config.yaml")
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		return Result{}, err
	}
	content := ""
	if err == nil {
		content = current.Output
	}
	legacy := ""
	if definition.Namespace != definition.Package {
		legacy = definition.Package
	}
	updated, err := mergeConfigValuesWithLegacy(content, definition.Namespace, legacy, definition.Fields, values)
	if err != nil {
		return Result{}, err
	}
	return m.Write(root, "alemon.config.yaml", updated)
}

func (m Manager) SaveCurrentPackageConfig(root string, values map[string]any) (Result, error) {
	definition, err := m.CurrentPackageConfig(root)
	if err != nil {
		return Result{}, err
	}
	return m.savePackageConfigDefinition(root, definition, values)
}

// SaveLogin only changes login after the selected connection's declared
// required fields already have values. This keeps a package from being made
// active in alemon.config.yaml with an unusable, half-filled configuration.
// Third-party platform packages additionally get an explicit platform key when
// the runtime cannot derive it from the login value.
func (m Manager) SaveLogin(root, login, packageName string) (Result, error) {
	login = strings.TrimSpace(login)
	if login == "" || strings.ContainsAny(login, "\r\n") {
		return Result{}, errors.New("请填写有效的登录连接")
	}
	platformValue := ""
	if packageName != "" {
		definition, err := m.PackageConfig(root, packageName)
		if err != nil {
			return Result{}, err
		}
		missing := make([]string, 0)
		for _, field := range definition.Fields {
			configured := !isConfigEmpty(definition.Values[field.Name])
			if field.Required && !configured && !field.DefaultConfigured() {
				label := field.Description
				if label == "" {
					label = field.Name
				}
				missing = append(missing, label)
			}
		}
		if len(missing) > 0 {
			return Result{}, fmt.Errorf("请先完成 %s 的必填配置：%s", definition.Package, strings.Join(missing, "、"))
		}
		platformValue = definition.PlatformValueForLogin(login)
	}
	current, err := m.Read(root, "alemon.config.yaml")
	if err != nil && !strings.Contains(err.Error(), "no such file") {
		return Result{}, err
	}
	content := ""
	if err == nil {
		content = stripYAMLBOM(current.Output)
	}
	content = setTopLevelScalar(content, "login", strconv.Quote(login))
	if platformValue != "" {
		content = setTopLevelScalar(content, "platform", strconv.Quote(platformValue))
	}
	result, err := m.Write(root, "alemon.config.yaml", content)
	if err != nil {
		return Result{}, err
	}
	result.Output = "登录连接已保存。"
	return result, nil
}

// setTopLevelScalar replaces or appends a quoted top-level YAML scalar.
func setTopLevelScalar(content, key, value string) string {
	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*.*$`)
	line := key + ": " + value
	if pattern.MatchString(content) {
		return pattern.ReplaceAllString(content, line)
	}
	content = strings.TrimRight(content, "\n")
	if content != "" {
		content += "\n"
	}
	return content + line + "\n"
}

func readConfigValues(content string, namespaces []string, fields []packageschema.Field) map[string]any {
	content = stripYAMLBOM(content)
	content = dedupeYAMLSections(content)
	root := map[string]any{}
	if strings.TrimSpace(content) != "" {
		if err := yaml.Unmarshal([]byte(content), &root); err != nil {
			return map[string]any{}
		}
	}
	raw := map[string]any{}
	for _, namespace := range namespaces {
		section := normalizeSection(root[namespace])
		for key, value := range section {
			if _, exists := raw[key]; !exists {
				raw[key] = value
			}
		}
	}
	values := map[string]any{}
	for i := range fields {
		field := &fields[i]
		value, present := raw[field.Name]
		if !present {
			continue
		}
		coerced, err := field.Coerce(value)
		if err != nil {
			values[field.Name] = packageschema.NormalizeValue(value)
			continue
		}
		if coerced != nil {
			values[field.Name] = packageschema.NormalizeValue(coerced)
		}
	}
	return values
}

// mergeConfigValuesWithLegacy writes the short connection key and, when a file
// still carries the old scoped-package key ('@alemonjs/onebot'), migrates its
// section into the new key instead of leaving a stale duplicate. The managed
// section is regenerated in schema order; other sections and their comments
// are preserved verbatim.
func mergeConfigValuesWithLegacy(content, namespace, legacyNamespace string, fields []packageschema.Field, values map[string]any) (string, error) {
	content = stripYAMLBOM(content)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	legacyLines := []string{}
	if legacyNamespace != "" {
		lines = keepLastYAMLSections(lines, yamlKey(legacyNamespace)+":")
		if legacyStart, legacyEnd := findYAMLSection(lines, yamlKey(legacyNamespace)+":"); legacyStart >= 0 {
			legacyLines = append(legacyLines, lines[legacyStart+1:legacyEnd]...)
			lines = removeYAMLSection(lines, yamlKey(legacyNamespace)+":")
		}
	}
	// A Windows editor can leave a UTF-8 BOM in front of the first section;
	// an older save then appended a clean replacement. After the BOM is
	// stripped both sections share the same key, and a duplicate top-level key
	// makes the whole file unparseable. Use the newest occurrence as the base
	// and rebuild one canonical section by keeping only that last occurrence.
	lines = keepLastYAMLSections(lines, yamlKey(namespace)+":")
	start, end := findYAMLSection(lines, yamlKey(namespace)+":")
	existingLines := []string{}
	if start >= 0 {
		existingLines = append(existingLines, lines[start+1:end]...)
	} else {
		existingLines = append(existingLines, legacyLines...)
	}
	section := parseIndentedBlock(existingLines)
	for i := range fields {
		field := &fields[i]
		value, submitted := values[field.Name]
		if !submitted {
			continue
		}
		if isConfigEmpty(value) {
			delete(section, field.Name)
			continue
		}
		coerced, err := field.Coerce(value)
		if err != nil {
			return "", err
		}
		section[field.Name] = coerced
	}
	// Required and rule validation runs against the merged section so clearing
	// a value cannot silently leave an invalid configuration behind.
	for i := range fields {
		field := &fields[i]
		finalValue := section[field.Name]
		if field.Required && (finalValue == nil || isConfigEmpty(finalValue)) && !field.DefaultConfigured() {
			label := field.Description
			if label == "" {
				label = field.Name
			}
			return "", fmt.Errorf("请填写必填配置项：%s", label)
		}
		if messages := field.ValidateValue(finalValue); len(messages) > 0 {
			return "", fmt.Errorf("%s 配置无效：%s", field.Name, strings.Join(messages, "；"))
		}
	}
	sectionLines := marshalSection(section, fields)
	result := make([]string, 0, len(lines)+len(sectionLines)+1)
	if start < 0 {
		result = append(result, lines...)
		if len(result) > 0 {
			result = append(result, "")
		}
		result = append(result, yamlKey(namespace)+":")
		result = append(result, sectionLines...)
	} else {
		result = append(result, lines[:start]...)
		result = append(result, yamlKey(namespace)+":")
		result = append(result, sectionLines...)
		result = append(result, lines[end:]...)
	}
	return strings.Join(result, "\n") + "\n", nil
}

// parseIndentedBlock decodes an indented YAML block (the body of a section)
// by wrapping it in a root key, since the parser rejects root indentation.
func parseIndentedBlock(lines []string) map[string]any {
	if len(lines) == 0 {
		return map[string]any{}
	}
	wrapped := "root:\n" + strings.Join(lines, "\n") + "\n"
	var decoded map[string]any
	if err := yaml.Unmarshal([]byte(wrapped), &decoded); err != nil {
		return map[string]any{}
	}
	return normalizeSection(decoded["root"])
}

func normalizeSection(raw any) map[string]any {
	normalized := packageschema.NormalizeValue(raw)
	if section, ok := normalized.(map[string]any); ok {
		return section
	}
	return map[string]any{}
}

// marshalSection emits the managed section deterministically: declared fields
// first in schema order, then remaining keys sorted alphabetically.
func marshalSection(section map[string]any, fields []packageschema.Field) []string {
	ordered := make([]string, 0, len(section))
	seen := map[string]bool{}
	for i := range fields {
		name := fields[i].Name
		if _, ok := section[name]; ok && !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(section))
	for name := range section {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)
	lines := []string{}
	for _, name := range ordered {
		appendYAMLLines(&lines, 1, name, section[name])
	}
	return lines
}

func appendYAMLLines(lines *[]string, indent int, key string, value any) {
	prefix := strings.Repeat("  ", indent)
	switch typed := value.(type) {
	case map[string]any:
		*lines = append(*lines, prefix+yamlKey(key)+":")
		keys := make([]string, 0, len(typed))
		for name := range typed {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			appendYAMLLines(lines, indent+1, name, typed[name])
		}
	case []any:
		if len(typed) == 0 {
			*lines = append(*lines, prefix+yamlKey(key)+": []")
			return
		}
		*lines = append(*lines, prefix+yamlKey(key)+":")
		for _, item := range typed {
			*lines = append(*lines, prefix+"  - "+yamlScalar(item))
		}
	default:
		*lines = append(*lines, prefix+yamlKey(key)+": "+yamlScalar(value))
	}
}

func yamlScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed)
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return strconv.Quote(fmt.Sprintf("%v", typed))
	}
}

func isConfigEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	case bool:
		return false
	default:
		return false
	}
}

// findYAMLSection returns the start and end line indices of the top-level
// section whose key matches. start is -1 when absent; end is len(lines).
func findYAMLSection(lines []string, key string) (int, int) {
	start, end := -1, len(lines)
	for index, line := range lines {
		if strings.TrimSpace(line) == key {
			start = index
			continue
		}
		if start >= 0 && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			end = index
			break
		}
	}
	return start, end
}

// removeYAMLSection drops the top-level section matching key, preserving every
// other line.
func removeYAMLSection(lines []string, key string) []string {
	return removeYAMLSections(lines, key)
}

// removeYAMLSections drops every top-level section whose key is listed. All
// occurrences are removed so a file that accumulated duplicate sections (for
// example a BOM-prefixed section plus a clean replacement) can be rebuilt
// into one canonical section.
func removeYAMLSections(lines []string, keys ...string) []string {
	keySet := map[string]bool{}
	for _, key := range keys {
		if key != "" {
			keySet[key] = true
		}
	}
	if len(keySet) == 0 {
		return lines
	}
	keep := make([]string, 0, len(lines))
	removing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !removing && keySet[trimmed] {
			removing = true
			continue
		}
		if removing {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && !keySet[trimmed] {
				removing = false
				keep = append(keep, line)
			}
			continue
		}
		keep = append(keep, line)
	}
	return keep
}

// keepLastYAMLSections removes every occurrence of the listed top-level keys
// except the last one. A file that accumulated duplicate sections (for example
// a BOM-prefixed section plus a clean replacement) can then be rebuilt into
// one canonical section without losing the newest values.
func keepLastYAMLSections(lines []string, keys ...string) []string {
	keySet := map[string]bool{}
	for _, key := range keys {
		if key != "" {
			keySet[key] = true
		}
	}
	if len(keySet) == 0 {
		return lines
	}
	type sectionSpan struct{ start, end int }
	keyOf := map[int]string{}
	var spans []sectionSpan
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !keySet[trimmed] {
			continue
		}
		end := len(lines)
		for j := index + 1; j < len(lines); j++ {
			next := lines[j]
			if next != "" && next[0] != ' ' && next[0] != '\t' && !keySet[strings.TrimSpace(next)] && !strings.HasPrefix(strings.TrimSpace(next), "#") {
				end = j
				break
			}
		}
		spans = append(spans, sectionSpan{start: index, end: end})
		keyOf[index] = trimmed
	}
	lastIndex := map[string]int{}
	for index, span := range spans {
		lastIndex[keyOf[span.start]] = index
	}
	remove := make([]bool, len(lines))
	for index, span := range spans {
		if lastIndex[keyOf[span.start]] == index {
			continue
		}
		for line := span.start; line < span.end; line++ {
			remove[line] = true
		}
	}
	out := make([]string, 0, len(lines))
	for index, line := range lines {
		if !remove[index] {
			out = append(out, line)
		}
	}
	return out
}

// stripYAMLBOM removes a leading UTF-8 byte-order mark. Windows editors (for
// example Notepad) write one by default, and a BOM in front of the first
// section key silently turns "qq-bot:" into an unmatchable key.
func stripYAMLBOM(content string) string {
	return strings.TrimPrefix(content, "\uFEFF")
}

// dedupeYAMLSections keeps the last occurrence of each repeated top-level
// mapping key. goccy/go-yaml rejects duplicate keys outright, so a file that
// accumulated both a BOM-prefixed section and a clean replacement would
// otherwise be unreadable until it was rewritten.
func dedupeYAMLSections(content string) string {
	content = stripYAMLBOM(content)
	lines := strings.Split(content, "\n")
	type sectionSpan struct{ start, end int }
	var spans []sectionSpan
	keys := map[int]string{}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" || line[0] == ' ' || line[0] == '\t' || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, ok := topLevelYAMLKey(strings.TrimSpace(line))
		if !ok {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if next != "" && next[0] != ' ' && next[0] != '\t' && !strings.HasPrefix(strings.TrimSpace(next), "#") {
				end = j
				break
			}
		}
		spans = append(spans, sectionSpan{start: i, end: end})
		keys[i] = key
	}
	if len(spans) < 2 {
		return content
	}
	lastIndex := map[string]int{}
	for index, span := range spans {
		lastIndex[keys[span.start]] = index
	}
	remove := make([]bool, len(lines))
	for index, span := range spans {
		if lastIndex[keys[span.start]] == index {
			continue
		}
		for line := span.start; line < span.end; line++ {
			remove[line] = true
		}
	}
	out := make([]string, 0, len(lines))
	for index, line := range lines {
		if remove[index] {
			continue
		}
		out = append(out, line)
	}
	// Collapse the blank lines that separated a removed duplicate section.
	compacted := make([]string, 0, len(out))
	previousBlank := false
	for _, line := range out {
		blank := strings.TrimSpace(line) == ""
		if blank && previousBlank {
			continue
		}
		compacted = append(compacted, line)
		previousBlank = blank
	}
	return strings.Join(compacted, "\n")
}

// topLevelYAMLKey extracts an unquoted or single-quoted mapping key from a
// top-level "key: ..." line. Quoted keys with a colon or scope such as
// "'@alemonjs/qq-bot':" are handled by stripping the surrounding quotes.
func topLevelYAMLKey(line string) (string, bool) {
	colon := strings.IndexByte(line, ':')
	if colon <= 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:colon])
	if len(key) >= 2 && key[0] == '\'' && key[len(key)-1] == '\'' {
		key = key[1 : len(key)-1]
	}
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", false
	}
	return key, true
}

func yamlKey(value string) string {
	if yamlNamePattern.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
