import { useLayoutEffect, useState, type CSSProperties, type RefObject } from 'react'

export type PopoverPlacement = 'top' | 'bottom' | 'left' | 'right'

export function clampPopoverAxis(value: number, size: number, start: number, end: number) {
  return Math.max(start, Math.min(end - size, value))
}

type Options = {
  anchor: DOMRect | null
  open: boolean
  popoverRef: RefObject<HTMLElement | null>
  placement?: PopoverPlacement
  gap?: number
  gutter?: number
}

/**
 * Places a portalled popover inside the visible viewport. It deliberately uses
 * the visual viewport so browser chrome and the on-screen keyboard cannot put
 * a menu beyond a phone's reachable area.
 */
export function useViewportPopoverPosition({
  anchor,
  open,
  popoverRef,
  placement = 'bottom',
  gap = 8,
  gutter = 12
}: Options): CSSProperties | undefined {
  const [style, setStyle] = useState<CSSProperties>()

  useLayoutEffect(() => {
    if (!open || !anchor || !popoverRef.current) return
    const update = () => {
      const popup = popoverRef.current
      if (!popup) return
      const viewport = window.visualViewport
      const leftEdge = (viewport?.offsetLeft ?? 0) + gutter
      const topEdge = (viewport?.offsetTop ?? 0) + gutter
      const rightEdge = (viewport?.offsetLeft ?? 0) + (viewport?.width ?? window.innerWidth) - gutter
      const bottomEdge = (viewport?.offsetTop ?? 0) + (viewport?.height ?? window.innerHeight) - gutter
      const rect = popup.getBoundingClientRect()
      let left = anchor.left
      let top = anchor.bottom + gap
      if (placement === 'top') top = anchor.top - rect.height - gap
      if (placement === 'left') {
        left = anchor.left - rect.width - gap
        top = anchor.top + (anchor.height - rect.height) / 2
      }
      if (placement === 'right') {
        left = anchor.right + gap
        top = anchor.top + (anchor.height - rect.height) / 2
      }
      // Vertical placements flip before clamping, retaining the anchor when
      // there is enough room on the opposite side.
      if (placement === 'bottom' && top + rect.height > bottomEdge && anchor.top - rect.height - gap >= topEdge)
        top = anchor.top - rect.height - gap
      if (placement === 'top' && top < topEdge && anchor.bottom + rect.height + gap <= bottomEdge)
        top = anchor.bottom + gap
      left = Math.max(leftEdge, Math.min(rightEdge - rect.width, left))
      top = Math.max(topEdge, Math.min(bottomEdge - rect.height, top))
      setStyle({
        left,
        maxHeight: Math.max(0, bottomEdge - top),
        maxWidth: Math.max(0, rightEdge - leftEdge),
        position: 'fixed',
        top
      })
    }
    update()
    const viewport = window.visualViewport
    window.addEventListener('resize', update)
    window.addEventListener('scroll', update, true)
    viewport?.addEventListener('resize', update)
    viewport?.addEventListener('scroll', update)
    return () => {
      window.removeEventListener('resize', update)
      window.removeEventListener('scroll', update, true)
      viewport?.removeEventListener('resize', update)
      viewport?.removeEventListener('scroll', update)
    }
  }, [anchor, gap, gutter, open, placement, popoverRef])

  return style
}
