import {
  createContext,
  type ComponentType,
  type KeyboardEvent,
  type ReactNode,
  useContext,
  useState
} from 'react'
import { createPortal } from 'react-dom'
import { DesktopWindow, type DesktopWindowProps } from './DesktopWindow'

export type SidebarWindowItem<ID extends string> = {
  id: ID
  label: string
  icon: ComponentType<{ className?: string }>
}

type SidebarWindowProps<ID extends string> = Omit<
  DesktopWindowProps,
  'children'
> & {
  activeItem?: ID
  children: ReactNode
  items?: readonly SidebarWindowItem<ID>[]
  onActiveItemChange?: (item: ID) => void
  sidebarAriaLabel: string
  sidebarContent?: ReactNode
  sidebarFooter?: ReactNode
  bodyClassName?: string
}

const SidebarWindowActionsContext = createContext<HTMLElement | null>(null)
const SidebarWindowSectionNavContext = createContext<HTMLElement | null>(null)

/** Renders a page-level persistent action into its enclosing side rail. */
export function SidebarWindowActions({ children }: { children: ReactNode }) {
  const target = useContext(SidebarWindowActionsContext)
  return target ? createPortal(children, target) : null
}

/** Renders page-specific sections immediately below the side-rail heading. */
export function SidebarWindowSectionNav({ children }: { children: ReactNode }) {
  const target = useContext(SidebarWindowSectionNavContext)
  return target ? createPortal(children, target) : null
}

/**
 * A desktop window with a compact navigation rail. It owns the layout and
 * keyboard navigation, while callers provide the items and panel content.
 */
export function SidebarWindow<ID extends string>({
  activeItem,
  bodyClassName = '',
  children,
  items,
  onActiveItemChange,
  sidebarAriaLabel,
  sidebarContent,
  sidebarFooter,
  ...windowProps
}: SidebarWindowProps<ID>) {
  const [actionsTarget, setActionsTarget] = useState<HTMLElement | null>(null)
  const [sectionNavTarget, setSectionNavTarget] =
    useState<HTMLElement | null>(null)
  const hasNavigation = Boolean(
    activeItem && items?.length && onActiveItemChange
  )
  const panelID = hasNavigation
    ? `${windowProps.id}-panel-${activeItem}`
    : `${windowProps.id}-panel`
  const activate = (next: ID) => {
    onActiveItemChange?.(next)
    window.requestAnimationFrame(() =>
      document.getElementById(`${windowProps.id}-tab-${next}`)?.focus()
    )
  }
  const handleKeyDown = (item: ID, event: KeyboardEvent<HTMLButtonElement>) => {
    if (!items?.length) return
    const currentIndex = items.findIndex(candidate => candidate.id === item)
    const move = (offset: number) => {
      const nextIndex = (currentIndex + offset + items.length) % items.length
      activate(items[nextIndex].id)
    }
    if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
      event.preventDefault()
      move(1)
    }
    if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
      event.preventDefault()
      move(-1)
    }
    if (event.key === 'Home') {
      event.preventDefault()
      activate(items[0].id)
    }
    if (event.key === 'End') {
      event.preventDefault()
      activate(items[items.length - 1].id)
    }
  }

  return (
    <DesktopWindow {...windowProps}>
      <SidebarWindowActionsContext.Provider value={actionsTarget}>
        <SidebarWindowSectionNavContext.Provider value={sectionNavTarget}>
          <div
          className="grid h-full min-h-0 grid-cols-[176px_minmax(0,1fr)] bg-(--theme-surface-panel)"
          data-sidebar-window-shell
        >
          <aside
            className="flex flex-col gap-2 border-r border-(--theme-border-default) bg-(--theme-surface-raised) px-3 py-4.5"
            data-sidebar-window-sidebar
            aria-label={sidebarAriaLabel}
          >
            {sidebarContent}
            <div data-sidebar-window-section-nav ref={setSectionNavTarget} />
            {hasNavigation && (
              <div
                className="grid gap-1"
                data-sidebar-window-nav
                role="tablist"
                aria-label={sidebarAriaLabel}
              >
                {items?.map(item => {
                  const Icon = item.icon
                  const selected = item.id === activeItem
                  return (
                    <button
                      className={`flex min-h-9 items-center gap-2 rounded-lg border px-2.5 text-left text-xs font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) ${selected ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)' : 'border-transparent bg-transparent text-(--theme-text-secondary) hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong)'}`}
                      key={item.id}
                      id={`${windowProps.id}-tab-${item.id}`}
                      role="tab"
                      type="button"
                      aria-selected={selected}
                      aria-controls={`${windowProps.id}-panel-${item.id}`}
                      tabIndex={selected ? 0 : -1}
                      onClick={() => activate(item.id)}
                      onKeyDown={event => handleKeyDown(item.id, event)}
                    >
                      <Icon className="size-4" />
                      <span>{item.label}</span>
                    </button>
                  )
                })}
              </div>
            )}
            <div
              className="mt-auto grid gap-1.5"
              data-sidebar-window-side-actions
            >
              <div data-sidebar-window-actions ref={setActionsTarget} />
              {sidebarFooter && (
                <div data-sidebar-window-footer>{sidebarFooter}</div>
              )}
            </div>
          </aside>
          <section
            className="min-h-0 bg-(--theme-surface-panel)"
            id={panelID}
            role="tabpanel"
            aria-labelledby={
              hasNavigation ? `${windowProps.id}-tab-${activeItem}` : undefined
            }
            aria-label={hasNavigation ? undefined : windowProps.title}
          >
            <div
              className={`sidebar-window-body h-full min-h-0 overflow-auto ${bodyClassName}`}
              data-sidebar-window-body
            >
              {children}
            </div>
          </section>
          </div>
        </SidebarWindowSectionNavContext.Provider>
      </SidebarWindowActionsContext.Provider>
    </DesktopWindow>
  )
}
