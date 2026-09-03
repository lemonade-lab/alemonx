package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentPackageConfigWithoutDeclarationIsEmpty(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)

	config, err := (Manager{}).CurrentPackageConfig(root)
	if err != nil {
		t.Fatalf("CurrentPackageConfig: %v", err)
	}
	if config.Package != "robot" || config.Namespace != "robot" || len(config.Fields) != 0 || len(config.Values) != 0 {
		t.Fatalf("config = %#v, want an empty robot schema", config)
	}
	if config.Fields == nil {
		t.Fatal("fields must serialize as [] rather than null")
	}
}

// TestScopedConnectionPackageUsesShortNamespace covers @alemonjs/* packages,
// whose desktop.platform entries declare only a short name (onebot) and no
// value. The config must be keyed by that short name, not the scoped package
// name, matching how the framework reads the connection section.
func TestScopedConnectionPackageUsesShortNamespace(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "onebot", "package.json"), `{
  "name":"@alemonjs/onebot",
  "alemonjs":{
    "config":[{"name":"token","type":"string","required":true,"description":"token"}],
    "desktop":{"platform":[{"name":"onebot"}]}
  }
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "onebot:\n  token: \"abc\"\n")

	config, err := (Manager{}).PackageConfig(root, "@alemonjs/onebot")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	if config.Namespace != "onebot" {
		t.Fatalf("namespace = %q, want onebot", config.Namespace)
	}
	if config.Values["token"] != "abc" {
		t.Fatalf("values = %#v, want token=abc", config.Values)
	}
}

func TestPackageConfigRejectsMalformedRuntimeYAML(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "example", "package.json"), `{
  "name":"example",
  "alemonjs":{"config":[{"name":"token","type":"string","description":"token"}]}
}`)
	malformed := "example:\n  token: value\n  broken: [\n"
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), malformed)

	if _, err := (Manager{}).PackageConfig(root, "example"); err == nil {
		t.Fatal("malformed runtime YAML must be reported instead of treated as empty config")
	}
	if _, err := (Manager{}).SavePackageConfig(root, "example", map[string]any{"token": "new"}); err == nil {
		t.Fatal("malformed runtime YAML must block package config writes")
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != malformed {
		t.Fatalf("malformed config was modified:\n%s", data)
	}
}

// TestScopedConnectionPackageReadsLegacyKey keeps values written by older
// versions that keyed the section by the scoped package name.
func TestScopedConnectionPackageReadsLegacyKey(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "onebot", "package.json"), `{
  "name":"@alemonjs/onebot",
	"alemonjs":{
		"config":[{"name":"token","type":"string","required":true,"description":"token"}],
		"desktop":{"platform":[{"name":"onebot"}]}
	}
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), `'@alemonjs/onebot':
  token: "legacy"
`)

	config, err := (Manager{}).PackageConfig(root, "@alemonjs/onebot")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	if config.Values["token"] != "legacy" {
		t.Fatalf("values = %#v, want legacy token preserved", config.Values)
	}
}

// TestSaveScopedConnectionMigratesLegacyKey writes to the short key and removes
// the stale scoped-package block in one pass.
func TestSaveScopedConnectionMigratesLegacyKey(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "onebot", "package.json"), `{
  "name":"@alemonjs/onebot",
  "alemonjs":{
    "config":[
      {"name":"token","type":"string","required":true,"description":"token"},
      {"name":"url","type":"string","required":true,"description":"连接地址"}
    ],
    "desktop":{"platform":[{"name":"onebot"}]}
  }
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), `'@alemonjs/onebot':
  token: "old"
  url: "ws://old"
