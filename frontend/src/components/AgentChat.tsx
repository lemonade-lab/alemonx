import { useStoreState } from '../store/guideStore'
import { DirectoryPicker } from './Dashboard'
import {
  ArrowUp,
  Check,
  ChevronDown,
  CircleX,
  ExternalLink,
  FileSearch,
  FileText,
  Folder,
  ListTodo,
  Loader2,
  Minimize2,
  Target,
  Pencil,
  Plus,
  Settings2,
  ShieldCheck,
  ShieldQuestion,
  Slash,
  Sparkles,
  Square,
  Terminal,
  Trash2,
  Unlock,
  X
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties
} from 'react'
import { AgentMarkdown } from './AgentMarkdown'
import { Modal } from './Modal'
import cn from 'classnames'

// 开发环境默认走 Vite 的同源 /api 代理：后端通常监听 localhost，不能
// 假定 127.0.0.1 可用。只有部署方明确提供地址时才绕过同源代理。
const AGENT_API_BASE = (import.meta.env.VITE_AGENT_API_BASE ?? '').replace(
  /\/$/,
  ''
)
const AGENT_TASKS_BASE = import.meta.env.DEV
  ? 'http://localhost:17390/api/v1/agent/tasks'
  : `${AGENT_API_BASE}/api/v1/agent/tasks`

type Provider = {
  ID: string
  Name: string
  BaseURL: string
  Model: string
  HasKey: boolean
}
type ChatMessage = { role: 'user' | 'assistant'; content: string }
type PickerMode = 'directory' | 'file' | 'extension'
type Activity = {
  id: number
  callId: string
  tool: string
  args: string
  status: 'running' | 'done' | 'error'
  output?: string
}
type SessionMeta = {
  id: string
  title: string
  root: string
  provider: string
  model: string
  status?: 'idle' | 'running' | 'completed' | 'failed'
  turn?: number
  lastError?: string
  updated: string
}
type TaskMeta = {
  id: string
  sessionId: string
  status: string
  turn: number
  lastError?: string
  isolation?: string
  plan?: {
    goal: string
    completion: string
    currentStep: number
    steps: Array<{ id: string; title: string; status: string }>
  }
}
type GoalMeta = {
  id: string
  title: string
  prompt: string
  root: string
  scheduleMinutes?: number
  nextRun?: string
  status: string
  lastTaskId?: string
  lastError?: string
}
type OpsIncident = {
  id: string
  processName: string
  fingerprint: string
  status: string
  severity: string
  occurrences: number
  decisionReason?: string
  projectRoot: string
}
type OpsTodo = {
  id: string
  title: string
  summary: string
  severity: string
  status: string
  incidentId: string
}
type OpsMaintenance = {
  id: string
  incidentId: string
  taskId?: string
  status: string
  error?: string
  rollbackPerformed?: boolean
}
type OpsPolicyUI = {
  projectRoot: string
  mode: 'off' | 'observe' | 'auto' | 'strict'
  autoAllowed: boolean
  allowCodeChanges: boolean
  allowPm2Control: boolean
  observationMinutes: number
  maxModifiedFiles: number
  maxPm2Actions: number
}
type OpsMetrics = {
  incidents: number
  openTodos: number
  maintenanceRuns: number
  autoFixSuccess: number
  rollbacks: number
  resolved: number
  averageRecoverySecs: number
}
type GoalDraft = {
  id?: string
  title: string
  prompt: string
  root: string
  provider: string
  model: string
  scheduleMinutes: number
  isolation: string
}

const TOOL_LABEL: Record<string, string> = {
  read_project_file: '读取文件',
  list_project_files: '列出文件',
  agent_search: '搜索代码',
  agent_edit_file: '编辑文件',
  agent_run_command: '运行命令',
  agent_verify: '自动验证'
}

const TOOL_DESCRIPTION: Record<string, string> = {
  read_project_file: '读取项目文件',
  list_project_files: '列出项目文件',
  agent_search: '在项目中搜索代码',
  agent_edit_file: '精确编辑项目文件',
  agent_run_command: '运行白名单命令',
  agent_verify: '检查改动是否通过验证'
}

const PROMPT_EXAMPLES: Array<[string, string]> = [
  ['修复当前报错', '收集并分析项目当前错误，定位原因，修复后运行验证。'],
  ['介绍一下这个项目', '读取项目结构，帮我介绍这个机器人项目。'],
  ['加一个新命令', '给机器人新增一个打招呼的命令并验证。'],
  ['找功能实现位置', '搜索某个功能的实现位置并解释。'],
  ['修复最近的报错', '查看最近改动，修复导致的报错。'],
  ['设置长期目标', '设置一个每 60 分钟检查项目并报告问题的长期目标。']
]

const PROVIDER_KEY_LINKS: Record<string, { href: string; label: string }> = {
  openai: {
    href: 'https://platform.openai.com/api-keys',
    label: '获取 OpenAI API Key'
  },
  deepseek: {
    href: 'https://platform.deepseek.com/api_keys',
    label: '获取 DeepSeek API Key'
  },
  claude: {
    href: 'https://platform.claude.com/settings/keys',
    label: '获取 Claude API Key'
  }
}

// formatToolArgs renders a tool's JSON arguments as a short friendly line, so
// the timeline shows "读取 src/index.ts" instead of raw `{"path":"..."}`.
function formatToolArgs(tool: string, rawArgs: string): string {
  if (!rawArgs) return ''
  try {
    const args = JSON.parse(rawArgs) as Record<string, unknown>
    const path = typeof args.path === 'string' ? args.path : ''
    switch (tool) {
      case 'read_project_file':
        return path ? `读取 ${path}` : rawArgs
      case 'list_project_files':
        return '列出项目文件'
      case 'agent_search': {
        const pattern = typeof args.pattern === 'string' ? args.pattern : ''
        return pattern ? `搜索 ${pattern}` : rawArgs
      }
      case 'agent_edit_file': {
        const mode = typeof args.mode === 'string' ? args.mode : 'edit'
        if (mode === 'create') return `新建 ${path}`
        if (mode === 'delete') return `删除 ${path}`
        const hunks = Array.isArray(args.edits) ? args.edits.length : 0
        return `编辑 ${path}${hunks > 0 ? `（${hunks} 处）` : ''}`
      }
      case 'agent_run_command': {
        const command = typeof args.command === 'string' ? args.command : ''
        const sub = Array.isArray(args.args)
          ? (args.args as string[]).join(' ')
          : ''
        return `运行 ${command}${sub ? ` ${sub}` : ''}`
      }
      case 'agent_verify':
        return '运行验证'
      default:
        return rawArgs
    }
  } catch {
    return rawArgs
  }
}

