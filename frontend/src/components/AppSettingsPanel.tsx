import { useState, type ComponentType } from 'react'
import {
  Database,
  KeyRound,
  Network,
  Power,
  RefreshCw,
  Server,
  UsersRound
} from 'lucide-react'
import { AccountManagementPage } from './AccountManagement'
import { AuthControl } from './AuthControl'
import { SetupUpdateButton } from './SetupUpdateButton'
import { NetworkSettingsPanel } from './NetworkSettingsPanel'
import { GithubSettingsPanel } from './GithubSettingsPanel'
import { GithubMark } from './GithubMark'
import { ServiceControlCard } from './ServiceControlCard'
import { ConfirmDialog } from './ConfirmDialog'
import { RedisSettingsPanel } from './RedisSettingsPanel'

type SettingsSection =
  'auth' | 'accounts' | 'github' | 'network' | 'update' | 'service' | 'redis'

const sections: Array<{
  id: SettingsSection
  label: string
  icon: ComponentType<{ className?: string }>
}> = [
  {
    id: 'auth',
    label: '认证',
    icon: KeyRound
  },
  {
    id: 'accounts',
    label: '账户',
    icon: UsersRound
  },
  {
    id: 'github',
    label: 'GitHub',
    icon: GithubMark
  },
  {
    id: 'network',
    label: '网络',
    icon: Network
  },
  {
    id: 'update',
    label: '更新',
    icon: RefreshCw
  },
  {
    id: 'service',
    label: '服务',
    icon: Server
  },
  {
    id: 'redis',
    label: 'Redis',
    icon: Database
  }
]

export function AppSettingsPanel() {
  const [active, setActive] = useState<SettingsSection>('update')
  const [stopBusy, setStopBusy] = useState(false)
  const [stopConfirm, setStopConfirm] = useState(false)
  const [stopMessage, setStopMessage] = useState('')
  const activePanelID = `app-settings-panel-${active}`
  const activateTab = (next: SettingsSection) => {
    setActive(next)
    window.requestAnimationFrame(() =>
      document.getElementById(`app-settings-tab-${next}`)?.focus()
    )
  }
  const moveTab = (current: SettingsSection, direction: number) => {
    const currentIndex = sections.findIndex(item => item.id === current)
    const nextIndex =
      (currentIndex + direction + sections.length) % sections.length
    const next = sections[nextIndex].id
    activateTab(next)
  }
  const stopService = async () => {
    setStopBusy(true)
    setStopMessage('')
    try {
      const response = await fetch('/api/v1/system/service', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'stop', confirm: true })
      })
      const result = (await response.json()) as {
        output?: string
        error?: string
      }
      if (!response.ok) throw new Error(result.error || '停止服务失败。')
      setStopMessage(result.output || '服务已停止，工作台即将断开连接。')
    } catch (reason) {
      setStopMessage(
        reason instanceof Error ? reason.message : '停止服务失败。'
      )
    } finally {
      setStopBusy(false)
    }
  }

  return (
    <div
      className="grid h-full min-h-0 grid-cols-[176px_minmax(0,1fr)] bg-(--theme-surface-panel)"
      data-app-settings-shell
    >
      <aside
        className="flex flex-col gap-2 border-r border-(--theme-border-default) bg-(--theme-surface-raised) px-3 py-4.5"
        data-app-settings-sidebar
        aria-label="设置分类"
      >
        <div
          className="grid gap-1"
          data-app-settings-nav
          role="tablist"
          aria-label="设置页面"
        >
          {sections.map(item => {
            const Icon = item.icon
            const selected = item.id === active
            return (
              <button
                className={`flex min-h-9 items-center gap-2 rounded-lg border px-2.5 text-left text-xs font-semibold transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) ${selected ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-text)' : 'border-transparent bg-transparent text-(--theme-text-secondary) hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong)'}`}
                key={item.id}
                id={`app-settings-tab-${item.id}`}
                role="tab"
                type="button"
                aria-selected={selected}
                aria-controls={`app-settings-panel-${item.id}`}
                tabIndex={selected ? 0 : -1}
                onClick={() => setActive(item.id)}
                onKeyDown={event => {
                  if (event.key === 'ArrowDown' || event.key === 'ArrowRight') {
                    event.preventDefault()
                    moveTab(item.id, 1)
                  }
                  if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') {
                    event.preventDefault()
                    moveTab(item.id, -1)
                  }
                  if (event.key === 'Home') {
                    event.preventDefault()
                    activateTab(sections[0].id)
                  }
                  if (event.key === 'End') {
                    event.preventDefault()
                    activateTab(sections[sections.length - 1].id)
                  }
                }}
              >
                <Icon className="size-4" />
                <span>{item.label}</span>
              </button>
            )
          })}
        </div>
        <div className="mt-auto grid gap-1.5 border-t border-(--theme-border-default) pt-2.5">
          <button
            className="flex min-h-9 items-center gap-2 rounded-lg border border-transparent bg-transparent px-2.5 text-left text-xs font-semibold text-(--theme-danger) transition hover:border-(--theme-danger) hover:bg-(--theme-danger-soft) disabled:cursor-not-allowed disabled:opacity-55"
            type="button"
            onClick={() => setStopConfirm(true)}
            disabled={stopBusy}
            aria-label="停止 AlemonX 服务"
            title="停止 AlemonX 服务并关闭工作台"
          >
            <Power className="size-4" />
            <span>停止</span>
          </button>
          {stopMessage && (
            <small className="px-0.5 text-[11px] leading-snug text-(--theme-text-muted)">
              {stopMessage}
            </small>
          )}
        </div>
      </aside>
      <section
        className="min-h-0 bg-(--theme-surface-panel)"
        id={activePanelID}
        role="tabpanel"
        aria-labelledby={`app-settings-tab-${active}`}
      >
        <div
          className="h-full min-h-0 overflow-auto px-6 py-5 pb-7 [&_.account-management]:mx-auto [&_.account-management]:max-w-190 [&_.account-management_[data-robot-panel-header]]:hidden"
          data-app-settings-body
        >
          {active === 'update' && <SetupUpdateButton embedded />}
          {active === 'network' && <NetworkSettingsPanel />}
          {active === 'github' && <GithubSettingsPanel />}
          {active === 'auth' && <AuthControl embedded />}
          {active === 'accounts' && <AccountManagementPage />}
          {active === 'service' && <ServiceControlCard />}
          {active === 'redis' && <RedisSettingsPanel />}
        </div>
      </section>
      <ConfirmDialog
        open={stopConfirm}
        title="停止 AlemonX"
        message="确认停止 AlemonX 服务并关闭工作台连接吗？"
        confirmLabel="确认"
        busy={stopBusy}
        onCancel={() => setStopConfirm(false)}
        onConfirm={() => {
          setStopConfirm(false)
          void stopService()
        }}
      />
    </div>
  )
}
