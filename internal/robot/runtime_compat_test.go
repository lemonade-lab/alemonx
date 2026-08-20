package robot

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const runtimeForwardFixture = `export function buildForwardMsgCompat(nodes: any[]) {
  const forwardNodes = Array.isArray(nodes) ? nodes : [];
  const parts = buildForwardMsgParts(forwardNodes);
  return {
    __forwardNodes: forwardNodes
  };
}
`

const runtimeOneBotFixture = `export const consume = (parsedMessage: any) => {
  if (![0, 1].includes(parsedMessage?.retcode)) {
    reject(parsedMessage?.data);

    return;
  }
};
`

const runtimeBridgeTSFixture = `async function sendFallback(reply: IPCReply) {
          void sendFn()
            .then(res => sendReplyResult(reply, true, res))
            .catch(() => sendReplyResult(reply, false));
}

async function sendReply(reply: IPCReply) {
  try {
    const result = await ctx.message.send({ format });
    sendReplyResult(reply, true, result);
  } catch {
    sendReplyResult(reply, false);
  }
}

async function handleApiRequest(req: IPCApiRequest, msgId?: string): Promise<void> {
  const { reqId, action, params } = req;
  try {
    const result = await dispatchApi(action, params, msgId);
    manager.sendToWorker({ type: 'api_response', reqId, ok: true, data: result });
  } catch (err: any) {
    manager.sendToWorker({ type: 'api_response', reqId, ok: false, error: err?.message ?? 'Unknown error' });
  }
}
`

func TestPatchYunzaiForwardSourcePreservesArrayContract(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "lf", "\r\n": "crlf"}[newline], func(t *testing.T) {
			fixture := strings.ReplaceAll(runtimeForwardFixture, "\n", newline)
			patched, changed, supported := patchYunzaiForwardSource(fixture)
			if !changed || !supported {
				t.Fatalf("patch = changed %v supported %v", changed, supported)
			}
			for _, want := range []string{
				"Object.defineProperty(forwardNodes, 'toJSON'",
				"value: () => forwardNodes",
				"enumerable: false",
				"const parts = buildForwardMsgParts(forwardNodes);",
			} {
				if !strings.Contains(patched, want) {
					t.Fatalf("patched source missing %q", want)
				}
			}
			again, changed, supported := patchYunzaiForwardSource(patched)
			if changed || !supported || again != patched {
				t.Fatalf("second patch = changed %v supported %v", changed, supported)
			}
		})
	}
}

func TestPatchOneBotActionErrorSourceKeepsCompleteResponse(t *testing.T) {
	patched, changed, supported := patchOneBotActionErrorSource(runtimeOneBotFixture)
	if !changed || !supported {
		t.Fatalf("patch = changed %v supported %v", changed, supported)
	}
	if !strings.Contains(patched, "reject(parsedMessage);") {
		t.Fatal("patched source does not reject the complete OneBot response")
	}
	if strings.Contains(patched, "reject(parsedMessage?.data);") {
		t.Fatal("patched source still discards OneBot response metadata")
	}
}

func TestPatchYunzaiBridgeSourceAddsFailureContext(t *testing.T) {
	tests := []struct {
		name       string
		typescript bool
		fixture    string
	}{
		{name: "typescript", typescript: true, fixture: runtimeBridgeTSFixture},
		{
			name:       "javascript",
			typescript: false,
			fixture: strings.NewReplacer(
				"reply: IPCReply", "reply",
				"req: IPCApiRequest, msgId?: string", "req, msgId",
				": Promise<void>", "",
				"err: any", "err",
			).Replace(runtimeBridgeTSFixture),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			patched, changed, supported := patchYunzaiBridgeSource(test.fixture, test.typescript)
			if !changed || !supported {
				t.Fatalf("patch = changed %v supported %v", changed, supported)
			}
			for _, want := range []string{
				"API 调用失败 action=${action} code=${err?.code ?? err?.retcode",
				"直发回复失败 route=${route} contents=${contentTypes",
				"回复发送失败 route=${route} contents=${contentTypes",
				"err?.message ?? err?.wording ?? 'Unknown error'",
			} {
				if !strings.Contains(patched, want) {
					t.Fatalf("patched source missing %q", want)
				}
			}
			for _, unwanted := range []string{"msgId=${", "id=${reply.id}", "params=${"} {
				if strings.Contains(patched, unwanted) {
					t.Fatalf("patched source logs sensitive context %q", unwanted)
				}
			}
			again, changed, supported := patchYunzaiBridgeSource(patched, test.typescript)
			if changed || !supported || again != patched {
				t.Fatalf("second patch = changed %v supported %v", changed, supported)
			}
		})
	}
}

func TestEnsureRuntimeCompatibilityPatchesOnce(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)

	loaderRoot := filepath.Join(root, "packages", "alemonjs-load-yunzai")
	writeAppPageFixture(t, filepath.Join(loaderRoot, "package.json"), `{"name":"alemonjs-load-yunzai"}`)
	writeAppPageFixture(t, filepath.Join(loaderRoot, "src", "yunzai", "forward.ts"), runtimeForwardFixture)
	writeAppPageFixture(t, filepath.Join(loaderRoot, "src", "yunzai", "bridge.ts"), runtimeBridgeTSFixture)

	oneBotRoot := filepath.Join(root, "node_modules", "@alemonjs", "onebot")
	writeAppPageFixture(t, filepath.Join(oneBotRoot, "package.json"), `{"name":"@alemonjs/onebot"}`)
	writeAppPageFixture(t, filepath.Join(oneBotRoot, "src", "sdk", "api.ts"), runtimeOneBotFixture)

	changed, err := EnsureRuntimeCompatibility(root)
	if err != nil {
		t.Fatal(err)
	}
	wantChanged := []string{
		"Yunzai 合并转发节点 JSON 兼容",
		"Yunzai 桥接错误详情",
		"OneBot 动作错误详情",
	}
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Fatalf("changed = %#v, want %#v", changed, wantChanged)
	}

	changed, err = EnsureRuntimeCompatibility(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("second patch changed = %#v", changed)
	}
}

func TestEnsureRuntimeCompatibilityRefusesUnknownForwardWithoutPartialWrite(t *testing.T) {
	root := t.TempDir()
	writeAppPageFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	loaderRoot := filepath.Join(root, "packages", "alemonjs-load-yunzai")
	writeAppPageFixture(t, filepath.Join(loaderRoot, "package.json"), `{"name":"alemonjs-load-yunzai"}`)
	sourcePath := filepath.Join(loaderRoot, "src", "yunzai", "forward.ts")
	writeAppPageFixture(t, sourcePath, runtimeForwardFixture)
	writeAppPageFixture(t, filepath.Join(loaderRoot, "lib", "yunzai", "forward.js"), "export const changedUpstream = true;\n")

	before, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureRuntimeCompatibility(root); err == nil {
		t.Fatal("unknown forward source should be rejected")
	}
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("supported source was written before another target failed validation")
	}
}
