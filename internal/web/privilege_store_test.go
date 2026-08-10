package web

import (
	"encoding/json"
	"testing"
)

func TestPrivilegeStoreSignsPlansAndDetectsAuditTampering(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	data, _ := json.Marshal(networkPlan{Operation: "open-port", Params: map[string]string{"port": "17117", "protocol": "tcp"}, Fingerprint: "snapshot", Risk: "high", Impact: "network", Verification: []string{"check"}, CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"})
	stored, err := store.saveNetworkPlan(data, "root")
	if err != nil {
		t.Fatal(err)
	}
	var plan networkPlan
	if err := json.Unmarshal(stored, &plan); err != nil || plan.ID == "" {
		t.Fatalf("stored plan = %s, %v", stored, err)
	}
	consumed, err := store.consumeNetworkPlan(plan.ID, "root")
	if err != nil || consumed.Operation != "open-port" {
		t.Fatalf("consumed = %#v, %v", consumed, err)
	}
	if err := store.appendAudit("open-port", consumed.Params, "done", "root"); err != nil {
		t.Fatal(err)
	}
	if status := store.auditStatus("test"); !status.Valid {
		t.Fatalf("audit invalid: %#v", status)
	}
	if _, err := store.db.Exec(`UPDATE privilege_audit SET output='tampered'`); err != nil {
		t.Fatal(err)
	}
	if status := store.auditStatus("test"); status.Valid {
		t.Fatal("tampered audit must fail validation")
	}
}

func TestPrivilegeStoreBindsPlanToCreator(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	data, _ := json.Marshal(networkPlan{Operation: "open-port", Params: map[string]string{"port": "17117"}, Fingerprint: "snapshot", Risk: "high", Impact: "network", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"})
	stored, err := store.saveNetworkPlan(data, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var plan networkPlan
	if err := json.Unmarshal(stored, &plan); err != nil {
		t.Fatal(err)
	}
	if _, err := store.consumeNetworkPlan(plan.ID, "bob"); err == nil {
		t.Fatal("another account must not consume the plan")
	}
}

func TestPrivilegeStorePersistsSudoRateLimit(t *testing.T) {
	directory := t.TempDir()
	store, err := newPrivilegeStoreAt(directory)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := store.recordSudoFailure("root\x00127.0.0.1\x00napcat"); err != nil {
			t.Fatal(err)
		}
	}
	store.close()
	reopened, err := newPrivilegeStoreAt(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if err := reopened.checkSudoAttempt("root\x00127.0.0.1\x00napcat"); err == nil {
		t.Fatal("sudo rate limit must survive restart")
	}
}
