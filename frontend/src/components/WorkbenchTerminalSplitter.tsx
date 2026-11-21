import { Splitter } from 'antd'
import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useIsPadViewport } from '../hooks/useIsPadViewport'
import { useNavigationState } from '../hooks/useNavigationState'

const terminalLimits = { min: 25, max: 55 }

function clampTerminalWidth(width: number) {
  return Math.max(
    terminalLimits.min,
    Math.min(terminalLimits.max, Math.round(width))
  )
}

export function WorkbenchTerminalSplitter({
  children,
  terminal,
  terminalOpen
}: {
  children: ReactNode
  terminal: ReactNode
  terminalOpen: boolean
}) {
  const isTablet = useIsPadViewport()
  const resizable = terminalOpen && !isTablet
  const root = useRef<HTMLDivElement>(null)
  const [preference, setPreference] = useNavigationState(
    'workbench.splitter',
    ['terminal'],
    terminalLimits.min
  )
  const terminalWidth = resizable ? clampTerminalWidth(preference) : 0

  useEffect(() => {
    const bar = root.current?.querySelector<HTMLElement>(
      '.workbench-split-main + [role="separator"]'
    )
    if (!bar) return
    bar.tabIndex = resizable ? 0 : -1
    bar.setAttribute('aria-orientation', 'vertical')
    bar.setAttribute('aria-label', '调整终端宽度')
  }, [resizable])

  function saveWidths([mainSize, terminalSize]: number[]) {
    const total = mainSize + terminalSize
    if (resizable && total > 0)
      setPreference(clampTerminalWidth((terminalSize * 100) / total))
  }

  function resizeWithKeyboard(event: KeyboardEvent<HTMLDivElement>) {
    if (
      !resizable ||
      !(event.target instanceof HTMLElement) ||
      !event.target.matches('.workbench-split-main + [role="separator"]')
    )
      return

    const delta =
      event.key === 'ArrowLeft' ? 2 : event.key === 'ArrowRight' ? -2 : 0
    if (!delta) return
    setPreference(clampTerminalWidth(terminalWidth + delta))
    event.preventDefault()
  }

  return (
    <div
      ref={root}
      className="workbench-terminal-layout relative flex min-h-0 min-w-0 flex-1"
      data-resizable={resizable || undefined}
      data-terminal-open={terminalOpen || undefined}
      onKeyDown={resizeWithKeyboard}
    >
      <Splitter
        className="workbench-terminal-splitter min-h-0 min-w-0 flex-1"
        lazy
        onResizeEnd={saveWidths}
      >
        <Splitter.Panel
          className="workbench-split-main"
          size={`${100 - terminalWidth}%`}
          min={resizable ? `${100 - terminalLimits.max}%` : undefined}
          resizable={resizable}
        >
          {children}
        </Splitter.Panel>
        <Splitter.Panel
          className="workbench-split-terminal"
          size={`${terminalWidth}%`}
          min={resizable ? `${terminalLimits.min}%` : 0}
          max={resizable ? `${terminalLimits.max}%` : undefined}
          resizable={resizable}
        >
          {terminal}
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
