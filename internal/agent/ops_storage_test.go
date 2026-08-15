package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenOpsRepositoryDefaultsToSQLiteOnFreshInstall(t *testing.T) {
	dataDir := t.TempDir()
	repo, err := OpenOpsRepository(filepath.Join(dataDir, "incidents"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, ok := repo.(*SQLiteOpsRepository); !ok {
		t.Fatalf("default backend = %T, want *SQLiteOpsRepository", repo)
	}
	if err := repo.SaveIncident(Incident{ID: "fresh", ProjectRoot: "/p", Status: IncidentDetected, Updated: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ops.db")); err != nil {
		t.Fatalf("ops.db missing: %v", err)
	}
}

func TestOpenOpsRepositoryJSONModeForcesJSONStore(t *testing.T) {
	opsDir := filepath.Join(t.TempDir(), "incidents")
	for _, mode := range []string{"json", "file"} {
		repo, err := OpenOpsRepository(opsDir, "", mode)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := repo.(*OpsStore); !ok {
			t.Fatalf("%s backend = %T, want *OpsStore", mode, repo)
		}
	}
}

func TestOpenOpsRepositoryMigratesExistingJSON(t *testing.T) {
	dataDir := t.TempDir()
	opsDir := filepath.Join(dataDir, "incidents")
	jsonStore := NewOpsStoreAt(opsDir)
	incident := Incident{ID: "legacy-1", ProjectRoot: "/p", Sample: "boom", Status: IncidentDetected, Updated: time.Now()}
	if err := jsonStore.SaveIncident(incident); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(dataDir, "ops.db")
	repo, err := OpenOpsRepository(opsDir, databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, ok := repo.(*SQLiteOpsRepository); !ok {
		t.Fatalf("migrated backend = %T", repo)
	}
	list, err := repo.ListIncidents()
	if err != nil || len(list) != 1 || list[0].ID != "legacy-1" {
		t.Fatalf("migrated incidents = %#v, %v", list, err)
	}
	backups, err := os.ReadDir(filepath.Join(opsDir, "backups"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	// Re-opening an existing database must not re-run the migration.
	repo2, err := OpenOpsRepository(opsDir, databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer repo2.Close()
	backups2, err := os.ReadDir(filepath.Join(opsDir, "backups"))
	if err != nil || len(backups2) != 1 {
		t.Fatalf("second open created another backup: %v, %v", backups2, err)
	}
}

func TestOpenOpsRepositoryReturnsErrorWhenSQLiteUnusable(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "ops.db")
	if err := os.MkdirAll(databasePath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOpsRepository(filepath.Join(dataDir, "incidents"), databasePath, ""); err == nil {
		t.Fatal("opening a directory as sqlite must fail")
	}
}