`)

	if _, err := (Manager{}).SavePackageConfig(root, "@alemonjs/onebot", map[string]any{
		"token": "new",
		"url":   "ws://new",
	}); err != nil {
		t.Fatalf("SavePackageConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "onebot:") {
		t.Fatalf("saved config misses short key:\n%s", output)
	}
	if strings.Contains(output, "@alemonjs/onebot") {
		t.Fatalf("saved config kept legacy scoped key:\n%s", output)
	}
	if !strings.Contains(output, `token: "new"`) || !strings.Contains(output, `url: "ws://new"`) {
		t.Fatalf("saved config misses new values:\n%s", output)
	}
}

func TestOfficialModuleConfigurationUsesUnscopedNamespace(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "db", "package.json"), `{
  "name":"@alemonjs/db",
  "alemonjs":{"config":[{"name":"db","type":"object","config":[{"name":"redis","type":"object","config":[{"name":"host","type":"string"}]}]}]}
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "db:\n  redis:\n    host: localhost\n")

	config, err := (Manager{}).PackageConfig(root, "@alemonjs/db")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	if config.Namespace != "db" {
		t.Fatalf("namespace = %q, want db", config.Namespace)
	}
	if _, err = (Manager{}).SavePackageConfig(root, "@alemonjs/db", map[string]any{
		"db": map[string]any{"redis": map[string]any{"host": "127.0.0.1"}},
	}); err != nil {
		t.Fatalf("SavePackageConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "@alemonjs/db") || !strings.Contains(string(data), "db:") {
		t.Fatalf("module configuration must use db key:\n%s", data)
	}
}

