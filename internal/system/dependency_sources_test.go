package system

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencySourceDistributionSupport(t *testing.T) {
	for _, distribution := range []string{"debian", "ubuntu", "Ubuntu"} {
		if !isAPTDistribution(distribution) {
			t.Fatalf("APT distribution %q should be supported", distribution)
		}
	}
	for _, distribution := range []string{"kali", "linuxmint", "fedora"} {
		if isAPTDistribution(distribution) {
			t.Fatalf("APT distribution %q must not be auto-managed", distribution)
		}
	}
	if !isCentOSStream("centos", "stream", "CentOS Stream", "CentOS Stream 9") {
		t.Fatal("CentOS Stream with VARIANT_ID should be supported")
	}
	if !isCentOSStream("centos", "", "CentOS Stream", "CentOS Stream 9") {
		t.Fatal("CentOS Stream without VARIANT_ID should be supported")
	}
	for _, item := range [][4]string{{"centos", "", "CentOS Linux", "CentOS Linux 8"}, {"rocky", "stream", "Rocky Linux", "Rocky Linux 9"}, {"alma", "stream", "AlmaLinux", "AlmaLinux 9"}} {
		if isCentOSStream(item[0], item[1], item[2], item[3]) {
			t.Fatalf("%q/%q must not use the CentOS Stream source template", item[0], item[1])
		}
	}
}

func TestDependencySourceOperationRejectsUnknownTarget(t *testing.T) {
	if code := DependencySourceOperationHelper([]byte(`{"action":"delete","target":"/tmp/not-allowed"}`)); code != 2 {
		t.Fatalf("invalid target exit code = %d, want 2", code)
	}
}

func TestDependencySourceOperationRejectsWrites(t *testing.T) {
	if code := DependencySourceOperationHelper([]byte(`{"action":"write","target":"/etc/apt/sources.list.d/alemonx-mirror.list","content":"x"}`)); code != 2 {
		t.Fatalf("legacy write operation exit code = %d, want 2", code)
	}
}

func TestLegacyDependencySourceRequiresOwnershipMarker(t *testing.T) {
	if !isALemonXManagedDependencySource("# Managed by ALemonX\n[alemonx-baseos]\n") {
		t.Fatal("ALemonX ownership marker must be recognized")
	}
	if isALemonXManagedDependencySource("[alemonx-baseos]\nbaseurl=https://example.invalid\n") {
		t.Fatal("a matching file name or repository id alone must never authorize deletion")
	}
}

func TestDependencySourceWritesAreDisabled(t *testing.T) {
	if _, err := ApplyDependencySource(context.Background(), "official"); err == nil || !strings.Contains(err.Error(), "已停用") {
		t.Fatalf("apply must be disabled, got %v", err)
	}
	if _, err := RestoreDependencySource(context.Background(), "backup"); err == nil || !strings.Contains(err.Error(), "已停用") {
		t.Fatalf("restore must be disabled, got %v", err)
	}
}

func TestDependencySourceBackupIntegrityAndRetention(t *testing.T) {
	previous := dependencySourceConfigRoot
	configDir := t.TempDir()
	dependencySourceConfigRoot = func() string { return configDir }
	t.Cleanup(func() { dependencySourceConfigRoot = previous })
	const target = "/etc/apt/sources.list.d/alemonx-mirror.list"
	for index := 0; index < 31; index++ {
		if _, err := saveDependencySourceBackup(target, "apply-aliyun", strings.Repeat("x", index)); err != nil {
			t.Fatal(err)
		}
	}
	backups := dependencySourceBackups()
	if len(backups) != 30 {
		t.Fatalf("backup count = %d, want 30", len(backups))
	}
	for _, backup := range backups {
		if !strings.HasPrefix(backup.Checksum, "sha256:") {
			t.Fatalf("backup checksum missing: %#v", backup)
		}
		if strings.ContainsAny(backup.ID, `/\\`) {
			t.Fatalf("backup id must not contain path separators: %q", backup.ID)
		}
		info, err := os.Stat(filepath.Join(configDir, dependencySourceBackupDir, backup.ID+".json"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("backup permission = %v, err = %v", info.Mode().Perm(), err)
		}
	}
}

func TestDependencySourceStatusSerializesEmptyBackupsAsArray(t *testing.T) {
	previous := dependencySourceConfigRoot
	configDir := t.TempDir()
	dependencySourceConfigRoot = func() string { return configDir }
	t.Cleanup(func() { dependencySourceConfigRoot = previous })
	data, err := json.Marshal(DependencySourceStatusSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"backups":[]`) {
		t.Fatalf("empty backups must be an array: %s", data)
	}
}
