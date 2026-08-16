import { useCallback, useEffect, useState } from 'react'
import { ShieldCheck, Trash2, UserPlus, UsersRound } from 'lucide-react'
import { Button } from './Button'
import { SettingsCard, SettingsMessage, SettingsPage } from './SettingsCard'

type Role = { id: string; name: string; permissions: string[] }
type Account = {
  account: string
  roles: string[]
  superAdmin: boolean
  enabled: boolean
}
type Management = {
  current: { account?: string; superAdmin?: boolean }
  accounts: Account[]
  roles: Role[]
  permissions: string[]
}

const permissionLabels: Record<string, string> = {
  'workbench.view': '查看工作台',
  'workbench.manage': '管理工作台',
  'system.manage': '管理系统设置',
  'operations.view': '查看运维',
  'operations.manage': '管理运维'
}

async function request(path: string, init?: RequestInit) {
  const response = await fetch(`/api/v1/auth/${path}`, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init
  })
  const data = (await response.json()) as { error?: string }
  if (!response.ok) throw new Error(data.error || '账户操作未完成。')
  return data
}

function asStringList(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : []
}

function normalizeManagement(value: unknown): Management {
  const source =
    value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
  const current =
    source.current && typeof source.current === 'object'
      ? (source.current as Management['current'])
      : {}
  const accounts = Array.isArray(source.accounts)
    ? source.accounts.flatMap(item => {
        if (!item || typeof item !== 'object') return []
        const account = item as Record<string, unknown>
        return typeof account.account === 'string'
          ? [
              {
                account: account.account,
                roles: asStringList(account.roles),
                superAdmin: account.superAdmin === true,
                enabled: account.enabled !== false
              }
            ]
          : []
      })
    : []
  const roles = Array.isArray(source.roles)
    ? source.roles.flatMap(item => {
        if (!item || typeof item !== 'object') return []
        const role = item as Record<string, unknown>
        return typeof role.id === 'string' && typeof role.name === 'string'
          ? [
              {
                id: role.id,
                name: role.name,
                permissions: asStringList(role.permissions)
              }
            ]
          : []
      })
    : []
  return {
    current,
    accounts,
    roles,
    permissions: asStringList(source.permissions)
  }
}

