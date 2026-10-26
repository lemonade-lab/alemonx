package robot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// PM2AuditQuery describes how a PM2 log view should be filtered and paged.
// Page 1 is always the newest matching lines, matching the previous viewer
// semantics while adding date/time-range, source and text filters for audit.
type PM2AuditQuery struct {
	Date    string // YYYY-MM-DD, local time
	Since   string // "2006-01-02 15:04:05" or RFC3339
	Until   string
	Source  string // "out", "err" or empty for both
	Query   string // case-insensitive substring
	Page    int
	PerPage int
}

// Validate rejects ambiguous filters before reading files or starting SSE.
func (q PM2AuditQuery) Validate() error {
	if q.Source != "" && !strings.EqualFold(q.Source, "all") && !strings.EqualFold(q.Source, "out") && !strings.EqualFold(q.Source, "err") {
		return errors.New("日志来源无效")
	}
	if q.Date != "" {
		if _, err := time.ParseInLocation("2006-01-02", q.Date, time.Local); err != nil {
			return fmt.Errorf("日期无效：%s", q.Date)
		}
	}
	var since, until time.Time
	var err error
	if q.Since != "" {
		if since, err = parsePM2AuditTime(q.Since); err != nil {
			return err
		}
	}
	if q.Until != "" {
		if until, err = parsePM2AuditTime(q.Until); err != nil {
			return err
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return errors.New("开始时间不能晚于结束时间")
	}
	return nil
}

// ValidatePM2LogRequest performs only input validation, without launching PM2
// or reading log content. SSE must do this before committing its 200 headers.
func (Manager) ValidatePM2LogRequest(root string, q PM2AuditQuery) error {
	if err := q.Validate(); err != nil {
		return err
	}
	_, err := projectPath(root)
	return err
}

// PM2AuditPage is one page of filtered PM2 output plus pagination metadata.
type PM2AuditPage struct {
	Output      string             `json:"output"`
	Lines       []PM2AuditLine     `json:"lines"`
	Page        int                `json:"page"`
	PerPage     int                `json:"perPage"`
	HasOlder    bool               `json:"hasOlder"`
	HasNewer    bool               `json:"hasNewer"`
	Total       int                `json:"total"`
	Truncated   bool               `json:"truncated"`
	Sources     []string           `json:"sources"`
	Date        string             `json:"date,omitempty"`
	Query       string             `json:"query,omitempty"`
	Diagnostics []PM2LogDiagnostic `json:"diagnostics,omitempty"`
}

// PM2AuditLine is one log line with its source stream ("out" or "err") so the
// viewer can render and audit stdout/stderr separately.
type PM2AuditLine struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

// PM2LogDay is one calendar day's worth of PM2 log lines, used to build a
// date navigator and to make audit checks by date practical.
type PM2LogDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
	Out   int    `json:"out"`
	Err   int    `json:"err"`
	First string `json:"first,omitempty"`
	Last  string `json:"last,omitempty"`
}

// PM2LogDeleteResult reports the scope of a date/source log deletion.
type PM2LogDeleteResult struct {
	Deleted int `json:"deleted"`
	Files   int `json:"files"`
}

// PM2LogDiagnostic is one known runtime issue detected after the latest
// Yunzai worker start in the selected log view.
type PM2LogDiagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Suggestion string `json:"suggestion"`
	Count      int    `json:"count"`
	LastSeen   string `json:"lastSeen,omitempty"`
}

const (
	pm2LogDefaultPageSize = 120
	pm2LogMaxPageSize     = 1000
	pm2LogMaxLines        = 2_000_000
	pm2LogPathCacheTTL    = 30 * time.Second
	pm2LogTailInterval    = 400 * time.Millisecond
	pm2LogMaxTailChunk    = 16 << 20
	pm2LogSnapshotBytes   = 8 << 20
)

// pm2AuditLogLine is one parsed line from a PM2 log file. Timestamps are
// parsed when the application writes a time prefix such as
// "[2026-08-07 16:58:50][INFO] ..."; lines without one stay usable for
// search/export but are excluded from date and time-range filters.
type pm2AuditLogLine struct {
	diagnosticMask uint16
	Text           string
	Timestamp      time.Time
	HasTime        bool
	Source         string
}

type pm2LogFile struct {
	Source string // "out" or "err"
	Path   string
}

type pm2LogPathCacheEntry struct {
	files     []pm2LogFile
	fetchedAt time.Time
}

var pm2LogPathCache struct {
	sync.Mutex
	entries map[string]pm2LogPathCacheEntry
}

var pm2LogMutationMu sync.Mutex

func init() {
	pm2LogPathCache.entries = make(map[string]pm2LogPathCacheEntry)
}

