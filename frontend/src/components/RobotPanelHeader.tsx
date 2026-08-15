import type { ReactNode } from 'react'

export type RobotPanelHeaderProps = {
  /** Optional context shown above the page title, such as a back action. */
  eyebrow?: ReactNode
  /** The page's stable, user-facing name. */
  title: ReactNode
  /** A concise explanation of the page or its current state. */
  description?: ReactNode
  /** A leading feature icon. */
  icon?: ReactNode
  /** Page-level actions, always placed at the right edge of the header. */
  actions?: ReactNode
  className?: string
}

/**
 * Shared header for every robot workbench page.
 *
 * It deliberately owns the icon, title, description and action regions so
 * feature pages cannot drift into different header structures over time.
 */
export function RobotPanelHeader({
  eyebrow,
  title,
  description,
  icon,
  actions,
  className = ''
}: RobotPanelHeaderProps) {
  return (
    <header
      data-robot-panel-header
      className={[
        'sticky top-2.5 z-5 mx-2.5 mt-2.5 flex min-h-13 flex-wrap items-center justify-between gap-2 rounded-[10px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-4 shadow-[0_4px_14px_var(--theme-shadow-soft)]',
        className
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="flex min-w-0 flex-1 items-center gap-3">
        {icon && (
          <span className="inline-flex size-7.5 shrink-0 items-center justify-center rounded-[9px] border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)">
            {icon}
          </span>
        )}
        <div className="grid min-w-0 gap-0.5">
          {eyebrow && (
            <div className="text-[0.7rem] leading-tight text-(--theme-text-muted)">
              {eyebrow}
            </div>
          )}
          <div className="min-w-0 text-[0.84rem] leading-tight font-semibold text-(--theme-text-strong)">
            {title}
          </div>
          {description && (
            <small className="max-w-90 truncate text-[0.7rem] leading-tight text-(--theme-text-muted)">
              {description}
            </small>
          )}
        </div>
      </div>
      {actions && (
        <div className="robot-panel-actions ml-auto flex shrink-0 flex-wrap items-center justify-end gap-2 [&_.icon-button]:size-9 [&_.icon-button]:shrink-0 [&_.primary-button]:min-h-9 [&_.secondary-button]:min-h-9 [&_.text-button]:min-h-9 [&_[role=tab]]:min-h-9">
          {actions}
        </div>
      )}
    </header>
  )
}
