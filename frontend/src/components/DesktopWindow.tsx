import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type ReactNode
} from 'react'
import { Minus, X } from 'lucide-react'
import { Modal } from './Modal'
import { registerDesktopWindowShortcut } from './desktopWindowShortcuts'
import { isWindowHeaderInteractiveTarget } from './desktopWindowInteraction'
import { useIsPadViewport, useIsPhoneViewport } from '../hooks/useIsPadViewport'
import { clampWindowRectToViewport } from '../lib/windowRect'

export type ResizeCorner = 'nw' | 'ne' | 'sw' | 'se'

export function WindowResizeHandles({
  label,
  onStart,
  onMove,
  onEnd,
  onCancel
}: {
  label: string
  onStart: (corner: ResizeCorner, event: ReactPointerEvent<HTMLButtonElement>) => void
  onMove: (event: ReactPointerEvent<HTMLButtonElement>) => void
  onEnd: (event: ReactPointerEvent<HTMLButtonElement>) => void
  onCancel: () => void
}) {
  const corners: ResizeCorner[] = ['nw', 'ne', 'sw', 'se']
  return (
    <>
      {corners.map(corner => (
        <button
          className={`desktop-window-resize desktop-window-resize-${corner}`}
          onPointerDown={event => onStart(corner, event)}
          onPointerMove={onMove}
          onPointerUp={onEnd}
          onPointerCancel={onCancel}
          aria-label={`从${corner === 'nw' ? '左上' : corner === 'ne' ? '右上' : corner === 'sw' ? '左下' : '右下'}角调整${label}窗口大小`}
          title="调整窗口大小"
          key={corner}
        />
      ))}
    </>
  )
}

export type DesktopWindowProps = {
  id: string
  open: boolean
  minimized: boolean
  title: string
  subtitle?: string
  icon: ReactNode
  headerLeft?: ReactNode
  actions?: ReactNode
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
  initialPosition?: { left: number; top: number }
  width?: number
  height?: number
  storageKey?: string
  children: ReactNode
}

