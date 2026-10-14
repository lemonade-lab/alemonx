import { useMemo } from 'react'
import { ChevronDown, File, Folder } from 'lucide-react'
import { changeStatusLabel, type ChangeTreeNode } from './gitChangeTree'

export function ChangeTree({
  nodes,
  selectedPath,
  onSelect,
  checkedPaths,
  onCheck
}: {
  nodes: ChangeTreeNode[]
  selectedPath: string | null
  onSelect: (path: string) => void
  checkedPaths: string[]
  onCheck: (path: string, checked: boolean) => void
}) {
  return (
    <ul className="grid min-w-0 gap-0.5 pl-4 first:pl-0">
      {nodes.map(node => {
        const folder = node.children.size > 0 || node.change?.path.endsWith('/')
        return (
          <li className="min-w-0" key={node.path}>
            {node.children.size > 0 ? (
              <details className="group min-w-0" open>
                <summary className="flex cursor-pointer list-none items-center gap-1.5 rounded px-1.5 py-1 text-xs text-slate-700 hover:bg-slate-100 [&::-webkit-details-marker]:hidden">
                  <ChevronDown className="size-3.5 shrink-0 transition-transform group-not-open:-rotate-90" />
                  <Folder className="size-3.5 shrink-0 text-slate-400" />
                  <span className="min-w-0 flex-1 truncate">{node.name}</span>
                  {node.change && (
                    <code
                      className={`shrink-0 rounded px-1 text-[10px] font-medium ${node.change.status.trim() === '??' ? 'bg-slate-100 text-slate-500' : 'bg-brand-50 text-brand-700'}`}
                    >
                      {changeStatusLabel(node.change.status)}
                    </code>
                  )}
                </summary>
                <ChangeTree
                  nodes={[...node.children.values()]}
                  selectedPath={selectedPath}
                  onSelect={onSelect}
                  checkedPaths={checkedPaths}
                  onCheck={onCheck}
                />
              </details>
            ) : (
              <div className="flex min-w-0 items-center gap-1">
                <input
                  type="checkbox"
                  checked={checkedPaths.includes(node.path)}
                  aria-label={`选择 ${node.path}`}
                  disabled={!node.change}
                  onChange={event => onCheck(node.path, event.target.checked)}
                />
                <button
                  type="button"
                  disabled={!node.change}
                  onClick={() => node.change && onSelect(node.path)}
                  title={
                    node.change ? `查看 ${node.path} 的变更对比` : undefined
                  }
                  className={`flex min-w-0 w-full items-center gap-1.5 rounded px-1.5 py-1 text-left text-xs text-slate-600 hover:bg-slate-100 disabled:cursor-default disabled:opacity-60 ${selectedPath === node.path ? 'bg-brand-50 text-brand-700' : ''}`}
                >
                  {folder ? (
                    <Folder className="size-3.5 shrink-0 text-slate-400" />
                  ) : (
                    <File className="size-3.5 shrink-0 text-slate-400" />
                  )}
                  <span className="min-w-0 flex-1 truncate">{node.name}</span>
                  <code
                    className={`shrink-0 rounded px-1 text-[10px] font-medium ${node.change && node.change.status.trim() === '??' ? 'bg-slate-100 text-slate-500' : 'bg-brand-50 text-brand-700'}`}
                  >
                    {node.change
                      ? `${node.change.conflict ? '冲突 · ' : ''}${changeStatusLabel(node.change.status)}${node.change.staged ? ' · 已暂存' : ''}${node.change.staged && node.change.unstaged ? ' · 另有修改' : ''}`
                      : '变更'}
                  </code>
                </button>
              </div>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function DiffView({
  diff,
  untracked
}: {
  diff: string
  untracked: boolean
}) {
  const lines = useMemo(() => diff.split('\n'), [diff])
  return (
    <pre className="git-diff-body">
      {lines.map((line, index) => {
        let className = 'git-diff-line'
        if (line.startsWith('@@')) className += ' git-diff-hunk'
        else if (
          line.startsWith('diff --git') ||
          line.startsWith('index ') ||
          line.startsWith('--- ') ||
          line.startsWith('+++ ') ||
          line.startsWith('new file') ||
          line.startsWith('deleted file') ||
          line.startsWith('similarity') ||
          line.startsWith('rename ')
        )
          className += ' git-diff-meta'
        else if (line.startsWith('+')) className += ' git-diff-add'
        else if (line.startsWith('-')) className += ' git-diff-del'
        return (
          <div key={index} className={className}>
            {line || (untracked ? '\u00a0' : ' ')}
          </div>
        )
      })}
    </pre>
  )
}