// TestConfigValuesSurviveWindowsBOM covers a UTF-8 byte-order mark written by
// Windows editors. The BOM must not turn the first section key into an
// unmatchable value, otherwise required fields always look empty.
func TestConfigValuesSurviveWindowsBOM(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "qq-bot", "package.json"), `{
  "name":"@alemonjs/qq-bot",
  "alemonjs":{
    "config":[
      {"name":"appid","type":"string","required":true,"description":"AppID"},
      {"name":"token","type":"string","required":true,"description":"token"}
    ],
    "desktop":{"platform":[{"name":"qq-bot","value":"@alemonjs/qq-bot"}]}
  }
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "\uFEFFqq-bot:\n  appid: \"123\"\n  token: \"abc\"\n")

	config, err := (Manager{}).PackageConfig(root, "@alemonjs/qq-bot")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	if config.Namespace != "qq-bot" {
		t.Fatalf("namespace = %q, want qq-bot", config.Namespace)
	}
	if config.Values["appid"] != "123" || config.Values["token"] != "abc" {
		t.Fatalf("values = %#v, want BOM-prefixed section to be readable", config.Values)
	}

	preflight, err := (Manager{}).RuntimePreflight(root)
	if err != nil {
		t.Fatalf("RuntimePreflight: %v", err)
	}
	if len(preflight.Missing) != 0 {
		t.Fatalf("preflight missing = %v, want none with a BOM-prefixed config", preflight.Missing)
	}
}

// TestSaveRemovesDuplicateSections repairs files that accumulated a
// BOM-prefixed section plus a clean replacement. Duplicate top-level keys make
// the whole file unparseable, so every save must collapse them into one
// canonical section carrying the newest values.
func TestSaveRemovesDuplicateSections(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "qq-bot", "package.json"), `{
  "name":"@alemonjs/qq-bot",
  "alemonjs":{
    "config":[
      {"name":"appid","type":"string","required":true,"description":"AppID"},
      {"name":"token","type":"string","required":true,"description":"token"}
    ],
    "desktop":{"platform":[{"name":"qq-bot","value":"@alemonjs/qq-bot"}]}
  }
}`)
	// The old BOM-prefixed section plus the clean replacement an older save
	// appended afterwards. After stripping the BOM, both keys are "qq-bot:".
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "\uFEFFqq-bot:\n  appid: \"old\"\n  token: \"old-token\"\n\nqq-bot:\n  appid: \"new\"\n  token: \"new-token\"\n")

	if _, err := (Manager{}).SavePackageConfig(root, "@alemonjs/qq-bot", map[string]any{
		"appid": "saved",
		"token": "saved-token",
	}); err != nil {
		t.Fatalf("SavePackageConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if count := strings.Count(output, "qq-bot:"); count != 1 {
		t.Fatalf("saved config has %d qq-bot sections, want exactly one:\n%s", count, output)
	}
	if strings.Contains(output, "old-token") {
		t.Fatalf("saved config kept the stale duplicate section:\n%s", output)
	}
	config, err := (Manager{}).PackageConfig(root, "@alemonjs/qq-bot")
	if err != nil {
		t.Fatalf("PackageConfig after save: %v", err)
	}
	if config.Values["appid"] != "saved" || config.Values["token"] != "saved-token" {
		t.Fatalf("values after save = %#v, want the freshly saved values", config.Values)
	}
}

func TestPackageConfigReadsNestedObjectAndArrayValues(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "example", "package.json"), `{
  "name":"example",
  "alemonjs":{
    "config":[
      {"name":"request_config","type":"object","description":"请求配置","config":[
        {"name":"timeout","type":"number","description":"超时"},
        {"name":"proxy","type":"object","config":[]}
      ]},
      {"name":"master_key","type":"array<string>","description":"密钥列表"}
    ]
  }
}`)
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "example:\n  request_config:\n    timeout: 20000\n  master_key:\n    - \"a\"\n    - \"b\"\n")

	config, err := (Manager{}).PackageConfig(root, "example")
	if err != nil {
		t.Fatalf("PackageConfig: %v", err)
	}
	requestConfig, ok := config.Values["request_config"].(map[string]any)
	if !ok || requestConfig["timeout"] != float64(20000) {
		t.Fatalf("request_config = %#v", config.Values["request_config"])
	}
	masterKey, ok := config.Values["master_key"].([]any)
	if !ok || len(masterKey) != 2 || masterKey[0] != "a" {
		t.Fatalf("master_key = %#v", config.Values["master_key"])
	}

	if _, err := (Manager{}).SavePackageConfig(root, "example", map[string]any{
		"request_config": map[string]any{"timeout": 30000, "proxy": map[string]any{}},
		"master_key":     []any{"x", "y"},
	}); err != nil {
		t.Fatalf("SavePackageConfig: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "timeout: 30000") || !strings.Contains(output, "- \"x\"") || !strings.Contains(output, "- \"y\"") {
		t.Fatalf("saved config misses nested/array values:\n%s", output)
	}
}

func TestSavePackageConfigEnforcesRules(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "example", "package.json"), `{
  "name":"example",
  "alemonjs":{
    "config":[
      {"name":"port","type":"number","required":true,"rules":[{"pattern":"^[0-9]+$","message":"端口必须为数字"}],"description":"服务端口"}
    ]
  }
}`)
	if _, err := (Manager{}).SavePackageConfig(root, "example", map[string]any{"port": "abc"}); err == nil {
		t.Fatal("rule violation must block saving")
	}
	if _, err := (Manager{}).SavePackageConfig(root, "example", map[string]any{"port": 8080}); err != nil {
		t.Fatalf("valid value must save: %v", err)
	}
}

func TestPackageConfigRejectsInvalidRulePattern(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "example", "package.json"), `{
  "name":"example",
  "alemonjs":{
    "config":[{"name":"port","type":"number","rules":[{"pattern":"(","message":"坏表达式"}],"description":"服务端口"}]
  }
}`)
	if _, err := (Manager{}).PackageConfig(root, "example"); err == nil {
		t.Fatal("invalid rule pattern must surface as a declaration error")
	}
}

func TestSaveLoginThirdPartyWritesPlatform(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@myorg", "example", "package.json"), `{
  "name":"@myorg/example",
  "description":"Example",
  "alemonjs":{
    "config":[{"name":"app_id","type":"string","required":true,"description":"app_id"}],
    "desktop":{"platform":[{"name":"example","value":"@myorg/example"}]}
  }
}`)
	if _, err := (Manager{}).SavePackageConfig(root, "@myorg/example", map[string]any{"app_id": "abc"}); err != nil {
		t.Fatalf("SavePackageConfig: %v", err)
	}
	if _, err := (Manager{}).SaveLogin(root, "example", "@myorg/example"); err != nil {
		t.Fatalf("SaveLogin: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "login: \"example\"") || !strings.Contains(output, "platform: \"@myorg/example\"") {
		t.Fatalf("login/platform missing:\n%s", output)
	}
}

func TestSaveLoginDefaultCountsAsConfigured(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "example", "package.json"), `{
  "name":"example",
  "alemonjs":{
    "config":[{"name":"url","type":"string","required":true,"default":"ws://127.0.0.1:3001","description":"连接地址"}],
    "desktop":{"platform":[{"name":"example"}]}
  }
}`)
	if _, err := (Manager{}).SaveLogin(root, "example", "example"); err != nil {
		t.Fatalf("SaveLogin must accept required field with a default: %v", err)
	}
}

func TestSaveLoginWithoutPlatformValueSkipsPlatformKey(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@alemonjs", "onebot", "package.json"), `{
  "name":"@alemonjs/onebot",
  "alemonjs":{
    "desktop":{"platform":[{"name":"onebot"}]}
  }
}`)
	if _, err := (Manager{}).SaveLogin(root, "onebot", "@alemonjs/onebot"); err != nil {
		t.Fatalf("SaveLogin: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "platform:") {
		t.Fatalf("derivable login must not write platform:\n%s", data)
	}
}

func TestResolveRuntimePlatformsMergesDeclaredOverBuiltin(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot","dependencies":{"@myorg/custom":"1.0.0"}}`)
	writeAppPageFixture(t, filepath.Join(root, "node_modules", "@myorg", "custom", "package.json"), `{
  "name":"@myorg/custom",
  "description":"自定义 OneBot",
  "version":"1.0.0",
  "alemonjs":{"desktop":{"platform":[{"name":"onebot","value":"@myorg/custom"}]}}
}`)
	writeAppPageFixture(t, filepath.Join(root, "packages", "third", "package.json"), `{
  "name":"third",
  "description":"第三方平台",
  "version":"2.0.0",
  "alemonjs":{"desktop":{"platform":[{"name":"example","value":"@org/third"}]}}
}`)

	platforms, err := resolveRuntimePlatforms(root)
	if err != nil {
		t.Fatalf("resolveRuntimePlatforms: %v", err)
	}
	byID := map[string]RuntimePackage{}
	for _, item := range platforms {
		byID[item.ID] = item
	}
	onebot := byID["onebot"]
	if onebot.Package != "@myorg/custom" || onebot.Source != "declared" || onebot.Installed != true {
		t.Fatalf("builtin onebot must be overridden by declared platform: %#v", onebot)
	}
	example := byID["example"]
	if example.Package != "@org/third" || example.Source != "declared" || example.Label != "第三方平台" {
		t.Fatalf("backpack platform missing: %#v", example)
	}
	qqBot := byID["qq-bot"]
	if qqBot.Package != "@alemonjs/qq-bot" || qqBot.Source != "builtin" {
		t.Fatalf("builtin candidates must remain: %#v", qqBot)
	}
}

