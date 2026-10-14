import { useStoreState } from '../store/guideStore'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  GitBranch,
  GitCommitHorizontal,
  History,
  Network,
  RefreshCw,
  Tags,
  X
} from 'lucide-react'
import { ConfirmDialog } from './ConfirmDialog'
import { DesktopWindow } from './DesktopWindow'
import { Tabs } from './Tabs'
import { gitErrorMessage } from './gitFeedback'
import { GitSettingsDialog } from './GitSettingsDialog'
import { GitBranchPanel } from './GitBranchPanel'
import { describeGitCommand } from './gitCommandDescription'
import { GitRemotePanel } from './GitRemotePanel'
import { GitHistoryPanel } from './GitHistoryPanel'
import { ChangeTree, DiffView } from './GitChanges'
import { buildChangeTree, changeStatusLabel } from './gitChangeTree'
import type { GitAction, GitActionInput } from '../store/workspaceApi'
import {
  useGitDiffQuery,
  useInitializeGitMutation,
  useGitWorkspaceActionMutation,
  useGitWorkspaceQuery
} from '../store/workspaceApi'

type Project = { name: string; path: string }
type Tab = 'commit' | 'history' | 'tag' | 'branch' | 'remote'
type Action = GitAction
type Pending = Omit<GitActionInput, 'root'> | null

const tabItems: Array<{
  id: Tab
  label: string
  icon: typeof GitCommitHorizontal
}> = [
  { id: 'commit', label: '提交', icon: GitCommitHorizontal },
  { id: 'history', label: '记录', icon: History },
  { id: 'tag', label: '标签', icon: Tags },
  { id: 'branch', label: '分支', icon: GitBranch },
  { id: 'remote', label: '远程', icon: Network }
]

const actionCopy: Record<
  Action,
  { title: string; confirm: string; destructive?: boolean }
> = {
  'fetch': { title: '获取远程更新', confirm: '确认获取' },
  'pull': { title: '更新当前分支', confirm: '确认更新' },
  'push': { title: '推送当前分支', confirm: '确认推送' },
  'identity-set': { title: '保存当前仓库提交身份', confirm: '确认保存' },
  'commit': { title: '提交已暂存内容', confirm: '确认提交' },
  'stage': { title: '暂存所选文件', confirm: '确认暂存' },
  'unstage': { title: '取消所选文件暂存', confirm: '确认取消暂存' },
  'restore': {
    title: '撤销未暂存改动',
    confirm: '确认撤销',
    destructive: true
  },
  'stash-save': { title: '临时保存全部改动', confirm: '确认保存' },
  'stash-apply': { title: '恢复临时保存', confirm: '确认恢复' },
  'stash-drop': {
    title: '删除临时保存记录',
    confirm: '确认删除',
    destructive: true
  },
  'merge': { title: '合并到当前分支', confirm: '确认合并' },
  'merge-abort': { title: '中止当前合并', confirm: '确认中止' },
  'branch-create': { title: '创建并切换分支', confirm: '确认创建' },
  'branch-switch': { title: '切换分支', confirm: '确认切换' },
  'branch-track': { title: '检出远程分支', confirm: '确认检出' },
  'branch-delete': {
    title: '删除本地分支',
    confirm: '确认删除',
    destructive: true
  },
  'tag-create': { title: '创建版本标签', confirm: '确认创建' },
  'tag-push': { title: '推送标签', confirm: '确认推送' },
  'tag-delete': {
    title: '删除本地标签',
    confirm: '确认删除',
    destructive: true
  },
  'remote-add': { title: '添加远程仓库', confirm: '确认添加' },
  'remote-set-url': { title: '修改远程地址', confirm: '确认修改' },
  'remote-remove': {
    title: '移除远程仓库',
    confirm: '确认移除',
    destructive: true
  }
}

