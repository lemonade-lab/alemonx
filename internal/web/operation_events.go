package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const operationHistoryLimit = 40
const operationEventRetention = 7 * 24 * time.Hour
const operationEventMaxBytes = 10 * 1024 * 1024
const operationVacuumThreshold = 20 * 1024 * 1024
const operationSchemaVersion = 1

var errOperationStoreReadOnly = errors.New("事件库版本高于当前程序")

type eventEnvelope struct {
	ID        int64           `json:"id"`
	Topic     string          `json:"topic"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"createdAt"`
	Data      json.RawMessage `json:"data"`
}

// OperationEventStore owns durable operation snapshots and the global event
// journal. A failed store never publishes an event: callers can safely fall
// back to REST snapshots while this type repairs its local database.
type OperationEventStore struct {
	mu             sync.Mutex
	db             *sql.DB
	path           string
	writes         int
	activeReplays  int
	health         string
	lastCheck      time.Time
	lastBackup     time.Time
	lastVacuum     time.Time
	degradedReason string
	recoverySource string
	recoveryAt     time.Time
	recovering     bool
	onRecovered    func()
}

func newOperationEventStore(path string) *OperationEventStore {
	s := &OperationEventStore{path: path, health: "degraded"}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		s.degradedReason = safeStoreError(err)
		return s
	}
	if err := s.open(path); err == nil {
		s.migrateJSON(filepath.Join(filepath.Dir(path), "events.json"))
		s.mu.Lock()
		s.maintenanceLocked(time.Now().UTC(), true)
		s.mu.Unlock()
		return s
	} else if errors.Is(err, errOperationStoreReadOnly) {
		return s
	}
	if !s.restore("startup") {
		s.mu.Lock()
		s.health = "degraded"
		s.mu.Unlock()
	}
	return s
}

func (s *OperationEventStore) setRecoveredHandler(handler func()) {
	s.mu.Lock()
	s.onRecovered = handler
	s.mu.Unlock()
}

func (s *OperationEventStore) wasRecovered() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health == "recovered" || s.health == "recovered-empty"
}

