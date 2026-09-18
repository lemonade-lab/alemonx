import { useId } from 'react'
import type { ProjectNavigationProps } from './ProjectNavigation.d'

const itemClass =
  'flex min-h-9 w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition hover:bg-(--theme-surface-hover)'

export function ProjectNavigation({
  items,
  activeID,
  children
}: ProjectNavigationProps) {
  const id = useId()
  return (
    <nav aria-label="机器人功能" data-project-navigation className="grid gap-1">
      {items.map(item => {
        const active = item.id === activeID
        const subitems = active ? item.children : undefined
        const panelID = `${id}-${item.id}`
        return (
          <div key={item.id}>
            <button
              type="button"
              onClick={item.onSelect}
              className={`${itemClass} ${active ? 'bg-(--theme-accent-soft) font-semibold text-(--theme-accent-text)' : 'text-(--theme-text-secondary)'}`}
              aria-current={active && !subitems?.length ? 'page' : undefined}
              aria-expanded={item.children?.length ? active : undefined}
              aria-controls={subitems?.length ? panelID : undefined}
            >
              <span className="inline-flex size-4 shrink-0" aria-hidden="true">
                {item.icon}
              </span>
              <span>{item.label}</span>
              {active && item.loading && (
                <span className="ml-auto text-xs" role="status">
                  加载中…
                </span>
              )}
            </button>
            {subitems && subitems.length > 0 && (
              <div
                id={panelID}
                className="my-1 ml-4 grid gap-1 border-l border-(--theme-border-default) pl-2"
                aria-label={`${item.label}子菜单`}
              >
                {subitems.map(child => (
                  <button
                    key={child.id}
                    type="button"
                    onClick={child.onSelect}
                    className={`${itemClass} ${child.active ? 'bg-(--theme-accent-soft) font-medium text-(--theme-accent-text)' : 'text-(--theme-text-secondary)'}`}
                    aria-current={child.active ? 'page' : undefined}
                  >
                    {child.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        )
      })}
      {children}
    </nav>
  )
}
