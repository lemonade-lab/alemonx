package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenOpsRepository returns the ops backend for the given storage mode.
// Modes "json" and "file" force the portable JSON store; anything else
// (default or "sqlite") uses the SQLite repository and migrates existing JSON
// records once. A nil store with a non-nil error means the caller should keep
// its JSON fallback.
func OpenOpsRepository(opsDir, databasePath, mode string) (OpsRepository, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "json", "file":
		return NewOpsStoreAt(opsDir), nil
	}
	if strings.TrimSpace(databasePath) == "" {
		databasePath = filepath.Join(filepath.Dir(opsDir), "ops.db")
	}
	if _, statErr := os.Stat(databasePath); os.IsNotExist(statErr) {
		// A missing JSON directory means a fresh install: skip migration and
		// let the SQLite repository create its own empty database.
		if _, dirErr := os.Stat(opsDir); dirErr == nil {
			if migrateErr := MigrateOpsJSONToSQLite(opsDir, databasePath, ""); migrateErr != nil {
				return nil, fmt.Errorf("迁移 JSON 记录到 SQLite 失败：%w", migrateErr)
			}
		} else if !os.IsNotExist(dirErr) {
			return nil, fmt.Errorf("无法检查运维数据目录：%w", dirErr)
		}
	} else if statErr != nil {
		return nil, fmt.Errorf("无法检查 SQLite 数据库：%w", statErr)
	}
	store, err := NewSQLiteOpsRepository(databasePath)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite 运维存储失败：%w", err)
	}
	return store, nil
}

// MigrateOpsJSONToSQLite migrates the portable JSON store into the pure-Go
// SQLite repository. The source remains untouched and is backed up before any
// records are written to the destination.
func MigrateOpsJSONToSQLite(sourceDir, databasePath, backupDir string) error {
	if strings.TrimSpace(sourceDir) == "" || strings.TrimSpace(databasePath) == "" {
		return errors.New("迁移路径不能为空")
	}
	entries := make([]string, 0)
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "backups") {
			return nil
		}
		entries = append(entries, path)
		return nil
	})
	if err != nil {
		return err
	}
	if backupDir == "" {
		backupDir = filepath.Join(sourceDir, "backups")
	}
	backup := filepath.Join(backupDir, time.Now().UTC().Format("20060102-150405.000000000"))
	for _, path := range entries {
		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(backup, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return err
		}
	}

	repo, err := NewSQLiteOpsRepository(databasePath)
	if err != nil {
		return err
	}
	defer repo.Close()
	source := NewOpsStoreAt(sourceDir)
	incidents, err := source.ListIncidents()
	if err != nil {
		return err
	}
	for _, value := range incidents {
		if err := repo.SaveIncident(value); err != nil {
			return err
		}
	}
	todos, err := source.ListTodos()
	if err != nil {
		return err
	}
	for _, value := range todos {
		if err := repo.SaveTodo(value); err != nil {
			return err
		}
	}
	runs, err := source.ListMaintenance()
	if err != nil {
		return err
	}
	for _, value := range runs {
		if err := repo.SaveMaintenance(value); err != nil {
			return err
		}
	}
	policies, err := source.ListPolicies()
	if err != nil {
		return err
	}
	for _, value := range policies {
		if err := repo.SavePolicy(value); err != nil {
			return err
		}
	}
	alerts, err := source.ListAlerts()
	if err != nil {
		return err
	}
	for _, value := range alerts {
		if err := repo.SaveAlert(value); err != nil {
			return err
		}
	}
	roles, err := source.ListRoles()
	if err != nil {
		return err
	}
	for _, value := range roles {
		if err := repo.SaveRole(value); err != nil {
			return err
		}
	}
	audits, err := source.ListAudit()
	if err != nil {
		return err
	}
	for _, value := range audits {
		if err := repo.AppendAudit(value); err != nil {
			return err
		}
	}
	signals, err := source.ListSignals()
	if err != nil {
		return err
	}
	for _, value := range signals {
		if err := repo.AppendSignal(value); err != nil {
			return err
		}
	}
	// Import persisted event samples and deduplication keys without depending
	// on the JSON layout beyond the public store API.
	for _, incident := range incidents {
		events, err := source.ListEvents(incident.ID)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := repo.AppendEvent(incident.ID, event); err != nil {
				return err
			}
		}
	}
	seenPath := filepath.Join(sourceDir, "seen-events.json")
	var seen map[string]time.Time
	if err := readJSONFile(seenPath, &seen); err == nil {
		for key := range seen {
			if _, err := repo.MarkEventSeen(key); err != nil {
				return err
			}
		}
	}
	return nil
}
