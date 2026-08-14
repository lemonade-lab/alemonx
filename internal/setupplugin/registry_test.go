package setupplugin

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testHTTPServer struct {
	client *http.Client
	URL    string
}

func (s *testHTTPServer) Close()               {}
func (s *testHTTPServer) Client() *http.Client { return s.client }

func newTestHTTPServer(t *testing.T, handler http.Handler) *testHTTPServer {
	t.Helper()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return &http.Response{StatusCode: recorder.Code, Header: recorder.Header(), Body: io.NopCloser(recorder.Body), Request: request}, nil
	})
	return &testHTTPServer{client: &http.Client{Transport: transport}, URL: "http://test.local"}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRegistryListsValidPluginWithNavigation(t *testing.T) {
	root := t.TempDir()
	docker := filepath.Join(root, "docker")
	if err := os.Mkdir(docker, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"docker","name":"Docker","version":"1.0.0","navigation":{"label":"Docker","icon":"◇","order":10},"entry":{"linux-amd64":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(docker, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	plugin, err := NewRegistry(root).Find("docker")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.Navigation.Label != "Docker" || plugin.Web == nil || plugin.Web.Root != "web" || !plugin.Runnable {
		t.Fatalf("unexpected plugin: %#v", plugin)
	}
}

func TestDecodeManifestAcceptsValidWebRoot(t *testing.T) {
	plugin, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"}}`), "/plugins/demo")
	if err != nil {
		t.Fatalf("valid web root rejected: %v", err)
	}
	if plugin.Web == nil || plugin.Web.Root != "web" {
		t.Fatalf("web root not preserved: %#v", plugin.Web)
	}
}

func TestDecodeManifestKeepsPrivilegedOperation(t *testing.T) {
	plugin, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"privilegedOperations":[{"action":"apply-plan","runnerAction":"apply","title":"应用变更","authorization":"native","platforms":["linux"]}]}`), "/plugins/demo")
	if err != nil {
		t.Fatal(err)
	}
	if operation, ok := plugin.PrivilegedOperation("apply-plan"); !ok || operation.RunnerAction != "apply" {
		t.Fatalf("unexpected privileged operation: %#v", operation)
	}
}

func TestDecodeManifestRejectsInvalidPrivilegedOperation(t *testing.T) {
	_, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"privilegedOperations":[{"action":"apply-plan","title":"应用变更","authorization":"native","platforms":["linux"]}]}`), "/plugins/demo")
	if err == nil {
		t.Fatal("missing runner action must be rejected")
	}
}

func TestDecodeManifestAcceptsOnlyTypedSystemPickers(t *testing.T) {
	plugin, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"systemPickers":[{"id":"runtime-directory","kind":"directory","title":"选择运行目录"},{"id":"config-file","kind":"file"}]}`), "/plugins/demo")
	if err != nil {
		t.Fatal(err)
	}
	picker, ok := plugin.SystemPicker("runtime-directory")
	if !ok || picker.Kind != "directory" || picker.Title != "选择运行目录" {
		t.Fatalf("system picker = %#v, ok=%v", picker, ok)
	}
	if _, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"systemPickers":[{"id":"bad","kind":"command"}]}`), "/plugins/demo"); err == nil {
		t.Fatal("arbitrary picker kinds must be rejected")
	}
}

func TestDecodeManifestAcceptsOnlyDeclaredUploadActions(t *testing.T) {
	plugin, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"uploads":[{"action":"upload","maxBytes":1048576}]}`), "/plugins/demo")
	if err != nil {
		t.Fatal(err)
	}
	upload, ok := plugin.UploadAction("upload")
	if !ok || upload.MaxBytes != 1048576 {
		t.Fatalf("upload declaration = %#v, %t", upload, ok)
	}
	if _, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"uploads":[{"action":"upload","maxBytes":0}]}`), "/plugins/demo"); err == nil {
		t.Fatal("upload declaration without a size limit must be rejected")
	}
}

