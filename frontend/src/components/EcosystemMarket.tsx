import { factFor, resourceTarget } from '../lib/officialResources'
import { useState } from 'react'
import Markdown from 'markdown-to-jsx'
import { ArrowLeft, Package, Search } from 'lucide-react'
import { RobotPanel } from './RobotPanel'
import { ConfigFieldsEditor } from './PackageConfigFields'
import {
  useMarketResourcesQuery,
  useMarketResourceQuery,
  useCatalogVersionsQuery,
  usePackageConfigQuery,
  usePackageInventoryQuery,
  type MarketResource
} from '../store/workspaceApi'

function errorMessage(error: unknown) {
  const data = (error as { data?: { error?: string } })?.data
  return typeof data?.error === 'string'
    ? data.error
    : '资源暂时无法读取，请重试。'
}
const gitTarget = resourceTarget

export function EcosystemMarket({
  root,
  type,
  subtype,
  category,
  busy,
  onRun,
  onSaveConfig
}: {
  root: string
  type: string
  subtype?: string
  category: string
  busy: boolean
  onRun: (action: string, packageName: string) => Promise<boolean>
  onSaveConfig: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
}) {
  const [search, setSearch] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState('')
  const {
    currentData: list,
    isFetching,
    error,
    refetch
  } = useMarketResourcesQuery({ type, subtype, q: query, page })
  const { data: inventory } = usePackageInventoryQuery(root, { skip: !root })
  const installed = (item: MarketResource) =>
    factFor(item, inventory?.items ?? [])?.installed
  const title = type === 'connector' ? '连接' : '插件'
  if (selected)
    return (
      <ResourceDetail
        key={selected}
        id={selected}
        root={root}
        busy={busy}
        onBack={() => setSelected('')}
        onRun={onRun}
        onSaveConfig={onSaveConfig}
      />
    )
  return (
    <RobotPanel
      title={title}
      description={`${category} · ALemon 生态资源平台`}
      icon={<Package className="size-4" />}
      actions={
        <button className="secondary-button" onClick={() => void refetch()}>
          刷新
        </button>
      }
    >
      <form
        className="flex gap-2"
        onSubmit={event => {
          event.preventDefault()
          setPage(1)
          setQuery(search.trim())
        }}
      >
        <input
          className="min-w-0 flex-1 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) px-3 py-2 text-sm"
          aria-label="搜索资源"
          placeholder="搜索名称、简介或分类"
          maxLength={200}
          value={search}
          onChange={event => setSearch(event.target.value)}
        />
        <button className="secondary-button" type="submit">
          <Search className="size-4" />
          搜索
        </button>
      </form>
      {error ? (
        <p role="alert">{errorMessage(error)}</p>
      ) : isFetching ? (
        <p role="status">正在读取资源…</p>
      ) : (
        <>
          <section className="grid gap-2">
            {list?.data.map(item => (
              <button
                key={item.id}
                className="grid gap-1 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-3 text-left"
                onClick={() => setSelected(item.id)}
              >
                <strong className="text-sm">
                  {item.name}
                  {installed(item) && (
                    <small className="ml-2 text-emerald-600">已安装</small>
                  )}
                </strong>
                <small className="text-(--theme-text-muted)">
                  {item.description}
                </small>
                <small className="text-(--theme-text-muted)">
                  {item.installMode === 'npm' ? 'npm' : 'Git'}
                  {item.ownerLogin ? ` · ${item.ownerLogin}` : ''}
                </small>
              </button>
            ))}
          </section>
          {!list?.data.length && <p>暂无匹配资源。</p>}
        </>
      )}
      <footer className="flex items-center justify-between gap-2 text-xs">
        <button
          className="secondary-button"
          disabled={page <= 1 || isFetching}
          onClick={() => setPage(page - 1)}
        >
          上一页
        </button>
        <span>
          第 {page} 页 · 共 {list?.total ?? 0} 项
        </span>
        <button
          className="secondary-button"
          disabled={!list || page * list.pageSize >= list.total || isFetching}
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </footer>
    </RobotPanel>
  )
}

