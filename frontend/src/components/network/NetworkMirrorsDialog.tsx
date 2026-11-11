import { ArrowDown, ArrowUp } from 'lucide-react'
import { useStoreState } from '../../store/guideStore'
import {
  useTestSystemNetworkMirrorMutation,
  type SystemNetworkDefinition,
  type SystemNetworkSettings
} from '../../store/workspaceApi'
import { FormDialog } from '../FormDialog'
import { Button } from '../Button'
import { networkInputClass, networkError } from './networkSettings'

export function NetworkMirrorsDialog({
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
  const [testing, setTesting] = useStoreState<string | null>(null)
  const [feedback, setFeedback] = useStoreState<Record<string, string>>({})
  const [saving, setSaving] = useStoreState(false)
  const [error, setError] = useStoreState('')
  const [testMirror] = useTestSystemNetworkMirrorMutation()
  const busy = saving || testing !== null

  const update = (next: string[]) => {
    setEntries(next)
    setFeedback({})
    setError('')
  }
  const move = (from: number, to: number) => {
    const next = [...entries]
    next[from] = entries[to]
    next[to] = entries[from]
    update(next)
  }
  const test = async (key: string, mirror: string) => {
    setTesting(key)
    try {
      const result = await testMirror({ target: group.id, mirror }).unwrap()
      const message = result.ok
        ? `${result.message} · ${result.latencyMs} ms`
        : result.message
      setFeedback(previous => ({ ...previous, [key]: message }))
    } catch (reason) {
      setFeedback(previous => ({ ...previous, [key]: networkError(reason) }))
    } finally {
      setTesting(null)
    }
  }
  const save = async () => {
    setSaving(true)
    setError('')
    try {
      await onSave({
        ...settings,
        automatic: {
          ...settings.automatic,
          groups: settings.automatic.groups.map(item =>
            item.id === group.id
              ? { ...item, candidates: entries.map(entry => entry.trim()) }
              : item
          )
        }
      })
      onClose()
    } catch (reason) {
      setError(networkError(reason))
    } finally {
      setSaving(false)
    }
  }

  const result = (key: string) => {
    const message = testing === key ? '测试中…' : feedback[key]
    return (
      message && (
        <p role="status" className="mt-1 text-xs">
          {message}
        </p>
      )
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
          <Button
            loading={saving}
            disabled={busy || entries.some(entry => !entry.trim())}
            onClick={() => void save()}
          >
            保存并应用
          </Button>
        </>
      }
    >
      <p className="text-sm text-(--theme-text-secondary)">
        按从上到下的顺序尝试镜像，全部不可用时访问官方源。
      </p>
      {entries.map((entry, i) => (
        <div key={i} role="group" aria-label={`镜像 ${i + 1}`}>
          <label className="grid gap-1 text-sm">
            镜像 {i + 1}
            <input
              className={networkInputClass}
              disabled={busy}
              value={entry}
              placeholder="https://mirror.example"
              onChange={event =>
                update(
                  entries.map((value, index) =>
                    index === i ? event.target.value : value
                  )
                )
              }
            />
          </label>
          <div className="mt-1 flex flex-wrap gap-1">
            <Button
              variant="icon"
              disabled={busy || i === 0}
              aria-label={`上移镜像 ${i + 1}`}
              onClick={() => move(i, i - 1)}
            >
              <ArrowUp className="size-4" aria-hidden="true" />
            </Button>
            <Button
              variant="icon"
              disabled={busy || i === entries.length - 1}
              aria-label={`下移镜像 ${i + 1}`}
              onClick={() => move(i, i + 1)}
            >
              <ArrowDown className="size-4" aria-hidden="true" />
            </Button>
            <Button
              disabled={busy || !entry.trim()}
              aria-label={`测试镜像 ${i + 1}`}
              onClick={() => void test(String(i), entry)}
            >
              测试
            </Button>
            <Button
              disabled={busy}
              aria-label={`删除镜像 ${i + 1}`}
              onClick={() => update(entries.filter((_, index) => index !== i))}
            >
              删除
            </Button>
          </div>
          {result(String(i))}
        </div>
      ))}
      {group.mirrors && (
        <div>
          <Button
            disabled={busy || entries.length >= 16}
            onClick={() => update([...entries, ''])}
          >
            添加镜像
          </Button>
          <details className="mt-2 text-xs text-(--theme-text-secondary)">
            <summary>地址填写方式</summary>
            <p className="mt-1">
              填写镜像的基础地址，系统会自动拼接资源路径。转发完整原始网址的镜像可填写{' '}
              <code>https://mirror.example/{'{url}'}</code>。
            </p>
          </details>
        </div>
      )}
      <div className="min-w-0 rounded-md bg-(--theme-surface-raised) p-3">
        <div className="flex items-center justify-between gap-2">
          <p className="text-sm">官方源 · 最后回退</p>
          <Button
            disabled={busy}
            aria-label="测试官方源"
            onClick={() => void test('official', 'official')}
          >
            测试
          </Button>
        </div>
        <p className="mt-1 break-all text-xs text-(--theme-text-muted)">
          测试资源 {group.probe}
        </p>
        {result('official')}
      </div>
      {error && (
        <p role="alert" className="text-xs">
          {error}
        </p>
      )}
    </FormDialog>
  )
}
