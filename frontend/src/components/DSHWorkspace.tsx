import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ArrowUp,
  Archive,
  ArchiveRestore,
  ChevronDown,
  Clock3,
  Folder,
  Loader2,
  ListTodo,
  MessageSquareMore,
  Minimize2,
  Plus,
  Settings2,
  ShieldCheck,
  ShieldQuestion,
  Sparkles,
  Target,
  Unlock,
  X
} from 'lucide-react'
import cn from 'classnames'
import { AgentMarkdown } from './AgentMarkdown'
import { DirectoryPicker } from './Dashboard'
import { dshCredentialPolicy, type DSHCredentialSource } from '../lib/dshCredentials'

type Status = { version: string; ready: boolean; lastError?: string }
type Configuration = {
  configured: boolean
  provider?: string
  model?: string
  credentialConfigured?: boolean
  credentialSource?: DSHCredentialSource
}
type Approval = {
  id: string
  sessionId: string
  action: string
  summary: string
  expiresAt: string
}
type Event = {
  id?: number
  error?: string
  type: string
  sessionId?: string
  status?: string
  text?: string
  tool?: string
  approval?: Approval
}
type Message = {
  role: 'user' | 'assistant' | 'thinking'
  content: string
  steps?: string[]
  durationMs?: number
}
type Access = 'ask' | 'auto' | 'full'
type Session = { id: string; createdAt: string; updatedAt: string; access?: Access; archived?: boolean }
type Props = { root: string }
const suggestedModels = ['deepseek-chat', 'deepseek-reasoner']
const examples: Array<[string, string]> = [
  ['分析当前项目', '分析当前机器人项目的结构、运行状态和待处理风险。'],
  ['修复一个问题', '定位当前项目的错误，提出计划，并在批准后修复和验证。'],
  ['解释代码', '介绍这个机器人项目的入口、核心模块和配置方式。'],
  ['规划改动', '为我描述的需求制定可执行计划，先等待我确认再修改。']
]
const rootToken = (root: string) =>
  btoa(unescape(encodeURIComponent(root)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '')

// Intermediate planning and tool progress belong to a compact, per-turn
// disclosure. The final assistant response remains the primary full Markdown
// content below it.
function elapsedLabel(milliseconds?: number) {
  if ((milliseconds ?? 0) < 1000) return '用时不足 1 秒'
  const seconds = Math.ceil((milliseconds ?? 0) / 1000)
  if (seconds < 60) return `用时 ${seconds} 秒`
  return `用时 ${Math.floor(seconds / 60)} 分钟 ${seconds % 60} 秒`
}

function ThinkingProcess({
  steps = [],
  durationMs,
  forceExpanded = false
}: {
  steps?: string[]
  durationMs?: number
  forceExpanded?: boolean
}) {
  const [expanded, setExpanded] = useState(forceExpanded)
  if (!steps.length) return null
  const visible = forceExpanded || expanded
  return (
    <article className="w-full max-w-[84%] justify-self-start border-b border-(--theme-border-subtle) pb-2">
      <button
        className="flex min-w-48 appearance-none items-center justify-between gap-3 !border-0 !bg-transparent px-0 py-1.5 text-left text-xs text-(--theme-text-muted) !shadow-none outline-none transition hover:text-(--theme-text-secondary) focus-visible:ring-2 focus-visible:ring-(--theme-accent-soft-border)"
        onClick={() => setExpanded(value => !value)}
      >
        <span className="flex items-center gap-1.5">
          <Clock3
            className={cn('size-3.5', forceExpanded && 'animate-pulse')}
          />
          {forceExpanded ? '正在处理' : elapsedLabel(durationMs)}
        </span>
        <ChevronDown
          className={cn(
            'size-3.5 transition-transform',
            visible && 'rotate-180'
          )}
        />
      </button>
      {visible && (
        <div className="mt-2 grid max-h-80 gap-3 overflow-y-auto rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-panel) p-3 pr-2">
          {steps.map((step, index) => (
            <div
              className="border-l-2 border-(--theme-accent-soft-border) pl-2.5 text-[0.8rem] text-(--theme-text-muted)"
              key={`${index}-${step.slice(0, 24)}`}
            >
              <AgentMarkdown content={step} />
            </div>
          ))}
        </div>
      )}
    </article>
  )
}

export function DSHWorkspace({ root }: Props) {
  const [status, setStatus] = useState<Status | null>(null)
  const [model, setModel] = useState('deepseek-flash')
  const [apiKey, setAPIKey] = useState('')
  const [configuration, setConfiguration] = useState<Configuration>({
    configured: false,
    credentialConfigured: false
  })
  const [prompt, setPrompt] = useState('')
  const [session, setSession] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [liveSteps, setLiveSteps] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [streamReady, setStreamReady] = useState(false)
  const [notice, setNotice] = useState('')
  const [settings, setSettings] = useState(false)
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const [sessions, setSessions] = useState<Session[]>([])
  const [showArchived, setShowArchived] = useState(false)
  const [access, setAccess] = useState<Access>('ask')
  const [accessOpen, setAccessOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  const [filePickerOpen, setFilePickerOpen] = useState(false)
  const [planMode, setPlanMode] = useState(false)
  const threadRef = useRef<HTMLElement | null>(null)
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
  const composerRef = useRef<HTMLDivElement | null>(null)
  const pendingFinalRef = useRef('')
  const processStepsRef = useRef<string[]>([])
  const turnStartedAtRef = useRef<number | null>(null)
  const sessionCreationRef = useRef(false)
  const sessionMessagesRef = useRef<Record<string, Message[]>>({})
  const runtime = `/api/v1/dsh/runtimes/${rootToken(root)}`
  const refresh = useCallback(async () => {
    try {
      const [statusResponse, configResponse] = await Promise.all([
        fetch(`${runtime}/status`),
        fetch(`${runtime}/config`)
      ])
      if (statusResponse.ok) setStatus((await statusResponse.json()) as Status)
      if (configResponse.ok) {
        const saved = (await configResponse.json()) as Configuration
        setConfiguration(saved)
        if (saved.model) setModel(saved.model)
      }
    } catch {
      setStatus({ version: '', ready: false, lastError: '无法连接 DSH 服务' })
    }
  }, [runtime])
  const loadSessions = useCallback(async () => {
    try {
      const response = await fetch(`${runtime}/sessions`)
      if (!response.ok) throw new Error('无法读取会话列表')
      const items = (await response.json()) as Session[]
      setSessions(items)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '无法读取会话列表')
    }
  }, [runtime])
  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => void refresh(), 4000)
    return () => clearInterval(id)
  }, [refresh])
  useEffect(() => {
    if (sessionsOpen) void loadSessions()
  }, [loadSessions, sessionsOpen])
  useEffect(() => {
    // A browser restart loses only its local selection. Reopen the most recent
    // active runtime-owned session first; create one only for a first-time
    // project (or when all prior sessions are archived).
    if (!status?.ready || session) return
    let active = true
    sessionCreationRef.current = true
    void (async () => {
      const listed = await fetch(`${runtime}/sessions`)
      if (!listed.ok) throw new Error('无法读取 DSH 会话列表')
      const items = (await listed.json()) as Session[]
      if (!active) return
      const latest = items.find(item => !item.archived)
      if (latest) {
        if (active) {
          setSessions(items)
          setSession(latest.id)
          setAccess(latest.access ?? 'ask')
          setNotice('已恢复最近会话。')
        }
        return
      }
      const response = await fetch(`${runtime}/sessions`, { method: 'POST' })
      const body = (await response.json()) as { sessionId?: string; error?: string }
      if (!response.ok || !body.sessionId) throw new Error(body.error || '无法创建 DSH 会话')
      if (active) {
        setSession(body.sessionId)
        setAccess('ask')
        setSessions(items)
        setNotice('已创建新的 DSH 会话。')
      }
    })()
      .catch(error => {
        if (active)
          setNotice(
            error instanceof Error ? error.message : '无法恢复 DSH 会话'
          )
      })
      .finally(() => {
        sessionCreationRef.current = false
      })
    return () => {
      active = false
      sessionCreationRef.current = false
    }
  }, [runtime, session, status?.ready])
  useEffect(() => {
    // A selected robot directory owns its own runtime and browser session.
    setStatus(null)
    setSession('')
    setStreamReady(false)
    setAccess('ask')
    setMessages([])
    setApprovals([])
    setLiveSteps([])
    pendingFinalRef.current = ''
    processStepsRef.current = []
    turnStartedAtRef.current = null
    sessionCreationRef.current = false
    sessionMessagesRef.current = {}
  }, [runtime])
  useEffect(() => {
    if (session) sessionMessagesRef.current[session] = messages
  }, [messages, session])
  useEffect(() => {
    if (!session) return
    setStreamReady(false)
    let active = true
    let lastID = 0
    let historyLoaded = false
    const stream = new EventSource(
      `${runtime}/sessions/${encodeURIComponent(session)}/events?live=1`
    )
    stream.addEventListener('ready', () => {
      if (historyLoaded) { if (active) setStreamReady(true); return }
      void (async () => {
        const response = await fetch(`${runtime}/sessions/${encodeURIComponent(session)}/history`)
        if (!response.ok) throw new Error('无法加载对话记录，请重新打开会话重试。')
        const history = await response.json() as { messages: Message[] }
        if (!active) return
        historyLoaded = true
        setMessages(history.messages ?? [])
        setStreamReady(true)
        setNotice('')
      })().catch(error => { if (active) setNotice(error instanceof Error ? error.message : '历史加载失败') })
    })
    stream.onerror = () => { if (active) setStreamReady(false) }
    stream.onopen = () => { if (active && lastID > 0) setStreamReady(true) }
    stream.addEventListener('dsh', value => {
      try {
        if (!active) return
        const event = JSON.parse(String((value as MessageEvent).data)) as Event
        if (event.id && event.id <= lastID) return
        lastID = event.id ?? lastID
        // The first connection deliberately skips old events. A session can
        // still receive a replay after a network reconnect, but only a turn
        // initiated in this browser may add a new process card.
        if (turnStartedAtRef.current === null) return
        const refreshLiveSteps = () =>
          setLiveSteps(
            [
              ...processStepsRef.current,
              ...(pendingFinalRef.current ? [pendingFinalRef.current] : [])
            ].slice(-100)
          )
        const appendStep = (step: string) => {
          if (processStepsRef.current.at(-1) === step) return
          processStepsRef.current = [...processStepsRef.current, step].slice(
            -100
          )
          refreshLiveSteps()
        }
        if (event.sessionId && event.sessionId !== session) return
        if (event.type === 'assistant/message' && event.text) {
          if (pendingFinalRef.current && pendingFinalRef.current !== event.text)
            appendStep(pendingFinalRef.current)
          pendingFinalRef.current = event.text
          refreshLiveSteps()
        } else if (event.type === 'tool/call' && event.tool) {
          const label = `正在使用受限工具：${event.tool}`
          appendStep(label)
        }
        if (event.approval)
          setApprovals(current =>
            current.some(item => item.id === event.approval!.id)
              ? current
              : [...current, event.approval!]
          )
        if (event.type === 'turn/end') {
          const final = pendingFinalRef.current
          const steps = processStepsRef.current.slice(0, 100)
          const durationMs =
            turnStartedAtRef.current === null
              ? 0
              : Date.now() - turnStartedAtRef.current
          const answer = event.error || final || '本次任务未返回文本回复，请重试。'
          if (answer || steps.length)
            setMessages(current => [
              ...current,
              ...(steps.length
                ? [
                    {
                      role: 'thinking' as const,
                      content: '',
                      steps,
                      durationMs
                    }
                  ]
                : []),
              { role: 'assistant' as const, content: answer }
            ])
          pendingFinalRef.current = ''
          processStepsRef.current = []
          setLiveSteps([])
          turnStartedAtRef.current = null
          setBusy(false)
        }
      } catch {
        setNotice('运行事件无法解析，已等待状态同步。')
      }
    })
    return () => { active = false; stream.close() }
  }, [runtime, session])
  useEffect(() => {
    threadRef.current?.scrollTo({
      top: threadRef.current.scrollHeight,
      behavior: 'smooth'
    })
  }, [messages, busy])
  useEffect(() => {
    if (!moreOpen && !accessOpen) return
    const close = (event: PointerEvent) => {
      if (!composerRef.current?.contains(event.target as Node)) {
        setMoreOpen(false)
        setAccessOpen(false)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setMoreOpen(false)
        setAccessOpen(false)
      }
    }
    document.addEventListener('pointerdown', close, true)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', close, true)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [accessOpen, moreOpen])
  const configure = async () => {
    sessionCreationRef.current = true
    setBusy(true)
    setNotice('')
    try {
      const response = await fetch(`${runtime}/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider: 'deepseek-official', model, apiKey: dshCredentialPolicy(configuration.credentialSource).readOnly ? '' : apiKey })
      })
      const body = (await response.json()) as Status & { error?: string }
      if (!response.ok) throw new Error(body.error || 'DSH 启动失败')
      setStatus(body)
      setAPIKey('')
      setConfiguration({
        ...configuration,
        configured: true,
        provider: 'deepseek-official',
        model,
        credentialConfigured: true
      })
      setSettings(false)
      const created = await fetch(`${runtime}/sessions`, { method: 'POST' })
      const createdBody = (await created.json()) as {
        sessionId?: string
        error?: string
      }
      if (!created.ok || !createdBody.sessionId)
        throw new Error(createdBody.error || '无法创建 DSH 会话')
      setSession(createdBody.sessionId)
      setAccess('ask')
      setMessages([])
      setNotice('模型已连接到受管 DSH runtime。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'DSH 启动失败')
    } finally {
      sessionCreationRef.current = false
      setBusy(false)
    }
  }
  const switchModel = async (nextModel: string) => {
    if (
      !nextModel ||
      nextModel === model ||
      busy ||
      !configuration.credentialConfigured
    )
      return
    setBusy(true)
    setNotice('')
    try {
      const response = await fetch(`${runtime}/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          provider: configuration.provider || 'deepseek-official',
          model: nextModel
        })
      })
      const body = (await response.json()) as Status & { error?: string }
      if (!response.ok) throw new Error(body.error || '切换模型失败')
      setModel(nextModel)
      setStatus(body)
      setConfiguration(current => ({
        ...current,
        configured: true,
        model: nextModel
      }))
      setNotice(`已切换到 ${nextModel}。`)
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '切换模型失败')
    } finally {
      setBusy(false)
    }
  }
  const submit = async () => {
    const text = prompt.trim()
    if (!text || !status?.ready || !session || !streamReady || busy) return
    const dshText = planMode
      ? `先只输出清晰、可验证的执行计划；在我明确批准前，不要执行任何写操作或项目命令。\n\n任务：${text}`
      : text
    setBusy(true)
    setPrompt('')
    setNotice('')
    turnStartedAtRef.current = Date.now()
    processStepsRef.current = []
    setLiveSteps([])
    setMessages(current => [...current, { role: 'user', content: text }])
    try {
      const response = await fetch(
        `${runtime}/sessions/${encodeURIComponent(session)}/prompt`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ text: dshText })
        }
      )
      const body = (await response.json()) as {
        messageId?: string
        error?: string
      }
      if (!response.ok) throw new Error(body.error || '发送失败')
      setNotice(`已交给 DSH 处理 · ${body.messageId ?? session}`)
    } catch (error) {
      turnStartedAtRef.current = null
      setBusy(false)
      setMessages(current => [
        ...current,
        { role: 'assistant', content: '任务未能提交。请检查模型连接后重试。' }
      ])
      setNotice(error instanceof Error ? error.message : '发送失败')
    }
  }
  const decideApproval = async (approval: Approval, approve: boolean) => {
    try {
      const response = await fetch(
        `${runtime}/approvals/${encodeURIComponent(approval.id)}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ approve })
        }
      )
      const body = (await response.json()) as { error?: string }
      if (!response.ok) throw new Error(body.error || '审批未能提交')
      setApprovals(current => current.filter(item => item.id !== approval.id))
      setNotice(approve ? '已批准本次操作。' : '已拒绝本次操作。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '审批未能提交')
    }
  }
  const reset = async () => {
    if (!ready) return
    const response = await fetch(`${runtime}/sessions`, { method: 'POST' })
    const body = (await response.json()) as { sessionId?: string }
    if (!response.ok || !body.sessionId) {
      setNotice('无法创建新会话。')
      return
    }
    setSession(body.sessionId)
    setAccess('ask')
    setMessages([])
    setApprovals([])
    pendingFinalRef.current = ''
    processStepsRef.current = []
    setLiveSteps([])
    turnStartedAtRef.current = null
    setNotice('已创建新会话。')
    void loadSessions()
    promptRef.current?.focus()
  }
  const selectSession = (nextSession: string) => {
    if (!nextSession || nextSession === session || busy) return
    if (session) sessionMessagesRef.current[session] = messages
    setSession(nextSession)
    setAccess(sessions.find(item => item.id === nextSession)?.access ?? 'ask')
    setMessages(sessionMessagesRef.current[nextSession] ?? [])
    setApprovals([])
    pendingFinalRef.current = ''
    processStepsRef.current = []
    setLiveSteps([])
    turnStartedAtRef.current = null
    setSessionsOpen(false)
    setNotice('已切换会话。')
  }
  const archiveSession = async (item: Session, archived: boolean) => {
    if (busy) return
    try {
      const response = await fetch(
        `${runtime}/sessions/${encodeURIComponent(item.id)}/archive`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ archived })
        }
      )
      const body = (await response.json()) as { error?: string }
      if (!response.ok) throw new Error(body.error || '会话归档失败')
      setSessions(current =>
        current.map(sessionItem =>
          sessionItem.id === item.id
            ? { ...sessionItem, archived }
            : sessionItem
        )
      )
      if (archived && item.id === session) {
        setSession('')
        setAccess('ask')
        setMessages([])
        setApprovals([])
      }
      setNotice(archived ? '会话已归档。' : '会话已恢复。')
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '会话归档失败')
    }
  }
  const changeAccess = async (nextAccess: Access) => {
    if (!session || busy || nextAccess === access) {
      setAccessOpen(false)
      return
    }
    const previous = access
    setAccess(nextAccess)
    setAccessOpen(false)
    try {
      const response = await fetch(
        `${runtime}/sessions/${encodeURIComponent(session)}/access`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ access: nextAccess })
        }
      )
      const body = (await response.json()) as { error?: string }
      if (!response.ok) throw new Error(body.error || '权限模式更新失败')
      setSessions(current =>
        current.map(item =>
          item.id === session ? { ...item, access: nextAccess } : item
        )
      )
      setNotice(
        nextAccess === 'ask'
          ? '已设为逐次批准。'
          : nextAccess === 'auto'
            ? '已设为帮我批准：文件修改将自动执行。'
            : '已设为完全访问：受管能力内的操作将自动执行。'
      )
    } catch (error) {
      setAccess(previous)
      setNotice(error instanceof Error ? error.message : '权限模式更新失败')
    }
  }
  const selectProjectPath = (paths: string[]) => {
    const path = paths[0]
    if (!path) return
    const base = root.replace(/[\\/]+$/, '')
    const separator = path.includes('\\') ? '\\' : '/'
    if (path !== base && !path.startsWith(`${base}${separator}`)) {
      setNotice('只能引用当前机器人项目内的目录或文件。')
      return
    }
    const relative = path === base ? '项目根目录' : path.slice(base.length + 1)
    setPrompt(`请先查看项目内的 ${relative}，基于其内容理解后继续。`)
    setFilePickerOpen(false)
    promptRef.current?.focus()
  }
  const ready = Boolean(status?.ready && session && streamReady)
  const availableModels =
    configuration.configured && configuration.credentialConfigured
      ? Array.from(
          new Set(
            [configuration.model, model, ...suggestedModels].filter(
              (item): item is string => Boolean(item)
            )
          )
        )
      : []
  const visibleSessions = sessions.filter(item => Boolean(item.archived) === showArchived)
  return (
    <section className="agent-workspace relative flex h-full min-h-0 min-w-0 w-full flex-1 flex-col overflow-hidden [container-name:agent-workspace] [container-type:inline-size]">
      <header className="agent-header sticky top-2.5 z-5 mx-2.5 mt-2.5 flex min-h-13 shrink-0 items-center justify-between rounded-[10px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-4 shadow-[0_4px_14px_var(--theme-shadow-soft)]">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex size-7.5 items-center justify-center rounded-[9px] border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)">
            <Sparkles className="size-4" />
          </span>
          <div className="grid min-w-0 gap-px">
            <span className="text-[0.84rem] font-semibold leading-tight text-(--theme-text-strong)">
              Agent
            </span>
            <span
              className="agent-header-root max-w-85 truncate text-[0.7rem] leading-tight text-(--theme-text-muted)"
              title={root}
            >
              {root || '请选择机器人目录'}
            </span>
          </div>
        </div>
        <div className="robot-panel-actions flex items-center gap-2">
          <span className="hidden rounded-full bg-(--theme-surface-raised) px-2 py-1 text-[0.68rem] text-(--theme-text-muted) sm:inline">
            {ready
              ? `DSH ${status?.version} 已就绪`
              : status?.lastError || '连接模型以开始'}
          </span>
          <button
            className="icon-button size-9 p-0"
            onClick={reset}
            title="新增对话"
            aria-label="新增对话"
          >
            <Plus className="size-4" />
          </button>
          <button
            className="icon-button size-9 p-0"
            onClick={() => {
              setSessionsOpen(value => !value)
              setSettings(false)
            }}
            title="会话列表"
            aria-label="会话列表"
          >
            <MessageSquareMore className="size-4" />
          </button>
          <button
            className="icon-button size-9 p-0"
            onClick={() => {
              setSettings(value => !value)
              setSessionsOpen(false)
            }}
            title="AI 设置"
            aria-label="AI 设置"
          >
            <Settings2 className="size-4" />
          </button>
        </div>
      </header>
      <div className="agent-body min-h-0 min-w-0 w-full flex-1">
        <div className="agent-main relative flex h-full min-h-0 min-w-0 w-full flex-col">
          <section
            className="agent-thread grid min-h-0 w-full flex-1 content-start justify-items-stretch gap-5 overflow-y-auto px-8 pb-3 pt-3.5"
            ref={threadRef}
          >
            {!messages.length && !busy && (
              <div className="m-auto grid max-w-[440px] justify-items-center px-0 py-5 text-center">
                <span className="flex size-10 items-center justify-center rounded-xl border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)">
                  <Sparkles className="size-5" />
                </span>
                <h3 className="mt-3.5 mb-1.5 text-base font-semibold text-(--theme-text-strong)">
                  让 ALemonX 来搞定它
                </h3>
                <div className="mt-[18px] grid w-full grid-cols-2 gap-2">
                  {examples.map(([label, text]) => (
                    <button
                      className="grid w-full justify-items-start gap-0.5 rounded-[11px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-3 py-2 text-left transition hover:border-(--theme-accent-soft-border) hover:bg-(--theme-surface-hover)"
                      key={label}
                      onClick={() => {
                        setPrompt(text)
                        promptRef.current?.focus()
                      }}
                    >
                      <strong className="text-[0.84rem] text-(--theme-text-strong)">
                        {label}
                      </strong>
                      <small className="text-[0.74rem] text-(--theme-text-muted)">
                        {text}
                      </small>
                    </button>
                  ))}
                </div>
              </div>
            )}
            {messages.map((message, index) =>
              message.role === 'thinking' ? (
                <ThinkingProcess
                  key={index}
                  steps={message.steps}
                  durationMs={message.durationMs}
                />
              ) : message.role === 'user' ? (
                <article
                  className="w-fit max-w-[78%] justify-self-end"
                  key={index}
                >
                  <div className="rounded-[10px_10px_3px_10px] border border-(--theme-border-default) bg-(--theme-surface-raised) px-[15px] py-[9px] text-[0.87rem] leading-[1.65] text-(--theme-text-strong) whitespace-pre-wrap">
                    {message.content}
                  </div>
                </article>
              ) : (
                <article
                  className="flex w-full max-w-[84%] items-start gap-2.5 justify-self-start"
                  key={index}
                >
                  <span className="flex my-4 size-7.5 shrink-0 items-center justify-center rounded-[9px] border border-(--theme-border-default) bg-(--theme-surface-raised) text-(--theme-accent-text)">
                    <Sparkles className="size-3.5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <AgentMarkdown content={message.content} />
                  </div>
                </article>
              )
            )}
            {busy && <ThinkingProcess steps={liveSteps} forceExpanded />}
            {!!approvals.length && (
              <div className="grid w-full max-w-[min(960px,100%)] gap-2 rounded-xl border border-(--theme-accent-soft-border) bg-(--theme-surface-panel) p-[11px_13px] shadow-[var(--theme-shadow-soft)]">
                <div className="flex items-center gap-2">
                  <ShieldQuestion className="size-4 text-(--theme-accent-text)" />
                  <strong className="text-xs text-(--theme-text-strong)">
                    等待你的逐次批准
                  </strong>
                </div>
                <div className="grid max-h-64 gap-2 overflow-y-auto pr-1">
                  {approvals.map(approval => (
                    <div
                      className="grid gap-2 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-raised) p-2.5"
                      key={approval.id}
                    >
                      <div>
                        <strong className="block text-xs text-(--theme-text-secondary)">
                          {approval.summary}
                        </strong>
                        <small className="text-[0.7rem] text-(--theme-text-faint)">
                          {approval.action} · 截止{' '}
                          {new Date(approval.expiresAt).toLocaleTimeString()}
                        </small>
                      </div>
                      <div className="flex gap-2">
                        <button
                          className="rounded-md border border-(--theme-border-default) px-2 py-1 text-xs text-(--theme-text-secondary)"
                          onClick={() => void decideApproval(approval, false)}
                        >
                          拒绝
                        </button>
                        <button
                          className="rounded-md bg-(--theme-accent) px-2 py-1 text-xs text-(--theme-on-accent)"
                          onClick={() => void decideApproval(approval, true)}
                        >
                          批准本次
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </section>
          <footer className="agent-composer shrink-0 bg-transparent px-4 pb-[18px] pt-2.5">
            {notice && (
              <small className="mb-2 block px-0.5 text-xs text-(--theme-text-muted)">
                {notice}
              </small>
            )}
            <div ref={composerRef} className="relative rounded-[10px] border border-(--theme-border-strong) bg-(--theme-surface-input) p-2.5 pb-2 shadow-[var(--theme-shadow-soft)_0_2px_6px] transition focus-within:border-(--theme-accent) focus-within:shadow-[0_0_0_3px_var(--theme-accent-soft)]">
              <textarea
                ref={promptRef}
                value={prompt}
                onChange={event => setPrompt(event.target.value)}
                onKeyDown={event => {
                  if (
                    event.key === 'Enter' &&
                    !event.shiftKey &&
                    !event.nativeEvent.isComposing
                  ) {
                    event.preventDefault()
                    void submit()
                  }
                }}
                disabled={!ready || busy}
                className="block min-h-11 max-h-[180px] w-full resize-none overflow-y-auto !border-0 !rounded-none !bg-transparent !p-0 !shadow-none text-[0.86rem] leading-[1.6] text-(--theme-text-primary) !outline-none focus:!outline-none focus-visible:!outline-none placeholder:text-(--theme-text-faint) disabled:cursor-not-allowed disabled:opacity-60"
                rows={1}
                aria-label="描述要交给 Agent 的任务"
                placeholder={
                  ready ? '描述任务，Enter 发送' : configuration.configured ? '正在恢复模型与会话连接…' : '请先在 AI 设置中连接模型'
                }
              />
              <div className="mt-2 flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-1.5">
                  <div className="relative">
                    <button
                      className="flex size-[30px] items-center justify-center rounded-lg border border-transparent text-(--theme-text-secondary) transition hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong) disabled:cursor-not-allowed disabled:opacity-35"
                      onClick={() => {
                        setMoreOpen(value => !value)
                        setAccessOpen(false)
                      }}
                      disabled={busy}
                      title="更多功能"
                      aria-label="更多功能"
                    >
                      <Plus className="size-4" />
                    </button>
                    {moreOpen && (
                      <div className="agent-composer-popup absolute bottom-[calc(100%+8px)] left-0 z-40 w-[250px] gap-1 rounded-[14px] p-2 shadow-[var(--theme-shadow-pop)]">
                        <button className="!min-h-10 !px-3 !py-2.5" onClick={() => { setFilePickerOpen(true); setMoreOpen(false) }} disabled={busy}>
                          <Folder className="size-4" />选择项目文件
                        </button>
                        <button className="!min-h-10 !px-3 !py-2.5" onClick={() => { void reset(); setMoreOpen(false) }} disabled={!ready || busy}>
                          <Plus className="size-4" />新会话
                        </button>
                        <button className={cn('!min-h-10 !px-3 !py-2.5', planMode && '!bg-(--theme-accent-soft) !text-(--theme-accent-text)')} onClick={() => { setPlanMode(value => !value); setMoreOpen(false); promptRef.current?.focus() }} disabled={busy}>
                          <ListTodo className="size-4" />{planMode ? '退出计划模式' : '计划模式'}
                        </button>
                        <button className="!min-h-10 !px-3 !py-2.5" onClick={() => { setPrompt('请将以下内容设为当前项目的目标，并先给出可验证的分步计划：'); setMoreOpen(false); promptRef.current?.focus() }} disabled={busy}>
                          <Target className="size-4" />目标
                        </button>
                        <button className="!min-h-10 !px-3 !py-2.5" onClick={() => { setPrompt('请压缩当前会话上下文：保留已经确认的目标、决定、改动、待办与风险，移除重复过程；随后等待我的下一条任务。'); setMoreOpen(false); promptRef.current?.focus() }} disabled={!session || busy}>
                          <Minimize2 className="size-4" />压缩上下文
                        </button>
                        <button className="!min-h-10 !px-3 !py-2.5" onClick={() => { setSettings(true); setMoreOpen(false) }} disabled={busy}>
                          <Settings2 className="size-4" />AI 设置
                        </button>
                      </div>
                    )}
                  </div>
                  <div className="relative">
                    <button
                      className="flex size-[30px] items-center justify-center rounded-lg border border-transparent text-(--theme-text-secondary) transition hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong) disabled:cursor-not-allowed disabled:opacity-35"
                      onClick={() => {
                        setAccessOpen(value => !value)
                        setMoreOpen(false)
                      }}
                      disabled={!session || busy}
                      title="权限模式"
                      aria-label="权限模式"
                    >
                      {access === 'full' ? <Unlock className="size-4" /> : access === 'auto' ? <ShieldCheck className="size-4" /> : <ShieldQuestion className="size-4" />}
                    </button>
                    {accessOpen && (
                      <div className="agent-composer-popup absolute bottom-[calc(100%+8px)] left-0 z-40 w-[340px] gap-1 rounded-[14px] p-2 shadow-[var(--theme-shadow-pop)]">
                        {([['ask', '请求批准', '每次写操作都征求你的同意'], ['auto', '帮我批准', '项目文件修改自动执行；命令和 PM2 仍会询问'], ['full', '完全访问', '仅受管项目能力内的操作自动执行']] as const).map(([id, label, description]) => (
                          <button className={cn('!items-start !px-3 !py-2.5', access === id && '!bg-(--theme-accent-soft) !text-(--theme-accent-text)')} key={id} onClick={() => void changeAccess(id)}>
                            {id === 'full' ? <Unlock className="mt-0.5 size-4 shrink-0" /> : id === 'auto' ? <ShieldCheck className="mt-0.5 size-4 shrink-0" /> : <ShieldQuestion className="mt-0.5 size-4 shrink-0" />}
                            <span className="grid min-w-0 gap-0.5"><strong className="text-[0.82rem] leading-tight">{label}</strong><small className="text-[0.72rem] leading-snug text-(--theme-text-muted)">{description}</small></span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  <small className="truncate text-[0.7rem] text-(--theme-text-faint) max-[760px]:hidden">
                    {planMode ? '计划模式 · ' : ''}{access === 'ask' ? '请求批准' : access === 'auto' ? '帮我批准' : '完全访问'}
                  </small>
                </div>
                <div className="flex items-center gap-1.5">
                  {availableModels.length ? (
                    <label
                      className="inline-flex h-[30px] max-w-[190px] items-center gap-1.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) px-2 text-[0.72rem] text-(--theme-text-secondary)"
                      title="切换模型"
                    >
                      <Sparkles className="size-3.5 shrink-0" />
                      <select
                        value={model}
                        disabled={busy}
                        onChange={event => void switchModel(event.target.value)}
                        className="min-w-0 flex-1 appearance-none !border-0 !bg-transparent !p-0 text-[0.72rem] text-(--theme-text-secondary) !outline-none disabled:cursor-not-allowed"
                      >
                        <option value={model}>{model}</option>
                        {availableModels
                          .filter(item => item !== model)
                          .map(item => (
                            <option key={item} value={item}>
                              {item}
                            </option>
                          ))}
                      </select>
                    </label>
                  ) : (
                    <button
                      className="inline-flex h-[30px] max-w-[170px] items-center gap-1.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) px-2 text-[0.72rem] text-(--theme-text-secondary)"
                      onClick={() => setSettings(true)}
                      title="配置模型"
                    >
                      <Settings2 className="size-3.5" />
                      <span className="truncate">配置模型</span>
                    </button>
                  )}
                  <button
                    className={
                      busy
                        ? 'flex size-[30px] items-center justify-center rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-hover) text-(--theme-text-strong)'
                        : 'flex size-[30px] items-center justify-center rounded-[9px] bg-(--theme-accent) text-(--theme-accent-contrast) transition hover:-translate-y-px hover:bg-(--theme-accent-hover) disabled:cursor-not-allowed disabled:opacity-35'
                    }
                    disabled={!ready || busy || !prompt.trim()}
                    onClick={() => void submit()}
                    title={
                      busy
                        ? '正在处理（当前 DSH SDK 不支持逐会话取消）'
                        : '发送'
                    }
                  >
                    {busy ? (
                      <Loader2 className="size-3.5 animate-spin" />
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
      {sessionsOpen && (
        <aside className="absolute inset-y-0 right-0 z-20 flex w-full max-w-90 flex-col border-l border-(--theme-border-default) bg-(--theme-surface-panel) p-4 shadow-[var(--theme-shadow-pop)]">
          <header className="flex items-center justify-between">
            <strong className="flex items-center gap-2 text-sm text-(--theme-text-strong)">
              <MessageSquareMore className="size-4" />
              会话列表
            </strong>
            <button
              className="icon-button size-8 p-0"
              onClick={() => setSessionsOpen(false)}
              title="关闭会话列表"
              aria-label="关闭会话列表"
            >
              <X className="size-4" />
            </button>
          </header>
          <div className="mt-3 flex rounded-lg bg-(--theme-surface-raised) p-1">
            <button className={cn('flex-1 rounded-md px-2 py-1.5 text-xs transition', !showArchived ? 'bg-(--theme-surface-panel) text-(--theme-text-strong) shadow-sm' : 'text-(--theme-text-muted)')} onClick={() => setShowArchived(false)}>活跃</button>
            <button className={cn('flex-1 rounded-md px-2 py-1.5 text-xs transition', showArchived ? 'bg-(--theme-surface-panel) text-(--theme-text-strong) shadow-sm' : 'text-(--theme-text-muted)')} onClick={() => setShowArchived(true)}>已归档</button>
          </div>
          <div className="mt-3 grid min-h-0 flex-1 content-start gap-1.5 overflow-y-auto pr-1">
            {visibleSessions.length ? (
              visibleSessions.map(item => (
                <div key={item.id} className={cn('flex items-center gap-1 rounded-lg border p-1 transition', item.id === session ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft)' : 'border-(--theme-border-subtle) bg-(--theme-surface-raised)')}>
                  <button disabled={busy || item.archived} onClick={() => selectSession(item.id)} className="grid min-w-0 flex-1 gap-0.5 rounded-md px-1.5 py-1.5 text-left disabled:cursor-default">
                    <span className="truncate text-xs font-medium text-(--theme-text-strong)">{item.id === session ? '当前会话' : item.archived ? '已归档会话' : '会话'}</span>
                    <span className="text-[0.68rem] text-(--theme-text-faint)">{new Date(item.updatedAt).toLocaleString()}</span>
                  </button>
                  <button className="icon-button size-8 shrink-0 p-0" disabled={busy} onClick={() => void archiveSession(item, !item.archived)} title={item.archived ? '恢复会话' : '归档会话'} aria-label={item.archived ? '恢复会话' : '归档会话'}>{item.archived ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}</button>
                </div>
              ))
            ) : (
              <p className="py-6 text-center text-xs text-(--theme-text-faint)">
                {showArchived ? '暂无已归档会话' : '暂无活跃会话'}
              </p>
            )}
          </div>
          <button
            onClick={() => void reset()}
            disabled={!ready || busy}
            className="mt-3 inline-flex items-center justify-center gap-1.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) px-3 py-2 text-sm text-(--theme-text-secondary) disabled:opacity-40"
          >
            <Plus className="size-4" />
            新建会话
          </button>
        </aside>
      )}
      {settings && (
        <aside className="absolute inset-y-0 right-0 z-20 flex w-full max-w-90 flex-col border-l border-(--theme-border-default) bg-(--theme-surface-panel) p-4 shadow-[var(--theme-shadow-pop)]">
          <header className="flex justify-between">
            <strong className="flex items-center gap-2 text-sm text-(--theme-text-strong)">
              <Settings2 className="size-4" />
              DSH 设置
            </strong>
            <button
              className="icon-button size-8 p-0"
              onClick={() => setSettings(false)}
            >
              <X className="size-4" />
            </button>
          </header>
          <label className="mt-4 text-xs text-(--theme-text-secondary)">
            模型
            <input
              value={model}
              onChange={event => setModel(event.target.value)}
              className="mt-1 block w-full rounded-lg border border-(--theme-border-default) bg-(--theme-surface-input) p-2 text-sm"
            />
          </label>
          <label className="mt-3 text-xs text-(--theme-text-secondary)">
            DeepSeek API Key
            <input
              type="password"
              autoComplete="off"
              value={apiKey}
              disabled={dshCredentialPolicy(configuration.credentialSource).readOnly}
              onChange={event => setAPIKey(event.target.value)}
              placeholder={dshCredentialPolicy(configuration.credentialSource, configuration.credentialConfigured).placeholder}
              className="mt-1 block w-full rounded-lg border border-(--theme-border-default) bg-(--theme-surface-input) p-2 text-sm"
            />
          </label>
          {dshCredentialPolicy(configuration.credentialSource, configuration.credentialConfigured).missing && (
            <small className="mt-1 block text-xs text-(--theme-text-muted)">
              DSH Secret 不可用，请在部署端挂载密钥文件并设置 ALX_DSH_SECRET_FILE；工作台其他功能仍可使用。
            </small>
          )}
          {configuration.credentialConfigured && (
            <small className="mt-1 block text-xs text-(--theme-success)">
              {dshCredentialPolicy(configuration.credentialSource).description}
            </small>
          )}
          <button
            onClick={() => void configure()}
            disabled={busy}
            className="mt-5 rounded-lg bg-(--theme-accent) px-3 py-2 text-sm text-(--theme-on-accent) disabled:opacity-40"
          >
            {configuration.configured ? '已有配置连接 DSH' : '连接 DSH'}
          </button>
          <p className="mt-auto text-xs text-(--theme-text-muted)">
            <ShieldCheck className="mr-1 inline size-4 text-(--theme-success)" />
            DeepSeek Official · 每次写操作均需批准
          </p>
        </aside>
      )}
      <DirectoryPicker
        open={filePickerOpen}
        title="选择项目文件"
        multiple={false}
        priority
        includeFiles
        selectionMode="file"
        onClose={() => setFilePickerOpen(false)}
        onSelect={selectProjectPath}
      />
    </section>
  )
}
