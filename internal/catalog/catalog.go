// Package catalog reads the ALemon public resource platform and npm versions.
package catalog

import (
	"alemonx/internal/httpcache"
	"alemonx/internal/systemnetwork"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Item struct {
	Resource
	URL     string `json:"url"`
	Install string `json:"install"`
}
type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Items []Item `json:"items"`
}
type Document struct {
	Source   string `json:"source"`
	Markdown string `json:"markdown"`
}
type PackageVersions struct {
	Latest   string   `json:"latest"`
	Versions []string `json:"versions"`
}

// Fetch supplies sidebar categories from the platform taxonomy.
func Categories(kind string) ([]Group, error) {
	typ := "js-plugin"
	if kind == "environment" {
		typ = "connector"
	} else if kind != "apps" && kind != "modules" {
		return nil, fmt.Errorf("不支持的生态目录")
	}
	var response struct {
		Data []ResourceSubtype `json:"data"`
	}
	if err := PlatformGet("resource-subtypes", url.Values{"type": {typ}}, &response); err != nil {
		return nil, err
	}
	groups := []Group{{Title: "全部", Items: []Item{}}}
	for _, subtype := range response.Data {
		groups = append(groups, Group{ID: subtype.ID, Title: subtype.Name, Items: []Item{}})
	}
	return groups, nil
}

// Fetch preserves the MCP grouped response while using platform resources only.
func Fetch(kind string) ([]Group, error) {
	groups, err := Categories(kind)
	if err != nil {
		return nil, err
	}
	typ := "js-plugin"
	if kind == "environment" {
		typ = "connector"
	}
	for page := 1; ; page++ {
		var result ResourcePage
		if err := PlatformGet("resources", url.Values{"type": {typ}, "page": {fmt.Sprint(page)}, "pageSize": {"100"}}, &result); err != nil {
			return nil, err
		}
		for _, resource := range result.Data {
			install := resource.PackageName
			if resource.InstallMode != "npm" {
				install = "git+" + resource.RepositoryURL
			}
			item := Item{Resource: resource, URL: resource.RepositoryURL, Install: install}
			groups[0].Items = append(groups[0].Items, item)
			for i := 1; i < len(groups); i++ {
				if groups[i].ID == resource.Subtype {
					groups[i].Items = append(groups[i].Items, item)
				}
			}
		}
		if len(result.Data) == 0 || result.PageSize <= 0 || page*result.PageSize >= result.Total {
			break
		}
	}
	return groups, nil
}
func LoadDocument(id string) (Document, error) {
	var result struct {
		Data Resource `json:"data"`
	}
	if id == "" || strings.ContainsAny(id, "/?#") {
		return Document{}, fmt.Errorf("请提供平台资源 ID")
	}
	if err := PlatformGet("resources/"+url.PathEscape(id), nil, &result); err != nil {
		return Document{}, err
	}
	return Document{Source: "https://open.alemonjs.com/resources/" + url.PathEscape(id), Markdown: result.Data.Markdown}, nil
}
func LoadPackageVersions(name string) (PackageVersions, error) {
	name = strings.TrimSpace(name)
	if !regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`).MatchString(name) {
		return PackageVersions{}, fmt.Errorf("该目录条目不是可查询版本的 npm 包")
	}
	endpoint := "https://registry.npmjs.org/" + url.PathEscape(name)
	client := systemnetwork.DefaultClient(8 * time.Second)
	response, err := httpcache.GetWithHeaders(client, endpoint, 15*time.Minute, map[string]string{"Accept": "application/json"})
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

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}
