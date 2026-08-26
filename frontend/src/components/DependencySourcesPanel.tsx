import { Archive, CheckCircle2, Database, RotateCcw, Save, Trash2, Wifi } from 'lucide-react'
import { useEffect, useState } from 'react'
import {
  useApplyDependencySourceMutation,
  useDeleteDependencySourceBackupMutation,
  useDependencySourceTaskQuery,
  useDependencySourcesQuery,
  useRestoreDependencySourceMutation,
  useTestDependencySourceMutation
} from '../store/workspaceApi'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'
import { ConfirmDialog } from './ConfirmDialog'

function backupLabel(preset: string) {
  if (preset === 'before-restore') return '恢复前自动备份'
  if (preset.startsWith('apply-')) return `应用 ${preset.slice('apply-'.length)} 前的备份`
  return preset
}

function presetLabel(preset?: string) {
  return ({ aliyun: '阿里云', tencent: '腾讯云', official: '官方源' } as Record<string, string>)[preset ?? ''] || '未启用 ALemonX 管理源'
}

export function DependencySourcesPanel() {
  const { data, isLoading, isError, refetch } = useDependencySourcesQuery()
  const [apply, { isLoading: applying }] = useApplyDependencySourceMutation()
  const [restore, { isLoading: restoring }] = useRestoreDependencySourceMutation()
  const [deleteBackup, { isLoading: deleting }] = useDeleteDependencySourceBackupMutation()
  const [test, { isLoading: testing }] = useTestDependencySourceMutation()
  const [message, setMessage] = useState('')
  const [success, setSuccess] = useState(false)
  const [taskID, setTaskID] = useState('')
  const [deletingID, setDeletingID] = useState('')
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
    <SettingsPage title="依赖源" description="为系统包管理器增加 ALemonX 管理的镜像入口。原有系统源不会被覆盖，所有变更都可恢复。">
      <SettingsCard icon={<Database className="size-4" />} title="当前环境" description={`${data.distribution} · ${data.architecture} · ${data.manager || '未检测到包管理器'}`}>
        <p className="text-xs text-(--theme-text-muted)">{data.writable ? `当前：${presetLabel(data.activePreset)} · 管理文件：${data.target}` : (data.reason || '当前系统暂不支持自动改源。')}</p>
      </SettingsCard>
      {data.writable && (
        <SettingsCard icon={<Save className="size-4" />} title="选择镜像" description="先检测仓库元数据是否可访问；应用前会保存上一份 ALemonX 管理文件。">
          <div className="grid gap-2">
            {data.presets.map(preset => (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-(--theme-border) px-3 py-2" key={preset.id}>
                <div><strong className="block text-sm">{preset.name}</strong><small className="text-xs text-(--theme-text-muted)">{preset.description}</small></div>
                <div className="flex items-center gap-2">
                  <Button variant="secondary" loading={testing} loadingLabel="检测中…" onClick={() => void testPreset(preset.id, preset.name)}><Wifi className="size-3.5" />检测</Button>
                  <Button variant={preset.id === 'official' ? 'secondary' : 'primary'} loading={applying} loadingLabel="应用中…" onClick={() => void run(() => apply({ preset: preset.id }).unwrap(), `已应用${preset.name}依赖源。`)}>应用</Button>
                </div>
              </div>
            ))}
          </div>
        </SettingsCard>
      )}
      {data.backups.length > 0 && (
        <SettingsCard icon={<Archive className="size-4" />} title="备份与恢复" description="恢复会将对应备份写回 ALemonX 管理文件，不会删除备份。">
          <div className="grid gap-2">{data.backups.map(backup => <div className="flex flex-wrap items-center justify-between gap-3 text-xs" key={backup.id}><span>{new Date(backup.createdAt).toLocaleString()} · {backupLabel(backup.preset)}</span><span className="flex gap-2"><Button variant="secondary" loading={restoring} onClick={() => void run(() => restore({ id: backup.id }).unwrap(), '已恢复依赖源备份。')}><RotateCcw className="size-3.5" />恢复</Button><Button variant="secondary" onClick={() => setDeletingID(backup.id)}><Trash2 className="size-3.5" />删除</Button></span></div>)}</div>
        </SettingsCard>
      )}
      {task && <SettingsMessage tone="info">{task.progress}% · {task.output || '正在执行依赖源操作…'}</SettingsMessage>}
      {message && <SettingsMessage tone={success ? 'success' : 'error'}>{success && <CheckCircle2 className="mr-1 inline size-4" />}{message}</SettingsMessage>}
      <ConfirmDialog open={Boolean(deletingID)} title="删除依赖源备份" message="删除后无法恢复该备份。确认继续吗？" destructive busy={deleting} onCancel={() => setDeletingID('')} onConfirm={() => { const id = deletingID; setDeletingID(''); void run(() => deleteBackup({ id }).unwrap(), '备份已删除。') }} />
    </SettingsPage>
  )
}
