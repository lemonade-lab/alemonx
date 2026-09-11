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
