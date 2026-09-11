package web

import (
	"testing"

	"alemonx/internal/robot"
)

func TestSafeApprovalSummaryNeverIncludesToolArguments(t *testing.T) {
	if got := safeApprovalSummary("write"); got != "请求修改机器人项目文件" {
		t.Fatalf("write 摘要 = %q", got)
	}
	if got := safeApprovalSummary("bash"); got != "请求执行受控项目命令" {
		t.Fatalf("bash 摘要 = %q", got)
	}
	if got := safeApprovalSummary("custom-tool-with-secret-argument"); got != "请求执行一次受限工具操作" {
		t.Fatalf("未知工具摘要 = %q", got)
	}
}

func TestSafePM2StatusSummaryDoesNotExposeProcessDetails(t *testing.T) {
	if got := safePM2StatusSummary(robot.PM2Status{}); got != "当前项目尚未配置 PM2。" {
		t.Fatalf("未配置摘要 = %q", got)
	}
	if got := safePM2StatusSummary(robot.PM2Status{Configured: true, Running: true, Status: "online"}); got != "当前项目 PM2 进程正在运行。" {
		t.Fatalf("运行摘要泄露了进程详情：%q", got)
	}
}
