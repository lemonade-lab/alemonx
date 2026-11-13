import { useEffect, useId, useRef, useState } from 'react'
import { CircleAlert, Check, Copy } from 'lucide-react'
import { useViewportPopoverPosition } from '../hooks/useViewportPopoverPosition'
import { Button } from './Button'
import { GithubMark } from './GithubMark'
import { desktopPlatform } from './nativeWindow'

type AuthStatus = { loggedIn: boolean; login?: string }
type DeviceFlow = {
  flowId: string
  userCode: string
  verificationUri: string
  expiresIn: number
  interval: number
}

async function readJSON<T>(response: Response): Promise<T> {
  const data = (await response.json()) as T & { error?: string }
  if (!response.ok) throw new Error(data.error ?? '请求失败')
  return data
}

export function GitHubAuthControl({
  visible,
  menuItem = false,
  onLogin
}: {
  visible: boolean
  menuItem?: boolean
  onLogin?: () => void
}) {
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [flow, setFlow] = useState<DeviceFlow | null>(null)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  const [detailsOpen, setDetailsOpen] = useState(false)
  const errorTrigger = useRef<HTMLButtonElement>(null)
  const errorPanel = useRef<HTMLParagraphElement>(null)
  const errorID = useId()
  const anchor = errorTrigger.current?.getBoundingClientRect() ?? null
  const errorPosition = useViewportPopoverPosition({
    anchor,
    open: visible && Boolean(error) && detailsOpen,
    popoverRef: errorPanel,
    placement: anchor && anchor.left > 240 ? 'left' : 'bottom'
  })

  useEffect(() => {
    if (!visible || !error || !detailsOpen) return
    const closeOutside = (event: PointerEvent) => {
      const target = event.target as Node
      if (!errorTrigger.current?.contains(target) && !errorPanel.current?.contains(target))
        setDetailsOpen(false)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => document.removeEventListener('pointerdown', closeOutside)
  }, [visible, error, detailsOpen])

  useEffect(() => {
    if (!visible) return
    const controller = new AbortController()
    void fetch('/api/v1/github/auth/status', { signal: controller.signal })
      .then(readJSON<AuthStatus>)
      .then(data => { if (!controller.signal.aborted) setStatus(data) })
      .catch(() => {
        if (!controller.signal.aborted)
          setError(current => current || '无法读取 GitHub 授权状态，请检查网络后重试。')
      })
    return () => controller.abort()
  }, [visible])

  useEffect(() => {
    if (!flow) return
    const controller = new AbortController()
    const deadline = Date.now() + flow.expiresIn * 1000
    let timer: number
    const poll = async (interval: number) => {
      try {
        if (Date.now() >= deadline) throw new Error('授权码已过期，请重新登录。')
        const result = await fetch('/api/v1/github/auth/poll', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ flowId: flow.flowId }),
          signal: controller.signal
        }).then(readJSON<{ status: string; login?: string; interval?: number; message?: string }>)
        if (controller.signal.aborted) return
        if (result.status === 'pending' || result.status === 'slow_down') {
          const next = Math.max(result.interval ?? interval, 5)
          timer = window.setTimeout(() => void poll(next), next * 1000)
          return
        }
        if (result.status !== 'ok') throw new Error(
          result.status === 'denied' ? '已取消 GitHub 授权。'
            : result.status === 'expired' ? '授权码已过期，请重新登录。'
              : result.message || 'GitHub 授权失败，请重试。'
        )
        setStatus({ loggedIn: true, login: result.login })
        setFlow(null)
        onLogin?.()
      } catch (reason) {
        if (controller.signal.aborted) return
        setError(reason instanceof Error ? reason.message : '无法确认 GitHub 授权。')
        setFlow(null)
      }
    }
    timer = window.setTimeout(() => void poll(flow.interval), Math.max(flow.interval, 5) * 1000)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [flow, onLogin])

  const login = async () => {
    // Open synchronously so the browser associates the tab with this click.
    const popup = desktopPlatform ? null : window.open('about:blank', '_blank')
    if (popup) popup.opener = null
    setStarting(true)
    setDetailsOpen(false)
    setError('')
    setCopied(false)
    try {
      const data = await fetch('/api/v1/github/auth/device', { method: 'POST' })
        .then(readJSON<DeviceFlow>)
      const url = new URL(data.verificationUri)
      if (url.origin !== 'https://github.com') throw new Error('GitHub 授权地址无效。')
      setFlow(data)
      if (desktopPlatform) {
        await fetch('/api/v1/browser/external/open', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ url: url.href })
        }).then(readJSON)
      } else if (popup && !popup.closed) {
        popup.location.replace(url.href)
      }
    } catch (reason) {
      popup?.close()
      setError(reason instanceof Error ? reason.message : '无法启动 GitHub 授权。')
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className={menuItem ? undefined : 'grid justify-items-end'}>
      <div className="flex items-center">
        <Button
          variant={menuItem ? 'secondary' : 'primary'}
          className={menuItem ? 'workbench-action-item min-w-0 flex-1' : 'gap-1.5'}
          disabled={starting || Boolean(flow) || status?.loggedIn}
          onClick={() => void login()}
        >
          <GithubMark className="size-4" />
          {status?.loggedIn ? status.login || 'GitHub 已登录' : starting ? '正在打开 GitHub…' : '登录 GitHub'}
          {status?.loggedIn && <Check className="ml-auto size-4" aria-hidden="true" />}
        </Button>
        {menuItem && error && (
          <button
            ref={errorTrigger}
            type="button"
            className="icon-button size-7 shrink-0 border-0 p-0 text-red-600 dark:text-red-400"
            aria-label="查看 GitHub 登录错误"
            title="查看 GitHub 登录错误"
            aria-expanded={detailsOpen}
            aria-controls={detailsOpen ? errorID : undefined}
            onClick={() => setDetailsOpen(open => !open)}
            onKeyDown={event => {
              if (event.key === 'Escape' && detailsOpen) {
                event.preventDefault()
                setDetailsOpen(false)
              }
            }}
          >
            <CircleAlert className="size-4" aria-hidden="true" />
          </button>
        )}
      </div>
      {flow && (
        <div className="grid gap-1.5 px-2 pb-2 text-xs">
          <span role="status">在 GitHub 输入设备码</span>
          <div className="flex items-center justify-between gap-1">
            <code className="font-semibold">{flow.userCode}</code>
            <Button
              variant="icon"
              aria-label={copied ? '已复制设备码' : '复制设备码'}
              onClick={() => void navigator.clipboard.writeText(flow.userCode)
                .then(() => setCopied(true))
                .catch(() => setError('复制失败，可手动选择设备码。'))}
            >
              {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            </Button>
          </div>
          <div className="flex justify-between gap-2">
            <a href={flow.verificationUri} target="_blank" rel="noreferrer">打开 GitHub</a>
            <button type="button" onClick={() => { setFlow(null); setError('') }}>取消授权</button>
          </div>
        </div>
      )}
      {error && (menuItem ? (
        detailsOpen && visible && (
          <p
            id={errorID}
            ref={errorPanel}
            role="status"
            style={errorPosition}
            className="workbench-tools-menu fixed w-[min(224px,calc(100vw-24px))] overflow-auto break-words p-3 text-xs text-red-700 dark:text-red-300"
          >
            {error}
          </p>
        )
      ) : <p role="alert" className="px-2 pb-2 text-xs text-red-600">{error}</p>)}
    </div>
  )
}
