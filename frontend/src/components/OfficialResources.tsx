import {
  resourceTarget,
  findResource,
  factFor,
  packageState
} from '../lib/officialResources'
import { useState } from 'react'
import Markdown from 'markdown-to-jsx'
import {
  useResourceOptionsQuery,
  useMarketResourceQuery,
  useRobotRuntimeQuery,
  usePackageInventoryQuery,
  type MarketResource
} from '../store/workspaceApi'

export function OfficialResourcePicker({
  type = 'connector,js-plugin',
  label = '官方资源',
  onSelect,
  selected = [],
  multiple = false,
  root,
  npmOnly = false
}: {
  type?: string
  label?: string
  onSelect: (resource: MarketResource) => void
  selected?: string[]
  multiple?: boolean
  root?: string
  npmOnly?: boolean
}) {
  const [query, setQuery] = useState('')
  const { data, isFetching, error, refetch } = useResourceOptionsQuery(type)
  const { data: inventory, error: inventoryError } = usePackageInventoryQuery(
    root ?? '',
    {
      skip: !root
    }
  )
  const items = (data?.data ?? []).filter(
    item =>
      (!npmOnly || item.installMode === 'npm') &&
      `${item.name} ${item.description} ${item.packageName ?? ''}`
        .toLowerCase()
        .includes(query.toLowerCase())
  )
  return (
    <section className="grid gap-2" aria-label={label}>
      <strong className="text-xs">{label}</strong>
      <input
        aria-label={`搜索${label}`}
        className="min-h-9 w-full rounded border border-(--theme-border-default) bg-(--theme-surface-panel) px-2 text-sm"
        placeholder="搜索名称、包名或简介"
        maxLength={200}
        value={query}
        onChange={event => setQuery(event.target.value)}
      />
      {error ? (
        <div role="alert" className="text-xs">
          官方目录暂不可用，本地功能仍可使用。
          <button className="text-button" onClick={() => void refetch()}>
            重试
          </button>
        </div>
      ) : isFetching ? (
        <p role="status" className="text-xs">
          正在读取官方目录…
        </p>
      ) : (
        <div className="grid max-h-56 gap-1 overflow-y-auto">
          {items.map(item => (
            <button
              type="button"
              key={item.id}
              aria-pressed={multiple ? selected.includes(item.id) : undefined}
              disabled={!resourceTarget(item)}
              className={`grid gap-1 rounded border border-(--theme-border-default) p-2 text-left text-xs ${selected.includes(item.id) ? 'bg-(--theme-accent-soft)' : ''}`}
              onClick={() => onSelect(item)}
            >
              <strong>
                {multiple ? (selected.includes(item.id) ? '☑ ' : '☐ ') : ''}
                {item.name}
              </strong>
              <span>{item.description}</span>
              <small>
                {item.installMode === 'npm' ? item.packageName : 'Git'}
                {root
                  ? ` · ${inventoryError || !inventory ? '本地状态未读取' : packageState(factFor(item, inventory.items))}`
                  : ''}
              </small>
            </button>
          ))}
          {!items.length && <p className="text-xs">暂无匹配资源。</p>}
        </div>
      )}
    </section>
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
    <section className="grid gap-2 rounded border border-(--theme-border-default) p-3 text-xs">
      <strong>{item.name}</strong>
      <p>{item.description}</p>
      <button
        type="button"
        className="text-button w-fit"
        onClick={() => setOpen(!open)}
      >
        {open ? '收起平台文档' : '查看平台文档'}
      </button>
      {open && <ResourceDocument id={item.id} />}
    </section>
  )
}

