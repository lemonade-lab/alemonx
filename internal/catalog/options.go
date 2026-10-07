package catalog

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Options reads every page for small pickers; market screens keep server pagination.
func Options(typ string) ([]Resource, error) {
	items := []Resource{}
	for page := 1; page <= 100000; page++ {
		var result ResourcePage
		if err := PlatformGet("resources", url.Values{"type": {typ}, "page": {fmt.Sprint(page)}, "pageSize": {"100"}}, &result); err != nil {
			return nil, err
		}
		items = append(items, result.Data...)
		if len(result.Data) == 0 || result.PageSize <= 0 || page*result.PageSize >= result.Total {
			return items, nil
		}
	}
	return nil, fmt.Errorf("资源目录分页超出限制")
}
func Detail(id string) (Resource, error) {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return Resource{}, fmt.Errorf("资源 ID 无效")
	}
	var response struct {
		Data Resource `json:"data"`
	}
	if err := PlatformGet("resources/"+url.PathEscape(id), nil, &response); err != nil {
		return Resource{}, err
	}
	return response.Data, nil
}
func InstallTarget(resource Resource) (string, error) {
	if resource.InstallMode == "npm" {
		if !regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`).MatchString(resource.PackageName) {
			return "", fmt.Errorf("资源缺少有效 npm 包名")
		}
		return resource.PackageName, nil
	}
	if resource.InstallMode != "" && resource.InstallMode != "git" {
		return "", fmt.Errorf("不支持的资源安装方式")
	}
	parsed, err := url.Parse(resource.RepositoryURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Host != "github.com" && parsed.Host != "gitee.com") {
		return "", fmt.Errorf("资源 Git 仓库地址无效")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
		return "", fmt.Errorf("资源必须提供独立 Git 仓库地址")
	}
	return "git+" + parsed.Scheme + "://" + parsed.Host + "/" + strings.Join(parts, "/") + ".git", nil
}
