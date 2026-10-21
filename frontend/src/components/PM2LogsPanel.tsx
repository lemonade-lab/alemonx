import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import cn from 'classnames'
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  ChevronRight,
  Download,
  Pause,
  Radio,
  RefreshCw,
  Terminal,
  Trash2
} from 'lucide-react'
import { DesktopWindow } from './DesktopWindow'
import { ConfirmDialog } from './ConfirmDialog'

function operationErrorMessage(reason: unknown, fallback: string) {
  return reason instanceof Error ? reason.message : fallback
}

const PM2_LIVE_MAX_LINES = 2000

export function PM2LogsPanel({
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
  type PM2LogDiagnostic = {
    code: string
    severity: 'error' | 'warning' | 'info'
    title: string
    summary: string
    suggestion: string
    count: number
    lastSeen?: string
  }
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
    diagnostics?: PM2LogDiagnostic[]
  }
  type PM2LogDay = {
    date: string
    count: number
    out: number
    err: number
    first?: string
    last?: string
  }
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(120)
  const [date, setDate] = useState('')
  const [source, setSource] = useState('all')
  const [query, setQuery] = useState('')
  const [committedQuery, setCommittedQuery] = useState('')
  const [data, setData] = useState<PM2AuditPage | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [days, setDays] = useState<PM2LogDay[]>([])
  const [live, setLive] = useState(true)
  const [liveConnected, setLiveConnected] = useState(false)
  const [streamFailed, setStreamFailed] = useState(false)
  const [liveLines, setLiveLines] = useState<PM2AuditLine[]>([])
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [diagnosticsExpanded, setDiagnosticsExpanded] = useState(false)
  const outputRef = useRef<HTMLDivElement>(null)
  const followLatest = useRef(true)

  const [frozen, setFrozen] = useState(false)
  const [revision, setRevision] = useState(0)
  const [wrap, setWrap] = useState(true)
  const [following, setFollowing] = useState(true)
  const [daysError, setDaysError] = useState('')
  const daysRequest = useRef({ id: 0 })

  const loadDays = useCallback(async () => {
    const request = ++daysRequest.current.id
    try {
      const response = await fetch(
        `/api/v1/robot/pm2-logs/days?${new URLSearchParams({ root })}`
      )
      const result = await response.json()
      if (request !== daysRequest.current.id) return
      if (!response.ok) throw new Error(result.error || '无法读取日志日期')
      setDays(result.days ?? [])
      setDaysError('')
    } catch (reason) {
      if (request === daysRequest.current.id)
        setDaysError(operationErrorMessage(reason, '无法读取日志日期'))
    }
  }, [root])

  // A new workspace must never display the previous robot's records or filters.
  useEffect(() => {
    const requestState = daysRequest.current
    setPage(1)
    setDate('')
    setSource('all')
    setQuery('')
    setCommittedQuery('')
    setData(null)
    setError('')
    setDays([])
    setLive(true)
    setFrozen(false)
    setLiveLines([])
    setDiagnosticsExpanded(false)
    return () => {
      requestState.id++
    }
  }, [root])

  useEffect(() => {
    const requestState = daysRequest.current
    if (open && root) void loadDays()
    return () => {
      requestState.id++
    }
  }, [loadDays, open, root])

  useEffect(() => {
    if (query === committedQuery) return
    const timer = window.setTimeout(() => {
      setCommittedQuery(query)
      setPage(1)
      setFrozen(false)
    }, 300)
    return () => window.clearTimeout(timer)
  }, [query, committedQuery])

  // Each view owns one request or stream. Cleanup rejects stale completions.
  // Live history and appended records share a server-captured file boundary.
  useEffect(() => {
    if (!open || !root || frozen) return
    const controller = new AbortController()
    let active = true
    setLoading(true)
    setError('')
    setLiveConnected(false)
    setStreamFailed(false)
    const params = new URLSearchParams({
      root,
      page: String(page),
      perPage: String(perPage)
    })
    if (date) params.set('date', date)
    if (source !== 'all') params.set('source', source)
    if (committedQuery.trim()) params.set('query', committedQuery.trim())
    if (!live) {
      void (async () => {
        try {
          const response = await fetch(`/api/v1/robot/pm2-logs?${params}`, {
            signal: controller.signal
          })
          const result = await response.json()
          if (!active) return
          if (!response.ok) throw new Error(result.error || '无法读取 PM2 日志')
          setData(result)
          setLiveLines([])
        } catch (reason) {
          if (active)
            setError(operationErrorMessage(reason, '无法读取 PM2 日志'))
        } finally {
          if (active) setLoading(false)
        }
      })()
      return () => {
        active = false
        controller.abort()
      }
    }
    params.set('snapshot', '1')
    params.set('page', '1')
    const stream = new EventSource(`/api/v1/robot/pm2-logs/stream?${params}`)
    let pending: PM2AuditLine[] = []
    const flush = () => {
      if (!active || pending.length === 0) return
      const batch = pending
      pending = []
      setLiveLines(current => [...current, ...batch].slice(-PM2_LIVE_MAX_LINES))
    }
    const timer = window.setInterval(flush, 100)
    stream.onopen = () => {
      if (active) setLiveConnected(false)
    }
    stream.onerror = () => {
      if (active) {
        setLiveConnected(false)
        setLoading(false)
      }
    }
    stream.onmessage = event => {
      if (!active) return
      try {
        const payload = JSON.parse(event.data) as {
          snapshot?: PM2AuditPage
          source?: string
          text?: string
          error?: string
        }
        if (payload.error) {
          setError(payload.error)
          setStreamFailed(true)
          setLiveConnected(false)
          setLoading(false)
          stream.close()
          return
        }
        if (payload.snapshot) {
          pending = []
          setData(payload.snapshot)
          setLiveLines([])
          setLoading(false)
          setLiveConnected(true)
          setError('')
          return
        }
        if (typeof payload.text !== 'string') return
        if (source !== 'all' && payload.source !== source) return
        if (
          committedQuery.trim() &&
          !payload.text
            .toLowerCase()
            .includes(committedQuery.trim().toLowerCase())
        )
          return
        pending.push({ source: payload.source ?? 'out', text: payload.text })
        if (pending.length > PM2_LIVE_MAX_LINES)
          pending = pending.slice(-PM2_LIVE_MAX_LINES)
      } catch {
        /* Ignore malformed frames. */
      }
    }
    return () => {
      // Preserve all received lines when the user pauses, including this batch.
      flush()
      active = false
      window.clearInterval(timer)
      stream.close()
    }
  }, [
    committedQuery,
    date,
    frozen,
    live,
    open,
    page,
    perPage,
    revision,
    root,
    source
  ])

  const toggleLive = () => {
    if (live) {
      setFrozen(true)
      setLive(false)
      setLoading(false)
      setLiveConnected(false)
    } else {
      setFrozen(false)
      setDate('')
      setPage(1)
      setLive(true)
    }
  }

  useEffect(() => {
    followLatest.current = true
    setFollowing(true)
  }, [committedQuery, date, page, root, source])

  const visibleLines = useMemo<PM2AuditLine[]>(() => {
    const q = committedQuery.trim().toLowerCase()
    return [...(data?.lines ?? []), ...liveLines]
      .filter(
        line =>
          (source === 'all' || line.source === source) &&
          (!q || line.text.toLowerCase().includes(q))
      )
      .slice(-PM2_LIVE_MAX_LINES)
  }, [committedQuery, data, liveLines, source])
  const diagnostics = data?.diagnostics ?? []

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
    setFrozen(false)
    setLive(false)
    setDate(target)
    setPage(1)
    followLatest.current = true
  }
  const jumpLatest = () => {
    setFrozen(false)
    setLive(true)
    setDate('')
    setPage(1)
    followLatest.current = true
    setFollowing(true)
    setRevision(value => value + 1)
  }
  const refresh = () => {
    setFrozen(false)
    setRevision(value => value + 1)
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
  const deleteSelectedLogs = async () => {
    if (!date || deleteBusy) return
    setDeleteBusy(true)
    try {
      // Pause the live tail before rewriting a selected day from its PM2 log
      // files. The next open resumes live following by default.
      setLive(false)
      const params = new URLSearchParams({ root, date })
      if (source !== 'all') params.set('source', source)
      const response = await fetch(`/api/v1/robot/pm2-logs?${params}`, {
        method: 'DELETE'
      })
      const result = (await response.json().catch(() => null)) as {
        deleted?: number
        error?: string
      } | null
      if (!response.ok)
        throw new Error(result?.error || '删除指定 PM2 日志失败。')
      setLiveLines([])
      setDeleteDialogOpen(false)
      setPage(1)
      setFrozen(false)
      setRevision(value => value + 1)
      await loadDays()
    } catch (reason) {
      setError(operationErrorMessage(reason, '删除指定 PM2 日志失败。'))
    } finally {
      setDeleteBusy(false)
    }
  }
  const selectedLogScope = `${date} · ${
    source === 'all' ? '全部日志' : source === 'out' ? '输出日志' : '错误日志'
  }`

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
            : frozen
              ? '已暂停 · 保留当前记录'
              : '历史日志'
      }
      icon={
        <Terminal className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 168, top: 152 }}
      width={1000}
      height={680}
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
            {live ? '暂停接收' : '恢复实时'}
          </button>
          <div className="pm2-audit-source" role="group" aria-label="日志来源">
            {[
              ['all', '全部'],
              ['out', '标准输出'],
              ['err', '标准错误']
            ].map(([value, label]) => (
              <button
                key={value}
                className={cn(
                  'pm2-audit-source-btn',
                  source === value && 'active'
                )}
                aria-pressed={source === value}
                title={
                  value === 'err'
                    ? 'stderr 来源，不代表每一行都是错误'
                    : undefined
                }
                onClick={() => {
                  setSource(value)
                  setFrozen(false)
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
          <button
            className="pm2-audit-tool-btn"
            aria-pressed={wrap}
            onClick={() => setWrap(value => !value)}
          >
            自动换行
          </button>
          <button
            className="pm2-audit-tool-btn"
            onClick={() => void exportLogs()}
            title="导出当前筛选的日志"
          >
            {' '}
            <Download className="size-3" />
            导出
          </button>
        </div>
        <div className="pm2-audit-toolbar">
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
          <select
            className="pm2-audit-field"
            value={perPage}
            onChange={event => {
              setFrozen(false)
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
            onClick={() => {
              setFrozen(false)
              setPage(current => Math.max(1, current - 1))
            }}
            title="查看更新的一页"
          >
            <ArrowLeft className="size-3" />
            更新
          </button>
          <button
            className="pm2-audit-tool-btn"
            disabled={loading || live || !data?.hasOlder}
            onClick={() => {
              setFrozen(false)
              setPage(current => current + 1)
            }}
            title="查看更早的一页"
          >
            更早
            <ArrowRight className="size-3" />
          </button>
          <button
            className="pm2-audit-tool-btn text-rose-600 disabled:text-slate-400 dark:text-rose-300"
            disabled={!date || deleteBusy}
            onClick={() => setDeleteDialogOpen(true)}
            title={
              date ? `删除 ${selectedLogScope}` : '请先选择要删除的日志日期'
            }
          >
            <Trash2 className="size-3" />
            删除
          </button>
        </div>
        {diagnostics.length > 0 && (
          <section className="pm2-audit-diagnostics">
            <button
              className="pm2-audit-diagnostics-header"
              onClick={() => setDiagnosticsExpanded(value => !value)}
              aria-expanded={diagnosticsExpanded}
              aria-controls="pm2-log-diagnostics"
            >
              <AlertTriangle className="size-3.5" />
              <strong>当前日志诊断</strong>
              <span className="ml-auto text-[var(--theme-text-muted)]">
                {diagnostics.length} 项
              </span>
              <ChevronRight
                className={cn(
                  'size-3.5 transition-transform',
                  diagnosticsExpanded && 'rotate-90'
                )}
              />
            </button>
            {diagnosticsExpanded && (
              <div
                id="pm2-log-diagnostics"
                className="pm2-audit-diagnostics-list"
                aria-label="当前日志诊断详情"
              >
                {diagnostics.map(item => (
                  <div
                    key={item.code}
                    className={cn(
                      'grid gap-0.5 border-l-[3px] border-t border-t-[var(--theme-border-default)] px-2.5 py-[7px]',
                      item.severity === 'error' && 'border-l-[#dc2626]',
                      item.severity === 'warning' && 'border-l-[#d97706]',
                      item.severity === 'info' && 'border-l-[#0284c7]'
                    )}
                  >
                    <div className="flex items-baseline gap-2">
                      <strong className="text-[0.73rem] text-[var(--theme-text-primary)]">
                        {item.title}
                      </strong>
                      <span className="ml-auto whitespace-nowrap text-[0.66rem] text-[var(--theme-text-muted)]">
                        {item.count > 1 ? `${item.count} 次 · ` : ''}
                        {item.lastSeen || '当前日志'}
                      </span>
                    </div>
                    <p className="m-0 text-[0.69rem] leading-[1.4] text-[var(--theme-text-secondary)]">
                      {item.summary}
                    </p>
                    <small className="m-0 text-[0.66rem] leading-[1.4] text-[var(--theme-text-muted)]">
                      {item.suggestion}
                    </small>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}
        {error && (
          <div className="pm2-audit-notice" role="alert">
            {error}
            <button className="pm2-audit-tool-btn" onClick={refresh}>
              重试
            </button>
          </div>
        )}
        {daysError && (
          <div className="pm2-audit-notice" role="status">
            日期列表不可用：{daysError}
          </div>
        )}
        <div
          className={cn('pm2-audit-body', !wrap && 'pm2-audit-nowrap')}
          ref={outputRef}
          role="region"
          aria-label="PM2 日志内容"
          tabIndex={0}
          aria-busy={loading}
          onScroll={event => {
            const el = event.currentTarget
            followLatest.current =
              el.scrollHeight - el.scrollTop - el.clientHeight < 48
            setFollowing(followLatest.current)
          }}
        >
          {loading && !data ? (
            <div className="pm2-audit-hint">正在读取 PM2 日志…</div>
          ) : visibleLines.length === 0 ? (
            <div className="pm2-audit-hint">
              {committedQuery || date || source !== 'all'
                ? '没有匹配的日志，请调整筛选条件。'
                : '暂无日志，进程产生输出后会显示在这里。'}
            </div>
          ) : (
            visibleLines.map((line, index) => (
              <div
                key={`${line.source}-${index}`}
                className={cn(
                  'pm2-audit-line',
                  line.source === 'err' && 'pm2-audit-line-err'
                )}
              >
                <span
                  className="pm2-audit-line-source"
                  title={
                    line.source === 'err'
                      ? '标准错误 stderr'
                      : '标准输出 stdout'
                  }
                >
                  {line.source === 'err' ? 'ERR' : 'OUT'}
                </span>
                <span>{line.text}</span>
              </div>
            ))
          )}
        </div>
        <footer className="pm2-audit-footer">
          <span role="status">
            {loading
              ? '读取中…'
              : live
                ? streamFailed
                  ? '实时日志不可用，请重试'
                  : liveConnected
                    ? '● 实时连接'
                    : '连接中 / 断线重连中'
                : frozen
                  ? '已暂停接收'
                  : `第 ${data?.page ?? page} 页`}
          </span>
          <span>
            显示 {visibleLines.length} 行
            {!live && !frozen && data ? ` · 共 ${data.total} 条匹配` : ''}
            {data?.truncated ? ' · 超出读取上限，仅显示最近记录' : ''}
          </span>
          {!following && (
            <button
              className="pm2-audit-tool-btn"
              onClick={() => {
                followLatest.current = true
                setFollowing(true)
                if (outputRef.current)
                  outputRef.current.scrollTop = outputRef.current.scrollHeight
              }}
            >
              回到底部
            </button>
          )}
        </footer>
      </div>
      <ConfirmDialog
        open={deleteDialogOpen}
        title="删除指定 PM2 日志"
        subtitle={date ? selectedLogScope : ''}
        message="会删除所选日期和来源的日志记录；删除后无法恢复。未带时间戳的记录会保留。"
        confirmLabel="删除日志"
        destructive
        busy={deleteBusy}
        onCancel={() => !deleteBusy && setDeleteDialogOpen(false)}
        onConfirm={() => void deleteSelectedLogs()}
      />
    </DesktopWindow>
  )
}
