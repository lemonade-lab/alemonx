import { useStoreState } from '../store/guideStore'
import { useEffect, useState, type ReactNode } from 'react'
import { Settings, SlidersHorizontal } from 'lucide-react'
import cn from 'classnames'
import { load } from 'js-yaml'
import { RobotPanel } from './RobotPanel'

type Props = {
  content: string
  toolbar?: ReactNode
  extensionConfig?: ReactNode
  onChange: (content: string) => void
}
type ToggleKey = 'masterID' | 'masterKey' | 'botID' | 'botKey' | 'apps'
type AccessItem = { value: string; enabled: boolean }
type TextKey =
  | 'port'
  | 'serverPort'
  | 'input'
  | 'login'
  | 'url'
  | 'fullReceive'
  | 'disabledRegular'
  | 'disabledSelects'
  | 'disabledUserID'
  | 'disabledUserKey'
  | 'redirectRegular'
  | 'redirectTarget'
  | 'mappingRegular'
  | 'mappingTarget'
  | 'repeatedEventTime'
  | 'repeatedUserTime'
  | 'cbpTimeout'
  | 'cbpReconnect'
  | 'cbpHeartbeat'
  | 'cbpHealthCheck'
  | 'cbpUserAgent'
  | 'cbpDeviceID'
  | 'cbpFullReceive'
  | 'autoPort'
type Values = Record<TextKey, string> & Record<ToggleKey, AccessItem[]>
const empty: Values = {
  port: '',
  serverPort: '',
  input: '',
  login: '',
  url: '',
  fullReceive: '',
  masterID: [],
  masterKey: [],
  botID: [],
  botKey: [],
  disabledRegular: '',
  disabledSelects: '',
  disabledUserID: '',
  disabledUserKey: '',
  redirectRegular: '',
  redirectTarget: '',
  mappingRegular: '',
  mappingTarget: '',
  repeatedEventTime: '',
  repeatedUserTime: '',
  apps: [],
  autoPort: '',
  cbpTimeout: '',
  cbpReconnect: '',
  cbpHeartbeat: '',
  cbpHealthCheck: '',
  cbpUserAgent: '',
  cbpDeviceID: '',
  cbpFullReceive: ''
}

function sameValues(left: Values, right: Values) {
  return Object.keys(empty).every(
    key => JSON.stringify(left[key as keyof Values]) === JSON.stringify(right[key as keyof Values])
  )
}
const managed = new Set([
  'port',
  'serverPort',
  'input',
  'login',
  'url',
  'is_full_receive',
  'master_id',
  'master_key',
  'bot_id',
  'bot_key',
  'disabled_text_regular',
  'disabled_selects',
  'disabled_user_id',
  'disabled_user_key',
  'redirect_text_regular',
  'redirect_text_target',
  'mapping_text',
  'processor',
  'autoPort',
  'apps',
  'cbp'
])
const quote = (value: string) => `'${value.replace(/'/g, "''")}'`
const mapLines = (key: string, values: AccessItem[]) => {
  const entries = values.filter(item => item.value.trim())
  return entries.length
    ? [
        `${key}:`,
        ...entries.map(item => `  ${quote(item.value.trim())}: ${item.enabled}`)
      ]
    : []
}
const stringMapLines = (key: string, values: string) => {
  const entries = values
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  return entries.length
    ? [`${key}:`, ...entries.map(value => `  ${quote(value)}: true`)]
    : []
}
const record = (value: unknown): Record<string, unknown> | null =>
  value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
const text = (value: unknown) =>
  typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
    ? String(value)
    : ''