func TestDecodeManifestAcceptsFixedPrivilegedOperation(t *testing.T) {
	plugin, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"privilegedOperations":[{"action":"prepare-runtime","runnerAction":"prepare-runtime","title":"准备运行环境","authorization":"password","platforms":["linux"],"commands":[{"program":"apt-get","args":["install","-y","xvfb"]}]}]}`), "/plugins/demo")
	if err != nil {
		t.Fatal(err)
	}
	operation, ok := plugin.PrivilegedOperation("prepare-runtime")
	if !ok || operation.Commands[0].Program != "apt-get" {
		t.Fatalf("privileged operation = %#v, ok=%v", operation, ok)
	}
}

func TestDecodeManifestRejectsUnsafePrivilegedOperation(t *testing.T) {
	_, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"web"},"privilegedOperations":[{"action":"prepare-runtime","runnerAction":"prepare-runtime","title":"准备运行环境","authorization":"password","platforms":["linux"],"commands":[{"program":"/bin/sh","args":["-c","echo unsafe"]}]}]}`), "/plugins/demo")
	if err == nil {
		t.Fatal("shell privileged operation must be rejected")
	}
}

func TestRegistryRejectsDirectPrivilegedAction(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"},"privilegedOperations":[{"action":"apply-plan","runnerAction":"apply","title":"应用变更","authorization":"native","platforms":["` + runtime.GOOS + `"]}]}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte("ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := NewRegistry(root).RunResultWithProgress("fixture", "apply-plan", nil, false, nil)
	if err == nil || !strings.Contains(err.Error(), "权限代理") {
		t.Fatalf("direct privileged action = %v", err)
	}
}

func TestDecodeManifestRejectsUnsafeWebRoot(t *testing.T) {
	for _, root := range []string{"/etc", "../escape", "a/../b", "", "  "} {
		if _, err := decodeManifest([]byte(`{"id":"demo","name":"Demo","version":"1.0.0","web":{"root":"`+root+`"}}`), "/plugins/demo"); err == nil {
			t.Fatalf("unsafe web root %q must be rejected", root)
		}
	}
}

func TestRegistryRunsDeclaredBinaryAction(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte(`#!/bin/sh
cat >/dev/null
printf '{"output":"已检查"}'
`), 0755); err != nil {
		t.Fatal(err)
	}
	output, err := NewRegistry(root).Run("fixture", "check", nil, false)
	if err != nil || output != "已检查" {
		t.Fatalf("run = %q, %v", output, err)
	}
}

func TestRegistryProvidesHostApprovedRunnerEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte(`#!/bin/sh
cat >/dev/null
printf '{"output":"%s"}' "$ALX_TEST_HOST_VALUE"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(root)
	registry.SetRunnerEnvironmentProvider(func(plugin Plugin, action string) []string {
		if plugin.ID != "fixture" {
			t.Fatalf("provider plugin = %#v", plugin)
		}
		if action != "check" {
			t.Fatalf("provider action = %q", action)
		}
		return []string{"ALX_TEST_HOST_VALUE=host-approved"}
	})
	output, err := registry.Run("fixture", "check", nil, false)
	if err != nil || output != "host-approved" {
		t.Fatalf("run = %q, %v", output, err)
	}
}

func TestRegistryRunWithProgressForwardsStructuredStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte(`#!/bin/sh
cat >/dev/null
printf '%s\n' '@alx-progress {"stage":"download","percent":25,"message":"正在下载"}' >&2
printf '{"output":"完成"}'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	var received []Progress
	output, err := NewRegistry(root).RunWithProgress("fixture", "install", nil, true, func(event Progress) {
		received = append(received, event)
	})
	if err != nil || output != "完成" {
		t.Fatalf("run = %q, %v", output, err)
	}
	if len(received) != 1 || received[0] != (Progress{Stage: "download", Percent: 25, Message: "正在下载"}) {
		t.Fatalf("progress = %#v", received)
	}
}

func TestRegistryReturnsStructuredActionData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte(`#!/bin/sh
cat >/dev/null
printf '%s' '{"output":"完成","data":{"ready":true}}'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := NewRegistry(root).RunResultWithProgress("fixture", "status", nil, false, nil)
	if err != nil || result.Output != "完成" || string(result.Data) != `{"ready":true}` {
		t.Fatalf("result = %#v, %v", result, err)
	}
}

func TestRegistryRunForwardsExecutorConfirmation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","entry":{"` + key + `":"runner"},"web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "runner"), []byte(`#!/bin/sh
payload="$(cat)"
case "$payload" in *'"confirm":true'*) printf '{"output":"confirmed"}' ;; *) printf '{"error":"confirmation missing"}' ;; esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	output, err := NewRegistry(root).Run("fixture", "install", nil, true)
	if err != nil || output != "confirmed" {
		t.Fatalf("run = %q, %v", output, err)
	}
}

