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
                role="tab"
                type="button"
                aria-selected={selected}
                onClick={() => setActive(item.id)}
              >
                <Icon className="size-4" />
                <span>{item.label}</span>
              </button>
            )
          })}
        </div>
      </aside>
      <section className="app-settings-content" role="tabpanel">
        <div className="app-settings-body">
          {active === 'update' && <SetupUpdateButton embedded />}
          {active === 'auth' && <AuthControl embedded />}
          {active === 'accounts' && <AccountManagementPage />}
        </div>
      </section>
    </div>
  )
}