func TestPackageConfigsOnlyEnabledBackpackPackages(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	for _, name := range []string{"alpha", "beta"} {
		writeAppPageFixture(t, filepath.Join(root, "packages", name, "package.json"), `{
  "name":"`+name+`",
  "alemonjs":{
    "config":[{"name":"token","type":"string","description":"token"}]
  }
}`)
	}
	writeAppPageFixture(t, filepath.Join(root, "alemon.config.yaml"), "apps:\n  - alpha\n")

	items, err := (Manager{}).PackageConfigs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Package != "alpha" {
		t.Fatalf("items = %#v, want only enabled backpack package alpha", items)
	}
}

func TestDefaultEnableLocalPackageSkipsUnmanagedPackage(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	target := filepath.Join(root, "packages", "demo")
	writeAppPageFixture(t, filepath.Join(target, "package.json"), `{"name":"demo-pkg"}`)

	note := defaultEnableLocalPackage(root, target)
	if note != "" {
		t.Fatalf("note = %q, want no auto-enable for unmanaged package", note)
	}
	enabled, err := (Manager{}).EnabledApps(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Fatalf("enabled = %#v, want none", enabled)
	}
}

func TestInstallLocalPackageDoesNotEnableUnmanagedNPMPackage(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"robot"}`)
	source := filepath.Join(t.TempDir(), "market-plugin")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAppPageFixture(t, filepath.Join(source, "package.json"), `{
  "name":"market-plugin",
  "version":"1.0.0",
  "alemonjs":{"config":[{"name":"key","type":"string","description":"key"}]}
}`)
	writeAppPageFixture(t, filepath.Join(source, "index.js"), "module.exports = {};\n")

	result, err := installLocalPackage(root, source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Output, "已默认启用") {
		t.Fatalf("install output = %q, unmanaged package must not auto-enable", result.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "market-plugin", "package.json")); err != nil {
		t.Fatalf("package did not enter backpack: %v", err)
	}
	enabled, err := (Manager{}).EnabledApps(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Fatalf("enabled = %#v, want none", enabled)
	}
}
