import type { ReactNode } from 'react'

type Props = {
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
}: Props) {
  return (
    <header
      className={['bot-page-header', className].filter(Boolean).join(' ')}
    >
      <div className="robot-panel-header-leading">
        {icon && <span className="bot-page-header-icon">{icon}</span>}
        <div className="bot-page-header-meta">
          {eyebrow && (
            <div className="robot-panel-header-eyebrow">{eyebrow}</div>
          )}
          <div className="robot-panel-header-title">{title}</div>
          {description && <small>{description}</small>}
        </div>
      </div>
      {actions && <div className="robot-panel-header-actions">{actions}</div>}
    </header>
  )
}
