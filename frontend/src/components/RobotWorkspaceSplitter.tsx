import { Splitter } from 'antd'
import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useNavigationState } from '../hooks/useNavigationState'
import { useIsMobileViewport } from '../hooks/useIsMobileViewport'
import { Modal } from './Modal'
import { Button } from './Button'
import { X } from 'lucide-react'

const listLimits = { min: 17, max: 35, initial: 20 }

function clampListWidth(value: number) {
  return Math.max(listLimits.min, Math.min(listLimits.max, Math.round(value)))
}

export function RobotWorkspaceSplitter({
  list,
  children,
  controls,
  listHidden = false,
  mobilePanel = null,
  onCloseMobilePanel
}: {
  list: ReactNode
  children: ReactNode
  controls?: ReactNode
  listHidden?: boolean
  mobilePanel?: 'projects' | 'robot' | null
  onCloseMobilePanel?: () => void
}) {
  const isCompact = useIsMobileViewport()
  const root = useRef<HTMLDivElement>(null)
  const [listPreference, setListPreference] = useNavigationState(
    'workbench.splitter',
    ['list'],
    listLimits.initial
  )
  const listWidth = listHidden ? 0 : clampListWidth(listPreference)

  useEffect(() => {
    const bar = root.current?.querySelector<HTMLElement>(
      '.robot-workspace-list + [role="separator"]'
    )
    if (!bar) return
    bar.tabIndex = listHidden ? -1 : 0
    bar.setAttribute('aria-orientation', 'vertical')
    bar.setAttribute('aria-label', '调整机器人列表宽度')
  }, [isCompact, listHidden])

  const saveWidths = (sizes: number[]) => {
    const total = sizes.reduce((sum, size) => sum + size, 0)
    if (!listHidden && total > 0)
      setListPreference(clampListWidth((sizes[0] * 100) / total))
  }

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (
      listHidden ||
      !(event.target instanceof HTMLElement) ||
      !event.target.matches('.robot-workspace-list + [role="separator"]')
    )
      return
    const delta =
      event.key === 'ArrowLeft' ? -2 : event.key === 'ArrowRight' ? 2 : 0
    if (!delta) return
    setListPreference(clampListWidth(listWidth + delta))
    event.preventDefault()
  }

  if (isCompact)
    return (
      <div className="robot-workspace">
        <section className="console-page flex-1 min-w-0">{children}</section>
        <Modal
          open={
            mobilePanel === 'projects' ||
            (mobilePanel === 'robot' && Boolean(controls))
          }
          onClose={onCloseMobilePanel}
          ariaLabel={
            mobilePanel === 'projects' ? '机器人与系统导航' : '机器人功能导航'
          }
          className={
            mobilePanel === 'projects' ? 'justify-start!' : 'justify-end!'
          }
        >
          <section className="workbench-mobile-navigation flex h-dvh w-[min(24rem,100%)] min-w-0 flex-col bg-(--theme-surface-panel) text-(--theme-text-strong) shadow-xl">
            <header className="flex shrink-0 items-center justify-between gap-2 border-b border-(--theme-border-default) px-3 pt-[env(safe-area-inset-top)]">
              <h2 className="m-0 text-base font-semibold">
                {mobilePanel === 'projects' ? '机器人与系统' : '机器人功能'}
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
      </div>
    )

  return (
    <div ref={root} className="robot-workspace" onKeyDown={resizeWithKeyboard}>
      <Splitter
        className="robot-workspace-splitter"
        lazy
        onResizeEnd={saveWidths}
        style={{ minHeight: 0 }}
      >
        <Splitter.Panel
          className={`robot-workspace-list${listHidden ? ' robot-workspace-hidden' : ''}`}
          size={`${listWidth}%`}
          min={listHidden ? 0 : `${listLimits.min}%`}
          max={`${listLimits.max}%`}
          resizable={!listHidden}
        >
          {list}
        </Splitter.Panel>
        <Splitter.Panel
          className="robot-workspace-content"
          size={`${100 - listWidth}%`}
          min="30%"
        >
          <section className="console-page">{children}</section>
          {controls && (
            <div className="robot-workspace-controls">{controls}</div>
          )}
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
