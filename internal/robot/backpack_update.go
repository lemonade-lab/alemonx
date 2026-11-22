package robot

import (
	"errors"
	"fmt"
	"strings"
)

// SafeUpdateBackpackPlugins follows the workbench's bulk-sync policy. The
// normal switch rechecks the worktree after fetching before changing it.
func (m Manager) SafeUpdateBackpackPlugins(root string, packages []string) (Result, error) {
	messages := make([]string, 0, len(packages))
	failed := false
	for _, name := range packages {
		item, err := m.localPackage(root, name)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s：检查失败：%v", name, err))
			failed = true
			continue
		}
		if !isLocalPackageRepository(item.Path) {
			messages = append(messages, name+"：跳过，无法验证非 Git 插件是否有本地改动；请确认后指定版本切换")
			continue
		}
		versions, err := m.LocalPackageVersions(root, name, 1)
		switch {
		case err != nil:
			messages = append(messages, fmt.Sprintf("%s：检查失败：%v", name, err))
			failed = true
		case versions.Dirty || versions.Ahead > 0:
			messages = append(messages, name+"：跳过，存在本地修改或领先提交")
		case versions.Latest == "":
			messages = append(messages, name+"：跳过，找不到可用版本")
		case versions.Latest == versions.Current ||
			(versions.Source == "git" && len(versions.Current) >= 7 && strings.HasPrefix(versions.Latest, versions.Current)):
			messages = append(messages, name+"：已是最新版本")
		default:
			result, err := m.Run(root, "sync-local-package-release", "", name, versions.Latest, "", "", true)
			if err != nil {
				messages = append(messages, fmt.Sprintf("%s：更新失败：%v\n%s", name, err, result.Output))
				failed = true
			} else {
				messages = append(messages, name+"：已更新到 "+versions.Latest)
			}
		}
	}
	result := Result{Path: root, Output: strings.Join(messages, "\n")}
	if failed {
		return result, errors.New("部分背包插件未能更新，请查看任务输出")
	}
	return result, nil
}
