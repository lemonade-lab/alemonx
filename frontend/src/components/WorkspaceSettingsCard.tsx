import { useCallback, useState } from 'react'
import { FolderOpen, HardDrive } from 'lucide-react'
import { useWorkspaceQuery } from '../store/workspaceApi'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

type WorkspaceRow = { label: string; path: string }

export function WorkspaceSettingsCard() {
  const { data: workspace } = useWorkspaceQuery()
  const [busyPath, setBusyPath] = useState('')
  const [message, setMessage] = useState('')

  const openDirectory = useCallback(async (path: string) => {
    setBusyPath(path)
    setMessage('')
    try {
      const response = await fetch('/api/v1/workspace/open', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path })
      })
      const result = (await response.json()) as {
        output?: string
        error?: string
      }
      if (!response.ok) throw new Error(result.error || '打开目录失败。')
      setMessage(result.output || '已交给系统打开。')
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '打开目录失败。')
    } finally {
      setBusyPath('')
    }
  }, [])

  const rows: WorkspaceRow[] = workspace
    ? [
        { label: '工作区根目录', path: workspace.root },
        { label: '模板（templates）', path: workspace.templates },
        { label: '机器人（bots）', path: workspace.bots },
        { label: '工具（packages）', path: workspace.packages }
      ]
    : []

  const refreshable =
    workspace?.refreshable?.templates ||
    Object.values(workspace?.refreshable?.packages ?? {}).some(Boolean)

  return (
    <SettingsPage
      title="工作区"
      description="统一工作区的位置与内置工具状态；模板与工具来自内嵌资源，按需物化到这里。"
    >
      <SettingsCard
        icon={<HardDrive className="size-4" />}
        title="目录位置"
        description={
          refreshable
            ? '有可刷新的内置资源（不会自动覆盖你的修改）。'
            : '所有目录都解析为绝对路径，不依赖当前运行位置。'
        }
      >
        <div className="grid gap-2 text-xs text-(--theme-text-secondary)">
          {rows.map(row => (
            <div
              key={row.label}
              className="flex items-center justify-between gap-3"
            >
              <span className="min-w-0">
                <strong className="block text-(--theme-text-strong)">
                  {row.label}
                </strong>
                <span className="block truncate">{row.path}</span>
              </span>
              <Button
                variant="secondary"
                size="sm"
                className="shrink-0 gap-1.5"
                disabled={busyPath !== ''}
                onClick={() => openDirectory(row.path)}
              >
                <FolderOpen className="size-3.5" /> 打开
              </Button>
            </div>
          ))}
          {rows.length === 0 && <span>正在读取工作区…</span>}
        </div>
        {workspace?.refreshable?.templates && (
          <p className="mt-2 text-xs text-(--theme-warning)">
            项目模板有可刷新版本；刷新不会覆盖你修改过的模板文件。
          </p>
        )}
        {Object.entries(workspace?.refreshable?.packages ?? {}).map(
          ([name, outdated]) =>
            outdated ? (
              <p key={name} className="mt-1 text-xs text-(--theme-warning)">
                内置 {name} 有可刷新版本；刷新不会覆盖现有副本。
              </p>
            ) : null
        )}
      </SettingsCard>
      {message && (
        <SettingsMessage
          tone={message.includes('失败') ? 'error' : 'info'}
        >
          {message}
        </SettingsMessage>
      )}
    </SettingsPage>
  )
}
