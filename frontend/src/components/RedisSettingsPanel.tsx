import { useEffect, useState } from 'react'
import { Check, Copy, Database, Power, RefreshCw, RotateCcw, Save, Settings2 } from 'lucide-react'
import { useControlSystemRedisMutation, useSaveSystemRedisConfigMutation, useSystemRedisQuery } from '../store/workspaceApi'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage, SettingsSwitch } from './SettingsCard'

function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'data' in error) {
    const value = (error as { data?: { error?: string } }).data?.error
    if (value) return value
  }
  return fallback
}

export function RedisSettingsPanel() {
  const { data, isLoading, isError, refetch } = useSystemRedisQuery()
  const [control, { isLoading: controlling }] = useControlSystemRedisMutation()
  const [saveConfig, { isLoading: saving }] = useSaveSystemRedisConfigMutation()
  const [advanced, setAdvanced] = useState(false)
  const [port, setPort] = useState('6379')
  const [autoStart, setAutoStart] = useState(true)
  const [disabled, setDisabled] = useState(false)
  const [changed, setChanged] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!data || changed) return
    setPort(String(data.port)); setAutoStart(data.autoStart); setDisabled(data.disabled)
  }, [data, changed])
  if (isLoading) return <div className="settings-panel-content">正在检查本地数据服务…</div>
  if (isError || !data) return <div className="settings-panel-content">暂时无法读取 Redis 状态。<Button variant="ghost" onClick={() => void refetch()}>重试</Button></div>

  const preparing = data.phase === 'preparing-runtime' || data.mode === 'migrating'
  const ready = data.running && !data.external
  const title = data.disabled ? 'Redis 已关闭' : data.external ? '正在使用已有 Redis' : preparing ? '正在准备更稳定的本地 Redis' : ready ? 'Redis 已准备好' : 'Redis 需要处理'
  const description = data.disabled ? '机器人和插件当前不能使用 Redis。' : data.external ? '这是已有服务，ALemonX 只连接使用，不会停止或修改它。' : preparing ? '应用正在后台准备；机器人和插件可以继续使用。' : ready ? '数据保存在本机，ALemonX 启动时会自动运行。' : '启动服务后，机器人和插件即可使用本地数据存储。'
  const run = async (action: 'start' | 'stop' | 'restart' | 'retry-runtime') => {
    if ((action === 'stop' || action === 'restart') && !window.confirm(`${action === 'stop' ? '停止' : '重启'}后，机器人和插件将暂时不可使用。是否继续？`)) return
    setMessage(''); setCopied(false)
    try { await control(action).unwrap() } catch (error) { setMessage(errorMessage(error, 'Redis 操作未完成。')) }
  }
  const save = async () => {
    const nextPort = Number(port)
    if (!Number.isInteger(nextPort) || nextPort < 1 || nextPort > 65535) { setMessage('端口需要在 1-65535 之间。'); return }
    if ((disabled || nextPort !== data.port) && !window.confirm(disabled ? '关闭后机器人和插件将无法使用 Redis。是否继续？' : '修改端口会短暂重启本地 Redis。是否继续？')) return
    try { await saveConfig({ port: nextPort, autoStart, disabled }).unwrap(); setChanged(false); setMessage('配置已保存。') } catch (error) { setMessage(errorMessage(error, 'Redis 配置未保存。')) }
  }
  const copy = async () => {
    try { await navigator.clipboard.writeText(data.connectionUri || `redis://${data.address}`); setCopied(true); window.setTimeout(() => setCopied(false), 1600) } catch { setMessage('无法复制，请手动复制连接信息。') }
  }
  return <SettingsPage title="Redis" description="为机器人和插件自动管理的本地数据服务。">
    <SettingsCard icon={<Database className="size-4" />} title={title} description={description} actions={<Button variant="ghost" className="gap-1" onClick={() => void refetch()} disabled={controlling}><RefreshCw className="size-3.5" />刷新</Button>}>
      <div className="settings-service-status" aria-live="polite"><span className={ready ? 'is-ready' : preparing ? 'is-idle' : 'is-offline'} aria-hidden="true" /><small>{data.message}</small></div>
      <div className="settings-redis-address"><span>连接信息</span><code>{data.address}</code><button type="button" className="text-button" onClick={() => void copy()} disabled={!data.running}>{copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}{copied ? '已复制' : '复制连接串'}</button></div>
      {!data.external && <div className="settings-card-actions settings-card-actions-end">{!data.running && !data.disabled && <Button variant="primary" disabled={controlling} onClick={() => void run('start')}>启动服务</Button>}{data.mode === 'fallback-running' && <Button variant="secondary" disabled={controlling} onClick={() => void run('retry-runtime')}>重试准备</Button>}</div>}
    </SettingsCard>
    <Button variant="ghost" className="gap-1" onClick={() => setAdvanced(value => !value)}><Settings2 className="size-3.5" />{advanced ? '收起高级设置' : '高级设置'}</Button>
    {advanced && !data.external && <SettingsCard icon={<Settings2 className="size-4" />} title="维护与配置" description="通常无需修改；改端口、关闭或重启会影响机器人和插件。"><div className="settings-redis-form">
      <label className="settings-redis-field"><span>端口</span><input type="number" min={1} max={65535} value={port} onChange={event => { setPort(event.target.value); setChanged(true) }} /></label>
      <SettingsSwitch checked={!disabled} onChange={checked => { setDisabled(!checked); setChanged(true) }} label="启用本地 Redis" hint="关闭后不会自动启动，也不会停止任何已有外部 Redis。" />
      <SettingsSwitch checked={autoStart} onChange={checked => { setAutoStart(checked); setChanged(true) }} label="启动 ALemonX 时自动开启" hint="默认开启。" />
      <Button variant="secondary" className="gap-1.5" disabled={saving || !changed} onClick={() => void save()}><Save className="size-3.5" />保存配置</Button>
      {data.managed && <div className="settings-card-actions"><Button variant="secondary" disabled={controlling} onClick={() => void run('restart')}><RotateCcw className="size-3.5" />重启</Button><Button variant="danger" disabled={controlling} onClick={() => void run('stop')}><Power className="size-3.5" />停止</Button></div>}
    </div></SettingsCard>}
    {message && <SettingsMessage tone={message.includes('失败') || message.includes('无法') ? 'error' : 'info'}>{message}</SettingsMessage>}
  </SettingsPage>
}
