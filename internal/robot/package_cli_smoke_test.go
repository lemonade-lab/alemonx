package robot

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alemonx/internal/systemnetwork"
)

// This opt-in check executes real CLIs against throwaway, local-only projects.
// ALEMONX_PACKAGE_CLI_SMOKE points to [{manager,version,entry}], where entry is
// the absolute JavaScript entry of an already-downloaded package manager.
func TestPackageCLISmoke(t *testing.T) {
	config := os.Getenv("ALEMONX_PACKAGE_CLI_SMOKE")
	if config == "" {
		t.Skip("set ALEMONX_PACKAGE_CLI_SMOKE to run real package-manager CLIs")
	}
	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var tools []struct{ Manager, Version, Entry string }
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatal(err)
	}
	network, err := systemnetwork.NewAt("")
	if err != nil {
		t.Fatal(err)
	}
	settings := network.Settings()
	settings.Config.Mode = "direct"
	if _, err := network.Save(settings); err != nil {
		t.Fatal(err)
	}
	systemnetwork.SetDefault(network)
	t.Cleanup(func() { initial, _ := systemnetwork.NewAt(""); systemnetwork.SetDefault(initial) })
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		t.Run(tool.Manager+"@"+tool.Version, func(t *testing.T) {
			root := t.TempDir()
			manifest := map[string]any{"name": "alemonx-cli-fixture", "private": true, "version": "1.0.0", "packageManager": tool.Manager + "@" + tool.Version, "workspaces": []string{"packages/*"}, "scripts": map[string]string{"build": "node check.cjs", "install": "node check.cjs"}}
			data, _ := json.Marshal(manifest)
			for name, content := range map[string][]byte{
				"package.json":        data,
				"check.cjs":           []byte(`if (process.argv.length !== 2) throw new Error('Unexpected script arguments: '+process.argv.slice(2)); console.log('SCRIPT_ARGS_OK')`),
				"pnpm-workspace.yaml": []byte("packages:\n  - 'packages/*'\n"),
			} {
				if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			home := t.TempDir()
			if tool.Manager == "yarn" && strings.HasPrefix(tool.Version, "1.") {
				// Empty CLI proxy values must not fall back to saved Yarn/npm
				// proxies. Exercise real HTTP and HTTPS requests, not just parsing.
				var staleRequests atomic.Int32
				stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					staleRequests.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"name":"alx-proxy-check","dist-tags":{"latest":"9.0.0"},"versions":{"9.0.0":{"version":"9.0.0"}}}`))
				}))
				defer stale.Close()
				for _, name := range []string{".yarnrc", ".npmrc"} {
					content := "proxy=" + stale.URL + "\nhttps-proxy=" + stale.URL + "\n"
					if name == ".yarnrc" {
						content = "proxy \"" + stale.URL + "\"\nhttps-proxy \"" + stale.URL + "\"\n"
					}
					if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				for _, secure := range []bool{false, true} {
					var registryRequests atomic.Int32
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						registryRequests.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"name":"alx-proxy-check","dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"version":"1.0.0"}}}`))
					})
					registry := httptest.NewUnstartedServer(handler)
					if secure {
						registry.StartTLS()
					} else {
						registry.Start()
					}
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					cmd := exec.CommandContext(ctx, node, tool.Entry, "info", "alx-proxy-check", "version", "--non-interactive", "--network-timeout=1000")
					cmd.Dir = root
					cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "YARN_STRICT_SSL=false")
					cleanup, err := systemnetwork.ApplyCommand(cmd)
					if err != nil {
						t.Fatal(err)
					}
					// Use a local registry and disable loopback bypass so a stale
					// proxy cannot be hidden by NO_PROXY in this regression check.
					cmd.Args = append(cmd.Args, "--registry="+registry.URL)
					cmd.Env = append(cmd.Env, "NO_PROXY=", "no_proxy=")
					output, err := cmd.CombinedOutput()
					cleanup()
					cancel()
					registry.Close()
					if err != nil || registryRequests.Load() == 0 || staleRequests.Load() != 0 {
						t.Fatalf("Yarn direct request (TLS=%v): registry=%d stale proxy=%d err=%v\n%s", secure, registryRequests.Load(), staleRequests.Load(), err, output)
					}
				}
				for _, name := range []string{".yarnrc", ".npmrc"} {
					if err := os.Remove(filepath.Join(root, name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			runCLI := func(args ...string) string {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, node, append([]string{tool.Entry}, args...)...)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "NPM_CONFIG_USERCONFIG="+os.DevNull, "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_FUND=false", "COREPACK_ENABLE_PROJECT_SPEC=0", "YARN_ENABLE_IMMUTABLE_INSTALLS=false", "YARN_ENABLE_TELEMETRY=0")
				// Classic does not know Berry's settings (and vice versa).
				if tool.Manager != "yarn" || strings.HasPrefix(tool.Version, "1.") {
					filtered := cmd.Env[:0]
					for _, entry := range cmd.Env {
						if !strings.HasPrefix(entry, "YARN_ENABLE_") {
							filtered = append(filtered, entry)
						}
					}
					cmd.Env = filtered
				}
				cleanup, err := systemnetwork.ApplyCommand(cmd)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s %v: %v\n%s", tool.Manager, args, err, output)
				}
				return string(output)
			}
			runCLI("install")
			if output := runCLI("run", "build"); !strings.Contains(output, "SCRIPT_ARGS_OK") {
				t.Fatalf("build script did not execute: %s", output)
			}
			// A script named install must not be confused with yarn/pnpm install.
			if output := runCLI("run", "install"); !strings.Contains(output, "SCRIPT_ARGS_OK") {
				t.Fatalf("install script did not execute: %s", output)
			}
			if tool.Manager == "npm" {
				output := runCLI("pack", "--dry-run", "--json", "--ignore-scripts")
				if _, err := packedFilename(output); err != nil {
					t.Fatalf("npm pack JSON was polluted by configuration warnings: %v\n%s", err, output)
				}
				if output := runCLI("exec", "--", "node", "check.cjs"); !strings.Contains(output, "SCRIPT_ARGS_OK") {
					t.Fatalf("npm exec changed child arguments: %s", output)
				}
				entry := tool.Entry
				tool.Entry = filepath.Join(filepath.Dir(entry), "npx-cli.js")
				if output := runCLI("--yes", "--", "node", "check.cjs"); !strings.Contains(output, "SCRIPT_ARGS_OK") {
					t.Fatalf("npx changed child arguments: %s", output)
				}
				tool.Entry = entry
			}
			// Yarn Classic interprets @ in an absolute tarball path as a
			// package version delimiter; keep the fixture archive outside the
			// Go subtest directory, whose name contains manager@version.
			archiveDir, err := os.MkdirTemp("", "alx-package-fixture-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(archiveDir) })
			archive := filepath.Join(archiveDir, "local-dependency.tgz")
			writeLocalPackageArchive(t, archive)
			for _, dev := range []bool{false, true} {
				source := "alemonx-local-dependency@file:" + filepath.ToSlash(archive)
				if tool.Manager == "yarn" && strings.HasPrefix(tool.Version, "1.") {
					source = filepath.ToSlash(archive)
				}
				_, args, err := connectionPackageCommand(root, "add", source)
				if err != nil {
					t.Fatal(err)
				}
				if dev {
					if tool.Manager == "yarn" {
						args = append(args, "--dev")
					} else {
						args = append(args, "--save-dev")
					}
				}
				runCLI(args...)
				installed, err := readPackage(root)
				if err != nil {
					t.Fatal(err)
				}
				field := "dependencies"
				if dev {
					field = "devDependencies"
				}
				dependencies, _ := installed[field].(map[string]any)
				if dependencies["alemonx-local-dependency"] == nil {
					t.Fatalf("missing %s after add", field)
				}
				_, args, err = connectionPackageCommand(root, "remove", "alemonx-local-dependency")
				if err != nil {
					t.Fatal(err)
				}
				runCLI(args...)
			}
			runCLI(packageVersionArgs(root, tool.Manager, "1.2.3")...)
			updated, err := readPackage(root)
			if err != nil || updated["version"] != "1.2.3" {
				t.Fatalf("version was not updated: %v", err)
			}
		})
	}
}

func writeLocalPackageArchive(t *testing.T, filename string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	zip := gzip.NewWriter(file)
	archive := tar.NewWriter(zip)
	data := []byte(`{"name":"alemonx-local-dependency","version":"1.0.0"}`)
	if err := archive.WriteHeader(&tar.Header{Name: "package/", Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if err := archive.WriteHeader(&tar.Header{Name: "package/package.json", Mode: 0644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(data); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{archive.Close, zip.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
}