var pm2LogTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.000",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05.000Z07:00",
	"2006-01-02 15:04:05 -0700",
}

var (
	pm2SensitiveFieldNames = `token|access_token|refresh_token|cookie|authorization|aid|mid|account_name|email|unmasked_email|mobile|safe_mobile|rebind_mobile|identity_code|realname|union_id|sub_union_id|nickname`
	pm2SensitiveJSONField  = regexp.MustCompile(`(?i)("(?:` + pm2SensitiveFieldNames + `)"\s*:\s*")([^"]*)(")`)
	pm2SensitiveJSField    = regexp.MustCompile(`(?i)('(?:` + pm2SensitiveFieldNames + `)'\s*:\s*')([^']*)(')`)
	pm2SensitiveTokenPair  = regexp.MustCompile(`(?i)((?:authorization|cookie|access[_-]?token|refresh[_-]?token)\s*[:=]\s*(?:bearer\s+)?)([A-Za-z0-9._~+/\-=]{8,})`)
	pm2SensitiveDataPair   = regexp.MustCompile(`(?i)((?:aid|mid|account_name|email|unmasked_email|mobile|safe_mobile|rebind_mobile|identity_code|realname|union_id|sub_union_id|nickname)\s*[:=]\s*)([^,\s}\]]+)`)
	pm2MissingModulePath   = regexp.MustCompile(`(?i)Cannot find module\s+['"]([^'"]+)['"]`)
)

type pm2DiagnosticRule struct {
	code       string
	severity   string
	title      string
	summary    string
	suggestion string
	match      func(string) bool
	describe   func(string) string
}

var pm2DiagnosticRules = []pm2DiagnosticRule{
	{
		code:       "yunzai-forward-tojson",
		severity:   "warning",
		title:      "合并转发兼容方法缺失",
		summary:    "Yunzai 插件访问 __forwardNodes.toJSON 时触发兼容警告，合并转发可能退化。",
		suggestion: "请升级相关 AlemonJS/Yunzai 依赖到相互兼容的版本；ALemonX 不会修改 node_modules 中的依赖源码。",
		match: func(text string) bool {
			return strings.Contains(text, "__forwardNodes.toJSON") && strings.Contains(text, "缺失")
		},
	},
	{
		code:       "onebot-delete-failed",
		severity:   "warning",
		title:      "OneBot 消息撤回失败",
		summary:    "OneBot 已收到撤回动作，但适配端未返回成功结果。",
		suggestion: "重启后查看保留下来的 retcode 与 wording；同时确认协议端允许撤回该消息 ID。",
		match: func(text string) bool {
			return strings.Contains(text, "Delete not supported or failed") || strings.Contains(text, "message.delete")
		},
	},
	{
		code:       "plugin-module-missing",
		severity:   "error",
		title:      "插件运行模块缺失",
		summary:    "插件引用的运行模块不存在，相关功能可能加载或执行失败。",
		suggestion: "检查报错插件的依赖声明和版本；下次运行会自动同步依赖，必要时升级产生该引用的插件。",
		match: func(text string) bool {
			return strings.Contains(strings.ToLower(text), "cannot find module")
		},
		describe: func(text string) string {
			matches := pm2MissingModulePath.FindStringSubmatch(text)
			if len(matches) < 2 {
				return ""
			}
			return "插件运行时缺少模块：" + matches[1]
		},
	},
	{
		code:       "puppeteer-navigation-timeout",
		severity:   "warning",
		title:      "Puppeteer 页面加载超时",
		summary:    "帮助图页面在 30 秒内未完成导航，图片渲染已回退或失败。",
		suggestion: "检查渲染页面依赖和外部资源连通性，并在插件侧放宽 navigation timeout。",
		match: func(text string) bool {
			return strings.Contains(text, "Navigation timeout") && strings.Contains(text, "exceeded")
		},
	},
	{
		code:       "moment-inverted-add",
		severity:   "info",
		title:      "Moment 参数顺序已弃用",
		summary:    "插件仍在调用 moment().add(period, number)。",
		suggestion: "在产生警告的插件中调整为 moment().add(number, period)。",
		match: func(text string) bool {
			return strings.Contains(text, "moment().add(period, number) is deprecated")
		},
	},
	{
		code:       "onebot-request-failed",
		severity:   "error",
		title:      "OneBot 消息发送失败",
		summary:    "适配层只报告了“请求失败”，旧日志没有保留协议端的具体错误。",
		suggestion: "重启机器人以启用详细错误日志，再根据 retcode、wording 和消息类型定位。",
		match: func(text string) bool {
			return strings.Contains(text, "code: 4000") && strings.Contains(text, "请求失败")
		},
	},
	{
		code:       "sensitive-login-data",
		severity:   "error",
		title:      "日志包含登录凭据或账户字段",
		summary:    "PM2 原始日志中出现了登录 token 或账户隐私字段。",
		suggestion: "刷新相关登录凭据并清理服务器上的原始日志；AlemonX 查看、实时流和导出会自动脱敏。",
		match: func(text string) bool {
			return redactPM2LogText(text) != text
		},
	},
}

