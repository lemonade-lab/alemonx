package access

import (
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
	if _, err := manager.CreateAccount("other", "password", "password", nil); err != nil {
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

func TestAccountsRolesAndPermissions(t *testing.T) {
	manager, err := NewAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	superToken, err := manager.Enable("root", "secret", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if status, err := manager.Status(superToken); err != nil || !status.SuperAdmin || !manager.Authorize(superToken, "system.manage") {
		t.Fatalf("super administrator status = %#v, %v", status, err)
	}
	if _, err := manager.SaveRole(Role{ID: "reader", Name: "只读", Permissions: []string{"workbench.view"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveRole(Role{ID: "operator", Name: "操作员", Permissions: []string{"workbench.manage", "operations.view"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateAccount("alice", "password", "password", []string{"reader"}); err != nil {
		t.Fatal(err)
	}
	aliceToken, err := manager.Login("alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Authorize(aliceToken, "workbench.view") || manager.Authorize(aliceToken, "workbench.manage") || manager.Authorize(aliceToken, "system.manage") {
		t.Fatal("reader permissions were not applied exactly")
	}
	if _, err := manager.UpdateAccount("alice", []string{"reader", "operator"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !manager.Authorize(aliceToken, "workbench.manage") || !manager.Authorize(aliceToken, "operations.view") {
		t.Fatal("multiple roles should combine their permissions")
	}
	if err := manager.DeleteRole("operator"); err != nil {
		t.Fatal(err)
	}
	if manager.Authorize(aliceToken, "workbench.manage") {
		t.Fatal("deleting a role should revoke its permission from bound accounts")
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
	if _, err := manager.CreateAccount("reader", "password", "password", nil); err != nil {
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
	if err != nil || account != "root" || !manager.Authorize(session, "system.manage") {
		t.Fatalf("GitHub session for %q failed: %v", account, err)
	}
	if _, err := manager.ResetSuperAdmin("root", "new-secret", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.LoginWithGitHub(123); err == nil || manager.Authenticate(session) {
		t.Fatal("administrator recovery must invalidate the GitHub binding and old sessions")
	}
}
