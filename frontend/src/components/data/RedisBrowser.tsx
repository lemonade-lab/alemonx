import { useEffect, useRef } from 'react'
import { useStoreState } from '../../store/guideStore'
import {
  dataError,
  useDataRedisMutation,
  type RedisConnection
} from '../../store/dataApi'
import { Button } from '../Button'
import { dataInputClass } from './DataResults'
import { ConnectionContext } from './ConnectionContext'
import { RedisValueEditor } from './RedisValueEditor'
import { useDataConfirmation } from './useDataConfirmation'
import { DataDialog } from './DataDialog'

const readCommands = new Set(
  'PING DBSIZE SCAN TYPE TTL PTTL GET STRLEN HSCAN HGET HLEN LRANGE LLEN SSCAN SCARD ZSCAN ZCARD ZRANGE XRANGE XLEN'.split(
    ' '
  )
)
const display = (value: unknown) => JSON.stringify(value, null, 2)

export function RedisBrowser({
  defaultAddress,
  workbench = false
}: {
  defaultAddress: string
  workbench?: boolean
}) {
  const { confirm, dialog } = useDataConfirmation('确认 Redis 数据变更')
  const [connection, setConnection] = useStoreState<RedisConnection>({
    address: '',
    username: '',
    password: '',
    db: 0
  })
  const [connectionDraft, setConnectionDraft] = useStoreState<RedisConnection>({
    address: '',
    username: '',
    password: '',
    db: 0
  })
  const [pattern, setPattern] = useStoreState('*')
  const [cursor, setCursor] = useStoreState('0')
  const [scanned, setScanned] = useStoreState(false)
  const [keys, setKeys] = useStoreState<string[]>([])
  const [key, setKey] = useStoreState('')
  const [type, setType] = useStoreState('')
  const [ttl, setTTL] = useStoreState('')
  const [value, setValue] = useStoreState('')
  const [originalValue, setOriginalValue] = useStoreState('')
  const [command, setCommand] = useStoreState('["PING"]')
  const [output, setOutput] = useStoreState('')
  const [error, setError] = useStoreState('')
  const [busy, setBusy] = useStoreState(false)
  const [loaded, setLoaded] = useStoreState(false)
  const [connectionState, setConnectionState] = useStoreState('未验证连接')
  const [configOpen, setConfigOpen] = useStoreState(false)
  const [createOpen, setCreateOpen] = useStoreState(false)
  const [newKey, setNewKey] = useStoreState('')
  const [newType, setNewType] = useStoreState<
    'string' | 'hash' | 'list' | 'set' | 'zset'
  >('string')
  const [newField, setNewField] = useStoreState('field')
  const [newValue, setNewValue] = useStoreState('')
  const [newScore, setNewScore] = useStoreState('0')
  const [renameOpen, setRenameOpen] = useStoreState(false)
  const [renamedKey, setRenamedKey] = useStoreState('')
  const [run, state] = useDataRedisMutation()
  const pending = useRef<ReturnType<typeof run> | null>(null)
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
      pending.current?.abort()
    }
  }, [])
  const send = async (args: string[], confirmed = false, expected?: string) => {
    if (!alive.current) throw new Error('窗口已关闭')
    const request = run({ ...connection, args, confirmed, expected })
    pending.current = request
    try {
      const reply = await request.unwrap()
      if (!alive.current) throw new Error('窗口已关闭')
      setConnectionState('最近请求成功')
      return reply.value
    } catch (e) {
      if (alive.current) setConnectionState('最近请求失败')
      throw e
    } finally {
      pending.current = null
      state.reset()
    }
  }
  const task = async (action: () => Promise<void>) => {
    setBusy(true)
    setError('')
    try {
      await action()
      return true
    } catch (e) {
      if (alive.current) setError(e instanceof Error ? e.message : dataError(e))
      return false
    } finally {
      if (alive.current) setBusy(false)
    }
  }
  const changeConnection = (next: Partial<RedisConnection>) => {
    setConnectionState('未验证连接')
    setLoaded(false)
    setConnection({ ...connection, ...next })
    setKeys([])
    setCursor('0')
    setScanned(false)
    setKey('')
    setType('')
    setValue('')
    setTTL('')
    setOutput('')
    setError('')
  }
  const scan = (next = '0') =>
    task(async () => {
      const reply = await send([
        'SCAN',
        next,
        'MATCH',
        pattern || '*',
        'COUNT',
        '100'
      ])
      if (!Array.isArray(reply) || !Array.isArray(reply[1]))
        throw new Error('Redis 扫描响应无效')
      setCursor(String(reply[0]))
      setKeys([...new Set(reply[1].map(String))])
      setScanned(true)
    })
  const inspect = (name: string) =>
    task(async () => {
      setKey(name)
      setType('')
      setValue('')
      setTTL('')
      setLoaded(false)
      const kind = String(await send(['TYPE', name]))
      setType(kind)
      setTTL(String(await send(['TTL', name])))
      const args =
        kind === 'string'
          ? ['GET', name]
          : kind === 'hash'
            ? ['HSCAN', name, '0', 'COUNT', '100']
            : kind === 'list'
              ? ['LRANGE', name, '0', '99']
              : kind === 'set'
                ? ['SSCAN', name, '0', 'COUNT', '100']
                : kind === 'zset'
                  ? ['ZSCAN', name, '0', 'COUNT', '100']
                  : kind === 'stream'
                    ? ['XRANGE', name, '-', '+', 'COUNT', '100']
                    : ['TYPE', name]
      const data = await send(args)
      setValue(
        kind === 'string' && typeof data === 'string' ? data : display(data)
      )
      if (kind === 'string' && typeof data !== 'string')
        throw new Error('键已过期或删除，请重新扫描')
      setOriginalValue(kind === 'string' ? String(data) : '')
      setCommand(display(args))
      setLoaded(true)
    })
  const mutate = async (args: string[], expected?: string) => {
    if (
      !(await confirm(
        `确认在 ${connection.address || defaultAddress} / DB ${connection.db} 执行 ${args[0]}，目标键：${args[1]}？变更无法通过此界面撤销。`
      ))
    )
      return false
    return await task(async () => {
      setOutput(display(await send(args, true, expected)))
      if (expected !== undefined) setOriginalValue(args[2])
      if (['DEL', 'UNLINK'].includes(args[0])) {
        setKeys(keys.filter(k => k !== args[1]))
        setType('none')
        setValue('')
        setTTL('-2')
      }
    })
  }
  const writeSelected = async (args: string[]) => {
    if (!key) return
    if (await mutate([args[0], key, ...args.slice(1)])) await inspect(key)
  }
  const create = async () => {
    if (!newKey.trim() || !newValue.trim()) return
    const args =
      newType === 'string'
        ? ['SET', newKey, newValue]
        : newType === 'hash'
          ? ['HSET', newKey, newField, newValue]
          : newType === 'list'
            ? ['RPUSH', newKey, newValue]
            : newType === 'set'
              ? ['SADD', newKey, newValue]
              : ['ZADD', newKey, newScore, newValue]
    if (await mutate(args)) {
      setKeys(values => [...new Set([...values, newKey])])
      setCreateOpen(false)
      setNewKey('')
      setNewValue('')
      await inspect(args[1])
    }
  }
  const rename = async () => {
    if (!key || !renamedKey.trim() || renamedKey === key) return
    if (await mutate(['RENAME', key, renamedKey])) {
      setKeys(values =>
        values.map(value => (value === key ? renamedKey : value))
      )
      setRenameOpen(false)
      await inspect(renamedKey)
    }
  }
  const execute = async () => {
    let args: unknown
    try {
      args = JSON.parse(command)
    } catch {
      setError('请以 JSON 字符串数组输入命令，例如 ["GET","my-key"]')
      return
    }
    if (
      !Array.isArray(args) ||
      !args.length ||
      !args.every(a => typeof a === 'string')
    ) {
      setError('命令必须是非空字符串数组')
      return
    }
    const values = args as string[]
    const writing = !readCommands.has(values[0].toUpperCase())
    if (
      writing &&
      !(await confirm(
        `确认在 ${connection.address || defaultAddress} / DB ${connection.db} 执行以下命令？\n${command.slice(0, 500)}`
      ))
    )
      return
    await task(async () => {
      setOutput(display(await send(values, writing)))
    })
  }
  return (
    <div className="grid min-w-0 gap-4">
      {dialog}
      <ConnectionContext
        engine="Redis"
        target={`${connection.address || defaultAddress || '本地默认服务'} / DB ${connection.db}`}
        state={busy ? '请求中' : connectionState}
        access="写入需确认"
      />
      <div className="flex flex-wrap gap-2">
        <Button
          disabled={busy}
          onClick={() =>
            void task(async () => {
              const reply = await send(['PING'])
              if (reply !== 'PONG')
                throw new Error('服务未返回预期的 PONG 响应')
            })
          }
        >
          测试连接
        </Button>
        <Button
          aria-expanded={createOpen}
          onClick={() => setCreateOpen(!createOpen)}
        >
          新建键
        </Button>
        <Button
          aria-expanded={configOpen}
          onClick={() => {
            setConnectionDraft(connection)
            setConfigOpen(true)
          }}
        >
          连接配置
        </Button>
      </div>
      {createOpen && (
        <DataDialog
          open
          title="新建 Redis 键"
          description={`${connection.address || defaultAddress} / DB ${connection.db} · 创建操作需再次确认`}
          busy={busy}
          onClose={() => setCreateOpen(false)}
        >
          <section
            aria-label="新建 Redis 键"
            className="grid gap-2 rounded-md border border-(--theme-border-default) p-3 sm:grid-cols-2"
          >
            <label className="text-xs">
              键名
              <input
                className={dataInputClass}
                value={newKey}
                disabled={busy}
                onChange={e => setNewKey(e.target.value)}
              />
            </label>
            <label className="text-xs">
              类型
              <select
                className={dataInputClass}
                value={newType}
                disabled={busy}
                onChange={e => setNewType(e.target.value as typeof newType)}
              >
                <option value="string">字符串</option>
                <option value="hash">Hash</option>
                <option value="list">列表</option>
                <option value="set">集合</option>
                <option value="zset">有序集合</option>
              </select>
            </label>
            {newType === 'hash' && (
              <label className="text-xs">
                字段名
                <input
                  className={dataInputClass}
                  value={newField}
                  disabled={busy}
                  onChange={e => setNewField(e.target.value)}
                />
              </label>
            )}
            <label className="text-xs">
              {newType === 'hash'
                ? '字段值'
                : newType === 'zset'
                  ? '成员'
                  : '初始值'}
              <input
                className={dataInputClass}
                value={newValue}
                disabled={busy}
                onChange={e => setNewValue(e.target.value)}
              />
            </label>
            {newType === 'zset' && (
              <label className="text-xs">
                分数
                <input
                  className={dataInputClass}
                  value={newScore}
                  disabled={busy}
                  onChange={e => setNewScore(e.target.value)}
                />
              </label>
            )}
            <div className="flex items-end">
              <Button
                disabled={
                  busy ||
                  !newKey.trim() ||
                  !newValue.trim() ||
                  (newType === 'hash' && !newField.trim()) ||
                  (newType === 'zset' && !Number.isFinite(Number(newScore)))
                }
                onClick={() => void create()}
              >
                创建并打开
              </Button>
            </div>
          </section>
          {error && (
            <p role="alert" className="text-sm text-(--theme-danger)">
              {error}
            </p>
          )}
        </DataDialog>
      )}
      <DataDialog
        open={configOpen}
        title="Redis 连接配置"
        busy={busy}
        onClose={() => setConfigOpen(false)}
        footer={
          <Button
            onClick={() => {
              if (
                JSON.stringify(connectionDraft) !== JSON.stringify(connection)
              )
                changeConnection(connectionDraft)
              setConfigOpen(false)
            }}
            disabled={busy}
          >
            完成
          </Button>
        }
      >
        <div className="grid gap-2 sm:grid-cols-2">
          <label className="text-xs">
            Redis 地址
            <input
              className={dataInputClass}
              value={connectionDraft.address}
              placeholder={defaultAddress || '127.0.0.1:6379'}
              disabled={busy}
              onChange={e =>
                setConnectionDraft({
                  ...connectionDraft,
                  address: e.target.value
                })
              }
            />
          </label>
          <label className="text-xs">
            数据库编号
            <input
              type="number"
              min="0"
              max="65535"
              className={dataInputClass}
              value={connectionDraft.db}
              disabled={busy}
              onChange={e =>
                setConnectionDraft({
                  ...connectionDraft,
                  db: Number(e.target.value)
                })
              }
            />
          </label>
          <label className="text-xs">
            用户名（可选）
            <input
              className={dataInputClass}
              autoComplete="off"
              value={connectionDraft.username}
              disabled={busy}
              onChange={e =>
                setConnectionDraft({
                  ...connectionDraft,
                  username: e.target.value
                })
              }
            />
          </label>
          <label className="text-xs">
            密码（仅本次窗口）
            <input
              type="password"
              autoComplete="new-password"
              className={dataInputClass}
              value={connectionDraft.password}
              disabled={busy}
              onChange={e =>
                setConnectionDraft({
                  ...connectionDraft,
                  password: e.target.value
                })
              }
            />
          </label>
        </div>
        <p className="text-xs text-(--theme-text-secondary)">
          地址留空使用本地 Redis，可在上方“本地服务”中管理。自定义连接使用普通
          TCP，不支持 TLS / Cluster。大值会拒绝返回，不做截断保存。
        </p>
      </DataDialog>
      <div className={workbench ? 'hidden' : 'flex flex-wrap items-end gap-2'}>
        <label className="min-w-36 flex-1 text-xs">
          键名匹配
          <input
            className={dataInputClass}
            value={pattern}
            disabled={busy}
            onChange={e => {
              setPattern(e.target.value)
              setScanned(false)
              setCursor('0')
            }}
          />
        </label>
        <Button disabled={busy} onClick={() => void scan()}>
          扫描键
        </Button>
        <Button
          disabled={busy || !scanned || cursor === '0'}
          onClick={() => void scan(cursor)}
        >
          下一批
        </Button>
      </div>
      {busy && (
        <p role="status" className="text-xs">
          正在访问 Redis…
        </p>
      )}
      {error && !createOpen && !renameOpen && !configOpen && (
        <p
          role="alert"
          className="whitespace-pre-wrap text-sm text-red-600 dark:text-red-400"
        >
          {error}
        </p>
      )}
      <div
        className={`grid min-w-0 gap-4 ${workbench ? '' : 'sm:grid-cols-[14rem_minmax(0,1fr)]'}`}
      >
        <aside
          className={`${workbench ? 'hidden' : ''} max-h-96 overflow-auto rounded-md border border-(--theme-border-default) p-2`}
          aria-label="Redis 键列表"
        >
          <p className="mb-2 text-xs">
            本批 {keys.length} 个键 ·{' '}
            {scanned && cursor === '0' ? '扫描结束' : `游标 ${cursor}`}
          </p>
          {keys.map(name => (
            <Button
              className="mb-1 w-full justify-start truncate"
              title={name}
              key={name}
              aria-pressed={key === name}
              disabled={busy}
              onClick={() => void inspect(name)}
            >
              {name || '（空键名）'}
            </Button>
          ))}
          {!keys.length && (
            <p className="text-xs">
              {scanned
                ? '本批为空；游标非 0 时可继续扫描。'
                : '点击“扫描键”连接服务'}
            </p>
          )}
        </aside>
        <div className="grid min-w-0 gap-3">
          <div hidden={workbench} className="grid gap-3">
            <h3 className="break-all text-sm font-medium">
              {type ? `${key || '（空键名）'} · ${type}` : '选择一个键查看数据'}
            </h3>
            {type && type !== 'none' && (
              <>
                <div className="flex flex-wrap items-end gap-2">
                  <label className="text-xs">
                    TTL 秒（-1 永久，-2 不存在）
                    <input
                      className={dataInputClass}
                      value={ttl}
                      disabled={busy}
                      onChange={e => setTTL(e.target.value)}
                    />
                  </label>
                  <Button
                    disabled={
                      busy ||
                      !loaded ||
                      !/^-?\d+$/.test(ttl) ||
                      Number(ttl) < -1
                    }
                    onClick={() =>
                      void mutate(
                        ttl === '-1' ? ['PERSIST', key] : ['EXPIRE', key, ttl]
                      )
                    }
                  >
                    保存 TTL
                  </Button>
                  <Button
                    variant="danger"
                    disabled={busy}
                    onClick={() => void mutate(['DEL', key])}
                  >
                    删除键
                  </Button>
                  <Button
                    disabled={busy}
                    aria-expanded={renameOpen}
                    onClick={() => {
                      setRenameOpen(!renameOpen)
                      setRenamedKey(key)
                    }}
                  >
                    重命名键
                  </Button>
                </div>
                {renameOpen && (
                  <DataDialog
                    open
                    title="重命名 Redis 键"
                    description={`当前键：${key} · 目标名称已存在时可能被覆盖`}
                    busy={busy}
                    onClose={() => setRenameOpen(false)}
                  >
                    <div className="flex flex-wrap items-end gap-2">
                      <label className="min-w-48 flex-1 text-xs">
                        新键名
                        <input
                          className={dataInputClass}
                          value={renamedKey}
                          disabled={busy}
                          onChange={e => setRenamedKey(e.target.value)}
                        />
                      </label>
                      <Button
                        disabled={
                          busy || !renamedKey.trim() || renamedKey === key
                        }
                        onClick={() => void rename()}
                      >
                        确认重命名
                      </Button>
                    </div>
                    {error && (
                      <p role="alert" className="text-sm text-(--theme-danger)">
                        {error}
                      </p>
                    )}
                  </DataDialog>
                )}
                <label className="text-xs">
                  {type === 'string'
                    ? '字符串值'
                    : '数据预览（集合游标在数组首项；列表 / Stream 最多 100 项）'}
                  <textarea
                    className={`${dataInputClass} mt-1 min-h-32 font-mono`}
                    value={value}
                    readOnly={type !== 'string'}
                    disabled={busy}
                    onChange={e => setValue(e.target.value)}
                  />
                </label>
                {type === 'string' && (
                  <Button
                    disabled={busy || !loaded}
                    onClick={() =>
                      void mutate(['SET', key, value, 'KEEPTTL'], originalValue)
                    }
                  >
                    保存字符串（保留 TTL）
                  </Button>
                )}
                <RedisValueEditor
                  type={type}
                  value={value}
                  busy={busy}
                  onWrite={writeSelected}
                />
              </>
            )}
          </div>
          <section
            hidden={!workbench}
            className="rounded-md border border-(--theme-border-default) p-3"
          >
            <h3 className="text-sm font-medium">命令工作台</h3>
            <p className="my-2 text-xs text-(--theme-text-secondary)">
              使用 JSON 数组保留空格和换行。支持 SET、HSET / HDEL、LPUSH /
              LSET、SADD / SREM、ZADD / ZREM 等。集合可用 HSCAN / SSCAN / ZSCAN
              的返回游标继续读取。禁止 FLUSH、EVAL、CONFIG。
            </p>
            <label className="text-xs">
              Redis 命令
              <textarea
                className={`${dataInputClass} mt-1 min-h-24 font-mono`}
                value={command}
                disabled={busy}
                spellCheck={false}
                onChange={e => setCommand(e.target.value)}
              />
            </label>
            <Button
              className="mt-2"
              disabled={busy}
              onClick={() => void execute()}
            >
              执行命令
            </Button>
          </section>
          {output && (
            <pre
              aria-label="Redis 执行结果"
              className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs"
            >
              {output}
            </pre>
          )}
        </div>
      </div>
    </div>
  )
}
