import { AlertTriangle, Archive, CheckCircle2, Database, Trash2, Wifi } from 'lucide-react'
import { useEffect, useState } from 'react'
import {
  useDeleteDependencySourceBackupMutation,
  useDependencySourceTaskQuery,
  useDependencySourcesQuery,
  useRemoveManagedDependencySourceMutation,
  useTestDependencySourceMutation
} from '../store/workspaceApi'
import { frontendBuildID } from '../buildInfo'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'
import { ConfirmDialog } from './ConfirmDialog'

function backupLabel(preset: string) {
  if (preset === 'before-restore') return '恢复前自动备份'
  if (preset.startsWith('apply-')) return `应用 ${preset.slice('apply-'.length)} 前的备份`
  return preset
}

export function DependencySourcesPanel() {
  const { data, isLoading, isError, refetch } = useDependencySourcesQuery()
  const [deleteBackup, { isLoading: deleting }] = useDeleteDependencySourceBackupMutation()
  const [removeManagedSource, { isLoading: removing }] = useRemoveManagedDependencySourceMutation()
  const [test, { isLoading: testing }] = useTestDependencySourceMutation()
  const [message, setMessage] = useState('')
  const [success, setSuccess] = useState(false)
  const [taskID, setTaskID] = useState('')
  const [deletingID, setDeletingID] = useState('')
  const [removeConfirm, setRemoveConfirm] = useState(false)
  const { data: task } = useDependencySourceTaskQuery(taskID, {
    skip: !taskID,
    pollingInterval: taskID ? 1000 : 0
  })
  useEffect(() => {
    if (!task || task.status === 'running') return
    setTaskID('')
    setSuccess(task.status === 'completed')
    setMessage(task.error || task.output || '依赖源操作已结束。')
    void refetch()
  }, [refetch, task])
  if (isLoading) return <div className="settings-network-panel">正在检查系统依赖源…</div>
  if (isError || !data) return <div className="settings-network-panel">无法读取系统依赖源状态。</div>
  const run = async (action: () => Promise<unknown>, ok: string) => {
    setMessage(''); setSuccess(false)
    try {
      const result = await action() as { id?: string }
      if (result.id) {
        setTaskID(result.id)
        setMessage('依赖源操作已加入队列，正在后台执行。')
        return
      }
      setSuccess(true); if (ok) setMessage(ok)
    } catch (error) {
      const detail = error as { data?: { error?: string } }
      setMessage(detail.data?.error || '操作失败，系统源未改变。')
    }
  }
  const testPreset = async (preset: string, name: string) => {
    setMessage(''); setSuccess(false)
    try {
      const result = await test({ preset }).unwrap()
      setSuccess(result.ok)
      setMessage(`${name}：${result.message}${result.latencyMs ? ` · ${result.latencyMs} ms` : ''}`)
    } catch (error) {
      const detail = error as { data?: { error?: string } }
      setMessage(detail.data?.error || '镜像检测失败。')
    }
  }
  return (
    <SettingsPage title="镜像连通性检查" description="仅检查经验证镜像的元数据可访问性；MVP 不会自动追加、切换或恢复系统仓库。">
      <SettingsCard icon={<Database className="size-4" />} title="当前环境" description={`${data.distribution} · ${data.architecture} · ${data.manager || '未检测到包管理器'}`}>
        <p className="text-xs text-(--theme-text-muted)">{data.reason || '当前系统暂不支持镜像检查。'}</p>
      </SettingsCard>
      {data.checksAvailable && (
        <SettingsCard icon={<Wifi className="size-4" />} title="镜像连通性" description="检测不会提权、写入配置或刷新系统包索引。">
          <div className="grid gap-2">
            {data.presets.map(preset => (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-(--theme-border) px-3 py-2" key={preset.id}>
                <div><strong className="block text-sm">{preset.name}</strong><small className="text-xs text-(--theme-text-muted)">{preset.description}</small></div>
                <div className="flex items-center gap-2">
                  <Button variant="secondary" loading={testing} loadingLabel="检测中…" onClick={() => void testPreset(preset.id, preset.name)}><Wifi className="size-3.5" />检测</Button>
                </div>
              </div>
            ))}
          </div>
        </SettingsCard>
      )}
      {data.cleanupAvailable && (
        <SettingsCard icon={<AlertTriangle className="size-4" />} title="移除受管源" description="当前系统的软件源存在兼容风险。移除只会删除 ALemonX 创建的固定文件，不会修改系统原有仓库。">
          <Button variant="danger" loading={removing} loadingLabel="移除中…" onClick={() => setRemoveConfirm(true)}><Trash2 className="size-3.5" />移除 ALemonX 受管源</Button>
        </SettingsCard>
      )}
      {data.sameNameUnmanaged && <SettingsMessage tone="error">检测到同名文件 {data.target}，但它不带 ALemonX 所有权标记。为保护你的系统，ALemonX 不会删除它。</SettingsMessage>}
      {(data.backups ?? []).length > 0 && (
        <SettingsCard icon={<Archive className="size-4" />} title="备份审计" description="备份保存清理前状态和校验和；为避免重新启用不兼容仓库，MVP 不提供恢复写入。">
          <div className="grid gap-2">{(data.backups ?? []).map(backup => <div className="flex flex-wrap items-center justify-between gap-3 text-xs" key={backup.id}><span>{new Date(backup.createdAt).toLocaleString()} · {backupLabel(backup.preset)}</span><Button variant="secondary" onClick={() => setDeletingID(backup.id)}><Trash2 className="size-3.5" />删除</Button></div>)}</div>
        </SettingsCard>
      )}
      {task && <SettingsMessage tone="info">{task.progress}% · {task.output || '正在执行依赖源操作…'}</SettingsMessage>}
      {message && <SettingsMessage tone={success ? 'success' : 'error'}>{success && <CheckCircle2 className="mr-1 inline size-4" />}{message}</SettingsMessage>}
      {data.frontendBuild && data.serverBuild && data.frontendBuild !== frontendBuildID && <SettingsMessage tone="error">浏览器资源与服务端构建不一致（页面 {frontendBuildID}，服务端嵌入资源 {data.frontendBuild}）。请强制刷新页面；若仍存在，重新部署并重启新二进制。</SettingsMessage>}
      <ConfirmDialog open={Boolean(deletingID)} title="删除依赖源备份" message="删除后无法恢复该备份。确认继续吗？" destructive busy={deleting} onCancel={() => setDeletingID('')} onConfirm={() => { const id = deletingID; setDeletingID(''); void run(() => deleteBackup({ id }).unwrap(), '备份已删除。') }} />
      <ConfirmDialog open={removeConfirm} title="移除 ALemonX 受管依赖源" message="这会删除 ALemonX 创建的依赖源文件，系统将继续使用原有软件源。不会修改系统原有仓库。" destructive busy={removing} onCancel={() => setRemoveConfirm(false)} onConfirm={() => { setRemoveConfirm(false); void run(() => removeManagedSource().unwrap(), '已移除 ALemonX 受管依赖源。') }} />
    </SettingsPage>
  )
}
