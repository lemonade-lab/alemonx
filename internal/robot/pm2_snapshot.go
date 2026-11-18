package robot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"alemonx/internal/packagemanager"
	"alemonx/internal/processutil"
	"alemonx/internal/resources"
	"alemonx/internal/system"
)

const (
	pm2ReadTimeout = 3 * time.Second
	pm2SnapshotTTL = 2 * time.Second
)

type pm2Snapshot struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	expires   time.Time
	processes []PM2Process
	err       error
}

var pm2Snapshots = struct {
	sync.Mutex
	entries map[string]*pm2Snapshot
}{entries: make(map[string]*pm2Snapshot)}

// A status poll can use an installed CLI but must never provision a package.
func pm2ReadLauncher(root string) (name string, prefix []string, err error) {
	switch {
	case localPM2(root):
		if packagemanager.UsesBunRuntime(root) {
			name = bunToolPath()
		} else {
			name = "node"
		}
		prefix = []string{filepath.Join(root, "node_modules", "pm2", "bin", "pm2")}
	default:
		if command, err := resolvePM2Command("pm2"); err == nil {
			name = command
		} else if command, args, ok := resources.ExistingToolCommand("pm2"); ok {
			name, prefix = command, args
			if packagemanager.UsesBunRuntime(root) {
				name = bunToolPath()
			}
		}
	}
	if name == "" {
		return "", nil, errors.New("未找到已安装的 PM2，请先安装 PM2 或在工作台注册后台服务")
	}
	if name == "node" {
		name, err = system.ResolveCommand(name)
		if err != nil {
			return "", nil, missingCommandAdvice("node")
		}
	}
	return name, prefix, nil
}

func pm2ReadCommand(ctx context.Context, root, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	if base := filepath.Base(name); base == "bun" || strings.EqualFold(base, "bun.exe") {
		cmd.Env = system.BunEnvironment(cmd.Environ())
	}
	if err := applyPM2SocketEnvironment(cmd); err != nil {
		return nil, err
	}
	HideWindow(cmd)
	processutil.ConfigureCancellableCommand(cmd)
	return cmd, nil
}

func pm2SnapshotKey(root string) string {
	// Resolve relative daemon paths against the command's working directory.
	if socket := os.Getenv("PM2_DAEMON_RPC_PORT"); runtime.GOOS != "windows" && socket != "" {
		return pm2AbsolutePath(root, socket)
	}
	return filepath.Join(pm2HomePath(root), "rpc.sock")
}

func pm2HomePath(root string) string {
	home := os.Getenv("PM2_HOME")
	if home == "" {
		home = os.Getenv("HOME")
		// Windows installations can report different PM2 homes even when
		// they use the same named pipe. Keep their snapshots isolated.
		if profile := os.Getenv("HOMEPATH"); profile != "" {
			if home != "" {
				profile = home
			}
			if !filepath.IsAbs(profile) {
				profile = os.Getenv("HOMEDRIVE") + profile
			}
			home = profile
		}
		if home == "" {
			home = "/etc"
		}
		home = filepath.Join(home, ".pm2")
	}
	return pm2AbsolutePath(root, home)
}

func pm2AbsolutePath(root, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func pm2JList(ctx context.Context, root string, shared bool) ([]PM2Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, prefix, err := pm2ReadLauncher(root)
	if err != nil {
		return nil, err
	}
	args := append(prefix, "jlist")
	if !shared {
		ctx, cancel := context.WithTimeout(ctx, pm2ReadTimeout)
		defer cancel()
		return readPM2List(ctx, root, name, args)
	}

	// Project-local installations may use different PM2 builds. A result
	// from one executable must not replace another executable's result.
	key := pm2SnapshotKey(root) + "\x00" + name + "\x00" + strings.Join(prefix, "\x00")
	now := time.Now()
	pm2Snapshots.Lock()
	for key, entry := range pm2Snapshots.entries {
		if !entry.expires.IsZero() && !now.Before(entry.expires) {
			delete(pm2Snapshots.entries, key)
		}
	}
	entry := pm2Snapshots.entries[key]
	if entry == nil {
		queryCtx, cancel := context.WithTimeout(context.Background(), pm2ReadTimeout)
		entry = &pm2Snapshot{done: make(chan struct{}), cancel: cancel}
		pm2Snapshots.entries[key] = entry
		go func(entry *pm2Snapshot) {
			defer cancel()
			processes, err := readPM2List(queryCtx, root, name, args)
			pm2Snapshots.Lock()
			entry.processes, entry.err = processes, err
			entry.expires = time.Now().Add(pm2SnapshotTTL)
			close(entry.done)
			pm2Snapshots.Unlock()
		}(entry)
	}
	entry.waiters++
	pm2Snapshots.Unlock()

	defer func() {
		pm2Snapshots.Lock()
		defer pm2Snapshots.Unlock()
		entry.waiters--
		if entry.waiters == 0 && entry.expires.IsZero() {
			entry.cancel()
			if pm2Snapshots.entries[key] == entry {
				delete(pm2Snapshots.entries, key)
			}
		}
	}()
	select {
	case <-entry.done:
		return entry.processes, entry.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func readPM2List(ctx context.Context, root, name string, args []string) ([]PM2Process, error) {
	command, err := pm2ReadCommand(ctx, root, name, args...)
	if err != nil {
		return nil, err
	}
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("读取 PM2 进程状态超时或已取消：%w", ctx.Err())
	}
	// Some PM2 version mismatches print a banner and exit non-zero even
	// though stdout contains a valid JSON array.
	result := stripPM2Banner(strings.TrimSpace(string(output)))
	if strings.HasPrefix(result, "[") {
		if processes, parseErr := parsePM2Processes(result); parseErr == nil {
			return processes, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("pm2 jlist 失败：%w", err)
	}
	return nil, errors.New("无法解析 PM2 进程列表")
}
