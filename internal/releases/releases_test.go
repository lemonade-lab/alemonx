package releases

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMatchingAssetForDoesNotTreatDarwinAsWindows(t *testing.T) {
	assets := []Asset{
		{Name: "alx-darwin-arm64.zip", URL: "mac-arm"},
		{Name: "alx-darwin-amd64.zip", URL: "mac-intel"},
		{Name: "alx-windows-amd64.zip", URL: "windows"},
	}

	if got := matchingAssetFor(assets, "windows", "amd64"); got.URL != "windows" {
		t.Fatalf("Windows asset = %#v, want windows archive", got)
	}
	if got := matchingAssetFor(assets, "darwin", "arm64"); got.URL != "mac-arm" {
		t.Fatalf("Darwin arm64 asset = %#v, want mac arm archive", got)
	}
}

func TestVersionCompareSupportsSemverPrereleases(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.2.3", "v1.2.3-beta.1", 1},
		{"1.2.3-beta.1", "v1.2.3-beta.2", -1},
		{"v1.2.3+build.8", "v1.2.3", 0},
		{"v2.0.0", "v1.99.99", 1},
	}
	for _, test := range tests {
		if got := versionCompare(test.left, test.right); got != test.want {
			t.Errorf("versionCompare(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestMatchingAssetForRequiresExactPlatformAndArchitecture(t *testing.T) {
	assets := []Asset{{Name: "alx-darwin-arm64.zip", URL: "mac-arm"}}
	if got := matchingAssetFor(assets, "windows", "amd64"); got.Name != "" {
		t.Fatalf("Windows should not receive unmatched asset: %#v", got)
	}
}

func TestReleasesForPlatformHidesIncompatibleReleasesAndAssets(t *testing.T) {
	items := []Item{
		{
			Tag: "v2.0.0",
			Assets: []Asset{
				{Name: "alemonx-darwin-arm64.zip", URL: "mac"},
				{Name: "alemonx-windows-amd64.zip", URL: "windows"},
			},
		},
		{
			Tag:    "v1.0.0",
			Assets: []Asset{{Name: "alemonx-linux-amd64.zip", URL: "linux"}},
		},
	}

	got := releasesForPlatform(items, "windows", "amd64")
	if len(got) != 1 || got[0].Tag != "v2.0.0" || len(got[0].Assets) != 1 || got[0].Assets[0].URL != "windows" {
		t.Fatalf("releasesForPlatform() = %#v, want only the matching Windows release asset", got)
	}
}

func TestMatchingAssetForSupportsExtendedArchitectures(t *testing.T) {
	assets := []Asset{
		{Name: "alx-linux-armv7.zip", URL: "linux-arm"},
		{Name: "alx-linux-386.zip", URL: "linux-386"},
		{Name: "alx-linux-ppc64le.zip", URL: "linux-ppc64le"},
		{Name: "alx-linux-s390x.zip", URL: "linux-s390x"},
		{Name: "alx-linux-riscv64.zip", URL: "linux-riscv64"},
		{Name: "alx-windows-arm64.zip", URL: "windows-arm64"},
		{Name: "alx-freebsd-amd64.zip", URL: "freebsd-amd64"},
	}
	for _, test := range []struct {
		platform, architecture, want string
	}{
		{"linux", "arm", "linux-arm"},
		{"linux", "386", "linux-386"},
		{"linux", "ppc64le", "linux-ppc64le"},
		{"linux", "s390x", "linux-s390x"},
		{"linux", "riscv64", "linux-riscv64"},
		{"windows", "arm64", "windows-arm64"},
		{"freebsd", "amd64", "freebsd-amd64"},
	} {
		if got := matchingAssetFor(assets, test.platform, test.architecture); got.URL != test.want {
			t.Fatalf("matchingAssetFor(%s/%s) = %#v, want %q", test.platform, test.architecture, got, test.want)
		}
	}
}

func TestUpdateForReleaseUsesIndexChecksum(t *testing.T) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	item := Item{
		Tag: "v9.9.9",
		URL: "https://example.invalid/release",
		Assets: []Asset{{
			Name:   "alx-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip",
			URL:    "https://example.invalid/alx.zip",
			SHA256: checksum,
		}},
	}
	update, err := updateForRelease(Update{Current: "v1.0.0"}, item)
	if err != nil || !update.Available || update.SHA256 != checksum || !update.IntegrityReady {
		t.Fatalf("indexed update = %#v, %v", update, err)
	}
}

func TestSetupUpdateFreshUsesLatestReleaseAPI(t *testing.T) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	assetName := "alx-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	var sawNoCache bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/SHA256SUMS" {
			_, _ = fmt.Fprintf(w, "%s  %s\n", checksum, assetName)
			return
		}
		if r.Header.Get("Cache-Control") == "no-cache" {
			sawNoCache = true
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"tag_name": "v9.9.9",
			"name":     "Latest",
			"html_url": "https://example.invalid/v9.9.9",
			"assets": []map[string]any{
				{"name": assetName, "browser_download_url": "https://example.invalid/" + assetName},
				{"name": "SHA256SUMS", "browser_download_url": server.URL + "/SHA256SUMS"},
			},
		}})
	}))
	defer server.Close()
	previous := githubReleasesURL
	githubReleasesURL = server.URL + "/repos/%s/releases?per_page=30"
	t.Cleanup(func() { githubReleasesURL = previous })

	update, err := SetupUpdateFresh("v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !sawNoCache || update.Latest != "v9.9.9" || !update.Available || !update.IntegrityReady {
		t.Fatalf("fresh update = %#v, no-cache=%t", update, sawNoCache)
	}
}

