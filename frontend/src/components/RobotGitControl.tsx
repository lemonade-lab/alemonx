import { createPortal } from 'react-dom'
import { RefreshButton } from './RefreshButton'
import { useNavigationState } from '../hooks/useNavigationState'
import { useStoreState } from '../store/guideStore'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import {
  Archive,
  ArrowUpDown,
  ChevronDown,
  FilePenLine,
  GitBranch,
  GitCommitHorizontal,
  History,
  RefreshCw,
  Tags,
  UserRound,
  X
} from 'lucide-react'
import { Button } from './Button'
import { WorkbenchTools } from './WorkbenchTools'
import { Modal } from './Modal'
import { ConfirmDialog } from './ConfirmDialog'
import { Tabs } from './Tabs'
import { gitErrorMessage } from './gitFeedback'
import { GitSettingsDialog } from './GitSettingsDialog'
import { GitBranchPanel } from './GitBranchPanel'
import { describeGitCommand } from './gitCommandDescription'
import { GitRemotePanel } from './GitRemotePanel'
import { GitCommitDetail, GitHistoryPanel } from './GitHistoryPanel'
import { DiffView } from './GitChanges'
import { GitChangeList, type GitDiffMode } from './GitChangeList'
import { GitDiffPane } from './GitDiffPane'
import type { GitAction, GitActionInput } from '../store/workspaceApi'
import {
  useGitDiffQuery,
  useInitializeGitMutation,
  useGitWorkspaceActionMutation,
  useGitWorkspaceQuery
} from '../store/workspaceApi'

type Project = { name: string; path: string }
type Tab = 'commit' | 'history' | 'tag'
type Overlay = 'branch-manage' | 'remote' | 'stash'
type Action = GitAction
type Pending = Omit<GitActionInput, 'root'> | null

const tabItems = [
  { id: 'commit', label: '变更', icon: <GitCommitHorizontal className="size-3.5" /> },
  { id: 'history', label: '历史', icon: <History className="size-3.5" /> }
] as const

const historyItems = [
  { id: 'history', label: '提交记录' },
  { id: 'tag', label: '版本标签', icon: <Tags className="size-3.5" /> }
] as const

const overlayTitles: Record<Overlay, string> = {
  'branch-manage': '管理分支',
  remote: '远程同步',
  stash: '临时保存'
}

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

function GitBranchOptions({ root, busy, onSelect, onManage }: {
  root: string
  busy: boolean
  onSelect: (branch: string, currentBranch: string | undefined) => void
  onManage: () => void
}) {
  const { currentData, isFetching, error, refetch } = useGitWorkspaceQuery({
    root, view: 'branch', remote: '', offset: 0
  })
  return (
    <>
      <div className="flex min-h-0 flex-col gap-0.5 overflow-y-auto overscroll-contain">
        {error ? (
          <div className="px-2 py-1 text-xs" role="alert">
            {gitErrorMessage(error, '无法读取 Git 分支。')}
            <RefreshButton label="重新读取" onClick={() => void refetch()} />
          </div>
        ) : !currentData ? (
          <p role="status" className="px-2 py-1 text-xs">正在读取分支…</p>
        ) : currentData.branches.length === 0 ? (
          <p className="px-2 py-1 text-xs text-(--theme-text-muted)">暂无本地分支</p>
        ) : currentData.branches.map(item => (
          <Button
            variant="ghost"
            key={item.name}
            className="workbench-action-item h-auto! min-w-0 shrink-0 leading-snug! aria-current:bg-(--theme-accent-soft)! aria-current:text-(--theme-accent-text)! aria-current:opacity-100!"
            disabled={busy || isFetching || item.current}
            aria-current={item.current ? 'true' : undefined}
            onClick={() => onSelect(item.name, currentData.branch)}
          >
            <GitBranch className="size-4 shrink-0" aria-hidden="true" />
            <span className="min-w-0 flex-1 whitespace-normal [overflow-wrap:anywhere]">{item.name}</span>
            {item.current && <span className="shrink-0 text-xs">当前分支</span>}
          </Button>
        ))}
      </div>
      <Button variant="ghost" className="workbench-action-item shrink-0 bg-(--theme-surface-raised)! hover:bg-(--theme-surface-hover)!" onClick={onManage}>
        <GitBranch className="size-4 shrink-0" aria-hidden="true" />管理分支
      </Button>
    </>
  )
}

