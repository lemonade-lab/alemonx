package pm2config

import (
	"strings"
	"testing"
)

func TestNameIsStableAndSeparatesSameNamedProjects(t *testing.T) {
	first := Name("/robots/one/bot")
	if first != Name("/robots/one/bot") {
		t.Fatal("PM2 name should be stable for one project")
	}
	if first == Name("/robots/two/bot") {
		t.Fatal("same directory name at different paths must not share a PM2 name")
	}
	if !strings.HasPrefix(first, "alemonx-bot-") {
		t.Fatalf("unexpected PM2 name: %q", first)
	}
}

func TestConfigPinsNameNamespaceAndSelfLocatedCWD(t *testing.T) {
	config := Config("/robots/example")
	for _, want := range []string{"name: \"alemonx-example-", "namespace: \"alemonx\"", "cwd: __dirname", "script: './index.js'"} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "/robots/example") {
		t.Fatalf("config must not embed the absolute project path:\n%s", config)
	}
}
