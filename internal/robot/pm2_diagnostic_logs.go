package robot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const diagnosticLogBytes int64 = 4 << 20
const diagnosticLogFiles = 16
const diagnosticLogLines = 20_000

// DiagnosticPM2Logs reads a bounded tail of logs attributable to this project.
// PM2 can be offline if its saved process list identifies the files.
func (Manager) DiagnosticPM2Logs(ctx context.Context, root string, query PM2AuditQuery) (PM2AuditPage, error) {
	if err := query.Validate(); err != nil {
		return PM2AuditPage{}, err
	}
	path, err := projectPath(root)
	if err != nil {
		return PM2AuditPage{}, err
	}
	processes, err := observePM2Daemon(ctx, path)
	if ctx.Err() != nil {
		return PM2AuditPage{}, ctx.Err()
	}
	home := pm2HomePath(path)
	files := diagnosticPM2Files(path, home, processes)
	saved, savedErr := savedPM2Processes(home)
	registered := append(processes, saved...)
	if len(files) == 0 && savedErr == nil {
		// Include live entries so a shared path remains ineligible.
		files = diagnosticPM2Files(path, home, registered)
	}
	if len(files) == 0 {
		if err != nil && !errors.Is(err, errPM2Offline) {
			return PM2AuditPage{}, err
		}
		return PM2AuditPage{}, errors.New("未找到可归属到此机器人目录的 PM2 日志。请查看本机运行日志或操作记录。")
	}
	truncated := len(files) > diagnosticLogFiles
	files = files[:min(len(files), diagnosticLogFiles)]
	bases := diagnosticPM2Files(path, home, registered)
	archives, incomplete, err := pm2LogArchives(ctx, bases, registered, diagnosticLogFiles-len(files))
	if err != nil {
		return PM2AuditPage{}, err
	}
	truncated = truncated || incomplete
	scope := PM2LogReadScope{
		ArchiveFormats:  []string{"pm2-logrotate-default", "logrotate-numbered"},
		MaxFiles:        diagnosticLogFiles,
		MaxBytesPerFile: diagnosticLogBytes,
		MaxLinesPerFile: diagnosticLogLines,
	}
	var lines []pm2AuditLogLine
	sources := make([]string, 0, 2)
	read := func(files []pm2LogFile, archive bool) error {
		for _, file := range files {
			items, cut, err := readDiagnosticPM2File(ctx, file)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if (archive && err != nil) || os.IsNotExist(err) {
				truncated = true
				continue
			}
			if err != nil {
				return err
			}
			truncated = truncated || cut
			lines = append(lines, items...)
			if archive {
				scope.ArchiveFiles++
			} else {
				scope.ActiveFiles++
			}
			if !slices.Contains(sources, file.Source) {
				sources = append(sources, file.Source)
			}
		}
		return nil
	}
	if err := read(archives, true); err != nil {
		return PM2AuditPage{}, err
	}
	if err := read(files, false); err != nil {
		return PM2AuditPage{}, err
	}
	sortPM2AuditLines(lines)
	var oldest, newest time.Time
	for _, line := range lines {
		if query.Source != "" && !strings.EqualFold(query.Source, "all") && !strings.EqualFold(query.Source, line.Source) {
			continue
		}
		scope.ScannedLines++
		if line.HasTime {
			if newest.IsZero() {
				newest = line.Timestamp.UTC()
			}
			oldest = line.Timestamp.UTC()
		}
	}
	if !newest.IsZero() {
		scope.OldestTimestamp = &oldest
		scope.NewestTimestamp = &newest
	}
	query.includeTimestamps = true
	page, err := buildPM2AuditPageContext(ctx, lines, sources, truncated, query)
	page.ReadScope = &scope
	return page, err
}

