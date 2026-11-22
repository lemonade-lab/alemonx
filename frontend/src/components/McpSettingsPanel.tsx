import { RefreshButton } from './RefreshButton'
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
        body: JSON.stringify({ action })
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
  const local = /^(localhost|127\.0\.0\.1|\[::1\])$/.test(window.location.hostname)
  const url = local ? data?.url || '' : new URL('/mcp', window.location.origin).href
  const locked = controlling || savingKey || isFetching
  return (
    <SettingsPage title="MCP">
      <SettingsCard
        icon={<Cable className="size-4" />}
        title="流式 HTTP"
        actions={
          <>
            <RefreshButton label="刷新" disabled={locked} onClick={() => void refetch()} />
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
            <div className="flex flex-wrap items-center gap-2">
              <p className="wrap-break-word">
                连接地址 <code>{url}</code>
              </p>
              <Button onClick={() => void copy(url, '连接地址')}>
                复制地址
              </Button>
            </div>
            {data.canIssueToken && (
              <>
                <p>{data.hasToken ? '已配置 Bearer Token' : '尚未配置 Bearer Token'}</p>
                <div className="flex flex-wrap gap-2">
                  {data.hasToken ? (
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
                <p className="basis-full">密钥只显示一次。</p>
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
          <p>启用工作台认证时，设置 <code>ALX_AUTH_TOKEN</code>。</p>
          <pre className="overflow-x-auto">{stdioConfig}</pre>
        </SettingsCard>
      )}
      <ConfirmDialog
        open={keyChange !== null}
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
