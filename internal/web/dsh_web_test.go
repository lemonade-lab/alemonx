package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"alemonx/internal/dsh"
	"alemonx/internal/resources"
	"alemonx/internal/workspace"
)

func TestDSHWebRejectsInvalidRoots(t *testing.T) {
	s := &server{dshRuntimes: dsh.NewRegistry(t.TempDir(), nil)}
	for _, token := range []string{"invalid", base64.RawURLEncoding.EncodeToString([]byte("relative/path")), base64.RawURLEncoding.EncodeToString([]byte(filepath.Join(t.TempDir(), "missing")))} {
		response := httptest.NewRecorder()
		s.dshHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/dsh/runtimes/"+token+"/web", nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid root accepted: %d", response.Code)
		}
	}
}

func TestDSHWebRejectsUnreadableSecret(t *testing.T) {
	t.Setenv("ALX_DSH_SECRET_FILE", filepath.Join(t.TempDir(), "missing-secret"))
	s := &server{dshRuntimes: dsh.NewRegistry(t.TempDir(), nil)}
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"web-test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.dshHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/dsh/runtimes/"+robotAppToken(root)+"/web", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing secret accepted: %d", response.Code)
	}
}

// Opt in after packing the managed runtime: this starts the real official Web
// composition and exercises the production opener and existing browser proxy.
func TestDSHOfficialWebThroughBrowserProxy(t *testing.T) {
	if os.Getenv("ALX_DSH_WEB_SMOKE") != "1" {
		t.Skip("requires packed DSH archive and Node.js")
	}
	t.Setenv("ALX_DSH_BIN", "")
	t.Setenv("ALX_DSH_SECRET_FILE", "")
	layout := workspace.Layout{Root: t.TempDir()}
	resources.Init(os.DirFS("../../resources"), layout)
	defer resources.Init(nil, workspace.Layout{})
	// Production prepares the archive in the background before the opener.
	// Keep cold extraction outside the HTTP startup deadline in this smoke test.
	if _, err := resources.DSHProgram(); err != nil {
		t.Fatal(err)
	}
	registry := dsh.NewRegistry(filepath.Join(layout.Root, "dsh"), nil)
	defer registry.StopAll(context.Background())
	s := &server{dshRuntimes: registry}
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"web-test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	s.dshHandler(response, httptest.NewRequest(http.MethodPost, "/api/v1/dsh/runtimes/"+robotAppToken(root)+"/web", nil))
	if response.Code != 200 {
		t.Fatalf("opener failed: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("launch token response must not be cached")
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(result.URL)
	if err != nil || target.Hostname() != "127.0.0.1" || target.Query().Get("token") == "" {
		t.Fatal("invalid managed launch URL")
	}
	// Mirror Vite: only /api reaches the backend; escaped plugin paths get HTML.
	backend := s.browserHTTPRecovery(http.HandlerFunc(s.browserHTTPProxyHandler))
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			backend.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<!doctype html><html><body>Workbench development fallback</body></html>")
	}))
	defer proxy.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	prefix := proxy.URL + "/api/v1/browser/http/" + base64.RawURLEncoding.EncodeToString([]byte(target.Scheme+"://"+target.Host)) + "/"
	index, err := client.Get(prefix + "?" + target.RawQuery)
	if err != nil {
		t.Fatal(err)
	}
	html, err := io.ReadAll(index.Body)
	index.Body.Close()
	if err != nil || index.StatusCode != 200 || !strings.Contains(string(html), "<html") {
		t.Fatalf("official index failed through proxy: %d", index.StatusCode)
	}
	match := regexp.MustCompile(`src="([^"]+\.js)"`).FindSubmatch(html)
	if len(match) < 2 {
		t.Fatal("official Web entry asset missing")
	}
	proxyURL, _ := url.Parse(proxy.URL)
	assetURL, err := proxyURL.Parse(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	asset, err := client.Get(assetURL.String())
	if err != nil {
		t.Fatal(err)
	}
	asset.Body.Close()
	if asset.StatusCode != 200 {
		t.Fatalf("official JS asset failed through proxy: %d", asset.StatusCode)
	}
	clean, err := client.Get(prefix)
	if err != nil {
		t.Fatal(err)
	}
	clean.Body.Close()
	if clean.StatusCode != 200 {
		t.Fatalf("isolated auth cookie not restored: %d", clean.StatusCode)
	}
	if os.Getenv("ALX_DSH_WEB_BROWSER_SMOKE") == "1" {
		secondRoot := filepath.Join(root, "switched-project")
		if err := os.Mkdir(secondRoot, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(secondRoot, "package.json"), []byte(`{"name":"switched-project"}`), 0600); err != nil {
			t.Fatal(err)
		}
		secondResponse := httptest.NewRecorder()
		s.dshHandler(secondResponse, httptest.NewRequest(http.MethodPost, "/api/v1/dsh/runtimes/"+robotAppToken(secondRoot)+"/web", nil))
		if secondResponse.Code != 200 {
			t.Fatalf("second project failed: %s", secondResponse.Body.String())
		}
		var secondResult struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(secondResponse.Body.Bytes(), &secondResult); err != nil {
			t.Fatal(err)
		}
		secondTarget, err := url.Parse(secondResult.URL)
		if err != nil {
			t.Fatal(err)
		}
		secondPrefix := proxy.URL + "/api/v1/browser/http/" + base64.RawURLEncoding.EncodeToString([]byte(secondTarget.Scheme+"://"+secondTarget.Host)) + "/"
		otherRoot := filepath.Join(root, "other-project")
		if err := os.Mkdir(otherRoot, 0700); err != nil {
			t.Fatal(err)
		}
		browser := exec.Command("node", "--input-type=module", "-e", `
import { chromium } from '@playwright/test';
const browser = await chromium.launch({ channel: 'chrome' });
try {
  const context = await browser.newContext();
  const page = await context.newPage();
  const failures = [];
  const requests = [];
  let created = 0;
  page.on('request', request => { if (new URL(request.url()).pathname.endsWith('/api/session/create')) created++; });
  page.on('response', response => { if (response.status() >= 400) requests.push(response.status() + ' ' + new URL(response.url()).pathname); });
  page.on('pageerror', error => failures.push(error.message));
  page.on('console', message => { if (message.type() === 'error') failures.push(message.text().replace(/token=[^&\s]+/g, 'token=redacted')); });
  page.on('requestfailed', request => requests.push('failed ' + new URL(request.url()).pathname));
  const parallel = await page.context().newPage();
  await Promise.all([page.goto(process.env.ALX_WEB_PROXY_URL), parallel.goto(process.env.ALX_WEB_PROXY_URL)]);
  await parallel.locator('[contenteditable="true"], textarea:not([disabled])').first().waitFor({ timeout:15000 });
  await parallel.close();
  try {
    await page.getByRole('button', { name: '设置', exact: true }).waitFor({ timeout: 15000 });
    await page.locator('[contenteditable="true"], textarea:not([disabled])').first().waitFor({ timeout: 15000 });
    if (await page.locator('html').getAttribute('lang') !== 'zh-CN') throw new Error('default locale is not Chinese');
    const target = await page.evaluate(() => window.__ALX_DSH_WORKSPACE__);
    if (!target || target.path !== process.env.ALX_WEB_PROJECT_ROOT) throw new Error('wrong default workspace');
    await page.waitForFunction(() => !document.body.innerText.includes('Reconnecting'), null, { timeout: 10000 });
  } catch {
    throw new Error(JSON.stringify({ body: await page.locator('body').innerText(), failures, requests }));
  }
  if (await page.getByRole('button', { name: '继续', exact: true }).isVisible()) await page.getByRole('button', { name: '继续', exact: true }).click();
  const selection = () => {
    const key = Array.from({length: localStorage.length}, (_, i) => localStorage.key(i)).find(key => key.includes('dsh.sessions.current'));
    return key ? localStorage.getItem(key) : null;
  };
  const current = await page.evaluate(selection);
  if (!current) throw new Error('official session selection was not persisted');
  const firstCreated = created;
  await page.reload();
  await page.locator('[contenteditable="true"], textarea:not([disabled])').first().waitFor({ timeout: 15000 });
  await page.waitForFunction(() => !document.body.innerText.includes('Reconnecting'), null, { timeout: 10000 });
  const restored = await page.evaluate(selection);
  if (created !== firstCreated || restored !== current) throw new Error('reopening created or switched session');
  const target = await page.evaluate(() => window.__ALX_DSH_WORKSPACE__);
  const rpc = async (endpoint, args) => {
    const envelope = await page.evaluate(async ({endpoint, args}) => {
      const response = await fetch('/api/' + endpoint, { method: 'POST', headers: {'content-type':'application/json'}, body: JSON.stringify({type:'client-request', rpcId:crypto.randomUUID(), method:endpoint, payload:{args}}) });
      return response.json();
    }, {endpoint, args});
    if (!envelope.result?.ok) throw new Error('official RPC failed: ' + endpoint + ' ' + JSON.stringify(envelope.result));
    return envelope.result.value;
  };
  await rpc('session/create', {request:{sessionId:'alx-project-latest', workspaceId:target.workspaceId}});
  const foreign = await rpc('workspace/create', {request:{path:process.env.ALX_WEB_OTHER_ROOT}});
  await rpc('session/create', {request:{sessionId:'alx-foreign-newer', workspaceId:foreign.workspace.workspaceId}});
  await rpc('session/create', {request:{sessionId:'alx-project-archived', workspaceId:target.workspaceId}});
  await rpc('workspace/archiveSession', {request:{sessionId:'alx-project-archived'}});
  const seededCount = created;
  await page.reload();
  await page.waitForFunction(() => {
    const key = Array.from({length:localStorage.length},(_,i)=>localStorage.key(i)).find(key=>key.includes('dsh.sessions.current'));
    return key && localStorage.getItem(key).includes('alx-project-latest');
  }, null, { timeout:15000 });
  if (created !== seededCount) throw new Error('opening project history created an extra session');
  await page.goto(process.env.ALX_WEB_SECOND_PROXY_URL);
  await page.locator('[contenteditable="true"], textarea:not([disabled])').first().waitFor({ timeout:15000 });
  const secondTarget = await page.evaluate(() => window.__ALX_DSH_WORKSPACE__);
  if (secondTarget.path !== process.env.ALX_WEB_SECOND_ROOT) throw new Error('project switch used previous directory');
  const secondSelection = await page.evaluate(selection);
  if (secondSelection?.includes('alx-project-latest')) throw new Error('projects shared a selected session');
  const switchedCount = created;
  await page.goto(process.env.ALX_WEB_PROXY_URL);
  await page.waitForFunction(() => {
    const key = Array.from({length:localStorage.length},(_,i)=>localStorage.key(i)).find(key=>key.includes('dsh.sessions.current'));
    return key && localStorage.getItem(key).includes('alx-project-latest');
  }, null, { timeout:15000 });
  if (created !== switchedCount) throw new Error('returning to project created an extra session');
  if (failures.length) throw new Error('Official Web UI reported browser errors');
  console.log('Official Web UI loaded and connected through the production HTTP proxy');
} finally { await browser.close(); }
`)
		browser.Dir = "../../frontend"
		browser.Env = append(os.Environ(), "ALX_WEB_PROXY_URL="+prefix+"?"+target.RawQuery, "ALX_WEB_PROJECT_ROOT="+root, "ALX_WEB_OTHER_ROOT="+otherRoot, "ALX_WEB_SECOND_PROXY_URL="+secondPrefix+"?"+secondTarget.RawQuery, "ALX_WEB_SECOND_ROOT="+secondRoot)
		output, err := browser.CombinedOutput()
		if err != nil {
			t.Fatalf("official Web browser failed: %v\n%s", err, output)
		}
		t.Log(strings.TrimSpace(string(output)))
	}
}
