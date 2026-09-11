import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowUp, ChevronDown, Clock3, Loader2, Plus, Settings2, ShieldCheck, ShieldQuestion, Sparkles, X } from 'lucide-react'
import cn from 'classnames'
import { AgentMarkdown } from './AgentMarkdown'

type Status = { version: string; ready: boolean; lastError?: string }
type Configuration = { configured: boolean; provider?: string; model?: string; credentialConfigured?: boolean }
type Approval = { id: string; sessionId: string; action: string; summary: string; expiresAt: string }
type Event = { type: string; sessionId?: string; status?: string; text?: string; tool?: string; approval?: Approval }
type Message = { role: 'user' | 'assistant' | 'thinking'; content: string; steps?: string[]; durationMs?: number }
type Props = { root: string }
const examples: Array<[string, string]> = [
  ['分析当前项目', '分析当前机器人项目的结构、运行状态和待处理风险。'],
  ['修复一个问题', '定位当前项目的错误，提出计划，并在批准后修复和验证。'],
  ['解释代码', '介绍这个机器人项目的入口、核心模块和配置方式。'],
  ['规划改动', '为我描述的需求制定可执行计划，先等待我确认再修改。']
]
const rootToken = (root: string) => btoa(unescape(encodeURIComponent(root))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')

// Intermediate planning and tool progress belong to a compact, per-turn
// disclosure. The final assistant response remains the primary full Markdown
// content below it.
function elapsedLabel(milliseconds?: number) {
  const seconds = Math.max(0, Math.round((milliseconds ?? 0) / 1000))
  if (seconds < 60) return `用时 ${seconds} 秒`
  return `用时 ${Math.floor(seconds / 60)} 分钟 ${seconds % 60} 秒`
}

function ThinkingProcess({ steps = [], durationMs }: { steps?: string[]; durationMs?: number }) {
  const [expanded, setExpanded] = useState(false)
  if (!steps.length) return null
  return <article className="w-full max-w-[84%] justify-self-start border-b border-(--theme-border-subtle) pb-2"><button className="flex min-w-48 appearance-none items-center justify-between gap-3 !border-0 !bg-transparent px-0 py-1.5 text-left text-xs text-(--theme-text-muted) !shadow-none outline-none transition hover:text-(--theme-text-secondary) focus-visible:ring-2 focus-visible:ring-(--theme-accent-soft-border)" onClick={() => setExpanded(value => !value)}><span className="flex items-center gap-1.5"><Clock3 className="size-3.5" />{elapsedLabel(durationMs)}</span><ChevronDown className={cn('size-3.5 transition-transform', expanded && 'rotate-180')} /></button>{expanded && <div className="mt-2 grid max-h-80 gap-3 overflow-y-auto rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-panel) p-3 pr-2">{steps.map((step, index) => <div className="border-l-2 border-(--theme-accent-soft-border) pl-2.5 text-[0.8rem] text-(--theme-text-muted)" key={`${index}-${step.slice(0, 24)}`}><AgentMarkdown content={step} /></div>)}</div>}</article>
}

export function DSHWorkspace({ root }: Props) {
  const [status, setStatus] = useState<Status | null>(null)
  const [model, setModel] = useState('deepseek-flash')
  const [apiKey, setAPIKey] = useState('')
	const [configuration, setConfiguration] = useState<Configuration>({ configured: false, credentialConfigured: false })
  const [prompt, setPrompt] = useState('')
  const [session, setSession] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [approvals, setApprovals] = useState<Approval[]>([])
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState('')
  const [settings, setSettings] = useState(false)
  const threadRef = useRef<HTMLElement | null>(null)
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
  const pendingFinalRef = useRef('')
  const processStepsRef = useRef<string[]>([])
  const turnStartedAtRef = useRef<number | null>(null)
  const runtime = `/api/v1/dsh/runtimes/${rootToken(root)}`
  const refresh = useCallback(async () => {
    try {
      const [statusResponse, configResponse] = await Promise.all([fetch(`${runtime}/status`), fetch(`${runtime}/config`)])
      if (statusResponse.ok) setStatus(await statusResponse.json() as Status)
      if (configResponse.ok) {
        const saved = await configResponse.json() as Configuration
        setConfiguration(saved)
        if (saved.model) setModel(saved.model)
      }
    } catch { setStatus({ version: '', ready: false, lastError: '无法连接 DSH 服务' }) }
  }, [runtime])
  useEffect(() => { void refresh(); const id = window.setInterval(() => void refresh(), 4000); return () => clearInterval(id) }, [refresh])
  useEffect(() => {
    if (!session) return
    const stream = new EventSource(`${runtime}/sessions/${encodeURIComponent(session)}/events`)
    stream.addEventListener('dsh', value => { try {
      const event = JSON.parse(String((value as MessageEvent).data)) as Event
      if (event.sessionId && event.sessionId !== session) return
      if (event.type === 'assistant/message' && event.text) {
        if (pendingFinalRef.current && pendingFinalRef.current !== event.text) processStepsRef.current.push(pendingFinalRef.current)
        pendingFinalRef.current = event.text
      } else if (event.tool) {
        const label = `正在使用受限工具：${event.tool}`
        if (processStepsRef.current.at(-1) !== label) processStepsRef.current.push(label)
      }
      if (event.approval) setApprovals(current => current.some(item => item.id === event.approval!.id) ? current : [...current, event.approval!])
      if (event.status === 'idle' || event.type === 'turn/end') {
        const final = pendingFinalRef.current
        const steps = processStepsRef.current.slice(0, 100)
        const durationMs = turnStartedAtRef.current === null ? 0 : Date.now() - turnStartedAtRef.current
        if (final || steps.length) setMessages(current => [...current, ...(steps.length ? [{ role: 'thinking' as const, content: '', steps, durationMs }] : []), ...(final ? [{ role: 'assistant' as const, content: final }] : [])])
        pendingFinalRef.current = ''
        processStepsRef.current = []
        turnStartedAtRef.current = null
        setBusy(false)
      }
    } catch { setNotice('运行事件无法解析，已等待状态同步。') } })
    return () => stream.close()
  }, [runtime, session])
  useEffect(() => { threadRef.current?.scrollTo({ top: threadRef.current.scrollHeight, behavior: 'smooth' }) }, [messages, busy])
  const configure = async () => {
    setBusy(true); setNotice('')
    try {
      const response = await fetch(`${runtime}/config`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ provider: 'deepseek-official', model, apiKey }) })
      const body = await response.json() as Status & { error?: string }
      if (!response.ok) throw new Error(body.error || 'DSH 启动失败')
      setStatus(body); setAPIKey(''); setConfiguration({ configured: true, provider: 'deepseek-official', model, credentialConfigured: true }); setSettings(false)
      const created = await fetch(`${runtime}/sessions`, { method: 'POST' })
      const createdBody = await created.json() as { sessionId?: string; error?: string }
      if (!created.ok || !createdBody.sessionId) throw new Error(createdBody.error || '无法创建 DSH 会话')
      setSession(createdBody.sessionId); setMessages([]); setNotice('模型已连接到受管 DSH runtime。')
    } catch (error) { setNotice(error instanceof Error ? error.message : 'DSH 启动失败') } finally { setBusy(false) }
  }
  const submit = async () => {
    const text = prompt.trim(); if (!text || !status?.ready || busy) return
    setBusy(true); setPrompt(''); setNotice(''); turnStartedAtRef.current = Date.now(); setMessages(current => [...current, { role: 'user', content: text }])
    try {
      const response = await fetch(`${runtime}/sessions/${encodeURIComponent(session)}/prompt`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text }) })
      const body = await response.json() as { messageId?: string; error?: string }
      if (!response.ok) throw new Error(body.error || '发送失败')
      setNotice(`已交给 DSH 处理 · ${body.messageId ?? session}`)
    } catch (error) { turnStartedAtRef.current = null; setBusy(false); setMessages(current => [...current, { role: 'assistant', content: '任务未能提交。请检查模型连接后重试。' }]); setNotice(error instanceof Error ? error.message : '发送失败') }
  }
  const decideApproval = async (approval: Approval, approve: boolean) => {
    try {
      const response = await fetch(`${runtime}/approvals/${encodeURIComponent(approval.id)}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ approve }) })
      const body = await response.json() as { error?: string }
      if (!response.ok) throw new Error(body.error || '审批未能提交')
      setApprovals(current => current.filter(item => item.id !== approval.id))
      setNotice(approve ? '已批准本次操作。' : '已拒绝本次操作。')
    } catch (error) { setNotice(error instanceof Error ? error.message : '审批未能提交') }
  }
  const reset = async () => { if (!ready) return; const response = await fetch(`${runtime}/sessions`, { method: 'POST' }); const body = await response.json() as { sessionId?: string }; if (!response.ok || !body.sessionId) { setNotice('无法创建新会话。'); return }; setSession(body.sessionId); setMessages([]); setApprovals([]); pendingFinalRef.current = ''; processStepsRef.current = []; turnStartedAtRef.current = null; setNotice('已创建新会话。'); promptRef.current?.focus() }
  const ready = Boolean(status?.ready)
  return <section className="agent-workspace relative flex h-full min-h-0 min-w-0 w-full flex-1 flex-col overflow-hidden [container-name:agent-workspace] [container-type:inline-size]">
    <header className="agent-header sticky top-2.5 z-5 mx-2.5 mt-2.5 flex min-h-13 shrink-0 items-center justify-between rounded-[10px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-4 shadow-[0_4px_14px_var(--theme-shadow-soft)]">
      <div className="flex min-w-0 items-center gap-2.5"><span className="flex size-7.5 items-center justify-center rounded-[9px] border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)"><Sparkles className="size-4" /></span><div className="grid min-w-0 gap-px"><span className="text-[0.84rem] font-semibold leading-tight text-(--theme-text-strong)">Agent</span><span className="agent-header-root max-w-85 truncate text-[0.7rem] leading-tight text-(--theme-text-muted)" title={root}>{root || '请选择机器人目录'}</span></div></div>
      <div className="robot-panel-actions flex items-center gap-2"><span className="hidden rounded-full bg-(--theme-surface-raised) px-2 py-1 text-[0.68rem] text-(--theme-text-muted) sm:inline">{ready ? `DSH ${status?.version} 已就绪` : status?.lastError || '连接模型以开始'}</span><button className="icon-button size-9 p-0" onClick={reset} title="新增对话" aria-label="新增对话"><Plus className="size-4" /></button><button className="icon-button size-9 p-0" onClick={() => setSettings(value => !value)} title="AI 设置" aria-label="AI 设置"><Settings2 className="size-4" /></button></div>
    </header>
    <div className="agent-body min-h-0 min-w-0 w-full flex-1"><div className="agent-main relative flex h-full min-h-0 min-w-0 w-full flex-col">
      <section className="agent-thread grid min-h-0 w-full flex-1 content-start justify-items-stretch gap-5 overflow-y-auto px-8 pb-3 pt-3.5" ref={threadRef}>
        {!messages.length && !busy && <div className="m-auto grid max-w-[440px] justify-items-center px-0 py-5 text-center"><span className="flex size-10 items-center justify-center rounded-xl border border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)"><Sparkles className="size-5" /></span><h3 className="mt-3.5 mb-1.5 text-base font-semibold text-(--theme-text-strong)">让 ALemonX 来搞定它</h3><p className="m-0 text-xs leading-relaxed text-(--theme-text-muted)">DSH 在后台执行计划、工具和审批；交互仍留在你的工作台。</p><div className="mt-[18px] grid w-full grid-cols-2 gap-2">{examples.map(([label, text]) => <button className="grid w-full justify-items-start gap-0.5 rounded-[11px] border border-(--theme-border-default) bg-(--theme-surface-panel) px-3 py-2 text-left transition hover:border-(--theme-accent-soft-border) hover:bg-(--theme-surface-hover)" key={label} onClick={() => { setPrompt(text); promptRef.current?.focus() }}><strong className="text-[0.84rem] text-(--theme-text-strong)">{label}</strong><small className="text-[0.74rem] text-(--theme-text-muted)">{text}</small></button>)}</div></div>}
        {messages.map((message, index) => message.role === 'thinking' ? <ThinkingProcess key={index} steps={message.steps} durationMs={message.durationMs} /> : message.role === 'user' ? <article className="w-fit max-w-[78%] justify-self-end" key={index}><div className="rounded-[10px_10px_3px_10px] border border-(--theme-border-default) bg-(--theme-surface-raised) px-[15px] py-[9px] text-[0.87rem] leading-[1.65] text-(--theme-text-strong) whitespace-pre-wrap">{message.content}</div></article> : <article className="flex w-full max-w-[84%] items-start gap-2.5 justify-self-start" key={index}><span className="flex size-7.5 shrink-0 items-center justify-center rounded-[9px] border border-(--theme-border-default) bg-(--theme-surface-raised) text-(--theme-accent-text)"><Sparkles className="size-3.5" /></span><div className="min-w-0 flex-1"><AgentMarkdown content={message.content} /></div></article>)}
        {!!approvals.length && <div className="grid w-full max-w-[min(960px,100%)] gap-2 rounded-xl border border-(--theme-accent-soft-border) bg-(--theme-surface-panel) p-[11px_13px] shadow-[var(--theme-shadow-soft)]"><div className="flex items-center gap-2"><ShieldQuestion className="size-4 text-(--theme-accent-text)" /><strong className="text-xs text-(--theme-text-strong)">等待你的逐次批准</strong></div><div className="grid max-h-64 gap-2 overflow-y-auto pr-1">{approvals.map(approval => <div className="grid gap-2 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-raised) p-2.5" key={approval.id}><div><strong className="block text-xs text-(--theme-text-secondary)">{approval.summary}</strong><small className="text-[0.7rem] text-(--theme-text-faint)">{approval.action} · 截止 {new Date(approval.expiresAt).toLocaleTimeString()}</small></div><div className="flex gap-2"><button className="rounded-md border border-(--theme-border-default) px-2 py-1 text-xs text-(--theme-text-secondary)" onClick={() => void decideApproval(approval, false)}>拒绝</button><button className="rounded-md bg-(--theme-accent) px-2 py-1 text-xs text-(--theme-on-accent)" onClick={() => void decideApproval(approval, true)}>批准本次</button></div></div>)}</div></div>}
        {busy && <div className="flex items-center gap-2 justify-self-start text-[0.78rem] text-(--theme-text-muted)"><Loader2 className="spinner size-3.5 animate-spin" />ALemonX 正在处理…</div>}
      </section>
      <footer className="agent-composer shrink-0 bg-transparent px-4 pb-[18px] pt-2.5">{notice && <small className="mb-2 block px-0.5 text-xs text-(--theme-text-muted)">{notice}</small>}<div className="relative rounded-[10px] border border-(--theme-border-strong) bg-(--theme-surface-input) p-2.5 pb-2 shadow-[var(--theme-shadow-soft)_0_2px_6px] transition focus-within:border-(--theme-accent) focus-within:shadow-[0_0_0_3px_var(--theme-accent-soft)]"><textarea ref={promptRef} value={prompt} onChange={event => setPrompt(event.target.value)} onKeyDown={event => { if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); void submit() } }} disabled={!ready || busy} className="block min-h-11 max-h-[180px] w-full resize-none overflow-y-auto border-0 bg-transparent text-[0.86rem] leading-[1.6] text-(--theme-text-primary) outline-none placeholder:text-(--theme-text-faint) disabled:cursor-not-allowed disabled:opacity-60" rows={1} aria-label="描述要交给 Agent 的任务" placeholder={ready ? '描述任务，Enter 发送' : '请先在 AI 设置中连接模型'} /><div className="mt-2 flex items-center justify-between gap-2"><small className="text-[0.7rem] text-(--theme-text-faint)"><ShieldQuestion className="mr-1 inline size-3.5" />写操作需要批准</small><div className="flex items-center gap-1.5"><button className="inline-flex h-[30px] max-w-[170px] items-center gap-1.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) px-2 text-[0.72rem] text-(--theme-text-secondary)" onClick={() => setSettings(true)} title="AI 设置"><Sparkles className="size-3.5" /><span className="truncate">DeepSeek · {model}</span></button><button className={busy ? 'flex size-[30px] items-center justify-center rounded-[9px] border border-(--theme-border-strong) bg-(--theme-surface-hover) text-(--theme-text-strong)' : 'flex size-[30px] items-center justify-center rounded-[9px] bg-(--theme-accent) text-(--theme-accent-contrast) transition hover:-translate-y-px hover:bg-(--theme-accent-hover) disabled:cursor-not-allowed disabled:opacity-35'} disabled={busy || !prompt.trim()} onClick={() => void submit()} title={busy ? '正在处理（当前 DSH SDK 不支持逐会话取消）' : '发送'}>{busy ? <Loader2 className="size-3.5 animate-spin" /> : <ArrowUp className="size-4" />}</button></div></div></div></footer>
    </div></div>
    {settings && <aside className="absolute inset-y-0 right-0 z-20 flex w-full max-w-90 flex-col border-l border-(--theme-border-default) bg-(--theme-surface-panel) p-4 shadow-[var(--theme-shadow-pop)]"><header className="flex justify-between"><strong className="flex items-center gap-2 text-sm text-(--theme-text-strong)"><Settings2 className="size-4" />DSH 设置</strong><button className="icon-button size-8 p-0" onClick={() => setSettings(false)}><X className="size-4" /></button></header><p className="mt-4 text-xs leading-relaxed text-(--theme-text-muted)">模型只交给当前机器人目录的受管 sidecar；Docker 可从只读 Secret 自动读取密钥。</p><label className="mt-4 text-xs text-(--theme-text-secondary)">模型<input value={model} onChange={event => setModel(event.target.value)} className="mt-1 block w-full rounded-lg border border-(--theme-border-default) bg-(--theme-surface-input) p-2 text-sm" /></label><label className="mt-3 text-xs text-(--theme-text-secondary)">DeepSeek API Key<input type="password" autoComplete="off" value={apiKey} onChange={event => setAPIKey(event.target.value)} placeholder={configuration.credentialConfigured ? '已安全保存；留空即可沿用' : '首次连接时填写'} className="mt-1 block w-full rounded-lg border border-(--theme-border-default) bg-(--theme-surface-input) p-2 text-sm" /></label>{configuration.credentialConfigured && <small className="mt-1 block text-xs text-(--theme-success)">密钥已保存到系统钥匙串，不会回显或写入配置文件。</small>}<button onClick={() => void configure()} disabled={busy} className="mt-5 rounded-lg bg-(--theme-accent) px-3 py-2 text-sm text-(--theme-on-accent) disabled:opacity-40">{configuration.configured ? '使用已保存配置连接 DSH' : '连接 DSH'}</button><p className="mt-auto text-xs text-(--theme-text-muted)"><ShieldCheck className="mr-1 inline size-4 text-(--theme-success)" />DeepSeek Official · 每次写操作均需批准</p></aside>}
  </section>
}
