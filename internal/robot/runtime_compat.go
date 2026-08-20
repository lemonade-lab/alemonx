package robot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type runtimeCompatibilityPatch struct {
	name      string
	path      string
	transform func(string) (string, bool, bool)
}

type runtimeCompatibilityWrite struct {
	name    string
	path    string
	content string
	mode    os.FileMode
}

// EnsureRuntimeCompatibility applies guarded, idempotent compatibility fixes
// to installed runtime packages before a robot starts or rebuilds.
func EnsureRuntimeCompatibility(root string) ([]string, error) {
	projectRoot, err := projectPath(root)
	if err != nil {
		return nil, err
	}
	patches, err := runtimeCompatibilityPatches(projectRoot)
	if err != nil {
		return nil, err
	}
	changed, err := applyRuntimeCompatibilityPatches(projectRoot, patches)
	if err != nil {
		return nil, err
	}
	cbpChanged, err := EnsureCBPIPCActionBridge(root)
	if err != nil {
		return nil, err
	}
	if cbpChanged {
		changed = append(changed, "AlemonJS CBP IPC Action 桥接")
	}
	return changed, nil
}

func runtimeCompatibilityPatches(projectRoot string) ([]runtimeCompatibilityPatch, error) {
	var patches []runtimeCompatibilityPatch
	loaderRoots := []string{
		filepath.Join(projectRoot, "packages", "alemonjs-load-yunzai"),
		filepath.Join(projectRoot, "node_modules", "alemonjs-load-yunzai"),
	}
	for _, packageRoot := range loaderRoots {
		ok, err := packageHasName(packageRoot, "alemonjs-load-yunzai")
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		patches = append(patches,
			runtimeCompatibilityPatch{
				name:      "Yunzai 合并转发节点 JSON 兼容",
				path:      filepath.Join(packageRoot, "src", "yunzai", "forward.ts"),
				transform: patchYunzaiForwardSource,
			},
			runtimeCompatibilityPatch{
				name:      "Yunzai 合并转发节点 JSON 兼容",
				path:      filepath.Join(packageRoot, "lib", "yunzai", "forward.js"),
				transform: patchYunzaiForwardSource,
			},
			runtimeCompatibilityPatch{
				name: "Yunzai 桥接错误详情",
				path: filepath.Join(packageRoot, "src", "yunzai", "bridge.ts"),
				transform: func(value string) (string, bool, bool) {
					return patchYunzaiBridgeSource(value, true)
				},
			},
			runtimeCompatibilityPatch{
				name: "Yunzai 桥接错误详情",
				path: filepath.Join(packageRoot, "lib", "yunzai", "bridge.js"),
				transform: func(value string) (string, bool, bool) {
					return patchYunzaiBridgeSource(value, false)
				},
			},
		)
	}

	oneBotRoots := []string{
		filepath.Join(projectRoot, "node_modules", "@alemonjs", "onebot"),
		filepath.Join(projectRoot, "packages", "onebot"),
		filepath.Join(projectRoot, "packages", "alemonjs", "packages", "onebot"),
	}
	for _, packageRoot := range oneBotRoots {
		ok, err := packageHasName(packageRoot, "@alemonjs/onebot")
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		patches = append(patches,
			runtimeCompatibilityPatch{
				name:      "OneBot 动作错误详情",
				path:      filepath.Join(packageRoot, "src", "sdk", "api.ts"),
				transform: patchOneBotActionErrorSource,
			},
			runtimeCompatibilityPatch{
				name:      "OneBot 动作错误详情",
				path:      filepath.Join(packageRoot, "lib", "sdk", "api.js"),
				transform: patchOneBotActionErrorSource,
			},
		)
	}
	return patches, nil
}

func packageHasName(packageRoot, want string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(packageRoot, "package.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("读取运行包信息失败：%w", err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false, fmt.Errorf("解析 %s/package.json 失败：%w", filepath.Base(packageRoot), err)
	}
	return manifest.Name == want, nil
}

