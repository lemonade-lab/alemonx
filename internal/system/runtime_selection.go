package system

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Selections belong to ALemonX, independently of the user's NVM/pyenv defaults.
type runtimeSelection struct {
	Node   string `json:"node,omitempty"`
	Python string `json:"python,omitempty"`
}

var runtimeSelectionMu sync.Mutex

func runtimeSelectionPath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".alemonx", "runtime-selection.json"), nil
}

func readRuntimeSelection() (runtimeSelection, error) {
	var selection runtimeSelection
	path, err := runtimeSelectionPath()
	if err != nil {
		return selection, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return selection, nil
	}
	if err != nil {
		return selection, err
	}
	err = json.Unmarshal(data, &selection)
	return selection, err
}

func writeRuntimeSelection(selection runtimeSelection) error {
	path, err := runtimeSelectionPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".runtime-selection-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	data, err := json.Marshal(selection)
	if err == nil {
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

func selectRuntime(kind, bin string) error {
	runtimeSelectionMu.Lock()
	defer runtimeSelectionMu.Unlock()
	selection, err := readRuntimeSelection()
	if err != nil {
		return err
	}
	previous := os.Getenv("PATH")
	switch kind {
	case "node":
		if _, err = ApplyNodeRuntime(bin); err != nil {
			return err
		}
		selection.Node = bin
	case "python":
		if err = applyPythonRuntime(bin); err != nil {
			return err
		}
		selection.Python = bin
	default:
		return fmt.Errorf("未知运行时：%s", kind)
	}
	if err = writeRuntimeSelection(selection); err != nil {
		_ = os.Setenv("PATH", previous)
		_, _ = ConfigureNodeRuntime()
		return fmt.Errorf("保存运行时选择失败，已恢复原环境：%w", err)
	}
	return nil
}

func applyPythonRuntime(bin string) error {
	if !filepath.IsAbs(bin) {
		return fmt.Errorf("Python 运行时必须使用绝对路径")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, filepath.Join(bin, "python3"), "--version").Output()
	fields := strings.Fields(string(output))
	if err != nil || len(fields) != 2 || fields[0] != "Python" || !pythonVersion(fields[1]) {
		return fmt.Errorf("所选 Python 无法正常运行")
	}
	// Keep all existing Node directories; a Python switch must never remove NVM.
	previous := os.Getenv("PATH")
	node, nodeErr := CurrentNodeRuntime()
	entries := []string{bin}
	for _, entry := range filepath.SplitList(previous) {
		if entry != "" && filepath.Clean(entry) != filepath.Clean(bin) {
			entries = append(entries, entry)
		}
	}
	path := strings.Join(entries, string(os.PathListSeparator))
	if err := os.Setenv("PATH", path); err != nil {
		return err
	}
	if nodeErr == nil {
		after, err := CurrentNodeRuntime()
		if err != nil || after.Path != node.Path {
			_ = os.Setenv("PATH", previous)
			return fmt.Errorf("Python 目录包含冲突的 Node.js，已保留原环境")
		}
	}
	_, _ = ConfigureNodeRuntime()
	return nil
}

func RestorePythonRuntime() error {
	// Container Python belongs to the image, including after a /root restore.
	if InContainer() {
		return nil
	}
	runtimeSelectionMu.Lock()
	defer runtimeSelectionMu.Unlock()
	selection, err := readRuntimeSelection()
	if err != nil {
		return err
	}
	if selection.Python == "" {
		return nil
	}
	return applyPythonRuntime(selection.Python)
}
