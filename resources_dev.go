//go:build alxdev

package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"alemonx/internal/processutil"
	"alemonx/internal/resources"
)

//go:embed all:resources/templates all:resources/nvm all:resources/packages/yarn all:resources/packages/pm2
var developmentBase embed.FS

// Only the optional DSH archive is live. Never embed a placeholder archive or
// require npm dependencies to exist before the development server compiles.
type developmentResourceFS struct {
	fs.FS
	disk fs.FS
}

func (f developmentResourceFS) Open(name string) (fs.File, error) {
	if name == "resources/dsh/runtime.zip" {
		return f.disk.Open(name)
	}
	return f.FS.Open(name)
}

var resourceFiles fs.FS = developmentResourceFS{FS: developmentBase, disk: os.DirFS(".")}
var developmentDSHOnce sync.Once

func prepareDevelopmentDSH(ctx context.Context) {
	developmentDSHOnce.Do(func() {
		go func() {
			resources.SetDSHPreparation(fmt.Errorf("DSH 正在后台准备，请稍后重试连接"))
			ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			defer cancel()
			logPath := filepath.Join("resources", "dsh", "prepare.log")
			err := os.MkdirAll(filepath.Dir(logPath), 0700)
			if err == nil {
				var output *os.File
				output, err = os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
				if err == nil {
					log.Printf("DSH 后台准备已开始，不影响工作台使用；详细日志：%s", logPath)
					err = runDevelopmentDSH(ctx, func(ctx context.Context, dir, name string, args ...string) error {
						cmd := exec.CommandContext(ctx, name, args...)
						cmd.Dir, cmd.Stdout, cmd.Stderr = dir, output, output
						cmd.WaitDelay = 3 * time.Second
						processutil.HideWindow(cmd)
						return cmd.Run()
					})
					_ = output.Close()
				}
			}
			if err != nil {
				resources.SetDSHPreparation(fmt.Errorf("DSH 后台准备失败，请查看 %s；可运行 make dsh-runtime 后重试连接", logPath))
				log.Printf("DSH 后台准备失败：%v；日志：%s", err, logPath)
				return
			}
			resources.SetDSHPreparation(nil)
			if _, err := resources.DSHProgram(); err != nil {
				log.Printf("DSH 程序已打包，但工作区释放失败：%v", err)
				return
			}
			log.Print("DSH 后台准备完成，可连接使用，无需重启工作台")
		}()
	})
}

func runDevelopmentDSH(ctx context.Context, run func(context.Context, string, string, ...string) error) error {
	for _, step := range []struct {
		dir, name string
		args      []string
	}{
		{"resources/packages/dsh", "npm", []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}},
		{"resources/packages/dsh", "npm", []string{"run", "check"}},
		{".", "go", []string{"run", "./scripts/pack-dsh.go"}},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := run(ctx, step.dir, step.name, step.args...); err != nil {
			return fmt.Errorf("%s %v: %w", step.name, step.args, err)
		}
	}
	return nil
}
