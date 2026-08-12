// Package releases reads public GitHub release metadata for supported AlemonJS apps.
package releases

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"alemonx/internal/systemnetwork"
)

type Item struct {
	Tag         string  `json:"tag"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	PublishedAt string  `json:"publishedAt"`
	Assets      []Asset `json:"assets"`
}
type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

var allowed = map[string]string{"alemondesk": "lemonade-lab/alemondesk", "alemonapp": "lemonade-lab/alemonapp", "alx": "lemonade-lab/alx", "alemonx": "lemonade-lab/alemonx"}

// githubReleasesURL is a package variable so tests can point it at an
// unreachable host and exercise the offline cache fallback path.
var githubReleasesURL = "https://api.github.com/repos/%s/releases?per_page=30"

// latestIndexURL addresses a small release asset rather than the GitHub REST
// API. GitHub serves /releases/latest/download without consuming API quota.
var latestIndexURL = "https://github.com/%s/releases/latest/download/alx-update-index.json"

const releaseListCacheTTL = 12 * time.Hour

type cachedReleaseList struct {
	items     []Item
	expiresAt time.Time
}

type persistedReleaseList struct {
	Items     []Item    `json:"items"`
	FetchedAt time.Time `json:"fetchedAt"`
}

var releaseListCache = struct {
	sync.Mutex
	items map[string]cachedReleaseList
}{items: map[string]cachedReleaseList{}}

type Update struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	Available       bool   `json:"available"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	AssetName       string `json:"assetName,omitempty"`
	SHA256          string `json:"sha256,omitempty"`
	IntegrityError  string `json:"integrityError,omitempty"`
	IntegrityReady  bool   `json:"integrityReady"`
	PlatformMatched bool   `json:"platformMatched"`
	DownloadReady   bool   `json:"downloadReady"`
}

func SetupUpdate(current string) (Update, error) {
	return setupUpdate(current, false)
}

// SetupUpdateFresh bypasses a still-valid local release cache. An explicit
// user recheck should contact the release source when it is available.
func SetupUpdateFresh(current string) (Update, error) {
	result := Update{Current: current}
	// An explicit user recheck must prefer GitHub's release history over the
	// static latest index. CDN caching of the latter can briefly lag a newly
	// published release, while the release API is also what manual install uses.
	items, err := freshReleaseList("alemonx")
	if err == nil && len(items) > 0 {
		return updateForRelease(result, items[0])
	}
	if latest, indexErr := latestRelease("alemonx"); indexErr == nil {
		return updateForRelease(result, latest)
	}
	if err != nil {
		return result, err
	}
	return result, fmt.Errorf("暂未找到可用的正式版本")
}

func setupUpdate(current string, fresh bool) (Update, error) {
	result := Update{Current: current}
	latest, err := latestRelease("alemonx")
	if err == nil {
		return updateForRelease(result, latest)
	}
	items, err := list("alemonx", fresh)
	if err != nil {
		return result, err
	}
	return updateForRelease(result, items[0])
}

func updateForRelease(result Update, latest Item) (Update, error) {
	result.Latest, result.ReleaseURL = latest.Tag, latest.URL
	result.Available = versionCompare(latest.Tag, result.Current) > 0
	if !result.Available {
		return result, nil
	}
	asset := matchingAsset(latest.Assets)
	if asset.Name != "" {
		result.DownloadURL, result.AssetName, result.PlatformMatched = asset.URL, asset.Name, true
		result.SHA256 = asset.SHA256
		var err error
		if result.SHA256 == "" {
			result.SHA256, err = checksumForAsset(latest.Assets, asset.Name)
		}
		if err != nil {
			result.IntegrityError = err.Error()
		}
		result.IntegrityReady = result.SHA256 != ""
	}
	return result, nil
}

func latestRelease(id string) (Item, error) {
	repository, ok := allowed[id]
	if !ok {
		return Item{}, fmt.Errorf("不支持该下载项目")
	}
	response, err := systemnetwork.DefaultClient(8 * time.Second).Get(fmt.Sprintf(latestIndexURL, repository))
	if err != nil {
		return Item{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Item{}, fmt.Errorf("更新索引不可用")
	}
	var item Item
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&item); err != nil || item.Tag == "" || item.URL == "" || len(item.Assets) == 0 {
		return Item{}, fmt.Errorf("更新索引内容无效")
	}
	return item, nil
}

