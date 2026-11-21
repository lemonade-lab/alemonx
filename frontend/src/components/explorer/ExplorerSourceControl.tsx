import { useId } from 'react'
import cn from 'classnames'
import { GitBranch, Minus, Plus, RefreshCw } from 'lucide-react'
import { useStoreState } from '../../store/guideStore'
import {
  useGitWorkspaceActionMutation,
  useGitWorkspaceQuery,
  type GitActionInput
} from '../../store/workspaceApi'
import { Button } from '../Button'
import { ConfirmDialog } from '../ConfirmDialog'

export function ExplorerSourceControl({
  project,
  defaultOpen,
  onOpen
}: {
  project: { name: string; path: string }
  defaultOpen: boolean
  onOpen: (path: string) => void
}) {
  const bodyId = useId()
  const [expanded, setExpanded] = useStoreState(defaultOpen)
  const [message, setMessage] = useStoreState('')
  const [pending, setPending] = useStoreState<GitActionInput | null>(null)
  const [failure, setFailure] = useStoreState('')
  const {
    currentData: git,
    isFetching,
    isError,
    refetch
  } = useGitWorkspaceQuery(
    { root: project.path, view: 'commit' },
    { skip: !expanded }
  )
  const [run, { isLoading }] = useGitWorkspaceActionMutation()
  const changes = git?.changes ?? []
  const canCommit =
    git?.atRepositoryRoot &&
    Boolean(git.indexRevision) &&
    changes.some(change => change.staged)

  async function execute(action: GitActionInput) {
    setFailure('')
    try {
      await run(action).unwrap()
      if (action.action === 'commit') setMessage('')
      setPending(null)
    } catch (error) {
      const detail = error as { data?: { error?: string } }
      setFailure(detail.data?.error || 'Git 操作未完成。')
      setPending(null)
    }
  }

  return (
    <section
      className={cn(
        'flex min-w-0 flex-col border-b border-(--theme-border-subtle) py-1',
        expanded && changes.length > 0 ? 'min-h-56 flex-[0_1_auto]' : 'shrink-0'
      )}
    >
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={expanded ? bodyId : undefined}
        className="flex min-h-8 w-full shrink-0 items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-(--theme-surface-hover)"
        onClick={() => setExpanded(value => !value)}
      >
        <GitBranch className="size-4 shrink-0" aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate font-medium">
          {project.name}
        </span>
        {git?.branch && (
          <span className="max-w-28 truncate text-xs text-(--theme-text-muted)">
            {git.branch}
          </span>
        )}
      </button>
      {expanded && (
        <div id={bodyId} className="flex min-h-0 flex-col gap-2 px-2 py-2">
          <div className="flex shrink-0 justify-between gap-1">
            <span className="text-xs text-(--theme-text-muted)">
              {isFetching
                ? '正在读取 Git 状态…'
                : git?.repository
                  ? `${changes.length} 项更改`
                  : !isError
                    ? '暂无 Git 仓库'
                    : '无法读取 Git 状态'}
            </span>
            <Button
              variant="icon"
              aria-label={`刷新 ${project.name} 的 Git 状态`}
              disabled={isFetching}
              onClick={() => void refetch()}
            >
              <RefreshCw className="size-4" aria-hidden="true" />
            </Button>
          </div>
          {git?.repository && (
            <>
              <form
                className="grid shrink-0 gap-2"
                onSubmit={event => {
                  event.preventDefault()
                  if (canCommit && message.trim())
                    setPending({
                      root: project.path,
                      action: 'commit',
                      message: message.trim(),
                      indexRevision: git.indexRevision,
                      expectedBranch: git.branch
                    })
                }}
              >
                <input
                  className="settings-input min-w-0"
                  aria-label={`${project.name} 的提交说明`}
                  placeholder="提交说明"
                  value={message}
                  onChange={event => setMessage(event.target.value)}
                />
                <Button
                  type="submit"
                  size="sm"
                  disabled={
                    !canCommit || !message.trim() || isLoading || isFetching
                  }
                >
                  提交已暂存更改
                </Button>
              </form>
              <ul
                role="list"
                aria-label={`${project.name} 的更改文件`}
                tabIndex={changes.length > 0 ? 0 : undefined}
                className="m-0 grid min-h-0 list-none content-start gap-1 overflow-y-auto overscroll-contain p-0"
              >
                {changes.map(change => (
                  <li
                    className="flex min-w-0 items-center gap-1"
                    key={change.path}
                  >
                    <button
                      type="button"
                      className="min-w-0 flex-1 truncate rounded-md px-1 py-1 text-left text-xs hover:bg-(--theme-surface-hover)"
                      title={change.path}
                      onClick={() => onOpen(change.path)}
                    >
                      <span className="mr-2 font-mono">{change.status}</span>
                      {change.path}
                    </button>
                    {change.staged && (
                      <Button
                        variant="icon"
                        aria-label={`取消暂存 ${change.path}`}
                        disabled={
                          isLoading || isFetching || !git.atRepositoryRoot
                        }
                        onClick={() =>
                          void execute({
                            root: project.path,
                            action: 'unstage',
                            paths: [change.path],
                            expectedBranch: git.branch
                          })
                        }
                      >
                        <Minus className="size-4" aria-hidden="true" />
                      </Button>
                    )}
                    {change.unstaged && (
                      <Button
                        variant="icon"
                        aria-label={`暂存 ${change.path}`}
                        disabled={
                          isLoading || isFetching || !git.atRepositoryRoot
                        }
                        onClick={() =>
                          void execute({
                            root: project.path,
                            action: 'stage',
                            paths: [change.path],
                            expectedBranch: git.branch
                          })
                        }
                      >
                        <Plus className="size-4" aria-hidden="true" />
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            </>
          )}
          {failure && (
            <p
              className="shrink-0 text-xs text-(--theme-danger-text)"
              role="alert"
            >
              {failure}
            </p>
          )}
        </div>
      )}
      <ConfirmDialog
        open={Boolean(pending)}
        title="提交已暂存更改"
        message={
          pending
            ? `${project.name} · ${pending.expectedBranch ?? ''} · ${pending.message}`
            : ''
        }
        confirmLabel="提交"
        busy={isLoading}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (pending) void execute(pending)
        }}
      />
    </section>
  )
}
