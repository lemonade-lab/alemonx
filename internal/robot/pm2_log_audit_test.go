package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPM2AuditLine(at, text string) pm2AuditLogLine {
	timestamp, err := time.ParseInLocation("2006-01-02 15:04:05", at, time.Local)
	if err != nil {
		panic(err)
	}
	return pm2AuditLogLine{Text: "[" + at + "]" + text, Timestamp: timestamp, HasTime: true, Source: "out"}
}

func TestDeletePM2LogsDeletesOnlySelectedDateAndSource(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	outPath := filepath.Join(root, "pm2-out.log")
	errPath := filepath.Join(root, "pm2-error.log")
	writeAppPageFixture(t, outPath, strings.Join([]string{
		"[2026-08-17 10:00:00][INFO] remove this output",
		"[2026-08-18 10:00:00][INFO] keep this output",
		"unstructured output stays",
	}, "\n")+"\n")
	writeAppPageFixture(t, errPath, "[2026-08-17 10:00:00][ERROR] keep this error\n")

	pm2LogPathCache.Lock()
	previous, hadPrevious := pm2LogPathCache.entries[root]
	pm2LogPathCache.entries[root] = pm2LogPathCacheEntry{
		files: []pm2LogFile{
			{Source: "out", Path: outPath},
			{Source: "err", Path: errPath},
		},
		fetchedAt: time.Now(),
	}
	pm2LogPathCache.Unlock()
	t.Cleanup(func() {
		pm2LogPathCache.Lock()
		defer pm2LogPathCache.Unlock()
		if hadPrevious {
			pm2LogPathCache.entries[root] = previous
		} else {
			delete(pm2LogPathCache.entries, root)
		}
	})

	result, err := (Manager{}).DeletePM2Logs(root, PM2AuditQuery{
		Date: "2026-08-17", Source: "out",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || result.Files != 1 {
		t.Fatalf("delete result = %#v", result)
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "remove this output") ||
		!strings.Contains(string(out), "keep this output") ||
		!strings.Contains(string(out), "unstructured output stays") {
		t.Fatalf("unexpected output log after deletion: %q", out)
	}
	errLog, err := os.ReadFile(errPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errLog), "keep this error") {
		t.Fatalf("error log was modified: %q", errLog)
	}
}

func diagnosticCodes(items []PM2LogDiagnostic) map[string]PM2LogDiagnostic {
	result := make(map[string]PM2LogDiagnostic, len(items))
	for _, item := range items {
		result[item.Code] = item
	}
	return result
}

func TestDiagnosePM2LogLinesUsesLatestWorkerLifetime(t *testing.T) {
	lines := []pm2AuditLogLine{
		testPM2AuditLine("2026-08-17 16:12:29", "[ERROR] { code: 4000, message: '请求失败', data: null }"),
		testPM2AuditLine("2026-08-17 16:08:03", "[WARN] [bh3] Cannot find module '/plugins/crystelf-plugin/lib/system/puppeteerRenderer.js' / Navigation timeout of 30000 ms exceeded"),
		testPM2AuditLine("2026-08-17 16:04:50", "[WARN] 缺失方法: makeForwardMsg().__forwardNodes.toJSON"),
		testPM2AuditLine("2026-08-17 16:04:48", `[INFO] {"token":"secret-token","email":"person@example.com"}`),
		testPM2AuditLine("2026-08-17 16:04:42", "[ERROR] { code: 2100, message: 'Delete not supported or failed', data: null }"),
		testPM2AuditLine("2026-08-17 16:04:12", "[WARN] moment().add(period, number) is deprecated"),
		testPM2AuditLine("2026-08-17 16:02:50", "[INFO] [Yunzai] Worker 启动, cwd=/bot"),
		testPM2AuditLine("2026-08-17 15:55:40", "[WARN] 缺失方法: old.__forwardNodes.toJSON"),
	}

	diagnostics := diagnosePM2LogLines(lines)
	codes := diagnosticCodes(diagnostics)
	for _, want := range []string{
		"yunzai-forward-tojson",
		"onebot-delete-failed",
		"plugin-module-missing",
		"puppeteer-navigation-timeout",
		"moment-inverted-add",
		"onebot-request-failed",
		"sensitive-login-data",
	} {
		if _, ok := codes[want]; !ok {
			t.Fatalf("missing diagnostic %q in %#v", want, diagnostics)
		}
	}
	if got := codes["yunzai-forward-tojson"].Count; got != 1 {
		t.Fatalf("latest forward diagnostic count = %d, want 1", got)
	}
	if got := codes["onebot-request-failed"].LastSeen; got != "2026-08-17 16:12:29" {
		t.Fatalf("last seen = %q", got)
	}
	if got := codes["plugin-module-missing"].Summary; !strings.Contains(got, "/plugins/crystelf-plugin/lib/system/puppeteerRenderer.js") {
		t.Fatalf("missing module summary = %q", got)
	}
	seenWarning := false
	seenInfo := false
	for _, item := range diagnostics {
		switch item.Severity {
		case "error":
			if seenWarning || seenInfo {
				t.Fatalf("error diagnostic sorted after lower severity: %#v", diagnostics)
			}
		case "warning":
			seenWarning = true
			if seenInfo {
				t.Fatalf("warning diagnostic sorted after info: %#v", diagnostics)
			}
		case "info":
			seenInfo = true
		}
	}
}

func TestRedactPM2LogText(t *testing.T) {
	tests := []struct {
		name string
		text string
		hide []string
	}{
		{
			name: "json login payload",
			text: `{"token":"TOKEN_VALUE","aid":"123456","mid":"account-mid","email":"person@example.com","mobile":"13800138000","identity_code":"110101199001011234","union_id":"union-value","nickname":"Visible Name"}`,
			hide: []string{"TOKEN_VALUE", "123456", "account-mid", "person@example.com", "13800138000", "110101199001011234", "union-value", "Visible Name"},
		},
		{
			name: "javascript object",
			text: `payload = {'safe_mobile':'13900139000','realname':'Example User','sub_union_id':'sub-union'}`,
			hide: []string{"13900139000", "Example User", "sub-union"},
		},
		{
			name: "plain pairs",
			text: `authorization=Bearer abcdefghijklmnop mobile=13700137000 email=user@example.net`,
			hide: []string{"abcdefghijklmnop", "13700137000", "user@example.net"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			redacted := redactPM2LogText(test.text)
			for _, secret := range test.hide {
				if strings.Contains(redacted, secret) {
					t.Fatalf("redacted output still contains %q: %s", secret, redacted)
				}
			}
			if !strings.Contains(redacted, "[REDACTED]") {
				t.Fatalf("redacted output has no replacement marker: %s", redacted)
			}
		})
	}

	ordinary := "[OneBot] WebSocket closed: 1008 - invalid access token"
	if got := redactPM2LogText(ordinary); got != ordinary {
		t.Fatalf("ordinary error text changed to %q", got)
	}
}

func TestPM2AuditLogsDiagnosesFullLatestRunAndRedactsOutputs(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	logPath := filepath.Join(root, "pm2-out.log")
	writeAppPageFixture(t, logPath, strings.Join([]string{
		`[2026-08-17 15:59:00][WARN] 缺失方法: old.__forwardNodes.toJSON`,
		`[2026-08-17 16:02:50][INFO] [Yunzai] Worker 启动, cwd=/bot`,
		`[2026-08-17 16:04:48][INFO] {"token":"TOKEN_VALUE","email":"person@example.com"}`,
		`[2026-08-17 16:04:50][WARN] 缺失方法: makeForwardMsg().__forwardNodes.toJSON`,
		`[2026-08-17 16:09:30][ERROR] { code: 4000, message: '请求失败', data: null }`,
	}, "\n")+"\n")

	pm2LogPathCache.Lock()
	previous, hadPrevious := pm2LogPathCache.entries[root]
	pm2LogPathCache.entries[root] = pm2LogPathCacheEntry{
		files:     []pm2LogFile{{Source: "out", Path: logPath}},
		fetchedAt: time.Now(),
	}
	pm2LogPathCache.Unlock()
	t.Cleanup(func() {
		pm2LogPathCache.Lock()
		defer pm2LogPathCache.Unlock()
		if hadPrevious {
			pm2LogPathCache.entries[root] = previous
		} else {
			delete(pm2LogPathCache.entries, root)
		}
	})

	page, err := (Manager{}).PM2AuditLogs(root, PM2AuditQuery{Query: "请求失败"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 1 || !strings.Contains(page.Lines[0].Text, "请求失败") {
		t.Fatalf("filtered lines = %#v", page.Lines)
	}
	codes := diagnosticCodes(page.Diagnostics)
	if codes["yunzai-forward-tojson"].Count != 1 {
		t.Fatalf("diagnostics did not stay within latest run: %#v", page.Diagnostics)
	}
	if _, ok := codes["sensitive-login-data"]; !ok {
		t.Fatalf("filtered view lost full-log sensitive-data diagnostic: %#v", page.Diagnostics)
	}

	fullPage, err := (Manager{}).PM2AuditLogs(root, PM2AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fullPage.Output, "TOKEN_VALUE") || strings.Contains(fullPage.Output, "person@example.com") {
		t.Fatalf("viewer output leaked login data: %s", fullPage.Output)
	}

	exported, err := (Manager{}).PM2LogExport(root, PM2AuditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(exported, "TOKEN_VALUE") || strings.Contains(exported, "person@example.com") {
		t.Fatalf("export leaked login data: %s", exported)
	}
	if !strings.Contains(exported, "[REDACTED]") {
		t.Fatalf("export has no redaction marker: %s", exported)
	}
}
