package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
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
	return repo, nil
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
	var table, keyColumn string
	switch kind {
	case "incident":
		table, keyColumn = "incidents", "id"
	case "todo":
		table, keyColumn = "todos", "id"
	case "maintenance":
		table, keyColumn = "maintenance_runs", "id"
	case "policy":
		table, keyColumn = "policies", "project_root"
	case "alert":
		table, keyColumn = "alerts", "id"
	}
	if table != "" {
		if kind == "incident" {
			var incident Incident
			if json.Unmarshal(payload, &incident) != nil {
				_ = tx.Rollback()
				return errors.New("事件数据序列化失败")
			}
			_, err = tx.Exec("INSERT OR REPLACE INTO incidents(id,fingerprint,payload,updated) VALUES(?,?,?,?)", id, incident.Fingerprint, string(payload), updated)
		} else {
			_, err = tx.Exec("INSERT OR REPLACE INTO "+table+"("+keyColumn+",payload,updated) VALUES(?,?,?)", id, string(payload), updated)
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
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
	res, err := tx.Exec(`DELETE FROM records WHERE kind='incident' AND id=?`, id)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
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
	res, err := s.db.Exec(`DELETE FROM records WHERE kind='todo' AND id=?`, id)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return errors.New("待办不存在")
	}
	return nil
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
		_, err = tx.Exec(`INSERT OR REPLACE INTO audit_entries(id,payload,created) VALUES(?,?,?)`, id, payload, updated)
	} else if strings.HasPrefix(kind, "event-") {
		_, err = tx.Exec(`INSERT OR REPLACE INTO ops_events(id,incident_id,payload,created) VALUES(?,?,?,?)`, id, strings.TrimPrefix(kind, "event-"), payload, updated)
	} else if kind == "cursor" {
		_, err = tx.Exec(`INSERT OR REPLACE INTO log_cursors(cursor_key,payload,updated) VALUES(?,?,?)`, id, payload, updated)
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
	runs, err := s.ListMaintenance()
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.TaskID == taskID {
			run.Status, run.Error = status, lastError
			return s.SaveMaintenance(run)
		}
	}
	return nil
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
	res, err := tx.Exec(`UPDATE leases SET payload=?, expires_at=? WHERE lease_key=? AND (expires_at<=? OR json_extract(payload,'$.ownerId')=?)`, string(b), lease.ExpiresAt.Format(time.RFC3339Nano), key, now.Format(time.RFC3339Nano), owner)
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
		if _, err = tx.Exec(`INSERT INTO leases(lease_key,payload,expires_at) VALUES(?,?,?)`, key, string(b), lease.ExpiresAt.Format(time.RFC3339Nano)); err != nil {
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
	result, err := s.db.Exec(`UPDATE leases SET payload=?, expires_at=? WHERE lease_key=? AND expires_at>? AND json_extract(payload,'$.ownerId')=?`, string(b), cur.ExpiresAt.Format(time.RFC3339Nano), key, now.Format(time.RFC3339Nano), owner)
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
	if err := s.db.QueryRow(`SELECT payload FROM leases WHERE lease_key=? AND json_extract(payload,'$.ownerId')=?`, key, owner).Scan(&payload); err != nil {
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
	_, err = s.db.Exec(`UPDATE leases SET payload=?, expires_at=? WHERE lease_key=? AND json_extract(payload,'$.ownerId')=?`, string(b), lease.ExpiresAt.Format(time.RFC3339Nano), key, owner)
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
