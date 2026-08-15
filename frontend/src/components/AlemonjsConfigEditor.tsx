import { ChevronDown, ChevronUp, Plus, Trash2 } from 'lucide-react'
import type { PackageConfigField } from '../store/workspaceApi'

const fieldTypes = [
  ['string', '文本'],
  ['number', '数字'],
  ['integer', '整数'],
  ['boolean', '开关'],
  ['array<string>', '文本列表'],
  ['array<number>', '数字列表'],
  ['object', '对象']
] as const

const inputClass =
  'min-h-8 w-full rounded-md border border-slate-300 bg-white px-2 text-xs font-normal text-slate-800 outline-none transition focus:border-brand-600 focus:ring-2 focus:ring-brand-100'
const actionClass =
  'inline-flex size-8 items-center justify-center rounded-md border border-slate-200 bg-white text-slate-500 transition hover:border-brand-300 hover:text-brand-700 disabled:cursor-not-allowed disabled:opacity-40'

function emptyField(): PackageConfigField {
  return { name: '', type: 'string', required: false, description: '' }
}

function defaultText(value: unknown): string {
  if (value === undefined || value === null) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function parseDefault(value: string, type: string): unknown {
  if (value === '') return undefined
  if (type === 'boolean') return value === 'true'
  if (type === 'number' || type === 'integer') {
    const number = Number(value)
    return Number.isNaN(number) ? value : number
  }
  if (type === 'object' || type.startsWith('array<')) {
    try {
      return JSON.parse(value)
    } catch {
      return value
    }
  }
  return value
}

function updateAt<T>(items: T[], index: number, value: T) {
  return items.map((item, itemIndex) => (itemIndex === index ? value : item))
}

function FieldCard({
  field,
  index,
  count,
  depth,
  onChange,
  onMove,
  onRemove
}: {
  field: PackageConfigField
  index: number
  count: number
  depth: number
  onChange: (next: PackageConfigField) => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
}) {
  const update = (next: Partial<PackageConfigField>) => onChange({ ...field, ...next })
  const rules = field.rules ?? []
  return (
    <article className="grid gap-3 rounded-lg border border-slate-200 bg-slate-50/60 p-3">
      <header className="flex items-center justify-between gap-3">
        <strong className="text-xs font-semibold text-slate-700">
          {depth ? '嵌套字段' : '配置字段'} {index + 1}
        </strong>
        <div className="flex items-center gap-1">
          <button type="button" className={actionClass} onClick={() => onMove(-1)} disabled={index === 0} aria-label="上移字段">
            <ChevronUp className="size-3.5" />
          </button>
          <button type="button" className={actionClass} onClick={() => onMove(1)} disabled={index === count - 1} aria-label="下移字段">
            <ChevronDown className="size-3.5" />
          </button>
          <button type="button" className={`${actionClass} hover:border-red-300 hover:text-red-600`} onClick={onRemove} aria-label="删除字段">
            <Trash2 className="size-3.5" />
          </button>
        </div>
      </header>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <label className="grid gap-1 text-[11px] font-semibold text-slate-600">
          字段名
          <input className={inputClass} value={field.name} onChange={event => update({ name: event.target.value })} placeholder="token" />
        </label>
        <label className="grid gap-1 text-[11px] font-semibold text-slate-600">
          类型
          <select
            className={inputClass}
            value={field.type || 'string'}
            onChange={event => {
              const type = event.target.value
              update(type === 'object' ? { type } : { type, config: undefined })
            }}
          >
            {fieldTypes.map(([value, label]) => <option key={value} value={value}>{label}（{value}）</option>)}
          </select>
        </label>
        <label className="grid gap-1 text-[11px] font-semibold text-slate-600">
          显示说明
          <input className={inputClass} value={field.description} onChange={event => update({ description: event.target.value })} placeholder="访问令牌" />
        </label>
        <label className="grid gap-1 text-[11px] font-semibold text-slate-600">
          默认值
          {field.type === 'boolean' ? (
            <select className={inputClass} value={field.default === undefined ? '' : String(field.default)} onChange={event => update({ default: parseDefault(event.target.value, field.type) })}>
              <option value="">未设置</option>
              <option value="true">开启</option>
              <option value="false">关闭</option>
            </select>
          ) : (
            <input className={inputClass} value={defaultText(field.default)} onChange={event => update({ default: parseDefault(event.target.value, field.type) })} placeholder={field.type === 'object' || field.type.startsWith('array<') ? 'JSON 值' : '可选'} />
          )}
        </label>
      </div>
      <label className="flex items-center gap-2 text-[11px] font-semibold text-slate-600">
        <input className="size-3.5 accent-brand-600" type="checkbox" checked={field.required} onChange={event => update({ required: event.target.checked })} />
        必填字段
      </label>
      <div className="grid gap-1.5 rounded-md border border-slate-200 bg-white p-2">
        <span className="text-[11px] font-semibold text-slate-600">校验规则</span>
        {rules.map((rule, ruleIndex) => (
          <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] gap-1.5" key={ruleIndex}>
            <input className={inputClass} value={rule.pattern} onChange={event => update({ rules: updateAt(rules, ruleIndex, { ...rule, pattern: event.target.value }) })} placeholder="正则表达式" />
            <input className={inputClass} value={rule.message ?? ''} onChange={event => update({ rules: updateAt(rules, ruleIndex, { ...rule, message: event.target.value }) })} placeholder="校验提示" />
            <button type="button" className={`${actionClass} hover:border-red-300 hover:text-red-600`} onClick={() => update({ rules: rules.filter((_, itemIndex) => itemIndex !== ruleIndex) })} aria-label="删除校验规则">
              <Trash2 className="size-3.5" />
            </button>
          </div>
        ))}
        <button type="button" className="inline-flex h-7 w-fit items-center gap-1 rounded-md px-1.5 text-[11px] font-semibold text-slate-500 transition hover:bg-slate-100 hover:text-brand-700" onClick={() => update({ rules: [...rules, { pattern: '', message: '' }] })}>
          <Plus className="size-3" /> 添加规则
        </button>
      </div>
      {field.type === 'object' && (
        <div className="grid gap-2 rounded-md border border-brand-100 bg-brand-50/35 p-2.5">
          <span className="text-[11px] font-semibold text-brand-800">对象子字段</span>
          <FieldList fields={field.config ?? []} depth={depth + 1} onChange={config => update({ config })} />
        </div>
      )}
    </article>
  )
}

