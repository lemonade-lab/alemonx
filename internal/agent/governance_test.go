package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteV2MigrationBackfillsCoreColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ops.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	incident := Incident{ID: "legacy", ProjectRoot: "/project", ProcessName: "bot", Fingerprint: "fp", Status: IncidentTriaged, Severity: "high", Occurrences: 2, FirstSeen: time.Now().Add(-time.Minute), LastSeen: time.Now(), Updated: time.Now()}
	payload, _ := json.Marshal(incident)
	_, err = db.Exec(`CREATE TABLE schema_meta(version INTEGER NOT NULL); INSERT INTO schema_meta(version) VALUES(1); CREATE TABLE incidents(id TEXT PRIMARY KEY, fingerprint TEXT, payload TEXT NOT NULL, updated TEXT NOT NULL); INSERT INTO incidents(id,fingerprint,payload,updated) VALUES(?,?,?,?)`, incident.ID, "", string(payload), time.Now().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := NewSQLiteOpsRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	var version, occurrences int
	var root, status string
	if err := repo.db.QueryRow(`SELECT (SELECT version FROM schema_meta LIMIT 1), project_root, status, occurrences FROM incidents WHERE id=?`, incident.ID).Scan(&version, &root, &status, &occurrences); err != nil {
		t.Fatal(err)
	}
	if version != sqliteOpsSchemaVersion || root != "/project" || status != string(IncidentTriaged) || occurrences != 2 {
		t.Fatalf("migration backfill version=%d root=%q status=%q occurrences=%d", version, root, status, occurrences)
	}
}