function ResourceDetail({
  id,
  root,
  busy,
  onBack,
  onRun,
  onSaveConfig
}: {
  id: string
  root: string
  busy: boolean
  onBack: () => void
  onRun: (action: string, packageName: string) => Promise<boolean>
  onSaveConfig: (
    packageName: string,
    values: Record<string, unknown>
  ) => Promise<boolean>
}) {
  const { data, isFetching, error, refetch } = useMarketResourceQuery(id)
  const item = error ? undefined : data?.data
  const npm = item?.installMode === 'npm'
  const target = item
    ? npm
      ? /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/.test(
          item.packageName ?? ''
        )
        ? item.packageName!
        : ''
      : !item.installMode || item.installMode === 'git'
        ? gitTarget(item)
        : ''
    : ''
  const [tab, setTab] = useState('overview')
  const [version, setVersion] = useState('')
  const [branch, setBranch] = useState('')
  const branchValid =
    !branch ||
    (/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(branch) && !branch.includes('..'))
  const {
    data: versions,
    isFetching: versionsLoading,
    error: versionsError
  } = useCatalogVersionsQuery(target, { skip: !npm || !target })
  const { data: inventory, refetch: refetchPackages } =
    usePackageInventoryQuery(root, { skip: !root })
  const local = item ? factFor(item, inventory?.items ?? []) : undefined
  const configPackage = npm ? item?.packageName : local?.name
  const {
    currentData: config,
    isFetching: configLoading,
    refetch: refetchConfig
  } = usePackageConfigQuery(
    { root, package: configPackage ?? '' },
    { skip: !configPackage }
  )
  const [values, setValues] = useState<Record<string, unknown> | null>(null)
  const [saveMessage, setSaveMessage] = useState('')
  const runInstall = async () => {
    const action = npm
      ? item?.type === 'connector'
        ? 'install-connection'
        : 'install-module'
      : item?.type === 'connector'
        ? 'install-connection'
        : 'install-package'
    if (
      await onRun(
        action,
        npm
          ? version
            ? `${target}@${version}`
            : target
          : branch
            ? `${target}#${branch}`
            : target
      )
    ) {
      void refetchPackages()
      if (configPackage) void refetchConfig()
    }
  }
  return (
    <RobotPanel
      title={item?.name ?? '资源详情'}
      description={item?.description}
      icon={<Package className="size-4" />}
      actions={
        <button className="text-button" onClick={onBack}>
          <ArrowLeft className="size-4" />
          返回目录
        </button>
      }
    >
      {isFetching && <p role="status">正在读取详情…</p>}
      {error && (
        <div role="alert">
          <p>{errorMessage(error)}</p>
          <button className="secondary-button" onClick={() => void refetch()}>
            重试
          </button>
        </div>
      )}
      {item && (
        <>
          <div className="flex gap-2" role="tablist" aria-label="资源详情">
            {[
              ['overview', '概览'],
              ['document', '文档'],
              ['config', '配置']
            ].map(([value, label]) => (
              <button
                className={
                  tab === value ? 'primary-button' : 'secondary-button'
                }
                key={value}
                role="tab"
                aria-selected={tab === value}
                onClick={() => setTab(value)}
              >
                {label}
              </button>
            ))}
          </div>
          {tab === 'overview' && (
            <section className="grid gap-4 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel) p-4 text-sm">
              <p>{item.description}</p>
              <span>安装方式：{npm ? 'npm' : 'Git'}</span>
              {npm && target && (
                <label className="grid gap-1">
                  安装版本
                  <select
                    aria-label="安装版本"
                    className="rounded border border-(--theme-border-default) bg-(--theme-surface-panel) p-2"
                    value={version}
                    onChange={event => setVersion(event.target.value)}
                  >
                    <option value="">默认版本</option>
                    {versions?.versions.map(value => (
                      <option key={value} value={value}>
                        {value}
                        {value === versions.latest ? ' · 最新' : ''}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {versionsLoading && <small>正在读取 npm 版本…</small>}
              {versionsError && (
                <small>版本列表读取失败，可重试或安装默认版本。</small>
              )}
              {!npm && (
                <label className="grid gap-1">
                  Git 分支（可选）
                  <input
                    aria-label="Git 分支"
                    className="rounded border border-(--theme-border-default) bg-(--theme-surface-panel) p-2"
                    placeholder="留空使用仓库默认分支"
                    value={branch}
                    onChange={event => setBranch(event.target.value.trim())}
                  />
                  <small>安装后可在背包管理分支与提交。</small>
                  {!branchValid && (
                    <small role="alert">请输入有效的 Git 分支。</small>
                  )}
                </label>
              )}
              {!target && (
                <p role="alert">资源缺少有效安装信息，暂时无法安装。</p>
              )}
              <button
                className="primary-button w-fit"
                disabled={busy || isFetching || !target || !branchValid}
                onClick={() => void runInstall()}
              >
                {busy ? '处理中…' : '安装'}
              </button>
              {npm && config && (
                <button
                  className="secondary-button w-fit"
                  disabled={busy}
                  onClick={() => {
                    void onRun(
                      item.type === 'connector'
                        ? 'uninstall-connection'
                        : 'uninstall-module',
                      target
                    ).then(success => {
                      if (success) {
                        setValues(null)
                        void refetchConfig()
                      }
                    })
                  }}
                >
                  卸载
                </button>
              )}
              {item.repositoryUrl && (
                <a href={item.repositoryUrl} target="_blank" rel="noreferrer">
                  查看仓库 ↗
                </a>
              )}
            </section>
          )}
          {tab === 'document' && (
            <article className="markdown-page">
              <Markdown
                options={{
                  disableParsingRawHTML: true,
                  overrides: {
                    a: { props: { target: '_blank', rel: 'noreferrer' } }
                  }
                }}
              >
                {item.markdown || '该资源暂未提供文档。'}
              </Markdown>
            </article>
          )}
          {tab === 'config' &&
            (configLoading ? (
              <p role="status">正在读取本地配置…</p>
            ) : config ? (
              <section className="grid gap-3">
                <ConfigFieldsEditor
                  fields={config.fields ?? []}
                  values={values ?? config.values}
                  onChange={(name, value) =>
                    setValues({ ...(values ?? config.values), [name]: value })
                  }
                />
                <button
                  className="primary-button w-fit"
                  disabled={busy || !values}
                  onClick={() => {
                    void onSaveConfig(
                      config.package,
                      values ?? config.values
                    ).then(success => {
                      setSaveMessage(
                        success ? '配置已保存。' : '配置保存失败。'
                      )
                      if (success) {
                        setValues(null)
                        void refetchConfig()
                      }
                    })
                  }}
                >
                  保存配置
                </button>
                <p role="status">{saveMessage}</p>
              </section>
            ) : (
              <p>安装后可在此读取本地配置；Git 插件也可在背包中管理配置。</p>
            ))}
        </>
      )}
    </RobotPanel>
  )
}
