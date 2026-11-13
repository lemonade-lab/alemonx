import { Splitter } from 'antd'
import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useNavigationState } from '../hooks/useNavigationState'

const listLimits = { min: 17, max: 35, initial: 20 }
const controlsWidthPercent = 23

function clampListWidth(value: number) {
  return Math.max(listLimits.min, Math.min(listLimits.max, Math.round(value)))
}

export function RobotWorkspaceSplitter({
  list,
  children,
  controls,
  listHidden = false
}: {
  list: ReactNode
  children: ReactNode
  controls?: ReactNode
  listHidden?: boolean
}) {
  const root = useRef<HTMLDivElement>(null)
  const [listPreference, setListPreference] = useNavigationState(
    'workbench.splitter',
    ['list'],
    listLimits.initial
  )
  const listWidth = listHidden ? 0 : clampListWidth(listPreference)
  const controlsWidth = controls ? controlsWidthPercent : 0

  useEffect(() => {
    const bar = root.current?.querySelector<HTMLElement>(
      '.robot-workspace-list + [role="separator"]'
    )
    if (!bar) return
    bar.tabIndex = listHidden ? -1 : 0
    bar.setAttribute('aria-orientation', 'vertical')
    bar.setAttribute('aria-label', '调整机器人列表宽度')
  }, [listHidden])

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
          size={`${100 - listWidth - controlsWidth}%`}
          min="30%"
        >
          <section className="console-page">{children}</section>
        </Splitter.Panel>
        <Splitter.Panel
          className={`robot-workspace-controls${controls ? '' : ' robot-workspace-hidden'}`}
          size={`${controlsWidth}%`}
          resizable={false}
        >
          {controls}
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
