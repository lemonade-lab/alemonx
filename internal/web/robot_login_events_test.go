package web

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestRobotLoginJournalReadsCompletedNewRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	write := func(value string, appendOnly bool) {
		t.Helper()
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendOnly {
			flags = os.O_WRONLY | os.O_APPEND
		}
		file, err := os.OpenFile(path, flags, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.WriteString(value)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("write %v, close %v", writeErr, closeErr)
		}
	}
	var journal robotLoginJournal
	var ids []string
	read := func() {
		journal.read(context.Background(), path, func(record robotCBPFileRecord) {
			ids = append(ids, record.ID)
		})
	}
	write("{\"id\":\"one\"}\n{\"id\":", false)
	read()
	read()
	write("\"two\"}\n", true)
	read()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	write("{\"id\":\"three\"}\n", false)
	read()
	write("{\"id\":\"4\"}\n", false)
	read()
	if want := []string{"one", "two", "three", "4"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("events = %v, want %v", ids, want)
	}
}

func TestRobotLoginWatcherReplacesPreviousGeneration(t *testing.T) {
	s := &server{}
	root := t.TempDir()
	at := time.Now()
	s.startRobotLoginWatcher(root, at, false)
	t.Cleanup(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, watcher := range s.loginWatchers {
			watcher.cancel()
		}
	})
	s.mu.RLock()
	initialCount := len(s.loginWatchers)
	var previous *robotLoginWatcher
	for _, watcher := range s.loginWatchers {
		previous = watcher
	}
	s.mu.RUnlock()
	if initialCount != 1 {
		t.Fatalf("expected one initial watcher, got %d", initialCount)
	}

	s.startRobotLoginWatcher(root, at.Add(time.Second), false)
	s.startRobotLoginWatcher(root, at, false)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.loginWatchers) != 1 {
		t.Fatalf("expected one watcher, got %d", len(s.loginWatchers))
	}
	for _, watcher := range s.loginWatchers {
		if watcher == previous || !watcher.startedAt.Equal(at.Add(time.Second)) {
			t.Fatal("new watcher was replaced by an older launch")
		}
	}
}