// dump.pm2 records the cwd and log paths directly, unlike getMonitorData,
// which wraps these fields in pm2_env. Decode only the fields needed here.
func savedPM2Processes(home string) ([]PM2Process, error) {
	name := filepath.Join(home, "dump.pm2")
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, errors.New("PM2 保存的进程列表不可读取")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var saved []struct {
		CWD string `json:"pm_cwd"`
		Out string `json:"pm_out_log_path"`
		Err string `json:"pm_err_log_path"`
	}
	if err := json.NewDecoder(io.LimitReader(file, 16<<20)).Decode(&saved); err != nil {
		return nil, err
	}
	processes := make([]PM2Process, 0, len(saved))
	for _, process := range saved {
		processes = append(processes, PM2Process{CWD: process.CWD, ErrorLog: process.Err, OutputLog: process.Out})
	}
	return processes, nil
}

func diagnosticPM2Files(project, home string, processes []PM2Process) []pm2LogFile {
	// Do not guess ownership from a basename, or return a file shared with
	// another project. PM2 may be configured to share a path across entries.
	paths := make(map[string]pm2LogFile)
	shared := make(map[string]bool)
	ordered := make([]string, 0)
	logRoot, _ := filepath.EvalSymlinks(filepath.Join(home, "logs"))
	projectRoot, _ := filepath.EvalSymlinks(project)
	for _, process := range processes {
		for _, log := range []pm2LogFile{{"out", process.OutputLog}, {"err", process.ErrorLog}} {
			if !filepath.IsAbs(log.Path) || !strings.EqualFold(filepath.Ext(log.Path), ".log") {
				continue
			}
			resolved, err := filepath.EvalSymlinks(log.Path)
			if os.IsNotExist(err) {
				var dir string
				dir, err = filepath.EvalSymlinks(filepath.Dir(log.Path))
				resolved = filepath.Join(dir, filepath.Base(log.Path))
			}
			if err != nil || (!diagnosticPathInside(projectRoot, resolved) && !diagnosticPathInside(logRoot, resolved)) {
				continue
			}
			key := pm2AbsolutePath("", resolved)
			if !sameWorkspacePath(process.CWD, project) {
				shared[key] = true
				continue
			}
			if _, exists := paths[key]; !exists {
				ordered = append(ordered, key)
				paths[key] = pm2LogFile{Source: log.Source, Path: resolved}
			}
		}
	}
	files := make([]pm2LogFile, 0, len(ordered))
	for _, path := range ordered {
		if !shared[path] {
			files = append(files, paths[path])
		}
	}
	return files
}

func diagnosticPathInside(root, path string) bool {
	if root == "" {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func readDiagnosticPM2File(ctx context.Context, log pm2LogFile) ([]pm2AuditLogLine, bool, error) {
	info, err := os.Lstat(log.Path)
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, errors.New("PM2 日志目标不是普通文件")
	}
	file, err := os.Open(log.Path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !os.SameFile(info, opened) {
		return nil, false, errors.New("PM2 日志文件在打开时发生变化")
	}
	var lines []pm2AuditLogLine
	var truncated bool
	if strings.HasSuffix(log.Path, ".gz") {
		lines, truncated, err = readCompressedPM2Log(ctx, file)
	} else {
		lines, truncated, err = readDiagnosticPM2Tail(ctx, file, info.Size())
	}
	for i := range lines {
		if i%256 == 0 && ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		lines[i].Source = log.Source
		// Filter against the redacted text as well, so a credential cannot
		// be inferred from the number of keyword matches.
		lines[i].Text = RedactRuntimeLog(lines[i].Text)
		if len(lines[i].Text) > 4096 {
			lines[i].Text = lines[i].Text[:4096]
			truncated = true
		}
	}
	return lines, truncated, err
}

func readDiagnosticPM2Tail(ctx context.Context, file *os.File, size int64) ([]pm2AuditLogLine, bool, error) {
	start := max(0, size-diagnosticLogBytes)
	reader := bufio.NewReader(pm2ContextReader{ctx, io.NewSectionReader(file, start, size-start)})
	if start > 0 {
		// Skip a partial line without allocating a multi-megabyte string.
		for {
			_, err := reader.ReadSlice('\n')
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil && err != io.EOF {
				return nil, false, err
			}
			break
		}
	}
	lines, truncated, err := readPM2LogReader(reader, diagnosticLogLines)
	return lines, start > 0 || truncated, err
}
