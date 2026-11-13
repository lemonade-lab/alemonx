package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const readyPrefix = "ALX_DESKTOP_READY="

type workbench struct {
	url      string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	done     chan struct{}
	exitErr  error
	stopOnce sync.Once
}

func startWorkbench(workspace string, port int) (*workbench, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("本地工作台端口超出范围")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("无法定位用户配置目录：%w", err)
	}
	configDir = filepath.Join(configDir, "AlemonX Desktop")
	if workspace == "" {
		workspace = os.Getenv("ALX_WORKSPACE")
	}
	if workspace == "" {
		workspace = filepath.Join(configDir, "workspace")
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("无法解析工作区目录：%w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("无法创建桌面配置目录：%w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("无法定位桌面程序：%w", err)
	}
	name := "alx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(executable), name), "serve", "--desktop",
		"--host", "127.0.0.1", "--port", strconv.Itoa(port), "--workspace", workspace)
	configureWorkbenchProcess(cmd)
	cmd.Dir = configDir
	cmd.Env = append(os.Environ(), "ALEMONX_PROCESS_FILE="+filepath.Join(configDir, "processes.json"))
	cmd.Stderr = os.Stderr

	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = output.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		_ = stdin.Close()
		return nil, fmt.Errorf("无法启动工作台服务：%w", err)
	}

	backend := &workbench{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	ready := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(output)
		for {
			line, err := reader.ReadString('\n')
			if address, found := strings.CutPrefix(strings.TrimSpace(line), readyPrefix); found {
				ready <- address
				_, _ = io.Copy(os.Stdout, reader)
				break
			}
			_, _ = io.WriteString(os.Stdout, line)
			if err != nil {
				break
			}
		}
		backend.exitErr = cmd.Wait()
		close(backend.done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case address := <-ready:
		host, actualPort, err := net.SplitHostPort(address)
		boundPort, portErr := strconv.Atoi(actualPort)
		if err != nil || host != "127.0.0.1" || portErr != nil || boundPort < 1 || boundPort > 65535 {
			backend.stop()
			return nil, fmt.Errorf("工作台返回了无效的本机地址")
		}
		backend.url = "http://" + address
		if err := backend.waitReady(ctx); err != nil {
			backend.stop()
			return nil, err
		}
		return backend, nil
	case <-backend.done:
		return nil, backend.startupError()
	case <-ctx.Done():
		backend.stop()
		return nil, fmt.Errorf("等待工作台启动超时，请检查端口 %d 是否被占用", port)
	}
}

func (b *workbench) waitReady(ctx context.Context) error {
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url+"/healthz", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-b.done:
			return b.startupError()
		case <-ctx.Done():
			return fmt.Errorf("等待工作台响应超时")
		case <-ticker.C:
		}
	}
}

func (b *workbench) startupError() error {
	if b.exitErr != nil {
		return fmt.Errorf("工作台服务提前退出，请检查工作区和端口：%w", b.exitErr)
	}
	return fmt.Errorf("工作台服务提前退出")
}

func (b *workbench) stop() {
	b.stopOnce.Do(func() {
		_ = b.stdin.Close()
		select {
		case <-b.done:
		case <-time.After(12 * time.Second):
			log.Print("工作台服务未能及时退出，正在结束服务")
			_ = b.cmd.Process.Kill()
			<-b.done
		}
	})
}
