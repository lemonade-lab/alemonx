import type { GitChange as Change } from '../store/workspaceApi'

export type ChangeTreeNode = {
  name: string
  path: string
  change?: Change
  children: Map<string, ChangeTreeNode>
}

export const changeStatusLabel = (status: string) => {
  const codes = status.replace(/\s/g, '')
  if (codes === '??') return '新增'
  if (codes.includes('D')) return '删除'
  if (codes.includes('R')) return '重命名'
  if (codes.includes('A')) return '新增'
  if (codes.includes('M')) return '修改'
  return status || '变更'
}

export function buildChangeTree(changes: Change[]) {
  const root: ChangeTreeNode = { name: '', path: '', children: new Map() }
  for (const change of changes) {
    const parts = change.path.replace(/\/$/, '').split('/').filter(Boolean)
    let parent = root
    parts.forEach((name, index) => {
      const path = parent.path ? `${parent.path}/${name}` : name
      const existing = parent.children.get(name) ?? {
        name,
        path,
        children: new Map<string, ChangeTreeNode>()
      }
      if (index === parts.length - 1) existing.change = change
      parent.children.set(name, existing)
      parent = existing
    })
  }
  return [...root.children.values()]
}
