package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivilegeStoreSignsPlansAndDetectsAuditTampering(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	data, _ := json.Marshal(privilegedPlan{Operation: "open-port", Params: map[string]string{"port": "17117", "protocol": "tcp"}, Fingerprint: "snapshot", Risk: "high", Impact: "network", Verification: []string{"check"}, CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"})
	stored, err := store.savePlan("demo", "apply", data, "root")
	if err != nil {
		t.Fatal(err)
	}
	var plan privilegedPlan
	if err := json.Unmarshal(stored, &plan); err != nil || plan.ID == "" {
		t.Fatalf("stored plan = %s, %v", stored, err)
	}
	consumed, err := store.consumePlan(plan.ID, "demo", "apply", "root")
	if err != nil || consumed.Operation != "open-port" {
		t.Fatalf("consumed = %#v, %v", consumed, err)
	}
	if err := store.appendAudit("demo", "apply", "open-port", consumed.Params, "done", "root"); err != nil {
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

func TestPrivilegeStoreKeepsV1AuditAndExcludesItFromRestore(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	params := []byte(`{"port":"17117"}`)
	created := "2025-01-01T00:00:00Z"
	mac := hmac.New(sha256.New, store.key)
	_, _ = mac.Write([]byte(strings.Join([]string{"", "open-port", string(params), "done", "root", created}, "\x00")))
	chain := hex.EncodeToString(mac.Sum(nil))
	if _, err := store.db.Exec(`INSERT INTO privilege_audit(plugin_id,action,signature_version,operation,params,output,account,created_at,previous_hash,chain_hash) VALUES('alemonx-network', 'legacy', 1, 'open-port', ?, 'done', 'root', ?, '', ?)`, params, created, chain); err != nil {
		t.Fatal(err)
	}
	if status := store.auditStatus("test"); !status.Valid {
		t.Fatalf("v1 audit invalid: %#v", status)
	}
	if _, err := store.latestAudit("alemonx-network"); err == nil {
		t.Fatal("legacy audit must not be restored")
	}
	items, err := store.auditItems("alemonx-network")
	if err != nil || len(items) != 1 || items[0]["legacy"] != true {
		t.Fatalf("legacy items = %#v, %v", items, err)
	}
}

func TestPrivilegeStoreBindsPlanToCreator(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	data, _ := json.Marshal(privilegedPlan{Operation: "open-port", Params: map[string]string{"port": "17117"}, Fingerprint: "snapshot", Risk: "high", Impact: "network", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"})
	stored, err := store.savePlan("demo", "apply", data, "alice")
	if err != nil {
		t.Fatal(err)
	}
	var plan privilegedPlan
	if err := json.Unmarshal(stored, &plan); err != nil {
		t.Fatal(err)
	}
	if _, err := store.consumePlan(plan.ID, "demo", "apply", "bob"); err == nil {
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

func TestPrivilegeIntentBindsAccountSourceOperationAndSingleUse(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	intent, err := store.createIntent("alemonx-qq", "napcat-install-dependencies", "", "root", "127.0.0.1", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.validateIntent(intent.ID, "alemonx-qq", "napcat-install-dependencies", "", "other", "127.0.0.1", "password"); err == nil {
		t.Fatal("another account must not use an authorization intent")
	}
	if _, err := store.validateIntent(intent.ID, "alemonx-qq", "napcat-install-dependencies", "", "root", "::1", "password"); err == nil {
		t.Fatal("another source must not use an authorization intent")
	}
	validated, err := store.validateIntent(intent.ID, "alemonx-qq", "napcat-install-dependencies", "", "root", "127.0.0.1", "password")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.consumeIntent(validated); err != nil {
		t.Fatal(err)
	}
	if _, err := store.validateIntent(intent.ID, "alemonx-qq", "napcat-install-dependencies", "", "root", "127.0.0.1", "password"); err == nil {
		t.Fatal("consumed authorization intent must not be reusable")
	}
}

func TestPrivilegeIntentExpires(t *testing.T) {
	store, err := newPrivilegeStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	intent, err := store.createIntent("alemonx-qq", "napcat-install-dependencies", "", "root", "127.0.0.1", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE privilege_intents SET expires_at=? WHERE id=?`, "2000-01-01T00:00:00Z", intent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.validateIntent(intent.ID, "alemonx-qq", "napcat-install-dependencies", "", "root", "127.0.0.1", "password"); err == nil {
		t.Fatal("expired intent must be rejected")
	}
}
