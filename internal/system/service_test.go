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

func TestSplitShellArgsHonorsSingleQuotes(t *testing.T) {
	arguments := splitShellArgs(`'/tmp/my alx/alx' serve --port 17390 --host 0.0.0.0 --workspace '/data/my workspace'`)
	want := []string{"/tmp/my alx/alx", "serve", "--port", "17390", "--host", "0.0.0.0", "--workspace", "/data/my workspace"}
	if len(arguments) != len(want) {
		t.Fatalf("arguments = %#v, want %#v", arguments, want)
	}
	for index := range want {
		if arguments[index] != want[index] {
			t.Fatalf("arguments = %#v, want %#v", arguments, want)
		}
	}
}

func TestParseExecStartArgs(t *testing.T) {
	unit := "[Unit]\nDescription=ALemonX\n[Service]\nExecStart='/opt/alx' serve --port 17390 --host 127.0.0.1 --workspace '/Users/me/workspace'\nRestart=on-failure\n"
	arguments, ok := parseExecStartArgs(unit)
	if !ok {
		t.Fatal("ExecStart should parse")
	}
	registration := registrationFromArguments(arguments)
	if registration.Executable != "/opt/alx" {
		t.Fatalf("executable = %q", registration.Executable)
	}
	if registration.Port != "17390" || registration.Host != "127.0.0.1" || registration.Workspace != "/Users/me/workspace" {
		t.Fatalf("registration = %+v", registration)
	}
}

func TestParsePlistProgramArguments(t *testing.T) {
	plist := `<plist version="1.0"><dict><key>Label</key><string>com.alemonjs.alx</string><key>ProgramArguments</key><array><string>/tmp/my alx</string><string>serve</string><string>--port</string><string>17390</string><string>--host</string><string>0.0.0.0</string><string>--workspace</string><string>/Users/me/my workspace</string></array><key>RunAtLoad</key><true/></dict></plist>`
	arguments, ok := parsePlistProgramArguments(plist)
	if !ok {
		t.Fatal("plist arguments should parse")
	}
	registration := registrationFromArguments(arguments)
	if registration.Executable != "/tmp/my alx" {
		t.Fatalf("executable = %q", registration.Executable)
	}
	if registration.Workspace != "/Users/me/my workspace" {
		t.Fatalf("workspace = %q", registration.Workspace)
	}
}

func TestParsePlistProgramArgumentsUnescapesXML(t *testing.T) {
	plist := `<plist><dict><key>ProgramArguments</key><array><string>/tmp/a&amp;b/alx</string><string>serve</string></array></dict></plist>`
	arguments, ok := parsePlistProgramArguments(plist)
	if !ok || arguments[0] != "/tmp/a&b/alx" {
		t.Fatalf("arguments = %#v, ok = %t", arguments, ok)
	}
}

func TestWindowsTaskCommandParsing(t *testing.T) {
	command := `cmd.exe /d /s /c ""C:\Program Files\alx\alx.exe" serve --port 17390 --host 0.0.0.0 --workspace "C:\my workspace" >> "C:\logs\alx.log" 2>&1"`
	executable := windowsTaskExecutable(command)
	if executable != `C:\Program Files\alx\alx.exe` {
		t.Fatalf("executable = %q", executable)
	}
	if commandFlagValue(command, "--port") != "17390" {
		t.Fatalf("port = %q", commandFlagValue(command, "--port"))
	}
	if commandFlagValue(command, "--workspace") != `C:\my workspace` {
		t.Fatalf("workspace = %q", commandFlagValue(command, "--workspace"))
	}
}

func TestFlagValues(t *testing.T) {
	values := flagValues([]string{"serve", "--port", "17390", "--host=127.0.0.1", "--workspace", "/ws"}, "--port", "--host", "--workspace")
	if values["--port"] != "17390" || values["--host"] != "127.0.0.1" || values["--workspace"] != "/ws" {
		t.Fatalf("values = %#v", values)
	}
}
