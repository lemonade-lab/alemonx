import { RefreshButton } from './RefreshButton'
import { useEffect, useRef, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { frontendBuildID } from '../buildInfo'
import { bridge, markNativeRuntimeReady } from './nativeWindow'
import { Button } from './Button'
import { ConfirmDialog } from './ConfirmDialog'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

type DesktopUpdate = {
  action?: 'status' | UpdateAction
  current: string
  latest?: string
  asset?: string
  ready: boolean
  error?: string
}

type UpdateAction = 'check' | 'download' | 'install'

const actionTimeouts: Record<UpdateAction, number> = {
  check: 25_000,
  download: 22 * 60_000,
  install: 30_000
}

const displayVersion = (version: string) =>
  version === 'dev' ? '开发版' : version

const actionMessages: Record<UpdateAction, string> = {
  check: '正在检查更新…',
  download: '正在下载并校验桌面包…',
  install: '正在退出应用并安装更新…'
}

export function DesktopUpdatePanel() {
  const [state, setState] = useState<DesktopUpdate>()
  const [busy, setBusy] = useState<UpdateAction>()
  const [confirm, setConfirm] = useState(false)
  const [error, setError] = useState('')
  const timer = useRef<number | undefined>(undefined)
  const pending = useRef<UpdateAction | undefined>(undefined)

  useEffect(() => {
    const onResult = (event: Event) => {
      const result = (event as CustomEvent<DesktopUpdate>).detail
      if (pending.current && result.action && result.action !== pending.current) return
      window.clearTimeout(timer.current)
      pending.current = undefined
      setState(result)
      setError(result.error ?? '')
      setBusy(undefined)
    }
    window.addEventListener('alx:desktop-update', onResult)
    markNativeRuntimeReady()
    bridge?.postMessage('alx:desktop-update:status')
    return () => {
      window.clearTimeout(timer.current)
      window.removeEventListener('alx:desktop-update', onResult)
    }
  }, [])

  const run = (action: UpdateAction) => {
    pending.current = action
    setError('')
    setBusy(action)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      pending.current = undefined
      setBusy(undefined)
      setError('桌面更新程序未响应，请稍后重试。')
    }, actionTimeouts[action])
    bridge?.postMessage(`alx:desktop-update:${action}`)
  }

  return (
    <SettingsPage title="更新">
      <SettingsCard
        icon={<RefreshCw size={16} />}
        title="ALemonX Desktop"
        description={`当前版本 ${displayVersion(state?.current || frontendBuildID)}`}
        actions={
          <RefreshButton
            label="检查更新"
            onClick={() => run('check')}
            disabled={Boolean(busy)}
            busy={busy === 'check'}
          />
        }
      >
        <div role={error && !busy ? 'alert' : 'status'}>
          {busy ? (
            <SettingsMessage>{actionMessages[busy]}</SettingsMessage>
          ) : error ? (
            <SettingsMessage tone="error">{error}</SettingsMessage>
          ) : state?.ready ? (
            <SettingsMessage tone="success">更新包已校验，可以安装。</SettingsMessage>
          ) : state?.latest ? (
            <SettingsMessage>
              {state.asset ? `发现新版本 ${state.latest}` : '当前没有可用更新'}
            </SettingsMessage>
          ) : null}
        </div>
        {state?.asset && (
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button
              variant="primary"
              disabled={Boolean(busy)}
              onClick={() => state.ready ? setConfirm(true) : run('download')}
            >
              {state.ready ? '安装并重启' : '下载桌面更新包'}
            </Button>
          </div>
        )}
      </SettingsCard>
      <ConfirmDialog
        open={confirm}
        title="安装桌面更新"
        message="应用会退出，安装新版本后自动重新打开。"
        confirmLabel="安装并重启"
        onCancel={() => setConfirm(false)}
        onConfirm={() => {
          setConfirm(false)
          run('install')
        }}
      />
    </SettingsPage>
  )
}
