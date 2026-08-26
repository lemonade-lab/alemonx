package system

import (
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
	if !isCentOSStream("centos", "stream") {
		t.Fatal("CentOS Stream should be supported")
	}
	for _, item := range [][2]string{{"centos", ""}, {"rocky", "stream"}, {"alma", "stream"}} {
		if isCentOSStream(item[0], item[1]) {
			t.Fatalf("%q/%q must not use the CentOS Stream source template", item[0], item[1])
		}
	}
}

func TestAPTDependencySourceContent(t *testing.T) {
	ubuntu := aptContent("aliyun", "ubuntu", "noble")
	if !strings.Contains(ubuntu, "https://mirrors.aliyun.com/ubuntu noble") || !strings.Contains(ubuntu, "noble-updates") {
		t.Fatalf("unexpected Ubuntu source: %q", ubuntu)
	}
	debian := aptContent("official", "debian", "bookworm")
	if !strings.Contains(debian, "https://deb.debian.org/debian bookworm") || !strings.Contains(debian, "non-free-firmware") {
		t.Fatalf("unexpected Debian source: %q", debian)
	}
}

func TestDependencySourceOperationRejectsUnknownTarget(t *testing.T) {
	if code := DependencySourceOperationHelper([]byte(`{"action":"delete","target":"/tmp/not-allowed"}`)); code != 2 {
		t.Fatalf("invalid target exit code = %d, want 2", code)
	}
}
