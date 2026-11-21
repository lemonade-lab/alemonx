import { type ReactNode } from 'react'
import { FolderTree, House, Settings } from 'lucide-react'
import { Button } from './Button'
import { DSHWhaleIcon } from './DSHWhaleIcon'
import { useWorkbenchWindowDrag } from '../hooks/useWorkbenchWindowDrag'

export type WorkbenchToolAction = {
  id: string
  label: string
  icon: ReactNode
  activate: () => void
}

export type WorkbenchView = 'home' | 'dsh' | 'explorer' | 'guide'

type WorkbenchToolRailProps = {
  windows: WorkbenchToolAction[]
  activeView: WorkbenchView
  onShowHome: () => void
  onOpenDSH: () => void
  onOpenExplorer: () => void
  onOpenSettings: () => void
}

export function WorkbenchToolRail({
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
      id: 'dsh',
      label: 'DSH',
      icon: <DSHWhaleIcon />,
      onClick: onOpenDSH
    },
    {
      id: 'explorer',
      label: '资源管理器',
      icon: <FolderTree aria-hidden="true" />,
      onClick: onOpenExplorer
    }
  ]
  const buttonClass = 'size-8 shrink-0 [&>svg]:size-4'

  return (
    <nav
      aria-label="工作台工具"
      {...dragHandlers}
      className="workbench-tool-rail flex w-12 shrink-0 flex-col items-center gap-2 bg-[oklch(from_var(--theme-surface-raised)_calc(l_-_0.012)_c_h)] py-2"
    >
      {pinnedViews.map(view => (
        <Button
          key={view.id}
          variant="icon"
          className={`${buttonClass} aria-[current=page]:bg-(--theme-accent-soft)! aria-[current=page]:text-(--theme-accent-text)!`}
          aria-label={view.label}
          aria-current={activeView === view.id ? 'page' : undefined}
          title={view.label}
          onClick={view.onClick}
        >
          {view.icon}
        </Button>
      ))}
      <div
        className="w-5 shrink-0 border-t border-(--theme-border-default)"
        role="separator"
      />
      <div
        className="flex min-h-0 w-full flex-1 flex-col items-center gap-1 overflow-y-auto"
        role="group"
        aria-label="已打开窗口"
      >
        {windows.map(window => (
          <Button
            key={window.id}
            variant="icon"
            className={buttonClass}
            aria-label={window.label}
            title={window.label}
            onClick={window.activate}
          >
            {window.icon}
          </Button>
        ))}
      </div>
      <Button
        variant="icon"
        className={buttonClass}
        aria-label="打开设置"
        title="设置"
        onClick={onOpenSettings}
      >
        <Settings aria-hidden="true" />
      </Button>
    </nav>
  )
}
