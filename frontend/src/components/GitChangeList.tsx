import { File, Minus, Plus } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from './Button'
import { changeStatusLabel } from './gitChangeTree'
import type { GitChange } from '../store/workspaceApi'

export type GitDiffMode = 'staged' | 'unstaged'

export function GitChangeList({
  label,
  changes,
  mode,
  selection,
  checkedPaths,
  busy,
  onSelect,
  onCheck,
  onToggle,
  actions
}: {
  label: string
  changes: GitChange[]
  mode: GitDiffMode
  selection: { path: string | null; mode: GitDiffMode }
  checkedPaths: string[]
  busy: boolean
  onSelect: (path: string, mode: GitDiffMode) => void
  onCheck?: (path: string, checked: boolean) => void
  onToggle?: (path: string) => void
  actions?: ReactNode
}) {
  const verb = mode === 'staged' ? '取消暂存' : '暂存'
  const Icon = mode === 'staged' ? Minus : Plus
  return (
    <section aria-label={label} className="grid gap-1">
      <header className="flex min-h-8 items-center justify-between gap-2 px-1">
        <h3 className="m-0 text-xs font-semibold text-(--theme-text-muted)">
          {label} <span className="ml-1 font-normal tabular-nums">{changes.length}</span>
        </h3>
        {actions}
      </header>
      {changes.length > 0 ? (
        <ul className="m-0 grid list-none gap-0.5 p-0">
          {changes.map(change => {
            const displayPath = change.path.replace(/\/+$/, '')
            const slash = displayPath.lastIndexOf('/')
            const selected = selection.path === change.path && selection.mode === mode
            return (
              <li className="flex min-w-0 items-center gap-1" key={change.path}>
                {onCheck && (
                  <input
                    type="checkbox"
                    aria-label={`选择 ${label}的 ${change.path}`}
                    checked={checkedPaths.includes(change.path)}
                    onChange={event => onCheck(change.path, event.target.checked)}
                  />
                )}
                <button
                  type="button"
                  className="flex min-h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-xs hover:bg-(--theme-surface-hover) aria-pressed:bg-(--theme-accent-soft) aria-pressed:text-(--theme-accent-text)"
                  title={change.path}
                  aria-label={`查看 ${change.path} 的${label}差异`}
                  aria-pressed={selected}
                  onClick={() => onSelect(change.path, mode)}
                >
                  <File className="size-4 shrink-0" aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate">
                    {displayPath.slice(slash + 1)}
                    {slash >= 0 && <small className="ml-1 text-(--theme-text-muted)">{displayPath.slice(0, slash)}</small>}
                  </span>
                  <span className="shrink-0 text-(--theme-text-muted)">
                    {change.conflict ? '冲突' : changeStatusLabel(change.status)}
                  </span>
                </button>
                {onToggle && (
                  <Button
                    variant="icon"
                    className="shrink-0 pointer-coarse:min-h-11 pointer-coarse:min-w-11"
                    aria-label={`${verb} ${change.path}`}
                    title={verb}
                    disabled={busy}
                    onClick={() => onToggle(change.path)}
                  >
                    <Icon className="size-4" aria-hidden="true" />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      ) : (
        <p className="m-0 px-1 py-1 text-xs text-(--theme-text-muted)">暂无{label}文件</p>
      )}
    </section>
  )
}