export function ConnectionCatalog({
  root,
  busy,
  onInstall,
  onLogin
}: {
  root: string
  busy?: boolean
  onInstall: (action: string, target: string) => Promise<boolean>
  onLogin: (login: string, pkg: string, platformValue?: string) => void
}) {
  const [selected, setSelected] = useState<MarketResource | null>(null)
  const [working, setWorking] = useState(false)
  const [message, setMessage] = useState('')
  const { refetch } = useRobotRuntimeQuery(root, { skip: !root })
  const { refetch: refetchInventory } = usePackageInventoryQuery(root, {
    skip: !root
  })
  return (
    <div className="grid gap-3">
      <details>
        <summary className="cursor-pointer text-xs font-semibold">
          从官方目录选择连接包
        </summary>
        <div className="mt-3">
          <OfficialResourcePicker
            type="connector"
            root={root}
            label="官方连接包"
            onSelect={item => {
              setSelected(item)
              setMessage('')
            }}
          />
        </div>
      </details>
      {selected && (
        <div className="grid gap-2 rounded border border-(--theme-border-default) p-3 text-xs">
          <strong>{selected.name}</strong>
          <span>{selected.description}</span>
          <button
            type="button"
            className="primary-button w-fit"
            disabled={busy || working || !resourceTarget(selected)}
            onClick={() => {
              setWorking(true)
              setMessage('')
              void (async () => {
                try {
                  const inventory = await refetchInventory().unwrap()
                  const existing = factFor(selected, inventory.items)
                  const before = await refetch().unwrap()
                  let choices = before.platforms.filter(
                    platform =>
                      platform.package ===
                        (existing?.name ?? selected.packageName) &&
                      platform.installed
                  )
                  if (!existing?.installed) {
                    if (
                      !(await onInstall(
                        'install-connection',
                        resourceTarget(selected)
                      ))
                    ) {
                      setMessage('安装未完成，请查看操作记录。')
                      return
                    }
                    const refreshed = await refetchInventory().unwrap()
                    const installed = factFor(selected, refreshed.items)
                    const overview = await refetch().unwrap()
                    choices = overview.platforms.filter(
                      platform =>
                        platform.package ===
                          (installed?.name ?? selected.packageName) &&
                        platform.installed
                    )
                  }
                  if (choices.length === 1) {
                    onLogin(
                      choices[0].id,
                      choices[0].package,
                      choices[0].platformValue
                    )
                    setMessage('已读取本地登录声明，请填写配置。')
                  } else
                    setMessage(
                      choices.length
                        ? '连接包包含多个登录入口，请从已安装连接中选择。'
                        : '连接包已安装，但尚未读取到登录声明；可刷新或手动填写。'
                    )
                } catch {
                  setMessage('连接操作未完成，请检查本地包与操作记录。')
                } finally {
                  setWorking(false)
                }
              })()
            }}
          >
            {working ? '处理中…' : '安装／使用此连接'}
          </button>
          <details>
            <summary className="cursor-pointer">连接文档</summary>
            <ResourceDocument id={selected.id} />
          </details>
          <p role="status">{message}</p>
        </div>
      )}
    </div>
  )
}

export function ModuleCatalog({
  root,
  onInstall,
  onEnable
}: {
  root: string
  onInstall: (action: string, target: string) => Promise<boolean>
  onEnable: (name: string) => void
}) {
  const [selected, setSelected] = useState<MarketResource | null>(null)
  const [working, setWorking] = useState(false)
  const [message, setMessage] = useState('')
  const { refetch } = usePackageInventoryQuery(root)
  return (
    <div className="grid gap-2">
      <details>
        <summary className="cursor-pointer text-xs font-semibold">
          从官方目录添加模块
        </summary>
        <div className="mt-3">
          <OfficialResourcePicker
            root={root}
            type="js-plugin"
            label="官方插件与模块"
            onSelect={resource => {
              setSelected(resource)
              setMessage('')
            }}
          />
        </div>
      </details>
      {selected && (
        <section className="grid gap-2 rounded border border-(--theme-border-default) p-3 text-xs">
          <strong>{selected.name}</strong>
          <button
            type="button"
            className="primary-button w-fit"
            disabled={working || !resourceTarget(selected)}
            onClick={() => {
              setWorking(true)
              setMessage('')
              void (async () => {
                try {
                  let fact = factFor(selected, (await refetch().unwrap()).items)
                  if (!fact?.installed) {
                    if (
                      !(await onInstall(
                        selected.installMode === 'npm'
                          ? 'install-module'
                          : 'install-package',
                        resourceTarget(selected)
                      ))
                    ) {
                      setMessage('安装未完成，请查看操作记录。')
                      return
                    }
                    fact = factFor(selected, (await refetch().unwrap()).items)
                  }
                  if (fact?.installed && fact.loadable) {
                    onEnable(fact.name)
                    setMessage('已加入启用列表，请保存配置。')
                  } else
                    setMessage(
                      '未读取到可加载的本地模块声明，请检查安装结果或编辑自定义启用项。'
                    )
                } catch {
                  setMessage('模块操作未完成，请检查操作记录。')
                } finally {
                  setWorking(false)
                }
              })()
            }}
          >
            {working ? '处理中…' : '安装／启用此模块'}
          </button>
          <details>
            <summary className="cursor-pointer">模块文档</summary>
            <ResourceDocument id={selected.id} />
          </details>
          <p role="status">{message}</p>
        </section>
      )}
    </div>
  )
}
