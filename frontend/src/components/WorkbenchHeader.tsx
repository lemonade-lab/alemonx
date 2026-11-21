import { createPortal } from 'react-dom'
import { Minus, Square, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useWorkbenchWindowDrag } from '../hooks/useWorkbenchWindowDrag'

import { bridge, desktopPlatform, windows } from './nativeWindow'

const resizeEdges = ['n', 'ne', 'e', 'se', 's', 'sw', 'w', 'nw'] as const

function doubleClickTitle() {
  bridge?.postMessage(
    windows ? 'alx:window:toggle-maximise' : 'wails:drag:doubleclick'
  )
}

export function WorkbenchDragRegion({ className = 'native-workbench-drag' }: {
  className?: string
}) {
  const dragHandlers = useWorkbenchWindowDrag(desktopPlatform !== 'linux')
  return (
    <div
      className={className}
      {...dragHandlers}
      onDoubleClick={desktopPlatform === 'linux' ? undefined : doubleClickTitle}
    />
  )
}

export function WorkbenchHeader({
  children,
  className = '',
  fallback = false
}: {
  children?: ReactNode
  className?: string
  fallback?: boolean
}) {
  if (!desktopPlatform) return <header className={className}>{children}</header>

  return createPortal(
    <header
      className="topbar native-workbench-header"
      data-platform={desktopPlatform}
      data-fallback={fallback || undefined}
    >
      {children}
      <WorkbenchDragRegion />
      {desktopPlatform === 'windows' && (
        <div
          className="native-window-controls"
          role="group"
          aria-label="窗口控制"
        >
          <button
            type="button"
            aria-label="最小化窗口"
            title="最小化窗口"
            onClick={() => bridge?.postMessage('alx:window:minimise')}
          >
            <Minus size={15} />
          </button>
          <button
            type="button"
            aria-label="最大化或还原窗口"
            title="最大化或还原窗口"
            onClick={() => bridge?.postMessage('alx:window:toggle-maximise')}
          >
            <Square size={12} />
          </button>
          <button
            type="button"
            aria-label="关闭窗口"
            title="关闭窗口"
            onClick={() => bridge?.postMessage('alx:window:close')}
          >
            <X size={16} />
          </button>
        </div>
      )}
      {desktopPlatform === 'windows' &&
        resizeEdges.map(edge => (
          <div
            key={edge}
            className={`native-resize-edge native-resize-${edge}`}
            onMouseDown={event => {
              if (event.button === 0)
                bridge?.postMessage(`wails:resize:${edge}-resize`)
            }}
          />
        ))}
    </header>,
    document.body
  )
}
