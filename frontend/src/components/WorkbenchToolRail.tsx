import { useId, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { FolderTree, House, Settings } from 'lucide-react'
import { Button } from './Button'
import { DSHWhaleIcon } from './DSHWhaleIcon'
import { useWorkbenchWindowDrag } from '../hooks/useWorkbenchWindowDrag'
import { useViewportPopoverPosition } from '../hooks/useViewportPopoverPosition'

export type WorkbenchToolAction = {
  id: string
  label: string
  icon: ReactNode
  activate: () => void
}

export type WorkbenchView = 'home' | 'dsh' | 'explorer' | 'guide'

type WorkbenchToolRailProps = {
  systemTools: WorkbenchToolAction[]
  activeSystemTool: string | null
  windows: WorkbenchToolAction[]
  activeView: WorkbenchView
  onShowHome: () => void
  onOpenDSH: () => void
  onOpenExplorer: () => void
  onOpenSettings: () => void
}

function RailButton({
  label,
  tooltip = label,
  icon,
  active,
  onClick
}: {
  label: string
  tooltip?: string
  icon: ReactNode
  active?: boolean
  onClick: () => void
}) {
  const id = useId()
  const trigger = useRef<HTMLButtonElement>(null)
  const popup = useRef<HTMLDivElement>(null)
  const [visible, setVisible] = useState(false)
  const position = useViewportPopoverPosition({
    anchor: null,
    anchorRef: trigger,
    open: visible,
    placement: 'right',
    popoverRef: popup
  })

  return (
    <>
      <Button
        ref={trigger}
        type="button"
        variant="icon"
        className="size-8 shrink-0 [&>svg]:size-4 aria-[current=page]:bg-(--theme-accent-soft)! aria-[current=page]:text-(--theme-accent-text)!"
        aria-label={label}
        aria-current={active ? 'page' : undefined}
        aria-describedby={visible && tooltip !== label ? id : undefined}
        onPointerEnter={event => {
          if (event.pointerType !== 'touch') setVisible(true)
        }}
        onPointerLeave={event => {
          if (!event.currentTarget.matches(':focus-visible')) setVisible(false)
        }}
        onFocus={event => {
          if (event.currentTarget.matches(':focus-visible')) setVisible(true)
        }}
        onBlur={() => setVisible(false)}
        onKeyDown={event => {
          if (event.key === 'Escape' && visible) {
            event.preventDefault()
            event.stopPropagation()
            setVisible(false)
          }
        }}
        onClick={() => {
          setVisible(false)
          onClick()
        }}
      >
        {icon}
      </Button>
      {visible && createPortal(
        <div
          id={id}
          ref={popup}
          role="tooltip"
          className="pointer-events-none fixed z-[1200] max-w-56 rounded-control border border-(--theme-border-default) bg-(--theme-surface-panel) px-2 py-1 text-xs text-(--theme-text-strong) shadow-(--theme-shadow-pop)"
          style={position}
        >
          {tooltip}
        </div>,
        document.body
      )}
    </>
  )
}

export function WorkbenchToolRail({
  systemTools,
  activeSystemTool,
  windows,
  activeView,
  onShowHome,
  onOpenDSH,
  onOpenExplorer,
  onOpenSettings
}: WorkbenchToolRailProps) {
  const dragHandlers = useWorkbenchWindowDrag(true)
  const pinnedViews = [
    {
      id: 'home',
      label: '主页',
      icon: <House aria-hidden="true" />,
      onClick: onShowHome
    },
    {
      id: 'explorer',
      label: '资源管理器',
      icon: <FolderTree aria-hidden="true" />,
      onClick: onOpenExplorer
    },
    {
      id: 'dsh',
      label: 'DSH',
      icon: <DSHWhaleIcon />,
      onClick: onOpenDSH
    }
  ]
  return (
    <nav
      aria-label="工作台工具"
      {...dragHandlers}
      className="workbench-tool-rail flex w-12 shrink-0 flex-col items-center gap-2 bg-[oklch(from_var(--theme-surface-raised)_calc(l_-_0.012)_c_h)] py-2"
    >
      {pinnedViews.map(view => (
        <RailButton
          key={view.id}
          label={view.label}
          active={activeView === view.id}
          icon={view.icon}
          onClick={view.onClick}
        />
      ))}
      <div
        className="w-5 shrink-0 border-t border-(--theme-border-default)"
        role="separator"
      />
      <div className="flex min-h-0 w-full flex-1 flex-col items-center gap-2 overflow-y-auto [scrollbar-width:none]">
        <div className="flex shrink-0 flex-col items-center gap-1" role="group" aria-label="系统功能">
          {systemTools.map(tool => (
            <RailButton
              key={tool.id}
              label={tool.label}
              icon={tool.icon}
              active={activeSystemTool === tool.id}
              onClick={tool.activate}
            />
          ))}
        </div>
        <div className="flex min-h-4 w-full flex-1 flex-col items-center gap-1" role="group" aria-label="已打开窗口">
          {windows.map(window => (
            <RailButton key={window.id} label={window.label} icon={window.icon} onClick={window.activate} />
          ))}
        </div>
      </div>
      <RailButton
        label="打开设置"
        tooltip="设置"
        icon={<Settings aria-hidden="true" />}
        onClick={onOpenSettings}
      />
    </nav>
  )
}
