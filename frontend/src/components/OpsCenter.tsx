import { useCallback, useEffect, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { ConfirmDialog } from './ConfirmDialog'

type Incident = {
  id: string
  processName: string
  status: string
  severity: string
  occurrences: number
  fingerprint: string
  sample?: string
  decisionReason?: string
}
type Todo = { id: string; title: string; status: string; severity: string; reason: string }
type Maintenance = {
  id: string
  incidentId: string
  taskId?: string
  status: string
  error?: string
  rollbackPerformed?: boolean
}
type Metrics = {
  incidents: number
  openTodos: number
  maintenanceRuns: number
  autoFixSuccess: number
  rollbacks: number
  resolved: number
  averageRecoverySecs: number
}
type Policy = {
  projectRoot: string
  mode: 'off' | 'observe' | 'canary' | 'auto' | 'strict'
  autoAllowed: boolean
  allowCodeChanges: boolean
  allowPm2Control: boolean
  observationMinutes: number
  maxModifiedFiles: number
  maxPm2Actions: number
  verificationCommand?: string
}
type Audit = { id: string; actor: string; role: string; action: string; resource: string; result: string; created: string }
type AlertRecord = { id: string; severity: string; kind: string; message: string; status: string; updated: string }
type Lease = { key: string; ownerId: string; fencingToken: number; renewedAt: string; expiresAt: string }
type CanaryReadiness = {
  ready: boolean
  checks: Array<{ name: string; passed: boolean; detail: string }>
}

const METRIC_ITEMS: Array<{ key: keyof Metrics; label: string }> = [
  { key: 'incidents', label: '事件' },
  { key: 'openTodos', label: '待办' },
  { key: 'maintenanceRuns', label: '维护' },
  { key: 'resolved', label: '已恢复' },
  { key: 'rollbacks', label: '回滚' }
]

function toneClass(tone: BadgeTone) {
  const map = {
    danger: 'bg-(--theme-danger-soft) text-(--theme-danger-text)',
    warning: 'bg-(--theme-warning-soft) text-(--theme-warning-text)',
    success: 'bg-(--theme-success-soft) text-(--theme-success-text)',
    info: 'bg-(--theme-info-soft) text-(--theme-info-text)',
    neutral: 'bg-(--theme-accent-soft) text-(--theme-accent-soft-text)'
  }
  return map[tone]
}

function severityTone(severity: string) {
  const value = severity.toLowerCase()
  if (value === 'high' || value === 'critical' || value === 'severe')
    return 'danger' as const
  if (value === 'medium') return 'warning' as const
  if (value === 'low') return 'info' as const
  return 'neutral' as const
}

function statusTone(status: string) {
  const value = status.toLowerCase()
  if (value === 'resolved' || value === 'fixed' || value === 'completed')
    return 'success' as const
  if (value === 'failed' || value === 'error') return 'danger' as const
  if (value === 'observing' || value === 'verifying' || value === 'silenced')
    return 'info' as const
  if (value === 'queued') return 'neutral' as const
  return 'warning' as const
}

type BadgeTone = 'danger' | 'warning' | 'success' | 'info' | 'neutral'

function Badge({ text, tone }: { text: string; tone: BadgeTone }) {
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded px-1.5 py-0.5 text-[10px] font-medium ${toneClass(tone)}`}
    >
      {text}
    </span>
  )
}

const inputClass =
  'h-8 rounded-md border border-(--theme-border-strong) bg-(--theme-surface-input) px-2.5 text-xs text-(--theme-text-primary) outline-none transition focus:border-(--theme-accent) focus:ring-2 focus:ring-(--theme-accent-soft)'

export function OpsCenter({ root, onBack }: { root: string; onBack?: () => void }) {
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [todos, setTodos] = useState<Todo[]>([])
  const [maintenance, setMaintenance] = useState<Maintenance[]>([])
  const [metrics, setMetrics] = useState<Metrics | null>(null)
  const [audits, setAudits] = useState<Audit[]>([])
  const [alerts, setAlerts] = useState<AlertRecord[]>([])
  const [leases, setLeases] = useState<Lease[]>([])
  const [readiness, setReadiness] = useState<CanaryReadiness | null>(null)
  const [policy, setPolicy] = useState<Policy>({
    projectRoot: root,
    mode: 'observe',
    autoAllowed: false,
    allowCodeChanges: false,
    allowPm2Control: false,
    observationMinutes: 5,
    maxModifiedFiles: 10,
    maxPm2Actions: 3
  })
  const [paused, setPaused] = useState(false)
  const [busy, setBusy] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<Incident | null>(null)
  const [confirmDeleteTodo, setConfirmDeleteTodo] = useState<Todo | null>(null)
  const [operationReason, setOperationReason] = useState('')
  const load = useCallback(async () => {
    const [i, t, m, x, p, a, h, l, readinessResponse] = await Promise.all([
      fetch('/api/v1/ops/incidents'),
      fetch('/api/v1/ops/todos'),
      fetch('/api/v1/ops/maintenance'),
      fetch('/api/v1/ops/metrics'),
      fetch(`/api/v1/ops/policy?root=${encodeURIComponent(root)}`),
      fetch('/api/v1/ops/audit'),
      fetch('/api/v1/ops/alerts'),
      fetch('/api/v1/ops/leases'),
      fetch(`/api/v1/ops/canary-readiness?root=${encodeURIComponent(root)}`)
    ])
    if (i.ok) setIncidents((await i.json()) as Incident[])
    if (t.ok) setTodos((await t.json()) as Todo[])
    if (m.ok) setMaintenance((await m.json()) as Maintenance[])
    if (x.ok) setMetrics((await x.json()) as Metrics)
    if (p.ok) setPolicy((await p.json()) as Policy)
    if (a.ok) setAudits((await a.json()) as Audit[])
    if (h.ok) setAlerts((await h.json()) as AlertRecord[])
    if (l.ok) setLeases((await l.json()) as Lease[])
    if (readinessResponse.ok) setReadiness((await readinessResponse.json()) as CanaryReadiness)
  }, [root])
  useEffect(() => {
    void load()
    const refresh = () => void load()
    window.addEventListener('alx:ops-changed', refresh)
    return () => window.removeEventListener('alx:ops-changed', refresh)
  }, [load])
  const post = async (path: string, body?: unknown) => {
    setBusy(true)
    try {
      await fetch(path, {
        method: 'POST',
        headers: body ? { 'Content-Type': 'application/json' } : undefined,
        body: body ? JSON.stringify(body) : undefined
      })
      await load()
    } finally {
      setBusy(false)
    }
  }
  const removeIncident = async (incident: Incident) => {
    setBusy(true)
    try {
      await fetch(`/api/v1/ops/incidents/${incident.id}`, { method: 'DELETE' })
      await load()
    } finally {
      setBusy(false)
      setConfirmDelete(null)
    }
  }
  const completeTodo = async (item: Todo) => {
    setBusy(true)
    try {
      await fetch(`/api/v1/ops/todos/${item.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: 'done' })
      })
      await load()
    } finally {
      setBusy(false)
    }
  }
  const removeTodo = async (item: Todo) => {
    setBusy(true)
    try {
      await fetch(`/api/v1/ops/todos/${item.id}`, { method: 'DELETE' })
      await load()
    } finally {
      setBusy(false)
      setConfirmDeleteTodo(null)
    }
  }
  const savePolicy = async () => {
	setBusy(true)
	try {
	  await fetch('/api/v1/ops/policy', {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json', 'X-Operation-Reason': operationReason },
        body: JSON.stringify(policy)
      })
      await load()
    } finally {
      setBusy(false)
    }
  }
  return (
    <section
      className="workspace-content system-feature-page mx-auto max-w-215"
      aria-label="运维中心"
    >
      <header className="system-feature-header">
        <span className="system-feature-header-icon bg-brand-50 text-brand-600 dark:bg-brand-900/40 dark:text-brand-300">
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <strong className="block truncate text-sm font-semibold text-slate-900 dark:text-slate-100">
            {root}
          </strong>
          <span className="block truncate text-xs text-slate-400 dark:text-slate-500">
            {paused ? '监控已暂停' : '监控运行中'}
          </span>
        </div>
        <div className="flex shrink-0 gap-2">
          {onBack && (
            <button className="secondary-button" onClick={onBack}>
              返回运行
            </button>
          )}
          <button
            className="secondary-button"
            disabled={busy}
            onClick={() => {
              const next = !paused
              setPaused(next)
              void post(`/api/v1/ops/monitor/${next ? 'pause' : 'resume'}`)
            }}
          >
            {paused ? '恢复采集' : '暂停采集'}
          </button>
          <button
            className="danger-button"
            disabled={busy}
            onClick={() => setConfirmStop(true)}
          >
            紧急停止
          </button>
        </div>
      </header>
      {metrics && (
        <div className="mx-2.5 mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {METRIC_ITEMS.map(({ key, label }) => (
            <div
              className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-3"
              key={key}
            >
              <small className="block text-xs text-(--theme-text-muted)">
                {label}
              </small>
              <strong className="block text-lg text-(--theme-text-strong)">
                {metrics[key]}
              </strong>
            </div>
          ))}
        </div>
      )}
      <div className="mx-2.5 mb-4 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-3 text-xs text-(--theme-text-muted)">
        <div className="mb-2 flex items-center justify-between">
          <strong className="text-(--theme-text-strong)">执行租约</strong>
          <span>{leases.length ? `${leases.length} 个活动租约` : '暂无租约'}</span>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          {leases.slice(0, 4).map(lease => (
            <div className="rounded border border-(--theme-border-subtle) px-2 py-1.5" key={lease.key}>
              <div className="flex items-center justify-between gap-2">
                <span className="truncate">{lease.key}</span>
                <Badge text={`fencing ${lease.fencingToken}`} tone="info" />
              </div>
              <span className="block truncate text-[10px]">{lease.ownerId} · {lease.expiresAt ? new Date(lease.expiresAt).toLocaleTimeString() : '未知过期时间'}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="mx-2.5 grid gap-4 lg:grid-cols-2">
        <section className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
          <h2 className="mb-3 text-sm font-semibold text-(--theme-text-strong)">
            实时事件
          </h2>
          {incidents.slice(0, 12).map(item => (
            <article
              className="mb-3 rounded-md border border-(--theme-border-subtle) p-3"
              key={item.id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <strong className="min-w-0 truncate text-sm text-(--theme-text-strong)">
                  {item.processName}
                </strong>
                <Badge text={item.severity} tone={severityTone(item.severity)} />
                <Badge text={item.status} tone={statusTone(item.status)} />
                <small className="ml-auto shrink-0 text-xs text-(--theme-text-muted)">
                  {item.occurrences} 次
                </small>
              </div>
              <p className="mt-1 truncate text-xs text-(--theme-text-secondary)">
                {item.sample || item.fingerprint}
              </p>
              <small className="block text-xs text-(--theme-text-faint)">
                {item.decisionReason || '等待策略分析'}
              </small>
              <div className="mt-2 flex flex-wrap gap-1">
                <button className="text-button" onClick={() => void post(`/api/v1/ops/incidents/${item.id}/analyze`)}>
                  分析
                </button>
                <button className="text-button" onClick={() => void post(`/api/v1/ops/incidents/${item.id}/dry-run`)}>
                  预览修复
                </button>
                <button className="text-button" onClick={() => void post(`/api/v1/ops/incidents/${item.id}/approve-once`)}>
                  单次批准
                </button>
                <button className="text-button" onClick={() => void post(`/api/v1/ops/incidents/${item.id}/todo`)}>
                  加入待办
                </button>
                <button className="text-button" onClick={() => void post(`/api/v1/ops/incidents/${item.id}/silence`)}>
                  静默
                </button>
                <button
                  className="text-button text-(--theme-danger-text)"
                  onClick={() => setConfirmDelete(item)}
                >
                  删除
                </button>
              </div>
            </article>
          ))}
          {incidents.length === 0 && (
            <p className="text-sm text-(--theme-text-muted)">暂无事件。</p>
          )}
        </section>
        <section className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
          <h2 className="mb-3 text-sm font-semibold text-(--theme-text-strong)">
            待办与维护
          </h2>
          {todos.slice(0, 8).map(item => (
            <article
              className="mb-2 rounded-md border border-(--theme-border-subtle) p-3"
              key={item.id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <strong className="min-w-0 truncate text-sm text-(--theme-text-strong)">
                  {item.title}
                </strong>
                <Badge text={item.status} tone={statusTone(item.status)} />
                <Badge text={item.severity} tone={severityTone(item.severity)} />
              </div>
              <p className="mt-1 text-xs text-(--theme-text-secondary)">
                {item.reason}
              </p>
              <div className="mt-1.5 flex flex-wrap gap-1">
                {item.status !== 'done' && (
                  <button className="text-button" onClick={() => void completeTodo(item)}>
                    完成
                  </button>
                )}
                <button
                  className="text-button text-(--theme-danger-text)"
                  onClick={() => setConfirmDeleteTodo(item)}
                >
                  删除
                </button>
              </div>
            </article>
          ))}
          {maintenance.slice(0, 8).map(item => (
            <article
              className="mb-2 rounded-md border border-(--theme-border-subtle) p-3"
              key={item.id}
            >
              <div className="flex flex-wrap items-center gap-2">
                <strong className="min-w-0 truncate text-sm text-(--theme-text-strong)">
                  维护 {item.id}
                </strong>
                <Badge text={item.status} tone={statusTone(item.status)} />
              </div>
              {item.error && (
                <p className="mt-1 text-xs text-(--theme-danger-text)">
                  {item.error}
                </p>
              )}
              <div className="mt-1.5 flex flex-wrap gap-1">
                {item.status === 'observing' && (
                  <button className="text-button" onClick={() => void post(`/api/v1/ops/maintenance/${item.id}/observe`)}>
                    结束观察
                  </button>
                )}
                {item.taskId && (
                  <button className="text-button" onClick={() => void post(`/api/v1/ops/maintenance/${item.id}/rollback`)}>
                    回滚
                  </button>
                )}
                {item.status === 'fixing' && (
                  <button className="text-button" onClick={() => void post(`/api/v1/ops/maintenance/${item.id}/takeover`)}>
                    人工接管
                  </button>
                )}
              </div>
            </article>
          ))}
          {todos.length === 0 && maintenance.length === 0 && (
            <p className="text-sm text-(--theme-text-muted)">暂无待办或维护任务。</p>
          )}
        </section>
      </div>
      <section className="mx-2.5 mt-4 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
		{readiness && (
		  <div className="mb-4 rounded-md border border-(--theme-border-default) bg-(--theme-surface-muted) p-3">
			<div className="flex items-center justify-between gap-3">
			  <strong className="text-xs text-(--theme-text-strong)">Canary 准入检查</strong>
			  <Badge text={readiness.ready ? 'ready' : 'blocked'} tone={readiness.ready ? 'success' : 'warning'} />
			</div>
			<div className="mt-2 grid gap-1 sm:grid-cols-2">
			  {readiness.checks.map(check => (
				<div className="flex items-center gap-1.5 text-[11px] text-(--theme-text-secondary)" key={check.name} title={check.detail}>
				  <span className={check.passed ? 'text-(--theme-success-text)' : 'text-(--theme-warning-text)'}>{check.passed ? '✓' : '!'}</span>
				  {check.name}
				</div>
			  ))}
			</div>
		  </div>
		)}
        <header className="mb-4 flex items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-(--theme-text-strong)">
            AI 策略
          </h2>
          <button
            className="primary-button"
            disabled={busy}
            onClick={() => void savePolicy()}
          >
            保存策略
          </button>
        </header>
        <div className="grid gap-6 lg:grid-cols-2">
          <div className="grid content-start gap-4 sm:grid-cols-2">
            <label className="grid gap-1.5 text-xs text-(--theme-text-secondary)">
              模式
              <select
                className={inputClass}
                value={policy.mode}
                onChange={event =>
                  setPolicy({
                    ...policy,
                    mode: event.target.value as Policy['mode']
                  })
                }
              >
                <option value="observe">观察</option>
                <option value="canary">灰度</option>
                <option value="auto">自动维护</option>
                <option value="strict">严格确认</option>
                <option value="off">关闭</option>
              </select>
            </label>
            <label className="grid gap-1.5 text-xs text-(--theme-text-secondary)">
              观察分钟数
              <input
                className={inputClass}
                type="number"
                min="1"
                value={policy.observationMinutes}
                onChange={event =>
                  setPolicy({
                    ...policy,
                    observationMinutes: Number(event.target.value) || 1
                  })
                }
              />
            </label>
          </div>
          <fieldset className="grid content-start gap-2">
            <legend className="text-xs text-(--theme-text-secondary)">
              自动化权限
            </legend>
            <label className="flex items-center gap-2 text-xs text-(--theme-text-secondary)">
              <input
                className="size-3.5 accent-(--theme-accent)"
                type="checkbox"
                checked={policy.autoAllowed}
                onChange={event =>
                  setPolicy({ ...policy, autoAllowed: event.target.checked })
                }
              />
              项目白名单
            </label>
            <label className="flex items-center gap-2 text-xs text-(--theme-text-secondary)">
              <input
                className="size-3.5 accent-(--theme-accent)"
                type="checkbox"
                checked={policy.allowCodeChanges}
                onChange={event =>
                  setPolicy({ ...policy, allowCodeChanges: event.target.checked })
                }
              />
              允许代码修改
            </label>
            <label className="flex items-center gap-2 text-xs text-(--theme-text-secondary)">
              <input
                className="size-3.5 accent-(--theme-accent)"
                type="checkbox"
                checked={policy.allowPm2Control}
                onChange={event =>
                  setPolicy({ ...policy, allowPm2Control: event.target.checked })
                }
              />
              允许 PM2 控制
            </label>
          </fieldset>
		  <label className="grid gap-1.5 text-xs text-(--theme-text-secondary)">
			策略验证命令（允许代码修改时必填）
			<input
			  className={inputClass}
			  placeholder="例如：yarn test"
			  value={policy.verificationCommand ?? ''}
			  onChange={event => setPolicy({ ...policy, verificationCommand: event.target.value })}
			/>
		  </label>
		  <label className="grid gap-1.5 text-xs text-(--theme-text-secondary)">
			本次策略调整理由
			<input
			  className={inputClass}
			  placeholder="开启 canary 或代码修改前请填写"
			  value={operationReason}
			  onChange={event => setOperationReason(event.target.value)}
			/>
		  </label>
        </div>
      </section>
      {(alerts.length > 0 || audits.length > 0) && (
        <div className="mx-2.5 mt-4 grid gap-4 lg:grid-cols-2">
          <section className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
            <h2 className="mb-3 text-sm font-semibold text-(--theme-text-strong)">
              告警中心
            </h2>
            {alerts.slice(0, 8).map(item => (
              <div className="mb-2 flex items-center gap-2 text-xs" key={item.id}>
                <Badge text={item.severity} tone={severityTone(item.severity)} />
                <Badge text={item.status} tone={statusTone(item.status)} />
                <span className="min-w-0 flex-1 truncate text-(--theme-text-secondary)">
                  {item.message}
                </span>
              </div>
            ))}
          </section>
          <section className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
            <h2 className="mb-3 text-sm font-semibold text-(--theme-text-strong)">
              审计记录
            </h2>
            {audits.slice(0, 8).map(item => (
              <div className="mb-2 flex items-center gap-2 text-xs" key={item.id}>
                <strong className="shrink-0 text-(--theme-text-strong)">
                  {item.actor}
                </strong>
                <span className="min-w-0 flex-1 truncate text-(--theme-text-secondary)">
                  {item.action}
                </span>
                <Badge text={item.result} tone={statusTone(item.result)} />
              </div>
            ))}
          </section>
        </div>
      )}
      <ConfirmDialog
        open={confirmStop}
        title="紧急停止"
        subtitle="此操作会立即停止当前项目的自动维护。"
        message={`确定要对「${root}」执行紧急停止吗？正在进行的分析、预览修复与自动维护任务都会中止。`}
        confirmLabel="紧急停止"
        destructive
        busy={busy}
        onCancel={() => setConfirmStop(false)}
        onConfirm={() => {
          setConfirmStop(false)
          void post('/api/v1/ops/monitor/emergency-stop')
        }}
      />
      <ConfirmDialog
        open={Boolean(confirmDelete)}
        title="删除事件"
        subtitle="此操作会从运维中心移除该事件记录。"
        message={`确定删除「${confirmDelete?.processName} · ${confirmDelete?.severity}」事件吗？已生成的待办与维护记录不受影响。`}
        confirmLabel="删除"
        destructive
        busy={busy}
        onCancel={() => setConfirmDelete(null)}
        onConfirm={() => {
          if (confirmDelete) void removeIncident(confirmDelete)
        }}
      />
      <ConfirmDialog
        open={Boolean(confirmDeleteTodo)}
        title="删除待办"
        subtitle="此操作会从运维中心移除该待办。"
        message={`确定删除「${confirmDeleteTodo?.title}」待办吗？此操作不会影响对应的事件记录。`}
        confirmLabel="删除"
        destructive
        busy={busy}
        onCancel={() => setConfirmDeleteTodo(null)}
        onConfirm={() => {
          if (confirmDeleteTodo) void removeTodo(confirmDeleteTodo)
        }}
      />
    </section>
  )
}