func parsePM2LogTimestamp(line string) (time.Time, bool) {
	candidate := line
	if strings.HasPrefix(candidate, "[") {
		if end := strings.IndexByte(candidate, ']'); end > 0 {
			candidate = candidate[1:end]
		}
	}
	for _, layout := range pm2LogTimeLayouts {
		if parsed, err := time.ParseInLocation(layout, candidate, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// Every redaction pattern requires at least one of these field names. This
// guard avoids running four regex replacements on ordinary log messages.
func pm2MayContainSensitiveData(text string) bool {
	lower := strings.ToLower(text)
	for _, field := range pm2SensitiveFields {
		if strings.Contains(lower, field) {
			return true
		}
	}
	return false
}

var pm2SensitiveFields = strings.Split(pm2SensitiveFieldNames, "|")

func pm2DiagnosticMask(text string) uint16 {
	mask := uint16(1 << 15) // distinguish an evaluated, clean line from unset
	for i, rule := range pm2DiagnosticRules {
		if rule.match(text) {
			mask |= 1 << i
		}
	}
	return mask
}

func redactPM2LogText(text string) string {
	if !pm2MayContainSensitiveData(text) {
		return text
	}
	text = pm2SensitiveJSONField.ReplaceAllString(text, `${1}[REDACTED]${3}`)
	text = pm2SensitiveJSField.ReplaceAllString(text, `${1}[REDACTED]${3}`)
	text = pm2SensitiveTokenPair.ReplaceAllString(text, `${1}[REDACTED]`)
	return pm2SensitiveDataPair.ReplaceAllString(text, `${1}[REDACTED]`)
}

func diagnosePM2LogLines(lines []pm2AuditLogLine) []PM2LogDiagnostic {
	result, _ := diagnosePM2LogLinesContext(context.Background(), lines)
	return result
}

func diagnosePM2LogLinesContext(ctx context.Context, lines []pm2AuditLogLine) ([]PM2LogDiagnostic, error) {
	// pm2AuditLines is newest-first. Restrict diagnostics to the latest Yunzai
	// worker lifetime when its start marker is available.
	scoped := lines
	for index, line := range lines {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if strings.Contains(line.Text, "[Yunzai] Worker 启动") ||
			strings.Contains(line.Text, "[Yunzai] Worker started") {
			scoped = lines[:index+1]
			break
		}
	}
	result := make([]PM2LogDiagnostic, 0, len(pm2DiagnosticRules))
	for ruleIndex, rule := range pm2DiagnosticRules {
		diagnostic := PM2LogDiagnostic{
			Code:       rule.code,
			Severity:   rule.severity,
			Title:      rule.title,
			Summary:    rule.summary,
			Suggestion: rule.suggestion,
		}
		for index, line := range scoped {
			if index%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			mask := line.diagnosticMask
			if mask == 0 {
				mask = pm2DiagnosticMask(line.Text)
			}
			if mask&(1<<ruleIndex) == 0 {
				continue
			}
			diagnostic.Count++
			if diagnostic.LastSeen == "" {
				if rule.describe != nil {
					if summary := rule.describe(line.Text); summary != "" {
						diagnostic.Summary = summary
					}
				}
				if line.HasTime {
					diagnostic.LastSeen = line.Timestamp.Format("2006-01-02 15:04:05")
				} else {
					diagnostic.LastSeen = "当前日志"
				}
			}
		}
		if diagnostic.Count > 0 {
			result = append(result, diagnostic)
		}
	}
	severityRank := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.SliceStable(result, func(i, j int) bool {
		return severityRank[result[i].Severity] < severityRank[result[j].Severity]
	})
	return result, ctx.Err()
}

// pm2LogFiles resolves the out/err log file paths belonging to the given
// robot directory. Results are cached briefly because pm2 jlist is not free.
func (Manager) pm2LogFiles(root string) ([]pm2LogFile, error) {
	project, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	pm2LogPathCache.Lock()
	defer pm2LogPathCache.Unlock()
	if entry, ok := pm2LogPathCache.entries[root]; ok && time.Since(entry.fetchedAt) < pm2LogPathCacheTTL {
		return entry.files, nil
	}
	output, err := pm2JList(project)
	if err != nil {
		return nil, fmt.Errorf("无法读取 PM2 进程：%w", err)
	}
	var raw []struct {
		PM2Env struct {
			CWD       string `json:"pm_cwd"`
			ErrorLog  string `json:"pm_err_log_path"`
			OutputLog string `json:"pm_out_log_path"`
		} `json:"pm2_env"`
	}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("无法解析 PM2 进程：%w", err)
	}
	var files []pm2LogFile
	seen := make(map[string]bool)
	for _, p := range raw {
		if !sameWorkspacePath(p.PM2Env.CWD, root) {
			continue
		}
		for _, candidate := range []pm2LogFile{
			{Source: "out", Path: p.PM2Env.OutputLog},
			{Source: "err", Path: p.PM2Env.ErrorLog},
		} {
			if candidate.Path == "" || seen[candidate.Path] {
				continue
			}
			seen[candidate.Path] = true
			files = append(files, candidate)
		}
	}
	if len(files) == 0 {
		files = defaultPM2LogFiles(project)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("未找到当前机器人的 PM2 日志文件，请确认已用 PM2 启动。")
	}
	pm2LogPathCache.entries[root] = pm2LogPathCacheEntry{files: files, fetchedAt: time.Now()}
	return files, nil
}

// defaultPM2LogFiles locates the robot's PM2 log files under ~/.pm2/logs by
// project name when the daemon currently has no matching process (for example
// after the robot stopped), so past logs remain available for audit.
func defaultPM2LogFiles(project string) []pm2LogFile {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	logsDir := filepath.Join(home, ".pm2", "logs")
	base := strings.ToLower(filepath.Base(project))
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return nil
	}
	var files []pm2LogFile
	seen := make(map[string]bool)
	add := func(path, source string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		files = append(files, pm2LogFile{Source: source, Path: path})
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		if !strings.HasSuffix(lower, "-out.log") {
			continue
		}
		stem := strings.TrimSuffix(lower, "-out.log")
		if stem == "" {
			continue
		}
		matches := stem == base ||
			strings.HasPrefix(stem, base+"-") ||
			strings.HasPrefix(stem, "alemonx-"+base+"-") ||
			strings.HasPrefix(stem, "alemonx-"+base)
		if !matches {
			continue
		}
		add(filepath.Join(logsDir, entry.Name()), "out")
		errPath := filepath.Join(logsDir, stem+"-error.log")
		if info, statErr := os.Stat(errPath); statErr == nil && !info.IsDir() {
			add(errPath, "err")
		}
	}
	return files
}

func readPM2LogFile(path string, maxLines int) ([]pm2AuditLogLine, bool, error) {
	return readCachedPM2LogFile(context.Background(), path, maxLines)
}

func readPM2LogReader(reader io.Reader, maxLines int) ([]pm2AuditLogLine, bool, error) {
	var lines []pm2AuditLogLine
	next := 0
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	truncated := false
	texts := make([]string, 0, pm2ParsedBlockLines)
	blockBytes := 0
	flush := func() {
		if len(texts) == 0 {
			return
		}
		for _, line := range parsePM2LogBlock(texts) {
			if maxLines > 0 && len(lines) >= maxLines {
				truncated = true
				lines[next] = line
				next = (next + 1) % maxLines
			} else {
				lines = append(lines, line)
			}
		}
		clear(texts)
		texts = texts[:0]
		blockBytes = 0
	}
	for scanner.Scan() {
		text := scanner.Text()
		texts = append(texts, text)
		blockBytes += len(text)
		if len(texts) >= pm2ParsedBlockLines || blockBytes >= pm2ParsedBlockBytes {
			flush()
		}
	}
	flush()
	if truncated && next > 0 {
		lines = append(lines[next:], lines[:next]...)
	}
	return lines, truncated, scanner.Err()
}

func parsePM2AuditTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("时间格式无效：%s", value)
}

