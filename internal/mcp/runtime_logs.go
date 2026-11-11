package mcp

import (
	"strings"
	"sync"

	"alemonx/internal/robot"
)

const runtimeLogLimit = 64 << 10

type runtimeLogBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *runtimeLogBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := len(data)
	if count >= runtimeLogLimit {
		b.data = append(b.data[:0], data[count-runtimeLogLimit:]...)
		return count, nil
	}
	if overflow := len(b.data) + count - runtimeLogLimit; overflow > 0 {
		b.data = b.data[:copy(b.data, b.data[overflow:])]
	}
	b.data = append(b.data, data...)
	return count, nil
}

func (b *runtimeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

func (s *Server) developmentLogs(root, query string) (string, error) {
	var latest operationTask
	for _, task := range s.listTasks(root) {
		if task.Action == "dev" && task.CreatedAt.After(latest.CreatedAt) {
			latest = task
		}
	}
	if latest.ID != "" {
		latest, _ = s.getTask(latest.ID)
	}
	return RuntimeLogResult(latest.ID, latest.Action, latest.Status, latest.Error, latest.Output, query)
}

func RuntimeLogResult(id, mode, status, runError, output, query string) (string, error) {
	if len(output) > runtimeLogLimit {
		output = output[len(output)-runtimeLogLimit:]
		if _, rest, found := strings.Cut(output, "\n"); found {
			output = rest
		}
	}
	keyword := strings.ToLower(strings.TrimSpace(query))
	lines := make([]string, 0, 100)
	for _, line := range strings.Split(output, "\n") {
		line = robot.RedactRuntimeLog(line)
		if strings.TrimSpace(line) != "" && strings.Contains(strings.ToLower(line), keyword) {
			lines = append(lines, line)
		}
	}
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
	}
	return encodeResult(map[string]any{
		"taskId": id, "mode": mode, "status": status,
		"error": robot.RedactRuntimeLog(runError), "lines": lines,
	})
}
