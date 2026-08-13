package robot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cbpServerFixture = `import { getClientChild } from '../../process/ipc-bridge.js';
const setChildrenClient = (originId, ws) => {
    ws.on('message', (message) => {
        if (platformClient.size > 0) {
            platformClient.forEach((platformWs, platformId) => {
                if (platformWs.readyState === WebSocket.OPEN) {
                    platformWs.send(message);
                }
                else {
                    platformClient.delete(platformId);
                }
            });
        }
    });
};
const setFullClient = (originId, ws) => {
    fullClient.set(originId, ws);
    ws.on('message', (message) => {
        if (global.__sandbox) {
            return;
        }
        if (platformClient.size > 0) {
            platformClient.forEach((platformWs, platformId) => {
                if (platformWs.readyState === WebSocket.OPEN) {
                    platformWs.send(message);
                }
                else {
                    platformClient.delete(platformId);
                }
            });
        }
    });
};
`

func TestEnsureCBPIPCActionBridgePatchesOnce(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	path := filepath.Join(root, "node_modules", "alemonjs", "lib", "core", "cbp", "server", "main.js")
	writeWebViewFixture(t, path, cbpServerFixture)
	changed, err := EnsureCBPIPCActionBridge(root)
	if err != nil || !changed {
		t.Fatalf("first patch = changed %v err %v", changed, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"getClientChild, getPlatformChild", "Fork/IPC platform adapters", "platformChild.send({ type: 'ipc:data', data: parsed })"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("patched source missing %q", want)
		}
	}
	if strings.Contains(string(content[:strings.Index(string(content), "const setFullClient")]), "Fork/IPC platform adapters") {
		t.Fatal("bridge was added to ordinary children instead of full-receive client")
	}
	changed, err = EnsureCBPIPCActionBridge(root)
	if err != nil || changed {
		t.Fatalf("second patch = changed %v err %v", changed, err)
	}
}

func TestEnsureCBPIPCActionBridgeRefusesUnknownSource(t *testing.T) {
	root := t.TempDir()
	writeWebViewFixture(t, filepath.Join(root, "package.json"), `{"name":"bot"}`)
	writeWebViewFixture(t, filepath.Join(root, "node_modules", "alemonjs", "lib", "core", "cbp", "server", "main.js"), "export {};\n")
	if _, err := EnsureCBPIPCActionBridge(root); err == nil {
		t.Fatal("unknown source should be rejected")
	}
}
