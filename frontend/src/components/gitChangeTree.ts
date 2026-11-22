export const changeStatusLabel = (status: string) => {
  const codes = status.replace(/\s/g, '')
  if (codes === '??') return '新增'
  if (codes.includes('D')) return '删除'
  if (codes.includes('R')) return '重命名'
  if (codes.includes('A')) return '新增'
  if (codes.includes('M')) return '修改'
  return status || '变更'
}
