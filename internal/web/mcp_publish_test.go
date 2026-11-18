package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alemonx/internal/access"
	"alemonx/internal/systemnetwork"
)

const (
	publishFixtureName     = "alx-local-publish-fixture"
	publishFixtureArtifact = "export const fixture = true;\n"
)

// Run the fixture in a separate test process so its network policy, TLS roots
// and child-process configuration cannot affect any other workbench tests.
func TestMCPPersistsSuccessfulPublications(t *testing.T) {
	if os.Getenv("ALX_LOCAL_PUBLISH_TEST_CHILD") == "lookup" {
		verifyPublishedTasks(t)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX build script")
	}
	for _, command := range []string{"node", "npm", "git"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is required for this local integration test", command)
		}
	}
	if os.Getenv("ALX_LOCAL_PUBLISH_TEST_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.CommandContext(ctx, executable, "-test.run=^TestMCPPersistsSuccessfulPublications$", "-test.v")
		child.Env = append(os.Environ(), "ALX_LOCAL_PUBLISH_TEST_CHILD=1")
		if out, err := child.CombinedOutput(); err != nil {
			t.Fatalf("isolated publication fixture: %v\n%s", err, out)
		} else {
			t.Logf("%s", out)
		}
		return
	}

	fixture := t.TempDir()
	root, remote := filepath.Join(fixture, "robot"), filepath.Join(fixture, "remote.git")
	for _, path := range []string{root, remote} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_ALLOW_PROTOCOL", "file")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"} {
		old, ok := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Cleanup(func() { _ = os.Setenv(name, old) })
		}
	}
	npmConfig := filepath.Join(fixture, "npmrc")
	if err := os.WriteFile(npmConfig, []byte("//registry.npmjs.org/:_authToken=fixture-local-npm-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("npm_config_userconfig", npmConfig)
	t.Setenv("npm_config_globalconfig", os.DevNull)
	t.Setenv("npm_config_cache", filepath.Join(fixture, "npm-cache"))
	t.Setenv("npm_config_fund", "false")
	t.Setenv("npm_config_audit", "false")
	t.Setenv("npm_config_update_notifier", "false")
	t.Setenv("npm_config_fetch_retries", "0")
	t.Setenv("npm_config_fetch_timeout", "10000")

	requests := startLocalPublishRegistry(t, fixture)
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(remote, "init", "--bare", "-b", "main")
	git(root, "init", "-b", "main")
	git(root, "config", "user.name", "Local Publish Fixture")
	git(root, "config", "user.email", "fixture@example.invalid")
	git(root, "config", "commit.gpgsign", "false")
	git(root, "config", "tag.gpgsign", "false")
	manifest, err := json.Marshal(map[string]any{
		"name": publishFixtureName, "version": "1.2.3", "files": []string{"lib"}, "packageManager": "npm@10.0.0",
		"scripts": map[string]string{"build": "mkdir -p lib && printf '%s\\n' 'export const fixture = true;' > lib/index.js"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	git(root, "add", "package.json")
	git(root, "commit", "-m", "Fixture source")
	commit := git(root, "rev-parse", "HEAD")
	git(root, "remote", "add", "origin", remote)
	git(root, "push", "origin", "main")

	const account = "local-publish-fixture"
	s := openPublicationWorkbench(t, fixture)
	session, err := s.auth.Enable(account, "local-fixture-password", "local-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := s.auth.Status(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.addCreatedRobot(fixture, identity.RobotScope, root); err != nil {
		t.Fatal(err)
	}
	operator, err := s.auth.UpdateMCPKey(session, identity, "token", false)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := s.auth.UpdateMCPKey(session, identity, "token", true)
	if err != nil {
		t.Fatal(err)
	}

	releaseArgs, err := json.Marshal(map[string]any{"branch": "main", "commit": commit, "artifacts": []string{"lib"}})
	if err != nil {
		t.Fatal(err)
	}
	actions := []struct{ action, message, tag, version, expected string }{
		{"npm-publish", commit, "fixture", "", "打包并发布到 npm"},
		{"git-release", string(releaseArgs), "", "1.2.3", "已发布 v1.2.3"},
	}
	taskIDs := make(map[string]string)
	for _, entry := range actions {
		result, failed := callTaskMCP(t, s, fixture, operator, "alemonjs_start_project_action", map[string]any{
			"root": root, "action": entry.action, "message": entry.message, "tag": entry.tag, "version": entry.version, "confirm": true,
		})
		var task operationTask
		if failed || json.Unmarshal([]byte(result), &task) != nil || task.ID == "" || task.Status != "running" {
			t.Fatalf("%s acceptance: %s", entry.action, result)
		}
		taskIDs[entry.action] = task.ID
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if task, _ = s.operationEvents.mcpTaskByID(context.Background(), taskIDs[entry.action]); task.FinishedAt != nil {
				break
			}
		}
		if task.Status != "completed" || !strings.Contains(task.Output, entry.expected) {
			t.Fatalf("%s outcome: %+v", entry.action, task)
		}
	}
	if requests.metadata.Load() != 1 || requests.identity.Load() != 1 || requests.published.Load() != 1 {
		t.Fatalf("registry calls: metadata=%d identity=%d published=%d", requests.metadata.Load(), requests.identity.Load(), requests.published.Load())
	}
	release := git(remote, "rev-parse", "refs/heads/release")
	if git(remote, "rev-parse", "refs/tags/v1.2.3^{}") != release ||
		git(remote, "rev-parse", "refs/heads/main") != commit ||
		git(remote, "show", "refs/heads/release:lib/index.js") != strings.TrimSpace(publishFixtureArtifact) ||
		!strings.Contains(git(remote, "log", "-1", "--format=%B", "refs/heads/release"), "Source-Commit: "+commit) {
		t.Fatal("local Git remote is missing the release artifact, source mapping or tag")
	}

	if err := s.operationEvents.Close(); err != nil {
		t.Fatal(err)
	}
	lookup := publicationLookup{Workspace: fixture, Robot: root, Account: account, DiagnosticKey: diagnostic}
	for _, entry := range actions {
		lookup.Tasks = append(lookup.Tasks, publishedTask{ID: taskIDs[entry.action], Action: entry.action, Expected: entry.expected})
	}
	raw, err := json.Marshal(lookup)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestMCPPersistsSuccessfulPublications$", "-test.v")
	child.Env = append(os.Environ(), "ALX_LOCAL_PUBLISH_TEST_CHILD=lookup")
	child.Stdin = bytes.NewReader(raw)
	out, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh workbench process: %v\n%s", err, out)
	}
	t.Logf("%s", out)
	if requests.published.Load() != 1 || git(remote, "rev-parse", "refs/heads/release") != release {
		t.Fatal("restart replayed publication")
	}
}

type publishedTask struct{ ID, Action, Expected string }

type publicationLookup struct {
	Workspace, Robot, Account, DiagnosticKey string
	Tasks                                    []publishedTask
}

func openPublicationWorkbench(t *testing.T, dir string) *server {
	t.Helper()
	auth, err := access.NewAt(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	store := newOperationEventStore(filepath.Join(dir, "operations", "events.db"))
	t.Cleanup(func() { _ = store.Close() })
	s := &server{auth: auth, operationEvents: store, operations: store.snapshot(), events: newRobotEventHub(), eventGateway: newEventGateway()}
	if err := s.settleRecoveredMCPTasks(); err != nil {
		t.Fatal(err)
	}
	return s
}

func verifyPublishedTasks(t *testing.T) {
	t.Helper()
	var lookup publicationLookup
	if err := json.NewDecoder(os.Stdin).Decode(&lookup); err != nil {
		t.Fatal(err)
	}
	s := openPublicationWorkbench(t, lookup.Workspace)
	for _, entry := range lookup.Tasks {
		result, failed := callTaskMCP(t, s, lookup.Workspace, lookup.DiagnosticKey, "alemonjs_get_project_task", map[string]any{"taskId": entry.ID})
		var task operationTask
		if failed || json.Unmarshal([]byte(result), &task) != nil || task.ID != entry.ID ||
			task.Action != entry.Action || task.Account != lookup.Account || task.Root != lookup.Robot ||
			task.Status != "completed" || task.FinishedAt == nil || !strings.Contains(task.Output, entry.Expected) {
			t.Fatalf("%s after workbench restart: %s", entry.Action, result)
		}
		checkPublishTaskAudit(t, s.operationEvents, entry.ID, lookup.Account, lookup.Robot, entry.Action)
		t.Logf("%s task %s: completed and found in a fresh process, with one acceptance and one result", entry.Action, entry.ID)
	}
}

func checkPublishTaskAudit(t *testing.T, store *OperationEventStore, id, account, root, action string) {
	t.Helper()
	rows, err := store.db.Query(`SELECT type, payload FROM event_journal WHERE topic='mcp'
		AND json_extract(CAST(payload AS TEXT), '$.task_id')=? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	expected := []struct{ event, status string }{{mcpTaskAccepted, "running"}, {mcpTaskResult, "completed"}}
	for _, want := range expected {
		var event string
		var payload []byte
		fields := map[string]string{}
		if !rows.Next() || rows.Scan(&event, &payload) != nil || json.Unmarshal(payload, &fields) != nil ||
			event != want.event || fields["status"] != want.status || fields["account"] != account ||
			fields["root"] != root || fields["action"] != action {
			t.Fatalf("%s audit: got %s %s, expected %+v", action, event, payload, want)
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("%s: expected exactly two audit events: %v", id, rows.Err())
	}
}

type publishRegistryRequests struct{ metadata, identity, published atomic.Int32 }

func startLocalPublishRegistry(t *testing.T, fixture string) *publishRegistryRequests {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"registry.npmjs.org"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certPath := filepath.Join(fixture, "npm-test-ca.pem")
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_EXTRA_CA_CERTS", certPath)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to load npm fixture CA")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	http.DefaultTransport = transport
	t.Cleanup(transport.CloseIdleConnections)

	counts := &publishRegistryRequests{}
	registry := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/"+publishFixtureName:
			counts.metadata.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not_found"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/npm":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not_found"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/-/whoami":
			counts.identity.Add(1)
			if r.Header.Get("Authorization") != "Bearer fixture-local-npm-token" {
				http.Error(w, "missing fixture credentials", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"username":"local-fixture"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/"+publishFixtureName:
			if r.Header.Get("Authorization") != "Bearer fixture-local-npm-token" {
				http.Error(w, "missing fixture credentials", http.StatusUnauthorized)
				return
			}
			var body struct {
				DistTags    map[string]string                `json:"dist-tags"`
				Attachments map[string]struct{ Data []byte } `json:"_attachments"`
			}
			expectedArchive := publishFixtureName + "-1.2.3.tgz"
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil ||
				body.DistTags["fixture"] != "1.2.3" || len(body.Attachments) != 1 ||
				!publishArchiveHasArtifact(body.Attachments[expectedArchive].Data) {
				http.Error(w, "invalid publication tag or archive", http.StatusBadRequest)
				return
			}
			counts.published.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			t.Errorf("unexpected npm fixture request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	registry.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	registry.StartTLS()
	t.Cleanup(registry.Close)

	// The production publish path intentionally pins the registry host. This
	// fixture routes that exact host to loopback and rejects every other host.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "registry.npmjs.org:443" {
			t.Errorf("unexpected proxy target: %s %s", r.Method, r.Host)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", registry.Listener.Addr().String(), 3*time.Second)
		if err != nil {
			http.Error(w, "fixture registry unavailable", http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		deadline := time.Now().Add(15 * time.Second)
		_ = conn.SetDeadline(deadline)
		_ = upstream.SetDeadline(deadline)
		if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = io.Copy(upstream, buffer)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(conn, upstream)
		_ = conn.Close()
		<-done
	}))
	t.Cleanup(proxy.Close)

	network, err := systemnetwork.NewAt(filepath.Join(fixture, "network.json"))
	if err != nil {
		t.Fatal(err)
	}
	config := *network.Settings().Config
	config.Mode = "proxy"
	config.Proxy = systemnetwork.ProxyConfig{URL: proxy.URL, Credentials: "remove"}
	if _, err := network.Save(systemnetwork.Settings{Config: &config}); err != nil {
		t.Fatal(err)
	}
	systemnetwork.SetDefault(network)
	return counts
}

func publishArchiveHasArtifact(archive []byte) bool {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return false
	}
	defer reader.Close()
	files := tar.NewReader(reader)
	for {
		header, err := files.Next()
		if err != nil {
			return false
		}
		if header.Name == "package/lib/index.js" {
			content, err := io.ReadAll(io.LimitReader(files, 1024))
			return err == nil && string(content) == publishFixtureArtifact
		}
	}
}
