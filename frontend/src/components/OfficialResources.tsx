import { resourceTarget, findResource, factFor } from '../lib/officialResources'
import { useState } from 'react'
import Markdown from 'markdown-to-jsx'
import {
  useResourceOptionsQuery,
  useMarketResourceQuery,
  useRobotRuntimeQuery,
  usePackageInventoryQuery,
  type MarketResource
} from '../store/workspaceApi'

export function OfficialExtensionChoices({
  label,
  selected,
  onSelect
}: {
  label: string
  selected: string[]
  onSelect: (resource: MarketResource) => void
}) {
  const { data, isFetching, error, refetch } = useResourceOptionsQuery(
    'connector,js-plugin'
  )
  return (
    <div className="grid gap-2.5" aria-label={label}>
      {error ? (
        <p role="alert" className="text-xs">
          扩展选项暂时无法读取。
          <button
            type="button"
            className="text-button"
            onClick={() => void refetch()}
          >
            重试
          </button>
        </p>
      ) : isFetching ? (
        <p role="status" className="text-xs">
          正在读取扩展选项…
        </p>
      ) : (
        (data?.data ?? []).map(resource => (
          <button
            type="button"
            key={resource.id}
            className={
              selected.includes(resource.id) ? 'choice selected' : 'choice'
            }
            aria-pressed={selected.includes(resource.id)}
            disabled={!resourceTarget(resource)}
            onClick={() => onSelect(resource)}
          >
            <strong>
              {resource.name === resource.packageName
                ? resource.description || resource.name
                : resource.name}
            </strong>
            <small>
              {selected.includes(resource.id) ? '已选择' : '点击添加'}
            </small>
          </button>
        ))
      )}
    </div>
  )
}

export function OfficialDependencyOptions({ id }: { id: string }) {
  const { data } = useResourceOptionsQuery('connector,js-plugin')
  return (
    <datalist id={id}>
      {(data?.data ?? [])
        .filter(item => item.installMode === 'npm' && resourceTarget(item))
        .map(item => (
          <option key={item.id} value={item.packageName}>
            {item.name === item.packageName ? item.description : item.name}
          </option>
        ))}
    </datalist>
  )
}

export function ResourceDocument({ id }: { id: string }) {
  const { data, isFetching, error, refetch } = useMarketResourceQuery(id)
  return (
    <div className="grid gap-2 text-sm">
      {isFetching ? (
        <p role="status">正在读取平台文档…</p>
      ) : error ? (
        <p role="alert">
          文档读取失败。
          <button className="text-button" onClick={() => void refetch()}>
            重试
          </button>
        </p>
      ) : (
        <article className="markdown-page">
          <Markdown
            options={{
              disableParsingRawHTML: true,
              overrides: {
                a: { props: { target: '_blank', rel: 'noreferrer' } }
              }
            }}
          >
            {data?.data.markdown || '平台暂未提供正文。'}
          </Markdown>
        </article>
      )}
    </div>
  )
}
export function OfficialResourceInfo({
  name,
  repository
}: {
  name: string
  repository?: string
}) {
  const [open, setOpen] = useState(false)
  const { data } = useResourceOptionsQuery('connector,js-plugin')
  const item = findResource(data?.data ?? [], name, repository)
  if (!item) return null
  return (
    <div className="grid gap-2 text-xs">
      <button
        type="button"
        className="text-button w-fit"
        onClick={() => setOpen(!open)}
      >
        {open ? '收起官方文档' : '官方文档'}
      </button>
      {open && <ResourceDocument id={item.id} />}
    </div>
  )
}