func TestParseProgressRejectsInvalidFrames(t *testing.T) {
	if _, ok := parseProgress(`@alx-progress {"stage":"download","percent":101}`); ok {
		t.Fatal("out-of-range progress must be ignored")
	}
	if _, ok := parseProgress("runner diagnostic"); ok {
		t.Fatal("ordinary stderr must not become progress")
	}
}

func TestRegistrySkipsMalformedPlugin(t *testing.T) {
	root := t.TempDir()
	broken := filepath.Join(root, "broken")
	if err := os.Mkdir(broken, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, manifestName), []byte(`{"id":"bad"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistry(root).Find("bad"); err == nil {
		t.Fatal("malformed plugin must not be discovered")
	}
}

func TestRegistrySkipsPluginWithoutWeb(t *testing.T) {
	root := t.TempDir()
	noweb := filepath.Join(root, "noweb")
	if err := os.Mkdir(noweb, 0755); err != nil {
		t.Fatal(err)
	}
	// A manifest without a web root is no longer a usable setup plugin.
	manifest := `{"id":"noweb","name":"NoWeb","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(noweb, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistry(root).Find("noweb"); err == nil {
		t.Fatal("plugin without web root must not be discovered")
	}
}

func TestRegistryCanDisableAndReenablePlugin(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "fixture")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"fixture","name":"Fixture","version":"1.0.0","web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(root)
	registry.statePath = filepath.Join(t.TempDir(), "plugins.json")
	if err := registry.SetEnabled("fixture", false); err != nil {
		t.Fatal(err)
	}
	if len(registry.List()) != 0 {
		t.Fatal("disabled plugin must not appear in active list")
	}
	if all := registry.All(); len(all) != 1 || all[0].Enabled {
		t.Fatalf("disabled plugin should remain manageable: %#v", all)
	}
	if err := registry.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	if active := registry.List(); len(active) != 1 || !active[0].Enabled {
		t.Fatalf("plugin should be active again: %#v", active)
	}
}