func TestWithLatestFirstReplacesStaleCopy(t *testing.T) {
	latest := Item{Tag: "v2.0.0", URL: "new"}
	items := withLatestFirst(latest, []Item{{Tag: "v2.0.0", URL: "old"}, {Tag: "v1.0.0", URL: "older"}})
	if len(items) != 2 || items[0].URL != "new" || items[1].Tag != "v1.0.0" {
		t.Fatalf("latest-first items = %#v", items)
	}
}

func TestChecksumForAssetReadsReleaseManifest(t *testing.T) {
	checksum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  alx-linux-amd64.zip\n", checksum)
	}))
	defer server.Close()

	got, err := checksumForAsset([]Asset{{Name: "SHA256SUMS", URL: server.URL}}, "alx-linux-amd64.zip")
	if err != nil || got != checksum {
		t.Fatalf("checksum = %q, %v", got, err)
	}
}

func TestChecksumForAssetReportsManifestFetchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	if _, err := checksumForAsset([]Asset{{Name: "SHA256SUMS", URL: server.URL}}, "alx-linux-amd64.zip"); err == nil {
		t.Fatal("checksum manifest fetch failure must be reported")
	}
}

func TestPersistedReleaseItemsRemainAvailableAfterRestart(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	want := []Item{{Tag: "v1.2.3", Name: "AlemonX", URL: "https://example.invalid/release"}}
	if err := persistReleaseItems("alemonx", want); err != nil {
		t.Fatal(err)
	}
	got, fetchedAt, ok := readPersistedReleaseItems("alemonx")
	if !ok || len(got) != 1 || got[0].Tag != want[0].Tag || time.Since(fetchedAt) > time.Minute {
		t.Fatalf("persisted releases = %#v, %s, %t", got, fetchedAt, ok)
	}
	if path, err := releaseCachePath("alemonx"); err != nil || filepath.Base(path) != "alemonx.json" {
		t.Fatalf("release cache path = %q, %v", path, err)
	}
}

// pointGitHubAtUnreachable makes the live GitHub request fail fast so tests
// can exercise the offline cache fallback.
func pointGitHubAtUnreachable(t *testing.T) {
	t.Helper()
	previous := githubReleasesURL
	githubReleasesURL = "http://127.0.0.1:1/repos/%s/releases?per_page=30"
	t.Cleanup(func() { githubReleasesURL = previous })
}

func TestListFallsBackToStalePersistedCacheWhenGitHubUnreachable(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	// Use a distinct app id to avoid cross-test in-memory cache pollution.
	id := "alemonapp"
	want := []Item{{Tag: "v1.0.0", Name: "AlemonApp", URL: "https://example.invalid/release"}}
	path, err := releaseCachePath(id)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(persistedReleaseList{
		Items:     want,
		FetchedAt: time.Now().Add(-(releaseListCacheTTL + time.Hour)),
	})
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	pointGitHubAtUnreachable(t)
	items, err := List(id)
	if err != nil {
		t.Fatalf("List should fall back to stale cache, got error: %v", err)
	}
	if len(items) != 1 || items[0].Tag != want[0].Tag {
		t.Fatalf("List = %#v, want cached release %#v", items, want)
	}
}

func TestListReportsErrorWhenGitHubUnreachableAndNoCache(t *testing.T) {
	t.Setenv("ALX_TEST_CACHE_DIR", t.TempDir())
	id := "alemondesk"
	pointGitHubAtUnreachable(t)
	if _, err := List(id); err == nil {
		t.Fatal("List should report an error when GitHub is unreachable and no cache exists")
	}
}
