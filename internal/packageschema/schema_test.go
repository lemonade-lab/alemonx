package packageschema

import (
	"strings"
	"testing"
)

func TestParseReadsFullDeclaration(t *testing.T) {
	declaration, err := Parse([]byte(`{
  "name": "@alemonjs/example",
  "description": "示例",
  "alemonjs": {
    "config-source": {
      "readme": "https://raw.githubusercontent.com/example/readme.md",
      "official": "https://example.com/docs",
      "platform": "https://example.com/keys"
    },
    "config": [
      {
        "name": "request_config",
        "type": "object",
        "default": {},
        "description": "请求配置",
        "config": [
          {"name": "timeout", "type": "number", "default": 20000, "description": "超时"}
        ]
      },
      {"name": "master_key", "type": "array<number>", "rules": [{"pattern": "^[0-9]+$", "message": "必须为数字"}], "description": "密钥"}
    ],
    "desktop": {
      "logo": "antd.RobotOutlined",
      "platform": [{"name": "example", "value": "@alemonjs/example"}],
      "command": [{"name": "打开 Example", "command": "open.example"}],
      "sidebars": [{"name": "Example", "command": "open.example"}]
    },
    "web": {"root": "dist", "serverPort": true}
  }
}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if declaration.Name != "@alemonjs/example" || declaration.ConfigSource.Official != "https://example.com/docs" {
		t.Fatalf("declaration = %#v", declaration)
	}
	if declaration.ResolveNamespace() != "example" {
		t.Fatalf("namespace = %q, want example", declaration.ResolveNamespace())
	}
	if len(declaration.Config) != 2 || len(declaration.Config[0].Config) != 1 {
		t.Fatalf("nested config missing: %#v", declaration.Config)
	}
	if !declaration.Web.ServerPort || len(declaration.Desktop.Commands) != 1 || len(declaration.Desktop.Sidebars) != 1 {
		t.Fatalf("desktop/web declarations missing: %#v", declaration)
	}
	if got := declaration.PlatformValueForLogin("example"); got != "" {
		t.Fatalf("default platform should be derivable, got %q", got)
	}
}

func TestParseRejectsInvalidRulePattern(t *testing.T) {
	if _, err := Parse([]byte(`{"name":"x","alemonjs":{"config":[{"name":"p","type":"number","rules":[{"pattern":"(","message":"坏"}]}]}}`)); err == nil {
		t.Fatal("invalid pattern must fail parsing")
	}
}

func TestFieldValidateValue(t *testing.T) {
	declaration, err := Parse([]byte(`{"name":"x","alemonjs":{"config":[
	  {"name":"port","type":"number","rules":[{"pattern":"^[0-9]+$","message":"端口必须为数字"}]},
	  {"name":"tags","type":"array<string>","rules":[{"pattern":"^[a-z]+$","message":"必须小写"}]},
	  {"name":"obj","type":"object","config":[{"name":"inner","type":"string","required":true}]}
	]}}`))
	if err != nil {
		t.Fatal(err)
	}
	port := &declaration.Config[0]
	if messages := port.ValidateValue(float64(80)); len(messages) != 0 {
		t.Fatalf("valid number should pass: %v", messages)
	}
	if messages := port.ValidateValue(float64(-1)); len(messages) == 0 {
		t.Fatal("invalid number must report rule message")
	}
	tags := &declaration.Config[1]
	if messages := tags.ValidateValue([]any{"a", "Bad"}); len(messages) != 1 || messages[0] != "必须小写" {
		t.Fatalf("array item rule messages = %v", messages)
	}
	obj := &declaration.Config[2]
	if messages := obj.ValidateValue(map[string]any{}); len(messages) != 1 || !strings.Contains(messages[0], "必填") {
		t.Fatalf("nested required message = %v", messages)
	}
}

func TestFieldCoerce(t *testing.T) {
	declaration, err := Parse([]byte(`{"name":"x","alemonjs":{"config":[
	  {"name":"port","type":"number"},
	  {"name":"enabled","type":"boolean"},
	  {"name":"list","type":"array<number>"}
	]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if value, err := declaration.Config[0].Coerce("8080"); err != nil || value != float64(8080) {
		t.Fatalf("number coerce = %v, %v", value, err)
	}
	if value, err := declaration.Config[1].Coerce("true"); err != nil || value != true {
		t.Fatalf("bool coerce = %v, %v", value, err)
	}
	if value, err := declaration.Config[2].Coerce([]any{"1", 2}); err != nil || len(value.([]any)) != 2 {
		t.Fatalf("array coerce = %#v, %v", value, err)
	}
}
