import { useEffect } from 'react'
import { useStoreState } from '../store/guideStore'
import { useGitCommitQuery, type GitWorkspace } from '../store/workspaceApi'
import { Modal } from './Modal'
import { DiffView } from './GitChanges'
import { gitErrorMessage } from './gitFeedback'

export function GitHistoryPanel({
  root,
  commits,
  offset,
  hasMore,
  busy,
  onPage
}: {
  root: string
  commits: GitWorkspace['commits']
  offset: number
  hasMore: boolean
  busy: boolean
  onPage: (offset: number) => void
}) {
  const [sha, setSha] = useStoreState<string | null>(null)
  const detail = useGitCommitQuery({ root, sha: sha ?? '' }, { skip: !sha })
  useEffect(() => {
    setSha(null)
  }, [root, offset, setSha])
  return (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
        <strong className="text-sm font-semibold text-slate-800">
          提交记录 · 第 {offset / 20 + 1} 页
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
            <li key={item.sha}>
              <button
                className="flex w-full items-start gap-3 rounded-md border-b border-slate-100 px-2 py-2 text-left hover:bg-slate-100"
                disabled={busy}
                aria-label={`查看提交 ${item.shortSha} ${item.subject}`}
                aria-pressed={sha === item.sha}
                onClick={() => setSha(item.sha)}
              >
                <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-700">
                  {item.shortSha}
                </code>
                <span className="grid min-w-0 gap-1">
                  <strong className="truncate text-xs font-semibold text-slate-700">
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
      {sha && (
        <Modal
          open
          onClose={() => setSha(null)}
          ariaLabel="提交详情"
          className="bg-slate-950/25 p-4"
        >
          <section className="flex max-h-[calc(100dvh-32px)] w-full max-w-4xl min-w-0 flex-col overflow-hidden rounded-lg border border-slate-200 bg-white">
            <header className="flex shrink-0 items-center justify-between gap-2 border-b border-slate-200 p-3">
              <strong className="text-sm text-slate-800">
                提交详情 · {sha.slice(0, 8)}
              </strong>
              <button className="text-button" onClick={() => setSha(null)}>
                关闭详情
              </button>
            </header>
            <div className="min-h-0 overflow-auto">
              {detail.isFetching ? (
                <p className="p-3 text-xs text-slate-500">正在读取提交详情…</p>
              ) : detail.error ? (
                <div className="p-3 text-xs text-slate-500">
                  <p role="alert">
                    {gitErrorMessage(detail.error, '读取提交详情失败。')}
                  </p>
                  <button
                    className="text-button"
                    onClick={() => void detail.refetch()}
                  >
                    重新读取
                  </button>
                </div>
              ) : (
                <DiffView diff={detail.data?.output ?? ''} untracked={false} />
              )}
            </div>
          </section>
        </Modal>
      )}
    </section>
  )
}
