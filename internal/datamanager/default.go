package datamanager

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var defaultSQLiteMu sync.Mutex

// The fixed local database is separate from application-owned operational data.
// Existing files are validated, never truncated or replaced.
func EnsureDefaultSQLite(ctx context.Context, root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("工作区尚未就绪，无法打开默认数据库")
	}
	defaultSQLiteMu.Lock()
	defer defaultSQLiteMu.Unlock()
	dir := filepath.Join(root, "data")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "default.sqlite")
	if _, err := os.Lstat(path); err == nil {
		return SQLitePath(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".default-*.sqlite")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if err := f.Close(); err != nil {
		return "", err
	}
	uriPath := filepath.ToSlash(f.Name())
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return "", err
	}
	defer db.Close()
	// Materialize the SQLite header without introducing application tables.
	if _, err := db.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	// Publish the completed file without overwriting another process's database.
	if err := os.Link(f.Name(), path); err != nil && !os.IsExist(err) {
		return "", err
	}
	return SQLitePath(path)
}
