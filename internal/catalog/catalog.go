// Package catalog retrieves and formats the official AlemonJS ecosystem catalog.
package catalog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"alemonx/internal/httpcache"
	"alemonx/internal/packageschema"
	"alemonx/internal/systemnetwork"
)

type Item struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Install     string `json:"install"`
}
type Group struct {
	Title string `json:"title"`
	Items []Item `json:"items"`
}

type Document struct {
	Source   string `json:"source"`
	Markdown string `json:"markdown"`
}

// PackageVersions is the small, UI-safe portion of the npm package document.
// It lets the install screen offer published versions without exposing the
// registry's full metadata payload to the browser.
type PackageVersions struct {
	Latest   string   `json:"latest"`
	Versions []string `json:"versions"`
}

type PackageConfig struct {
	Package       string                     `json:"package"`
	Namespace     string                     `json:"namespace"`
	Fields        []packageschema.Field      `json:"fields"`
	Values        map[string]any             `json:"values"`
	ConfigSource  packageschema.ConfigSource `json:"configSource,omitempty"`
	Logo          string                     `json:"logo,omitempty"`
	Commands      []packageschema.Command    `json:"commands,omitempty"`
	Platforms     []packageschema.Platform   `json:"platforms,omitempty"`
	WebServerPort bool                       `json:"webServerPort,omitempty"`
}

var sources = map[string]string{
	"apps":        "https://raw.githubusercontent.com/lemonade-lab/alemonjs.dev/main/docs/apps.md",
	"environment": "https://raw.githubusercontent.com/lemonade-lab/alemonjs.dev/main/docs/environment.md",
	"modules":     "https://raw.githubusercontent.com/lemonade-lab/alemonjs.dev/main/docs/apps-module.md",
}

func Fetch(kind string) ([]Group, error) {
	raw, ok := sources[kind]
	if !ok {
		return nil, fmt.Errorf("不支持的生态目录")
	}
	client := systemnetwork.DefaultClient(8 * time.Second)
	var body []byte
	if candidate := jsDelivrURL(raw); candidate != "" {
		if response, err := httpcache.Get(client, candidate, 10*time.Minute); err == nil && response.Status == http.StatusOK {
			body = response.Body
		}
	}
	if body == nil {
		response, err := httpcache.Get(client, raw, 10*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("无法读取官方目录，请检查网络后重试")
		}
		if response.Status != http.StatusOK {
			return nil, fmt.Errorf("官方目录暂时不可用")
		}
		body = response.Body
	}
	groups, references, err := parseCatalog(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for groupIndex := range groups {
		for itemIndex := range groups[groupIndex].Items {
			item := &groups[groupIndex].Items[itemIndex]
			if item.URL == "" {
				item.URL = references[item.Name]
			}
			item.Install = catalogInstall(kind, *item)
		}
	}
	return groups, nil
}

// catalogInstall keeps JS modules as project dependencies even when their
// documentation happens to link to a source repository. Apps are deliberately
// different: they are installed into the robot backpack from their release
// branch. The catalog documentation commonly links to main or a version tag,
// neither of which is the distributable branch for a robot plugin.
func catalogInstall(kind string, item Item) string {
	// Connections and modules are normal npm dependencies. Only robot plugins
	// are source worktrees, always checked out from their release branch.
	if kind == "modules" || kind == "environment" {
		return item.Name
	}
	if repository := repositoryURL(item.URL); repository != "" {
		return "git+" + repository + ".git#release"
	}
	return ""
}

// jsDelivrURL converts a raw.githubusercontent.com URL into its jsDelivr CDN
// mirror. jsDelivr serves cached GitHub content without touching GitHub's
// rate-limited endpoints, so it is preferred for README/package.json reads.
func jsDelivrURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host != "raw.githubusercontent.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 {
		return ""
	}
	return "https://cdn.jsdelivr.net/gh/" + parts[0] + "/" + parts[1] + "@" + parts[2] + "/" + strings.Join(parts[3:], "/")
}

