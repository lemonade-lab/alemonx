package system

import "testing"

func TestBunAssetName(t *testing.T) {
	for _, tc := range []struct {
		os, arch string
		musl     bool
		want     string
	}{
		{"darwin", "arm64", false, "bun-darwin-aarch64.zip"},
		{"darwin", "amd64", false, "bun-darwin-x64-baseline.zip"},
		{"windows", "amd64", false, "bun-windows-x64-baseline.zip"},
		{"windows", "arm64", false, "bun-windows-aarch64.zip"},
		{"linux", "arm64", true, "bun-linux-aarch64-musl.zip"},
		{"linux", "amd64", true, "bun-linux-x64-musl-baseline.zip"},
		{"linux", "386", false, ""},
	} {
		if got := bunAssetName(tc.os, tc.arch, tc.musl); got != tc.want {
			t.Errorf("bunAssetName(%s, %s, %t) = %q, want %q", tc.os, tc.arch, tc.musl, got, tc.want)
		}
	}
}
