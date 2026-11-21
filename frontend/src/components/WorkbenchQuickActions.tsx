import { Cable, Check, ChevronRight, Code2, Network } from 'lucide-react'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { useDispatch } from 'react-redux'
import { setDeveloperMode } from '../store/workspaceStore'
import {
  useControlSystemMcpMutation,
  useSaveSystemNetworkMutation,
  useSystemMcpQuery,
  useSystemNetworkQuery,
  type SystemNetworkMode
} from '../store/workspaceApi'
import { useViewportPopoverPosition } from '../hooks/useViewportPopoverPosition'
import { Button } from './Button'
import { GitHubAuthControl } from './GitHubAuthControl'
import { ThemeToggle } from './ThemeToggle'
import { WorkbenchTools } from './WorkbenchTools'
import type { WorkbenchToolAction } from './WorkbenchToolRail'
import { networkError, networkModes } from './network/networkSettings'

export function WorkbenchQuickActions({
  developerMode,
  minimizedWindows,
  panelActions = []
}: {
  developerMode: boolean
  minimizedWindows: WorkbenchToolAction[]
  panelActions?: Array<{
    title: string
    label: string
    icon: ReactNode
    expanded: boolean
    toggle: () => void
  }>
}) {
  return (
    <WorkbenchTools
      keepMounted
      restoreFocusOnClose={panelActions.length > 0}
      menuClassName={panelActions.length > 0 ? 'workbench-settings-menu overflow-y-auto overscroll-contain' : undefined}
    >
      {(close, open) => (
        <>
          {panelActions.length > 0 && (
            <>
              {panelActions.map(action => (
                <Button
                  key={action.title}
                  className="workbench-action-item"
                  aria-expanded={action.expanded}
                  onClick={() => {
                    close()
                    action.toggle()
                  }}
                >
                  {action.icon}
                  {action.label}
                </Button>
              ))}
              <div className="workbench-action-divider" aria-hidden="true" />
            </>
          )}
          {minimizedWindows.length > 0 && (
            <div className="min-[1200px]:hidden" role="group" aria-label="已最小化窗口">
              {minimizedWindows.map(window => (
                <Button
                  key={window.id}
                  className="workbench-action-item [&>svg]:size-4"
                  onClick={() => {
                    window.activate()
                    close()
                  }}
                >
                  {window.icon}
                  {window.label}
                </Button>
              ))}
              <div className="workbench-action-divider" aria-hidden="true" />
            </div>
          )}
          <QuickActions developerMode={developerMode} open={open} />
        </>
      )}
    </WorkbenchTools>
  )
}