// readPM2AuditLines reads every matching PM2 log file once and returns all
// lines sorted newest-first. Keeping filtering separate lets diagnostics use
// the full latest worker lifetime even while the viewer filters by source or
// search text.
func (m Manager) readPM2AuditLines(root string) ([]pm2AuditLogLine, []string, bool, error) {
	return m.readPM2AuditLinesContext(context.Background(), root)
}

func (m Manager) readPM2AuditLinesContext(ctx context.Context, root string) ([]pm2AuditLogLine, []string, bool, error) {
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return nil, nil, false, err
	}
	var lines []pm2AuditLogLine
	sourceSet := make(map[string]bool)
	truncated := false
	for _, file := range files {
		fileLines, cut, readErr := readCachedPM2LogFile(ctx, file.Path, pm2LogMaxLines)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, nil, false, fmt.Errorf("读取日志失败：%w", readErr)
		}
		truncated = truncated || cut
		sourceSet[file.Source] = true
		for i := range fileLines {
			fileLines[i].Source = file.Source
		}
		if len(lines) == 0 {
			lines = fileLines
		} else {
			lines = append(lines, fileLines...)
		}
	}
	sortPM2AuditLines(lines)
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return lines, sources, truncated, nil
}

func sortPM2AuditLines(lines []pm2AuditLogLine) {
	// Reverse physical order first so equal or missing timestamps still show
	// the latest records on page one, and retain stack trace line order.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.HasTime && b.HasTime && !a.Timestamp.Equal(b.Timestamp) {
			return a.Timestamp.After(b.Timestamp)
		}
		return a.HasTime && !b.HasTime
	})
}

