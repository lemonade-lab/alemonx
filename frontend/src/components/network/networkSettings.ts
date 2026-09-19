import type {
  SystemNetworkMode,
  SystemNetworkGroupState
} from '../../store/workspaceApi'

export const networkModes: Record<SystemNetworkMode, string> = {
  auto: '自动',
  direct: '直连',
  proxy: '代理'
}
export const networkInputClass =
  'w-full min-w-0 rounded-md border border-(--theme-border-default) bg-(--theme-surface-raised) px-3 py-2 text-sm text-(--theme-text-primary) outline-none focus:border-(--theme-accent) focus:ring-2 focus:ring-(--theme-accent-soft-border)'
export function networkError(error: unknown) {
  if (error instanceof Error) return error.message
  if (typeof error === 'object' && error && 'data' in error) {
    const data = (error as { data?: { error?: string; message?: string } }).data
    if (data?.error || data?.message)
      return data.error ?? data.message ?? '操作失败，请重试'
  }
  return '操作失败，请重试'
}
export function groupStatus(state?: SystemNetworkGroupState) {
  if (!state || state.state === 'idle') return '待检测'
  if (state.state === 'checking') return '检测中…'
  if (state.state === 'unavailable') return '没有可用入口'
  return `可用 · ${state.latencyMs} ms`
}
export function entryLabel(entry?: string) {
  return entry === 'official' ? '官方地址' : entry || '尚未选择'
}
