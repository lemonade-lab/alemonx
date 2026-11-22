package mcp

import (
	"alemonx/internal/robot"
	"alemonx/internal/workspace"
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func backpackFixture(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	backpackWrite(t, filepath.Join(root, "package.json"), `{"name":"fixture","packageManager":"npm"}`)
	policy := Policy{RobotList: func() (workspace.RobotList, error) {
		return workspace.RobotList{Projects: []workspace.RobotEntry{{Path: root}}}, nil
	}}
	return NewServerWithPolicy("test", nil, policy), root
}

func backpackWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func backpackCall(t *testing.T, s *Server, name string, args map[string]any) (string, error) {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return s.execute(context.Background(), name, data)
}

func backpackOK(t *testing.T, s *Server, name string, args map[string]any) string {
	t.Helper()
	result, err := backpackCall(t, s, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func backpackTask(t *testing.T, s *Server, name string, args map[string]any, want string) operationTask {
	t.Helper()
	result := backpackOK(t, s, name, args)
	var task operationTask
	if err := json.Unmarshal([]byte(result), &task); err != nil || task.ID == "" {
		t.Fatalf("unexpected task: %s", result)
	}
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		taskResult := backpackOK(t, s, "alemonjs_get_project_task", map[string]any{"taskId": task.ID})
		if err := json.Unmarshal([]byte(taskResult), &task); err != nil {
			t.Fatal(err)
		}
		if task.FinishedAt != nil {
			if task.Status != want {
				t.Fatalf("want %s, task = %+v", want, task)
			}
			return task
		}
	}
	t.Fatalf("task did not finish: %+v", task)
	return task
}

func TestBackpackToolsScopeAndConfirmation(t *testing.T) {
	s, root := backpackFixture(t)
	// Use the registered tool schemas to exercise every write, including
	// changes that would otherwise queue a background task.
	for _, definition := range backpackTools() {
		name := definition["name"].(string)
		readonly := definition["annotations"].(map[string]any)["readOnlyHint"].(bool)
		if !readonly {
			if _, err := backpackCall(t, s, name, map[string]any{"root": root}); err == nil || !strings.Contains(err.Error(), "confirm=true") {
				t.Errorf("%s accepted missing confirmation: %v", name, err)
			}
		}
		if _, err := backpackCall(t, s, name, map[string]any{"root": t.TempDir(), "confirm": true}); err == nil ||
			!strings.Contains(err.Error(), "当前账户") {
			t.Errorf("%s accepted a robot outside the account: %v", name, err)
		}
	}
}

func TestBackpackMCPCloneKeepsWorkbenchOptions(t *testing.T) {
	s, root := backpackFixture(t)
	var submitted ProjectAction
	s.policy.SubmitProjectAction = func(action ProjectAction) (string, error) {
		submitted = action
		return `{"id":"clone-task"}`, nil
	}
	args := map[string]any{
		"root": root, "repository": "https://example.invalid/plugin.git", "branch": "release",
		"name": "local-name", "mirror": "official", "depth": 5, "confirm": true,
	}
	backpackOK(t, s, "alemonjs_clone_backpack_plugin", args)
	var options map[string]any
	if err := json.Unmarshal([]byte(submitted.Message), &options); err != nil ||
		submitted.Action != "clone-backpack-plugin" || submitted.Root != root ||
		options["name"] != "local-name" || options["depth"] != float64(5) || options["branch"] != "release" {
		t.Fatalf("clone options were not forwarded: %+v (%v)", submitted, err)
	}
	for _, repository := range []string{"file:///outside/repo", "https://user:secret@example.invalid/plugin.git"} {
		args["repository"] = repository
		if _, err := backpackCall(t, s, "alemonjs_clone_backpack_plugin", args); err == nil {
			t.Fatal("unapproved repository format accepted")
		}
	}
}

func TestBackpackMCPImportOrderWorkspaceAndConfigTypes(t *testing.T) {
	s, root := backpackFixture(t)
	const manifest = `{"name":"folder-plugin","alemonjs":{"config":[{"name":"enabled","type":"boolean"},{"name":"limit","type":"number"},{"name":"options","type":"object"}]}}`
	backpackWrite(t, filepath.Join(root, "imports", "folder", "package.json"), manifest)
	backpackWrite(t, filepath.Join(root, "imports", "folder", ".env"), "SECRET=fixture")
	archive := func(name string) []byte {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		file, err := writer.Create("package.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(file).Encode(map[string]string{"name": name, "version": "1.0.0"}); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	backpackWrite(t, filepath.Join(root, "imports", "second.zip"), string(archive("archive-plugin")))
	for _, path := range []string{"imports/folder", "imports/second.zip"} {
		backpackTask(t, s, "alemonjs_import_backpack_plugin", map[string]any{"root": root, "path": path, "confirm": true}, "completed")
	}
	inline := map[string]any{"root": root, "archiveBase64": base64.StdEncoding.EncodeToString(archive("inline-plugin")), "confirm": true}
	backpackTask(t, s, "alemonjs_import_backpack_plugin", inline, "completed")
	task := backpackTask(t, s, "alemonjs_import_backpack_plugin", inline, "failed")
	if !strings.Contains(task.Error, "同名") {
		t.Fatalf("inline archive did not reach the installer: %+v", task)
	}
	inline["path"] = "imports/second.zip"
	if _, err := backpackCall(t, s, "alemonjs_import_backpack_plugin", inline); err == nil {
		t.Fatal("accepted two import sources")
	}
	if _, err := os.Lstat(filepath.Join(root, "packages", "folder-plugin", ".env")); !os.IsNotExist(err) {
		t.Fatalf("protected file was copied: %v", err)
	}
	if _, err := backpackCall(t, s, "alemonjs_import_backpack_plugin", map[string]any{"root": root, "path": "../outside.zip", "confirm": true}); err == nil {
		t.Fatal("parent path accepted")
	}
	backpackTask(t, s, "alemonjs_import_backpack_plugin", map[string]any{"root": root, "path": "imports/folder", "confirm": true}, "failed")

	args := map[string]any{"root": root, "packages": []string{"folder-plugin", "missing"}, "enabled": true, "confirm": true}
	if _, err := backpackCall(t, s, "alemonjs_set_backpack_plugins_enabled", args); err == nil {
		t.Fatal("a missing plugin was accepted")
	}
	if order := backpackOK(t, s, "alemonjs_get_backpack_plugin_order", map[string]any{"root": root}); order != "[]" {
		t.Fatalf("failed batch made a partial change: %s", order)
	}
	args["packages"] = []string{"folder-plugin", "archive-plugin"}
	backpackOK(t, s, "alemonjs_set_backpack_plugins_enabled", args)
	backpackOK(t, s, "alemonjs_move_backpack_plugin", map[string]any{
		"root": root, "source": "folder-plugin", "target": "archive-plugin", "after": true, "confirm": true,
	})
	if order := backpackOK(t, s, "alemonjs_get_backpack_plugin_order", map[string]any{"root": root}); order != `["archive-plugin","folder-plugin"]` {
		t.Fatalf("unexpected load order: %s", order)
	}
	args["enabled"] = false
	backpackOK(t, s, "alemonjs_set_backpack_plugins_enabled", args)
	args["enabled"] = true
	backpackOK(t, s, "alemonjs_set_backpack_plugins_enabled", args)
	if order := backpackOK(t, s, "alemonjs_get_backpack_plugin_order", map[string]any{"root": root}); order != `["archive-plugin","folder-plugin"]` {
		t.Fatalf("load order changed after toggling: %s", order)
	}
	for _, enabled := range []bool{true, false} {
		backpackTask(t, s, "alemonjs_set_backpack_workspace", map[string]any{"root": root, "enabled": enabled, "confirm": true}, "completed")
		want, _ := json.Marshal(map[string]bool{"enabled": enabled})
		if got := backpackOK(t, s, "alemonjs_get_backpack_workspace", map[string]any{"root": root}); got != string(want) {
			t.Fatalf("workspace = %s, want %s", got, want)
		}
	}
	configArgs := map[string]any{
		"root": root, "package": "folder-plugin", "confirm": true,
		"values": map[string]any{"enabled": true, "limit": float64(3), "options": map[string]any{"nested": []any{"text", false}}},
	}
	backpackOK(t, s, "alemonjs_save_package_runtime_config", configArgs)
	c, err := s.robots.PackageConfig(root, "folder-plugin")
	if err != nil || c.Values["enabled"] != true || !reflect.DeepEqual(c.Values["options"], map[string]any{"nested": []any{"text", false}}) {
		data, _ := os.ReadFile(filepath.Join(root, "alemon.config.yaml"))
		t.Fatalf("config lost its types: %+v (%v)\n%s", c.Values, err, data)
	}
	if number, _ := json.Marshal(c.Values["limit"]); string(number) != "3" {
		t.Fatalf("config number lost its type: %s", number)
	}
	if order := backpackOK(t, s, "alemonjs_get_backpack_plugin_order", map[string]any{"root": root}); order != `["archive-plugin","folder-plugin"]` {
		t.Fatalf("saving plugin configuration changed the load order: %s", order)
	}
	task = backpackTask(t, s, "alemonjs_update_backpack_plugins", map[string]any{
		"root": root, "packages": []string{"folder-plugin"}, "confirm": true,
	}, "completed")
	if !strings.Contains(task.Output, "无法验证非 Git 插件") {
		t.Fatalf("safe update accepted an unverifiable local folder: %+v", task)
	}
	for _, definition := range tools() {
		if definition["name"] == "alemonjs_save_package_runtime_config" {
			properties := definition["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if properties["values"].(map[string]any)["additionalProperties"] != true {
				t.Fatal("MCP schema rejects non-string config values")
			}
		}
	}
}

func TestBackpackMCPVersionQuerySafeUpdateAndRecovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	if runtime.GOOS == "windows" {
		t.Skip("test npm stub requires a POSIX executable")
	}
	s, root := backpackFixture(t)
	npm := filepath.Join(t.TempDir(), "npm")
	backpackWrite(t, npm, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(npm, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(npm)+string(os.PathListSeparator)+os.Getenv("PATH"))
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	remote := t.TempDir()
	git(remote, "init", "-b", "release")
	backpackWrite(t, filepath.Join(remote, "package.json"), `{"name":"fixture-plugin","version":"1.0.0"}`)
	backpackWrite(t, filepath.Join(remote, "index.js"), "first\n")
	git(remote, "add", ".")
	git(remote, "commit", "-m", "First")
	first := git(remote, "rev-parse", "HEAD")
	plugin := filepath.Join(root, "packages", "fixture-plugin")
	git(root, "clone", remote, plugin)
	git(plugin, "config", "user.name", "Fixture")
	git(plugin, "config", "user.email", "fixture@example.invalid")
	backpackWrite(t, filepath.Join(remote, "index.js"), "second\n")
	git(remote, "commit", "-am", "Second")
	second := git(remote, "rev-parse", "HEAD")

	base := map[string]any{"root": root, "package": "fixture-plugin"}
	query := func() robot.LocalPackageVersions {
		t.Helper()
		var versions robot.LocalPackageVersions
		raw := backpackOK(t, s, "alemonjs_list_backpack_plugin_versions", base)
		if err := json.Unmarshal([]byte(raw), &versions); err != nil {
			t.Fatal(err)
		}
		return versions
	}
	if got := query().Latest; got != first || git(plugin, "rev-parse", "origin/release") != first {
		t.Fatalf("read-only query fetched remote refs: %s", got)
	}
	versionTask := func(action, version, stash, want string) operationTask {
		t.Helper()
		return backpackTask(t, s, "alemonjs_manage_backpack_plugin_version", map[string]any{
			"root": root, "package": "fixture-plugin", "action": action, "version": version, "stash": stash, "confirm": true,
		}, want)
	}
	versionTask("refresh-versions", "", "", "completed")
	if got := query().Latest; got != second {
		t.Fatalf("confirmed refresh = %s, want %s", got, second)
	}

	safe := func() operationTask {
		t.Helper()
		return backpackTask(t, s, "alemonjs_update_backpack_plugins", map[string]any{
			"root": root, "packages": []string{"fixture-plugin"}, "confirm": true,
		}, "completed")
	}
	localFile := filepath.Join(plugin, "local.txt")
	backpackWrite(t, localFile, "keep this\n")
	diffArgs := map[string]any{"root": root, "package": "fixture-plugin", "view": "diff", "path": "local.txt"}
	if diff := backpackOK(t, s, "alemonjs_get_backpack_plugin_details", diffArgs); !strings.Contains(diff, "keep this") {
		t.Fatalf("expected local diff: %s", diff)
	}
	diffArgs["path"] = ".env"
	if _, err := backpackCall(t, s, "alemonjs_get_backpack_plugin_details", diffArgs); err == nil {
		t.Fatal("protected file was accepted by the diff tool")
	}
	if task := safe(); !strings.Contains(task.Output, "跳过") || git(plugin, "rev-parse", "HEAD") != first {
		t.Fatalf("safe update did not skip dirty workspace: %+v", task)
	}
	versionTask("switch", second, "", "failed")
	if contents, err := os.ReadFile(localFile); err != nil || string(contents) != "keep this\n" {
		t.Fatalf("normal switch lost local edits: %s (%v)", contents, err)
	}
	versionTask("stash-switch", second, "", "completed")
	if git(plugin, "rev-parse", "HEAD") != second {
		t.Fatal("stash and switch did not select the new version")
	}
	stashJSON := backpackOK(t, s, "alemonjs_get_backpack_plugin_details", map[string]any{"root": root, "package": "fixture-plugin", "view": "stashes"})
	var stashes []robot.LocalPackageStash
	if err := json.Unmarshal([]byte(stashJSON), &stashes); err != nil || len(stashes) != 1 {
		t.Fatalf("expected one recoverable stash: %s", stashJSON)
	}
	versionTask("restore-stash", "", stashes[0].Ref, "completed")
	if contents, err := os.ReadFile(localFile); err != nil || string(contents) != "keep this\n" {
		t.Fatalf("stash restore lost local edits: %s (%v)", contents, err)
	}
	if err := os.Remove(localFile); err != nil {
		t.Fatal(err)
	}
	versionTask("switch", first, "", "completed")
	if task := safe(); !strings.Contains(task.Output, "已更新") || git(plugin, "rev-parse", "HEAD") != second {
		t.Fatalf("safe update failed on a clean workspace: %+v", task)
	}
	backpackWrite(t, localFile, "local commit\n")
	git(plugin, "add", ".")
	git(plugin, "commit", "-m", "Local commit")
	ahead := git(plugin, "rev-parse", "HEAD")
	if task := safe(); !strings.Contains(task.Output, "跳过") || git(plugin, "rev-parse", "HEAD") != ahead {
		t.Fatalf("safe update discarded a local commit: %+v", task)
	}
	versionTask("stash-switch", first, "", "completed")
	if backups := git(plugin, "branch", "--list", "alx-backup-*", "--format=%(objectname)"); backups != ahead {
		t.Fatalf("backup should retain the local commit %s, got %s", ahead, backups)
	}
	backpackWrite(t, localFile, "discard after confirmation\n")
	versionTask("force-switch", second, "", "completed")
	if _, err := os.Lstat(localFile); !os.IsNotExist(err) {
		t.Fatalf("confirmed force switch did not discard the test edit: %v", err)
	}
}