func filterPM2AuditLines(lines []pm2AuditLogLine, q PM2AuditQuery) ([]pm2AuditLogLine, error) {
	return filterPM2AuditLinesContext(context.Background(), lines, q)
}

func filterPM2AuditLinesContext(ctx context.Context, lines []pm2AuditLogLine, q PM2AuditQuery) ([]pm2AuditLogLine, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.Date == "" && q.Since == "" && q.Until == "" && q.Query == "" && (q.Source == "" || strings.EqualFold(q.Source, "all")) {
		return lines, nil
	}
	var since, until time.Time
	var err error
	if q.Since != "" {
		if since, err = parsePM2AuditTime(q.Since); err != nil {
			return nil, err
		}
	}
	if q.Until != "" {
		if until, err = parsePM2AuditTime(q.Until); err != nil {
			return nil, err
		}
	}
	var date time.Time
	if q.Date != "" {
		if date, err = time.ParseInLocation("2006-01-02", q.Date, time.Local); err != nil {
			return nil, fmt.Errorf("日期无效：%s", q.Date)
		}
	}
	lowerQuery := strings.ToLower(q.Query)
	wantSource := func(source string) bool {
		return q.Source == "" || strings.EqualFold(q.Source, "all") || strings.EqualFold(q.Source, source)
	}
	wantLine := func(line pm2AuditLogLine) bool {
		if !wantSource(line.Source) {
			return false
		}
		if lowerQuery != "" && !strings.Contains(strings.ToLower(line.Text), lowerQuery) {
			return false
		}
		if !line.HasTime {
			return q.Date == "" && since.IsZero() && until.IsZero()
		}
		if !date.IsZero() {
			y, mo, d := line.Timestamp.Date()
			dy, dmo, dd := date.Date()
			if y != dy || mo != dmo || d != dd {
				return false
			}
		}
		if !since.IsZero() && line.Timestamp.Before(since) {
			return false
		}
		if !until.IsZero() && line.Timestamp.After(until) {
			return false
		}
		return true
	}
	matched := make([]pm2AuditLogLine, 0, len(lines))
	for index, line := range lines {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if wantLine(line) {
			matched = append(matched, line)
		}
	}
	return matched, nil
}

// pm2AuditLines applies the query to every matching PM2 log file and returns
// the matching lines sorted newest-first, the source names that were read and
// whether any underlying file was truncated at the read limit.
func (m Manager) pm2AuditLines(root string, q PM2AuditQuery) ([]pm2AuditLogLine, []string, bool, error) {
	lines, sources, truncated, err := m.readPM2AuditLines(root)
	if err != nil {
		return nil, nil, false, err
	}
	matched, err := filterPM2AuditLines(lines, q)
	if err != nil {
		return nil, nil, false, err
	}
	return matched, sources, truncated, nil
}

// PM2AuditLogs returns one filtered, newest-first page of PM2 log output.
func (m Manager) PM2AuditLogs(root string, q PM2AuditQuery) (PM2AuditPage, error) {
	return m.PM2AuditLogsContext(context.Background(), root, q)
}

func (m Manager) PM2AuditLogsContext(ctx context.Context, root string, q PM2AuditQuery) (PM2AuditPage, error) {
	allLines, sources, truncated, err := m.readPM2AuditLinesContext(ctx, root)
	if err != nil {
		return PM2AuditPage{}, err
	}
	return buildPM2AuditPageContext(ctx, allLines, sources, truncated, q)
}

func buildPM2AuditPage(allLines []pm2AuditLogLine, sources []string, truncated bool, q PM2AuditQuery) (PM2AuditPage, error) {
	return buildPM2AuditPageContext(context.Background(), allLines, sources, truncated, q)
}

