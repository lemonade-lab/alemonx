import { useEffect, useMemo, useRef } from 'react'
import cn from 'classnames'
import { ClipboardList, Download, RefreshCw, Search } from 'lucide-react'
import { DesktopWindow } from './DesktopWindow'
import { operationErrorMessage } from '../lib/operationErrorMessage'
import { useStoreState } from '../store/guideStore'
import { useLazyRobotConsoleQuery } from '../store/workspaceApi'
import {
  collapseForegroundLog,
  filterForegroundLog,
  foregroundLogDomains,
  foregroundLogDisplayText,
  foregroundLogSummary as summarizeForegroundLog,
  createForegroundLogNormalizer,
  redactForegroundLog,
  type ForegroundLogAction,
  type ForegroundLogDomain,
  type ForegroundLogLevel,
  type ForegroundLogItem,
  type ForegroundLogMode
} from '../lib/foregroundLogInterpreter'

export function ReadonlyConsole({
  open,
  taskId,
  logTitle,
  minimized,
  root,
  onClose,
  onMinimize,
  onOpenRuntime,
  onOpenConfig,
  onOpenEnvironment,
  masterAuthorizationIDs = [],
  onToggleMasterAuthorization,
  onOpenService,
  zIndex,
  onActivate,
  native = false
}: {
  native?: boolean
  open: boolean
  taskId?: string
  logTitle?: string
  minimized: boolean
  root: string
  onClose: () => void
  onMinimize: () => void
  onOpenRuntime?: () => void
  onOpenConfig?: () => void
  onOpenEnvironment?: () => void
  masterAuthorizationIDs?: string[]
  onToggleMasterAuthorization?: (
    userID: string,
    currentlyAuthorized: boolean
  ) => void
  onOpenService?: (url: string) => void
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

  const [load, { data, error, isFetching }] = useLazyRobotConsoleQuery()
  const outputRef = useRef<HTMLDivElement>(null)
  const followLatest = useRef(true)
  // liveOutput accumulates incremental output pushed over SSE; the initial
  // load seeds it with the current buffer so the terminal is real-time without
  // polling. The static snapshot still comes from the query.
  const [liveOutput, setLiveOutput] = useStoreState('')
  const [foregroundLogMode, setForegroundLogMode] =
    useStoreState<ForegroundLogMode>('simple')
  const [foregroundLogQuery, setForegroundLogQuery] = useStoreState('')
  const [foregroundLogLevels, setForegroundLogLevels] = useStoreState<
    ForegroundLogLevel[]
  >([])
  const [foregroundLogDomainsSelected, setForegroundLogDomainsSelected] =
    useStoreState<ForegroundLogDomain[]>([])
  const [foregroundLogFiltersOpen, setForegroundLogFiltersOpen] =
    useStoreState(false)
  const [expandedForegroundRepeats, setExpandedForegroundRepeats] =
    useStoreState<string[]>([])
  const [foregroundLogContextIndex, setForegroundLogContextIndex] =
    useStoreState<number | null>(null)
  const [pendingForegroundLogCount, setPendingForegroundLogCount] =
    useStoreState(0)
  const previousForegroundLogRef = useRef('')
  // A task ID is allocated for every foreground/development launch. Keep it
  // beside the live buffer so output from a new launch never joins the old
  // run just because the logger did not print a timestamp.
  const foregroundSessionRef = useRef('')
  const runError = error
    ? operationErrorMessage(error, '无法读取当前目录的运行终端信息。')
    : ''
  const running = Boolean(data?.running)
  const message = runError || liveOutput
  const normalizeLog = useMemo(createForegroundLogNormalizer, [])
  const rawForegroundLogItems = useMemo(() => normalizeLog(message), [message, normalizeLog])
  const foregroundLogItems = useMemo(() => {
    const modeVisible = rawForegroundLogItems.filter(
      item =>
        foregroundLogMode !== 'simple' ||
        item.level !== 'info' ||
        item.domain === 'login' ||
        item.domain === 'service' ||
        item.action !== null ||
        item.details.length > 0
    )
    const modeItems =
      foregroundLogMode === 'advanced'
        ? modeVisible
        : collapseForegroundLog(modeVisible)
    return filterForegroundLog(modeItems, {
      query: foregroundLogQuery,
      levels: foregroundLogLevels,
      domains: foregroundLogDomainsSelected
    })
  }, [
    foregroundLogDomainsSelected,
    foregroundLogLevels,
    foregroundLogMode,
    foregroundLogQuery,
    rawForegroundLogItems
  ])
  const foregroundLogSummary = useMemo(
    () => summarizeForegroundLog(foregroundLogItems),
    [foregroundLogItems]
  )
  const foregroundLogContext = useMemo(() => {
    if (foregroundLogContextIndex === null) return []
    const position = rawForegroundLogItems.findIndex(
      item => item.index === foregroundLogContextIndex
    )
    return position < 0
      ? []
      : rawForegroundLogItems.slice(
          Math.max(0, position - 20),
          Math.min(rawForegroundLogItems.length, position + 21)
        )
  }, [foregroundLogContextIndex, rawForegroundLogItems])
  const buildForegroundLogExport = () => {
    const filters = [
      foregroundLogQuery.trim() ? `关键词：${foregroundLogQuery.trim()}` : '',
      foregroundLogLevels.length
        ? `级别：${foregroundLogLevels.join('、')}`
        : '',
      foregroundLogDomainsSelected.length
        ? `领域：${foregroundLogDomainsSelected.join('、')}`
        : ''
    ].filter(Boolean)
    return [
      'AlemonX 智能日志（已脱敏）',
      `会话：${data?.sessionId || '当前运行'}`,
      `模式：${data?.mode || '最近运行'}`,
      data?.startedAt ? `启动时间：${data.startedAt}` : '',
      filters.length ? `筛选：${filters.join('；')}` : '筛选：全部',
      '',
      ...filterForegroundLog(rawForegroundLogItems, {
        query: foregroundLogQuery,
        levels: foregroundLogLevels,
        domains: foregroundLogDomainsSelected
      }).map(item => item.safeRaw)
    ]
      .filter(Boolean)
      .join('\n')
  }
  useEffect(() => {
    if (!open || !root) return
    void load({ root, taskId }).then(result => {
      if (result.data) {
        const changedSession =
          Boolean(result.data.sessionId) &&
          result.data.sessionId !== foregroundSessionRef.current
        foregroundSessionRef.current = result.data.sessionId ?? ''
        if (changedSession) {
          setExpandedForegroundRepeats([])
          setForegroundLogContextIndex(null)
        }
        setLiveOutput(appendTerminalOutput('', result.data.output ?? ''))
      }
    })
    // No polling: output streams in via the SSE robot-output event; the manual
    // refresh button re-reads the snapshot.
  }, [
    load,
    open,
    root,
    taskId,
    setExpandedForegroundRepeats,
    setForegroundLogContextIndex,
    setLiveOutput
  ])
  useEffect(() => {
    if (!native || !open || !root) return
    const query = new URLSearchParams(taskId ? { taskId } : {})
    const source = new EventSource(`/api/v1/robot/events?${query}`)
    let timer: number | undefined
    let disposed = false
    const refresh = () => {
      if (timer !== undefined) return
      timer = window.setTimeout(() => {
        void load({ root, taskId })
          .then(result => {
            if (!disposed && result.data)
              setLiveOutput(result.data.output ?? '')
          })
          .finally(() => {
            timer = undefined
          })
      }, 350)
    }
    source.onopen = refresh
    source.onmessage = event => {
      try {
        const payload = JSON.parse(event.data) as { type?: string }
        if (payload.type === 'output' || payload.type === 'task') refresh()
      } catch {
        // Ignore malformed frames.
      }
    }
    return () => {
      disposed = true
      source.close()
      window.clearTimeout(timer)
    }
  }, [load, native, open, root, setLiveOutput, taskId])
  useEffect(() => {
    if (!open || native) return
    let timer: number | undefined
    let pending = ''
    let resetSession = false
    const flush = () => {
      timer = undefined
      const output = pending
      const reset = resetSession
      pending = ''
      resetSession = false
      if (reset) {
        setExpandedForegroundRepeats([])
        setForegroundLogContextIndex(null)
      }
      setLiveOutput(current =>
        appendTerminalOutput(reset ? '' : current, output)
      )
    }
    const handler = (event: Event) => {
      const detail = (
        event as CustomEvent<{
          taskId?: string
          text?: string
          truncated?: boolean
        }>
      ).detail
      if (!detail?.text || (taskId && detail.taskId !== taskId)) return
      if (detail.taskId && detail.taskId !== foregroundSessionRef.current) {
        foregroundSessionRef.current = detail.taskId
        resetSession = true
        pending = ''
      }
      pending = appendTerminalOutput(
        pending,
        `${detail.truncated ? '…早期输出已省略…\n' : ''}${detail.text}`
      )
      // Bound parsing and React updates when one process emits many SSE frames.
      timer ??= window.setTimeout(flush, 50)
    }
    window.addEventListener('alx:robot-output', handler)
    return () => {
      window.removeEventListener('alx:robot-output', handler)
      window.clearTimeout(timer)
    }
  }, [
    native,
    open,
    root,
    taskId,
    setExpandedForegroundRepeats,
    setForegroundLogContextIndex,
    setLiveOutput
  ])
  useEffect(() => {
    if (!open) return
    const previous = previousForegroundLogRef.current
    if (previous && message.length > previous.length && !followLatest.current)
      setPendingForegroundLogCount(value => value + 1)
    previousForegroundLogRef.current = message
  }, [message, open, setPendingForegroundLogCount])
  useEffect(() => {
    if (!open) return
    followLatest.current = true
    setPendingForegroundLogCount(0)
  }, [open, setPendingForegroundLogCount])
  useEffect(() => {
    if (!open || !followLatest.current) return
    const frame = window.requestAnimationFrame(() => {
      const output = outputRef.current
      if (output) output.scrollTop = output.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
  }, [message, open])
  const foregroundLogActionLabel = (item: ForegroundLogItem) => {
    if (!item.action) return ''
    if (
      native &&
      item.action === 'manage-master' &&
      !onToggleMasterAuthorization
    )
      return ''
    if (
      native &&
      (item.action === 'open-runtime' ||
        item.action === 'install-dependencies') &&
      !onOpenRuntime
    )
      return ''
    if (native && item.action === 'open-config' && !onOpenConfig) return ''
    if (native && item.action === 'open-environment' && !onOpenEnvironment)
      return ''
    const labels: Record<NonNullable<ForegroundLogAction>, string> = {
      'install-dependencies': '自动同步依赖',
      'open-runtime': '查看运行',
      'open-config': '查看配置',
      'open-environment': '环境检查',
      'open-service': '打开服务',
      'copy-service-url': '复制地址',
      'manage-master':
        item.masterUserID && masterAuthorizationIDs.includes(item.masterUserID)
          ? '取消主人授权'
          : '授权为主人'
    }
    return labels[item.action]
  }
  const runForegroundLogAction = (item: ForegroundLogItem) => {
    switch (item.action) {
      case 'install-dependencies':
        onOpenRuntime?.()
        return
      case 'open-runtime':
        onOpenRuntime?.()
        return
      case 'open-config':
        onOpenConfig?.()
        return
      case 'open-environment':
        onOpenEnvironment?.()
        return
      case 'open-service':
        if (item.serviceURL) onOpenService?.(item.serviceURL)
        return
      case 'copy-service-url':
        if (item.serviceURL)
          void navigator.clipboard?.writeText(
            redactForegroundLog(item.serviceURL)
          )
        return
      case 'manage-master':
        if (!item.masterUserID) return
        onToggleMasterAuthorization?.(
          item.masterUserID,
          masterAuthorizationIDs.includes(item.masterUserID)
        )
    }
  }
  const toggleLogLevel = (level: ForegroundLogLevel) =>
    setForegroundLogLevels(current =>
      current.includes(level)
        ? current.filter(value => value !== level)
        : [...current, level]
    )
  const toggleLogDomain = (domain: ForegroundLogDomain) =>
    setForegroundLogDomainsSelected(current =>
      current.includes(domain)
        ? current.filter(value => value !== domain)
        : [...current, domain]
    )
  const toggleForegroundLogContext = (index: number) =>
    setForegroundLogContextIndex(current => (current === index ? null : index))
  const copyForegroundLog = () =>
    void navigator.clipboard?.writeText(buildForegroundLogExport())
  const downloadForegroundLog = () => {
    const blob = new Blob([buildForegroundLogExport()], {
      type: 'text/plain;charset=utf-8'
    })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `alemonx-log-${data?.sessionId || 'current'}.txt`
    anchor.click()
    URL.revokeObjectURL(url)
  }
  const jumpToLatestForegroundLog = () => {
    const output = outputRef.current
    if (output) output.scrollTop = output.scrollHeight
    followLatest.current = true
    setPendingForegroundLogCount(0)
  }
  if (!open) return null
  return (
    <DesktopWindow
      native={native}
      id="foreground-logs"
      open={open}
      minimized={minimized}
      title={logTitle ?? '日志'}
      subtitle={
        running
          ? `${data?.mode ?? '进程'}实时输出 · 智能识别`
          : '最近运行输出 · 智能整理'
      }
      icon={
        <ClipboardList className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 16, top: 16 }}
      width={560}
      height={650}
      actions={
        <div className="flex items-center gap-1">
          <button
            className={cn(
              'smart-log-window-action',
              (foregroundLogQuery ||
                foregroundLogLevels.length ||
                foregroundLogDomainsSelected.length) &&
                'active'
            )}
            onClick={() => setForegroundLogFiltersOpen(value => !value)}
            title="筛选日志"
          >
            {foregroundLogSummary.errors > 0
              ? `异常 ${foregroundLogSummary.errors}`
              : foregroundLogSummary.warnings > 0
                ? `提醒 ${foregroundLogSummary.warnings}`
                : '筛选'}
          </button>
          <button
            className="smart-log-window-action"
            onClick={() =>
              setForegroundLogMode(mode =>
                mode === 'simple'
                  ? 'common'
                  : mode === 'common'
                    ? 'advanced'
                    : 'simple'
              )
            }
            title="切换日志显示层级"
          >
            {
              { simple: '简洁', common: '常用', advanced: '详细' }[
                foregroundLogMode
              ]
            }
          </button>
          <button
            className="icon-button size-8 p-0"
            disabled={!rawForegroundLogItems.length}
            onClick={copyForegroundLog}
            aria-label="复制脱敏运行日志"
            title="复制脱敏日志"
          >
            <ClipboardList className="size-4" />
          </button>
          <button
            className="icon-button size-8 p-0"
            disabled={!rawForegroundLogItems.length}
            onClick={downloadForegroundLog}
            aria-label="下载脱敏运行日志"
            title="下载脱敏日志"
          >
            <Download className="size-4" />
          </button>
          <button
            className="icon-button size-8 p-0"
            disabled={isFetching}
            onClick={() => void load({ root, taskId, refresh: true })}
            aria-label="刷新运行日志"
            title="刷新"
          >
            <RefreshCw className="size-4" />
          </button>
        </div>
      }
    >
      <div className="readonly-console-body min-h-0">
        <div className="smart-foreground-log">
          <div
            ref={outputRef}
            className="smart-foreground-log-list"
            onScroll={event => {
              const output = event.currentTarget
              followLatest.current =
                output.scrollHeight - output.scrollTop - output.clientHeight <
                24
              if (followLatest.current) setPendingForegroundLogCount(0)
            }}
          >
            {pendingForegroundLogCount > 0 && (
              <button
                className="smart-log-pending"
                onClick={jumpToLatestForegroundLog}
              >
                有新日志，跳到最新
              </button>
            )}
            {foregroundLogFiltersOpen && (
              <section className="smart-log-filter-panel" aria-label="日志筛选">
                <div className="flex items-center gap-2">
                  <Search className="size-3.5 shrink-0 text-slate-400" />
                  <input
                    value={foregroundLogQuery}
                    onChange={event =>
                      setForegroundLogQuery(event.target.value)
                    }
                    placeholder="搜索当前会话（已脱敏）"
                    aria-label="搜索日志"
                  />
                  <button
                    type="button"
                    className="text-button text-[11px]"
                    onClick={() => {
                      setForegroundLogQuery('')
                      setForegroundLogLevels([])
                      setForegroundLogDomainsSelected([])
                    }}
                  >
                    清除
                  </button>
                </div>
                <div className="smart-log-filter-row">
                  <small>级别</small>
                  {(
                    [
                      'error',
                      'warning',
                      'success',
                      'info'
                    ] as ForegroundLogLevel[]
                  ).map(level => (
                    <button
                      type="button"
                      key={level}
                      className={cn(
                        'smart-log-filter-chip',
                        foregroundLogLevels.includes(level) && 'active'
                      )}
                      onClick={() => toggleLogLevel(level)}
                    >
                      {
                        {
                          error: '异常',
                          warning: '提醒',
                          success: '正常',
                          info: '信息'
                        }[level]
                      }
                    </button>
                  ))}
                </div>
                <div className="smart-log-filter-row">
                  <small>领域</small>
                  {foregroundLogDomains.map(domain => (
                    <button
                      type="button"
                      key={domain.value}
                      className={cn(
                        'smart-log-filter-chip',
                        foregroundLogDomainsSelected.includes(domain.value) &&
                          'active'
                      )}
                      onClick={() => toggleLogDomain(domain.value)}
                    >
                      {domain.label}
                    </button>
                  ))}
                </div>
              </section>
            )}
            {isFetching && !message ? (
              <div className="smart-log-empty">正在读取运行输出…</div>
            ) : foregroundLogItems.length === 0 ? (
              <div className="smart-log-empty">
                {foregroundLogQuery ||
                foregroundLogLevels.length ||
                foregroundLogDomainsSelected.length
                  ? '当前筛选条件下没有日志。'
                  : '当前没有可显示的前台日志。'}
              </div>
            ) : (
              foregroundLogItems.map((item, index) => (
                <div key={item.id}>
                  {foregroundLogMode !== 'advanced' &&
                    item.timeLabel &&
                    item.timeLabel !==
                      foregroundLogItems[index - 1]?.timeLabel && (
                      <div className="smart-log-time-divider">
                        <span>{item.timeLabel}</span>
                      </div>
                    )}
                  {foregroundLogMode === 'advanced' ? (
                    <div className="smart-log-raw-wrap">
                      <pre className={`smart-log-raw is-${item.level}`}>
                        {foregroundLogDisplayText(item, foregroundLogMode)}
                      </pre>
                      <button
                        className="smart-log-context-button"
                        onClick={() => toggleForegroundLogContext(item.index)}
                      >
                        上下文
                      </button>
                    </div>
                  ) : (
                    <article
                      className={cn('smart-log-line', `is-${item.level}`)}
                    >
                      <div className="smart-log-line-meta">
                        <span className="smart-log-title">
                          {item.title}
                          {item.time && <small>{item.time}</small>}
                          {item.count > 1 && (
                            <small className="smart-log-repeat">
                              × {item.count}
                            </small>
                          )}
                        </span>
                        <span className="smart-log-line-actions">
                          {foregroundLogActionLabel(item) &&
                            item.action !== 'install-dependencies' && (
                              <button
                                onClick={() => runForegroundLogAction(item)}
                              >
                                {foregroundLogActionLabel(item)}
                              </button>
                            )}
                          {item.count > 1 && (
                            <button
                              onClick={() =>
                                setExpandedForegroundRepeats(current =>
                                  current.includes(item.id)
                                    ? current.filter(value => value !== item.id)
                                    : [...current, item.id]
                                )
                              }
                            >
                              {expandedForegroundRepeats.includes(item.id)
                                ? '收起重复'
                                : '展开重复'}
                            </button>
                          )}
                          <button
                            onClick={() =>
                              toggleForegroundLogContext(
                                item.occurrenceIndexes[0] ?? item.index
                              )
                            }
                          >
                            上下文
                          </button>
                        </span>
                      </div>
                      <code>
                        {foregroundLogDisplayText(item, foregroundLogMode)}
                      </code>
                      {(foregroundLogMode === 'simple' ||
                        (item.level !== 'error' && item.level !== 'warning')) &&
                        item.details.filter(detail => detail.label !== '信息')
                          .length > 0 && (
                          <div className="smart-log-extracts">
                            {item.details
                              .filter(detail => detail.label !== '信息')
                              .map(detail => (
                                <span key={`${detail.label}:${detail.value}`}>
                                  <small>{detail.label}</small>
                                  {detail.value}
                                </span>
                              ))}
                          </div>
                        )}
                    </article>
                  )}
                  {foregroundLogMode !== 'advanced' &&
                    item.count > 1 &&
                    expandedForegroundRepeats.includes(item.id) && (
                      <div className="smart-log-repeat-list">
                        {item.occurrenceIndexes.map(occurrence => {
                          const repeated = rawForegroundLogItems.find(
                            value => value.index === occurrence
                          )
                          return repeated ? (
                            <pre key={repeated.id}>{repeated.safeRaw}</pre>
                          ) : null
                        })}
                      </div>
                    )}
                  {foregroundLogContextIndex !== null &&
                    item.occurrenceIndexes.includes(
                      foregroundLogContextIndex
                    ) && (
                      <section
                        className="smart-log-context"
                        aria-label="日志上下文"
                      >
                        <header>
                          <strong>上下文</strong>
                        </header>
                        {foregroundLogContext.map(context => (
                          <pre
                            key={context.id}
                            className={cn(
                              context.index === foregroundLogContextIndex &&
                                'active'
                            )}
                          >
                            {context.safeRaw}
                          </pre>
                        ))}
                      </section>
                    )}
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </DesktopWindow>
  )
}