// formatUpdated renders an ISO timestamp as a short relative time in Chinese.
function formatUpdated(iso: string): string {
  if (!iso) return ''
  const updated = new Date(iso).getTime()
  if (Number.isNaN(updated)) return ''
  const diff = Date.now() - updated
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  if (diff < minute) return '刚刚'
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`
  return `${Math.floor(diff / day)} 天前`
}

// sessionToMessages keeps only the plain conversation turns when resuming a
// history. The persisted transcript also carries role="tool" result payloads
// (raw JSON / command output) and empty assistant tool-call frames; neither
// belongs in the chat timeline. Adjacent duplicate user turns are collapsed
// too: older transcripts once wrote the same user message twice.
function sessionToMessages(
  messages: Array<{ role: string; content?: string }>
): ChatMessage[] {
  const out: ChatMessage[] = []
  for (const message of messages) {
    if (
      (message.role !== 'user' && message.role !== 'assistant') ||
      !message.content
    ) {
      continue
    }
    const prev = out[out.length - 1]
    if (
      message.role === 'user' &&
      prev &&
      prev.role === 'user' &&
      prev.content === message.content
    ) {
      continue
    }
    out.push({ role: message.role, content: message.content })
  }
  return out
}

function ToolIcon({ name }: { name: string }) {
  switch (name) {
    case 'agent_edit_file':
      return <Pencil className="size-3.5" />
    case 'agent_search':
      return <FileSearch className="size-3.5" />
    case 'agent_run_command':
      return <Terminal className="size-3.5" />
    case 'agent_verify':
      return <Check className="size-3.5" />
    default:
      return <FileText className="size-3.5" />
  }
}

function StepStatus({ status }: { status: Activity['status'] }) {
  if (status === 'running') {
    return (
      <>
        <Loader2 className="spinner size-3.5 animate-spin" />
        <span>执行中</span>
      </>
    )
  }
  if (status === 'done') {
    return (
      <>
        <Check className="size-3.5" />
        <span>完成</span>
      </>
    )
  }
  return (
    <>
      <CircleX className="size-3.5" />
      <span>失败</span>
    </>
  )
}

function AgentAssistantMessage({
  content,
  streaming
}: {
  content: string
  streaming: boolean
}) {
  return (
    <article className="flex w-full max-w-full items-start gap-2.5 self-start">
      <span className="flex size-7.5 shrink-0 items-center justify-center rounded-[9px] border border-(--theme-border-default) bg-(--theme-surface-raised) text-(--theme-accent-text)">
        <Sparkles className="size-3.5" />
      </span>
      <div className="min-w-0 w-full flex-1">
        <span className="mb-1.5 mt-0.5 block text-[0.7rem] font-semibold text-(--theme-text-muted)">
          Agent
        </span>
        <AgentMarkdown content={content} streaming={streaming} />
      </div>
    </article>
  )
}

function AgentQuestionRail({
  messages,
  threadRef
}: {
  messages: ChatMessage[]
  threadRef: React.RefObject<HTMLElement | null>
}) {
  const questions = messages
    .map((message, index) => ({ message, index }))
    .filter(item => item.message.role === 'user')
  const [active, setActive] = useState(
    questions.length ? questions.length - 1 : 0
  )
  const [preview, setPreview] = useState<number | null>(null)
  const railRef = useRef<HTMLElement | null>(null)
  const [railLeft, setRailLeft] = useState<number | null>(null)

  // A sticky element keeps its original document-flow position until scrolling
  // reaches it. The question rail is navigation, so anchor it to the visible
  // chat viewport instead and only derive its horizontal offset from agent-main.
  useLayoutEffect(() => {
    const rail = railRef.current
    const host = rail?.closest<HTMLElement>('.agent-main')
    if (!rail || !host) return
    const update = () => setRailLeft(host.getBoundingClientRect().left)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(host)
    window.addEventListener('resize', update)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', update)
    }
  }, [questions.length])

  if (questions.length === 0) return null
  // The rail has a fixed visual budget. As question history grows, shrink the
  // gap/hit area instead of making a second, arbitrarily tall timeline.
  const slot = Math.min(20, Math.max(5, Math.floor(360 / questions.length)))
  const gap =
    questions.length === 1
      ? 0
      : Math.min(8, Math.max(1, Math.floor(slot * 0.3)))
  const previewOffset =
    preview === null ? 0 : (preview - (questions.length - 1) / 2) * slot
  const railStyle = {
    '--agent-question-gap': `${gap}px`,
    '--agent-question-slot': `${Math.max(2, slot - gap)}px`,
    '--agent-question-preview-offset': `${previewOffset}px`,
    'left': railLeft === null ? undefined : `${railLeft}px`
  } as CSSProperties
  const previewQuestion = preview === null ? null : questions[preview]
  return (
    <aside
      className="agent-question-rail box-border block h-[min(360px,calc(100dvh-80px))] min-h-0 -translate-y-1/2 self-start border-0 bg-transparent px-0.5 fixed top-1/2 z-2 [grid-column:1] [grid-row:1/span_2]"
      aria-label="问题记录导航"
      ref={railRef}
      style={railStyle}
    >
      <div className="agent-question-list relative flex h-full min-h-0 flex-col items-center justify-center gap-[var(--agent-question-gap,8px)]" role="list">
        {questions.map(({ message, index }, questionIndex) => (
          <button
            className={cn(
              'group relative z-1 flex w-4 min-h-[2px] flex-[0_0_var(--agent-question-slot,10px)] cursor-pointer items-center justify-center rounded-full border border-transparent bg-transparent p-0 hover:bg-(--theme-surface-hover)',
              questionIndex === active && 'active'
            )}
            key={index}
            title={message.content}
            aria-label={`问题 ${questionIndex + 1}：${message.content.trim() || '空问题'}`}
            onBlur={() => setPreview(null)}
            onFocus={() => setPreview(questionIndex)}
            onMouseEnter={() => setPreview(questionIndex)}
            onMouseLeave={() => setPreview(null)}
            onClick={() => {
              const target = threadRef.current?.querySelector<HTMLElement>(
                `[data-agent-message-index="${index}"]`
              )
              target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
              setActive(questionIndex)
            }}
          >
            <span
              className={cn(
                'block h-px w-2 bg-(--theme-text-faint) transition-[background,width] duration-150 group-hover:w-3 group-hover:bg-(--theme-text-strong)',
                questionIndex === active && 'w-3 bg-(--theme-text-strong)'
              )}
              aria-hidden="true"
            />
          </button>
        ))}
      </div>
      {previewQuestion && (
        <div className="agent-question-preview absolute top-[calc(50%+var(--agent-question-preview-offset,0px))] left-[calc(100%+10px)] z-20 grid w-[270px] min-w-[210px] max-w-[calc(100vw-96px)] -translate-y-1/2 gap-[5px] rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) px-[14px] py-3 text-(--theme-text-primary) shadow-[var(--theme-shadow-pop)]" role="status">
          <strong className="overflow-hidden text-[0.78rem] leading-[1.45] font-[650] text-(--theme-text-strong) text-ellipsis whitespace-nowrap">
            {previewQuestion.message.content.trim() || '空问题'}
          </strong>
          <span className="text-[0.7rem] leading-[1.5] text-(--theme-text-muted)">
            跳转到这次提问及其回答
          </span>
        </div>
      )}
    </aside>
  )
}

// AgentChatPage is the built-in coding-agent workspace. It streams the agent
// loop's progress over SSE and renders each tool call as a timeline step while
// the final answer streams in. Provider settings live behind the gear button.
export function AgentChatPage({
  root,
  initialSessionId
}: {
  root: string
  initialSessionId?: string
}) {
  const [providers, setProviders] = useStoreState<Provider[]>([])
  const [provider, setProvider] = useStoreState('')
  const [model, setModel] = useStoreState('')
  const [messages, setMessages] = useStoreState<ChatMessage[]>([])
  const [activity, setActivity] = useStoreState<Activity[]>([])
  const [prompt, setPrompt] = useStoreState('')
  const [busy, setBusy] = useStoreState(false)
  const [notice, setNotice] = useStoreState('')
  const [diagnostic, setDiagnostic] = useStoreState<{
    kind?: string
    file?: string
    line?: number
    message?: string
    explanation?: string
  } | null>(null)
  const [settings, setSettings] = useStoreState(false)
  const [baseURL, setBaseURL] = useStoreState('')
  const [apiKey, setAPIKey] = useStoreState('')
  const streamRef = useRef<AbortController | null>(null)
  const taskIdRef = useRef('')
  const activityId = useRef(0)
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
  const composerRef = useRef<HTMLElement | null>(null)
  const threadRef = useRef<HTMLElement | null>(null)
  const [access, setAccess] = useStoreState<'ask' | 'auto' | 'full'>('auto')
  const [timelineOpen, setTimelineOpen] = useStoreState(false)
  const [pendingConfirm, setPendingConfirm] = useStoreState<{
    id: string
    tool: string
    args: string
    diff?: {
      path: string
      mode?: string
      content?: string
      hunks?: Array<{ old: string; new: string }>
    } | null
  } | null>(null)
  const [sessions, setSessions] = useStoreState<SessionMeta[]>([])
  const [tasks, setTasks] = useStoreState<TaskMeta[]>([])
  const [goals, setGoals] = useStoreState<GoalMeta[]>([])
  const [opsIncidents, setOpsIncidents] = useStoreState<OpsIncident[]>([])
  const [opsTodos, setOpsTodos] = useStoreState<OpsTodo[]>([])
  const [opsMaintenance, setOpsMaintenance] = useStoreState<OpsMaintenance[]>(
    []
  )
  const [opsPaused, setOpsPaused] = useStoreState(false)
  const [opsPolicy, setOpsPolicy] = useStoreState<OpsPolicyUI>({
    projectRoot: root,
    mode: 'observe',
    autoAllowed: false,
    allowCodeChanges: false,
    allowPm2Control: false,
    observationMinutes: 5,
    maxModifiedFiles: 10,
    maxPm2Actions: 3
  })
  const [opsMetrics, setOpsMetrics] = useStoreState<OpsMetrics | null>(null)
  const [goalDraft, setGoalDraft] = useStoreState<GoalDraft | null>(null)
  const [goalRuns, setGoalRuns] = useStoreState<
    Array<{
      id: string
      taskId: string
      status: string
      error?: string
      startedAt: string
      finishedAt?: string
    }>
  >([])
  const [planDraft, setPlanDraft] = useStoreState<{
    taskId: string
    goal: string
    completion: string
    steps: Array<{ id: string; title: string; status: string }>
  } | null>(null)
  const [sessionId, setSessionId] = useStoreState('')
  const [sessionOpen, setSessionOpen] = useStoreState(false)
  const [models, setModels] = useStoreState<string[]>([])
  const [modelCardOpen, setModelCardOpen] = useStoreState(false)
  const [moreOpen, setMoreOpen] = useStoreState(false)
  const [accessOpen, setAccessOpen] = useStoreState(false)
  const [editingIndex, setEditingIndex] = useStoreState(-1)
  // Keep the unsent composer text while temporarily replacing it with a
  // historical message for editing. Cancelling must restore this exact draft.
  const [editDraft, setEditDraft] = useStoreState('')
  // 斜杠命令：slashOpen 控制 / 菜单，slashDialog 是当前命令弹窗。
  const [slashOpen, setSlashOpen] = useStoreState(false)
  const [slashDialog, setSlashDialog] = useStoreState<
    '' | 'compress' | 'goal' | 'plan'
  >('')
  const [goalText, setGoalText] = useStoreState('')
  const [planText, setPlanText] = useStoreState('')
  const [filePickerOpen, setFilePickerOpen] = useStoreState(false)
  const [filePickerMode, setFilePickerMode] =
    useStoreState<PickerMode>('directory')
  const [fileExtensions, setFileExtensions] = useStoreState('ts, tsx')
  const [slashIndex, setSlashIndex] = useStoreState(-1)

  // Composer popovers are mutually exclusive. Keeping this in one place
  // prevents the menus from stacking over one another when switching tools.
  const toggleComposerMenu = (menu: 'more' | 'access' | 'model') => {
    if (menu === 'more') {
      setMoreOpen(current => {
        const next = !current
        if (next) {
          setAccessOpen(false)
          setModelCardOpen(false)
        }
        return next
      })
      return
    }
    if (menu === 'access') {
      setAccessOpen(current => {
        const next = !current
        if (next) {
          setMoreOpen(false)
          setModelCardOpen(false)
        }
        return next
      })
      return
    }
    setModelCardOpen(current => {
      const next = !current
      if (next) {
        setMoreOpen(false)
        setAccessOpen(false)
      }
      return next
    })
  }

  const closeComposerMenus = useCallback(() => {
    setMoreOpen(false)
    setAccessOpen(false)
    setModelCardOpen(false)
    setSlashOpen(false)
    setSlashIndex(-1)
  }, [
    setAccessOpen,
    setModelCardOpen,
    setMoreOpen,
    setSlashIndex,
    setSlashOpen
  ])

  // Composer popovers should behave like native menus: a click anywhere else
  // in the chat closes them. Capture pointerdown so the close happens even
  // when the outside target later stops click propagation.
  useEffect(() => {
    if (!moreOpen && !accessOpen && !modelCardOpen && !slashOpen) return
    const onPointerDown = (event: PointerEvent) => {
      if (composerRef.current?.contains(event.target as Node)) return
      closeComposerMenus()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeComposerMenus()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [accessOpen, closeComposerMenus, modelCardOpen, moreOpen, slashOpen])

  const current = providers.find(item => item.ID === provider)

  const loadModels = useCallback(
    async (id: string) => {
      try {
        const response = await fetch(
          `/api/v1/ai/models?provider=${encodeURIComponent(id)}`
        )
        if (!response.ok) return
        const data = (await response.json()) as { models?: string[] }
        const list = data.models ?? []
        setModels(list)
        // 若当前 model 不在该 provider 的模型列表里（例如旧配置残留了
        // 别的服务商模型），回退到列表第一个，避免把脏模型发给后端。
        if (list.length > 0) {
          setModel(currentModel => {
            const stale = !list.includes(currentModel)
            return stale ? list[0] : currentModel
          })
        }
      } catch {
        // 模型列表加载失败不阻塞
      }
    },
    [setModels, setModel]
  )

  const loadProviders = useCallback(async () => {
    const response = await fetch('/api/v1/ai/providers')
    const data = (await response.json()) as Provider[]
    setProviders(data)
    const item = data.find(value => value.HasKey) ?? data[0]
    if (item) {
      setProvider(item.ID)
      setModel(item.Model)
      setBaseURL(item.BaseURL)
      void loadModels(item.ID)
    }
  }, [loadModels, setProviders, setProvider, setModel, setBaseURL])
  useEffect(() => {
    void loadProviders()
  }, [loadProviders])

  useEffect(() => {
    threadRef.current?.scrollTo({ top: threadRef.current.scrollHeight })
  }, [messages, activity])

  const loadSessions = useCallback(async () => {
    try {
      const response = await fetch(`${AGENT_API_BASE}/api/v1/agent/sessions`)
      if (!response.ok) return
      const data = (await response.json()) as SessionMeta[]
      setSessions(data)
    } catch {
      // 列表加载失败不阻塞对话
    }
  }, [setSessions])
  useEffect(() => {
    void loadSessions()
  }, [loadSessions])

  const loadTasks = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/agent/tasks')
      if (response.ok) setTasks((await response.json()) as TaskMeta[])
    } catch {
      // Task status is auxiliary to the conversation view.
    }
  }, [setTasks])
  useEffect(() => {
    void loadTasks()
    const refresh = () => {
      if (document.visibilityState === 'visible') void loadTasks()
    }
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    window.addEventListener('alx:agent-task-changed', refresh)
    return () => {
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', refresh)
      window.removeEventListener('alx:agent-task-changed', refresh)
    }
  }, [loadTasks])
  const loadGoals = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/agent/goals')
      if (response.ok) setGoals((await response.json()) as GoalMeta[])
    } catch {
      /* auxiliary */
    }
  }, [setGoals])
  useEffect(() => {
    void loadGoals()
  }, [loadGoals])
  const loadOps = useCallback(async () => {
    try {
      const [incidents, todos, maintenance, policy, metrics] =
        await Promise.all([
          fetch('/api/v1/ops/incidents'),
          fetch('/api/v1/ops/todos'),
          fetch('/api/v1/ops/maintenance'),
          fetch(`/api/v1/ops/policy?root=${encodeURIComponent(root)}`),
          fetch('/api/v1/ops/metrics')
        ])
      if (incidents.ok)
        setOpsIncidents((await incidents.json()) as OpsIncident[])
      if (todos.ok) setOpsTodos((await todos.json()) as OpsTodo[])
      if (maintenance.ok)
        setOpsMaintenance((await maintenance.json()) as OpsMaintenance[])
      if (policy.ok) setOpsPolicy((await policy.json()) as OpsPolicyUI)
      if (metrics.ok) setOpsMetrics((await metrics.json()) as OpsMetrics)
    } catch {
      /* 运维面板是辅助信息，不阻塞对话 */
    }
  }, [
    root,
    setOpsIncidents,
    setOpsTodos,
    setOpsMaintenance,
    setOpsPolicy,
    setOpsMetrics
  ])
  useEffect(() => {
    void loadOps()
    const refresh = () => void loadOps()
    window.addEventListener('alx:ops-changed', refresh)
    return () => window.removeEventListener('alx:ops-changed', refresh)
  }, [loadOps])

  // When opened from the directory session tree, load that conversation.
  // Track the last loaded id so a repeated id is not re-fetched, while a new id
  // (switching records after starting a fresh session) loads every time.
  const lastLoadedSessionId = useRef('')
  useEffect(() => {
    if (
      !initialSessionId ||
      typeof initialSessionId !== 'string' ||
      initialSessionId === lastLoadedSessionId.current
    )
      return
    lastLoadedSessionId.current = initialSessionId
    void (async () => {
      try {
        const response = await fetch(
          `${AGENT_API_BASE}/api/v1/agent/sessions/${encodeURIComponent(initialSessionId)}`
        )
        if (!response.ok) {
          const error = (await response.json().catch(() => ({}))) as {
            error?: string
          }
          setNotice(error.error || '会话记录不存在或已损坏。')
          return
        }
        const data = (await response.json()) as {
          session: SessionMeta
          messages: Array<{ role: string; content: string }> | null
        }
        setSessionId(data.session.id)
        setMessages(sessionToMessages(data.messages ?? []))
        setActivity([])
      } catch {
        setNotice('加载会话失败。')
      }
    })()
  }, [initialSessionId, setSessionId, setMessages, setActivity, setNotice])

  const newSession = () => {
    if (busy) return
    setSessionId('')
    setMessages([])
    setActivity([])
    setDiagnostic(null)
    setSessionOpen(false)
    // 清除已加载记录，让之后点开同一记录仍能重新加载；并通知 Dashboard
    // 清空 agentSessionId，否则重开同一条记录时 prop 不变、不触发加载。
    lastLoadedSessionId.current = ''
    window.dispatchEvent(new CustomEvent('alx:agent-new-session'))
  }

  const openSession = async (id: string) => {
    if (busy || id === sessionId) return
    if (typeof id !== 'string' || !id.startsWith('s')) {
      console.warn('openSession 收到非法会话 ID：', id)
      setNotice('无法打开该对话记录。')
      return
    }
    try {
      const response = await fetch(
        `${AGENT_API_BASE}/api/v1/agent/sessions/${encodeURIComponent(id)}`
      )
      if (!response.ok) {
        const error = (await response.json().catch(() => ({}))) as {
          error?: string
        }
        setNotice(error.error || '会话记录不存在或已损坏。')
        return
      }
      const data = (await response.json()) as {
        session: SessionMeta
        messages: Array<{ role: string; content: string }> | null
      }
      setSessionId(data.session.id)
      setMessages(sessionToMessages(data.messages ?? []))
      setActivity([])
      setSessionOpen(false)
    } catch {
      setNotice('加载会话失败。')
    }
  }

  const deleteSession = async (id: string) => {
    if (busy) return
    if (typeof id !== 'string' || !id.startsWith('s')) {
      console.warn('deleteSession 收到非法会话 ID：', id)
      return
    }
    try {
      await fetch(
        `${AGENT_API_BASE}/api/v1/agent/sessions/${encodeURIComponent(id)}`,
        { method: 'DELETE' }
      )
      if (id === sessionId) newSession()
      await loadSessions()
    } catch {
      setNotice('删除会话失败。')
    }
  }

  const resumeTask = async (id: string) => {
    const response = await fetch(
      `/api/v1/agent/tasks/${encodeURIComponent(id)}/resume`,
      { method: 'POST' }
    )
    if (!response.ok) setNotice('任务无法恢复。')
    await loadTasks()
  }

  const approveTask = async (id: string) => {
    const response = await fetch(
      `${AGENT_TASKS_BASE}/${encodeURIComponent(id)}/plan/approve`,
      { method: 'POST' }
    )
    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as {
        error?: string
      }
      setNotice(body.error || '计划批准失败。')
      await loadTasks()
      return
    }
    setBusy(true)
    setNotice('计划已批准，正在连接任务执行进度…')
    void subscribeTaskEvents(id)
    await loadTasks()
  }
  const editPlan = (task: TaskMeta) => {
    if (task.plan)
      setPlanDraft({
        taskId: task.id,
        goal: task.plan.goal,
        completion: task.plan.completion,
        steps: task.plan.steps.map(step => ({ ...step }))
      })
  }
  const savePlan = async () => {
    if (!planDraft) return
    const response = await fetch(
      `/api/v1/agent/tasks/${encodeURIComponent(planDraft.taskId)}/plan`,
      {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          goal: planDraft.goal,
          completion: planDraft.completion,
          currentStep: 0,
          steps: planDraft.steps
        })
      }
    )
    setNotice(
      response.ok
        ? '计划已保存，任务将按更新后的计划继续执行。'
        : '计划保存失败。'
    )
    setPlanDraft(null)
    await loadTasks()
  }

  const rollbackTask = async (id: string) => {
    const response = await fetch(
      `/api/v1/agent/tasks/${encodeURIComponent(id)}/rollback`,
      { method: 'POST' }
    )
    if (!response.ok) setNotice('回滚失败：可能有文件被外部修改。')
    await loadTasks()
  }

  const mergeTask = async (id: string) => {
    const response = await fetch(
      `/api/v1/agent/tasks/${encodeURIComponent(id)}/merge`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ confirm: true })
      }
    )
    setNotice(
      response.ok
        ? 'worktree 修改已合并到主工作区。'
        : '合并失败，请先查看任务报告。'
    )
    await loadTasks()
  }

  const showTaskReport = async (id: string) => {
    const response = await fetch(
      `/api/v1/agent/tasks/${encodeURIComponent(id)}/report`
    )
    if (!response.ok) {
      setNotice('任务报告尚未生成。')
      return
    }
    const report = (await response.json()) as {
      summary?: string
      modifiedFiles?: string[]
    }
    setNotice(
      `${report.summary || '任务报告已生成。'}${report.modifiedFiles?.length ? ` 修改 ${report.modifiedFiles.length} 个文件。` : ''}`
    )
  }

  const createGoal = () =>
    setGoalDraft({
      title: '',
      prompt: '',
      root,
      provider,
      model,
      scheduleMinutes: 0,
      isolation: 'workspace'
    })
  const editGoal = (goal: GoalMeta) =>
    setGoalDraft({
      id: goal.id,
      title: goal.title,
      prompt: goal.prompt,
      root: goal.root,
      provider,
      model,
      scheduleMinutes: goal.scheduleMinutes || 0,
      isolation: 'workspace'
    })
  const saveGoal = async () => {
    if (!goalDraft?.prompt.trim() || !goalDraft.root.trim()) {
      setNotice('目标提示词和项目目录不能为空。')
      return
    }
    const url = goalDraft.id
      ? `/api/v1/agent/goals/${encodeURIComponent(goalDraft.id)}`
      : '/api/v1/agent/goals'
    const response = await fetch(url, {
      method: goalDraft.id ? 'PATCH' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...goalDraft, access: 'ask' })
    })
    if (!response.ok) {
      setNotice('目标保存失败。')
      return
    }
    setGoalDraft(null)
    await loadGoals()
    setNotice('长期目标已保存。')
  }
  const deleteGoal = async (id: string) => {
    if (!window.confirm('删除目标但保留运行历史？')) return
    await fetch(`/api/v1/agent/goals/${encodeURIComponent(id)}`, {
      method: 'DELETE'
    })
    await loadGoals()
  }
  const showGoalRuns = async (id: string) => {
    const response = await fetch(
      `/api/v1/agent/goals/${encodeURIComponent(id)}/runs`
    )
    if (response.ok) setGoalRuns((await response.json()) as typeof goalRuns)
    setNotice('已加载目标运行历史。')
  }
  const runGoal = async (id: string) => {
    await fetch(`/api/v1/agent/goals/${encodeURIComponent(id)}/run`, {
      method: 'POST'
    })
    await loadGoals()
    await loadTasks()
    setNotice('长期目标已创建任务。')
  }
  const toggleGoal = async (goal: GoalMeta) => {
    await fetch(
      `/api/v1/agent/goals/${encodeURIComponent(goal.id)}/${goal.status === 'active' ? 'pause' : 'resume'}`,
      { method: 'POST' }
    )
    await loadGoals()
  }
  const analyzeOps = async (id: string) => {
    await fetch(`/api/v1/ops/incidents/${encodeURIComponent(id)}/analyze`, {
      method: 'POST'
    })
    await loadOps()
    setNotice('已提交 运维分析。')
  }
  const createOpsTodo = async (id: string) => {
    await fetch(`/api/v1/ops/incidents/${encodeURIComponent(id)}/todo`, {
      method: 'POST'
    })
    await loadOps()
    setNotice('已加入运维待办。')
  }
  const toggleOpsMonitor = async () => {
    const next = !opsPaused
    await fetch(`/api/v1/ops/monitor/${next ? 'pause' : 'resume'}`, {
      method: 'POST'
    })
    setOpsPaused(next)
    setNotice(next ? '运维已暂停采集。' : '运维已恢复采集。')
  }
  const saveOpsPolicy = async () => {
    await fetch('/api/v1/ops/policy', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(opsPolicy)
    })
    setNotice('运维策略已保存。')
  }

  const handleEvent = useCallback(
    (event: {
      type: string
      text?: string
      tool?: string
      callId?: string
      output?: string
      diff?: {
        path: string
        mode?: string
        content?: string
        hunks?: Array<{ old: string; new: string }>
      } | null
    }) => {
      switch (event.type) {
        case 'tool':
          setActivity(value => [
            ...value,
            {
              id: activityId.current++,
              callId: event.callId || `event-${activityId.current}`,
              tool: event.tool || '',
              args: event.text || '',
              status: 'running'
            }
          ])
          break
        case 'result':
          setActivity(value =>
            value.map(item =>
              item.callId === event.callId && item.status === 'running'
                ? {
                    ...item,
                    status: event.output?.startsWith('错误') ? 'error' : 'done',
                    output: event.output
                  }
                : item
            )
          )
          break
        case 'text':
          setMessages(value => [
            ...value,
            { role: 'assistant', content: event.text || '' }
          ])
          break
        case 'done':
          setTimelineOpen(false)
          setMessages(value => [
            ...value,
            { role: 'assistant', content: event.text || '完成。' }
          ])
          setNotice('')
          void loadTasks()
          break
        case 'error':
          setTimelineOpen(false)
          setNotice(event.text || 'Agent 执行出错。')
          void loadTasks()
          break
        case 'plan':
          setNotice(event.text ? `当前步骤：${event.text}` : '任务计划已更新。')
          void loadTasks()
          break
        case 'review':
          try {
            const review = JSON.parse(event.text || '{}') as {
              goalSatisfied?: boolean
              summary?: string
            }
            setNotice(
              review.goalSatisfied
                ? 'Reviewer 已通过：' + (review.summary || '任务目标已满足。')
                : 'Reviewer 未通过：' + (review.summary || '请查看失败步骤。')
            )
          } catch {
            setNotice('Reviewer 已完成审查。')
          }
          break
        case 'confirm':
          setPendingConfirm({
            id: event.tool || '',
            tool: event.text || '',
            args: event.output || '',
            diff: event.diff ?? null
          })
          break
        case 'session':
          setSessionId(event.tool || '')
          void loadSessions()
          // 通知 Dashboard 刷新左侧"记录"里的会话列表，让新建的对话立即可见。
          window.dispatchEvent(new CustomEvent('alx:agent-session-created'))
          break
      }
    },
    [
      loadSessions,
      loadTasks,
      setActivity,
      setMessages,
      setNotice,
      setPendingConfirm,
      setSessionId,
      setTimelineOpen
    ]
  )

  // A plan-pending task has no SSE subscription yet. Once the user approves
  // it, reconnect from the durable event log so fast early events are replayed
  // rather than leaving the UI at “approved” with no visible execution.
  const subscribeTaskEvents = useCallback(
    async (taskID: string) => {
      streamRef.current?.abort()
      const controller = new AbortController()
      streamRef.current = controller
      try {
        const response = await fetch(
          `${AGENT_TASKS_BASE}/${encodeURIComponent(taskID)}/events`,
          {
            signal: controller.signal
          }
        )
        if (!response.ok || !response.body) {
          throw new Error('无法订阅 Agent 任务事件。')
        }
        const reader = response.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() ?? ''
          for (const line of lines) {
            const trimmed = line.trim()
            if (!trimmed.startsWith('data: ')) continue
            handleEvent(
              JSON.parse(trimmed.slice(6)) as Parameters<typeof handleEvent>[0]
            )
          }
        }
      } catch (reason) {
        if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
          setNotice(
            reason instanceof Error ? reason.message : '任务事件连接已断开。'
          )
        }
      } finally {
        if (streamRef.current === controller) streamRef.current = null
        setBusy(false)
        void loadTasks()
        void loadSessions()
      }
    },
    [handleEvent, loadSessions, loadTasks, setBusy, setNotice]
  )

  const send = async () => {
    if (!prompt.trim() || busy) return
    if (!current?.HasKey) {
      setSettings(true)
      setNotice('请先配置 AI 接口与 API Key。')
      return
    }
    // Editing replaces the last user message and drops its prior assistant
    // 斜杠命令优先：若输入是 /压缩 /目标 /计划，执行命令而非发送给 Agent。
    if (handleSlashExecution(prompt.trim())) {
      return
    }
    // reply, so the corrected prompt is re-sent in full.
    let next: ChatMessage[]
    if (editingIndex >= 0 && editingIndex < messages.length) {
      const prefix = messages.slice(0, editingIndex)
      const suffix = messages
        .slice(editingIndex + 1)
        .filter(item => item.role !== 'assistant')
      next = [...prefix, { role: 'user', content: prompt.trim() }, ...suffix]
    } else {
      next = [...messages, { role: 'user', content: prompt.trim() }]
    }
    setMessages(next)
    setPrompt('')
    setEditingIndex(-1)
    setEditDraft('')
    setActivity([])
    setTimelineOpen(false)
    setNotice('Agent 正在执行…')
    setBusy(true)
    if (/当前报错|最近的报错|修复报错/.test(prompt)) {
      void fetch('/api/v1/agent/diagnostics', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ root, error: prompt })
      })
        .then(async response => {
          if (response.ok)
            setDiagnostic(
              (await response.json()) as {
                kind?: string
                file?: string
                line?: number
                message?: string
                explanation?: string
              }
            )
        })
        .catch(() => undefined)
    }
    const controller = new AbortController()
    streamRef.current = controller
    try {
      const createResponse = await fetch(AGENT_TASKS_BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider,
          model,
          root,
          access,
          sessionId,
          messages: next
        }),
        signal: controller.signal
      })
      if (!createResponse.ok) {
        const data = (await createResponse.json().catch(() => ({}))) as {
          error?: string
        }
        throw new Error(data.error || 'Agent 请求失败。')
      }
      const created = (await createResponse.json()) as {
        taskId?: string
        sessionId?: string
        status?: string
      }
      if (!created.taskId) throw new Error('Agent 未返回任务 ID。')
      taskIdRef.current = created.taskId
      if (created.sessionId && created.sessionId !== sessionId)
        setSessionId(created.sessionId)
      if (created.status === 'plan_pending') {
        await loadTasks()
        setNotice('任务计划已生成，请批准后开始执行。')
        return
      }
      // The task starts before this request subscribes; SSE replays its
      // persisted events from the beginning so no tool/result update is lost.
      await subscribeTaskEvents(created.taskId)
    } catch (reason) {
      if (!(reason instanceof DOMException && reason.name === 'AbortError')) {
        const message =
          reason instanceof Error ? reason.message : 'Agent 请求失败。'
        console.error('[agent] 请求错误：', reason)
        setMessages(value => [
          ...value,
          { role: 'assistant', content: '⚠ ' + message }
        ])
        setNotice(message)
      }
    } finally {
      setBusy(false)
      streamRef.current = null
      taskIdRef.current = ''
    }
  }

  const cancel = () => {
    const taskId = taskIdRef.current
    if (taskId) {
      void fetch(`${AGENT_TASKS_BASE}/${encodeURIComponent(taskId)}/cancel`, {
        method: 'POST'
      })
    }
    streamRef.current?.abort()
    setBusy(false)
    setNotice('已停止本次执行。')
  }

  // 估算当前上下文占用（前端近似：消息总字符数 / 预算）。
  const contextUsage = useMemo(() => {
    const totalChars = messages.reduce((sum, m) => sum + m.content.length, 0)
    const budget = 120 * 1024
    const usage = Math.min(100, Math.round((totalChars / budget) * 100))
    return Math.max(1, usage)
  }, [messages])

  const slashCommands = useMemo(
    () => [
      {
        id: 'compress',
        label: '压缩',
        hint: `查看当前聊天上下文 ${contextUsage}%`
      },
      {
        id: 'goal',
        label: '目标',
        hint: '设置要持续的目标'
      },
      {
        id: 'plan',
        label: '计划',
        hint: '开始设定计划'
      }
    ],
    [contextUsage]
  )

  // 执行斜杠命令。选中命令先插入输入框，用户回车时才触发。
  const runSlashCommand = (command: (typeof slashCommands)[number]) => {
    setSlashOpen(false)
    setPrompt(`/${command.label}`)
    promptRef.current?.focus()
  }

  // 检测输入是否为斜杠命令并执行。
  const handleSlashExecution = (value: string): boolean => {
    const match = value.match(/^\/(压缩|目标|计划)(?:\s+(.+))?$/)
    if (!match) return false
    const [, command, arg] = match
    if (command === '压缩') {
      setNotice(`当前聊天上下文已使用约 ${contextUsage}%。`)
      setSlashDialog('compress')
    } else if (command === '目标') {
      setGoalText(arg ?? '')
      setSlashDialog('goal')
    } else if (command === '计划') {
      setPlanText(arg ?? '')
      setSlashDialog('plan')
    }
    setPrompt('')
    return true
  }

  const resolveConfirm = useCallback(
    async (approve: boolean) => {
      if (!pendingConfirm) return
      const id = pendingConfirm.id
      setPendingConfirm(null)
      try {
        const response = await fetch('/api/v1/agent/approve', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ confirmId: id, approve })
        })
        if (!response.ok) {
          const data = (await response.json().catch(() => ({}))) as {
            error?: string
          }
          setNotice(data.error || (approve ? '批准失败。' : '拒绝失败。'))
        }
      } catch {
        setNotice(approve ? '批准失败：网络错误。' : '拒绝失败：网络错误。')
      }
    },
    [pendingConfirm, setNotice, setPendingConfirm]
  )

  useEffect(() => {
    if (!pendingConfirm) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        void resolveConfirm(false)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [pendingConfirm, resolveConfirm])

  const saveSettings = async () => {
    const response = await fetch('/api/v1/ai/providers', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider, model, baseURL, apiKey })
    })
    const data = (await response.json()) as { error?: string }
    if (!response.ok) {
      setNotice(data.error || '保存失败。')
      return
    }
    setAPIKey('')
    await loadProviders()
    setSettings(false)
    setNotice('AI 接口已保存。')
  }

  const fillExample = (text: string) => {
    setPrompt(text)
    promptRef.current?.focus()
  }

  const resizePrompt = () => {
    const el = promptRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`
  }

  const changeProvider = (id: string) => {
    setProvider(id)
    setModel('')
    const item = providers.find(value => value.ID === id)
    if (item) {
      setBaseURL(item.BaseURL)
      void loadModels(id)
    }
  }

  let lastUserIndex = -1
  for (let index = messages.length - 1; index >= 0; index--) {
    if (messages[index].role === 'user') {
      lastUserIndex = index
      break
    }
  }

  const completedSteps = activity.filter(item => item.status === 'done').length
  const runningSteps = activity.filter(item => item.status === 'running').length
  const failedSteps = activity.filter(item => item.status === 'error').length
  const activitySummary = [
    `${activity.length} 个操作`,
    completedSteps > 0 && `完成 ${completedSteps}`,
    runningSteps > 0 && `进行中 ${runningSteps}`,
    failedSteps > 0 && `失败 ${failedSteps}`
  ]
    .filter(Boolean)
    .join(' · ')
  const activeTask = tasks.find(
    task => task.sessionId === sessionId && task.status === 'running'
  )
  const pendingTask = tasks.find(
    task => task.sessionId === sessionId && task.status === 'plan_pending'
  )

  return (
    <section className="relative flex min-w-0 w-full flex-col [container-name:agent-workspace] [container-type:inline-size]">
      <header className="sticky top-2.5 z-5 mx-2.5 mt-2.5 flex min-h-13 items-center justify-between rounded-[10px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-4 shadow-[0_4px_14px_var(--theme-shadow-soft)]">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex size-7.5 items-center justify-center rounded-[9px] border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)">
            <Sparkles className="size-4" />
          </span>
          <div className="grid min-w-0 gap-px">
            <span className="text-[0.84rem] font-semibold leading-tight text-(--theme-text-strong)">
              Agent
            </span>
            <span
              className="max-w-85 truncate text-[0.7rem] leading-tight text-(--theme-text-muted)"
              title={root}
            >
              {root}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            className="icon-button size-8 p-0"
            onClick={newSession}
            title="新增对话"
            aria-label="新增对话"
          >
            <Plus className="size-4" />
          </button>
          <button
            className="icon-button size-8 p-0"
            onClick={() => setSettings(value => !value)}
            title="AI 设置"
            aria-label="AI 设置"
          >
            <Settings2 className="size-4" />
          </button>
        </div>
      </header>

      <div className="agent-body block min-h-0 min-w-0 w-full has-[.agent-sessions]:grid has-[.agent-sessions]:grid-cols-[240px_minmax(0,1fr)] has-[.agent-sessions]:items-start max-[760px]:has-[.agent-sessions]:block @max-[720px]:has-[.agent-sessions]:block">
        <div className="agent-main relative flex min-h-0 min-w-0 w-full flex-[1_1_auto] flex-col has-[.agent-question-rail]:grid has-[.agent-question-rail]:items-start has-[.agent-question-rail]:pl-0">
          <AgentQuestionRail messages={messages} threadRef={threadRef} />
          <section
            className="grid min-h-0 w-full justify-items-stretch gap-5 overflow-visible px-8 pb-3 pt-3.5"
            ref={threadRef}
          >
            {messages.length === 0 && !busy && (
              <div className="m-auto grid max-w-[440px] justify-items-center px-0 py-5 text-center">
                <h3 className="mt-3.5 mb-1.5 text-base font-semibold text-(--theme-text-strong)">
                  让 ALemonX 来搞定它
                </h3>
                <div className="mt-[18px] grid w-full grid-cols-2 gap-2">
                  {PROMPT_EXAMPLES.map(([label, text]) => (
                    <button
                      className="grid w-full justify-items-start gap-0.5 rounded-[11px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-3 py-2 text-left transition hover:border-(--theme-accent-soft-border) hover:bg-(--theme-surface-hover) active:translate-y-px"
                      key={label}
                      onClick={() => fillExample(text)}
                    >
                      <strong className="text-[0.84rem] font-semibold text-(--theme-text-strong)">
                        {label}
                      </strong>
                      <small className="text-[0.74rem] leading-normal text-(--theme-text-muted)">
                        {text}
                      </small>
                    </button>
                  ))}
                </div>
              </div>
            )}
            {messages.map((item, index) =>
              item.role === 'user' ? (
                <article
                  className="group relative w-fit max-w-[78%] justify-self-end pb-7"
                  data-agent-message-index={index}
                  key={index}
                >
                  <div className="rounded-[10px_10px_3px_10px] border border-(--theme-border-default) bg-(--theme-surface-raised) px-[15px] py-[9px] text-[0.87rem] leading-[1.65] text-(--theme-text-strong) whitespace-pre-wrap">
                    {item.content}
                  </div>
                  {index === lastUserIndex && !busy && (
                    <button
                      className="absolute bottom-0 left-0 flex size-[22px] items-center justify-center rounded-md border border-(--theme-border-default) bg-(--theme-surface-panel) p-0 text-(--theme-text-muted) opacity-0 transition hover:border-(--theme-accent-soft-border) hover:text-(--theme-accent-text) focus-visible:opacity-100 group-hover:opacity-100"
                      onClick={() => {
                        if (editingIndex < 0) setEditDraft(prompt)
                        setEditingIndex(index)
                        setPrompt(item.content)
                        promptRef.current?.focus()
                      }}
                      title="编辑这条消息"
                      aria-label="编辑这条消息"
                    >
                      <Pencil className="size-3.5" />
                    </button>
                  )}
                </article>
              ) : (
                <AgentAssistantMessage
                  content={item.content}
                  key={index}
                  streaming={busy && index === messages.length - 1}
                />
              )
            )}
            {diagnostic && (
              <div className="agent-plan-card agent-diagnostic-card">
                <strong>错误诊断</strong>
                <small>
                  {diagnostic.kind || 'unknown'} · {diagnostic.file || '未定位'}
                  {diagnostic.line ? `:${diagnostic.line}` : ''}
                </small>
                <p>
                  {diagnostic.message || '已收集错误上下文，Agent 将继续分析。'}
                </p>
                {diagnostic.explanation && (
                  <small>{diagnostic.explanation}</small>
                )}
              </div>
            )}
            {(activeTask?.plan || pendingTask?.plan) && (
              <div className="grid w-full max-w-[min(960px,100%)] min-w-0 gap-1.5 justify-self-stretch rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-[11px_13px] shadow-[var(--theme-shadow-soft)]">
                <div className="flex items-center justify-between gap-2">
                  <strong className="text-xs leading-tight text-(--theme-text-strong)">
                    {pendingTask ? '执行计划' : '任务进度'}
                  </strong>
                  {pendingTask && (
                    <span className="rounded-full border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) px-2 py-0.5 text-[0.68rem] text-(--theme-accent-soft-text)">
                      待批准
                    </span>
                  )}
                </div>
                <p className="m-0 overflow-wrap-anywhere text-[0.78rem] leading-snug text-(--theme-text-primary)">
                  {(activeTask || pendingTask)?.plan?.goal}
                </p>
                <small className="text-[0.66rem] leading-tight text-(--theme-text-muted)">
                  完成条件 ·{' '}
                  {(activeTask || pendingTask)?.plan?.completion || '验证通过'}
                </small>
                <div className="mt-px grid min-w-0 gap-1">
                  {((activeTask || pendingTask)?.plan?.steps || []).map(
                    (step, index) => (
                      <span
                        className="grid min-w-0 grid-cols-[22px_minmax(0,1fr)_auto] items-center gap-2 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-raised) px-2 py-1.5 text-(--theme-text-secondary) data-[status=running]:border-(--theme-accent-soft-border) data-[status=running]:bg-(--theme-accent-soft) data-[status=failed]:border-(--theme-danger)"
                        key={step.id}
                        data-status={step.status}
                      >
                        <b className="flex size-5 items-center justify-center rounded-full border border-(--theme-border-subtle) bg-(--theme-surface-panel) text-[0.68rem]">
                          {index + 1}
                        </b>
                        <span className="min-w-0 overflow-wrap-anywhere">
                          {step.title}
                        </span>
                        <em className="whitespace-nowrap text-[0.66rem] not-italic text-(--theme-text-muted)">
                          {step.status === 'completed'
                            ? '已完成'
                            : step.status === 'running'
                              ? '进行中'
                              : step.status === 'failed'
                                ? '失败'
                                : '待执行'}
                        </em>
                      </span>
                    )
                  )}
                </div>
                {pendingTask && (
                  <div className="mt-0.5 flex flex-wrap items-center justify-end gap-1.5">
                    <button
                      className="rounded-md border border-(--theme-border-default) bg-transparent px-2.5 py-1.5 text-[0.72rem] leading-none text-(--theme-text-secondary)"
                      onClick={() => editPlan(pendingTask)}
                    >
                      编辑
                    </button>
                    <button
                      className="rounded-md border border-(--theme-accent) bg-(--theme-accent) px-2.5 py-1.5 text-[0.72rem] leading-none text-(--theme-on-accent)"
                      onClick={() => void approveTask(pendingTask.id)}
                    >
                      批准并开始
                    </button>
                  </div>
                )}
              </div>
            )}
            {activity.length > 0 && (
              <div className="mt-1 w-full max-w-[84%] justify-self-start">
                <button
                  className="flex w-full items-center justify-between rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-panel) px-2.5 py-1.5 text-xs text-(--theme-text-secondary) hover:bg-(--theme-surface-hover)"
                  onClick={() => setTimelineOpen(value => !value)}
                  aria-expanded={timelineOpen}
                  aria-controls="agent-activity-timeline"
                >
                  <span className="text-[0.72rem]">{activitySummary}</span>
                  <ChevronDown
                    className={cn(
                      'size-3.5 transition-transform',
                      timelineOpen && 'rotate-180'
                    )}
                  />
                </button>
                {timelineOpen && (
                  <div className="grid gap-0.5" id="agent-activity-timeline">
                    {activity.map((item, index) => (
                      <div
                        className="grid grid-cols-[30px_minmax(0,1fr)] gap-2.5"
                        data-status={item.status}
                        key={item.id}
                      >
                        <div className="flex flex-col items-center">
                          <span className="flex size-[30px] items-center justify-center rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-panel) text-(--theme-text-secondary) data-[status=running]:border-(--theme-accent-soft-border) data-[status=running]:bg-(--theme-accent-soft) data-[status=running]:text-(--theme-accent-text) data-[status=done]:border-(--theme-success) data-[status=done]:bg-(--theme-success-soft) data-[status=done]:text-(--theme-success) data-[status=error]:border-(--theme-danger) data-[status=error]:bg-(--theme-danger-soft) data-[status=error]:text-(--theme-danger)">
                            <ToolIcon name={item.tool} />
                          </span>
                          {index < activity.length - 1 && (
                            <span className="w-0.5 flex-1 bg-(--theme-border-default)" />
                          )}
                        </div>
                        <div className="min-w-0 pb-3">
                          <div className="flex min-h-[30px] items-start justify-between gap-2.5 pt-0.5">
                            <div className="grid min-w-0 gap-px">
                              <span className="text-[0.78rem] font-semibold leading-snug text-(--theme-text-secondary)">
                                {TOOL_LABEL[item.tool] || item.tool}
                              </span>
                              <small className="text-[0.7rem] leading-snug text-(--theme-text-faint)">
                                {TOOL_DESCRIPTION[item.tool]}
                              </small>
                            </div>
                            <span
                              className="inline-flex shrink-0 items-center gap-1 text-[0.72rem] font-medium text-(--theme-text-muted) data-[status=running]:text-(--theme-accent-text) data-[status=done]:text-(--theme-success) data-[status=error]:text-(--theme-danger)"
                              data-status={item.status}
                            >
                              <StepStatus status={item.status} />
                            </span>
                          </div>
                          {item.args && (
                            <div className="mt-2 rounded-[9px] border border-(--theme-border-subtle) bg-(--theme-surface-raised)">
                              <span className="block overflow-hidden px-2.5 py-1.5 text-[0.72rem] leading-relaxed text-(--theme-text-secondary) text-ellipsis whitespace-nowrap">
                                {formatToolArgs(item.tool, item.args)}
                              </span>
                            </div>
                          )}
                          {item.output && (
                            <details
                              className="mt-2 rounded-[9px] border border-(--theme-border-subtle) bg-(--theme-surface-code)"
                              open={item.status === 'error'}
                            >
                              <summary>
                                {item.status === 'error'
                                  ? '查看错误'
                                  : '查看结果'}
                              </summary>
                              <pre className="max-h-[200px] overflow-auto whitespace-pre-wrap break-all rounded-[0_0_9px_9px] px-2.5 py-2 text-[0.72rem] leading-relaxed text-(--theme-text-code)">
                                {item.output}
                              </pre>
                            </details>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
            {busy && activity.length === 0 && messages.length > 0 && (
              <div className="mt-0.5 flex items-center gap-2 justify-self-start text-[0.78rem] text-(--theme-text-muted)">
                <Loader2 className="spinner size-3.5 animate-spin" />
                <span>Agent 正在思考…</span>
              </div>
            )}
          </section>
          <footer
            className="bg-transparent px-4 pb-[18px] pt-2.5"
            ref={composerRef}
          >
            {editingIndex >= 0 && (
              <div className="mb-2 flex items-center gap-1.5 px-0.5 text-xs text-(--theme-text-muted)">
                <Pencil className="size-3" />
                正在编辑这条消息
                <button
                  onClick={() => {
                    setPrompt(editDraft)
                    setEditDraft('')
                    setEditingIndex(-1)
                    promptRef.current?.focus()
                  }}
                >
                  取消
                </button>
              </div>
            )}
            {notice && (
              <small className="mb-2 block px-0.5 text-xs text-(--theme-text-muted)">
                {notice}
              </small>
            )}
            <div className="relative rounded-[10px] border border-(--theme-border-strong) bg-(--theme-surface-input) p-2.5 pb-2 shadow-[var(--theme-shadow-soft)_0_2px_6px] transition focus-within:border-(--theme-accent) focus-within:shadow-[0_0_0_3px_var(--theme-accent-soft)]">
              <textarea
                ref={promptRef}
                value={prompt}
                onChange={event => {
                  const value = event.target.value
                  setPrompt(value)
                  resizePrompt()
                  // 输入 / 且是首字符时弹出命令菜单。
                  if (value === '/') setSlashOpen(true)
                  else if (!value.startsWith('/')) setSlashOpen(false)
                }}
                onKeyDown={event => {
                  if (slashOpen) {
                    if (event.key === 'ArrowDown') {
                      event.preventDefault()
                      setSlashIndex(current =>
                        current >= slashCommands.length - 1 ? 0 : current + 1
                      )
                      return
                    }
                    if (event.key === 'ArrowUp') {
                      event.preventDefault()
                      setSlashIndex(current =>
                        current <= 0 ? slashCommands.length - 1 : current - 1
                      )
                      return
                    }
                    if (event.key === 'Enter') {
                      event.preventDefault()
                      const selected =
                        slashIndex >= 0
                          ? slashCommands[slashIndex]
                          : slashCommands[0]
                      runSlashCommand(selected)
                      setSlashIndex(-1)
                      return
                    }
                    if (event.key === 'Tab') {
                      event.preventDefault()
                      const selected =
                        slashIndex >= 0
                          ? slashCommands[slashIndex]
                          : slashCommands[0]
                      runSlashCommand(selected)
                      setSlashIndex(-1)
                      return
                    }
                    if (event.key === 'Escape') {
                      setSlashOpen(false)
                      setSlashIndex(-1)
                      return
                    }
                  }
                  if (
                    event.key === 'Enter' &&
                    !event.shiftKey &&
                    !event.nativeEvent.isComposing
                  ) {
                    event.preventDefault()
                    void send()
                  }
                  if (event.key === 'Escape') setSlashOpen(false)
                }}
                className="block min-h-11 max-h-[180px] w-full resize-none overflow-y-auto border-0 bg-transparent text-[0.86rem] leading-[1.6] text-(--theme-text-primary) outline-none placeholder:text-(--theme-text-faint)"
                rows={1}
                aria-label="描述要交给 Agent 的任务"
                placeholder="描述任务，输入 / 查看命令，Enter 发送"
              />
              {slashOpen && (
                <div className="absolute bottom-[calc(100%+8px)] left-0 right-0 z-[60] grid min-w-[280px] gap-0.5 overflow-hidden rounded-[10px] border border-(--theme-border-default) bg-(--theme-surface-panel) p-1 shadow-[var(--theme-shadow-pop)]">
                  {slashCommands.map((command, index) => (
                    <button
                      className={cn(
                        'flex items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 py-2 text-left text-(--theme-text-primary) transition hover:bg-(--theme-surface-hover)',
                        index === slashIndex && 'bg-(--theme-accent-soft)'
                      )}
                      key={command.id}
                      onClick={() => {
                        runSlashCommand(command)
                        setSlashIndex(-1)
                      }}
                      onMouseEnter={() => setSlashIndex(index)}
                    >
                      <span className="min-w-14 text-xs font-semibold text-(--theme-accent-text)">
                        /{command.label}
                      </span>
                      <small className="text-[0.72rem] text-(--theme-text-muted)">
                        {command.hint}
                      </small>
                    </button>
                  ))}
                </div>
              )}
              <div className="mt-2 flex items-center justify-between gap-2">
                <div className="flex items-center gap-1.5">
                  <div className="relative">
                    <button
                      className="flex size-[30px] items-center justify-center rounded-lg border border-transparent text-(--theme-text-secondary) transition hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong) disabled:cursor-not-allowed disabled:opacity-35"
                      onClick={() => toggleComposerMenu('more')}
                      disabled={busy}
                      title="更多功能"
                      aria-label="更多功能"
                    >
                      <Plus className="size-4" />
                    </button>
                    {moreOpen && (
                      <div className="agent-composer-popup absolute bottom-[calc(100%+6px)] left-0 z-40 grid min-w-45 gap-0.5 rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-1.5 shadow-[var(--theme-shadow-pop)]">
                        <button
                          onClick={() => {
                            setFilePickerOpen(true)
                            setMoreOpen(false)
                          }}
                          disabled={busy}
                        >
                          <Folder className="size-3.5" />
                          选择目录或文件
                        </button>
                        <button
                          onClick={() => {
                            newSession()
                            setMoreOpen(false)
                          }}
                          disabled={busy}
                        >
                          <Plus className="size-3.5" />
                          新会话
                        </button>
                        <button
                          onClick={() => {
                            setPrompt('/目标 ')
                            setMoreOpen(false)
                            promptRef.current?.focus()
                          }}
                        >
                          <Target className="size-3.5" />
                          目标
                        </button>
                        <button
                          onClick={() => {
                            setPrompt('/计划 ')
                            setMoreOpen(false)
                            promptRef.current?.focus()
                          }}
                        >
                          <ListTodo className="size-3.5" />
                          计划
                        </button>
                        <button
                          onClick={() => {
                            setPrompt('/压缩')
                            setMoreOpen(false)
                            promptRef.current?.focus()
                          }}
                        >
                          <Minimize2 className="size-3.5" />
                          压缩
                        </button>
                        <button
                          onClick={() => {
                            setSettings(true)
                            setMoreOpen(false)
                          }}
                          disabled={busy}
                        >
                          <Settings2 className="size-3.5" />
                          AI 设置
                        </button>
                      </div>
                    )}
                  </div>
                  <div className="relative">
                    <button
                      className="flex size-[30px] items-center justify-center rounded-lg border border-transparent text-(--theme-text-secondary) transition hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong) disabled:cursor-not-allowed disabled:opacity-35"
                      onClick={() => toggleComposerMenu('access')}
                      disabled={busy}
                      title="权限模式"
                      aria-label="权限模式"
                    >
                      {access === 'full' ? (
                        <Unlock className="size-4" />
                      ) : access === 'auto' ? (
                        <ShieldCheck className="size-4" />
                      ) : (
                        <ShieldQuestion className="size-4" />
                      )}
                    </button>
                    {accessOpen && (
                      <div className="agent-composer-popup absolute bottom-[calc(100%+6px)] left-0 z-40 grid w-[230px] gap-0.5 rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-1.5 shadow-[var(--theme-shadow-pop)]">
                        {(
                          [
                            ['ask', '请求批准', '每次修改文件前都征求你的同意'],
                            [
                              'auto',
                              '帮我审批',
                              '自动批准文件修改，你只看到结果'
                            ],
                            ['full', '完全访问', '全部操作自动执行，不做确认']
                          ] as const
                        ).map(([id, label, desc]) => (
                          <button
                            className={access === id ? 'active' : ''}
                            key={id}
                            onClick={() => {
                              setAccess(id)
                              setAccessOpen(false)
                            }}
                            disabled={busy}
                          >
                            <span className="text-xs font-semibold">
                              {label}
                            </span>
                            <small>{desc}</small>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  <small className="text-[0.7rem] text-(--theme-text-faint) max-[760px]:hidden">
                    {access === 'ask'
                      ? '请求批准'
                      : access === 'auto'
                        ? '帮我审批'
                        : '完全访问'}
                  </small>
                </div>
                <div className="flex items-center gap-1.5">
                  <div className="relative">
                    <button
                      className="inline-flex h-[30px] max-w-[170px] items-center gap-1.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) px-2 text-[0.72rem] text-(--theme-text-secondary) transition hover:border-(--theme-border-strong) hover:bg-(--theme-surface-hover) disabled:cursor-not-allowed disabled:opacity-50"
                      onClick={() => toggleComposerMenu('model')}
                      disabled={busy}
                      title="选择 AI 服务与模型"
                    >
                      <Sparkles className="size-3.5" />
                      <span className="truncate">
                        {current?.Name || '选择服务'}
                      </span>
                      <ChevronDown className="size-3" />
                    </button>
                    {modelCardOpen && (
                      <div className="agent-composer-popup agent-model-card absolute bottom-[calc(100%+6px)] right-0 z-40 grid w-[250px] gap-1.5 rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-1.5 shadow-[var(--theme-shadow-pop)]">
                        <strong>服务商</strong>
                        <div className="flex flex-wrap gap-1">
                          {providers
                            .filter(item => item.HasKey)
                            .map(item => (
                              <button
                                className={cn(
                                  'rounded-md border px-2 py-1 text-[0.72rem] transition',
                                  provider === item.ID
                                    ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-soft-text)'
                                    : 'border-(--theme-border-default) bg-(--theme-surface-raised) text-(--theme-text-secondary) hover:border-(--theme-border-strong) hover:bg-(--theme-surface-hover)'
                                )}
                                key={item.ID}
                                onClick={() => changeProvider(item.ID)}
                              >
                                {item.Name}
                              </button>
                            ))}
                        </div>
                        <strong>模型</strong>
                        <div className="grid max-h-45 gap-0.5 overflow-auto">
                          {models.length === 0 ? (
                            <small>未获取到模型列表</small>
                          ) : (
                            models.map(item => (
                              <button
                                className={cn(
                                  'truncate rounded-md border-0 px-2 py-1.5 text-left text-[0.72rem]',
                                  model === item
                                    ? 'bg-(--theme-accent-soft) text-(--theme-accent-soft-text)'
                                    : 'text-(--theme-text-secondary) hover:bg-(--theme-surface-hover)'
                                )}
                                key={item}
                                onClick={() => setModel(item)}
                              >
                                {item}
                              </button>
                            ))
                          )}
                        </div>
                      </div>
                    )}
                  </div>
                  <button
                    className={
                      busy
                        ? 'flex size-[30px] items-center justify-center rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-hover) text-(--theme-text-strong) transition hover:border-(--theme-danger) hover:bg-(--theme-danger-soft) hover:text-(--theme-danger)'
                        : 'flex size-[30px] items-center justify-center rounded-[9px] bg-(--theme-accent) text-(--theme-accent-contrast) transition hover:-translate-y-px hover:bg-(--theme-accent-hover) disabled:cursor-not-allowed disabled:opacity-35'
                    }
                    disabled={!busy && !prompt.trim()}
                    onClick={busy ? () => cancel() : () => void send()}
                    aria-label={busy ? '停止' : '发送'}
                    title={busy ? '停止' : '发送'}
                  >
                    {busy ? (
                      <Square className="size-3.5" />
                    ) : (
                      <ArrowUp className="size-4" />
                    )}
                  </button>
                </div>
              </div>
            </div>
          </footer>
        </div>
      </div>

      <Modal
        open={settings}
        ariaLabel="AI 接口配置"
        onClose={() => setSettings(false)}
      >
        <section className="grid max-h-[calc(100vh-32px)] w-full max-w-[540px] content-start gap-4 overflow-auto rounded-[14px] border border-(--theme-border-default) bg-(--theme-surface-panel) p-6 shadow-[var(--theme-shadow-pop)]">
          <header className="flex items-start justify-between gap-3.5">
            <div>
              <h3 className="text-base font-semibold text-(--theme-text-strong)">
                AI 接口配置
              </h3>
            </div>
            <button
              className="icon-button size-8 p-0"
              onClick={() => setSettings(false)}
              aria-label="关闭设置"
              title="关闭"
            >
              <X className="size-4" />
            </button>
          </header>
          <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
            服务商
            <div className="flex flex-wrap gap-2">
              {providers.map(item => (
                <button
                  className={cn(
                    'rounded-[9px] border px-3 py-2 text-xs font-medium transition hover:bg-(--theme-surface-hover)',
                    provider === item.ID
                      ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-soft-text)'
                      : 'border-(--theme-border-default) text-(--theme-text-secondary)'
                  )}
                  key={item.ID}
                  onClick={() => {
                    setProvider(item.ID)
                    setBaseURL(item.BaseURL)
                    setAPIKey('')
                  }}
                >
                  {item.Name}
                  {item.HasKey && (
                    <small className="ml-1.5 text-[0.68rem] text-(--theme-success)">
                      已配置
                    </small>
                  )}
                </button>
              ))}
            </div>
          </label>
          <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
            接口地址
            <input
              value={baseURL}
              onChange={event => setBaseURL(event.target.value)}
              className="h-9 w-full rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-input) px-3 text-[0.84rem] font-normal text-(--theme-text-primary) outline-none focus:border-(--theme-accent) focus:shadow-[0_0_0_3px_var(--theme-accent-soft)]"
              placeholder="https://api.example.com/v1"
            />
          </label>
          <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
            API Key
            <input
              type="password"
              value={apiKey}
              onChange={event => setAPIKey(event.target.value)}
              className="h-9 w-full rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-input) px-3 text-[0.84rem] font-normal text-(--theme-text-primary) outline-none focus:border-(--theme-accent) focus:shadow-[0_0_0_3px_var(--theme-accent-soft)]"
              placeholder={
                current?.HasKey ? '重新填写以更新密钥' : '填写 API Key'
              }
            />
          </label>
          {PROVIDER_KEY_LINKS[provider] && (
            <a
              className="inline-flex items-center gap-1.5 justify-self-start text-xs font-semibold text-(--theme-accent-text) no-underline hover:underline"
              href={PROVIDER_KEY_LINKS[provider].href}
              target="_blank"
              rel="noreferrer"
            >
              {PROVIDER_KEY_LINKS[provider].label}
              <ExternalLink className="size-3" />
            </a>
          )}
          <footer className="flex justify-end gap-2 border-t border-(--theme-border-subtle) pt-4">
            <button
              className="secondary-button"
              onClick={() => setSettings(false)}
            >
              返回
            </button>
            <button
              className="primary-button"
              disabled={!apiKey}
              onClick={() => void saveSettings()}
            >
              保存
            </button>
          </footer>
        </section>
      </Modal>

      {sessionOpen && (
        <aside className="order-[-1] grid min-h-0 grid-rows-[auto_minmax(0,1fr)] border-r border-(--theme-border-default) bg-(--theme-surface-raised)">
          <div className="flex items-center justify-between px-3 py-2.5">
            <strong className="text-xs font-semibold text-(--theme-text-secondary)">
              会话
            </strong>
            <button
              className="icon-button size-7 p-0"
              onClick={() => newSession()}
              title="新会话"
              aria-label="新会话"
            >
              <Plus className="size-3.5" />
            </button>
          </div>
          <div className="grid min-h-0 content-start gap-1 overflow-auto px-2 pb-3">
            {sessions.length === 0 && (
              <p className="m-2 text-center text-xs text-(--theme-text-faint)">
                还没有会话记录。
              </p>
            )}
            {sessions.map(item => (
              <div
                className={
                  item.id === sessionId
                    ? 'group flex items-center gap-1 rounded-[9px] border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) p-0.5'
                    : 'group flex items-center gap-1 rounded-[9px] border border-transparent p-0.5 hover:bg-(--theme-surface-hover)'
                }
                key={item.id}
              >
                <button
                  className="grid min-w-0 flex-1 gap-0.5 bg-transparent px-2 py-1.5 text-left"
                  onClick={() => void openSession(item.id)}
                >
                  <span className="truncate text-xs font-medium text-(--theme-text-primary)">
                    {item.title}
                  </span>
                  <small className="text-[0.66rem] text-(--theme-text-faint)">
                    {item.status === 'running'
                      ? `执行中 · 第 ${item.turn ?? 0} 轮`
                      : item.status === 'failed'
                        ? '上次执行失败'
                        : formatUpdated(item.updated)}
                  </small>
                </button>
                <button
                  className="flex size-[26px] items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-faint) opacity-0 transition group-hover:opacity-100 hover:text-(--theme-danger)"
                  onClick={() => void deleteSession(item.id)}
                  title="删除会话"
                  aria-label="删除会话"
                >
                  <Trash2 className="size-3.5" />
                </button>
                {tasks
                  .filter(
                    task =>
                      task.sessionId === item.id &&
                      ['failed', 'paused', 'cancelled', 'completed'].includes(
                        task.status
                      )
                  )
                  .slice(0, 1)
                  .map(task => (
                    <span className="agent-session-actions" key={task.id}>
                      {['failed', 'paused', 'cancelled'].includes(
                        task.status
                      ) && (
                        <button
                          className="agent-session-action"
                          onClick={() => void resumeTask(task.id)}
                          title="从 checkpoint 恢复"
                        >
                          继续
                        </button>
                      )}
                      {task.status === 'completed' && (
                        <>
                          <button
                            className="agent-session-action"
                            onClick={() => void showTaskReport(task.id)}
                            title="查看任务报告"
                          >
                            报告
                          </button>
                          {task.isolation === 'worktree' && (
                            <button
                              className="agent-session-action"
                              onClick={() => void mergeTask(task.id)}
                              title="确认合并 worktree 修改"
                            >
                              合并
                            </button>
                          )}
                          <button
                            className="agent-session-action"
                            onClick={() => void rollbackTask(task.id)}
                            title="回滚 Agent 修改"
                          >
                            回滚
                          </button>
                        </>
                      )}
                      {task.status === 'failed' && (
                        <button
                          className="agent-session-action"
                          onClick={() => void showTaskReport(task.id)}
                          title="查看失败报告"
                        >
                          报告
                        </button>
                      )}
                    </span>
                  ))}
              </div>
            ))}
          </div>
          <div className="grid gap-2 border-t border-(--theme-border-subtle) p-2">
            <div className="flex items-center justify-between gap-2">
              <strong className="text-xs text-(--theme-text-secondary)">
                长期目标
              </strong>
              <button
                className="rounded-md border border-(--theme-border-default) bg-transparent px-2 py-1 text-[0.68rem] text-(--theme-text-secondary)"
                onClick={() => void createGoal()}
              >
                新建
              </button>
            </div>
            {goals.map(goal => (
              <div
                className="grid gap-1 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-panel) p-2"
                key={goal.id}
              >
                <span className="text-xs text-(--theme-text-primary)">
                  {goal.title || goal.prompt}
                </span>
                <small className="text-[0.66rem] text-(--theme-text-faint)">
                  {goal.status}
                  {goal.lastError ? ` · ${goal.lastError}` : ''}
                </small>
                <div className="flex flex-wrap gap-1">
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void runGoal(goal.id)}
                  >
                    运行
                  </button>
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void toggleGoal(goal)}
                  >
                    {goal.status === 'active' ? '暂停' : '恢复'}
                  </button>
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => editGoal(goal)}
                  >
                    编辑
                  </button>
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void showGoalRuns(goal.id)}
                  >
                    历史
                  </button>
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void deleteGoal(goal.id)}
                  >
                    删除
                  </button>
                </div>
              </div>
            ))}
          </div>
          <div className="grid gap-2 border-t border-(--theme-border-subtle) p-2">
            <div className="flex items-center justify-between gap-2">
              <strong className="text-xs text-(--theme-text-secondary)">
                运维
              </strong>
              <small>
                {opsPaused ? '已暂停' : '运行中'} · {opsTodos.length} 个待办
              </small>
              <button
                className="rounded-md border border-(--theme-border-default) bg-transparent px-2 py-1 text-[0.68rem] text-(--theme-text-secondary)"
                onClick={() => void toggleOpsMonitor()}
              >
                {opsPaused ? '恢复' : '暂停'}
              </button>
            </div>
            {opsMetrics && (
              <small>
                事件 {opsMetrics.incidents} · 已恢复 {opsMetrics.resolved} ·
                回滚 {opsMetrics.rollbacks} · 平均{' '}
                {Math.round(opsMetrics.averageRecoverySecs)} 秒
              </small>
            )}
            {opsIncidents.slice(0, 5).map(incident => (
              <div
                className="grid gap-1 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-panel) p-2"
                key={incident.id}
              >
                <span className="text-xs text-(--theme-text-primary)">
                  {incident.processName} · {incident.severity}
                </span>
                <small className="text-[0.66rem] text-(--theme-text-faint)">
                  {incident.status} · {incident.occurrences} 次
                </small>
                <div className="flex flex-wrap gap-1">
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void analyzeOps(incident.id)}
                  >
                    分析
                  </button>
                  <button
                    className="rounded border border-(--theme-border-default) bg-transparent px-1.5 py-0.5 text-[0.66rem] text-(--theme-text-secondary)"
                    onClick={() => void createOpsTodo(incident.id)}
                  >
                    加入待办
                  </button>
                </div>
              </div>
            ))}
            {opsIncidents.length === 0 && (
              <p className="m-2 text-center text-xs text-(--theme-text-faint)">
                暂无生产事件。
              </p>
            )}
            {opsMaintenance.length > 0 && (
              <small>
                最近维护：{opsMaintenance[0].status}
                {opsMaintenance[0].rollbackPerformed ? ' · 已回滚' : ''}
              </small>
            )}
            <label className="agent-ops-policy">
              策略
              <select
                value={opsPolicy.mode}
                onChange={event =>
                  setOpsPolicy({
                    ...opsPolicy,
                    mode: event.target.value as OpsPolicyUI['mode']
                  })
                }
              >
                <option value="observe">观察</option>
                <option value="auto">自动维护</option>
                <option value="strict">严格确认</option>
                <option value="off">关闭</option>
              </select>
            </label>
            <label className="agent-ops-policy">
              <input
                type="checkbox"
                checked={opsPolicy.autoAllowed}
                onChange={event =>
                  setOpsPolicy({
                    ...opsPolicy,
                    autoAllowed: event.target.checked
                  })
                }
              />
              项目加入自动维护白名单
            </label>
            <label className="agent-ops-policy">
              <input
                type="checkbox"
                checked={opsPolicy.allowCodeChanges}
                onChange={event =>
                  setOpsPolicy({
                    ...opsPolicy,
                    allowCodeChanges: event.target.checked
                  })
                }
              />
              允许代码修改
            </label>
            <label className="agent-ops-policy">
              <input
                type="checkbox"
                checked={opsPolicy.allowPm2Control}
                onChange={event =>
                  setOpsPolicy({
                    ...opsPolicy,
                    allowPm2Control: event.target.checked
                  })
                }
              />
              允许 PM2 控制
            </label>
            <button
              className="agent-session-action"
              onClick={() => void saveOpsPolicy()}
            >
              保存策略
            </button>
          </div>
        </aside>
      )}

      {goalDraft && (
        <Modal open ariaLabel="长期目标" className="agent-confirm-overlay">
          <div className="agent-confirm-dialog" role="dialog" aria-modal="true">
            <header>
              <strong>{goalDraft.id ? '编辑长期目标' : '新建长期目标'}</strong>
            </header>
            <label>
              名称
              <input
                value={goalDraft.title}
                onChange={event =>
                  setGoalDraft({ ...goalDraft, title: event.target.value })
                }
              />
            </label>
            <label>
              提示词
              <textarea
                value={goalDraft.prompt}
                onChange={event =>
                  setGoalDraft({ ...goalDraft, prompt: event.target.value })
                }
              />
            </label>
            <label>
              项目目录
              <input
                value={goalDraft.root}
                onChange={event =>
                  setGoalDraft({ ...goalDraft, root: event.target.value })
                }
              />
            </label>
            <label>
              调度分钟数
              <input
                type="number"
                min="0"
                value={goalDraft.scheduleMinutes}
                onChange={event =>
                  setGoalDraft({
                    ...goalDraft,
                    scheduleMinutes: Math.max(
                      0,
                      Number(event.target.value) || 0
                    )
                  })
                }
              />
            </label>
            <div>
              <button onClick={() => setGoalDraft(null)}>取消</button>
              <button onClick={() => void saveGoal()}>保存</button>
            </div>
          </div>
        </Modal>
      )}
      {goalRuns.length > 0 && (
        <Modal open ariaLabel="目标运行历史" className="agent-confirm-overlay">
          <div className="agent-confirm-dialog" role="dialog" aria-modal="true">
            <header>
              <strong>目标运行历史</strong>
            </header>
            {goalRuns.map(run => (
              <p key={run.id}>
                {run.status} · {run.taskId}
                {run.error ? ` · ${run.error}` : ''}
              </p>
            ))}
            <button onClick={() => setGoalRuns([])}>关闭</button>
          </div>
        </Modal>
      )}
      {planDraft && (
        <Modal open ariaLabel="任务计划" className="agent-confirm-overlay">
          <div className="agent-confirm-dialog" role="dialog" aria-modal="true">
            <header>
              <strong>编辑任务计划</strong>
            </header>
            <label>
              目标
              <textarea
                value={planDraft.goal}
                onChange={event =>
                  setPlanDraft({ ...planDraft, goal: event.target.value })
                }
              />
            </label>
            <label>
              完成条件
              <textarea
                value={planDraft.completion}
                onChange={event =>
                  setPlanDraft({ ...planDraft, completion: event.target.value })
                }
              />
            </label>
            {planDraft.steps.map((step, index) => (
              <label key={step.id}>
                步骤 {index + 1}
                <input
                  value={step.title}
                  onChange={event =>
                    setPlanDraft({
                      ...planDraft,
                      steps: planDraft.steps.map((item, itemIndex) =>
                        itemIndex === index
                          ? { ...item, title: event.target.value }
                          : item
                      )
                    })
                  }
                />
              </label>
            ))}
            <div>
              <button onClick={() => setPlanDraft(null)}>取消</button>
              <button onClick={() => void savePlan()}>保存计划</button>
            </div>
          </div>
        </Modal>
      )}
      {pendingConfirm && (
        <Modal
          open
          ariaLabel="确认项目修改"
          className="items-center justify-center bg-(--theme-surface-overlay)"
        >
          <div
            className="w-full max-w-[460px] rounded-[14px] border border-(--theme-border-default) bg-(--theme-surface-panel) p-5 shadow-[var(--theme-shadow-pop)]"
            role="dialog"
            aria-modal="true"
            aria-labelledby="agent-confirm-title"
          >
            <header className="mb-2.5 flex items-center gap-2 text-sm text-(--theme-accent-text)">
              <Pencil className="size-4" />
              <strong id="agent-confirm-title">Agent 请求修改项目</strong>
            </header>
            <p className="mb-2.5 text-[0.82rem] leading-relaxed text-(--theme-text-secondary)">
              工具{' '}
              <code className="rounded bg-(--theme-surface-code) px-1 py-px font-mono text-[0.74rem] text-(--theme-text-code)">
                {pendingConfirm.tool}
              </code>{' '}
              想要修改项目文件。确认后才会写入。
            </p>
            {pendingConfirm.diff ? (
              <div className="mb-4 grid gap-2">
                <div className="flex items-center gap-2 font-mono text-xs font-semibold text-(--theme-text-secondary)">
                  {pendingConfirm.diff.path}
                  {pendingConfirm.diff.mode === 'create' && (
                    <em className="rounded bg-(--theme-accent-soft) px-1.5 py-px text-[0.66rem] not-italic text-(--theme-accent-soft-text)">
                      新建
                    </em>
                  )}
                  {pendingConfirm.diff.mode === 'delete' && (
                    <em className="rounded bg-(--theme-accent-soft) px-1.5 py-px text-[0.66rem] not-italic text-(--theme-accent-soft-text)">
                      删除
                    </em>
                  )}
                </div>
                {pendingConfirm.diff.mode === 'create' && (
                  <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg border-l-[3px] border-(--theme-success) bg-(--theme-success-soft) p-2 text-[0.7rem] leading-relaxed text-(--theme-success-text)">
                    {pendingConfirm.diff.content}
                  </pre>
                )}
                {pendingConfirm.diff.mode === 'delete' && (
                  <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg border-l-[3px] border-(--theme-danger) bg-(--theme-danger-soft) p-2 text-[0.7rem] leading-relaxed text-(--theme-danger-text)">
                    （将删除此文件）
                  </pre>
                )}
                {!pendingConfirm.diff.mode &&
                  (pendingConfirm.diff.hunks ?? []).map((hunk, index) => (
                    <div
                      className="overflow-hidden rounded-lg border border-(--theme-border-subtle)"
                      key={index}
                    >
                      <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-t-lg border-b border-(--theme-border-subtle) border-l-[3px] border-(--theme-danger) bg-(--theme-danger-soft) p-2 text-[0.7rem] leading-relaxed text-(--theme-danger-text)">
                        {hunk.old}
                      </pre>
                      <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-b-lg border-l-[3px] border-(--theme-success) bg-(--theme-success-soft) p-2 text-[0.7rem] leading-relaxed text-(--theme-success-text)">
                        {hunk.new}
                      </pre>
                    </div>
                  ))}
              </div>
            ) : (
              pendingConfirm.args && (
                <pre className="mb-4 max-h-45 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-raised) p-2.5 font-mono text-[0.72rem] leading-relaxed text-(--theme-text-secondary)">
                  {pendingConfirm.args}
                </pre>
              )
            )}
            <footer className="flex justify-end gap-2">
              <button
                className="secondary-button"
                onClick={() => void resolveConfirm(false)}
              >
                拒绝
              </button>
              <button
                className="primary-button"
                onClick={() => void resolveConfirm(true)}
              >
                批准修改
              </button>
            </footer>
          </div>
        </Modal>
      )}

      {slashDialog && (
        <Modal open ariaLabel="Agent 操作" className="agent-confirm-overlay">
          <div className="agent-confirm-dialog" role="dialog" aria-modal="true">
            <header>
              <Slash className="size-4" />
              <strong>
                {slashDialog === 'compress'
                  ? '压缩上下文'
                  : slashDialog === 'goal'
                    ? '设置持续目标'
                    : '开始设定计划'}
              </strong>
            </header>
            {slashDialog === 'compress' ? (
              <div className="grid gap-2.5">
                <div className="h-2 w-full overflow-hidden rounded-full bg-(--theme-surface-raised)">
                  <div
                    className="h-full rounded-full bg-(--theme-accent) transition-[width] duration-300"
                    style={{ width: `${contextUsage}%` }}
                  />
                </div>
                <p className="m-0 text-[0.78rem] leading-relaxed text-(--theme-text-secondary)">
                  当前聊天上下文已使用约 <strong>{contextUsage}%</strong>。
                  超过预算时会自动压缩较早的工具结果。
                </p>
              </div>
            ) : slashDialog === 'goal' ? (
              <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                目标描述
                <textarea
                  className="w-full resize-y rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-input) px-3 py-2 text-[0.8rem] leading-relaxed text-(--theme-text-primary) outline-none focus:border-(--theme-accent) focus:shadow-[0_0_0_3px_var(--theme-accent-soft)]"
                  rows={3}
                  value={goalText}
                  onChange={event => setGoalText(event.target.value)}
                  placeholder="例如：始终遵循项目约定，修改前先验证"
                />
              </label>
            ) : (
              <label className="grid gap-1.5 text-xs font-medium text-slate-600">
                计划要点
                <textarea
                  className="w-full resize-y rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-input) px-3 py-2 text-[0.8rem] leading-relaxed text-(--theme-text-primary) outline-none focus:border-(--theme-accent) focus:shadow-[0_0_0_3px_var(--theme-accent-soft)]"
                  rows={3}
                  value={planText}
                  onChange={event => setPlanText(event.target.value)}
                  placeholder="例如：1. 读取结构 2. 定位实现 3. 修改 4. 验证"
                />
              </label>
            )}
            <footer>
              <button
                className="secondary-button"
                onClick={() => setSlashDialog('')}
              >
                关闭
              </button>
              {slashDialog !== 'compress' && (
                <button
                  className="primary-button"
                  disabled={
                    slashDialog === 'goal' ? !goalText.trim() : !planText.trim()
                  }
                  onClick={() => {
                    if (slashDialog === 'goal' && goalText.trim()) {
                      setPrompt(`请始终遵循以下目标：${goalText.trim()}`)
                    } else if (slashDialog === 'plan' && planText.trim()) {
                      setPrompt(`请按以下计划执行：\n${planText.trim()}`)
                    }
                    setSlashDialog('')
                    setGoalText('')
                    setPlanText('')
                    promptRef.current?.focus()
                  }}
                >
                  确定
                </button>
              )}
            </footer>
          </div>
        </Modal>
      )}

      <DirectoryPicker
        open={filePickerOpen}
        multiple={false}
        priority
        includeFiles
        selectionMode={filePickerMode}
        extensions={fileExtensions}
        onModeChange={mode => {
          setFilePickerMode(mode)
        }}
        onExtensionsChange={setFileExtensions}
        onClose={() => setFilePickerOpen(false)}
        onSelect={paths => {
          const path = paths[0]
          if (!path) return
          const target =
            filePickerMode === 'extension'
              ? `目录 ${path} 中所有 ${fileExtensions.trim() || '匹配格式'} 文件`
              : filePickerMode === 'file'
                ? `文件：${path}`
                : `目录：${path}`
          setPrompt(`请先查看${target}，基于其内容理解后继续。`)
          setFilePickerOpen(false)
          promptRef.current?.focus()
        }}
      />
    </section>
  )
}