func buildPM2AuditPageContext(ctx context.Context, allLines []pm2AuditLogLine, sources []string, truncated bool, q PM2AuditQuery) (PM2AuditPage, error) {
	if err := ctx.Err(); err != nil {
		return PM2AuditPage{}, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPage < 1 {
		q.PerPage = pm2LogDefaultPageSize
	}
	if q.PerPage > pm2LogMaxPageSize {
		q.PerPage = pm2LogMaxPageSize
	}
	matched, err := filterPM2AuditLinesContext(ctx, allLines, q)
	if err != nil {
		return PM2AuditPage{}, err
	}
	diagnostics, err := diagnosePM2LogLinesContext(ctx, allLines)
	if err != nil {
		return PM2AuditPage{}, err
	}
	total := len(matched)
	// Bound the page before multiplication to prevent signed integer overflow.
	start := total
	if q.Page-1 <= total/q.PerPage {
		start = (q.Page - 1) * q.PerPage
	}
	if start >= total {
		return PM2AuditPage{
			Output:      "没有更早的 PM2 日志。",
			Lines:       []PM2AuditLine{},
			Page:        q.Page,
			PerPage:     q.PerPage,
			HasNewer:    q.Page > 1 && total > 0,
			Total:       total,
			Truncated:   truncated,
			Sources:     sources,
			Date:        q.Date,
			Query:       q.Query,
			Diagnostics: diagnostics,
		}, nil
	}
	end := start + q.PerPage
	if end > total {
		end = total
	}
	pageLines := matched[start:end]
	var builder strings.Builder
	lines := make([]PM2AuditLine, 0, len(pageLines))
	for i := len(pageLines) - 1; i >= 0; i-- {
		text := redactPM2LogText(pageLines[i].Text)
		lines = append(lines, PM2AuditLine{Source: pageLines[i].Source, Text: text})
		builder.WriteString(text)
		if i > 0 {
			builder.WriteByte('\n')
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("PM2 暂无可读取的日志。")
	}
	return PM2AuditPage{
		Output:      builder.String(),
		Lines:       lines,
		Page:        q.Page,
		PerPage:     q.PerPage,
		HasOlder:    end < total,
		HasNewer:    start > 0,
		Total:       total,
		Truncated:   truncated,
		Sources:     sources,
		Date:        q.Date,
		Query:       q.Query,
		Diagnostics: diagnostics,
	}, nil
}

// PM2LogDays summarizes PM2 log lines by calendar day, newest day first, so
// the UI can offer date-based navigation instead of blind page walking.
func (m Manager) PM2LogDays(root string) ([]PM2LogDay, error) {
	return m.PM2LogDaysContext(context.Background(), root)
}

func (m Manager) PM2LogDaysContext(ctx context.Context, root string) ([]PM2LogDay, error) {
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return nil, err
	}
	type dayStats struct {
		day   *PM2LogDay
		first time.Time
		last  time.Time
	}
	days := make(map[string]*dayStats)
	for _, file := range files {
		lines, _, readErr := readCachedPM2LogFile(ctx, file.Path, pm2LogMaxLines)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("读取日志失败：%w", readErr)
		}
		for i := range lines {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			line := lines[i]
			line.Source = file.Source
			if !line.HasTime {
				continue
			}
			key := line.Timestamp.Format("2006-01-02")
			stats, ok := days[key]
			if !ok {
				stats = &dayStats{day: &PM2LogDay{Date: key}}
				days[key] = stats
			}
			stats.day.Count++
			if line.Source == "err" {
				stats.day.Err++
			} else {
				stats.day.Out++
			}
			if stats.first.IsZero() || line.Timestamp.Before(stats.first) {
				stats.first = line.Timestamp
			}
			if line.Timestamp.After(stats.last) {
				stats.last = line.Timestamp
			}
		}
	}
	result := make([]PM2LogDay, 0, len(days))
	for _, stats := range days {
		if !stats.first.IsZero() {
			stats.day.First = stats.first.Format("2006-01-02 15:04:05")
		}
		if !stats.last.IsZero() {
			stats.day.Last = stats.last.Format("2006-01-02 15:04:05")
		}
		result = append(result, *stats.day)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date > result[j].Date
	})
	return result, nil
}

