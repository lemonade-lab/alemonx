export type WindowRect = {
  left: number
  top: number
  width: number
  height: number
}

export type ViewportClampOptions = {
  minWidth: number
  minHeight: number
  /** Horizontal and vertical space kept clear of the window when the viewport
   *  is the limiting factor. */
  gutter?: number
  /** Smallest size the window is allowed to reach when the viewport shrinks. */
  minViewportWidth?: number
  minViewportHeight?: number
  /** Minimum distance from the window to the viewport edge. */
  margin?: number
}

/**
 * Fit a window rect inside the current viewport without forgetting the
 * window's preferred (user-chosen) size. Clamping is a pure function of the
 * preferred rect: shrinking the viewport only shrinks the *displayed* window,
 * and growing the viewport back restores the preferred size automatically.
 */
export function clampWindowRectToViewport(
  rect: WindowRect,
  options: ViewportClampOptions,
  viewport?: { width: number; height: number }
): WindowRect {
  const {
    minWidth,
    minHeight,
    gutter = 48,
    minViewportWidth = 320,
    minViewportHeight = 280,
    margin = 16
  } = options
  const viewportWidth = viewport?.width ?? window.innerWidth
  const viewportHeight = viewport?.height ?? window.innerHeight
  // On a 320px phone, a 16px desktop gutter plus a 320px minimum width used
  // to place the right edge outside the viewport. Preserve gutters whenever
  // possible, but reduce them before allowing a displayed rect to overflow.
  const horizontalMargin = Math.max(
    0,
    Math.min(margin, (viewportWidth - Math.min(minViewportWidth, viewportWidth)) / 2)
  )
  const verticalMargin = Math.max(
    0,
    Math.min(margin, (viewportHeight - Math.min(minViewportHeight, viewportHeight)) / 2)
  )
  const minimumWidth = Math.min(
    minViewportWidth,
    viewportWidth - horizontalMargin * 2
  )
  const minimumHeight = Math.min(
    minViewportHeight,
    viewportHeight - verticalMargin * 2
  )
  const width = Math.min(
    Math.max(minimumWidth, viewportWidth - gutter),
    Math.max(minWidth, rect.width)
  )
  const height = Math.min(
    Math.max(minimumHeight, viewportHeight - gutter),
    Math.max(minHeight, rect.height)
  )
  return {
    width,
    height,
    left: Math.max(
      horizontalMargin,
      Math.min(viewportWidth - width - horizontalMargin, rect.left)
    ),
    top: Math.max(
      verticalMargin,
      Math.min(viewportHeight - height - verticalMargin, rect.top)
    )
  }
}
