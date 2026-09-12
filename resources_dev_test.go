//go:build alxdev

package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestDevelopmentArchiveAppearsWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	root := developmentResourceFS{
		FS:   fstest.MapFS{"resources/templates/example": {Data: []byte("template")}},
		disk: os.DirFS(dir),
	}
	resources, err := fs.Sub(root, "resources")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(resources, "dsh/runtime.zip"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cold start must allow missing archive: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "resources", "dsh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "resources", "dsh", "runtime.zip"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := fs.ReadFile(resources, "dsh/runtime.zip"); err != nil || string(data) != "ready" {
		t.Fatalf("late archive unavailable: %q, %v", data, err)
	}
	if data, err := fs.ReadFile(resources, "templates/example"); err != nil || string(data) != "template" {
		t.Fatalf("embedded templates changed: %q, %v", data, err)
	}
}

func TestDevelopmentPreparationOrdersAndStopsOnFailure(t *testing.T) {
	for _, failAt := range []int{0, 1, 2, 3} {
		var calls []string
		err := runDevelopmentDSH(context.Background(), func(_ context.Context, dir, name string, args ...string) error {
			calls = append(calls, name+" "+args[0])
			if failAt == len(calls) {
				return errors.New("test failure")
			}
			return nil
		})
		want := []string{"npm ci", "npm run", "go run"}
		if failAt > 0 {
			want = want[:failAt]
		}
		if !reflect.DeepEqual(calls, want) || (err != nil) != (failAt > 0) {
			t.Fatalf("failAt=%d: calls=%v err=%v", failAt, calls, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runDevelopmentDSH(ctx, func(context.Context, string, string, ...string) error {
		t.Fatal("canceled task launched a command")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