function QuickActions({ developerMode, open }: { developerMode: boolean; open: boolean }) {
  const dispatch = useDispatch()
  const { currentData: mcp, isError: mcpUnavailable, isFetching: checkingMcp } =
    useSystemMcpQuery(undefined, { skip: !open, refetchOnMountOrArgChange: true })
  const [controlMcp, { isLoading: controlling }] = useControlSystemMcpMutation()
  const { currentData: network, isError: networkUnavailable, isFetching: checkingNetwork } =
    useSystemNetworkQuery(undefined, { skip: !open, refetchOnMountOrArgChange: true })
  const [saveNetwork, { isLoading: saving }] = useSaveSystemNetworkMutation()
  const [networkOpen, setNetworkOpen] = useState(false)
  const networkRow = useRef<HTMLDivElement>(null)
  const networkPanel = useRef<HTMLDivElement>(null)
  const networkPanelID = useId()
  const anchor = networkRow.current?.getBoundingClientRect() ?? null
  const networkPosition = useViewportPopoverPosition({
    anchor,
    open: networkOpen,
    popoverRef: networkPanel,
    placement: anchor && anchor.left >= 180 ? 'left' : 'bottom',
    gap: 5
  })

  useEffect(() => {
    const closeNetwork = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== 'network') setNetworkOpen(false)
    }
    window.addEventListener('alx:top-tool-open', closeNetwork)
    return () => window.removeEventListener('alx:top-tool-open', closeNetwork)
  }, [])

  const [mcpError, setMcpError] = useState('')
  const [networkMessage, setNetworkMessage] = useState('')

  const mcpPending = mcp?.autoStart && !mcp.managed

  const toggleMcp = async () => {
    setMcpError('')
    try {
      await controlMcp(mcp?.autoStart || mcp?.managed ? 'stop' : 'start').unwrap()
    } catch (error) {
      setMcpError(networkError(error))
    }
  }
  const selectMode = async (mode: SystemNetworkMode) => {
    if (!network || network.mode === mode) return
    setNetworkMessage('')
    if (mode === 'proxy' && !network.proxy.url) {
      setNetworkMessage('尚未配置代理地址。')
      return
    }
    try {
      await saveNetwork({ ...network, mode }).unwrap()
    } catch (error) {
      setNetworkMessage(networkError(error))
    }
  }

  return (
    <>
      <ThemeToggle menuItem />
      <Button
        className="workbench-action-item"
        aria-pressed={developerMode}
        onClick={() => dispatch(setDeveloperMode(!developerMode))}
      >
        <Code2 className="size-4" aria-hidden="true" />
        开发模式
        {developerMode && <Check className="ml-auto size-4" aria-hidden="true" />}
      </Button>
      <div className="workbench-action-divider" aria-hidden="true" />
      {mcp?.canControl && (
        <>
          <Button
            className="workbench-action-item"
            disabled={mcpUnavailable || checkingMcp || controlling || (mcp.running && !mcp.managed && !mcpPending)}
            onClick={() => void toggleMcp()}
          >
            <Cable className="size-4" aria-hidden="true" />
            {mcpUnavailable
              ? 'MCP HTTP 状态不可用'
              : mcpPending
                ? '取消 MCP 自动恢复'
                : mcp.running && !mcp.managed
                  ? 'MCP HTTP 由外部运行'
                  : mcp.managed ? '停止 MCP HTTP' : '启动 MCP HTTP'}
          </Button>
          {mcpError && <p role="alert" className="px-2 text-xs text-red-600">{mcpError}</p>}
        </>
      )}
      <GitHubAuthControl visible={open} menuItem />
      <div className="workbench-action-divider" aria-hidden="true" />
      <div ref={networkRow}>
        <Button
          className="workbench-action-item"
          aria-controls={networkOpen ? networkPanelID : undefined}
          aria-expanded={networkOpen}
          onClick={() => {
            if (!networkOpen)
              window.dispatchEvent(new CustomEvent('alx:top-tool-open', { detail: 'network' }))
            setNetworkOpen(!networkOpen)
          }}
          onKeyDown={event => {
            if (event.key === 'Escape' && networkOpen) {
              event.preventDefault()
              setNetworkOpen(false)
            }
          }}
        >
          <Network className="size-4" aria-hidden="true" />
          网络
          {network && <span className="ml-auto text-(--theme-text-muted)">{networkModes[network.mode]}</span>}
          <ChevronRight className={`size-3.5 text-(--theme-text-muted) ${network ? '' : 'ml-auto'}`} aria-hidden="true" />
        </Button>
      </div>
      {networkOpen && (
        <div
          ref={networkPanel}
          id={networkPanelID}
          role="group"
          aria-label="网络模式"
          style={networkPosition}
          className="workbench-tools-menu workbench-side-submenu"
          onKeyDown={event => {
            if (event.key !== 'Escape') return
            event.preventDefault()
            setNetworkOpen(false)
            networkRow.current?.querySelector('button')?.focus()
          }}
        >
          {networkUnavailable && <p role="alert" className="px-2 py-1 text-xs text-red-600">无法读取网络模式。</p>}
          {(Object.keys(networkModes) as SystemNetworkMode[]).map(mode => (
            <Button
              key={mode}
              className="workbench-action-item"
              aria-pressed={network?.mode === mode}
              disabled={!network || networkUnavailable || checkingNetwork || saving}
              onClick={() => void selectMode(mode)}
            >
              {networkModes[mode]}
              {network?.mode === mode && <Check className="ml-auto size-4" aria-hidden="true" />}
            </Button>
          ))}
          {networkMessage && <p role="alert" className="px-2 py-1 text-xs text-red-600">{networkMessage}</p>}
        </div>
      )}
    </>
  )
}
