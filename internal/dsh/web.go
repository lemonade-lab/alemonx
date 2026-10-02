package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"alemonx/internal/resources"
	"alemonx/internal/system"
)

func defaultWebCommand(ctx context.Context, home string) (*exec.Cmd, error) {
	bin := os.Getenv("ALX_DSH_BIN")
	var prefix []string
	var patch string
	if bin == "" {
		entry, err := resources.DSHProgram()
		if err != nil {
			return nil, err
		}
		node, err := system.CurrentNodeRuntime()
		if err != nil {
			return nil, fmt.Errorf("DSH 需要可用的 Node.js：%w", err)
		}
		bin, prefix = node.Path, []string{entry}
		patch, err = writeWebWorkspacePatch(home, entry)
		if err != nil {
			return nil, err
		}
	}
	args := append(prefix, "web")
	if patch != "" {
		args = append(args, "--patch", patch)
	}
	args = append(args, "--host", "127.0.0.1", "--port", "0", "--no-open")
	cmd := exec.CommandContext(ctx, bin, args...)
	system.HideWindow(cmd)
	return cmd, nil
}

func writeWebWorkspacePatch(home, entry string) (string, error) {
	packageRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry)))))
	plugin := filepath.Join(packageRoot, "plugins", "workspace-entry", "index.js")
	if !fileExists(plugin) {
		return "", errors.New("安装包缺少 DSH 工作区入口插件，请重新构建")
	}
	quoted, err := json.Marshal(filepath.ToSlash(plugin))
	if err != nil {
		return "", err
	}
	patch := filepath.Join(home, "alemonx.web.patch.yml")
	data := "- insert:\n    - id: alemonx-workspace-entry\n      name: " + string(quoted) + "\n"
	temporary := patch + ".tmp"
	if err := os.WriteFile(temporary, []byte(data), 0600); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, patch); err != nil {
		return "", err
	}
	return patch, nil
}

// StartWeb uses the official Web composition with a workspace-entry plugin. The
// SDK and Web process never write the same DSH_HOME concurrently. The launch
// token stays in memory and is returned only to the authenticated opener.
func (r *Runtime) StartWeb(ctx context.Context, root, apiKey string) (string, error) {
	r.startMu.Lock()
	defer r.startMu.Unlock()
	if !filepath.IsAbs(root) {
		return "", errors.New("请选择有效的机器人目录")
	}
	r.mu.Lock()
	current := r.webURL
	running := r.webCmd != nil
	r.mu.Unlock()
	if running && current != "" {
		return current, nil
	}
	if err := r.stop(ctx, true); err != nil {
		return "", err
	}
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		return "", err
	}
	lock, err := acquireRuntimeLock(r.dir)
	if err != nil {
		return "", err
	}
	var cmd *exec.Cmd
	if r.webCommand != nil {
		cmd = r.webCommand(context.Background(), r.dir)
	} else {
		cmd, err = defaultWebCommand(context.Background(), r.dir)
	}
	if err != nil {
		unlockRuntime(lock)
		return "", err
	}
	cmd.Dir = root
	cmd.Env = managedEnvironment(r.dir, "", Config{APIKey: apiKey})
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		unlockRuntime(lock)
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		unlockRuntime(lock)
		return "", err
	}
	if err := cmd.Start(); err != nil {
		unlockRuntime(lock)
		return "", errors.New("无法启动 DSH Web 版，请检查受管程序和 Node.js")
	}
	ready := make(chan string, 1)
	done := make(chan struct{})
	r.mu.Lock()
	r.webCmd, r.webDone, r.webMode, r.webURL = cmd, done, true, ""
	r.mu.Unlock()
	var readers sync.WaitGroup
	readers.Add(2)
	read := func(source io.Reader) {
		defer readers.Done()
		scanner := bufio.NewScanner(source)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			if address := webLaunchURL(scanner.Text()); address != "" {
				select {
				case ready <- address:
				default:
				}
			}
			// Output may contain credentials and conversation content; do not log it.
		}
	}
	go read(stdout)
	go read(stderr)
	go func() {
		_ = cmd.Wait()
		readers.Wait()
		r.mu.Lock()
		if r.webCmd == cmd {
			r.webCmd, r.webURL = nil, ""
		}
		unlockRuntime(lock)
		close(done)
		r.mu.Unlock()
	}()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case address := <-ready:
		r.mu.Lock()
		if r.webCmd != cmd {
			r.mu.Unlock()
			return "", errors.New("DSH Web 版已退出，请重试")
		}
		r.webURL = address
		r.mu.Unlock()
		return address, nil
	case <-done:
		return "", errors.New("DSH Web 版启动失败，请检查受管程序和 Node.js")
	case <-ctx.Done():
		_ = r.stopWeb(context.Background())
		return "", ctx.Err()
	case <-timer.C:
		_ = r.stopWeb(context.Background())
		return "", errors.New("DSH Web 版启动超时，请重试")
	}
}

func webLaunchURL(line string) string {
	_, address, ok := strings.Cut(line, "dsh web: ")
	if !ok {
		return ""
	}
	fields := strings.Fields(address)
	if len(fields) == 0 {
		return ""
	}
	target, err := url.Parse(fields[0])
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.User != nil || target.Path != "/" || target.Fragment != "" {
		return ""
	}
	port, err := strconv.Atoi(target.Port())
	if err != nil || port < 1 || port > 65535 || target.Query().Get("token") == "" {
		return ""
	}
	return target.String()
}

func (r *Runtime) stopWeb(ctx context.Context) error {
	r.mu.Lock()
	cmd, done := r.webCmd, r.webDone
	r.mu.Unlock()
	if cmd == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
