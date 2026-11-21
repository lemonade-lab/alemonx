import type { ReactNode } from 'react'
import { WorkspaceSplitPane } from './WorkspaceSplitPane'
import { useIsPadViewport } from '../hooks/useIsPadViewport'
import { useNavigationState } from '../hooks/useNavigationState'

const terminalLimits = { min: 25, max: 55 }

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
  const [preference, setPreference] = useNavigationState(
    'workbench.splitter',
    ['terminal'],
    terminalLimits.min
  )
  return (
    <div
      className="workbench-terminal-layout relative flex min-h-0 min-w-0 flex-1"
      data-terminal-open={terminalOpen || undefined}
    >
      <WorkspaceSplitPane
        className="workbench-terminal-splitter min-h-0 min-w-0 flex-1"
        leadingClassName="workbench-split-main"
        trailingClassName="workbench-split-terminal"
        leading={children}
        trailing={terminal}
        sizedPane="trailing"
        width={preference}
        min={terminalLimits.min}
        max={terminalLimits.max}
        resizable={resizable}
        separatorLabel="调整终端宽度"
        onWidthChange={setPreference}
      />
    </div>
  )
}