func matchingAsset(assets []Asset) Asset {
	return matchingAssetFor(assets, runtime.GOOS, runtime.GOARCH)
}

// matchingAssetFor compares filename segments instead of substrings. In
// particular, "darwin" contains "win", so a strings.Contains(name, "win")
// check can incorrectly offer a macOS archive to a Windows user.
func matchingAssetFor(assets []Asset, platform, architecture string) Asset {
	for _, asset := range assets {
		tokens := assetNameTokens(asset.Name)
		system := (platform == "darwin" && (tokens["darwin"] || tokens["macos"] || tokens["mac"])) ||
			(platform == "windows" && (tokens["windows"] || tokens["win32"])) ||
			(platform == "linux" && tokens["linux"])
		arch := (architecture == "arm64" && (tokens["arm64"] || tokens["aarch64"])) ||
			(architecture == "amd64" && (tokens["amd64"] || tokens["x64"] || tokens["x86_64"]))
		if system && arch {
			return asset
		}
	}
	return Asset{}
}

func assetNameTokens(name string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	}) {
		tokens[token] = true
	}
	return tokens
}

func versionCompare(left, right string) int {
	left, right = normalizeVersion(left), normalizeVersion(right)
	if semver.IsValid(left) && semver.IsValid(right) {
		return semver.Compare(left, right)
	}
	parse := func(value string) [3]int {
		var result [3]int
		for index, part := range strings.Split(strings.TrimPrefix(strings.TrimSpace(value), "v"), ".") {
			if index >= 3 {
				break
			}
			number, err := strconv.Atoi(part)
			if err != nil {
				return [3]int{}
			}
			result[index] = number
		}
		return result
	}
	l, r := parse(left), parse(right)
	for index := range l {
		if l[index] > r[index] {
			return 1
		}
		if l[index] < r[index] {
			return -1
		}
	}
	return 0
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	return value
}

func checksumForAsset(assets []Asset, name string) (string, error) {
	for _, asset := range assets {
		upper := strings.ToUpper(asset.Name)
		if upper != "SHA256SUMS" && upper != "SHA256SUMS.TXT" && upper != "CHECKSUMS.TXT" {
			continue
		}
		response, err := systemnetwork.DefaultClient(8 * time.Second).Get(asset.URL)
		if err != nil {
			// Keep the underlying error (timeout, DNS, TLS, …) in the console
			// log for diagnosis; the caller must not surface it to the user,
			// who only needs to know the network call failed.
			log.Printf("[releases] 读取发布校验文件失败 %s: %v", asset.URL, err)
			return "", fmt.Errorf("无法读取发布校验文件，请检查网络后重试")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("无法读取发布校验文件")
		}
		for _, line := range strings.Split(string(body), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.EqualFold(strings.TrimPrefix(fields[1], "*"), name) && len(fields[0]) == 64 {
				return strings.ToLower(fields[0]), nil
			}
		}
	}
	return "", nil
}

func List(id string) ([]Item, error) {
	return list(id, false)
}

// ListFresh is used by the manual installer. It bypasses cached history and
// uses the GitHub release list as the source of truth; the static latest index
// remains a fallback when that list cannot be reached.
func ListFresh(id string) ([]Item, error) {
	items, err := list(id, true)
	if err == nil && len(items) > 0 {
		return items, nil
	}
	if latest, latestErr := latestRelease(id); latestErr == nil {
		return []Item{latest}, nil
	}
	return items, err
}

func withLatestFirst(latest Item, items []Item) []Item {
	result := make([]Item, 0, len(items)+1)
	result = append(result, latest)
	for _, item := range items {
		if item.Tag != latest.Tag {
			result = append(result, item)
		}
	}
	return result
}

func list(id string, fresh bool) ([]Item, error) {
	return listWithFallback(id, fresh, true)
}

// freshReleaseList deliberately does not return a stale local release list.
// An explicit automatic-update check must either reach the actual Release API
// or fall back to the release index in SetupUpdateFresh.
func freshReleaseList(id string) ([]Item, error) {
	return listWithFallback(id, true, false)
}

