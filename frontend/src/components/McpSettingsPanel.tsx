import { useEffect, useState } from 'react'
import { Cable } from 'lucide-react'
import {
  useControlSystemMcpMutation,
  useSystemMcpQuery
} from '../store/workspaceApi'
import { Button } from './Button'
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
  const [revealing, setRevealing] = useState(false)
  const [message, setMessage] = useState<{
    text: string
    error?: boolean
  } | null>(null)

  useEffect(() => {
    const onSystemEvent = (event: Event) => {
      const detail = (
        event as CustomEvent<{
          topic?: string
          data?: { type?: string }
        }>
      ).detail
      if (detail?.topic === 'system' && detail.data?.type === 'mcp.changed') {
        setToken('')
        void refetch()
      }
    }
    window.addEventListener('alx:unified-event', onSystemEvent)
    return () => window.removeEventListener('alx:unified-event', onSystemEvent)
  }, [refetch])

  const run = async (action: 'start' | 'stop') => {
    setToken('')
    setMessage(null)
    try {
      await control(action).unwrap()
    } catch (reason) {
      const error = (reason as { data?: { error?: string } })?.data?.error
      setMessage({ text: error || 'MCP 服务操作失败，请重试。', error: true })
    }
  }

  const revealToken = async () => {
    setRevealing(true)
    setMessage(null)
    try {
      const response = await fetch('/api/v1/system/mcp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'token' })
      })
      const result = (await response.json()) as {
        token?: string
        error?: string
      }
      if (!response.ok || !result.token) {
        throw new Error(result.error || '无法读取连接令牌。')
      }
      setToken(result.token)
    } catch (reason) {
      setMessage({
        text: reason instanceof Error ? reason.message : '无法读取连接令牌。',
        error: true
      })
    } finally {
      setRevealing(false)
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
  const locked = controlling || revealing || isFetching
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
            {data && !isError && !external && (
              <Button
                variant={data.managed ? 'secondary' : 'primary'}
                disabled={locked}
                onClick={() => void run(data.managed ? 'stop' : 'start')}
              >
                {data.managed ? '停止 HTTP 服务' : '启动 HTTP 服务'}
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
              {external
                ? '外部 MCP 服务正在运行'
                : data.managed
                  ? 'MCP HTTP 服务正在运行'
                  : 'MCP HTTP 服务已停止'}
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <p className="wrap-break-word">
                连接地址 <code>{data.url}</code>
              </p>
              <div className="ml-auto flex flex-wrap justify-end gap-2">
                <Button onClick={() => void copy(data.url, '连接地址')}>
                  复制地址
                </Button>
                {data.managed && (
                  <Button
                    disabled={locked}
                    onClick={() => (token ? setToken('') : void revealToken())}
                  >
                    {token ? '隐藏连接令牌' : '显示连接令牌'}
                  </Button>
                )}
              </div>
            </div>
            <p>
              在运行 ALemonX 的电脑上，将此地址和 Bearer Token 填入 MCP
              客户端的流式 HTTP 配置。
            </p>
            {data.managed && (
              <p>重新启动 HTTP 服务后，需要在客户端更新令牌。</p>
            )}
            {token && (
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <input
                  aria-label="连接令牌"
                  className="min-w-0 flex-1 rounded border border-(--theme-border-default) bg-(--theme-surface-panel) p-2 font-mono"
                  readOnly
                  value={token}
                  onFocus={event => event.target.select()}
                />
                <Button onClick={() => void copy(token, '连接令牌')}>
                  复制令牌
                </Button>
              </div>
            )}
          </>
        )}
      </SettingsCard>
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
          工作台开启账户认证后，可在客户端环境中配置 <code>ALX_AUTH_TOKEN</code>
          ， 或在本页使用流式 HTTP 连接。
        </p>
        <pre className="overflow-x-auto">{stdioConfig}</pre>
      </SettingsCard>
      {message && (
        <SettingsMessage tone={message.error ? 'error' : 'success'}>
          {message.text}
        </SettingsMessage>
      )}
    </SettingsPage>
  )
}
