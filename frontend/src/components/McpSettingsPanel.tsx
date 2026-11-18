import { useEffect, useRef, useState } from 'react'
import { Cable } from 'lucide-react'
import {
  useControlSystemMcpMutation,
  useSystemMcpQuery
} from '../store/workspaceApi'
import { Button } from './Button'
import { ConfirmDialog } from './ConfirmDialog'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

const stdioConfig = JSON.stringify(
  {
    mcpServers: { alemonx: { command: 'alx', args: ['mcp'] } }
  },
  null,
  2
)

export function McpSettingsPanel() {
  const { data, isError, isFetching, refetch } = useSystemMcpQuery(undefined, {
    refetchOnMountOrArgChange: true
  })
  const [control, { isLoading: controlling }] = useControlSystemMcpMutation()
  const [token, setToken] = useState('')
  const [keyType, setKeyType] = useState<'diagnostic' | 'operator'>(
    'diagnostic'
  )
  const tokenRequest = useRef<AbortController | null>(null)
  const [savingKey, setSavingKey] = useState(false)
  const [keyChange, setKeyChange] = useState<'rotate' | 'revoke' | null>(null)
  const [message, setMessage] = useState<{
    text: string
    error?: boolean
  } | null>(null)

  useEffect(() => {
    const clearToken = () => {
      tokenRequest.current?.abort()
      setToken('')
      setSavingKey(false)
      setKeyChange(null)
    }
    const onSystemEvent = (event: Event) => {
      const detail = (
        event as CustomEvent<{
          topic?: string
          data?: { type?: string }
        }>
      ).detail
      if (detail?.topic === 'system' && detail.data?.type === 'mcp.changed') {
        void refetch()
      }
    }
    const onAuthChange = () => {
      clearToken()
      void refetch()
    }
    window.addEventListener('alx:auth-changed', onAuthChange)
    window.addEventListener('alx:unified-event', onSystemEvent)
    return () => {
      tokenRequest.current?.abort()
      window.removeEventListener('alx:auth-changed', onAuthChange)
      window.removeEventListener('alx:unified-event', onSystemEvent)
    }
  }, [refetch])

  const run = async (action: 'start' | 'stop') => {
    setMessage(null)
    try {
      await control(action).unwrap()
    } catch (reason) {
      const error = (reason as { data?: { error?: string } })?.data?.error
      setMessage({ text: error || 'MCP 服务操作失败，请重试。', error: true })
    }
  }

  const updateKey = async (action: 'token' | 'rotate' | 'revoke') => {
    const request = new AbortController()
    tokenRequest.current = request
    setSavingKey(true)
    setMessage(null)
    try {
      const response = await fetch('/api/v1/system/mcp', {
        method: 'POST',
        signal: request.signal,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, keyType })
      })
      const result =
        response.status === 204
          ? {}
          : ((await response.json()) as { token?: string; error?: string })
      if (!response.ok || (action !== 'revoke' && !result.token)) {
        throw new Error(result.error || '无法更新连接密钥。')
      }
      if (request.signal.aborted) return
      setToken(result.token || '')
      setKeyChange(null)
      if (action === 'revoke') setMessage({ text: '连接密钥已撤销。' })
      void refetch()
    } catch (reason) {
      if (request.signal.aborted) return
      setKeyChange(null)
      setMessage({
        text: reason instanceof Error ? reason.message : '无法更新连接密钥。',
        error: true
      })
      void refetch()
    } finally {
      if (!request.signal.aborted) setSavingKey(false)
    }
  }

  const copy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setMessage({ text: `已复制${label}。` })
    } catch {
      setMessage({ text: `无法访问剪贴板，请手动复制${label}。`, error: true })
    }
  }

  const external = data?.running && !data.managed
  const pending = data?.autoStart && !data.managed
  const hasSelectedKey =
    keyType === 'diagnostic' ? data?.hasDiagnosticToken : data?.hasToken
  const keyLabel = keyType === 'diagnostic' ? '只读诊断密钥' : '操作密钥'
  const locked = controlling || savingKey || isFetching
  return (
    <SettingsPage title="MCP">
      <SettingsCard
        icon={<Cable className="size-4" />}
        title="流式 HTTP"
        actions={
          <>
            <Button disabled={locked} onClick={() => void refetch()}>
              刷新
            </Button>
            {data?.canControl && !isError && (!external || pending) && (
              <Button
                variant={data.autoStart ? 'secondary' : 'primary'}
                disabled={locked}
                onClick={() =>
                  void run(data.autoStart || data.managed ? 'stop' : 'start')
                }
              >
                {pending
                  ? '取消自动恢复'
                  : data.managed
                    ? '停止 HTTP 服务'
                    : '启动 HTTP 服务'}
              </Button>
            )}
          </>
        }
      >
        {isError ? (
          <SettingsMessage tone="error">
            无法读取 MCP 服务状态。
          </SettingsMessage>
        ) : !data ? (
          <SettingsMessage>正在读取 MCP 服务状态…</SettingsMessage>
        ) : (
          <>
            <p role="status">
              {pending
                ? 'MCP HTTP 正在等待自动恢复'
                : external
                  ? '外部 MCP 服务正在运行'
                  : data.managed
                    ? 'MCP HTTP 服务正在运行'
                    : 'MCP HTTP 服务已停止'}
            </p>
            {pending && <p>如果端口被占用，服务会在端口释放后重试。</p>}
            {data.canControl && data.managed && data.autoStart && (
              <p>工作台重启时会自动恢复。手动停止后会保持停止。</p>
            )}
            {!data.canControl && !data.managed && !pending && (
              <p>超级管理员启动工作台服务后，客户端即可使用密钥连接。</p>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <p className="wrap-break-word">
                连接地址 <code>{data.url}</code>
              </p>
              <Button onClick={() => void copy(data.url, '连接地址')}>
                复制地址
              </Button>
            </div>
            {data.canIssueToken && (
              <>
                <label className="flex flex-wrap items-center gap-2">
                  密钥类型
                  <select
                    className="settings-input"
                    value={keyType}
                    disabled={locked || keyChange !== null}
                    onChange={event => {
                      setToken('')
                      setMessage(null)
                      setKeyType(
                        event.target.value === 'operator'
                          ? 'operator'
                          : 'diagnostic'
                      )
                    }}
                  >
                    <option value="diagnostic">只读诊断密钥</option>
                    <option value="operator">操作密钥</option>
                  </select>
                </label>
                <p>
                  {keyType === 'diagnostic'
                    ? '仅用于查询机器人状态、脱敏日志和任务历史。'
                    : '用于账户范围内的完整 MCP 操作，包括修改和发布。'}
                </p>
                <p>
                  {data.account || '本机账户'}
                  {hasSelectedKey ? ' 已配置此密钥。' : ' 尚未配置此密钥。'}
                  在客户端将它填为 Bearer Token。
                </p>
                <div className="flex flex-wrap gap-2">
                  {hasSelectedKey ? (
                    <>
                      <Button
                        disabled={locked}
                        onClick={() => setKeyChange('rotate')}
                      >
                        重新生成密钥
                      </Button>
                      <Button
                        variant="danger"
                        disabled={locked}
                        onClick={() => setKeyChange('revoke')}
                      >
                        撤销密钥
                      </Button>
                    </>
                  ) : (
                    <Button
                      variant="primary"
                      disabled={locked}
                      onClick={() => void updateKey('token')}
                    >
                      创建连接密钥
                    </Button>
                  )}
                </div>
              </>
            )}
            {data.canIssueToken && token && (
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <p className="basis-full">明文只在本次显示，请立即复制。</p>
                <input
                  aria-label="连接密钥"
                  className="min-w-0 flex-1 rounded border border-(--theme-border-default) bg-(--theme-surface-panel) p-2 font-mono"
                  readOnly
                  value={token}
                  onFocus={event => event.target.select()}
                />
                <Button onClick={() => void copy(token, '连接密钥')}>
                  复制密钥
                </Button>
              </div>
            )}
          </>
        )}
      </SettingsCard>
      {data?.canControl && (
        <SettingsCard
          title="STDIO"
          actions={
            <Button onClick={() => void copy(stdioConfig, 'STDIO 配置')}>
              复制 JSON 配置
            </Button>
          }
        >
          <p>
            在 MCP 客户端选择 STDIO，由客户端启动本机的 <code>alx mcp</code>。
            工作台开启账户认证后，客户端环境需配置超级管理员的{' '}
            <code>ALX_AUTH_TOKEN</code>。
          </p>
          <pre className="overflow-x-auto">{stdioConfig}</pre>
        </SettingsCard>
      )}
      <ConfirmDialog
        open={keyChange !== null}
        subtitle={keyLabel}
        title={keyChange === 'revoke' ? '撤销 MCP 密钥' : '重新生成 MCP 密钥'}
        message={
          keyChange === 'revoke'
            ? '撤销后，使用这把密钥的客户端将无法继续调用。'
            : '新密钥生效后，旧密钥会立即失效。请在客户端更新密钥。'
        }
        confirmLabel={keyChange === 'revoke' ? '确认撤销' : '确认重新生成'}
        busy={savingKey}
        destructive
        onCancel={() => {
          if (!savingKey) setKeyChange(null)
        }}
        onConfirm={() => {
          if (keyChange && !savingKey) void updateKey(keyChange)
        }}
      />
      {message && (
        <SettingsMessage tone={message.error ? 'error' : 'success'}>
          {message.text}
        </SettingsMessage>
      )}
    </SettingsPage>
  )
}
