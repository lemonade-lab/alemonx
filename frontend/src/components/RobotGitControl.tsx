import { useStoreState } from '../store/guideStore'
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ChevronDown,
  CloudDownload,
  File,
  Folder,
  GitBranch,
  GitCommitHorizontal,
  History,
  Network,
  Plus,
  RefreshCw,
  Tags,
  Trash2,
  X
} from 'lucide-react'
import { ConfirmDialog } from './ConfirmDialog'
import { DesktopWindow } from './DesktopWindow'
import { Tabs } from './Tabs'
import {
  useGitDiffQuery,
  useInitializeGitMutation,
  useGitWorkspaceActionMutation,
  useGitWorkspaceQuery
} from '../store/workspaceApi'

type Project = { name: string; path: string }
type Tab = 'commit' | 'history' | 'tag' | 'branch' | 'remote'
type Action =
  | 'fetch'
  | 'pull'
  | 'push'
  | 'commit'
  | 'branch-create'
  | 'branch-switch'
  | 'branch-track'
  | 'branch-delete'
  | 'tag-create'
  | 'tag-push'
  | 'tag-delete'
  | 'remote-add'
  | 'remote-set-url'
  | 'remote-remove'
type Pending = { action: Action; value?: string; message?: string } | null
type Change = { status: string; path: string }
type ChangeTreeNode = {
  name: string
  path: string
  change?: Change
  children: Map<string, ChangeTreeNode>
}

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

const changeStatusLabel = (status: string) => {
  const codes = status.replace(/\s/g, '')
  if (codes === '??') return '新增'
  if (codes.includes('D')) return '删除'
  if (codes.includes('R')) return '重命名'
  if (codes.includes('A')) return '新增'
  if (codes.includes('M')) return '修改'
  return status || '变更'
}

const actionCopy: Record<
  Action,
  { title: string; confirm: string; destructive?: boolean }
