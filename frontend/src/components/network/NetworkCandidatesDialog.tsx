import { useEffect } from 'react'
import { useStoreState } from '../../store/guideStore'
import {
  usePreviewSystemNetworkMutation,
  useSystemNetworkStatusQuery,
  type SystemNetworkDefinition,
  type SystemNetworkSettings
} from '../../store/workspaceApi'
import { FormDialog } from '../FormDialog'
import { Button } from '../Button'
import { networkInputClass, networkError } from './networkSettings'

export function NetworkCandidatesDialog({
  settings,
  group,
  onSave,
  onClose
}: {
  settings: SystemNetworkSettings
  group: SystemNetworkDefinition
  onSave: (next: SystemNetworkSettings) => Promise<void>
  onClose: () => void
}) {
  const [entries, setEntries] = useStoreState(
    settings.automatic.groups.find(g => g.id === group.id)?.candidates || []
  )
  const [task, setTask] = useStoreState('')
  const [polling, setPolling] = useStoreState(false)
  const [busy, setBusy] = useStoreState(false)
  const [error, setError] = useStoreState('')
  const [invalid, setInvalid] = useStoreState<Record<number, string>>({})
  const [preview, previewState] = usePreviewSystemNetworkMutation()
  const status = useSystemNetworkStatusQuery(task, {
    skip: !task,
    pollingInterval: polling ? 2000 : 0
  })
  const current = status.currentData?.groups.find(g => g.id === group.id)
  useEffect(() => {
    setPolling(current?.state === 'checking')
  }, [current, setPolling])
  const update = (next: string[]) => {
    setEntries(next)
    setTask('')
    setInvalid({})
    setError('')
  }
  const draft = (): SystemNetworkSettings | null => {
    const errors: Record<number, string> = {}
    entries.forEach((entry, i) => {
      const markers = entry.match(/\{[^}]+\}/g) || []
      const allowed = [
        '{url}',
        '{path}',
        ...(group.id === 'node' ? ['{nodepath}'] : []),
        ...(group.id === 'python' ? ['{pythonpath}'] : [])
      ]
      if (markers.length !== 1 || !allowed.includes(markers[0])) {
        errors[i] =
          '入口需包含一个路径占位符，例如 https://mirror.example{path}'
        return
      }
      try {
        const url = new URL(entry.replace(/\{[^}]+\}/g, 'probe'))
        if (
          !['https:', 'http:'].includes(url.protocol) ||
          url.username ||
          url.password ||
          url.hash
        )
          throw new Error()
      } catch {
        errors[i] = '请输入有效的 HTTP 或 HTTPS 入口'
      }
    })
    setInvalid(errors)
    if (Object.keys(errors).length) return null
    return {
      ...settings,
      automatic: {
        groups: settings.automatic.groups.map(g =>
          g.id === group.id
            ? { ...g, candidates: [...new Set(entries.map(v => v.trim()))] }
            : g
        )
      }
    }
  }
  const run = async (save: boolean) => {
    const next = draft()
    if (!next) return
    setError('')
    setBusy(true)
    try {
      if (save) {
        await onSave(next)
        onClose()
      } else {
        const value = await preview({
          target: group.id,
          settings: next
        }).unwrap()
        setTask(value.task)
        setPolling(true)
      }
    } catch (e) {
      setError(networkError(e))
    } finally {
      setBusy(false)
      previewState.reset()
    }
  }
  const feedback = (url: string) => {
    if (!task) return null
    const value = current?.candidates?.find(c => c.url === url)
    return (
      <p role="status" className="mt-1 text-xs">
        {status.isError
          ? '检测状态读取失败'
          : current?.state === 'checking' || !current
            ? '检测中…'
            : value
              ? `${value.message}${value.ok ? ' · ' + value.latencyMs + ' ms' : ''}`
              : '待检测'}
      </p>
    )
  }
  return (
    <FormDialog
      open
      title={`配置 ${group.label}`}
      busy={busy}
      onClose={onClose}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button loading={busy} onClick={() => void run(true)}>
            保存并应用
          </Button>
        </>
      }
    >
      <div className="min-w-0 rounded-md bg-(--theme-surface-raised) p-3">
        <p className="text-sm">官方地址 · 固定</p>
        <p className="mt-1 break-all text-xs text-(--theme-text-muted)">
          {group.probe}
        </p>
        {feedback('official')}
      </div>
      {entries.map((entry, i) => (
        <div key={i}>
          <div className="flex items-end gap-2">
            <label className="grid min-w-0 flex-1 gap-1 text-sm">
              候选入口 {i + 1}
              <input
                className={networkInputClass}
                disabled={busy}
                value={entry}
                aria-invalid={!!invalid[i]}
                onChange={e =>
                  update(
                    entries.map((v, index) =>
                      index === i ? e.target.value : v
                    )
                  )
                }
              />
            </label>
            <Button
              disabled={busy}
              aria-label={`删除候选入口 ${i + 1}`}
              onClick={() => update(entries.filter((_, index) => index !== i))}
            >
              删除
            </Button>
          </div>
          {invalid[i] && (
            <p role="alert" className="text-xs">
              {invalid[i]}
            </p>
          )}
          {feedback(entry)}
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        {group.mirrors && (
          <Button
            disabled={busy || entries.length >= 16}
            onClick={() => update([...entries, ''])}
          >
            添加入口
          </Button>
        )}
        <Button disabled={busy || polling} onClick={() => void run(false)}>
          测试候选
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-xs">
          {error}
        </p>
      )}
    </FormDialog>
  )
}
