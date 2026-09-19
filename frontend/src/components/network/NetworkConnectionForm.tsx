import { useStoreState } from '../../store/guideStore'
import {
  useTestSystemNetworkMutation,
  type SystemNetworkSettings,
  type SystemNetworkMode
} from '../../store/workspaceApi'
import { FormDialog } from '../FormDialog'
import { Button } from '../Button'
import {
  networkInputClass,
  networkError,
  networkModes
} from './networkSettings'

export function NetworkConnectionForm({
  settings,
  migration = false,
  onSave,
  onClose
}: {
  settings: SystemNetworkSettings
  migration?: boolean
  onSave: (next: SystemNetworkSettings) => Promise<void>
  onClose?: () => void
}) {
  const initial = settings.proxy.url ? new URL(settings.proxy.url) : null
  const [mode, setMode] = useStoreState<SystemNetworkMode>(
    migration ? settings.mode : 'proxy'
  )
  const [protocol, setProtocol] = useStoreState(
    initial?.protocol.replace(':', '') || 'http'
  )
  const [host, setHost] = useStoreState(initial?.hostname || '')
  const [port, setPort] = useStoreState(
    initial
      ? initial.port ||
          (initial.protocol === 'https:'
            ? '443'
            : initial.protocol === 'socks5:'
              ? '1080'
              : '80')
      : ''
  )
  const [legacyId, setLegacyId] = useStoreState(settings.proxy.legacyId || '')
  const [auth, setAuth] = useStoreState<'preserve' | 'replace' | 'remove'>(
    'preserve'
  )
  const [username, setUsername] = useStoreState('')
  const [password, setPassword] = useStoreState('')
  const [errors, setErrors] = useStoreState<Record<string, string>>({})
  const [busy, setBusy] = useStoreState(false)
  const [result, setResult] = useStoreState('')
  const [saved, setSaved] = useStoreState(false)
  const [test, testState] = useTestSystemNetworkMutation()
  const draft = (): SystemNetworkSettings | null => {
    const invalid: Record<string, string> = {}
    let address = settings.proxy.url
    if (mode === 'proxy') {
      if (!host.trim() || /[\s/@?#]/.test(host))
        invalid.host = '请输入有效主机名或 IP'
      if (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535)
        invalid.port = '端口范围为 1–65535'
      if (auth === 'replace' && !username) invalid.username = '请输入用户名'
      address = `${protocol}://${host.includes(':') && !host.startsWith('[') ? '[' + host + ']' : host}:${port}`
      try {
        new URL(address)
      } catch {
        invalid.host = '请输入有效主机名或 IP'
      }
    }
    setErrors(invalid)
    if (Object.keys(invalid).length) return null
    return {
      ...settings,
      mode,
      confirmMigration: migration,
      proxy:
        mode === 'proxy'
          ? {
              url: address,
              credentials: auth,
              legacyId: migration ? legacyId : undefined,
              ...(auth === 'replace' ? { username, password } : {})
            }
          : settings.proxy
    }
  }
  const save = async () => {
    const next = draft()
    if (!next) return
    setBusy(true)
    try {
      await onSave(next)
      setAuth('preserve')
      setUsername('')
      setPassword('')
      setResult('')
      setSaved(true)
      onClose?.()
    } catch (error) {
      setErrors(current => ({ ...current, save: networkError(error) }))
    } finally {
      setBusy(false)
    }
  }
  const testDraft = async () => {
    const next = draft()
    if (!next) return
    setResult('测试中…')
    try {
      const value = await test({
        target: 'github-api',
        settings: next
      }).unwrap()
      setResult(`${value.message} · ${value.latencyMs ?? 0} ms`)
    } catch (error) {
      setResult(networkError(error))
    } finally {
      testState.reset()
    }
  }
  const fieldError = (name: string) =>
    errors[name] && (
      <span role="alert" className="text-xs text-(--theme-danger-text)">
        {errors[name]}
      </span>
    )
  const actions = (
    <div className="flex flex-wrap items-center gap-3">
      {onClose && (
        <Button disabled={busy || testState.isLoading} onClick={onClose}>
          取消
        </Button>
      )}
      <Button
        loading={busy}
        disabled={testState.isLoading}
        onClick={() => void save()}
      >
        保存并应用
      </Button>
      {saved && (
        <span role="status" className="text-xs text-(--theme-text-muted)">
          已应用
        </span>
      )}
    </div>
  )
  const fields = (
    <fieldset
      disabled={busy || testState.isLoading}
      onChange={() => {
        setResult('')
        setErrors({})
        setSaved(false)
      }}
      className="m-0 grid min-w-0 gap-3 border-0 p-0"
    >
      {migration && (
        <label className="grid gap-1 text-sm">
          全局模式
          <select
            aria-label="全局模式"
            className={networkInputClass}
            value={mode}
            onChange={e => setMode(e.target.value as SystemNetworkMode)}
          >
            {(['auto', 'direct', 'proxy'] as const).map(value => (
              <option key={value} value={value}>
                {networkModes[value]}
              </option>
            ))}
          </select>
        </label>
      )}
      {mode === 'proxy' && (
        <>
          {migration && !!settings.migration?.proxies.length && (
            <label className="grid gap-1 text-sm">
              旧代理地址
              <select
                className={networkInputClass}
                defaultValue=""
                onChange={e => {
                  if (!e.target.value) return
                  const choice = settings.migration?.choices?.find(
                    item => item.id === e.target.value
                  )
                  const url = new URL(choice?.url || e.target.value)
                  setLegacyId(choice?.id || '')
                  setProtocol(url.protocol.replace(':', ''))
                  setHost(url.hostname)
                  setPort(
                    url.port || (url.protocol === 'https:' ? '443' : '80')
                  )
                }}
              >
                <option value="">选择地址</option>
                {(
                  settings.migration.choices ||
                  settings.migration.proxies.map(url => ({
                    id: url,
                    url,
                    hasCredentials: false
                  }))
                ).map(item => (
                  <option key={item.id} value={item.id}>
                    {item.url}
                    {item.hasCredentials ? ' · 已认证' : ''} · {item.id}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="grid gap-1 text-sm">
            协议
            <select
              aria-label="协议"
              className={networkInputClass}
              value={protocol}
              onChange={e => setProtocol(e.target.value)}
            >
              {['http', 'https', 'socks5'].map(value => (
                <option key={value} value={value}>
                  {value.toUpperCase()}
                </option>
              ))}
            </select>
          </label>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="grid gap-1 text-sm sm:col-span-2">
              主机
              <input
                className={networkInputClass}
                autoComplete="off"
                value={host}
                aria-invalid={!!errors.host}
                onChange={e => setHost(e.target.value)}
              />
              {fieldError('host')}
            </label>
            <label className="grid gap-1 text-sm">
              端口
              <input
                className={networkInputClass}
                inputMode="numeric"
                value={port}
                aria-invalid={!!errors.port}
                onChange={e => setPort(e.target.value)}
              />
              {fieldError('port')}
            </label>
          </div>
          <label className="grid gap-1 text-sm">
            认证
            <select
              aria-label="认证"
              className={networkInputClass}
              value={auth}
              onChange={e => setAuth(e.target.value as typeof auth)}
            >
              <option value="preserve">
                {settings.proxy.hasCredentials ? '保留现有认证' : '无认证'}
              </option>
              <option value="replace">设置用户名和密码</option>
              <option value="remove">移除认证</option>
            </select>
          </label>
          {auth === 'replace' && (
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="grid gap-1 text-sm">
                用户名
                <input
                  className={networkInputClass}
                  autoComplete="off"
                  value={username}
                  onChange={e => setUsername(e.target.value)}
                />
                {fieldError('username')}
              </label>
              <label className="grid gap-1 text-sm">
                密码
                <input
                  className={networkInputClass}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                />
              </label>
            </div>
          )}
        </>
      )}
      {mode !== 'auto' && (
        <div>
          <Button
            disabled={busy}
            loading={testState.isLoading}
            onClick={() => void testDraft()}
          >
            测试连接
          </Button>
          {result && (
            <p role="status" className="mt-2 text-xs">
              {result}
            </p>
          )}
        </div>
      )}
      {fieldError('save')}
    </fieldset>
  )
  if (migration && onClose)
    return (
      <FormDialog
        open
        title="确认网络迁移"
        busy={busy || testState.isLoading}
        onClose={onClose}
        footer={actions}
      >
        {fields}
      </FormDialog>
    )
  return (
    <section aria-label="代理配置" className="mt-4 min-w-0 space-y-4">
      {fields}
      {actions}
    </section>
  )
}
