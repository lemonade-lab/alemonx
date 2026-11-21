import { authRequest, readAuthStatus, type AuthStatus } from '../lib/workbenchAuth'
import { useStoreState } from '../store/guideStore'
import {
  Fragment,
  useCallback,
  useEffect,
  useRef,
  type FormEvent,
  type ReactNode
} from 'react'
import { LockKeyhole, LogOut, UserRound, X } from 'lucide-react'
import { Button } from './Button'
import { desktopPlatform } from './nativeWindow'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'
import { WorkbenchGitHubFlow } from './WorkbenchGitHubFlow'
import {
  canRememberPassword,
  lastAccount,
  readRememberedPassword,
  rememberAccount,
  remembersPassword,
  setRememberedPassword
} from './workbenchCredentials'

function notifyAuthChanged() {
  window.dispatchEvent(new Event('alx:auth-changed'))
  if (desktopPlatform) {
    const channel = new BroadcastChannel('alx-native-auth')
    channel.postMessage('changed')
    channel.close()
  }
}

export function AuthGate({
  children,
  initialStatus
}: {
  children: (status: AuthStatus) => ReactNode
  initialStatus: Promise<AuthStatus>
}) {
  const [status, setStatus] = useStoreState<AuthStatus | null>(null)
  const [loading, setLoading] = useStoreState(true)
  const [loadError, setLoadError] = useStoreState('')
  const [account, setAccount] = useStoreState('')
  const [remember, setRemember] = useStoreState(false)
  const [githubPending, setGitHubPending] = useStoreState(false)
  const passwordChanged = useRef(false)
  const [password, setPassword] = useStoreState('')
  const [error, setError] = useStoreState('')
  const [busy, setBusy] = useStoreState(false)
  const requestID = useRef(0)
  const loadStatus = useCallback((request: Promise<AuthStatus>) => {
    const id = ++requestID.current
    setLoading(true)
    setLoadError('')
    void request
      .then(value => {
        if (id === requestID.current) setStatus(value)
      })
      .catch(reason => {
        if (id !== requestID.current) return
        setStatus(null)
        setLoadError(
          reason instanceof Error ? reason.message : '无法读取身份认证状态。'
        )
      })
      .finally(() => {
        if (id === requestID.current) setLoading(false)
      })
  }, [setLoadError, setLoading, setStatus])
  const refresh = useCallback(() => loadStatus(readAuthStatus()), [loadStatus])
  useEffect(() => {
    loadStatus(initialStatus)
    window.addEventListener('alx:auth-changed', refresh)
    const channel = desktopPlatform
      ? new BroadcastChannel('alx-native-auth')
      : null
    if (channel) {
      channel.onmessage = () => {
        setStatus(null)
        window.dispatchEvent(new Event('alx:auth-changed'))
      }
    }
    return () => {
      requestID.current += 1
      channel?.close()
      window.removeEventListener('alx:auth-changed', refresh)
    }
  }, [initialStatus, loadStatus, refresh, setStatus])
  const loginVisible = status?.enabled === true && !status.authenticated
  useEffect(() => {
    if (!loginVisible) return
    const saved = lastAccount()
    const controller = new AbortController()
    passwordChanged.current = false
    setAccount(saved)
    setPassword('')
    setRemember(remembersPassword(saved))
    void readRememberedPassword(saved, controller.signal)
      .then(value => {
        if (!controller.signal.aborted && !passwordChanged.current)
          setPassword(value)
      })
      .catch(reason => {
        if (!controller.signal.aborted) {
          setError(
            reason instanceof Error ? reason.message : '无法读取已记住的密码。'
          )
        }
      })
    return () => controller.abort()
  }, [loginVisible, setAccount, setError, setPassword, setRemember])

  const completeLogin = useCallback(
    (signedInAccount?: string) => {
      if (signedInAccount) rememberAccount(signedInAccount)
      setPassword('')
      setError('')
      notifyAuthChanged()
    },
    [setError, setPassword]
  )

  const login = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (busy || githubPending) return
    const values = new FormData(event.currentTarget)
    const username = String(values.get('username') ?? '').trim()
    const secret = String(values.get('password') ?? '')
    const rememberPassword = values.has('remember-password')
    if (!username || !secret) return
    setBusy(true)
    setError('')
    try {
      const result = await authRequest('login', {
        method: 'POST',
        body: JSON.stringify({ account: username, password: secret })
      })
      const signedInAccount = result.account || username
      try {
        await setRememberedPassword(signedInAccount, secret, rememberPassword)
      } catch {
        window.alert('已登录。系统或浏览器密码管理器未能更新已保存的密码。')
      }
      completeLogin(signedInAccount)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '登录未完成。')
    } finally {
      setBusy(false)
    }
  }
  if (loadError)
    return (
      <main className="auth-gate flex min-h-screen items-center justify-center p-5">
        <section className="grid w-full max-w-90 gap-3 rounded-xl border border-slate-200 bg-white p-6 text-center shadow-[0_18px_52px_rgb(28_26_23/0.12)] dark:border-slate-700 dark:bg-slate-900">
          <div className="grid gap-1">
            <strong className="text-sm text-slate-800 dark:text-slate-100">
              无法连接到后台服务
            </strong>
            <p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">
              {loadError}。请确认 alx 后台仍在运行，然后重试。
            </p>
          </div>
          <button
            className="primary-button"
            onClick={refresh}
            disabled={loading}
          >
            {loading ? '重试中…' : '重试'}
          </button>
        </section>
      </main>
    )
  if (!status)
    return (
      <main className="auth-gate flex min-h-screen items-center justify-center p-5 text-sm text-slate-500">
        <span>正在读取身份认证状态…</span>
      </main>
    )
  if (!status.enabled || status.authenticated)
    return (
      <Fragment key={status.enabled ? `account:${status.account}` : 'local'}>
        {children(status)}
      </Fragment>
    )
  return (
    <main className="auth-gate flex min-h-screen items-center justify-center p-5">
      <section className="grid w-full max-w-90 gap-4 rounded-xl border border-slate-200 bg-white p-6 shadow-[0_18px_52px_rgb(28_26_23/0.12)] dark:border-slate-700 dark:bg-slate-900">
        <LockKeyhole className="size-6 text-brand-600" />
        <div>
          <strong className="text-sm text-(--theme-text-strong)">
            身份认证
          </strong>
          <p className="mt-1 text-xs leading-5 text-(--theme-text-secondary)">
            此 alx 服务已开启账户保护，请登录后继续。
          </p>
        </div>
        <form className="grid gap-3" onSubmit={event => void login(event)}>
          <label className="grid gap-1 text-xs font-semibold text-(--theme-text-secondary)">
            账户
            <input
              className="settings-input"
              name="username"
              autoComplete="username"
              value={account}
              disabled={busy || githubPending}
              required
              onChange={event => {
                passwordChanged.current = true
                setAccount(event.target.value)
                setPassword('')
                setRemember(remembersPassword(event.target.value))
              }}
            />
          </label>
          <label className="grid gap-1 text-xs font-semibold text-(--theme-text-secondary)">
            密码
            <input
              className="settings-input"
              name="password"
              autoComplete="current-password"
              type="password"
              value={password}
              disabled={busy || githubPending}
              required
              onChange={event => {
                passwordChanged.current = true
                setPassword(event.target.value)
              }}
            />
          </label>
          {canRememberPassword && (
            <label className="flex items-center gap-2 text-xs text-(--theme-text-secondary)">
              <input
                type="checkbox"
                name="remember-password"
                checked={remember}
                disabled={busy || githubPending}
                onChange={event => setRemember(event.target.checked)}
              />
              记住密码
            </label>
          )}
          {error && (
            <p
              role="alert"
              className="m-0 text-xs text-red-700 dark:text-red-300"
            >
              {error}
            </p>
          )}
          <Button
            type="submit"
            variant="primary"
            loading={busy}
            loadingLabel="正在登录…"
            disabled={githubPending}
          >
            登录
          </Button>
        </form>
        <WorkbenchGitHubFlow
          mode="login"
          disabled={busy}
          onComplete={completeLogin}
          onPendingChange={setGitHubPending}
        />
      </section>
    </main>
  )
}

