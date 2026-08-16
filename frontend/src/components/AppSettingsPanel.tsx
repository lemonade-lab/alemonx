import { useState } from 'react'
import {
  Database,
  HardDrive,
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
import { WorkspaceSettingsCard } from './WorkspaceSettingsCard'
import { ConfirmDialog } from './ConfirmDialog'
import { RedisSettingsPanel } from './RedisSettingsPanel'
import { SidebarWindow, type SidebarWindowItem } from './SidebarWindow'
import type { DesktopWindowProps } from './DesktopWindow'

type SettingsSection =
  | 'auth'
  | 'accounts'
  | 'github'
  | 'network'
  | 'update'
  | 'workspace'
  | 'service'
  | 'redis'

const sections: SidebarWindowItem<SettingsSection>[] = [
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
    id: 'workspace',
    label: '工作区',
    icon: HardDrive
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

export function AppSettingsPanel({
  ...windowProps
}: Omit<DesktopWindowProps, 'children'>) {
  const [active, setActive] = useState<SettingsSection>('update')
  const [stopBusy, setStopBusy] = useState(false)
  const [stopConfirm, setStopConfirm] = useState(false)
  const [stopMessage, setStopMessage] = useState('')
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
    <SidebarWindow
      {...windowProps}
      activeItem={active}
      items={sections}
      onActiveItemChange={setActive}
      sidebarAriaLabel="设置页面"
      sidebarFooter={
        <>
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
        </>
      }
    >
      {active === 'update' && <SetupUpdateButton embedded />}
      {active === 'workspace' && <WorkspaceSettingsCard />}
      {active === 'network' && <NetworkSettingsPanel />}
      {active === 'github' && <GithubSettingsPanel />}
      {active === 'auth' && <AuthControl embedded />}
      {active === 'accounts' && <AccountManagementPage />}
      {active === 'service' && <ServiceControlCard />}
      {active === 'redis' && <RedisSettingsPanel />}
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
    </SidebarWindow>
  )
}
