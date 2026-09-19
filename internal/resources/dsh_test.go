package resources

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"alemonx/internal/workspace"
)

func dshArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	if _, ok := files["plugins/approval-bridge/package.json"]; ok {
		files["plugins/approval-bridge/index.js"] = "export {}"
		files["plugins/approval-bridge/session-resume.js"] = "export {}"
		files["plugins/approval-bridge/network.js"] = "export {}"
	}
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, content := range files {
		file, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestDSHBackgroundPreparationRetainsOldPackageUntilSuccess(t *testing.T) {
	files := fstest.MapFS{}
	Init(files, workspace.Layout{Root: t.TempDir()})
	defer Init(nil, workspace.Layout{})
	SetDSHPreparation(fmt.Errorf("后台准备中"))
	if _, err := DSHProgram(); err == nil || !strings.Contains(err.Error(), "后台准备中") {
		t.Fatalf("missing preparation feedback: %v", err)
	}
	files["dsh/runtime.zip"] = &fstest.MapFile{Data: dshArchive(t, map[string]string{dshEntry: "old", "plugins/approval-bridge/package.json": "{}"})}
	SetDSHPreparation(nil)
	old, err := DSHProgram()
	if err != nil {
		t.Fatal(err)
	}
	SetDSHPreparation(fmt.Errorf("更新失败"))
	if current, err := DSHProgram(); err != nil || current != old {
		t.Fatalf("failed update lost usable runtime: %s, %v", current, err)
	}
	files["dsh/runtime.zip"] = &fstest.MapFile{Data: dshArchive(t, map[string]string{dshEntry: "new", "plugins/approval-bridge/package.json": "{}"})}
	SetDSHPreparation(nil)
	if current, err := DSHProgram(); err != nil || current == old {
		t.Fatalf("successful update not loaded: %s, %v", current, err)
	}
}

func TestDSHPackageUpgradeAndRollbackPreserveVersions(t *testing.T) {
	base := t.TempDir()
	one := dshArchive(t, map[string]string{dshEntry: "version-one", "plugins/approval-bridge/package.json": "{}"})
	two := dshArchive(t, map[string]string{dshEntry: "version-two", "plugins/approval-bridge/package.json": "{}"})
	first, err := materializeDSH(one, base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := materializeDSH(two, base)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("upgrade overwrote old package")
	}
	rollback, err := materializeDSH(one, base)
	if err != nil || rollback != first {
		t.Fatalf("rollback = %s, %v", rollback, err)
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "version-one" {
		t.Fatalf("old version changed: %s, %v", data, err)
	}
}

func TestDSHPackageRejectsTraversalAndIncompleteArchive(t *testing.T) {
	for _, files := range []map[string]string{{"../escape": "bad"}, {dshEntry: "missing bridge"}, {"alx-runtime-platform.json": `{"os":"invalid","arch":"invalid"}`}} {
		base := filepath.Join(t.TempDir(), "packages")
		if _, err := materializeDSH(dshArchive(t, files), base); err == nil {
			t.Fatal("invalid archive accepted")
		}
		entries, err := os.ReadDir(base)
		if err != nil || len(entries) != 1 || entries[0].Name() != ".install.lock" {
			t.Fatalf("incomplete installation left behind: %v, %v", entries, err)
		}
	}
}

func TestDSHPackageRepairsMissingFilesAndPreservesDamagedCopy(t *testing.T) {
	for _, missing := range []string{"node_modules", ".complete", "plugins/approval-bridge/index.js"} {
		t.Run(missing, func(t *testing.T) {
			base := t.TempDir()
			data := dshArchive(t, map[string]string{dshEntry: "working", "plugins/approval-bridge/package.json": "{}"})
			entry, err := materializeDSH(data, base)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			target := filepath.Join(base, hex.EncodeToString(sum[:]))
			if err := os.WriteFile(filepath.Join(target, "preserve"), []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(target, missing)); err != nil {
				t.Fatal(err)
			}
			fixed, err := materializeDSH(data, base)
			if err != nil || fixed != entry || !completeDSHPackage(target) {
				t.Fatalf("repair failed: %s %v", fixed, err)
			}
			backups, err := filepath.Glob(filepath.Join(base, ".damaged-*", filepath.Base(target), "preserve"))
			if err != nil || len(backups) != 1 {
				t.Fatalf("backup missing: %v %v", backups, err)
			}
			if content, err := os.ReadFile(backups[0]); err != nil || string(content) != "original" {
				t.Fatalf("original lost: %v", err)
			}
		})
	}
}

func TestDSHInvalidReplacementDoesNotMoveExistingDirectory(t *testing.T) {
	base := t.TempDir()
	data := dshArchive(t, map[string]string{dshEntry: "incomplete"})
	sum := sha256.Sum256(data)
	target := filepath.Join(base, hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "preserve"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeDSH(data, base); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "preserve")); err != nil {
		t.Fatal("original moved before replacement validation", err)
	}
}

func TestDSHConcurrentMaterializationPublishesOneCompletePackage(t *testing.T) {
	base := t.TempDir()
	data := dshArchive(t, map[string]string{dshEntry: "working", "plugins/approval-bridge/package.json": "{}"})
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		go func() { _, err := materializeDSH(data, base); results <- err }()
	}
	for i := 0; i < cap(results); i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected installation artifacts: %v %v", entries, err)
	}
}
