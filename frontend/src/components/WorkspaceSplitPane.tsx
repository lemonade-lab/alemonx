import { useId, useRef, type PointerEvent, type ReactNode } from 'react'
import cn from 'classnames'

type Props = {
  className: string
  leadingClassName: string
  trailingClassName: string
  leading: ReactNode
  trailing: ReactNode
  sizedPane: 'leading' | 'trailing'
  width: number
  min: number
  max: number
  resizable: boolean
  separatorLabel: string
  onWidthChange: (width: number) => void
}

export function WorkspaceSplitPane({
  className,
  leadingClassName,
  trailingClassName,
  leading,
  trailing,
  sizedPane,
  width,
  min,
  max,
  resizable,
  separatorLabel,
  onWidthChange
}: Props) {
  const root = useRef<HTMLDivElement>(null)
  const sizedPaneID = useId()
  const drag = useRef<{
    pointer: number
    x: number
    width: number
    pixels: number
  } | null>(null)
  const fixedLeading = sizedPane === 'leading'
  const growDirection = fixedLeading ? 1 : -1
  const clamp = (value: number) =>
    Math.max(min, Math.min(max, Math.round(value)))
  const paneWidth = resizable ? clamp(width) : 0
  const size = { flexBasis: `${paneWidth}%` }

  function widthAt(event: PointerEvent<HTMLDivElement>) {
    const start = drag.current
    if (!start || start.pointer !== event.pointerId) return
    return clamp(
      start.width +
        (growDirection * (event.clientX - start.x) * 100) / start.pixels
    )
  }

  function clearDrag(event: PointerEvent<HTMLDivElement>) {
    if (drag.current?.pointer !== event.pointerId) return
    drag.current = null
    delete event.currentTarget.dataset.dragging
    event.currentTarget.style.removeProperty('--split-preview')
  }

  return (
    <div className={cn('workspace-split', className)} ref={root}>
      <div
        id={fixedLeading ? sizedPaneID : undefined}
        className={cn(
          'workspace-split-panel',
          leadingClassName,
          !fixedLeading && 'workspace-split-fill'
        )}
        style={fixedLeading ? size : undefined}
      >
        {leading}
      </div>
      {resizable && (
        <div
          className="workspace-split-divider"
          role="separator"
          aria-label={separatorLabel}
          aria-orientation="vertical"
          aria-controls={sizedPaneID}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={paneWidth}
          tabIndex={0}
          onPointerDown={event => {
            const pixels = root.current?.getBoundingClientRect().width
            if (event.button !== 0 || !event.isPrimary || !pixels) return
            const separator = event.currentTarget
            separator.focus({ preventScroll: true })
            separator.setPointerCapture(event.pointerId)
            drag.current = {
              pointer: event.pointerId,
              x: event.clientX,
              width: paneWidth,
              pixels
            }
            separator.dataset.dragging = ''
            event.preventDefault()
          }}
          onPointerMove={event => {
            const next = widthAt(event)
            if (next === undefined || !drag.current) return
            // Preview the divider only. The terminal and other panes resize once on release.
            const { width, pixels } = drag.current
            event.currentTarget.style.setProperty(
              '--split-preview',
              `${(growDirection * (next - width) * pixels) / 100}px`
            )
          }}
          onPointerUp={event => {
            const next = widthAt(event)
            clearDrag(event)
            if (next !== undefined && next !== paneWidth) onWidthChange(next)
          }}
          onPointerCancel={clearDrag}
          onLostPointerCapture={clearDrag}
          onKeyDown={event => {
            let next: number
            switch (event.key) {
              case 'Home':
                next = min
                break
              case 'End':
                next = max
                break
              case 'ArrowLeft':
                next = clamp(paneWidth - growDirection * 2)
                break
              case 'ArrowRight':
                next = clamp(paneWidth + growDirection * 2)
                break
              default:
                return
            }
            event.preventDefault()
            if (next !== paneWidth) onWidthChange(next)
          }}
        >
          <span className="workspace-split-handle" aria-hidden="true" />
        </div>
      )}
      <div
        id={fixedLeading ? undefined : sizedPaneID}
        className={cn(
          'workspace-split-panel',
          trailingClassName,
          fixedLeading && 'workspace-split-fill'
        )}
        style={fixedLeading ? undefined : size}
      >
        {trailing}
      </div>
    </div>
  )
}
