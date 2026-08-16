import type { ReactNode } from 'react'
import cn from 'classnames'

// 设置窗口统一的页面骨架：标题区 + 卡片列表，全部走主题 token。
export function SettingsPage({
  title,
  description,
  children,
  className
}: {
  title?: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('settings-page', className)}>
      {(title || description) && (
        <header className="settings-page-head">
          {title && <h2 className="settings-page-title">{title}</h2>}
          {description && <p className="settings-page-desc">{description}</p>}
        </header>
      )}
      {children}
    </div>
  )
}

// 设置卡片：图标头 + 标题/说明 + 右侧操作 + 内容区。
export function SettingsCard({
  icon,
  title,
  description,
  actions,
  children,
  className
}: {
  icon?: ReactNode
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  children?: ReactNode
  className?: string
}) {
  const hasHead = Boolean(icon || title || description || actions)
  return (
    <section className={cn('settings-card', className)}>
      {hasHead && (
        <header className="settings-card-head">
          {icon && <i className="settings-card-icon">{icon}</i>}
          {(title || description) && (
            <div className="settings-card-copy">
              {title && (
                <strong className="settings-card-title">{title}</strong>
              )}
              {description && (
                <p className="settings-card-desc">{description}</p>
              )}
            </div>
          )}
          {actions && <div className="settings-card-actions">{actions}</div>}
        </header>
      )}
      {children && <div className="settings-card-body">{children}</div>}
    </section>
  )
}

// 统一的成功/错误/提示反馈。
export function SettingsMessage({
  tone = 'info',
  children
}: {
  tone?: 'info' | 'success' | 'error'
  children: ReactNode
}) {
  return <p className={cn('settings-message', `is-${tone}`)}>{children}</p>
}

// 主题化开关。
export function SettingsSwitch({
  checked,
  onChange,
  disabled,
  label,
  hint
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  label: ReactNode
  hint?: ReactNode
}) {
  return (
    <label className="settings-switch-row">
      <span className="settings-switch">
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={event => onChange(event.target.checked)}
        />
      </span>
      <span className="settings-switch-copy">
        <strong>{label}</strong>
        {hint && <small>{hint}</small>}
      </span>
    </label>
  )
}
