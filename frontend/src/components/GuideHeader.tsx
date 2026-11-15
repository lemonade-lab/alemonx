import { ArrowLeft, LayoutDashboard } from 'lucide-react'
import { ThemeToggle } from './ThemeToggle'
import { WorkbenchHeader } from './WorkbenchHeader'
import { Button } from './Button'

type GuideHeaderProps = {
  onBack: () => void
  onClose: () => void
  onOpenSettings: () => void
  showBack: boolean
}

export function GuideHeader({ onBack, onClose, showBack }: GuideHeaderProps) {
  return (
    <WorkbenchHeader className="topbar relative flex min-h-11 shrink-0 items-center justify-between gap-2 border-b border-slate-200 bg-white/90 px-3 dark:border-slate-700">
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <ThemeToggle />
        {showBack && (
          <Button
            variant="icon"
            onClick={onBack}
            aria-label="返回"
            title="返回"
          >
            <ArrowLeft className="size-4" />
          </Button>
        )}
      </div>
      <Button
        variant="icon"
        className="focus:outline-none focus:ring-2 focus:ring-brand-200"
        aria-label="返回工作台"
        title="返回工作台"
        onClick={onClose}
      >
        <LayoutDashboard className="size-4" />
      </Button>
    </WorkbenchHeader>
  )
}