func (s *OperationEventStore) open(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	// auto_vacuum is intentionally set before schema creation for fresh stores;
	// old stores remain compatible and incremental_vacuum is simply a no-op until
	// SQLite has been rebuilt once by normal maintenance.
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA auto_vacuum=INCREMENTAL;
CREATE TABLE IF NOT EXISTS schema_meta(version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS event_journal(id INTEGER PRIMARY KEY AUTOINCREMENT, topic TEXT NOT NULL, type TEXT NOT NULL, payload BLOB NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operation_snapshots(id TEXT PRIMARY KEY, payload BLOB NOT NULL, created_at TEXT NOT NULL, position INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS event_journal_topic_id ON event_journal(topic,id);
CREATE INDEX IF NOT EXISTS event_journal_created_at ON event_journal(created_at);
INSERT INTO schema_meta(version) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM schema_meta);`); err != nil {
		_ = db.Close()
		return err
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version); err != nil {
		_ = db.Close()
		return err
	}
	if version > operationSchemaVersion {
		s.mu.Lock()
		s.db, s.health, s.degradedReason = db, "read-only", "事件库由较新版本创建"
		s.mu.Unlock()
		return errOperationStoreReadOnly
	}
	var result string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil || result != "ok" {
		_ = db.Close()
		if err != nil {
			return err
		}
		return errors.New("SQLite 完整性检查失败")
	}
	s.mu.Lock()
	if s.db != nil {
		_ = s.db.Close()
	}
	s.db, s.health, s.degradedReason = db, "healthy", ""
	s.lastCheck = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

func safeStoreError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 160 {
		message = message[:160]
	}
	return sensitiveEventText.ReplaceAllString(message, "[REDACTED]")
}

func isStructuralSQLiteError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"malformed", "not a database", "database disk image", "database or disk is full", "database schema is locked", "file is not a database", "i/o error"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (s *OperationEventStore) fail(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	if s.health == "read-only" || s.recovering {
		s.mu.Unlock()
		return
	}
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.health, s.degradedReason, s.recovering = "degraded", safeStoreError(err), true
	s.mu.Unlock()
	go func() {
		s.restore("runtime")
		s.mu.Lock()
		s.recovering = false
		handler := s.onRecovered
		recovered := s.health == "recovered" || s.health == "recovered-empty"
		s.mu.Unlock()
		if recovered && handler != nil {
			handler()
		}
	}()
}

func (s *OperationEventStore) archiveCorrupt() {
	dir := filepath.Join(filepath.Dir(s.path), "backups", "corrupt-"+time.Now().UTC().Format("20060102-150405.000000000"))
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		source := s.path + suffix
		if _, err := os.Stat(source); err == nil {
			_ = os.Rename(source, filepath.Join(dir, filepath.Base(source)))
		}
	}
}

func (s *OperationEventStore) backupsNewestFirst() []string {
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "backups", "events-*.db"))
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

func validateOperationDatabase(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err = db.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil || result != "ok" {
		if err != nil {
			return err
		}
		return errors.New("SQLite 完整性检查失败")
	}
	var version int
	if err = db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version); err != nil {
		return err
	}
	if version > operationSchemaVersion {
		return errOperationStoreReadOnly
	}
	return nil
}

// restore never overwrites a corrupt file. It first archives all SQLite side
// files, then validates copies of newest backups before atomically installing
// the first good candidate.
func (s *OperationEventStore) restore(reason string) bool {
	s.mu.Lock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.health = "recovering"
	s.mu.Unlock()
	s.archiveCorrupt()
	for _, backup := range s.backupsNewestFirst() {
		temporary := s.path + ".restore"
		data, err := os.ReadFile(backup)
		if err != nil || os.WriteFile(temporary, data, 0600) != nil {
			continue
		}
		if err = validateOperationDatabase(temporary); err != nil {
			_ = os.Remove(temporary)
			continue
		}
		if err = os.Rename(temporary, s.path); err != nil {
			_ = os.Remove(temporary)
			continue
		}
		if err = s.open(s.path); err == nil {
			s.mu.Lock()
			s.health, s.recoverySource, s.recoveryAt = "recovered", backup, time.Now().UTC()
			s.mu.Unlock()
			return true
		}
	}
	_ = os.Remove(s.path)
	_ = os.Remove(s.path + "-wal")
	_ = os.Remove(s.path + "-shm")
	if err := s.open(s.path); err != nil {
		s.mu.Lock()
		s.health, s.degradedReason = "degraded", safeStoreError(err)
		s.mu.Unlock()
		return false
	}
	s.mu.Lock()
	s.health, s.recoverySource, s.recoveryAt = "recovered-empty", reason, time.Now().UTC()
	s.mu.Unlock()
	return true
}

func (s *OperationEventStore) beginReplay() func() {
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	s.activeReplays++
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		if s.activeReplays > 0 {
			s.activeReplays--
		}
		s.mu.Unlock()
	}
}

func (s *OperationEventStore) diagnostics() map[string]any {
	if s == nil {
		return map[string]any{"health": "degraded"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil && time.Since(s.lastCheck) > 24*time.Hour {
		if !s.quickCheckLocked() {
			go s.fail(errors.New("SQLite 完整性检查失败"))
		}
	}
	var version, count, earliest, latest int64
	if s.db != nil {
		_ = s.db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM event_journal`).Scan(&count)
		_ = s.db.QueryRow(`SELECT COALESCE(MIN(id),0),COALESCE(MAX(id),0) FROM event_journal`).Scan(&earliest, &latest)
	}
	var size, walSize int64
	if info, _ := os.Stat(s.path); info != nil {
		size = info.Size()
	}
	if info, _ := os.Stat(s.path + "-wal"); info != nil {
		walSize = info.Size()
	}
	return map[string]any{"health": s.health, "schemaVersion": version, "eventCount": count, "databaseBytes": size, "walBytes": walSize, "earliestEventId": earliest, "latestEventId": latest, "lastCheck": s.lastCheck, "lastBackup": s.lastBackup, "lastVacuum": s.lastVacuum, "backupCount": len(s.backupsNewestFirst()), "degradedReason": s.degradedReason, "recoverySource": s.recoverySource, "recoveryAt": s.recoveryAt, "activeReplays": s.activeReplays}
}