func applyRuntimeCompatibilityPatches(projectRoot string, patches []runtimeCompatibilityPatch) ([]string, error) {
	var staged []runtimeCompatibilityWrite
	for _, patch := range patches {
		data, err := os.ReadFile(patch.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("读取运行兼容目标失败：%w", err)
		}
		updated, changed, supported := patch.transform(string(data))
		if !supported {
			relative, _ := filepath.Rel(projectRoot, patch.path)
			return nil, fmt.Errorf("%s 的源码结构尚未受 AlemonX 兼容层支持：%s", patch.name, filepath.ToSlash(relative))
		}
		if !changed {
			continue
		}
		info, err := os.Stat(patch.path)
		if err != nil {
			return nil, fmt.Errorf("读取运行兼容目标权限失败：%w", err)
		}
		staged = append(staged, runtimeCompatibilityWrite{
			name:    patch.name,
			path:    patch.path,
			content: updated,
			mode:    info.Mode().Perm(),
		})
	}

	changedNames := make([]string, 0, len(staged))
	seenNames := make(map[string]bool)
	for _, patch := range staged {
		if err := os.WriteFile(patch.path, []byte(patch.content), patch.mode); err != nil {
			return nil, fmt.Errorf("写入%s失败：%w", patch.name, err)
		}
		if !seenNames[patch.name] {
			seenNames[patch.name] = true
			changedNames = append(changedNames, patch.name)
		}
	}
	return changedNames, nil
}

func replaceRuntimeCompatibilityBlock(value, needle, replacement string) (string, bool) {
	if strings.Contains(value, needle) {
		return strings.Replace(value, needle, replacement, 1), true
	}
	crlfNeedle := strings.ReplaceAll(needle, "\n", "\r\n")
	if strings.Contains(value, crlfNeedle) {
		crlfReplacement := strings.ReplaceAll(replacement, "\n", "\r\n")
		return strings.Replace(value, crlfNeedle, crlfReplacement, 1), true
	}
	return value, false
}

func patchYunzaiForwardSource(value string) (string, bool, bool) {
	const marker = "AlemonX compatibility: expose icqq-style forwardNodes.toJSON"
	if strings.Contains(value, marker) ||
		(strings.Contains(value, "Object.defineProperty(forwardNodes") && strings.Contains(value, "'toJSON'")) {
		return value, false, true
	}
	const anchor = "const forwardNodes = Array.isArray(nodes) ? nodes : [];"
	if strings.Count(value, anchor) != 1 ||
		!strings.Contains(value, "buildForwardMsgParts(forwardNodes)") {
		return value, false, false
	}
	start := strings.Index(value, anchor)
	lineStart := strings.LastIndex(value[:start], "\n") + 1
	indent := value[lineStart:start]
	if strings.TrimSpace(indent) != "" {
		return value, false, false
	}
	newline := "\n"
	if strings.Contains(value, "\r\n") {
		newline = "\r\n"
	}
	insertAt := start + len(anchor)
	block := newline + newline +
		indent + "// AlemonX compatibility: expose icqq-style forwardNodes.toJSON without" + newline +
		indent + "// changing array identity or adding an enumerable message node." + newline +
		indent + "if (typeof Reflect.get(forwardNodes, 'toJSON') !== 'function') {" + newline +
		indent + "  Object.defineProperty(forwardNodes, 'toJSON', {" + newline +
		indent + "    value: () => forwardNodes," + newline +
		indent + "    enumerable: false," + newline +
		indent + "    configurable: true" + newline +
		indent + "  });" + newline +
		indent + "}"
	return value[:insertAt] + block + value[insertAt:], true, true
}

