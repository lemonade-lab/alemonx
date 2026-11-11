import { Splitter } from 'antd'
import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useNavigationState } from '../hooks/useNavigationState'

const widthLimits = {
  list: { min: 17, max: 35, initial: 20 },
  controls: { min: 21, max: 35, initial: 23 }
}

function clampWidth(value: number, limits: { min: number; max: number }) {
  return Math.max(limits.min, Math.min(limits.max, Math.round(value)))
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
    widthLimits.list.initial
  )
  const [controlsPreference, setControlsPreference] = useNavigationState(
    'workbench.splitter',
    ['controls'],
    widthLimits.controls.initial
  )
  const listWidth = listHidden
    ? 0
    : clampWidth(listPreference, widthLimits.list)
  const controlsWidth = controls
    ? clampWidth(controlsPreference, widthLimits.controls)
    : 0

  useEffect(() => {
    // Ant Design 5 provides separator semantics and pointer dragging.
    // Add focus and names to its handles so the same dividers work by keyboard.
    const bars = root.current?.querySelectorAll<HTMLElement>(
      '.robot-workspace-splitter > [role="separator"]'
    )
    bars?.forEach((bar, index) => {
      bar.tabIndex = 0
      bar.setAttribute('aria-orientation', 'vertical')
      bar.setAttribute(
        'aria-label',
        index === 0 ? '调整机器人列表宽度' : '调整机器人面板宽度'
      )
    })
  }, [])

  const saveWidths = (sizes: number[]) => {
    const total = sizes.reduce((sum, size) => sum + size, 0)
    if (total <= 0) return
    if (!listHidden)
      setListPreference(clampWidth((sizes[0] * 100) / total, widthLimits.list))
    if (controls)
      setControlsPreference(
        clampWidth((sizes[2] * 100) / total, widthLimits.controls)
      )
  }

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (
      !(event.target instanceof HTMLElement) ||
      event.target.getAttribute('role') !== 'separator' ||
      !event.target.parentElement?.classList.contains(
        'robot-workspace-splitter'
      )
    )
      return
    const delta =
      event.key === 'ArrowLeft' ? -2 : event.key === 'ArrowRight' ? 2 : 0
    if (!delta) return
    const listHandle = event.target.previousElementSibling?.classList.contains(
      'robot-workspace-list'
    )
    if (listHandle && !listHidden) {
      setListPreference(clampWidth(listWidth + delta, widthLimits.list))
    } else if (!listHandle && controls) {
      setControlsPreference(
        clampWidth(controlsWidth - delta, widthLimits.controls)
      )
    }
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
          min={listHidden ? 0 : `${widthLimits.list.min}%`}
          max={`${widthLimits.list.max}%`}
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
          min={controls ? `${widthLimits.controls.min}%` : 0}
          max={`${widthLimits.controls.max}%`}
          resizable={Boolean(controls)}
        >
          {controls}
        </Splitter.Panel>
      </Splitter>
    </div>
  )
}
