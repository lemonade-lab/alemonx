import cn from 'classnames'
import type { ReactNode } from 'react'

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
  variant?: 'underline' | 'segmented' | 'pill'
  className?: string
}

/** Shared accessible tab switcher for workspace panels and dialogs. */
export function Tabs<T extends string>({
  items,
  value,
  onChange,
  ariaLabel,
  variant = 'underline',
  className
}: TabsProps<T>) {
  const tabClassName = (active: boolean) =>
    cn(
      'inline-flex min-h-8 items-center justify-center gap-1.5 border-0 bg-transparent px-2.5 py-1.5 text-xs font-semibold leading-5 text-(--theme-ink-500) transition-colors hover:text-(--theme-ink-950) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) disabled:cursor-not-allowed disabled:opacity-50 [&_small]:text-[0.65rem] [&_small]:font-semibold [&_small]:opacity-70',
      variant === 'underline' &&
        `-mb-px border-b-2 ${active ? 'border-(--theme-accent) text-(--theme-accent-text)' : 'border-transparent'}`,
      variant === 'segmented' &&
        `min-w-0 flex-1 rounded-[5px] ${active ? 'bg-(--theme-brand-100) text-(--theme-ink-950) shadow-[0_1px_2px_var(--theme-shadow-soft)]' : ''}`,
      variant === 'pill' &&
        `rounded-md hover:bg-(--theme-brand-50) ${active ? 'bg-(--theme-brand-50) text-(--theme-accent-text)' : ''}`
    )
  return (
    <div
      className={cn('flex min-w-0 gap-1', className)}
      role="tablist"
      aria-label={ariaLabel}
    >
      {items.map(item => {
        const active = value === item.id
        return (
          <button
            className={tabClassName(active)}
            key={item.id}
            role="tab"
            aria-selected={active}
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
