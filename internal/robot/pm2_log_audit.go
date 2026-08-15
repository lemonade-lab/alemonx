package robot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// PM2AuditPage is one page of filtered PM2 output plus pagination metadata.
type PM2AuditPage struct {
	Output    string         `json:"output"`
	Lines     []PM2AuditLine `json:"lines"`
	Page      int            `json:"page"`
	PerPage   int            `json:"perPage"`
	HasOlder  bool           `json:"hasOlder"`
	HasNewer  bool           `json:"hasNewer"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated"`
	Sources   []string       `json:"sources"`
	Date      string         `json:"date,omitempty"`
	Query     string         `json:"query,omitempty"`
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

const (
	pm2LogDefaultPageSize = 120
	pm2LogMaxPageSize     = 1000
	pm2LogMaxLines        = 2_000_000
	pm2LogPathCacheTTL    = 30 * time.Second
	pm2LogTailInterval    = 400 * time.Millisecond
	pm2LogMaxTailChunk    = 16 << 20
)

// pm2AuditLogLine is one parsed line from a PM2 log file. Timestamps are
// parsed when the application writes a time prefix such as
// "[2026-08-07 16:58:50][INFO] ..."; lines without one stay usable for
// search/export but are excluded from date and time-range filters.
type pm2AuditLogLine struct {
	Text      string
	Timestamp time.Time
	HasTime   bool
	Source    string
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
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	var lines []pm2AuditLogLine
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	truncated := false
	for scanner.Scan() {
		if maxLines > 0 && len(lines) >= maxLines {
			truncated = true
			break
		}
		text := scanner.Text()
		ts, has := parsePM2LogTimestamp(text)
		lines = append(lines, pm2AuditLogLine{Text: text, Timestamp: ts, HasTime: has})
	}
	if err := scanner.Err(); err != nil {
		return lines, truncated, err
	}
	return lines, truncated, nil
}

func parsePM2AuditTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("时间格式无效：%s", value)
}

// pm2AuditLines applies the query to every matching PM2 log file and returns
// the matching lines sorted newest-first, the source names that were read and
// whether any underlying file was truncated at the read limit.
func (m Manager) pm2AuditLines(root string, q PM2AuditQuery) ([]pm2AuditLogLine, []string, bool, error) {
	var since, until time.Time
	var err error
	if q.Since != "" {
		if since, err = parsePM2AuditTime(q.Since); err != nil {
			return nil, nil, false, err
		}
	}
	if q.Until != "" {
		if until, err = parsePM2AuditTime(q.Until); err != nil {
			return nil, nil, false, err
		}
	}
	var date time.Time
	if q.Date != "" {
		if date, err = time.ParseInLocation("2006-01-02", q.Date, time.Local); err != nil {
			return nil, nil, false, fmt.Errorf("日期无效：%s", q.Date)
		}
	}
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return nil, nil, false, err
	}
	lowerQuery := strings.ToLower(q.Query)
	wantSource := func(source string) bool {
		return q.Source == "" || q.Source == "all" || strings.EqualFold(q.Source, source)
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
	var matched []pm2AuditLogLine
	sourceSet := make(map[string]bool)
	truncated := false
	for _, file := range files {
		lines, cut, readErr := readPM2LogFile(file.Path, pm2LogMaxLines)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, nil, false, fmt.Errorf("读取日志失败：%w", readErr)
		}
		truncated = truncated || cut
		sourceSet[file.Source] = true
		for i := range lines {
			lines[i].Source = file.Source
			if wantLine(lines[i]) {
				matched = append(matched, lines[i])
			}
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		if a.HasTime && b.HasTime && !a.Timestamp.Equal(b.Timestamp) {
			return a.Timestamp.After(b.Timestamp)
		}
		return a.HasTime && !b.HasTime
	})
	sources := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return matched, sources, truncated, nil
}

// PM2AuditLogs returns one filtered, newest-first page of PM2 log output.
func (m Manager) PM2AuditLogs(root string, q PM2AuditQuery) (PM2AuditPage, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPage < 1 {
		q.PerPage = pm2LogDefaultPageSize
	}
	if q.PerPage > pm2LogMaxPageSize {
		q.PerPage = pm2LogMaxPageSize
	}
	matched, sources, truncated, err := m.pm2AuditLines(root, q)
	if err != nil {
		return PM2AuditPage{}, err
	}
	total := len(matched)
	start := (q.Page - 1) * q.PerPage
	if start >= total {
		return PM2AuditPage{
			Output:    "没有更早的 PM2 日志。",
			Lines:     []PM2AuditLine{},
			Page:      q.Page,
			PerPage:   q.PerPage,
			Total:     total,
			Truncated: truncated,
			Sources:   sources,
			Date:      q.Date,
			Query:     q.Query,
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
		lines = append(lines, PM2AuditLine{Source: pageLines[i].Source, Text: pageLines[i].Text})
		builder.WriteString(pageLines[i].Text)
		if i > 0 {
			builder.WriteByte('\n')
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("PM2 暂无可读取的日志。")
	}
	return PM2AuditPage{
		Output:    builder.String(),
		Lines:     lines,
		Page:      q.Page,
		PerPage:   q.PerPage,
		HasOlder:  end < total,
		HasNewer:  start > 0,
		Total:     total,
		Truncated: truncated,
		Sources:   sources,
		Date:      q.Date,
		Query:     q.Query,
	}, nil
}

// PM2LogDays summarizes PM2 log lines by calendar day, newest day first, so
// the UI can offer date-based navigation instead of blind page walking.
func (m Manager) PM2LogDays(root string) ([]PM2LogDay, error) {
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
		lines, _, readErr := readPM2LogFile(file.Path, pm2LogMaxLines)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("读取日志失败：%w", readErr)
		}
		for i := range lines {
			line := lines[i]
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
	if len(days) == 0 {
		return nil, fmt.Errorf("PM2 日志中暂无可识别日期的记录。")
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
		builder.WriteString(line.Text)
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

// tailPM2LogFile follows one log file from its current end, emitting complete
// lines as they are appended. Rotation/truncation restarts the tail.
func tailPM2LogFile(ctx context.Context, file pm2LogFile, onLine func(source, text string)) {
	offset := int64(0)
	if info, err := os.Stat(file.Path); err == nil {
		offset = info.Size()
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
		if info.Size() < offset {
			offset = 0
			pending.Reset()
		}
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
					if idx := strings.IndexByte(string(data[start:n]), '\n'); idx >= 0 {
						pending.Write(data[start : start+idx])
						if strings.TrimSpace(pending.String()) != "" {
							onLine(file.Source, pending.String())
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
	files, err := m.pm2LogFiles(root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines := make(chan pm2LogStreamLine, 128)
	var group sync.WaitGroup
	for _, file := range files {
		file := file
		group.Add(1)
		go func() {
			defer group.Done()
			tailPM2LogFile(ctx, file, func(source, text string) {
				select {
				case lines <- pm2LogStreamLine{Source: source, Text: text}:
				case <-ctx.Done():
				}
			})
		}()
	}
	dispatched := make(chan struct{})
	go func() {
		defer close(dispatched)
		for line := range lines {
			onLine(line.Source, line.Text)
		}
	}()
	group.Wait()
	close(lines)
	<-dispatched
	return nil
}
