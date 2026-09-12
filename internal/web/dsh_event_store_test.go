package web

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestDSHEventStorePersistsSafeReplayBySession(t *testing.T) {
	dir := t.TempDir()
	store := newDSHEventStore(dir)
	store.append("runtime-a", dshEventDTO{SessionID: "session-a", Type: "assistant/message", Text: "safe"})
	store.append("runtime-a", dshEventDTO{SessionID: "session-b", Type: "assistant/message", Text: "other"})
	restarted := newDSHEventStore(dir)
	events := restarted.after("runtime-a", "session-a", 0)
	if len(events) != 1 || events[0].Text != "safe" || events[0].ID != 1 {
		t.Fatalf("会话回放 = %#v", events)
	}
	if events := restarted.after("runtime-a", "session-a", 1); len(events) != 0 {
		t.Fatalf("Last-Event-ID 回放 = %#v", events)
	}
}

func TestDSHEventStoreCompactsDurableReplayWithoutRenumbering(t *testing.T) {
	dir := t.TempDir()
	store := newDSHEventStore(dir)
	for i := 0; i < maxPersistedDSHEvents+8; i++ {
		store.append("runtime-a", dshEventDTO{SessionID: "session-a", Type: "assistant/message", Text: "safe"})
	}
	restarted := newDSHEventStore(dir)
	events := restarted.after("runtime-a", "session-a", 0)
	if len(events) != maxPersistedDSHEvents || events[0].ID != 9 || events[len(events)-1].ID != maxPersistedDSHEvents+8 {
		t.Fatalf("unexpected compacted replay bounds: first=%+v last=%+v len=%d", events[0], events[len(events)-1], len(events))
	}
	file, err := os.Open(filepath.Join(dir, "runtime-a.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil || count != maxPersistedDSHEvents {
		t.Fatalf("durable file count=%d err=%v", count, err)
	}
}

func TestDSHEventStoreMigrationAndRelocationPreserveReplay(t *testing.T) {
	legacyDir, nextDir := t.TempDir(), t.TempDir()
	legacy := newDSHEventStore(legacyDir)
	legacy.append("old", dshEventDTO{RuntimeID: "old", SessionID: "session", Text: "retained"})
	next := newDSHEventStore(nextDir)
	next.legacyDir = legacyDir
	if events := next.after("old", "session", 0); len(events) != 1 || events[0].ID != 1 {
		t.Fatalf("legacy replay = %+v", events)
	}
	if err := next.relocateRuntime("old", "new"); err != nil {
		t.Fatal(err)
	}
	restarted := newDSHEventStore(nextDir)
	restarted.append("new", dshEventDTO{RuntimeID: "new", SessionID: "session", Text: "continued"})
	events := restarted.after("new", "session", 0)
	if len(events) != 2 || events[0].RuntimeID != "new" || events[1].ID != 2 {
		t.Fatalf("relocated replay = %+v", events)
	}
	for _, dir := range []string{legacyDir, nextDir} {
		if _, err := os.Stat(filepath.Join(dir, "old.jsonl")); err != nil {
			t.Fatalf("original replay removed: %v", err)
		}
	}
	if err := restarted.relocateRuntime("old", "new"); err == nil {
		t.Fatal("existing target was overwritten")
	}
}