// DeletePM2Logs removes only timestamped records in the requested calendar
// day. A source can narrow deletion to stdout or stderr; no date means no
// deletion, so the current live log cannot be cleared accidentally.
func (m Manager) DeletePM2Logs(root string, q PM2AuditQuery) (PM2LogDeleteResult, error) {
	if err := q.Validate(); err != nil {
		return PM2LogDeleteResult{}, err
	}
	if q.Query != "" || q.Since != "" || q.Until != "" || q.Page > 1 || q.PerPage > 0 {
		return PM2LogDeleteResult{}, errors.New("删除仅支持按日期和来源指定范围，不支持搜索、时间范围或分页条件")
	}
	q.Source = strings.ToLower(q.Source)
	if q.Source == "all" {
		q.Source = ""
	}
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(q.Date), time.Local)
	if err != nil {
		return PM2LogDeleteResult{}, errors.New("请选择要删除的日志日期")
	}
	if q.Source != "" && q.Source != "out" && q.Source != "err" {
		return PM2LogDeleteResult{}, errors.New("日志来源无效")
	}
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return PM2LogDeleteResult{}, err
	}
	pm2LogMutationMu.Lock()
	defer pm2LogMutationMu.Unlock()
	result := PM2LogDeleteResult{}
	for _, file := range files {
		if q.Source != "" && file.Source != q.Source {
			continue
		}
		content, readErr := os.ReadFile(file.Path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return result, fmt.Errorf("读取日志失败：%w", readErr)
		}
		var kept strings.Builder
		removed := 0
		for _, line := range strings.SplitAfter(string(content), "\n") {
			text := strings.TrimSuffix(line, "\n")
			stamp, hasTime := parsePM2LogTimestamp(text)
			if hasTime && stamp.Year() == day.Year() && stamp.YearDay() == day.YearDay() {
				removed++
				continue
			}
			kept.WriteString(line)
		}
		if removed == 0 {
			continue
		}
		info, statErr := os.Stat(file.Path)
		if statErr != nil {
			return result, fmt.Errorf("读取日志属性失败：%w", statErr)
		}
		// Truncating the existing inode keeps PM2's open append handle valid;
		// replacing the file would make a running PM2 process write to an
		// unlinked old file instead.
		handle, openErr := os.OpenFile(file.Path, os.O_WRONLY|os.O_TRUNC, info.Mode())
		if openErr != nil {
			return result, fmt.Errorf("写入日志失败：%w", openErr)
		}
		_, writeErr := io.WriteString(handle, kept.String())
		closeErr := handle.Close()
		if writeErr != nil {
			return result, fmt.Errorf("写入日志失败：%w", writeErr)
		}
		if closeErr != nil {
			return result, fmt.Errorf("关闭日志文件失败：%w", closeErr)
		}
		result.Deleted += removed
		result.Files++
	}
	return result, nil
}

// PM2LogExport renders every line matching the query, oldest first, with an
// explicit [OUT]/[ERR] marker per line so an exported audit file is
// unambiguous even after the original log files rotate away.
func (m Manager) PM2LogExport(root string, q PM2AuditQuery) (string, error) {
	matched, _, truncated, err := m.pm2AuditLines(root, q)
	if err != nil {
		return "", err
	}
	limit := pm2LogMaxLines
	if len(matched) > limit {
		matched = matched[:limit]
		truncated = true
	}
	var builder strings.Builder
	if truncated {
		builder.WriteString("# 注意：日志行数超过导出上限，已截断最早的部分。\n")
	}
	for i := len(matched) - 1; i >= 0; i-- {
		line := matched[i]
		builder.WriteByte('[')
		builder.WriteString(strings.ToUpper(line.Source))
		builder.WriteString("] ")
		builder.WriteString(redactPM2LogText(line.Text))
		builder.WriteByte('\n')
	}
	if builder.Len() == 0 {
		builder.WriteString("PM2 暂无可导出的日志。\n")
	}
	return builder.String(), nil
}

type pm2LogStreamLine struct {
	Source string
	Text   string
}

type pm2TailStart struct {
	offset int64
	info   os.FileInfo
}

// tailPM2LogFile follows one log file, emitting complete appended lines.
// A replaced or truncated file restarts the tail.
func tailPM2LogFile(ctx context.Context, file pm2LogFile, onLine func(source, text string), starts ...pm2TailStart) {
	offset := int64(0)
	var previous os.FileInfo
	if len(starts) > 0 {
		offset, previous = starts[0].offset, starts[0].info
	} else if info, err := os.Stat(file.Path); err == nil {
		offset, previous = info.Size(), info
	}
	var pending strings.Builder
	ticker := time.NewTicker(pm2LogTailInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		handle, err := os.Open(file.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			offset = 0
			pending.Reset()
			continue
		}
		info, statErr := handle.Stat()
		if statErr != nil {
			_ = handle.Close()
			continue
		}
		if info.Size() < offset || (previous != nil && !os.SameFile(previous, info)) {
			offset = 0
			pending.Reset()
		}
		previous = info
		toRead := info.Size() - offset
		if toRead > 0 {
			if toRead > pm2LogMaxTailChunk {
				toRead = pm2LogMaxTailChunk
			}
			data := make([]byte, toRead)
			n, readErr := handle.ReadAt(data, offset)
			if n > 0 {
				offset += int64(n)
				start := 0
				for start < n {
					if idx := bytes.IndexByte(data[start:n], '\n'); idx >= 0 {
						pending.Write(data[start : start+idx])
						if strings.TrimSpace(pending.String()) != "" {
							onLine(file.Source, strings.TrimSuffix(pending.String(), "\r"))
						}
						pending.Reset()
						start += idx + 1
					} else {
						pending.Write(data[start:n])
						start = n
					}
				}
			}
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				_ = handle.Close()
				continue
			}
		}
		_ = handle.Close()
	}
}

