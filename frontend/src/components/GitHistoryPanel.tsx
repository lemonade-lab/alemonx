import { RefreshButton } from './RefreshButton'
import { useGitCommitQuery, type GitWorkspace } from '../store/workspaceApi'
import { GitDiffPane } from './GitDiffPane'
import { DiffView } from './GitChanges'
import { gitErrorMessage } from './gitFeedback'

export function GitHistoryPanel({
  commits,
  offset,
  hasMore,
  busy,
  onPage,
  sha,
  onSelect
}: {
  commits: GitWorkspace['commits']
  offset: number
  hasMore: boolean
  busy: boolean
  onPage: (offset: number) => void
  sha: string | null
  onSelect: (sha: string) => void
}) {
  return (
    <section className="grid gap-3 p-1">
      <div className="flex flex-wrap items-center justify-between gap-3 pb-2">
        <strong className="text-sm font-semibold text-slate-800">
          第 {offset / 20 + 1} 页
        </strong>
        <div className="flex gap-2">
          <button
            className="secondary-button whitespace-nowrap"
            disabled={busy || offset === 0}
            onClick={() => onPage(Math.max(0, offset - 20))}
          >
            上一页
          </button>
          <button
            className="secondary-button whitespace-nowrap"
            disabled={busy || !hasMore}
            onClick={() => onPage(offset + 20)}
          >
            下一页
          </button>
        </div>
      </div>
      {commits.length ? (
        <ul className="grid gap-1">
          {commits.map(item => (
            <li className="min-w-0" key={item.sha}>
              <button
                className="flex w-full items-start gap-3 rounded-md px-2 py-2 text-left hover:bg-(--theme-surface-hover) aria-pressed:bg-(--theme-accent-soft)"
                disabled={busy}
                aria-label={`查看提交 ${item.shortSha} ${item.subject}`}
                aria-pressed={sha === item.sha}
                onClick={() => onSelect(item.sha)}
              >
                <code className="shrink-0 rounded bg-(--theme-surface-input) px-1.5 py-0.5 text-[10px]">
                  {item.shortSha}
                </code>
                <span className="grid min-w-0 gap-1">
                  <strong className="line-clamp-2 text-xs font-semibold [overflow-wrap:anywhere]" title={item.subject}>
                    {item.subject}
                  </strong>
                  <small className="text-[11px] text-slate-400">
                    {item.createdAt}
                  </small>
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="py-6 text-center text-xs text-slate-500">
          还没有提交记录。
        </p>
      )}
    </section>
  )
}

export function GitCommitDetail({ root, sha, inline, onClose }: {
  root: string
  sha: string
  inline: boolean
  onClose: () => void
}) {
  const detail = useGitCommitQuery({ root, sha })
  return (
    <GitDiffPane title="提交详情" path={`提交详情 · ${sha.slice(0, 8)}`} inline={inline} onClose={onClose}>
      {detail.isFetching ? (
        <p className="p-3 text-xs text-(--theme-text-muted)">正在读取提交详情…</p>
      ) : detail.error ? (
        <div className="p-3 text-xs text-(--theme-text-muted)">
          <p role="alert">{gitErrorMessage(detail.error, '读取提交详情失败。')}</p>
          <RefreshButton label="重新读取" onClick={() => void detail.refetch()} />
        </div>
      ) : (
        <DiffView diff={detail.currentData?.output ?? ''} untracked={false} />
      )}
    </GitDiffPane>
  )
}
