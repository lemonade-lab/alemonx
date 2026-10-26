import { Loader2, RefreshCw } from 'lucide-react'
import { useStoreState } from '../store/guideStore'
import { useLazyLocalPackageVersionsQuery } from '../store/workspaceApi'
import type { BackpackPackageUpdateProps } from './BackpackPackageUpdate.d'

export function BackpackPackageUpdate({
  root,
  name,
  busy,
  status,
  onReplace,
  onRefresh
}: BackpackPackageUpdateProps) {
  const [phase, setPhase] = useStoreState<'idle' | 'checking' | 'updating'>(
    'idle'
  )
  const [message, setMessage] = useStoreState('')
  const [failed, setFailed] = useStoreState(false)
  const [readVersions] = useLazyLocalPackageVersionsQuery()
  const update = async () => {
    if (busy || phase !== 'idle') return
    setMessage('')
    setFailed(false)
    setPhase('checking')
    try {
      const result = await readVersions(
        { root, package: name, page: 1 },
        false
      ).unwrap()
      if (!result.latest)
        throw new Error('未找到可更新的版本，请进入插件详情查看。')
      const current = result.current
      const latest = result.latest
      const same =
        current === latest ||
        (result.source === 'git' &&
          current.length >= 7 &&
          latest.startsWith(current))
      if (same) {
        setMessage('已是最新版本')
        return
      }
      if (result.dirty || result.ahead || status?.dirty || status?.ahead) {
        throw new Error('有本地修改或领先提交，请进入插件详情处理后再更新。')
      }
      setPhase('updating')
      if (!(await onReplace(name, latest)))
        throw new Error('更新失败，请查看操作日志后重试。')
      setMessage('已更新')
      onRefresh()
    } catch (reason) {
      setFailed(true)
      const error = reason as { data?: { error?: string }; message?: string }
      setMessage(error.data?.error || error.message || '检查更新失败，请重试。')
    } finally {
      setPhase('idle')
    }
  }
  return (
    <div className="flex max-w-full flex-wrap items-center justify-end gap-2">
      {message && (
        <span
          role={failed ? 'alert' : 'status'}
          className={
            failed
              ? 'text-xs text-amber-700 dark:text-amber-300'
              : 'text-xs text-(--theme-text-secondary)'
          }
        >
          {message}
        </span>
      )}
      <button
        type="button"
        className="secondary-button gap-1.5"
        disabled={busy || phase !== 'idle'}
        onClick={() => void update()}
        aria-label={`更新 ${name}`}
        title="检查最新版本并安全更新"
      >
        {phase === 'idle' ? (
          <RefreshCw className="size-3.5" />
        ) : (
          <Loader2 className="size-3.5 animate-spin" />
        )}
        {phase === 'checking'
          ? '检查中…'
          : phase === 'updating'
            ? '更新中…'
            : '更新'}
      </button>
    </div>
  )
}
