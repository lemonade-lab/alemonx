package datamanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSQLitePersistsAndDoesNotReplaceFiles(t *testing.T) {
	root := t.TempDir()
	path, err := EnsureDefaultSQLite(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(filepath.Join(root, "data", "default.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if path != expected {
		t.Fatalf("unexpected path %s", path)
	}
	if _, err := QuerySQLite(context.Background(), SQLRequest{Path: path, SQL: "CREATE TABLE mine(id INTEGER)", Write: true, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	again, err := EnsureDefaultSQLite(context.Background(), root)
	if err != nil || again != path {
		t.Fatal(again, err)
	}
	if _, err := QuerySQLite(context.Background(), SQLRequest{Path: path, SQL: "SELECT * FROM mine"}); err != nil {
		t.Fatal("lost user table", err)
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(other, "data", "default.sqlite")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDefaultSQLite(context.Background(), other); err == nil {
		t.Fatal("accepted invalid database")
	}
	content, _ := os.ReadFile(file)
	if string(content) != "keep" {
		t.Fatal("overwrote existing file")
	}
}

func TestDefaultSQLiteFailedCreationIsRetryable(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := EnsureDefaultSQLite(ctx, root); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := os.Stat(filepath.Join(root, "data", "default.sqlite")); !os.IsNotExist(err) {
		t.Fatal("published incomplete database")
	}
	if _, err := EnsureDefaultSQLite(context.Background(), root); err != nil {
		t.Fatal(err)
	}
}