function FieldList({ fields, depth = 0, onChange }: { fields: PackageConfigField[]; depth?: number; onChange: (fields: PackageConfigField[]) => void }) {
  const move = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= fields.length) return
    const next = [...fields]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }
  return (
    <div className="grid gap-2">
      {fields.map((field, index) => (
        <FieldCard
          key={`${field.name}-${index}`}
          field={field}
          index={index}
          count={fields.length}
          depth={depth}
          onChange={next => onChange(updateAt(fields, index, next))}
          onMove={direction => move(index, direction)}
          onRemove={() => onChange(fields.filter((_, itemIndex) => itemIndex !== index))}
        />
      ))}
      <button type="button" className="inline-flex h-8 w-fit items-center gap-1.5 rounded-md border border-dashed border-slate-300 bg-white px-2.5 text-xs font-semibold text-slate-600 transition hover:border-brand-400 hover:text-brand-700" onClick={() => onChange([...fields, emptyField()])}>
        <Plus className="size-3.5" /> 添加{depth ? '子' : ''}字段
      </button>
    </div>
  )
}

export function AlemonjsConfigEditor({ fields, onChange }: { fields: PackageConfigField[]; onChange: (fields: PackageConfigField[]) => void }) {
  if (!fields.length)
    return (
      <div className="grid justify-items-start gap-2 rounded-lg border border-dashed border-slate-300 bg-slate-50/70 p-4 text-xs text-slate-500">
        <p>尚未声明可配置字段。添加后，工作台会自动生成对应的配置表单。</p>
        <button type="button" className="inline-flex h-8 items-center gap-1.5 rounded-md bg-brand-600 px-2.5 text-xs font-semibold text-white transition hover:bg-brand-700" onClick={() => onChange([emptyField()])}>
          <Plus className="size-3.5" /> 添加字段
        </button>
      </div>
    )
  return <FieldList fields={fields} onChange={onChange} />
}
