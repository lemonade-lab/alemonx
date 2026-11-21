import cn from 'classnames'
import type { KeyboardEvent, ReactNode } from 'react'

export type TabItem<T extends string> = {
  id: T
  label: ReactNode
  icon?: ReactNode
  meta?: ReactNode
  disabled?: boolean
}

type TabsProps<T extends string> = {
  items: readonly TabItem<T>[]
  value: T
  onChange: (value: T) => void
  ariaLabel: string
  idPrefix?: string
  variant?: 'underline' | 'segmented' | 'inset' | 'pill'
  className?: string
}

/** Shared accessible tab switcher for workspace panels and dialogs. */
export function Tabs<T extends string>({
  items,
  value,
  onChange,
  ariaLabel,
  idPrefix,
  variant = 'underline',
  className
}: TabsProps<T>) {
  function navigateTabs(event: KeyboardEvent<HTMLDivElement>) {
    const { key } = event
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(key)) return

    const tabs = Array.from(
      event.currentTarget.querySelectorAll<HTMLButtonElement>(
        'button[role="tab"]:enabled'
      )
    )
    const current = tabs.findIndex(tab => tab === event.target)
    if (current < 0) return

    let next = 0
    if (key === 'End') next = tabs.length - 1
    if (key === 'ArrowLeft') next = (current - 1 + tabs.length) % tabs.length
    if (key === 'ArrowRight') next = (current + 1) % tabs.length
    event.preventDefault()
    tabs[next].focus()
    tabs[next].click()
  }

  const tabClassName = (active: boolean) =>
    cn(
      'inline-flex min-h-8 items-center justify-center gap-1.5 border-0 bg-transparent px-2.5 py-1.5 text-xs font-semibold leading-5 text-(--theme-ink-500) transition-colors hover:text-(--theme-ink-950) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) disabled:cursor-not-allowed disabled:opacity-50 [&_small]:text-[0.65rem] [&_small]:font-semibold [&_small]:opacity-70',
      variant === 'inset' &&
        'min-w-0 flex-1 rounded-[5px] aria-selected:bg-(--theme-accent-soft) aria-selected:text-(--theme-accent-text)',
      variant === 'underline' &&
        `-mb-px border-b-2 ${active ? 'border-(--theme-accent) text-(--theme-accent-text)' : 'border-transparent'}`,
      variant === 'segmented' &&
        `min-w-0 flex-1 rounded-[5px] ${active ? 'bg-(--theme-brand-100) text-(--theme-ink-950) shadow-[0_1px_2px_var(--theme-shadow-soft)]' : ''}`,
      variant === 'pill' &&
        `rounded-md hover:bg-(--theme-brand-50) ${active ? 'bg-(--theme-brand-50) text-(--theme-accent-text)' : ''}`
    )
  return (
    <div
      className={cn(
        'flex min-w-0 gap-1',
        variant === 'inset' && 'rounded-lg bg-(--theme-surface-raised) p-1',
        className
      )}
      role="tablist"
      aria-label={ariaLabel}
      onKeyDown={idPrefix ? navigateTabs : undefined}
    >
      {items.map(item => {
        const active = value === item.id
        return (
          <button
            type="button"
            className={tabClassName(active)}
            key={item.id}
            id={idPrefix && `${idPrefix}-tab-${item.id}`}
            role="tab"
            aria-selected={active}
            aria-controls={idPrefix && `${idPrefix}-panel-${item.id}`}
            tabIndex={idPrefix ? (active ? 0 : -1) : undefined}
            disabled={item.disabled}
            onClick={() => onChange(item.id)}
          >
            {item.icon}
            <span>{item.label}</span>
            {item.meta && <small>{item.meta}</small>}
          </button>
        )
      })}
    </div>
  )
}