export function RobotGitControl({
  project,
  onClose,
  onMinimize,
  minimized,
  zIndex,
  onActivate
}: {
  project: Project | null
  onClose: () => void
  onMinimize: () => void
  minimized: boolean
  zIndex: number
  onActivate: () => void
}) {
  const root = project?.path ?? ''
  const [tab, setTab] = useStoreState<Tab>('commit')
  const [historyOffset, setHistoryOffset] = useStoreState(0)
  const [inspectedRemote, setInspectedRemote] = useStoreState('')
  const [identityOpen, setIdentityOpen] = useStoreState(false)
  const [identityError, setIdentityError] = useStoreState('')
  const [selectedPaths, setSelectedPaths] = useStoreState<string[]>([])
  const [diffMode, setDiffMode] = useStoreState<'staged' | 'unstaged'>(
    'unstaged'
  )
  const gitQueryArgs = useMemo(
    () => ({
      root,
      view: tab,
      remote: tab === 'remote' ? inspectedRemote : '',
      offset: historyOffset
    }),
    [root, tab, inspectedRemote, historyOffset]
  )
  const {
    data,
    isLoading: isInitialLoading,
    isFetching,
    error,
    refetch
  } = useGitWorkspaceQuery(gitQueryArgs, { skip: !root })
  useEffect(() => {
    if (
      tab === 'remote' &&
      inspectedRemote &&
      data &&
      !data.remotes.some(item => item.name === inspectedRemote)
    )
      setInspectedRemote('')
  }, [tab, inspectedRemote, data, setInspectedRemote])
  const [diffPath, setDiffPath] = useState<string | null>(null)
  const diffQuery = useGitDiffQuery(
    { root, path: diffPath ?? '', view: diffMode },
    { skip: !root || !diffPath }
  )
  useEffect(() => {
    setDiffPath(null)
    setSelectedPaths([])
  }, [root, tab, setSelectedPaths])
  const [run, { isLoading }] = useGitWorkspaceActionMutation()
  const [initializeGit, { isLoading: isInitializing }] =
    useInitializeGitMutation()
  const [pending, setPending] = useStoreState<Pending>(null)
  const [output, setOutput] = useStoreState('')
  const [lastCommand, setLastCommand] = useStoreState('')
  const [commitMessage, setCommitMessage] = useStoreState('')
  const [branchName, setBranchName] = useStoreState('')
  const [tagName, setTagName] = useStoreState('')
  const [tagMessage, setTagMessage] = useStoreState('')
  const [authorName, setAuthorName] = useStoreState('')
  const [authorEmail, setAuthorEmail] = useStoreState('')
  const [repository, setRepository] = useStoreState('')
  const [initialMessage, setInitialMessage] = useStoreState(
    'chore: initialize project'
  )
  const commitTextareaRef = useRef<HTMLTextAreaElement>(null)
  // 提交说明框默认与按钮同高（单行），内容换行时才自动增高。
  useEffect(() => {
    const textarea = commitTextareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    const border = textarea.offsetHeight - textarea.clientHeight
    textarea.style.height = `${textarea.scrollHeight + border}px`
  }, [commitMessage])
  if (!project) return null

  const execute = async (request: NonNullable<Pending>) => {
    const object =
      request.action === 'fetch'
        ? request.remote
        : request.action === 'push' || request.action === 'pull'
          ? request.expectedBranch
          : request.value
    const destination =
      ['push', 'pull', 'tag-push'].includes(request.action) && request.remote
        ? ` → ${request.remote}`
        : ''
    setLastCommand(
      `${actionCopy[request.action].title}${object ? ` · ${object}` : ''}${destination}`
    )
    setOutput('正在执行…')
    try {
      const result = await run({ root, ...request }).unwrap()
      setOutput(result.output || 'Git 操作已完成。')
      if (request.action === 'commit') {
        setCommitMessage('')
        setDiffPath(null)
      }
      if (request.action === 'branch-create') setBranchName('')
      if (request.action === 'tag-create') {
        setTagName('')
        setTagMessage('')
      }
      if (
        [
          'commit',
          'stage',
          'unstage',
          'restore',
          'stash-save',
          'stash-apply',
          'branch-switch',
          'branch-create',
          'branch-track',
          'merge',
          'merge-abort'
        ].includes(request.action)
      )
        setSelectedPaths([])
      await refetch()
      return null
    } catch (reason) {
      const message = gitErrorMessage(reason, 'Git 操作未完成。')
      setOutput(message)
      await refetch()
      return message
    } finally {
      setPending(null)
    }
  }
  const request = (
    action: Action,
    value?: string,
    message?: string,
    paths?: string[],
    targetRemote?: string
  ) =>
    setPending({
      action,
      value,
      message,
      paths,
      remote: targetRemote ?? data?.remoteName,
      indexRevision: action === 'commit' ? data?.indexRevision : undefined,
      expectedBranch: data?.branch
    })
  const initialize = async () => {
    try {
      const result = await initializeGit({
        root,
        authorName: authorName.trim(),
        authorEmail: authorEmail.trim(),
        repository: repository.trim(),
        message: initialMessage.trim()
      }).unwrap()
      setOutput(result.output || 'Git 仓库已初始化。')
      await refetch()
    } catch (reason) {
      setOutput(gitErrorMessage(reason, 'Git 初始化未完成。'))
    }
  }
  const changes = data?.changes ?? []
  const stagedChanges = changes.filter(change => change.staged)
  const checkedChanges = changes.filter(change =>
    selectedPaths.includes(change.path)
  )
  const changeTree = buildChangeTree(changes)
  const remote = data?.remoteName || data?.remotes[0]?.name || ''
  const atRepositoryRoot = data?.atRepositoryRoot ?? false

  const confirm = pending ? actionCopy[pending.action] : null

  const commitPanel = (
    <section className="git-tab-panel git-commit-panel grid content-start gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="git-tab-heading git-commit-heading flex gap-3 border-b border-slate-200 pb-3">
        <textarea
          ref={commitTextareaRef}
          className="max-h-40 flex-1 min-h-9 min-w-0 resize-none overflow-y-auto rounded-md border border-slate-300 bg-white px-2.5 py-2 text-xs text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          aria-label="提交说明"
          value={commitMessage}
          onChange={event => setCommitMessage(event.target.value)}
          placeholder="例如：修复登录配置"
          rows={1}
        />
        <button
          className="primary-button shrink-0 max-w-60"
          disabled={
            isLoading ||
            isFetching ||
            !stagedChanges.length ||
            changes.some(change => change.conflict) ||
            !commitMessage.trim()
          }
          onClick={() =>
            request(
              'commit',
              undefined,
              commitMessage,
              stagedChanges.map(change => change.path)
            )
          }
          title={
            !stagedChanges.length
              ? '请先暂存要提交的文件'
              : !commitMessage.trim()
                ? '请先填写提交说明'
                : '仅提交已暂存内容'
          }
        >
          提交已暂存 ({stagedChanges.length})
        </button>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-slate-600">
          当前分支：{data?.branch || '分离 HEAD'}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <button
            className="secondary-button whitespace-nowrap"
            disabled={isLoading || isFetching || !remote || !data?.branch}
            onClick={() =>
              request('push', undefined, undefined, undefined, remote)
            }
            title={`推送当前分支已提交的内容到 ${remote}`}
          >
            推送当前分支
          </button>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <label className="flex items-center gap-1.5 text-slate-600">
          <input
            type="checkbox"
            aria-label="选择全部变更"
            checked={
              changes.length > 0 && checkedChanges.length === changes.length
            }
            disabled={isLoading || isFetching || !changes.length}
            onChange={event =>
              setSelectedPaths(
                event.target.checked ? changes.map(change => change.path) : []
              )
            }
          />
          已选 {checkedChanges.length} / {changes.length}
        </label>
        <button
          className="secondary-button"
          disabled={isLoading || isFetching || !checkedChanges.length}
          onClick={() =>
            request(
              'stage',
              undefined,
              undefined,
              checkedChanges.map(change => change.path)
            )
          }
        >
          暂存所选
        </button>
        <button
          className="secondary-button"
          disabled={
            isLoading ||
            isFetching ||
            !checkedChanges.some(change => change.staged)
          }
          onClick={() =>
            request(
              'unstage',
              undefined,
              undefined,
              checkedChanges
                .filter(change => change.staged)
                .map(change => change.path)
            )
          }
        >
          取消暂存
        </button>
        <button
          className="text-button text-red-700"
          disabled={
            isLoading ||
            isFetching ||
            !checkedChanges.length ||
            checkedChanges.some(
              change =>
                !change.unstaged || change.status === '??' || change.conflict
            )
          }
          onClick={() =>
            request(
              'restore',
              undefined,
              undefined,
              checkedChanges.map(change => change.path)
            )
          }
        >
          撤销未暂存改动
        </button>
      </div>
      {changes.some(change => change.conflict) && (
        <p
          role="alert"
          className="rounded bg-amber-50 p-2 text-xs text-amber-800"
        >
          存在冲突。请在文件编辑器处理冲突后暂存文件，或在“分支”页中止合并。
        </p>
      )}
      <div className="git-commit-workspace">
        <div className="git-commit-tree">
          {changes.length ? (
            <ChangeTree
              nodes={changeTree}
              selectedPath={diffPath}
              onSelect={path => {
                setDiffPath(path)
                setDiffMode(
                  changes.find(change => change.path === path)?.staged
                    ? 'staged'
                    : 'unstaged'
                )
              }}
              checkedPaths={selectedPaths}
              onCheck={(path, checked) =>
                setSelectedPaths(previous =>
                  checked
                    ? [...previous, path]
                    : previous.filter(item => item !== path)
                )
              }
            />
          ) : (
            <p className="grid min-h-24 place-items-center text-xs text-slate-500">
              工作区很干净，没有需要提交的改动。
            </p>
          )}
        </div>
        {diffPath && (
          <div className="git-diff-panel">
            <div className="git-diff-head">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                <code className="min-w-0 flex-1 truncate font-mono text-xs text-slate-700">
                  {diffPath}
                </code>
                {diffQuery.data?.status && (
                  <code className="shrink-0 rounded bg-brand-50 px-1 text-[10px] font-medium text-brand-700">
                    {changeStatusLabel(diffQuery.data.status)}
                  </code>
                )}
              </div>
              <button
                type="button"
                onClick={() => setDiffPath(null)}
                className="grid size-6 shrink-0 place-items-center rounded text-slate-400 hover:bg-slate-100 hover:text-slate-700"
                aria-label="关闭变更对比"
                title="关闭对比"
              >
                <X className="size-3.5" />
              </button>
            </div>
            <div className="flex gap-2 border-b border-slate-200 p-2">
              <button
                className="text-button"
                aria-pressed={diffMode === 'unstaged'}
                onClick={() => setDiffMode('unstaged')}
              >
                未暂存对比
              </button>
              <button
                className="text-button"
                aria-pressed={diffMode === 'staged'}
                onClick={() => setDiffMode('staged')}
              >
                已暂存对比
              </button>
            </div>
            {diffQuery.isFetching ? (
              <p className="git-diff-hint">正在读取变更对比…</p>
            ) : diffQuery.isError || !diffQuery.data ? (
              <p className="git-diff-hint">无法读取变更对比。</p>
            ) : diffQuery.data.missing || diffQuery.data.binary ? (
              <p className="git-diff-hint">{diffQuery.data.diff}</p>
            ) : diffQuery.data.diff ? (
              <>
                <DiffView
                  diff={diffQuery.data.diff}
                  untracked={diffQuery.data.untracked}
                />
                {diffQuery.data.truncated && (
                  <p className="git-diff-hint">diff 过长，已截断显示。</p>
                )}
              </>
            ) : (
              <p className="git-diff-hint">
                当前对比没有差异，可切换暂存视图或刷新状态。
              </p>
            )}
          </div>
        )}
      </div>
      <section className="grid gap-2 border-t border-slate-200 pt-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <strong className="text-xs text-slate-700">临时保存</strong>
          <button
            className="secondary-button"
            disabled={
              isLoading ||
              isFetching ||
              !atRepositoryRoot ||
              !changes.length ||
              changes.some(change => change.conflict)
            }
            onClick={() =>
              request(
                'stash-save',
                undefined,
                commitMessage || 'ALemonX 临时保存'
              )
            }
          >
            临时保存全部改动
          </button>
        </div>
        <p className="text-xs text-slate-500">
          保存包含新增文件和暂存内容。恢复后保留记录，确认无误后可删除。
          {!atRepositoryRoot && '请在仓库根目录使用临时保存。'}
        </p>
        {data?.stashes?.map(stash => (
          <div
            key={stash.sha}
            className="flex flex-wrap items-center gap-2 text-xs"
          >
            <span
              className="min-w-0 flex-1 truncate text-slate-700"
              title={stash.subject}
            >
              {stash.ref} · {stash.subject}
            </span>
            <button
              className="text-button"
              disabled={
                isLoading ||
                isFetching ||
                !atRepositoryRoot ||
                changes.length > 0
              }
              onClick={() => request('stash-apply', stash.sha)}
            >
              恢复
            </button>
            <button
              className="text-button text-red-700"
              disabled={isLoading || isFetching || !atRepositoryRoot}
              onClick={() => request('stash-drop', stash.sha)}
            >
              删除记录
            </button>
          </div>
        ))}
      </section>
    </section>
  )

  const historyPanel = (
    <GitHistoryPanel
      root={root}
      commits={data?.commits ?? []}
      offset={historyOffset}
      hasMore={data?.historyHasMore ?? false}
      busy={isFetching || isLoading}
      onPage={setHistoryOffset}
    />
  )

  const tagPanel = (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-end justify-between gap-3 border-b border-slate-200 pb-3">
        <div className="grid gap-1">
          <strong className="text-sm font-semibold text-slate-800">
            版本标签
          </strong>
          <span className="text-xs text-slate-500">
            创建带说明的本地标签；确认后可单独推送。
          </span>
        </div>
        <small className="text-xs text-slate-400">
          {data?.tags.length ?? 0} 个
        </small>
      </div>
      <div className="git-tag-form grid gap-2">
        <input
          className="h-9 rounded-md border border-slate-300 px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          aria-label="标签名"
          value={tagName}
          onChange={event => setTagName(event.target.value)}
          placeholder="标签名，例如 v1.2.3"
        />
        <input
          className="h-9 rounded-md border border-slate-300 px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          aria-label="标签说明"
          value={tagMessage}
          onChange={event => setTagMessage(event.target.value)}
          placeholder="标签说明，例如 release: v1.2.3"
        />
        <button
          className="secondary-button"
          disabled={
            isLoading || isFetching || !tagName.trim() || !tagMessage.trim()
          }
          onClick={() => request('tag-create', tagName, tagMessage)}
        >
          创建标签
        </button>
      </div>
      {data?.tags.length ? (
        <ul className="grid gap-1">
          {data.tags.map(item => (
            <li
              className="flex flex-wrap items-center gap-3 border-b border-slate-100 px-2 py-2 last:border-0"
              key={item.name}
            >
              <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-700">
                {item.name}
              </code>
              <span className="grid min-w-0 flex-1 gap-1">
                <strong className="text-xs font-semibold text-slate-700">
                  {item.subject || '无说明'}
                </strong>
                <small className="text-[11px] text-slate-400">
                  {item.createdAt || '本地标签'}
                </small>
              </span>
              <div className="flex items-center gap-1">
                <button
                  className="text-button"
                  disabled={isLoading || isFetching || !remote}
                  onClick={() => request('tag-push', item.name)}
                >
                  推送
                </button>
                <button
                  className="text-button text-red-700"
                  disabled={isLoading || isFetching}
                  onClick={() => request('tag-delete', item.name)}
                >
                  删除
                </button>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <p className="grid min-h-24 place-items-center text-xs text-slate-500">
          尚无标签。
        </p>
      )}
    </section>
  )

  const branchPanel = (
    <GitBranchPanel
      data={data}
      busy={isLoading || isFetching}
      branchName={branchName}
      onBranchNameChange={setBranchName}
      request={request}
    />
  )

  const remotePanel = (
    <GitRemotePanel
      data={data}
      busy={isLoading || isFetching}
      request={request}
      remoteAction={(action, name) => {
        setInspectedRemote(name)
        request(action, undefined, undefined, undefined, name)
      }}
      save={(action, value, message) =>
        execute({ action, value, message, expectedBranch: data?.branch })
      }
    />
  )

  const panel =
    tab === 'commit'
      ? commitPanel
      : tab === 'history'
        ? historyPanel
        : tab === 'tag'
          ? tagPanel
          : tab === 'branch'
            ? branchPanel
            : remotePanel
  return (
    <DesktopWindow
      id="git"
      open
      minimized={minimized}
      title={`${project.name} · Git`}
      subtitle={`${data?.branch ? `分支 ${data.branch}` : data?.repository ? '分离 HEAD' : '未初始化'} · ${data?.gitRoot || project.path}`}
      icon={
        <GitBranch className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      zIndex={zIndex}
      onActivate={onActivate}
      onClose={onClose}
      onMinimize={onMinimize}
      initialPosition={{ left: 96, top: 80 }}
      width={920}
      height={700}
      actions={
        <button
          className="icon-button size-8 p-0"
          disabled={isFetching || isLoading}
          onClick={() => {
            void refetch()
            if (diffPath) void diffQuery.refetch()
          }}
          aria-label="刷新 Git 状态"
          title="刷新 Git 状态"
        >
          <RefreshCw className="size-4" />
        </button>
      }
    >
      {isInitialLoading ? (
        <p className="grid min-h-40 place-items-center text-sm text-slate-500">
          正在读取 Git 状态…
        </p>
      ) : error ? (
        <div className="grid min-h-40 place-items-center gap-2 p-4 text-sm text-slate-500">
          <p role="alert">
            {gitErrorMessage(error, '无法读取 Git 状态，请确认目录可访问。')}
          </p>
          <button className="secondary-button" onClick={() => void refetch()}>
            重新读取
          </button>
        </div>
      ) : !data?.repository ? (
        <section className="grid min-h-56 content-center gap-4 p-6 text-center">
          <strong className="text-sm font-semibold text-slate-800">
            此机器人目录尚未初始化 Git
          </strong>
          <span className="text-xs text-slate-500">
            填写本项目的 Git 信息后初始化；不会修改你的全局 Git 身份。
          </span>
          <div className="mx-auto grid w-full max-w-md gap-3 text-left">
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              提交姓名
              <input
                className="h-9 rounded-md border border-slate-300 px-2.5 text-sm font-normal outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                autoFocus
                value={authorName}
                onChange={event => setAuthorName(event.target.value)}
                placeholder="你的姓名"
              />
            </label>
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              提交邮箱
              <input
                className="h-9 rounded-md border border-slate-300 px-2.5 text-sm font-normal outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                type="email"
                value={authorEmail}
                onChange={event => setAuthorEmail(event.target.value)}
                placeholder="name@example.com"
              />
            </label>
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              远程仓库（可选）
              <input
                className="h-9 rounded-md border border-slate-300 px-2.5 text-sm font-normal outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                value={repository}
                onChange={event => setRepository(event.target.value)}
                placeholder="https://github.com/owner/repo.git"
              />
            </label>
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              首个提交说明
              <input
                className="h-9 rounded-md border border-slate-300 px-2.5 text-sm font-normal outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                value={initialMessage}
                onChange={event => setInitialMessage(event.target.value)}
              />
            </label>
            <button
              className="primary-button justify-self-center"
              disabled={
                isInitializing || !authorName.trim() || !authorEmail.trim()
              }
              onClick={() => void initialize()}
            >
              {isInitializing ? '正在初始化…' : '填写 Git 信息并初始化'}
            </button>
          </div>
          {output && (
            <pre className="mx-auto w-full max-w-md overflow-auto rounded-lg bg-slate-950 p-3 text-left text-xs leading-5 text-slate-200">
              {output}
            </pre>
          )}
        </section>
      ) : (
        <div className="flex min-h-0 min-w-0 flex-col gap-3 overflow-hidden p-4">
          <div className="flex min-w-0 shrink-0 items-center gap-2">
            <Tabs
              className="min-h-10 min-w-0 flex-1 overflow-x-auto overflow-y-hidden [&_button]:shrink-0 [&_button]:whitespace-nowrap [&_svg]:shrink-0"
              ariaLabel="Git 功能"
              items={tabItems.map(item => {
                const Icon = item.icon
                return {
                  id: item.id,
                  icon: <Icon className="size-3.5" />,
                  label: item.label
                }
              })}
              onChange={setTab}
              value={tab}
              variant="pill"
            />
            <button
              className="secondary-button shrink-0 whitespace-nowrap"
              disabled={isLoading || isFetching}
              onClick={() => {
                setAuthorName(data.authorName ?? '')
                setAuthorEmail(data.authorEmail ?? '')
                setIdentityError('')
                setIdentityOpen(true)
              }}
            >
              提交身份
            </button>
          </div>
          <div
            key={tab}
            role="tabpanel"
            aria-label={tabItems.find(item => item.id === tab)?.label}
            className="min-h-0 min-w-0 flex-1 overflow-auto"
          >
            <div className="grid min-w-0 content-start gap-3">
              {!atRepositoryRoot && (
                <p className="break-words rounded bg-amber-50 p-2 text-xs text-amber-800">
                  当前项目位于父仓库内。文件暂存和撤销限制在本项目；分支切换、合并、同步和推送作用于整个仓库。仓库：
                  {data.gitRoot}
                </p>
              )}
              {panel}
              {output && (
                <section aria-label="执行结果" className="grid min-w-0 gap-1">
                  <strong className="text-xs text-slate-600">
                    上次操作：{lastCommand || 'Git 初始化'}
                  </strong>
                  <pre className="overflow-auto rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-200">
                    {output}
                  </pre>
                </section>
              )}
            </div>
          </div>
        </div>
      )}
      <GitSettingsDialog
        editor={identityOpen ? 'identity' : null}
        busy={isLoading || isFetching}
        error={identityError}
        name={authorName}
        email={authorEmail}
        url=""
        onNameChange={setAuthorName}
        onEmailChange={setAuthorEmail}
        onURLChange={() => {}}
        onClose={() => setIdentityOpen(false)}
        onSubmit={() => {
          void (async () => {
            const error = await execute({
              action: 'identity-set',
              value: authorName.trim(),
              message: authorEmail.trim()
            })
            if (error) setIdentityError(error)
            else setIdentityOpen(false)
          })()
        }}
      />
      <ConfirmDialog
        open={pending !== null}
        title={confirm?.title || '确认 Git 操作'}
        subtitle={
          confirm?.destructive
            ? '此操作会修改本地 Git 历史或配置，无法自动恢复。'
            : `当前仓库：${data?.gitRoot || project.path}`
        }
        message={pending ? describeGitCommand(pending, data) : ''}
        confirmLabel={confirm?.confirm || '确认'}
        busy={isLoading}
        destructive={confirm?.destructive}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (pending) void execute(pending)
        }}
      >
        {pending &&
          ['push', 'tag-push', 'pull'].includes(pending.action) &&
          (data?.remotes.length ?? 0) > 1 &&
          (pending.action !== 'pull' || !data?.upstream) && (
            <label className="grid gap-1 text-xs text-slate-600">
              目标仓库
              <select
                aria-label="目标仓库"
                className="h-9 rounded-md border border-slate-300 bg-white px-2"
                value={pending.remote ?? ''}
                disabled={isLoading}
                onChange={event =>
                  setPending(previous =>
                    previous
                      ? { ...previous, remote: event.target.value }
                      : previous
                  )
                }
              >
                {data?.remotes.map(item => (
                  <option key={item.name} value={item.name}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
      </ConfirmDialog>
    </DesktopWindow>
  )
}
