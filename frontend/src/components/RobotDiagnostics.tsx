import { useState } from 'react'
import {
  useRobotDiagnosticLogsQuery,
  useRobotDiagnosticTaskQuery,
  useRobotDiagnosticsQuery
} from '../store/workspaceApi'
import { Button } from './Button'
import { Modal } from './Modal'

const pm2Status = {
  unavailable: '无法读取',
  daemon_offline: '守护进程离线',
  unregistered: '当前项目未登记',
  registered: '已登记'
}

const taskStatus: Record<string, string> = {
  running: '运行中',
  completed: '已完成',
  failed: '失败',
  stopped: '已停止',
  cancelled: '已取消',
  interrupted: '执行结果未知',
  online: '在线',
  launching: '启动中',
  errored: '异常'
}

const tcpLabels: Record<string, string> = {
  reachable: '可达',
  unreachable: '未连通'
}
const ownerLabels: Record<string, string> = {
  project: '本机器人',
  other: '其他进程'
}
const readinessLabels: Record<string, string> = {
  ready: '已就绪',
  not_ready: '未就绪'
}
const platformLabels: Record<string, string> = {
  connected: '已连接',
  disconnected: '未连接'
}

const timestamp = (value: string) => new Date(value).toLocaleString()

export function RobotDiagnostics({ root }: { root: string }) {
  const [open, setOpen] = useState(false)
  const [taskId, setTaskId] = useState('')
  const [logSource, setLogSource] = useState<'' | 'all' | 'err'>('')
  const {
    currentData: report,
    isFetching,
    isError,
    refetch
  } = useRobotDiagnosticsQuery(root, {
    skip: !open,
    refetchOnMountOrArgChange: true
  })
  const {
    currentData: task,
    isFetching: loadingTask,
    isError: taskError
  } = useRobotDiagnosticTaskQuery({ root, taskId }, { skip: !open || !taskId })

  const {
    currentData: logs,
    isFetching: loadingLogs,
    error: logError,
    refetch: refreshLogs
  } = useRobotDiagnosticLogsQuery(
    { root, source: logSource || 'all' },
    { skip: !open || !logSource, refetchOnMountOrArgChange: true }
  )

  const close = () => {
    setTaskId('')
    setLogSource('')
    setOpen(false)
  }

  return (
    <>
      <Button disabled={!root} onClick={() => setOpen(true)}>
        生产诊断
      </Button>
      <Modal open={open} onClose={close} ariaLabel="生产诊断">
        <section className="modal-surface grid max-h-[calc(100dvh-48px)] w-full max-w-2xl gap-4 overflow-auto">
          <header className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="m-0 text-base font-semibold">生产诊断</h2>
            <div className="flex gap-2">
              <Button disabled={isFetching} onClick={() => void refetch()}>
                刷新
              </Button>
              <Button onClick={close}>关闭</Button>
            </div>
          </header>
          {isFetching && <p role="status">正在检查运行状态…</p>}
          {isError && <p role="alert">无法读取机器人诊断，请刷新重试。</p>}
          {report && (
            <>
              <div className="grid gap-1.5 text-sm">
                <p className="text-(--theme-text-muted)">
                  采集时间 {timestamp(report.observedAt)}
                </p>
                {report.project && (
                  <p>
                    {report.project.name} {report.project.version}
                  </p>
                )}
                <p>
                  工作台本机运行{' '}
                  {report.localRuntime
                    ? taskStatus[report.localRuntime.status] ||
                      report.localRuntime.status
                    : '暂无本机运行记录'}
                </p>
                <p>PM2 {pm2Status[report.pm2.state]}</p>
                {report.pm2.processes.map(process => (
                  <p key={process.id}>
                    {process.name} ·{' '}
                    {taskStatus[process.status] || process.status}
                    {process.pid > 0 && ` · PID ${process.pid}`}
                    {' · '}重启 {process.restarts} 次
                  </p>
                ))}
                <p>
                  平台连接{' '}
                  {platformLabels[report.platformConnection?.state] || '未知'}
                </p>
                {report.ports.map(port => (
                  <div
                    key={port.kind}
                    className="grid gap-1 rounded-md border border-(--theme-border-default) p-2"
                  >
                    <p>
                      {port.label}
                      {port.configuredPort && ` · 配置 ${port.configuredPort}`}
                      {port.actualPort
                        ? ` · 实际 ${port.actualPort}`
                        : ` · 检测 ${port.port}`}
                    </p>
                    <p>
                      TCP {tcpLabels[port.tcp?.state] || '未知'} · 归属{' '}
                      {ownerLabels[port.ownership?.state] || '未知'} · 应用就绪{' '}
                      {readinessLabels[port.applicationReadiness?.state] ||
                        '未知'}
                      {port.httpStatus && ` · HTTP ${port.httpStatus}`}
                    </p>
                  </div>
                ))}
              </div>
              {report.findings.length > 0 && (
                <ul className="list-disc pl-5 text-sm">
                  {report.findings.map((finding, index) => (
                    <li key={`${finding.code}-${index}`}>{finding.message}</li>
                  ))}
                  {report.preflight?.missing.map((item, index) => (
                    <li key={`${item}-${index}`}>{item}</li>
                  ))}
                </ul>
              )}
              <div className="grid gap-2 text-sm">
                <div className="flex items-center justify-between">
                  <h3 className="m-0 text-sm font-semibold">近期操作</h3>
                  <Button
                    aria-pressed={Boolean(logSource)}
                    onClick={() =>
                      setLogSource(current => (current ? '' : 'all'))
                    }
                  >
                    {logSource ? '收起 PM2 日志' : '查看 PM2 日志'}
                  </Button>
                </div>
                {logSource && (
                  <div className="grid gap-2 rounded-md border border-(--theme-border-default) p-3">
                    <div className="flex flex-wrap gap-2">
                      <Button
                        aria-pressed={logSource === 'all'}
                        onClick={() => setLogSource('all')}
                      >
                        全部
                      </Button>
                      <Button
                        aria-pressed={logSource === 'err'}
                        onClick={() => setLogSource('err')}
                      >
                        仅 stderr
                      </Button>
                      <Button
                        disabled={loadingLogs}
                        onClick={() => void refreshLogs()}
                      >
                        刷新日志
                      </Button>
                    </div>
                    {loadingLogs && <p role="status">正在读取 PM2 日志…</p>}
                    {logError && (
                      <p role="alert">
                        {(logError as { data?: { error?: string } }).data
                          ?.error || '无法读取 PM2 日志。'}
                      </p>
                    )}
                    {logs && (
                      <>
                        {(logs.truncated || logs.hasOlder) && (
                          <p>显示最近的匹配日志。</p>
                        )}
                        {logs.lines.length === 0 ? (
                          <p>没有匹配的日志。</p>
                        ) : (
                          <pre className="m-0 max-h-56 overflow-auto whitespace-pre-wrap break-words text-xs">
                            {logs.lines
                              .map(line => `[${line.source}] ${line.text}`)
                              .join('\n')}
                          </pre>
                        )}
                      </>
                    )}
                  </div>
                )}
                {report.recentOperations.length === 0 && <p>暂无操作记录。</p>}
                {report.recentOperations.map(operation => (
                  <button
                    key={operation.id}
                    type="button"
                    aria-pressed={taskId === operation.id}
                    className="secondary-button justify-start whitespace-normal text-left"
                    onClick={() =>
                      setTaskId(current =>
                        current === operation.id ? '' : operation.id
                      )
                    }
                  >
                    {operation.action} ·{' '}
                    {taskStatus[operation.status] || operation.status}
                    {' · '}
                    {timestamp(operation.createdAt)}
                  </button>
                ))}
                {loadingTask && <p role="status">正在读取操作详情…</p>}
                {taskError && <p role="alert">无法读取这条操作记录。</p>}
                {task && (
                  <div className="grid gap-2 rounded-md border border-(--theme-border-default) p-3">
                    <p className="break-all font-mono text-xs">{task.id}</p>
                    {task.account && <p>发起账户 {task.account}</p>}
                    {task.root && <p className="break-all">目标机器人 {task.root}</p>}
                    {task.finishedAt && (
                      <p>
                        {taskStatus[task.status] || task.status} ·{' '}
                        {timestamp(task.finishedAt)}
                      </p>
                    )}
                    {task.error && (
                      <p className="break-words text-(--theme-danger)">
                        {task.error}
                      </p>
                    )}
                    {task.steps?.map((step, index) => (
                      <p key={`${step.at}-${index}`} className="break-words">
                        {timestamp(step.at)} · {step.progress}% · {step.message}
                      </p>
                    ))}
                    {task.lines.length > 0 && (
                      <pre className="m-0 max-h-56 overflow-auto whitespace-pre-wrap break-words text-xs">
                        {task.lines.join('\n')}
                      </pre>
                    )}
                  </div>
                )}
              </div>
            </>
          )}
        </section>
      </Modal>
    </>
  )
}
