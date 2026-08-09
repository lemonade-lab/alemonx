import { useCallback, useEffect, useState } from 'react'
import { ShieldCheck, Trash2, UserPlus } from 'lucide-react'
import { Button } from './Button'
import { BotWorkspace } from './BotWorkspace'
import { RobotPanelHeader } from './RobotPanelHeader'

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
      setData((await request('management')) as Management)
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
    <BotWorkspace
      className="max-w-250"
      header={
        <RobotPanelHeader
          icon={<ShieldCheck className="size-4" />}
          title="账户"
          description="管理登录账户、角色及系统权限"
        />
      }
    >
      {loading && <p className="text-sm text-slate-500">正在读取账户配置…</p>}
      {error && (
        <p className="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
          {error}
        </p>
      )}
      {data && (
        <div className="grid gap-5">
          <section className="grid gap-3 rounded-lg border border-slate-200 bg-slate-50/60 p-4">
            <div>
              <strong className="text-sm text-slate-800">
                新增非超级管理员账户
              </strong>
              <p className="mt-1 text-xs text-slate-500">
                新账户必须归属至少一个已创建的角色。
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <input
                className="min-h-9 rounded-md border border-slate-300 px-2.5 text-sm"
                placeholder="账户名"
                value={account}
                onChange={event => setAccount(event.target.value)}
              />
              <input
                className="min-h-9 rounded-md border border-slate-300 px-2.5 text-sm"
                placeholder="密码"
                type="password"
                value={password}
                onChange={event => setPassword(event.target.value)}
              />
              <input
                className="min-h-9 rounded-md border border-slate-300 px-2.5 text-sm"
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
            <Button
              variant="primary"
              className="justify-self-start gap-1.5"
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
          </section>

          <section className="grid gap-3">
            <div>
              <strong className="text-sm text-slate-800">账户列表</strong>
              <p className="mt-1 text-xs text-slate-500">
                超级管理员拥有全部权限，不能被此处删除或降权。
              </p>
            </div>
            <div className="grid gap-2">
              {data.accounts.map(item => (
                <article
                  className="grid gap-2 rounded-lg border border-slate-200 p-3 sm:grid-cols-[minmax(9rem,1fr)_minmax(15rem,2fr)_auto] sm:items-center"
                  key={item.account}
                >
                  <div>
                    <strong className="text-sm text-slate-800">
                      {item.account}
                    </strong>
                    <small className="ml-2 text-xs text-slate-500">
                      {item.superAdmin ? '超级管理员' : '普通账户'}
                    </small>
                  </div>
                  {item.superAdmin ? (
                    <span className="text-xs text-slate-500">
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
                      className="justify-self-start text-red-600"
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
          </section>

          <section className="grid gap-3 rounded-lg border border-slate-200 p-4">
            <div>
              <strong className="text-sm text-slate-800">新建角色与权限</strong>
              <p className="mt-1 text-xs text-slate-500">
                一个账户可以同时拥有多个角色，最终权限为各角色权限的并集。
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <input
                className="min-h-9 rounded-md border border-slate-300 px-2.5 text-sm"
                placeholder="角色标识，例如 release-manager"
                value={roleID}
                onChange={event => setRoleID(event.target.value)}
              />
              <input
                className="min-h-9 rounded-md border border-slate-300 px-2.5 text-sm"
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
            <Button
              variant="primary"
              className="justify-self-start"
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
            {data.roles.length > 0 && (
              <div className="grid gap-2 border-t border-slate-100 pt-3">
                {data.roles.map(role => (
                  <div
                    className="flex items-center justify-between gap-3 text-xs"
                    key={role.id}
                  >
                    <span>
                      <b className="text-slate-700">{role.name}</b>{' '}
                      <span className="text-slate-400">({role.id})</span> ·{' '}
                      {role.permissions
                        .map(
                          permission =>
                            permissionLabels[permission] || permission
                        )
                        .join('、') || '无权限'}
                    </span>
                    <Button
                      variant="icon"
                      className="size-7 text-red-600"
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
          </section>
        </div>
      )}
    </BotWorkspace>
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
    return <span className="text-xs text-amber-700">请先新建角色</span>
  return (
    <div
      className={`flex flex-wrap gap-2 ${compact ? '' : 'rounded-md border border-slate-200 bg-white p-2'}`}
    >
      {roles.map(role => (
        <label
          className="inline-flex cursor-pointer items-center gap-1.5 text-xs text-slate-600"
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
    <div className="grid gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 sm:grid-cols-2">
      {permissions.map(permission => (
        <label
          className="flex cursor-pointer items-center gap-2 text-xs text-slate-700"
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
