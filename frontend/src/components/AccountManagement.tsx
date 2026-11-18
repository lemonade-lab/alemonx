import { useCallback, useEffect, useState } from 'react'
import { Trash2, UserPlus, UsersRound } from 'lucide-react'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

type Account = {
  account: string
  superAdmin?: boolean
}
type Management = {
  accounts: Account[]
}

async function request<T = unknown>(path: string, init?: RequestInit) {
  const response = await fetch(`/api/v1/auth/${path}`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init
  })
  const data = (await response.json()) as { error?: string }
  if (!response.ok) throw new Error(data.error || '账户操作未完成。')
  return data as T
}

export function AccountManagementPage() {
  const [data, setData] = useState<Management | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      setData(await request<Management>('management'))
      setError('')
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : '无法读取账户管理数据。'
      )
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => void refresh(), [refresh])

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await action()
      await refresh()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '账户操作未完成。')
    } finally {
      setBusy(false)
    }
  }
  return (
    <SettingsPage title="账户" description="新增和删除登录账户">
      {loading && <SettingsMessage>正在读取账户配置…</SettingsMessage>}
      {error && <SettingsMessage tone="error">{error}</SettingsMessage>}
      {data && (
        <>
          <SettingsCard
            icon={<UserPlus className="size-4" />}
            title="新增普通账户"
          >
            <div className="account-create-fields grid gap-3">
              <input
                className="settings-input"
                placeholder="账户名"
                value={account}
                onChange={event => setAccount(event.target.value)}
              />
              <input
                className="settings-input"
                placeholder="密码"
                type="password"
                value={password}
                onChange={event => setPassword(event.target.value)}
              />
              <input
                className="settings-input"
                placeholder="确认密码"
                type="password"
                value={confirmation}
                onChange={event => setConfirmation(event.target.value)}
              />
            </div>
            <div className="settings-card-actions settings-card-actions-end">
              <Button
                variant="primary"
                className="gap-1.5"
                disabled={!account.trim() || !password || !confirmation || busy}
                onClick={() =>
                  void run(async () => {
                    await request('accounts', {
                      method: 'POST',
                      body: JSON.stringify({
                        account,
                        password,
                        confirmation
                      })
                    })
                    setAccount('')
                    setPassword('')
                    setConfirmation('')
                  })
                }
              >
                <UserPlus className="size-3.5" />
                新增账户
              </Button>
            </div>
          </SettingsCard>

          <SettingsCard
            icon={<UsersRound className="size-4" />}
            title="账户列表"
            description="超级管理员负责账户管理和 MCP 服务启停。"
          >
            <div className="grid gap-2">
              {data.accounts.map(item => (
                <article
                  className="account-row flex items-center justify-between gap-2 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) p-3"
                  key={item.account}
                >
                  <div className="flex items-center gap-2">
                    <strong className="text-sm text-(--theme-text-strong)">
                      {item.account}
                    </strong>
                    <small className="text-xs text-(--theme-text-muted)">
                      {item.superAdmin ? '超级管理员' : '普通账户'}
                    </small>
                  </div>
                  {!item.superAdmin && (
                    <Button
                      variant="icon"
                      className="text-(--theme-danger)"
                      aria-label={`删除 ${item.account}`}
                      title="删除账户"
                      disabled={busy}
                      onClick={() =>
                        void run(() =>
                          request(
                            `accounts/${encodeURIComponent(item.account)}`,
                            { method: 'DELETE' }
                          )
                        )
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  )}
                </article>
              ))}
            </div>
          </SettingsCard>
        </>
      )}
    </SettingsPage>
  )
}
