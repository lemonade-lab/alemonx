import type { MarketResource, PackageFact } from '../store/workspaceApi'

export function resourceTarget(resource: MarketResource) {
  if (resource.installMode === 'npm')
    return /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/.test(
      resource.packageName ?? ''
    )
      ? resource.packageName!
      : ''
  if (resource.installMode && resource.installMode !== 'git') return ''
  try {
    const url = new URL(resource.repositoryUrl)
    const parts = url.pathname
      .replace(/\.git$/, '')
      .split('/')
      .filter(Boolean)
    return url.protocol === 'https:' &&
      ['github.com', 'gitee.com'].includes(url.host) &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash &&
      parts.length === 2 &&
      parts.every(part => !part.includes('..'))
      ? `git+${url.origin}/${parts.join('/')}.git`
      : ''
  } catch {
    return ''
  }
}
export const repositoryKey = (value = '') =>
  value
    .replace(/^git\+/, '')
    .split('#')[0]
    .replace(/\.git$/, '')
    .replace(/\/$/, '')
export function findResource(
  resources: MarketResource[],
  name: string,
  repository?: string
) {
  return resources.find(
    resource =>
      (resource.installMode === 'npm' && resource.packageName === name) ||
      Boolean(
        repository &&
        repositoryKey(repository) === repositoryKey(resource.repositoryUrl)
      )
  )
}
export function factFor(resource: MarketResource, facts: PackageFact[]) {
  return facts.find(fact =>
    resource.installMode === 'npm'
      ? fact.name === resource.packageName
      : Boolean(
          fact.repository &&
          repositoryKey(fact.repository) ===
            repositoryKey(resource.repositoryUrl)
        )
  )
}
export function packageState(fact?: PackageFact) {
  return !fact
    ? '未安装'
    : !fact.installed
      ? '已声明，未安装'
      : fact.loadable
        ? fact.enabled
          ? '已安装 · 已启用'
          : '已安装 · 未启用'
        : '已安装'
}
