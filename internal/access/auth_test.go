package access

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnableLoginAndDisable(t *testing.T) {
	manager, err := NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Enable("lemonade", "secret", "different"); err == nil {
		t.Fatal("confirmation mismatch must fail")
	}
	token, err := manager.Enable("lemonade", "secret", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if status, err := manager.Status(token); err != nil || !status.Enabled || !status.Authenticated || status.Account != "lemonade" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	if _, err := manager.Login("lemonade", "wrong"); err == nil {
		t.Fatal("wrong password must fail")
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if status, err := manager.Status(""); err != nil || status.Enabled {
		t.Fatalf("disabled status = %#v, %v", status, err)
	}
}

func TestResetSuperAdminRevokesSessionsAndOtherAdministrators(t *testing.T) {
	manager, err := NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldToken, err := manager.Enable("root", "old-password", "old-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateAccount("other", "password", "password"); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.data.Accounts[1].SuperAdmin = true
	if err := manager.persist(); err != nil {
		manager.mu.Unlock()
		t.Fatal(err)
	}
	manager.mu.Unlock()
	otherToken, err := manager.Login("other", "password")
	if err != nil {
		t.Fatal(err)
	}
	newToken, err := manager.ResetSuperAdmin("recovery", "new-password", "new-password")
	if err != nil {
		t.Fatal(err)
	}
	if manager.Authenticate(oldToken) || manager.Authenticate(otherToken) {
		t.Fatal("old sessions must be invalid after emergency reset")
	}
	if status, err := manager.Status(newToken); err != nil || !status.SuperAdmin || status.Account != "recovery" {
		t.Fatalf("recovery status = %#v, %v", status, err)
	}
	if _, err := manager.Login("root", "old-password"); err == nil {
		t.Fatal("old administrator password must no longer work")
	}
	if _, err := manager.Login("other", "password"); err == nil {
		t.Fatal("other super administrator must be disabled")
	}
	if _, err := manager.Login("recovery", "new-password"); err != nil {
		t.Fatalf("new recovery administrator cannot log in: %v", err)
	}
}

func TestLegacyRolesDoNotAffectAccountSession(t *testing.T) {
	manager, err := NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Enable("root", "secret", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateAccount("alice", "password", "password"); err != nil {
		t.Fatal(err)
	}
	token, err := manager.Login("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	before, err := manager.Status(token)
	if err != nil {
		t.Fatal(err)
	}
	// Add fields persisted by older versions without changing account identity.
	raw, err := os.ReadFile(manager.Path())
	if err != nil {
		t.Fatal(err)
	}
	legacy := bytes.Replace(raw, []byte(`"account": "alice"`), []byte(`"account": "alice", "roles": ["operator"]`), 1)
	legacy = bytes.Replace(legacy, []byte(`"accounts":`), []byte(`"roles": [{"id": "operator", "name": "Operator", "permissions": ["system.manage"]}], "accounts":`), 1)
	if err := os.WriteFile(manager.Path(), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := manager.Status(token)
	if err != nil || !after.Authenticated || after.SuperAdmin || after != before {
		t.Fatalf("legacy account status = %#v, %v", after, err)
	}
}

func TestGitHubBindingLifecycle(t *testing.T) {
	manager, err := NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := manager.Enable("root", "secret", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateAccount("reader", "password", "password"); err != nil {
		t.Fatal(err)
	}
	reader, err := manager.Login("reader", "password")
	if err != nil {
		t.Fatal(err)
	}
	wrong, correct := "wrong", "secret"
	if _, err := manager.GitHubBindingKey(root, &wrong); err == nil {
		t.Fatal("binding accepted the wrong workbench password")
	}
	rootKey, err := manager.GitHubBindingKey(root, &correct)
	if err != nil {
		t.Fatal(err)
	}
	readerKey, err := manager.GitHubBindingKey(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.LoginWithGitHub(123); err == nil {
		t.Fatal("unbound GitHub identity logged in")
	}
	if err := manager.SetGitHubBinding(root, rootKey, 123, "original-name"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetGitHubBinding(reader, readerKey, 123, "original-name"); err == nil {
		t.Fatal("duplicate GitHub binding succeeded")
	}
	session, account, err := manager.LoginWithGitHub(123)
	if err != nil || account != "root" || !manager.Authenticate(session) {
		t.Fatalf("GitHub session for %q failed: %v", account, err)
	}
	if _, err := manager.ResetSuperAdmin("root", "new-secret", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.LoginWithGitHub(123); err == nil || manager.Authenticate(session) {
		t.Fatal("administrator recovery must invalidate the GitHub binding and old sessions")
	}
}
