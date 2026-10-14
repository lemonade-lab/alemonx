import type { GitAction, GitWorkspace } from '../store/workspaceApi'

type Props = {
  data?: GitWorkspace
  busy: boolean
  branchName: string
  onBranchNameChange: (name: string) => void
  request: (
    action: GitAction,
    value?: string,
    message?: string,
    paths?: string[],
    remote?: string
  ) => void
}

export function GitBranchPanel({
  data,
  busy,
  branchName,
  onBranchNameChange,
  request
}: Props) {
  const branches = [...(data?.branches ?? [])].sort(
    (a, b) => Number(b.current) - Number(a.current)
  )
  const remoteBranches = data?.remoteBranches ?? []
  const remote = data?.remoteName || data?.remotes[0]?.name
  return (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <form
        className="flex min-w-0 items-center gap-2 border-b border-slate-200 pb-3"
        onSubmit={event => {
          event.preventDefault()
          if (!busy && branchName.trim()) request('branch-create', branchName)
        }}
      >
        <input
          className="h-9 min-w-0 rounded-md border border-slate-300 bg-white px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100 sm:w-64"
          aria-label="新分支名称"
          value={branchName}
          onChange={event => onBranchNameChange(event.target.value)}
          placeholder="新分支名称，例如 feat/login"
          required
          disabled={busy}
        />
        <button
          type="submit"
          className="secondary-button shrink-0 whitespace-nowrap"
          disabled={busy || !branchName.trim()}
        >
          创建并切换
        </button>
      </form>
      {branches.length || remoteBranches.length ? (
        <ul className="grid gap-1">
          {branches.map(item => (
            <li
              key={item.name}
              aria-label={`本地分支 ${item.name}`}
              className="flex min-w-0 flex-col gap-2 border-b border-slate-100 px-2 py-2 last:border-0 sm:flex-row sm:items-center sm:gap-3"
            >
              <code
                className={`shrink-0 self-start rounded px-1.5 py-0.5 text-[10px] sm:self-auto ${item.current ? 'bg-brand-50 font-semibold text-brand-700' : 'bg-slate-100 text-slate-600'}`}
              >
                {item.name}
              </code>
              <span className="grid min-w-0 flex-1 gap-0.5">
                <strong className="text-xs font-semibold text-slate-700">
                  {item.current ? '当前分支' : '本地分支'}
                </strong>
                <small className="text-[11px] text-slate-500">
                  {item.upstream ? `跟踪 ${item.upstream}` : '尚未关联远程分支'}
                </small>
              </span>
              <div className="flex shrink-0 flex-wrap items-center gap-1">
                {item.current ? (
                  <>
                    <button
                      className="text-button whitespace-nowrap"
                      disabled={busy || !remote || data?.mergeInProgress}
                      title={
                        !remote
                          ? '请先配置远程仓库'
                          : item.upstream
                            ? `从 ${item.upstream} 快进更新 ${item.name}`
                            : `从 ${remote} 的同名分支更新 ${item.name}`
                      }
                      onClick={() =>
                        request('pull', item.name, undefined, undefined, remote)
                      }
                    >
                      更新
                    </button>
                    {data?.mergeInProgress && (
                      <button
                        className="text-button whitespace-nowrap"
                        disabled={busy}
                        onClick={() => request('merge-abort', item.name)}
                      >
                        中止合并
                      </button>
                    )}
                  </>
                ) : (
                  <>
                    <button
                      className="text-button whitespace-nowrap"
                      disabled={busy}
                      onClick={() => request('branch-switch', item.name)}
                    >
                      切换
                    </button>
                    <button
                      className="text-button whitespace-nowrap"
                      disabled={busy || !data?.branch}
                      onClick={() => request('merge', item.name)}
                    >
                      合并到当前分支
                    </button>
                    <button
                      className="text-button whitespace-nowrap text-red-700"
                      disabled={busy}
                      onClick={() => request('branch-delete', item.name)}
                    >
                      删除
                    </button>
                  </>
                )}
              </div>
            </li>
          ))}
          {remoteBranches.map(item => (
            <li
              key={item.name}
              aria-label={`远程分支 ${item.name}`}
              className="flex min-w-0 flex-col gap-2 border-b border-slate-100 px-2 py-2 last:border-0 sm:flex-row sm:items-center sm:gap-3"
            >
              <code className="shrink-0 self-start rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600 sm:self-auto">
                {item.branch}
              </code>
              <span className="min-w-0 flex-1 text-xs text-slate-700">
                远程分支 · {item.name}
              </span>
              <div className="flex shrink-0 flex-wrap items-center gap-1">
                {!branches.some(branch => branch.name === item.branch) && (
                  <button
                    className="text-button whitespace-nowrap"
                    disabled={busy}
                    onClick={() => request('branch-track', item.name)}
                  >
                    在本机打开
                  </button>
                )}
                <button
                  className="text-button whitespace-nowrap"
                  disabled={busy || !data?.branch}
                  onClick={() => request('merge', `refs/remotes/${item.name}`)}
                >
                  合并到当前分支
                </button>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="py-6 text-center text-xs text-slate-500">
          还没有分支。在顶部输入名称，创建你的第一个分支。
        </p>
      )}
    </section>
  )
}
