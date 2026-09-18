import { useEffect, useRef } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useStoreState, type RootState } from '../../store/guideStore'
import { forgetSQLite, rememberSQLite, nameSQLite } from '../../store/dataStore'
import { ConnectionContext } from './ConnectionContext'
import {
  dataError,
  useDataSQLMutation,
  type SQLResult
} from '../../store/dataApi'
import { Button } from '../Button'
import { DataResults, dataInputClass } from './DataResults'
import {
  rowMutation,
  tableSelect,
  type SQLiteColumn
} from '../../lib/sqliteEditing'
import { SQLiteRowEditor } from './SQLiteRowEditor'
import { useDataConfirmation } from './useDataConfirmation'

const tablesSQL =
  "SELECT name, type, sql FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name"
const identifier = (name: string) => '"' + name.replace(/"/g, '""') + '"'
const literal = (name: string) => "'" + name.replace(/'/g, "''") + "'"

export function SQLiteBrowser({
  defaults,
  fixedPath
}: {
  defaults: string[]
  fixedPath?: string
}) {
  const dispatch = useDispatch()
  const { confirm, dialog } = useDataConfirmation('确认 SQLite 写入')
  const saved = useSelector(
    (state: RootState) => state.dataPreferences.sqlitePaths
  )
  const [path, setPath] = useStoreState(
    fixedPath ?? saved[0] ?? defaults[0] ?? ''
  )
  const [connected, setConnected] = useStoreState('')
  const names = useSelector(
    (state: RootState) => state.dataPreferences.sqliteNames
  )
  const [connectionName, setConnectionName] = useStoreState(names[path] ?? '')
  const [testMessage, setTestMessage] = useStoreState('')
  const [testedPath, setTestedPath] = useStoreState('')
  const [filter, setFilter] = useStoreState('')
  const [tables, setTables] = useStoreState<string[]>([])
  const [selected, setSelected] = useStoreState('')
  const [primaryKeys, setPrimaryKeys] = useStoreState<string[]>([])
  const [columns, setColumns] = useStoreState<SQLiteColumn[]>([])
  const [editingRow, setEditingRow] = useStoreState<unknown[] | null>(null)
  const [systemDatabase, setSystemDatabase] = useStoreState(false)
  const [browsing, setBrowsing] = useStoreState(false)
  const [offset, setOffset] = useStoreState(0)
  const [sql, setSQL] = useStoreState('SELECT sqlite_version() AS version;')
  const [write, setWrite] = useStoreState(false)
  const [result, setResult] = useStoreState<SQLResult | null>(null)
  const [error, setError] = useStoreState('')
  const [run, state] = useDataSQLMutation()
  const pending = useRef<ReturnType<typeof run> | null>(null)
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
      pending.current?.abort()
    }
  }, [])
  const [busy, setBusy] = useStoreState(false)
  const disconnect = () => {
    setConnected('')
    setTables([])
    setSelected('')
    setResult(null)
    setSystemDatabase(false)
    setWrite(false)
    setBrowsing(false)
    setEditingRow(null)
  }
  const choosePath = (next: string) => {
    disconnect()
    setPath(next)
    setConnectionName(names[next] ?? '')
    setTestedPath('')
    setTestMessage('')
    setError('')
  }
  const testConnection = async () => {
    setBusy(true)
    setTestMessage('')
    setTestedPath('')
    setError('')
    const request = run({ path, sql: 'SELECT sqlite_version() AS version;' })
    pending.current = request
    try {
      await request.unwrap()
      if (alive.current) {
        setTestedPath(path)
        setTestMessage('连接测试成功，未修改数据库。')
      }
    } catch (e) {
      if (alive.current) setError(dataError(e))
    } finally {
      pending.current = null
      state.reset()
      if (alive.current) setBusy(false)
    }
  }
  const query = async (
    statement: string,
    writing = false,
    target = connected
  ) => {
    setError('')
    setResult(null)
    const request = run({
      path: target,
      sql: statement,
      write: writing,
      confirmed: writing
    })
    pending.current = request
    try {
      return await request.unwrap()
    } finally {
      pending.current = null
      state.reset()
    }
  }
  const connect = async () => {
    setBusy(true)
    setConnected('')
    setTables([])
    setSelected('')
    setBrowsing(false)
    setWrite(false)
    setSystemDatabase(false)
    try {
      const response = await query(tablesSQL, false, path)
      if (!alive.current) return
      setTables(response.rows.map(row => String(row[0])))
      setConnected(path)
      setSystemDatabase(Boolean(response.systemDatabase))
      setResult(response)
      if (!fixedPath) dispatch(rememberSQLite(path))
    } catch (e) {
      if (alive.current) setError(dataError(e))
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const connectRef = useRef(connect)
  connectRef.current = connect
  useEffect(() => {
    let active = true
    queueMicrotask(() => {
      if (active && fixedPath) void connectRef.current()
    })
    return () => {
      active = false
    }
  }, [fixedPath])
  const browse = async (name: string, page = 0, schema = false) => {
    setBusy(true)
    setWrite(false)
    setSelected(name)
    setOffset(page)
    setBrowsing(false)
    setPrimaryKeys([])
    try {
      const metadata = await query(
        `SELECT name, pk, hidden FROM pragma_table_xinfo(${literal(name)}) ORDER BY cid`
      )
      if (!alive.current) return
      if (metadata.truncated)
        throw new Error(
          '表字段过多，无法安全生成编辑与分页操作，请使用 SQL 编辑器。'
        )
      const info = metadata.rows.map(row => ({
        name: String(row[0]),
        pk: Number(row[1]),
        hidden: Number(row[2])
      }))
      setColumns(info)
      setPrimaryKeys(info.filter(c => c.pk > 0).map(c => c.name))
      const statement = schema
        ? `SELECT * FROM pragma_table_xinfo(${literal(name)})`
        : tableSelect(name, info, page)
      setSQL(statement)
      const data = await query(statement)
      if (alive.current) {
        setResult(data)
        setBrowsing(!schema)
      }
    } catch (e) {
      if (alive.current) setError(e instanceof Error ? e.message : dataError(e))
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const execute = async () => {
    if (write && systemDatabase) {
      setError('系统运维数据库仅允许只读访问。')
      return
    }
    if (
      write &&
      !(await confirm(
        `即将在 ${connected} 执行写入或表结构变更，无法通过此界面撤销。已备份并确认继续？\n\n${sql.slice(0, 500)}`
      ))
    )
      return
    setBrowsing(false)
    setBusy(true)
    try {
      const data = await query(sql, write)
      if (alive.current) setResult(data)
    } catch (e) {
      if (alive.current) setError(dataError(e))
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const rowSQL = (row: unknown[], remove: boolean) => {
    if (!result) return
    if (!remove) {
      setEditingRow(row)
      return
    }
    try {
      setSQL(rowMutation(selected, result.columns, row, columns, null))
      setWrite(true)
      setError('')
    } catch (e) {
      setError(e instanceof Error ? e.message : '无法生成 SQL')
    }
  }
  return (
    <div className="grid min-w-0 gap-4">
      {dialog}
      <ConnectionContext
        engine="SQLite"
        target={connected || path || '尚未选择数据库'}
        state={busy ? '请求中' : connected ? '已连接' : '未连接'}
        access={
          systemDatabase
            ? '系统库 · 强制只读'
            : write
              ? '允许写入 · 执行需确认'
              : '只读模式'
        }
      />
      {editingRow && result && (
        <SQLiteRowEditor
          names={result.columns}
          row={editingRow}
          metadata={columns}
          onClose={() => setEditingRow(null)}
          onGenerate={changes => {
            setSQL(
              rowMutation(
                selected,
                result.columns,
                editingRow,
                columns,
                changes
              )
            )
            setWrite(true)
            setEditingRow(null)
            setError('')
          }}
        />
      )}
      {systemDatabase && (
        <p role="status" className="text-sm">
          系统运维数据库 · 强制只读，请通过对应业务功能修改数据。
        </p>
      )}
      {fixedPath ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button disabled={busy} onClick={() => void connect()}>
            刷新表
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-end gap-2">
          <label className="min-w-48 flex-1 text-xs">
            SQLite 文件（服务器绝对路径）
            <input
              className={dataInputClass}
              list="sqlite-connections"
              value={path}
              disabled={busy}
              onChange={e => choosePath(e.target.value)}
              placeholder="/path/to/database.sqlite 或 C:\\data\\app.db"
            />
          </label>
          <datalist id="sqlite-connections">
            {[...new Set([...defaults, ...saved])].map(p => (
              <option key={p} value={p} />
            ))}
          </datalist>
          <Button
            disabled={busy || !path.trim()}
            onClick={() => void testConnection()}
          >
            测试连接
          </Button>
          <Button
            disabled={busy || !path.trim()}
            onClick={() => void connect()}
          >
            连接 / 刷新表
          </Button>
          <Button
            disabled={!saved.includes(path) || busy}
            onClick={() => dispatch(forgetSQLite(path))}
          >
            忘记连接
          </Button>
        </div>
      )}
      {!fixedPath && (
        <div className="grid min-w-0 gap-2 rounded-md border border-(--theme-border-default) p-3">
          <div className="flex flex-wrap items-end gap-2">
            <label className="min-w-0 flex-1 text-xs">
              连接名称
              <input
                className={dataInputClass}
                maxLength={80}
                value={connectionName}
                disabled={busy}
                onChange={e => setConnectionName(e.target.value)}
                placeholder="例如：机器人业务库"
              />
            </label>
            <Button
              disabled={
                busy ||
                !connectionName.trim() ||
                (testedPath !== path && connected !== path)
              }
              onClick={() => {
                dispatch(rememberSQLite(path))
                dispatch(nameSQLite({ path, name: connectionName }))
                setTestMessage('连接已保存，仅保存名称和路径。')
              }}
            >
              保存连接
            </Button>
            <Button disabled={busy} onClick={() => choosePath('')}>
              新建连接
            </Button>
            <Button disabled={busy || !connected} onClick={disconnect}>
              断开连接
            </Button>
          </div>
          {testMessage && (
            <p className="text-xs" role="status">
              {testMessage}
            </p>
          )}
          <nav aria-label="已保存的连接" className="flex flex-wrap gap-2">
            {saved.map(p => (
              <Button
                key={p}
                className="min-w-0 max-w-full"
                title={p}
                disabled={busy}
                aria-pressed={path === p}
                onClick={() => choosePath(p)}
              >
                <span className="truncate">{names[p] || p}</span>
              </Button>
            ))}
          </nav>
          {!saved.length && (
            <p className="text-xs">
              尚无已保存连接。填写服务器文件路径，测试成功后命名保存。
            </p>
          )}
        </div>
      )}
      {!fixedPath && (
        <p className="text-xs text-(--theme-text-secondary)">
          连接服务器上的已有 SQLite
          文件，可选系统运维库（只读）。仅记录连接名称和路径，不创建文件。忘记连接不会删除数据库文件。
        </p>
      )}
      <div className="grid min-w-0 gap-4 sm:grid-cols-[12rem_minmax(0,1fr)]">
        <aside
          className="max-h-96 overflow-auto rounded-md border border-(--theme-border-default) p-2"
          aria-label="数据库表"
        >
          <h3 className="mb-2 text-sm font-medium">
            表与视图（{tables.length}）
          </h3>
          <label className="text-xs">
            搜索表或视图
            <input
              className={dataInputClass}
              value={filter}
              onChange={e => setFilter(e.target.value)}
            />
          </label>
          {tables
            .filter(name => name.toLowerCase().includes(filter.toLowerCase()))
            .map(name => (
              <Button
                key={name}
                className="mb-1 w-full justify-start truncate"
                title={name}
                aria-pressed={selected === name}
                disabled={busy}
                onClick={() => void browse(name)}
              >
                {name}
              </Button>
            ))}
          {tables.length > 0 &&
            !tables.some(name =>
              name.toLowerCase().includes(filter.toLowerCase())
            ) && <p className="text-xs">没有匹配的表或视图</p>}
          {!tables.length && (
            <p className="text-xs">
              {connected ? '数据库没有表或视图' : '请先连接数据库'}
            </p>
          )}
        </aside>
        <div className="grid min-w-0 gap-3">
          {selected && (
            <div className="flex flex-wrap gap-2">
              <Button disabled={busy} onClick={() => void browse(selected)}>
                浏览数据
              </Button>
              <Button
                disabled={busy}
                onClick={() => void browse(selected, 0, true)}
              >
                表结构
              </Button>
              <Button
                disabled={busy || systemDatabase}
                onClick={() => {
                  setWrite(true)
                  setSQL(
                    `INSERT INTO ${identifier(selected)} ("列名") VALUES ('值');`
                  )
                }}
              >
                新增 SQL
              </Button>
              <Button
                disabled={busy || systemDatabase}
                onClick={() => {
                  setWrite(true)
                  setSQL(
                    `UPDATE ${identifier(selected)} SET "列名" = '值' WHERE "主键" = '目标值';`
                  )
                }}
              >
                编辑 SQL
              </Button>
              <Button
                disabled={busy || systemDatabase}
                onClick={() => {
                  setWrite(true)
                  setSQL(
                    `DELETE FROM ${identifier(selected)} WHERE "主键" = '目标值';`
                  )
                }}
              >
                删除 SQL
              </Button>
            </div>
          )}
          <label className="text-xs">
            SQL 编辑器
            <textarea
              className={`${dataInputClass} mt-1 min-h-32 font-mono`}
              value={sql}
              disabled={busy}
              onChange={e => setSQL(e.target.value)}
              spellCheck={false}
            />
          </label>
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-xs">
              <input
                type="checkbox"
                checked={write}
                disabled={busy || systemDatabase}
                onChange={e => setWrite(e.target.checked)}
              />
              允许写入（执行时需确认）
            </label>
            <Button
              variant={write ? 'danger' : 'primary'}
              disabled={!connected || busy || !sql.trim()}
              onClick={() => void execute()}
            >
              {busy ? '执行中…' : write ? '执行写入' : '执行查询'}
            </Button>
            {busy && (
              <Button onClick={() => pending.current?.abort()}>取消请求</Button>
            )}
          </div>
          <p className="text-xs text-(--theme-text-secondary)">
            每次执行一条语句，8
            秒超时。取消写入请求不保证撤销已提交的变更。大整数建议 CAST 为
            TEXT；BLOB 以 base64 显示。
          </p>
          {error && (
            <p
              role="alert"
              className="whitespace-pre-wrap text-sm text-red-600 dark:text-red-400"
            >
              {error}
            </p>
          )}
          {result && (
            <DataResults
              result={result}
              onEdit={
                browsing && primaryKeys.length && !busy && !systemDatabase
                  ? row => rowSQL(row, false)
                  : undefined
              }
              onDelete={row => rowSQL(row, true)}
            />
          )}
          {browsing && !primaryKeys.length && (
            <p className="text-xs">
              此表没有声明主键或是视图，请通过 SQL 编辑器指定唯一条件后修改。
            </p>
          )}
          {selected && browsing && (
            <div className="flex items-center gap-2 text-xs">
              <Button
                disabled={busy || offset === 0}
                onClick={() => void browse(selected, Math.max(0, offset - 100))}
              >
                上一页
              </Button>
              <span>偏移 {offset}，每页 100 行；并发变更可能导致记录移动</span>
              <Button
                disabled={busy || !result || result.rows.length < 100}
                onClick={() => void browse(selected, offset + 100)}
              >
                下一页
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
