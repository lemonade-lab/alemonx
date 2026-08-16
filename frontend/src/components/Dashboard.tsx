import { useStoreState } from '../store/guideStore'
import { useAutoSave } from '../hooks/useAutoSave'
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
  type CSSProperties,
  type ReactNode,
  type SyntheticEvent
} from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  readDashboardNavigation,
  writeDashboardNavigation,
  type DashboardBuildMode,
  type DashboardConfigEditor,
  type DashboardPage,
  type DashboardSection
} from '../lib/dashboardNavigation'
import { createRandomID } from '../lib/randomId'
import cn from 'classnames'
import Markdown from 'markdown-to-jsx'
import {
  AlertTriangle,
  Activity,
  Archive,
  ArrowLeft,
  ArrowRight,
  ArrowRightLeft,
  Bot,
  Blocks,
  Cable,
  Check,
  CheckCircle2,
  CircleCheckBig,
  Info,
  CircleQuestionMark,
  ChevronRight,
  Circle,
  Loader2,
  ClipboardList,
  Code2,
  Download,
  Eye,
  EyeOff,
  FileText,
  FlaskConical,
  Folder,
  Gamepad2,
  GitBranch,
  Globe,
  Globe2,
  HardDrive,
  Headphones,
  KeyRound,
  Link,
  MessageSquare,
  Monitor,
  MoreVertical,
  Network,
  Package,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Pause,
  Pencil,
  Pin,
  Play,
  Plug,
  Plus,
  Radio,
  RefreshCw,
  Route,
  Search,
  Send,
  Settings,
  Shield,
  ShieldCheck,
  Terminal,
  Trash2,
  Upload,
  Waypoints,
  Wifi,
  X,
  type LucideIcon
} from 'lucide-react'
import { RobotConfigForm } from './RobotConfigForm'
import { ThemeToggle } from './ThemeToggle'
import { Button } from './Button'
import { Tabs } from './Tabs'
import { NpmrcConfigForm } from './NpmrcConfigForm'
import { EnvConfigForm } from './EnvConfigForm'
import { RobotPanel } from './RobotPanel'
import { OpsCenter } from './OpsCenter'
import { OpsOverview } from './OpsOverview'
import { NpmPublishPanel } from './NpmPublishPanel'
import { PackageManifestPanel } from './PackageManifestPanel'
import { AgentChatPage } from './AgentChat'
import { ErrorNotice } from './ErrorNotice'
import { EmptyState } from './EmptyState'
import { ConfirmDialog } from './ConfirmDialog'
import { DownloadProgress } from './DownloadProgress'
import {
  HostPluginAlert,
  HostPluginModal,
  HostUiToasts,
  PluginWebviewWindow,
  type HostToast,
  type HostUiRequest,
  type HostWebview
} from './PluginHostUi'
import { GLOBAL_MODAL_Z_INDEX, Modal } from './Modal'
import { AccountManagementPage } from './AccountManagement'
import { RobotGitControl } from './RobotGitControl'
import { useIsMobileViewport } from '../hooks/useIsMobileViewport'
import { TestCenter } from './TestCenter'
import { LiveChat } from './LiveChat'
import { DesktopWindow } from './DesktopWindow'
import {
  SidebarWindow,
  SidebarWindowActions,
  SidebarWindowSectionNav
} from './SidebarWindow'
import { qqChatWindowStorageKey } from './qqChatStorage'
import { SSHControl } from './SSHControl'
import { GitHubAuthControl } from './GitHubAuthControl'
import { ConfigFieldsEditor, ConfigSourceLinks } from './PackageConfigFields'
import { sameConfigValues } from './configFieldUtils'
import {
  workspaceApi,
  useCatalogDocumentQuery,
  useCatalogPackageConfigQuery,
  useCatalogQuery,
  useCatalogVersionsQuery,
  useGitStatusQuery,
  useGitWorkspaceQuery,
  useLazyRobotConsoleQuery,
  useLazyRobotFileQuery,
  useLazyPackageConfigQuery,
  useLazyRobotRuntimePreflightQuery,
  useLazyRobotRuntimeRepairQuery,
  useApplyRuntimeRepairMutation,
  useLazyRobotProjectQuery,
  useLocalPackagesQuery,
  useLocalPackageVersionsQuery,
  useLocalPackageReadmeQuery,
  usePackageManifestQuery,
  usePackageConfigQuery,
  useRobotRuntimeQuery,
  useRobotPM2StatusQuery,
  useRobotPM2ProcessesQuery,
  useLazyAppPortQuery,
  useLazyTestPortQuery,
  useLazyRobotPortsQuery,
  useSaveAppPortMutation,
  useSaveTestPortMutation,
  useRobotAppsQuery,
  useBotAppPagesQuery,
  useSetAppEnabledMutation,
  useRobotTasksQuery,
  useLazyRobotTaskQuery,
  useSaveRobotLoginMutation,
  useSetSetupPluginEnabledMutation,
  useInstallSetupPluginMutation,
  useUninstallSetupPluginMutation,
  useLazySetupPluginReleasesQuery,
  useLazySetupPluginVersionsQuery,
  useSetupPluginCacheQuery,
  usePluginDownloadCacheQuery,
  useSwitchSetupPluginVersionMutation,
  useDeleteSetupPluginVersionMutation,
  useCleanupSetupPluginCacheMutation,
  useClearPluginDownloadCacheMutation,
  useSetupPluginDevelopmentQuery,
  useRegisterSetupPluginDevelopmentMutation,
  useRunSetupPluginDevelopmentMutation,
  useRemoveSetupPluginDevelopmentMutation,
  useLazySetupPluginDevelopmentLogsQuery,
  useUploadSetupPluginArchiveMutation,
  useUploadRobotPackageMutation,
  useSetSystemCurrentRobotMutation,
  useSystemMcpQuery,
  useSetupPluginMarketQuery,
  useSetupPluginsQuery,
  useStartRobotTaskMutation,
  useWritePackageConfigMutation,
  useWriteRobotFileMutation,
  type RuntimeOverview,
  type PM2Status,
  type RuntimePreflight,
  type RobotPortStatus,
  type PackageConfig,
  type PackageConfigField,
  type SetupPlugin,
  type SetupPluginRelease,
  type SetupPluginVersion,
  type SetupPluginDevelopment
} from '../store/workspaceApi'
import {
  addProjects,
  removeProject as removeWorkspaceProject,
  reorderProjects,
  selectProject,
  pinProject as pinWorkspaceProject,
  setDeveloperMode,
  setDraft,
  setRobotConfig
} from '../store/workspaceStore'
import {
  setProject as setGuideProject,
  type RootState
} from '../store/guideStore'

type Check = {
  id: string
  name: string
  status: 'ready' | 'missing' | 'warning' | 'outdated'
  detail: string
  suggestion: string
  optional?: boolean
}
type CatalogItem = {
  name: string
  description: string
  url: string
  install: string
}
type CatalogGroup = { title: string; items: CatalogItem[] }
type Page = DashboardPage
type Section = DashboardSection
type Project = { id: string; path: string; name: string; pinned?: boolean }
type SystemFeature = string
type SystemWindowState = { minimized: boolean }

type FloatingWindowID =
  | 'terminal'
  | 'git'
  | 'app'
  | 'test'
  | 'live'
  | 'pm2Logs'
  | 'pm2Status'
  | 'ops'
  | `system:${string}`
type Props = {
  report: { checks: Check[] } | null
  checking: boolean
  error: string
  defaultPage: string
  onOpenGuide: () => void
  onOpenSettings: () => void
  onClearError: () => void
  onCheck: () => void
  onFix: (check: Check) => void
  windowStyle?: CSSProperties
  windowControls?: ReactNode
  onWindowStateChange?: (state: {
    terminal: { open: boolean; minimized: boolean }
    git: { open: boolean; minimized: boolean }
    app: { open: boolean; minimized: boolean }
    test: { open: boolean; minimized: boolean }
    live: { open: boolean; minimized: boolean }
    pm2Logs: { open: boolean; minimized: boolean }
    pm2Status: { open: boolean; minimized: boolean }
    ops: { open: boolean; minimized: boolean }
    system: Record<string, { open: boolean; minimized: boolean; label: string }>
  }) => void
  goals?: unknown
  goal?: unknown
  onSelect?: (id: string) => void
}

const coreFeatureCatalog: Array<{
  id: SystemFeature
  label: string
  icon: ReactNode
  status?: string
}> = [
  { id: 'plugins', label: '插件', icon: <Plug /> },
  { id: 'ops-overview', label: '运维', icon: <ShieldCheck /> }
]

function systemFeatureLabel(feature: SystemFeature, plugins: SetupPlugin[]) {
  return (
    plugins.find(item => feature === `setup:${item.id}`)?.name ??
    coreFeatureCatalog.find(item => item.id === feature)?.label ??
    { tasks: '任务', environment: '环境检查' }[feature] ??
    '系统功能'
  )
}

function usesSystemFeatureSidebar(feature: SystemFeature) {
  return ['plugins', 'ops-overview', 'tasks', 'environment'].includes(feature)
}
const directoryActions: Array<{
  id: Section | Page
  label: string
  icon: ReactNode
  kind: 'section' | 'page'
}> = [
  { id: 'runtime', label: '运行', icon: <Play />, kind: 'section' },
  { id: 'config', label: '配置', icon: <Settings />, kind: 'section' },
  { id: 'backpack', label: '背包', icon: <Archive />, kind: 'section' },
  { id: 'plugins', label: '插件', icon: <Package />, kind: 'page' },
  { id: 'connections', label: '连接', icon: <Link />, kind: 'page' },
  { id: 'modules', label: '模块', icon: <Blocks />, kind: 'page' },
  { id: 'build', label: '发布', icon: <Send />, kind: 'page' }
]
const emptyGitCommits: Array<{
  sha: string
  shortSha: string
  subject: string
  createdAt: string
}> = []
const emptyGitBranches: Array<{
  name: string
  commits: typeof emptyGitCommits
}> = []

const setupPluginIconMap: Record<string, LucideIcon> = {
  network: Network,
  forward: ArrowRightLeft,
  forwarding: ArrowRightLeft,
  interface: Radio,
  lan: Radio,
  wifi: Wifi,
  route: Route,
  dns: Globe,
  mirror: Globe,
  proxy: Globe,
  firewall: Shield,
  shield: Shield,
  port: Cable,
  traffic: Waypoints
}

function setupPluginIcon(icon?: string) {
  const Icon = icon ? setupPluginIconMap[icon] : undefined
  if (Icon) return <Icon />
  if (icon && icon.length === 1)
    return <span className="inline-block leading-none">{icon}</span>
  return <Plug />
}

// 平台连接包通过 desktop.logo 声明 antd 图标名；这里映射到 lucide 展示。
function platformLogoIcon(logo?: string): LucideIcon {
  const text = (logo ?? '').toLowerCase()
  if (text.includes('qq') || text.includes('wechat') || text.includes('weixin'))
    return MessageSquare
  if (text.includes('discord')) return Gamepad2
  if (text.includes('kook')) return Headphones
  if (text.includes('telegram')) return Send
  return Bot
}

function PlatformLogo({
  logo,
  className
}: {
  logo?: string
  className?: string
}) {
  const Icon = platformLogoIcon(logo)
  return <Icon className={className ?? 'size-4'} />
}

function projectName(path: string) {
  const cleaned = path.replace(/[\\/]+$/, '')
  const leaf = cleaned.split(/[\\/]/).filter(Boolean).pop()
  return leaf || cleaned || path
}

function formatTaskTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}

function isMissingConfigValue(value: unknown) {
  if (value === undefined || value === null) return true
  if (typeof value === 'string') return value.trim() === ''
  if (Array.isArray(value)) return value.length === 0
  if (typeof value === 'object') return Object.keys(value).length === 0
  return false
}

// RTK Query rejects with a serialised object rather than an Error. Keep the
// server's explanation intact so a permission problem is never shown as the
// unhelpful generic "操作未完成".
function operationErrorMessage(reason: unknown, fallback: string) {
  if (reason instanceof Error && reason.message) return reason.message
  if (typeof reason === 'string' && reason) return reason
  if (reason && typeof reason === 'object') {
    const value = reason as {
      data?: { error?: unknown; message?: unknown } | string
      error?: unknown
      message?: unknown
    }
    const data = value.data
    if (typeof data === 'string' && data) return data
    if (data && typeof data === 'object') {
      if (typeof data.error === 'string' && data.error) return data.error
      if (typeof data.message === 'string' && data.message) return data.message
    }
    if (typeof value.error === 'string' && value.error) return value.error
    if (typeof value.message === 'string' && value.message) return value.message
  }
  return fallback
}

export function DirectoryPicker({
  open,
  title = '选择目录',
  multiple = true,
  priority = false,
  includeFiles = false,
  selectionMode = 'directory',
  extensions = '',
  onModeChange,
  onExtensionsChange,
  onClose,
  onSelect
}: {
  open: boolean
  title?: string
  multiple?: boolean
  priority?: boolean
  includeFiles?: boolean
  selectionMode?: 'directory' | 'file' | 'extension'
  extensions?: string
  onModeChange?: (mode: 'directory' | 'file' | 'extension') => void
  onExtensionsChange?: (extensions: string) => void
  onClose: () => void
  onSelect: (paths: string[]) => void
}) {
  type Directory = { name: string; path: string }
  type File = { name: string; path: string }
  type DirectoryData = {
    path: string
    parent: string
    roots: string[]
    locations: Array<{
      name: string
      path: string
      kind: 'home' | 'disk' | 'volume'
    }>
    directories: Directory[]
    files?: File[]
  }
  const [path, setPath] = useStoreState('')
  const [query, setQuery] = useStoreState('')
  const [hidden, setHidden] = useStoreState(false)
  const [data, setData] = useStoreState<DirectoryData | null>(null)
  const [directoryError, setDirectoryError] = useStoreState('')
  const [directoryReload, setDirectoryReload] = useStoreState(0)
  const [selected, setSelected] = useStoreState<string[]>([])
  const [history, setHistory] = useStoreState<string[]>([])
  const [historyIndex, setHistoryIndex] = useStoreState(-1)
  const [contextMenu, setContextMenu] = useStoreState<{
    x: number
    y: number
    target?: Directory
  } | null>(null)
  const [newFolderName, setNewFolderName] = useStoreState('')
  const [deleteTarget, setDeleteTarget] = useStoreState<Directory | null>(null)

  const visit = (nextPath: string) => {
    if (!nextPath || nextPath === path) return
    setPath(nextPath)
    setSelected([])
    setHistory(entries => {
      const next = [...entries.slice(0, historyIndex + 1), nextPath]
      setHistoryIndex(next.length - 1)
      return next
    })
  }
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    const parameters = new URLSearchParams(
      path
        ? {
            path,
            hidden: String(hidden),
            files: String(includeFiles || selectionMode !== 'directory')
          }
        : {
            hidden: String(hidden),
            files: String(includeFiles || selectionMode !== 'directory')
          }
    )
    void fetch(`/api/v1/directories?${parameters}`, {
      signal: controller.signal
    })
      .then(async response => {
        const body = await response.text()
        if (!response.ok) {
          try {
            const payload = JSON.parse(body) as { error?: string }
            throw new Error(payload.error || '目录无法读取。')
          } catch (reason) {
            if (reason instanceof Error) throw reason
            throw new Error('目录无法读取。')
          }
        }
        return JSON.parse(body) as DirectoryData
      })
      .then(result => {
        setData(result)
        setDirectoryError('')
        if (!path) {
          setPath(result.path)
          setHistory([result.path])
          setHistoryIndex(0)
        }
      })
      .catch((reason: unknown) => {
        if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
          setDirectoryError(
            reason instanceof Error ? reason.message : '目录无法读取。'
          )
        }
      })
    return () => controller.abort()
  }, [
    directoryReload,
    hidden,
    includeFiles,
    open,
    path,
    selectionMode,
    setData,
    setDirectoryError,
    setHistory,
    setHistoryIndex,
    setPath
  ])
  if (!open) return null
  const suffixes = extensions
    .split(/[,，\s]+/)
    .map(item => item.replace(/^\./, '').toLowerCase())
    .filter(Boolean)
  const items = [
    ...(data?.directories ?? []).map(item => ({
      ...item,
      kind: 'directory' as const
    })),
    ...(includeFiles || selectionMode !== 'directory'
      ? (data?.files ?? []).map(item => ({ ...item, kind: 'file' as const }))
      : [])
  ].filter(item => {
    if (!item.name.toLowerCase().includes(query.toLowerCase())) return false
    return (
      item.kind !== 'file' ||
      selectionMode !== 'extension' ||
      suffixes.length === 0 ||
      suffixes.some(suffix => item.name.toLowerCase().endsWith(`.${suffix}`))
    )
  })
  const selectDirectory = (
    itemPath: string,
    event: Pick<MouseEvent<HTMLButtonElement>, 'metaKey' | 'ctrlKey'>
  ) =>
    setSelected(current =>
      multiple && (event.metaKey || event.ctrlKey)
        ? current.includes(itemPath)
          ? current.filter(entry => entry !== itemPath)
          : [...current, itemPath]
        : [itemPath]
    )
  const home = data?.roots[0] ?? ''
  const favorites = [
    { name: 'home', path: home },
    ...['Desktop', 'Documents', 'Downloads', 'Pictures'].map(name => ({
      name,
      path: `${home}/${name}`
    }))
  ]
  const locations = data?.locations ?? []
  // Finder sidebars represent directories too. In a directory picker, picking
  // one should both navigate there and make it immediately confirmable.
  const selectSidebarLocation = (nextPath: string) => {
    visit(nextPath)
    if (selectionMode !== 'file') setSelected([nextPath])
  }
  const goHistory = (step: number) => {
    const target = history[historyIndex + step]
    if (target) {
      setHistoryIndex(historyIndex + step)
      setPath(target)
      setSelected([])
    }
  }
  const directoryAction = async (
    method: 'POST' | 'DELETE',
    body: Record<string, string>
  ) => {
    try {
      const response = await fetch('/api/v1/directories', {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })
      const data = (await response.json()) as { error?: string }
      if (!response.ok) throw new Error(data.error || '目录操作未完成。')
      setDirectoryReload(current => current + 1)
      setDirectoryError('')
    } catch (reason) {
      setDirectoryError(
        reason instanceof Error ? reason.message : '目录操作未完成。'
      )
    }
  }
  return (
    <Modal
      open
      zIndex={priority ? GLOBAL_MODAL_Z_INDEX + 1 : undefined}
      onBackdropClick={() => setContextMenu(null)}
      ariaLabel={title}
    >
      <section
        className="directory-picker finder-picker theme-finder grid h-[min(700px,calc(100vh-32px))] w-full max-w-5xl grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_24px_70px_rgb(28_26_23/0.26)]"
        role="dialog"
        aria-label={title}
      >
        <header className="finder-header grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-4 border-b border-slate-200 px-4 py-3">
          <div className="finder-header-navigation flex flex-row items-center gap-2">
            <nav
              className="finder-navigation flex items-center gap-1"
              aria-label="目录导航"
            >
              <button
                className="icon-button size-8 p-0"
                disabled={historyIndex <= 0 && !data?.parent}
                onClick={() =>
                  historyIndex > 0 ? goHistory(-1) : visit(data?.parent ?? '')
                }
                title="后退"
                aria-label="后退"
              >
                <ArrowLeft className="size-4" />
              </button>
              <button
                className="icon-button size-8 p-0"
                disabled={historyIndex >= history.length - 1}
                onClick={() => goHistory(1)}
                title="前进"
                aria-label="前进"
              >
                <ArrowRight className="size-4" />
              </button>
              <button
                className="icon-button size-8 p-0"
                onClick={() => setHidden(value => !value)}
                title={hidden ? '隐藏隐藏目录' : '显示隐藏目录'}
                aria-label={hidden ? '隐藏隐藏目录' : '显示隐藏目录'}
              >
                {hidden ? (
                  <EyeOff className="size-4" />
                ) : (
                  <Eye className="size-4" />
                )}
              </button>
            </nav>
          </div>
          <strong className="finder-location truncate text-center text-sm font-semibold text-slate-800">
            {data?.path
              ? /^[a-z]:[\\/]?$/i.test(data.path)
                ? `本地磁盘（${data.path.slice(0, 2).toUpperCase()}）`
                : data.path.split(/[\\/]/).filter(Boolean).pop() || '系统磁盘'
              : title}
          </strong>
          <div className="finder-context-tools flex items-center justify-end gap-2">
            {onModeChange && (
              <div
                className="flex rounded-md border border-slate-200 bg-slate-50 p-0.5"
                role="tablist"
                aria-label="选择方式"
              >
                {(
                  [
                    ['directory', '目录'],
                    ['file', '文件'],
                    ['extension', '指定格式']
                  ] as const
                ).map(([mode, label]) => (
                  <button
                    className={cn(
                      'rounded px-2 py-1 text-[11px] font-medium',
                      selectionMode === mode
                        ? 'bg-white text-slate-800 shadow-sm'
                        : 'text-slate-500 hover:text-slate-700'
                    )}
                    key={mode}
                    onClick={() => onModeChange(mode)}
                    role="tab"
                    aria-selected={selectionMode === mode}
                  >
                    {label}
                  </button>
                ))}
              </div>
            )}
            {selectionMode === 'extension' && onExtensionsChange && (
              <input
                className="h-9 w-20 rounded-md border border-slate-300 px-2 text-xs text-slate-700 outline-none focus:border-brand-600"
                value={extensions}
                onChange={event => onExtensionsChange(event.target.value)}
                placeholder="ts, tsx"
                aria-label="文件格式"
              />
            )}
            <label className="flex h-9 w-44 items-center gap-2 rounded-md border border-slate-300 px-2.5 text-slate-400 focus-within:border-brand-600 focus-within:ring-2 focus-within:ring-brand-100">
              <Search className="size-4" />
              <input
                className="min-w-0 flex-1 bg-transparent text-xs text-slate-800 outline-none placeholder:text-slate-400"
                value={query}
                onChange={event => setQuery(event.target.value)}
                placeholder="搜索当前目录"
              />
            </label>
          </div>
        </header>
        <section className="grid min-h-0 grid-cols-[190px_minmax(0,1fr)]">
          <aside className="grid content-start gap-1 overflow-auto border-r border-slate-200 bg-slate-50 p-3">
            <small className="mb-1 px-2 text-[11px] font-semibold text-slate-400">
              常用
            </small>
            {favorites.map(item => (
              <button
                className={cn(
                  'flex min-h-8 items-center gap-2 rounded-md px-2 text-xs font-medium transition',
                  item.path === data?.path
                    ? 'bg-slate-200 text-slate-900'
                    : 'text-slate-600 hover:bg-slate-100'
                )}
                key={item.path}
                onClick={() => selectSidebarLocation(item.path)}
              >
                <Folder className="size-4 text-slate-500" />
                {item.name}
              </button>
            ))}
            {locations.length > 0 && (
              <>
                <small className="mb-1 mt-3 px-2 text-[11px] font-semibold text-slate-400">
                  磁盘与位置
                </small>
                {locations.map(location => (
                  <button
                    className={cn(
                      'flex min-h-8 items-center gap-2 rounded-md px-2 text-xs font-medium transition',
                      location.path === data?.path
                        ? 'bg-slate-200 text-slate-900'
                        : 'text-slate-600 hover:bg-slate-100'
                    )}
                    key={location.path}
                    onClick={() => selectSidebarLocation(location.path)}
                    title={location.path}
                  >
                    {location.kind === 'home' ? (
                      <Folder className="size-4 text-slate-500" />
                    ) : (
                      <HardDrive className="size-4 text-slate-500" />
                    )}
                    {location.name}
                  </button>
                ))}
              </>
            )}
          </aside>
          <main className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)]">
            <header className="grid grid-cols-[minmax(0,1fr)_100px] border-b border-slate-200 px-4 py-2 text-[11px] font-semibold text-slate-400">
              <span>名称</span>
              <span>种类</span>
            </header>
            <div className="grid content-start gap-0.5 overflow-auto p-2">
              {directoryError && (
                <div className="m-2 grid gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-800">
                  <strong>需要访问授权</strong>
                  <span>{directoryError}</span>
                  <button
                    className="secondary-button justify-self-start"
                    onClick={() => setDirectoryReload(current => current + 1)}
                  >
                    重试
                  </button>
                </div>
              )}
              {items.map(item => (
                <button
                  className={cn(
                    'grid min-h-9 grid-cols-[minmax(0,1fr)_100px] items-center rounded-md px-2 text-left text-xs transition',
                    selected.includes(item.path)
                      ? 'bg-slate-200 text-slate-900'
                      : 'text-slate-700 hover:bg-slate-100',
                    selectionMode === 'directory' &&
                      item.kind === 'file' &&
                      'cursor-default opacity-55 hover:bg-transparent'
                  )}
                  key={item.path}
                  onClick={event => {
                    if (selectionMode === 'directory' && item.kind === 'file')
                      return
                    if (selectionMode === 'file' && item.kind === 'directory') {
                      visit(item.path)
                      return
                    }
                    // 指定格式以当前目录为范围；点击匹配文件只是快捷地选中它所在目录。
                    selectDirectory(
                      selectionMode === 'extension' && item.kind === 'file'
                        ? (data?.path ?? item.path)
                        : item.path,
                      event
                    )
                  }}
                  onDoubleClick={() =>
                    item.kind === 'directory' && visit(item.path)
                  }
                  onContextMenu={event => {
                    if (item.kind !== 'directory') return
                    event.preventDefault()
                    event.stopPropagation()
                    setSelected([item.path])
                    setContextMenu({
                      x: event.clientX,
                      y: event.clientY,
                      target: item
                    })
                  }}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    {item.kind === 'directory' ? (
                      <Folder className="size-4 shrink-0 text-slate-500" />
                    ) : (
                      <FileText className="size-4 shrink-0 text-slate-400" />
                    )}
                    <span className="truncate">{item.name}</span>
                  </span>
                  <small className="text-[11px] text-slate-400">
                    {item.kind === 'directory' ? '文件夹' : '文件'}
                  </small>
                </button>
              ))}
            </div>
          </main>
        </section>
        <footer className="flex items-center justify-between gap-3 border-t border-slate-200 px-4 py-3">
          <span
            className="min-w-0 truncate text-xs text-slate-500"
            title={data?.path ?? ''}
          >
            {data?.path ?? '正在读取目录…'}
          </span>
          <div className="flex shrink-0 gap-2">
            <button className="secondary-button" onClick={onClose}>
              取消
            </button>
            <button
              className="primary-button"
              disabled={!selected.length}
              onClick={() => onSelect(selected)}
            >
              {multiple
                ? '添加'
                : selectionMode === 'file'
                  ? '选择文件'
                  : '选择'}
            </button>
          </div>
        </footer>
      </section>
      {contextMenu && (
        <div
          className="fixed z-210 grid min-w-32 overflow-hidden rounded-md border border-slate-200 bg-white py-1 shadow-lg"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          role="menu"
          onClick={event => event.stopPropagation()}
        >
          <button
            className="px-3 py-2 text-left text-xs text-slate-700 hover:bg-slate-100"
            onClick={() => {
              setNewFolderName(' ')
              setContextMenu(null)
            }}
          >
            新建文件夹
          </button>
          {contextMenu.target && (
            <button
              className="px-3 py-2 text-left text-xs text-red-700 hover:bg-red-50"
              onClick={() => {
                setDeleteTarget(contextMenu.target ?? null)
                setContextMenu(null)
              }}
            >
              删除文件夹
            </button>
          )}
        </div>
      )}
      {newFolderName !== '' && (
        <Modal open ariaLabel="新建文件夹">
          <form
            className="grid w-full max-w-sm gap-3 rounded-xl bg-white p-4 shadow-xl"
            onSubmit={event => {
              event.preventDefault()
              const name = newFolderName.trim()
              if (!name || !data?.path) return
              void directoryAction('POST', { path: data.path, name })
              setNewFolderName(' ')
            }}
          >
            <strong className="text-sm text-slate-800">新建文件夹</strong>
            <input
              autoFocus
              className="h-9 rounded-md border border-slate-300 px-2 text-sm outline-none focus:border-brand-600"
              value={newFolderName}
              onChange={event => setNewFolderName(event.target.value)}
              placeholder="文件夹名称"
            />
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="secondary-button"
                onClick={() => setNewFolderName('')}
              >
                取消
              </button>
              <button
                className="primary-button"
                disabled={!newFolderName.trim()}
              >
                新建
              </button>
            </div>
          </form>
        </Modal>
      )}
      {deleteTarget && (
        <ConfirmDialog
          open
          title="删除文件夹"
          subtitle={deleteTarget.name}
          message="将永久删除该文件夹及其中的全部内容，此操作无法撤销。"
          confirmLabel="删除"
          destructive
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => {
            void directoryAction('DELETE', { path: deleteTarget.path })
            setDeleteTarget(null)
          }}
        />
      )}
    </Modal>
  )
}

export function Dashboard({
  report,
  checking,
  error,
  defaultPage,
  onOpenGuide,
  onOpenSettings,
  onClearError,
  onCheck,
  onFix,
  windowStyle,
  windowControls,
  onWindowStateChange
}: Props) {
  const dispatch = useDispatch()
  const navigate = useNavigate()
  const location = useLocation()
  const navigationFromURL = useMemo(
    () => readDashboardNavigation(location.search),
    [location.search]
  )
  const [page, setPage] = useStoreState<Page>(() => navigationFromURL.page)
  const [sidebarCollapsed, setSidebarCollapsed] = useStoreState(false)
  const [robotNavigationHidden, setRobotNavigationHidden] = useStoreState(false)
  const [systemFeature, setSystemFeature] = useStoreState<SystemFeature | null>(
    null
  )
  const [systemWindowFeature, setSystemWindowFeature] =
    useStoreState<SystemFeature | null>(null)
  const [systemWindows, setSystemWindows] = useStoreState<
    Record<SystemFeature, SystemWindowState>
  >({})
  const [section, setSection] = useStoreState<Section>(
    () => navigationFromURL.section
  )
  const [file, setFile] = useStoreState('.npmrc')
  const [output, setOutput] = useStoreState('')
  const [outputFailed, setOutputFailed] = useStoreState(false)
  const [consoleOpen, setConsoleOpen] = useStoreState(false)
  const [consoleMinimized, setConsoleMinimized] = useStoreState(false)
  const [busy, setBusy] = useStoreState(false)
  const [catalogTitle, setCatalogTitle] = useStoreState('')
  const [catalogItem, setCatalogItem] = useStoreState<CatalogItem | null>(null)
  const [configEditor, setConfigEditor] = useStoreState<DashboardConfigEditor>(
    () => navigationFromURL.configEditor
  )
  const [buildMode, setBuildMode] = useStoreState<DashboardBuildMode>(
    () => navigationFromURL.buildMode
  )
  const [releaseVersion, setReleaseVersion] = useStoreState('')
  const [directoryPickerOpen, setDirectoryPickerOpen] = useStoreState(false)
  const [cloneProgress, setCloneProgress] = useStoreState(0)
  const [cloneStatus, setCloneStatus] = useStoreState('正在准备克隆…')
  const [gitCloneMode, setGitCloneMode] = useStoreState<'robot' | 'package'>(
    'robot'
  )
  const [gitCloneOpen, setGitCloneOpen] = useStoreState(false)
  const [gitDestinationPickerOpen, setGitDestinationPickerOpen] =
    useStoreState(false)
  const [gitDestination, setGitDestination] = useStoreState('')
  const openRobotClone = () => {
    setGitCloneMode('robot')
    setGitCloneOpen(true)
  }
  const openBackpackClone = () => {
    setGitCloneMode('package')
    setGitCloneOpen(true)
  }
  const [gitProject, setGitProject] = useStoreState<Project | null>(null)
  const [appPortDialog, setAppPortDialog] = useStoreState(false)
  const [appPortValue, setAppPortValue] = useStoreState('')
  const [appPortBusy, setAppPortBusy] = useStoreState(false)
  const [testPortDialog, setTestPortDialog] = useStoreState(false)
  const [testPortValue, setTestPortValue] = useStoreState('')
  const [testPortBusy, setTestPortBusy] = useStoreState(false)
  const [livePortDialog, setLivePortDialog] = useStoreState(false)
  const [livePortValue, setLivePortValue] = useStoreState('')
  const [livePortBusy, setLivePortBusy] = useStoreState(false)
  const [liveLoginRequest, setLiveLoginRequest] = useStoreState(0)
  const [appLaunching, setAppLaunching] = useStoreState(false)
  const [appContentOpen, setAppContentOpen] = useStoreState(false)
  const [appMinimized, setAppMinimized] = useStoreState(false)
  const [testLaunching, setTestLaunching] = useStoreState(false)
  const [testContentOpen, setTestContentOpen] = useStoreState(false)
  const [testMinimized, setTestMinimized] = useStoreState(false)
  const [liveContentOpen, setLiveContentOpen] = useStoreState(false)
  const [liveMinimized, setLiveMinimized] = useStoreState(false)
  const [selectedAppPageID, setSelectedAppPageID] = useStoreState('')
  const [pendingAppPageID, setPendingAppPageID] = useStoreState('')
  const [gitMinimized, setGitMinimized] = useStoreState(false)
  const [pm2LogsOpen, setPM2LogsOpen] = useStoreState(false)
  const [pm2LogsMinimized, setPM2LogsMinimized] = useStoreState(false)
  const [pm2ProcessesOpen, setPM2ProcessesOpen] = useStoreState(false)
  const [pm2ProcessesMinimized, setPM2ProcessesMinimized] = useStoreState(false)
  const [opsOpen, setOpsOpen] = useStoreState(false)
  const [opsMinimized, setOpsMinimized] = useStoreState(false)
  const [windowLayers, setWindowLayers] = useStoreState<Record<string, number>>(
    {
      terminal: 101,
      git: 102,
      app: 103,
      test: 104,
      live: 105,
      pm2Logs: 106,
      pm2Status: 107,
      ops: 108
    }
  )
  const nextWindowLayer = useRef(108)
  const openAppRef = useRef<() => void>(() => {})
  const openTestRef = useRef<() => void>(() => {})
  const openLiveRef = useRef<() => void>(() => {})
  const [invalidDirectory, setInvalidDirectory] = useStoreState('')
  const [pendingBackpackRemoval, setPendingBackpackRemoval] = useStoreState('')
  const [pendingProjectRemoval, setPendingProjectRemoval] = useStoreState<
    string | null
  >(null)
  const [aiOpen, setAIOpen] = useStoreState(() => navigationFromURL.agentOpen)
  const [agentSessions, setAgentSessions] = useStoreState<
    Array<{ id: string; title: string; root: string; updated: string }>
  >([])
  const [agentSessionId, setAgentSessionId] = useStoreState(
    () => navigationFromURL.sessionID
  )
  const [renameTarget, setRenameTarget] = useStoreState<{
    id: string
    title: string
  } | null>(null)
  const [renameTitle, setRenameTitle] = useStoreState('')
  const activateFloatingWindow = useCallback(
    (id: FloatingWindowID) => {
      const layer = ++nextWindowLayer.current
      setWindowLayers(current => ({ ...current, [id]: layer }))
      window.dispatchEvent(
        new CustomEvent('alx:desktop-window-layer', { detail: layer })
      )
    },
    [setWindowLayers]
  )
  useEffect(() => {
    const syncWindowLayer = (event: Event) => {
      const layer = (event as CustomEvent<number>).detail
      if (typeof layer === 'number')
        nextWindowLayer.current = Math.max(nextWindowLayer.current, layer)
    }
    window.addEventListener('alx:desktop-window-layer', syncWindowLayer)
    return () =>
      window.removeEventListener('alx:desktop-window-layer', syncWindowLayer)
  }, [])
  // Plugin list changes arrive over SSE (setup/plugins/events), so the query
  // only refetches when the registry actually changes instead of polling.
  const {
    data: setupPluginsData,
    refetch: refetchSetupPlugins,
    isLoading: isPluginsLoading,
    isFetching: isPluginsFetching
  } = useSetupPluginsQuery(undefined, {
    refetchOnMountOrArgChange: true
  })
  const pluginMarketOpen =
    Boolean(systemWindows.plugins) || systemFeature === 'plugins'
  const {
    data: setupPluginMarketData,
    isLoading: isPluginMarketLoading,
    isFetching: isPluginMarketFetching
  } = useSetupPluginMarketQuery(undefined, {
    skip: !pluginMarketOpen,
    refetchOnMountOrArgChange: true
  })
  // The backend serialises an empty plugin registry as JSON null; normalise to
  // an array so render-time .find/.filter never reads a null value.
  const setupPlugins = useMemo(() => setupPluginsData ?? [], [setupPluginsData])
  const setupPluginMarket = useMemo(
    () => setupPluginMarketData ?? [],
    [setupPluginMarketData]
  )
  useEffect(() => {
    onWindowStateChange?.({
      terminal: { open: consoleOpen, minimized: consoleMinimized },
      git: { open: Boolean(gitProject), minimized: gitMinimized },
      app: { open: appContentOpen, minimized: appMinimized },
      test: { open: testContentOpen, minimized: testMinimized },
      live: { open: liveContentOpen, minimized: liveMinimized },
      pm2Logs: { open: pm2LogsOpen, minimized: pm2LogsMinimized },
      pm2Status: { open: pm2ProcessesOpen, minimized: pm2ProcessesMinimized },
      ops: { open: opsOpen, minimized: opsMinimized },
      system: Object.fromEntries(
        Object.entries(systemWindows).map(([feature, state]) => [
          feature,
          {
            open: true,
            minimized: state.minimized,
            label: systemFeatureLabel(feature, setupPlugins)
          }
        ])
      )
    })
  }, [
    appContentOpen,
    appMinimized,
    consoleMinimized,
    consoleOpen,
    gitMinimized,
    gitProject,
    onWindowStateChange,
    opsMinimized,
    opsOpen,
    pm2LogsMinimized,
    pm2LogsOpen,
    pm2ProcessesMinimized,
    pm2ProcessesOpen,
    setupPlugins,
    systemWindows,
    testContentOpen,
    testMinimized,
    liveContentOpen,
    liveMinimized
  ])
  useEffect(() => {
    const toggleTerminal = () => {
      activateFloatingWindow('terminal')
      if (!consoleOpen) {
        setConsoleOpen(true)
        setConsoleMinimized(false)
      } else {
        setConsoleMinimized(value => !value)
      }
    }
    window.addEventListener('alx:desktop-terminal-toggle', toggleTerminal)
    return () =>
      window.removeEventListener('alx:desktop-terminal-toggle', toggleTerminal)
  }, [activateFloatingWindow, consoleOpen, setConsoleMinimized, setConsoleOpen])
  const loadAgentSessions = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/agent/sessions')
      if (!response.ok) return
      const data = (await response.json()) as Array<{
        id: string
        title: string
        root: string
        updated: string
      }>
      setAgentSessions(data)
    } catch {
      // 会话列表加载失败不阻塞
    }
  }, [setAgentSessions])
  useEffect(() => {
    void loadAgentSessions()
  }, [loadAgentSessions, setAgentSessionId])
  useEffect(() => {
    const refresh = () => {
      void loadAgentSessions()
    }
    const clearSession = () => {
      setAgentSessionId('')
    }
    window.addEventListener('alx:agent-session-created', refresh)
    window.addEventListener('alx:agent-new-session', clearSession)
    return () => {
      window.removeEventListener('alx:agent-session-created', refresh)
      window.removeEventListener('alx:agent-new-session', clearSession)
    }
  }, [loadAgentSessions, setAgentSessionId])
  const environmentChecked = useRef(false)
  const pendingRootValidation = useRef<string | null>(null)
  const eventsRef = useRef<EventSource | null>(null)
  const opsRefreshTimer = useRef<number | null>(null)
  const rawProjects = useSelector(
    (state: RootState) => state.workspace.projects
  )
  // Keep a stable array reference so effects depending on it do not re-run on
  // every render when the selector briefly yields null/undefined.
  // Persisted projects may carry a stale full-path name from older releases
  // (Windows separators were not split), so always derive the display name
  // from the path here.
  const projects = useMemo(
    () =>
      ((rawProjects ?? []) as Project[]).map(project => ({
        ...project,
        name: projectName(project.path)
      })),
    [rawProjects]
  )
  const activeProjectID = useSelector(
    (state: RootState) => state.workspace.activeProjectID
  )
  const developerMode = useSelector(
    (state: RootState) => state.workspace.developerMode
  )
  const activeProject = projects.find(item => item.id === activeProjectID)
  const root = activeProject?.path ?? ''
  const [setSystemCurrentRobot] = useSetSystemCurrentRobotMutation()
  useEffect(() => {
    void setSystemCurrentRobot({ root })
      .unwrap()
      .catch(() => {
        // The bridge is auxiliary: an unavailable host context must never
        // interrupt ordinary project navigation.
      })
  }, [root, setSystemCurrentRobot])
  const draftKey = `${root}:${file}`
  const applyingURLNavigation = useRef(false)
  const lastURLNavigationSearch = useRef(location.search)
  const pendingHistoryNavigation = useRef(false)

  // User-initiated page changes should behave like ordinary browser navigation;
  // defaults, capability checks and URL normalisation intentionally replace.
  const markUserNavigation = useCallback(() => {
    pendingHistoryNavigation.current = true
  }, [])

  // Project selection is navigation as well. Writing the destination root
  // first prevents the URL restoration effect from immediately selecting the
  // previous project again.
  const openProject = useCallback(
    (id: string) => {
      const project = projects.find(item => item.id === id)
      if (!project) return
      const search = writeDashboardNavigation(location.search, {
        root: project.path,
        page: 'robot',
        section: 'runtime',
        buildMode,
        configEditor,
        agentOpen: false,
        sessionID: ''
      })
      if (search === location.search) {
        dispatch(selectProject(id))
        return
      }
      navigate(
        { pathname: location.pathname, search, hash: location.hash },
        { replace: false }
      )
    },
    [
      buildMode,
      configEditor,
      dispatch,
      location.hash,
      location.pathname,
      location.search,
      navigate,
      projects
    ]
  )

  // URL is the durable source for navigation only. Effects triggered by a
  // browser back/forward action update state first; the following state-to-URL
  // effect then deliberately skips one turn to avoid replacing that history
  // entry with stale state.
  useEffect(() => {
    if (lastURLNavigationSearch.current === location.search) return
    lastURLNavigationSearch.current = location.search
    let changed = false
    if (page !== navigationFromURL.page) {
      setPage(navigationFromURL.page)
      changed = true
    }
    if (section !== navigationFromURL.section) {
      setSection(navigationFromURL.section)
      changed = true
    }
    if (buildMode !== navigationFromURL.buildMode) {
      setBuildMode(navigationFromURL.buildMode)
      changed = true
    }
    if (configEditor !== navigationFromURL.configEditor) {
      setConfigEditor(navigationFromURL.configEditor)
      changed = true
    }
    if (aiOpen !== navigationFromURL.agentOpen) {
      setAIOpen(navigationFromURL.agentOpen)
      changed = true
    }
    if (agentSessionId !== navigationFromURL.sessionID) {
      setAgentSessionId(navigationFromURL.sessionID)
      changed = true
    }
    if (changed) applyingURLNavigation.current = true
  }, [
    agentSessionId,
    aiOpen,
    buildMode,
    configEditor,
    location.search,
    navigationFromURL,
    page,
    section,
    setAgentSessionId,
    setAIOpen,
    setBuildMode,
    setConfigEditor,
    setPage,
    setSection
  ])

  useEffect(() => {
    if (applyingURLNavigation.current) {
      applyingURLNavigation.current = false
      return
    }
    // Do not replace an incoming root before its project has been selected or
    // validated. This makes direct links and browser back/forward deterministic.
    if (navigationFromURL.root && navigationFromURL.root !== root) return
    const search = writeDashboardNavigation(location.search, {
      root,
      page,
      section,
      buildMode,
      configEditor,
      agentOpen: aiOpen,
      sessionID: agentSessionId
    })
    if (search === location.search) {
      pendingHistoryNavigation.current = false
      return
    }
    navigate(
      { pathname: location.pathname, search, hash: location.hash },
      { replace: !pendingHistoryNavigation.current }
    )
    pendingHistoryNavigation.current = false
  }, [
    agentSessionId,
    aiOpen,
    buildMode,
    configEditor,
    location.hash,
    location.pathname,
    location.search,
    navigate,
    navigationFromURL.root,
    page,
    root,
    section
  ])
  const content = useSelector((state: RootState) =>
    file === 'alemon.config.yaml'
      ? (state.workspace.robotConfigs[root] ?? '')
      : (state.workspace.drafts[draftKey] ?? '')
  )
  const configContent = useSelector(
    (state: RootState) => state.workspace.robotConfigs[root] ?? ''
  )
  const hasRobotConfig = useSelector(
    (state: RootState) => Boolean(root) && root in state.workspace.robotConfigs
  )
  const catalogKind =
    page === 'plugins'
      ? 'apps'
      : page === 'connections'
        ? 'environment'
        : 'modules'
  const {
    data: catalogData,
    isFetching: catalogLoading,
    error: catalogQueryError
  } = useCatalogQuery(catalogKind, {
    skip: !['plugins', 'connections', 'modules'].includes(page),
    refetchOnMountOrArgChange: true
  })
  // RTK Query leaves data undefined until the first fetch, and a backend may
  // respond with JSON null for an empty list. Normalise both to an array so
  // render-time .find/.map never touches a null value.
  const catalog = useMemo(() => catalogData ?? [], [catalogData])
  const {
    data: localPackages,
    isFetching: packagesLoading,
    error: packagesError,
    refetch: refetchPackages
  } = useLocalPackagesQuery(root, { skip: !root || section !== 'backpack' })
  const {
    data: runtime,
    isFetching: runtimeLoading,
    refetch: refetchRuntime
  } = useRobotRuntimeQuery(root, { skip: !root })
  const {
    data: pm2Status,
    error: pm2StatusError,
    refetch: refetchPM2Status
  } = useRobotPM2StatusQuery(root, {
    // Always query once a root is selected. Skipping on pm2Configured meant a
    // freshly generated pm2.config.cjs (right after "修复后台运行") never woke
    // the query, so the card stayed on "启动服务" even after PM2 came online.
    skip: !root,
    refetchOnMountOrArgChange: true
  })
  const {
    data: currentPackageConfig,
    // Keep the form mounted while an automatic save refreshes its data.
    // `isFetching` also becomes true for that background refresh.
    isLoading: currentPackageConfigLoading
  } = usePackageConfigQuery(
    { root, package: '' },
    { skip: !root || section !== 'config' || configEditor !== 'visual' }
  )
  const watchDevelopmentTask = page === 'robot' && section === 'runtime'
  const { data: operationTasksData } = useRobotTasksQuery(undefined, {
    skip: !watchDevelopmentTask,
    // Task state is driven by SSE task events (invalidateTags); no polling.
    pollingInterval: 0,
    refetchOnMountOrArgChange: true
  })
  const operationTasks = operationTasksData ?? []
  const [loadRobotTask] = useLazyRobotTaskQuery()
  const [readRobotFile] = useLazyRobotFileQuery()
  const [validateRobot, { data: projectValidation }] =
    useLazyRobotProjectQuery()
  const [startRobotTask] = useStartRobotTaskMutation()
  const [loadAppPort] = useLazyAppPortQuery()
  const [saveAppPort] = useSaveAppPortMutation()
  const [loadTestPort] = useLazyTestPortQuery()
  const [saveTestPort] = useSaveTestPortMutation()
  const { data: botAppPages = [] } = useBotAppPagesQuery(root, {
    skip: !root
  })
  const [writeRobotFile] = useWriteRobotFileMutation()
  const [writePackageConfig] = useWritePackageConfigMutation()
  const [saveRobotLogin] = useSaveRobotLoginMutation()
  const fileSaveTimers = useRef(new Map<string, number>())
  useEffect(
    () => () => {
      fileSaveTimers.current.forEach(timer => window.clearTimeout(timer))
    },
    []
  )
  const catalogError = catalogQueryError ? '在线目录暂时无法读取。' : ''
  const showOutput = (message: string, failed = false) => {
    setOutput(message)
    setOutputFailed(failed)
  }
  const waitForRobotTask = (
    taskID: string,
    options: {
      appReady?: boolean
      testReady?: boolean
      timeoutMs?: number
      onTask?: (task: {
        status?: string
        progress?: number
        path?: string
        output?: string
        error?: string
      }) => void
    } = {}
  ) =>
    new Promise<{
      status?: string
      progress?: number
      path?: string
      output?: string
      error?: string
    }>((resolve, reject) => {
      let settled = false
      const timeout = window.setTimeout(
        () => {
          finish(new Error('任务事件连接超时。'))
        },
        options.timeoutMs ??
          (options.appReady || options.testReady ? 35_000 : 30 * 60 * 1000)
      )
      const finish = (
        reason?: Error,
        task?: Parameters<NonNullable<typeof options.onTask>>[0]
      ) => {
        if (settled) return
        settled = true
        window.clearTimeout(timeout)
        window.removeEventListener('alx:unified-event', onEvent)
        if (reason) reject(reason)
        else resolve(task ?? {})
      }
      const settleTask = (
        task: Parameters<NonNullable<typeof options.onTask>>[0]
      ) => {
        if (task.status === 'running') return
        options.onTask?.(task)
        if (task.status === 'failed') {
          finish(new Error(task.error || '操作未完成。'))
          return
        }
        if (options.appReady || options.testReady) {
          finish(new Error('应用进程在端口就绪前结束。'))
          return
        }
        finish(undefined, task)
      }
      const onEvent = (event: Event) => {
        try {
          const envelope = (
            event as CustomEvent<{ topic?: string; data?: unknown }>
          ).detail
          if (envelope?.topic !== 'robot') return
          const payload = envelope.data as {
            type?: string
            taskId?: string
            text?: string
            task?: Parameters<NonNullable<typeof options.onTask>>[0]
          }
          if (payload.taskId !== taskID) return
          if (payload.type === 'app-ready' && options.appReady) {
            finish()
            return
          }
          if (payload.type === 'app-failed' && options.appReady) {
            finish(new Error(payload.text || '应用服务未能启动。'))
            return
          }
          if (payload.type === 'test-ready' && options.testReady) {
            finish()
            return
          }
          if (payload.type === 'test-failed' && options.testReady) {
            finish(new Error(payload.text || '测试服务未能启动。'))
            return
          }
          if (payload.type !== 'task' || !payload.task) return
          settleTask(payload.task)
        } catch {
          // Ignore malformed frames and rely on EventSource reconnection.
        }
      }
      window.addEventListener('alx:unified-event', onEvent)
      // A stop can finish before its HTTP response reaches the browser. Read
      // the authoritative snapshot after installing the listener so that race
      // cannot leave the whole panel in its global busy state.
      void loadRobotTask(taskID, true)
        .unwrap()
        .then(settleTask)
        .catch(() => {})
    })
  const persistFile = async (
    targetRoot: string,
    targetFile: string,
    nextContent: string
  ) => {
    try {
      await writeRobotFile({
        root: targetRoot,
        file: targetFile,
        content: nextContent
      }).unwrap()
      if (targetFile === 'alemon.config.yaml')
        dispatch(setRobotConfig({ root: targetRoot, content: nextContent }))
      else
        dispatch(
          setDraft({
            key: `${targetRoot}:${targetFile}`,
            content: nextContent
          })
        )
    } catch (reason) {
      showOutput(
        operationErrorMessage(reason, `${targetFile} 自动保存失败。`),
        true
      )
    }
  }
  const updateFileContent = (targetFile: string, nextContent: string) => {
    if (!root) return
    if (targetFile === 'alemon.config.yaml')
      dispatch(setRobotConfig({ root, content: nextContent }))
    else
      dispatch(setDraft({ key: `${root}:${targetFile}`, content: nextContent }))
    const key = `${root}:${targetFile}`
    const activeTimer = fileSaveTimers.current.get(key)
    if (activeTimer !== undefined) window.clearTimeout(activeTimer)
    fileSaveTimers.current.set(
      key,
      window.setTimeout(() => {
        fileSaveTimers.current.delete(key)
        void persistFile(root, targetFile, nextContent)
      }, 500)
    )
  }
  // "应用" = 机器人 + 应用端口。读取 serverPort；未配置则先让用户输入并
  // 保存后优先启动开发脚本；没有 dev 时才安全回退到 app 前台脚本。
  const openApp = async () => {
    if (!root || appLaunching) return
    setAppLaunching(true)
    try {
      const info = await loadAppPort(root, true).unwrap()
      if (info.configured) {
        await launchApp()
      } else {
        setAppPortValue(String(info.port))
        setAppPortDialog(true)
      }
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '无法读取应用端口。'), true)
    } finally {
      setAppLaunching(false)
    }
  }
  openAppRef.current = () => void openApp()
  const confirmAppPort = async () => {
    if (!root) return
    const port = Number(appPortValue.trim())
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      showOutput('应用端口应为 1-65535 之间的整数。', true)
      return
    }
    setAppPortBusy(true)
    try {
      await saveAppPort({ root, port }).unwrap()
      await refreshConfigDraft()
      setAppPortDialog(false)
      // Launch happens after the dialog closes; reflect it on the toolbar icon.
      setAppLaunching(true)
      await launchApp()
      if (pendingAppPageID) {
        setSelectedAppPageID(pendingAppPageID)
        setPendingAppPageID('')
        setAppContentOpen(true)
        setAppMinimized(false)
        activateFloatingWindow('app')
      }
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '应用端口保存失败。'), true)
    } finally {
      setAppPortBusy(false)
      setAppLaunching(false)
    }
  }
  const launchApp = async () => {
    if (!root) return
    try {
      // If the app is already serving, render it in-page instead of starting
      // another dev/app process (which would conflict with the running one).
      if (await checkAppReachable()) {
        setAppContentOpen(true)
        setAppMinimized(false)
        activateFloatingWindow('app')
        return
      }
      const task = await startRobotTask({
        root,
        action: 'app-open',
        ready: 'app'
      }).unwrap()
      await waitForRobotTask(task.id, { appReady: true })
      setAppContentOpen(true)
      setAppMinimized(false)
      activateFloatingWindow('app')
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '应用启动失败。'), true)
    }
  }
  const checkAppReachable = async () => {
    try {
      const response = await fetch(
        `/api/v1/robot/app-port?${new URLSearchParams({ root, probe: '1' })}`
      )
      const data = (await response.json()) as { reachable?: boolean }
      return response.ok && data.reachable === true
    } catch {
      return false
    }
  }
  // "测试" = 隔离的 testone 沙盒：后台临时嗅探空闲端口并用临时无 login
  // 配置启动，testone 的 /testone WebSocket 由后端同源代理，用户无需配置
  // 任何端口或登录，也不会影响机器人原有的运行形态。
  const openTest = async () => {
    if (!root || testLaunching) return
    setTestLaunching(true)
    try {
      const info = await loadTestPort(root, true).unwrap()
      if (info.configured) {
        await launchTest()
      } else {
        setTestPortValue(String(info.port))
        setTestPortDialog(true)
      }
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '测试台启动失败。'), true)
    } finally {
      setTestLaunching(false)
    }
  }
  openTestRef.current = () => void openTest()
  const confirmTestPort = async () => {
    if (!root) return
    const port = Number(testPortValue.trim())
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      showOutput('服务端口应为 1-65535 之间的整数。', true)
      return
    }
    setTestPortBusy(true)
    try {
      await saveTestPort({ root, port }).unwrap()
      await refreshConfigDraft()
      setTestPortDialog(false)
      setTestLaunching(true)
      await launchTest()
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '服务端口保存失败。'), true)
    } finally {
      setTestPortBusy(false)
      setTestLaunching(false)
    }
  }
  const launchTest = async () => {
    if (!root) return
    try {
      // If the sandbox service is already serving, open the test center
      // instead of starting another dev/app process.
      if (await checkTestReachable()) {
        setTestContentOpen(true)
        setTestMinimized(false)
        activateFloatingWindow('test')
        return
      }
      const task = await startRobotTask({
        root,
        action: 'dev',
        ready: 'test'
      }).unwrap()
      await waitForRobotTask(task.id, { testReady: true })
      setTestContentOpen(true)
      setTestMinimized(false)
      activateFloatingWindow('test')
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '测试服务启动失败。'), true)
    }
  }
  const checkTestReachable = async () => {
    try {
      const response = await fetch(
        `/api/v1/robot/test-port?${new URLSearchParams({ root, probe: '1' })}`
      )
      const data = (await response.json()) as { reachable?: boolean }
      return response.ok && data.reachable === true
    } catch {
      return false
    }
  }
  const requestLiveLogin = () => {
    markUserNavigation()
    closeTemporaryContentPage()
    setPage('robot')
    setSection('runtime')
    setLiveLoginRequest(value => value + 1)
  }
  const openLive = async () => {
    if (!root) return
    try {
      const info = await loadTestPort(root, true).unwrap()
      if (!info.configured) {
        setLivePortValue(String(info.port))
        setLivePortDialog(true)
        return
      }
      // Open the chat window even when the CBP endpoint is temporarily
      // unreachable. Its own connection state explains what is wrong, while
      // attempting to start again here conflicts with an existing robot
      // process and prevents the user from seeing the chat window at all.
      setLiveContentOpen(true)
      setLiveMinimized(false)
      activateFloatingWindow('live')
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '在线聊天不可用。'), true)
    }
  }
  openLiveRef.current = () => void openLive()
  const confirmLivePort = async () => {
    if (!root) return
    const port = Number(livePortValue.trim())
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      showOutput('CBP 服务端口应为 1-65535 之间的整数。', true)
      return
    }
    setLivePortBusy(true)
    try {
      await saveTestPort({ root, port }).unwrap()
      await refreshConfigDraft()
      setLivePortDialog(false)
      requestLiveLogin()
    } catch (reason) {
      showOutput(operationErrorMessage(reason, 'CBP 服务端口保存失败。'), true)
    } finally {
      setLivePortBusy(false)
    }
  }
  useEffect(() => {
    const toggleGit = () => {
      activateFloatingWindow('git')
      if (!gitProject && activeProject) {
        setGitProject(activeProject)
        setGitMinimized(false)
        return
      }
      if (gitProject) setGitMinimized(value => !value)
    }
    const toggleApp = () => {
      activateFloatingWindow('app')
      if (appContentOpen) {
        setAppMinimized(value => !value)
        return
      }
      openAppRef.current()
    }
    const toggleTest = () => {
      activateFloatingWindow('test')
      if (testContentOpen) {
        setTestMinimized(value => !value)
        return
      }
      openTestRef.current()
    }
    const toggleLive = () => {
      activateFloatingWindow('live')
      if (liveContentOpen) {
        setLiveMinimized(value => !value)
        return
      }
      openLiveRef.current()
    }
    const togglePM2Logs = () => {
      activateFloatingWindow('pm2Logs')
      if (!pm2LogsOpen) {
        setPM2LogsOpen(true)
        setPM2LogsMinimized(false)
        return
      }
      setPM2LogsMinimized(value => !value)
    }
    const togglePM2Status = () => {
      activateFloatingWindow('pm2Status')
      if (!pm2ProcessesOpen) {
        setPM2ProcessesOpen(true)
        setPM2ProcessesMinimized(false)
        return
      }
      setPM2ProcessesMinimized(value => !value)
    }
    const toggleOps = () => {
      activateFloatingWindow('ops')
      if (!opsOpen) {
        setOpsOpen(true)
        setOpsMinimized(false)
        return
      }
      setOpsMinimized(value => !value)
    }
    const toggleSystem = (event: Event) => {
      const feature = (event as CustomEvent<string>).detail
      if (!feature || !systemWindows[feature]) return
      setSystemWindowFeature(feature)
      activateFloatingWindow(`system:${feature}`)
      setSystemWindows(current => ({
        ...current,
        [feature]: { minimized: !current[feature].minimized }
      }))
    }
    window.addEventListener('alx:desktop-git-toggle', toggleGit)
    window.addEventListener('alx:desktop-app-toggle', toggleApp)
    window.addEventListener('alx:desktop-test-toggle', toggleTest)
    window.addEventListener('alx:desktop-live-toggle', toggleLive)
    window.addEventListener('alx:desktop-pm2-logs-toggle', togglePM2Logs)
    window.addEventListener('alx:desktop-pm2-status-toggle', togglePM2Status)
    window.addEventListener('alx:desktop-ops-toggle', toggleOps)
    window.addEventListener('alx:desktop-system-toggle', toggleSystem)
    return () => {
      window.removeEventListener('alx:desktop-git-toggle', toggleGit)
      window.removeEventListener('alx:desktop-app-toggle', toggleApp)
      window.removeEventListener('alx:desktop-test-toggle', toggleTest)
      window.removeEventListener('alx:desktop-live-toggle', toggleLive)
      window.removeEventListener('alx:desktop-pm2-logs-toggle', togglePM2Logs)
      window.removeEventListener(
        'alx:desktop-pm2-status-toggle',
        togglePM2Status
      )
      window.removeEventListener('alx:desktop-ops-toggle', toggleOps)
      window.removeEventListener('alx:desktop-system-toggle', toggleSystem)
    }
  }, [
    activeProject,
    activateFloatingWindow,
    appContentOpen,
    gitProject,
    setAppMinimized,
    setGitMinimized,
    setGitProject,
    setTestMinimized,
    testContentOpen,
    liveContentOpen,
    setLiveMinimized,
    pm2LogsOpen,
    pm2ProcessesOpen,
    setPM2LogsMinimized,
    setPM2LogsOpen,
    setPM2ProcessesMinimized,
    setPM2ProcessesOpen,
    setOpsMinimized,
    setOpsOpen,
    setSystemWindowFeature,
    opsOpen,
    setSystemWindows,
    systemWindows
  ])
  const refreshConfigDraft = async () => {
    if (!root) return
    const result = await readRobotFile(
      { root, file: 'alemon.config.yaml' },
      true
    ).unwrap()
    dispatch(
      setRobotConfig({
        root,
        content: result.output ?? ''
      })
    )
  }

  useEffect(() => {
    if (defaultPage === 'robot' && !navigationFromURL.hasPage) setPage('robot')
  }, [defaultPage, navigationFromURL.hasPage, setPage])
  // One durable event gateway owns the Dashboard's robot, ops, system and
  // plugin notifications. Its cursor is carried in the reconnect URL so a
  // temporary network loss replays persisted events before live delivery.
  useEffect(() => {
    let source: EventSource | null = null
    let retry: number | null = null
    let heartbeat: number | null = null
    let lastEventID = 0
    const tabID = createRandomID()
    const channel =
      typeof BroadcastChannel === 'undefined'
        ? null
        : new BroadcastChannel('alx-events')
    let disposed = false
    const broadcast = (message: unknown) => {
      if (!channel || disposed) return
      try {
        channel.postMessage(message)
      } catch (error) {
        // Web Lock acquisition may resolve after effect cleanup, when this
        // channel has already been closed. That late message is obsolete.
        if (!(
          error instanceof DOMException && error.name === 'InvalidStateError'
        ))
          throw error
      }
    }
    const leaseKey = 'alx-events-leader'
    let leader = false
    let releaseWebLock: (() => void) | null = null
    const webLocks = typeof navigator !== 'undefined' && 'locks' in navigator
    const dispatchEnvelope = (envelope: {
      id?: number
      topic?: string
      type?: string
      data?: {
        taskId?: string
        text?: string
        truncated?: boolean
        running?: boolean
        task?: unknown
      }
    }) => {
      if (typeof envelope.id === 'number')
        lastEventID = Math.max(lastEventID, envelope.id)
      const payload = envelope.data ?? {}
      window.dispatchEvent(
        new CustomEvent('alx:unified-event', { detail: envelope })
      )
      if (envelope.topic === 'robot') {
        if (envelope.type === 'task')
          dispatch(workspaceApi.util.invalidateTags(['OperationTasks']))
        else if (envelope.type === 'output' && payload.taskId)
          window.dispatchEvent(
            new CustomEvent('alx:robot-output', {
              detail: {
                taskId: payload.taskId,
                text: payload.text ?? '',
                truncated: payload.truncated === true
              }
            })
          )
      } else if (envelope.topic === 'ops') {
        if (opsRefreshTimer.current === null)
          opsRefreshTimer.current = window.setTimeout(() => {
            opsRefreshTimer.current = null
            window.dispatchEvent(new CustomEvent('alx:ops-changed'))
          }, 100)
      } else if (envelope.topic === 'plugins')
        dispatch(workspaceApi.util.invalidateTags(['SetupPlugins']))
      if (envelope.type === 'system.update.changed')
        dispatch(workspaceApi.util.invalidateTags(['SetupUpdate']))
      if (envelope.type === 'system.store-recovered') {
        dispatch(
          workspaceApi.util.invalidateTags([
            'OperationTasks',
            'SetupPlugins',
            'SetupUpdate'
          ])
        )
        window.dispatchEvent(new CustomEvent('alx:ops-changed'))
      }
      if (envelope.type === 'system.cursor-expired') {
        dispatch(
          workspaceApi.util.invalidateTags(['OperationTasks', 'SetupPlugins'])
        )
        window.dispatchEvent(new CustomEvent('alx:ops-changed'))
      }
    }
    const ownsLease = () => {
      if (!channel) return true
      try {
        const current = JSON.parse(localStorage.getItem(leaseKey) || '{}') as {
          id?: string
          expires?: number
        }
        const now = Date.now()
        if (current.id && current.id !== tabID && (current.expires ?? 0) > now)
          return false
        localStorage.setItem(
          leaseKey,
          JSON.stringify({ id: tabID, expires: now + 3500 })
        )
        return true
      } catch {
        return true
      }
    }
    const connect = () => {
      if (!leader) return
      source = new EventSource(
        `/api/v1/events?${new URLSearchParams({ topics: 'robot,ops,system,plugins', lastEventId: String(lastEventID) })}`
      )
      eventsRef.current = source
      source.onmessage = event => {
        try {
          const envelope = JSON.parse(event.data) as {
            id?: number
            topic?: string
            type?: string
            data?: {
              taskId?: string
              text?: string
              running?: boolean
              task?: unknown
            }
          }
          dispatchEnvelope(envelope)
          broadcast({ type: 'event', envelope })
        } catch {
          // Ignore malformed frames; reconnect remains cursor-based.
        }
      }
      source.onerror = () => {
        source?.close()
        if (!disposed && retry === null)
          retry = window.setTimeout(() => {
            retry = null
            connect()
          }, 1000)
      }
    }
    channel?.addEventListener('message', event => {
      const message = event.data as {
        type?: string
        envelope?: Parameters<typeof dispatchEnvelope>[0]
      }
      if (message.type === 'event' && message.envelope)
        dispatchEnvelope(message.envelope)
    })
    const acquireWebLock = () => {
      if (!webLocks || leader || releaseWebLock) return
      void navigator.locks
        .request('alx-events-leader', { ifAvailable: true }, lock => {
          if (!lock || disposed) return
          leader = true
          broadcast({ type: 'leader', id: tabID })
          connect()
          return new Promise<void>(resolve => {
            releaseWebLock = resolve
          })
        })
        .finally(() => {
          releaseWebLock = null
          if (leader) {
            leader = false
            source?.close()
            source = null
          }
        })
    }
    const elect = () => {
      if (webLocks) {
        acquireWebLock()
        return
      }
      const nextLeader = ownsLease()
      if (nextLeader && !leader) {
        leader = true
        connect()
      }
      if (!nextLeader && leader) {
        leader = false
        source?.close()
        source = null
      }
    }
    elect()
    heartbeat = window.setInterval(elect, 1000)
    const releaseForPageLifecycle = () => {
      if (document.visibilityState === 'hidden') releaseWebLock?.()
    }
    document.addEventListener('visibilitychange', releaseForPageLifecycle)
    window.addEventListener('pagehide', releaseForPageLifecycle)
    return () => {
      disposed = true
      source?.close()
      eventsRef.current = null
      if (retry !== null) window.clearTimeout(retry)
      if (heartbeat !== null) window.clearInterval(heartbeat)
      releaseWebLock?.()
      if (leader && !webLocks) localStorage.removeItem(leaseKey)
      channel?.close()
      document.removeEventListener('visibilitychange', releaseForPageLifecycle)
      window.removeEventListener('pagehide', releaseForPageLifecycle)
      if (opsRefreshTimer.current !== null) {
        window.clearTimeout(opsRefreshTimer.current)
        opsRefreshTimer.current = null
      }
    }
  }, [dispatch])

  // A root link may arrive during initial hydration, after a browser back/
  // forward action, or from an edited address bar. Resolve it every time the
  // query changes instead of treating it as a one-shot bootstrap parameter.
  useEffect(() => {
    const path = navigationFromURL.root
    if (!path || path === root) {
      pendingRootValidation.current = null
      return
    }
    if (projects.some(item => item.path === path)) {
      dispatch(selectProject(projects.find(item => item.path === path)!.id))
      return
    }
    if (pendingRootValidation.current === path) return
    pendingRootValidation.current = path
    void (async () => {
      try {
        const response = await fetch(
          `/api/v1/robot/validate?${new URLSearchParams({ root: path })}`
        )
        const data = (await response.json()) as { valid?: boolean }
        if (response.ok && data.valid === true) {
          dispatch(addProjects([{ id: path, path, name: projectName(path) }]))
          return
        }
      } catch {
        // A missing/invalid directory must not break the dashboard render.
      }
      if (pendingRootValidation.current !== path) return
      pendingRootValidation.current = null
      const parameters = new URLSearchParams(location.search)
      parameters.delete('root')
      const search = parameters.toString()
      navigate(
        {
          pathname: location.pathname,
          search: search ? `?${search}` : '',
          hash: location.hash
        },
        { replace: true }
      )
    })()
  }, [
    dispatch,
    location.hash,
    location.pathname,
    location.search,
    navigate,
    navigationFromURL.root,
    projects,
    root
  ])
  useEffect(() => {
    if (report || checking || environmentChecked.current) return
    environmentChecked.current = true
    onCheck()
  }, [checking, onCheck, page, report])
  useEffect(() => {
    if (catalog.length && !catalog.some(group => group.title === catalogTitle))
      setCatalogTitle(catalog[0].title)
  }, [catalog, catalogTitle, setCatalogTitle])
  useEffect(() => {
    if (root) void validateRobot(root)
  }, [root, validateRobot])
  useEffect(() => {
    if (!root || section !== 'config' || hasRobotConfig) return
    void readRobotFile({ root, file: 'alemon.config.yaml' }, true)
      .unwrap()
      .then(result =>
        dispatch(
          setRobotConfig({
            root,
            content: result.output ?? ''
          })
        )
      )
      .catch(() => dispatch(setRobotConfig({ root, content: '' })))
  }, [dispatch, hasRobotConfig, readRobotFile, root, section])
  useEffect(() => {
    if (developerMode) return
    if (page === 'build') setPage('robot')
    if (section === 'npmrc' || section === 'env') setSection('config')
    setConsoleOpen(false)
  }, [developerMode, page, section, setConsoleOpen, setPage, setSection])

  async function api(
    method: string,
    data: Record<string, string>
  ): Promise<boolean> {
    if (!root) {
      showOutput('请先在左侧添加机器人目录。', true)
      return false
    }
    setBusy(true)
    try {
      if (method === 'GET') {
        const result = await readRobotFile(
          { root: data.root, file: data.file },
          true
        ).unwrap()
        if (data.file === 'alemon.config.yaml')
          dispatch(
            setRobotConfig({ root: data.root, content: result.output ?? '' })
          )
        else
          dispatch(
            setDraft({
              key: `${data.root}:${data.file}`,
              content: result.output ?? ''
            })
          )
        return true
      }
      if (method === 'PUT') {
        const result = await writeRobotFile({
          root: data.root,
          file: data.file,
          content: data.content
        }).unwrap()
        if (data.file === 'alemon.config.yaml')
          dispatch(setRobotConfig({ root: data.root, content: data.content }))
        showOutput(result.output ?? '操作完成。')
        return true
      }
      const task = await startRobotTask(data).unwrap()
      if (data.action === 'dev' || data.action === 'app') {
        setOutput('')
        setConsoleOpen(true)
        return true
      }
      if (data.action === 'dev-stop' || data.action === 'app-stop') {
        // Stopping is asynchronous: the task state keeps only the affected
        // runtime controls disabled. Do not retain the panel-wide busy lock while
        // a process handles SIGINT or the server performs its force-stop fallback.
        showOutput(task.output || '已请求停止，正在等待进程退出…')
        return task.status !== 'failed'
      }
      showOutput('操作已开始，正在等待完成…')
      const current =
        task.status && task.status !== 'running'
          ? task
          : await waitForRobotTask(task.id, {
              timeoutMs:
                data.action === 'dev-stop' || data.action === 'app-stop'
                  ? 12_000
                  : undefined
            })
      dispatch(
        workspaceApi.util.invalidateTags([{ type: 'Runtime', id: root }])
      )
      if (
        [
          'install-package',
          'uninstall-package',
          'install-connection',
          'uninstall-connection',
          'install-module',
          'uninstall-module',
          'remove-local-package',
          'replace-local-package',
          'switch-local-package-version',
          'force-switch-local-package-version',
          'enable-backpack-workspace',
          'disable-backpack-workspace'
        ].includes(data.action)
      ) {
        // The task mutation invalidates when it starts, which is still too
        // early for a download. Invalidate once it has actually finished so
        // the backpack updates without a page reload.
        dispatch(
          workspaceApi.util.invalidateTags([
            { type: 'LocalPackages', id: root },
            { type: 'PackageManifest', id: root },
            // Installing/uninstalling a connection package changes whether
            // its alemon.config.yaml section can be parsed, so drop any
            // cached PackageConfig for this root.
            { type: 'PackageConfig', id: root },
            { type: 'PackageConfig', id: `${root}:${data.package ?? ''}` }
          ])
        )
      }
      showOutput(current.output ?? '操作完成。')
      return true
    } catch (reason) {
      showOutput(
        operationErrorMessage(
          reason,
          '操作未完成，请在右上角任务记录中查看详情。'
        ),
        true
      )
      return false
    } finally {
      setBusy(false)
    }
  }

  async function savePackageConfig(
    packageName: string,
    values: Record<string, unknown>
  ): Promise<boolean> {
    if (!root) return false
    try {
      await writePackageConfig({
        root,
        package: packageName,
        values
      }).unwrap()
      await refreshConfigDraft()
      return true
    } catch (reason) {
      showOutput(
        operationErrorMessage(reason, '配置未保存，请检查所选机器人目录。'),
        true
      )
      return false
    }
  }

  async function saveRuntimeLogin(
    login: string,
    packageName = ''
  ): Promise<boolean> {
    if (!root || !login.trim()) return false
    try {
      await saveRobotLogin({
        root,
        login: login.trim(),
        package: packageName
      }).unwrap()
      await refreshConfigDraft()
      return true
    } catch (reason) {
      showOutput(operationErrorMessage(reason, '登录连接未保存。'), true)
      return false
    }
  }

  async function chooseDirectories() {
    setDirectoryPickerOpen(true)
  }
  async function addSelectedDirectories(paths: string[]) {
    if (!paths.length) return
    const checks = await Promise.all(
      paths.map(async path => {
        try {
          const response = await fetch(
            `/api/v1/robot/validate?${new URLSearchParams({ root: path })}`
          )
          const data = (await response.json()) as { valid?: boolean }
          return { path, valid: response.ok && data.valid === true }
        } catch {
          return { path, valid: false }
        }
      })
    )
    const validPaths = checks.filter(item => item.valid).map(item => item.path)
    const invalid = checks.find(item => !item.valid)?.path
    if (validPaths.length)
      dispatch(
        addProjects(
          validPaths.map(path => ({ id: path, path, name: projectName(path) }))
        )
      )
    setDirectoryPickerOpen(false)
    if (invalid) {
      setInvalidDirectory(invalid)
      return
    }
    markUserNavigation()
    setPage('robot')
    setSection('config')
    setOutput('')
  }
  async function cloneRobotRepository(
    repository: string,
    branch: string,
    name: string,
    mirror: string,
    depth: number
  ) {
    if (!gitDestination) return
    setBusy(true)
    setCloneProgress(0)
    setCloneStatus('正在启动 Git…')
    try {
      const response = await fetch('/api/v1/robot/git-clone', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          destination: gitDestination,
          repository,
          branch,
          name,
          mirror,
          depth
        })
      })
      const data = (await response.json()) as {
        id?: string
        output?: string
        error?: string
      }
      if (!response.ok || !data.id)
        throw new Error(data.error || '克隆仓库失败。')
      const cloneTaskID = data.id
      setCloneStatus(data.output || '正在连接远程仓库…')
      const task = await waitForRobotTask(cloneTaskID, {
        onTask: current => {
          setCloneProgress(current.progress ?? 10)
          setCloneStatus(current.output || '正在克隆仓库…')
        }
      })
      const targetPath = task.path
      if (!targetPath) throw new Error('克隆完成，但无法识别机器人目录。')
      showOutput(task.output || '仓库已克隆。')
      setGitCloneOpen(false)
      await addSelectedDirectories([targetPath])
      return
    } catch (reason) {
      showOutput(
        operationErrorMessage(reason, '克隆仓库失败，请检查 Git 地址和网络。'),
        true
      )
    } finally {
      setBusy(false)
      setCloneProgress(0)
      setCloneStatus('正在准备克隆…')
    }
  }

  async function cloneBackpackPackage(
    repository: string,
    branch: string,
    name: string,
    mirror: string,
    depth: number
  ) {
    if (!root) return
    setBusy(true)
    setCloneProgress(0)
    setCloneStatus('正在启动 Git…')
    try {
      const response = await fetch('/api/v1/robot/packages/git-clone', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ root, repository, branch, name, mirror, depth })
      })
      const data = (await response.json()) as {
        id?: string
        output?: string
        error?: string
      }
      if (!response.ok || !data.id)
        throw new Error(data.error || '克隆插件包失败。')
      setCloneStatus(data.output || '正在连接远程仓库…')
      const task = await waitForRobotTask(data.id, {
        onTask: current => {
          setCloneProgress(current.progress ?? 10)
          setCloneStatus(current.output || '正在克隆插件包…')
        }
      })
      await refetchPackages()
      showOutput(task.output || '插件包已克隆到背包。')
      setGitCloneOpen(false)
    } catch (reason) {
      showOutput(
        operationErrorMessage(
          reason,
          '克隆插件包失败，请检查 Git 地址和网络。'
        ),
        true
      )
    } finally {
      setBusy(false)
      setCloneProgress(0)
      setCloneStatus('正在准备克隆…')
    }
  }

  function removeProject(id: string) {
    setPendingProjectRemoval(id)
  }

  const pinProject = (id: string) => {
    dispatch(pinWorkspaceProject(id))
  }

  const reorderProject = (sourceID: string, targetID: string) => {
    dispatch(reorderProjects({ sourceID, targetID }))
  }

  function confirmRemoveProject() {
    if (!pendingProjectRemoval) return
    const removedPath = pendingProjectRemoval
    dispatch(removeWorkspaceProject(removedPath))
    setPendingProjectRemoval(null)
    setOutput('')
    // The URL root is the durable navigation contract: leaving it in place
    // makes the URL restoration effect re-validate and re-add the directory
    // immediately after it is removed from the management list.
    if (navigationFromURL.root === removedPath) {
      const parameters = new URLSearchParams(location.search)
      parameters.delete('root')
      const search = parameters.toString()
      navigate(
        {
          pathname: location.pathname,
          search: search ? `?${search}` : '',
          hash: location.hash
        },
        { replace: true }
      )
    }
  }

  // AI is a content page, not an overlay. Any normal navigation must leave it
  // first so the control card always works.
  function closeTemporaryContentPage() {
    setAIOpen(false)
  }

  function openSection(nextSection: Section) {
    markUserNavigation()
    closeTemporaryContentPage()
    setSection(nextSection)
    setOutput('')
    if (nextSection === 'npmrc') {
      setFile('.npmrc')
      api('GET', { root, file: '.npmrc' })
    }
    if (nextSection === 'env') {
      setFile('.env')
      api('GET', { root, file: '.env' })
    }
  }
  function openTextConfig() {
    markUserNavigation()
    setConfigEditor('text')
    setFile('alemon.config.yaml')
  }
  function openVisualConfig() {
    markUserNavigation()
    setConfigEditor('visual')
  }
  function selectPage(nextPage: Page) {
    markUserNavigation()
    closeTemporaryContentPage()
    setSystemFeature(null)
    setSystemWindowFeature(null)
    setSystemWindows({})
    setPage(nextPage)
    setCatalogItem(null)
    setOutput('')
  }
  function openAI(sessionID?: string) {
    markUserNavigation()
    closeTemporaryContentPage()
    setSystemFeature(null)
    setPage('robot')
    if (typeof sessionID === 'object' && sessionID !== null) {
      console.warn('openAI 收到对象参数，已忽略：', sessionID)
      sessionID = ''
    }
    setAgentSessionId(sessionID ?? '')
    setAIOpen(true)
    setOutput('')
    // 每次进入 Agent 都刷新会话列表，确保"记录"能看到新建的对话。
    void loadAgentSessions()
  }
  function requestRename(id: string, title: string) {
    setRenameTarget({ id, title })
    setRenameTitle(title)
  }
  async function archiveSession(id: string) {
    try {
      const response = await fetch(`/api/v1/agent/sessions/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived: true })
      })
      if (!response.ok) return
      if (id === agentSessionId) openAI()
      await loadAgentSessions()
    } catch {
      // 归档失败不阻塞
    }
  }
  async function renameSession(id: string) {
    if (!renameTarget || renameTitle.trim().length < 2) return
    try {
      const response = await fetch(`/api/v1/agent/sessions/${id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: renameTitle.trim() })
      })
      if (!response.ok) return
      setRenameTarget(null)
      await loadAgentSessions()
    } catch {
      // 重命名失败不阻塞
    }
  }
  function closeSystemWindow(feature: SystemFeature) {
    setSystemWindows(current => {
      const remaining = { ...current }
      delete remaining[feature]
      return remaining
    })
    setSystemWindowFeature(current => (current === feature ? null : current))
  }
  function selectSystemFeature(nextFeature: SystemFeature) {
    closeTemporaryContentPage()
    setSystemFeature(null)
    setSystemWindowFeature(nextFeature)
    setSystemWindows(current => ({
      ...current,
      [nextFeature]: { minimized: false }
    }))
    activateFloatingWindow(`system:${nextFeature}`)
    setOutput('')
  }

  const currentCatalog =
    catalog.find(group => group.title === catalogTitle) ?? catalog[0]
  const robotContent = aiOpen ? (
    <AgentChatPage root={root} initialSessionId={agentSessionId} />
  ) : (
    <>
      {section === 'backpack' && (
        <BackpackPanel
          root={root}
          items={localPackages?.items ?? []}
          loading={packagesLoading}
          failed={Boolean(packagesError)}
          onRefresh={() => void refetchPackages()}
          onOpenPlugins={() => selectPage('plugins')}
          onClone={openBackpackClone}
          busy={busy}
          onSetPrivate={enabled =>
            api('POST', {
              root,
              action: enabled
                ? 'enable-backpack-workspace'
                : 'disable-backpack-workspace'
            })
          }
          onSaveConfig={savePackageConfig}
          onConfigChanged={refreshConfigDraft}
          onRemove={async packageName => setPendingBackpackRemoval(packageName)}
          onReplace={async (packageName, version, force = false) =>
            api('POST', {
              root,
              action: force
                ? 'force-switch-local-package-version'
                : 'switch-local-package-version',
              package: packageName,
              version,
              ...(force ? { confirm: 'true' } : {})
            })
          }
        />
      )}
      {developerMode && section === 'npmrc' && (
        <NpmrcConfigForm
          content={content}
          onChange={nextContent => updateFileContent('.npmrc', nextContent)}
        />
      )}
      {developerMode && section === 'env' && (
        <EnvConfigForm
          content={content}
          onChange={nextContent => updateFileContent('.env', nextContent)}
        />
      )}
      {section === 'config' && (
        <section className="config-form">
          {configEditor === 'visual' ? (
            <>
              <RobotConfigForm
                content={configContent}
                onChange={next => updateFileContent('alemon.config.yaml', next)}
                toolbar={
                  <EditorMode
                    active={configEditor}
                    onVisual={openVisualConfig}
                    onText={openTextConfig}
                  />
                }
              />
              <CurrentProjectConfigPanel
                config={currentPackageConfig}
                loading={currentPackageConfigLoading}
                onSave={values => savePackageConfig('', values)}
              />
            </>
          ) : (
            <FileEditor
              toolbar={
                <EditorMode
                  active={configEditor}
                  onVisual={openVisualConfig}
                  onText={openTextConfig}
                />
              }
              content={content}
              placeholder="配置内容"
              onChange={nextContent => updateFileContent(file, nextContent)}
            />
          )}
        </section>
      )}
      {section === 'runtime' && (
        <RuntimePanel
          overview={runtime}
          pm2Status={pm2Status}
          pm2StatusError={Boolean(pm2StatusError)}
          root={root}
          loading={runtimeLoading}
          busy={busy}
          developmentRunning={operationTasks.some(
            item =>
              item.root === root &&
              item.action === 'dev' &&
              item.status === 'running'
          )}
          foregroundRunning={operationTasks.some(
            item =>
              item.root === root &&
              item.action === 'app' &&
              item.status === 'running'
          )}
          developmentStopping={operationTasks.some(
            item =>
              item.root === root &&
              item.action === 'dev-stop' &&
              item.status === 'running'
          )}
          foregroundStopping={operationTasks.some(
            item =>
              item.root === root &&
              item.action === 'app-stop' &&
              item.status === 'running'
          )}
          onRefresh={() => {
            void refetchRuntime()
            void refetchPM2Status()
          }}
          onOpenPM2Logs={() => {
            setPM2LogsOpen(true)
            setPM2LogsMinimized(false)
            activateFloatingWindow('pm2Logs')
          }}
          onOpenPM2Processes={() => {
            setPM2ProcessesOpen(true)
            setPM2ProcessesMinimized(false)
            activateFloatingWindow('pm2Status')
          }}
          onRun={(action, packageName) =>
            api('POST', {
              root,
              action,
              ...(packageName ? { package: packageName } : {})
            }).then(async success => {
              if (success) {
                // Refresh before the caller continues (e.g. installing a
                // connection package) so the runtime overview's per-platform
                // "installed" flag is already fresh. A failed refresh must not
                // reject the original operation result.
                try {
                  await refetchRuntime().unwrap()
                  // PM2 status is always refreshed here; gating it on the
                  // closure's pm2Configured would skip the fetch right after a
                  // "修复后台运行" generated the config, leaving the card stuck
                  // on "启动服务".
                  await refetchPM2Status()
                } catch {
                  // Keep the original result; the overview refetches on next poll.
                }
              }
              return success
            })
          }
          onRefreshOverview={async () => {
            try {
              return await refetchRuntime().unwrap()
            } catch {
              return runtime
            }
          }}
          pm2Running={Boolean(pm2Status?.running)}
          onSaveLogin={saveRuntimeLogin}
          onSavePackageConfig={savePackageConfig}
          liveLoginRequest={liveLoginRequest}
          developerMode={developerMode}
        />
      )}
    </>
  )

  const catalogContent =
    catalogItem && currentCatalog ? (
      <CatalogDetail
        item={catalogItem}
        group={currentCatalog.title}
        kind={
          page === 'connections'
            ? 'connection'
            : page === 'modules'
              ? 'module'
              : 'plugin'
        }
        busy={busy}
        onBack={() => setCatalogItem(null)}
        onRun={(action, packageName) =>
          api('POST', { root, action, package: packageName })
        }
        onSaveConfig={savePackageConfig}
      />
    ) : (
      <RobotPanel
        className="catalog-workspace max-w-190"
        icon={<Globe className="size-4" />}
        title={currentCatalog?.title || '目录'}
        description={
          page === 'modules'
            ? '浏览并管理机器人项目的 JS 模块依赖'
            : '浏览并管理可安装的机器人包'
        }
      >
        {catalogLoading && (
          <EmptyState
            loading
            icon={
              page === 'modules' ? (
                <Blocks className="size-6 text-slate-400 dark:text-slate-500" />
              ) : page === 'connections' ? (
                <Link className="size-6 text-slate-400 dark:text-slate-500" />
              ) : (
                <Package className="size-6 text-slate-400 dark:text-slate-500" />
              )
            }
            title={`正在读取${
              page === 'modules'
                ? '模块目录'
                : page === 'connections'
                  ? '连接目录'
                  : '插件目录'
            }`}
            description="正在加载可用内容，请稍候。"
          />
        )}
        {catalogError && (
          <EmptyState
            icon={<AlertTriangle className="size-6 text-amber-500" />}
            title="目录暂时不可用"
            description={catalogError}
          />
        )}
        {!catalogLoading && !catalogError && currentCatalog && (
          <section className="grid gap-2">
            {currentCatalog.items.map(item => (
              <button
                className="flex items-center gap-3 rounded-lg border border-slate-200 bg-white p-3 text-left transition hover:border-slate-300 hover:bg-slate-50"
                key={`${currentCatalog.title}-${item.name}`}
                onClick={() => setCatalogItem(item)}
              >
                <span className="grid min-w-0 flex-1 gap-1">
                  <strong className="truncate text-sm font-semibold text-slate-800">
                    {item.name}
                  </strong>
                  <small className="truncate text-xs text-slate-500">
                    {item.description || '查看包说明、安装与配置'}
                  </small>
                </span>
                <ChevronRight className="size-4 shrink-0 text-slate-400" />
              </button>
            ))}
          </section>
        )}
      </RobotPanel>
    )
  const workspaceSetupPlugin = setupPlugins.find(
    item => systemFeature === `setup:${item.id}`
  )
  const systemWindowContent = (
    feature: SystemFeature,
    sidebarLayout = false
  ) => {
    const setupPlugin = setupPlugins.find(
      item => feature === `setup:${item.id}`
    )
    return feature === 'ops-overview' ? (
      <OpsOverview
        projects={projects}
        sidebarLayout={sidebarLayout}
        onOpenProject={id => {
          openProject(id)
          closeSystemWindow(feature)
        }}
      />
    ) : feature === 'plugins' ? (
      <SystemPluginCenter
        plugins={setupPlugins}
        marketPlugins={setupPluginMarket}
        marketLoading={isPluginMarketLoading || isPluginMarketFetching}
        pluginsLoading={isPluginsLoading || isPluginsFetching}
        sidebarLayout={sidebarLayout}
        onOpen={id => selectSystemFeature(`setup:${id}`)}
        onRefresh={() => void refetchSetupPlugins()}
      />
    ) : feature === 'accounts' ? (
      <AccountManagementPage />
    ) : feature === 'tasks' ? (
      <OperationTasksPage root={root} sidebarLayout={sidebarLayout} />
    ) : feature === 'environment' ? (
      <EnvironmentPage
        report={report}
        checking={checking}
        sidebarLayout={sidebarLayout}
        onRefresh={onCheck}
        onFix={onFix}
      />
    ) : setupPlugin ? (
      <SetupPluginCenter plugin={setupPlugin} />
    ) : null
  }
  const invalidProject = Boolean(
    activeProject && projectValidation && !projectValidation.valid
  )
  const workspace =
    systemFeature === 'ops-overview' ? (
      <OpsOverview
        projects={projects}
        onOpenProject={id => {
          openProject(id)
          setSystemFeature(null)
        }}
      />
    ) : systemFeature === 'plugins' ? (
      <SystemPluginCenter
        plugins={setupPlugins}
        marketPlugins={setupPluginMarket}
        marketLoading={isPluginMarketLoading || isPluginMarketFetching}
        pluginsLoading={isPluginsLoading || isPluginsFetching}
        onOpen={id => selectSystemFeature(`setup:${id}`)}
        onRefresh={() => void refetchSetupPlugins()}
      />
    ) : systemFeature === 'accounts' ? (
      <AccountManagementPage />
    ) : systemFeature === 'tasks' ? (
      <OperationTasksPage root={root} />
    ) : systemFeature === 'environment' ? (
      <EnvironmentPage
        report={report}
        checking={checking}
        onRefresh={onCheck}
        onFix={onFix}
      />
    ) : workspaceSetupPlugin ? (
      <SetupPluginCenter plugin={workspaceSetupPlugin} />
    ) : invalidProject ? (
      <InvalidWorkspace
        project={activeProject!}
        reason={projectValidation?.error}
        onRemove={() => removeProject(activeProject!.id)}
        onChoose={chooseDirectories}
      />
    ) : activeProject ? (
      <>
        {page === 'robot' && robotContent}
        {developerMode && page === 'build' && (
          <section className="bot-build-page">
            {buildMode === 'manifest' ? (
              <PackageManifestPanel
                root={root}
                onSaveError={message => showOutput(message)}
              />
            ) : buildMode === 'npm' ? (
              <NpmPublishPanel
                root={root}
                busy={busy}
                onRun={(action, values) =>
                  api('POST', { root, action, ...values })
                }
              />
            ) : (
              <GitReleasePanelNext
                root={root}
                busy={busy}
                version={releaseVersion}
                onVersionChange={setReleaseVersion}
              />
            )}
            {output && (
              <OperationLog
                output={output}
                failed={outputFailed}
                onClose={() => {
                  setOutput('')
                  setOutputFailed(false)
                }}
              />
            )}
          </section>
        )}
        {['plugins', 'connections', 'modules'].includes(page) && catalogContent}
        {page !== 'build' && output && (
          <OperationLog
            output={output}
            failed={outputFailed}
            onClose={() => {
              setOutput('')
              setOutputFailed(false)
            }}
          />
        )}
      </>
    ) : (
      <EmptyWorkspace onAdd={chooseDirectories} onClone={openRobotClone} />
    )

  const environmentWarning = Boolean(
    report?.checks.some(item => item.status !== 'ready' && !item.optional)
  )

  return (
    <>
      <main className="guide-shell">
        <section
          className="guide-window dashboard-window"
          style={windowStyle}
          data-window-container="workbench"
        >
          <header className="topbar flex min-h-11 min-w-0 items-center justify-between gap-2 border-b border-slate-200 bg-white/90 px-3 dark:border-slate-700">
            <div className="flex min-w-0 flex-1 items-center gap-1.5">
              <Button
                variant="icon"
                onClick={onOpenSettings}
                aria-label="打开设置"
                title="设置"
              >
                <Settings className="size-4" />
              </Button>
              <a
                className="truncate px-1 text-[0.82rem] font-semibold tracking-[-0.01em] text-slate-800 no-underline transition-colors hover:text-brand-600 dark:text-slate-200"
                href="https://alemonjs.com/"
                target="_blank"
                rel="noreferrer"
              >
                ALemonX
              </a>
              <ThemeToggle />
              <Button
                variant="icon"
                onClick={() => setSidebarCollapsed(value => !value)}
                aria-label={sidebarCollapsed ? '展开侧边栏' : '折叠侧边栏'}
                aria-pressed={!sidebarCollapsed}
                title={sidebarCollapsed ? '展开侧边栏' : '折叠侧边栏'}
              >
                {sidebarCollapsed ? (
                  <PanelLeftOpen className="size-4" />
                ) : (
                  <PanelLeftClose className="size-4" />
                )}
              </Button>
            </div>
            <div className="ml-auto flex min-w-0 items-center gap-1">
              {developerMode && <McpControl />}
              <Button
                variant="secondary"
                className={cn(
                  'gap-1.5 px-2',
                  developerMode
                    ? 'border-blue-400 bg-slate-100 '
                    : 'border-slate-200 bg-white  hover:bg-slate-50'
                )}
                onClick={() => dispatch(setDeveloperMode(!developerMode))}
                aria-pressed={developerMode}
                title={
                  developerMode
                    ? '关闭开发模式，收起源码与发布工具'
                    : '开启开发模式，显示源码、终端与发布工具'
                }
              >
                <Code2 className="size-4" />
              </Button>
              <GitHubAuthControl />
              <SSHControl />
              <Button
                variant="icon"
                onClick={() => setRobotNavigationHidden(value => !value)}
                aria-label={
                  robotNavigationHidden
                    ? '显示机器人功能导航'
                    : '隐藏机器人功能导航'
                }
                aria-pressed={!robotNavigationHidden}
                title={
                  robotNavigationHidden
                    ? '显示机器人功能导航'
                    : '隐藏机器人功能导航'
                }
              >
                {robotNavigationHidden ? (
                  <PanelRightOpen className="size-4" />
                ) : (
                  <PanelRightClose className="size-4" />
                )}
              </Button>
              <button
                className="icon-button size-8 p-0"
                onClick={onOpenGuide}
                aria-label="打开引导"
                title="打开引导"
              >
                <CircleQuestionMark className="size-4" />
              </button>
            </div>
          </header>
          <ConfirmDialog
            open={Boolean(pendingBackpackRemoval)}
            title="从背包移除插件"
            subtitle="这会删除当前机器人 packages 目录中的本地插件文件。"
            message={`确定移除 ${pendingBackpackRemoval} 吗？此操作不会删除机器人主项目，但该插件需要重新安装后才能使用。`}
            confirmLabel="移除插件"
            destructive
            busy={busy}
            onCancel={() => setPendingBackpackRemoval('')}
            onConfirm={() => {
              const packageName = pendingBackpackRemoval
              if (!packageName) return
              void (async () => {
                if (
                  await api('POST', {
                    root,
                    action: 'remove-local-package',
                    package: packageName
                  })
                )
                  void refetchPackages()
                setPendingBackpackRemoval('')
              })()
            }}
          />
          <ConfirmDialog
            open={Boolean(pendingProjectRemoval)}
            title="移除项目"
            subtitle="仅从管理列表中移除，不会删除磁盘上的项目文件。"
            message={`确定将「${pendingProjectRemoval ? (projects.find(p => p.id === pendingProjectRemoval)?.name ?? pendingProjectRemoval) : ''}」从机器人目录移除吗？其磁盘文件保持不变，可随时重新添加。`}
            confirmLabel="移除"
            destructive
            onCancel={() => setPendingProjectRemoval(null)}
            onConfirm={confirmRemoveProject}
          />
          <Modal
            open={appPortDialog}
            ariaLabel="设置应用端口"
            // 不点遮罩关闭：端口输入框聚焦/输入时若 backdrop 收到 mousedown 会把
            // 弹窗误关。用户应通过取消或保存来关闭。
          >
            <form
              className="grid w-full max-w-sm gap-4 rounded-xl border border-slate-200 bg-white p-5 shadow-[0_20px_58px_rgb(28_26_23/0.22)]"
              onSubmit={event => {
                event.preventDefault()
                void confirmAppPort()
              }}
              onMouseDown={event => event.stopPropagation()}
            >
              <div className="grid gap-1">
                <strong className="text-sm text-ink-950">配置应用端口</strong>
              </div>
              <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                应用端口（1-65535）
                <input
                  className="h-10 rounded-md border border-slate-300 px-3 text-sm text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                  value={appPortValue}
                  onChange={event => {
                    setAppPortValue(event.target.value)
                  }}
                  type="number"
                  min={1}
                  max={65535}
                  autoFocus
                />
              </label>
              <footer className="flex justify-end gap-2">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => setAppPortDialog(false)}
                  disabled={appPortBusy}
                >
                  取消
                </button>
                <button className="primary-button" disabled={appPortBusy}>
                  {appPortBusy ? '保存中…' : '启动应用'}
                </button>
              </footer>
            </form>
          </Modal>
          <Modal open={testPortDialog} ariaLabel="设置服务端口">
            <form
              className="grid w-full max-w-sm gap-4 rounded-xl border border-slate-200 bg-white p-5 shadow-[0_20px_58px_rgb(28_26_23/0.22)]"
              onSubmit={event => {
                event.preventDefault()
                void confirmTestPort()
              }}
              onMouseDown={event => event.stopPropagation()}
            >
              <div className="grid gap-1">
                <strong className="text-sm text-ink-950">配置服务端口</strong>
                <span className="text-xs text-slate-500">
                  用于当前机器人的测试台（testone）。
                </span>
              </div>
              <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                服务端口（1-65535）
                <input
                  className="h-10 rounded-md border border-slate-300 px-3 text-sm text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                  value={testPortValue}
                  onChange={event => setTestPortValue(event.target.value)}
                  type="number"
                  min={1}
                  max={65535}
                  autoFocus
                />
              </label>
              <footer className="flex justify-end gap-2">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => setTestPortDialog(false)}
                  disabled={testPortBusy}
                >
                  取消
                </button>
                <button className="primary-button" disabled={testPortBusy}>
                  {testPortBusy ? '保存中…' : '启动测试'}
                </button>
              </footer>
            </form>
          </Modal>
          <Modal open={livePortDialog} ariaLabel="配置在线聊天端口">
            <form
              className="grid w-full max-w-sm gap-4 rounded-xl border border-slate-200 bg-white p-5 shadow-[0_20px_58px_rgb(28_26_23/0.22)]"
              onSubmit={event => {
                event.preventDefault()
                void confirmLivePort()
              }}
              onMouseDown={event => event.stopPropagation()}
            >
              <div className="grid gap-1">
                <strong className="text-sm text-ink-950">
                  配置 CBP 服务端口
                </strong>
                <span className="text-xs leading-5 text-slate-500">
                  在线聊天通过 AlemonJS 已登录机器人的 CBP
                  连接通信。保存后将打开登录并启动弹窗。
                </span>
              </div>
              <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                CBP 服务端口（1-65535）
                <input
                  className="h-10 rounded-md border border-slate-300 px-3 text-sm text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                  value={livePortValue}
                  onChange={event => setLivePortValue(event.target.value)}
                  type="number"
                  min={1}
                  max={65535}
                  autoFocus
                />
              </label>
              <footer className="flex justify-end gap-2">
                <button
                  type="button"
                  className="secondary-button"
                  onClick={() => setLivePortDialog(false)}
                  disabled={livePortBusy}
                >
                  取消
                </button>
                <button className="primary-button" disabled={livePortBusy}>
                  {livePortBusy ? '保存中…' : '继续登录'}
                </button>
              </footer>
            </form>
          </Modal>
          <DirectoryPicker
            open={directoryPickerOpen}
            onClose={() => setDirectoryPickerOpen(false)}
            onSelect={paths => void addSelectedDirectories(paths)}
          />
          <DirectoryPicker
            open={gitDestinationPickerOpen}
            multiple={false}
            priority
            onClose={() => setGitDestinationPickerOpen(false)}
            onSelect={paths => {
              setGitDestination(paths[0] ?? '')
              setGitDestinationPickerOpen(false)
            }}
          />
          <GitCloneDialog
            open={gitCloneOpen}
            mode={gitCloneMode}
            root={root}
            destination={
              gitCloneMode === 'package'
                ? backpackDirectory(root)
                : gitDestination
            }
            busy={busy}
            progress={cloneProgress}
            status={cloneStatus}
            onClose={() => setGitCloneOpen(false)}
            onChooseDestination={() => setGitDestinationPickerOpen(true)}
            onConfirm={
              gitCloneMode === 'package'
                ? cloneBackpackPackage
                : cloneRobotRepository
            }
          />
          {renameTarget && (
            <Modal open className="bg-slate-900/40">
              <div className="grid w-full max-w-sm gap-4 rounded-xl bg-white p-5 shadow-2xl">
                <h3 className="text-base font-semibold text-slate-900">
                  重命名对话
                </h3>
                <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                  名称（2-8 个字）
                  <input
                    className="h-10 rounded-md border border-slate-300 px-3 text-sm outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                    value={renameTitle}
                    onChange={event => setRenameTitle(event.target.value)}
                    maxLength={8}
                    autoFocus
                    onKeyDown={event => {
                      if (event.key === 'Enter') {
                        event.preventDefault()
                        if (renameTarget) void renameSession(renameTarget.id)
                      }
                    }}
                  />
                </label>
                <footer className="flex justify-end gap-2">
                  <button
                    className="secondary-button"
                    onClick={() => setRenameTarget(null)}
                  >
                    取消
                  </button>
                  <button
                    className="primary-button"
                    disabled={renameTitle.trim().length < 2}
                    onClick={() => {
                      if (renameTarget) void renameSession(renameTarget.id)
                    }}
                  >
                    确定
                  </button>
                </footer>
              </div>
            </Modal>
          )}
          <section
            className={cn('console-layout', {
              'sidebar-collapsed': sidebarCollapsed
            })}
          >
            <ProjectRail
              feature={systemWindowFeature}
              setupPlugins={setupPlugins}
              projects={projects}
              activeID={activeProjectID}
              agentSessions={agentSessions}
              checking={checking}
              environmentWarning={environmentWarning}
              onFeature={feature => {
                selectSystemFeature(feature)
                if (feature === 'environment') onCheck()
              }}
              onOpenAgent={openAI}
              onPinProject={pinProject}
              onReorderProject={reorderProject}
              onRenameSession={requestRename}
              onArchiveSession={archiveSession}
              onAdd={chooseDirectories}
              onClone={openRobotClone}
              onSelect={id => {
                openProject(id)
                setSystemFeature(null)
                setOutput('')
              }}
              onRemove={removeProject}
            />
            <section className="console-page">
              {workspace}
              {error && <ErrorNotice message={error} onClose={onClearError} />}
              {!robotNavigationHidden &&
                !systemFeature &&
                activeProject &&
                !invalidProject && (
                  <ControlCard
                    page={page}
                    section={section}
                    project={activeProject}
                    buildMode={buildMode}
                    catalog={catalog}
                    catalogTitle={catalogTitle}
                    developerMode={developerMode}
                    agentOpen={aiOpen}
                    onOpenConsole={() => {
                      setConsoleOpen(true)
                      setConsoleMinimized(false)
                      activateFloatingWindow('terminal')
                    }}
                    onOpenAI={openAI}
                    onOpenOps={() => {
                      setOpsOpen(true)
                      setOpsMinimized(false)
                      activateFloatingWindow('ops')
                    }}
                    appLaunching={appLaunching}
                    onOpenApp={() => void openApp()}
                    testLaunching={testLaunching}
                    onOpenTest={() => void openTest()}
                    onOpenLive={() => void openLive()}
                    onPage={selectPage}
                    onSection={openSection}
                    onBuildMode={mode => {
                      markUserNavigation()
                      setBuildMode(mode)
                      setOutput('')
                    }}
                    onCatalog={title => {
                      setCatalogTitle(title)
                      setCatalogItem(null)
                    }}
                    onGit={() => {
                      setGitProject(activeProject)
                      setGitMinimized(false)
                      activateFloatingWindow('git')
                    }}
                  />
                )}
            </section>
          </section>
          {windowControls}
        </section>
      </main>
      {appContentOpen && (
        <AppEmbed
          root={root}
          minimized={appMinimized}
          zIndex={windowLayers.app}
          appPages={botAppPages}
          selectedAppPageID={selectedAppPageID}
          onActivate={() => activateFloatingWindow('app')}
          onMinimize={() => setAppMinimized(true)}
          onClose={() => {
            setAppContentOpen(false)
            setAppMinimized(false)
          }}
        />
      )}
      {testContentOpen && (
        <TestCenterWindow
          root={root}
          minimized={testMinimized}
          zIndex={windowLayers.test}
          onActivate={() => activateFloatingWindow('test')}
          onMinimize={() => setTestMinimized(true)}
          onClose={() => {
            setTestContentOpen(false)
            setTestMinimized(false)
            // 关闭仅断开测试台界面与 WebSocket；测试服务与机器人运行状态
            // 由用户在运行控制中显式管理，不能随窗口生命周期被停止。
          }}
        />
      )}
      {liveContentOpen && (
        <LiveChatWindow
          root={root}
          minimized={liveMinimized}
          zIndex={windowLayers.live}
          onActivate={() => activateFloatingWindow('live')}
          onMinimize={() => setLiveMinimized(true)}
          onClose={() => {
            setLiveContentOpen(false)
            setLiveMinimized(false)
          }}
        />
      )}
      {consoleOpen && (
        <ReadonlyConsole
          open
          minimized={consoleMinimized}
          root={root}
          zIndex={windowLayers.terminal}
          onActivate={() => activateFloatingWindow('terminal')}
          onMinimize={() => setConsoleMinimized(true)}
          onClose={() => {
            setConsoleOpen(false)
            setConsoleMinimized(false)
          }}
        />
      )}
      {gitProject && (
        <RobotGitControl
          project={gitProject}
          minimized={gitMinimized}
          zIndex={windowLayers.git}
          onActivate={() => activateFloatingWindow('git')}
          onMinimize={() => setGitMinimized(true)}
          onClose={() => {
            setGitProject(null)
            setGitMinimized(false)
          }}
        />
      )}
      <PM2LogsPanel
        open={pm2LogsOpen}
        minimized={pm2LogsMinimized}
        root={root}
        zIndex={windowLayers.pm2Logs}
        onActivate={() => activateFloatingWindow('pm2Logs')}
        onMinimize={() => setPM2LogsMinimized(true)}
        onClose={() => {
          setPM2LogsOpen(false)
          setPM2LogsMinimized(false)
        }}
      />
      <PM2ProcessesPanel
        open={pm2ProcessesOpen}
        minimized={pm2ProcessesMinimized}
        root={root}
        zIndex={windowLayers.pm2Status}
        onActivate={() => activateFloatingWindow('pm2Status')}
        onMinimize={() => setPM2ProcessesMinimized(true)}
        onClose={() => {
          setPM2ProcessesOpen(false)
          setPM2ProcessesMinimized(false)
        }}
      />
      <OpsWindow
        open={opsOpen}
        minimized={opsMinimized}
        root={root}
        zIndex={windowLayers.ops}
        onActivate={() => activateFloatingWindow('ops')}
        onMinimize={() => setOpsMinimized(true)}
        onClose={() => {
          setOpsOpen(false)
          setOpsMinimized(false)
        }}
      />
      {Object.entries(systemWindows).map(([feature, state], index) => {
        const windowID: FloatingWindowID = `system:${feature}`
        const zIndex = windowLayers[windowID] ?? 107
        const offset = (index % 6) * 28
        if (usesSystemFeatureSidebar(feature)) {
          return (
            <SidebarWindow
              id={`system:${feature}`}
              key={feature}
              open
              minimized={state.minimized}
              title={systemFeatureLabel(feature, setupPlugins)}
              icon={
                <Settings className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
              }
              sidebarAriaLabel="系统功能"
              onClose={() => closeSystemWindow(feature)}
              onMinimize={() =>
                setSystemWindows(current => ({
                  ...current,
                  [feature]: { minimized: true }
                }))
              }
              zIndex={zIndex}
              onActivate={() => {
                setSystemWindowFeature(feature)
                activateFloatingWindow(windowID)
              }}
              initialPosition={{ left: 216 + offset, top: 200 + offset }}
              width={1080}
              height={720}
            >
              {systemWindowContent(feature, true)}
            </SidebarWindow>
          )
        }
        return (
          <DesktopWindow
            id={`system:${feature}`}
            key={feature}
            open
            minimized={state.minimized}
            title={systemFeatureLabel(feature, setupPlugins)}
            subtitle=""
            icon={
              <Settings className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
            }
            onClose={() => closeSystemWindow(feature)}
            onMinimize={() =>
              setSystemWindows(current => ({
                ...current,
                [feature]: { minimized: true }
              }))
            }
            zIndex={zIndex}
            onActivate={() => {
              setSystemWindowFeature(feature)
              activateFloatingWindow(windowID)
            }}
            initialPosition={{ left: 216 + offset, top: 200 + offset }}
            width={1080}
            height={720}
          >
            <div className="min-h-0 overflow-auto">
              {systemWindowContent(feature)}
            </div>
          </DesktopWindow>
        )
      })}
      {invalidDirectory && (
        <InvalidDirectoryDialog
          path={invalidDirectory}
          onClose={() => setInvalidDirectory('')}
          onCreate={() => {
            dispatch(
              setGuideProject({
                destinationMode: 'custom',
                destination: invalidDirectory
              })
            )
            setInvalidDirectory('')
            onOpenGuide()
          }}
        />
      )}
    </>
  )
}

function ProjectRail({
  feature,
  setupPlugins,
  projects,
  activeID,
  agentSessions,
  checking,
  environmentWarning,
  onFeature,
  onAdd,
  onClone,
  onSelect,
  onRemove,
  onOpenAgent,
  onPinProject,
  onReorderProject,
  onRenameSession,
  onArchiveSession
}: {
  feature: SystemFeature | null
  setupPlugins: SetupPlugin[]
  projects: Project[]
  activeID: string
  agentSessions: Array<{
    id: string
    title: string
    root: string
    updated: string
  }>
  checking: boolean
  environmentWarning: boolean
  onFeature: (feature: SystemFeature) => void
  onAdd: () => void
  onClone: () => void
  onSelect: (id: string) => void
  onRemove: (id: string) => void
  onOpenAgent: (sessionID?: string) => void
  onPinProject: (id: string) => void
  onReorderProject: (sourceID: string, targetID: string) => void
  onRenameSession: (id: string, title: string) => void
  onArchiveSession: (id: string) => void
}) {
  const [draggingProjectID, setDraggingProjectID] = useStoreState<
    string | null
  >(null)
  const [dragTargetID, setDragTargetID] = useStoreState<string | null>(null)
  const longPressTimer = useRef<number | null>(null)
  const ignoreProjectSelect = useRef(false)
  const activePlugins = setupPlugins.filter(
    item => item.enabled && !item.online
  )
  const [canManageAccounts, setCanManageAccounts] = useStoreState(false)
  const [authRevision, setAuthRevision] = useStoreState(0)
  const isMobile = useIsMobileViewport()
  const [mobileOpen, setMobileOpen] = useState<{
    robot: boolean
    plugins: boolean
    system: boolean
  }>({ robot: false, plugins: false, system: false })
  const toggleMobileGroup = (group: 'robot' | 'plugins' | 'system') =>
    setMobileOpen(current => ({ ...current, [group]: !current[group] }))
  useEffect(() => {
    const refreshAuth = () => setAuthRevision(value => value + 1)
    window.addEventListener('alx:auth-changed', refreshAuth)
    return () => window.removeEventListener('alx:auth-changed', refreshAuth)
  }, [setAuthRevision])
  useEffect(() => {
    let active = true
    void fetch('/api/v1/auth/status', { credentials: 'same-origin' })
      .then(async response => {
        if (!response.ok) return null
        return (await response.json()) as {
          enabled?: boolean
          superAdmin?: boolean
        }
      })
      .then(status => {
        if (active)
          setCanManageAccounts(Boolean(status?.enabled && status.superAdmin))
      })
      .catch(() => active && setCanManageAccounts(false))
    return () => {
      active = false
    }
  }, [authRevision, setCanManageAccounts])
  const clearLongPress = () => {
    if (longPressTimer.current === null) return
    window.clearTimeout(longPressTimer.current)
    longPressTimer.current = null
  }
  const startProjectDrag = (
    event: ReactPointerEvent<HTMLButtonElement>,
    projectID: string
  ) => {
    if (event.button !== 0) return
    clearLongPress()
    longPressTimer.current = window.setTimeout(() => {
      longPressTimer.current = null
      ignoreProjectSelect.current = true
      setDraggingProjectID(projectID)
      setDragTargetID(projectID)
    }, 380)
  }
  const finishProjectDrag = () => {
    clearLongPress()
    if (draggingProjectID && dragTargetID && draggingProjectID !== dragTargetID)
      onReorderProject(draggingProjectID, dragTargetID)
    if (draggingProjectID) {
      setDraggingProjectID(null)
      setDragTargetID(null)
      window.setTimeout(() => {
        ignoreProjectSelect.current = false
      }, 0)
    }
  }
  const robotActions = (
    <div className="flex items-center gap-0.5">
      <button
        className="inline-flex size-6 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-200/60 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300"
        onClick={onClone}
        aria-label="从 Git 克隆机器人"
        title="从 Git 克隆机器人"
      >
        <GitBranch className="size-3.5" />
      </button>
      <button
        className="inline-flex size-6 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-200/60 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300"
        onClick={onAdd}
        aria-label="添加本地机器人目录"
        title="添加本地机器人目录"
      >
        <Plus className="size-3.5" />
      </button>
    </div>
  )
  const robotList = (
    <div
      className="grid content-start h-full gap-1.5 overflow-auto px-1.5 pb-2"
      onPointerUp={finishProjectDrag}
      onPointerCancel={finishProjectDrag}
    >
      {projects.map(project => (
        <ProjectItem
          active={project.id === activeID}
          dragging={project.id === draggingProjectID}
          dragTarget={
            project.id === dragTargetID && project.id !== draggingProjectID
          }
          key={project.id}
          project={project}
          agentSessions={agentSessions}
          onSelect={id => {
            if (!ignoreProjectSelect.current) onSelect(id)
          }}
          onRemove={onRemove}
          onOpenAgent={onOpenAgent}
          onPin={onPinProject}
          onRename={onRenameSession}
          onArchive={onArchiveSession}
          onDragStart={startProjectDrag}
          onDragTarget={id => {
            if (draggingProjectID && id !== draggingProjectID)
              setDragTargetID(id)
          }}
        />
      ))}
      {!projects.length && (
        <p className="px-2 py-4 text-center text-xs text-slate-400">
          添加机器人目录开始管理
        </p>
      )}
    </div>
  )
  const pluginNav = (
    <nav className="grid gap-0.5">
      {activePlugins.map(item => (
        <button
          className={cn(
            'flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
            feature === `setup:${item.id}`
              ? 'workspace-nav-active'
              : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
          )}
          key={item.id}
          onClick={() => onFeature(`setup:${item.id}`)}
        >
          <i className="inline-flex size-4 items-center justify-center not-italic">
            {setupPluginIcon(item.navigation.icon)}
          </i>
          <span className="min-w-0 flex-1 truncate">
            {item.navigation.label || item.name}
          </span>
        </button>
      ))}
    </nav>
  )
  const systemNav = (
    <nav className="grid gap-0.5">
      {coreFeatureCatalog
        .filter(item => item.id !== 'accounts' || canManageAccounts)
        .map(item => (
          <button
            className={cn(
              'flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
              feature === item.id
                ? 'workspace-nav-active'
                : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
            )}
            key={item.id}
            onClick={() => onFeature(item.id)}
          >
            <i className="inline-flex size-4 items-center justify-center not-italic">
              {item.icon}
            </i>
            <span className="min-w-0 flex-1 truncate">{item.label}</span>
            {item.status && (
              <small className="text-[10px] text-slate-400">
                {item.status}
              </small>
            )}
          </button>
        ))}
      <button
        className={cn(
          'flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
          feature === 'tasks'
            ? 'workspace-nav-active'
            : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
        )}
        onClick={() => onFeature('tasks')}
        aria-label="操作记录"
        title="当前目录操作记录"
      >
        <i className="inline-flex size-4 shrink-0 items-center justify-center not-italic">
          <ClipboardList className="size-4" />
        </i>
        <span className="min-w-0 flex-1 truncate">日志</span>
      </button>
      <button
        className={cn(
          'flex min-h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
          feature === 'environment'
            ? 'workspace-nav-active'
            : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
        )}
        onClick={() => onFeature('environment')}
        aria-label="全局环境"
        title="查看并检查全局环境"
      >
        <i className="inline-flex size-4 shrink-0 items-center justify-center not-italic">
          {checking ? (
            <Loader2 className="size-4 animate-spin" />
          ) : environmentWarning ? (
            <AlertTriangle className="size-4" />
          ) : (
            <CheckCircle2 className="size-4" />
          )}
        </i>
        <span className="min-w-0 flex-1 truncate">环境</span>
      </button>
    </nav>
  )
  return (
    <aside className="project-rail flex min-h-0 min-w-0 flex-col border-r border-slate-200 bg-slate-50">
      {isMobile ? (
        <>
          <MobileRailGroup
            title="机器人"
            count={projects.length}
            icon={<Bot className="size-4" />}
            actions={robotActions}
            open={mobileOpen.robot}
            onToggle={() => toggleMobileGroup('robot')}
          >
            {robotList}
          </MobileRailGroup>
          {activePlugins.length > 0 && (
            <MobileRailGroup
              title="插件"
              count={activePlugins.length}
              icon={<Plug className="size-4" />}
              open={mobileOpen.plugins}
              onToggle={() => toggleMobileGroup('plugins')}
            >
              {pluginNav}
            </MobileRailGroup>
          )}
          <MobileRailGroup
            title="系统"
            icon={<Settings className="size-4" />}
            open={mobileOpen.system}
            onToggle={() => toggleMobileGroup('system')}
          >
            {systemNav}
          </MobileRailGroup>
        </>
      ) : (
        <>
          <section
            className="order-3 border-t border-slate-200 px-2.5 py-2 dark:border-slate-700"
            aria-label="系统功能目录"
          >
            <header className="px-1.5 pb-1 text-[0.7rem] font-semibold uppercase tracking-wider text-slate-400 dark:text-slate-500">
              系统
            </header>
            {systemNav}
          </section>
          {activePlugins.length > 0 && (
            <section
              className="order-2 border-t border-slate-200 px-2.5 py-2 dark:border-slate-700"
              aria-label="已加载插件"
            >
              <header className="flex  px-1.5 pb-1">
                <span className="text-[0.7rem] font-semibold uppercase tracking-wider text-slate-400 dark:text-slate-500">
                  插件
                  <span className="ml-1.5 text-slate-300 dark:text-slate-600">
                    {activePlugins.length}
                  </span>
                </span>
              </header>
              {pluginNav}
            </section>
          )}
          <section className="order-1 flex min-h-0 flex-1 flex-col">
            <header className="flex min-h-9 items-center justify-between px-2.5 py-1.5">
              <span className="text-[0.7rem] font-semibold uppercase tracking-wider text-slate-400 dark:text-slate-500">
                机器人
                <span className="ml-1.5 text-slate-300 dark:text-slate-600">
                  {projects.length}
                </span>
              </span>
              {robotActions}
            </header>
            {robotList}
          </section>
        </>
      )}
    </aside>
  )
}

function MobileRailGroup({
  title,
  count,
  icon,
  actions,
  open,
  onToggle,
  children
}: {
  title: string
  count?: number
  icon: ReactNode
  actions?: ReactNode
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  return (
    <section className="border-b border-slate-200 dark:border-slate-700">
      <div className="flex min-h-10 items-center gap-0.5 py-0.5 pl-1 pr-2">
        <button
          type="button"
          className={cn(
            'mobile-rail-group-toggle flex min-h-9 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-[0.8rem] font-semibold transition-colors',
            open
              ? 'workspace-nav-active'
              : 'text-slate-700 hover:bg-slate-200/40 dark:text-slate-200 dark:hover:bg-slate-700/40'
          )}
          onClick={onToggle}
          aria-expanded={open}
        >
          <i className="inline-flex size-4 shrink-0 items-center justify-center not-italic">
            {icon}
          </i>
          <span className="min-w-0 flex-1 truncate">{title}</span>
          {typeof count === 'number' && (
            <small className="text-[10px] text-slate-300 dark:text-slate-600">
              {count}
            </small>
          )}
          <ChevronRight
            className={cn(
              'size-4 shrink-0 text-slate-400 transition-transform',
              open && 'rotate-90'
            )}
          />
        </button>
        {actions}
      </div>
      {open && (
        <div className="mobile-rail-group-content px-2 pb-2">{children}</div>
      )}
    </section>
  )
}

function GitCloneDialog({
  open,
  mode,
  root,
  destination,
  busy,
  progress,
  status,
  onClose,
  onChooseDestination,
  onConfirm
}: {
  open: boolean
  mode: 'robot' | 'package'
  root: string
  destination: string
  busy: boolean
  progress: number
  status: string
  onClose: () => void
  onChooseDestination: () => void
  onConfirm: (
    repository: string,
    branch: string,
    name: string,
    mirror: string,
    depth: number
  ) => Promise<void>
}) {
  const [repository, setRepository] = useStoreState('')
  const [branch, setBranch] = useStoreState('')
  const [branches, setBranches] = useStoreState<string[]>([])
  const [branchesLoading, setBranchesLoading] = useStoreState(false)
  const [name, setName] = useStoreState('')
  const [mirror, setMirror] = useStoreState('official')
  const [depth, setDepth] = useStoreState(1)
  const [connection, setConnection] = useStoreState<'ssh' | 'https'>('https')
  const [sshKeys, setSSHKeys] = useStoreState<Array<{ name: string }>>([])
  const [sshLoading, setSSHLoading] = useStoreState(false)
  const [target, setTarget] = useStoreState<{
    path: string
    exists: boolean
  } | null>(null)
  const [targetError, setTargetError] = useStoreState('')
  useEffect(() => {
    if (open) {
      setRepository('')
      setBranch(mode === 'package' ? 'release' : '')
      setBranches([])
      setBranchesLoading(false)
      setName('')
      setMirror('official')
      setDepth(1)
      setConnection('https')
      setSSHKeys([])
      setTarget(null)
      setTargetError('')
    }
  }, [
    open,
    mode,
    setBranch,
    setBranches,
    setBranchesLoading,
    setConnection,
    setDepth,
    setMirror,
    setName,
    setRepository,
    setSSHKeys,
    setTarget,
    setTargetError
  ])
  useEffect(() => {
    if (!open) return
    let active = true
    setSSHLoading(true)
    void fetch('/api/v1/system/ssh')
      .then(async response => {
        const data = (await response.json()) as {
          keys?: Array<{ name: string }>
          error?: string
        }
        if (!response.ok) throw new Error(data.error || '无法读取 SSH 状态。')
        return data.keys ?? []
      })
      .then(keys => {
        if (!active) return
        setSSHKeys(keys)
        if (keys.length) setConnection('ssh')
      })
      .catch(() => {
        if (active) setSSHKeys([])
      })
      .finally(() => {
        if (active) setSSHLoading(false)
      })
    return () => {
      active = false
    }
  }, [open, setConnection, setSSHKeys, setSSHLoading])
  useEffect(() => {
    if (!open || !destination || !repository.trim() || !name.trim()) {
      setTarget(null)
      setTargetError('')
      return
    }
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      const checkPath =
        mode === 'package'
          ? '/api/v1/robot/packages/git-clone/check'
          : '/api/v1/robot/git-clone/check'
      const checkParameters = new URLSearchParams({ repository, name })
      checkParameters.set(
        mode === 'package' ? 'root' : 'destination',
        mode === 'package' ? root : destination
      )
      void fetch(`${checkPath}?${checkParameters}`, {
        signal: controller.signal
      })
        .then(async response => {
          const data = (await response.json()) as {
            path?: string
            exists?: boolean
            error?: string
          }
          if (!response.ok) throw new Error(data.error || '无法检查目标目录。')
          return data
        })
        .then(data => {
          setTarget({ path: data.path ?? '', exists: Boolean(data.exists) })
          setTargetError('')
        })
        .catch(reason => {
          if (!(
            reason instanceof DOMException && reason.name === 'AbortError'
          )) {
            setTarget(null)
            setTargetError(operationErrorMessage(reason, '无法检查目标目录。'))
          }
        })
    }, 260)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [
    destination,
    mode,
    name,
    open,
    repository,
    root,
    setTarget,
    setTargetError
  ])
  useEffect(() => {
    const value = repository.trim()
    if (!open || !isCompleteGitRepositoryURL(value)) {
      setBranches([])
      setBranchesLoading(false)
      return
    }
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      setBranchesLoading(true)
      void fetch(
        `/api/v1/robot/git-clone/branches?${new URLSearchParams({ repository: value })}`,
        { signal: controller.signal }
      )
        .then(async response => {
          const data = (await response.json()) as {
            branches?: string[]
            defaultBranch?: string
            error?: string
          }
          if (!response.ok) throw new Error(data.error || '无法读取远程分支。')
          return data
        })
        .then(data => {
          setBranches(data.branches ?? [])
          setBranch(current => {
            if (mode === 'package') return current || 'release'
            return data.branches?.includes(current)
              ? current
              : (data.defaultBranch ?? data.branches?.[0] ?? '')
          })
        })
        .catch(() => {
          // 地址输入过程中或私有仓库尚未授权时保持静默，不打断用户填写。
          if (!controller.signal.aborted) setBranches([])
        })
        .finally(() => {
          if (!controller.signal.aborted) setBranchesLoading(false)
        })
    }, 500)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [mode, open, repository, setBranch, setBranches, setBranchesLoading])
  if (!open) return null
  const usesSSH = /^(git@|ssh:\/\/)/.test(repository.trim())
  return (
    <Modal
      open
      ariaLabel={mode === 'package' ? '克隆插件包' : '从 Git 克隆机器人'}
    >
      <section
        className="git-dialog git-clone-dialog grid max-h-[min(720px,calc(100vh-32px))] w-full max-w-xl grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_22px_58px_rgb(28_26_23/0.25)]"
        role="dialog"
        aria-label={mode === 'package' ? '克隆插件包' : '从 Git 克隆机器人'}
      >
        <header className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
          <div className="grid gap-1">
            <strong className="text-sm font-semibold text-slate-900">
              {mode === 'package' ? '克隆插件包' : '添加 Git 仓库'}
            </strong>
            <span className="text-xs text-slate-500">
              {mode === 'package'
                ? '克隆到当前机器人的背包，完成后即可管理和启用。'
                : '下载完成后会自动加入机器人目录。'}
            </span>
          </div>
          <button
            className="icon-button size-8 p-0"
            onClick={onClose}
            aria-label="关闭"
          >
            <X className="size-4" />
          </button>
        </header>
        <div className="grid gap-3 overflow-auto p-4">
          <section aria-label="仓库连接方式">
            <header className="flex items-center justify-between gap-2">
              <small
                className="truncate text-[11px] text-slate-500"
                title={
                  connection === 'ssh' && !sshLoading && !sshKeys.length
                    ? '未配置 SSH 密钥；请在顶部 SSH 管理中生成并添加公钥，或改用 HTTPS。'
                    : connection === 'https'
                      ? 'HTTPS 可直接使用；访问私有仓库时，需要在代码平台完成在线授权。'
                      : undefined
                }
              >
                {sshLoading
                  ? '正在检查 SSH…'
                  : connection === 'ssh' && !sshKeys.length
                    ? 'SSH 未配置'
                    : connection === 'https'
                      ? 'HTTPS 已选择'
                      : ''}
              </small>
            </header>
            <Tabs
              ariaLabel="仓库连接方式"
              items={[
                {
                  id: 'ssh',
                  icon: <KeyRound className="size-3.5" />,
                  label: 'SSH',
                  meta: sshKeys.length ? '推荐' : undefined
                },
                {
                  id: 'https',
                  icon: <Globe2 className="size-3.5" />,
                  label: 'HTTPS'
                }
              ]}
              onChange={setConnection}
              value={connection}
              variant="segmented"
            />
          </section>
          <section className="grid gap-3">
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              <span className="flex min-w-0 items-center justify-between gap-2">
                <span>仓库地址</span>
                {usesSSH && !sshLoading && !sshKeys.length && (
                  <small
                    className="truncate font-normal text-red-700"
                    title="请先配置 SSH 密钥，或改用 HTTPS 地址。"
                  >
                    未配置 SSH 密钥
                  </small>
                )}
              </span>
              <input
                className="h-9 rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                autoFocus
                value={repository}
                onChange={event => {
                  const value = event.target.value
                  setRepository(value)
                  setBranch(mode === 'package' ? 'release' : '')
                  if (/^(git@|ssh:\/\/)/.test(value.trim()))
                    setConnection('ssh')
                  const derived =
                    value
                      .trim()
                      .replace(/\/$/, '')
                      .split('/')
                      .pop()
                      ?.replace(/\.git$/, '') ?? ''
                  setName(derived)
                }}
                placeholder={
                  connection === 'ssh'
                    ? 'git@github.com:组织/机器人仓库.git'
                    : 'https://github.com/组织/机器人仓库.git'
                }
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="grid gap-1 text-xs font-semibold text-slate-600">
                {mode === 'package' ? '插件分支' : '分支'}
                {branchesLoading ? '（正在读取…）' : '（可选）'}
                <select
                  className="h-9 rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100 disabled:bg-slate-100"
                  value={branch}
                  onChange={event => setBranch(event.target.value)}
                  disabled={!branches.length || branchesLoading}
                >
                  {mode === 'package' ? (
                    <option value="release">发布分支（release）</option>
                  ) : (
                    <option value="">默认分支</option>
                  )}
                  {branches
                    .filter(item => mode !== 'package' || item !== 'release')
                    .map(item => (
                      <option key={item} value={item}>
                        {formatBranchLabel(item)}
                      </option>
                    ))}
                </select>
              </label>
              <label className="grid gap-1 text-xs font-semibold text-slate-600">
                克隆深度
                <select
                  className="h-9 rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                  value={depth}
                  onChange={event => setDepth(Number(event.target.value))}
                >
                  <option value={1}>仅最新提交（推荐）</option>
                  <option value={50}>最近 50 条提交</option>
                  <option value={200}>最近 200 条提交</option>
                  <option value={0}>完整历史</option>
                </select>
              </label>
              <label className="grid gap-1 text-xs font-semibold text-slate-600">
                下载来源
                <select
                  className="h-9 rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                  value={mirror}
                  onChange={event => setMirror(event.target.value)}
                >
                  <option value="official">Git 官方（推荐）</option>
                  <option value="gh-proxy">GitHub 加速 · gh-proxy</option>
                  <option value="ghproxy-net">GitHub 加速 · ghproxy.net</option>
                </select>
              </label>
            </div>
          </section>
          <section className="grid gap-3 sm:grid-cols-2">
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              {mode === 'package' ? '存放位置' : '所在文件夹'}
              {mode === 'package' ? (
                <span
                  className="h-9 truncate rounded-md border border-slate-200 bg-slate-50 px-2.5 leading-9 text-sm font-normal text-slate-600"
                  title={destination}
                >
                  当前机器人背包（packages）
                </span>
              ) : (
                <button
                  type="button"
                  className="h-9 truncate rounded-md border border-slate-300 bg-white px-2.5 text-left text-sm font-normal text-slate-700"
                  onClick={onChooseDestination}
                >
                  {gitDestinationLabel(destination)}
                </button>
              )}
            </label>
            <label className="grid gap-1 text-xs font-semibold text-slate-600">
              <span className="flex min-w-0 items-center justify-between gap-2">
                <span>新目录名称</span>
                {target?.exists ? (
                  <small
                    className="truncate font-normal text-red-700"
                    title={`目标已存在：${target.path}`}
                  >
                    目标已存在
                  </small>
                ) : target?.path ? null : targetError ? (
                  <small
                    className="truncate font-normal text-red-700"
                    title={targetError}
                  >
                    无法检查目标
                  </small>
                ) : null}
              </span>
              <input
                className={cn(
                  'h-9 rounded-md border bg-white px-2.5 text-sm font-normal text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100',
                  target?.exists || targetError
                    ? 'border-red-400 focus:ring-red-100'
                    : 'border-slate-300'
                )}
                value={name}
                onChange={event => setName(event.target.value)}
                placeholder="默认使用仓库名"
              />
            </label>
          </section>
        </div>
        <footer className="flex justify-end gap-2 border-t border-slate-200 px-4 py-3">
          {busy && (
            <div className="mr-auto grid min-w-44 gap-1 self-center">
              <div className="flex justify-between text-[11px] text-slate-500">
                <span>{status}</span>
                <span>{progress}%</span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-slate-200">
                <div
                  className="h-full rounded-full bg-brand-600 transition-[width] duration-500"
                  style={{ width: `${Math.max(8, progress)}%` }}
                />
              </div>
            </div>
          )}
          <button className="secondary-button" onClick={onClose}>
            取消
          </button>
          <button
            className="primary-button gap-1.5"
            disabled={
              busy ||
              ((connection === 'ssh' || usesSSH) &&
                !sshLoading &&
                !sshKeys.length) ||
              !repository.trim() ||
              !destination ||
              !name.trim() ||
              !target ||
              target.exists ||
              Boolean(targetError)
            }
            onClick={() =>
              void onConfirm(
                repository.trim(),
                branch.trim(),
                name.trim(),
                mirror,
                depth
              )
            }
          >
            {busy
              ? '正在下载…'
              : mode === 'package'
                ? '克隆到背包'
                : '克隆并添加'}
          </button>
        </footer>
      </section>
    </Modal>
  )
}

function gitDestinationLabel(path: string) {
  return path || '选择存放位置'
}

function backpackDirectory(root: string) {
  if (!root) return ''
  const cleaned = root.replace(/[\\/]+$/, '')
  const separator = cleaned.includes('\\') ? '\\' : '/'
  return `${cleaned}${separator}packages`
}

function formatBranchLabel(branch: string) {
  const limit = 48
  return branch.length > limit ? `${branch.slice(0, limit - 1)}…` : branch
}

function isCompleteGitRepositoryURL(value: string) {
  return /^(https:\/\/(github\.com|gitee\.com)\/[\w.-]+\/[\w.-]+(?:\.git)?\/?|git@(github\.com|gitee\.com):[\w.-]+\/[\w.-]+(?:\.git)?)$/.test(
    value
  )
}

function ProjectItem({
  project,
  active,
  dragging,
  dragTarget,
  agentSessions,
  onSelect,
  onRemove,
  onOpenAgent,
  onPin,
  onRename,
  onArchive,
  onDragStart,
  onDragTarget
}: {
  project: Project
  active: boolean
  dragging: boolean
  dragTarget: boolean
  agentSessions: Array<{
    id: string
    title: string
    root: string
    updated: string
  }>
  onSelect: (id: string) => void
  onRemove: (id: string) => void
  onOpenAgent: (sessionID?: string) => void
  onPin: (id: string) => void
  onRename: (id: string, title: string) => void
  onArchive: (id: string) => void
  onDragStart: (
    event: ReactPointerEvent<HTMLButtonElement>,
    projectID: string
  ) => void
  onDragTarget: (projectID: string) => void
}) {
  const [validate, { data }] = useLazyRobotProjectQuery()
  const [recordsOpen, setRecordsOpen] = useStoreState(false)
  const [moreOpen, setMoreOpen] = useStoreState(false)
  const [ctxMenu, setCtxMenu] = useStoreState<{
    id: string
    title: string
    x: number
    y: number
  } | null>(null)
  const [projectMenu, setProjectMenu] = useStoreState<{
    x: number
    y: number
  } | null>(null)
  const moreRef = useRef<HTMLDivElement | null>(null)
  const ctxRef = useRef<HTMLDivElement | null>(null)
  const projectMenuRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    void validate(project.path)
  }, [project.path, validate])
  useEffect(() => {
    const close = (event: globalThis.MouseEvent) => {
      if (moreRef.current && !moreRef.current.contains(event.target as Node)) {
        setMoreOpen(false)
      }
      if (ctxRef.current && !ctxRef.current.contains(event.target as Node)) {
        setCtxMenu(null)
      }
      if (
        projectMenuRef.current &&
        !projectMenuRef.current.contains(event.target as Node)
      ) {
        setProjectMenu(null)
      }
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [setCtxMenu, setMoreOpen, setProjectMenu])
  const invalid = data?.valid === false
  const ownSessions = agentSessions.filter(item => item.root === project.path)
  return (
    <article
      className={cn(
        'workspace-project-item group relative rounded-lg transition-colors',
        dragging && 'workspace-project-dragging',
        dragTarget && 'workspace-project-drop-target',
        active
          ? invalid
            ? 'workspace-project-invalid'
            : 'workspace-project-active'
          : 'hover:bg-slate-200/40 dark:hover:bg-slate-800/40',
        invalid ? 'bg-amber-50 dark:bg-amber-950/20' : ''
      )}
    >
      <button
        className="flex w-full items-center gap-2 py-1.5 pl-2 pr-12 text-left"
        onClick={() => onSelect(project.id)}
        onPointerDown={event => onDragStart(event, project.id)}
        onPointerEnter={() => onDragTarget(project.id)}
        onContextMenu={event => {
          event.preventDefault()
          setMoreOpen(false)
          setProjectMenu({ x: event.clientX, y: event.clientY })
        }}
        title={invalid ? data.error || project.path : project.path}
      >
        <Bot
          className={cn(
            'size-3.5 shrink-0',
            invalid ? 'text-amber-500' : 'text-slate-400 dark:text-slate-500'
          )}
        />
        <span
          className={cn(
            'min-w-0 flex-1 truncate text-xs',
            invalid
              ? 'text-amber-700 dark:text-amber-400'
              : active
                ? 'font-medium text-(--theme-accent-text)'
                : 'text-slate-700 dark:text-slate-300'
          )}
        >
          {project.name}
        </span>
      </button>
      <div className="absolute right-1.5 top-2 flex items-center gap-0.5">
        <button
          className={cn(
            'inline-flex size-7 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-200/60 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300',
            recordsOpen &&
              'bg-slate-200/60 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
          )}
          onClick={() => setRecordsOpen(value => !value)}
          aria-expanded={recordsOpen}
          aria-label={`${project.name} 的 Agent 对话记录`}
          title="Agent 对话记录"
        >
          <MessageSquare className="size-3.5" />
        </button>
        <div ref={moreRef} className="relative">
          <button
            className={cn(
              'inline-flex size-7 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-200/60 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300',
              moreOpen &&
                'bg-slate-200/60 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
            )}
            onClick={() => setMoreOpen(value => !value)}
            aria-expanded={moreOpen}
            aria-label={`${project.name} 的更多操作`}
            title="更多操作"
          >
            <MoreVertical className="size-3.5" />
          </button>
          {moreOpen && (
            <div
              className="workspace-context-menu absolute right-0 top-7 z-20"
              role="menu"
            >
              <button
                className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-slate-600 transition hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
                onClick={() => {
                  onPin(project.id)
                  setMoreOpen(false)
                }}
              >
                <Pin className="size-3.5 text-slate-400" />
                置顶
              </button>
              <button
                className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-red-600 transition hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/30"
                onClick={() => {
                  onRemove(project.id)
                  setMoreOpen(false)
                }}
              >
                <Trash2 className="size-3.5" />
                移除
              </button>
            </div>
          )}
        </div>
      </div>
      {recordsOpen && (
        <div className="workspace-project-records mt-1 grid gap-0.5 pt-1">
          {ownSessions.length === 0 ? (
            <p className="px-1 py-0.5 text-[0.72rem] text-slate-400 dark:text-slate-500">
              还没有对话记录
            </p>
          ) : (
            ownSessions.map(item => (
              <div
                className="workspace-session-row flex min-w-0 items-center gap-0.5"
                key={item.id}
              >
                <button
                  className="flex min-h-7 min-w-0 flex-1 items-center gap-1.5 rounded px-1.5 text-left text-xs text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-700/60"
                  onClick={() => onOpenAgent(item.id)}
                  onContextMenu={event => {
                    event.preventDefault()
                    setCtxMenu({
                      id: item.id,
                      title: item.title,
                      x: event.clientX,
                      y: event.clientY
                    })
                  }}
                  title={item.title}
                >
                  <MessageSquare className="size-3.5 shrink-0 text-slate-300 dark:text-slate-600" />
                  <span className="min-w-0 flex-1 truncate">{item.title}</span>
                </button>
                <button
                  className="inline-flex size-7 shrink-0 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300"
                  onClick={event => {
                    const rect = event.currentTarget.getBoundingClientRect()
                    setCtxMenu({
                      id: item.id,
                      title: item.title,
                      x: rect.right,
                      y: rect.bottom
                    })
                  }}
                  aria-label={`${item.title} 的更多操作`}
                  title="更多操作"
                >
                  <MoreVertical className="size-3.5" />
                </button>
              </div>
            ))
          )}
        </div>
      )}
      {ctxMenu && (
        <div
          ref={ctxRef}
          className="workspace-context-menu fixed z-200"
          style={{ left: ctxMenu.x, top: ctxMenu.y }}
        >
          <button
            className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-slate-600 transition hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
            onClick={() => {
              onRename(ctxMenu.id, ctxMenu.title)
              setCtxMenu(null)
            }}
          >
            <Pencil className="size-3.5 text-slate-400" />
            重命名
          </button>
          <button
            className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-slate-600 transition hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
            onClick={() => {
              onArchive(ctxMenu.id)
              setCtxMenu(null)
            }}
          >
            <Archive className="size-3.5 text-slate-400" />
            归档
          </button>
        </div>
      )}
      {projectMenu && (
        <div
          ref={projectMenuRef}
          className="workspace-context-menu fixed z-200"
          style={{ left: projectMenu.x, top: projectMenu.y }}
          role="menu"
          aria-label={`${project.name} 的操作`}
        >
          <button
            className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-slate-600 transition hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
            onClick={() => {
              onSelect(project.id)
              setProjectMenu(null)
            }}
          >
            <Bot className="size-3.5 text-slate-400" />
            打开
          </button>
          <button
            className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-slate-600 transition hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700"
            onClick={() => {
              onPin(project.id)
              setProjectMenu(null)
            }}
          >
            <Pin className="size-3.5 text-slate-400" />
            {project.pinned ? '取消置顶' : '置顶'}
          </button>
          <button
            className="flex min-h-8 items-center gap-2 rounded px-2 text-left text-xs text-red-600 transition hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/30"
            onClick={() => {
              onRemove(project.id)
              setProjectMenu(null)
            }}
          >
            <Trash2 className="size-3.5" />
            移除
          </button>
        </div>
      )}
    </article>
  )
}
function McpControl() {
  const [open, setOpen] = useStoreState(false)
  const [transport, setTransport] = useStoreState<'stdio' | 'http'>('stdio')
  const [copied, setCopied] = useStoreState(false)
  const { data: mcpStatus, refetch: refetchMCP } = useSystemMcpQuery()
  const mcpRunning = mcpStatus?.running ?? false
  const stdioConfig =
    '{\n  "mcpServers": {\n    "alemonx": {\n      "command": "alx",\n      "args": ["mcp"]\n    }\n  }\n}'
  const httpCommand =
    "MCP_TOKEN='请生成高强度随机值' alx --mcp-port 17391 mcp-http"
  const copy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      setCopied(false)
    }
  }
  const http = transport === 'http'
  useEffect(() => {
    const onSystemEvent = (event: Event) => {
      try {
        const envelope = (
          event as CustomEvent<{ topic?: string; data?: unknown }>
        ).detail
        if (envelope?.topic !== 'system') return
        const payload = envelope.data as { type?: string; running?: boolean }
        if (
          payload.type === 'mcp.changed' &&
          typeof payload.running === 'boolean'
        )
          void refetchMCP()
      } catch {
        // A malformed application event does not affect the last known state.
      }
    }
    window.addEventListener('alx:unified-event', onSystemEvent)
    return () => window.removeEventListener('alx:unified-event', onSystemEvent)
  }, [refetchMCP])
  useEffect(() => {
    const closeWhenAnotherToolOpens = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== 'mcp') setOpen(false)
    }
    window.addEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
    return () =>
      window.removeEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
  }, [setOpen])
  return (
    <div className="mcp-control relative">
      <button
        className={cn(
          'mcp-control-button inline-flex min-h-8 items-center gap-1.5 rounded-lg border px-2.5 text-xs font-semibold transition',
          mcpRunning
            ? 'border-blue-200 bg-blue-50 text-blue-700 hover:bg-blue-100 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-300 dark:hover:bg-blue-950/70'
            : 'border-slate-200 bg-slate-100 text-slate-500 hover:bg-slate-200 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400 dark:hover:bg-slate-700'
        )}
        onClick={() =>
          setOpen(value => {
            const next = !value
            if (next) {
              void refetchMCP()
              window.dispatchEvent(
                new CustomEvent('alx:top-tool-open', { detail: 'mcp' })
              )
            }
            return next
          })
        }
        aria-expanded={open}
        title={
          mcpRunning
            ? '本机 MCP HTTP 服务正在运行'
            : '本机 MCP HTTP 服务未运行；点击查看连接方式'
        }
      >
        <i className="inline-flex size-4 items-center justify-center rounded-full bg-white text-[10px] not-italic dark:bg-slate-900">
          {mcpRunning ? (
            <CircleCheckBig className="size-3.5" />
          ) : (
            <Info className="size-3.5" />
          )}
        </i>
        <span>MCP</span>
      </button>
      {open && (
        <section
          className="topbar-popover mcp-popover absolute right-0 top-[calc(100%+8px)] z-50 grid w-[min(390px,calc(100vw-32px))] gap-2.5 rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700 dark:bg-slate-900"
          role="dialog"
          aria-label="连接 MCP"
        >
          <header className="flex items-start justify-between gap-3">
            <div className="grid gap-0.5">
              <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                连接 Codex / 自定义 MCP
              </strong>
              <small className="text-xs font-semibold text-blue-600 dark:text-blue-400">
                两种标准传输均可用
              </small>
            </div>
            <button
              className="topbar-popover-close size-7"
              onClick={() => setOpen(false)}
              aria-label="关闭 MCP 说明"
            >
              <X className="size-4" />
            </button>
          </header>
          <div
            className={cn(
              'mcp-status-line m-0 flex items-center gap-1.5 rounded-lg border px-2.5 py-2 text-xs',
              mcpRunning
                ? 'border-blue-100 bg-blue-50 text-blue-700 dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-300'
                : 'border-slate-200 bg-slate-50 text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400'
            )}
          >
            <div
              className={cn(
                'inline-block size-1.5 shrink-0 rounded-full',
                mcpRunning ? 'bg-blue-500' : 'bg-slate-400'
              )}
            />
            <div>
              <div className="flex-1">
                {mcpRunning
                  ? '本机 MCP HTTP 服务正在运行（端口 17391）'
                  : '本机 MCP HTTP 服务未运行'}
              </div>
              <div>{mcpRunning ? '' : '本机 MCP HTTP 不会随后台自动常驻'}</div>
            </div>
            <button
              type="button"
              className="ml-auto text-xs font-semibold text-blue-600 hover:underline dark:text-blue-400"
              onClick={() => void refetchMCP()}
            >
              刷新
            </button>
          </div>
          <Tabs
            ariaLabel="MCP 接入类型"
            className="mcp-transport-tabs"
            items={[
              { id: 'stdio', label: 'STDIO', meta: '推荐' },
              { id: 'http', label: '流式 HTTP', meta: '本机' }
            ]}
            onChange={transport => setTransport(transport)}
            value={http ? 'http' : 'stdio'}
            variant="segmented"
          />
          {http ? (
            <>
              <p className="m-0 text-xs leading-5 text-slate-600 dark:text-slate-300">
                先在终端启动受 Token 保护的服务；随后在 Codex 的“连接至自定义
                MCP”中选择<strong> 流式 HTTP</strong>，填写地址与 Bearer Token。
              </p>
              <dl className="mcp-form-guide m-0 overflow-hidden rounded-lg border border-blue-100 dark:border-blue-900">
                <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2 border-b border-blue-100 px-2 py-2 last:border-b-0 dark:border-blue-900">
                  <dt className="text-xs font-semibold text-slate-500">名称</dt>
                  <dd className="m-0 min-w-0 wrap-break-word text-xs text-slate-700 dark:text-slate-200">
                    alemonx
                  </dd>
                </div>
                <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2 border-b border-blue-100 px-2 py-2 last:border-b-0 dark:border-blue-900">
                  <dt className="text-xs font-semibold text-slate-500">类型</dt>
                  <dd className="m-0 min-w-0 wrap-break-word text-xs text-slate-700 dark:text-slate-200">
                    流式 HTTP
                  </dd>
                </div>
                <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2 border-b border-blue-100 px-2 py-2 last:border-b-0 dark:border-blue-900">
                  <dt className="text-xs font-semibold text-slate-500">地址</dt>
                  <dd className="m-0 min-w-0 wrap-break-word text-xs text-slate-700 dark:text-slate-200">
                    <code>http://127.0.0.1:17391/mcp</code>
                  </dd>
                </div>
                <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2 border-b border-blue-100 px-2 py-2 last:border-b-0 dark:border-blue-900">
                  <dt className="text-xs font-semibold text-slate-500">认证</dt>
                  <dd className="m-0 min-w-0 wrap-break-word text-xs text-slate-700 dark:text-slate-200">
                    Bearer Token：<code>&lt;MCP_TOKEN&gt;</code>
                  </dd>
                </div>
                <div className="grid grid-cols-[92px_minmax(0,1fr)] gap-2 border-b border-blue-100 px-2 py-2 last:border-b-0 dark:border-blue-900">
                  <dt className="text-xs font-semibold text-slate-500">
                    启动命令
                  </dt>
                  <dd className="m-0 min-w-0 wrap-break-word text-xs text-slate-700 dark:text-slate-200">
                    <code>{httpCommand}</code>
                  </dd>
                </div>
              </dl>
              <button
                className="mcp-copy-button justify-self-end rounded-lg bg-blue-600 px-3 py-2 text-xs font-semibold text-white transition hover:bg-blue-700"
                onClick={() => void copy(httpCommand)}
              >
                {copied ? '已复制启动命令' : '复制启动命令'}
              </button>
              <small className="mcp-note text-xs leading-5 text-slate-500">
                服务仅绑定 127.0.0.1；不要把地址、Token
                或端口转发到局域网和公网。流式 HTTP 兼容 MCP 的 POST
                请求，服务不提供独立 SSE 推送流。
              </small>
            </>
          ) : (
            <>
              <p className="m-0 text-xs leading-5 text-slate-600 dark:text-slate-300">
                在 Codex 的“连接至自定义 MCP”中选择<strong> STDIO</strong>
                ，把下列字段逐行填入。Codex 会直接启动本机 <code>alx</code>
                ，无需额外开启端口。
              </p>
              <dl className="mcp-form-guide m-0 overflow-hidden rounded-lg border border-blue-100 dark:border-blue-900">
                <div>
                  <dt>名称</dt>
                  <dd>alemonx</dd>
                </div>
                <div>
                  <dt>类型</dt>
                  <dd>STDIO</dd>
                </div>
                <div>
                  <dt>启动命令</dt>
                  <dd>
                    <code>alx</code>
                  </dd>
                </div>
                <div>
                  <dt>参数</dt>
                  <dd>
                    <code>mcp</code>
                  </dd>
                </div>
                <div>
                  <dt>环境变量（可选）</dt>
                  <dd>
                    <code>MCP_ALLOWED_ROOTS=/你的/机器人目录</code>
                  </dd>
                </div>
              </dl>
              <button
                className="mcp-copy-button"
                onClick={() => void copy(stdioConfig)}
              >
                {copied ? '已复制 JSON 配置' : '复制 JSON 配置'}
              </button>
              <small className="mcp-note">
                涉及安装、构建、写入或执行脚本时，助手仍必须取得你的本次确认；密钥、.env、.npmrc、Git
                元数据与依赖目录不开放。
              </small>
            </>
          )}
        </section>
      )}
    </div>
  )
}
function OperationTasksPage({
  root,
  sidebarLayout = false
}: {
  root: string
  sidebarLayout?: boolean
}) {
  const { data, isFetching } = useRobotTasksQuery(undefined, {
    // Task state is driven by SSE task events (invalidateTags); no polling.
    pollingInterval: 0,
    refetchOnMountOrArgChange: true
  })
  const tasks = (Array.isArray(data) ? data : []).filter(
    item => !root || !item.root || item.root === root
  )
  const [selected, setSelected] = useStoreState<string>('')
  const current = tasks.find(item => item.id === selected) ?? tasks[0]
  const visibleTasks = tasks.slice(0, 20)
  const label = (action: string) =>
    action.startsWith('setup:')
      ? `系统插件 · ${action.split(':').slice(-1)[0]}`
      : ({
          'install': '安装依赖',
          'upgrade-alemon': '升级 AlemonJS 依赖',
          'dependency-status': '检查依赖',
          'dev': '开发启动',
          'dev-stop': '停止开发模式',
          'app': '前台运行',
          'app-stop': '停止前台运行',
          'pm2': '后台启动',
          'pm2-stop': '停止 PM2',
          'pm2-restart': '重启 PM2',
          'pm2-reload': '重载 PM2',
          'pm2-delete': '删除 PM2 进程',
          'pm2-status': '查看 PM2 状态',
          'pm2-logs': '查看 PM2 日志',
          'install-package': '安装插件',
          'uninstall-package': '卸载插件',
          'install-connection': 'Yarn 安装连接包',
          'uninstall-connection': 'Yarn 卸载连接包',
          'git-release': 'GIT 发布',
          'npm-publish': 'NPM 发布'
        }[action] ?? action)
  return (
    <section className="workspace-content system-feature-page mx-auto max-w-215">
      {!sidebarLayout && (
        <header className="system-feature-header">
          <span className="system-feature-header-icon bg-brand-50 text-brand-600 dark:bg-brand-900/40 dark:text-brand-300">
            <ClipboardList className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <strong className="block text-sm font-semibold text-slate-900 dark:text-slate-100">
              操作记录
            </strong>
            <span className="block truncate text-xs text-slate-400 dark:text-slate-500">
              {root ? '当前机器人与系统操作' : '系统操作'}
            </span>
          </div>
        </header>
      )}
      {sidebarLayout && visibleTasks.length > 0 && (
        <SidebarWindowSectionNav>
          <div className="sidebar-window-log-list" aria-label="日志列表">
            {visibleTasks.map(item => (
              <button
                key={item.id}
                className="sidebar-window-log-item"
                type="button"
                aria-current={current?.id === item.id ? 'page' : undefined}
                onClick={() => setSelected(item.id)}
              >
                <i
                  className={cn(
                    'inline-flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] not-italic',
                    item.status === 'completed' &&
                      'system-feature-status-success',
                    item.status === 'failed' && 'system-feature-status-danger',
                    item.status === 'running' && 'system-feature-status-running'
                  )}
                >
                  {item.status === 'running'
                    ? '◌'
                    : item.status === 'completed'
                      ? '✓'
                      : '!'}
                </i>
                <span className="min-w-0">
                  <strong>{label(item.action)}</strong>
                  <small>
                    {item.status === 'running'
                      ? '进行中'
                      : item.status === 'failed'
                        ? '需要处理'
                        : '已完成'}
                    {item.createdAt && ` · ${formatTaskTime(item.createdAt)}`}
                  </small>
                </span>
              </button>
            ))}
          </div>
        </SidebarWindowSectionNav>
      )}
      {isFetching && !tasks.length ? (
        <p className="m-0 text-xs text-slate-500">正在读取任务…</p>
      ) : !tasks.length ? (
        <p className="m-0 text-xs text-slate-500">
          暂无与当前位置相关的操作记录。
        </p>
      ) : (
        <div className="grid gap-3">
          {!sidebarLayout && (
            <div className="task-list grid gap-1">
              {visibleTasks.map(item => (
                <button
                  key={item.id}
                  className={cn(
                    'system-feature-row flex items-center gap-2 text-left text-xs transition',
                    current?.id === item.id && 'system-feature-row-active'
                  )}
                  onClick={() => setSelected(item.id)}
                >
                  <i
                    className={cn(
                      'inline-flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] not-italic',
                      item.status === 'completed' &&
                        'system-feature-status-success',
                      item.status === 'failed' &&
                        'system-feature-status-danger',
                      item.status === 'running' &&
                        'system-feature-status-running'
                    )}
                  >
                    {item.status === 'running'
                      ? '◌'
                      : item.status === 'completed'
                        ? '✓'
                        : '!'}
                  </i>
                  <span className="grid gap-0.5 text-slate-700 dark:text-slate-200">
                    {label(item.action)}
                    <small className="text-[11px] text-slate-400">
                      {item.status === 'running'
                        ? '进行中'
                        : item.status === 'failed'
                          ? '需要处理'
                          : '已完成'}
                    </small>
                  </span>
                  {item.createdAt && (
                    <time
                      className="ml-auto shrink-0 text-[11px] tabular-nums text-slate-400 dark:text-slate-500"
                      dateTime={item.createdAt}
                    >
                      {formatTaskTime(item.createdAt)}
                    </time>
                  )}
                </button>
              ))}
            </div>
          )}
          {current && (
            <div className="grid gap-2">
              {sidebarLayout && (
                <div className="grid gap-0.5 px-0.5">
                  <strong className="text-sm font-semibold text-(--theme-text-strong)">
                    {label(current.action)}
                  </strong>
                  <small className="text-xs text-(--theme-text-muted)">
                    {current.status === 'running'
                      ? '正在执行'
                      : current.status === 'failed'
                        ? '执行失败'
                        : '已完成'}
                    {current.createdAt &&
                      ` · ${formatTaskTime(current.createdAt)}`}
                  </small>
                </div>
              )}
              <pre className="overflow-auto rounded-lg bg-slate-950 p-2 text-[11px] leading-5 text-slate-200">
                {current.status === 'failed'
                  ? current.error
                  : current.output || '正在执行…'}
              </pre>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
function EnvironmentPage({
  report,
  checking,
  onRefresh,
  onFix,
  sidebarLayout = false
}: {
  report: { checks: Check[] } | null
  checking: boolean
  onRefresh: () => void
  onFix: (check: Check) => void
  sidebarLayout?: boolean
}) {
  const checks = report?.checks ?? []
  const requiredChecks = checks.filter(check => !check.optional)
  const readyCount = requiredChecks.filter(
    check => check.status === 'ready'
  ).length
  const allReady =
    requiredChecks.length > 0 && readyCount === requiredChecks.length
  return (
    <section className="workspace-content system-feature-page mx-auto max-w-215">
      {!sidebarLayout && (
        <header className="system-feature-header">
          <span
            className={cn(
              'system-feature-header-icon',
              checking
                ? 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400'
                : allReady
                  ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-950/40 dark:text-emerald-400'
                  : 'bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-400'
            )}
          >
            {checking ? (
              <Loader2 className="size-4 animate-spin" />
            ) : allReady ? (
              <CheckCircle2 className="size-4" />
            ) : (
              <AlertTriangle className="size-4" />
            )}
          </span>
          <div className="min-w-0 flex-1">
            <strong className="block text-sm font-semibold text-slate-900 dark:text-slate-100">
              全局环境
            </strong>
            <span className="block truncate text-xs text-slate-400 dark:text-slate-500">
              {checking
                ? '正在检查…'
                : checks.length
                  ? `${readyCount}/${checks.length} 已就绪`
                  : '等待检查'}
            </span>
          </div>
        </header>
      )}

      {checking && (
        <p className="m-0 py-3 text-xs leading-5 text-slate-500">
          正在读取 Node.js、Git 和系统工具状态。
        </p>
      )}

      {!checking && checks.length > 0 && (
        <div className="grid gap-1.5 py-3">
          {checks.map(check => {
            const ready = check.status === 'ready'
            return (
              <article
                className={cn(
                  'system-feature-card flex items-start gap-2.5',
                  ready
                    ? 'border-emerald-200/70 bg-emerald-50/60 dark:border-emerald-900/60 dark:bg-emerald-950/20'
                    : 'border-amber-200/70 bg-amber-50/60 dark:border-amber-900/60 dark:bg-amber-950/20'
                )}
                key={check.id}
              >
                <span
                  className={cn(
                    'mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-full',
                    ready
                      ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/60 dark:text-emerald-400'
                      : 'bg-amber-100 text-amber-600 dark:bg-amber-900/60 dark:text-amber-400'
                  )}
                >
                  {ready ? (
                    <Check className="size-3" />
                  ) : (
                    <AlertTriangle className="size-3" />
                  )}
                </span>
                <div className="grid min-w-0 flex-1 gap-0.5">
                  <strong className="text-xs font-semibold text-slate-800 dark:text-slate-100">
                    {check.name}
                    {check.optional && (
                      <span className="ml-1.5 rounded bg-slate-200/70 px-1 py-0.5 text-[10px] font-medium text-slate-500 dark:bg-slate-700 dark:text-slate-300">
                        可选
                      </span>
                    )}
                  </strong>
                  <span className="text-xs leading-5 text-slate-500">
                    {check.detail}
                  </span>
                  {!ready && check.suggestion && (
                    <small className="text-xs leading-5 text-amber-700 dark:text-amber-300">
                      {check.suggestion}
                    </small>
                  )}
                </div>
                {!ready && (
                  <button
                    className="shrink-0 self-center rounded-md px-2 py-1 text-xs font-semibold text-brand-600 transition-colors hover:bg-white dark:text-brand-200 dark:hover:bg-slate-900"
                    onClick={() => onFix(check)}
                  >
                    {check.id === 'node' && check.status === 'outdated'
                      ? '升级'
                      : check.id === 'browser' || check.id === 'fonts'
                        ? '安装'
                        : '修复'}
                  </button>
                )}
              </article>
            )
          })}
        </div>
      )}

      {!checking && !checks.length && (
        <p className="m-0 py-3 text-xs text-slate-500">尚未获取检查结果。</p>
      )}

      {sidebarLayout ? (
        <SidebarWindowActions>
          <button
            className={
              sidebarLayout
                ? 'sidebar-window-action'
                : 'system-feature-refresh inline-flex min-h-8 items-center gap-1.5 justify-center rounded-lg px-3 text-xs font-semibold transition-colors'
            }
            disabled={checking}
            onClick={onRefresh}
          >
            <RefreshCw
              className={cn(
                sidebarLayout ? 'size-4' : 'size-3.5',
                checking && 'animate-spin'
              )}
            />
            重新检查
          </button>
        </SidebarWindowActions>
      ) : (
        <footer className="flex justify-end border-t border-slate-100 pt-3 dark:border-slate-800">
          <button
            className="system-feature-refresh inline-flex min-h-8 items-center gap-1.5 justify-center rounded-lg px-3 text-xs font-semibold transition-colors"
            disabled={checking}
            onClick={onRefresh}
          >
            <RefreshCw className={cn('size-3.5', checking && 'animate-spin')} />
            重新检查
          </button>
        </footer>
      )}
    </section>
  )
}
function EmptyWorkspace({
  onAdd,
  onClone
}: {
  onAdd: () => void
  onClone: () => void
}) {
  return (
    <section className="grid min-h-full content-center justify-items-center p-0 text-center">
      <span className="inline-flex size-8 items-center justify-center rounded-[10px] bg-(--theme-accent-soft) text-lg text-(--theme-accent-text)">
        ◈
      </span>
      <div className="grid gap-1.5">
        <strong className="my-2 text-sm text-(--theme-text-primary)">
          开始管理你的机器人
        </strong>
        <p className="m-0 text-xs text-(--theme-text-muted)">
          选择已有目录，或从 Git 克隆一个新的机器人项目。
        </p>
      </div>
      <footer className="flex flex-wrap justify-center gap-2">
        <button className="secondary-button" onClick={onClone}>
          <GitBranch className="size-3.5" />从 Git 克隆
        </button>
        <button className="primary-button" onClick={onAdd}>
          添加本地目录
        </button>
      </footer>
    </section>
  )
}
function InvalidWorkspace({
  project,
  reason,
  onRemove,
  onChoose
}: {
  project: Project
  reason?: string
  onRemove: () => void
  onChoose: () => void
}) {
  return (
    <section className="mx-auto my-7 grid w-full max-w-155 grid-cols-[auto_minmax(0,1fr)] items-center gap-3.5 rounded-panel border border-(--theme-danger) bg-(--theme-warning-soft) p-4">
      <i className="inline-flex size-9.5 items-center justify-center rounded-[10px] bg-(--theme-danger-soft) text-lg font-extrabold not-italic text-(--theme-danger-text)">
        !
      </i>
      <div className="grid min-w-0 gap-1">
        <strong className="text-[0.92rem] text-(--theme-danger-text)">
          机器人目录不可用
        </strong>
        <span
          className="truncate text-[0.74rem] text-(--theme-text-muted)"
          title={project.path}
        >
          {project.path}
        </span>
        <small className="truncate text-[0.74rem] text-(--theme-text-muted)">
          {reason || '目录不存在或不再是可管理的机器人项目。'}
        </small>
      </div>
      <footer className="col-span-full flex justify-end gap-2 border-t border-(--theme-danger-soft) pt-3">
        <button className="secondary-button" onClick={onRemove}>
          移除旧目录
        </button>
        <button className="primary-button" onClick={onChoose}>
          重新选择目录
        </button>
      </footer>
    </section>
  )
}
function InvalidDirectoryDialog({
  path,
  onClose,
  onCreate
}: {
  path: string
  onClose: () => void
  onCreate: () => void
}) {
  return (
    <Modal
      open
      onClose={onClose}
      ariaLabel="目录不是合法机器人项目"
      className="bg-slate-950/25 p-6"
    >
      <section
        className="invalid-directory-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="目录不是合法机器人项目"
      >
        <header>
          <i>!</i>
          <div>
            <strong>所选目录不是合法机器人目录</strong>
            <small title={path}>{path}</small>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label="关闭提示"
          >
            <X />
          </button>
        </header>
        <p>
          这里缺少 <code>package.json</code>
          ，因此不能作为已有机器人管理。你可以选择一个已有机器人目录，或在这里创建新机器人。
        </p>
        <footer>
          <button className="secondary-button" onClick={onClose}>
            重新选择
          </button>
          <button className="primary-button" onClick={onCreate}>
            前往引导创建
          </button>
        </footer>
      </section>
    </Modal>
  )
}
function SystemPluginCenter({
  plugins,
  marketPlugins,
  marketLoading,
  pluginsLoading,
  onOpen,
  onRefresh,
  sidebarLayout = false
}: {
  plugins: SetupPlugin[]
  marketPlugins: SetupPlugin[]
  marketLoading: boolean
  pluginsLoading: boolean
  onOpen: (id: string) => void
  onRefresh: () => void
  sidebarLayout?: boolean
}) {
  const [setEnabled, { isLoading }] = useSetSetupPluginEnabledMutation()
  const [installPlugin, { isLoading: installing }] =
    useInstallSetupPluginMutation()
  const [uninstallPlugin, { isLoading: uninstalling }] =
    useUninstallSetupPluginMutation()
  const [loadReleases, { isFetching: releasesFetching }] =
    useLazySetupPluginReleasesQuery()
  const [loadVersions, { isFetching: versionsFetching }] =
    useLazySetupPluginVersionsQuery()
  const [switchVersion, { isLoading: switching }] =
    useSwitchSetupPluginVersionMutation()
  const [deleteVersion] = useDeleteSetupPluginVersionMutation()
  const [cleanupCache, { isLoading: cleaningCache }] =
    useCleanupSetupPluginCacheMutation()
  const { data: cacheSummary } = useSetupPluginCacheQuery()
  const { data: downloadCacheSummary, refetch: refetchDownloadCache } =
    usePluginDownloadCacheQuery()
  const [clearDownloadCache, { isLoading: clearingDownloadCache }] =
    useClearPluginDownloadCacheMutation()
  const {
    data: developmentData,
    refetch: refetchDevelopment,
    isLoading: isDevelopmentLoading,
    isFetching: isDevelopmentFetching
  } = useSetupPluginDevelopmentQuery()
  const [registerDevelopment, { isLoading: registeringDevelopment }] =
    useRegisterSetupPluginDevelopmentMutation()
  const [runDevelopment, { isLoading: runningDevelopment }] =
    useRunSetupPluginDevelopmentMutation()
  const [removeDevelopment, { isLoading: removingDevelopment }] =
    useRemoveSetupPluginDevelopmentMutation()
  const [loadDevelopmentLogs] = useLazySetupPluginDevelopmentLogsQuery()
  const [uploadPluginArchiveAction, { isLoading: uploadingPluginArchive }] =
    useUploadSetupPluginArchiveMutation()
  const [pluginUploadError, setPluginUploadError] = useState('')
  const [installTarget, setInstallTarget] = useStoreState<SetupPlugin | null>(
    null
  )
  const [versionTarget, setVersionTarget] = useStoreState<SetupPlugin | null>(
    null
  )
  const [versionItems, setVersionItems] = useStoreState<SetupPluginVersion[]>(
    []
  )
  const [selectedVersion, setSelectedVersion] = useStoreState('')
  const [selectedAsset, setSelectedAsset] = useStoreState('')
  const [releaseOptions, setReleaseOptions] = useStoreState<
    SetupPluginRelease[]
  >([])
  const [message, setMessage] = useStoreState('')
  const [developmentLogTarget, setDevelopmentLogTarget] =
    useStoreState<SetupPluginDevelopment | null>(null)
  const [developmentLog, setDevelopmentLog] = useStoreState('')
  const [developmentFinderOpen, setDevelopmentFinderOpen] = useStoreState(false)
  const [pluginView, setPluginView] = useStoreState<
    'market' | 'mine' | 'development'
  >('market')
  const developmentItems = developmentData?.items ?? []
  const isDevelopmentView = sidebarLayout && pluginView === 'development'
  const visiblePlugins = !sidebarLayout
    ? plugins
    : pluginView === 'market'
      ? marketPlugins
      : pluginView === 'mine'
        ? plugins.filter(plugin => !plugin.developmentSource)
        : []
  const registerDevelopmentPath = async (paths: string[]) => {
    const path = paths[0]
    if (!path) return
    try {
      await registerDevelopment({ path }).unwrap()
      setDevelopmentLog('')
      setDevelopmentFinderOpen(false)
      await refetchDevelopment()
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '未能登记插件源码目录。'))
    }
  }
  const runDevelopmentAction = async (
    session: SetupPluginDevelopment,
    action: 'build' | 'start' | 'stop' | 'restart'
  ) => {
    if (
      (action === 'start' || action === 'restart') &&
      !window.confirm(
        `将执行“${session.name}”源码目录中 alx.json 声明的本地开发命令，并临时覆盖同 ID 的正式插件。是否继续？`
      )
    )
      return
    try {
      await runDevelopment({
        pluginID: session.id,
        action,
        confirm: action !== 'stop'
      }).unwrap()
      await refetchDevelopment()
      setMessage(
        action === 'stop'
          ? `已停止“${session.name}”源码开发。`
          : `“${session.name}”源码开发已${action === 'build' ? '构建' : '启动'}。`
      )
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '源码开发操作未完成。'))
      await refetchDevelopment()
    }
  }
  const showDevelopmentLogs = async (session: SetupPluginDevelopment) => {
    try {
      const logs = await loadDevelopmentLogs(session.id).unwrap()
      setDevelopmentLogTarget(session)
      setDevelopmentLog(logs.output)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '读取源码开发日志失败。'))
    }
  }
  const removeDevelopmentSession = async (session: SetupPluginDevelopment) => {
    const detail = session.running
      ? '会先停止开发进程并恢复正式插件。'
      : '会移除主应用保存的登记记录。'
    if (
      !window.confirm(
        `移除“${session.name}”开发会话？${detail}不会删除源码目录。`
      )
    )
      return
    try {
      await removeDevelopment(session.id).unwrap()
      if (developmentLogTarget?.id === session.id) {
        setDevelopmentLogTarget(null)
        setDevelopmentLog('')
      }
      await refetchDevelopment()
      setMessage(`已移除“${session.name}”开发会话。源码目录未删除。`)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '移除源码开发会话失败。'))
      await refetchDevelopment()
    }
  }
  const clearDownloads = async () => {
    if (
      !window.confirm(
        '清理系统插件下载缓存？不会删除已安装插件、NapCat、LuckyLillia 或其配置；下次需要时会自动重新下载。'
      )
    )
      return
    try {
      await clearDownloadCache().unwrap()
      await refetchDownloadCache()
      setMessage('已清理系统插件下载缓存。')
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '下载缓存清理失败。'))
    }
  }
  const uploadPluginArchive = async (file: File) => {
    setPluginUploadError('')
    try {
      const installed = await uploadPluginArchiveAction(file).unwrap()
      setMessage(
        `已上传并安装“${installed.name}”。现在可以点击「启动」加载它。`
      )
      onRefresh()
    } catch (reason) {
      setPluginUploadError(operationErrorMessage(reason, '插件上传未完成。'))
    }
  }
  const toggle = async (plugin: SetupPlugin) => {
    try {
      await setEnabled({
        pluginID: plugin.id,
        enabled: !plugin.enabled
      }).unwrap()
      setMessage(
        plugin.enabled ? `已停用“${plugin.name}”。` : `已启动“${plugin.name}”。`
      )
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件状态未更新。'))
    }
  }
  const uninstall = async (plugin: SetupPlugin) => {
    if (
      !window.confirm(
        `卸载“${plugin.name}”会删除其本地插件文件。已下载的 Release 缓存会保留，以便以后重新安装，是否继续？`
      )
    )
      return
    try {
      await uninstallPlugin({ pluginID: plugin.id }).unwrap()
      setMessage(`已卸载“${plugin.name}”。你可以随时重新安装。`)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件卸载未完成。'))
    }
  }
  const install = async (plugin: SetupPlugin) => {
    setVersionTarget(null)
    setInstallTarget(plugin)
    setReleaseOptions([])
    setSelectedVersion('')
    setSelectedAsset('')
    try {
      const releases = await loadReleases(plugin.id).unwrap()
      setReleaseOptions(releases)
      const first = releases[0]
      setSelectedVersion(first?.tag ?? '')
      setSelectedAsset(
        first?.assets.find(
          asset => asset.compatible && /\.(zip|tar\.gz|tgz)$/i.test(asset.name)
        )?.name ?? ''
      )
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件版本读取失败。'))
    }
  }
  const confirmInstall = async () => {
    if (!installTarget || !selectedVersion || !selectedAsset) return
    try {
      await installPlugin({
        pluginID: installTarget.id,
        version: selectedVersion,
        assetName: selectedAsset
      }).unwrap()
      setInstallTarget(null)
      setMessage(`已下载“${installTarget.name}”。现在可以点击「启动」加载它。`)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件安装未完成。'))
    }
  }
  const manageVersions = async (plugin: SetupPlugin) => {
    setVersionItems([])
    setVersionTarget(plugin)
    try {
      const versions = await loadVersions(plugin.id).unwrap()
      setVersionItems(versions)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件版本读取失败。'))
    }
  }
  const refreshVersions = async () => {
    if (!versionTarget) return
    setVersionItems(await loadVersions(versionTarget.id).unwrap())
  }
  const switchCachedVersion = async (item: SetupPluginVersion) => {
    if (!versionTarget || item.active || !item.cached) return
    if (
      !window.confirm(
        `切换到 ${item.tag} 会替换当前插件文件。插件自身管理的外部进程需要先停止，是否继续？`
      )
    )
      return
    try {
      await switchVersion({
        pluginID: versionTarget.id,
        version: item.tag,
        assetName: item.asset
      }).unwrap()
      await refreshVersions()
      setMessage(`已切换“${versionTarget.name}”到 ${item.tag}。`)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '插件版本切换失败。'))
    }
  }
  const removeCachedVersion = async (item: SetupPluginVersion) => {
    if (!versionTarget || item.active || !item.cached) return
    if (!window.confirm(`确定删除已缓存版本 ${item.tag} 吗？`)) return
    try {
      await deleteVersion({
        pluginID: versionTarget.id,
        tag: item.tag
      }).unwrap()
      await refreshVersions()
      setMessage(`已删除缓存版本 ${item.tag}。`)
    } catch (reason) {
      setMessage(operationErrorMessage(reason, '缓存版本删除失败。'))
    }
  }
  const currentRelease = releaseOptions.find(
    item => item.tag === selectedVersion
  )
  const archiveAssets =
    currentRelease?.assets.filter(
      asset => asset.compatible && /\.(zip|tar\.gz|tgz)$/i.test(asset.name)
    ) ?? []
  const persistentActions = (
    <>
      <Button
        variant="secondary"
        size="sm"
        className={sidebarLayout ? 'sidebar-window-action' : undefined}
        disabled={clearingDownloadCache || !downloadCacheSummary?.entries}
        onClick={() => void clearDownloads()}
        title={
          downloadCacheSummary
            ? `${(downloadCacheSummary.bytes / 1024 / 1024).toFixed(1)} MB · ${downloadCacheSummary.entries} 项下载缓存`
            : '正在读取下载缓存'
        }
      >
        {sidebarLayout && <Trash2 className="size-4" />}
        {sidebarLayout
          ? clearingDownloadCache
            ? '清理缓存中…'
            : '清理下载缓存'
          : downloadCacheSummary
            ? `${(downloadCacheSummary.bytes / 1024 / 1024).toFixed(1)} MB · ${downloadCacheSummary.entries} 项`
            : '正在读取…'}
        {!sidebarLayout && (clearingDownloadCache ? '清理中…' : '清理下载缓存')}
      </Button>
    </>
  )
  const developmentAction = (
    <Button
      variant="secondary"
      size="sm"
      disabled={registeringDevelopment}
      onClick={() => setDevelopmentFinderOpen(true)}
      className="shrink-0"
    >
      <Code2 className="mr-1 size-3.5" />
      {registeringDevelopment ? '正在登记…' : '开发插件'}
    </Button>
  )
  return (
    <section className="workspace-content system-feature-page mx-auto max-w-215">
      {sidebarLayout && (
        <SidebarWindowSectionNav>
          <button
            type="button"
            aria-current={pluginView === 'market' ? 'page' : undefined}
            onClick={() => setPluginView('market')}
          >
            <Globe className="size-4" /> 市场
          </button>
          <button
            type="button"
            aria-current={pluginView === 'mine' ? 'page' : undefined}
            onClick={() => setPluginView('mine')}
          >
            <HardDrive className="size-4" /> 我的
          </button>
          <button
            type="button"
            aria-current={pluginView === 'development' ? 'page' : undefined}
            onClick={() => setPluginView('development')}
          >
            <Code2 className="size-4" /> 开发
          </button>
        </SidebarWindowSectionNav>
      )}
      {sidebarLayout ? (
        pluginView === 'mine' && (
          <SidebarWindowActions>
            <div className="grid gap-1.5">{persistentActions}</div>
          </SidebarWindowActions>
        )
      ) : (
        <header className="system-feature-header">
          <span className="system-feature-header-icon bg-brand-50 text-brand-600 dark:bg-brand-900/40 dark:text-brand-300">
            <Package className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <strong className="block text-sm font-semibold text-slate-900 dark:text-slate-100">
              插件中心
            </strong>
            <span className="block truncate text-xs text-slate-400 dark:text-slate-500">
              管理系统级插件与扩展能力
            </span>
          </div>
          <div className="flex gap-2">
            {persistentActions}
            {developmentAction}
          </div>
        </header>
      )}
      {isDevelopmentView && (
        <button
          type="button"
          className="setup-plugin-development-add"
          disabled={registeringDevelopment}
          onClick={() => setDevelopmentFinderOpen(true)}
        >
          <span className="setup-plugin-development-add-icon">
            <Plus className="size-5" />
          </span>
          <span>
            <strong>
              {registeringDevelopment ? '正在登记源码…' : '添加新源码'}
            </strong>
            <small>选择包含 alx.json 的本地插件目录</small>
          </span>
        </button>
      )}
      {(!sidebarLayout || pluginView === 'development') &&
        developmentItems.length > 0 && (
          <section
            className="setup-plugin-development-list"
            aria-label="已登记源码"
          >
            {developmentItems.map(session => {
              const developmentStatus = session.busy
                ? `正在${session.state === 'starting' ? '启动' : session.state === 'stopping' ? '停止' : '构建'}`
                : session.state === 'running'
                  ? '运行中'
                  : session.state === 'failed'
                    ? '操作失败'
                    : '未启动'
              const developmentState = session.busy ? 'busy' : session.state

              return (
                <article
                  key={session.id}
                  className="setup-plugin-development-card"
                  data-state={developmentState}
                >
                  <div className="setup-plugin-development-card-main">
                    <span
                      className="setup-plugin-development-card-icon"
                      aria-hidden="true"
                    >
                      <Code2 className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <div className="setup-plugin-development-card-title">
                        <strong>{session.name}</strong>
                        <span className="setup-plugin-development-badge">
                          源码开发
                        </span>
                        <span
                          className="setup-plugin-development-state"
                          data-state={developmentState}
                        >
                          <span aria-hidden="true" />
                          {developmentStatus}
                        </span>
                      </div>
                      <code
                        className="setup-plugin-development-source"
                        title={session.source}
                      >
                        <Folder
                          className="size-3.5 shrink-0"
                          aria-hidden="true"
                        />
                        <span>{session.source}</span>
                      </code>
                      {session.privileges?.length ||
                      session.services?.length ? (
                        <div className="setup-plugin-development-meta">
                          {session.privileges?.length ? (
                            <span
                              title={`权限：${session.privileges.join('、')}`}
                            >
                              <Shield className="size-3.5" aria-hidden="true" />
                              {session.privileges.join('、')}
                            </span>
                          ) : null}
                          {session.services?.length ? (
                            <span
                              title={session.services
                                .map(
                                  service =>
                                    `${service.id}${service.port ? `:${service.port}` : ''}${service.running ? '（运行中）' : ''}`
                                )
                                .join('、')}
                            >
                              <Network
                                className="size-3.5"
                                aria-hidden="true"
                              />
                              {session.services
                                .map(
                                  service =>
                                    `${service.id}${service.port ? `:${service.port}` : ''}${service.running ? '（运行中）' : ''}`
                                )
                                .join('、')}
                            </span>
                          ) : null}
                        </div>
                      ) : null}
                      {session.lastError && (
                        <p
                          className="setup-plugin-development-error"
                          title={session.lastError}
                        >
                          {session.lastError}
                        </p>
                      )}
                    </div>
                  </div>
                  <div className="setup-plugin-development-actions">
                    {session.running && (
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => onOpen(session.id)}
                      >
                        打开
                      </Button>
                    )}
                    {session.buildAvailable && (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={runningDevelopment || session.busy}
                        onClick={() =>
                          void runDevelopmentAction(session, 'build')
                        }
                      >
                        构建
                      </Button>
                    )}
                    <Button
                      variant="secondary"
                      size="sm"
                      disabled={runningDevelopment || session.busy}
                      onClick={() =>
                        void runDevelopmentAction(
                          session,
                          session.running ? 'restart' : 'start'
                        )
                      }
                    >
                      {session.running ? '重启' : '启动开发'}
                    </Button>
                    {session.running && (
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={runningDevelopment || session.busy}
                        onClick={() =>
                          void runDevelopmentAction(session, 'stop')
                        }
                      >
                        停止
                      </Button>
                    )}
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => void showDevelopmentLogs(session)}
                    >
                      日志
                    </Button>
                    <Button
                      className="setup-plugin-development-remove"
                      variant="secondary"
                      size="sm"
                      disabled={
                        removingDevelopment ||
                        runningDevelopment ||
                        session.busy
                      }
                      onClick={() => void removeDevelopmentSession(session)}
                    >
                      移除
                    </Button>
                  </div>
                </article>
              )
            })}
          </section>
        )}
      {/* 插件列表 */}
      {!isDevelopmentView && (!sidebarLayout || pluginView === 'mine') && (
        <section className="grid gap-1.5">
          <PluginUploadDropzone
            label="拖入或点击上传"
            hint="支持 .zip / .tar.gz / .tgz，上传内含 alx.json 的 Setup 插件安装包。"
            busy={uploadingPluginArchive}
            onFile={file => void uploadPluginArchive(file)}
          />
          {pluginUploadError && (
            <small className="rounded-md border border-amber-200 bg-amber-50 p-2 text-[11px] leading-4 text-amber-800">
              {pluginUploadError}
            </small>
          )}
        </section>
      )}
      {!isDevelopmentView &&
        (visiblePlugins.length ? (
          <div className="grid gap-2">
            {visiblePlugins.map(plugin => {
              const isOnline = Boolean(plugin.online)
              const isEnabled = Boolean(plugin.enabled)
              const isManagedRelease = Boolean(plugin.installedTag)
              const isDevelopment = Boolean(plugin.developmentSource)
              const hasPanel = Boolean(plugin.web)
              const canOpen = isEnabled && hasPanel
              return (
                <article
                  key={plugin.id}
                  className="system-feature-row group relative flex items-center gap-3.5"
                >
                  {/* 图标 */}
                  <div
                    className={cn(
                      'relative flex size-10 shrink-0 items-center justify-center rounded-lg transition-colors',
                      isEnabled
                        ? 'bg-brand-50 text-brand-600 dark:bg-brand-900/40 dark:text-brand-300'
                        : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500'
                    )}
                  >
                    {setupPluginIcon(plugin.navigation.icon)}
                    {isOnline && (
                      <span
                        className="absolute -right-1 -top-1 size-2.5 rounded-full border-2 border-white bg-emerald-400 dark:border-slate-900"
                        title="在线目录"
                      />
                    )}
                  </div>

                  {/* 信息 */}
                  <button
                    className="min-w-0 flex-1 text-left"
                    onClick={() => {
                      if (isOnline) {
                        return
                      }
                      if (canOpen) {
                        onOpen(plugin.id)
                      }
                    }}
                    disabled={!canOpen}
                  >
                    <div className="flex items-center gap-2">
                      <strong className="truncate text-sm font-medium text-slate-800 dark:text-slate-100">
                        {plugin.name}
                      </strong>
                      {plugin.description && (
                        <span className="setup-plugin-description hidden truncate text-xs text-slate-400 sm:inline dark:text-slate-500">
                          · {plugin.description}
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-400 dark:text-slate-500">
                      <span className="font-mono">
                        {plugin.installedTag ?? `v${plugin.version}`}
                      </span>
                      <span className="size-0.5 rounded-full bg-slate-300 dark:bg-slate-600" />
                      {isOnline ? (
                        <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                          <Globe className="size-3" />
                          在线目录
                        </span>
                      ) : isDevelopment ? (
                        <span className="inline-flex items-center gap-1 text-violet-600 dark:text-violet-400">
                          <Code2 className="size-3" />
                          源码开发中
                        </span>
                      ) : isManagedRelease && !isEnabled ? (
                        <span className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400">
                          <Circle className="size-3" />
                          已下载，等待启动
                        </span>
                      ) : isEnabled ? (
                        <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                          <CheckCircle2 className="size-3" />
                          已启用
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1">
                          <Circle className="size-3" />
                          已停用
                        </span>
                      )}
                      {!hasPanel && !isOnline && (
                        <>
                          <span className="size-0.5 rounded-full bg-slate-300 dark:bg-slate-600" />
                          <span className="text-slate-400 dark:text-slate-500">
                            无界面
                          </span>
                        </>
                      )}
                    </div>
                    {!isOnline && plugin.source && (
                      <div
                        className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-slate-400 dark:text-slate-500"
                        title={plugin.source}
                      >
                        <Folder className="size-3 shrink-0" />
                        <span className="shrink-0">本机目录</span>
                        <code className="min-w-0 truncate font-mono text-[11px]">
                          {plugin.source}
                        </code>
                      </div>
                    )}
                  </button>

                  {/* 操作 */}
                  {isOnline ? (
                    <Button
                      variant="primary"
                      size="sm"
                      disabled={installing}
                      onClick={() => void install(plugin)}
                      className="system-feature-actions h-7 shrink-0 rounded-md px-3 text-xs font-medium"
                    >
                      {installing ? (
                        <>
                          <span className="mr-1.5 inline-block size-3 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                          安装中
                        </>
                      ) : (
                        '下载'
                      )}
                    </Button>
                  ) : isDevelopment ? (
                    <div className="system-feature-actions flex shrink-0 gap-1.5">
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => onOpen(plugin.id)}
                        className="h-7 rounded-md px-2.5 text-xs font-medium"
                      >
                        打开
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={runningDevelopment}
                        onClick={() => {
                          const session = developmentItems.find(
                            item => item.id === plugin.id
                          )
                          if (session)
                            void runDevelopmentAction(session, 'stop')
                        }}
                        className="h-7 rounded-md px-2.5 text-xs font-medium"
                      >
                        停止开发
                      </Button>
                    </div>
                  ) : isManagedRelease ? (
                    <div className="system-feature-actions flex shrink-0 gap-1.5">
                      {!isEnabled ? (
                        <Button
                          variant="primary"
                          size="sm"
                          disabled={isLoading}
                          onClick={() => void toggle(plugin)}
                          className="h-7 rounded-md px-2.5 text-xs font-medium"
                        >
                          启动
                        </Button>
                      ) : (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={isLoading}
                          onClick={() => void toggle(plugin)}
                          className="h-7 rounded-md px-2.5 text-xs font-medium"
                        >
                          停止
                        </Button>
                      )}
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => void manageVersions(plugin)}
                        className="h-7 rounded-md px-2.5 text-xs font-medium"
                      >
                        切换版本
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={uninstalling}
                        onClick={() => void uninstall(plugin)}
                        className={cn(
                          'h-7 rounded-md px-2.5 text-xs font-medium',
                          'border-slate-200 text-slate-600 hover:border-red-200 hover:bg-red-50 hover:text-red-600 dark:border-slate-700 dark:text-slate-400 dark:hover:border-red-800 dark:hover:bg-red-900/20 dark:hover:text-red-400'
                        )}
                      >
                        {uninstalling ? '卸载中…' : '卸载'}
                      </Button>
                    </div>
                  ) : (
                    <Button
                      variant={isEnabled ? 'secondary' : 'primary'}
                      size="sm"
                      disabled={isLoading}
                      onClick={() => void toggle(plugin)}
                      className="system-feature-actions h-7 shrink-0 rounded-md px-2.5 text-xs font-medium"
                    >
                      {isEnabled ? '停止' : '启动'}
                    </Button>
                  )}
                </article>
              )
            })}
          </div>
        ) : (pluginView === 'market' && marketLoading) ||
          (pluginView === 'mine' && pluginsLoading) ? (
          <EmptyState
            icon={
              <Loader2 className="size-6 animate-spin text-slate-400 dark:text-slate-500" />
            }
            title={
              pluginView === 'market'
                ? '正在加载插件市场…'
                : '正在加载插件列表…'
            }
            description="正在获取最新数据，请稍候。"
          />
        ) : (
          /* 空状态 */
          <EmptyState
            icon={
              <Package className="size-6 text-slate-400 dark:text-slate-500" />
            }
            title={
              !sidebarLayout
                ? '暂未发现插件'
                : pluginView === 'market'
                  ? '市场暂未提供插件'
                  : '还没有已下载的插件'
            }
            description={
              !sidebarLayout
                ? '将插件目录放入 plugins 后刷新即可'
                : pluginView === 'market'
                  ? '稍后再试，或检查网络连接。'
                  : '前往市场下载插件后会显示在这里。'
            }
          />
        ))}
      {isDevelopmentView &&
        developmentItems.length === 0 &&
        (isDevelopmentLoading || isDevelopmentFetching ? (
          <EmptyState
            icon={
              <Loader2 className="size-6 animate-spin text-slate-400 dark:text-slate-500" />
            }
            title="正在加载已登记的源码…"
            description="正在获取开发插件列表，请稍候。"
          />
        ) : (
          <EmptyState
            icon={
              <Code2 className="size-6 text-slate-400 dark:text-slate-500" />
            }
            title="还没有已登记的源码"
            description="点击上方“添加新源码”，选择本地插件目录开始开发。"
          />
        ))}

      {/* 消息提示 */}
      {message && (
        <div className="flex items-center gap-2 rounded-lg border border-brand-200/50 bg-brand-50/80 px-4 py-2.5 text-sm text-brand-700 dark:border-brand-800/30 dark:bg-brand-900/20 dark:text-brand-300">
          <CheckCircle2 className="size-4 shrink-0" />
          <span>{message}</span>
        </div>
      )}
      {installTarget && (
        <Modal
          open
          onClose={() => setInstallTarget(null)}
          ariaLabel={`下载${installTarget.name}`}
        >
          <div className="grid w-full max-w-md gap-3 rounded-xl bg-white p-5 shadow-xl dark:bg-slate-900">
            <strong className="text-base text-slate-900 dark:text-slate-100">
              下载“{installTarget.name}”
            </strong>
            <label className="grid gap-1 text-xs font-semibold text-slate-500">
              版本
              {releasesFetching && releaseOptions.length === 0 ? (
                <span className="flex items-center gap-1.5 rounded-md border border-slate-200 bg-white px-2 py-2 text-sm font-normal text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300">
                  <Loader2 className="size-3.5 animate-spin" />
                  正在加载可用版本…
                </span>
              ) : (
                <select
                  className="rounded-md border border-slate-200 bg-white px-2 py-2 text-sm font-normal dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
                  value={selectedVersion}
                  onChange={event => {
                    const next = event.target.value
                    const release = releaseOptions.find(
                      item => item.tag === next
                    )
                    setSelectedVersion(next)
                    setSelectedAsset(
                      release?.assets.find(
                        asset =>
                          asset.compatible &&
                          /\.(zip|tar\.gz|tgz)$/i.test(asset.name)
                      )?.name ?? ''
                    )
                  }}
                >
                  {releaseOptions.map(item => (
                    <option key={item.tag} value={item.tag}>
                      {item.name || item.tag} ({item.tag})
                    </option>
                  ))}
                </select>
              )}
            </label>
            <label className="grid gap-1 text-xs font-semibold text-slate-500">
              当前系统安装包
              <select
                className="rounded-md border border-slate-200 bg-white px-2 py-2 text-sm font-normal dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
                value={selectedAsset}
                onChange={event => setSelectedAsset(event.target.value)}
              >
                <option value="">请选择安装包</option>
                {archiveAssets.map(asset => (
                  <option key={asset.name} value={asset.name}>
                    {asset.name}{' '}
                    {asset.size
                      ? `(${(asset.size / 1024 / 1024).toFixed(1)} MB)`
                      : ''}
                  </option>
                ))}
              </select>
            </label>
            {archiveAssets.length === 0 && !releasesFetching && (
              <p className="m-0 text-xs text-amber-600">
                该版本没有可用的 zip / tar.gz 插件包。
              </p>
            )}
            {installing && (
              <DownloadProgress
                label={`正在下载 ${installTarget.name}`}
                detail="正在从选定 Release 下载、解包并校验插件；请保持此窗口打开。"
              />
            )}
            <div className="flex justify-end gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setInstallTarget(null)}
              >
                取消
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={installing || !selectedAsset}
                onClick={() => void confirmInstall()}
              >
                {installing ? '下载中…' : '下载'}
              </Button>
            </div>
          </div>
        </Modal>
      )}
      <DirectoryPicker
        open={developmentFinderOpen}
        multiple={false}
        priority
        onClose={() => setDevelopmentFinderOpen(false)}
        onSelect={paths => void registerDevelopmentPath(paths)}
      />
      {developmentLogTarget && (
        <Modal
          open
          onClose={() => {
            setDevelopmentLogTarget(null)
            setDevelopmentLog('')
          }}
          ariaLabel={`${developmentLogTarget.name}开发日志`}
        >
          <div className="grid w-full max-w-2xl gap-3 rounded-xl bg-white p-5 shadow-xl dark:bg-slate-900">
            <div>
              <strong className="block text-base text-slate-900 dark:text-slate-100">
                {developmentLogTarget.name} · 开发日志
              </strong>
              <code className="mt-1 block break-all text-xs text-slate-500">
                {developmentLogTarget.source}
              </code>
            </div>
            <pre className="max-h-[min(70vh,36rem)] overflow-auto rounded-lg bg-slate-950 p-3 text-[11px] leading-5 text-slate-100">
              {developmentLog || '暂无日志。'}
            </pre>
            <div className="flex justify-end">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  setDevelopmentLogTarget(null)
                  setDevelopmentLog('')
                }}
              >
                关闭
              </Button>
            </div>
          </div>
        </Modal>
      )}
      {versionTarget && (
        <Modal
          open
          onClose={() => setVersionTarget(null)}
          ariaLabel={`${versionTarget.name}版本管理`}
        >
          <div className="grid max-h-[min(80vh,48rem)] w-full max-w-lg min-w-0 gap-3 overflow-y-auto rounded-xl bg-white p-5 shadow-xl dark:bg-slate-900">
            <div className="min-w-0">
              <strong className="block break-words text-base text-slate-900 dark:text-slate-100">
                {versionTarget.name} · 版本管理
              </strong>
              <p className="m-0 mt-1 text-xs text-slate-500">
                缓存{' '}
                {cacheSummary
                  ? `${(cacheSummary.bytes / 1024 / 1024).toFixed(1)} MB / ${(cacheSummary.limit / 1024 / 1024).toFixed(0)} MB`
                  : '读取中…'}
              </p>
              {cacheSummary && cacheSummary.bytes > cacheSummary.limit && (
                <p className="m-0 mt-1 text-xs text-amber-600">
                  缓存已超过上限，建议立即清理。
                </p>
              )}
            </div>
            <div className="grid gap-2">
              {versionsFetching && versionItems.length === 0 ? (
                <p className="m-0 flex items-center gap-1.5 text-xs text-slate-500">
                  <Loader2 className="size-3 animate-spin" />
                  正在加载版本列表…
                </p>
              ) : versionItems.length ? (
                versionItems.map(item => (
                  <div
                    key={`${item.tag}:${item.asset}`}
                    className="flex min-w-0 flex-col gap-2 rounded-lg border border-slate-200 px-3 py-2 sm:flex-row sm:items-center dark:border-slate-700"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm text-slate-800 dark:text-slate-100">
                        <span className="break-all font-mono">{item.tag}</span>
                        {item.active && (
                          <span className="text-xs text-emerald-600">
                            当前活动
                          </span>
                        )}
                      </div>
                      <div className="break-all text-xs leading-4 text-slate-400">
                        {item.cached
                          ? `${item.asset} · ${(item.size / 1024 / 1024).toFixed(1)} MB`
                          : '当前版本未进入缓存'}
                      </div>
                      {item.cached && (
                        <div
                          className="break-all text-[11px] leading-4 text-slate-400"
                          title={`SHA-256: ${item.archiveSha256 ?? ''}\n指纹: ${item.fingerprint ?? ''}`}
                        >
                          SHA-256 {item.archiveSha256 ?? '未知'} · 指纹{' '}
                          {item.fingerprint ?? '未知'}
                          {item.lastUsedAt
                            ? ` · 最近使用 ${new Date(item.lastUsedAt).toLocaleString()}`
                            : ''}
                        </div>
                      )}
                    </div>
                    {!item.active && item.cached && (
                      <div className="flex shrink-0 gap-1.5 self-end sm:self-auto">
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={switching}
                          onClick={() => void switchCachedVersion(item)}
                        >
                          切换
                        </Button>
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void removeCachedVersion(item)}
                        >
                          删除
                        </Button>
                      </div>
                    )}
                  </div>
                ))
              ) : (
                <p className="m-0 text-xs text-slate-500">暂无已缓存版本。</p>
              )}
            </div>
            <p className="m-0 text-xs leading-5 text-amber-600">
              切换版本会替换当前插件文件；插件自身管理的外部进程需要先停止。
            </p>
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void install(versionTarget)}
              >
                下载其他 Release
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={cleaningCache}
                onClick={() =>
                  void cleanupCache().then(() => refreshVersions())
                }
              >
                清理缓存
              </Button>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setVersionTarget(null)}
              >
                关闭
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </section>
  )
}
function stableHash(value: string) {
  let hash = 0
  for (let index = 0; index < value.length; index++) {
    hash = (hash * 31 + value.charCodeAt(index)) | 0
  }
  return (hash >>> 0).toString(36)
}

function SetupPluginCenter({ plugin }: { plugin: SetupPlugin }) {
  // A setup plugin's interface is its panel page, served same-origin by alx.
  // The declarative action-list model was removed; the panel calls the plugin's
  // action forward API itself.
  const hasPanel = Boolean(plugin.web && plugin.runnable && !plugin.online)
  const theme = document.documentElement.dataset.theme ?? 'light'
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const [finderRequest, setFinderRequest] = useState<{
    requestId: string
    kind: 'directory' | 'file'
    title: string
    multiple: boolean
    source?: Window
  } | null>(null)
  const panelSrc =
    plugin.developmentSource && plugin.developmentWebProxy
      ? `/api/v1/setup/plugins/development/${plugin.id}/web/?theme=${theme}`
      : `/api/v1/setup/plugins/web/${plugin.id}/index.html?theme=${theme}`
  const postHostResult = useCallback(
    (
      type: string,
      requestId: string,
      result: Record<string, unknown>,
      target?: Window | null
    ) => {
      const win = target ?? iframeRef.current?.contentWindow
      win?.postMessage(
        {
          source: 'alx-parent',
          type,
          requestId,
          ...result
        },
        window.location.origin
      )
    },
    []
  )
  const [hostWebviews, setHostWebviews] = useState<HostWebview[]>([])
  const hostWebviewsRef = useRef<HostWebview[]>([])
  const webviewFrames = useRef(new Map<string, Window>())
  const hostWebviewLayer = useRef(1080)
  const [uiDialogQueue, setUiDialogQueue] = useState<HostUiRequest[]>([])
  const uiDialogQueueRef = useRef<HostUiRequest[]>([])
  const [activeModalBusy, setActiveModalBusy] = useState(false)
  const [uiToasts, setUiToasts] = useState<HostToast[]>([])
  useEffect(() => {
    hostWebviewsRef.current = hostWebviews
  }, [hostWebviews])
  useEffect(() => {
    uiDialogQueueRef.current = uiDialogQueue
  }, [uiDialogQueue])
  useEffect(() => {
    setActiveModalBusy(false)
  }, [uiDialogQueue[0]?.requestId])
  const registerWebviewFrame = useCallback((id: string, win: Window | null) => {
    const frames = webviewFrames.current
    if (win) frames.set(id, win)
    else frames.delete(id)
  }, [])
  useEffect(() => {
    const receivePluginBridgeMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return
      const sourceWindow = event.source as Window | null
      const isMainFrame = sourceWindow === iframeRef.current?.contentWindow
      let sourceWebviewId: string | null = null
      if (!isMainFrame && sourceWindow) {
        for (const [id, win] of webviewFrames.current) {
          if (win === sourceWindow) {
            sourceWebviewId = id
            break
          }
        }
      }
      if (!isMainFrame && !sourceWebviewId) return
      const value = event.data as {
        source?: string
        type?: string
        requestId?: string
        pluginId?: string
        pickerId?: string
        webviewId?: string
        webview?: {
          title?: string
          src?: string
          kind?: string
          width?: number
          height?: number
        }
        ui?: {
          kind?: 'alert' | 'message' | 'modal' | 'notification' | 'set-busy'
          busy?: boolean
          title?: string
          message?: string
          confirmText?: string
          cancelText?: string
          type?: 'info' | 'success' | 'warning' | 'error'
          duration?: number
        }
      }
      if (
        value?.source !== 'alx-setup-plugin' ||
        !value.requestId ||
        value.pluginId !== plugin.id
      )
        return
      const requestId = value.requestId
      if (value.type === 'finder-request') {
        if (!value.pickerId) return
        if (finderRequest) {
          postHostResult(
            'finder-result',
            requestId,
            { error: '已有一个 Finder 选择正在进行。' },
            sourceWindow
          )
          return
        }
        void fetch('/api/v1/system/capabilities/finder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            pluginId: plugin.id,
            pickerId: value.pickerId
          })
        })
          .then(async response => {
            const body = (await response.json()) as {
              error?: string
              kind?: 'directory' | 'file'
              title?: string
              multiple?: boolean
            }
            if (
              !response.ok ||
              (body.kind !== 'directory' && body.kind !== 'file')
            )
              throw new Error(body.error || 'Finder 请求无效。')
            setFinderRequest({
              requestId,
              kind: body.kind,
              title: body.title || '选择位置',
              multiple: body.multiple === true,
              source: event.source as Window
            })
          })
          .catch(reason =>
            postHostResult(
              'finder-result',
              requestId,
              {
                error:
                  reason instanceof Error
                    ? reason.message
                    : 'Finder 请求未完成。'
              },
              sourceWindow
            )
          )
        return
      }
      if (value.type === 'webview-request') {
        if (!isMainFrame) {
          postHostResult(
            'webview-result',
            requestId,
            { ok: false, error: '仅插件主页可管理 WebView。' },
            sourceWindow
          )
          return
        }
        const webview = value.webview
        if (
          !webview ||
          typeof webview.src !== 'string' ||
          typeof webview.title !== 'string'
        ) {
          postHostResult(
            'webview-result',
            requestId,
            { ok: false, error: 'WebView 参数无效。' },
            sourceWindow
          )
          return
        }
        if (hostWebviewsRef.current.length >= 8) {
          postHostResult(
            'webview-result',
            requestId,
            { ok: false, error: '该插件打开的 WebView 数量已达上限（8 个）。' },
            sourceWindow
          )
          return
        }
        const offset = (hostWebviewsRef.current.length % 6) * 32
        const isStatic = webview.kind === 'static'
        const keySrc = webview.src.replace(/[?&]theme=[^&#]*/g, '')
        const item: HostWebview = {
          id: requestId,
          title: webview.title,
          src: isStatic
            ? webview.src +
              (webview.src.includes('?') ? '&' : '?') +
              `theme=${encodeURIComponent(theme)}`
            : webview.src,
          kind: isStatic ? 'static' : 'url',
          width: Math.max(480, Math.min(1600, Number(webview.width) || 960)),
          height: Math.max(420, Math.min(1200, Number(webview.height) || 680)),
          left: 120 + offset,
          top: 96 + Math.round(offset * 0.875),
          storageKey: `alx:plugin-webview:${plugin.id}:${stableHash(keySrc)}`,
          minimized: false,
          zIndex: ++hostWebviewLayer.current
        }
        setHostWebviews(list => [...list, item])
        window.requestAnimationFrame(() =>
          postHostResult(
            'webview-result',
            requestId,
            { ok: true, id: requestId },
            sourceWindow
          )
        )
        return
      }
      if (value.type === 'webview-close-request') {
        if (!isMainFrame) {
          postHostResult(
            'webview-close-result',
            requestId,
            { ok: false, error: '仅插件主页可管理 WebView。' },
            sourceWindow
          )
          return
        }
        const webviewId =
          typeof value.webviewId === 'string' && value.webviewId
            ? value.webviewId
            : ''
        const current = hostWebviewsRef.current
        const next = webviewId
          ? current.filter(item => item.id !== webviewId)
          : []
        setHostWebviews(next)
        postHostResult(
          'webview-close-result',
          requestId,
          { ok: true, closed: current.length - next.length },
          sourceWindow
        )
        return
      }
      if (value.type === 'ui-request') {
        const ui = value.ui
        if (!ui || typeof ui.kind !== 'string') return
        const kind = ui.kind
        if (kind === 'set-busy') {
          const active = uiDialogQueueRef.current[0]
          if (active?.kind !== 'modal') {
            postHostResult(
              'ui-result',
              requestId,
              { ok: false, error: '当前没有可更新的确认弹窗。' },
              sourceWindow
            )
          } else {
            setActiveModalBusy(ui.busy === true)
            postHostResult('ui-result', requestId, { ok: true }, sourceWindow)
          }
          return
        }
        if (kind === 'alert' || kind === 'modal') {
          if (uiDialogQueueRef.current.length >= 8) {
            postHostResult(
              'ui-result',
              requestId,
              { ok: false, error: '宿主弹窗队列已达上限（8 个）。' },
              sourceWindow
            )
            return
          }
          setUiDialogQueue(list => [
            ...list,
            {
              requestId,
              kind,
              source: sourceWindow ?? undefined,
              title: ui.title,
              message: ui.message,
              confirmText: ui.confirmText,
              cancelText: ui.cancelText
            }
          ])
          return
        }
        if (kind === 'message' || kind === 'notification') {
          const toastKind: HostToast['kind'] = kind
          const fallback = toastKind === 'message' ? 4000 : 6000
          const duration = Math.max(
            1500,
            Math.min(15000, Number(ui.duration) || fallback)
          )
          const toastType: HostToast['type'] =
            ui.type === 'success' ||
            ui.type === 'warning' ||
            ui.type === 'error'
              ? ui.type
              : 'info'
          setUiToasts(list => {
            const next = [
              ...list,
              {
                id: requestId,
                kind: toastKind,
                type: toastType,
                title: ui.title,
                message: ui.message,
                duration
              }
            ]
            const sameKind = next.filter(toast => toast.kind === kind)
            return sameKind.length > 5
              ? next.filter(toast => toast.id !== sameKind[0].id)
              : next
          })
          postHostResult('ui-result', requestId, { ok: true }, sourceWindow)
          return
        }
        postHostResult(
          'ui-result',
          requestId,
          { ok: false, error: '不支持的宿主 UI 组件。' },
          sourceWindow
        )
      }
    }
    window.addEventListener('message', receivePluginBridgeMessage)
    return () =>
      window.removeEventListener('message', receivePluginBridgeMessage)
  }, [finderRequest, plugin.id, postHostResult, theme])
  const applyScrollbarTheme = (event: SyntheticEvent<HTMLIFrameElement>) => {
    const document = event.currentTarget.contentDocument
    if (!document || document.head.querySelector('[data-alx-scrollbar-theme]'))
      return
    const style = document.createElement('style')
    style.dataset.alxScrollbarTheme = 'true'
    style.textContent = `
      html { scrollbar-color: rgb(148 163 184 / 0.55) transparent; scrollbar-width: thin; }
      ::-webkit-scrollbar { width: 10px; height: 10px; }
      ::-webkit-scrollbar-track { background: transparent; }
      ::-webkit-scrollbar-thumb { background: rgb(148 163 184 / 0.42); border: 3px solid transparent; border-radius: 999px; background-clip: padding-box; }
      ::-webkit-scrollbar-thumb:hover { background-color: rgb(100 116 139 / 0.7); }
      ::-webkit-scrollbar-corner { background: transparent; }
    `
    document.head.append(style)
  }

  const activeUiDialog = uiDialogQueue[0] ?? null
  const activeUiDialogKind = activeUiDialog?.kind
  const dismissUiDialog = (result: Record<string, unknown>) => {
    const active = uiDialogQueue[0]
    if (active)
      postHostResult('ui-result', active.requestId, result, active.source)
    setUiDialogQueue(list => list.slice(1))
  }

  return (
    <>
      <section className="setup-plugin-panel">
        {hasPanel ? (
          <iframe
            ref={iframeRef}
            className="setup-plugin-panel-frame"
            src={panelSrc}
            title={`${plugin.name} 界面`}
            onLoad={applyScrollbarTheme}
          />
        ) : (
          <div className="setup-plugin-panel-missing grid gap-2">
            <strong className="text-sm text-slate-700 dark:text-slate-200">
              此插件需要面板页
            </strong>
            <p className="m-0 text-xs leading-5 text-slate-500">
              {plugin.online
                ? '该插件由在线目录识别，安装到本机后才能打开其界面。'
                : '插件清单未声明 web 目录，或缺少可用的执行器，因此无法展示界面。'}
            </p>
          </div>
        )}
        <DirectoryPicker
          open={finderRequest !== null}
          title={finderRequest?.title ?? '选择位置'}
          multiple={finderRequest?.multiple ?? false}
          priority
          includeFiles={finderRequest?.kind === 'file'}
          selectionMode={finderRequest?.kind === 'file' ? 'file' : 'directory'}
          onClose={() => {
            if (finderRequest)
              postHostResult(
                'finder-result',
                finderRequest.requestId,
                { error: '已取消选择。' },
                finderRequest.source
              )
            setFinderRequest(null)
          }}
          onSelect={paths => {
            if (finderRequest)
              postHostResult(
                'finder-result',
                finderRequest.requestId,
                { paths },
                finderRequest.source
              )
            setFinderRequest(null)
          }}
        />
      </section>
      {hostWebviews.map(webview => (
        <PluginWebviewWindow
          key={webview.id}
          webview={webview}
          onFrame={registerWebviewFrame}
          onClose={() =>
            setHostWebviews(list => list.filter(item => item.id !== webview.id))
          }
          onMinimize={() =>
            setHostWebviews(list =>
              list.map(item =>
                item.id === webview.id
                  ? { ...item, minimized: !item.minimized }
                  : item
              )
            )
          }
          onActivate={() =>
            setHostWebviews(list =>
              list.map(item =>
                item.id === webview.id
                  ? { ...item, zIndex: ++hostWebviewLayer.current }
                  : item
              )
            )
          }
        />
      ))}
      <HostPluginAlert
        open={activeUiDialogKind === 'alert'}
        title={activeUiDialog?.title ?? ''}
        message={activeUiDialog?.message ?? ''}
        confirmText={activeUiDialog?.confirmText}
        onClose={() => dismissUiDialog({ ok: true })}
      />
      <HostPluginModal
        request={activeUiDialogKind === 'modal' ? activeUiDialog : null}
        busy={activeModalBusy}
        onConfirm={() => dismissUiDialog({ ok: true, confirmed: true })}
        onCancel={() => dismissUiDialog({ ok: true, confirmed: false })}
      />
      <HostUiToasts
        toasts={uiToasts}
        onDismiss={id =>
          setUiToasts(list => list.filter(toast => toast.id !== id))
        }
      />
    </>
  )
}
// PluginUploadDropzone is a fixed-size drag-and-drop / click upload control
// shared by the system plugin center and the robot backpack. It only forwards
// one selected file; the caller owns validation, upload and error reporting.
function PluginUploadDropzone({
  label,
  hint,
  busy,
  onFile
}: {
  label: string
  hint: string
  busy: boolean
  onFile: (file: File) => void
}) {
  const inputID = useId()
  const [dragOver, setDragOver] = useState(false)
  return (
    <div
      className="grid"
      onDragOver={event => {
        event.preventDefault()
        if (!busy) setDragOver(true)
      }}
      onDragLeave={() => setDragOver(false)}
      onDrop={event => {
        event.preventDefault()
        setDragOver(false)
        if (busy) return
        const file = event.dataTransfer.files?.[0] ?? null
        if (file) onFile(file)
      }}
    >
      <input
        className="sr-only"
        id={inputID}
        type="file"
        accept=".zip,.tar.gz,.tgz,application/zip,application/gzip"
        disabled={busy}
        onChange={event => {
          const file = event.target.files?.[0] ?? null
          event.currentTarget.value = ''
          if (file) onFile(file)
        }}
      />
      <label
        htmlFor={inputID}
        className={cn(
          'flex cursor-pointer items-center gap-2.5 rounded-lg border border-dashed p-3 transition',
          busy
            ? 'cursor-default border-brand-300 bg-brand-50/60'
            : dragOver
              ? 'border-brand-600 bg-brand-50'
              : 'border-brand-200 bg-brand-50/40 hover:border-brand-600 hover:bg-brand-50'
        )}
      >
        <i className="inline-flex size-8 shrink-0 items-center justify-center rounded-md bg-brand-50 text-brand-600">
          {busy ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Upload className="size-4" />
          )}
        </i>
        <span className="grid min-w-0 flex-1 gap-0.5">
          <strong className="truncate text-xs font-medium text-slate-700 dark:text-slate-200">
            {busy ? '正在上传…' : label}
          </strong>
          <small className="text-[11px] leading-4 text-slate-500 dark:text-slate-400">
            {hint}
          </small>
        </span>
      </label>
    </div>
  )
}

function BackpackPanel({
  root,
  items,
  loading,
  failed,
  onRefresh,
  onOpenPlugins,
  onClone,
  busy,
  onSetPrivate,
  onSaveConfig,
  onConfigChanged,
  onRemove,
  onReplace
}: {
  root: string
  items: Array<{
    name: string
    version?: string
    description?: string
    path: string
    valid: boolean
  }>
  loading: boolean
  failed: boolean
  onRefresh: () => void
  onOpenPlugins: () => void
  onClone: () => void
  busy: boolean
  onSetPrivate: (enabled: boolean) => Promise<boolean>
  onSaveConfig: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
  onConfigChanged: () => Promise<void>
  onRemove: (packageName: string) => Promise<void>
  onReplace: (
    packageName: string,
    version: string,
    force?: boolean
  ) => Promise<boolean>
}) {
  const [selectedName, setSelectedName] = useStoreState('')
  const [appToggleBusy, setAppToggleBusy] = useStoreState('')
  const [privacyToggleBusy, setPrivacyToggleBusy] = useStoreState(false)
  const [uploadRobotPackage, { isLoading: uploadingPackage }] =
    useUploadRobotPackageMutation()
  const [uploadPackageError, setUploadPackageError] = useState('')
  const [uploadPackageDone, setUploadPackageDone] = useState('')
  const { data: appsData, refetch: refetchApps } = useRobotAppsQuery(root, {
    skip: !root
  })
  const {
    data: manifest,
    isFetching: manifestLoading,
    refetch: refetchManifest
  } = usePackageManifestQuery(root, { skip: !root })
  const [setAppEnabled] = useSetAppEnabledMutation()
  const enabledApps = new Set(appsData?.items ?? [])
  const isPrivateBackpack = Boolean(
    manifest?.private &&
    manifest.workspacesEnabled &&
    manifest.workspaces?.includes('packages/*')
  )
  useEffect(() => {
    if (selectedName && !items.some(item => item.name === selectedName))
      setSelectedName('')
  }, [items, selectedName, setSelectedName])
  const selected = items.find(item => item.name === selectedName)
  const toggleApp = async (packageName: string, enabled: boolean) => {
    if (!root) return
    setAppToggleBusy(packageName)
    try {
      await setAppEnabled({ root, package: packageName, enabled }).unwrap()
      await Promise.all([refetchApps(), onConfigChanged()])
    } finally {
      setAppToggleBusy('')
    }
  }
  const toggleBackpackPrivacy = async () => {
    if (!manifest || privacyToggleBusy) return
    setPrivacyToggleBusy(true)
    try {
      if (await onSetPrivate(!isPrivateBackpack)) await refetchManifest()
    } finally {
      setPrivacyToggleBusy(false)
    }
  }
  const uploadPackageArchive = async (file: File) => {
    setUploadPackageError('')
    setUploadPackageDone('')
    try {
      const installed = await uploadRobotPackage({ root, file }).unwrap()
      setUploadPackageDone(`已上传“${installed.name}”。`)
      await onRefresh()
    } catch (reason) {
      setUploadPackageError(operationErrorMessage(reason, '插件包上传未完成。'))
    }
  }
  if (selected)
    return (
      <BackpackPackageManager
        root={root}
        item={selected}
        busy={busy}
        onSave={onSaveConfig}
        onRemove={onRemove}
        onReplace={onReplace}
        onBack={() => setSelectedName('')}
        onRefresh={onRefresh}
      />
    )
  return (
    <RobotPanel
      className="backpack-panel"
      icon={<Archive className="size-4" />}
      title="背包"
      description={<span title={`${root}/packages`}>packages</span>}
      actions={
        <>
          <button
            type="button"
            className={cn(
              'secondary-button min-h-9 gap-1.5',
              isPrivateBackpack && 'border-brand-300 bg-brand-50 text-brand-700'
            )}
            disabled={!manifest || manifestLoading || privacyToggleBusy || busy}
            onClick={() => void toggleBackpackPrivacy()}
            aria-pressed={isPrivateBackpack}
            title={isPrivateBackpack ? '切换为公开背包' : '切换为私有背包'}
          >
            {privacyToggleBusy || manifestLoading ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : isPrivateBackpack ? (
              <EyeOff className="size-3.5" />
            ) : (
              <Eye className="size-3.5" />
            )}
            {isPrivateBackpack ? '私有' : '公开'}
            <span
              className={cn(
                'relative ml-0.5 inline-flex shrink-0 items-center rounded-full transition',
                isPrivateBackpack ? 'bg-brand-600' : 'bg-slate-300'
              )}
              style={{
                boxSizing: 'border-box',
                height: 16,
                padding: 2,
                width: 28
              }}
              aria-hidden="true"
            >
              <span
                className="block shrink-0 rounded-full bg-white shadow-sm transition-transform"
                style={{
                  height: 12,
                  transform: `translateX(${isPrivateBackpack ? 12 : 0}px)`,
                  width: 12
                }}
              />
            </span>
          </button>
          <button className="text-button gap-1.5" onClick={onOpenPlugins}>
            <Blocks className="size-4" />
            插件中心
          </button>
          <button
            className="secondary-button min-h-9 gap-1.5"
            onClick={onClone}
          >
            <GitBranch className="size-3.5" />
            克隆
          </button>
          <button
            className="icon-button size-9 shrink-0 p-0"
            disabled={loading}
            onClick={onRefresh}
            aria-label="刷新背包"
            title="刷新背包"
          >
            {loading ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <RefreshCw className="size-4" />
            )}
          </button>
        </>
      }
    >
      <section className="grid gap-1.5">
        <PluginUploadDropzone
          label="拖入或点击上传"
          hint="支持 .zip / .tar.gz / .tgz，上传内含 package.json 的 AlemonJS 插件包。"
          busy={uploadingPackage}
          onFile={file => void uploadPackageArchive(file)}
        />
        {uploadPackageError && (
          <small className="rounded-md border border-amber-200 bg-amber-50 p-2 text-[11px] leading-4 text-amber-800">
            {uploadPackageError}
          </small>
        )}
        {uploadPackageDone && (
          <small className="rounded-md border border-emerald-200 bg-emerald-50 p-2 text-[11px] leading-4 text-emerald-700">
            {uploadPackageDone}
          </small>
        )}
      </section>
      <div className="grid gap-2">
        {loading ? (
          <p className="grid min-h-32 place-items-center text-sm text-slate-500">
            正在读取本地插件包…
          </p>
        ) : items.length ? (
          <div className="grid gap-2">
            {items.map(item => (
              <article
                className={cn(
                  'rounded-lg border bg-white transition hover:border-slate-300',
                  item.valid
                    ? 'border-slate-200'
                    : 'border-amber-200 bg-amber-50'
                )}
                key={item.path}
              >
                <button
                  type="button"
                  className="flex w-full items-center gap-3 p-3 text-left"
                  onClick={() => setSelectedName(item.name)}
                >
                  <div className="min-w-0 flex-1">
                    <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                      {item.name}
                      {item.version && (
                        <em className="not-italic text-xs text-slate-400">
                          v{item.version}
                        </em>
                      )}
                    </strong>
                    <span className="text-xs text-slate-500">
                      {item.valid
                        ? item.description || '本地 AlemonJS 插件包'
                        : '缺少有效 package.json，暂不能作为插件运行。'}
                    </span>
                    <small
                      className="truncate text-[11px] text-slate-400"
                      title={item.path}
                    >
                      {item.path}
                    </small>
                  </div>
                  <ChevronRight className="size-4 shrink-0 text-slate-400" />
                </button>
                <div className="flex justify-end shrink-0 items-center gap-2 border-t border-slate-100 px-3 py-2">
                  <span className="text-[11px] text-slate-400">
                    {enabledApps.has(item.name)
                      ? '已加入 apps，机器人启动时会加载'
                      : '未加入 apps'}
                  </span>
                  {item.valid && (
                    <button
                      className={
                        enabledApps.has(item.name)
                          ? 'secondary-button gap-1.5'
                          : 'primary-button gap-1.5'
                      }
                      disabled={appToggleBusy === item.name}
                      onClick={() =>
                        void toggleApp(item.name, !enabledApps.has(item.name))
                      }
                    >
                      {appToggleBusy === item.name ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : enabledApps.has(item.name) ? (
                        <X className="size-4" />
                      ) : (
                        <Play className="size-4" />
                      )}
                      {appToggleBusy === item.name
                        ? '切换中…'
                        : enabledApps.has(item.name)
                          ? '停用'
                          : '启动'}
                    </button>
                  )}
                  {!item.valid && (
                    <button
                      className="inline-flex h-8 items-center justify-center gap-1.5 rounded-md border border-red-200 px-2.5 text-xs font-semibold text-red-700 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-60"
                      disabled={busy}
                      onClick={() => void onRemove(item.name)}
                      title="删除安装失败遗留的目录"
                    >
                      <Trash2 className="size-3.5" />
                      删除
                    </button>
                  )}
                </div>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState
            icon={
              <Archive className="size-6 text-slate-400 dark:text-slate-500" />
            }
            title={failed ? '暂时无法读取背包' : '背包还是空的'}
            description={
              failed
                ? '请检查 packages 目录后刷新重试。'
                : '克隆或安装插件包后，会显示在这里。'
            }
          />
        )}
      </div>
    </RobotPanel>
  )
}

function BackpackPackageManager({
  root,
  item,
  busy,
  onSave,
  onRemove,
  onReplace,
  onBack,
  onRefresh
}: {
  root: string
  item: {
    name: string
    version?: string
    description?: string
    path: string
    valid: boolean
  }
  busy: boolean
  onSave: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
  onRemove: (packageName: string) => Promise<void>
  onReplace: (
    packageName: string,
    version: string,
    force?: boolean
  ) => Promise<boolean>
  onBack: () => void
  onRefresh: () => void
}) {
  const [tab, setTab] = useStoreState<'readme' | 'config' | 'version'>('readme')
  const [version, setVersion] = useStoreState('')
  const [forceSwitchOpen, setForceSwitchOpen] = useStoreState(false)
  const {
    data,
    isLoading: isConfigLoading,
    error
  } = usePackageConfigQuery({ root, package: item.name }, { skip: !item.valid })
  const {
    data: readme,
    isFetching: isReadmeFetching,
    error: readmeError
  } = useLocalPackageReadmeQuery(
    { root, package: item.name },
    { skip: !item.valid || tab !== 'readme' }
  )
  const {
    data: versions,
    isFetching: versionsFetching,
    error: versionsError
  } = useLocalPackageVersionsQuery(
    { root, package: item.name },
    {
      skip: !item.valid || tab !== 'version'
    }
  )
  const [values, setValues] = useStoreState<Record<string, unknown>>({})
  const scheduleSave = useAutoSave<Record<string, unknown>>(next =>
    onSave(item.name, next)
  )
  const updateValue = (name: string, value: unknown) => {
    const next = { ...values, [name]: value }
    setValues(next)
    scheduleSave(next)
  }
  useEffect(() => {
    if (!data) return
    const next: Record<string, unknown> = {}
    for (const field of data.fields ?? []) {
      if (field.name in data.values) {
        next[field.name] = data.values[field.name]
      } else if (field.default !== undefined && field.default !== null) {
        next[field.name] = field.default
      }
    }
    setValues(current => (sameConfigValues(current, next) ? current : next))
  }, [data, setValues])
  useEffect(() => {
    if (versions?.latest) setVersion(versions.latest)
  }, [versions, setVersion])
  return (
    <RobotPanel
      className="backpack-manager"
      icon={<Package className="size-4" />}

      title={
        <>
          {item.name}
          {item.version && (
            <em className="ml-2 not-italic text-xs text-slate-400">
              v{item.version}
            </em>
          )}
        </>
      }
      actions={
        <>
          <button className="text-button gap-1.5" onClick={onBack}>
            <ArrowLeft className="size-4" />
            返回背包
          </button>
          <button
            className="icon-button size-9 p-0"
            onClick={onRefresh}
            title="刷新背包"
          >
            <RefreshCw className="size-4" />
          </button>
          <button
            className="inline-flex h-9 items-center justify-center gap-1.5 rounded-md border border-red-200 px-3 text-xs font-semibold text-red-700 transition hover:bg-red-50"
            disabled={busy}
            onClick={() => void onRemove(item.name)}
          >
            <Trash2 className="size-4" />
            {item.valid ? '卸载' : '删除目录'}
          </button>
        </>
      }
    >
      <Tabs
        ariaLabel="插件详情"
        items={[
          {
            id: 'readme',
            label: '文档',
            icon: <FileText className="size-3.5" />
          },
          {
            id: 'config',
            label: '配置',
            icon: <Settings className="size-3.5" />
          },
          {
            id: 'version',
            label: '版本',
            icon: <GitBranch className="size-3.5" />
          }
        ]}
        onChange={setTab}
        value={tab}
      />
      <div className="grid gap-3">
        {!item.valid ? (
          <p className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-800">
            这个目录没有有效的 package.json，因此只能从文件系统修复或移除。
          </p>
        ) : tab === 'readme' ? (
          isReadmeFetching ? (
            <p className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs text-slate-500">
              正在读取 README.md…
            </p>
          ) : readmeError || !readme ? (
            <p className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs leading-5 text-slate-500">
              这个插件没有 README.md；请在“配置”页查看可用设置。
            </p>
          ) : (
            <MarkdownPage markdown={readme.output} />
          )
        ) : tab === 'config' ? (
          isConfigLoading ? (
            <p className="backpack-manager-note">正在读取插件的配置声明…</p>
          ) : error || !data || !data.fields?.length ? (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white p-4">
              <p className="m-0 text-sm text-slate-500">
                该插件没有可填写的可视化配置。使用方式请查看“文档”页。
              </p>
              <ConfigSourceLinks source={data?.configSource} />
            </div>
          ) : (
            <div className="package-config-panel grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
              <header className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
                <div className="grid gap-1">
                  <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                    <PlatformLogo logo={data.logo} className="size-4" />
                    插件配置
                  </strong>
                  <span className="text-xs text-slate-500">
                    保存到当前机器人的 alemon.config.yaml · {data.namespace}.*
                  </span>
                </div>
                <div className="flex items-center gap-3">
                  <ConfigSourceLinks source={data.configSource} />
                </div>
              </header>
              <ConfigFieldsEditor
                fields={data.fields ?? []}
                values={values}
                onChange={updateValue}
              />
              {data.commands?.length ? (
                <div className="grid gap-1.5 border-t border-slate-100 pt-3">
                  <strong className="text-xs font-semibold text-slate-600">
                    桌面命令
                  </strong>
                  {data.commands.map(command => (
                    <code
                      key={command.command}
                      className="rounded-md bg-slate-100 px-2 py-1 text-[11px] text-slate-600"
                    >
                      {command.name} · {command.command}
                    </code>
                  ))}
                </div>
              ) : null}
            </div>
          )
        ) : versionsFetching ? (
          <p className="backpack-manager-note">正在读取可安装版本…</p>
        ) : versionsError || !versions?.versions.length ? (
          <p className="backpack-manager-note">
            暂时无法读取此插件的版本。当前本地版本为 {item.version || '未知'}。
          </p>
        ) : (
          <section className="grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
            <div className="grid gap-1">
              <strong className="text-sm font-semibold text-slate-800">
                {versions.source === 'git' ? 'Git 版本' : 'npm 版本'}
              </strong>
              <span className="text-xs leading-5 text-slate-500">
                当前使用 {versions.current || item.version || '未知'}；
                {versions.source === 'git'
                  ? '此插件是 Git 工作区，版本以标签为准；强制切换会丢弃该插件的本地 Git 修改。'
                  : '未检测到 Git，使用 npm 已发布版本。'}
              </span>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2 border-t border-slate-200 pt-3">
              <select
                className="h-9 min-w-40 rounded-md border border-slate-300 bg-white px-2.5 text-xs font-medium text-slate-700 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                value={version}
                onChange={event => setVersion(event.target.value)}
              >
                {versions.versions.map(candidate => (
                  <option key={candidate} value={candidate}>
                    {versions.source === 'npm' ? `v${candidate}` : candidate}
                  </option>
                ))}
              </select>
              <button
                className="primary-button gap-1.5"
                disabled={
                  busy ||
                  !version ||
                  version === versions.current ||
                  version.replace(/^v/, '') === item.version
                }
                onClick={() => void onReplace(item.name, version)}
              >
                <GitBranch className="size-4" />
                切换版本
              </button>
              {versions.source === 'git' && (
                <button
                  className="inline-flex min-h-9 items-center justify-center gap-1.5 rounded-md border border-orange-200 px-3 text-xs font-semibold text-orange-700 transition hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={
                    busy ||
                    !version ||
                    version === versions.current ||
                    version.replace(/^v/, '') === item.version
                  }
                  onClick={() => setForceSwitchOpen(true)}
                >
                  <AlertTriangle className="size-4" />
                  强制切换
                </button>
              )}
            </div>
          </section>
        )}
      </div>
      <ConfirmDialog
        open={forceSwitchOpen}
        title="强制切换插件版本"
        subtitle="此操作只影响当前背包插件的 Git 工作区"
        message={`将切换“${item.name}”到 ${version}，并永久丢弃该插件目录中所有未提交及未跟踪的 Git 文件。\n\n机器人主项目和其他插件不会受到影响。`}
        confirmLabel="丢弃修改并切换"
        destructive
        busy={busy}
        onCancel={() => setForceSwitchOpen(false)}
        onConfirm={() => {
          void onReplace(item.name, version, true).then(success => {
            if (success) setForceSwitchOpen(false)
          })
        }}
      />
    </RobotPanel>
  )
}
function CatalogDetail({
  item,
  group,
  kind,
  busy,
  onBack,
  onRun,
  onSaveConfig
}: {
  item: CatalogItem
  group: string
  kind: 'connection' | 'module' | 'plugin'
  busy: boolean
  onBack: () => void
  onRun: (action: string, packageName: string) => void
  onSaveConfig: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
}) {
  const [version, setVersion] = useStoreState('')
  const packageName =
    item.install ||
    (item.name === 'alemonjs' || item.name.startsWith('@alemonjs/')
      ? item.name
      : '')
  const repositoryInstall = packageName.startsWith('git+')
  const npmPackage = Boolean(packageName && !repositoryInstall)
  const {
    data: packageVersions,
    isFetching: versionsLoading,
    error: versionsError
  } = useCatalogVersionsQuery(packageName, { skip: !packageName })
  useEffect(() => {
    setVersion('')
  }, [packageName, setVersion])
  useEffect(() => {
    if (!version && packageVersions?.latest) setVersion(packageVersions.latest)
  }, [packageVersions?.latest, version, setVersion])
  const noRepositoryTag =
    repositoryInstall &&
    !versionsLoading &&
    !versionsError &&
    packageVersions?.versions.length === 0
  const installTarget = version.trim()
    ? npmPackage
      ? `${packageName}@${version.trim()}`
      : `${packageName.split('#')[0]}#${version.trim()}`
    : packageName
  const installAction =
    kind === 'connection'
      ? 'install-connection'
      : kind === 'module'
        ? 'install-module'
        : 'install-package'
  const uninstallAction =
    kind === 'connection'
      ? 'uninstall-connection'
      : kind === 'module'
        ? 'uninstall-module'
        : 'uninstall-package'
  return (
    <RobotPanel
      className="catalog-detail max-w-190"
      icon={<Globe className="size-4" />}
      title={group}
      description={
        kind === 'module'
          ? '作为当前机器人项目依赖安装'
          : '查看版本、安装与配置'
      }
      actions={
        <>
          <button className="text-button gap-1.5" onClick={onBack}>
            <ArrowLeft className="size-4" />
            返回目录
          </button>
        </>
      }
    >
      <section className="catalog-control flex flex-wrap items-start justify-between gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <div className="grid min-w-0 gap-1">
          <h1 className="m-0 break-all text-lg font-semibold text-ink-950">
            {item.name}
          </h1>
          <p className="m-0 text-sm text-slate-500">
            {item.description || '在线生态目录条目'}
          </p>
        </div>
        <div className="flex flex-wrap items-end justify-end gap-2">
          {packageName ? (
            <label className="grid gap-1 text-[11px] font-semibold text-slate-500">
              <select
                className="h-9 min-w-32 rounded-md border border-slate-300 bg-white px-2 text-xs font-medium text-slate-700 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
                value={version}
                onChange={event => setVersion(event.target.value)}
                disabled={
                  versionsLoading || Boolean(versionsError) || noRepositoryTag
                }
              >
                {versionsLoading && <option value="">读取版本…</option>}
                {versionsError && <option value="">版本读取失败</option>}
                {noRepositoryTag && (
                  <option value="">该插件没有可用的 Release</option>
                )}
                {packageVersions?.versions.map(itemVersion => (
                  <option key={itemVersion} value={itemVersion}>
                    {itemVersion}
                    {itemVersion === packageVersions.latest ? ' · 最新版' : ''}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <span className="rounded-md bg-slate-100 px-2.5 py-2 text-xs font-semibold text-slate-600">
              {repositoryInstall ? 'Git' : '拒绝'}
            </span>
          )}
          <button
            className="primary-button"
            disabled={
              busy ||
              !packageName ||
              versionsLoading ||
              Boolean(versionsError) ||
              noRepositoryTag ||
              (repositoryInstall && !version.trim())
            }
            onClick={() => onRun(installAction, installTarget)}
          >
            {busy ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Package className="size-4" />
            )}
            {busy ? '处理中…' : kind === 'connection' ? '安装' : '安装'}
          </button>
          <button
            className="secondary-button gap-1.5"
            disabled={
              busy || !packageName || (kind === 'plugin' && repositoryInstall)
            }
            title={
              repositoryInstall && kind === 'plugin'
                ? '仓库插件请按文档卸载'
                : '卸载当前包'
            }
            onClick={() => onRun(uninstallAction, packageName)}
          >
            <Trash2 className="size-4" />
            卸载
          </button>
          {item.url && (
            <a
              className="inline-flex min-h-9 items-center justify-center rounded-md border border-slate-300 bg-white px-3 text-sm font-semibold text-slate-700 transition hover:border-slate-400 hover:bg-slate-50"
              href={item.url}
              target="_blank"
              rel="noreferrer"
            >
              ↗
            </a>
          )}
        </div>
      </section>
      {repositoryInstall && noRepositoryTag && (
        <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          该插件仓库没有可用的 Release，不能作为可复现的版本安装。
        </p>
      )}
      {repositoryInstall && versionsError && (
        <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          无法读取插件 Release，请检查网络后重试。
        </p>
      )}
      <PackageConfigPanel
        source={item.url}
        readmeURL={item.url}
        onSave={onSaveConfig}
      />
    </RobotPanel>
  )
}
function ConfigReadmeCard({
  docURL,
  document,
  loading,
  error
}: {
  docURL?: string
  document?: { source: string; markdown: string }
  loading: boolean
  error: boolean
}) {
  return (
    <section className="catalog-document grid gap-3 rounded-xl border border-slate-200 bg-white p-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <strong className="text-sm font-semibold text-slate-800">
          配置说明
        </strong>
        {docURL && (
          <a
            className="text-xs font-semibold text-slate-600 hover:text-slate-900"
            href={docURL}
            target="_blank"
            rel="noreferrer"
          >
            在浏览器打开 ↗
          </a>
        )}
      </header>
      {loading && <p className="text-sm text-slate-500">正在读取配置文档…</p>}
      {error && (
        <p className="text-sm text-slate-500">
          配置文档暂时无法读取，请使用右上角链接查看。
        </p>
      )}
      {document && (
        <div className="max-h-96 overflow-auto rounded-lg border border-slate-100 bg-slate-50/40 p-3">
          <MarkdownPage markdown={document.markdown} />
        </div>
      )}
    </section>
  )
}
function PackageConfigPanel({
  source,
  readmeURL,
  onSave
}: {
  source: string
  readmeURL?: string
  onSave: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
}) {
  const {
    data,
    isLoading: isConfigLoading,
    error
  } = useCatalogPackageConfigQuery(source, { skip: !source })
  const docURL = data?.configSource?.readme || readmeURL
  const {
    data: document,
    isFetching: isDocumentFetching,
    error: documentError
  } = useCatalogDocumentQuery(docURL ?? '', { skip: !docURL })
  const [values, setValues] = useStoreState<Record<string, unknown>>({})
  const scheduleSave = useAutoSave<Record<string, unknown>>(next =>
    onSave(data?.package ?? '', next)
  )
  const updateValue = (name: string, value: unknown) => {
    const next = { ...values, [name]: value }
    setValues(next)
    scheduleSave(next)
  }
  useEffect(() => {
    if (!data) return
    const next: Record<string, unknown> = {}
    for (const field of data.fields ?? []) {
      if (field.name in data.values) {
        next[field.name] = data.values[field.name]
      } else if (field.default !== undefined && field.default !== null) {
        next[field.name] = field.default
      }
    }
    setValues(current => (sameConfigValues(current, next) ? current : next))
  }, [data, setValues])
  if (isConfigLoading)
    return (
      <section className="package-config-panel grid gap-3 rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-500">
        <p>正在读取包配置声明…</p>
      </section>
    )
  if (error || !data)
    return (
      <section className="package-config-panel rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-500">
        <p>该条目没有可读取的 alemonjs.config 声明。</p>
      </section>
    )
  if (!data.fields?.length)
    return (
      <div className="grid gap-4">
        <section className="package-config-panel grid gap-3 rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-500">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="m-0">该条目没有可填写的配置项。</p>
            <ConfigSourceLinks
              source={data.configSource}
              readmeURL={readmeURL}
            />
          </div>
        </section>
        <ConfigReadmeCard
          docURL={docURL}
          document={document}
          loading={isDocumentFetching}
          error={Boolean(documentError)}
        />
      </div>
    )
  return (
    <div className="grid gap-4">
      <section className="package-config-panel grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
          <div className="grid gap-1">
            <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
              <PlatformLogo logo={data.logo} className="size-4" />
              运行配置
            </strong>
            <span className="text-xs text-slate-500">
              保存至 alemon.config.yaml · {data.namespace}.*
            </span>
          </div>
          <div className="flex items-center gap-3">
            <ConfigSourceLinks
              source={data.configSource}
              readmeURL={readmeURL}
            />
            <small className="text-xs text-slate-400">修改后自动保存</small>
          </div>
        </header>
        <ConfigFieldsEditor
          fields={data.fields ?? []}
          values={values}
          onChange={updateValue}
        />
      </section>
      <ConfigReadmeCard
        docURL={docURL}
        document={document}
        loading={isDocumentFetching}
        error={Boolean(documentError)}
      />
    </div>
  )
}
function CurrentProjectConfigPanel({
  config,
  loading,
  onSave
}: {
  config?: PackageConfig
  loading: boolean
  onSave: (values: Record<string, unknown>) => Promise<boolean>
}) {
  const [values, setValues] = useStoreState<Record<string, unknown>>({})
  const scheduleSave = useAutoSave(onSave)
  const updateValue = (name: string, value: unknown) => {
    const next = { ...values, [name]: value }
    setValues(next)
    scheduleSave(next)
  }
  useEffect(() => {
    if (config) {
      const next: Record<string, unknown> = {}
      for (const field of config.fields ?? []) {
        if (field.name in config.values) {
          next[field.name] = config.values[field.name]
        } else if (field.default !== undefined && field.default !== null) {
          next[field.name] = field.default
        }
      }
      setValues(current => (sameConfigValues(current, next) ? current : next))
    }
  }, [config, setValues])
  // A config declaration is optional. Do not turn its absence into an error
  // for ordinary robots that do not expose project-specific settings.
  if (loading)
    return (
      <section className="project-config-panel rounded-xl border border-slate-200 bg-white p-4 text-sm text-slate-500">
        <p>正在识别当前项目的扩展配置…</p>
      </section>
    )
  if (!config?.fields?.length) return null
  return (
    <section className="project-config-panel grid gap-4 rounded-xl border border-slate-200 bg-white p-4">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 pb-3">
        <div className="grid gap-1">
          <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
            <PlatformLogo logo={config.logo} className="size-4" />
            项目扩展配置
          </strong>
          <span className="text-xs text-slate-500">
            {config.package} · 保存至 alemon.config.yaml 的 {config.namespace}{' '}
            区域
          </span>
        </div>
        <div className="flex items-center gap-3">
          <ConfigSourceLinks source={config.configSource} />
        </div>
      </header>
      <ConfigFieldsEditor
        fields={config.fields ?? []}
        values={values}
        onChange={updateValue}
      />
      {config.commands?.length ? (
        <div className="grid gap-1.5 border-t border-slate-100 pt-3">
          <strong className="text-xs font-semibold text-slate-600">
            桌面命令
          </strong>
          {config.commands.map(command => (
            <code
              key={command.command}
              className="rounded-md bg-slate-100 px-2 py-1 text-[11px] text-slate-600"
            >
              {command.name} · {command.command}
            </code>
          ))}
        </div>
      ) : null}
    </section>
  )
}
function MarkdownPage({ markdown }: { markdown: string }) {
  return (
    <article className="markdown-page">
      <Markdown
        options={{
          forceBlock: true,
          overrides: {
            a: {
              component: ({
                href,
                children,
                ...props
              }: {
                href?: string
                children?: ReactNode
              }) => (
                <a href={href} target="_blank" rel="noreferrer" {...props}>
                  {children}
                </a>
              )
            }
          }
        }}
      >
        {markdown}
      </Markdown>
    </article>
  )
}
function RuntimePanel({
  overview,
  pm2Status,
  pm2StatusError,
  root,
  loading,
  busy,
  developmentRunning,
  foregroundRunning,
  developmentStopping,
  foregroundStopping,
  pm2Running,
  onRefresh,
  onRefreshOverview,
  onOpenPM2Logs,
  onOpenPM2Processes,
  onRun,
  onSaveLogin,
  onSavePackageConfig,
  liveLoginRequest,
  developerMode
}: {
  overview?: RuntimeOverview
  pm2Status?: PM2Status
  pm2StatusError: boolean
  root: string
  loading: boolean
  busy: boolean
  developmentRunning: boolean
  foregroundRunning: boolean
  developmentStopping: boolean
  foregroundStopping: boolean
  pm2Running: boolean
  onRefresh: () => void
  onRefreshOverview: () => Promise<RuntimeOverview | undefined>
  onOpenPM2Logs: () => void
  onOpenPM2Processes: () => void
  onRun: (action: string, packageName?: string) => Promise<boolean>
  onSaveLogin: (login: string, packageName?: string) => Promise<boolean>
  onSavePackageConfig: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
  liveLoginRequest: number
  developerMode: boolean
}) {
  type PendingAction = { label: string; note: string; execute: () => void }
  type LoginChoice = {
    action: string
    label: string
    note: string
    preflight: RuntimePreflight
    requireLogin?: boolean
  }
  const [customLogin, setCustomLogin] = useStoreState('')
  const [customPackage, setCustomPackage] = useStoreState('')
  const [selectedPlatform, setSelectedPlatform] = useStoreState('')
  const [pending, setPending] = useStoreState<PendingAction | null>(null)
  const [validationMessage, setValidationMessage] = useStoreState('')
  const [validationTitle, setValidationTitle] =
    useStoreState('运行前配置不完整')
  const [loadPackageConfig] = useLazyPackageConfigQuery()
  const [loadRuntimePreflight] = useLazyRobotRuntimePreflightQuery()
  const [loadRuntimeRepair] = useLazyRobotRuntimeRepairQuery()
  const [loadRobotPorts] = useLazyRobotPortsQuery()
  const [applyRuntimeRepair] = useApplyRuntimeRepairMutation()
  const [portStatus, setPortStatus] = useState<RobotPortStatus[]>([])
  const [portStatusError, setPortStatusError] = useState('')
  const [portStatusBusy, setPortStatusBusy] = useState(false)
  const refreshPorts = useCallback(async () => {
    if (!root) return
    setPortStatusBusy(true)
    try {
      const result = await loadRobotPorts(root, false).unwrap()
      setPortStatus(result.items ?? [])
      setPortStatusError('')
    } catch (reason) {
      setPortStatusError(
        operationErrorMessage(reason, '端口检测失败，请稍后重试。')
      )
    } finally {
      setPortStatusBusy(false)
    }
  }, [loadRobotPorts, root])
  useEffect(() => {
    void refreshPorts()
  }, [refreshPorts])
  const handleRefresh = () => {
    onRefresh()
    void refreshPorts()
  }
  const [loginChoice, setLoginChoice] = useStoreState<LoginChoice | null>(null)
  const [connectionConfig, setConnectionConfig] = useStoreState<{
    package: string
    fields: PackageConfigField[]
    values: Record<string, unknown>
    logo?: string
    configSource?: { readme?: string; official?: string; platform?: string }
  } | null>(null)
  const [connectionValues, setConnectionValues] = useStoreState<
    Record<string, unknown>
  >({})
  const [loginDialogError, setLoginDialogError] = useStoreState('')
  const [loginDialogBusy, setLoginDialogBusy] = useStoreState(false)
  const askStartRef = useRef<
    (
      action: string,
      label: string,
      note: string,
      requireLogin?: boolean
    ) => Promise<void>
  >(() => Promise.resolve())
  const persistentReady = overview?.pm2Configured && overview.hasStartScript
  const pm2Managed = Boolean(pm2Status?.managed)
  const pm2LocalRunning = pm2Running
  const localRunning = developmentRunning || foregroundRunning
  // A missing dependency blocks every run action until dependencies install.
  const knownPlatform = (overview?.platforms ?? []).find(
    item => item.id === selectedPlatform
  )
  const packageTarget = knownPlatform?.package || customPackage.trim()
  const connectionPackage = connectionConfig?.package || packageTarget
  const valuesForConnectionPackage = (values: Record<string, unknown>) => {
    if (!connectionConfig?.fields?.length) return values
    const allowed = new Set(connectionConfig.fields.map(field => field.name))
    return Object.fromEntries(
      Object.entries(values).filter(([name]) => allowed.has(name))
    )
  }
  const scheduleConnectionSave = useAutoSave<Record<string, unknown>>(next => {
    if (!connectionPackage || !connectionConfig?.fields?.length) return
    return onSavePackageConfig(
      connectionPackage,
      valuesForConnectionPackage(next)
    )
  })
  const scheduleLoginSave = useAutoSave<{
    login: string
    packageName: string
  }>(({ login, packageName }) => {
    if (!login.trim()) return
    return onSaveLogin(login, packageName)
  })
  const updateConnectionValue = (name: string, value: unknown) => {
    const next = { ...connectionValues, [name]: value }
    setConnectionValues(next)
    scheduleConnectionSave(next)
    // A stale "请先填写必填项" banner from a previous start attempt must not
    // outlive the edit that resolves it; the start button and the click-time
    // validation share the same check, so once the banner is actionable it is
    // already contradictory.
    if (loginDialogError) setLoginDialogError('')
  }
  const ask = (label: string, note: string, execute: () => void) =>
    setPending({ label, note, execute })
  const repairRuntime = async (mode: string) => {
    try {
      const plan = await loadRuntimeRepair({ root, mode }, false).unwrap()
      if (plan.blocked.length) {
        setValidationTitle('无法自动修复')
        setValidationMessage(plan.blocked.join('。'))
        return
      }
      const apply = async (confirmOverrides: boolean) => {
        try {
          const result = await applyRuntimeRepair({
            root,
            mode,
            confirmOverrides
          }).unwrap()
          setValidationTitle(
            result.phase === 'healthy' ? '修复完成' : '修复结果'
          )
          setValidationMessage(result.output || result.diagnostics.join('。'))
          await onRefreshOverview()
        } catch (reason) {
          setValidationTitle('修复未完成')
          setValidationMessage(
            operationErrorMessage(reason, '修复失败，已尝试恢复运行配置。')
          )
        }
      }
      const details =
        [...plan.automatic, ...plan.requiresConfirmation].join('；') ||
        '当前运行配置无需修改。'
      if (plan.requiresConfirmation.length) {
        ask('确认覆盖自定义运行配置', details, () => {
          void apply(true)
        })
      } else {
        await apply(false)
      }
    } catch (reason) {
      setValidationTitle('无法读取修复诊断')
      setValidationMessage(
        operationErrorMessage(reason, '无法生成运行修复计划。')
      )
    }
  }
  const confirm = () => {
    pending?.execute()
    setPending(null)
  }
  // "谁最后启动谁为准": starting a new mode automatically stops the previous
  // one (the backend stops a running PM2 service before a local start, and a
  // local process before a background start). No upfront block here.
  const askStart = async (
    action: string,
    label: string,
    note: string,
    requireLogin = false
  ) => {
    try {
      // Bypass the 1-hour query cache so a just-installed connection package is
      // already reflected when the start dialog opens.
      setValidationTitle('运行前配置不完整')
      const preflight = await loadRuntimePreflight(root, false).unwrap()
      await refreshPorts()
      const freshOverview = await onRefreshOverview()
      const platform = (freshOverview?.platforms ?? []).find(
        item => item.id === preflight.login
      )
      setCustomLogin(preflight.login)
      setSelectedPlatform(platform?.id ?? (preflight.login ? '__custom__' : ''))
      setCustomPackage(platform?.package ?? '')
      setConnectionConfig(null)
      setConnectionValues({})
      if (platform?.installed && platform.package)
        void loadConnectionConfig(platform.package)
      setLoginDialogError('')
      setLoginChoice({ action, label, note, preflight, requireLogin })
    } catch (reason) {
      setValidationMessage(
        operationErrorMessage(reason, '无法完成运行前检查。')
      )
    }
  }
  askStartRef.current = askStart
  const handledLiveLoginRequest = useRef(0)
  useEffect(() => {
    if (
      !liveLoginRequest ||
      handledLiveLoginRequest.current === liveLoginRequest
    )
      return
    handledLiveLoginRequest.current = liveLoginRequest
    void askStartRef.current(
      'app',
      '登录并启动机器人',
      '在线聊天需要机器人以登录连接运行；启动后可重新打开聊天。',
      true
    )
  }, [liveLoginRequest])
  const closeLoginDialog = () => {
    setLoginChoice(null)
    setLoginDialogError('')
  }
  const loadConnectionConfig = async (packageName: string) => {
    if (!packageName) {
      setConnectionConfig(null)
      setConnectionValues({})
      return
    }
    try {
      const config = await loadPackageConfig({
        root,
        package: packageName
      }).unwrap()
      setConnectionConfig(config)
      setConnectionValues(current =>
        sameConfigValues(current, config.values) ? current : config.values
      )
    } catch (reason) {
      const message = operationErrorMessage(reason, '无法读取连接包配置。')
      // A config declaration is optional; it is valid to continue without a
      // form when the installed package declares no alemonjs.config fields.
      if (message.includes('没有声明 alemonjs.config')) {
        setConnectionConfig({
          package: packageName,
          fields: [],
          values: {},
          logo: ''
        })
        setConnectionValues({})
        return
      }
      setLoginDialogError(message)
    }
  }
  const choosePlatform = async (id: string) => {
    setSelectedPlatform(id)
    if (!id) {
      // "不选择" clears every login trace so the dialog looks untouched.
      setCustomLogin('')
      setCustomPackage('')
      setConnectionConfig(null)
      setConnectionValues({})
      setLoginDialogError('')
      return
    }
    if (id === '__custom__') {
      setLoginDialogError('')
      return
    }
    // Use the freshest installed flag, not the possibly-stale render snapshot,
    // so a package installed moments ago loads its config (with any saved
    // required values) instead of being treated as "not installed".
    const freshOverview = await onRefreshOverview()
    const platform = (
      freshOverview?.platforms ??
      overview?.platforms ??
      []
    ).find(item => item.id === id)
    if (platform) {
      setCustomLogin(platform.id)
      setCustomPackage(platform.package)
      setLoginDialogError('')
      void loadConnectionConfig(platform.package)
    }
  }
  const installSelectedConnection = async () => {
    if (!packageTarget) return
    setLoginDialogBusy(true)
    try {
      if (await onRun('install-connection', packageTarget)) {
        await loadConnectionConfig(packageTarget)
        // Re-read the preflight so the "确认启动" gate sees the package as
        // installed instead of keeping it disabled on the pre-install snapshot.
        const preflight = await loadRuntimePreflight(root, false).unwrap()
        setLoginChoice(current =>
          current ? { ...current, preflight } : current
        )
        setLoginDialogError('连接包已安装。请填写下方配置后点击“启动”。')
      }
    } catch (reason) {
      setLoginDialogError(operationErrorMessage(reason, '连接包安装未完成。'))
    } finally {
      setLoginDialogBusy(false)
    }
  }
  // Unified start action for the login dialog. It always saves the current
  // connection config (and login when one is chosen), then starts the robot.
  // Without a login it starts directly; with one it persists the required
  // fields silently before launching.
  const startFromDialog = async () => {
    if (!loginChoice) return
    // 登录值只来自用户的选择：已识别平台或自由输入。选择“不选择”时清空，
    // 直接无 login 启动，避免沿用文件里的旧登录连接。
    const login =
      customLogin.trim() ||
      (selectedPlatform && selectedPlatform !== '__custom__'
        ? selectedPlatform
        : '')
    const hasLogin = Boolean(login)
    if (loginChoice.requireLogin && !hasLogin) {
      setLoginDialogError('在线聊天必须选择或填写登录连接后才能启动。')
      return
    }
    const missing = (connectionConfig?.fields ?? [])
      .filter(
        field =>
          field.required &&
          !field.default &&
          isMissingConfigValue(connectionValues[field.name])
      )
      .map(field => field.description || field.name)
    if (hasLogin && missing.length) {
      setLoginDialogError(`请先填写必填项：${missing.join('、')} 再启动。`)
      return
    }
    setLoginDialogBusy(true)
    try {
      // Persist required fields silently when a connection package is selected,
      // then save the login before launching.
      if (connectionPackage && connectionConfig?.fields?.length) {
        if (
          !(await onSavePackageConfig(
            connectionPackage,
            valuesForConnectionPackage(connectionValues)
          ))
        )
          return
      }
      if (login) {
        if (!(await onSaveLogin(login, connectionPackage))) return
      }
      if (await onRun(loginChoice.action)) {
        await refreshPorts()
        closeLoginDialog()
      }
    } catch (reason) {
      setLoginDialogError(
        operationErrorMessage(reason, '启动失败，请查看操作记录。')
      )
    } finally {
      setLoginDialogBusy(false)
    }
  }
  return (
    <RobotPanel
      className="runtime-overview max-w-190"
      icon={<Play className="size-4" />}
      title={overview?.name || '正在读取项目…'}
      description={
        overview
          ? `${overview.version || '未设置版本'} · ${overview.packageManager} · ${overview.hasDevScript ? '已配置开发命令' : '未配置 dev 命令'}`
          : '读取包信息、平台包与运行状态。'
      }
      actions={
        <button
          className="icon-button size-9 shrink-0 p-0"
          disabled={loading}
          onClick={handleRefresh}
          aria-label="刷新运行状态"
          title="刷新运行状态"
        >
          <RefreshCw className="size-4" />
        </button>
      }
    >
      <ConfirmDialog
        open={Boolean(pending)}
        title={pending?.label ?? ''}
        message={pending?.note ?? ''}
        busy={busy}
        onCancel={() => setPending(null)}
        onConfirm={confirm}
      />
      <ConfirmDialog
        open={Boolean(validationMessage)}
        title={validationTitle}
        subtitle={
          validationTitle === '已有进程在运行'
            ? '同一机器人目录同时只能以一种方式运行。'
            : '请先填写连接包声明的必填字段。'
        }
        message={validationMessage}
        confirmLabel="知道了"
        cancelLabel="关闭"
        onCancel={() => setValidationMessage('')}
        onConfirm={() => setValidationMessage('')}
      />
      {loginChoice && (
        <Modal
          open
          onClose={closeLoginDialog}
          ariaLabel={loginChoice.label}
          className="bg-slate-950/25 p-6"
        >
          <section
            className="grid max-h-[min(720px,calc(100vh-48px))] w-full max-w-2xl grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_20px_58px_rgb(28_26_23/0.22)]"
            role="dialog"
            aria-modal="true"
            aria-label={loginChoice.label}
          >
            <header className="flex items-center justify-between border-b border-slate-200 px-5 py-4">
              <div>
                <strong className="text-sm text-ink-950">
                  {loginChoice.label}
                </strong>
                <p className="mt-1 text-xs text-slate-500">
                  {loginChoice.preflight.login
                    ? `将使用 ${loginChoice.preflight.login} 登录连接启动。`
                    : '尚未配置 login；可在这里完成连接配置后启动。'}
                </p>
              </div>
              <button
                className="icon-button"
                onClick={closeLoginDialog}
                aria-label="关闭"
              >
                <X />
              </button>
            </header>
            <div className="grid min-h-0 gap-4 overflow-auto p-5">
              {loginDialogError && (
                <p className="m-0 rounded-md border border-orange-200 bg-orange-50 px-3 py-2 text-xs leading-5 text-orange-800">
                  {loginDialogError}
                </p>
              )}
              <section className="rounded-lg border border-slate-200">
                <header className="border-b border-slate-200 bg-slate-50 px-3 py-2">
                  <strong className="text-xs text-slate-700">
                    选择登录平台
                  </strong>
                </header>
                <div className="grid gap-3 p-3 sm:grid-cols-3">
                  <label className="grid gap-1 text-xs font-semibold text-slate-600">
                    已识别平台
                    <select
                      value={selectedPlatform}
                      onChange={event => choosePlatform(event.target.value)}
                    >
                      <option value="">不选择</option>
                      <option value="__custom__">自由输入</option>
                      {(overview?.platforms ?? []).map(item => (
                        <option key={item.id} value={item.id}>
                          {item.id}
                          {item.installed ? ' · 已安装' : ' · 需安装'}
                        </option>
                      ))}
                    </select>
                  </label>
                  {selectedPlatform === '__custom__' && (
                    <>
                      <label className="grid gap-1 text-xs font-semibold text-slate-600">
                        登录连接
                        <input
                          value={customLogin}
                          onChange={event => {
                            setCustomLogin(event.target.value)
                            scheduleLoginSave({
                              login: event.target.value,
                              packageName: customPackage.trim()
                            })
                          }}
                          placeholder="如 onebot"
                        />
                      </label>
                      <label className="grid gap-1 text-xs font-semibold text-slate-600">
                        连接包（可选）
                        <input
                          value={customPackage}
                          onChange={event => {
                            setCustomPackage(event.target.value)
                            setConnectionConfig(null)
                          }}
                          placeholder="如 @alemonjs/onebot"
                        />
                      </label>
                    </>
                  )}
                </div>
                {packageTarget &&
                  (!knownPlatform || !knownPlatform.installed) && (
                    <footer className="flex items-center justify-between border-t border-slate-200 bg-slate-50 px-3 py-2">
                      <small className="text-xs text-slate-500">
                        {packageTarget} 尚未安装；安装后才能读取它的连接配置。
                      </small>
                      <button
                        className="secondary-button gap-1.5"
                        disabled={loginDialogBusy || busy}
                        onClick={() => void installSelectedConnection()}
                      >
                        <Package className="size-4" />
                        安装连接包
                      </button>
                    </footer>
                  )}
              </section>
              {connectionConfig?.fields?.length ? (
                <section className="rounded-lg border border-slate-200">
                  <header className="border-b border-slate-200 bg-slate-50 px-3 py-2">
                    <strong className="flex items-center gap-1.5 text-xs text-slate-700">
                      <PlatformLogo
                        logo={connectionConfig.logo}
                        className="size-3.5"
                      />
                      连接配置
                    </strong>
                    <div className="flex items-center justify-between">
                      <div className="text-[11px] text-slate-400">
                        保存到 alemon.config.yaml
                      </div>
                      <ConfigSourceLinks
                        source={connectionConfig.configSource}
                      />
                    </div>
                  </header>
                  <div className="p-3">
                    <ConfigFieldsEditor
                      fields={connectionConfig.fields}
                      values={connectionValues}
                      onChange={updateConnectionValue}
                      className="grid gap-3 sm:grid-cols-2"
                    />
                  </div>
                </section>
              ) : packageTarget && knownPlatform?.installed ? (
                <p className="m-0 text-xs text-slate-500">
                  该连接包没有声明可填写的 alemonjs.config，保存 login
                  后即可启动。
                </p>
              ) : null}
            </div>
            <footer className="flex flex-wrap items-center justify-end gap-2 border-t border-slate-200 px-5 py-3">
              {(() => {
                // Whether the user actively chose a login (picked a platform
                // or typed one). The configured login is ignored here so the
                // button stays enabled when the user makes no choice.
                const userLogin = Boolean(
                  customLogin.trim() ||
                  (selectedPlatform && selectedPlatform !== '__custom__')
                )
                // Missing required fields only block start when a login is
                // chosen; without a login the robot starts directly.
                const missing = (connectionConfig?.fields ?? [])
                  .filter(
                    field =>
                      field.required &&
                      !field.default &&
                      isMissingConfigValue(connectionValues[field.name])
                  )
                  .map(field => field.description || field.name)
                const blocked =
                  (loginChoice.requireLogin && !userLogin) ||
                  (userLogin && missing.length > 0)
                return (
                  <button
                    className="primary-button gap-1.5"
                    disabled={loginDialogBusy || busy || blocked}
                    title={
                      blocked
                        ? !userLogin
                          ? '在线聊天必须选择或填写登录连接。'
                          : `请先填写必填项：${missing.join('、')}`
                        : userLogin
                          ? '会先保存当前连接配置，再启动机器人。'
                          : '无 login 启动机器人。'
                    }
                    onClick={() => void startFromDialog()}
                  >
                    <Play className="size-4" />
                    {loginDialogBusy || busy ? '启动中…' : '启动'}
                  </button>
                )
              })()}
            </footer>
          </section>
        </Modal>
      )}
      <section className="grid gap-3">
        {portStatus.length > 0 || portStatusBusy || portStatusError ? (
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
            <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div className="grid gap-1">
                <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                  端口检查
                  <StatusDot
                    active={
                      portStatus.length > 0 &&
                      portStatus.every(item => !item.occupied || item.owned)
                    }
                    label={
                      portStatus.length > 0
                        ? portStatus.some(item => item.occupied && !item.owned)
                          ? '有端口被其他进程占用'
                          : '可正常启动'
                        : '检测中'
                    }
                  />
                </strong>
                <span className="block text-xs text-slate-500">
                  启动前会主动确认机器人要绑定的端口没有被其他进程占用。
                </span>
              </div>
              <button
                className="text-button gap-1.5"
                disabled={portStatusBusy}
                onClick={() => void refreshPorts()}
              >
                {portStatusBusy ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RefreshCw className="size-4" />
                )}
                {portStatusBusy ? '检测中…' : '重新检测'}
              </button>
            </div>
            <div className="divide-y divide-slate-100 border-t border-slate-100">
              {portStatusError && (
                <p className="m-0 px-4 py-3 text-xs leading-5 text-orange-700">
                  {portStatusError}
                </p>
              )}
              {!portStatusError && portStatus.length === 0 && (
                <p className="m-0 px-4 py-3 text-xs leading-5 text-slate-500">
                  {portStatusBusy ? '正在检测端口…' : '暂无可检测的端口。'}
                </p>
              )}
              {!portStatusError &&
                portStatus.map(item => {
                  const blocked = item.occupied && !item.owned
                  return (
                    <div
                      key={`${item.kind}:${item.port}`}
                      className="flex flex-wrap items-center gap-2 px-4 py-2.5 text-xs"
                    >
                      <i
                        className={cn(
                          'inline-block size-2 rounded-full',
                          blocked
                            ? 'bg-red-500'
                            : item.occupied
                              ? 'bg-amber-500'
                              : 'bg-emerald-500'
                        )}
                        aria-hidden="true"
                      />
                      <span className="font-semibold text-slate-700">
                        {item.label}
                      </span>
                      <span className="text-slate-500">端口 {item.port}</span>
                      {item.occupied ? (
                        <span
                          className={
                            blocked ? 'text-red-600' : 'text-amber-600'
                          }
                        >
                          {blocked
                            ? `已被其他进程占用${
                                item.pid
                                  ? `（PID ${item.pid}${
                                      item.process ? `：${item.process}` : ''
                                    }）`
                                  : ''
                              }`
                            : '由本机器人进程占用'}
                        </span>
                      ) : (
                        <span className="text-emerald-600">空闲</span>
                      )}
                    </div>
                  )
                })}
            </div>
          </section>
        ) : null}
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          <div className="divide-y divide-slate-200">
            <section className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div>
                <strong className="block text-sm font-semibold text-slate-700">
                  依赖
                  <StatusDot
                    active={overview?.dependenciesComplete}
                    label={overview?.dependenciesComplete ? '已安装' : '未安装'}
                  />
                </strong>
                <span className="block text-xs text-slate-500">
                  {overview?.dependenciesComplete
                    ? '依赖完整；可只升级 AlemonJS 相关依赖，或重新安装全部依赖。'
                    : '依赖未安装或缺失；启动、构建和后台运行时会自动同步。'}
                </span>
              </div>
              <div className="ml-auto flex shrink-0 flex-wrap justify-end gap-2">
                <button
                  className="secondary-button gap-1.5"
                  disabled={busy || !overview?.dependenciesComplete}
                  title={
                    !overview?.dependenciesComplete
                      ? '启动、构建或后台运行时会自动同步依赖。'
                      : ''
                  }
                  onClick={() =>
                    ask(
                      '升级 AlemonJS',
                      '会升级 package.json 中直接声明的 alemonjs 和 @alemonjs/ 相关依赖到最新稳定版，并更新锁文件；不会升级其他业务依赖。',
                      () => onRun('upgrade-alemon')
                    )
                  }
                >
                  <RefreshCw className="size-4" />
                  一键升级
                </button>
                <button
                  className={
                    overview?.dependenciesComplete
                      ? 'secondary-button gap-1.5'
                      : 'danger-button gap-1.5'
                  }
                  disabled={busy}
                  onClick={() =>
                    ask(
                      overview?.dependenciesComplete
                        ? '重新安装依赖'
                        : '安装依赖',
                      overview?.dependenciesComplete
                        ? '会根据 package.json 重新安装当前机器人的全部依赖。'
                        : '会安装 package.json 声明的全部依赖。',
                      () => onRun('install')
                    )
                  }
                >
                  <Package className="size-4" />
                  {overview?.dependenciesComplete ? '重新安装' : '安装'}
                </button>
              </div>
            </section>
            <section className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div>
                <strong className="flex items-center gap-2 text-sm font-semibold text-slate-700">
                  前台运行
                  <StatusDot
                    active={foregroundRunning}
                    stopping={foregroundStopping}
                  />
                </strong>
                <span className="block text-xs text-slate-500">
                  {overview?.hasAppScript
                    ? foregroundStopping
                      ? '正在停止…'
                      : foregroundRunning
                        ? '正在运行，可随时停止。'
                        : developmentRunning
                          ? '当前正在开发运行，请先停止开发进程。'
                          : '直接启动机器人，方便查看输出。'
                    : '还没有前台运行命令。'}
                </span>
              </div>
              <div className="ml-auto flex gap-2 shrink-0 justify-end">
                {overview?.hasAppScript ? (
                  <button
                    className={
                      foregroundRunning || foregroundStopping
                        ? 'secondary-button gap-1.5'
                        : 'primary-button gap-1.5'
                    }
                    disabled={busy || foregroundStopping}
                    title={
                      developmentRunning || pm2LocalRunning
                        ? '启动会自动停止当前正在运行的进程。'
                        : ''
                    }
                    onClick={() =>
                      foregroundRunning
                        ? ask(
                            '停止前台运行',
                            '会停止当前项目的前台运行。',
                            () => onRun('app-stop')
                          )
                        : void askStart(
                            'app',
                            '启动前台',
                            '会直接启动机器人，并打开运行日志。'
                          )
                    }
                  >
                    {foregroundRunning || foregroundStopping ? (
                      <X className="size-4" />
                    ) : (
                      <Play className="size-4" />
                    )}
                    {foregroundStopping
                      ? '正在停止…'
                      : foregroundRunning
                        ? '停止运行'
                        : '启动前台'}
                  </button>
                ) : developerMode ? (
                  <button
                    className="secondary-button gap-1.5"
                    disabled={busy}
                    onClick={() =>
                      ask(
                        '修复前台运行',
                        '会补齐前台运行所需的命令。',
                        () => void repairRuntime('app')
                      )
                    }
                  >
                    <Settings className="size-4" />
                    修复
                  </button>
                ) : (
                  <small>还没有可直接运行的命令。</small>
                )}
              </div>
            </section>
          </div>
        </section>
        {developerMode && (
          <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
            <header className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
              <div className="grid gap-1">
                <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                  开发运行
                  <StatusDot
                    active={developmentRunning}
                    stopping={developmentStopping}
                  />
                </strong>
                <span className="block text-xs text-slate-500">
                  {developmentStopping
                    ? '正在停止…'
                    : developmentRunning
                      ? '正在运行，可随时停止。'
                      : foregroundRunning
                        ? '当前正在前台运行，请先停止前台进程。'
                        : pm2LocalRunning
                          ? '当前正在后台运行，请先停止后台服务。'
                          : overview?.hasDevScript
                            ? '适合改代码、排查问题。'
                            : '还没有开发命令。'}
                </span>
              </div>
              <div className="flex shrink-0 flex-wrap justify-end gap-2">
                {overview?.hasBuildScript && (
                  <button
                    className="secondary-button gap-1.5"
                    disabled={busy}
                    onClick={() =>
                      ask(
                        '构建脚本',
                        '会运行 build 脚本生成构建产物；后台运行需要先构建才能识别本地应用。',
                        () => onRun('build')
                      )
                    }
                  >
                    <Code2 className="size-4" />
                    构建脚本
                  </button>
                )}
                {overview?.hasDevScript ? (
                  <button
                    className={
                      developmentRunning || developmentStopping
                        ? 'secondary-button gap-1.5'
                        : 'primary-button gap-1.5'
                    }
                    disabled={busy || developmentStopping}
                    title={
                      foregroundRunning || pm2LocalRunning
                        ? '启动会自动停止当前正在运行的进程。'
                        : ''
                    }
                    onClick={() =>
                      developmentRunning
                        ? ask('停止开发', '会停止当前项目的开发运行。', () =>
                            onRun('dev-stop')
                          )
                        : void askStart(
                            'dev',
                            '启动开发',
                            '会以开发模式启动，并打开运行日志。'
                          )
                    }
                  >
                    {developmentRunning || developmentStopping ? (
                      <X className="size-4" />
                    ) : (
                      <Play className="size-4" />
                    )}
                    {developmentStopping
                      ? '正在停止…'
                      : developmentRunning
                        ? '停止开发'
                        : '启动开发'}
                  </button>
                ) : (
                  <button
                    className="secondary-button gap-1.5"
                    disabled={busy}
                    onClick={() =>
                      ask(
                        '修复开发命令',
                        '会补齐开发所需的运行命令，并保留现有设置。',
                        () => void repairRuntime('dev')
                      )
                    }
                  >
                    <Settings className="size-4" />
                    修复
                  </button>
                )}
              </div>
            </header>
          </section>
        )}
        <section className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          <header className="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
            <div className="grid gap-1">
              <strong className="flex items-center gap-2 text-sm font-semibold text-slate-800">
                后台运行
                {persistentReady && (
                  <StatusDot
                    active={pm2Running}
                    label={
                      pm2Running
                        ? '运行中'
                        : pm2Managed
                          ? `已注册 · ${pm2Status?.status || '未知'}`
                          : '未启动'
                    }
                  />
                )}
              </strong>
              <span className="text-xs text-slate-500">
                {persistentReady
                  ? pm2Status
                    ? pm2Running
                      ? '服务运行中；关闭本窗口后仍会继续运行。'
                      : pm2Managed
                        ? '服务已注册，当前未运行；可重启或删除。'
                        : '服务尚未启动。'
                    : pm2StatusError
                      ? '无法读取服务状态；仍可尝试启动服务。'
                      : '正在读取服务状态。'
                  : '还未准备好，修复后可长期在线。'}
              </span>
            </div>
            <button
              className="primary-button gap-1.5"
              disabled={busy || !persistentReady}
              title={
                pm2Running
                  ? '重新加载 pm2.config（pm2 reload），使配置改动生效且尽量不中断服务。'
                  : localRunning
                    ? '启动会自动停止当前正在运行的进程。'
                    : !persistentReady
                      ? '补齐 start 脚本和 PM2 配置后可使用。'
                      : ''
              }
              onClick={() =>
                void askStart(
                  pm2Running ? 'pm2-reload' : 'pm2',
                  pm2Running ? '重载配置' : '启动服务',
                  pm2Running
                    ? '会重新加载 pm2.config（pm2 reload），使配置改动生效且尽量不中断正在运行的机器人。'
                    : '会在后台启动机器人。'
                )
              }
            >
              {pm2Running ? (
                <RefreshCw className="size-4" />
              ) : (
                <Play className="size-4" />
              )}
              {pm2Running ? '重载配置' : '启动服务'}
            </button>
          </header>
          <div className="flex flex-wrap items-center justify-end gap-2 px-4 py-3">
            {/* PM2 status detection can be unreliable (daemon/version mismatch,
                sandboxed reads), so these actions stay clickable regardless of
                the detected state. The backend reports errors per action. */}
            {persistentReady && (
              <>
                <button
                  className="secondary-button gap-1.5"
                  disabled={busy}
                  onClick={() =>
                    ask('停止服务', '会停止当前项目在后台运行的机器人。', () =>
                      onRun('pm2-stop')
                    )
                  }
                >
                  <X className="size-4" />
                  停止服务
                </button>
                <button
                  className="secondary-button gap-1.5"
                  disabled={busy}
                  onClick={() =>
                    ask('重启服务', '会停止并重新启动后台运行的机器人。', () =>
                      onRun('pm2-restart')
                    )
                  }
                >
                  <RefreshCw className="size-4" />
                  重启
                </button>
                <button
                  className="secondary-button gap-1.5"
                  disabled={busy}
                  onClick={() =>
                    ask(
                      '重载配置',
                      '会重新加载 pm2.config（pm2 reload），使配置改动生效且尽量不中断正在运行的机器人。',
                      () => onRun('pm2-reload')
                    )
                  }
                >
                  <RefreshCw className="size-4" />
                  重载
                </button>
              </>
            )}
            {!persistentReady && (
              <button
                className="secondary-button gap-1.5"
                disabled={busy}
                onClick={() =>
                  ask(
                    '修复后台运行',
                    '会补齐后台运行所需的设置和依赖。',
                    () => void repairRuntime('pm2')
                  )
                }
              >
                <Settings className="size-4" />
                修复
              </button>
            )}
            <div className="runtime-persistent-utilities">
              <button
                className="text-button gap-1.5"
                disabled={busy}
                onClick={onOpenPM2Processes}
              >
                <Activity className="size-4" />
                状态
              </button>
              <button
                className="text-button gap-1.5"
                disabled={busy}
                onClick={onOpenPM2Logs}
              >
                <ClipboardList className="size-4" />
                日志
              </button>
              <button
                className="text-button danger-action gap-1.5"
                disabled={busy}
                onClick={() =>
                  ask(
                    '移除后台服务',
                    '会移除后台运行记录；以后仍可再次启动。',
                    () => onRun('pm2-delete')
                  )
                }
              >
                <Trash2 className="size-4" />
                删除
              </button>
            </div>
          </div>
        </section>
      </section>
    </RobotPanel>
  )
}
// robotAppToken base64url-encodes a robot directory path without padding so it
// can sit in a URL path segment, matching the backend's robotAppToken.
function robotAppToken(root: string) {
  return btoa(
    Array.from(new TextEncoder().encode(root), byte =>
      String.fromCharCode(byte)
    ).join('')
  )
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
}

function AppEmbed({
  root,
  onClose,
  onMinimize,
  zIndex,
  onActivate,
  minimized,
  appPages,
  selectedAppPageID
}: {
  root: string
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
  minimized: boolean
  appPages: Array<{
    id: string
    name: string
    package: string
    logo?: string
  }>
  selectedAppPageID: string
}) {
  const token = robotAppToken(root)
  const appSrc = `/api/v1/robot/app/${token}/`
  const selectedAppPage = appPages.find(item => item.id === selectedAppPageID)
  const appPageSrc = selectedAppPage
    ? `/api/v1/robot/webview/${token}/${selectedAppPage.id}/`
    : appSrc
  return (
    <DesktopWindow
      id="app"
      open
      minimized={minimized}
      title="机器人应用"
      subtitle={root || '当前机器人目录'}
      icon={
        <Monitor className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 72, top: 56 }}
      width={980}
      height={680}
    >
      <div className="flex min-h-0 flex-1 flex-col">
        <iframe
          className="min-h-0 flex-1 border-0"
          src={appPageSrc}
          title={selectedAppPage ? selectedAppPage.name : '机器人应用'}
        />
      </div>
    </DesktopWindow>
  )
}

function TestCenterWindow({
  root,
  minimized,
  zIndex,
  onClose,
  onMinimize,
  onActivate
}: {
  root: string
  minimized: boolean
  zIndex: number
  onClose: () => void
  onMinimize: () => void
  onActivate: () => void
}) {
  return (
    <DesktopWindow
      id="test"
      open
      minimized={minimized}
      title="机器人测试"
      subtitle={root || '当前机器人目录'}
      icon={
        <FlaskConical className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 72, top: 56 }}
      width={1180}
      height={760}
    >
      <TestCenter root={root} />
    </DesktopWindow>
  )
}

function LiveChatWindow({
  root,
  minimized,
  zIndex,
  onClose,
  onMinimize,
  onActivate
}: {
  root: string
  minimized: boolean
  zIndex: number
  onClose: () => void
  onMinimize: () => void
  onActivate: () => void
}) {
  return (
    <DesktopWindow
      id="live"
      open
      minimized={minimized}
      title="在线聊天"
      subtitle=""
      icon={
        <MessageSquare className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 92, top: 72 }}
      width={1180}
      height={760}
      storageKey={qqChatWindowStorageKey(root)}
    >
      <LiveChat root={root} />
    </DesktopWindow>
  )
}

function ControlCard({
  page,
  section,
  project,
  buildMode,
  catalog,
  catalogTitle,
  developerMode,
  agentOpen,
  appLaunching,
  testLaunching,
  onOpenConsole,
  onOpenAI,
  onOpenOps,
  onOpenApp,
  onOpenTest,
  onOpenLive,
  onPage,
  onSection,
  onBuildMode,
  onCatalog,
  onGit
}: {
  page: Page
  section: Section
  project?: Project
  buildMode: 'manifest' | 'npm' | 'git'
  catalog: CatalogGroup[]
  catalogTitle: string
  developerMode: boolean
  agentOpen: boolean
  appLaunching: boolean
  testLaunching: boolean
  onOpenConsole: () => void
  onOpenAI: () => void
  onOpenOps: () => void
  onOpenApp: () => void
  onOpenTest: () => void
  onOpenLive: () => void
  onPage: (page: Page) => void
  onSection: (section: Section) => void
  onBuildMode: (mode: 'manifest' | 'npm' | 'git') => void
  onCatalog: (title: string) => void
  onGit: () => void
}) {
  const gitRoot = project?.path ?? ''
  const gitOverviewBranchesArgs = useMemo(
    () => ({ root: gitRoot, view: 'branch' as const }),
    [gitRoot]
  )
  const gitOverviewChangesArgs = useMemo(
    () => ({ root: gitRoot, view: 'commit' as const }),
    [gitRoot]
  )
  const { data: gitBranches } = useGitWorkspaceQuery(gitOverviewBranchesArgs, {
    skip: !gitRoot || page !== 'robot' || section !== 'runtime'
  })
  const { data: gitChanges } = useGitWorkspaceQuery(gitOverviewChangesArgs, {
    skip: !gitRoot || page !== 'robot' || section !== 'runtime'
  })
  const gitOverview =
    gitBranches && gitChanges
      ? { ...gitBranches, changes: gitChanges.changes }
      : (gitBranches ?? gitChanges ?? undefined)
  const activePrimary = agentOpen
    ? null
    : page === 'robot'
      ? section === 'backpack'
        ? 'backpack'
        : section === 'runtime'
          ? 'runtime'
          : 'config'
      : page
  const subitems = agentOpen
    ? []
    : activePrimary === 'config'
      ? developerMode
        ? [
            { id: 'npmrc', label: 'npm 源' },
            { id: 'env', label: '环境变量' }
          ]
        : []
      : activePrimary === 'build'
        ? [
            { id: 'manifest', label: '包配置' },
            { id: 'git', label: 'GIT 发布' },
            { id: 'npm', label: 'NPM 发布' }
          ]
        : activePrimary === 'plugins' ||
            activePrimary === 'connections' ||
            activePrimary === 'modules'
          ? catalog.map(item => ({ id: item.title, label: item.title }))
          : []
  const activeSecondary =
    activePrimary === 'config'
      ? section
      : activePrimary === 'build'
        ? buildMode
        : catalogTitle
  function selectPrimary(item: (typeof directoryActions)[number]) {
    if (item.kind === 'section') {
      onPage('robot')
      onSection(item.id as Section)
      return
    }
    onPage(item.id as Page)
  }
  function selectSecondary(id: string) {
    if (activePrimary === 'config') {
      onSection(id as Section)
      return
    }
    if (activePrimary === 'build') {
      onBuildMode(id as 'manifest' | 'npm' | 'git')
      return
    }
    onCatalog(id)
  }
  return (
    <aside className="control-dock flex min-h-0 flex-col" aria-label="目录操作">
      <section className="control-card control-navigation">
        <header className="control-identity flex items-start justify-between gap-2">
          <div className="min-w-0">
            <span className="block truncate text-sm font-medium text-slate-800 dark:text-slate-100">
              {project?.name ?? '未选择目录'}
            </span>
            {project && (
              <span className="block truncate text-[11px] text-slate-400 dark:text-slate-500">
                {project.path}
              </span>
            )}
          </div>
          <button
            className="inline-flex size-7 shrink-0 items-center justify-center rounded text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600 dark:text-slate-500 dark:hover:bg-slate-700 dark:hover:text-slate-300"
            onClick={onGit}
            aria-label={`管理 ${project?.name ?? '当前机器人'} 的 Git`}
            title="Git 管理"
          >
            <GitBranch className="size-3.5" />
          </button>
        </header>
        {project && gitOverview && (
          <div className="pb-2">
            {gitOverview.repository ? (
              <div
                className="flex items-center gap-1.5 px-1 text-[11px] text-slate-500 dark:text-slate-400"
                title={
                  gitOverview.upstream
                    ? `领先 ${gitOverview.ahead} · 落后 ${gitOverview.behind}`
                    : undefined
                }
              >
                <GitBranch className="size-3 shrink-0 text-slate-400 dark:text-slate-500" />
                <span className="truncate">
                  {gitOverview.branch || '未知分支'}
                </span>
                {gitOverview.changes.length > 0 && (
                  <span className="ml-auto shrink-0 font-medium text-amber-600 dark:text-amber-400">
                    {gitOverview.changes.length} 项
                  </span>
                )}
              </div>
            ) : null}
          </div>
        )}
        <div
          className="control-primary-nav grid gap-0.5"
          aria-label="机器人功能"
        >
          {directoryActions
            .filter(item => developerMode || item.id !== 'build')
            .map(item => (
              <button
                className={cn(
                  'flex min-h-8 items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
                  activePrimary === item.id
                    ? 'workspace-nav-active'
                    : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
                )}
                onClick={() => selectPrimary(item)}
                key={item.id}
              >
                <i className="inline-flex size-4 items-center justify-center not-italic">
                  {item.icon}
                </i>
                <span className="min-w-0 flex-1">{item.label}</span>
              </button>
            ))}
          {project && (
            <button
              className={cn(
                'flex min-h-8 items-center gap-2 rounded-md px-2 text-left text-xs font-medium transition-colors',
                agentOpen
                  ? 'workspace-nav-active'
                  : 'text-slate-600 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
              )}
              onClick={onOpenAI}
              aria-label="使用 Agent 协助当前机器人"
              title="使用 Agent 协助当前机器人"
            >
              <i className="inline-flex size-4 items-center justify-center not-italic">
                <MessageSquare className="size-4" />
              </i>
              <span className="min-w-0 flex-1">Code</span>
            </button>
          )}
        </div>
        {subitems.length > 0 && (
          <div
            className="control-secondary-nav grid gap-0.5"
            aria-label="当前功能子菜单"
          >
            {subitems.map(item => (
              <button
                className={cn(
                  'flex min-h-7 items-center rounded-md pl-7 pr-2 text-left text-xs transition-colors',
                  activeSecondary === item.id
                    ? 'workspace-nav-sub-active'
                    : 'text-slate-500 hover:bg-slate-200/40 dark:text-slate-400 dark:hover:bg-slate-700/40'
                )}
                onClick={() => selectSecondary(item.id)}
                key={item.id}
              >
                {item.label}
              </button>
            ))}
          </div>
        )}
        {project && (
          <footer
            className="control-quick-actions mt-2 grid grid-cols-5 gap-1 border-t border-slate-100 pt-2 dark:border-slate-700"
            title={project.path}
          >
            <button
              className="inline-flex min-h-8 items-center justify-center gap-1 rounded-md px-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200"
              onClick={onOpenConsole}
              aria-label="查看运行终端"
              title="查看运行终端"
            >
              <Terminal className="size-3.5" />
              <span>终端</span>
            </button>
            <button
              className="inline-flex min-h-8 items-center justify-center gap-1 rounded-md px-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 disabled:opacity-40 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200"
              onClick={onOpenApp}
              disabled={appLaunching}
              aria-label={appLaunching ? '正在启动应用…' : '打开应用'}
              title={
                appLaunching
                  ? '正在启动应用，请稍候…'
                  : '打开应用（需配置应用端口并启动机器人）'
              }
            >
              {appLaunching ? (
                <span className="inline-block size-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-brand-600" />
              ) : (
                <Monitor className="size-3.5" />
              )}
              <span>应用</span>
            </button>
            <button
              className="inline-flex min-h-8 items-center justify-center gap-1 rounded-md px-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 disabled:opacity-40 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200"
              onClick={onOpenTest}
              disabled={testLaunching}
              aria-label={testLaunching ? '正在启动测试…' : '打开测试'}
              title={
                testLaunching
                  ? '正在启动测试，请稍候…'
                  : '打开测试（需配置机器人端口并启动沙盒模式）'
              }
            >
              {testLaunching ? (
                <span className="inline-block size-3.5 animate-spin rounded-full border-2 border-slate-300 border-t-brand-600" />
              ) : (
                <FlaskConical className="size-3.5" />
              )}
              <span>测试</span>
            </button>
            <button
              className="inline-flex min-h-8 items-center justify-center gap-1 rounded-md px-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200"
              onClick={onOpenLive}
              aria-label="打开在线聊天"
              title="连接已登录机器人的 CBP 在线聊天"
            >
              <MessageSquare className="size-3.5" />
              <span>聊天</span>
            </button>
            <button
              className="inline-flex min-h-8 items-center justify-center gap-1 rounded-md px-1 text-[11px] text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-200"
              onClick={onOpenOps}
              aria-label="打开运维"
              title="打开运维"
            >
              <ShieldCheck className="size-3.5" />
              <span>运维</span>
            </button>
          </footer>
        )}
      </section>
    </aside>
  )
}
// StatusDot is a small state indicator for a running process. active means
// running (pulsing green dot), stopping means a graceful shutdown is in
// progress (amber), and muted with a label means "registered but not running".
function StatusDot({
  active,
  stopping,
  label
}: {
  active?: boolean
  stopping?: boolean
  label?: string
}) {
  if (!active && !stopping && !label) return null
  const tone = active
    ? 'bg-emerald-500'
    : stopping
      ? 'bg-amber-500'
      : 'bg-slate-300 dark:bg-slate-600'
  return (
    <span className="inline-flex items-center gap-1.5">
      <i
        className={cn(
          'inline-block size-2 rounded-full',
          tone,
          active && 'animate-pulse'
        )}
        aria-hidden="true"
      />
      {label && (
        <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400">
          {label}
        </span>
      )}
    </span>
  )
}
function ReadonlyConsole({
  open,
  minimized,
  root,
  onClose,
  onMinimize,
  zIndex,
  onActivate
}: {
  open: boolean
  minimized: boolean
  root: string
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
}) {
  // 终端输出按字符数封顶，避免机器人持续输出时 DOM 无限增长卡死浏览器。
  const MAX_TERMINAL_OUTPUT_CHARS = 100_000
  const appendTerminalOutput = (current: string, chunk: string): string => {
    const next = `${current}${chunk}`
    if (next.length <= MAX_TERMINAL_OUTPUT_CHARS) return next
    return `…早期输出已省略…\n${next.slice(-MAX_TERMINAL_OUTPUT_CHARS)}`
  }

  type TerminalTab = { id: string; label: string; kind: 'readonly' | 'shell' }
  const [load, { data, error, isFetching }] = useLazyRobotConsoleQuery()
  const outputRef = useRef<HTMLPreElement>(null)
  const shellOutputRef = useRef<HTMLPreElement>(null)
  const shellInputRef = useRef<HTMLInputElement>(null)
  const followLatest = useRef(true)
  const [tabs, setTabs] = useStoreState<TerminalTab[]>([])
  const [activeTab, setActiveTab] = useStoreState('runtime')
  const [shellCommand, setShellCommand] = useStoreState('')
  const [shellOutput, setShellOutput] = useStoreState('')
  const [shellDirectory, setShellDirectory] = useStoreState('')
  const [shellBusy, setShellBusy] = useStoreState(false)
  const [shellHistory, setShellHistory] = useStoreState<string[]>([])
  const [shellHistoryIndex, setShellHistoryIndex] = useStoreState(-1)
  // liveOutput accumulates incremental output pushed over SSE; the initial
  // load seeds it with the current buffer so the terminal is real-time without
  // polling. The static snapshot still comes from the query.
  const [liveOutput, setLiveOutput] = useStoreState('')
  const runError = error
    ? operationErrorMessage(error, '无法读取当前目录的运行终端信息。')
    : ''
  const running = Boolean(data?.running)
  const message = runError || liveOutput
  const activeTerminal = tabs.find(tab => tab.id === activeTab)
  const resetTerminals = useCallback(() => {
    setTabs([{ id: 'runtime', label: '前台', kind: 'readonly' }])
    setActiveTab('runtime')
    setShellCommand('')
    setShellOutput('')
    setShellDirectory('')
    setShellHistory([])
    setShellHistoryIndex(-1)
  }, [
    setActiveTab,
    setShellCommand,
    setShellDirectory,
    setShellHistory,
    setShellHistoryIndex,
    setShellOutput,
    setTabs
  ])
  useEffect(() => {
    if (!open || !root) return
    resetTerminals()
    void load({ root }).then(result => {
      if (result.data) {
        setLiveOutput(appendTerminalOutput('', result.data.output ?? ''))
      }
    })
    // No polling: output streams in via the SSE robot-output event; the manual
    // refresh button re-reads the snapshot.
  }, [load, open, resetTerminals, root, setLiveOutput])
  useEffect(() => {
    if (!open) return
    const handler = (event: Event) => {
      const detail = (
        event as CustomEvent<{ text?: string; truncated?: boolean }>
      ).detail
      if (detail?.text)
        setLiveOutput(current =>
          appendTerminalOutput(
            current,
            `${detail.truncated ? '…早期输出已省略…\n' : ''}${detail.text}`
          )
        )
    }
    window.addEventListener('alx:robot-output', handler)
    return () => window.removeEventListener('alx:robot-output', handler)
  }, [open, setLiveOutput])
  useEffect(() => {
    if (open) followLatest.current = true
  }, [open, setLiveOutput])
  useEffect(() => {
    if (!open || !followLatest.current) return
    const frame = window.requestAnimationFrame(() => {
      const output = outputRef.current
      if (output) output.scrollTop = output.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
  }, [message, open])
  useEffect(() => {
    if (!open || minimized || activeTerminal?.kind !== 'shell') return
    const frame = window.requestAnimationFrame(() => {
      if (shellOutputRef.current) {
        shellOutputRef.current.scrollTop = shellOutputRef.current.scrollHeight
      }
      shellInputRef.current?.focus()
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeTerminal?.kind, minimized, open, shellOutput])
  const closeTab = (id: string) => {
    setTabs(current => current.filter(tab => tab.id !== id))
    if (activeTab === id) setActiveTab('')
  }
  const addTab = () => {
    if (!tabs.some(tab => tab.id === 'runtime')) {
      setTabs(current => [
        { id: 'runtime', label: '前台', kind: 'readonly' },
        ...current
      ])
      setActiveTab('runtime')
      return
    }
    const id = `shell-${Date.now()}`
    setTabs(current => [...current, { id, label: '终端', kind: 'shell' }])
    setActiveTab(id)
    setShellOutput('')
  }
  const executeShell = async (event: FormEvent) => {
    event.preventDefault()
    const command = shellCommand.trim()
    if (!command || shellBusy) return
    if (command === 'clear' || command === 'cls') {
      setShellOutput('')
      setShellCommand('')
      setShellHistoryIndex(-1)
      return
    }
    setShellBusy(true)
    try {
      const response = await fetch('/api/v1/robot/terminal', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ root, command, directory: shellDirectory })
      })
      const result = (await response.json()) as {
        output?: string
        error?: string
        directory?: string
      }
      setShellOutput(current =>
        appendTerminalOutput(
          current,
          `${current ? '\n' : ''}${response.ok ? (result.output ?? '') : `错误：${result.error ?? '命令执行失败。'}`}`
        )
      )
      if (response.ok && typeof result.directory === 'string')
        setShellDirectory(result.directory)
      setShellHistory(current => [...current, command].slice(-50))
    } catch {
      setShellOutput(current =>
        appendTerminalOutput(
          current,
          `${current ? '\n' : ''}错误：无法连接终端服务。`
        )
      )
    } finally {
      setShellCommand('')
      setShellHistoryIndex(-1)
      setShellBusy(false)
    }
  }
  const completeShellInput = async () => {
    if (shellBusy) return
    const parts = shellCommand.split(/(\s+)/)
    const tokenIndex = parts.length - 1
    const token = parts[tokenIndex] ?? ''
    const commandNames = [
      'cd',
      'clear',
      'cls',
      'dir',
      'git',
      'go',
      'ls',
      'npm',
      'node',
      'pnpm',
      'pwd',
      'yarn'
    ]
    const completingCommand = parts.length === 1
    if (completingCommand) {
      const matches = commandNames.filter(name => name.startsWith(token))
      if (matches.length === 1) setShellCommand(matches[0] + ' ')
      return
    }
    const normalizedToken = token.replace(/\\/g, '/')
    if (
      normalizedToken.split('/').includes('..') ||
      normalizedToken.startsWith('/')
    )
      return
    const separator = normalizedToken.lastIndexOf('/')
    const base = separator >= 0 ? normalizedToken.slice(0, separator + 1) : ''
    const prefix =
      separator >= 0 ? normalizedToken.slice(separator + 1) : normalizedToken
    const directory = [root, shellDirectory, base].filter(Boolean).join('/')
    try {
      const response = await fetch(
        `/api/v1/directories?${new URLSearchParams({ path: directory, files: 'true' })}`
      )
      if (!response.ok) return
      const result = (await response.json()) as {
        directories?: Array<{ name: string }>
        files?: Array<{ name: string }>
      }
      const candidates = [
        ...(result.directories ?? []).map(item => ({
          name: item.name,
          directory: true
        })),
        ...(result.files ?? []).map(item => ({
          name: item.name,
          directory: false
        }))
      ].filter(item => item.name.startsWith(prefix))
      const names = candidates.map(item => item.name)
      if (names.length === 1) {
        const match = candidates[0]
        parts[tokenIndex] = base + names[0] + (match?.directory ? '/' : '')
        setShellCommand(parts.join(''))
        return
      }
      const commonPrefix = names.reduce((shared, name) => {
        let index = 0
        while (index < shared.length && shared[index] === name[index]) index++
        return shared.slice(0, index)
      }, names[0] ?? '')
      if (commonPrefix.length > prefix.length) {
        parts[tokenIndex] = base + commonPrefix
        setShellCommand(parts.join(''))
      } else if (names.length > 1) {
        setShellOutput(current =>
          appendTerminalOutput(
            current,
            `${current ? '\n' : ''}补全候选：${names.join('  ')}`
          )
        )
      }
    } catch {
      // 补全失败不应打断输入。
    }
  }
  const handleShellKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Tab') {
      event.preventDefault()
      void completeShellInput()
      return
    }
    if (event.key === 'ArrowUp' && shellHistory.length > 0) {
      event.preventDefault()
      const next = Math.max(
        0,
        shellHistoryIndex < 0 ? shellHistory.length - 1 : shellHistoryIndex - 1
      )
      setShellHistoryIndex(next)
      setShellCommand(shellHistory[next] ?? '')
      return
    }
    if (event.key === 'ArrowDown' && shellHistoryIndex >= 0) {
      event.preventDefault()
      const next = shellHistoryIndex + 1
      setShellHistoryIndex(next >= shellHistory.length ? -1 : next)
      setShellCommand(
        next >= shellHistory.length ? '' : (shellHistory[next] ?? '')
      )
      return
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'l') {
      event.preventDefault()
      setShellOutput('')
    }
  }
  const terminalTabs = (
    <nav className="readonly-console-tabs" aria-label="终端列表">
      {tabs.map(tab => (
        <button
          className={cn(
            'readonly-console-tab',
            activeTab === tab.id && 'active'
          )}
          key={tab.id}
          onClick={() => setActiveTab(tab.id)}
        >
          <span>{tab.label}</span>
          <X
            className="size-3"
            onClick={event => {
              event.stopPropagation()
              closeTab(tab.id)
            }}
          />
        </button>
      ))}
      <button
        className="readonly-console-tab-add"
        onClick={addTab}
        aria-label="添加终端"
        title="添加终端"
      >
        <Plus className="size-4" />
      </button>
      {tabs.length === 0 && (
        <span className="readonly-console-empty">
          没有打开的终端，点击 + 添加
        </span>
      )}
    </nav>
  )
  if (!open) return null
  return (
    <DesktopWindow
      id="terminal"
      open={open}
      minimized={minimized}
      title="终端"
      subtitle={
        activeTerminal?.kind === 'shell'
          ? `仅限当前机器人目录 · ${shellDirectory ? `./${shellDirectory}` : '.'}`
          : running
            ? `${data?.mode ?? '进程'}实时输出 · 只读`
            : '查看最近运行输出 · 只读'
      }
      icon={
        <Terminal className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      headerLeft={terminalTabs}
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={() => {
        onActivate()
        if (activeTerminal?.kind === 'shell' && !minimized)
          window.requestAnimationFrame(() => shellInputRef.current?.focus())
      }}
      initialPosition={{ left: 16, top: 16 }}
      width={560}
      height={650}
      actions={
        <button
          className="icon-button size-8 p-0"
          disabled={isFetching}
          onClick={() => void load({ root, refresh: true })}
          aria-label="刷新运行终端"
          title="刷新"
        >
          <RefreshCw className="size-4" />
        </button>
      }
    >
      <div className="readonly-console-body min-h-0">
        {activeTerminal?.kind === 'shell' ? (
          <div className="readonly-console-shell">
            <pre ref={shellOutputRef}>{shellOutput || ''}</pre>
            <form onSubmit={executeShell}>
              <span>$</span>
              <input
                ref={shellInputRef}
                autoFocus
                value={shellCommand}
                onChange={event => setShellCommand(event.target.value)}
                onKeyDown={handleShellKeyDown}
                disabled={shellBusy}
                placeholder="输入命令 · 支持 shell 语法 · Tab 补全 · ↑↓ 历史"
                aria-label="机器人目录终端命令"
              />
            </form>
          </div>
        ) : activeTerminal ? (
          <pre
            ref={outputRef}
            className="readonly-console-output"
            onScroll={event => {
              const output = event.currentTarget
              followLatest.current =
                output.scrollHeight - output.scrollTop - output.clientHeight <
                24
            }}
          >
            {isFetching && !message ? '正在读取运行输出…' : message}
          </pre>
        ) : null}
      </div>
    </DesktopWindow>
  )
}

// Kept as an exported compatibility surface while every caller uses the
// shared window chrome, geometry and container-query contract.
export function FloatingWindow({
  open,
  minimized,
  title,
  subtitle,
  icon,
  actions,
  onClose,
  onMinimize,
  zIndex,
  onActivate,
  initialPosition,
  width = 860,
  height = 620,
  children
}: {
  open: boolean
  minimized: boolean
  title: string
  subtitle?: string
  icon: ReactNode
  actions?: ReactNode
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
  initialPosition?: { left: number; top: number }
  width?: number
  height?: number
  children: ReactNode
}) {
  const id = useId()
  return (
    <DesktopWindow
      id={`legacy-window-${id}`}
      open={open}
      minimized={minimized}
      title={title}
      subtitle={subtitle}
      icon={icon}
      actions={actions}
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={initialPosition}
      width={width}
      height={height}
    >
      {children}
    </DesktopWindow>
  )
}

function OpsWindow({
  open,
  minimized,
  root,
  onClose,
  onMinimize,
  zIndex,
  onActivate
}: {
  open: boolean
  minimized: boolean
  root: string
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
}) {
  return (
    <SidebarWindow
      id="ops"
      open={open}
      minimized={minimized}
      title="运维"
      subtitle={root || '当前机器人目录'}
      icon={
        <ShieldCheck className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 120, top: 104 }}
      width={1080}
      height={720}
      sidebarAriaLabel="运维操作"
    >
      <OpsCenter root={root} sidebarLayout />
    </SidebarWindow>
  )
}

const PM2_LIVE_MAX_LINES = 2000

function PM2LogsPanel({
  open,
  root,
  minimized,
  onClose,
  onMinimize,
  zIndex,
  onActivate
}: {
  open: boolean
  root: string
  minimized: boolean
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
}) {
  type PM2AuditLine = { source: string; text: string }
  type PM2AuditPage = {
    lines: PM2AuditLine[]
    page: number
    perPage: number
    hasOlder: boolean
    hasNewer: boolean
    total: number
    truncated: boolean
    sources: string[]
    date?: string
    query?: string
  }
  type PM2LogDay = {
    date: string
    count: number
    out: number
    err: number
    first?: string
    last?: string
  }
  const [page, setPage] = useStoreState(1)
  const [perPage, setPerPage] = useStoreState(120)
  const [date, setDate] = useStoreState('')
  const [source, setSource] = useStoreState('all')
  const [query, setQuery] = useStoreState('')
  const [committedQuery, setCommittedQuery] = useStoreState('')
  const [data, setData] = useStoreState<PM2AuditPage | null>(null)
  const [error, setError] = useStoreState('')
  const [loading, setLoading] = useStoreState(false)
  const [days, setDays] = useStoreState<PM2LogDay[]>([])
  const [live, setLive] = useStoreState(false)
  const [liveConnected, setLiveConnected] = useStoreState(false)
  const [liveLines, setLiveLines] = useStoreState<PM2AuditLine[]>([])
  const outputRef = useRef<HTMLDivElement>(null)
  const followLatest = useRef(true)

  const loadPage = useCallback(
    async (targetPage: number) => {
      setLoading(true)
      try {
        const params = new URLSearchParams({
          root,
          page: String(targetPage),
          perPage: String(perPage)
        })
        if (date) params.set('date', date)
        if (source !== 'all') params.set('source', source)
        if (committedQuery.trim()) params.set('query', committedQuery.trim())
        const response = await fetch(`/api/v1/robot/pm2-logs?${params}`)
        const result = (await response.json()) as PM2AuditPage & {
          error?: string
        }
        if (!response.ok) throw new Error(result.error || '无法读取 PM2 日志。')
        setData(result)
        setError('')
      } catch (reason) {
        setError(operationErrorMessage(reason, '无法读取 PM2 日志。'))
      } finally {
        setLoading(false)
      }
    },
    [committedQuery, date, perPage, root, setData, setError, setLoading, source]
  )

  const loadDays = useCallback(async () => {
    try {
      const response = await fetch(
        `/api/v1/robot/pm2-logs/days?${new URLSearchParams({ root })}`
      )
      const result = (await response.json()) as {
        days?: PM2LogDay[]
        error?: string
      }
      if (!response.ok)
        throw new Error(result.error || '无法读取 PM2 日志日期。')
      setDays(result.days ?? [])
    } catch {
      setDays([])
    }
  }, [root, setDays])

  // Reset the view every time the window opens.
  useEffect(() => {
    if (open) {
      setPage(1)
      setDate('')
      setSource('all')
      setQuery('')
      setCommittedQuery('')
      void loadDays()
    }
  }, [loadDays, open, setCommittedQuery, setDate, setPage, setQuery, setSource])

  // Debounce the search box so audit filters do not hit the API per keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => setCommittedQuery(query), 300)
    return () => window.clearTimeout(timer)
  }, [query, setCommittedQuery])

  // Load the current page whenever filters or paging change.
  useEffect(() => {
    if (open && root) void loadPage(page)
  }, [loadPage, open, page, root])

  // Real-time follow: tail the PM2 log files over SSE.
  useEffect(() => {
    if (!open || !live) return
    setLiveConnected(false)
    const stream = new EventSource(
      `/api/v1/robot/pm2-logs/stream?${new URLSearchParams({ root })}`
    )
    stream.onopen = () => setLiveConnected(true)
    stream.onmessage = event => {
      try {
        const payload = JSON.parse(event.data) as {
          source?: string
          text?: string
          error?: string
        }
        if (payload.error) {
          setError(payload.error)
          return
        }
        if (!payload.text) return
        setLiveLines(current => {
          const next = [
            ...current,
            { source: payload.source ?? 'out', text: payload.text ?? '' }
          ]
          return next.length > PM2_LIVE_MAX_LINES
            ? next.slice(next.length - PM2_LIVE_MAX_LINES)
            : next
        })
      } catch {
        // Ignore malformed frames; EventSource reconnects on its own.
      }
    }
    return () => stream.close()
  }, [live, open, root, setError, setLiveConnected, setLiveLines])

  const toggleLive = () => {
    if (!live) {
      setDate('')
      setPage(1)
    }
    setLive(current => !current)
  }

  useEffect(() => {
    followLatest.current = true
  }, [committedQuery, date, live, page])

  const visibleLines = useMemo<PM2AuditLine[]>(() => {
    const snapshot = data?.lines ?? []
    if (!live) return snapshot
    const q = committedQuery.trim().toLowerCase()
    const streamed = liveLines.filter(line => {
      if (source !== 'all' && line.source !== source) return false
      return !q || line.text.toLowerCase().includes(q)
    })
    return [...snapshot, ...streamed]
  }, [committedQuery, data, live, liveLines, source])

  useEffect(() => {
    if (!open) return
    const el = outputRef.current
    if (!el || !followLatest.current) return
    const frame = window.requestAnimationFrame(() => {
      if (outputRef.current)
        outputRef.current.scrollTop = outputRef.current.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
  }, [open, visibleLines])

  const dayIndex = days.findIndex(day => day.date === date)
  const olderDay =
    dayIndex >= 0 && dayIndex + 1 < days.length ? days[dayIndex + 1].date : ''
  const newerDay = dayIndex > 0 ? days[dayIndex - 1].date : ''
  const selectDay = (target: string) => {
    setDate(target)
    setPage(1)
    followLatest.current = true
  }
  const jumpLatest = () => {
    setDate('')
    setPage(1)
    followLatest.current = true
  }
  const refresh = () => {
    void loadPage(live ? 1 : page)
    void loadDays()
  }
  const exportLogs = async () => {
    try {
      const params = new URLSearchParams({ root })
      if (date) params.set('date', date)
      if (source !== 'all') params.set('source', source)
      if (committedQuery.trim()) params.set('query', committedQuery.trim())
      const response = await fetch(`/api/v1/robot/pm2-logs/export?${params}`)
      if (!response.ok) {
        const result = (await response.json().catch(() => null)) as {
          error?: string
        } | null
        throw new Error(result?.error || '导出失败。')
      }
      const blob = await response.blob()
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = date ? `pm2-${date}.log` : 'pm2-logs.log'
      link.click()
      URL.revokeObjectURL(url)
    } catch (reason) {
      setError(operationErrorMessage(reason, '导出 PM2 日志失败。'))
    }
  }

  if (!open) return null
  return (
    <DesktopWindow
      id="pm2Logs"
      open
      minimized={minimized}
      title="PM2 运行日志"
      subtitle={
        live
          ? '实时跟随最新记录'
          : date
            ? `按日期审计 · ${date}`
            : '最新日志 · 只读'
      }
      icon={
        <Terminal className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 168, top: 152 }}
      actions={
        <button
          className="icon-button size-8 p-0"
          disabled={loading}
          onClick={refresh}
          aria-label="刷新 PM2 日志"
          title="刷新"
        >
          <RefreshCw className="size-4" />
        </button>
      }
    >
      <div className="pm2-audit">
        <div className="pm2-audit-toolbar">
          <button
            className={cn('pm2-audit-live', live && 'active')}
            onClick={toggleLive}
            title={live ? '暂停实时跟随' : '实时跟随最新记录'}
          >
            {live ? <Pause className="size-3" /> : <Radio className="size-3" />}
            {live ? '跟随中' : '实时'}
          </button>
          <select
            className="pm2-audit-field"
            value={date}
            onChange={event => selectDay(event.target.value)}
            aria-label="按日期查看 PM2 日志"
            title="按日期查看日志"
          >
            <option value="">最新（全部日期）</option>
            {days.map(day => (
              <option key={day.date} value={day.date}>
                {day.date}（{day.count} 条 · err {day.err}）
              </option>
            ))}
          </select>
          <button
            className="pm2-audit-tool-btn"
            disabled={!olderDay}
            onClick={() => selectDay(olderDay)}
            title="查看更早一天的日志"
          >
            <ArrowLeft className="size-3" />
            前一天
          </button>
          <button
            className="pm2-audit-tool-btn"
            disabled={!newerDay}
            onClick={() => selectDay(newerDay)}
            title="查看更新一天的日志"
          >
            后一天
            <ArrowRight className="size-3" />
          </button>
          <button
            className="pm2-audit-tool-btn"
            disabled={!date}
            onClick={jumpLatest}
            title="回到最新日志"
          >
            最新
          </button>
        </div>
        <div className="pm2-audit-toolbar">
          <div className="pm2-audit-source" role="group" aria-label="日志来源">
            {[
              ['all', '全部'],
              ['out', '输出'],
              ['err', '错误']
            ].map(([value, label]) => (
              <button
                key={value}
                className={cn(
                  'pm2-audit-source-btn',
                  source === value && 'active'
                )}
                onClick={() => {
                  setSource(value)
                  setPage(1)
                }}
              >
                {label}
              </button>
            ))}
          </div>
          <input
            className="pm2-audit-field pm2-audit-search"
            value={query}
            onChange={event => setQuery(event.target.value)}
            placeholder="搜索日志内容…"
            aria-label="搜索 PM2 日志"
          />
          <select
            className="pm2-audit-field"
            value={perPage}
            onChange={event => {
              setPerPage(Number(event.target.value))
              setPage(1)
            }}
            aria-label="每页行数"
            title="每页行数"
          >
            {[120, 300, 1000].map(size => (
              <option key={size} value={size}>
                {size} 行/页
              </option>
            ))}
          </select>
          <button
            className="pm2-audit-tool-btn"
            disabled={loading || live || !data?.hasNewer}
            onClick={() => setPage(current => Math.max(1, current - 1))}
            title="查看更新的一页"
          >
            <ArrowLeft className="size-3" />
            更新
          </button>
          <span className="pm2-audit-page">
            {live ? '实时' : `第 ${data?.page ?? page} 页`}
            {data ? ` · 共 ${data.total} 条匹配` : ''}
            {data?.truncated ? ' · 已截断' : ''}
          </span>
          <button
            className="pm2-audit-tool-btn"
            disabled={loading || live || !data?.hasOlder}
            onClick={() => setPage(current => current + 1)}
            title="查看更早的一页"
          >
            更早
            <ArrowRight className="size-3" />
          </button>
          <button
            className="pm2-audit-tool-btn"
            onClick={refresh}
            title="刷新当前视图"
          >
            <RefreshCw className="size-3" />
            刷新
          </button>
          <button
            className="pm2-audit-tool-btn"
            onClick={() => void exportLogs()}
            title="导出当前筛选的完整日志（含 out/err 标记）"
          >
            <Download className="size-3" />
            导出
          </button>
        </div>
        <div
          className="pm2-audit-body"
          ref={outputRef}
          onScroll={event => {
            const el = event.currentTarget
            followLatest.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 48
          }}
        >
          {loading && !data && !live ? (
            <div className="pm2-audit-hint">正在读取最新 PM2 日志…</div>
          ) : error ? (
            <div className="pm2-audit-line pm2-audit-line-err">{error}</div>
          ) : visibleLines.length === 0 ? (
            <div className="pm2-audit-hint">PM2 暂无可读取的日志。</div>
          ) : (
            visibleLines.map((line, index) => (
              <div
                key={`${line.source}-${index}`}
                className={cn(
                  'pm2-audit-line',
                  line.source === 'err' && 'pm2-audit-line-err'
                )}
              >
                {line.text}
              </div>
            ))
          )}
          {live && (
            <div className="pm2-audit-hint">
              {liveConnected
                ? '已连接，正在跟随最新记录…'
                : '正在连接实时日志…'}
            </div>
          )}
        </div>
      </div>
    </DesktopWindow>
  )
}
function PM2ProcessesPanel({
  open,
  root,
  minimized,
  onClose,
  onMinimize,
  zIndex,
  onActivate
}: {
  open: boolean
  root: string
  minimized: boolean
  onClose: () => void
  onMinimize: () => void
  zIndex: number
  onActivate: () => void
}) {
  const { data, isFetching, isError, error, refetch } =
    useRobotPM2ProcessesQuery(root, {
      skip: !open || !root
    })
  const items = data?.items ?? []
  if (!open) return null
  const formatBytes = (value: number) => {
    if (!value) return '—'
    const units = ['B', 'KB', 'MB', 'GB']
    let size = value
    let index = 0
    while (size >= 1024 && index < units.length - 1) {
      size /= 1024
      index++
    }
    return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
  }
  const formatUptime = (value: number) => {
    if (!value) return '—'
    const seconds = Math.floor((Date.now() - value) / 1000)
    if (seconds < 60) return `${seconds}s`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
    return `${Math.floor(seconds / 86400)}d`
  }
  const statusTone = (status: string) =>
    status === 'online'
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300'
      : status === 'launching'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300'
        : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400'
  return (
    <DesktopWindow
      id="pm2Status"
      open
      minimized={minimized}
      title="PM2 进程"
      subtitle="当前机器人的 PM2 守护进程管理的全部服务"
      icon={
        <Activity className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 216, top: 200 }}
      actions={
        <button
          className="icon-button size-8 p-0"
          disabled={isFetching}
          onClick={() => void refetch()}
          aria-label="刷新 PM2 进程"
          title="刷新"
        >
          <RefreshCw className="size-4" />
        </button>
      }
    >
      <section className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto] overflow-hidden">
        <header className="hidden">
          <div className="flex min-w-0 items-center gap-2">
            <Terminal className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
            <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              PM2 进程
            </strong>
            <small className="hidden text-xs text-slate-400 sm:inline">
              当前机器人的 PM2 守护进程管理的全部服务
            </small>
          </div>
          <div className="flex items-center gap-1">
            <button
              className="inline-flex size-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
              disabled={isFetching}
              onClick={() => void refetch()}
              aria-label="刷新 PM2 进程"
              title="刷新"
            >
              <RefreshCw className="size-4" />
            </button>
            <button
              className="inline-flex size-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 hover:bg-slate-50 dark:border-slate-600 dark:text-slate-300 dark:hover:bg-slate-800"
              onClick={onClose}
              aria-label="关闭 PM2 进程"
              title="关闭"
            >
              <X className="size-4" />
            </button>
          </div>
        </header>
        {isError ? (
          <div className="grid min-h-0 place-items-center gap-3 p-8 text-center">
            <p className="text-xs text-slate-500">
              {operationErrorMessage(error, '无法读取 PM2 进程。')}
            </p>
            <button className="secondary-button" onClick={() => void refetch()}>
              重试
            </button>
          </div>
        ) : isFetching && !items.length ? (
          <div className="grid min-h-0 place-items-center p-8 text-xs text-slate-400">
            正在读取 PM2 进程…
          </div>
        ) : !items.length ? (
          <div className="grid min-h-0 place-items-center p-8 text-xs text-slate-400">
            当前没有正在运行的 PM2 进程。
          </div>
        ) : (
          <div className="min-h-0 overflow-auto">
            <table className="w-full border-collapse text-left text-xs">
              <thead className="sticky top-0 bg-slate-50 text-slate-400 dark:bg-slate-800 dark:text-slate-300">
                <tr>
                  <th className="px-3 py-2 font-medium">ID</th>
                  <th className="px-3 py-2 font-medium">名称</th>
                  <th className="px-3 py-2 font-medium">状态</th>
                  <th className="px-3 py-2 font-medium">PID</th>
                  <th className="px-3 py-2 font-medium">内存</th>
                  <th className="px-3 py-2 font-medium">CPU</th>
                  <th className="px-3 py-2 font-medium">重启</th>
                  <th className="px-3 py-2 font-medium">运行时长</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {items.map(item => (
                  <tr
                    key={item.id}
                    className="text-slate-700 dark:text-slate-200"
                  >
                    <td className="px-3 py-2 font-mono text-slate-400">
                      {item.id}
                    </td>
                    <td className="px-3 py-2 font-semibold">{item.name}</td>
                    <td className="px-3 py-2">
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-medium ${statusTone(item.status)}`}
                      >
                        <i className="inline-block size-1.5 rounded-full bg-current" />
                        {item.status}
                      </span>
                    </td>
                    <td className="px-3 py-2 font-mono">{item.pid || '—'}</td>
                    <td className="px-3 py-2 font-mono">
                      {formatBytes(item.memory)}
                    </td>
                    <td className="px-3 py-2 font-mono">
                      {item.cpu ? `${item.cpu.toFixed(1)}%` : '—'}
                    </td>
                    <td className="px-3 py-2 font-mono">{item.restarts}</td>
                    <td className="px-3 py-2 font-mono">
                      {formatUptime(item.uptime)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <footer className="flex items-center justify-between border-t border-slate-200 px-4 py-3 dark:border-slate-700">
          <span className="text-xs text-slate-500">
            共 {items.length} 个进程
          </span>
          <button
            className="secondary-button"
            disabled={isFetching}
            onClick={() => void refetch()}
          >
            刷新
          </button>
        </footer>
      </section>
    </DesktopWindow>
  )
}
function EditorMode({
  active,
  onVisual,
  onText
}: {
  active: 'visual' | 'text'
  onVisual: () => void
  onText: () => void
}) {
  return (
    <Tabs
      ariaLabel="配置编辑模式"
      value={active}
      onChange={value => (value === 'text' ? onText() : onVisual())}
      variant="segmented"
      items={[
        {
          id: 'visual',
          label: '表单',
          icon: <ClipboardList className="size-3.5" />
        },
        { id: 'text', label: '文本', icon: <FileText className="size-3.5" /> }
      ]}
    />
  )
}
function FileEditor({
  toolbar,
  content,
  placeholder,
  onChange
}: {
  toolbar?: ReactNode
  content: string
  placeholder: string
  onChange: (value: string) => void
}) {
  return (
    <RobotPanel
      className="file-editor"
      icon={<Settings className="size-4" />}
      title="文本编辑"
      description="直接编辑当前配置文件内容"
      actions={toolbar}
    >
      <textarea
        className="min-h-105 w-full resize-y rounded-lg border border-slate-300 bg-white p-3 font-mono text-xs leading-5 text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100"
        value={content}
        onChange={event => onChange(event.target.value)}
        placeholder={placeholder}
      />
    </RobotPanel>
  )
}
function OperationLog({
  output,
  failed,
  onClose
}: {
  output: string
  failed: boolean
  onClose: () => void
}) {
  const needsPermission =
    failed && /没有权限|访问权限|permission denied|eacces/i.test(output)
  return (
    <aside
      className={`fixed bottom-5.5 right-5.5 z-80 grid max-h-[min(240px,38vh)] w-[min(420px,calc(100vw-44px))] gap-2 overflow-auto rounded-panel border p-3 text-(--theme-text-primary) shadow-[0_16px_38px_rgb(28_26_23/0.13)] ${failed ? 'border-(--theme-danger) bg-(--theme-danger-soft)' : 'border-(--theme-accent-soft-border) bg-(--theme-success-soft)'}`}
      aria-live="polite"
      aria-label="最近操作结果"
    >
      <header className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <i
            className={`inline-flex size-5 items-center justify-center rounded-full text-xs not-italic font-bold ${failed ? 'bg-(--theme-danger) text-white' : 'bg-(--theme-success-soft) text-(--theme-success-text)'}`}
          >
            {failed ? '!' : '✓'}
          </i>
          <strong
            className={`text-xs ${failed ? 'text-(--theme-danger-text)' : 'text-(--theme-success-text)'}`}
          >
            {needsPermission
              ? '需要访问授权'
              : failed
                ? '操作未完成'
                : '操作已完成'}
          </strong>
        </div>
        <button
          className="text-button size-7 min-h-7 p-0"
          onClick={onClose}
          aria-label="关闭操作结果"
        >
          ×
        </button>
      </header>
      <pre className="m-0 max-h-45 overflow-auto whitespace-pre-wrap rounded-md bg-(--theme-surface-code) p-2.5 font-mono text-[0.73rem] leading-relaxed text-(--theme-text-code)">
        {output}
      </pre>
      <small className="text-[11px] text-(--theme-text-muted)">
        {needsPermission ? '授权完成后，请回到这里重新执行本次操作。' : ''}
      </small>
    </aside>
  )
}

function GitReleasePanelNext({
  root,
  busy,
  version,
  onVersionChange
}: {
  root: string
  busy: boolean
  version: string
  onVersionChange: (value: string) => void
}) {
  type SourceCommit = {
    sha: string
    shortSha: string
    subject: string
    createdAt: string
  }
  type GitStatus = {
    packageName?: string
    packageVersion?: string
    packageManager?: string
    repository?: string
    branch?: string
    remoteBranch?: string
    remoteReachable?: boolean
    remoteAdvice?: string
    suggestedVersion?: string
    sourceCommits?: SourceCommit[]
    sourceBranches?: Array<{ name: string; commits: SourceCommit[] }>
    gitReady?: boolean
    checks?: string[]
    issues?: string[]
  }
  type BuildSession = {
    sessionId: string
    branch: string
    commit: string
    target: string
    files: string[]
    logs: string
    expiresAt: string
  }
  type PublishResult = { output?: string; path?: string }
  const {
    data,
    isFetching: loading,
    error,
    refetch
  } = useGitStatusQuery(root, { skip: !root })
  const [sourceCommit, setSourceCommit] = useStoreState('')
  const [sourceBranch, setSourceBranch] = useStoreState('')
  const [phase, setPhase] = useStoreState<
    'source' | 'building' | 'artifacts' | 'confirm' | 'published'
  >('source')
  const [session, setSession] = useStoreState<BuildSession | null>(null)
  const [artifacts, setArtifacts] = useStoreState<string[]>([])
  const [expandedArtifacts, setExpandedArtifacts] = useStoreState<string[]>([])
  const [requestError, setRequestError] = useStoreState('')
  const [result, setResult] = useStoreState<PublishResult | null>(null)
  const [retryingTag, setRetryingTag] = useStoreState(false)
  const remoteBranchesRefreshed = useRef('')
  const status = error
    ? { issues: ['无法读取 Git 发布状态。'] }
    : (data as GitStatus | undefined)
  const branches = status?.sourceBranches ?? emptyGitBranches
  const selectedBranch =
    branches.find(item => item.name === sourceBranch) ??
    branches.find(item => item.name === status?.branch) ??
    branches[0]
  const targetReleaseBranch =
    selectedBranch?.name === status?.remoteBranch
      ? 'release'
      : `${(selectedBranch?.name || 'source').replace(/[\s/]+/g, '-')}-release`
  const commits =
    selectedBranch?.commits ?? status?.sourceCommits ?? emptyGitCommits
  useEffect(() => {
    if (!root || !status?.gitReady || remoteBranchesRefreshed.current === root)
      return
    remoteBranchesRefreshed.current = root
    void fetch('/api/v1/publish/git/refresh-branches', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root })
    })
      .then(response => {
        if (response.ok) return refetch()
      })
      .catch(() => {
        // 远程不可用时仍可使用已缓存的本地分支，不打断发布页。
      })
  }, [refetch, root, status?.gitReady])
  useEffect(() => {
    if (!branches.some(item => item.name === sourceBranch))
      setSourceBranch(status?.branch || branches[0]?.name || '')
    if (!commits.some(item => item.sha === sourceCommit))
      setSourceCommit(commits[0]?.sha ?? '')
  }, [
    branches,
    commits,
    sourceBranch,
    sourceCommit,
    status?.branch,
    setSourceBranch,
    setSourceCommit
  ])
  const issues = status?.issues ?? []
  const blockingIssues = issues
  const ready = !loading && blockingIssues.length === 0 && !!sourceCommit
  const refresh = () => {
    setPhase('source')
    setSession(null)
    setArtifacts([])
    setRequestError('')
    setResult(null)
    void refetch()
  }
  const post = async <T,>(url: string, body: object): Promise<T> => {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    })
    const payload = (await response.json().catch(() => ({}))) as T & {
      error?: string
    }
    if (!response.ok) throw new Error(payload.error || '请求失败，请稍后重试。')
    return payload
  }
  const prepareBuild = async () => {
    if (!selectedBranch?.name || !sourceCommit) return
    setPhase('building')
    setRequestError('')
    setResult(null)
    try {
      const next = await post<BuildSession>('/api/v1/publish/git/prepare', {
        root,
        branch: selectedBranch.name,
        commit: sourceCommit
      })
      setSession(next)
      setArtifacts(
        ['lib', 'dist', 'README.md'].filter(item => next.files.includes(item))
      )
      setPhase('artifacts')
    } catch (err) {
      setRequestError(
        err instanceof Error ? err.message : '构建失败，请重新构建。'
      )
      setPhase('source')
    }
  }
  const publish = async () => {
    if (!session || !artifacts.length) return
    setRequestError('')
    try {
      const next = await post<PublishResult>('/api/v1/publish/git/publish', {
        sessionId: session.sessionId,
        version,
        artifacts,
        confirm: true
      })
      setResult(next)
      setPhase('published')
    } catch (err) {
      setRequestError(
        err instanceof Error ? err.message : '发布失败，请检查日志后重试。'
      )
    }
  }
  const retryTag = async () => {
    if (!session) return
    setRetryingTag(true)
    setRequestError('')
    try {
      const next = await post<PublishResult>('/api/v1/publish/git/retry-tag', {
        sessionId: session.sessionId
      })
      setResult(next)
      setPhase('published')
    } catch (err) {
      setRequestError(
        err instanceof Error ? err.message : '标签重试失败，请稍后再试。'
      )
    } finally {
      setRetryingTag(false)
    }
  }
  const artifactIndex = useMemo(() => {
    const files = session?.files ?? []
    const directories = new Set<string>()
    const children = new Map<string, string[]>()
    for (const path of files) {
      const pieces = path.split('/')
      for (let index = 1; index < pieces.length; index += 1) {
        const parent = pieces.slice(0, index).join('/')
        const child = pieces.slice(0, index + 1).join('/')
        directories.add(parent)
        const current = children.get(parent) ?? []
        if (!current.includes(child)) children.set(parent, [...current, child])
      }
    }
    const leaves = files.filter(path => !directories.has(path))
    const descendants = new Map<string, string[]>()
    for (const leaf of leaves) {
      descendants.set(leaf, [leaf])
      const pieces = leaf.split('/')
      for (let index = 1; index < pieces.length; index += 1) {
        const parent = pieces.slice(0, index).join('/')
        descendants.set(parent, [...(descendants.get(parent) ?? []), leaf])
      }
    }
    return {
      directories,
      children,
      descendants,
      top: files.filter(path => !path.includes('/'))
    }
  }, [session])
  const selectedArtifacts = useMemo(() => new Set(artifacts), [artifacts])
  const descendantFiles = (item: string) =>
    artifactIndex.descendants.get(item) ?? []
  const isDirectory = (item: string) => artifactIndex.directories.has(item)
  const artifactSelected = (item: string) => {
    const leaves = descendantFiles(item)
    return (
      leaves.length > 0 &&
      leaves.every(leaf => {
        const parts = leaf.split('/')
        return parts.some((_, index) =>
          selectedArtifacts.has(parts.slice(0, index + 1).join('/'))
        )
      })
    )
  }
  const toggleArtifact = (item: string) => {
    setArtifacts(current =>
      current.includes(item)
        ? current.filter(value => value !== item)
        : [...current, item]
    )
  }
  return (
    <RobotPanel
      className="git-release-panel max-w-230 content-start"
      headerClassName="release-toolbar"
      icon={<GitBranch className="size-4" />}
      title={
        status?.packageName
          ? `${status.packageName}@${status.packageVersion || '未设置版本'}`
          : 'GIT 发布'
      }
      description={`GIT 发布 · ${status?.packageManager || '管理分支、构建产物与版本标签'}`}
      actions={
        <div className="release-toolbar-actions flex flex-wrap items-end justify-end gap-2">
          {(phase === 'artifacts' || phase === 'confirm') && (
            <button
              className="inline-flex h-9 items-center justify-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 text-xs font-semibold text-slate-600 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-300"
              onClick={() =>
                setPhase(phase === 'confirm' ? 'artifacts' : 'source')
              }
            >
              <ArrowLeft className="size-4" />
              上一步
            </button>
          )}
          <button
            className="inline-flex size-9 items-center justify-center rounded-lg border border-slate-200 bg-white p-0 text-xs font-semibold text-slate-600 disabled:opacity-40 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-300"
            onClick={refresh}
            disabled={loading || busy}
            aria-label="刷新发布状态"
            title="刷新发布状态"
          >
            <RefreshCw className="size-4" />
          </button>
          <button
            className="inline-flex h-9 items-center justify-center gap-1.5 rounded-lg bg-brand-600 px-3 text-xs font-semibold text-white transition hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={
              busy ||
              loading ||
              phase === 'building' ||
              (phase === 'source' && !ready) ||
              (phase === 'artifacts' && !artifacts.length)
            }
            onClick={() => {
              if (phase === 'source') void prepareBuild()
              else if (phase === 'artifacts') setPhase('confirm')
              else if (phase === 'confirm') void publish()
              else if (phase === 'published') refresh()
            }}
          >
            {phase === 'source' ? (
              <Code2 className="size-4" />
            ) : phase === 'artifacts' ? (
              <ArrowRight className="size-4" />
            ) : phase === 'confirm' ? (
              <CheckCircle2 className="size-4" />
            ) : (
              <RefreshCw className="size-4" />
            )}
            {busy || phase === 'building'
              ? '构建中…'
              : phase === 'source'
                ? '开始构建'
                : phase === 'artifacts'
                  ? '继续确认'
                  : phase === 'confirm'
                    ? '确认发布'
                    : '重新开始'}
          </button>
        </div>
      }
    >
      {loading ? (
        <p className="m-0 rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-500 dark:bg-slate-800 dark:text-slate-400">
          正在读取所选目录的 Git 状态…
        </p>
      ) : (
        <>
          {phase === 'source' && (
            <section className="release-source-card grid gap-3 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
              <div className="grid gap-1">
                <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  1. 选择源码提交
                </strong>
                <p className="m-0 text-xs leading-5 text-slate-500">
                  只会构建这次已提交的代码，不会包含你还没提交的本地修改。
                </p>
              </div>
              <label className="release-field grid gap-1 text-xs font-semibold text-slate-500">
                源码分支{' '}
                <select
                  className="min-h-9 rounded-lg border border-slate-200 bg-white px-2 text-sm font-normal text-slate-700 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
                  value={selectedBranch?.name || ''}
                  disabled={phase !== 'source'}
                  onChange={event => setSourceBranch(event.target.value)}
                >
                  {branches.map(item => (
                    <option key={item.name} value={item.name}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </label>
              <label className="release-field grid gap-1 text-xs font-semibold text-slate-500">
                发布目标{' '}
                <input
                  className="min-h-9 rounded-lg border border-slate-200 bg-slate-50 px-2 text-sm font-normal text-slate-500 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-400"
                  value={targetReleaseBranch}
                  readOnly
                />
              </label>
              <label className="release-field release-commit-field grid gap-1 text-xs font-semibold text-slate-500">
                提交{' '}
                <select
                  value={sourceCommit}
                  onChange={event => setSourceCommit(event.target.value)}
                  disabled={!commits.length || phase !== 'source'}
                >
                  {commits.length ? (
                    commits.map(item => (
                      <option key={item.sha} value={item.sha}>
                        {item.shortSha} · {item.subject} · {item.createdAt}
                      </option>
                    ))
                  ) : (
                    <option value="">暂无可选提交</option>
                  )}
                </select>
              </label>
            </section>
          )}
          {phase === 'confirm' && (
            <section className="release-source-card compact grid gap-3 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
              <div className="grid gap-1">
                <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  2. 设置发布版本
                </strong>
                <p className="m-0 text-xs leading-5 text-slate-500">
                  发布时会创建不可覆盖的 Git Tag，并推送到{' '}
                  {session?.target || targetReleaseBranch}。
                </p>
              </div>
              <label className="grid max-w-xs gap-1 text-xs font-semibold text-slate-500">
                版本{' '}
                <input
                  value={version || status?.suggestedVersion || ''}
                  onChange={event => onVersionChange(event.target.value)}
                  className="min-h-9 rounded-lg border border-slate-200 bg-white px-2 text-sm font-normal text-slate-700 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
                  placeholder="v0.0.1"
                />
              </label>
            </section>
          )}
          {phase === 'building' && (
            <section className="release-source-card grid gap-1 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
              <div>
                <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  正在构建
                </strong>
                <p className="m-0 text-xs leading-5 text-slate-500">
                  正在隔离目录中安装依赖并执行 build。完成前不能选择产物。
                </p>
              </div>
            </section>
          )}
          {session && phase === 'artifacts' && (
            <section className="release-source-card release-artifact-card grid gap-3 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
              <div className="grid gap-1">
                <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  3. 选择最终产物
                </strong>
                <p className="m-0 text-xs leading-5 text-slate-500">
                  以下是本次构建实际生成的可发布文件。默认全选；依赖、隐藏文件和
                  package.json 不会显示。
                </p>
              </div>
              <div className="release-artifacts flex flex-wrap gap-2">
                {artifactIndex.top.map(item => (
                  <div className="release-artifact-tree" key={item}>
                    <label
                      className={cn(
                        'inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-2 text-xs transition',
                        artifactSelected(item)
                          ? 'border-brand-200 bg-brand-50 text-brand-600 dark:border-brand-700 dark:bg-brand-100/30 dark:text-brand-200'
                          : 'border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300'
                      )}
                    >
                      <input
                        className="accent-brand-600"
                        type="checkbox"
                        checked={artifactSelected(item)}
                        onChange={() => toggleArtifact(item)}
                      />
                      {isDirectory(item) && (
                        <button
                          type="button"
                          className="inline-flex size-5 items-center justify-center"
                          onClick={() =>
                            setExpandedArtifacts(current =>
                              current.includes(item)
                                ? current.filter(value => value !== item)
                                : [...current, item]
                            )
                          }
                        >
                          <ChevronRight
                            className={cn(
                              'size-3.5 transition',
                              expandedArtifacts.includes(item) && 'rotate-90'
                            )}
                          />
                        </button>
                      )}
                      <span>{item}</span>
                    </label>
                    {expandedArtifacts.includes(item) &&
                      (artifactIndex.children.get(item) ?? []).map(child => (
                        <label
                          className="artifact-child ml-5 mt-1 flex items-center gap-1.5 text-xs text-slate-500"
                          key={child}
                        >
                          <input
                            className="accent-brand-600"
                            type="checkbox"
                            checked={artifactSelected(child)}
                            onChange={() => toggleArtifact(child)}
                          />
                          <span>{child.slice(item.length + 1)}</span>
                        </label>
                      ))}
                  </div>
                ))}
              </div>
              <p className="m-0 text-xs text-slate-500">
                已选择 {artifacts.length} 项，将发布到{' '}
                <code className="rounded bg-slate-100 px-1 dark:bg-slate-800">
                  {session.target}
                </code>
                。本次构建保留至{' '}
                {new Date(session.expiresAt).toLocaleTimeString([], {
                  hour: '2-digit',
                  minute: '2-digit'
                })}
                。
              </p>
              {session.logs && (
                <details className="release-build-log">
                  <summary>查看构建日志</summary>
                  <pre>{session.logs}</pre>
                </details>
              )}
            </section>
          )}
          {phase === 'source' && (
            <p
              className={cn(
                'rounded-lg border px-3 py-2 text-xs font-semibold',
                ready
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-300'
                  : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300'
              )}
            >
              {ready ? '✓ 可以从所选提交开始构建' : '！ 发布前需要处理以下问题'}
            </p>
          )}
          {phase === 'source' && blockingIssues.length > 0 && (
            <section className="grid gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-900 dark:bg-amber-950/30">
              <ul className="m-0 grid gap-1 pl-4 text-xs leading-5 text-amber-800 dark:text-amber-300">
                {blockingIssues.map(item => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
              {!status?.gitReady && null}
              {status?.repository && (
                <p className="m-0 text-xs text-amber-800 dark:text-amber-300">
                  远程仓库：
                  <code className="break-all">{status.repository}</code>
                  {status.remoteAdvice ? ` · ${status.remoteAdvice}` : ''}
                </p>
              )}
            </section>
          )}
          {phase === 'confirm' && session && (
            <p className="m-0 rounded-lg border border-brand-200 bg-brand-50 px-3 py-2 text-xs leading-5 text-brand-700 dark:border-brand-200 dark:bg-brand-100/30 dark:text-brand-200">
              即将把 {artifacts.length} 项构建产物发布到{' '}
              <code>{session.target}</code>，并创建标签{' '}
              <code>{version || status?.suggestedVersion}</code>。
            </p>
          )}
          {requestError && (
            <p className="m-0 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs leading-5 text-rose-700 dark:border-rose-900 dark:bg-rose-950/30 dark:text-rose-300">
              ！ {requestError}
            </p>
          )}
          {session && requestError.includes('release 分支已推送') && (
            <button
              className="inline-flex min-h-9 w-fit items-center justify-center gap-1.5 rounded-lg bg-brand-600 px-3 text-xs font-semibold text-white hover:bg-brand-700 disabled:opacity-50"
              disabled={retryingTag}
              onClick={() => void retryTag()}
            >
              {retryingTag ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <GitBranch className="size-4" />
              )}
              {retryingTag ? '正在重试标签…' : '重试推送 Tag'}
            </button>
          )}
          {phase === 'published' && result?.output && (
            <pre className="release-result">{result.output}</pre>
          )}
        </>
      )}
    </RobotPanel>
  )
}
