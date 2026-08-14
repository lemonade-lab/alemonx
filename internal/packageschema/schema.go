// Package packageschema parses the "alemonjs" declaration that npm packages
// embed in their package.json. It is the single contract shared by the robot
// configuration screen and the online catalog: fields, rules, config sources,
// desktop registrations and WebView declarations all come from here.
package packageschema

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxPatternLength guards the workbench against ReDoS-style declarations from
// untrusted packages. Patterns longer than this are rejected as invalid.
const maxPatternLength = 500

// yamlNamePattern matches a plain YAML mapping key that needs no quoting.
var yamlNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

type Rule struct {
	Pattern string `json:"pattern"`
	Message string `json:"message"`
}

// Field is one configurable value. object fields carry nested Config; array
// fields carry item-level Rules; Default is returned to the editor so empty
// required values can be prefilled and treated as configured.
type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Default     any      `json:"default,omitempty"`
	Rules       []Rule   `json:"rules,omitempty"`
	Config      []*Field `json:"config,omitempty"`
}

type ConfigSource struct {
	Readme   string `json:"readme,omitempty"`
	Official string `json:"official,omitempty"`
	Platform string `json:"platform,omitempty"`
}

// Platform registers a connection package: Name is the login identifier
// (--login) and Value is the platform package (--platform).
type Platform struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Command struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type Sidebar struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type Desktop struct {
	Logo      string     `json:"logo,omitempty"`
	Platforms []Platform `json:"platform,omitempty"`
	Commands  []Command  `json:"command,omitempty"`
	Sidebars  []Sidebar  `json:"sidebars,omitempty"`
}

type Web struct {
	Root       string `json:"root,omitempty"`
	ServerPort bool   `json:"serverPort,omitempty"`
}

type Declaration struct {
	Name         string       `json:"name"`
	Description  string       `json:"description,omitempty"`
	Config       []Field      `json:"config,omitempty"`
	ConfigSource ConfigSource `json:"config-source,omitempty"`
	Desktop      Desktop      `json:"desktop,omitempty"`
	Web          Web          `json:"web,omitempty"`
}

