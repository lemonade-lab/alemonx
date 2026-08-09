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
