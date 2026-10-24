package robot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPublishTagsOnlyExactReleaseTarget(t *testing.T) {
	for _, target := range []string{"release", "dev-release", "feature-release"} {
		t.Run(target, func(t *testing.T) {
			root, remote, source := t.TempDir(), t.TempDir(), t.TempDir()
			run := func(dir string, args ...string) string {
				t.Helper()
				output, err := gitRun(dir, args...)
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, output)
				}
				return strings.TrimSpace(output)
			}
			run(remote, "init", "--bare")
			run(root, "init", "-b", "main")
			run(root, "config", "user.name", "Test")
			run(root, "config", "user.email", "test@example.invalid")
			run(root, "config", "commit.gpgsign", "false")
			run(root, "config", "tag.gpgsign", "false")
			for _, dir := range []string{root, source} {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"example","version":"1.2.3","packageManager":"yarn@1.22.22","gitHead":"stale"}`), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "artifact.js"), []byte("built"), 0644); err != nil {
				t.Fatal(err)
			}
			run(root, "add", ".")
			run(root, "commit", "-m", "source")
			head := run(root, "rev-parse", "HEAD")
			run(root, "remote", "add", "origin", remote)
			run(root, "push", "origin", "main")
			// An existing formal tag must not block unrelated branch artifacts.
			if target != "release" {
				run(root, "tag", "v2.0.0")
				run(root, "push", "origin", "v2.0.0")
				run(root, "tag", "-a", "v9.9.9", "-m", "unpublished local tag")
				run(root, "config", "push.followTags", "true")
			}
			beforeLocal := run(root, "tag", "--list")
			_, result, err := publishRelease(root, source, "main", head, "2.0.0", "v2.0.0", []string{"artifact.js"}, target, true)
			if err != nil {
				t.Fatal(err)
			}
			releaseHead := run(remote, "rev-parse", "refs/heads/"+target)
			if run(root, "rev-parse", "HEAD") != head {
				t.Fatal("source HEAD changed")
			}
			if run(remote, "show", target+":artifact.js") != "built" {
				t.Fatal("artifact missing")
			}
			var pkg map[string]any
			if err := json.Unmarshal([]byte(run(remote, "show", target+":package.json")), &pkg); err != nil {
				t.Fatal(err)
			}
			if pkg["gitHead"] != head {
				t.Fatalf("gitHead = %v, want %s", pkg["gitHead"], head)
			}
			if _, ok := pkg["packageManager"]; ok {
				t.Fatal("packageManager leaked into artifact")
			}
			if _, err := gitRun(remote, "show", target+":.alx-release.json"); err == nil {
				t.Fatal("legacy mapping must not be generated")
			}
			message := run(remote, "log", "-1", "--format=%B", target)
			if !strings.Contains(message, "\nSource-Branch: main\nSource-Commit: "+head) {
				t.Fatalf("missing source trailers: %s", message)
			}
			var sourcePkg map[string]any
			if err := json.Unmarshal([]byte(run(root, "show", "HEAD:package.json")), &sourcePkg); err != nil {
				t.Fatal(err)
			}
			if sourcePkg["gitHead"] != "stale" || sourcePkg["packageManager"] != "yarn@1.22.22" {
				t.Fatal("source manifest changed")
			}
			if target == "release" {
				if run(remote, "rev-parse", "refs/tags/v2.0.0^{}") != releaseHead {
					t.Fatal("tag must point to release commit")
				}
				if pkg["version"] != "2.0.0" {
					t.Fatalf("release package version: %v", pkg["version"])
				}
				if _, _, err := publishRelease(root, source, "main", head, "2.0.0", "", []string{"artifact.js"}, target, true); err == nil {
					t.Fatal("duplicate release tag accepted")
				}
			} else {
				if run(root, "tag", "--list") != beforeLocal || run(remote, "rev-parse", "refs/tags/v2.0.0") != head {
					t.Fatal("branch publish modified tags")
				}
				if run(remote, "tag", "--list") != "v2.0.0" {
					t.Fatal("unexpected remote tags")
				}
				if pkg["version"] != "1.2.3" {
					t.Fatalf("source package version changed: %v", pkg["version"])
				}
				if !strings.Contains(result.Output, "未创建或推送标签") {
					t.Fatal(result.Output)
				}
				// Version validation belongs only to formal releases.
				if err := os.WriteFile(filepath.Join(source, "artifact.js"), []byte("rebuilt"), 0644); err != nil {
					t.Fatal(err)
				}
				if _, _, err := publishRelease(root, source, "main", head, "not-a-version", "", []string{"artifact.js"}, target, true); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRetryTagRejectsNonReleaseTarget(t *testing.T) {
	if _, err := retryPreparedGitTag(gitBuildState{GitBuildSession: GitBuildSession{Target: "dev-release"}}); err == nil {
		t.Fatal("non-release target accepted a tag retry")
	}
}

func TestScanPublishFilesExcludesDependenciesAndHiddenFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"dist/index.js", "dist/assets/app.js", "node_modules/pkg/index.js", ".cache/item", ".git/config", "package.json", "README.md"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	items := scanPublishFiles(root, "")
	has := func(value string) bool {
		for _, item := range items {
			if item == value {
				return true
			}
		}
		return false
	}
	for _, value := range []string{"dist", "dist/index.js", "dist/assets", "dist/assets/app.js", "README.md"} {
		if !has(value) {
			t.Fatalf("expected scanned artifact %q, got %#v", value, items)
		}
	}
	for _, value := range []string{"node_modules", ".cache", ".git", "package.json"} {
		if has(value) {
			t.Fatalf("unexpected scanned artifact %q", value)
		}
	}
}

func TestDefaultGitArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name, rules string
		want        []string
	}{
		{"missing", "", []string{"lib", "dist", "README.md"}},
		{"empty", `,"files":[]`, []string{"lib", "dist", "README.md"}},
		{"directory", `,"files":["dist"]`, []string{"dist/index.js", "dist/nested/app.js", "dist/debug.map", "README.md"}},
		{"glob", `,"files":["dist/**/*.js"]`, []string{"dist/index.js", "dist/nested/app.js", "README.md"}},
		{"unmatched", `,"files":["missing"]`, []string{"README.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"lib/index.js", "dist/index.js", "dist/nested/app.js", "dist/debug.map", "README.md", "src/index.js", ".npmignore"} {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("fixture"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			original := []byte(`{"name":"selection-fixture","version":"1.0.0","scripts":{"prepare":"node -e \"process.exit(99)\"","prepack":"node -e \"process.exit(99)\""}` + tc.rules + `}`)
			manifest := filepath.Join(root, "package.json")
			if err := os.WriteFile(manifest, original, 0644); err != nil {
				t.Fatal(err)
			}
			got, err := defaultGitArtifacts(root, scanPublishFiles(root, ""))
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(got)
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			restored, err := os.ReadFile(manifest)
			if err != nil || string(restored) != string(original) {
				t.Fatal("source manifest not restored")
			}
		})
	}
}

func TestReleasePackageScriptsMatchAlemonc(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scripts map[string]any
		custom  string
		want    map[string]any
	}{
		{"runtime", map[string]any{"bundle": "echo bundle", "build": "echo build", "prepublish": "echo prepublish", "prepare": "echo prepare", "prepublishOnly": "echo prepublishOnly", "prepack": "echo prepack", "postpack": "echo postpack", "publish": "echo publish", "postpublish": "echo postpublish", "start": "node lib/index.js", "install": "node install.js", "postinstall": "node setup.js", "test": "node test.js", "custom": "echo custom"}, "custom", map[string]any{"start": "node lib/index.js", "install": "node install.js", "postinstall": "node setup.js", "test": "node test.js"}},
		{"only build", map[string]any{"build": "echo build", "prepare": "echo prepare"}, "", nil},
		{"empty", map[string]any{}, "", nil},
		{"absent", nil, "", nil},
		{"unselected custom", map[string]any{"bundle": "echo bundle", "custom": "echo custom", "start": "node lib/index.js"}, "", map[string]any{"custom": "echo custom", "start": "node lib/index.js"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, destination := t.TempDir(), t.TempDir()
			pkg := map[string]any{"name": "fixture", "version": "1.0.0"}
			if tc.scripts != nil {
				pkg["scripts"] = tc.scripts
			}
			if tc.custom != "" {
				pkg["alemonjs"] = map[string]any{"build": tc.custom}
			}
			original, err := json.Marshal(pkg)
			if err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(source, "package.json")
			if err := os.WriteFile(manifest, original, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "artifact.js"), []byte("built"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := copyReleaseFiles(source, destination, "2.0.0", []string{"artifact.js"}, "source-sha"); err != nil {
				t.Fatal(err)
			}
			published, err := readPackage(destination)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if _, exists := published["scripts"]; exists {
					t.Fatalf("expected omitted scripts, got %v", published["scripts"])
				}
			} else if !reflect.DeepEqual(published["scripts"], tc.want) {
				t.Fatalf("scripts = %v, want %v", published["scripts"], tc.want)
			}
			unchanged, err := os.ReadFile(manifest)
			if err != nil || string(unchanged) != string(original) {
				t.Fatal("source manifest changed")
			}
		})
	}
}