// parseCatalog keeps the meaning of a Markdown table instead of assuming that
// the second column is always its description. Connection tables, for example,
// use “项目 | 版本 | 说明”; the version badge is not user-facing copy.
//
// The description column is selected by its header (说明 / docs / description).
// Old two-column catalogs without such a header retain the former second-column
// fallback for backwards compatibility.
func parseCatalog(reader io.Reader) ([]Group, map[string]string, error) {
	var groups []Group
	references := map[string]string{}
	current := -1
	columns := catalogColumns{name: 0, description: -1}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if name, link, ok := referenceDefinition(line); ok {
			references[name] = link
			continue
		}
		if strings.HasPrefix(line, "### ") {
			groups = append(groups, Group{Title: strings.TrimSpace(strings.TrimPrefix(line, "### "))})
			current = len(groups) - 1
			columns = catalogColumns{name: 0, description: -1}
			continue
		}
		if current < 0 || !strings.HasPrefix(line, "|") || isMarkdownTableDivider(line) {
			continue
		}
		values := markdownTableValues(line)
		if len(values) < 2 {
			continue
		}
		if header, ok := parseCatalogTableHeader(values); ok {
			columns = header
			continue
		}
		nameIndex := columns.name
		if nameIndex < 0 || nameIndex >= len(values) {
			nameIndex = 0
		}
		name, link := markdownLink(values[nameIndex])
		if name == "" || isCatalogTableHeader(name) {
			continue
		}
		descriptionIndex := columns.description
		if descriptionIndex < 0 {
			descriptionIndex = 1
		}
		description := ""
		if descriptionIndex < len(values) {
			description = strings.TrimSpace(values[descriptionIndex])
		}
		groups[current].Items = append(groups[current].Items, Item{Name: name, URL: link, Description: description})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("读取官方目录失败")
	}
	if groups == nil {
		groups = []Group{}
	}
	return groups, references, nil
}

type catalogColumns struct {
	name        int
	description int
}

func markdownTableValues(line string) []string {
	values := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}

func isMarkdownTableDivider(line string) bool {
	for _, value := range markdownTableValues(line) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, char := range value {
			if char != '-' && char != ':' {
				return false
			}
		}
	}
	return true
}

func parseCatalogTableHeader(values []string) (catalogColumns, bool) {
	columns := catalogColumns{name: -1, description: -1}
	for index, value := range values {
		switch normalizeCatalogHeader(value) {
		case "项目", "项目名", "project", "package", "name":
			columns.name = index
		case "说明", "描述", "简介", "description", "desc", "docs", "doc", "documentation":
			columns.description = index
		}
	}
	return columns, columns.name >= 0
}

func normalizeCatalogHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

// isCatalogTableHeader keeps Markdown column labels out of the selectable
// ecosystem entries. The environment catalog uses “项目”, while older pages
// used “项目名”; both must be treated as table structure, not a package.
func isCatalogTableHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "项目" || name == "项目名" || name == "project" || name == "package"
}

func repositoryURL(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Host != "github.com" && parsed.Host != "gitee.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return "https://" + parsed.Host + "/" + parts[0] + "/" + strings.TrimSuffix(parts[1], ".git")
}

// repositoryInstallURL retains a tree/blob ref from the official catalog. A
// link such as /tree/v1.2.3/packages/foo must install v1.2.3, not silently
// clone the repository's moving default branch.
func repositoryInstallURL(source string) string {
	base := repositoryURL(source)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return base
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") && validGitRef(parts[3]) {
		return base + ".git#" + parts[3]
	}
	return base + ".git"
}

func validGitRef(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") || strings.Contains(value, "..") || strings.ContainsAny(value, "\\~^:?*[ \t\r\n") {
		return false
	}
	return true
}

// Document loads an online README only from the repository hosts represented
// by the official catalog. This keeps the local API from becoming a general
// network proxy while still allowing catalog entries to render their docs.
func LoadDocument(source string) (Document, error) {
	data, candidate, err := loadRepositoryFile(source, "README.md")
	if err != nil {
		return Document{}, err
	}
	return Document{Source: candidate, Markdown: string(data)}, nil
}

