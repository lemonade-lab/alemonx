package web

import (
	"net/url"
	"os/exec"
	"strings"
	"testing"
)

func TestBrowserRuntimeMapsWorkbenchWebSocketsAndAlreadyProxiedURLs(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js unavailable")
	}
	target, _ := url.Parse("http://127.0.0.1:4242")
	script := `
const assert = require('node:assert/strict');
globalThis.window = globalThis;
globalThis.location = { protocol: 'http:', host: 'workbench:8090', origin: 'http://workbench:8090', pathname: '/', search: '', hash: '' };
globalThis.parent = { postMessage() {} };
globalThis.document = { addEventListener() {} };
window.Element = class { setAttribute(name, value) { this[name + 'Attribute'] = value; } };
window.HTMLScriptElement = class extends Element {};
window.HTMLLinkElement = class extends Element {};
for (const [Ctor, property] of [[HTMLScriptElement, 'src'], [HTMLLinkElement, 'href']]) {
  Object.defineProperty(Ctor.prototype, property, { configurable:true, enumerable:true,
    get() { return this['_' + property]; }, set(value) { this['_' + property] = value; } });
}

globalThis.history = { pushState() {}, replaceState() {} };
globalThis.addEventListener = () => {};
Object.defineProperty(globalThis, 'navigator', { value: {} });
const sent = [];
window.fetch = input => { sent.push(String(input)); };
window.XMLHttpRequest = function() {};
window.WebSocket = function(input) { sent.push(input); };
window.open = () => {};
` + browserHTTPRuntime("test", target, "/") + `
new WebSocket('ws://workbench:8090/api/remote.mux');
assert.equal(sent.pop(), 'ws://workbench:8090/api/v1/browser/http/aHR0cDovLzEyNy4wLjAuMTo0MjQy/api/remote.mux');
const mounted = 'http://workbench:8090/api/v1/browser/http/aHR0cDovLzEyNy4wLjAuMTo0MjQy/api/settings';
const bundle = '/plugins/??@deepseek-ai/dsh-client-hmr/client.js,@alemonx/dsh-workspace-entry/client.js&rev=test';
const expectedBundle = '/api/v1/browser/http/aHR0cDovLzEyNy4wLjAuMTo0MjQy' + bundle;
const scriptElement = new HTMLScriptElement();
scriptElement.src = bundle;
assert.equal(scriptElement.src, expectedBundle);
scriptElement.setAttribute('src', bundle);
assert.equal(scriptElement.srcAttribute, expectedBundle);
scriptElement.src = expectedBundle;
assert.equal(scriptElement.src, expectedBundle);
scriptElement.src = 'https://cdn.example.com/client.js';
assert.equal(scriptElement.src, 'https://cdn.example.com/client.js');
const stylesheet = new HTMLLinkElement();
stylesheet.href = '/plugins/theme.css';
assert.equal(stylesheet.href, '/api/v1/browser/http/aHR0cDovLzEyNy4wLjAuMTo0MjQy/plugins/theme.css');
fetch(new URL(mounted));
assert.equal(sent.pop(), mounted);
new WebSocket(mounted.replace('http:', 'ws:'));
assert.equal(sent.pop(), mounted.replace('http:', 'ws:'));
`
	output, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("browser runtime URL mapping failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}
