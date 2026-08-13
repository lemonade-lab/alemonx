import cn from 'classnames'

type Props = {
  label: string
  detail?: string
  progress?: number | null
  handoff?: boolean
  className?: string
}

/**
 * A truthful transfer indicator.  Browser uploads can pass a byte percentage;
 * server-side downloads often cannot expose a total size, so they use the
 * indeterminate form instead of a made-up percentage.
 */
export function DownloadProgress({
  label,
  detail,
  progress = null,
  handoff = false,
  className
}: Props) {
  const determinate = typeof progress === 'number'
  const safeProgress = determinate
    ? Math.max(0, Math.min(100, Math.round(progress)))
    : null

  return (
    <div
      className={cn(
        'grid gap-1.5 rounded-lg border border-brand-100 bg-brand-50/60 p-2.5 text-xs text-brand-800',
        className
      )}
      aria-live="polite"
    >
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium">{label}</span>
        {safeProgress !== null && <span>{safeProgress}%</span>}
        {handoff && <span>浏览器下载</span>}
      </div>
      <div
        className="h-1.5 overflow-hidden rounded-full bg-brand-200/80"
        role="progressbar"
        aria-label={label}
        aria-valuenow={safeProgress ?? undefined}
        aria-valuemin={determinate ? 0 : undefined}
        aria-valuemax={determinate ? 100 : undefined}
        aria-valuetext={
          safeProgress !== null
            ? `${safeProgress}%`
            : handoff
              ? '下载已交给浏览器'
              : '正在传输，等待服务器完成'
        }
      >
        <div
          className={cn(
            'h-full rounded-full bg-brand-600 transition-[width] duration-200',
            safeProgress === null && !handoff && 'w-2/5 animate-pulse',
            handoff && 'w-full opacity-70'
          )}
          style={safeProgress !== null ? { width: `${safeProgress}%` } : undefined}
        />
      </div>
      {detail && <small className="leading-4 text-brand-700/80">{detail}</small>}
    </div>
  )
}
