package web

import "testing"

func TestDSHApprovalIsScopedAndSingleUse(t *testing.T) {
	manager := newDSHApprovalManager()
	approval, decisions, cleanup, err := manager.register("/robots/a", "session-1", "file.write", "更新项目文件")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if approval.ID == "" || approval.SessionID != "session-1" {
		t.Fatalf("审批预览 = %#v", approval)
	}
	if manager.resolve("/robots/b", approval.ID, true) {
		t.Fatal("不同根目录不得批准审批")
	}
	if !manager.resolve("/robots/a", approval.ID, true) {
		t.Fatal("当前根目录应能批准审批")
	}
	if decision := <-decisions; !decision.approved {
		t.Fatal("批准结果丢失")
	}
	if manager.resolve("/robots/a", approval.ID, true) {
		t.Fatal("审批 ID 必须只能使用一次")
	}
}

func TestDSHApprovalShutdownRejectsPending(t *testing.T) {
	manager := newDSHApprovalManager()
	_, decisions, cleanup, err := manager.register("/robots/a", "session-1", "pm2.restart", "重启进程")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	manager.cancelAll()
	if decision := <-decisions; decision.approved {
		t.Fatal("关闭时必须拒绝所有未决审批")
	}
}
