package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasPortArgument(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      bool
	}{
		{name: "separate value", arguments: []string{"serve", "--port", "17390"}, want: true},
		{name: "equals value", arguments: []string{"--port=17390"}, want: true},
		{name: "missing value", arguments: []string{"serve", "--port"}, want: false},
		{name: "not configured", arguments: []string{"serve", "--host", "127.0.0.1"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasPortArgument(test.arguments); got != test.want {
				t.Fatalf("hasPortArgument(%q) = %t, want %t", test.arguments, got, test.want)
			}
		})
	}
}

func TestEnsureSystemdUserUnitKillModeAddsRestartBackoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alx.service")
	input := "[Unit]\nDescription=ALemonX\n[Service]\nExecStart=/tmp/alx serve\nRestart=on-failure\n[Install]\nWantedBy=default.target\n"
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSystemdUserUnitKillMode(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"StartLimitIntervalSec=120", "StartLimitBurst=5", "RestartSec=3", "KillMode=process"} {
		if !strings.Contains(text, required) {
			t.Fatalf("updated service does not contain %q:\n%s", required, text)
		}
	}
}