func TestSQLiteV2PreventsDuplicateActiveIncidentFingerprint(t *testing.T) {
	repo, err := NewSQLiteOpsRepository(filepath.Join(t.TempDir(), "ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	first := Incident{ID: "one", ProjectRoot: "/project", ProcessName: "bot", Fingerprint: "same", Status: IncidentDetected, Updated: time.Now()}
	if err := repo.SaveIncident(first); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveIncident(Incident{ID: "two", ProjectRoot: "/project", ProcessName: "bot", Fingerprint: "same", Status: IncidentDetected, Updated: time.Now()}); err == nil {
		t.Fatal("duplicate active fingerprint must be rejected")
	}
	first.Status = IncidentResolved
	if err := repo.SaveIncident(first); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveIncident(Incident{ID: "two", ProjectRoot: "/project", ProcessName: "bot", Fingerprint: "same", Status: IncidentDetected, Updated: time.Now()}); err != nil {
		t.Fatalf("resolved fingerprint may create a new incident: %v", err)
	}
}

func TestOpsLeaseRejectsSecondOwnerAndReleases(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	release, err := store.AcquireOpsLease("monitor", "one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireOpsLease("monitor", "two", time.Minute); err == nil {
		t.Fatal("second owner should be rejected")
	}
	release()
	if next, err := store.AcquireOpsLease("monitor", "two", time.Minute); err != nil {
		t.Fatal(err)
	} else {
		next()
	}
}

func TestMigrateJSONToSQLiteKeepsSource(t *testing.T) {
	source, db := t.TempDir(), filepath.Join(t.TempDir(), "ops.db")
	store := NewOpsStoreAt(source)
	if err := store.SaveIncident(Incident{ID: "i1", ProjectRoot: "/tmp/project", Status: IncidentDetected, Updated: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateOpsJSONToSQLite(source, db, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.incidentPath("i1")); err != nil {
		t.Fatalf("source should remain: %v", err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database missing: %v", err)
	}
}

func TestSQLiteOpsRepositoryRoundTrip(t *testing.T) {
	repo, err := NewSQLiteOpsRepository(filepath.Join(t.TempDir(), "ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	incident := Incident{ID: "sqlite-1", ProjectRoot: "/p", Status: IncidentDetected, Updated: time.Now()}
	if err := repo.SaveIncident(incident); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE id=?`, incident.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("业务表未写入 count=%d err=%v", count, err)
	}
	loaded, err := repo.GetIncident(incident.ID)
	if err != nil || loaded.ID != incident.ID {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if err := repo.SaveLogCursor(LogCursor{ProjectRoot: "/p", ProcessName: "web", Offset: 12, WindowHash: "h"}); err != nil {
		t.Fatal(err)
	}
	cursor, err := repo.GetLogCursor("/p", "web")
	if err != nil || cursor.Offset != 12 || cursor.WindowHash != "h" {
		t.Fatalf("cursor=%+v err=%v", cursor, err)
	}
	release, err := repo.AcquireOpsLease("worker", "one", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AcquireOpsLease("worker", "two", time.Minute); err == nil {
		t.Fatal("sqlite lease should reject a second owner")
	}
	if err := repo.RenewOpsLease("worker", "one", time.Minute); err != nil {
		t.Fatal(err)
	}
	lease, err := repo.GetLease("worker")
	if err != nil || lease.FencingToken == 0 || lease.OwnerID != "one" {
		t.Fatalf("lease metadata = %+v err=%v", lease, err)
	}
	release()
	if _, err := repo.AcquireOpsLease("worker", "two", time.Minute); err != nil {
		t.Fatal(err)
	}
	lease, err = repo.GetLease("worker")
	if err != nil || lease.FencingToken < 2 {
		t.Fatalf("fencing token did not advance: %+v err=%v", lease, err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryLeaseManagerHonorsContextAndOwnership(t *testing.T) {
	repo := NewOpsStoreAt(t.TempDir())
	manager := NewLeaseManager(repo)
	if err := manager.Acquire(context.Background(), "worker", "one", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := manager.Acquire(context.Background(), "worker", "two", time.Minute); err == nil {
		t.Fatal("second owner should be rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Renew(ctx, "worker", "one", time.Minute); err == nil {
		t.Fatal("cancelled context should stop renewal")
	}
	if err := manager.Release(context.Background(), "worker", "one"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Acquire(context.Background(), "worker", "two", time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestMetricsRepositoryAtomicSnapshot(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	for i := 0; i < 3; i++ {
		if err := store.Increment("incident_total", "/p", "fp", 1); err != nil {
			t.Fatal(err)
		}
	}
	metrics, err := store.Snapshot("/p")
	if err != nil || metrics.Incidents != 3 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
	points, err := store.Query("/p", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil || len(points) == 0 {
		t.Fatalf("metric query=%+v err=%v", points, err)
	}
	repo, err := NewSQLiteOpsRepository(filepath.Join(t.TempDir(), "ops.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Increment("incident_total", "/p", "fp", 2); err != nil {
		t.Fatal(err)
	}
	metrics, err = repo.Snapshot("/p")
	if err != nil || metrics.Incidents != 2 {
		t.Fatalf("sqlite metrics=%+v err=%v", metrics, err)
	}
	points, err = repo.Query("/p", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil || len(points) == 0 {
		t.Fatalf("sqlite metric query=%+v err=%v", points, err)
	}
}

func TestMaintenanceTaskTransitionRepositoryContract(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) OpsRepository
	}{
		{name: "json", new: func(t *testing.T) OpsRepository { return NewOpsStoreAt(t.TempDir()) }},
		{name: "sqlite", new: func(t *testing.T) OpsRepository {
			repo, err := NewSQLiteOpsRepository(filepath.Join(t.TempDir(), "ops.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repo.Close() })
			return repo
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			now := time.Now()
			incident := Incident{ID: "inc-1", ProjectRoot: "/p", ProcessName: "bot", Status: IncidentFixing, Updated: now}
			run := MaintenanceRun{ID: "run-1", IncidentID: incident.ID, TaskID: "task-1", Status: "fixing", Created: now}
			if err := repo.SaveIncident(incident); err != nil {
				t.Fatal(err)
			}
			if err := repo.SaveMaintenance(run); err != nil {
				t.Fatal(err)
			}
			if err := repo.SavePolicy(OpsPolicy{ProjectRoot: "/p", ObservationMinutes: 1}); err != nil {
				t.Fatal(err)
			}
			if err := repo.TransitionMaintenanceForTask("task-1", "completed", ""); err != nil {
				t.Fatal(err)
			}
			gotRun, err := repo.GetMaintenance("run-1")
			if err != nil || gotRun.Status != "observing" || gotRun.ObservationUntil.IsZero() {
				t.Fatalf("run=%+v err=%v", gotRun, err)
			}
			gotIncident, err := repo.GetIncident("inc-1")
			if err != nil || gotIncident.Status != IncidentObserving {
				t.Fatalf("incident=%+v err=%v", gotIncident, err)
			}
			if err := repo.TransitionMaintenanceForTask("task-1", "failed", "verify failed"); err != nil {
				t.Fatal(err)
			}
			gotRun, _ = repo.GetMaintenance("run-1")
			gotIncident, _ = repo.GetIncident("inc-1")
			if gotRun.Status != "failed" || gotRun.Finished == nil || gotIncident.Status != IncidentTodo {
				t.Fatalf("terminal projection run=%+v incident=%+v", gotRun, gotIncident)
			}
			if _, err := repo.GetTodo("todo-inc-1"); err != nil {
				t.Fatalf("todo should be created: %v", err)
			}
		})
	}
}

func TestAuditAndAlertPersistence(t *testing.T) {
	store := NewOpsStoreAt(t.TempDir())
	if err := store.AppendAudit(AuditEntry{Actor: "alice", Role: "approver", Action: "incident.approve", Result: "accepted"}); err != nil {
		t.Fatal(err)
	}
	audits, err := store.ListAudit()
	if err != nil || len(audits) != 1 || audits[0].Actor != "alice" {
		t.Fatalf("audits = %#v, err=%v", audits, err)
	}
	if err := store.SaveAlert(AlertRecord{Alert: Alert{ID: "a1", Severity: "high", Message: "boom"}, Status: "open"}); err != nil {
		t.Fatal(err)
	}
	alerts, err := store.ListAlerts()
	if err != nil || len(alerts) != 1 || alerts[0].ID != "a1" {
		t.Fatalf("alerts = %#v, err=%v", alerts, err)
	}
}
