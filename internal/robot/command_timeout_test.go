package robot

import (
	"testing"
	"time"
)

func TestCommandTimeouts(t *testing.T) {
	if got := commandTimeout("git", "ls-remote", "origin"); got != 30*time.Second {
		t.Fatalf("git status probe timeout = %s, want 30s", got)
	}
	if got := commandTimeout("git", "fetch", "origin"); got != 10*time.Minute {
		t.Fatalf("git fetch timeout = %s, want 10m", got)
	}
	if got := commandTimeout("yarn", "install"); got != 20*time.Minute {
		t.Fatalf("yarn install timeout = %s, want 20m", got)
	}
	if got := commandTimeout("node", "--version"); got != 2*time.Minute {
		t.Fatalf("node timeout = %s, want 2m", got)
	}
}
