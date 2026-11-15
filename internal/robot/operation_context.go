package robot

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Mutating robot tasks own a directory until their worker has exited. Commands
// launched below that directory inherit its cancellation, including Git helpers.
var operationContexts = struct {
	sync.Mutex
	roots map[string]context.Context
}{roots: map[string]context.Context{}}

func BeginOperation(ctx context.Context, root string) (func(), error) {
	root, err := operationRoot(root)
	if err != nil {
		return nil, err
	}
	operationContexts.Lock()
	defer operationContexts.Unlock()
	if _, exists := operationContexts.roots[root]; exists {
		return nil, errors.New("当前目录仍有任务执行中，请先取消或等待完成")
	}
	operationContexts.roots[root] = ctx
	var once sync.Once
	return func() {
		once.Do(func() { operationContexts.Lock(); delete(operationContexts.roots, root); operationContexts.Unlock() })
	}, nil
}

func operationContext(root string) context.Context {
	root, _ = operationRoot(root)
	operationContexts.Lock()
	defer operationContexts.Unlock()
	for path := root; ; path = filepath.Dir(path) {
		if ctx, ok := operationContexts.roots[path]; ok {
			return ctx
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	return context.Background()
}

func operationRoot(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root, nil
}
