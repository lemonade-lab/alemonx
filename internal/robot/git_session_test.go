package robot

import (
	"encoding/json"
	"os"
	"path/filepath"
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
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"example","version":"1.2.3"}`), 0644); err != nil {
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