export function DesktopWindow({
  id,
  open,
  minimized,
  title,
  subtitle,
  icon,
  headerLeft,
  actions,
  onClose,
  onMinimize,
  zIndex,
  onActivate,
  initialPosition,
  width = 860,
  height = 620,
  storageKey,
  children
}: DesktopWindowProps) {
  const [windowRect, setWindowRect] = useState(() => ({
    left: initialPosition?.left ?? 64,
    top: initialPosition?.top ?? 56,
    width,
    height
  }))
  // The window's preferred (user-chosen) size. `windowRect` holds the
  // viewport-clamped display rect; keeping the ideal rect separate means
  // shrinking the browser only temporarily shrinks the window, and growing it
  // back restores the remembered size instead of staying small forever.
  const preferredRect = useRef<typeof windowRect>(windowRect)
  const [maximized, setMaximized] = useState(false)
  const [layoutReady, setLayoutReady] = useState(!storageKey)
  const windowRef = useRef<HTMLElement>(null)
  const isPadView = useIsPadViewport()
  const isPhoneView = useIsPhoneViewport()
  const dragStart = useRef<{
    pointerId: number
    x: number
    y: number
    left: number
    top: number
  } | null>(null)
  const resizeStart = useRef<{
    corner: ResizeCorner
    x: number
    y: number
    width: number
    height: number
    left: number
    top: number
  } | null>(null)

  useLayoutEffect(() => {
    if (!storageKey) return
    setLayoutReady(false)
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || 'null') as { rect?: typeof windowRect; maximized?: boolean } | null
      if (saved?.rect && Number.isFinite(saved.rect.left) && Number.isFinite(saved.rect.top) && Number.isFinite(saved.rect.width) && Number.isFinite(saved.rect.height)) {
        preferredRect.current = saved.rect
        setWindowRect(
          clampWindowRectToViewport(saved.rect, {
            minWidth: 440,
            minHeight: 320
          })
        )
      }
      if (saved?.maximized) setMaximized(true)
    } catch { /* An invalid local layout should never prevent opening chat. */ }
    setLayoutReady(true)
  }, [storageKey])

  useEffect(() => {
    if (!storageKey || !open || !layoutReady) return
    localStorage.setItem(
      storageKey,
      JSON.stringify({ rect: preferredRect.current, maximized })
    )
  }, [layoutReady, maximized, open, storageKey, windowRect])

  useLayoutEffect(() => {
    if (!open) return
    const applyViewport = () => {
      if (isPhoneView) {
        setWindowRect({
          left: 0,
          top: 0,
          width: window.innerWidth,
          height: window.innerHeight
        })
        return
      }
      if (maximized) {
        setWindowRect({
          left: 16,
          top: 16,
          width: Math.max(440, window.innerWidth - 32),
          height: Math.max(320, window.innerHeight - 32)
        })
        return
      }
      setWindowRect(
        clampWindowRectToViewport(preferredRect.current, {
          minWidth: 440,
          minHeight: 320
        })
      )
    }
    applyViewport()
    window.addEventListener('resize', applyViewport)
    return () => window.removeEventListener('resize', applyViewport)
  }, [isPhoneView, maximized, open])

  const previewMove = useCallback((event: Pick<PointerEvent, 'clientX' | 'clientY' | 'pointerId'>) => {
    const start = dragStart.current
    if (!start || start.pointerId !== event.pointerId) return
    const left = Math.max(16, Math.min(window.innerWidth - windowRect.width - 16, start.left + event.clientX - start.x))
    const top = Math.max(16, Math.min(window.innerHeight - windowRect.height - 16, start.top + event.clientY - start.y))
    windowRef.current?.style.setProperty('left', `${left}px`)
    windowRef.current?.style.setProperty('top', `${top}px`)
  }, [windowRect.height, windowRect.width])
  const commitMove = useCallback((event: Pick<PointerEvent, 'clientX' | 'clientY' | 'pointerId'>) => {
    const start = dragStart.current
    if (!start || start.pointerId !== event.pointerId) return
    const left = Math.max(16, Math.min(window.innerWidth - windowRect.width - 16, start.left + event.clientX - start.x))
    const top = Math.max(16, Math.min(window.innerHeight - windowRect.height - 16, start.top + event.clientY - start.y))
    // A drag only moves the window; keep the preferred size intact so it can
    // be restored when the viewport grows. A click without movement must not
    // re-anchor the preferred position either.
    if (left !== start.left || top !== start.top) {
      preferredRect.current = { ...preferredRect.current, left, top }
    }
    setWindowRect(current => ({ ...current, left, top }))
    dragStart.current = null
  }, [windowRect.height, windowRect.width])
  const cancelMove = useCallback(() => {
    const start = dragStart.current
    if (start) {
      windowRef.current?.style.setProperty('left', `${start.left}px`)
      windowRef.current?.style.setProperty('top', `${start.top}px`)
    }
    dragStart.current = null
  }, [])
  useEffect(() => {
    const onPointerMove = (event: PointerEvent) => {
      if (!dragStart.current || dragStart.current.pointerId !== event.pointerId) return
      event.preventDefault()
      previewMove(event)
    }
    const onPointerUp = (event: PointerEvent) => commitMove(event)
    const onPointerCancel = () => cancelMove()
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerCancel)
    return () => {
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', onPointerCancel)
    }
  }, [cancelMove, commitMove, previewMove])
  const getResizedRect = (event: ReactPointerEvent<HTMLElement>) => {
    const start = resizeStart.current
    if (!start) return null
    const horizontal = start.corner.endsWith('e') ? 1 : -1
    const vertical = start.corner.startsWith('s') ? 1 : -1
    const nextWidth = Math.max(
      440,
      Math.min(
        start.corner.endsWith('e') ? window.innerWidth - start.left - 16 : start.width + start.left - 16,
        start.width + horizontal * (event.clientX - start.x)
      )
    )
    const nextHeight = Math.max(
      320,
      Math.min(
        start.corner.startsWith('s') ? window.innerHeight - start.top - 16 : start.height + start.top - 16,
        start.height + vertical * (event.clientY - start.y)
      )
    )
    return {
      width: nextWidth,
      height: nextHeight,
      left: start.corner.endsWith('w') ? start.left + start.width - nextWidth : start.left,
      top: start.corner.startsWith('n') ? start.top + start.height - nextHeight : start.top
    }
  }
  const previewResize = (event: ReactPointerEvent<HTMLElement>) => {
    const rect = getResizedRect(event)
    if (!rect) return
    windowRef.current?.style.setProperty('width', `${rect.width}px`)
    windowRef.current?.style.setProperty('height', `${rect.height}px`)
    windowRef.current?.style.setProperty('left', `${rect.left}px`)
    windowRef.current?.style.setProperty('top', `${rect.top}px`)
  }
  const commitResize = (event: ReactPointerEvent<HTMLElement>) => {
    const rect = getResizedRect(event)
    const start = resizeStart.current
    if (!rect || !start) return
    // Only an actual resize is a user choice worth remembering; touching a
    // handle on a viewport-constrained window must not forget the ideal size.
    if (
      rect.width !== start.width ||
      rect.height !== start.height ||
      rect.left !== start.left ||
      rect.top !== start.top
    ) {
      preferredRect.current = rect
    }
    setWindowRect(current => ({ ...current, ...rect }))
    resizeStart.current = null
  }
  const cancelResize = () => {
    windowRef.current?.style.removeProperty('width')
    windowRef.current?.style.removeProperty('height')
    windowRef.current?.style.removeProperty('left')
    windowRef.current?.style.removeProperty('top')
    resizeStart.current = null
  }
  const toggleMaximize = useCallback(() => {
    if (isPadView) return
    if (maximized) {
      setWindowRect(
        clampWindowRectToViewport(preferredRect.current, {
          minWidth: 440,
          minHeight: 320
        })
      )
      setMaximized(false)
      return
    }
    setWindowRect({
      left: 16,
      top: 16,
      width: Math.max(440, window.innerWidth - 32),
      height: Math.max(320, window.innerHeight - 32)
    })
    setMaximized(true)
  }, [isPadView, maximized])

  useEffect(() => {
    if (!open) return
    return registerDesktopWindowShortcut({
      id,
      zIndex,
      minimized,
      onClose,
      // Phone windows are full-screen task pages. Keyboard shortcuts must not
      // put them into a hidden/minimized state with no desktop dock to restore.
      onMinimize: isPhoneView ? undefined : onMinimize,
      onToggleMaximize: isPhoneView ? undefined : toggleMaximize
    })
  }, [id, isPhoneView, minimized, onClose, onMinimize, open, toggleMaximize, zIndex])

  // A persisted window must not paint at its fallback coordinates first. Read
  // its saved geometry before the browser paints, otherwise users see a brief
  // jump from the default top-left position to the restored position.
  if (!open || !layoutReady) return null
  return (
    <Modal
      open
      zIndex={zIndex}
      className="floating-window-backdrop"
      trapFocus={isPhoneView}
      lockScroll={isPhoneView}
    >
      <section
        ref={windowRef}
        className="floating-window grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900"
        style={{ ...windowRect, display: minimized ? 'none' : undefined }}
        data-window-container="desktop"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onPointerDownCapture={onActivate}
        onDoubleClickCapture={event => {
          if (isPadView) return
          const target = event.target as HTMLElement
          if (
            !target.closest('.floating-window-header') ||
            isWindowHeaderInteractiveTarget(target)
          )
            return
          toggleMaximize()
        }}
      >
        <header
          className={`floating-window-header flex min-h-12 items-center justify-between gap-3 border-b border-slate-200 px-4 dark:border-slate-700${headerLeft ? ' floating-window-header-custom' : ''}`}
          onPointerDown={event => {
            if (isPadView) return
            if (isWindowHeaderInteractiveTarget(event.target)) return
            dragStart.current = {
              pointerId: event.pointerId,
              x: event.clientX,
              y: event.clientY,
              left: windowRect.left,
              top: windowRect.top
            }
          }}
        >
          <div className="flex min-w-0 items-center gap-2">
            {headerLeft ?? (
              <>
                {icon}
                <span className="grid min-w-0 gap-0.5">
                  <strong className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</strong>
                  {subtitle && <small className="truncate text-xs text-slate-400">{subtitle}</small>}
                </span>
              </>
            )}
          </div>
          <div className="flex items-center gap-1">
            {actions}
            {!isPhoneView && (
              <button className="icon-button size-8 p-0" onClick={onMinimize} aria-label={`最小化${title}`} title="最小化">
                <Minus className="size-4" />
              </button>
            )}
            <button className="icon-button size-8 p-0" onClick={onClose} aria-label={`关闭${title}`} title="关闭">
              <X className="size-4" />
            </button>
          </div>
        </header>
        {children}
        {!isPadView && (
          <WindowResizeHandles
            label={title}
            onStart={(corner, event) => {
              event.preventDefault()
              event.stopPropagation()
              resizeStart.current = {
                corner,
                x: event.clientX,
                y: event.clientY,
                width: windowRect.width,
                height: windowRect.height,
                left: windowRect.left,
                top: windowRect.top
              }
              event.currentTarget.setPointerCapture(event.pointerId)
            }}
            onMove={previewResize}
            onEnd={commitResize}
            onCancel={cancelResize}
          />
        )}
      </section>
    </Modal>
  )
}