// LoadPackageVersions returns installable versions for a catalog entry. npm
// packages use registry versions; repository-backed plugins use published
// Release tags. A source checkout without a Release must never be presented as
// a versioned plugin.
func LoadPackageVersions(name string) (PackageVersions, error) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "git+") {
		return loadRepositoryReleases(strings.TrimPrefix(name, "git+"))
	}
	if !regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`).MatchString(name) {
		return PackageVersions{}, fmt.Errorf("该目录条目不是可查询版本的 npm 包")
	}
	endpoint := "https://registry.npmjs.org/" + url.PathEscape(name)
	client := systemnetwork.DefaultClient(8 * time.Second)
	response, err := httpcache.GetWithHeaders(client, endpoint, 15*time.Minute, map[string]string{"Accept": "application/vnd.github+json"})
	if err != nil || response.Status != http.StatusOK {
		return PackageVersions{}, fmt.Errorf("无法读取 npm 版本列表，请检查网络后重试")
	}
	var metadata struct {
		DistTags map[string]string `json:"dist-tags"`
		Versions map[string]any    `json:"versions"`
		Time     map[string]string `json:"time"`
	}
	if err := json.Unmarshal(response.Body, &metadata); err != nil {
		return PackageVersions{}, fmt.Errorf("npm 版本列表无法识别")
	}
	versions := make([]string, 0, len(metadata.Versions))
	for version := range metadata.Versions {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return metadata.Time[versions[i]] > metadata.Time[versions[j]] })
	if latest := metadata.DistTags["latest"]; latest != "" {
		versions = append([]string{latest}, versions...)
	}
	return PackageVersions{Latest: metadata.DistTags["latest"], Versions: uniqueStrings(versions)}, nil
}

// loadRepositoryReleases deliberately reads release tag_name values rather
// than the repository's complete Git tag list. A Git repository often carries
// experimental tags; only a published Release is an installable plugin version.
func loadRepositoryReleases(source string) (PackageVersions, error) {
	parsed, err := url.Parse(strings.SplitN(source, "#", 2)[0])
	if err != nil || (parsed.Host != "github.com" && parsed.Host != "gitee.com") {
		return PackageVersions{}, fmt.Errorf("该插件仓库不支持读取版本 tag")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return PackageVersions{}, fmt.Errorf("插件仓库地址无效")
	}
	endpoint := ""
	if parsed.Host == "github.com" {
		endpoint = "https://api.github.com/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/releases?per_page=100"
	} else {
		endpoint = "https://gitee.com/api/v5/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) + "/releases?per_page=100"
	}
	client := systemnetwork.DefaultClient(8 * time.Second)
	response, err := httpcache.Get(client, endpoint, 15*time.Minute)
	if err != nil || response.Status != http.StatusOK {
		return PackageVersions{}, fmt.Errorf("无法读取插件 Release，请检查网络后重试")
	}
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(response.Body, &releases); err != nil {
		return PackageVersions{}, fmt.Errorf("插件 Release 无法识别")
	}
	versions := make([]string, 0, len(releases))
	for _, release := range releases {
		// A published prerelease is still a real GitHub Release and is useful
		// for repository-backed plugins that have not cut a stable version yet.
		if release.TagName != "" && !release.Draft {
			versions = append(versions, release.TagName)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return gitTagHigher(versions[i], versions[j]) })
	versions = uniqueStrings(versions)
	if len(versions) == 0 {
		// GitHub hides draft releases from the public API. The repository page
		// can still show those releases to an authenticated user, so fall back
		// to published-looking tags instead of reporting a false empty result.
		return loadRepositoryTags(parsed.Host, parts[0], parts[1])
	}
	return PackageVersions{Latest: versions[0], Versions: versions}, nil
}

func loadRepositoryTags(host, owner, repository string) (PackageVersions, error) {
	endpoint := "https://" + host + "/api/v5/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/tags?per_page=100"
	if host == "github.com" {
		endpoint = "https://api.github.com/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/tags?per_page=100"
	}
	response, err := httpcache.GetWithHeaders(systemnetwork.DefaultClient(8*time.Second), endpoint, 15*time.Minute, map[string]string{"Accept": "application/vnd.github+json"})
	if err != nil || response.Status != http.StatusOK {
		return PackageVersions{}, fmt.Errorf("无法读取插件版本，请检查网络后重试")
	}
	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(response.Body, &tags); err != nil {
		return PackageVersions{}, fmt.Errorf("插件版本无法识别")
	}
	versions := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag.Name != "" {
			versions = append(versions, tag.Name)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return gitTagHigher(versions[i], versions[j]) })
	versions = uniqueStrings(versions)
	if len(versions) == 0 {
		return PackageVersions{Versions: []string{}}, nil
	}
	return PackageVersions{Latest: versions[0], Versions: versions}, nil
}

// gitTagHigher keeps semantic-looking release tags ahead of arbitrary tags,
// then compares numeric components via the standard semver module.
func gitTagHigher(left, right string) bool {
	canonical := func(tag string) (string, bool) {
		tag = strings.TrimSpace(tag)
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		if !semver.IsValid(tag) {
			return "", false
		}
		return semver.Canonical(tag), true
	}
	leftVersion, leftOK := canonical(left)
	rightVersion, rightOK := canonical(right)
	if leftOK != rightOK {
		return leftOK
	}
	if leftOK {
		return semver.Compare(leftVersion, rightVersion) > 0
	}
	return left > right
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func LoadPackageConfig(source string) (PackageConfig, error) {
	data, _, err := loadRepositoryFile(source, "package.json")
	if err != nil {
		return PackageConfig{}, err
	}
	declaration, err := packageschema.Parse(data)
	if err != nil {
		return PackageConfig{}, fmt.Errorf("在线 %s", err)
	}
	config := PackageConfig{
		Package:       declaration.Name,
		Namespace:     declaration.ResolveNamespace(),
		Fields:        declaration.Config,
		Values:        map[string]any{},
		ConfigSource:  declaration.ConfigSource,
		Logo:          declaration.Desktop.Logo,
		Commands:      declaration.Desktop.Commands,
		Platforms:     declaration.Desktop.Platforms,
		WebServerPort: declaration.Web.ServerPort,
	}
	if config.Fields == nil {
		config.Fields = []packageschema.Field{}
	}
	return config, nil
}

func loadRepositoryFile(source, filename string) ([]byte, string, error) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "https" {
		return nil, "", fmt.Errorf("文档地址无效")
	}
	candidates, err := repositoryFileCandidates(parsed, filename)
	if err != nil {
		return nil, "", err
	}
	// A manifest defines executable configuration, so it must not silently be
	// read from an eventually-consistent CDN snapshot of a moving branch. CDN
	// files remain a resilience fallback, but GitHub/Gitee's source endpoint
	// wins whenever it is reachable.
	if filename == "package.json" {
		candidates = preferAuthoritativeManifestCandidates(candidates)
	}
	client := systemnetwork.DefaultClient(10 * time.Second)
	for _, candidate := range candidates {
		response, fetchErr := httpcache.Get(client, candidate, 10*time.Minute)
		if fetchErr != nil || response.Status != http.StatusOK {
			continue
		}
		return response.Body, candidate, nil
	}
	return nil, "", fmt.Errorf("暂时无法读取在线文档")
}

func preferAuthoritativeManifestCandidates(candidates []string) []string {
	primary := make([]string, 0, len(candidates))
	fallback := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parsed, err := url.Parse(candidate)
		if err == nil && parsed.Host == "cdn.jsdelivr.net" {
			fallback = append(fallback, candidate)
			continue
		}
		primary = append(primary, candidate)
	}
	return append(primary, fallback...)
}

// repositoryFileCandidates keeps a catalog URL's file or directory context.
// A blob/raw Markdown URL is read as-is; a tree/directory URL resolves files
// in that directory, so packages can live below a repository root.
func repositoryFileCandidates(parsed *url.URL, filename string) ([]string, error) {
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	isDocument := filename == "README.md"
	var candidates []string
	switch parsed.Host {
	case "raw.githubusercontent.com":
		if isDocument {
			candidates = []string{parsed.String()}
		} else {
			if len(parts) < 4 {
				return nil, fmt.Errorf("仓库地址无效")
			}
			candidates = []string{"https://raw.githubusercontent.com/" + path.Join(append(parts[:len(parts)-1], filename)...)}
		}
	case "github.com":
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("仓库地址无效")
		}
		repo := path.Join(parts[0], strings.TrimSuffix(parts[1], ".git"))
		candidates = githubCandidates(repo, parts[2:], filename, isDocument)
	case "gitee.com":
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("仓库地址无效")
		}
		repo := path.Join(parts[0], strings.TrimSuffix(parts[1], ".git"))
		candidates = giteeCandidates(repo, parts[2:], filename, isDocument)
	default:
		return nil, fmt.Errorf("暂不支持该文档来源")
	}
	// jsDelivr mirrors GitHub raw files on its own CDN, which is not subject
	// to GitHub's API/raw rate limits. Try it before direct GitHub hosts and
	// keep the direct URLs as fallbacks.
	js := []string{}
	for _, candidate := range candidates {
		if mirror := jsDelivrURL(candidate); mirror != "" {
			js = append(js, mirror)
		}
	}
	return append(js, candidates...), nil
}

func githubCandidates(repo string, suffix []string, filename string, isDocument bool) []string {
	branches, directory, direct := sourceLocation(suffix, isDocument)
	result := make([]string, 0, len(branches))
	for _, branch := range branches {
		target := append([]string{repo, branch}, directory...)
		if !direct {
			target = append(target, filename)
		}
		result = append(result, "https://raw.githubusercontent.com/"+path.Join(target...))
	}
	return result
}

func giteeCandidates(repo string, suffix []string, filename string, isDocument bool) []string {
	branches, directory, direct := sourceLocation(suffix, isDocument)
	result := make([]string, 0, len(branches))
	for _, branch := range branches {
		target := append([]string{repo, "raw", branch}, directory...)
		if !direct {
			target = append(target, filename)
		}
		result = append(result, "https://gitee.com/"+path.Join(target...))
	}
	return result
}

func sourceLocation(suffix []string, isDocument bool) (branches, directory []string, direct bool) {
	if len(suffix) >= 2 && (suffix[0] == "blob" || suffix[0] == "tree") {
		branches, directory = []string{suffix[1]}, suffix[2:]
		direct = suffix[0] == "blob" && isDocument
		if !isDocument && suffix[0] == "blob" && len(directory) > 0 && strings.Contains(path.Base(directory[len(directory)-1]), ".") {
			directory = directory[:len(directory)-1]
		}
		return
	}
	if isDocument && len(suffix) > 0 && strings.HasSuffix(strings.ToLower(suffix[len(suffix)-1]), ".md") {
		return []string{"main", "master"}, suffix, true
	}
	if !isDocument && len(suffix) > 0 && strings.Contains(path.Base(suffix[len(suffix)-1]), ".") {
		suffix = suffix[:len(suffix)-1]
	}
	return []string{"main", "master"}, suffix, false
}

func markdownLink(value string) (string, string) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") {
		return value, ""
	}
	end := strings.Index(value, "](")
	if end >= 0 && strings.HasSuffix(value, ")") {
		return value[1:end], value[end+2 : len(value)-1]
	}
	if strings.HasSuffix(value, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"), ""
	}
	return value, ""
}

func referenceDefinition(value string) (string, string, bool) {
	if !strings.HasPrefix(value, "[") {
		return "", "", false
	}
	end := strings.Index(value, "]:")
	if end < 1 {
		return "", "", false
	}
	name := value[1:end]
	link := strings.TrimSpace(value[end+2:])
	return name, link, name != "" && link != ""
}
