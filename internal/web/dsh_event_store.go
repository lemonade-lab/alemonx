package web

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"alemonx/internal/dsh"
)

const maxPersistedDSHEvents = 1024

// dshEventStore persists only browser-safe DTOs. Raw SDK notifications never
// reach this store, so reconnect replay cannot leak tool arguments or provider
// responses that were intentionally filtered by publicDSHEvent.
type dshEventStore struct {
	mu      sync.Mutex
	dir     string
	events  map[string][]dshEventDTO
	nextID  map[string]int64
	relayed map[string]bool
}

func newDSHEventStore(dir string) *dshEventStore {
	return &dshEventStore{dir: dir, events: map[string][]dshEventDTO{}, nextID: map[string]int64{}, relayed: map[string]bool{}}
}

func (s *dshEventStore) ensureRelay(runtimeID string, runtime *dsh.Runtime) {
	s.mu.Lock()
	if s.relayed[runtimeID] {
		s.mu.Unlock()
		return
	}
	s.loadLocked(runtimeID)
	s.relayed[runtimeID] = true
	s.mu.Unlock()
	events, _ := runtime.Subscribe()
	go func() {
		for event := range events {
			s.append(runtimeID, publicDSHEvent(event, runtimeID))
		}
	}()
}

func (s *dshEventStore) append(runtimeID string, event dshEventDTO) {
	s.mu.Lock()
	s.loadLocked(runtimeID)
	s.nextID[runtimeID]++
	event.ID = s.nextID[runtimeID]
	s.events[runtimeID] = append(s.events[runtimeID], event)
	trimmed := false
	if len(s.events[runtimeID]) > maxPersistedDSHEvents {
		s.events[runtimeID] = append([]dshEventDTO(nil), s.events[runtimeID][len(s.events[runtimeID])-maxPersistedDSHEvents:]...)
		trimmed = true
	}
	_ = s.appendLocked(runtimeID, event)
	if trimmed {
		_ = s.compactLocked(runtimeID)
	}
	s.mu.Unlock()
}

func (s *dshEventStore) after(runtimeID, sessionID string, after int64) []dshEventDTO {
	s.mu.Lock()
	s.loadLocked(runtimeID)
	result := make([]dshEventDTO, 0)
	for _, event := range s.events[runtimeID] {
		if event.ID > after && (event.SessionID == "" || event.SessionID == sessionID) {
			result = append(result, event)
		}
	}
	s.mu.Unlock()
	return result
}

func (s *dshEventStore) loadLocked(runtimeID string) {
	if _, ok := s.events[runtimeID]; ok {
		return
	}
	s.events[runtimeID] = []dshEventDTO{}
	raw, err := os.Open(filepath.Join(s.dir, runtimeID+".jsonl"))
	if err != nil {
		return
	}
	defer raw.Close()
	scanner := bufio.NewScanner(raw)
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		var event dshEventDTO
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.ID > 0 {
			s.events[runtimeID] = append(s.events[runtimeID], event)
			if event.ID > s.nextID[runtimeID] {
				s.nextID[runtimeID] = event.ID
			}
		}
	}
	if len(s.events[runtimeID]) > maxPersistedDSHEvents {
		s.events[runtimeID] = append([]dshEventDTO(nil), s.events[runtimeID][len(s.events[runtimeID])-maxPersistedDSHEvents:]...)
		_ = s.compactLocked(runtimeID)
	}
}

func (s *dshEventStore) appendLocked(runtimeID string, event dshEventDTO) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.dir, runtimeID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(raw, '\n'))
	return err
}

// compactLocked keeps durable replay bounded too, not merely the in-memory
// cache. IDs are never renumbered, so a reconnecting client can safely use a
// Last-Event-ID that predates the retained window.
func (s *dshEventStore) compactLocked(runtimeID string) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, runtimeID+"-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	for _, event := range s.events[runtimeID] {
		raw, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			_ = tmp.Close()
			return marshalErr
		}
		if _, writeErr := tmp.Write(append(raw, '\n')); writeErr != nil {
			_ = tmp.Close()
			return writeErr
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(s.dir, runtimeID+".jsonl"))
}
