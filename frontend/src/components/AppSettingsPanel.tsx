import { useState } from 'react'
import { KeyRound, RefreshCw, UsersRound } from 'lucide-react'
import { AccountManagementPage } from './AccountManagement'
import { AuthControl } from './AuthControl'
import { SetupUpdateButton } from './SetupUpdateButton'

type SettingsSection = 'update' | 'auth' | 'accounts'

const sections: Array<{
  id: SettingsSection
  label: string
  icon: typeof RefreshCw
}> = [
  {
    id: 'update',
    label: '更新',
    icon: RefreshCw
  },
  {
    id: 'auth',
    label: '认证',
    icon: KeyRound
  },
  {
    id: 'accounts',
    label: '账户',
    icon: UsersRound
  }
]

export function AppSettingsPanel() {
  const [active, setActive] = useState<SettingsSection>('update')
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
      </aside>
      <section
        className="app-settings-content"
        id={activePanelID}
        role="tabpanel"
        aria-labelledby={`app-settings-tab-${active}`}
      >
        <div className="app-settings-body">
          {active === 'update' && <SetupUpdateButton embedded />}
          {active === 'auth' && <AuthControl embedded />}
          {active === 'accounts' && <AccountManagementPage />}
        </div>
      </section>
    </div>
  )
}
