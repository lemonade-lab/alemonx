import { Folder, GitBranch, GitFork, Terminal } from 'lucide-react'

export function WorkbenchAuxiliaryStart({
  onOpenTerminal,
  onOpenRepository,
  extra,
  hasProject
}: {
  onOpenTerminal: () => void
  onOpenRepository: () => void
  extra: { kind: 'directory' | 'git'; open: () => void }
  hasProject: boolean
}) {
  const shortcuts = [
    { label: '终端', icon: Terminal, open: onOpenTerminal, disabled: false },
    { label: '仓库', icon: GitBranch, open: onOpenRepository, disabled: !hasProject },
    {
      label: extra.kind === 'git' ? 'Git 管理' : '目录',
      icon: extra.kind === 'git' ? GitFork : Folder,
      open: extra.open,
      disabled: !hasProject
    }
  ]

  return (
    <section
      aria-label="新标签页"
      className="flex min-h-0 flex-1 flex-col overflow-y-auto bg-(--theme-surface-panel) px-4 py-6 sm:px-6"
    >
      <div className="mx-auto my-auto w-full max-w-sm">
        <h2 className="mb-3 text-sm font-semibold text-(--theme-text-secondary)">
          打开
        </h2>
        <div className="grid grid-cols-3 gap-2">
          {shortcuts.map(({ label, icon: Icon, open, disabled }) => (
            <button
              key={label}
              type="button"
              className="flex min-h-20 flex-col items-start justify-center gap-2 rounded-lg bg-(--theme-surface-raised) px-2 py-2 text-left text-sm font-medium text-(--theme-text-strong) transition-colors enabled:hover:bg-(--theme-accent-soft) enabled:hover:text-(--theme-accent-text) disabled:opacity-50 sm:px-3"
              onClick={open}
              disabled={disabled}
            >
              <Icon className="size-4" aria-hidden="true" />
              {label}
            </button>
          ))}
        </div>
      </div>
    </section>
  )
}
