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
  | 'auth'
  | 'accounts'
  | 'github'
  | 'network'
  | 'update'
  | 'service'
  | 'redis'

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
      setStopMessage(reason instanceof Error ? reason.message : '停止服务失败。')
    } finally {
      setStopBusy(false)
    }
  }

  return (
    <div className="app-settings-shell">
      <aside className="app-settings-sidebar" aria-label="设置分类">
        <div className="app-settings-nav" role="tablist" aria-label="设置页面">
          {sections.map(item => {
            const Icon = item.icon
            const selected = item.id === active
            return (
              <button
                className={selected ? 'active' : ''}
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
        <div className="app-settings-stop-wrap">
          <button
            className="app-settings-stop"
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
            <small className="app-settings-stop-message">{stopMessage}</small>
          )}
        </div>
      </aside>
      <section
        className="app-settings-content"
        id={activePanelID}
        role="tabpanel"
        aria-labelledby={`app-settings-tab-${active}`}
      >
        <div className="app-settings-body">
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