// UnmarshalJSON flattens the "alemonjs" manifest section into the Declaration
// so callers can read config/desktop/web fields directly.
func (d *Declaration) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Alemonjs    struct {
			Config       []Field      `json:"config"`
			ConfigSource ConfigSource `json:"config-source"`
			Desktop      Desktop      `json:"desktop"`
			Web          Web          `json:"web"`
		} `json:"alemonjs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Name = raw.Name
	d.Description = raw.Description
	d.Config = raw.Alemonjs.Config
	d.ConfigSource = raw.Alemonjs.ConfigSource
	d.Desktop = raw.Alemonjs.Desktop
	d.Web = raw.Alemonjs.Web
	return nil
}

// Parse decodes a package.json manifest. A missing name is always an error;
// invalid rule patterns are also reported so broken declarations fail fast.
func Parse(data []byte) (*Declaration, error) {
	var declaration Declaration
	if err := json.Unmarshal(data, &declaration); err != nil {
		return nil, errors.New("package.json 无法识别")
	}
	if declaration.Name == "" {
		return nil, errors.New("package.json 缺少 name")
	}
	if err := declaration.ValidateDeclaration(); err != nil {
		return nil, err
	}
	return &declaration, nil
}

// ValidateDeclaration compiles every rule pattern and rejects declarations
// whose expressions are too long or cannot compile.
func (d *Declaration) ValidateDeclaration() error {
	return validateFields(d.Config)
}

func validateFields(fields []Field) error {
	var problems []error
	for i := range fields {
		field := &fields[i]
		if strings.TrimSpace(field.Name) == "" {
			problems = append(problems, fmt.Errorf("配置项缺少 name"))
		}
		for _, rule := range field.Rules {
			if rule.Pattern == "" {
				continue
			}
			if len(rule.Pattern) > maxPatternLength {
				problems = append(problems, fmt.Errorf("配置项 %s 的校验表达式超过 %d 字符上限", field.Name, maxPatternLength))
				continue
			}
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				problems = append(problems, fmt.Errorf("配置项 %s 的校验表达式无效：%v", field.Name, err))
			}
		}
		for _, child := range field.Config {
			if child != nil {
				problems = append(problems, validateFields([]Field{*child}))
			}
		}
	}
	return errors.Join(problems...)
}

// ResolveNamespace returns the top-level YAML section key for this package's
// configuration. Official @alemonjs/* packages always use their unscoped
// name (for example @alemonjs/db writes db:). A desktop.platform name can
// still provide the same short key for connection packages.
func (d *Declaration) ResolveNamespace() string {
	namespace := d.Name
	baseName := d.Name
	if slash := strings.LastIndex(baseName, "/"); slash >= 0 {
		baseName = baseName[slash+1:]
	}
	if strings.HasPrefix(d.Name, "@alemonjs/") && baseName != "" {
		namespace = baseName
	}
	for _, platform := range d.Desktop.Platforms {
		if !yamlNamePattern.MatchString(platform.Name) {
			continue
		}
		if platform.Value == "" || platform.Value == d.Name || platform.Value == baseName {
			namespace = platform.Name
			break
		}
	}
	return namespace
}

// PlatformNamed returns the desktop.platform entry whose Name matches login.
func (d *Declaration) PlatformNamed(login string) (Platform, bool) {
	for _, platform := range d.Desktop.Platforms {
		if platform.Name == login {
			return platform, true
		}
	}
	return Platform{}, false
}

// PlatformValueForLogin returns the explicit --platform value that must be
// written next to login. A value is only needed when it cannot be derived by
// the runtime (empty or the default @alemonjs/<login> shorthand).
func (d *Declaration) PlatformValueForLogin(login string) string {
	platform, ok := d.PlatformNamed(login)
	if !ok {
		return ""
	}
	value := strings.TrimSpace(platform.Value)
	if value == "" || value == "@alemonjs/"+login {
		return ""
	}
	return value
}

// DefaultConfigured reports whether the field has a usable default that can
// stand in for an empty value in required checks.
func (f *Field) DefaultConfigured() bool {
	return f.Default != nil && !isEmpty(f.Default)
}

// Coerce converts a raw YAML/JSON value into the declared type. nil and empty
// values stay empty so the writer can remove the key instead of writing noise.
func (f *Field) Coerce(raw any) (any, error) {
	if raw == nil || isEmpty(raw) {
		return nil, nil
	}
	switch f.Type {
	case "number", "integer":
		number, err := coerceNumber(raw)
		if err != nil {
			return nil, fmt.Errorf("配置项 %s 必须为数字", f.Name)
		}
		return number, nil
	case "boolean", "bool":
		boolean, err := coerceBool(raw)
		if err != nil {
			return nil, fmt.Errorf("配置项 %s 必须为 true 或 false", f.Name)
		}
		return boolean, nil
	case "object":
		return f.coerceObject(raw)
	case "array<string>", "array<number>":
		return f.coerceArray(raw)
	default:
		// Unknown types are kept as strings so exotic declarations never make
		// a package impossible to configure.
		return coerceString(raw), nil
	}
}

func (f *Field) coerceObject(raw any) (map[string]any, error) {
	object, ok := normalizeMap(raw)
	if !ok {
		return nil, fmt.Errorf("配置项 %s 必须为对象", f.Name)
	}
	result := map[string]any{}
	declared := map[string]bool{}
	for _, child := range f.Config {
		declared[child.Name] = true
		childRaw, present := object[child.Name]
		if !present {
			continue
		}
		coerced, err := child.Coerce(childRaw)
		if err != nil {
			return nil, err
		}
		if coerced != nil {
			result[child.Name] = coerced
		}
	}
	for key, value := range object {
		if !declared[key] {
			result[key] = value
		}
	}
	return result, nil
}

func (f *Field) coerceArray(raw any) ([]any, error) {
	array, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("配置项 %s 必须为数组", f.Name)
	}
	result := make([]any, 0, len(array))
	for _, item := range array {
		if item == nil || isEmpty(item) {
			continue
		}
		if f.Type == "array<number>" {
			number, err := coerceNumber(item)
			if err != nil {
				return nil, fmt.Errorf("配置项 %s 的数组元素必须为数字", f.Name)
			}
			result = append(result, number)
			continue
		}
		result = append(result, coerceString(item))
	}
	return result, nil
}

// ValidateValue returns user-facing rule messages for a coerced value.
func (f *Field) ValidateValue(value any) []string {
	if value == nil {
		return nil
	}
	if f.Type == "object" {
		var messages []string
		object, ok := value.(map[string]any)
		if !ok {
			return messages
		}
		for _, child := range f.Config {
			childValue := object[child.Name]
			if child.Required && (childValue == nil || isEmpty(childValue)) && !child.DefaultConfigured() {
				messages = append(messages, child.label()+" 为必填项")
			}
			messages = append(messages, child.ValidateValue(childValue)...)
		}
		return messages
	}
	if f.Type == "array<string>" || f.Type == "array<number>" {
		array, ok := value.([]any)
		if !ok {
			return nil
		}
		var messages []string
		for _, item := range array {
			messages = append(messages, validateRules(f, fmt.Sprintf("%v", item))...)
		}
		return messages
	}
	return validateRules(f, fmt.Sprintf("%v", value))
}

func (f *Field) label() string {
	if f.Description != "" {
		return f.Description
	}
	return f.Name
}

func validateRules(field *Field, text string) []string {
	var messages []string
	for _, rule := range field.Rules {
		if rule.Pattern == "" {
			continue
		}
		expression, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		if !expression.MatchString(text) {
			message := rule.Message
			if message == "" {
				message = "格式不正确"
			}
			messages = append(messages, message)
		}
	}
	return messages
}

func coerceString(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", raw)
	}
}

func coerceNumber(raw any) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case json.Number:
		return value.Float64()
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, err
		}
		return number, nil
	default:
		return 0, fmt.Errorf("不支持的数字类型 %T", raw)
	}
}

func coerceBool(raw any) (bool, error) {
	switch value := raw.(type) {
	case bool:
		return value, nil
	case string:
		switch strings.TrimSpace(value) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
	}
	return false, fmt.Errorf("无效的布尔值")
}

func normalizeMap(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case map[string]any:
		return value, true
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[fmt.Sprintf("%v", key)] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func isEmpty(raw any) bool {
	switch value := raw.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(value) == ""
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	case bool:
		return false
	default:
		return false
	}
}

// NormalizeValue deep-normalizes values decoded from YAML (ints, nested maps)
// into the JSON-friendly shapes the frontend expects.
func NormalizeValue(raw any) any {
	switch value := raw.(type) {
	case int:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	case uint:
		return float64(value)
	case uint64:
		return float64(value)
	case float32:
		return float64(value)
	case json.Number:
		if number, err := value.Float64(); err == nil {
			return number
		}
		return value.String()
	case []any:
		result := make([]any, 0, len(value))
		for _, item := range value {
			result = append(result, NormalizeValue(item))
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = NormalizeValue(item)
		}
		return result
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[fmt.Sprintf("%v", key)] = NormalizeValue(item)
		}
		return result
	default:
		return raw
	}
}
