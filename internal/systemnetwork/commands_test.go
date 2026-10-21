package systemnetwork

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPNPMCommandProxyOptions(t *testing.T) {
	previous := defaultManager
	t.Cleanup(func() { SetDefault(previous) })
	for _, mode := range []string{"direct", "proxy"} {
		for _, command := range [][]string{
			{"pnpm", "add", "@alemonjs/onebot"},
			{"pnpm.exe", "install"},
			{"pnpm.cmd", "update", "--latest", "alemonjs"},
			{"corepack", "pnpm", "remove", "@alemonjs/onebot"},
			{"node", "/tools/pnpm.cjs", "add", "@alemonjs/onebot"},
			{"npx", "--yes", "pnpm@latest", "add", "@alemonjs/onebot"},
			{"node", "/tools/npx-cli.js", "--yes", "pnpm@latest", "install"},
		} {
			t.Run(mode+"/"+strings.Join(command, " "), func(t *testing.T) {
				c := testConfig(mode)
				c.Proxy.URL = "http://127.0.0.1:1"
				SetDefault(&Manager{config: &c})
				cmd := exec.Command(command[0], command[1:]...)
				cmd.Env = []string{"HTTPS_PROXY=http://inherited.invalid:1"}
				original := inspectPackageCommand(cmd)
				if !ManagedCommand(cmd) {
					t.Fatal("pnpm installation must use managed networking")
				}
				cleanup, err := ApplyCommand(cmd)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				proxyURL := ""
				for _, entry := range cmd.Env {
					if strings.HasPrefix(entry, "HTTPS_PROXY=") {
						proxyURL = strings.TrimPrefix(entry, "HTTPS_PROXY=")
					}
				}
				if mode == "proxy" && !strings.HasPrefix(proxyURL, "http://alx:") {
					t.Fatal("missing command-scoped proxy gateway")
				}
				if mode == "direct" && proxyURL != "" {
					t.Fatal("inherited proxy was not cleared")
				}
				for index, arg := range cmd.Args {
					if strings.HasPrefix(arg, "--proxy=") || strings.HasPrefix(arg, "--https-proxy=") {
						if original.bootstrap < 0 || index >= original.start+3 {
							t.Fatal("pnpm rejects raw proxy options")
						}
					}
				}
				for _, want := range []string{"--config.registry=https://registry.npmjs.org/", "--config.proxy=" + proxyURL, "--config.https-proxy=" + proxyURL} {
					found := false
					for _, arg := range cmd.Args {
						found = found || arg == want
					}
					if !found {
						t.Fatalf("missing %s option", strings.SplitN(want, "=", 2)[0])
					}
				}
			})
		}
	}
}

func TestPackageCommandArgumentBoundaries(t *testing.T) {
	previous := defaultManager
	t.Cleanup(func() { SetDefault(previous) })
	c := testConfig("direct")
	SetDefault(&Manager{config: &c})
	cases := []struct {
		args      []string
		managed   bool
		bootstrap int
	}{
		{[]string{"yarn", "run", "install", "--", "add"}, false, -1},
		{[]string{"npm", "run", "build", "--", "install"}, false, -1},
		{[]string{"pnpm", "exec", "tool", "install"}, false, -1},
		{[]string{"node", "/tools/not-npm.js", "install"}, false, -1},
		{[]string{"npx", "--yes", "lvy", "build", "--port", "1234"}, true, 1},
		{[]string{"npx", "--yes", "pm2", "start", "app.js"}, true, 1},
		{[]string{"npx", "--yes", "pnpm@9.15.0", "run", "build", "install"}, true, 1},
		{[]string{"node", "/tools/npx-cli.js", "--yes", "yarn@1.22.22", "run", "install"}, true, 2},
		{[]string{"npm", "exec", "--", "lvy", "build"}, true, 1},
		{[]string{"corepack", "yarn", "run", "install"}, false, -1},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := exec.Command(tc.args[0], tc.args[1:]...)
			if got := ManagedCommand(cmd); got != tc.managed {
				t.Fatalf("managed = %v, want %v", got, tc.managed)
			}
			cleanup, err := ApplyCommand(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			want := append([]string(nil), tc.args...)
			if tc.bootstrap >= 0 {
				want = append([]string(nil), tc.args[:tc.bootstrap]...)
				want = append(want, "--registry=https://registry.npmjs.org/", "--proxy=false", "--https-proxy=false")
				want = append(want, tc.args[tc.bootstrap:]...)
			}
			if !reflect.DeepEqual(cmd.Args, want) {
				t.Fatalf("args = %#v, want %#v", cmd.Args, want)
			}
		})
	}
}

func TestModernYarnUsesEnvironmentInsteadOfClassicFlags(t *testing.T) {
	previous := defaultManager
	t.Cleanup(func() { SetDefault(previous) })
	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			c := testConfig(mode)
			c.Proxy.URL = "http://127.0.0.1:1"
			SetDefault(&Manager{config: &c})
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"yarn@4.9.2"}`), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("corepack", "yarn", "add", "alemonjs")
			cmd.Dir = root
			cmd.Env = []string{"YARN_REGISTRY=https://wrong.invalid/"}
			cleanup, err := ApplyCommand(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if !reflect.DeepEqual(cmd.Args, []string{"corepack", "yarn", "add", "alemonjs"}) {
				t.Fatal("Classic flags leaked to modern Yarn")
			}
			env := strings.Join(cmd.Env, "\n")
			if strings.Contains(env, "YARN_REGISTRY=") || !strings.Contains(env, "YARN_NPM_REGISTRY_SERVER=https://registry.npmjs.org/") || !strings.Contains(env, "YARN_HTTP_PROXY=") || !strings.Contains(env, "YARN_HTTPS_PROXY=") {
				t.Fatal("incorrect modern Yarn network environment")
			}
		})
	}
}
