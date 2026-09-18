import { useEffect, useRef } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useStoreState, type RootState } from '../../store/guideStore'
import { saveConnection, forgetConnection } from '../../store/dataStore'
import {
  newConnection,
  connectionProfile,
  type SQLConnection,
  type ConnectionProfile
} from '../../lib/connectModels'
import {
  useConnectSQLMutation,
  dataError,
  type ConnectOperation,
  type SQLResult
} from '../../store/dataApi'
import { Button } from '../Button'
import { ConfirmDialog } from '../ConfirmDialog'
import { ConnectionContext } from './ConnectionContext'
import { SQLConnectionForm } from './SQLConnectionForm'
import { DataResults, dataInputClass } from './DataResults'
import { DataDialog } from './DataDialog'
import { useDataConfirmation } from './useDataConfirmation'

type QueryTab = { id: number; sql: string }
type Navigation =
  | { kind: 'profile'; profile?: ConnectionProfile }
  | { kind: 'connect' | 'disconnect' }
  | { kind: 'database' | 'schema'; value: string }
export function ConnectWorkspace({ suggestions }: { suggestions: string[] }) {
  const profiles = useSelector((s: RootState) => s.dataPreferences.connections)
  const dispatch = useDispatch()
  const [connectionDialog, setConnectionDialog] = useStoreState(false)
  const { confirm: confirmForget, dialog: forgetDialog } =
    useDataConfirmation('忘记数据库连接')
  const [draft, setDraft] = useStoreState(newConnection())
  const [name, setName] = useStoreState('')
  const [profileId, setProfileId] = useStoreState('')
  const [active, setActive] = useStoreState<SQLConnection | null>(null)
  const [tested, setTested] = useStoreState(false)
  const [busy, setBusy] = useStoreState(false)
  const [error, setError] = useStoreState('')
  const [message, setMessage] = useStoreState('')
  const [databases, setDatabases] = useStoreState<string[]>([])
  const [schemas, setSchemas] = useStoreState<string[]>([])
  const [schema, setSchema] = useStoreState('')
  const [objects, setObjects] = useStoreState<
    Array<{ name: string; type: string }>
  >([])
  const [object, setObject] = useStoreState('')
  const [filter, setFilter] = useStoreState('')
  const [offset, setOffset] = useStoreState(0)
  const [browsing, setBrowsing] = useStoreState(false)
  const [result, setResult] = useStoreState<SQLResult | null>(null)
  const [systemDatabase, setSystemDatabase] = useStoreState(false)
  const [write, setWrite] = useStoreState(false)
  const [tabs, setTabs] = useStoreState<QueryTab[]>([
    { id: 1, sql: 'SELECT 1;' }
  ])
  const [tab, setTab] = useStoreState(1)
  const [confirmSQL, setConfirmSQL] = useStoreState<string | null>(null)
  const [dirty, setDirty] = useStoreState(false)
  const [navigation, setNavigation] = useStoreState<Navigation | null>(null)
  const [run, mutation] = useConnectSQLMutation()
  const pending = useRef<ReturnType<typeof run> | null>(null)
  const alive = useRef(true)
  const editor = useRef<HTMLTextAreaElement>(null)
  const sql = tabs.find(t => t.id === tab)?.sql ?? ''
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
      pending.current?.abort()
    }
  }, [])
  const send = async (input: ConnectOperation) => {
    const request = run(input)
    pending.current = request
    try {
      const response = await request.unwrap()
      if (!alive.current) throw new Error('窗口已关闭')
      return response
    } finally {
      pending.current = null
      mutation.reset()
    }
  }
  const task = async (operation: () => Promise<void>) => {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      await operation()
    } catch (e) {
      if (alive.current) setError(dataError(e))
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const disconnect = () => {
    setActive(null)
    setDatabases([])
    setSchemas([])
    setObjects([])
    setSchema('')
    setObject('')
    setResult(null)
    setSystemDatabase(false)
    setWrite(false)
    setBrowsing(false)
  }
  const selectProfile = (profile?: ConnectionProfile) => {
    setConnectionDialog(true)
    setDraft(profile ? { ...profile, password: '' } : newConnection())
    setProfileId(profile?.id ?? '')
    setName(profile?.name ?? '')
    setTested(false)
    setError('')
    setMessage('')
  }
  const listObjects = async (connection: SQLConnection, scope: string) => {
    const data = await send({ connection, action: 'tables', schema: scope })
    setSchema(scope)
    setObjects(
      data.rows.map(row => ({ name: String(row[0]), type: String(row[1]) }))
    )
    setObject('')
    setResult(null)
    setBrowsing(false)
    setWrite(false)
    setSystemDatabase(Boolean(data.systemDatabase))
    if (data.truncated)
      setMessage(
        '对象列表已截断，当前最多显示 200 个对象；可通过查询访问其他对象。'
      )
  }
  const openDatabase = async (connection: SQLConnection) => {
    setObjects([])
    setObject('')
    setResult(null)
    setSchema('')
    setSchemas([])
    setWrite(false)
    const data = await send({ connection, action: 'schemas' })
    const scopes = data.rows.map(row => String(row[0]))
    setSchemas(scopes)
    setActive(connection)
    const scope =
      connection.engine === 'sqlite'
        ? 'main'
        : connection.engine === 'postgres'
          ? scopes.includes('public')
            ? 'public'
            : scopes[0]
          : connection.database || scopes[0]
    if (scope) await listObjects(connection, scope)
  }
  const connect = () =>
    task(async () => {
      disconnect()
      const data = await send({ connection: draft, action: 'databases' })
      const available = data.rows.map(row => String(row[0]))
      setDatabases(available)
      await openDatabase({
        ...draft,
        database:
          draft.database ||
          (draft.engine === 'postgres'
            ? 'postgres'
            : draft.engine === 'sqlite'
              ? ''
              : available.find(
                  d =>
                    ![
                      'information_schema',
                      'mysql',
                      'performance_schema',
                      'sys'
                    ].includes(d)
                ) || '')
      })
      setTested(true)
      setConnectionDialog(false)
    })
  const navigate = (next: Navigation) => {
    if (next.kind === 'profile') {
      selectProfile(next.profile)
      return
    }
    setTabs([{ id: 1, sql: 'SELECT 1;' }])
    setTab(1)
    setDirty(false)
    setWrite(false)
    if (next.kind === 'disconnect') disconnect()
    else if (next.kind === 'connect') void connect()
    else if (active && next.kind === 'database') {
      const connection = { ...active, database: next.value }
      setActive(null)
      void task(() => openDatabase(connection))
    } else if (active && next.kind === 'schema')
      void task(() => listObjects(active, next.value))
  }
  const requestNavigation = (next: Navigation) => {
    if (dirty && next.kind !== 'profile') setNavigation(next)
    else navigate(next)
  }
  const inspect = (
    table: string,
    action: 'browse' | 'structure' | 'indexes',
    page = 0
  ) => {
    if (!active) return
    void task(async () => {
      const data = await send({
        connection: active,
        action,
        schema,
        table,
        offset: page
      })
      setObject(table)
      setResult(data)
      setOffset(page)
      setBrowsing(action === 'browse')
      setWrite(false)
    })
  }
  const execute = (statement: string, writing: boolean) => {
    if (!active) return
    void task(async () => {
      setResult(null)
      setBrowsing(false)
      setResult(
        await send({
          connection: active,
          action: 'query',
          schema,
          sql: statement,
          write: writing,
          confirmed: writing
        })
      )
    })
  }
  const requestExecution = (selection: boolean) => {
    const statement =
      selection && editor.current
        ? sql.slice(editor.current.selectionStart, editor.current.selectionEnd)
        : sql
    if (!statement.trim()) {
      setError('请先输入或选择要执行的 SQL')
      return
    }
    if (write) setConfirmSQL(statement)
    else execute(statement, false)
  }
  const target = active
    ? active.engine === 'sqlite'
      ? active.path
      : `${active.host}:${active.port} / ${active.database || schema || '未选择数据库'}${active.engine === 'postgres' && schema ? ` / ${schema}` : ''}`
    : '尚未连接'
  return (
    <div className="grid min-w-0 gap-4">
      <div className="grid min-w-0 gap-3">
        <aside
          aria-label="数据库连接"
          className="flex min-w-0 flex-wrap items-center gap-2 rounded-md border border-(--theme-border-default) p-2"
        >
          <Button
            disabled={busy}
            onClick={() => requestNavigation({ kind: 'profile' })}
          >
            新建连接
          </Button>
          {profiles.map(p => (
            <Button
              key={p.id}
              className="min-w-0 max-w-full justify-start"
              disabled={busy}
              title={`${p.name} · ${p.engine}`}
              aria-pressed={profileId === p.id}
              onClick={() => requestNavigation({ kind: 'profile', profile: p })}
            >
              <span className="truncate">
                {p.name} · {p.engine}
              </span>
            </Button>
          ))}
          {!profiles.length && (
            <p className="text-xs">尚无保存的连接，选择数据库类型开始。</p>
          )}
        </aside>
        <div className="flex flex-wrap gap-2">
          <Button
            disabled={busy}
            onClick={() => {
              setError('')
              setMessage('')
              setConnectionDialog(true)
            }}
          >
            配置连接
          </Button>
          <Button
            disabled={busy || !active}
            onClick={() => requestNavigation({ kind: 'disconnect' })}
          >
            断开连接
          </Button>
          <Button
            disabled={busy || !profileId}
            onClick={async () => {
              if (
                await confirmForget(
                  `仅移除“${name}”的连接记录，不会删除数据库或数据。是否继续？`
                )
              ) {
                dispatch(forgetConnection(profileId))
                setProfileId('')
                setMessage('已忘记连接，没有删除数据库。')
              }
            }}
          >
            忘记连接
          </Button>
        </div>
        <DataDialog
          open={connectionDialog}
          title="配置 SQL 连接"
          description="选择数据库类型，测试连接后保存或直接打开。密码仅用于本次窗口。"
          busy={busy}
          onClose={() => setConnectionDialog(false)}
          footer={
            <Button disabled={busy} onClick={() => setConnectionDialog(false)}>
              取消
            </Button>
          }
        >
          <SQLConnectionForm
            value={draft}
            disabled={busy}
            suggestions={suggestions}
            onChange={next => {
              setDraft(next)
              setTested(false)
            }}
          />
          <label className="text-xs">
            连接名称
            <input
              className={dataInputClass}
              disabled={busy}
              maxLength={80}
              value={name}
              onChange={e => setName(e.target.value)}
            />
          </label>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={busy}
              onClick={() =>
                void task(async () => {
                  setTested(false)
                  await send({ connection: draft, action: 'test' })
                  setTested(true)
                  setMessage('连接测试成功，未修改数据库。')
                })
              }
            >
              测试连接
            </Button>
            <Button
              disabled={busy}
              variant="primary"
              onClick={() => requestNavigation({ kind: 'connect' })}
            >
              连接
            </Button>
            <Button
              disabled={busy || !tested || !name.trim()}
              onClick={() => {
                const id = profileId || crypto.randomUUID()
                dispatch(
                  saveConnection(connectionProfile(id, name.trim(), draft))
                )
                setProfileId(id)
                setMessage('连接已保存，不保存密码。')
                setConnectionDialog(false)
              }}
            >
              保存连接
            </Button>
          </div>
          {message && (
            <p role="status" className="text-xs">
              {message}
            </p>
          )}
          {error && (
            <p role="alert" className="text-sm text-(--theme-danger)">
              {error}
            </p>
          )}
        </DataDialog>
      </div>
      <ConnectionContext
        engine={active?.engine ?? draft.engine}
        target={target}
        state={
          busy ? '请求中' : active ? '已连接 · 每次请求独立连接' : '未连接'
        }
        access={
          systemDatabase
            ? '系统库 · 强制只读'
            : write
              ? '允许写入 · 需确认'
              : '只读模式'
        }
      />
      {active && (
        <div className="flex flex-wrap gap-3">
          {active.engine !== 'sqlite' && (
            <label className="text-xs">
              数据库
              <select
                className={dataInputClass}
                disabled={busy}
                value={active.database}
                aria-label="数据库"
                onChange={e =>
                  requestNavigation({ kind: 'database', value: e.target.value })
                }
              >
                <option value="">请选择数据库</option>
                {databases.map(d => (
                  <option key={d}>{d}</option>
                ))}
              </select>
            </label>
          )}
          {(active.engine === 'postgres' || active.engine === 'sqlite') && (
            <label className="text-xs">
              Schema / 对象分组
              <select
                className={dataInputClass}
                disabled={busy}
                value={schema}
                aria-label="Schema / 对象分组"
                onChange={e =>
                  requestNavigation({ kind: 'schema', value: e.target.value })
                }
              >
                {schemas.map(s => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </label>
          )}
          <Button
            disabled={busy || !schema}
            onClick={() => void task(() => listObjects(active, schema))}
          >
            刷新对象
          </Button>
        </div>
      )}
      {message && !connectionDialog && (
        <p role="status" className="text-xs">
          {message}
        </p>
      )}
      {error && !connectionDialog && (
        <p role="alert" className="text-sm text-(--theme-danger)">
          {error}
        </p>
      )}
      <div className="grid min-w-0 gap-4 sm:grid-cols-[12rem_minmax(0,1fr)]">
        <aside
          aria-label="数据库对象"
          className="max-h-96 min-w-0 overflow-auto rounded-md border border-(--theme-border-default) p-2"
        >
          <label className="text-xs">
            搜索表或视图
            <input
              className={dataInputClass}
              value={filter}
              onChange={e => setFilter(e.target.value)}
            />
          </label>
          {objects
            .filter(o => o.name.toLowerCase().includes(filter.toLowerCase()))
            .map(o => (
              <Button
                key={o.name}
                title={`${o.name} · ${o.type}`}
                className="mt-1 w-full min-w-0 justify-start"
                aria-pressed={object === o.name}
                disabled={busy}
                onClick={() => inspect(o.name, 'browse')}
              >
                <span className="truncate">
                  {o.name} · {o.type}
                </span>
              </Button>
            ))}
          {!objects.length && (
            <p className="mt-2 text-xs">
              {active ? '当前分组没有可见表或视图' : '连接后浏览数据库对象'}
            </p>
          )}
        </aside>
        <div className="grid min-w-0 content-start gap-3">
          {object && (
            <div className="flex flex-wrap items-center gap-2">
              <strong className="break-all text-xs">
                {schema}.{object}
              </strong>
              <Button disabled={busy} onClick={() => inspect(object, 'browse')}>
                浏览数据
              </Button>
              <Button
                disabled={busy}
                onClick={() => inspect(object, 'structure')}
              >
                表结构
              </Button>
              <Button
                disabled={busy}
                onClick={() => inspect(object, 'indexes')}
              >
                索引
              </Button>
            </div>
          )}
          <div
            role="group"
            aria-label="查询页签"
            className="flex flex-wrap gap-2"
          >
            {tabs.map(t => (
              <Button
                key={t.id}
                disabled={busy}
                aria-pressed={t.id === tab}
                onClick={() => {
                  setTab(t.id)
                  setWrite(false)
                  setResult(null)
                  setBrowsing(false)
                }}
              >
                查询 {t.id}
              </Button>
            ))}
            <Button
              disabled={busy || tabs.length >= 8}
              onClick={() => {
                const id = Math.max(...tabs.map(t => t.id)) + 1
                setTabs([...tabs, { id, sql: '' }])
                setTab(id)
                setWrite(false)
                setResult(null)
                setBrowsing(false)
              }}
            >
              新建查询
            </Button>
          </div>
          <label className="text-xs">
            SQL 编辑器
            <textarea
              ref={editor}
              className={`${dataInputClass} min-h-32 font-mono`}
              value={sql}
              disabled={busy}
              spellCheck={false}
              onChange={e => {
                setDirty(true)
                setTabs(
                  tabs.map(t =>
                    t.id === tab ? { ...t, sql: e.target.value } : t
                  )
                )
              }}
            />
          </label>
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-2 text-xs">
              <input
                type="checkbox"
                checked={write}
                disabled={busy || !active || systemDatabase}
                onChange={e => setWrite(e.target.checked)}
              />
              允许写入（执行时需确认）
            </label>
            <Button
              disabled={busy || !active || !sql.trim()}
              variant={write ? 'danger' : 'primary'}
              onClick={() => requestExecution(false)}
            >
              {write ? '执行写入' : '执行查询'}
            </Button>
            <Button
              disabled={busy || !active}
              onClick={() => requestExecution(true)}
            >
              执行选中 SQL
            </Button>
            {busy && (
              <Button onClick={() => pending.current?.abort()}>取消请求</Button>
            )}
          </div>
          <p className="text-xs text-(--theme-text-secondary)">
            每次一条 SQL，8
            秒超时；不支持手动事务、批量脚本和存储过程。写入立即提交，取消不保证撤销。请使用最小权限数据库账号。查询草稿不持久化。
          </p>
          {result && <DataResults result={result} />}
          {browsing && (
            <div className="flex flex-wrap items-center gap-2 text-xs">
              <Button
                disabled={busy || offset === 0}
                onClick={() =>
                  inspect(object, 'browse', Math.max(0, offset - 100))
                }
              >
                上一页
              </Button>
              <span>
                偏移 {offset}，每页 100
                行；无显式排序，并发变更可能导致重复或遗漏。
              </span>
              <Button
                disabled={busy || !result || result.rows.length < 100}
                onClick={() => inspect(object, 'browse', offset + 100)}
              >
                下一页
              </Button>
            </div>
          )}
        </div>
      </div>
      <ConfirmDialog
        open={navigation !== null}
        title="切换查询上下文"
        message="切换连接、数据库或分组会清除当前查询草稿与结果，避免旧 SQL 在新目标误执行。是否放弃草稿并继续？"
        onCancel={() => setNavigation(null)}
        onConfirm={() => {
          if (navigation) navigate(navigation)
          setNavigation(null)
        }}
      />
      {forgetDialog}
      <ConfirmDialog
        open={confirmSQL !== null}
        title="确认 SQL 写入"
        message={`目标：${target}\n\n${confirmSQL?.slice(0, 500) ?? ''}\n\n数据或结构变更不可在此撤销，请确认已备份。`}
        destructive
        onCancel={() => setConfirmSQL(null)}
        onConfirm={() => {
          if (confirmSQL) execute(confirmSQL, true)
          setConfirmSQL(null)
        }}
      />
    </div>
  )
}
