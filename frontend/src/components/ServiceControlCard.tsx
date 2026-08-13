import { useCallback, useEffect, useState } from 'react'
import { Download, Power, RotateCcw, Server } from 'lucide-react'
import { Button } from './Button'
import { ConfirmDialog } from './ConfirmDialog'

type ServiceAction = 'install' | 'stop' | 'restart'

export function ServiceControlCard() {
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [serviceAction, setServiceAction] = useState<ServiceAction | null>(null)
  const [serviceStatus, setServiceStatus] = useState('')
  const [serviceInstalled, setServiceInstalled] = useState<boolean | null>(null)
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
      }
      if (!response.ok) throw new Error()
      setServiceStatus(result.status || '')
      setServiceInstalled(result.installed ?? null)
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
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '服务操作失败。')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="settings-panel-content grid gap-4">
      <section className="settings-service-card">
        <header>
          <i>
            <Server className="size-4" />
          </i>
          <span>
            <strong>AlemonX 服务</strong>
            <small>工作台后台进程</small>
          </span>
          <button
            className="text-button settings-service-refresh"
            onClick={() => void loadServiceStatus()}
            disabled={busy}
          >
            刷新状态
          </button>
        </header>
        <div className="settings-service-status" aria-live="polite">
          <span className={serviceStatusTone} aria-hidden="true" />
          <small>{serviceStatus || '正在读取服务状态…'}</small>
        </div>
        <footer>
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
          <Button
            variant="danger"
            className="gap-1.5"
            disabled={busy}
            onClick={() => setServiceAction('stop')}
          >
            <Power className="size-3.5" /> 停止
          </Button>
        </footer>
      </section>
      {message && (
        <small className="rounded-md bg-slate-50 p-2 text-[11px] leading-4 text-slate-500">
          {message}
        </small>
      )}
      <ConfirmDialog
        open={serviceAction !== null}
        title={
          serviceAction === 'install'
            ? '安装 AlemonX 后台服务'
            : serviceAction === 'stop'
              ? '停止 AlemonX 服务'
              : '重启 AlemonX 服务'
        }
        subtitle={
          serviceAction === 'install'
            ? ''
            : serviceAction === 'stop' && serviceInstalled === false
              ? '未安装后台守护服务；这会关闭当前前台运行的工作台服务。'
              : '仅影响工作台后台服务，不会停止机器人项目。'
        }
        message={
          serviceAction === 'install'
            ? '当前前台工作台会关闭，并切换为系统后台服务；页面恢复连接后会自动刷新。'
            : serviceAction === 'stop'
              ? serviceInstalled === false
                ? '当前页面会断开连接；之后可从启动应用的位置重新打开工作台。'
                : '服务停止后，工作台页面将无法继续连接，直到你从系统服务或命令行重新启动它。'
              : '工作台会短暂断开，服务恢复后可重新打开页面。'
        }
        confirmLabel={
          serviceAction === 'install'
            ? '安装并启动'
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
    </div>
  )
}
