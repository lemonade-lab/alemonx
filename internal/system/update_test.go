package system

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

func TestStageUploadedUpdateRecordsPendingAndReady(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	source := filepath.Join(t.TempDir(), "uploaded.zip")
	writeTestUpdatePackage(t, source, "v9.9.9")
	staged, err := StageUploadedUpdate(source, "alx-"+runtime.GOOS+"-"+runtime.GOARCH+".zip", "")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Version != "v9.9.9" {
		t.Fatalf("staged version = %q", staged.Version)
	}
	update, archive, ready, err := ReadyPendingUpdate()
	if err != nil || !ready {
		t.Fatalf("staged update must be ready: ready=%v err=%v", ready, err)
	}
	if update.AssetName != "alx-"+runtime.GOOS+"-"+runtime.GOARCH+".zip" || update.Version != "v9.9.9" || len(update.SHA256) != 64 {
		t.Fatalf("pending update = %#v", update)
	}
	if archive != staged.Path {
		t.Fatalf("ready archive = %q, want %q", archive, staged.Path)
	}
}

func TestStageUploadedUpdateRejectsPathTraversalFilename(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	source := filepath.Join(t.TempDir(), "pkg.zip")
	writeTestUpdatePackage(t, source, "v1.0.0")
	staged, err := StageUploadedUpdate(source, "../../evil.zip", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(staged.Path) != "evil.zip" || strings.Contains(staged.Path, "..") {
		t.Fatalf("staged path must be confined to the cache dir, got %q", staged.Path)
	}
}

func writeTestUpdatePackage(t *testing.T, path, version string) {
	t.Helper()
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(output)
	manifest, err := archive.Create("alx-update.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(manifest, `{"version":%q,"platform":%q,"architecture":%q}`, version, runtime.GOOS, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	binary, err := archive.Create(map[bool]string{true: "alx.exe", false: "alx"}[runtime.GOOS == "windows"])
	if err != nil {
		t.Fatal(err)
	}
	binarySource := os.Args[0]
	if runtime.GOOS != "windows" {
		binarySource = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	executable, err := os.Open(binarySource)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(binary, executable)
	_ = executable.Close()
	closeErr := archive.Close()
	fileErr := output.Close()
	if copyErr != nil || closeErr != nil || fileErr != nil {
		t.Fatalf("write test update package: copy=%v zip=%v file=%v", copyErr, closeErr, fileErr)
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
