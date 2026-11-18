import { ChevronRight } from 'lucide-react'
import { Fragment, useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useViewportPopoverPosition } from '../hooks/useViewportPopoverPosition'
import { useIsMobileViewport } from '../hooks/useIsMobileViewport'
import type { ProjectNavigationProps } from './ProjectNavigation.d'

const itemClass =
  'flex min-h-9 w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition hover:bg-(--theme-surface-hover)'
const selectedClass =
  'bg-(--theme-accent-soft) font-semibold text-(--theme-accent-text)'
const normalClass = 'text-(--theme-text-secondary)'

export function ProjectNavigation({
  items,
  activeID,
  children
}: ProjectNavigationProps) {
  const id = useId()
  const isCompact = useIsMobileViewport()
  const [expandedID, setExpandedID] = useState<string | null>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const expanded = items.find(
    item => item.id === expandedID && item.children?.length
  )
  const anchor = expanded
    ? (trigger.current?.getBoundingClientRect() ?? null)
    : null
  const position = useViewportPopoverPosition({
    anchor,
    open: Boolean(expanded) && !isCompact,
    popoverRef: panel,
    placement: anchor && anchor.left >= 230 ? 'left' : 'bottom',
    gap: 6
  })

  useEffect(() => {
    if (!expandedID) return
    const closeOutside = (event: PointerEvent) => {
      if (!(event.target instanceof Node)) return
      if (
        !trigger.current?.contains(event.target) &&
        !panel.current?.contains(event.target)
      )
        setExpandedID(null)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
    }
  }, [expandedID])

  useEffect(() => {
    if (expandedID) panel.current?.querySelector('button')?.focus()
  }, [expandedID])

  const submenu = expanded && (
    <div
      ref={panel}
      id={id}
      role="group"
      aria-label={`${expanded.label}子菜单`}
      className={
        isCompact
          ? 'grid gap-1 border-l-2 border-(--theme-accent-soft-border) pl-3'
          : 'fixed z-[10003] grid w-[min(210px,calc(100vw-24px))] gap-0.5 overflow-auto rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-1 shadow-(--theme-shadow-pop)'
      }
      style={isCompact ? undefined : position}
    >
      {expanded.children?.map(child => (
        <button
          key={child.id}
          type="button"
          onClick={() => {
            child.onSelect()
            setExpandedID(null)
            trigger.current?.focus()
          }}
          className={`${itemClass} ${child.active ? selectedClass : normalClass}`}
          aria-current={child.active ? 'page' : undefined}
        >
          {child.label}
        </button>
      ))}
    </div>
  )

  return (
    <nav
      aria-label="机器人功能"
      data-project-navigation
      className="grid gap-1"
      onKeyDown={event => {
        if (event.key !== 'Escape' || !expandedID || event.defaultPrevented)
          return
        event.preventDefault()
        event.stopPropagation()
        trigger.current?.focus()
        setExpandedID(null)
      }}
    >
      {items.map(item => {
        const active = item.id === activeID
        const hasSubitems = Boolean(item.children?.length)
        const open = expanded?.id === item.id
        return (
          <Fragment key={item.id}>
            <button
              type="button"
              onClick={event => {
                if (!hasSubitems) {
                  setExpandedID(null)
                  item.onSelect()
                  return
                }
                trigger.current = event.currentTarget
                setExpandedID(open ? null : item.id)
              }}
              className={`${itemClass} ${active || open ? selectedClass : normalClass}`}
              aria-current={active ? 'page' : undefined}
              aria-expanded={hasSubitems ? open : undefined}
              aria-controls={open ? id : undefined}
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
              {hasSubitems && (
                <ChevronRight
                  className="ml-auto size-4 shrink-0 opacity-60"
                  aria-hidden="true"
                />
              )}
            </button>
            {isCompact && open && submenu}
          </Fragment>
        )
      })}
      {children}
      {expanded && !isCompact && createPortal(submenu, document.body)}
    </nav>
  )
}
