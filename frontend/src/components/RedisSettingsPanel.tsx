import { useEffect, useState } from 'react'
import {
  Check,
  Copy,
  Database,
  Play,
  Power,
  RefreshCw,
  RotateCcw,
  Save
} from 'lucide-react'
import {
  useControlSystemRedisMutation,
  useSaveSystemRedisConfigMutation,
  useSystemRedisQuery
} from '../store/workspaceApi'
import { Button } from './Button'
import {
  SettingsCard,
  SettingsMessage,
  SettingsPage,
  SettingsSwitch
} from './SettingsCard'

function messageFrom(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'data' in error) {
    const data = (error as { data?: { error?: string } }).data
    if (data?.error) return data.error
  }
  return fallback
}

export function RedisSettingsPanel() {
  const { data, isLoading, isError, refetch } = useSystemRedisQuery()
  const [control, { isLoading: controlling }] = useControlSystemRedisMutation()
  const [saveConfig, { isLoading: saving }] =
    useSaveSystemRedisConfigMutation()
  const [port, setPort] = useState('6379')
  const [autoStart, setAutoStart] = useState(false)
  const [disabled, setDisabled] = useState(false)
  const [changed, setChanged] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!data || changed) return
    setPort(String(data.port))
    setAutoStart(data.autoStart)
    setDisabled(data.disabled)
  }, [data, changed])

  if (isLoading) {
    return <div className="settings-panel-content">正在读取 Redis 状态…</div>
  }
  if (isError || !data) {
    return <div className="settings-panel-content">无法读取 Redis 状态。</div>
  }

  const statusTone = data.disabled ? 'is-offline' : data.running ? 'is-ready' : 'is-idle'
  const statusLabel = data.disabled
    ? '已禁用'
    : data.running
      ? data.privateRunning
        ? '运行中 · 应用私有 Redis'
        : data.managed
          ? '运行中 · 内置持久化 Redis'
        : data.nativeRunning
          ? '运行中 · 独立 Redis（systemd）'
          : '使用中 · 外部 Redis'
      : '未运行'

  const runAction = async (
    action: 'start' | 'stop' | 'restart' | 'install-native'
  ) => {
    setMessage('')
    setCopied(false)
    try {
      await control(action).unwrap()
    } catch (error) {
      setMessage(messageFrom(error, 'Redis 操作未完成。'))
    }
  }

  const save = async () => {
    const parsed = Number(port)
    setMessage('')
    setCopied(false)
    if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535) {
      setMessage('端口需要在 1-65535 之间。')
      return
    }
    try {
      const next = await saveConfig({
        port: parsed,
        autoStart,
        disabled
      }).unwrap()
      setPort(String(next.port))
      setDisabled(next.disabled)
      setChanged(false)
      setMessage(next.disabled ? '' : '')
    } catch (error) {
      setMessage(messageFrom(error, 'Redis 配置未保存。'))
    }
  }

  const copyAddress = async () => {
    try {
      await navigator.clipboard.writeText(`redis://${data.address}`)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      setMessage('无法复制，请手动复制地址。')
    }
  }

  return (
    <SettingsPage
      title="Redis"
      description="工作台内置的持久化 Redis 服务，供机器人与插件使用。"
    >
      <SettingsCard
        icon={<Database className="size-4" />}
        title={data.privateRunning ? 'Redis 应用私有服务' : data.nativeRunning ? 'Redis 独立服务' : 'Redis 内置服务'}
        description={
          data.privateRunning
            ? '由 ALemonX 受控启动与关闭，数据持久化在应用目录。'
            : data.nativeRunning
            ? '由 Linux systemd 独立管理，ALemonX 重启不会影响 Redis。'
            : '工作台内置的持久化 Redis 服务，供机器人与插件使用。'
        }
        actions={
          <Button
            variant="ghost"
            className="gap-1"
            onClick={() => void refetch()}
            disabled={controlling}
          >
            刷新状态
          </Button>
        }
      >
        <div className="settings-service-status" aria-live="polite">
          <span className={statusTone} aria-hidden="true" />
          <small>
            {statusLabel}
            {data.skipped ? ' · 端口被占用已跳过启动' : ''}
            {data.message ? `\n${data.message}` : ''}
          </small>
        </div>
        <div className="settings-redis-address">
          <span>连接地址</span>
          <code>redis://{data.address}</code>
          <button
            type="button"
            className="text-button"
            onClick={() => void copyAddress()}
            disabled={!data.running}
          >
            {copied ? (
              <Check className="size-3.5" />
            ) : (
              <Copy className="size-3.5" />
            )}
            {copied ? '已复制' : '复制'}
          </button>
        </div>
        <div className="settings-card-actions settings-card-actions-end">
          {data.nativeSupported && !data.nativeRunning && (
            <Button
              variant="secondary"
              className="gap-1.5"
              disabled={controlling || saving || data.port !== 6379}
              onClick={() => void runAction('install-native')}
              title={
                data.port !== 6379
                  ? '独立 Redis 当前使用系统默认端口 6379，请先修改端口'
                  : undefined
              }
            >
              <Database className="size-3.5" />
              {data.nativeInstalled ? '启用独立 Redis' : '安装独立 Redis'}
            </Button>
          )}
          <Button
            variant="primary"
            className="gap-1.5"
            disabled={controlling || data.running || data.disabled}
            onClick={() => void runAction('start')}
          >
            <Play className="size-3.5" /> 启动
          </Button>
          <Button
            variant="secondary"
            className="gap-1.5"
            disabled={controlling || !data.managed || data.disabled}
            onClick={() => void runAction('restart')}
          >
            <RotateCcw className="size-3.5" /> 重启
          </Button>
          <Button
            variant="danger"
            className="gap-1.5"
            disabled={controlling || !data.managed || data.disabled}
            onClick={() => void runAction('stop')}
          >
            <Power className="size-3.5" /> 停止
          </Button>
        </div>
      </SettingsCard>

      <SettingsCard
        icon={<RefreshCw className="size-4" />}
        title="服务配置"
        description="端口与自启策略"
      >
        <div className="settings-redis-form">
          <label className="settings-redis-field">
            <span>端口</span>
            <input
              type="number"
              min={1}
              max={65535}
              value={port}
              onChange={event => {
                setPort(event.target.value)
                setChanged(true)
                setMessage('')
              }}
            />
          </label>
          <Button
            variant="secondary"
            className="gap-1.5"
            disabled={saving || (!changed && port === String(data.port))}
            onClick={() => void save()}
          >
            <Save className="size-3.5" /> 保存配置
          </Button>
          <SettingsSwitch
            checked={!disabled}
            onChange={checked => {
              setDisabled(!checked)
              setChanged(true)
              setMessage('')
            }}
            label={data.nativeRunning ? '使用独立 Redis 服务' : '启用内置 Redis 服务'}
            hint={
              data.nativeRunning
                ? '独立 Redis 由 systemd 管理；关闭此开关不会停止系统服务'
                : '关闭后不会启动；可用 alx --redis-off 快速禁用'
            }
          />
          <SettingsSwitch
            checked={autoStart}
            onChange={checked => {
              setAutoStart(checked)
              setChanged(true)
              setMessage('')
            }}
            label="工作台启动时自动开启"
            hint="默认开启；端口已有 Redis 时自动复用该服务"
          />
        </div>
      </SettingsCard>

      {message && (
        <SettingsMessage tone={message.includes('失败') || message.includes('无法') ? 'error' : 'info'}>
          {message}
        </SettingsMessage>
      )}
    </SettingsPage>
  )
}
