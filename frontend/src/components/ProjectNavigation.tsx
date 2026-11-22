import { Check, ChevronRight, LoaderCircle } from 'lucide-react'
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
  dropdown = false,
  children
}: ProjectNavigationProps) {
  const id = useId()
  const isMobile = useIsMobileViewport()
  const inlineSubmenu = isMobile && !dropdown
  const rowClass = dropdown ? 'workbench-action-item' : itemClass
  const loadingItem = items.find(item => item.id === activeID && item.loading)
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
    anchorRef: dropdown ? trigger : undefined,
    open: Boolean(expanded) && !inlineSubmenu,
    popoverRef: panel,
    placement: dropdown ? 'side' : anchor && anchor.left >= 230 ? 'left' : 'bottom',
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
    if (expandedID) panel.current?.querySelector('button')?.focus({ preventScroll: true })
  }, [expandedID])

  const submenu = expanded && (
    <div
      ref={panel}
      id={id}
      role="group"
      aria-label={`${expanded.label}子菜单`}
      className={
        dropdown
          ? 'workbench-tools-menu workbench-side-submenu fixed'
          : inlineSubmenu
            ? 'grid gap-1 border-l-2 border-(--theme-accent-soft-border) pl-3'
            : 'workbench-tools-menu workbench-settings-menu fixed overflow-auto [--workbench-menu-layer:10003]'
      }
      style={inlineSubmenu ? undefined : position}
    >
      {expanded.children?.map(child => (
        <button
          key={child.id}
          type="button"
          onClick={() => {
            trigger.current?.focus()
            setExpandedID(null)
            child.onSelect()
          }}
          className={inlineSubmenu ? `${itemClass} ${child.active ? selectedClass : normalClass}` : 'workbench-action-item'}
          aria-current={child.active ? 'page' : undefined}
        >
          {child.label}
          {!inlineSubmenu && child.active && <Check className="ml-auto size-4 shrink-0" aria-hidden="true" />}
        </button>
      ))}
    </div>
  )

  return (
    <nav
      aria-label="机器人功能"
      data-project-navigation
      className={dropdown ? 'grid gap-0.5' : 'grid gap-1'}
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
        const loading = item === loadingItem
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
              className={dropdown ? rowClass : `${rowClass} ${active || open ? selectedClass : normalClass}`}
              aria-current={active ? 'page' : undefined}
              aria-busy={loading || undefined}
              aria-expanded={hasSubitems ? open : undefined}
              aria-controls={open ? id : undefined}
            >
              <span className="grid size-4 shrink-0 place-items-center [&>svg]:size-4" aria-hidden="true">
                {item.icon}
              </span>
              <span>{item.label}</span>
              {loading && (
                <LoaderCircle
                  className="ml-auto size-4 shrink-0 motion-safe:animate-spin"
                  aria-hidden="true"
                />
              )}
              {hasSubitems && (
                <ChevronRight
                  className={`size-4 shrink-0 opacity-60 ${loading ? '' : 'ml-auto'}`}
                  aria-hidden="true"
                />
              )}
              {dropdown && active && !hasSubitems && !loading && <Check className="ml-auto size-4 shrink-0" aria-hidden="true" />}
            </button>
            {(dropdown || inlineSubmenu) && open && submenu}
          </Fragment>
        )
      })}
      <span className="sr-only" role="status">
        {loadingItem && `${loadingItem.label}加载中…`}
      </span>
      {children}
      {expanded && !dropdown && !inlineSubmenu && createPortal(submenu, document.body)}
    </nav>
  )
}
