import { useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { FolderTree, House, Settings } from 'lucide-react'
import { Button } from './Button'
import { DSHWhaleIcon } from './DSHWhaleIcon'
import { WorkbenchTools } from './WorkbenchTools'
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
  pluginTools: WorkbenchToolAction[]
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
  pluginTools,
  activeSystemTool,
  windows,
  activeView,
  onShowHome,
  onOpenDSH,
  onOpenExplorer,
  onOpenSettings
}: WorkbenchToolRailProps) {
  const dragHandlers = useWorkbenchWindowDrag(true)
  const toolList = useRef<HTMLDivElement>(null)
  const pluginGroup = useRef<HTMLDivElement>(null)
  const systemGroup = useRef<HTMLDivElement>(null)
  const windowGroup = useRef<HTMLDivElement>(null)
  const [pluginSlots, setPluginSlots] = useState(1)
  const hasPlugins = pluginTools.length > 0

  useLayoutEffect(() => {
    const list = toolList.current
    const plugins = pluginGroup.current
    const system = systemGroup.current
    const windows = windowGroup.current
    if (!hasPlugins || !list || !plugins || !system || !windows) return

    const update = () => {
      const button = system.querySelector('button') ?? plugins.querySelector('button')
      if (!button) return
      const rowHeight = button.getBoundingClientRect().height
      if (!rowHeight) return
      const groupGap = Number.parseFloat(getComputedStyle(list).rowGap)
      const rowGap = Number.parseFloat(getComputedStyle(plugins).rowGap)
      const space = list.clientHeight - system.offsetHeight - windows.offsetHeight - 2 * groupGap
      setPluginSlots(Math.max(1, Math.floor((space + rowGap) / (rowHeight + rowGap))))
    }

    const observer = new ResizeObserver(update)
    observer.observe(list)
    observer.observe(system)
    observer.observe(windows)
    update()
    return () => observer.disconnect()
  }, [hasPlugins])

  const visiblePluginCount = pluginTools.length > pluginSlots ? pluginSlots - 1 : pluginTools.length
  const overflowPlugins = pluginTools.slice(visiblePluginCount)
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
      className="workbench-tool-rail flex min-h-0 w-12 shrink-0 flex-col items-center gap-2 bg-[oklch(from_var(--theme-surface-raised)_calc(l_-_0.012)_c_h)] py-2"
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
      <div ref={toolList} className="flex min-h-0 w-full flex-1 flex-col items-center gap-2 overflow-y-auto [scrollbar-width:none]">
        <div ref={systemGroup} className="flex shrink-0 flex-col items-center gap-1" role="group" aria-label="系统功能">
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
        {hasPlugins && (
          <div ref={pluginGroup} className="flex shrink-0 flex-col items-center gap-1" role="group" aria-label="已启动插件">
            {pluginTools.slice(0, visiblePluginCount).map(tool => (
              <RailButton
                key={tool.id}
                label={tool.label}
                icon={tool.icon}
                active={activeSystemTool === tool.id}
                onClick={tool.activate}
              />
            ))}
            {overflowPlugins.length > 0 && (
              <WorkbenchTools
                label="更多插件"
                menuLabel="更多插件列表"
                placement="right"
                menuClassName="overflow-y-auto [--workbench-menu-layer:1250]"
                restoreFocusOnClose
              >
                {close => overflowPlugins.map(tool => (
                  <button
                    key={tool.id}
                    type="button"
                    className="workbench-action-item aria-[current=page]:bg-(--theme-accent-soft)!"
                    aria-current={activeSystemTool === tool.id ? 'page' : undefined}
                    title={tool.label}
                    onClick={() => {
                      close()
                      tool.activate()
                    }}
                  >
                    <span className="grid size-4 shrink-0 place-items-center [&>svg]:size-4" aria-hidden="true">{tool.icon}</span>
                    <span className="min-w-0 truncate">{tool.label}</span>
                  </button>
                ))}
              </WorkbenchTools>
            )}
          </div>
        )}
        <div ref={windowGroup} className="flex min-h-4 w-full shrink-0 flex-col items-center gap-1" role="group" aria-label="已打开窗口">
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
