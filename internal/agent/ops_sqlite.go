package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteOpsRepository is a pure-Go SQLite repository. It does not invoke a
// system sqlite3 binary and therefore works in minimal production images.
type SQLiteOpsRepository struct {
	path string
	db   *sql.DB
	mu   sync.Mutex
}

const sqliteOpsSchemaVersion = 2

func (s *SQLiteOpsRepository) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func NewSQLiteOpsRepository(path string) (*SQLiteOpsRepository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite 路径不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	repo := &SQLiteOpsRepository{path: path, db: db}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS schema_meta(version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS records(kind TEXT NOT NULL,id TEXT NOT NULL,payload TEXT NOT NULL,updated TEXT NOT NULL,PRIMARY KEY(kind,id));
CREATE TABLE IF NOT EXISTS incidents(id TEXT PRIMARY KEY, fingerprint TEXT, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS todos(id TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS maintenance_runs(id TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS policies(project_root TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS alerts(id TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS alert_deliveries(id TEXT PRIMARY KEY, payload TEXT NOT NULL, status TEXT NOT NULL, next_attempt TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS ops_metrics(metric_name TEXT NOT NULL, project_root TEXT NOT NULL, fingerprint TEXT NOT NULL, value REAL NOT NULL, window_start TEXT NOT NULL, window_end TEXT NOT NULL, updated TEXT NOT NULL, PRIMARY KEY(metric_name,project_root,fingerprint));
CREATE TABLE IF NOT EXISTS ops_metric_buckets(metric_name TEXT NOT NULL, project_root TEXT NOT NULL, fingerprint TEXT NOT NULL, bucket_start TEXT NOT NULL, value REAL NOT NULL, updated TEXT NOT NULL, PRIMARY KEY(metric_name,project_root,fingerprint,bucket_start));
INSERT OR IGNORE INTO ops_metric_buckets(metric_name,project_root,fingerprint,bucket_start,value,updated) SELECT metric_name,project_root,fingerprint,window_start,value,updated FROM ops_metrics;
CREATE TABLE IF NOT EXISTS audit_entries(id TEXT PRIMARY KEY, payload TEXT NOT NULL, created TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS ops_events(id TEXT PRIMARY KEY, incident_id TEXT, payload TEXT NOT NULL, created TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS log_cursors(cursor_key TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS budgets(project_root TEXT PRIMARY KEY, payload TEXT NOT NULL, updated TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS leases(lease_key TEXT PRIMARY KEY, payload TEXT NOT NULL, expires_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS records_kind_updated ON records(kind, updated DESC);
CREATE UNIQUE INDEX IF NOT EXISTS records_event_id ON records(kind, id) WHERE kind LIKE 'event-%';
INSERT INTO schema_meta(version) SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM schema_meta);`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repo.migrateSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repo, nil
}

// migrateSchema is intentionally incremental and idempotent. Existing v1
// installations keep their JSON payloads; v2 adds indexed core columns so
// production reads no longer depend on decoding every record first.
func (s *SQLiteOpsRepository) migrateSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var version int
	if err := tx.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&version); err != nil {
		return err
	}
	if version > sqliteOpsSchemaVersion {
		return fmt.Errorf("SQLite 运维数据版本 %d 高于当前程序支持的版本 %d", version, sqliteOpsSchemaVersion)
	}
	if version < 2 {
		columns := map[string][]string{
			"incidents":        {"project_root TEXT NOT NULL DEFAULT ''", "process_name TEXT NOT NULL DEFAULT ''", "status TEXT NOT NULL DEFAULT ''", "severity TEXT NOT NULL DEFAULT ''", "first_seen TEXT NOT NULL DEFAULT ''", "last_seen TEXT NOT NULL DEFAULT ''", "occurrences INTEGER NOT NULL DEFAULT 0"},
			"todos":            {"incident_id TEXT NOT NULL DEFAULT ''", "project_root TEXT NOT NULL DEFAULT ''", "status TEXT NOT NULL DEFAULT ''", "severity TEXT NOT NULL DEFAULT ''"},
			"maintenance_runs": {"incident_id TEXT NOT NULL DEFAULT ''", "task_id TEXT NOT NULL DEFAULT ''", "status TEXT NOT NULL DEFAULT ''", "created TEXT NOT NULL DEFAULT ''", "finished TEXT NOT NULL DEFAULT ''"},
			"policies":         {"mode TEXT NOT NULL DEFAULT ''", "auto_allowed INTEGER NOT NULL DEFAULT 0"},
			"alerts":           {"fingerprint TEXT NOT NULL DEFAULT ''", "status TEXT NOT NULL DEFAULT ''", "severity TEXT NOT NULL DEFAULT ''", "project_root TEXT NOT NULL DEFAULT ''"},
			"audit_entries":    {"actor TEXT NOT NULL DEFAULT ''", "role TEXT NOT NULL DEFAULT ''", "action TEXT NOT NULL DEFAULT ''", "resource TEXT NOT NULL DEFAULT ''", "result TEXT NOT NULL DEFAULT ''"},
			"ops_events":       {"project_root TEXT NOT NULL DEFAULT ''", "fingerprint TEXT NOT NULL DEFAULT ''"},
			"log_cursors":      {"project_root TEXT NOT NULL DEFAULT ''", "process_name TEXT NOT NULL DEFAULT ''", "mode TEXT NOT NULL DEFAULT ''"},
			"budgets":          {"used_tokens INTEGER NOT NULL DEFAULT 0", "used_pm2_actions INTEGER NOT NULL DEFAULT 0", "retry_count INTEGER NOT NULL DEFAULT 0"},
			"leases":           {"owner_id TEXT NOT NULL DEFAULT ''", "fencing_token INTEGER NOT NULL DEFAULT 0", "created_at TEXT NOT NULL DEFAULT ''", "renewed_at TEXT NOT NULL DEFAULT ''"},
		}
		for table, definitions := range columns {
			for _, definition := range definitions {
				name := strings.Fields(definition)[0]
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, name).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					if _, err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + definition); err != nil {
						return err
					}
				}
			}
		}
		backfill := []string{
			`UPDATE incidents SET fingerprint=COALESCE(NULLIF(fingerprint,''),json_extract(payload,'$.fingerprint')), project_root=COALESCE(NULLIF(project_root,''),json_extract(payload,'$.projectRoot')), process_name=COALESCE(NULLIF(process_name,''),json_extract(payload,'$.processName')), status=COALESCE(NULLIF(status,''),json_extract(payload,'$.status')), severity=COALESCE(NULLIF(severity,''),json_extract(payload,'$.severity')), first_seen=COALESCE(NULLIF(first_seen,''),json_extract(payload,'$.firstSeen')), last_seen=COALESCE(NULLIF(last_seen,''),json_extract(payload,'$.lastSeen')), occurrences=CASE WHEN occurrences=0 THEN COALESCE(json_extract(payload,'$.occurrences'),0) ELSE occurrences END`,
			`UPDATE todos SET incident_id=COALESCE(NULLIF(incident_id,''),json_extract(payload,'$.incidentId')), project_root=COALESCE(NULLIF(project_root,''),json_extract(payload,'$.projectRoot')), status=COALESCE(NULLIF(status,''),json_extract(payload,'$.status')), severity=COALESCE(NULLIF(severity,''),json_extract(payload,'$.severity'))`,
			`UPDATE maintenance_runs SET incident_id=COALESCE(NULLIF(incident_id,''),json_extract(payload,'$.incidentId')), task_id=COALESCE(NULLIF(task_id,''),json_extract(payload,'$.taskId')), status=COALESCE(NULLIF(status,''),json_extract(payload,'$.status')), created=COALESCE(NULLIF(created,''),json_extract(payload,'$.created')), finished=COALESCE(NULLIF(finished,''),json_extract(payload,'$.finished'))`,
			`UPDATE policies SET mode=COALESCE(NULLIF(mode,''),json_extract(payload,'$.mode')), auto_allowed=CASE WHEN auto_allowed=0 THEN COALESCE(json_extract(payload,'$.autoAllowed'),0) ELSE auto_allowed END`,
			`UPDATE alerts SET fingerprint=COALESCE(NULLIF(fingerprint,''),json_extract(payload,'$.fingerprint')), status=COALESCE(NULLIF(status,''),json_extract(payload,'$.status')), severity=COALESCE(NULLIF(severity,''),json_extract(payload,'$.severity')), project_root=COALESCE(NULLIF(project_root,''),json_extract(payload,'$.projectRoot'))`,
			`UPDATE audit_entries SET actor=COALESCE(NULLIF(actor,''),json_extract(payload,'$.actor')), role=COALESCE(NULLIF(role,''),json_extract(payload,'$.role')), action=COALESCE(NULLIF(action,''),json_extract(payload,'$.action')), resource=COALESCE(NULLIF(resource,''),json_extract(payload,'$.resource')), result=COALESCE(NULLIF(result,''),json_extract(payload,'$.result'))`,
			`UPDATE ops_events SET project_root=COALESCE(NULLIF(project_root,''),json_extract(payload,'$.projectRoot')), fingerprint=COALESCE(NULLIF(fingerprint,''),json_extract(payload,'$.fingerprint'))`,
			`UPDATE log_cursors SET project_root=COALESCE(NULLIF(project_root,''),json_extract(payload,'$.projectRoot')), process_name=COALESCE(NULLIF(process_name,''),json_extract(payload,'$.processName')), mode=COALESCE(NULLIF(mode,''),json_extract(payload,'$.mode'))`,
			`UPDATE budgets SET used_tokens=COALESCE(json_extract(payload,'$.usedTokens'),0), used_pm2_actions=COALESCE(json_extract(payload,'$.usedPm2Actions'),0), retry_count=COALESCE(json_extract(payload,'$.retryCount'),0)`,
			`UPDATE leases SET owner_id=COALESCE(NULLIF(owner_id,''),json_extract(payload,'$.ownerId')), fencing_token=COALESCE(json_extract(payload,'$.fencingToken'),0), created_at=COALESCE(NULLIF(created_at,''),json_extract(payload,'$.createdAt')), renewed_at=COALESCE(NULLIF(renewed_at,''),json_extract(payload,'$.renewedAt'))`,
		}
		for _, statement := range backfill {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		indexes := []string{
			`CREATE INDEX IF NOT EXISTS incidents_fingerprint_updated ON incidents(fingerprint,updated DESC)`,
			`CREATE INDEX IF NOT EXISTS incidents_root_status ON incidents(project_root,status,updated DESC)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS incidents_one_active_fingerprint ON incidents(project_root,process_name,fingerprint) WHERE status NOT IN ('resolved','silenced')`,
			`CREATE INDEX IF NOT EXISTS maintenance_active_incident ON maintenance_runs(incident_id,status,updated DESC)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS maintenance_one_active_incident ON maintenance_runs(incident_id) WHERE status IN ('queued','running','fixing','verifying','observing')`,
			`CREATE INDEX IF NOT EXISTS alerts_fingerprint_updated ON alerts(fingerprint,updated DESC)`,
			`CREATE INDEX IF NOT EXISTS ops_events_incident_created ON ops_events(incident_id,created DESC)`,
			`CREATE INDEX IF NOT EXISTS alert_deliveries_due ON alert_deliveries(status,next_attempt)`,
		}
		for _, statement := range indexes {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE schema_meta SET version=?`, sqliteOpsSchemaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteOpsRepository) save(kind, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO records(kind,id,payload,updated) VALUES(?,?,?,?)`, kind, id, string(payload), updated); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := saveSQLiteBusinessTx(tx, kind, id, value, payload, updated); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func saveSQLiteBusinessTx(tx *sql.Tx, kind, id string, value any, payload []byte, updated string) error {
	switch kind {
	case "incident":
		v, ok := value.(Incident)
		if !ok {
			return errors.New("事件数据类型无效")
		}
		_, err := tx.Exec(`INSERT INTO incidents(id,fingerprint,project_root,process_name,status,severity,first_seen,last_seen,occurrences,payload,updated) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET fingerprint=excluded.fingerprint,project_root=excluded.project_root,process_name=excluded.process_name,status=excluded.status,severity=excluded.severity,first_seen=excluded.first_seen,last_seen=excluded.last_seen,occurrences=excluded.occurrences,payload=excluded.payload,updated=excluded.updated`, id, v.Fingerprint, filepath.Clean(v.ProjectRoot), v.ProcessName, v.Status, v.Severity, formatSQLiteTime(v.FirstSeen), formatSQLiteTime(v.LastSeen), v.Occurrences, string(payload), updated)
		return err
	case "todo":
		v := value.(OpsTodo)
		_, err := tx.Exec(`INSERT INTO todos(id,incident_id,project_root,status,severity,payload,updated) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET incident_id=excluded.incident_id,project_root=excluded.project_root,status=excluded.status,severity=excluded.severity,payload=excluded.payload,updated=excluded.updated`, id, v.IncidentID, filepath.Clean(v.ProjectRoot), v.Status, v.Severity, string(payload), updated)
		return err
	case "maintenance":
		v := value.(MaintenanceRun)
		finished := ""
		if v.Finished != nil {
			finished = formatSQLiteTime(*v.Finished)
		}
		_, err := tx.Exec(`INSERT INTO maintenance_runs(id,incident_id,task_id,status,created,finished,payload,updated) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET incident_id=excluded.incident_id,task_id=excluded.task_id,status=excluded.status,created=excluded.created,finished=excluded.finished,payload=excluded.payload,updated=excluded.updated`, id, v.IncidentID, v.TaskID, v.Status, formatSQLiteTime(v.Created), finished, string(payload), updated)
		return err
	case "policy":
		v := value.(OpsPolicy)
		_, err := tx.Exec(`INSERT INTO policies(project_root,mode,auto_allowed,payload,updated) VALUES(?,?,?,?,?) ON CONFLICT(project_root) DO UPDATE SET mode=excluded.mode,auto_allowed=excluded.auto_allowed,payload=excluded.payload,updated=excluded.updated`, id, v.Mode, boolToInt(v.AutoAllowed), string(payload), updated)
		return err
	case "alert":
		v := value.(AlertRecord)
		_, err := tx.Exec(`INSERT INTO alerts(id,fingerprint,status,severity,project_root,payload,updated) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET fingerprint=excluded.fingerprint,status=excluded.status,severity=excluded.severity,project_root=excluded.project_root,payload=excluded.payload,updated=excluded.updated`, id, v.Fingerprint, v.Status, v.Severity, filepath.Clean(v.ProjectRoot), string(payload), updated)
		return err
	}
	return nil
}

func formatSQLiteTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *SQLiteOpsRepository) load(kind, id string, dst any) error {
	var payload string
	table, key := sqliteBusinessTable(kind)
	var err error
	if table != "" {
		err = s.db.QueryRow("SELECT payload FROM "+table+" WHERE "+key+"=?", id).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			err = s.db.QueryRow(`SELECT payload FROM records WHERE kind=? AND id=?`, kind, id).Scan(&payload)
		}
	} else {
		err = s.db.QueryRow(`SELECT payload FROM records WHERE kind=? AND id=?`, kind, id).Scan(&payload)
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(payload), dst)
}

func (s *SQLiteOpsRepository) query(kind string) ([]string, error) {
	table, _ := sqliteBusinessTable(kind)
	var rows *sql.Rows
	var err error
	if table != "" {
		rows, err = s.db.Query("SELECT payload FROM " + table + " ORDER BY updated DESC, rowid DESC")
	} else {
		rows, err = s.db.Query(`SELECT payload FROM records WHERE kind=? ORDER BY updated DESC, id DESC`, kind)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		out = append(out, payload)
	}
	return out, rows.Err()
}

func sqliteBusinessTable(kind string) (table, key string) {
	switch kind {
	case "incident":
		return "incidents", "id"
	case "todo":
		return "todos", "id"
	case "maintenance":
		return "maintenance_runs", "id"
	case "policy":
		return "policies", "project_root"
	case "alert":
		return "alerts", "id"
	case "cursor":
		return "log_cursors", "cursor_key"
	}
	return "", ""
}

func (s *SQLiteOpsRepository) SaveIncident(v Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("incident", v.ID, v)
}
func (s *SQLiteOpsRepository) GetIncident(id string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v Incident
	return v, s.load("incident", id, &v)
}
func (s *SQLiteOpsRepository) ListIncidents() ([]Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("incident")
	out := []Incident{}
	for _, raw := range rows {
		var v Incident
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *SQLiteOpsRepository) DeleteIncident(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM incidents WHERE id=?`, id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	legacy, err := tx.Exec(`DELETE FROM records WHERE kind='incident' AND id=?`, id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	affected, _ := res.RowsAffected()
	fallback, _ := legacy.RowsAffected()
	if affected == 0 && fallback == 0 {
		_ = tx.Rollback()
		return errors.New("事件不存在")
	}
	if _, err = tx.Exec(`DELETE FROM ops_events WHERE incident_id=?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *SQLiteOpsRepository) SaveTodo(v OpsTodo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("todo", v.ID, v)
}
func (s *SQLiteOpsRepository) GetTodo(id string) (OpsTodo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v OpsTodo
	return v, s.load("todo", id, &v)
}
func (s *SQLiteOpsRepository) ListTodos() ([]OpsTodo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("todo")
	out := []OpsTodo{}
	for _, raw := range rows {
		var v OpsTodo
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *SQLiteOpsRepository) DeleteTodo(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM todos WHERE id=?`, id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	legacy, err := tx.Exec(`DELETE FROM records WHERE kind='todo' AND id=?`, id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	affected, _ := res.RowsAffected()
	fallback, _ := legacy.RowsAffected()
	if affected == 0 && fallback == 0 {
		_ = tx.Rollback()
		return errors.New("待办不存在")
	}
	return tx.Commit()
}
func (s *SQLiteOpsRepository) SaveMaintenance(v MaintenanceRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("maintenance", v.ID, v)
}
func (s *SQLiteOpsRepository) GetMaintenance(id string) (MaintenanceRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v MaintenanceRun
	return v, s.load("maintenance", id, &v)
}
func (s *SQLiteOpsRepository) ListMaintenance() ([]MaintenanceRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("maintenance")
	out := []MaintenanceRun{}
	for _, raw := range rows {
		var v MaintenanceRun
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *SQLiteOpsRepository) SavePolicy(v OpsPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("policy", filepath.Clean(v.ProjectRoot), v)
}
func (s *SQLiteOpsRepository) GetPolicy(root string) (OpsPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v OpsPolicy
	return v, s.load("policy", filepath.Clean(root), &v)
}
func (s *SQLiteOpsRepository) ListPolicies() ([]OpsPolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("policy")
	out := []OpsPolicy{}
	for _, raw := range rows {
		var v OpsPolicy
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}

func (s *SQLiteOpsRepository) rawSave(kind, id, payload string) error {
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO records(kind,id,payload,updated) VALUES(?,?,?,?)`, kind, id, payload, updated); err != nil {
		_ = tx.Rollback()
		return err
	}
	if kind == "audit" {
		var value AuditEntry
		if err = json.Unmarshal([]byte(payload), &value); err == nil {
			_, err = tx.Exec(`INSERT OR REPLACE INTO audit_entries(id,actor,role,action,resource,result,payload,created) VALUES(?,?,?,?,?,?,?,?)`, id, value.Actor, value.Role, value.Action, value.Resource, value.Result, payload, formatSQLiteTime(value.Created))
		}
	} else if strings.HasPrefix(kind, "event-") {
		var value ErrorEvent
		if err = json.Unmarshal([]byte(payload), &value); err == nil {
			_, err = tx.Exec(`INSERT OR REPLACE INTO ops_events(id,incident_id,project_root,fingerprint,payload,created) VALUES(?,?,?,?,?,?)`, id, strings.TrimPrefix(kind, "event-"), filepath.Clean(value.ProjectRoot), value.Fingerprint, payload, formatSQLiteTime(value.Timestamp))
		}
	} else if kind == "cursor" {
		var value LogCursor
		if err = json.Unmarshal([]byte(payload), &value); err == nil {
			_, err = tx.Exec(`INSERT OR REPLACE INTO log_cursors(cursor_key,project_root,process_name,mode,payload,updated) VALUES(?,?,?,?,?,?)`, id, filepath.Clean(value.ProjectRoot), value.ProcessName, value.Mode, payload, updated)
		}
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *SQLiteOpsRepository) rawQuery(kind string) ([]string, error) { return s.query(kind) }
func (s *SQLiteOpsRepository) MarkEventSeen(key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	err := s.db.QueryRow(`SELECT payload FROM records WHERE kind='seen' AND id=?`, key).Scan(&payload)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return false, s.rawSave("seen", key, time.Now().Format(time.RFC3339Nano))
}
func (s *SQLiteOpsRepository) AppendEvent(id string, event ErrorEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.rawSave("event-"+id, event.ID, string(payload))
}
func (s *SQLiteOpsRepository) ListEvents(id string) ([]ErrorEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.rawQuery("event-" + id)
	out := []ErrorEvent{}
	for _, raw := range rows {
		var v ErrorEvent
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *SQLiteOpsRepository) AppendSignal(v OpsSignal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.rawSave("signal", newID("signal"), string(payload))
}
func (s *SQLiteOpsRepository) ListSignals() ([]OpsSignal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.rawQuery("signal")
	out := []OpsSignal{}
	for _, raw := range rows {
		var v OpsSignal
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}

func (s *SQLiteOpsRepository) SaveLogCursor(cursor LogCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cursor.Updated.IsZero() {
		cursor.Updated = time.Now()
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return s.rawSave("cursor", filepath.Clean(cursor.ProjectRoot)+"\x00"+cursor.ProcessName, string(payload))
}

func (s *SQLiteOpsRepository) GetLogCursor(projectRoot, processName string) (LogCursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cursor LogCursor
	return cursor, s.load("cursor", filepath.Clean(projectRoot)+"\x00"+processName, &cursor)
}
func (s *SQLiteOpsRepository) AppendAudit(v AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.rawSave("audit", v.ID, string(payload))
}
func (s *SQLiteOpsRepository) ListAudit() ([]AuditEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.rawQuery("audit")
	out := []AuditEntry{}
	for _, raw := range rows {
		var v AuditEntry
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}
func (s *SQLiteOpsRepository) SaveAlert(v AlertRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("alert", v.ID, v)
}
func (s *SQLiteOpsRepository) GetAlert(id string) (AlertRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v AlertRecord
	return v, s.load("alert", id, &v)
}
func (s *SQLiteOpsRepository) ListAlerts() ([]AlertRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("alert")
	out := []AlertRecord{}
	for _, raw := range rows {
		var v AlertRecord
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}

func (s *SQLiteOpsRepository) GetBudget(root string) (OpsBudget, error) {
	p, err := s.GetPolicy(root)
	if err != nil {
		return OpsBudget{}, err
	}
	return OpsBudget{TokenLimit: p.TokenBudget, UsedTokens: p.UsedTokens, MaxRetries: p.FailureCircuitBreak, RetryCount: p.RetryCount, MaxPM2Actions: p.MaxPM2Actions, UsedPM2Actions: p.UsedPM2Actions, MaxChangedFiles: p.MaxModifiedFiles}, nil
}
func (s *SQLiteOpsRepository) ConsumeBudget(root string, tokens, pm2, retries int) (OpsBudget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p OpsPolicy
	if err := s.load("policy", filepath.Clean(root), &p); err != nil {
		return OpsBudget{}, err
	}
	if p.TokenBudget > 0 && p.UsedTokens+tokens > p.TokenBudget || p.MaxPM2Actions > 0 && p.UsedPM2Actions+pm2 > p.MaxPM2Actions || p.FailureCircuitBreak > 0 && p.RetryCount+retries > p.FailureCircuitBreak {
		return OpsBudget{}, errors.New("运维预算已耗尽")
	}
	p.UsedTokens += tokens
	p.UsedPM2Actions += pm2
	p.RetryCount += retries
	if err := s.save("policy", filepath.Clean(root), p); err != nil {
		return OpsBudget{}, err
	}
	budget := OpsBudget{TokenLimit: p.TokenBudget, UsedTokens: p.UsedTokens, MaxRetries: p.FailureCircuitBreak, RetryCount: p.RetryCount, MaxPM2Actions: p.MaxPM2Actions, UsedPM2Actions: p.UsedPM2Actions, MaxChangedFiles: p.MaxModifiedFiles}
	payload, marshalErr := json.Marshal(budget)
	if marshalErr != nil {
		return OpsBudget{}, marshalErr
	}
	if _, execErr := s.db.Exec(`INSERT OR REPLACE INTO budgets(project_root,payload,updated) VALUES(?,?,?)`, filepath.Clean(root), string(payload), time.Now().UTC().Format(time.RFC3339Nano)); execErr != nil {
		return OpsBudget{}, execErr
	}
	return budget, nil
}
func (s *SQLiteOpsRepository) ResetBudget(root string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p OpsPolicy
	if err := s.load("policy", filepath.Clean(root), &p); err != nil {
		return err
	}
	p.UsedTokens, p.UsedPM2Actions, p.RetryCount = 0, 0, 0
	if err := s.save("policy", filepath.Clean(root), p); err != nil {
		return err
	}
	budget := OpsBudget{TokenLimit: p.TokenBudget, UsedTokens: p.UsedTokens, MaxRetries: p.FailureCircuitBreak, RetryCount: p.RetryCount, MaxPM2Actions: p.MaxPM2Actions, UsedPM2Actions: p.UsedPM2Actions, MaxChangedFiles: p.MaxModifiedFiles}
	payload, err := json.Marshal(budget)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO budgets(project_root,payload,updated) VALUES(?,?,?)`, filepath.Clean(root), string(payload), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *SQLiteOpsRepository) UpdateMaintenanceByTask(taskID, status, lastError string) error {
	return s.TransitionMaintenanceForTask(taskID, status, lastError)
}

func (s *SQLiteOpsRepository) TransitionMaintenanceForTask(taskID, status, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT payload FROM maintenance_runs ORDER BY updated DESC, rowid DESC`)
	if err != nil {
		return err
	}
	var run MaintenanceRun
	found := false
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return err
		}
		var candidate MaintenanceRun
		if json.Unmarshal([]byte(raw), &candidate) == nil && candidate.TaskID == taskID {
			run, found = candidate, true
			break
		}
	}
	_ = rows.Close()
	if !found {
		return nil
	}
	var incident Incident
	if err := loadSQLiteEntityTx(tx, "incidents", "id", run.IncidentID, &incident); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	now := time.Now()
	switch status {
	case "completed":
		duration := 5 * time.Minute
		var policy OpsPolicy
		if loadSQLiteEntityTx(tx, "policies", "project_root", filepath.Clean(incident.ProjectRoot), &policy) == nil && policy.ObservationMinutes > 0 {
			duration = time.Duration(policy.ObservationMinutes) * time.Minute
		}
		run.Status, run.Error, run.Finished = "observing", "", nil
		run.ObservationStarted, run.ObservationUntil = now, now.Add(duration)
		incident.Status = IncidentObserving
	case "failed", "cancelled":
		run.Status, run.Error, run.Finished = status, lastError, &now
		incident.Status = IncidentTodo
		todo := OpsTodo{ID: "todo-" + incident.ID, IncidentID: incident.ID, ProjectRoot: incident.ProjectRoot, Title: "处理：" + incident.ProcessName, Summary: incident.Sample, Severity: incident.Severity, Reason: lastError, Status: "open", Created: now, Updated: now}
		if err := saveSQLiteEntityTx(tx, "todo", todo.ID, todo); err != nil {
			return err
		}
	default:
		return nil
	}
	incident.Updated = now
	if err := saveSQLiteEntityTx(tx, "maintenance", run.ID, run); err != nil {
		return err
	}
	if err := saveSQLiteEntityTx(tx, "incident", incident.ID, incident); err != nil {
		return err
	}
	return tx.Commit()
}

func loadSQLiteEntityTx(tx *sql.Tx, table, key, id string, dst any) error {
	var raw string
	if err := tx.QueryRow(`SELECT payload FROM `+table+` WHERE `+key+`=?`, id).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dst)
}

func saveSQLiteEntityTx(tx *sql.Tx, kind, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	updated := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO records(kind,id,payload,updated) VALUES(?,?,?,?)`, kind, id, string(payload), updated); err != nil {
		return err
	}
	return saveSQLiteBusinessTx(tx, kind, id, value, payload, updated)
}
func (s *SQLiteOpsRepository) ReconcileMaintenance(tasks []AgentTask) error {
	byID := map[string]AgentTask{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	runs, err := s.ListMaintenance()
	if err != nil {
		return err
	}
	for _, run := range runs {
		if _, ok := byID[run.TaskID]; !ok && run.Status != "completed" && run.Status != "failed" {
			run.Status, run.Error = "recovery_required", "关联任务不存在"
			_ = s.SaveMaintenance(run)
		}
	}
	return nil
}
func (s *SQLiteOpsRepository) Metrics() (OpsMetrics, error) {
	inc, err := s.ListIncidents()
	if err != nil {
		return OpsMetrics{}, err
	}
	todo, err := s.ListTodos()
	if err != nil {
		return OpsMetrics{}, err
	}
	runs, err := s.ListMaintenance()
	if err != nil {
		return OpsMetrics{}, err
	}
	m := OpsMetrics{Incidents: len(inc), MaintenanceRuns: len(runs)}
	for _, t := range todo {
		if t.Status == "open" || t.Status == "in_progress" {
			m.OpenTodos++
		}
	}
	for _, i := range inc {
		if i.Status == IncidentResolved {
			m.Resolved++
		}
		if i.Occurrences > 1 {
			m.IncidentDeduplicated += i.Occurrences - 1
		}
		if i.Decision != "" {
			m.AIWakeups++
		}
	}
	for _, r := range runs {
		if r.RollbackPerformed {
			m.Rollbacks++
		}
		if r.Status == "completed" || r.Status == "observing" || r.Status == "resolved" {
			m.AutoFixSuccess++
		}
		if r.Status == "failed" {
			m.MaintenanceFailures++
		}
		if strings.Contains(strings.ToLower(r.Error), "pm2") {
			m.PM2ActionFailures++
		}
		if r.Status == "recovery_required" {
			m.RecoveryConflicts++
		}
	}
	if alerts, alertErr := s.ListAlerts(); alertErr == nil {
		m.AlertDeliveryTotal = len(alerts)
		for _, alert := range alerts {
			if alert.Status == "delivery_failed" {
				m.AlertDeliveryFailures++
			}
		}
	}
	if leases, leaseErr := s.ListLeases(); leaseErr == nil {
		for _, lease := range leases {
			if lease.FencingToken > 1 {
				m.LeaseTakeovers++
			}
		}
	}
	if snapshot, snapshotErr := s.Snapshot(""); snapshotErr == nil {
		m.IncidentDeduplicated = maxInt(m.IncidentDeduplicated, snapshot.IncidentDeduplicated)
		m.AIWakeups = maxInt(m.AIWakeups, snapshot.AIWakeups)
		m.MaintenanceFailures = maxInt(m.MaintenanceFailures, snapshot.MaintenanceFailures)
		m.Rollbacks = maxInt(m.Rollbacks, snapshot.Rollbacks)
		m.PM2ActionFailures = maxInt(m.PM2ActionFailures, snapshot.PM2ActionFailures)
		m.BudgetExhausted = maxInt(m.BudgetExhausted, snapshot.BudgetExhausted)
		m.AlertDeliveryTotal = maxInt(m.AlertDeliveryTotal, snapshot.AlertDeliveryTotal)
		m.AlertDeliveryFailures = maxInt(m.AlertDeliveryFailures, snapshot.AlertDeliveryFailures)
	}
	return m, nil
}
func (s *SQLiteOpsRepository) SaveScore(id string, v MaintenanceScore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("score", id, v)
}
func (s *SQLiteOpsRepository) SaveRole(v OpsRoleBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save("role", v.Account, v)
}
func (s *SQLiteOpsRepository) GetRole(account string) (OpsRoleBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v OpsRoleBinding
	return v, s.load("role", account, &v)
}
func (s *SQLiteOpsRepository) ListRoles() ([]OpsRoleBinding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.query("role")
	out := []OpsRoleBinding{}
	for _, raw := range rows {
		var v OpsRoleBinding
		if json.Unmarshal([]byte(raw), &v) == nil {
			out = append(out, v)
		}
	}
	return out, err
}

func (s *SQLiteOpsRepository) AcquireOpsLease(key, owner string, ttl time.Duration) (func(), error) {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(owner) == "" || ttl <= 0 {
		return nil, errors.New("租约参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	lease := OpsLease{Key: key, OwnerID: owner, CreatedAt: now, RenewedAt: now, ExpiresAt: now.Add(ttl)}
	var previous OpsLease
	var previousPayload string
	if err := s.db.QueryRow(`SELECT payload FROM leases WHERE lease_key=?`, key).Scan(&previousPayload); err == nil {
		if json.Unmarshal([]byte(previousPayload), &previous) == nil {
			lease.FencingToken = previous.FencingToken + 1
			if previous.OwnerID == owner && !previous.CreatedAt.IsZero() {
				lease.CreatedAt = previous.CreatedAt
			}
		}
	}
	if lease.FencingToken == 0 {
		lease.FencingToken = 1
	}
	b, err := json.Marshal(lease)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	// Update is conditional on expiry or ownership. This prevents a second
	// instance from overwriting a live lease between SELECT and INSERT.
	res, err := tx.Exec(`UPDATE leases SET payload=?, expires_at=?, owner_id=?, fencing_token=?, created_at=?, renewed_at=? WHERE lease_key=? AND (expires_at<=? OR owner_id=?)`, string(b), lease.ExpiresAt.Format(time.RFC3339Nano), lease.OwnerID, lease.FencingToken, formatSQLiteTime(lease.CreatedAt), formatSQLiteTime(lease.RenewedAt), key, now.Format(time.RFC3339Nano), owner)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		var existing string
		queryErr := tx.QueryRow(`SELECT payload FROM leases WHERE lease_key=?`, key).Scan(&existing)
		if queryErr == nil {
			_ = tx.Rollback()
			return nil, errors.New("运维执行租约已被其他实例持有")
		}
		if !errors.Is(queryErr, sql.ErrNoRows) {
			_ = tx.Rollback()
			return nil, queryErr
		}
		if _, err = tx.Exec(`INSERT INTO leases(lease_key,payload,expires_at,owner_id,fencing_token,created_at,renewed_at) VALUES(?,?,?,?,?,?,?)`, key, string(b), lease.ExpiresAt.Format(time.RFC3339Nano), lease.OwnerID, lease.FencingToken, formatSQLiteTime(lease.CreatedAt), formatSQLiteTime(lease.RenewedAt)); err != nil {
			_ = tx.Rollback()
			return nil, errors.New("运维执行租约竞争失败")
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return func() {
		_ = s.releaseSQLiteLease(key, owner)
	}, nil
}
func (s *SQLiteOpsRepository) RenewOpsLease(key, owner string, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("租约时长无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var cur OpsLease
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM leases WHERE lease_key=?`, key).Scan(&payload); err != nil || json.Unmarshal([]byte(payload), &cur) != nil || cur.OwnerID != owner {
		return errors.New("运维租约不存在或不属于当前实例")
	}
	now := time.Now()
	cur.ExpiresAt = now.Add(ttl)
	cur.RenewedAt = now
	b, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(`UPDATE leases SET payload=?, expires_at=?, renewed_at=? WHERE lease_key=? AND expires_at>? AND owner_id=?`, string(b), cur.ExpiresAt.Format(time.RFC3339Nano), formatSQLiteTime(cur.RenewedAt), key, now.Format(time.RFC3339Nano), owner)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			err = errors.New("运维租约已过期")
		}
	}
	return err
}

func (s *SQLiteOpsRepository) releaseSQLiteLease(key, owner string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM leases WHERE lease_key=? AND owner_id=?`, key, owner).Scan(&payload); err != nil {
		return err
	}
	var lease OpsLease
	if err := json.Unmarshal([]byte(payload), &lease); err != nil {
		return err
	}
	lease.ExpiresAt = time.Now().Add(-time.Second)
	lease.RenewedAt = time.Now()
	b, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE leases SET payload=?, expires_at=?, renewed_at=? WHERE lease_key=? AND owner_id=?`, string(b), lease.ExpiresAt.Format(time.RFC3339Nano), formatSQLiteTime(lease.RenewedAt), key, owner)
	return err
}

func (s *SQLiteOpsRepository) GetLease(key string) (OpsLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payload string
	if err := s.db.QueryRow(`SELECT payload FROM leases WHERE lease_key=?`, key).Scan(&payload); err != nil {
		return OpsLease{}, err
	}
	var lease OpsLease
	if err := json.Unmarshal([]byte(payload), &lease); err != nil {
		return OpsLease{}, err
	}
	return lease, nil
}

func (s *SQLiteOpsRepository) ListLeases() ([]OpsLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT payload FROM leases ORDER BY expires_at ASC, lease_key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpsLease
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var lease OpsLease
		if json.Unmarshal([]byte(payload), &lease) == nil {
			out = append(out, lease)
		}
	}
	return out, rows.Err()
}

var _ OpsRepository = (*SQLiteOpsRepository)(nil)
