import { useEffect, useState } from 'react'
import { AppWindow, Bot, Code2, Ellipsis, Network, Power, RotateCw, Terminal } from 'lucide-react'
import {
  useSaveSystemNetworkMutation,
  useSystemNetworkQuery,
  type SystemNetworkMode
} from '../store/workspaceApi'
import { Button } from './Button'
import { ThemeToggle } from './ThemeToggle'
import { networkError, networkModes } from './network/networkSettings'
import { bridge, markNativeRuntimeReady } from './nativeWindow'

export function TrayPanel() {
  const [service, setService] = useState<{ online: boolean; version?: string }>()

  const [moreOpen, setMoreOpen] = useState(false)
  const [networkMessage, setNetworkMessage] = useState('')
  const { currentData: network, isError, isFetching, refetch } = useSystemNetworkQuery()
  const [saveNetwork, { isLoading: saving }] = useSaveSystemNetworkMutation()

  useEffect(() => {
    const refresh = () => {
      setMoreOpen(false)
      setNetworkMessage('')
      void refetch()
    }
    window.addEventListener('focus', refresh)
    return () => window.removeEventListener('focus', refresh)
  }, [refetch])

  const selectMode = async (mode: SystemNetworkMode) => {
    if (!network || mode === network.mode) return
    setNetworkMessage('')
    if (mode === 'proxy' && !network.proxy.url) {
      setNetworkMessage('请先在网络设置中配置代理地址。')
      return
    }
    try {
      await saveNetwork({ ...network, mode }).unwrap()
    } catch (error) {
      setNetworkMessage(networkError(error))
    }
  }

  useEffect(() => {
    markNativeRuntimeReady()
    let request: AbortController | undefined
    const refresh = async () => {
      request?.abort()
      const controller = new AbortController()
      request = controller
      try {
        const response = await fetch('/healthz', {
          cache: 'no-store',
          signal: controller.signal
        })
        if (!response.ok) throw new Error()
        const result = (await response.json()) as { status?: string; version?: string }
        if (!controller.signal.aborted)
          setService({ online: result.status === 'ok', version: result.version })
      } catch {
        if (!controller.signal.aborted) setService({ online: false })
      }
    }
    void refresh()
    window.addEventListener('focus', refresh)
    return () => {
      request?.abort()
      window.removeEventListener('focus', refresh)
    }
  }, [])

  return (
    <main className="tray-panel" aria-label="ALemonX 菜单">
      <header className="tray-header">
        <span className="tray-app-icon">
          <Bot className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <h1 className="m-0 text-sm font-semibold">ALemonX</h1>
          <p className="m-0 flex items-center gap-1.5 whitespace-nowrap text-[11px] text-(--tray-muted)" role="status">
            <span
              className={`size-1.5 rounded-full ${service?.online ? 'bg-(--theme-success)' : service ? 'bg-(--theme-danger)' : 'bg-current'}`}
              aria-hidden="true"
            />
            {service?.online ? '工作台在线' : service ? '工作台不可用' : '正在检查…'}
          </p>
        </div>
        <div className="tray-shortcuts">
          <Button
            variant="icon"
            aria-label="打开终端"
            title="打开终端"
            onClick={() => bridge?.postMessage('alx:tray:terminal')}
          >
            <Terminal className="size-4" aria-hidden="true" />
          </Button>
          <ThemeToggle />
          <Button
            variant="icon"
            aria-label="更多"
            title="更多"
            aria-expanded={moreOpen}
            aria-controls="tray-more"
            onClick={() => setMoreOpen(open => !open)}
          >
            <Ellipsis className="size-4" aria-hidden="true" />
          </Button>
        </div>
      </header>
      <section className="tray-network" aria-labelledby="tray-network-title">
        <div className="flex items-center gap-2 text-xs text-(--tray-muted)">
          <Network className="size-4" aria-hidden="true" />
          <h2 id="tray-network-title" className="m-0 text-xs font-semibold">网络模式</h2>
          {service?.version && (
            <span className="ml-auto max-w-24 truncate text-(--tray-muted)" title={service.version}>{service.version}</span>
          )}
        </div>
        <div className="tray-segments" role="group" aria-label="网络模式">
          {(Object.keys(networkModes) as SystemNetworkMode[]).map(mode => (
            <Button
              key={mode}
              size="sm"
              aria-pressed={mode === network?.mode}
              disabled={!network || isError || isFetching || saving}
              onClick={() => void selectMode(mode)}
            >
              {networkModes[mode]}
            </Button>
          ))}
        </div>
        {(isError || networkMessage) && (
          <p className="tray-network-error" role="alert">
            {networkMessage || '无法读取网络模式。'}
          </p>
        )}
      </section>
      <div id="tray-more" hidden={!moreOpen} className="tray-more">
        <Button
          size="sm"
          className="w-full justify-start gap-2"
          onClick={() => {
            bridge?.postMessage('alx:tray:developer-mode')
            setMoreOpen(false)
          }}
        >
          <Code2 className="size-4" aria-hidden="true" />
          切换开发模式
        </Button>
      </div>
      <div className="tray-footer">
        {([
          ['open', '打开', AppWindow],
          ['restart', '重启应用', RotateCw],
          ['quit', '退出', Power]
        ] as const).map(([action, label, Icon]) => (
          <button
            key={action}
            type="button"
            data-action={action}
            onClick={() => bridge?.postMessage(`alx:tray:${action}`)}
          >
            <Icon className="size-4" aria-hidden="true" />
            {label}
          </button>
        ))}
      </div>
    </main>
  )
}
