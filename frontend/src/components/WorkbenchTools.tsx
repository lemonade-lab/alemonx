import { useEffect, useId, useRef, type ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { useStoreState } from '../store/guideStore'
import { useViewportPopoverPosition } from '../hooks/useViewportPopoverPosition'

export function WorkbenchTools({
  children,
  keepMounted = false,
  label = '操作',
  menuLabel = '操作菜单',
  icon = <MoreHorizontal className="size-4" aria-hidden="true" />,
  menuClassName,
  align,
  restoreFocusOnClose = false
}: {
  children: ReactNode | ((close: () => void, open: boolean) => ReactNode)
  keepMounted?: boolean
  label?: string
  menuLabel?: string
  icon?: ReactNode
  menuClassName?: string
  align?: 'start' | 'end'
  restoreFocusOnClose?: boolean
}) {
  const [open, setOpen] = useStoreState(false)
  const id = useId()
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const style = useViewportPopoverPosition({
    anchor: trigger.current?.getBoundingClientRect() ?? null,
    anchorRef: trigger,
    open,
    align,
    keepBelowAnchor: true,
    popoverRef: menu
  })
  const close = () => {
    setOpen(false)
    if (restoreFocusOnClose) trigger.current?.focus()
    window.dispatchEvent(
      new CustomEvent('alx:top-tool-open', { detail: 'close' })
    )
  }
  useEffect(() => {
    if (!open) return
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) {
        setOpen(false)
        window.dispatchEvent(
          new CustomEvent('alx:top-tool-open', { detail: 'close' })
        )
      }
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [open, setOpen])
  return (
    <div
      ref={root}
      className="workbench-tools relative"
      onKeyDown={event => {
        if (!open || event.key !== 'Escape' || event.defaultPrevented) return
        const expanded = menu.current?.querySelector<HTMLButtonElement>(
          'button[aria-haspopup="dialog"][aria-expanded="true"]'
        )
        event.preventDefault()
        event.stopPropagation()
        if (expanded) {
          window.dispatchEvent(
            new CustomEvent('alx:top-tool-open', { detail: 'close' })
          )
          expanded.focus()
        } else {
          close()
          trigger.current?.focus()
        }
      }}
    >
      <button
        ref={trigger}
        type="button"
        className="icon-button workbench-menu-trigger size-8 p-0"
        aria-label={label}
        title={label}
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => (open ? close() : setOpen(true))}
      >
        {icon}
      </button>
      <div
        ref={menu}
        id={id}
        role="region"
        aria-label={menuLabel}
        hidden={!open}
        style={style}
        className={
          `workbench-tools-menu ${menuClassName ?? (typeof children === 'function' ? 'workbench-settings-menu' : '')}`
        }
      >
        {typeof children === 'function' ? (open || keepMounted) && children(close, open) : children}
      </div>
    </div>
  )
}

export function useWorkbenchSubmenu(enabled: boolean, open: boolean) {
  const root = useRef<HTMLDivElement>(null)
  const panel = useRef<HTMLElement>(null)
  const anchor = root.current?.getBoundingClientRect() ?? null
  const style = useViewportPopoverPosition({
    anchor,
    open: enabled && open,
    popoverRef: panel,
    placement:
      anchor && anchor.right + 400 > window.innerWidth ? 'left' : 'right'
  })
  return { root, panel, style: enabled ? style : undefined }
}
