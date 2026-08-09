package system

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsRestartScriptQuotesPathsAndHandlesNoAppArguments(t *testing.T) {
	script := windowsRestartScript(`C:\Program Files\ALemonX\alx.exe`, `C:\Program Files\ALemonX\alx.exe.new.exe`, `C:\Program Files\ALemonX\alx.previous.exe`, nil, "17390")
	for _, want := range []string{
		"$target='C:\\Program Files\\ALemonX\\alx.exe'",
		"$arguments=@()",
		"$attempt -lt 150",
		"Remove-Item -LiteralPath $target -Force",
		"$watcherArgs=@('update-watch','--port','17390')",
		"Start-Process -FilePath $target -ArgumentList $watcherArgs",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("restart script missing %q: %s", want, script)
		}
	}
}

func TestCachedUpdateRequiresExpectedSHA256(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	path, ready, err := CachedUpdate("alx-linux-amd64.zip", fmt.Sprintf("%x", sha256.Sum256([]byte("expected"))))
	if err != nil || ready {
		t.Fatalf("empty cache = (%q, %v, %v), want not ready", path, ready, err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	_, ready, err = CachedUpdate("alx-linux-amd64.zip", fmt.Sprintf("%x", sha256.Sum256([]byte("expected"))))
	if err != nil || ready {
		t.Fatalf("tampered cache must not be ready: ready=%v err=%v", ready, err)
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte("tampered")))
	if _, ready, err = CachedUpdate("alx-linux-amd64.zip", checksum); err != nil || !ready {
		t.Fatalf("matching cache must be ready: ready=%v err=%v", ready, err)
	}
}

func TestReadyPendingUpdateRejectsTamperedArchive(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte("release")))
	path, _, err := CachedUpdate("alx-linux-amd64.zip", checksum)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := savePendingUpdate(PendingUpdate{AssetName: filepath.Base(path), SHA256: checksum, Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, ready, err := ReadyPendingUpdate()
	if err != nil || ready {
		t.Fatalf("tampered pending update must not be ready: ready=%v err=%v", ready, err)
	}
}

func TestUpdateTransactionPersistsAndConfirmsMatchingVersion(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	transaction := UpdateTransaction{
		Phase:           UpdatePhaseRestarting,
		TargetVersion:   "v1.2.3",
		PreviousVersion: "v1.2.2",
		ArchivePath:     "/tmp/alx.zip",
	}
	if err := SaveUpdateTransaction(transaction); err != nil {
		t.Fatal(err)
	}
	if _, healthy, err := MarkUpdateHealthy("1.2.2"); err != nil || healthy {
		t.Fatalf("wrong version must not confirm update: healthy=%v err=%v", healthy, err)
	}
	confirmed, healthy, err := MarkUpdateHealthy("1.2.3")
	if err != nil || !healthy || confirmed.Phase != UpdatePhaseHealthy {
		t.Fatalf("matching version = (%+v, %v, %v), want healthy transaction", confirmed, healthy, err)
	}
	stored, exists, err := ReadUpdateTransaction()
	if err != nil || !exists || stored.Phase != UpdatePhaseHealthy {
		t.Fatalf("stored transaction = (%+v, %v, %v), want healthy", stored, exists, err)
	}
}
