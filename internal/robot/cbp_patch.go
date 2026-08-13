package robot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureCBPIPCActionBridge installs a narrowly-scoped compatibility fix for
// AlemonJS releases that run the platform adapter in a forked IPC process.
// Those releases route browser Action requests only to WebSocket platform
// clients, even though the active qq-bot handler lives in platformChild. The
// patch forwards full-receive client messages to that child. It is idempotent
// and deliberately refuses an unknown upstream file instead of guessing.
func EnsureCBPIPCActionBridge(root string) (bool, error) {
	projectRoot, err := projectPath(root)
	if err != nil {
		return false, err
	}
	path := filepath.Join(projectRoot, "node_modules", "alemonjs", "lib", "core", "cbp", "server", "main.js")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("读取 AlemonJS CBP 服务失败：%w", err)
	}
	const importNeedle = "import { getClientChild } from '../../process/ipc-bridge.js';"
	const importReplacement = "import { getClientChild, getPlatformChild } from '../../process/ipc-bridge.js';"
	const fullReceiveNeedle = `const setFullClient = (originId, ws) => {
    fullClient.set(originId, ws);`
	const forwardNeedle = `        if (platformClient.size > 0) {
            platformClient.forEach((platformWs, platformId) => {
                if (platformWs.readyState === WebSocket.OPEN) {
                    platformWs.send(message);
                }
                else {
                    platformClient.delete(platformId);
                }
            });
        }`
	const forwardReplacement = `        if (platformClient.size > 0) {
            platformClient.forEach((platformWs, platformId) => {
                if (platformWs.readyState === WebSocket.OPEN) {
                    platformWs.send(message);
                }
                else {
                    platformClient.delete(platformId);
                }
            });
        }
        // Fork/IPC platform adapters (the default for the local qq-bot) are
        // not represented in platformClient. Forward browser Actions to that
        // active child as well so its registered onactions handlers run.
        const platformChild = getPlatformChild();
        if (platformChild?.connected) {
            try {
                const parsed = flattedJSON.parse(String(message));
                platformChild.send({ type: 'ipc:data', data: parsed });
            }
            catch {
            }
        }`

	value := string(content)
	if strings.Contains(value, "getPlatformChild") && strings.Contains(value, "Fork/IPC platform adapters") {
		return false, nil
	}
	fullReceiveStart := strings.Index(value, fullReceiveNeedle)
	if !strings.Contains(value, importNeedle) || fullReceiveStart < 0 {
		return false, errors.New("当前 AlemonJS 版本的 CBP 结构不受此兼容修复支持，请升级 AlemonJS")
	}
	forwardOffset := strings.Index(value[fullReceiveStart:], forwardNeedle)
	if forwardOffset < 0 {
		return false, errors.New("当前 AlemonJS 版本的 CBP 结构不受此兼容修复支持，请升级 AlemonJS")
	}
	forwardStart := fullReceiveStart + forwardOffset
	value = strings.Replace(value, importNeedle, importReplacement, 1)
	// The same forwarding block exists for ordinary child clients. Replace the
	// instance inside setFullClient only: this window is the full-receive
	// browser transport and must not duplicate a normal child's delivery.
	forwardStart += len(importReplacement) - len(importNeedle)
	value = value[:forwardStart] + strings.Replace(value[forwardStart:], forwardNeedle, forwardReplacement, 1)
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return false, fmt.Errorf("写入 AlemonJS CBP 兼容修复失败：%w", err)
	}
	return true, nil
}