export function RobotGitControl({ project, projects, onSelectProject, onOpenFile, diffTarget, diffTabsTarget }: {
  project: Project
  projects?: Array<Project & { id: string }>
  onSelectProject?: (id: string) => void
  onOpenFile?: (path: string) => void
  diffTarget?: HTMLElement | null
  diffTabsTarget?: HTMLElement | null
}) {
  const root = project.path
  const tabsID = useId()
  const [tab, setTab] = useNavigationState<Tab>('git.tab', [root], 'commit', ['commit', 'history', 'tag'])
  const [overlay, setOverlay] = useStoreState<Overlay | null>(null)
  const [selecting, setSelecting] = useStoreState(false)
  const [outputFailed, setOutputFailed] = useStoreState(false)
  const branchOverlay = overlay === 'branch-manage'
  const view: Tab | 'branch' | 'remote' = branchOverlay
    ? 'branch'
    : overlay === 'remote' ? 'remote' : overlay === 'stash' ? 'commit' : tab
  const [historyOffset, setHistoryOffset] = useStoreState(0)
  const [inspectedRemote, setInspectedRemote] = useStoreState('')
  const [identityOpen, setIdentityOpen] = useStoreState(false)
  const [identityError, setIdentityError] = useStoreState('')
  const [selectedPaths, setSelectedPaths] = useStoreState<string[]>([])
  const [diffMode, setDiffMode] = useStoreState<GitDiffMode>(
    'unstaged'
  )
  const gitQueryArgs = useMemo(
    () => ({
      root,
      view,
      remote: view === 'remote' ? inspectedRemote : '',
      offset: view === 'history' ? historyOffset : 0
    }),
    [root, view, inspectedRemote, historyOffset]
  )
  const {
    data,
    currentData,
    isLoading: isInitialLoading,
    isFetching,
    error,
    refetch
  } = useGitWorkspaceQuery(gitQueryArgs, { skip: !root })
  const changesQuery = useGitWorkspaceQuery(
    { root, view: 'commit', remote: '', offset: 0 },
    { skip: !root || view === 'commit' }
  )
  const workingData = view === 'commit' ? currentData : changesQuery.currentData
  const context = workingData ?? data
  useEffect(() => {
    if (
      view === 'remote' &&
      inspectedRemote &&
      currentData &&
      !currentData.remotes.some(item => item.name === inspectedRemote)
    )
      setInspectedRemote('')
  }, [view, inspectedRemote, currentData, setInspectedRemote])
  const [diffPath, setDiffPath] = useState<string | null>(null)
  const [diffOpen, setDiffOpen] = useStoreState(false)
  const [selectedCommit, setSelectedCommit] = useStoreState<string | null>(null)
  const diffTrigger = useRef<HTMLElement | null>(null)
  const changesBody = useRef<HTMLDivElement>(null)
  const scrollPosition = useRef(0)
  const inlineDiff = !diffTarget && (tab === 'commit' ? diffOpen && Boolean(diffPath) : tab === 'history' && Boolean(selectedCommit))

  function rememberDetailEntrance() {
    diffTrigger.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    scrollPosition.current = changesBody.current?.scrollTop ?? 0
  }

  function openDiff(path: string, mode: GitDiffMode) {
    rememberDetailEntrance()
    setDiffPath(path)
    setDiffMode(mode)
    setDiffOpen(true)
  }

  function closeDiff() {
    setDiffOpen(false)
    setSelectedCommit(null)
    window.requestAnimationFrame(() => {
      if (changesBody.current) changesBody.current.scrollTop = scrollPosition.current
      diffTrigger.current?.focus({ preventScroll: true })
    })
  }
  const diffQuery = useGitDiffQuery(
    { root, path: diffPath ?? '', view: diffMode },
    { skip: !root || !diffPath || !diffOpen || tab !== 'commit' }
  )
  const selectTab = (next: Tab) => {
    setTab(next)
    setDiffOpen(false)
    setSelectedCommit(null)
    setSelectedPaths([])
    setSelecting(false)
    if (changesBody.current) changesBody.current.scrollTop = 0
  }
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
  useEffect(() => {
    const textarea = commitTextareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    const border = textarea.offsetHeight - textarea.clientHeight
    textarea.style.height = `${textarea.scrollHeight + border}px`
  }, [commitMessage])
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
    setOutputFailed(false)
    setOutput('正在执行…')
    try {
      const result = await run({ root, ...request }).unwrap()
      setOutput(result.output || 'Git 操作已完成。')
      if (request.action === 'commit') {
        setCommitMessage('')
        setDiffPath(null)
        setDiffOpen(false)
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
      setOutputFailed(true)
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
      setOutputFailed(false)
      setOutput(result.output || 'Git 仓库已初始化。')
      await refetch()
    } catch (reason) {
      setOutputFailed(true)
      setOutput(gitErrorMessage(reason, 'Git 初始化未完成。'))
    }
  }
  const changes = workingData?.changes ?? []
  const stagedChanges = changes.filter(change => change.staged)
  const checkedChanges = changes.filter(change =>
    selectedPaths.includes(change.path)
  )
  const remote = data?.remoteName || data?.remotes[0]?.name || ''
  const atRepositoryRoot = context?.atRepositoryRoot ?? false
  const conflictChanges = changes.filter(change => change.conflict)
  const unstagedChanges = changes.filter(change => change.unstaged && !change.conflict)
  const eligibleToStage = checkedChanges.filter(change => change.unstaged)
  const eligibleToUnstage = checkedChanges.filter(change => change.staged)
  const gitBusy = isLoading || isFetching
  const needsIdentity = Boolean(workingData && (!workingData.authorName || !workingData.authorEmail))
  const toggleStage = (action: 'stage' | 'unstage', path: string) => {
    if (gitBusy) return
    void execute({ action, paths: [path], expectedBranch: workingData?.branch })
  }
  function checkFile(path: string, checked: boolean) {
    setSelectedPaths(previous => checked ? [...previous, path] : previous.filter(item => item !== path))
  }
  function openIdentity() {
    setAuthorName(context?.authorName ?? '')
    setAuthorEmail(context?.authorEmail ?? '')
    setIdentityError('')
    setOverlay(null)
    setIdentityOpen(true)
  }

  const confirm = pending ? actionCopy[pending.action] : null

  const commitPanel = (
    <section className="grid content-start gap-4">
      <div className="grid gap-2">
        <textarea
          ref={commitTextareaRef}
          className="max-h-40 min-h-12 w-full min-w-0 resize-none rounded-md border-0 bg-(--theme-surface-input) px-3 py-2 text-sm"
          aria-label="提交说明"
          value={commitMessage}
          onChange={event => setCommitMessage(event.target.value)}
          placeholder="例如：修复登录配置"
          rows={2}
        />
        <Button
          variant="primary"
          className="w-full gap-1.5"
          disabled={gitBusy || needsIdentity || !stagedChanges.length || conflictChanges.length > 0 || !commitMessage.trim()}
          onClick={() => request('commit', undefined, commitMessage, stagedChanges.map(change => change.path))}
        >
          {stagedChanges.length ? `提交 ${stagedChanges.length} 个文件` : '提交'}
        </Button>
        {needsIdentity && (
          <div className="flex items-center justify-between gap-2 text-xs text-(--theme-text-muted)">
            尚未设置提交身份
            <Button variant="ghost" size="sm" onClick={openIdentity}>设置身份</Button>
          </div>
        )}
      </div>
      {conflictChanges.length > 0 && (
        <div className="grid gap-2 rounded-md bg-(--theme-warning-soft) p-2 text-xs text-(--theme-warning-text)">
          <span role="alert">有 {conflictChanges.length} 个文件存在冲突。请先编辑并暂存冲突文件。</span>
          {context?.mergeInProgress && (
            <Button variant="ghost" disabled={gitBusy} onClick={() => request('merge-abort', context.branch)}>中止合并</Button>
          )}
        </div>
      )}
      {changes.length ? (
        <>
          {selecting && (
            <div className="flex flex-wrap items-center gap-2 text-xs">
                <label className="flex items-center gap-1.5">
                  <input
                    type="checkbox"
                    aria-label="选择全部变更"
                    checked={checkedChanges.length === changes.length}
                    onChange={event => setSelectedPaths(event.target.checked ? changes.map(change => change.path) : [])}
                  />
                  已选 {checkedChanges.length}
                </label>
                <Button variant="ghost" size="sm" disabled={gitBusy || !eligibleToStage.length} onClick={() => request('stage', undefined, undefined, eligibleToStage.map(change => change.path))}>暂存所选</Button>
                <Button variant="ghost" size="sm" disabled={gitBusy || !eligibleToUnstage.length} onClick={() => request('unstage', undefined, undefined, eligibleToUnstage.map(change => change.path))}>取消暂存</Button>
                <Button variant="danger" size="sm" disabled={gitBusy || !checkedChanges.length || checkedChanges.some(change => !change.unstaged || change.status === '??' || change.conflict)} onClick={() => request('restore', undefined, undefined, checkedChanges.map(change => change.path))}>
                  撤销未暂存改动
                </Button>
            </div>
          )}
          {conflictChanges.length > 0 && (
            <GitChangeList label="冲突" changes={conflictChanges} mode="unstaged" selection={{ path: diffPath, mode: diffMode }} checkedPaths={selectedPaths} busy={gitBusy} onSelect={openDiff} onCheck={selecting ? checkFile : undefined} onToggle={path => request('stage', undefined, undefined, [path])} />
          )}
          <GitChangeList
            label="已暂存"
            changes={stagedChanges.filter(change => !change.conflict)}
            mode="staged"
            selection={{ path: diffPath, mode: diffMode }}
            checkedPaths={selectedPaths}
            busy={gitBusy}
            onSelect={openDiff}
            onCheck={selecting ? checkFile : undefined}
            onToggle={path => toggleStage('unstage', path)}
            actions={
              <Button
                variant="ghost"
                size="sm"
                className="bg-transparent! hover:bg-(--theme-surface-hover)!"
                aria-pressed={selecting}
                onClick={() => {
                  setSelecting(!selecting)
                  setSelectedPaths([])
                }}
              >
                {selecting ? '退出选择' : '批量操作'}
              </Button>
            }
          />
          <GitChangeList label="未暂存" changes={unstagedChanges} mode="unstaged" selection={{ path: diffPath, mode: diffMode }} checkedPaths={selectedPaths} busy={gitBusy} onSelect={openDiff} onCheck={selecting ? checkFile : undefined} onToggle={path => toggleStage('stage', path)} />
        </>
      ) : (
        <p className="m-0 px-1 py-2 text-xs text-(--theme-text-muted)">工作区没有需要提交的改动。</p>
      )}
    </section>
  )

  const diffPanel = diffPath && diffOpen && tab === 'commit' && (
    <GitDiffPane
      title="文件差异"
      path={diffPath}
      inline={inlineDiff}
      onClose={closeDiff}
      actions={
        <div className="flex flex-wrap gap-1">
          <Button variant="ghost" size="sm" className="px-1.5! aria-pressed:bg-(--theme-accent-soft)! aria-pressed:text-(--theme-accent-text)!" aria-pressed={diffMode === 'unstaged'} onClick={() => setDiffMode('unstaged')}>未暂存差异</Button>
          <Button variant="ghost" size="sm" className="px-1.5! aria-pressed:bg-(--theme-accent-soft)! aria-pressed:text-(--theme-accent-text)!" aria-pressed={diffMode === 'staged'} onClick={() => setDiffMode('staged')}>已暂存差异</Button>
          {onOpenFile && (
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto gap-1.5 px-1.5!"
              disabled={diffQuery.isFetching || !diffQuery.currentData || diffQuery.currentData.missing || diffQuery.currentData.binary}
              onClick={() => {
                setDiffOpen(false)
                onOpenFile(diffPath)
              }}
            >
              <FilePenLine className="size-3.5" aria-hidden="true" /> 打开文件
            </Button>
          )}
        </div>
      }
    >
      {diffQuery.isFetching ? (
        <p className="git-diff-hint">正在读取变更对比…</p>
      ) : diffQuery.isError || !diffQuery.currentData ? (
        <div className="git-diff-hint">
          <p role="alert">无法读取变更对比。</p>
          <RefreshButton label="重新读取" onClick={() => void diffQuery.refetch()} />
        </div>
      ) : diffQuery.currentData.missing || diffQuery.currentData.binary ? (
        <p className="git-diff-hint">{diffQuery.currentData.diff}</p>
      ) : diffQuery.currentData.diff ? (
        <>
          <DiffView diff={diffQuery.currentData.diff} untracked={diffQuery.currentData.untracked} />
          {diffQuery.currentData.truncated && <p className="git-diff-hint">差异过长，已截断显示。</p>}
        </>
      ) : (
        <p className="git-diff-hint">当前没有差异。</p>
      )}
    </GitDiffPane>
  )

  const stashPanel = (
      <section className="grid gap-2">
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
        {workingData?.stashes?.map(stash => (
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
  )

  const historyPanel = (
    <GitHistoryPanel
      commits={data?.commits ?? []}
      offset={historyOffset}
      hasMore={data?.historyHasMore ?? false}
      busy={isFetching || isLoading}
      onPage={offset => {
        closeDiff()
        setHistoryOffset(offset)
      }}
      sha={selectedCommit}
      onSelect={sha => {
        rememberDetailEntrance()
        setSelectedCommit(sha)
      }}
    />
  )

  const tagPanel = (
    <section className="git-tag-panel grid min-w-0 gap-3">
      <div className="flex items-end justify-between gap-3 pb-1">
        <div className="grid gap-1">
          <strong className="text-sm font-semibold text-slate-800">
            版本标签
          </strong>
          <span className="text-xs text-slate-500">
            创建带说明的本地标签；确认后可单独推送。
          </span>
        </div>
        <small className="shrink-0 text-xs text-(--theme-text-muted)">
          {data?.tags.length ?? 0} 个
        </small>
      </div>
      <div className="git-tag-form grid gap-2">
        <input
          className="h-9 w-full min-w-0 rounded-md border-0 bg-(--theme-surface-input) px-2.5 text-xs"
          aria-label="标签名"
          value={tagName}
          onChange={event => setTagName(event.target.value)}
          placeholder="标签名，例如 v1.2.3"
        />
        <input
          className="h-9 w-full min-w-0 rounded-md border-0 bg-(--theme-surface-input) px-2.5 text-xs"
          aria-label="标签说明"
          value={tagMessage}
          onChange={event => setTagMessage(event.target.value)}
          placeholder="标签说明，例如 release: v1.2.3"
        />
        <button
          className="secondary-button whitespace-nowrap"
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
              className="flex min-w-0 flex-wrap items-center gap-2 rounded-md bg-(--theme-surface-input) px-2 py-2"
              key={item.name}
            >
              <code className="max-w-full rounded bg-(--theme-surface-raised) px-1.5 py-0.5 text-xs [overflow-wrap:anywhere]">
                {item.name}
              </code>
              <span className="grid min-w-0 flex-[1_1_6rem] gap-1">
                <strong className="text-xs font-semibold [overflow-wrap:anywhere]">
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

  const panel = tab === 'commit' ? commitPanel : tab === 'tag' ? tagPanel : historyPanel
  const primaryTab = tab === 'commit' ? 'commit' : 'history'
  const detail = tab === 'history' && selectedCommit
    ? <GitCommitDetail root={root} sha={selectedCommit} inline={inlineDiff} onClose={closeDiff} />
    : diffPanel
  const detailLabel = tab === 'history' && selectedCommit
    ? `提交 ${selectedCommit.slice(0, 8)}`
    : diffPath?.split('/').at(-1) || '文件差异'
  const hasRemote = Boolean(context?.remotes.length)
  const syncLabel = !hasRemote ? '未配置' : context?.upstream ? `↑${context.ahead} ↓${context.behind}` : '未关联'
  const refresh = () => {
    void refetch()
    if (view !== 'commit' && changesQuery.currentData) void changesQuery.refetch()
    if (diffPath && diffOpen && tab === 'commit') void diffQuery.refetch()
  }
  const operationStatus = output && (
    <div className="shrink-0 bg-(--theme-surface-input) px-3 py-1.5 text-xs">
      {outputFailed && <p role="alert" className="m-0 break-words text-(--theme-danger)">{output}</p>}
      {!outputFailed && (
        <details>
          <summary className="cursor-pointer truncate" title={lastCommand}>
            {isLoading ? '正在执行' : '操作结果'}{lastCommand ? ` · ${lastCommand}` : ''}
          </summary>
          <pre className="mt-2 max-h-28 overflow-auto whitespace-pre-wrap">{output}</pre>
        </details>
      )}
    </div>
  )
  return (
    <section aria-label="Git 管理" className="relative flex min-h-0 min-w-0 flex-1 flex-col bg-(--theme-surface-raised)">
      <header inert={inlineDiff} className="grid shrink-0 grid-cols-1 gap-2 px-3 pt-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0 flex-[1_1_5rem] text-sm font-semibold">
            <h2 className={projects && projects.length > 1 && onSelectProject ? 'sr-only' : 'truncate'} aria-label={`${project.name} · Git`} title={context?.gitRoot || project.path}>
              {project.name}
            </h2>
            {projects && projects.length > 1 && onSelectProject && (
              <WorkbenchTools
                label={`选择 Git 仓库，当前 ${project.name}`}
                menuLabel="选择 Git 仓库"
                menuClassName="overflow-y-auto border-0!"
                triggerView={{
                  className: 'flex min-h-8 max-w-full items-center gap-1 rounded-md px-1 text-left hover:bg-(--theme-surface-hover)',
                  title: context?.gitRoot || project.path,
                  content: <><span className="min-w-0 truncate">{project.name}</span><ChevronDown className="size-3.5 shrink-0" aria-hidden="true" /></>
                }}
                restoreFocusOnClose
              >
                {close => projects.map(item => (
                  <Button
                    variant="ghost"
                    key={item.id}
                    className="workbench-action-item"
                    aria-current={item.path === root ? 'true' : undefined}
                    onClick={() => {
                      close()
                      if (item.path !== root) onSelectProject(item.id)
                    }}
                  >
                    <GitBranch className="size-4 shrink-0" aria-hidden="true" />
                    <span className="grid min-w-0 flex-1 gap-0.5">
                      <strong className="truncate">{item.name}</strong>
                      <small className="truncate text-(--theme-text-muted)" title={item.path}>{item.path}</small>
                    </span>
                    {item.path === root && <span className="shrink-0 text-xs">当前</span>}
                  </Button>
                ))}
              </WorkbenchTools>
            )}
          </div>
          {context?.repository ? (
            <div className="flex min-w-0 max-w-full items-center gap-1">
              <WorkbenchTools
                label="切换 Git 分支"
                menuLabel="选择 Git 分支"
                menuClassName="flex! w-64! flex-col overflow-hidden border-0!"
                triggerView={{
                  className: 'secondary-button min-h-8 w-full max-w-34 gap-1.5 px-2!',
                  title: context.branch || '分离 HEAD',
                  content: <>
                    <GitBranch className="size-3.5 shrink-0" aria-hidden="true" />
                    <span className="truncate">{context.branch || '分离 HEAD'}</span>
                    <ChevronDown className="size-3 shrink-0" aria-hidden="true" />
                  </>
                }}
                restoreFocusOnClose
              >
                {close => (
                  <GitBranchOptions
                    root={root}
                    busy={gitBusy}
                    onSelect={(value, expectedBranch) => {
                      close()
                      void execute({ action: 'branch-switch', value, expectedBranch })
                    }}
                    onManage={() => {
                      close()
                      setOverlay('branch-manage')
                    }}
                  />
                )}
              </WorkbenchTools>
              <Button variant="secondary" size="sm" className="shrink-0 gap-1.5 px-2! whitespace-nowrap" aria-label={`同步状态 ${syncLabel}`} onClick={() => setOverlay('remote')}>
                <ArrowUpDown className="size-3.5" aria-hidden="true" />{syncLabel}
              </Button>
              <WorkbenchTools
                label="仓库更多操作"
                menuLabel="仓库操作菜单"
                align="end"
                menuClassName="overflow-y-auto border-0!"
                restoreFocusOnClose
              >
                {close => (
                  <>
                    <Button variant="ghost" className="workbench-action-item" disabled={gitBusy} onClick={() => { close(); refresh() }}>
                      <RefreshCw aria-hidden="true" />刷新 Git 状态
                    </Button>
                    <Button variant="ghost" className="workbench-action-item" onClick={() => { close(); openIdentity() }}>
                      <UserRound aria-hidden="true" />提交身份
                    </Button>
                    <Button variant="ghost" className="workbench-action-item" onClick={() => { close(); setOverlay('stash') }}>
                      <Archive aria-hidden="true" />临时保存
                    </Button>
                  </>
                )}
              </WorkbenchTools>
            </div>
          ) : (
            <RefreshButton label="刷新 Git 状态" disabled={gitBusy} onClick={refresh} />
          )}
        </div>
        {context?.repository && (
          <Tabs ariaLabel="Git 功能" idPrefix={tabsID} items={tabItems} onChange={selectTab} value={primaryTab} variant="pill" />
        )}
        {context?.repository && tab !== 'commit' && (
          <Tabs ariaLabel="历史内容" items={historyItems} onChange={selectTab} value={tab} variant="pill" />
        )}
      </header>
      {isInitialLoading ? (
        <p className="grid min-h-40 place-items-center text-sm text-slate-500">
          正在读取 Git 状态…
        </p>
      ) : error && !context?.repository ? (
        <div className="grid min-h-40 place-items-center gap-2 p-4 text-sm text-slate-500">
          <p role="alert">
            {gitErrorMessage(error, '无法读取 Git 状态，请确认目录可访问。')}
          </p>
          <RefreshButton label="重新读取" onClick={() => void refetch()} />
        </div>
      ) : !data?.repository ? (
        <section className="grid min-h-0 flex-1 content-start gap-4 overflow-auto p-4 text-center">
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
        <>
          <div
            ref={changesBody}
            inert={inlineDiff}
            id={`${tabsID}-panel-${primaryTab}`}
            role="tabpanel"
            aria-labelledby={`${tabsID}-tab-${primaryTab}`}
            className="relative min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain px-3 py-3"
          >
            {!atRepositoryRoot && context?.gitRoot && (
              <p className="mb-3 rounded bg-(--theme-warning-soft) p-2 text-xs break-words text-(--theme-warning-text)">
                当前项目位于父仓库内。文件暂存和撤销限制在本项目；分支和同步操作作用于整个仓库。
              </p>
            )}
            {!currentData && isFetching && !overlay ? (
              <p role="status" className="text-xs text-(--theme-text-muted)">正在读取 Git 状态…</p>
            ) : error && !overlay ? (
              <div className="grid gap-2 text-xs text-(--theme-text-muted)">
                <p role="alert">{gitErrorMessage(error, '无法读取 Git 状态。')}</p>
                <RefreshButton label="重新读取" onClick={refresh} />
              </div>
            ) : panel}
          </div>
          {!overlay && operationStatus}
          {detail && (diffTarget ? createPortal(detail, diffTarget) : detail)}
          {detail && diffTarget && diffTabsTarget && createPortal(
            <div className="flex max-w-40 shrink-0 items-center gap-1 rounded-md bg-(--theme-surface-active) pl-2" aria-current="page">
              <span className="min-w-0 truncate text-xs text-(--theme-accent-text)" title={detailLabel}>{detailLabel}</span>
              <Button variant="icon" className="shrink-0" aria-label="关闭差异标签" onClick={closeDiff}>
                <X className="size-3.5" aria-hidden="true" />
              </Button>
            </div>,
            diffTabsTarget
          )}
        </>
      )}
      <Modal open={overlay !== null} ariaLabel={overlay ? overlayTitles[overlay] : undefined} onClose={() => setOverlay(null)} className="p-3">
        <section className="modal-surface modal-surface-flush flex max-h-[75dvh] w-full max-w-xl min-w-0 flex-col overflow-hidden">
          <header className="flex shrink-0 items-center justify-between gap-2">
            <strong>{overlay && overlayTitles[overlay]}</strong>
            <Button variant="icon" aria-label="关闭仓库操作" onClick={() => setOverlay(null)}><X className="size-4" aria-hidden="true" /></Button>
          </header>
          <div className="grid min-h-0 min-w-0 gap-3 overflow-auto p-3">
            {overlay === 'branch-manage' && currentData && branchPanel}
            {overlay === 'remote' && currentData && (
              <>
                <div className="grid gap-1 rounded-lg bg-(--theme-surface-input) p-3">
                  <strong className="truncate text-sm" title={currentData.upstream || currentData.branch}>
                    {currentData.upstream || currentData.branch || '分离 HEAD'}
                  </strong>
                  <span className="text-xs text-(--theme-text-muted)">
                    {currentData.upstream ? `待推送 ${currentData.ahead} 个 · 待拉取 ${currentData.behind} 个` : '当前分支尚未关联上游'}
                  </span>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button disabled={gitBusy || !remote} onClick={() => request('fetch', undefined, undefined, undefined, remote)}>获取</Button>
                  <Button disabled={gitBusy || !remote || !currentData.branch || currentData.mergeInProgress} onClick={() => request('pull', currentData.branch, undefined, undefined, remote)}>拉取</Button>
                  <Button disabled={gitBusy || !remote || !currentData.branch} onClick={() => request('push', undefined, undefined, undefined, remote)}>推送</Button>
                </div>
                <details className="text-sm" open={!currentData.remotes.length}>
                  <summary className="cursor-pointer rounded-md px-1 py-2">管理远程</summary>
                  {remotePanel}
                </details>
              </>
            )}
            {(branchOverlay || overlay === 'remote') && !currentData && (
              error ? <p role="alert">{gitErrorMessage(error, '无法读取 Git 状态。')}</p> : <p role="status">正在读取 Git 状态…</p>
            )}
            {overlay === 'stash' && stashPanel}
          </div>
          {operationStatus}
        </section>
      </Modal>
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
    </section>
  )
}