// One selector serves local logins and official install candidates.
export function ConnectionCatalog({
  root,
  busy,
  onInstall,
  onLogin,
  value,
  label = '登录连接',
  onClear,
  onCustom,
  onPending
}: {
  root: string
  busy?: boolean
  value?: string
  label?: string
  onInstall: (action: string, target: string) => Promise<boolean>
  onLogin: (login: string, pkg: string, platformValue?: string) => void
  onClear?: () => void
  onCustom?: () => void
  onPending?: () => void
}) {
  const [pending, setPending] = useState<MarketResource | null>(null)
  const [localValue, setLocalValue] = useState('')
  const [working, setWorking] = useState(false)
  const [message, setMessage] = useState('')
  const { data: catalog, error: catalogError } =
    useResourceOptionsQuery('connector')
  const { data: runtime, refetch } = useRobotRuntimeQuery(root, { skip: !root })
  const { data: inventory, refetch: refetchInventory } =
    usePackageInventoryQuery(root, { skip: !root })
  const platforms = (runtime?.platforms ?? []).filter(item => item.installed)
  const candidates = (catalog?.data ?? []).filter(
    resource =>
      !platforms.some(
        platform =>
          platform.package ===
          (factFor(resource, inventory?.items ?? [])?.name ??
            resource.packageName)
      )
  )
  const selectLogin = (
    platform: NonNullable<typeof runtime>['platforms'][number]
  ) => {
    setPending(null)
    setLocalValue(platform.id)
    onLogin(platform.id, platform.package, platform.platformValue)
  }
  return (
    <div className="grid gap-2">
      <label className="grid gap-1 text-xs font-semibold text-slate-600">
        {label}
        <select
          aria-label={label}
          disabled={busy || working}
          value={pending ? `resource:${pending.id}` : (value ?? localValue)}
          onChange={event => {
            const next = event.target.value
            setMessage('')
            setPending(null)
            setLocalValue(next)
            if (!next) {
              onClear?.()
              return
            }
            if (next === '__custom__') {
              onCustom?.()
              return
            }
            const platform = platforms.find(item => item.id === next)
            if (platform) {
              selectLogin(platform)
              return
            }
            const resource = candidates.find(
              item => `resource:${item.id}` === next
            )
            if (resource) {
              setPending(resource)
              onPending?.()
            }
          }}
        >
          <option value="">不选择</option>
          {onCustom && <option value="__custom__">自由输入</option>}
          {platforms.map(platform => (
            <option key={platform.id} value={platform.id}>
              {platform.label} · {platform.id}
            </option>
          ))}
          {candidates.map(resource => (
            <option
              key={resource.id}
              value={`resource:${resource.id}`}
              disabled={!resourceTarget(resource)}
            >
              {resource.name === resource.packageName
                ? resource.description || resource.name
                : resource.name}{' '}
              · 需安装
            </option>
          ))}
          {value &&
            value !== '__pending__' &&
            value !== '__custom__' &&
            !platforms.some(item => item.id === value) && (
              <option value={value}>{value}</option>
            )}
        </select>
      </label>
      {catalogError && (
        <small className="text-(--theme-text-secondary)">
          官方选项暂时无法读取，仍可选择本地连接。
        </small>
      )}
      {pending && (
        <button
          type="button"
          className="secondary-button w-fit"
          disabled={busy || working}
          onClick={() => {
            setWorking(true)
            setMessage('')
            void (async () => {
              try {
                if (
                  !(await onInstall(
                    'install-connection',
                    resourceTarget(pending)
                  ))
                ) {
                  setMessage('安装未完成，请查看操作记录。')
                  return
                }
                const facts = await refetchInventory().unwrap()
                const installed = factFor(pending, facts.items)
                const fresh = await refetch().unwrap()
                const choices = fresh.platforms.filter(
                  item =>
                    item.installed &&
                    item.package === (installed?.name ?? pending.packageName)
                )
                if (choices.length === 1) selectLogin(choices[0])
                else {
                  setPending(null)
                  setLocalValue('')
                  onClear?.()
                  setMessage(
                    choices.length
                      ? '请在选项中选择此连接包的登录入口。'
                      : '未读取到登录声明，可使用自由输入。'
                  )
                }
              } catch {
                setMessage('安装未完成，请查看操作记录。')
              } finally {
                setWorking(false)
              }
            })()
          }}
        >
          {working ? '安装中…' : '安装连接包'}
        </button>
      )}
      {message && (
        <p role="status" className="m-0 text-xs">
          {message}
        </p>
      )}
    </div>
  )
}
