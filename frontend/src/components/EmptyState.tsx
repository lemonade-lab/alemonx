import cn from 'classnames'
import { Loader2 } from 'lucide-react'
import type { ReactNode } from 'react'

type EmptyStateProps = {
  icon: ReactNode
  title: ReactNode
  description?: ReactNode
  className?: string
  loading?: boolean
}

/**
 * A compact, centered empty state for feature panels. The content is kept
 * configurable while the visual hierarchy stays consistent across pages.
 */
export function EmptyState({
  icon,
  title,
  description,
  className,
  loading = false
}: EmptyStateProps) {
  return (
    <section
      className={cn(
        'system-feature-empty flex flex-col items-center justify-center text-center',
        className
      )}
      aria-busy={loading || undefined}
    >
      <div className="mb-3 rounded-full bg-slate-100 p-3 dark:bg-slate-800">
        {loading ? (
          <Loader2 className="size-6 animate-spin text-slate-400 dark:text-slate-500" />
        ) : (
          icon
        )}
      </div>
      <strong className="text-sm font-medium text-slate-600 dark:text-slate-300">
        {title}
      </strong>
      {description && (
        <span className="mt-1 text-xs text-slate-400">{description}</span>
      )}
    </section>
  )
}
