import { useEffect, useRef } from 'react'
import { useStoreState } from '../store/guideStore'
import { Button } from './Button'
import { GithubMark } from './GithubMark'
import { bridge } from './nativeWindow'

type DeviceFlow = {
  flowId: string
  userCode: string
  verificationUri: string
  expiresIn: number
  interval: number
}

type PollResult = {
  status: string
  account?: string
  interval?: number
  message?: string
}

async function request<T>(
  mode: 'login' | 'binding',
  body: { flowId: string } | { password: string },
  signal?: AbortSignal
) {
  const controller = new AbortController()
  const cancel = () => controller.abort()
  if (signal?.aborted) cancel()
  signal?.addEventListener('abort', cancel, { once: true })
  const timeout = window.setTimeout(cancel, 30000)
  try {
    const response = await fetch(`/api/v1/auth/github/${mode}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: controller.signal
    })
    const data = (await response.json()) as T & { error?: string }
    if (!response.ok) throw new Error(data.error ?? 'GitHub 授权失败。')
    return data
  } finally {
    window.clearTimeout(timeout)
    signal?.removeEventListener('abort', cancel)
  }
}

const verificationURL = 'https://github.com/login/device'

export function WorkbenchGitHubFlow({
  mode,
  disabled,
  onComplete,
  onPendingChange
}: {
  mode: 'login' | 'binding'
  disabled?: boolean
  onComplete: (account?: string) => void
  onPendingChange?: (pending: boolean) => void
}) {
  const [flow, setFlow] = useStoreState<DeviceFlow | null>(null)
  const [starting, setStarting] = useStoreState(false)
  const [error, setError] = useStoreState('')
  const [password, setPassword] = useStoreState('')
  const [bindingOpen, setBindingOpen] = useStoreState(false)
  const startRequest = useRef<AbortController | null>(null)

  useEffect(
    () => () => {
      startRequest.current?.abort()
      onPendingChange?.(false)
    },
    [onPendingChange]
  )

  useEffect(() => {
    if (!flow) return
    const controller = new AbortController()
    const deadline = Date.now() + flow.expiresIn * 1000
    let timer: number
    const poll = async () => {
      try {
        if (Date.now() >= deadline)
          throw new Error('授权码已过期，请重新授权。')
        const result = await request<PollResult>(
          mode,
          { flowId: flow.flowId },
          controller.signal
        )
        if (controller.signal.aborted) return
        if (result.status === 'pending' || result.status === 'slow_down') {
          timer = window.setTimeout(
            () => void poll(),
            Math.max(result.interval ?? flow.interval, 5) * 1000
          )
          return
        }
        if (result.status !== 'ok')
          throw new Error(
            result.status === 'denied'
              ? '已取消 GitHub 授权。'
              : result.status === 'expired'
                ? '授权码已过期，请重新授权。'
                : result.message || 'GitHub 授权失败。'
          )
        setBindingOpen(false)
        onComplete(result.account)
      } catch (reason) {
        if (controller.signal.aborted) return
        setError(
          reason instanceof Error ? reason.message : '无法确认 GitHub 授权。'
        )
      }
      setFlow(null)
      onPendingChange?.(false)
    }
    timer = window.setTimeout(
      () => void poll(),
      Math.max(flow.interval, 5) * 1000
    )
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [
    flow,
    mode,
    onComplete,
    onPendingChange,
    setBindingOpen,
    setError,
    setFlow
  ])

  const start = async () => {
    if (startRequest.current) return
    const controller = new AbortController()
    startRequest.current = controller
    setStarting(true)
    onPendingChange?.(true)
    setError('')
    try {
      const next = await request<DeviceFlow>(
        mode,
        { password },
        controller.signal
      )
      if (controller.signal.aborted) return
      if (next.verificationUri !== verificationURL)
        throw new Error('GitHub 授权地址无效。')
      setPassword('')
      setFlow(next)
    } catch (reason) {
      if (controller.signal.aborted) return
      setError(
        reason instanceof Error ? reason.message : '无法启动 GitHub 授权。'
      )
      onPendingChange?.(false)
    } finally {
      startRequest.current = null
      if (!controller.signal.aborted) setStarting(false)
    }
  }

  return (
    <div className="grid gap-2">
      {flow ? (
        <>
          <p className="m-0 text-xs text-(--theme-text-secondary)">
            在 GitHub 输入授权码{' '}
            <code className="select-all font-bold">{flow.userCode}</code>
          </p>
          <div className="flex flex-wrap gap-2">
            {bridge ? (
              <Button
                variant="secondary"
                onClick={() => bridge?.postMessage('alx:github-device:open')}
              >
                打开 GitHub
              </Button>
            ) : (
              <a
                className="text-sm underline"
                href={verificationURL}
                target="_blank"
                rel="noreferrer"
              >
                打开 GitHub
              </a>
            )}
            <Button
              variant="secondary"
              onClick={() => {
                setFlow(null)
                setBindingOpen(false)
                onPendingChange?.(false)
              }}
            >
              取消授权
            </Button>
          </div>
        </>
      ) : (
        <>
          {bindingOpen && (
            <label className="grid gap-1 text-xs text-(--theme-text-secondary)">
              当前工作台密码
              <input
                type="password"
                className="settings-input"
                autoComplete="current-password"
                value={password}
                onChange={event => setPassword(event.target.value)}
                disabled={starting}
              />
            </label>
          )}
          <Button
            variant="secondary"
            type="button"
            className="gap-2"
            disabled={disabled || (bindingOpen && !password)}
            loading={starting}
            onClick={() => {
              if (mode === 'binding' && !bindingOpen) setBindingOpen(true)
              else void start()
            }}
          >
            <GithubMark className="size-4" />
            {mode === 'login'
              ? '使用 GitHub 登录'
              : bindingOpen
                ? '确认并授权'
                : '绑定 GitHub'}
          </Button>
          {bindingOpen && (
            <Button
              variant="secondary"
              disabled={starting}
              onClick={() => {
                setBindingOpen(false)
                setPassword('')
                setError('')
              }}
            >
              取消绑定
            </Button>
          )}
        </>
      )}
      {error && (
        <p role="alert" className="m-0 text-xs text-red-700 dark:text-red-300">
          {error}
        </p>
      )}
    </div>
  )
}