const list = (value: unknown) => {
  if (Array.isArray(value)) return value.map(text).filter(Boolean).join(', ')
  const values = record(value)
  return values
    ? Object.entries(values)
        .filter(([, enabled]) => enabled === true)
        .map(([key]) => key)
        .join(', ')
    : ''
}
const accessItems = (value: unknown): AccessItem[] => {
  if (Array.isArray(value))
    return value.map(text).filter(Boolean).map(value => ({ value, enabled: true }))
  const values = record(value)
  return values
    ? Object.entries(values).map(([value, enabled]) => ({
        value,
        enabled: enabled === true
      }))
    : []
}
function readValues(source: string): Values | null {
  const values = { ...empty }
  if (!source.trim()) return values
  let root: Record<string, unknown>
  try {
    root = record(load(source)) ?? {}
  } catch {
    return null
  }
  const map: Record<string, TextKey> = {
    port: 'port',
    serverPort: 'serverPort',
    input: 'input',
    login: 'login',
    url: 'url',
    disabled_text_regular: 'disabledRegular',
    redirect_text_regular: 'redirectRegular',
    redirect_text_target: 'redirectTarget'
  }
  Object.entries(map).forEach(([yaml, field]) => (values[field] = text(root[yaml])))
  values.fullReceive = text(root.is_full_receive)
  values.masterID = accessItems(root.master_id)
  values.masterKey = accessItems(root.master_key)
  values.botID = accessItems(root.bot_id)
  values.botKey = accessItems(root.bot_key)
  values.disabledSelects = list(root.disabled_selects)
  values.disabledUserID = list(root.disabled_user_id)
  values.disabledUserKey = list(root.disabled_user_key)
  values.apps = accessItems(root.apps)
  values.autoPort = text(root.autoPort)
  const mappings = Array.isArray(root.mapping_text) ? root.mapping_text : []
  const mapping = record(mappings[0])
  values.mappingRegular = text(mapping?.regular)
  values.mappingTarget = text(mapping?.target)
  const processor = record(root.processor)
  values.repeatedEventTime = text(processor?.repeated_event_time)
  values.repeatedUserTime = text(processor?.repeated_user_time)
  const cbp = record(root.cbp)
  values.cbpTimeout = text(cbp?.timeout)
  values.cbpReconnect = text(cbp?.reconnectInterval)
  values.cbpHeartbeat = text(cbp?.heartbeatInterval)
  values.cbpHealthCheck = text(cbp?.healthCheckInterval)
  const headers = record(cbp?.headers)
  values.cbpUserAgent = text(headers?.['user-agent'])
  values.cbpDeviceID = text(headers?.['x-device-id'])
  values.cbpFullReceive = text(headers?.['x-full-receive'])
  return values
}
function toYaml(values: Values, includeMapping = true) {
  const lines: string[] = []
  const add = (key: string, value: string) => {
    if (value.trim()) lines.push(`${key}: ${quote(value.trim())}`)
  }
  add('port', values.port)
  add('serverPort', values.serverPort)
  add('input', values.input)
  add('login', values.login)
  add('url', values.url)
  if (values.fullReceive) lines.push(`is_full_receive: ${values.fullReceive}`)
  if (values.autoPort) lines.push(`autoPort: ${values.autoPort}`)
  lines.push(
    ...mapLines('master_id', values.masterID),
    ...mapLines('master_key', values.masterKey),
    ...mapLines('bot_id', values.botID),
    ...mapLines('bot_key', values.botKey)
  )
  add('disabled_text_regular', values.disabledRegular)
  lines.push(
    ...stringMapLines('disabled_selects', values.disabledSelects),
    ...stringMapLines('disabled_user_id', values.disabledUserID),
    ...stringMapLines('disabled_user_key', values.disabledUserKey)
  )
  add('redirect_text_regular', values.redirectRegular)
  add('redirect_text_target', values.redirectTarget)
  if (includeMapping && values.mappingRegular.trim() && values.mappingTarget.trim())
    lines.push(
      'mapping_text:',
      `  - regular: ${quote(values.mappingRegular.trim())}`,
      `    target: ${quote(values.mappingTarget.trim())}`
    )
  if (values.repeatedEventTime.trim() || values.repeatedUserTime.trim()) {
    lines.push('processor:')
    if (values.repeatedEventTime.trim())
      lines.push(`  repeated_event_time: ${values.repeatedEventTime.trim()}`)
    if (values.repeatedUserTime.trim())
      lines.push(`  repeated_user_time: ${values.repeatedUserTime.trim()}`)
  }
  lines.push(...mapLines('apps', values.apps))
  const cbp = [
    ['timeout', values.cbpTimeout],
    ['reconnectInterval', values.cbpReconnect],
    ['heartbeatInterval', values.cbpHeartbeat],
    ['healthCheckInterval', values.cbpHealthCheck]
  ] as const
  if (
    cbp.some(([, value]) => value.trim()) ||
    values.cbpUserAgent.trim() ||
    values.cbpDeviceID.trim() ||
    values.cbpFullReceive
  ) {
    lines.push('cbp:')
    cbp.forEach(([key, value]) => {
      if (value.trim()) lines.push(`  ${key}: ${value.trim()}`)
    })
    if (
      values.cbpUserAgent.trim() ||
      values.cbpDeviceID.trim() ||
      values.cbpFullReceive
    ) {
      lines.push('  headers:')
      if (values.cbpUserAgent.trim())
        lines.push(`    user-agent: ${quote(values.cbpUserAgent.trim())}`)
      if (values.cbpDeviceID.trim())
        lines.push(`    x-device-id: ${quote(values.cbpDeviceID.trim())}`)
      if (values.cbpFullReceive)
        lines.push(`    x-full-receive: ${quote(values.cbpFullReceive)}`)
    }
  }
  return lines.length ? `${lines.join('\n')}\n` : ''
}
function mergeConfig(existing: string, generated: string, preserved = new Set<string>()) {
  const normalized = /^\{\s*\}$/.test(existing.trim()) ? '' : existing
  const lines = normalized.replace(/\r/g, '').split('\n')
  const kept: string[] = []
  for (let index = 0; index < lines.length;) {
    const match = lines[index].match(/^([^ \t#][^:]*):/)
    if (!match || !managed.has(match[1]) || preserved.has(match[1])) {
      kept.push(lines[index++])
      continue
    }
    index++
    while (
      index < lines.length &&
      (/^\s/.test(lines[index]) || lines[index].trim() === '')
    )
      index++
  }
  while (kept.length && !kept[kept.length - 1].trim()) kept.pop()
  return `${kept.length && generated ? `${kept.join('\n')}\n\n` : kept.join('\n')}${generated}`
}

export function RobotConfigForm({ content, toolbar, onChange, extensionConfig }: Props) {
  const [values, setValues] = useStoreState<Values>(empty)
  const [advanced, setAdvanced] = useState(false)
  const [invalidConfig, setInvalidConfig] = useState(false)
  useEffect(() => {
    const next = readValues(content)
    setInvalidConfig(next === null)
    if (!next) return
    setValues(current => (sameValues(current, next) ? current : next))
  }, [content, setValues])
  // Redux's YAML draft is the shared editing source for both modes.
  const saveValues = (next: Values, changedKey?: keyof Values) => {
    if (invalidConfig) return
    const mappingChange =
      changedKey === 'mappingRegular' || changedKey === 'mappingTarget'
    const writeMapping = Boolean(
      mappingChange &&
        ((next.mappingRegular.trim() && next.mappingTarget.trim()) ||
          (!next.mappingRegular.trim() && !next.mappingTarget.trim()))
    )
    setValues(next)
    onChange(
      mergeConfig(
        content,
        toYaml(next, writeMapping),
        writeMapping ? undefined : new Set(['mapping_text'])
      )
    )
  }
  const set = (key: TextKey, value: string) =>
    saveValues({ ...values, [key]: value }, key)
  const setAccessItems = (key: ToggleKey, items: AccessItem[]) =>
    saveValues({ ...values, [key]: items }, key)
  const inputClass =
    'min-h-9 w-full rounded-md border border-slate-300 bg-white px-2.5 text-sm font-normal text-slate-800 outline-none transition focus:border-brand-600 focus:ring-2 focus:ring-brand-100'
  const labelClass = 'grid gap-1 text-xs font-semibold text-slate-600'
  const field = (key: TextKey, label: string, hint = '', title = '') => (
    <label className={labelClass} key={key} title={title || undefined}>
      {label}
      <input
        className={inputClass}
        disabled={invalidConfig}
        value={values[key]}
        onChange={event => set(key, event.target.value)}
        placeholder={hint}
      />
    </label>
  )
  const accessItemsField = (key: ToggleKey, label: string, hint: string) => {
    const items = values[key]
    const updateItem = (index: number, patch: Partial<AccessItem>) =>
      setAccessItems(
        key,
        items.map((item, current) =>
          current === index ? { ...item, ...patch } : item
        )
      )
    return (
      <section className="col-span-2 grid gap-2 rounded-lg border border-slate-200 bg-slate-50/50 p-2.5">
        <div className="flex items-center justify-between gap-2">
          <label className="text-xs font-semibold text-slate-600">{label}</label>
          <button
            type="button"
            className="secondary-button min-h-7 px-2 text-xs"
            disabled={invalidConfig}
            onClick={() => setAccessItems(key, [...items, { value: '', enabled: true }])}
          >
            + 新增项
          </button>
        </div>
        {items.length === 0 ? (
          <p className="m-0 text-xs text-slate-400">{hint}</p>
        ) : (
          <div className="grid gap-2">
            {items.map((item, index) => (
              <div className="flex items-center gap-2" key={`${key}-${index}`}>
                <input
                  className={inputClass}
                  disabled={invalidConfig}
                  value={item.value}
                  placeholder={hint}
                  onChange={event => updateItem(index, { value: event.target.value })}
                />
                <label className="flex shrink-0 items-center gap-1.5 text-xs font-medium text-slate-600">
                  <input
                    type="checkbox"
                    className="size-4 accent-brand-600"
                    disabled={invalidConfig}
                    checked={item.enabled}
                    onChange={event => updateItem(index, { enabled: event.target.checked })}
                  />
                  开启
                </label>
                <button
                  type="button"
                  className="secondary-button min-h-8 shrink-0 px-2 text-xs"
                  disabled={invalidConfig}
                  onClick={() =>
                    setAccessItems(
                      key,
                      items.filter((_, current) => current !== index)
                    )
                  }
                >
                  删除
                </button>
              </div>
            ))}
          </div>
        )}
      </section>
    )
  }
  const group = (
    name: string,
    note: string,
    children: ReactNode,
    open = false
  ) => (
    <details
      className="group overflow-hidden rounded-xl border border-slate-200"
      open={open}
    >
      <summary className="flex min-h-12 cursor-pointer list-none items-center gap-2 px-3 text-sm font-semibold text-slate-700 marker:content-none [&::-webkit-details-marker]:hidden">
        <strong>{name}</strong>
        <span className="ml-auto text-[11px] font-semibold text-slate-400">
          {note}
        </span>
        <i className="text-lg font-normal text-slate-400 transition-transform group-open:rotate-90">
          ›
        </i>
      </summary>
      <div className="robot-config-fields grid grid-cols-2 gap-3 border-t border-slate-100 p-3">
        {children}
      </div>
    </details>
  )
  const advancedSwitch = (
    <button
      type="button"
      className={cn(
        'secondary-button min-h-9 gap-1.5',
        advanced && 'border-brand-300 bg-brand-50 text-brand-700'
      )}
      onClick={() => setAdvanced(value => !value)}
      aria-pressed={advanced}
      title={advanced ? '收起高级参数' : '展开高级参数'}
    >
      <SlidersHorizontal className="size-3.5" />
      高级
      <span
        className={cn(
          'relative ml-0.5 inline-flex shrink-0 items-center rounded-full transition',
          advanced ? 'bg-brand-600' : 'bg-slate-300'
        )}
        style={{ boxSizing: 'border-box', height: 16, padding: 2, width: 28 }}
        aria-hidden="true"
      >
        <span
          className="block shrink-0 rounded-full bg-white shadow-sm transition-transform"
          style={{
            height: 12,
            transform: `translateX(${advanced ? 12 : 0}px)`,
            width: 12
          }}
        />
      </span>
    </button>
  )
  return (
    <RobotPanel
      className="robot-config-form max-w-190"
      icon={<Settings className="size-4" />}
      title="机器人配置"
      description="管理当前机器人的运行与连接参数"
      actions={
        <>
          {advancedSwitch}
          {toolbar}
        </>
      }
    >
      {invalidConfig && (
        <p className="m-0 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          配置包含无法解析的 YAML；为避免覆盖原内容，已暂停可视化编辑。请切换到文本模式修复后再继续。
        </p>
      )}
      {extensionConfig}
      {group(
        '常规运行',
        '常用',
        <>
          {field(
            'port',
            '运行端口',
            '17117',
            '机器人自身使用的端口；不确定时保持为空。'
          )}
          {field(
            'serverPort',
            '应用端口',
            '18110',
            '机器人应用对外提供服务的端口；不确定时保持为空。'
          )}
          <label
            className={labelClass}
            title="端口已被占用时，AlemonJS 会从配置端口开始依次尝试下一个可用端口；实际端口由运行期状态返回，不会写回配置文件。"
          >
            端口漂移
            <select
              className={inputClass}
              disabled={invalidConfig}
              value={values.autoPort}
              onChange={event => set('autoPort', event.target.value)}
            >
              <option value="">关闭</option>
              <option value="true">开启：冲突时自动尝试下一端口</option>
            </select>
          </label>
          {field(
            'input',
            '应用入口',
            'lib/index.js',
            '机器人启动时加载的文件。'
          )}
          {field(
            'login',
            '登录连接',
            '如 discord',
            '推荐在“运行”页直接选择已安装的平台。'
          )}
        </>
      )}
      {advanced &&
        group(
        'CBP 运行',
        '常用',
        <>
          {field(
            'url',
            'CBP 地址',
            'ws://127.0.0.1:17117',
            '机器人连接到 CBP 服务的地址。'
          )}
          <label
            className={labelClass}
            title="是否接收所有 CBP 事件；不确定时保持不设置。"
          >
            全量接收
            <select
              className={inputClass}
              disabled={invalidConfig}
              value={values.fullReceive}
              onChange={event => set('fullReceive', event.target.value)}
            >
              <option value="">不设置</option>
              <option value="true">开启</option>
              <option value="false">关闭</option>
            </select>
          </label>
        </>
        )}
      {group(
        '身份与权限',
        '按需',
        <>
          {accessItemsField('masterID', '主人 ID', '例如 123456')}
          {accessItemsField('masterKey', '主人 Key', '例如 master-key')}
          {accessItemsField('botID', '机器人 ID', '例如 987654')}
          {accessItemsField('botKey', '机器人 Key', '例如 bot-key')}
        </>
      )}
      {advanced &&
        group(
        '消息规则',
        '高级',
        <>
          {field('disabledRegular', '禁用文本正则', '/闭关')}
          {field(
            'disabledSelects',
            '禁用事件',
            'message.create, private.message.create'
          )}
          {field('disabledUserID', '禁用用户 ID', '多个用逗号分隔')}
          {field('disabledUserKey', '禁用用户 Key', '多个用逗号分隔')}
          {field('redirectRegular', '重定向正则', '^#')}
          {field('redirectTarget', '重定向目标', '/')}
          {field('mappingRegular', '映射匹配文本', '/帮助')}
          {field('mappingTarget', '映射替换文本', '/help')}
        </>
        )}
      {advanced &&
        group(
        '运行与模块',
        '高级',
        <>
          {field('repeatedEventTime', '重复事件窗口（毫秒）', '60000')}
          {field('repeatedUserTime', '重复用户窗口（毫秒）', '1000')}
          {accessItemsField('apps', '启用模块', '例如 alemonjs-openai')}
        </>
        )}
      {advanced &&
        group(
        'CBP 高级参数',
        '高级',
        <>
          {field('cbpTimeout', '连接超时（毫秒）', '180000')}
          {field('cbpReconnect', '重连间隔（毫秒）', '6000')}
          {field('cbpHeartbeat', '连接心跳（毫秒）', '18000')}
          {field('cbpHealthCheck', '连接健康检查（毫秒）', '30000')}
          {field('cbpUserAgent', 'User-Agent', 'platform')}
          {field('cbpDeviceID', '设备 ID', 'auto-generated')}
          <label className={labelClass}>
            CBP 全量接收
            <select
              className={inputClass}
              disabled={invalidConfig}
              value={values.cbpFullReceive}
              onChange={event => set('cbpFullReceive', event.target.value)}
            >
              <option value="">不设置</option>
              <option value="1">开启</option>
              <option value="0">关闭</option>
            </select>
          </label>
        </>
        )}
    </RobotPanel>
  )
}
