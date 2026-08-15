import { useCallback, useEffect, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { ConfirmDialog } from './ConfirmDialog'
import { SidebarWindowActions } from './SidebarWindow'

type Project = { id: string; name: string; path: string }
type Policy = { projectRoot: string; mode: string; autoAllowed: boolean }
type Overview = {
  metrics: {
    incidents: number
    openTodos: number
    maintenanceRuns: number
    resolved: number
    rollbacks: number
  }
  policies: Policy[]
  paused: boolean
  nodeId?: string
}

const METRIC_ITEMS: Array<{ key: keyof Overview['metrics']; label: string }> = [
  { key: 'incidents', label: '事件' },
  { key: 'openTodos', label: '待办' },
  { key: 'maintenanceRuns', label: '维护中' },
  { key: 'resolved', label: '已恢复' },
  { key: 'rollbacks', label: '回滚' }
]

export function OpsOverview({
  projects,
  onOpenProject,
  sidebarLayout = false
}: {
  projects: Project[]
  onOpenProject: (id: string) => void
  sidebarLayout?: boolean
}) {
  const [overview, setOverview] = useState<Overview | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmStop, setConfirmStop] = useState(false)
  const load = useCallback(async () => {
    const response = await fetch('/api/v1/ops/overview')
    if (response.ok) setOverview((await response.json()) as Overview)
  }, [])
  useEffect(() => {
    void load()
    const refresh = () => void load()
    window.addEventListener('alx:ops-changed', refresh)
    return () => window.removeEventListener('alx:ops-changed', refresh)
  }, [load])
  const control = async (action: 'pause' | 'resume' | 'emergency-stop') => {
    setBusy(true)
    try {
      await fetch(`/api/v1/ops/monitor/${action}`, { method: 'POST' })
      await load()
    } finally {
      setBusy(false)
    }
  }
  const persistentActions = (
    <>
      <button
        className={sidebarLayout ? 'secondary-button w-full justify-start' : 'secondary-button'}
        disabled={busy}
        onClick={() => void control(overview?.paused ? 'resume' : 'pause')}
      >
        {overview?.paused ? '恢复自动维护' : '暂停自动维护'}
      </button>
      <button
        className={sidebarLayout ? 'danger-button w-full justify-start' : 'danger-button'}
        disabled={busy}
        onClick={() => setConfirmStop(true)}
      >
        紧急停止全部
      </button>
    </>
  )
  return (
    <section
      className="workspace-content system-feature-page ops-panel mx-auto max-w-215"
      aria-label="全局运维总览"
    >
      {sidebarLayout ? (
        <SidebarWindowActions>
          <div className="grid gap-1.5">{persistentActions}</div>
        </SidebarWindowActions>
      ) : (
        <header className="system-feature-header">
        <span className="system-feature-header-icon bg-brand-50 text-brand-600 dark:bg-brand-900/40 dark:text-brand-300">
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <strong className="block text-sm font-semibold text-slate-900 dark:text-slate-100">
            运维总览
          </strong>
          <span className="block truncate text-xs text-slate-400 dark:text-slate-500">
            统一查看所有受管机器人和自动维护状态。
          </span>
        </div>
          <div className="flex shrink-0 gap-2">{persistentActions}</div>
        </header>
      )}
      {overview && (
        <div className="ops-metrics mx-2.5 mb-4 grid grid-cols-2 gap-3">
          {METRIC_ITEMS.map(({ key, label }) => (
            <div
              className="rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-3"
              key={key}
            >
              <small className="block text-xs text-(--theme-text-muted)">
                {label}
              </small>
              <strong className="block text-lg text-(--theme-text-strong)">
                {overview.metrics[key]}
              </strong>
            </div>
          ))}
        </div>
      )}
      <section className="mx-2.5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4">
        <h2 className="mb-3 text-sm font-semibold text-(--theme-text-strong)">
          受管项目
        </h2>
        <div className="grid gap-2">
          {projects.map(project => {
            const policy = overview?.policies.find(
              item => item.projectRoot === project.path
            )
            return (
              <button
                className="group flex w-full items-center justify-between gap-3 rounded-md border border-(--theme-border-subtle) px-3 py-2.5 text-left transition hover:border-(--theme-border-strong) hover:bg-(--theme-surface-hover)"
                key={project.id}
                onClick={() => onOpenProject(project.id)}
              >
                <span className="grid min-w-0 flex-1 gap-0.5">
                  <strong className="block truncate text-sm text-(--theme-text-strong)">
                    {project.name}
                  </strong>
                  <small className="block truncate text-xs text-(--theme-text-muted)">
                    {project.path}
                  </small>
                </span>
                <span className="shrink-0 text-xs text-(--theme-text-secondary)">
                  {policy?.mode ?? 'observe'} ·{' '}
                  {policy?.autoAllowed ? '白名单' : '未授权'}
                </span>
              </button>
            )
          })}
          {projects.length === 0 && (
            <p className="text-sm text-(--theme-text-muted)">暂无受管项目。</p>
          )}
        </div>
      </section>
      {overview?.nodeId && (
        <p className="mx-2.5 text-xs text-(--theme-text-faint)">
          当前执行节点：{overview.nodeId}
        </p>
      )}
      <ConfirmDialog
        open={confirmStop}
        title="紧急停止全部"
        subtitle="此操作会立即停止所有受管机器人的自动维护。"
        message="确定要对全部受管项目执行紧急停止吗？正在进行的分析、预览修复与自动维护任务都会中止。"
        confirmLabel="紧急停止"
        destructive
        busy={busy}
        onCancel={() => setConfirmStop(false)}
        onConfirm={() => {
          setConfirmStop(false)
          void control('emergency-stop')
        }}
      />
    </section>
  )
}
