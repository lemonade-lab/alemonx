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
)

const diagnosticLogBytes int64 = 4 << 20
const diagnosticLogFiles = 16

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
	if len(files) == 0 {
		if saved, err := savedPM2Processes(home); err == nil {
			// Include live entries so a shared path remains ineligible.
			files = diagnosticPM2Files(path, home, append(processes, saved...))
		}
	}
	if len(files) == 0 {
		if err != nil && !errors.Is(err, errPM2Offline) {
			return PM2AuditPage{}, err
		}
		return PM2AuditPage{}, errors.New("未找到可归属到此机器人目录的 PM2 日志。请查看本机运行日志或操作记录。")
	}
	truncated := len(files) > diagnosticLogFiles
	files = files[:min(len(files), diagnosticLogFiles)]
	var lines []pm2AuditLogLine
	sources := make([]string, 0, 2)
	for _, file := range files {
		items, cut, err := readDiagnosticPM2File(ctx, file)
		if os.IsNotExist(err) {
			continue // PM2 may have rotated the file since discovery.
		}
		if err != nil {
			return PM2AuditPage{}, err
		}
		truncated = truncated || cut
		lines = append(lines, items...)
		if !slices.Contains(sources, file.Source) {
			sources = append(sources, file.Source)
		}
	}
	sortPM2AuditLines(lines)
	query.includeTimestamps = true
	return buildPM2AuditPageContext(ctx, lines, sources, truncated, query)
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
			if err != nil || (!diagnosticPathInside(projectRoot, resolved) && !diagnosticPathInside(logRoot, resolved)) {
				continue
			}
			if !sameWorkspacePath(process.CWD, project) {
				shared[resolved] = true
				continue
			}
			if _, exists := paths[resolved]; !exists {
				ordered = append(ordered, resolved)
				paths[resolved] = pm2LogFile{Source: log.Source, Path: resolved}
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
	start := max(0, info.Size()-diagnosticLogBytes)
	reader := bufio.NewReader(pm2ContextReader{ctx, io.NewSectionReader(file, start, info.Size()-start)})
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
	lines, truncated, err := readPM2LogReader(reader, 20_000)
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
	return lines, start > 0 || truncated, err
}