export function AccountManagementPage() {
  const [data, setData] = useState<Management | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [account, setAccount] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [accountRoles, setAccountRoles] = useState<string[]>([])
  const [roleID, setRoleID] = useState('')
  const [roleName, setRoleName] = useState('')
  const [rolePermissions, setRolePermissions] = useState<string[]>([])
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      setData(normalizeManagement(await request('management')))
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
    <SettingsPage title="账户" description="管理登录账户、角色及系统权限">
      {loading && <SettingsMessage>正在读取账户配置…</SettingsMessage>}
      {error && <SettingsMessage tone="error">{error}</SettingsMessage>}
      {data && (
        <>
          <SettingsCard
            icon={<UserPlus className="size-4" />}
            title="新增非超级管理员账户"
            description="新账户必须归属至少一个已创建的角色。"
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
            <RolePicker
              roles={data.roles}
              selected={accountRoles}
              onChange={setAccountRoles}
            />
            <div className="settings-card-actions settings-card-actions-end">
              <Button
                variant="primary"
                className="gap-1.5"
                disabled={
                  !account ||
                  !password ||
                  !confirmation ||
                  accountRoles.length === 0 ||
                  busy
                }
                onClick={() =>
                  void run(async () => {
                    await request('accounts', {
                      method: 'POST',
                      body: JSON.stringify({
                        account,
                        password,
                        confirmation,
                        roles: accountRoles
                      })
                    })
                    setAccount('')
                    setPassword('')
                    setConfirmation('')
                    setAccountRoles([])
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
            description="超级管理员拥有全部权限，不能被此处删除或降权。"
          >
            <div className="grid gap-2">
              {data.accounts.map(item => (
                <article
                  className="account-row grid gap-2 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) p-3"
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
                  {item.superAdmin ? (
                    <span className="text-xs text-(--theme-text-muted)">
                      拥有系统全部权限
                    </span>
                  ) : (
                    <RolePicker
                      compact
                      roles={data.roles}
                      selected={item.roles}
                      onChange={roles =>
                        void run(() =>
                          request(
                            `accounts/${encodeURIComponent(item.account)}`,
                            { method: 'PATCH', body: JSON.stringify({ roles }) }
                          )
                        )
                      }
                    />
                  )}
                  {!item.superAdmin && (
                    <Button
                      variant="icon"
                      className="justify-self-start text-(--theme-danger)"
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

          <SettingsCard
            icon={<ShieldCheck className="size-4" />}
            title="新建角色与权限"
            description="一个账户可以同时拥有多个角色，最终权限为各角色权限的并集。"
          >
            <div className="account-role-fields grid gap-3">
              <input
                className="settings-input"
                placeholder="角色标识，例如 release-manager"
                value={roleID}
                onChange={event => setRoleID(event.target.value)}
              />
              <input
                className="settings-input"
                placeholder="角色名称，例如 发布管理员"
                value={roleName}
                onChange={event => setRoleName(event.target.value)}
              />
            </div>
            <PermissionPicker
              permissions={data.permissions}
              selected={rolePermissions}
              onChange={setRolePermissions}
            />
            <div className="settings-card-actions settings-card-actions-end">
              <Button
                variant="primary"
                className="gap-1.5"
                disabled={!roleID || !roleName || busy}
                onClick={() =>
                  void run(async () => {
                    await request('roles', {
                      method: 'POST',
                      body: JSON.stringify({
                        id: roleID,
                        name: roleName,
                        permissions: rolePermissions
                      })
                    })
                    setRoleID('')
                    setRoleName('')
                    setRolePermissions([])
                  })
                }
              >
                新建角色
              </Button>
            </div>
            {data.roles.length > 0 && (
              <div className="grid gap-2 border-t border-(--theme-border-subtle) pt-3">
                {data.roles.map(role => (
                  <div
                    className="flex items-center justify-between gap-3 text-xs"
                    key={role.id}
                  >
                    <span>
                      <b className="text-(--theme-text-strong)">{role.name}</b>{' '}
                      <span className="text-(--theme-text-muted)">
                        ({role.id})
                      </span>{' '}
                      ·{' '}
                      {role.permissions
                        .map(
                          permission =>
                            permissionLabels[permission] || permission
                        )
                        .join('、') || '无权限'}
                    </span>
                    <Button
                      variant="icon"
                      className="size-7 text-(--theme-danger)"
                      title="删除角色"
                      aria-label={`删除角色 ${role.name}`}
                      disabled={busy}
                      onClick={() =>
                        void run(() =>
                          request(`roles/${encodeURIComponent(role.id)}`, {
                            method: 'DELETE'
                          })
                        )
                      }
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </SettingsCard>
        </>
      )}
    </SettingsPage>
  )
}

function RolePicker({
  roles,
  selected,
  onChange,
  compact = false
}: {
  roles: Role[]
  selected: string[]
  onChange: (roles: string[]) => void
  compact?: boolean
}) {
  if (roles.length === 0)
    return (
      <span className="text-xs text-(--theme-warning-text)">请先新建角色</span>
    )
  return (
    <div
      className={`flex flex-wrap gap-2 ${
        compact
          ? ''
          : 'rounded-md border border-(--theme-border-default) bg-(--theme-surface-panel) p-2'
      }`}
    >
      {roles.map(role => (
        <label
          className="inline-flex cursor-pointer items-center gap-1.5 text-xs text-(--theme-text-secondary)"
          key={role.id}
        >
          <input
            type="checkbox"
            checked={selected.includes(role.id)}
            onChange={() =>
              onChange(
                selected.includes(role.id)
                  ? selected.filter(id => id !== role.id)
                  : [...selected, role.id]
              )
            }
          />
          {role.name}
        </label>
      ))}
    </div>
  )
}

function PermissionPicker({
  permissions,
  selected,
  onChange
}: {
  permissions: string[]
  selected: string[]
  onChange: (permissions: string[]) => void
}) {
  return (
    <div className="account-permission-grid grid gap-2 rounded-md border border-(--theme-border-default) bg-(--theme-surface-raised) p-3">
      {permissions.map(permission => (
        <label
          className="flex cursor-pointer items-center gap-2 text-xs text-(--theme-text-secondary)"
          key={permission}
        >
          <input
            type="checkbox"
            checked={selected.includes(permission)}
            onChange={() =>
              onChange(
                selected.includes(permission)
                  ? selected.filter(value => value !== permission)
                  : [...selected, permission]
              )
            }
          />
          {permissionLabels[permission] || permission}
        </label>
      ))}
    </div>
  )
}