// StreamPM2LogFiles tails every log file belonging to the robot and invokes
// onLine serially for each new complete line. It returns when ctx is cancelled
// or the underlying files disappear permanently.
func (m Manager) StreamPM2LogFiles(ctx context.Context, root string, onLine func(source, text string)) error {
	return m.StreamPM2LogFilesMatching(ctx, root, PM2AuditQuery{}, onLine)
}

// StreamPM2LogFilesMatching applies the same filters as history without sending
// an initial snapshot, for callers using the original tail-only protocol.
func (m Manager) StreamPM2LogFilesMatching(ctx context.Context, root string, q PM2AuditQuery, onLine func(source, text string)) error {
	return m.streamPM2LogFiles(ctx, root, q, nil, onLine)
}

// StreamPM2LogsWithSnapshot captures complete-line boundaries before reading
// history. Tails resume at those boundaries, preventing overlap and gaps.
func (m Manager) StreamPM2LogsWithSnapshot(ctx context.Context, root string, q PM2AuditQuery, onSnapshot func(PM2AuditPage), onLine func(source, text string)) error {
	return m.streamPM2LogFiles(ctx, root, q, onSnapshot, onLine)
}

func (m Manager) streamPM2LogFiles(ctx context.Context, root string, q PM2AuditQuery, onSnapshot func(PM2AuditPage), onLine func(source, text string)) error {
	if err := q.Validate(); err != nil {
		return err
	}
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return err
	}
	starts := make([]pm2TailStart, len(files))
	var snapshot []pm2AuditLogLine
	var sources []string
	truncated := false
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		handle, err := os.Open(file.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		info, err := handle.Stat()
		if err != nil {
			handle.Close()
			return err
		}
		starts[i] = pm2TailStart{offset: info.Size(), info: info}
		if onSnapshot != nil {
			// Leave an unfinished final line to the tail until its newline arrives.
			offset := info.Size()
			buffer := make([]byte, 4096)
			for offset > 0 {
				start := offset - int64(len(buffer))
				if start < 0 {
					start = 0
				}
				n, readErr := handle.ReadAt(buffer[:offset-start], start)
				if readErr != nil && !errors.Is(readErr, io.EOF) {
					handle.Close()
					return readErr
				}
				if end := bytes.LastIndexByte(buffer[:n], '\n'); end >= 0 {
					offset = start + int64(end) + 1
					break
				}
				offset = start
			}
			starts[i].offset = offset
			start := int64(0)
			if offset > pm2LogSnapshotBytes {
				start = offset - pm2LogSnapshotBytes
				// Discard only the partial first line of this byte window.
				reader := bufio.NewReader(io.NewSectionReader(handle, start, offset-start))
				skipped, _ := reader.ReadString('\n')
				start += int64(len(skipped))
				truncated = true
			}
			lines, cut, readErr := readPM2LogReader(pm2ContextReader{ctx, io.NewSectionReader(handle, start, offset-start)}, pm2LogMaxLines)
			if readErr != nil {
				handle.Close()
				return readErr
			}
			for j := range lines {
				lines[j].Source = file.Source
			}
			snapshot = append(snapshot, lines...)
			sources = append(sources, file.Source)
			truncated = truncated || cut
		}
		handle.Close()
	}
	if onSnapshot != nil {
		sortPM2AuditLines(snapshot)
		page, err := buildPM2AuditPageContext(ctx, snapshot, sources, truncated, q)
		if err != nil {
			return err
		}
		onSnapshot(page)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan pm2LogStreamLine, 128)
	var group sync.WaitGroup
	for i, file := range files {
		file := file
		start := starts[i]
		group.Add(1)
		go func() {
			defer group.Done()
			tailPM2LogFile(ctx, file, func(source, text string) {
				select {
				case lines <- pm2LogStreamLine{Source: source, Text: text}:
				case <-ctx.Done():
				}
			}, start)
		}()
	}
	dispatched := make(chan struct{})
	go func() {
		defer close(dispatched)
		for line := range lines {
			stamp, hasTime := parsePM2LogTimestamp(line.Text)
			matched, _ := filterPM2AuditLines([]pm2AuditLogLine{{Source: line.Source, Text: line.Text, Timestamp: stamp, HasTime: hasTime}}, q)
			if len(matched) > 0 {
				onLine(line.Source, redactPM2LogText(line.Text))
			}
		}
	}()
	group.Wait()
	close(lines)
	<-dispatched
	return nil
}
