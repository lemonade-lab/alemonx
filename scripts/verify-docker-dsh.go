//go:build ignore

// Runs only in the Docker verification stage, without sending a model prompt.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"alemonx/internal/dsh"
	"alemonx/internal/resources"
	"alemonx/internal/workspace"
)

func main() {
	if err := verify(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func verify() error {
	const root = "/tmp/alx-dsh-verification"
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	resources.Init(os.DirFS("/opt/alx-verification/resources"), workspace.Layout{Root: root})
	r := dsh.New(filepath.Join(root, "dsh", "runtimes", "smoke"), nil)
	cfg := dsh.Config{Provider: "deepseek-official", Model: "deepseek-chat", Root: root, APIKey: "initialization-only-not-a-real-key"}
	checkpoint := filepath.Join(root, "session-id")
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		id, err := os.ReadFile(checkpoint)
		if err != nil {
			return err
		}
		if !r.HasSession(string(id)) {
			return fmt.Errorf("DSH 会话未持久化")
		}
		if _, err := r.LoadPersistedConfig(); err != nil {
			return err
		}
	} else if err := r.SaveConfig(cfg); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := r.Start(ctx, cfg); err != nil {
		return err
	}
	defer r.Stop(context.Background())
	if !r.Status().Ready {
		return fmt.Errorf("DSH SDK 未就绪")
	}
	if len(os.Args) == 1 {
		id, err := r.CreateSession(ctx)
		if err != nil {
			return err
		}
		return os.WriteFile(checkpoint, []byte(id), 0600)
	}
	return nil
}