func (s *OperationEventStore) quickCheckLocked() bool {
	if s.db == nil {
		return false
	}
	var result string
	err := s.db.QueryRow(`PRAGMA quick_check`).Scan(&result)
	s.lastCheck = time.Now().UTC()
	return err == nil && result == "ok"
}

func (s *OperationEventStore) backupLocked(now time.Time) {
	if s.db == nil || now.Sub(s.lastBackup) < 24*time.Hour {
		return
	}
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	dir := filepath.Join(filepath.Dir(s.path), "backups")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	temporary := filepath.Join(dir, ".events-backup.tmp")
	target := filepath.Join(dir, "events-"+now.UTC().Format("20060102-150405")+".db")
	data, err := os.ReadFile(s.path)
	if err != nil || os.WriteFile(temporary, data, 0600) != nil || validateOperationDatabase(temporary) != nil {
		_ = os.Remove(temporary)
		return
	}
	if os.Rename(temporary, target) == nil {
		s.lastBackup = now
	}
	files := s.backupsNewestFirst()
	for len(files) > 7 {
		_ = os.Remove(files[len(files)-1])
		files = files[:len(files)-1]
	}
}

func (s *OperationEventStore) maintenanceLocked(now time.Time, force bool) {
	if s.db == nil || s.health == "degraded" || s.health == "recovering" || s.health == "read-only" {
		return
	}
	if force || s.writes%100 == 0 {
		s.pruneLocked(now)
		_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	}
	if force || now.Sub(s.lastCheck) >= 24*time.Hour {
		if !s.quickCheckLocked() {
			go s.fail(errors.New("SQLite 完整性检查失败"))
			return
		}
	}
	s.backupLocked(now)
	if s.activeReplays != 0 || now.Sub(s.lastVacuum) < 24*time.Hour {
		return
	}
	if info, err := os.Stat(s.path); err == nil && info.Size() > operationVacuumThreshold {
		if _, err = s.db.Exec(`PRAGMA incremental_vacuum`); err == nil {
			s.lastVacuum = now
		}
	}
}

