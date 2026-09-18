import { useMemo, useState } from 'react'
import { Button } from '../Button'
import { dataInputClass } from './DataResults'

type Props = {
  type: string
  value: string
  busy: boolean
  onWrite: (args: string[]) => Promise<void>
}

const parsed = (value: string): unknown => {
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

function PairEditor({
  type,
  value,
  busy,
  onWrite
}: Props & { type: 'hash' | 'zset' }) {
  const [field, setField] = useState('')
  const [content, setContent] = useState('')
  const pairs = useMemo(() => {
    const reply = parsed(value)
    const items =
      Array.isArray(reply) && Array.isArray(reply[1]) ? reply[1] : []
    const result: Array<[string, string]> = []
    for (let i = 0; i + 1 < items.length; i += 2)
      result.push([String(items[i]), String(items[i + 1])])
    return result
  }, [value])
  const add = async () => {
    if (!field.trim() || !content.trim()) return
    const args =
      type === 'hash' ? ['HSET', field, content] : ['ZADD', content, field]
    await onWrite(args)
    setField('')
    setContent('')
  }
  return (
    <section
      className="grid gap-2"
      aria-label={type === 'hash' ? 'Hash 编辑器' : '有序集合编辑器'}
    >
      <p className="text-xs text-(--theme-text-secondary)">
        {type === 'hash' ? '字段和值' : '成员和分数'} · 当前批次 {pairs.length}{' '}
        项
      </p>
      <div className="max-h-64 overflow-auto rounded-md border border-(--theme-border-default)">
        {pairs.map(([left, right]) => (
          <div
            key={`${left}:${right}`}
            className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2 border-b border-(--theme-border-default) p-2 text-xs last:border-0"
          >
            <code className="truncate" title={left}>
              {left}
            </code>
            <code className="truncate" title={right}>
              {right}
            </code>
            <Button
              size="sm"
              variant="danger"
              disabled={busy}
              onClick={() =>
                void onWrite(type === 'hash' ? ['HDEL', left] : ['ZREM', left])
              }
            >
              删除
            </Button>
          </div>
        ))}
        {!pairs.length && <p className="p-3 text-xs">此批次没有数据</p>}
      </div>
      <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
        <label className="text-xs">
          {type === 'hash' ? '字段' : '成员'}
          <input
            className={dataInputClass}
            value={field}
            disabled={busy}
            onChange={e => setField(e.target.value)}
          />
        </label>
        <label className="text-xs">
          {type === 'hash' ? '值' : '分数'}
          <input
            className={dataInputClass}
            value={content}
            disabled={busy}
            onChange={e => setContent(e.target.value)}
          />
        </label>
        <Button
          className="self-end"
          disabled={busy || !field.trim() || !content.trim()}
          onClick={() => void add()}
        >
          {type === 'hash' ? '保存字段' : '保存成员'}
        </Button>
      </div>
    </section>
  )
}

function MembersEditor({
  type,
  value,
  busy,
  onWrite
}: Props & { type: 'list' | 'set' }) {
  const [member, setMember] = useState('')
  const members = useMemo(() => {
    const reply = parsed(value)
    return Array.isArray(reply) && type === 'set' && Array.isArray(reply[1])
      ? reply[1].map(String)
      : Array.isArray(reply)
        ? reply.map(String)
        : []
  }, [type, value])
  const add = async () => {
    if (!member.trim()) return
    await onWrite([type === 'list' ? 'RPUSH' : 'SADD', member])
    setMember('')
  }
  return (
    <section
      className="grid gap-2"
      aria-label={type === 'list' ? '列表编辑器' : '集合编辑器'}
    >
      <p className="text-xs text-(--theme-text-secondary)">
        {type === 'list' ? '列表按读取顺序显示' : '集合成员'} · 当前批次{' '}
        {members.length} 项
      </p>
      <div className="max-h-64 overflow-auto rounded-md border border-(--theme-border-default)">
        {members.map((item, index) => (
          <div
            key={`${index}:${item}`}
            className="flex items-center gap-2 border-b border-(--theme-border-default) p-2 text-xs last:border-0"
          >
            <code className="min-w-0 flex-1 break-all">{item}</code>
            <Button
              size="sm"
              variant="danger"
              disabled={busy}
              onClick={() =>
                void onWrite(
                  type === 'list' ? ['LREM', '1', item] : ['SREM', item]
                )
              }
            >
              删除
            </Button>
          </div>
        ))}
        {!members.length && <p className="p-3 text-xs">此批次没有数据</p>}
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <label className="min-w-48 flex-1 text-xs">
          {type === 'list' ? '追加元素' : '新增成员'}
          <input
            className={dataInputClass}
            value={member}
            disabled={busy}
            onChange={e => setMember(e.target.value)}
          />
        </label>
        <Button disabled={busy || !member.trim()} onClick={() => void add()}>
          添加
        </Button>
      </div>
    </section>
  )
}

export function RedisValueEditor(props: Props) {
  if (props.type === 'hash') return <PairEditor {...props} type="hash" />
  if (props.type === 'zset') return <PairEditor {...props} type="zset" />
  if (props.type === 'list') return <MembersEditor {...props} type="list" />
  if (props.type === 'set') return <MembersEditor {...props} type="set" />
  if (props.type === 'stream')
    return (
      <p className="text-xs text-(--theme-text-secondary)">
        Stream 以只读预览显示；当前版本不提供追加或修改入口。
      </p>
    )
  return null
}
