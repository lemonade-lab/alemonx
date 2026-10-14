import type { GitActionInput, GitWorkspace } from '../store/workspaceApi'

export function describeGitCommand(
  command: Omit<GitActionInput, 'root'>,
  data?: GitWorkspace
): string {
  const { action, value, message, paths, remote, expectedBranch } = command
  if (paths)
    return `${action === 'commit' ? '仅提交已暂存内容' : action === 'restore' ? '撤销所选文件未暂存的修改，保留已暂存内容；撤销的修改无法自动恢复' : action === 'unstage' ? '取消所选文件的暂存，保留工作区改动' : '暂存所选文件的当前内容'}：\n${paths.join('\n')}${message ? `\n\n说明：${message}` : ''}`
  const branch = expectedBranch || '未选择'
  const tracked =
    remote && data?.upstream?.startsWith(`${remote}/`)
      ? data.upstream
      : `${remote || '未配置'}/${branch}`
  switch (action) {
    case 'fetch':
      return `获取 ${remote || '未配置'} 的分支和标签引用。`
    case 'pull':
      return `更新分支：${value || branch}\n来源：${tracked}\n更新方式：仅快进，分叉时保留本地提交并提示处理。`
    case 'push':
      return `推送分支：${branch}\n目标：${tracked}\n推送已提交的内容。`
    case 'tag-push':
      return `推送标签：${value}\n目标仓库：${remote || '未配置'}`
    case 'merge':
      return `将 ${value?.replace(/^refs\/remotes\//, '')} 合并到当前分支 ${branch}。`
    case 'merge-abort':
      return `中止 ${branch} 上正在进行的合并。`
    case 'branch-switch':
      return `从 ${branch} 切换到 ${value}。`
    case 'branch-create':
      return `从当前分支 ${branch} 新建 ${value} 并切换。`
    case 'branch-track':
      return `从 ${value} 创建本地跟踪分支并切换。`
    case 'branch-delete':
      return `删除本地分支 ${value}。`
    case 'stash-save':
      return `临时保存 ${branch} 的全部改动，包括新增文件和暂存内容，并清理工作区。`
    case 'stash-apply':
    case 'stash-drop': {
      const stash = data?.stashes.find(item => item.sha === value)
      const target = stash ? `${stash.ref} · ${stash.subject}` : value
      return action === 'stash-apply'
        ? `恢复临时保存：${target}\n恢复工作区与暂存状态，保留这条保存记录。`
        : `删除临时保存记录：${target}\n请确认其中的改动已经恢复或不再需要。`
    }
    case 'remote-remove':
      return `移除远程仓库 ${value} 的本地配置，保留远程服务器上的仓库。`
    default:
      return `${value ? `目标：${value}\n` : ''}${message ? `说明：${message}` : '请确认继续。'}`
  }
}