func TestRegistryRendersOnlinePluginFromAppsXIndex(t *testing.T) {
	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps-x.md":
			_, _ = w.Write([]byte("[network]: https://github.com/lemonade-lab/alemonx-network\n"))
		case "/alx.json":
			_, _ = w.Write([]byte(`{"id":"alemonx-network","name":"网络","version":"1.0.0","web":{"root":"web"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	registry := Registry{
		onlineIndexURL: server.URL + "/apps-x.md",
		httpClient:     server.Client(),
		onlineManifestURL: func(string) string {
			return server.URL + "/alx.json"
		},
	}
	plugins := registry.List()
	if len(plugins) != 1 || !plugins[0].Online || plugins[0].Runnable || plugins[0].Name != "网络" {
		t.Fatalf("online plugin = %#v", plugins)
	}
	if _, err := registry.Run("alemonx-network", "check", nil, false); err == nil {
		t.Fatal("online plugin must not execute before installation")
	}
}

func TestRegistryHotPlugReflectsAddedPlugin(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	registry.Rescan()
	before := registry.Revision()

	// Add a new plugin directory with a valid manifest and web root.
	directory := filepath.Join(root, "newone")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"newone","name":"New","version":"1.0.0","web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	registry.Rescan()
	after := registry.Revision()
	if after <= before {
		t.Fatalf("revision must bump after adding a plugin (before=%d after=%d)", before, after)
	}
	if _, err := registry.Find("newone"); err != nil {
		t.Fatalf("new plugin must be discoverable after rescan: %v", err)
	}
}

func TestRegistrySubscribeSignalsChange(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry(root)
	registry.Rescan()
	changes := registry.Subscribe()
	defer registry.Unsubscribe(changes)

	// A rescan that changes nothing must not wake subscribers.
	registry.Rescan()
	select {
	case <-changes:
		t.Fatal("unexpected change signal when the plugin set did not change")
	default:
	}

	directory := filepath.Join(root, "newone")
	if err := os.MkdirAll(filepath.Join(directory, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"newone","name":"New","version":"1.0.0","web":{"root":"web"}}`
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	registry.Rescan()
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("expected a change signal after the plugin set changed")
	}

	// Unsubscribe stops further delivery.
	registry.Unsubscribe(changes)
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	registry.Rescan()
	select {
	case <-changes:
		t.Fatal("subscriber still receives signals after unsubscribe")
	default:
	}
}

func makePluginArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	manifest := fmt.Sprintf(`{"id":"alemonx-network","name":"网络","version":"1.0.0","entry":{"%s":"runner"},"web":{"root":"web"}}`, runtime.GOOS+"-"+runtime.GOARCH)
	for name, content := range map[string]string{manifestName: manifest, "web/index.html": "<h1>network</h1>", "runner": "#!/bin/sh\nprintf '{\"output\":\"ok\"}'\n"} {
		file, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestRegistryInstallsOnlinePluginLocally(t *testing.T) {
	archive := makePluginArchive(t)
	assetName := "alemonx-network-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps-x.md":
			_, _ = w.Write([]byte("[network]: https://github.com/lemonade-lab/alemonx-network\n"))
		case "/alx.json":
			_, _ = w.Write([]byte(`{"id":"alemonx-network","name":"网络","version":"1.0.0","web":{"root":"web"}}`))
		case "/releases":
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"tag_name":"v1.0.0","name":"v1.0.0","html_url":"https://github.com/lemonade-lab/alemonx-network/releases/tag/v1.0.0","published_at":"2025-01-01T00:00:00Z","assets":[{"name":"%s","browser_download_url":"https://github.com/lemonade-lab/alemonx-network/releases/download/v1.0.0/%s","size":128}]}]`, assetName, assetName)))
		case "/lemonade-lab/alemonx-network/releases/download/v1.0.0/" + assetName:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	registry := Registry{
		roots:             []string{root},
		onlineIndexURL:    server.URL + "/apps-x.md",
		httpClient:        server.Client(),
		onlineManifestURL: func(string) string { return server.URL + "/alx.json" },
		releaseURL:        func(string) string { return server.URL + "/releases" },
	}
	installed, err := registry.Install("alemonx-network", "v1.0.0", assetName)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if installed.Online {
		t.Fatal("installed plugin must be local, not online-only")
	}
	if !installed.Enabled {
		t.Fatal("installed plugin must be enabled")
	}
	if installed.Version != "1.0.0" || installed.InstalledTag != "v1.0.0" || len(installed.Fingerprint) != 64 {
		t.Fatalf("installed plugin version must come from release metadata: %#v", installed)
	}
	if _, err := os.Stat(filepath.Join(root, "alemonx-network", installMetadataName)); err != nil {
		t.Fatalf("install metadata missing: %v", err)
	}
	summary, err := registry.CacheSummary()
	if err != nil || summary.Entries != 1 || summary.Bytes == 0 {
		t.Fatalf("installed release should be cached: %#v, %v", summary, err)
	}
	versions, err := registry.Versions("alemonx-network")
	if err != nil || len(versions) != 1 || !versions[0].Active || !versions[0].Cached {
		t.Fatalf("cached active version missing: %#v, %v", versions, err)
	}
	if err := registry.DeleteVersion("alemonx-network", "v1.0.0"); err == nil {
		t.Fatal("active cached version must not be deletable")
	}
	registry.httpClient = nil
	all := registry.All()
	if len(all) != 1 || all[0].ID != "alemonx-network" || all[0].Online || all[0].Source != filepath.Join(root, "alemonx-network") {
		t.Fatalf("installed plugin should be the sole local entry: %#v", all)
	}
	if _, err := registry.Install("alemonx-network", "v1.0.0", assetName); err != nil {
		t.Fatalf("reinstalling a cached version should not fail: %v", err)
	}
	if err := registry.Uninstall("alemonx-network"); err != nil {
		t.Fatalf("uninstalling an owned release should not fail: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alemonx-network")); !os.IsNotExist(err) {
		t.Fatalf("owned release directory should be removed, stat err=%v", err)
	}
	if cached, err := registry.listCached("alemonx-network"); err != nil || len(cached) != 1 {
		t.Fatalf("uninstall should retain release cache: %#v, %v", cached, err)
	}
}

func TestRegistryInstallRejectsUnknownPlugin(t *testing.T) {
	registry := Registry{roots: []string{t.TempDir()}}
	if _, err := registry.Install("nope", "v1.0.0", "x.zip"); err == nil {
		t.Fatal("unknown plugin id must be rejected")
	}
}

func TestDownloadAssetRejectsDeclaredOversizeArchive(t *testing.T) {
	registry := Registry{httpClient: newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxPluginArchiveSize+1, 10))
		w.WriteHeader(http.StatusOK)
	})).Client()}
	_, _, err := registry.downloadAsset(context.Background(), ReleaseAsset{Name: "plugin.zip", URL: "https://github.com/lemonade-lab/plugin/releases/download/v1/plugin.zip"})
	if err == nil || !strings.Contains(err.Error(), "超过 300 MB 限制") {
		t.Fatalf("oversized archive error = %v", err)
	}
}

func TestDownloadAssetRejectsUndeclaredOversizeArchive(t *testing.T) {
	registry := Registry{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(io.LimitReader(zeroReader{}, maxPluginArchiveSize+1)), ContentLength: -1, Request: request}, nil
	})}}
	_, _, err := registry.downloadAsset(context.Background(), ReleaseAsset{Name: "plugin.zip", URL: "https://github.com/lemonade-lab/plugin/releases/download/v1/plugin.zip"})
	if err == nil || !strings.Contains(err.Error(), "超过 300 MB 限制") {
		t.Fatalf("undeclared oversized archive error = %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) {
	for index := range data {
		data[index] = 0
	}
	return len(data), nil
}

func TestDownloadAssetRetriesTransientFailure(t *testing.T) {
	attempts := 0
	registry := Registry{httpClient: newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("archive"))
	})).Client()}
	path, _, err := registry.downloadAsset(context.Background(), ReleaseAsset{Name: "plugin.zip", URL: "https://github.com/lemonade-lab/plugin/releases/download/v1/plugin.zip"})
	if err != nil {
		t.Fatalf("transient download must retry: %v", err)
	}
	defer os.Remove(path)
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRegistrySwitchesBetweenCachedReleaseVersionsOffline(t *testing.T) {
	archive := makePluginArchive(t)
	assetName := "alemonx-network-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	downloads := 0
	server := newTestHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apps-x.md":
			_, _ = w.Write([]byte("[network]: https://github.com/lemonade-lab/alemonx-network\n"))
		case "/alx.json":
			_, _ = w.Write([]byte(`{"id":"alemonx-network","name":"网络","version":"1.0.0","web":{"root":"web"}}`))
		case "/releases":
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"tag_name":"v2.0.0","assets":[{"name":"%s","browser_download_url":"https://github.com/lemonade-lab/alemonx-network/releases/download/v2.0.0/%s"}]},{"tag_name":"v1.0.0","assets":[{"name":"%s","browser_download_url":"https://github.com/lemonade-lab/alemonx-network/releases/download/v1.0.0/%s"}]}]`, assetName, assetName, assetName, assetName)))
		default:
			if strings.Contains(r.URL.Path, "/releases/download/") {
				downloads++
				_, _ = w.Write(archive)
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	registry := Registry{roots: []string{root}, onlineIndexURL: server.URL + "/apps-x.md", httpClient: server.Client(), onlineManifestURL: func(string) string { return server.URL + "/alx.json" }, releaseURL: func(string) string { return server.URL + "/releases" }}
	if _, err := registry.Install("alemonx-network", "v1.0.0", assetName); err != nil {
		t.Fatalf("install v1 failed: %v", err)
	}
	if _, err := registry.Install("alemonx-network", "v2.0.0", assetName); err != nil {
		t.Fatalf("switch to v2 failed: %v", err)
	}
	if downloads != 2 {
		t.Fatalf("expected one download per release, got %d", downloads)
	}
	registry.httpClient = nil
	if _, err := registry.Install("alemonx-network", "v1.0.0", assetName); err != nil {
		t.Fatalf("switch back to cached v1 failed offline: %v", err)
	}
	if downloads != 2 {
		t.Fatalf("cached switch unexpectedly downloaded another archive: %d", downloads)
	}
	versions, err := registry.Versions("alemonx-network")
	if err != nil || len(versions) != 2 || !versions[1].Active {
		t.Fatalf("expected two versions with v1 active: %#v, %v", versions, err)
	}
}
