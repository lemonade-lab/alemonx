import { useCallback, useEffect, useState } from 'react'
import {
  Download,
  Power,
  RefreshCw,
  RotateCcw,
  Server,
  ShieldCheck,
  Trash2
} from 'lucide-react'
import { Button } from './Button'
import { ConfirmDialog } from './ConfirmDialog'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

type ServiceAction =
  | 'install'
  | 'uninstall'
  | 'stop'
  | 'restart'
  | 'enable-linger'
  | 'enable-startup'
  | 'disable-startup'

type ServiceResilience = {
  startupEnabled: boolean
  keepAlive: boolean
  lingerSupported: boolean
  lingerKnown: boolean
  lingerEnabled: boolean
  summary: string
}

export function ServiceControlCard() {
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [serviceAction, setServiceAction] = useState<ServiceAction | null>(null)
  const [serviceStatus, setServiceStatus] = useState('')
  const [serviceInstalled, setServiceInstalled] = useState<boolean | null>(null)
  const [resilience, setResilience] = useState<ServiceResilience | null>(null)
  const serviceStatusTone =
    serviceInstalled === false
      ? 'is-offline'
      : serviceStatus.includes('运行中')
        ? 'is-ready'
        : 'is-idle'
  const loadServiceStatus = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/system/service', {
        cache: 'no-store'
      })
      const result = (await response.json()) as {
        status?: string
        installed?: boolean
        resilience?: ServiceResilience
      }
      if (!response.ok) throw new Error()
      setServiceStatus(result.status || '')
      setServiceInstalled(result.installed ?? null)
      setResilience(result.resilience ?? null)
    } catch {
      setServiceStatus('无法读取 AlemonX 服务状态。')
    }
  }, [])
  useEffect(() => {
    void loadServiceStatus()
  }, [loadServiceStatus])

  const api = async (path: string, options: RequestInit) => {
    const response = await fetch(path, options)
    const result = (await response.json()) as {
      output?: string
      error?: string
    }
    if (!response.ok) throw new Error(result.error || '操作未完成。')
    return result
  }

  const reconnectAfterServiceInstall = () => {
    const deadline = Date.now() + 20_000
    const retry = () => {
      void fetch('/healthz', { cache: 'no-store' })
        .then(response => {
          if (response.ok) {
            window.location.reload()
            return
          }
          throw new Error()
        })
        .catch(() => {
          if (Date.now() < deadline) window.setTimeout(retry, 500)
          else {
            setBusy(false)
            setMessage('后台服务启动超时，请刷新状态查看具体原因。')
          }
        })
    }
    window.setTimeout(retry, 900)
  }

  const reconnectAfterServiceRestart = () => {
    const deadline = Date.now() + 20_000
    const retry = () => {
      void fetch('/healthz', { cache: 'no-store' })
        .then(response => {
          if (response.ok) {
            window.location.reload()
            return
          }
          throw new Error()
        })
        .catch(() => {
          if (Date.now() < deadline) window.setTimeout(retry, 500)
          else {
            setBusy(false)
            setMessage('服务重启超时，请刷新状态查看具体原因。')
          }
        })
    }
    // The API response is sent before the old service is stopped. Starting
    // the first probe after that shutdown window avoids treating the old
    // process as a successful restart.
    window.setTimeout(retry, 900)
  }

  const manageService = async (action: ServiceAction) => {
    setBusy(true)
    setMessage('')
    try {
      const result = await api('/api/v1/system/service', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, confirm: true })
      })
      setMessage(result.output || '服务操作已提交。')
      if (action === 'install') reconnectAfterServiceInstall()
      else if (action === 'restart') reconnectAfterServiceRestart()
      else void loadServiceStatus()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '服务操作失败。')
    } finally {
      setBusy(false)
    }
  }

  return (
    <SettingsPage
      title="服务"
      description="管理 AlemonX 后台服务进程、开机保活与无登录运行。"
    >
      <SettingsCard
        icon={<Server className="size-4" />}
        title="AlemonX 服务"
        description="工作台后台进程"
        actions={
          <Button
            variant="ghost"
            className="gap-1"
            onClick={() => void loadServiceStatus()}
            disabled={busy}
          >
            刷新状态
          </Button>
        }
      >
        <div className="settings-service-status" aria-live="polite">
          <span className={serviceStatusTone} aria-hidden="true" />
          <small>{serviceStatus || '正在读取服务状态…'}</small>
        </div>
        <div className="settings-card-actions settings-card-actions-end">
          {serviceInstalled === false && (
            <Button
              variant="primary"
              className="gap-1.5"
              disabled={busy}
              onClick={() => setServiceAction('install')}
            >
              <Download className="size-3.5" /> 安装并启动
            </Button>
          )}
          {serviceInstalled === true && (
            <Button
              variant="secondary"
              className="gap-1.5"
              disabled={busy}
              onClick={() => setServiceAction('install')}
            >
              <RefreshCw className="size-3.5" /> 更新注册
            </Button>
          )}
          {serviceInstalled !== false && (
            <Button
              variant="secondary"
              className="gap-1.5"
              disabled={busy}
              onClick={() => setServiceAction('restart')}
            >
              <RotateCcw className="size-3.5" /> 重启
            </Button>
          )}
          {serviceInstalled === true && (
            <Button
              variant="danger"
              className="gap-1.5"
              disabled={busy}
              onClick={() => setServiceAction('uninstall')}
            >
              <Trash2 className="size-3.5" /> 卸载服务
            </Button>
          )}
          <Button
            variant="danger"
            className="gap-1.5"
            disabled={busy}
            onClick={() => setServiceAction('stop')}
          >
            <Power className="size-3.5" /> 停止
          </Button>
        </div>
      </SettingsCard>
      <SettingsCard
        icon={<ShieldCheck className="size-4" />}
        title="保活与开机恢复"
        description={resilience?.summary || '正在读取保活配置…'}
      >
        {serviceInstalled && resilience && (
          <div className="grid gap-1.5 text-xs text-(--theme-text-secondary)">
            <span>
              登录/开机启动：{resilience.startupEnabled ? '已配置' : '未配置'}
            </span>
            <span>
              异常退出自动拉起：{resilience.keepAlive ? '已配置' : '未配置'}
            </span>
            {resilience.lingerKnown && (
              <span>
                Linux 无登录运行：
                {resilience.lingerEnabled ? '已启用' : '未启用'}
              </span>
            )}
          </div>
        )}
        {serviceInstalled && resilience && (
          <Button
            variant={resilience.startupEnabled ? 'secondary' : 'primary'}
            className="w-fit gap-1.5"
            disabled={busy}
            title={
              resilience.startupEnabled
                ? '下次登录时不再自动启动，当前运行中的服务不受影响'
                : '登录后自动启动 ALemonX 服务'
            }
            onClick={() =>
              setServiceAction(
                resilience.startupEnabled ? 'disable-startup' : 'enable-startup'
              )
            }
          >
            <Power className="size-3.5" />
            {resilience.startupEnabled ? '关闭开机自启' : '开启开机自启'}
          </Button>
        )}
        {serviceInstalled &&
          resilience?.lingerSupported &&
          !resilience.lingerEnabled && (
            <Button
              variant="secondary"
              className="w-fit gap-1.5"
              disabled={busy}
              onClick={() => setServiceAction('enable-linger')}
            >
              <ShieldCheck className="size-3.5" /> 启用无登录运行
            </Button>
          )}
      </SettingsCard>
      {message && (
        <SettingsMessage
          tone={
            message.includes('失败') || message.includes('超时')
              ? 'error'
              : 'info'
          }
        >
          {message}
        </SettingsMessage>
      )}
      <ConfirmDialog
        open={serviceAction !== null}
        title={
          serviceAction === 'install'
            ? serviceInstalled
              ? '更新后台服务注册'
              : '安装 AlemonX 后台服务'
            : serviceAction === 'uninstall'
              ? '卸载 AlemonX 后台服务'
            : serviceAction === 'enable-startup'
              ? '开启开机自启'
              : serviceAction === 'disable-startup'
                ? '关闭开机自启'
                : serviceAction === 'enable-linger'
                  ? '启用 Linux 无登录运行'
                  : serviceAction === 'stop'
                    ? '停止 AlemonX 服务'
                    : '重启 AlemonX 服务'
        }
        subtitle={
          serviceAction === 'install'
            ? serviceInstalled
              ? '以当前运行的前台程序与工作区重新注册后台服务。'
              : ''
            : serviceAction === 'uninstall'
              ? '移除后台服务注册与开机自启，不删除工作台数据与机器人项目。'
            : serviceAction === 'enable-startup'
              ? '登录后自动启动 AlemonX 服务，异常退出仍会由系统拉起。'
              : serviceAction === 'disable-startup'
                ? '下次登录时不会自动启动；当前运行中的服务不受影响。'
                : serviceAction === 'enable-linger'
                  ? '此操作可能需要系统管理员授权。'
                  : serviceAction === 'stop' && serviceInstalled === false
                    ? '未安装后台守护服务；这会关闭当前前台运行的工作台服务。'
                    : '仅影响工作台后台服务，不会停止机器人项目。'
        }
        message={
          serviceAction === 'install'
            ? serviceInstalled
              ? '后台服务将按当前程序与工作区重新注册并自动切换；页面会短暂断开。'
              : '当前前台工作台会关闭，并切换为系统后台服务；页面恢复连接后会自动刷新。'
            : serviceAction === 'uninstall'
              ? '卸载后需要手动运行 alx 才能再次打开工作台；页面会断开连接。'
            : serviceAction === 'enable-linger'
              ? '启用后，Linux 重启或用户退出登录时，已安装的 ALemonX systemd 用户服务仍可自动运行。'
              : serviceAction === 'stop'
                ? serviceInstalled === false
                  ? '当前页面会断开连接；之后可从启动应用的位置重新打开工作台。'
                  : '服务停止后，工作台页面将无法继续连接，直到你从系统服务或命令行重新启动它。'
                : '工作台会短暂断开，服务恢复后可重新打开页面。'
        }
        confirmLabel={
          serviceAction === 'install'
            ? serviceInstalled
              ? '更新并重启'
              : '安装并启动'
            : serviceAction === 'uninstall'
              ? '确认卸载'
            : serviceAction === 'enable-linger'
              ? '确认启用'
              : serviceAction === 'stop'
                ? '确认停止'
                : '确认重启'
        }
        busy={busy}
        onCancel={() => setServiceAction(null)}
        onConfirm={() => {
          if (!serviceAction) return
          const action = serviceAction
          setServiceAction(null)
          void manageService(action)
        }}
      />
    </SettingsPage>
  )
}
