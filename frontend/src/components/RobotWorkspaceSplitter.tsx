import type { ReactNode } from 'react'
import { WorkspaceSplitPane } from './WorkspaceSplitPane'
import { useNavigationState } from '../hooks/useNavigationState'
import { useIsMobileViewport } from '../hooks/useIsMobileViewport'
import { Modal } from './Modal'
import { Button } from './Button'
import { X } from 'lucide-react'

const listLimits = { min: 17, max: 35, initial: 20 }

export function RobotWorkspaceSplitter({
  list,
  children,
  controls,
  renderContent,
  listHidden = false,
  listTitle = '机器人',
  mobilePanel = null,
  onCloseMobilePanel
}: {
  list: ReactNode
  children: ReactNode
  controls?: ReactNode
  renderContent?: (body: ReactNode) => ReactNode
  listHidden?: boolean
  listTitle?: string
  mobilePanel?: 'projects' | 'robot' | null
  onCloseMobilePanel?: () => void
}) {
  const isCompact = useIsMobileViewport()
  const [listPreference, setListPreference] = useNavigationState(
    'workbench.splitter',
    ['list'],
    listLimits.initial
  )
  const showList = !isCompact && !listHidden
  const body = (
    <div className="robot-workspace-content">
      <section className="console-page">{children}</section>
      {!isCompact && controls && (
        <div className="robot-workspace-controls">{controls}</div>
      )}
    </div>
  )
  const content = renderContent ? renderContent(body) : body

  return (
    <div className="robot-workspace">
      <WorkspaceSplitPane
        className="robot-workspace-splitter"
        leadingClassName={`robot-workspace-list${showList ? '' : ' robot-workspace-hidden'}`}
        trailingClassName="robot-workspace-main"
        leading={isCompact ? null : list}
        trailing={content}
        sizedPane="leading"
        width={listPreference}
        min={listLimits.min}
        max={listLimits.max}
        resizable={showList}
        separatorLabel="调整机器人列表宽度"
        onWidthChange={setListPreference}
      />
      {isCompact && (
        <Modal
          open={
            mobilePanel === 'projects' ||
            (mobilePanel === 'robot' && Boolean(controls))
          }
          onClose={onCloseMobilePanel}
          ariaLabel={
            mobilePanel === 'projects' ? `${listTitle}导航` : '机器人功能导航'
          }
          className={
            mobilePanel === 'projects' ? 'justify-start!' : 'justify-end!'
          }
        >
          <section className="workbench-mobile-navigation flex h-dvh w-[min(24rem,100%)] min-w-0 flex-col bg-(--theme-surface-panel) text-(--theme-text-strong) shadow-xl">
            <header className="flex shrink-0 items-center justify-between gap-2 border-b border-(--theme-border-default) px-3 pt-[env(safe-area-inset-top)]">
              <h2 className="m-0 text-base font-semibold">
                {mobilePanel === 'projects' ? listTitle : '机器人功能'}
              </h2>
              <Button
                variant="icon"
                aria-label="关闭导航"
                onClick={onCloseMobilePanel}
              >
                <X className="size-5" />
              </Button>
            </header>
            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-[env(safe-area-inset-bottom)]">
              {mobilePanel === 'projects' ? list : controls}
            </div>
          </section>
        </Modal>
      )}
    </div>
  )
}