func patchYunzaiBridgeSource(value string, typescript bool) (string, bool, bool) {
	changed := false
	errorType := ""
	if typescript {
		errorType = ": any"
	}

	if !strings.Contains(value, "AlemonX compatibility: retain Yunzai API action errors") {
		needle := "  } catch (err" + errorType + ") {\n    manager.sendToWorker({ type: 'api_response', reqId, ok: false, error: err?.message ?? 'Unknown error' });\n  }"
		replacement := "  } catch (err" + errorType + ") {\n" +
			"    // AlemonX compatibility: retain Yunzai API action errors without logging params or message contents.\n" +
			"    logger.error(\n" +
			"      `[bridge] API 调用失败 action=${action} code=${err?.code ?? err?.retcode ?? '-'}: ${err?.message ?? err?.wording ?? String(err)}`\n" +
			"    );\n" +
			"    manager.sendToWorker({ type: 'api_response', reqId, ok: false, error: err?.message ?? err?.wording ?? 'Unknown error' });\n" +
			"  }"
		var applied bool
		value, applied = replaceRuntimeCompatibilityBlock(value, needle, replacement)
		changed = changed || applied
	}

	if !strings.Contains(value, "AlemonX compatibility: report direct reply failures") {
		needle := "            .catch(() => sendReplyResult(reply, false));"
		replacement := "            .catch((err" + errorType + ") => {\n" +
			"              // AlemonX compatibility: report direct reply failures without logging targets or message contents.\n" +
			"              const contentTypes = reply.contents.map(content => content.type).join(',');\n" +
			"              const route = (reply.isPrivate ?? true) ? 'private' : 'group';\n" +
			"              logger.error(\n" +
			"                `[bridge] 直发回复失败 route=${route} contents=${contentTypes || '-'} code=${err?.code ?? err?.retcode ?? '-'}: ${err?.message ?? err?.wording ?? String(err)}`\n" +
			"              );\n" +
			"              sendReplyResult(reply, false);\n" +
			"            });"
		var applied bool
		value, applied = replaceRuntimeCompatibilityBlock(value, needle, replacement)
		changed = changed || applied
	}

	if !strings.Contains(value, "AlemonX compatibility: report reply routing failures") {
		needle := "  } catch {\n    sendReplyResult(reply, false);\n  }"
		replacement := "  } catch (err" + errorType + ") {\n" +
			"    // AlemonX compatibility: report reply routing failures without logging targets or message contents.\n" +
			"    const contentTypes = reply.contents.map(content => content.type).join(',');\n" +
			"    const route = (reply.isPrivate ?? true) ? 'private' : 'group';\n" +
			"    logger.error(\n" +
			"      `[bridge] 回复发送失败 route=${route} contents=${contentTypes || '-'} code=${err?.code ?? err?.retcode ?? '-'}: ${err?.message ?? err?.wording ?? String(err)}`\n" +
			"    );\n" +
			"    sendReplyResult(reply, false);\n" +
			"  }"
		var applied bool
		value, applied = replaceRuntimeCompatibilityBlock(value, needle, replacement)
		changed = changed || applied
	}

	// This observability patch is optional on versions whose bridge has already
	// changed shape. The forward-node patch above remains the guarded behavior
	// fix and rejects unknown source instead of guessing.
	return value, changed, true
}

func patchOneBotActionErrorSource(value string) (string, bool, bool) {
	const marker = "AlemonX compatibility: reject the complete OneBot response"
	if strings.Contains(value, marker) {
		return value, false, true
	}
	if strings.Contains(value, "reject(parsedMessage)") {
		return value, false, true
	}
	const condition = "if (![0, 1].includes(parsedMessage?.retcode))"
	const rejectData = "reject(parsedMessage?.data);"
	if strings.Count(value, condition) != 1 || strings.Count(value, rejectData) != 1 {
		return value, false, false
	}
	rejectStart := strings.Index(value, rejectData)
	lineStart := strings.LastIndex(value[:rejectStart], "\n") + 1
	indent := value[lineStart:rejectStart]
	if strings.TrimSpace(indent) != "" {
		return value, false, false
	}
	newline := "\n"
	if strings.Contains(value, "\r\n") {
		newline = "\r\n"
	}
	replacement := indent + "// AlemonX compatibility: reject the complete OneBot response so callers" + newline +
		indent + "// keep retcode, wording and action details instead of receiving null data." + newline +
		indent + "reject(parsedMessage);"
	return value[:lineStart] + replacement + value[rejectStart+len(rejectData):], true, true
}
