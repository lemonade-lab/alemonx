import { RefreshButton } from '../RefreshButton'
import { useCallback, useEffect, useLayoutEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import {
  FilePlus,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  X
} from 'lucide-react'
import cn from 'classnames'
import { useStoreState } from '../../store/guideStore'
import {
  useCreateWorkspaceProjectEntryMutation,
  useWorkspaceProjectDirectoryQuery
} from '../../store/workspaceApi'
import { Button } from '../Button'
import { ConfirmDialog } from '../ConfirmDialog'
import { Modal } from '../Modal'
import {
  ExplorerFileEditor,
  type ExplorerFileDraft
} from './ExplorerFileEditor'

type Project = { id: string; name: string; path: string }
type OpenFile = { root: string; path: string }
const nodeButton =
  'flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-(--theme-surface-hover) focus-visible:outline-2 focus-visible:outline-(--theme-accent)'

function sameFile(file: OpenFile, current?: OpenFile | null) {
  return file.root === current?.root && file.path === current?.path
}

function ProjectFolder({
  root,
  path = '',
  name,
  selected,
  defaultOpen = false,
  onOpen
}: {
  root: string
  path?: string
  name: string
  selected?: string
  defaultOpen?: boolean
  onOpen: (file: OpenFile) => void
}) {
  const [expanded, setExpanded] = useStoreState(defaultOpen)
  const { currentData, isFetching, isError, refetch } =
    useWorkspaceProjectDirectoryQuery(
      { root, directory: path },
      { skip: !expanded }
    )
  const entries = [...(currentData?.entries ?? [])].sort(
    (left, right) => Number(right.directory) - Number(left.directory)
  )
  const Icon = expanded ? FolderOpen : Folder

  return (
    <details
      open={expanded}
      onToggle={event => setExpanded(event.currentTarget.open)}
    >
      <summary
        className={cn(
          nodeButton,
          'cursor-pointer list-none',
          !path && 'font-semibold'
        )}
        title={path || root}
      >
        <Icon
          className="size-4 shrink-0 text-(--theme-accent-text)"
          aria-hidden="true"
        />
        <span className="truncate">{name}</span>
      </summary>
      <div className="ml-3 border-l border-(--theme-border-subtle) pl-2">
        {isError ? (
          <div className="px-2 py-1 text-sm" role="alert">
            无法读取目录。
            <RefreshButton label="重试" onClick={() => void refetch()} />
          </div>
        ) : entries.length ? (
          <ul className="m-0 list-none space-y-0.5 p-0">
            {entries.map(entry => (
              <li key={entry.path}>
                {entry.directory ? (
                  <ProjectFolder
                    root={root}
                    path={entry.path}
                    name={entry.name}
                    selected={selected}
                    onOpen={onOpen}
                  />
                ) : (
                  <button
                    type="button"
                    className={cn(
                      nodeButton,
                      selected === entry.path &&
                        'bg-(--theme-accent-soft) text-(--theme-accent-text)'
                    )}
                    aria-current={selected === entry.path ? 'true' : undefined}
                    title={entry.path}
                    onClick={() => onOpen({ root, path: entry.path })}
                  >
                    <FileText className="size-4 shrink-0" aria-hidden="true" />
                    <span className="truncate">{entry.name}</span>
                  </button>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p
            className="px-2 py-1 text-xs text-(--theme-text-muted)"
            role="status"
          >
            {isFetching ? '正在读取文件…' : '此目录为空'}
          </p>
        )}
      </div>
    </details>
  )
}

export function ResourceExplorer({
  projects,
  currentRoot,
  directoryTarget,
  directoryInDrawer,
  currentProjectDirectoryOnly = false,
  fileTabsTarget,
  fileRequest,
  onConsumeFileRequest,
  onOpenFile
}: {
  projects: Project[]
  currentRoot: string
  directoryTarget: HTMLDivElement | null
  directoryInDrawer: boolean
  currentProjectDirectoryOnly?: boolean
  fileTabsTarget: HTMLDivElement | null
  fileRequest: OpenFile | null
  onConsumeFileRequest: () => void
  onOpenFile: () => void
}) {
  const [openFiles, setOpenFiles] = useStoreState<OpenFile[]>([])
  const [selected, setSelected] = useStoreState<OpenFile | null>(null)
  const [drafts, setDrafts] = useStoreState<Record<string, ExplorerFileDraft>>(
    {}
  )
  const [pendingClose, setPendingClose] = useStoreState<OpenFile | null>(null)
  const [createTarget, setCreateTarget] = useStoreState<{
    project: Project
    directory: boolean
  } | null>(null)
  const [entryName, setEntryName] = useStoreState('')
  const [createError, setCreateError] = useStoreState('')
  const [createEntry, { isLoading: creating }] =
    useCreateWorkspaceProjectEntryMutation()
  const currentProject = projects.find(project => project.path === currentRoot)
  const directoryProjects = currentProjectDirectoryOnly && currentProject
    ? [currentProject]
    : projects
  const availableFiles = openFiles.filter(file =>
    projects.some(project => project.path === file.root)
  )
  const active =
    availableFiles.find(file => sameFile(file, selected)) ?? availableFiles[0]
  const activeProject = projects.find(project => project.path === active?.root)
  const activeKey = active ? JSON.stringify(active) : ''
  const tabViewport = useRef<HTMLElement>(null)
  const activeTab = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const scroller = tabViewport.current
    const tab = activeTab.current
    if (!scroller || !tab) return
    const reveal = () => {
      const viewport = scroller.getBoundingClientRect()
      const bounds = tab.getBoundingClientRect()
      if (bounds.left < viewport.left)
        scroller.scrollLeft += bounds.left - viewport.left
      else if (bounds.right > viewport.right)
        scroller.scrollLeft += bounds.right - viewport.right
    }
    reveal()
    const observer = new ResizeObserver(reveal)
    observer.observe(scroller)
    observer.observe(tab)
    return () => observer.disconnect()
  }, [activeKey, fileTabsTarget])

  const hasDrafts = Object.keys(drafts).length > 0

  useEffect(() => {
    if (!hasDrafts) return
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = true
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [hasDrafts])

  function updateDraft(file: OpenFile, draft?: ExplorerFileDraft) {
    const key = JSON.stringify(file)
    setDrafts(current => {
      if (draft) return { ...current, [key]: draft }
      const next = { ...current }
      delete next[key]
      return next
    })
  }

  function close(file: OpenFile) {
    setOpenFiles(files => files.filter(item => !sameFile(item, file)))
    updateDraft(file)
    setPendingClose(null)
  }

  const open = useCallback((file: OpenFile) => {
    setOpenFiles(files =>
      files.some(item => sameFile(item, file)) ? files : [...files, file]
    )
    setSelected(file)
    onOpenFile()
  }, [onOpenFile, setOpenFiles, setSelected])

  useEffect(() => {
    if (!fileRequest) return
    onConsumeFileRequest()
    if (projects.some(project => project.path === fileRequest.root))
      open(fileRequest)
  }, [fileRequest, onConsumeFileRequest, open, projects])

  function startCreate(directory: boolean) {
    if (!currentProject) return
    setEntryName('')
    setCreateError('')
    setCreateTarget({ project: currentProject, directory })
  }

  async function submitCreate() {
    const name = entryName.trim()
    if (!createTarget || !name || creating) return
    const { project, directory } = createTarget
    try {
      const entry = await createEntry({
        root: project.path,
        name,
        directory
      }).unwrap()
      setCreateTarget(null)
      if (!entry.directory) open({ root: project.path, path: entry.path })
    } catch (failure) {
      const response = failure as { data?: { error?: string } } | undefined
      setCreateError(response?.data?.error || '创建失败，请重试。')
    }
  }

  const createTitle = createTarget?.directory ? '新建目录' : '新建文件'

  return (
    <section
      className="flex h-full min-h-0 min-w-0 flex-col"
      aria-label="资源管理器"
    >
      {directoryTarget &&
        createPortal(
          <aside
            className="flex min-h-0 min-w-0 flex-1 flex-col bg-(--theme-surface-raised)"
            aria-label="机器人项目文件"
          >
            <header className="flex shrink-0 items-center justify-between gap-2 px-4 pt-3 pb-1">
              <h2 className="text-sm font-semibold">目录</h2>
              <div className="flex items-center gap-1">
                <Button
                  variant="icon"
                  aria-label="新建文件"
                  title="新建文件"
                  disabled={!currentProject}
                  onClick={() => startCreate(false)}
                >
                  <FilePlus className="size-4" aria-hidden="true" />
                </Button>
                <Button
                  variant="icon"
                  aria-label="新建目录"
                  title="新建目录"
                  disabled={!currentProject}
                  onClick={() => startCreate(true)}
                >
                  <FolderPlus className="size-4" aria-hidden="true" />
                </Button>
              </div>
            </header>
            <div
              className={cn(
                'px-2 pb-2',
                !directoryInDrawer && 'min-h-0 flex-1 overflow-auto overscroll-contain'
              )}
            >
              {directoryProjects.map(project => (
                <ProjectFolder
                  key={project.id}
                  root={project.path}
                  name={project.name}
                  defaultOpen={project.path === currentRoot}
                  selected={
                    active?.root === project.path ? active.path : undefined
                  }
                  onOpen={open}
                />
              ))}
            </div>
          </aside>,
          directoryTarget
        )}
      {createTarget && (
        <Modal
          open
          ariaLabel={createTitle}
          className="p-4"
          onClose={() => {
            if (!creating) setCreateTarget(null)
          }}
        >
          <form
            className="modal-surface grid w-full max-w-md gap-4"
            onSubmit={event => {
              event.preventDefault()
              void submitCreate()
            }}
          >
            <header className="grid gap-1">
              <h2 className="text-sm font-semibold">{createTitle}</h2>
              <p className="break-all text-xs text-(--theme-text-secondary)">
                创建位置：{createTarget.project.path}
              </p>
            </header>
            <label className="grid gap-1 text-sm">
              {createTarget.directory ? '目录名称' : '文件名称'}
              <input
                className="settings-input min-w-0"
                value={entryName}
                disabled={creating}
                onChange={event => {
                  setEntryName(event.target.value)
                  setCreateError('')
                }}
              />
            </label>
            {createError && (
              <p role="alert" className="text-sm text-(--theme-danger-text)">
                {createError}
              </p>
            )}
            <footer className="flex justify-end gap-2">
              <Button type="button" disabled={creating} onClick={() => setCreateTarget(null)}>
                取消
              </Button>
              <Button type="submit" variant="primary" loading={creating} disabled={!entryName.trim()}>
                创建
              </Button>
            </footer>
          </form>
        </Modal>
      )}
      {fileTabsTarget &&
        availableFiles.length > 0 &&
        createPortal(
          <nav
            ref={tabViewport}
            className="terminal-tab-viewport flex min-w-0 max-w-full items-center gap-1 overflow-x-auto p-1"
            aria-label="已打开文件"
          >
            {availableFiles.map(file => (
              <div
                key={JSON.stringify(file)}
                ref={sameFile(file, active) ? activeTab : undefined}
                className={cn(
                  'flex min-w-0 max-w-full shrink-0 items-center rounded-md',
                  sameFile(file, active) && 'bg-(--theme-surface-active)'
                )}
              >
                <Button
                  variant="ghost"
                  size="sm"
                  className="min-w-0 max-w-40 gap-1.5 overflow-hidden bg-transparent!"
                  title={file.path}
                  aria-current={sameFile(file, active) ? 'page' : undefined}
                  onClick={() => setSelected(file)}
                >
                  <span className="min-w-0 truncate">
                    {file.path.split('/').at(-1)}
                  </span>
                  {drafts[JSON.stringify(file)] && (
                    <span
                      className="size-1.5 shrink-0 rounded-full bg-current"
                      title="未保存"
                      aria-label="未保存"
                    />
                  )}
                </Button>
                <Button
                  variant="icon"
                  className="shrink-0 bg-transparent! hover:bg-(--theme-surface-hover)!"
                  aria-label={`关闭文件 ${file.path}`}
                  disabled={drafts[JSON.stringify(file)]?.saving}
                  onClick={() => {
                    if (drafts[JSON.stringify(file)]) setPendingClose(file)
                    else close(file)
                  }}
                >
                  <X className="size-3" aria-hidden="true" />
                </Button>
              </div>
            ))}
          </nav>,
          fileTabsTarget
        )}
      {projects.length ? (
        <section
          className="flex min-h-0 min-w-0 flex-1 flex-col bg-(--theme-surface-panel)"
          aria-label="文件内容"
        >
          {active && activeProject ? (
            <ExplorerFileEditor
              key={JSON.stringify(active)}
              root={active.root}
              file={active.path}
              projectName={activeProject.name}
              draft={drafts[JSON.stringify(active)]}
              onDraftChange={draft => updateDraft(active, draft)}
            />
          ) : (
            <p className="m-auto p-4 text-sm text-(--theme-text-muted)">
              选择文件以查看内容。
            </p>
          )}
        </section>
      ) : (
        <p className="m-auto p-6 text-sm text-(--theme-text-muted)">
          请先在主页添加机器人目录。
        </p>
      )}
      <ConfirmDialog
        open={Boolean(pendingClose)}
        title="关闭未保存的文件"
        message={`${pendingClose?.path ?? '当前文件'} 有未保存的修改。`}
        confirmLabel="放弃修改并关闭"
        onCancel={() => setPendingClose(null)}
        onConfirm={() => {
          if (pendingClose) close(pendingClose)
        }}
      />
    </section>
  )
}
