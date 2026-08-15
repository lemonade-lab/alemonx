import { Plus, X } from 'lucide-react'
import { useState } from 'react'
import type { PackageConfigField } from '../store/workspaceApi'
import { validateFieldValue } from './configFieldUtils'

function defaultText(value: unknown): string {
  if (value === undefined) return ''
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function inputClass(invalid = false) {
  return `min-h-9 w-full rounded-md border bg-white px-2.5 text-sm font-normal text-slate-800 outline-none transition focus:ring-2 ${
    invalid
      ? 'border-red-400 focus:border-red-500 focus:ring-red-100'
      : 'border-slate-300 focus:border-brand-600 focus:ring-brand-100'
  }`
}

function FieldLabel({
  field,
  tone,
  messages = []
}: {
  field: PackageConfigField
  tone?: 'orange' | 'amber'
  messages?: string[]
}) {
  const defaultValue =
    field.default !== undefined && field.default !== null
      ? `默认：${defaultText(field.default)}`
      : ''
  const status = messages.length ? '输入有误' : defaultValue
  const statusTitle = messages.length ? messages.join('\n') : defaultValue
  return (
    <span className="flex min-w-0 items-center justify-between gap-2">
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="truncate">{field.description || field.name}</span>
        {field.required && (
          <em
            className={`not-italic ${
              tone === 'orange' ? 'text-orange-700' : 'text-amber-700'
            }`}
          >
            必填
          </em>
        )}
      </span>
      {status && (
        <span
          className={`max-w-[52%] shrink truncate font-normal ${
            messages.length ? 'text-red-600' : 'text-slate-400'
          }`}
          title={statusTitle}
        >
          {status}
        </span>
      )}
    </span>
  )
}

function FieldControl({
  field,
  value,
  onChange
}: {
  field: PackageConfigField
  value: unknown
  onChange: (value: unknown) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const type = field.type

  if (type === 'object') {
    const objectValue =
      value && typeof value === 'object' && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : {}
    const children = field.config ?? []
    return (
      <details
        className="grid gap-1 rounded-lg border border-slate-200 bg-slate-50/60 p-2.5"
        open={expanded}
        onToggle={event => setExpanded(event.currentTarget.open)}
      >
        <summary className="flex cursor-pointer list-none items-center gap-2 text-xs font-semibold text-slate-700 marker:content-none [&::-webkit-details-marker]:hidden">
          <FieldLabel field={field} />
          <i className="ml-auto text-sm font-normal text-slate-400 transition-transform group-open:rotate-90">
            ›
          </i>
        </summary>
        {children.length ? (
          <div className="grid gap-2.5 pt-2">
            <ConfigFieldsEditor
              fields={children}
              values={objectValue}
              onChange={(name, childValue) => {
                const next = { ...objectValue, [name]: childValue }
                onChange(next)
              }}
            />
          </div>
        ) : (
          <p className="pt-1 text-[11px] text-slate-400">自由对象，暂无子配置声明。</p>
        )}
      </details>
    )
  }

  if (type === 'array<string>' || type === 'array<number>') {
    const items = Array.isArray(value) ? (value as unknown[]) : []
    const isNumber = type === 'array<number>'
    const messages = validateFieldValue(field, items)
    return (
      <label className="grid gap-1 text-xs font-semibold text-slate-600">
        <FieldLabel field={field} messages={messages} />
        <div className="grid gap-1.5">
          {items.map((item, index) => (
            <div className="flex gap-1.5" key={index}>
              <input
                className={inputClass(messages.length > 0)}
                aria-invalid={messages.length > 0}
                type={isNumber ? 'number' : 'text'}
                value={String(item ?? '')}
                onChange={event => {
                  const raw = event.target.value
                  const next = [...items]
                  if (raw === '') {
                    next.splice(index, 1)
                  } else {
                    next[index] = isNumber ? Number(raw) : raw
                  }
                  onChange(next)
                }}
              />
              <button
                type="button"
                className="config-field-action inline-flex size-9 shrink-0 items-center justify-center rounded-md text-slate-500"
                onClick={() => {
                  const next = [...items]
                  next.splice(index, 1)
                  onChange(next)
                }}
                aria-label="移除一项"
              >
                <X className="size-4" />
              </button>
            </div>
          ))}
          <button
            type="button"
            className="config-field-action inline-flex h-8 items-center justify-center gap-1.5 rounded-md px-2.5 text-xs text-slate-500"
            onClick={() => onChange([...items, isNumber ? 0 : ''])}
          >
            <Plus className="size-3.5" />添加一项
          </button>
        </div>
      </label>
    )
  }

  if (type === 'boolean' || type === 'bool') {
    const current = value === true ? 'true' : value === false ? 'false' : ''
    const messages = validateFieldValue(field, value)
    return (
      <label className="grid gap-1 text-xs font-semibold text-slate-600">
        <FieldLabel field={field} messages={messages} />
        <select
          className={inputClass(messages.length > 0)}
          aria-invalid={messages.length > 0}
          value={current}
          onChange={event => {
            const next = event.target.value
            onChange(next === '' ? '' : next === 'true')
          }}
        >
          <option value="">不设置</option>
          <option value="true">开启</option>
          <option value="false">关闭</option>
        </select>
      </label>
    )
  }

  const isNumber = type === 'number' || type === 'integer'
  const textValue = value === undefined || value === null ? '' : String(value)
  const messages = validateFieldValue(field, value)
  return (
    <label className="grid gap-1 text-xs font-semibold text-slate-600">
      <FieldLabel field={field} messages={messages} />
      <input
        className={inputClass(messages.length > 0)}
        aria-invalid={messages.length > 0}
        type={isNumber ? 'number' : 'text'}
        value={textValue}
        onChange={event => {
          const raw = event.target.value
          if (isNumber && raw !== '') {
            const parsed = Number(raw)
            onChange(Number.isNaN(parsed) ? raw : parsed)
          } else {
            onChange(raw)
          }
        }}
        placeholder={field.name}
      />
    </label>
  )
}

export function ConfigFieldsEditor({
  fields,
  values,
  onChange,
  className = 'grid gap-3 sm:grid-cols-2'
}: {
  fields: PackageConfigField[]
  values: Record<string, unknown>
  onChange: (name: string, value: unknown) => void
  className?: string
}) {
  return (
    <div className={className}>
      {fields.map(field => (
        <FieldControl
          key={field.name}
          field={field}
          value={values[field.name]}
          onChange={value => onChange(field.name, value)}
        />
      ))}
    </div>
  )
}

export function ConfigSourceLinks({
  source,
  readmeURL
}: {
  source?: { readme?: string; official?: string; platform?: string }
  readmeURL?: string
}) {
  const links: Array<{ label: string; href: string }> = []
  if (source?.readme) links.push({ label: '配置文档', href: source.readme })
  if (source?.official) links.push({ label: '官方文档', href: source.official })
  if (source?.platform) links.push({ label: '获取配置', href: source.platform })
  if (readmeURL && !source?.readme)
    links.push({ label: '在线文档', href: readmeURL })
  if (!links.length) return null
  return (
    <div className="flex flex-wrap items-center gap-2">
      {links.map(link => (
        <a
          key={link.href}
          className="rounded-md border border-slate-200 px-2.5 py-1 text-[11px] font-semibold text-slate-600 hover:border-brand-600 hover:text-brand-600"
          href={link.href}
          target="_blank"
          rel="noreferrer"
        >
          {link.label} ↗
        </a>
      ))}
    </div>
  )
}
