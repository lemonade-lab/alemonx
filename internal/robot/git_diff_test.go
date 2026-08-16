package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitDiff(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"robot\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, command := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"add", "package.json"},
		{"commit", "-m", "initial"},
	} {
		if _, err := gitRun(root, command...); err != nil {
			t.Skipf("git is unavailable for diff test: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"robot\",\"version\":\"2.0.0\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	modified, err := GitDiff(root, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if modified.Untracked || modified.Binary || modified.Missing {
		t.Fatalf("modified diff flags = %#v", modified)
	}
	if modified.Status != "M" || !strings.Contains(modified.Diff, "+") || !strings.Contains(modified.Diff, "-") {
		t.Fatalf("modified diff = %#v", modified)
	}

	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}
	untracked, err := GitDiff(root, "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !untracked.Untracked || untracked.Status != "??" {
		t.Fatalf("untracked diff flags = %#v", untracked)
	}
	if !strings.HasPrefix(untracked.Diff, "+hello") || !strings.Contains(untracked.Diff, "+world") {
		t.Fatalf("untracked diff content = %q", untracked.Diff)
	}

	if _, err := GitDiff(root, "../secret"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
	missing, err := GitDiff(root, "missing-file.txt")
	if err != nil || missing.Diff != "" {
		t.Fatalf("unmatched pathspec should produce an empty diff, got %#v err=%v", missing, err)
	}
}