func listWithFallback(id string, fresh, allowStaleFallback bool) ([]Item, error) {
	repository, ok := allowed[id]
	if !ok {
		return nil, fmt.Errorf("不支持该下载项目")
	}
	if !fresh {
		if items, ok := cachedReleaseItems(id); ok {
			return items, nil
		}
		if items, fetchedAt, ok := readPersistedReleaseItems(id); ok && time.Since(fetchedAt) < releaseListCacheTTL {
			cacheReleaseItemsUntil(id, items, fetchedAt.Add(releaseListCacheTTL))
			return items, nil
		}
	}
	client := systemnetwork.DefaultClient(8 * time.Second)
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf(githubReleasesURL, repository), nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建版本列表请求：%w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "AlemonX-Update-Checker")
	if fresh {
		request.Header.Set("Cache-Control", "no-cache")
		request.Header.Set("Pragma", "no-cache")
	}
	response, err := client.Do(request)
	if err != nil {
		if allowStaleFallback {
			if items, ok := staleCachedReleaseItems(id); ok {
				return items, nil
			}
			if item, indexErr := latestRelease(id); indexErr == nil {
				return []Item{item}, nil
			}
		}
		return nil, fmt.Errorf("无法获取版本列表，请检查网络后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if allowStaleFallback {
			if items, ok := staleCachedReleaseItems(id); ok {
				return items, nil
			}
			if item, indexErr := latestRelease(id); indexErr == nil {
				return []Item{item}, nil
			}
		}
		if response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, fmt.Errorf("GitHub API 请求次数已用尽，请稍后重试或直接选择本地安装包")
		}
		return nil, fmt.Errorf("GitHub 暂时无法提供版本列表")
	}
	var data []struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		Draft       bool      `json:"draft"`
		Prerelease  bool      `json:"prerelease"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("版本列表内容无法识别")
	}
	items := make([]Item, 0, len(data))
	for _, item := range data {
		if item.Draft || item.Prerelease {
			continue
		}
		name := item.Name
		if name == "" {
			name = item.TagName
		}
		assets := make([]Asset, 0, len(item.Assets))
		for _, asset := range item.Assets {
			assets = append(assets, Asset{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
		}
		items = append(items, Item{Tag: item.TagName, Name: name, URL: item.HTMLURL, PublishedAt: item.PublishedAt.Format(time.RFC3339), Assets: assets})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("暂未找到可用的正式版本")
	}
	cacheReleaseItems(id, items)
	_ = persistReleaseItems(id, items)
	return items, nil
}

func cachedReleaseItems(id string) ([]Item, bool) {
	releaseListCache.Lock()
	defer releaseListCache.Unlock()
	entry, ok := releaseListCache.items[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.items, true
}

func cacheReleaseItems(id string, items []Item) {
	cacheReleaseItemsUntil(id, items, time.Now().Add(releaseListCacheTTL))
}

func cacheReleaseItemsUntil(id string, items []Item, expiresAt time.Time) {
	releaseListCache.Lock()
	defer releaseListCache.Unlock()
	releaseListCache.items[id] = cachedReleaseList{items: items, expiresAt: expiresAt}
}

func staleCachedReleaseItems(id string) ([]Item, bool) {
	releaseListCache.Lock()
	entry, ok := releaseListCache.items[id]
	releaseListCache.Unlock()
	if ok && len(entry.items) > 0 {
		return entry.items, true
	}
	items, _, ok := readPersistedReleaseItems(id)
	return items, ok
}

func releaseCachePath(id string) (string, error) {
	base := os.Getenv("ALX_TEST_CACHE_DIR")
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", err
		}
	}
	directory := filepath.Join(base, "alemonjs", "alx", "releases")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	return filepath.Join(directory, id+".json"), nil
}

func readPersistedReleaseItems(id string) ([]Item, time.Time, bool) {
	path, err := releaseCachePath(id)
	if err != nil {
		return nil, time.Time{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, false
	}
	var cached persistedReleaseList
	if json.Unmarshal(body, &cached) != nil || cached.FetchedAt.IsZero() || len(cached.Items) == 0 {
		return nil, time.Time{}, false
	}
	return cached.Items, cached.FetchedAt, true
}

func persistReleaseItems(id string, items []Item) error {
	path, err := releaseCachePath(id)
	if err != nil {
		return err
	}
	body, err := json.Marshal(persistedReleaseList{Items: items, FetchedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	temporary := path + ".new"
	if err := os.WriteFile(temporary, body, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
