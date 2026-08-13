import { ArrowLeft, Settings, X } from 'lucide-react'
import { ThemeToggle } from './ThemeToggle'
import { Button } from './Button'
import { GitHubAuthControl } from './GitHubAuthControl'

type GuideHeaderProps = {
  onBack: () => void
  onClose: () => void
  onOpenSettings: () => void
  showBack: boolean
}

export function GuideHeader({
  onBack,
  onClose,
  onOpenSettings,
  showBack
}: GuideHeaderProps) {
  return (
    <header className="topbar relative flex min-h-11 shrink-0 items-center justify-between gap-2 border-b border-slate-200 bg-white/90 px-3 dark:border-slate-700">
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <Button
          variant="icon"
          onClick={onOpenSettings}
          aria-label="打开设置"
          title="设置"
        >
          <Settings className="size-4" />
        </Button>
        <a
          className="truncate px-1 text-[0.82rem] font-semibold tracking-[-0.01em] text-slate-800 no-underline transition-colors hover:text-brand-600 dark:text-slate-200"
          href="https://alemonjs.com/"
          target="_blank"
          rel="noreferrer"
        >
          ALemonX
        </a>
        <ThemeToggle />
        <GitHubAuthControl />
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
        aria-label="关闭引导"
        title="关闭引导"
        onClick={onClose}
      >
        <X className="size-4" />
      </Button>
    </header>
  )
}