> = {
  'fetch': { title: '检查远程更新', confirm: '确认检查' },
  'pull': { title: '同步远程更新', confirm: '确认同步' },
  'push': { title: '推送当前分支', confirm: '确认推送' },
  'commit': { title: '创建本地提交', confirm: '确认提交' },
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

function buildChangeTree(changes: Change[]) {
  const root: ChangeTreeNode = { name: '', path: '', children: new Map() }
  for (const change of changes) {
    const parts = change.path.replace(/\/$/, '').split('/').filter(Boolean)
    let parent = root
    parts.forEach((name, index) => {
      const path = parent.path ? `${parent.path}/${name}` : name
      const existing = parent.children.get(name) ?? {
        name,
        path,
        children: new Map<string, ChangeTreeNode>()
      }
      if (index === parts.length - 1) existing.change = change
      parent.children.set(name, existing)
      parent = existing
    })
  }
  return [...root.children.values()]
}

function ChangeTree({
  nodes,
  selectedPath,
  onSelect
}: {
  nodes: ChangeTreeNode[]
  selectedPath: string | null
  onSelect: (path: string) => void
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
                />
              </details>
            ) : (
              <button
                type="button"
                disabled={!node.change}
                onClick={() => node.change && onSelect(node.path)}
                title={
                  node.change
                    ? `查看 ${node.path} 的变更对比`
                    : undefined
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
                  {node.change ? changeStatusLabel(node.change.status) : '变更'}
                </code>
              </button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

function DiffView({ diff, untracked }: { diff: string; untracked: boolean }) {
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
  const gitQueryArgs = useMemo(() => ({ root, view: tab }), [root, tab])
  const {
    data,
    isLoading: isInitialLoading,
    isFetching,
    error,
    refetch
  } = useGitWorkspaceQuery(gitQueryArgs, { skip: !root })
  const [diffPath, setDiffPath] = useState<string | null>(null)
  const diffQuery = useGitDiffQuery(
    { root, path: diffPath ?? '' },
    { skip: !root || !diffPath }
  )
  useEffect(() => {
    setDiffPath(null)
  }, [tab])
  const [run, { isLoading }] = useGitWorkspaceActionMutation()
  const [initializeGit, { isLoading: isInitializing }] =
    useInitializeGitMutation()
  const fetchedBranchView = useRef(false)
  useEffect(() => {
    if (tab !== 'branch' || fetchedBranchView.current || !root) return
    fetchedBranchView.current = true
    void run({ root, action: 'fetch' })
      .then(() => refetch())
      .catch(() => {})
  }, [refetch, root, run, tab])
  const [pending, setPending] = useStoreState<Pending>(null)
  const [output, setOutput] = useStoreState('')
  const [commitMessage, setCommitMessage] = useStoreState('')
  const [branchName, setBranchName] = useStoreState('')
  const [tagName, setTagName] = useStoreState('')
  const [tagMessage, setTagMessage] = useStoreState('')
  const [remoteName, setRemoteName] = useStoreState('origin')
  const [remoteURL, setRemoteURL] = useStoreState('')
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
      await refetch()
    } catch (reason) {
      setOutput(reason instanceof Error ? reason.message : 'Git 操作未完成。')
    } finally {
      setPending(null)
    }
  }
  const request = (action: Action, value?: string, message?: string) =>
    setPending({ action, value, message })
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
      setOutput(reason instanceof Error ? reason.message : 'Git 初始化未完成。')
    }
  }
  const changes = data?.changes ?? []
  const changeTree = buildChangeTree(changes)
  const syncText = !data?.upstream
    ? '当前分支尚未关联远程分支'
    : data.remoteChecked && !data.remoteSynced
      ? `实时远程不一致 · 缓存领先 ${data.ahead} · 落后 ${data.behind}`
      : data.remoteChecked && data.remoteSynced
        ? `已与远程实时同步 · 领先 ${data.ahead} · 落后 ${data.behind}`
    : `领先 ${data.ahead} · 落后 ${data.behind}`
  const confirm = pending ? actionCopy[pending.action] : null

  const commitPanel = (
    <section className="git-tab-panel git-commit-panel grid content-start gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="git-tab-heading git-commit-heading flex gap-3 border-b border-slate-200 pb-3">
        <textarea
          ref={commitTextareaRef}
          className="max-h-40 flex-1 min-h-9 min-w-0 resize-none overflow-y-auto rounded-md border border-slate-300 bg-white px-2.5 py-2 text-xs text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          value={commitMessage}
          onChange={event => setCommitMessage(event.target.value)}
          placeholder="例如：修复登录配置"
          rows={1}
        />
        <button
          className="primary-button shrink-0 max-w-60"
          disabled={isLoading || !changes.length || !commitMessage.trim()}
          onClick={() => request('commit', undefined, commitMessage)}
          title={
            !changes.length
              ? '工作区没有变更可提交'
              : !commitMessage.trim()
                ? '请先填写提交说明'
                : '提交全部变更'
          }
        >
          提交全部变更
        </button>
      </div>
      <div className="git-commit-workspace">
        <div className="git-commit-tree">
          {changes.length ? (
            <ChangeTree
              nodes={changeTree}
              selectedPath={diffPath}
              onSelect={setDiffPath}
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
            {diffQuery.isLoading ? (
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
                该文件当前与 HEAD 没有差异（可能已提交或变更已撤销），请刷新变更列表后重试。
              </p>
            )}
          </div>
        )}
      </div>
    </section>
  )

  const historyPanel = (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex items-end justify-between gap-3 border-b border-slate-200 pb-3">
        <div className="grid gap-1">
          <strong className="text-sm font-semibold text-slate-800">
            最近提交记录
          </strong>
          <span className="text-xs text-slate-500">
            用于确认当前机器人目录的本地历史。
          </span>
        </div>
        <small className="text-xs text-slate-400">
          {data?.commits.length ?? 0} 条
        </small>
      </div>
      {data?.commits.length ? (
        <ul className="grid gap-1">
          {data.commits.map(item => (
            <li
              className="flex items-start gap-3 rounded-md border-b border-slate-100 px-2 py-2 last:border-0"
              key={item.sha}
            >
              <code
                className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600"
                title={item.sha}
              >
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
            </li>
          ))}
        </ul>
      ) : (
        <p className="grid min-h-24 place-items-center text-xs text-slate-500">
          还没有提交记录。在「提交」页填写说明后，就能创建第一条。
        </p>
      )}
    </section>
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
          value={tagName}
          onChange={event => setTagName(event.target.value)}
          placeholder="标签名，例如 v1.2.3"
        />
        <input
          className="h-9 rounded-md border border-slate-300 px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
          value={tagMessage}
          onChange={event => setTagMessage(event.target.value)}
          placeholder="标签说明，例如 release: v1.2.3"
        />
        <button
          className="secondary-button"
          disabled={isLoading || !tagName.trim() || !tagMessage.trim()}
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
                  disabled={isLoading}
                  onClick={() => request('tag-push', item.name)}
                >
                  推送
                </button>
                <button
                  className="text-button text-red-700"
                  disabled={isLoading}
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

  const remoteOnlyBranches = (data?.remoteBranches ?? []).filter(
    item => !(data?.branches ?? []).some(branch => branch.name === item.branch)
  )
  const branchPanel = (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <input
            className="h-9 min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
            value={branchName}
            onChange={event => setBranchName(event.target.value)}
            placeholder="新分支名称，例如 feat/login"
          />
          <button
            className="secondary-button shrink-0"
            disabled={isLoading || !branchName.trim()}
            onClick={() => request('branch-create', branchName)}
          >
            创建并切换
          </button>
        </div>
      </div>
      {data?.branches.length || remoteOnlyBranches.length ? (
        <ul className="grid gap-1">
          {(data?.branches ?? []).map(item => (
            <li
              className="flex flex-wrap items-center gap-3 border-b border-slate-100 px-2 py-2 last:border-0"
              key={item.name}
            >
              <code
                className={
                  item.current
                    ? 'rounded bg-brand-50 px-1.5 py-0.5 text-[10px] font-semibold text-brand-700'
                    : 'rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600'
                }
              >
                {item.name}
              </code>
              <span className="grid min-w-0 flex-1 gap-0.5">
                <strong className="text-xs font-semibold text-slate-700">
                  {item.current
                    ? '当前正在使用'
                    : item.upstream
                      ? '已同步到远程'
                      : '仅在本机，尚未推送'}
                </strong>
              </span>
              <div className="flex items-center gap-1">
                {!item.current && (
                  <button
                    className="text-button"
                    disabled={isLoading}
                    onClick={() => request('branch-switch', item.name)}
                  >
                    切换
                  </button>
                )}
                {!item.current && (
                  <button
                    className="text-button text-red-700"
                    disabled={isLoading}
                    onClick={() => request('branch-delete', item.name)}
                  >
                    删除
                  </button>
                )}
              </div>
            </li>
          ))}
          {remoteOnlyBranches.map(item => (
            <li
              className="flex flex-wrap items-center gap-3 border-b border-slate-100 px-2 py-2 last:border-0"
              key={item.name}
            >
              <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600">
                {item.branch}
              </code>
              <span className="grid min-w-0 flex-1 gap-0.5">
                <strong className="text-xs font-semibold text-slate-700">
                  远程 {item.remote} 上的分支，本机尚未打开
                </strong>
              </span>
              <button
                className="text-button"
                disabled={isLoading}
                onClick={() => request('branch-track', item.name)}
              >
                在本机打开
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="grid min-h-24 place-items-center text-xs text-slate-500">
          还没有分支。在顶部输入名称，创建你的第一个分支。
        </p>
      )}
    </section>
  )

  const remotePanel = (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="grid gap-3">
        <section>
          <div className="git-remote-form flex flex-wrap items-center gap-2">
            <input
              className="h-9 w-36 rounded-md border border-slate-300 bg-white px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
              value={remoteName}
              onChange={event => setRemoteName(event.target.value)}
              placeholder="名称，如 origin"
              aria-label="远程仓库名称"
            />
            <input
              className="h-9 min-w-0 flex-1 rounded-md border border-slate-300 bg-white px-2.5 text-xs outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
              value={remoteURL}
              onChange={event => setRemoteURL(event.target.value)}
              placeholder="仓库地址，如 git@github.com:org/repo.git"
              aria-label="远程仓库地址"
            />
            <div className="git-remote-actions flex shrink-0 gap-2">
              <button
                className="primary-button"
                disabled={isLoading || !remoteName.trim() || !remoteURL.trim()}
                onClick={() => request('remote-add', remoteName, remoteURL)}
              >
                <Plus className="mr-1 size-3.5" />
                添加
              </button>
              <button
                className="secondary-button"
                disabled={isLoading || !remoteName.trim() || !remoteURL.trim()}
                onClick={() => request('remote-set-url', remoteName, remoteURL)}
              >
                更新地址
              </button>
            </div>
          </div>
        </section>
        <section className="grid gap-2">
          <header className="git-remote-heading flex items-center justify-between gap-2">
            <strong className="text-sm font-semibold text-slate-700">
              已配置远程
            </strong>
            <div className="flex items-center gap-2">
              <span className="text-[11px] text-slate-400">{syncText}</span>
              <button
                className="secondary-button"
                disabled={isLoading || !data?.remotes.length}
                onClick={() => request('fetch')}
                title={
                  data?.remotes.length
                    ? '读取远程仓库的最新状态'
                    : '还没有配置远程仓库，无法拉取'
                }
              >
                <CloudDownload className="mr-1.5 size-3.5" />
                拉取远程
              </button>
              <button
                className="secondary-button"
                disabled={isLoading || !data?.upstream || !data.behind}
                onClick={() => request('pull')}
                title={
                  !data?.remotes.length
                    ? '还没有配置远程仓库，无法同步'
                    : !data?.upstream
                      ? '当前分支尚未关联远程分支，无法同步'
                      : !data.behind
                        ? '没有落后于远程，无需同步'
                        : '把远程的新提交合并到本机'
                }
              >
                <RefreshCw className="mr-1.5 size-3.5" />
                同步
              </button>
            </div>
          </header>
          {data?.remotes.length ? (
            <ul className="grid gap-1">
              {data.remotes.map(item => (
                <li
                  className="flex items-center gap-3 rounded-md border border-slate-100 bg-slate-50/50 px-3 py-2"
                  key={item.name}
                >
                  <code className="shrink-0 rounded bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-700">
                    {item.name}
                  </code>
                  <span className="grid min-w-0 flex-1 gap-0.5">
                    <strong
                      className="truncate text-xs font-medium text-slate-700"
                      title={item.url}
                    >
                      {item.url}
                    </strong>
                    <small className="text-[10px] text-slate-400">
                      {item.name === 'origin' ? '默认远程仓库' : '远程仓库'}
                    </small>
                  </span>
                  <button
                    className="text-button text-red-700"
                    disabled={isLoading}
                    onClick={() => request('remote-remove', item.name)}
                  >
                    <Trash2 className="mr-1 size-3.5" />
                    移除
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="grid min-h-20 place-items-center text-xs text-slate-500">
              还没有配置远程仓库。在上方填写名称和地址后添加。
            </p>
          )}
        </section>
      </div>
    </section>
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
      subtitle={data?.gitRoot || project.path}
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
          onClick={() => void refetch()}
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
        <p className="grid min-h-40 place-items-center text-sm text-slate-500">
          无法读取 Git 状态，请确认目录可访问。
        </p>
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
        <div className="git-workspace-content grid min-h-0 content-start gap-3 overflow-auto p-4">
          <Tabs
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
          {panel}
          {output && (
            <pre className="overflow-auto rounded-lg bg-slate-950 p-3 text-xs leading-5 text-slate-200">
              {output}
            </pre>
          )}
        </div>
      )}
      <ConfirmDialog
        open={pending !== null}
        title={confirm?.title || '确认 Git 操作'}
        subtitle={
          confirm?.destructive
            ? '此操作会修改本地 Git 历史或配置，无法自动恢复。'
            : '操作将只在当前机器人目录执行。'
        }
        message={
          pending?.action === 'commit'
            ? `将提交工作区全部 ${changes.length} 项变更：\n${changes
                .map(item => item.path)
                .slice(0, 5)
                .join(
                  '\n'
                )}${changes.length > 5 ? `\n… 共 ${changes.length} 项` : ''}\n\n说明：${pending.message}`
            : pending?.value
              ? `目标：${pending.value}${pending.message ? `\n说明：${pending.message}` : ''}`
              : pending?.message
                ? `说明：${pending.message}`
                : '请确认继续。'
        }
        confirmLabel={confirm?.confirm || '确认'}
        busy={isLoading}
        onCancel={() => setPending(null)}
        onConfirm={() => {
          if (pending) void execute(pending)
        }}
      />
    </DesktopWindow>
  )
}
