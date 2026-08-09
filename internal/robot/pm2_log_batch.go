package robot

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alemonx/internal/agent"
)

// PM2LogBatchSource reads PM2's error log with durable file metadata. It is a
// read-only adapter; all policy, deduplication and incident decisions remain
// in agent.OpsMonitor.
type PM2LogBatchSource struct {
	Robots   Manager
	MaxBytes int64
}

func (s PM2LogBatchSource) ReadBatch(ctx context.Context, root, process string, cursor agent.LogCursor) (agent.LogBatch, error) {
	if err := ctx.Err(); err != nil {
		return agent.LogBatch{}, err
	}
	processes, err := s.Robots.PM2Processes(root)
	if err != nil {
		return agent.LogBatch{}, err
	}
	logPath := ""
	for _, item := range processes {
		if item.Name == process {
			logPath = item.ErrorLog
			break
		}
	}
	if logPath == "" {
		logPath = filepath.Join(os.Getenv("HOME"), ".pm2", "logs", process+"-error.log")
	}
	file, err := os.Open(logPath)
	if err != nil {
		return agent.LogBatch{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return agent.LogBatch{}, err
	}
	device, inode := fileIdentity(info)
	fileCursor := cursor.File
	if fileCursor.LogPath == "" {
		fileCursor = agent.FileLogCursor{LogPath: cursor.LogPath, Device: cursor.Device, Inode: cursor.Inode, Offset: cursor.Offset}
	}
	offset := fileCursor.Offset
	rotated := fileCursor.LogPath != "" && (fileCursor.LogPath != logPath || fileCursor.Device != device || fileCursor.Inode != inode || info.Size() < offset)
	if rotated {
		offset = 0
	}
	if offset > info.Size() {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return agent.LogBatch{}, err
	}
	maxBytes := s.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	reader := bufio.NewReader(io.LimitReader(file, maxBytes))
	var lines []string
	var consumed int64
	for {
		if err := ctx.Err(); err != nil {
			return agent.LogBatch{}, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
			if line != "" {
				lines = append(lines, line)
			}
			consumed += int64(len([]byte(line)) + 1)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return agent.LogBatch{}, readErr
		}
	}
	return agent.LogBatch{ProjectRoot: root, ProcessName: process, LogPath: logPath, Device: device, Inode: inode, Offset: offset + consumed, BytesRead: consumed, Lines: lines, Rotated: rotated}, nil
}
