import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Check,
  Copy,
  KeyRound,
  Loader2,
  LogOut,
  X
} from 'lucide-react'
import { Button } from './Button'
import { GithubMark } from './GithubMark'

type AuthStatus = {
  loggedIn: boolean
  login?: string
  clientIdConfigured: boolean
}

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

export function GitHubAuthControl() {
  const [open, setOpen] = useState(false)
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [flow, setFlow] = useState<DeviceFlow | null>(null)
  const [polling, setPolling] = useState(false)
  const [manual, setManual] = useState(false)
  const [manualToken, setManualToken] = useState('')
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<number | null>(null)
  const rootRef = useRef<HTMLDivElement | null>(null)

  const clearTimer = () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }

  const loadStatus = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/github/auth/status')
      setStatus(await readJSON<AuthStatus>(response))
      setError('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '状态读取失败')
    }
  }, [])

  useEffect(() => {
    if (!open) return
    void loadStatus()
    return clearTimer
  }, [open, loadStatus])

  useEffect(() => {
    const closeWhenAnotherToolOpens = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== 'github') setOpen(false)
    }
    window.addEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
    return () =>
      window.removeEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
  }, [])

  useEffect(() => {
    if (!open) return
    const onPointerDown = (event: PointerEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node))
        setOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const poll = async (flowID: string, interval: number) => {
    setPolling(true)
    try {
      const response = await fetch('/api/v1/github/auth/poll', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ flowId: flowID })
      })
      const data = await readJSON<{
        status: string
        login?: string
        interval?: number
        message?: string
      }>(response)
      if (data.status === 'ok') {
        setFlow(null)
        setPolling(false)
        setError('')
        await loadStatus()
        return
      }
      if (data.status === 'pending' || data.status === 'slow_down') {
        const next = Math.max(data.interval ?? interval, 5)
        timerRef.current = window.setTimeout(
          () => void poll(flowID, next),
          next * 1000
        )
        return
      }
      setFlow(null)
      setPolling(false)
      setError(
        data.status === 'expired'
          ? '授权码已过期，请重新发起授权。'
          : data.status === 'denied'
            ? '已取消 GitHub 授权。'
            : data.message || 'GitHub 授权失败。'
      )
    } catch (reason) {
      setPolling(false)
      setError(reason instanceof Error ? reason.message : '授权轮询失败，请重试。')
    }
  }

  const startFlow = async () => {
    setLoading(true)
    setError('')
    setManual(false)
    clearTimer()
    try {
      const response = await fetch('/api/v1/github/auth/device', {
        method: 'POST'
      })
      const data = await readJSON<DeviceFlow>(response)
      setFlow(data)
      timerRef.current = window.setTimeout(
        () => void poll(data.flowId, data.interval),
        Math.max(data.interval, 5) * 1000
      )
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '无法启动 GitHub 授权。')
    } finally {
      setLoading(false)
    }
  }

  const cancelFlow = () => {
    clearTimer()
    setFlow(null)
    setPolling(false)
    setError('')
  }

  const saveManual = async () => {
    const token = manualToken.trim()
    if (!token) {
      setError('请输入 GitHub Token。')
      return
    }
    setLoading(true)
    setError('')
    try {
      const response = await fetch('/api/v1/github/auth/token', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token })
      })
      const data = await readJSON<AuthStatus>(response)
      setStatus(data)
      setManual(false)
      setManualToken('')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Token 保存失败。')
    } finally {
      setLoading(false)
    }
  }

  const logout = async () => {
    setLoading(true)
    setError('')
    try {
      const response = await fetch('/api/v1/github/auth/logout', {
        method: 'POST'
      })
      await readJSON<{ loggedOut: boolean }>(response)
      setFlow(null)
      setStatus(current =>
        current ? { ...current, loggedIn: false, login: '' } : current
      )
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '退出登录失败。')
    } finally {
      setLoading(false)
    }
  }

  const copyCode = async () => {
    if (!flow) return
    try {
      await navigator.clipboard.writeText(flow.userCode)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard may be unavailable; the code is still visible to type.
    }
  }

  return (
    <div className="relative" ref={rootRef}>
      <Button
        variant="icon"
        onClick={() =>
          setOpen(value => {
            const next = !value
            if (next)
              window.dispatchEvent(
                new CustomEvent('alx:top-tool-open', { detail: 'github' })
              )
            return next
          })
        }
        aria-label="GitHub 登录状态"
        aria-expanded={open}
        title={
          status?.loggedIn
            ? `GitHub 已登录${status.login ? `：${status.login}` : ''}`
            : 'GitHub 未登录；点击查看或授权登录'
        }
      >
        <span className="relative inline-flex">
          <GithubMark className="size-4" />
          {status?.loggedIn && (
            <span className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-emerald-500 ring-2 ring-white dark:ring-slate-900" />
          )}
        </span>
      </Button>
      {open && (
        <section
          className="topbar-popover absolute right-0 top-[calc(100%+8px)] z-50 grid w-[min(340px,calc(100vw-32px))] gap-3 rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700 dark:bg-slate-900"
          role="dialog"
          aria-label="GitHub 授权"
        >
            <header className="flex items-start justify-between gap-3">
              <div className="grid gap-0.5">
                <strong className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  GitHub 连接
                </strong>
                <small className="text-xs text-slate-500 dark:text-slate-400">
                  授权后，可改善GitHub相关资源获取
                </small>
              </div>
              <button
                className="topbar-popover-close size-7"
                onClick={() => setOpen(false)}
                aria-label="关闭"
              >
                <X className="size-4" />
              </button>
            </header>
            {!status ? (
              <p className="m-0 text-sm text-slate-500">正在读取登录状态…</p>
            ) : status.loggedIn ? (
              <div className="grid gap-3">
                <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs font-semibold text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-300">
                  <Check className="size-4" />
                  {status.login ? `已登录：${status.login}` : '已登录 GitHub'}
                </div>
                <button
                  className="inline-flex min-h-8 items-center justify-center gap-1.5 rounded-md border border-slate-200 px-2.5 text-xs font-semibold text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                  disabled={loading}
                  onClick={() => void logout()}
                >
                  <LogOut className="size-3.5" />
                  退出登录
                </button>
              </div>
            ) : flow ? (
              <div className="grid gap-3">
                <p className="m-0 text-xs leading-5 text-slate-600 dark:text-slate-300">
                  在浏览器打开以下链接，输入设备码完成授权：
                </p>
                <div className="flex items-center justify-between gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 dark:border-slate-700 dark:bg-slate-800">
                  <code className="text-lg font-bold tracking-widest text-slate-900 dark:text-slate-100">
                    {flow.userCode}
                  </code>
                  <button
                    className="inline-flex min-h-7 items-center gap-1 rounded-md border border-slate-200 bg-white px-2 text-[11px] font-semibold text-slate-600 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-300"
                    onClick={() => void copyCode()}
                  >
                    {copied ? (
                      <Check className="size-3.5 text-emerald-600" />
                    ) : (
                      <Copy className="size-3.5" />
                    )}
                    {copied ? '已复制' : '复制'}
                  </button>
                </div>
                <a
                  className="inline-flex min-h-8 items-center justify-center rounded-md bg-brand-600 px-3 text-xs font-semibold text-white transition hover:bg-brand-700"
                  href={flow.verificationUri}
                  target="_blank"
                  rel="noreferrer"
                >
                  打开 {flow.verificationUri}
                </a>
                <p className="m-0 flex items-center gap-1.5 text-[11px] text-slate-500 dark:text-slate-400">
                  {polling && <Loader2 className="size-3 animate-spin" />}
                  等待你在浏览器中确认…（{Math.ceil(flow.expiresIn / 60)} 分钟内有效）
                </p>
                <button
                  className="text-left text-[11px] font-semibold text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200"
                  onClick={cancelFlow}
                >
                  取消授权
                </button>
              </div>
            ) : (
              <div className="grid gap-3">
                {!status.clientIdConfigured && (
                  <p className="m-0 rounded-md border border-amber-200 bg-amber-50 px-2.5 py-2 text-[11px] leading-4 text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300">
                    尚未配置 GitHub OAuth App 的 Client ID；可先手动填写
                    Token，或在“设置 → GitHub”中填写 Client ID。
                  </p>
                )}
                <button
                  className="inline-flex min-h-8 items-center justify-center gap-1.5 rounded-md bg-slate-900 px-3 text-xs font-semibold text-white transition hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
                  disabled={loading || !status.clientIdConfigured}
                  onClick={() => void startFlow()}
                >
                  <GithubMark className="size-3.5" />
                  {loading ? '启动中…' : '使用 GitHub 登录'}
                </button>
                <div className="flex items-center gap-2 text-[11px] text-slate-400">
                  <span className="h-px flex-1 bg-slate-200 dark:bg-slate-700" />
                  或
                  <span className="h-px flex-1 bg-slate-200 dark:bg-slate-700" />
                </div>
                {manual ? (
                  <div className="grid gap-2">
                    <label className="grid gap-1 text-[11px] font-semibold text-slate-600 dark:text-slate-300">
                      GitHub Token
                      <input
                        className="min-h-8 rounded-md border border-slate-300 bg-white px-2 text-xs text-slate-800 outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
                        type="password"
                        value={manualToken}
                        onChange={event => setManualToken(event.target.value)}
                        placeholder="ghp_… / gho_…"
                        autoFocus
                      />
                    </label>
                    <div className="flex gap-2">
                      <button
                        className="inline-flex min-h-8 flex-1 items-center justify-center gap-1.5 rounded-md border border-slate-200 px-2 text-xs font-semibold text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                        onClick={() => setManual(false)}
                      >
                        返回
                      </button>
                      <button
                        className="inline-flex min-h-8 flex-1 items-center justify-center gap-1.5 rounded-md bg-brand-600 px-2 text-xs font-semibold text-white hover:bg-brand-700"
                        disabled={loading}
                        onClick={() => void saveManual()}
                      >
                        <KeyRound className="size-3.5" />
                        保存
                      </button>
                    </div>
                  </div>
                ) : (
                  <button
                    className="text-left text-[11px] font-semibold text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200"
                    onClick={() => setManual(true)}
                  >
                    手动填写 Token
                  </button>
                )}
              </div>
            )}
            {error && (
              <p className="m-0 rounded-md border border-red-200 bg-red-50 px-2.5 py-2 text-[11px] leading-4 text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300">
                {error}
              </p>
            )}
        </section>
      )}
    </div>
  )
}