export function AuthControl({ embedded = false }: { embedded?: boolean }) {
  const [status, setStatus] = useStoreState<AuthStatus | null>(null)
  const [open, setOpen] = useStoreState(embedded)
  const [account, setAccount] = useStoreState('')
  const [password, setPassword] = useStoreState('')
  const [confirmation, setConfirmation] = useStoreState('')
  const [error, setError] = useStoreState('')
  const [busy, setBusy] = useStoreState(false)
  const refresh = useCallback(() => {
    void readAuthStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [setStatus])
  useEffect(() => {
    refresh()
    window.addEventListener('alx:auth-changed', refresh)
    return () => window.removeEventListener('alx:auth-changed', refresh)
  }, [refresh])
  useEffect(() => {
    if (embedded) return
    const closeWhenAnotherToolOpens = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== 'auth') setOpen(false)
    }
    window.addEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
    return () =>
      window.removeEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
  }, [embedded, setOpen])
  const enable = async () => {
    setBusy(true)
    setError('')
    try {
      const result = await authRequest('setup', {
        method: 'POST',
        body: JSON.stringify({ account, password, confirmation })
      })
      setPassword('')
      setConfirmation('')
      if (result.account) rememberAccount(result.account)
      if (!embedded) setOpen(false)
      notifyAuthChanged()
      refresh()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '身份认证未开启。')
    } finally {
      setBusy(false)
    }
  }
  const logout = async () => {
    try {
      await authRequest('logout', { method: 'POST' })
      notifyAuthChanged()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '退出登录失败。')
    }
  }
  const unlink = async () => {
    setBusy(true)
    setError('')
    try {
      await authRequest('github/binding', { method: 'DELETE' })
      refresh()
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : '解除 GitHub 绑定失败。'
      )
    } finally {
      setBusy(false)
    }
  }
  const content = status?.enabled ? (
    <>
      <p className="m-0 text-xs leading-5 text-(--theme-text-secondary)">
        当前账户：
        <b className="text-(--theme-text-strong)">
          {status.account || '已登录'}
        </b>
      </p>
      {status.githubLogin ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="m-0 text-xs text-(--theme-text-secondary)">
            已绑定 GitHub @{status.githubLogin}
          </p>
          <Button
            type="button"
            variant="secondary"
            loading={busy}
            onClick={() => void unlink()}
          >
            解除绑定
          </Button>
        </div>
      ) : (
        <WorkbenchGitHubFlow mode="binding" onComplete={refresh} />
      )}
      {error && <SettingsMessage tone="error">{error}</SettingsMessage>}
      <div className="settings-card-actions">
        <Button
          variant="secondary"
          className="gap-1.5"
          onClick={() => void logout()}
        >
          <LogOut className="size-3.5" />
          退出登录
        </Button>
      </div>
    </>
  ) : (
    <>
      <p className="m-0 text-xs leading-5 text-(--theme-text-secondary)">
        开启后，访问本机管理 API 前必须登录。
      </p>
      <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
        账户
        <input
          className="settings-input"
          autoComplete="username"
          value={account}
          onChange={event => setAccount(event.target.value)}
        />
      </label>
      <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
        密码
        <input
          className="settings-input"
          autoComplete="new-password"
          type="password"
          value={password}
          onChange={event => setPassword(event.target.value)}
        />
      </label>
      <label className="grid gap-1.5 text-xs font-semibold text-(--theme-text-secondary)">
        确认密码
        <input
          className="settings-input"
          autoComplete="new-password"
          type="password"
          value={confirmation}
          onChange={event => setConfirmation(event.target.value)}
        />
      </label>
      {error && <SettingsMessage tone="error">{error}</SettingsMessage>}
      <div className="settings-card-actions settings-card-actions-end">
        <Button
          variant="primary"
          className="gap-1.5"
          loading={busy}
          loadingLabel="正在开启…"
          disabled={!account || !password || !confirmation}
          onClick={() => void enable()}
        >
          <UserRound className="size-3.5" />
          开启身份认证
        </Button>
      </div>
    </>
  )
  return (
    <div className={embedded ? 'settings-auth-panel' : 'relative'}>
      {!embedded && (
        <Button
          variant="icon"
          className={
            status?.enabled ? 'bg-brand-50 text-brand-600' : ''
          }
          onClick={() =>
            setOpen(value => {
              const next = !value
              if (next)
                window.dispatchEvent(
                  new CustomEvent('alx:top-tool-open', { detail: 'auth' })
                )
              return next
            })
          }
          aria-label="身份认证"
          aria-expanded={open}
          title="身份认证"
        >
          <LockKeyhole className="size-4" />
        </Button>
      )}
      {open &&
        (embedded ? (
          <SettingsPage
            title="认证"
            description="控制本机管理 API 的账户保护。"
          >
            <SettingsCard
              icon={<LockKeyhole className="size-4" />}
              title={status?.enabled ? '身份认证已开启' : '开启身份认证'}
              description="开启后，访问本机管理 API 前必须登录。"
            >
              {content}
            </SettingsCard>
          </SettingsPage>
        ) : (
          <section
            className="topbar-popover absolute right-0 top-[calc(100%+8px)] z-50 grid w-[min(22.5rem,calc(100vw-2rem))] max-h-[calc(100dvh-5rem)] gap-2.5 overflow-y-auto rounded-xl border border-slate-200 bg-white p-3 shadow-[0_18px_42px_rgb(28_26_23/0.13)]"
            role="dialog"
            aria-label="身份认证"
            onKeyDown={event => {
              if (event.key === 'Escape') setOpen(false)
            }}
          >
            <header className="flex items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-2">
                <strong className="text-xs text-slate-800">
                  {status?.enabled ? '身份认证已开启' : '开启身份认证'}
                </strong>
                {error && !status?.enabled && (
                  <small
                    className="truncate text-[11px] text-amber-700"
                    title={error}
                  >
                    操作失败
                  </small>
                )}
              </div>
              <Button
                variant="icon"
                className="topbar-popover-close size-6"
                onClick={() => setOpen(false)}
                aria-label="关闭身份认证"
              >
                <X className="size-4" />
              </Button>
            </header>
            {content}
          </section>
        ))}
    </div>
  )
}