func (s *OperationEventStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	s.backupLocked(time.Now().UTC())
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *OperationEventStore) snapshot() []operationTask {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT payload FROM operation_snapshots ORDER BY position ASC`)
	if err != nil {
		if isStructuralSQLiteError(err) {
			go s.fail(err)
		}
		return nil
	}
	defer rows.Close()
	var out []operationTask
	for rows.Next() {
		var raw []byte
		if rows.Scan(&raw) == nil {
			var item operationTask
			if json.Unmarshal(raw, &item) == nil {
				out = append(out, item)
			}
		}
	}
	return out
}

func (s *OperationEventStore) append(topic, kind string, data any, operations []operationTask) (eventEnvelope, error) {
	if s == nil {
		return eventEnvelope{}, os.ErrInvalid
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return eventEnvelope{}, err
	}
	now := time.Now().UTC()
	s.mu.Lock()
	if s.db == nil || s.health == "degraded" || s.health == "recovering" || s.health == "read-only" {
		s.mu.Unlock()
		return eventEnvelope{}, os.ErrInvalid
	}
	tx, err := s.db.Begin()
	if err == nil && operations != nil {
		if len(operations) > operationHistoryLimit {
			operations = operations[:operationHistoryLimit]
		}
		_, err = tx.Exec(`DELETE FROM operation_snapshots`)
		for position, item := range operations {
			if err != nil {
				break
			}
			raw, marshalErr := json.Marshal(item)
			if marshalErr != nil {
				err = marshalErr
				break
			}
			_, err = tx.Exec(`INSERT INTO operation_snapshots(id,payload,created_at,position) VALUES(?,?,?,?)`, item.ID, raw, now.Format(time.RFC3339Nano), position)
		}
	}
	var insert sql.Result
	if err == nil {
		insert, err = tx.Exec(`INSERT INTO event_journal(topic,type,payload,created_at) VALUES(?,?,?,?)`, topic, kind, payload, now.Format(time.RFC3339Nano))
	}
	if err != nil {
		if tx != nil {
			_ = tx.Rollback()
		}
		s.mu.Unlock()
		if isStructuralSQLiteError(err) {
			s.fail(err)
		}
		return eventEnvelope{}, err
	}
	id, err := insert.LastInsertId()
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.mu.Unlock()
		if isStructuralSQLiteError(err) {
			s.fail(err)
		}
		return eventEnvelope{}, err
	}
	s.writes++
	s.maintenanceLocked(now, false)
	s.mu.Unlock()
	return eventEnvelope{ID: id, Topic: topic, Type: kind, CreatedAt: now, Data: payload}, nil
}

func (s *OperationEventStore) after(id int64, topics map[string]bool) []eventEnvelope {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.db == nil {
		s.mu.Unlock()
		return nil
	}
	rows, err := s.db.Query(`SELECT id,topic,type,payload,created_at FROM event_journal WHERE id>? ORDER BY id ASC`, id)
	if err != nil {
		s.mu.Unlock()
		if isStructuralSQLiteError(err) {
			s.fail(err)
		}
		return nil
	}
	var out []eventEnvelope
	for rows.Next() {
		var item eventEnvelope
		var created string
		if rows.Scan(&item.ID, &item.Topic, &item.Type, &item.Data, &created) == nil && (len(topics) == 0 || topics[item.Topic]) {
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			out = append(out, item)
		}
	}
	_ = rows.Close()
	s.mu.Unlock()
	return out
}

func (s *OperationEventStore) bounds() (int64, int64) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return 0, 0
	}
	var earliest, latest int64
	if err := s.db.QueryRow(`SELECT COALESCE(MIN(id),0),COALESCE(MAX(id),0) FROM event_journal`).Scan(&earliest, &latest); err != nil && isStructuralSQLiteError(err) {
		go s.fail(err)
	}
	return earliest, latest
}

func (s *OperationEventStore) pruneLocked(now time.Time) {
	if s.db == nil {
		return
	}
	_, _ = s.db.Exec(`DELETE FROM event_journal WHERE created_at < ?`, now.Add(-operationEventRetention).UTC().Format(time.RFC3339Nano))
	for {
		var bytes int64
		_ = s.db.QueryRow(`SELECT COALESCE(SUM(length(payload)),0) FROM event_journal`).Scan(&bytes)
		if bytes <= operationEventMaxBytes {
			break
		}
		result, _ := s.db.Exec(`DELETE FROM event_journal WHERE id=(SELECT id FROM event_journal ORDER BY id ASC LIMIT 1)`)
		changed, _ := result.RowsAffected()
		if changed == 0 {
			break
		}
	}
}

func (s *OperationEventStore) migrateJSON(path string) {
	if s == nil {
		return
	}
	if _, err := os.Stat(path); err != nil {
		return
	}
	var legacy struct {
		Operations []operationTask `json:"operations"`
		Events     []struct {
			Event     robotEvent `json:"event"`
			CreatedAt time.Time  `json:"createdAt"`
		} `json:"events"`
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &legacy) != nil {
		return
	}
	for _, item := range legacy.Events {
		_, _ = s.append("robot", item.Event.Type, item.Event, nil)
	}
	if len(legacy.Operations) > 0 {
		_, _ = s.append("migration", "operations.migrated", map[string]bool{"ok": true}, legacy.Operations)
	}
	_ = os.Rename(path, path+".migrated")
}

func (s *OperationEventStore) String() string { return fmt.Sprintf("OperationEventStore(%s)", s.path) }
