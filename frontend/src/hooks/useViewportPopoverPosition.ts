import { useLayoutEffect, useState, type CSSProperties, type RefObject } from 'react'

export type PopoverPlacement = 'top' | 'bottom' | 'left' | 'right' | 'side'

export function clampPopoverAxis(value: number, size: number, start: number, end: number) {
  return Math.max(start, Math.min(end - size, value))
}

type Options = {
  anchor: DOMRect | null
  anchorRef?: RefObject<HTMLElement | null>
  open: boolean
  popoverRef: RefObject<HTMLElement | null>
  placement?: PopoverPlacement
  align?: 'start' | 'end'
  keepBelowAnchor?: boolean
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
  anchorRef,
  open,
  popoverRef,
  placement = 'bottom',
  align = 'start',
  keepBelowAnchor = false,
  gap = 8,
  gutter = 12
}: Options): CSSProperties | undefined {
  const [style, setStyle] = useState<CSSProperties>()
  // Callers may construct a DOMRect during render. Depend on its numeric
  // values rather than object identity, otherwise positioning itself causes an
  // update loop (especially for a long-press context menu).
  const leftAtRender = anchor?.left ?? 0
  const topAtRender = anchor?.top ?? 0
  const rightAtRender = anchor?.right ?? 0
  const bottomAtRender = anchor?.bottom ?? 0
  const heightAtRender = anchor?.height ?? 0

  useLayoutEffect(() => {
    if (!open || !popoverRef.current) return
    const update = () => {
      const popup = popoverRef.current
      if (!popup) return
      const viewport = window.visualViewport
      const leftEdge = (viewport?.offsetLeft ?? 0) + gutter
      const topEdge = (viewport?.offsetTop ?? 0) + gutter
      const rightEdge = (viewport?.offsetLeft ?? 0) + (viewport?.width ?? window.innerWidth) - gutter
      const bottomEdge = (viewport?.offsetTop ?? 0) + (viewport?.height ?? window.innerHeight) - gutter
      const rect = popup.getBoundingClientRect()
      const liveAnchor = anchorRef?.current?.getBoundingClientRect()
      const anchorLeft = liveAnchor?.left ?? leftAtRender
      const anchorTop = liveAnchor?.top ?? topAtRender
      const anchorRight = liveAnchor?.right ?? rightAtRender
      const anchorBottom = liveAnchor?.bottom ?? bottomAtRender
      const anchorHeight = liveAnchor?.height ?? heightAtRender
      const spaceBelow = bottomEdge - anchorBottom - gap
      const spaceAbove = anchorTop - topEdge - gap
      let direction = placement
      if (direction === 'side') {
        if (anchorLeft - leftEdge >= rect.width + gap) direction = 'left'
        else if (rightEdge - anchorRight >= rect.width + gap) direction = 'right'
        else direction = spaceBelow >= spaceAbove ? 'bottom' : 'top'
      }
      const height = placement === 'side'
        ? Math.max(rect.height, popup.scrollHeight + popup.offsetHeight - popup.clientHeight)
        : rect.height
      let left = align === 'end' ? anchorRight - rect.width : anchorLeft
      let top = anchorBottom + gap
      if (direction === 'top') top = anchorTop - Math.min(height, placement === 'side' ? Math.max(0, spaceAbove) : height) - gap
      if (direction === 'left') {
        left = anchorLeft - rect.width - gap
        top = anchorTop + (anchorHeight - height) / 2
      }
      if (direction === 'right') {
        left = anchorRight + gap
        top = anchorTop + (anchorHeight - height) / 2
      }
      // Vertical placements flip before clamping, retaining the anchor when
      // there is enough room on the opposite side.
      if (placement === 'bottom' && !keepBelowAnchor && top + rect.height > bottomEdge && anchorTop - rect.height - gap >= topEdge)
        top = anchorTop - rect.height - gap
      if (placement === 'top' && top < topEdge && anchorBottom + rect.height + gap <= bottomEdge)
        top = anchorBottom + gap
      left = Math.max(leftEdge, Math.min(rightEdge - rect.width, left))
      const below = keepBelowAnchor || (placement === 'side' && direction === 'bottom')
      top = Math.max(topEdge, Math.min(below ? bottomEdge : bottomEdge - height, top))
      const maxHeight = placement === 'side' && direction === 'top'
        ? Math.max(0, anchorTop - gap - top)
        : Math.max(0, bottomEdge - top)
      const next = {
        bottom: 'auto',
        left,
        maxHeight,
        maxWidth: Math.max(0, rightEdge - leftEdge),
        position: 'fixed',
        right: 'auto',
        top
      } satisfies CSSProperties
      setStyle(current =>
        current?.left === next.left &&
        current?.top === next.top &&
        current?.maxHeight === next.maxHeight &&
        current?.maxWidth === next.maxWidth
          ? current
          : next
      )
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
  }, [align, anchorRef, bottomAtRender, heightAtRender, leftAtRender, rightAtRender, topAtRender, gap, gutter, keepBelowAnchor, open, placement, popoverRef])

  return style
}
