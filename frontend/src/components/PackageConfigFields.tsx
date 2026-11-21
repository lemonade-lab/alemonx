import { ChevronDown, Plus, X } from 'lucide-react'
import type { PackageConfigField } from '../store/workspaceApi'
import { validateFieldValue } from './configFieldUtils'

function defaultText(value: unknown): string {
  if (value === undefined) return ''
  if (typeof value === 'boolean') return value ? '开启' : '关闭'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

const fieldClass =
  'grid min-w-0 gap-2 text-sm font-medium text-(--theme-text-primary)'

function inputClass(invalid = false) {
  return `min-h-11 min-w-0 w-full rounded-md border bg-(--theme-surface-input) px-3 text-base font-normal text-(--theme-text-primary) outline-none focus:ring-2 sm:text-sm ${
    invalid
      ? 'border-(--theme-danger) focus:ring-(--theme-danger)'
      : 'border-(--theme-border-strong) focus:ring-(--theme-accent)'
  }`
}

function FieldLabel({
  field,
  messages = []
}: {
  field: PackageConfigField
  messages?: string[]
}) {
  const defaultValue =
    field.default !== undefined && field.default !== null
      ? `默认：${defaultText(field.default)}`
      : ''
  return (
    <span className="grid min-w-0 gap-0.5 wrap-anywhere">
      <span className="leading-relaxed">
        {field.description || field.name}
        {field.required && (
          <span className="ms-2 inline-block text-xs text-(--theme-accent-text)">
            必填
          </span>
        )}
      </span>
      {defaultValue && (
        <span className="text-xs font-normal leading-relaxed text-(--theme-text-muted)">
          {defaultValue}
        </span>
      )}
      {messages.length > 0 && (
        <span className="text-xs font-normal leading-relaxed text-(--theme-danger-text)">
          {messages.join('；')}
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
  const type = field.type

  if (type === 'object') {
    const objectValue =
      value && typeof value === 'object' && !Array.isArray(value)
        ? (value as Record<string, unknown>)
        : {}
    const children = field.config ?? []
    return (
      <details className="group/config col-span-full min-w-0 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-panel)">
        <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 rounded-lg px-3 py-2.5 text-sm font-semibold text-(--theme-text-primary) marker:content-none hover:bg-(--theme-surface-hover) focus-visible:outline-2 focus-visible:outline-(--theme-accent) [&::-webkit-details-marker]:hidden">
          <FieldLabel field={field} />
          <ChevronDown
            aria-hidden="true"
            className="size-4 shrink-0 text-(--theme-text-secondary) group-open/config:rotate-180"
          />
        </summary>
        {children.length ? (
          <div className="min-w-0 border-t border-(--theme-border-default) p-3 sm:p-4">
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
          <p className="border-t border-(--theme-border-default) p-3 text-xs text-(--theme-text-muted)">
            自由对象，暂无子配置声明。
          </p>
        )}
      </details>
    )
  }

  if (type === 'array<string>' || type === 'array<number>') {
    const items = Array.isArray(value) ? (value as unknown[]) : []
    const isNumber = type === 'array<number>'
    const messages = validateFieldValue(field, items)
    return (
      <fieldset className={fieldClass}>
        <legend className="mb-2 w-full">
          <FieldLabel field={field} messages={messages} />
        </legend>
        <div className="grid min-w-0 gap-2">
          {items.map((item, index) => (
            <div className="flex gap-1.5" key={index}>
              <input
                className={inputClass(messages.length > 0)}
                aria-label={`${field.description || field.name}，第 ${index + 1} 项`}
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
                className="config-field-remove inline-flex size-11 shrink-0 items-center justify-center rounded-md text-(--theme-text-secondary)"
                onClick={() => {
                  const next = [...items]
                  next.splice(index, 1)
                  onChange(next)
                }}
                aria-label={`移除第 ${index + 1} 项`}
              >
                <X className="size-4" />
              </button>
            </div>
          ))}
          <button
            type="button"
            className="config-field-action inline-flex min-h-11 w-fit items-center justify-center gap-2 rounded-md px-3 text-sm text-(--theme-text-secondary)"
            onClick={() => onChange([...items, isNumber ? 0 : ''])}
          >
            <Plus className="size-3.5" />
            添加一项
          </button>
        </div>
      </fieldset>
    )
  }

  if (type === 'boolean' || type === 'bool') {
    const current = value === true ? 'true' : value === false ? 'false' : ''
    const messages = validateFieldValue(field, value)
    return (
      <label className={fieldClass}>
        <FieldLabel field={field} messages={messages} />
        <select
          className={`${inputClass(messages.length > 0)} sm:max-w-48`}
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
    <label className={fieldClass}>
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
  className = 'grid min-w-0 gap-5'
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
          className="inline-flex min-h-9 items-center rounded-md border border-(--theme-border-default) px-2.5 text-xs font-medium text-(--theme-text-secondary) hover:border-(--theme-accent) hover:text-(--theme-accent-text)"
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
