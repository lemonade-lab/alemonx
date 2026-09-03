export type ForegroundLogMode = 'simple' | 'common' | 'advanced'
export type ForegroundLogLevel = 'success' | 'warning' | 'error' | 'info'
export type ForegroundLogDomain =
  | 'dependency'
  | 'port'
  | 'network'
  | 'config'
  | 'environment'
  | 'runtime'
  | 'login'
  | 'service'
  | 'plugin'
  | 'unknown'
export type ForegroundLogAction =
  | 'install-dependencies'
  | 'open-runtime'
  | 'open-config'
  | 'open-environment'
  | 'open-service'
  | 'copy-service-url'
  | 'manage-master'
  | null
export type ForegroundLogFilter = {
  query: string
  levels: ForegroundLogLevel[]
  domains: ForegroundLogDomain[]
}
export type ForegroundLogItem = {
  id: string
  index: number
  raw: string
  safeRaw: string
  text: string
  title: string
  timeLabel: string
  time: string | null
  level: ForegroundLogLevel
  domain: ForegroundLogDomain
  action: ForegroundLogAction
  serviceURL: string | null
  masterUserID: string | null
  details: Array<{ label: string; value: string }>
  count: number
  occurrenceIndexes: number[]
}

const MAX_CONTENT_CHARS = 48 * 1024
const MAX_ITEMS = 400
// eslint-disable-next-line no-control-regex -- terminal output can include ANSI CSI sequences.
const ANSI_PATTERN = /\x1b\[[0-?]*[ -/]*[@-~]/g
const TIMESTAMP_PATTERN =
  /^\[(?:\d{4}-)?(\d{2}-\d{2}\s+)?(\d{2}:\d{2}:\d{2})(?:\.\d{1,3})?\](?:\[(?:TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\])?\s*(.*)$/i
const STREAM_PATTERN = /^\[(?:stdout|stderr)\]\s*/i
const LEVEL_PATTERN = /^\[(?:TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\]\s*/i
const DEPENDENCY_PATTERN =
  /(Cannot find (?:module|package)|MODULE_NOT_FOUND|ERR_MODULE_NOT_FOUND|缺少依赖|依赖(?:未安装|缺失)|ERESOLVE|EINTEGRITY)/i
const PORT_PATTERN =
  /(EADDRINUSE|address already in use|listen\s+EADDRINUSE|端口.{0,12}(?:占用|被占用))/i
const NETWORK_PATTERN =
  /(ECONNREFUSED|ECONNRESET|ETIMEDOUT|ENOTFOUND|EAI_AGAIN|network timeout|连接超时|连接被拒绝|网络异常|域名解析失败)/i
const CONFIG_PATTERN =
  /(YAMLException|JSON\.parse|配置(?:错误|无效|缺失)|alemon\.config|invalid config)/i
const ENVIRONMENT_PATTERN =
  /(EACCES|EPERM|ENOENT|ENOSPC|permission denied|no space left|内存不足|heap out of memory|磁盘空间不足|权限不足)/i
const RUNTIME_PATTERN =
  /(SyntaxError|TypeError|ReferenceError|UnhandledPromiseRejection|uncaughtException|进程(?:已)?退出|启动失败|异常退出|崩溃|Killed|Aborted)/i
const LOGIN_PATTERN = /(二维码|扫码|qrcode|登录成功|login success|连接成功)/i
const SUCCESS_PATTERN =
  /(启动成功|已启动|就绪|connected|listening on|server listening|应用服务器:|Worker 就绪)/i
const WARNING_PATTERN = /(\bWARN(?:ING)?\b|警告|重试)/i
const ERROR_PATTERN =
  /(\bERROR\b|\bFATAL\b|异常|失败|崩溃|uncaught|SyntaxError|TypeError)/i
const NOISE_PATTERN =
  /^(?:\s*at\s+|node:internal|file:\/\/|\{|\}|\[stdout\]\s+at\s+|\[stderr\]\s+at\s+|[A-Za-z_$][\w$]*:\s*(?:\{|\[|'.*'|".*"|\d+|true|false|null|undefined),?$)/i
const URL_PATTERN = /https?:\/\/[^\s'"<>]+/gi
const BARE_LOCAL_SERVICE_PATTERN =
  /(?:localhost|127\.0\.0\.1|0\.0\.0\.0|(?:10|192\.168)\.\d{1,3}\.\d{1,3}|172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}):\d{2,5}(?:\/[^\s'"<>]*)?/i
const MASTER_COMMAND_PATTERN = /\[MessageText:[^\]\r\n]*\/我是主人[^\]\r\n]*\]/
const MESSAGE_USER_ID_PATTERN = /\[UserId:([^\]\r\n]+)\]/i
const STRUCTURED_FIELD_PATTERN =
  /(?:^\s*|[,\n]\s*)(?:'([A-Za-z_][\w-]*)'|"([A-Za-z_][\w-]*)"|([A-Za-z_][\w-]*))\s*:\s*(?:'([^']*)'|"([^"]*)"|([^,}\n]+))/g
const STRUCTURED_LABELS: Record<string, string> = {
  message: '信息',
  runtime_mode: '运行模式',
  transport: '传输方式',
  gscore_url: '服务地址',
  service_url: '服务地址',
  server_url: '服务地址',
  url: '服务地址',
  endpoint: '服务地址',
  port: '端口'
}

export const foregroundLogDomains: Array<{
  value: ForegroundLogDomain
  label: string
}> = [
  { value: 'dependency', label: '依赖' },
  { value: 'port', label: '端口' },
  { value: 'network', label: '网络' },
  { value: 'config', label: '配置' },
  { value: 'environment', label: '环境' },
  { value: 'runtime', label: '运行' },
  { value: 'login', label: '登录' },
  { value: 'service', label: '服务' },
  { value: 'plugin', label: '模块' },
  { value: 'unknown', label: '其他' }
]

export function redactForegroundLog(value: string) {
  return value
    .replace(/(Bearer\s+)[A-Za-z0-9._~+/-]+=*/gi, '$1[已隐藏]')
    .replace(
      /((?:token|password|passwd|secret|cookie|authorization|api[_-]?key)\s*[=:]\s*['"]?)([^\s,'"}]+)/gi,
      '$1[已隐藏]'
    )
    .replace(
      /([?&](?:token|code|ticket|access_token|authorization)=)[^&#\s]+/gi,
      '$1[已隐藏]'
    )
    .replace(
      /((?:qrcode|二维码)(?:_url|Url)?\s*[=:]\s*['"]?)([^\s,'"}]+)/gi,
      '$1[已隐藏]'
    )
}

function trimContent(value: string) {
  if (value.length <= MAX_CONTENT_CHARS) return value
  const start = value.indexOf('\n', value.length - MAX_CONTENT_CHARS)
  return start < 0 ? value.slice(-MAX_CONTENT_CHARS) : value.slice(start + 1)
}
function stripLogPrefix(value: string) {
  const clean = value.replace(ANSI_PATTERN, '').trim()
  const matched = clean.match(TIMESTAMP_PATTERN)
  return (matched ? (matched[3] ?? '') : clean)
    .replace(STREAM_PATTERN, '')
    .replace(LEVEL_PATTERN, '')
    .trim()
}
function timeLabel(value: string) {
  const clean = value.replace(ANSI_PATTERN, '').trim()
  if (!clean.match(TIMESTAMP_PATTERN)) return '本次运行'
  return (
    clean.match(
      /^\[(\d{4}-\d{2}-\d{2}|\d{2}-\d{2})\s+\d{2}:\d{2}:\d{2}/
    )?.[1] || '本次运行'
  )
}
function clockTime(value: string) {
  return (
    value.replace(ANSI_PATTERN, '').trim().match(TIMESTAMP_PATTERN)?.[2] ?? null
  )
}
function isBlockStart(value: string) {
  const clean = value.replace(ANSI_PATTERN, '').trim()
  if (stripLogPrefix(clean).startsWith('{')) return true
  if (!clean || NOISE_PATTERN.test(clean)) return false
  return (
    TIMESTAMP_PATTERN.test(clean) ||
    STREAM_PATTERN.test(clean) ||
    LEVEL_PATTERN.test(clean) ||
    /(?:AxiosError|Error|TypeError|SyntaxError|ReferenceError):\s+/i.test(clean)
  )
}
function buildBlocks(content: string) {
  const blocks: string[] = []
  let current = ''
  for (const line of trimContent(content).split(/\r?\n/)) {
    if (!line.trim()) continue
    if (!current || isBlockStart(line)) {
      if (current) blocks.push(current)
      current = line
    } else current += `\n${line}`
  }
  if (current) blocks.push(current)
  return blocks
}
function sanitizeSimpleText(value: string) {
  return stripLogPrefix(value)
    .replace(/\/Users\/[^/\s]+\//g, '~/')
    .replace(/\/root\/[^/\s]+\//g, '~/')
    .replace(/\/data\/user\/0\/[^/\s]+\//g, '~/')
    .replace(/file:\/\/[^\s'"<>]+/g, '本地文件')
    .replace(/\s+/g, ' ')
    .trim()
}
function serviceURL(raw: string) {
  const url = raw.match(URL_PATTERN)?.[0] ?? null
  URL_PATTERN.lastIndex = 0
  if (url) return url
  const local = raw.match(BARE_LOCAL_SERVICE_PATTERN)?.[0] ?? null
  return local ? `http://${local}` : null
}
function localService(url: string | null) {
  if (!url) return false
  try {
    const host = new URL(url).hostname
    return (
      host === 'localhost' ||
      host === '127.0.0.1' ||
      host === '0.0.0.0' ||
      host === '::1' ||
      /^10\./.test(host) ||
      /^192\.168\./.test(host) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(host)
    )
  } catch {
    return false
  }
}
function masterCommandUserID(raw: string) {
  if (!MASTER_COMMAND_PATTERN.test(raw)) return null
  const userID = raw.match(MESSAGE_USER_ID_PATTERN)?.[1]?.trim() ?? ''
  return /^[A-Za-z0-9._:-]{1,160}$/.test(userID) ? userID : null
}
function labelForKey(key: string) {
  return (
    STRUCTURED_LABELS[key] ??
    (/(?:^|_)url$|endpoint$/i.test(key) ? '服务地址' : '')
  )
}
function structuredDetails(raw: string) {
  const normalized = stripLogPrefix(raw),
    start = normalized.indexOf('{'),
    end = normalized.lastIndexOf('}')
  const body = start >= 0 && end > start ? normalized.slice(start, end + 1) : ''
  if (!body.startsWith('{') || !body.endsWith('}')) return []
  const details: Array<{ label: string; value: string }> = []
  try {
    const parsed = JSON.parse(body) as Record<string, unknown>
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      for (const [key, value] of Object.entries(parsed)) {
        const label = labelForKey(key)
        if (label && value !== null && typeof value !== 'object')
          details.push({ label, value: String(value) })
      }
      if (details.length) return details
    }
  } catch {
    /* Node inspect syntax falls through to the tolerant extractor. */
  }
  STRUCTURED_FIELD_PATTERN.lastIndex = 0
  for (const match of body.slice(1, -1).matchAll(STRUCTURED_FIELD_PATTERN)) {
    const key = match[1] ?? match[2] ?? match[3] ?? '',
      value = (match[4] ?? match[5] ?? match[6] ?? '').trim(),
      label = labelForKey(key)
    if (label && value) details.push({ label, value })
  }
  return details
}
function structuredSummary(details: Array<{ label: string; value: string }>) {
  const message = details.find(item => item.label === '信息')?.value
  const summary = details.filter(
    item => item.label !== '信息' && item.label !== '服务地址'
  )
  return summary.length
    ? summary.map(item => `${item.label}：${item.value}`).join(' · ')
    : (message ??
        details.map(item => `${item.label}：${item.value}`).join(' · '))
}
function classify(raw: string, index: number): ForegroundLogItem {
  const safeRaw = redactForegroundLog(raw),
    firstLine = safeRaw.split(/\r?\n/, 1)[0] ?? safeRaw
  const url = serviceURL(raw),
    details = structuredDetails(safeRaw),
    masterUserID = masterCommandUserID(raw)
  let domain: ForegroundLogDomain = 'unknown',
    level: ForegroundLogLevel = 'info',
    title = '运行输出',
    action: ForegroundLogAction = null
  if (details.length) {
    domain = details.some(item => item.label === '服务地址')
      ? 'service'
      : 'runtime'
    title = details.find(item => item.label === '信息')?.value || '运行信息'
    if (ERROR_PATTERN.test(safeRaw)) level = 'error'
    else if (WARNING_PATTERN.test(safeRaw)) level = 'warning'
    else if (SUCCESS_PATTERN.test(safeRaw)) level = 'success'
    if (url) action = localService(url) ? 'open-service' : 'copy-service-url'
  } else if (masterUserID) {
    domain = 'login'
    title = '主人权限请求'
    action = 'manage-master'
  } else if (DEPENDENCY_PATTERN.test(safeRaw)) {
    domain = 'dependency'
    level = 'error'
    title = '缺少运行依赖'
    action = 'install-dependencies'
  } else if (PORT_PATTERN.test(safeRaw)) {
    domain = 'port'
    level = 'error'
    title = '端口已被占用'
    action = 'open-runtime'
  } else if (NETWORK_PATTERN.test(safeRaw)) {
    domain = 'network'
    level = 'warning'
    title = '网络连接异常'
    action = 'open-environment'
  } else if (CONFIG_PATTERN.test(safeRaw)) {
    domain = 'config'
    level = 'error'
    title = '配置需要检查'
    action = 'open-config'
  } else if (ENVIRONMENT_PATTERN.test(safeRaw)) {
    domain = 'environment'
    level = 'warning'
    title = '运行环境需要检查'
    action = 'open-environment'
  } else if (LOGIN_PATTERN.test(safeRaw)) {
    domain = 'login'
    level = /成功|success|连接成功/i.test(safeRaw) ? 'success' : 'info'
    title = /二维码|扫码|qrcode/i.test(safeRaw)
      ? '等待扫码登录'
      : '登录连接状态'
  } else if (url) {
    domain = 'service'
    level = 'success'
    title = '服务地址已就绪'
    action = localService(url) ? 'open-service' : 'copy-service-url'
  } else if (RUNTIME_PATTERN.test(safeRaw)) {
    domain = 'runtime'
    level = 'error'
    title = '运行进程异常'
    action = 'open-environment'
  } else if (SUCCESS_PATTERN.test(safeRaw)) {
    level = 'success'
    title = '运行正常'
  } else if (ERROR_PATTERN.test(safeRaw)) {
    level = 'error'
    title = '运行异常'
  } else if (WARNING_PATTERN.test(safeRaw)) {
    level = 'warning'
    title = '需要留意'
  } else if (/插件|plugin|模块|module/i.test(safeRaw)) domain = 'plugin'
  return {
    id: `${index}:${safeRaw.slice(0, 80)}`,
    index,
    raw,
    safeRaw,
    text:
      structuredSummary(details) ||
      sanitizeSimpleText(firstLine) ||
      stripLogPrefix(firstLine) ||
      '运行输出',
    title,
    timeLabel: timeLabel(firstLine),
    time: clockTime(firstLine),
    level,
    domain,
    action,
    serviceURL: url,
    masterUserID,
    details,
    count: 1,
    occurrenceIndexes: [index]
  }
}

export function normalizeForegroundLog(content: string) {
  return buildBlocks(content).map(classify).slice(-MAX_ITEMS)
}
export function filterForegroundLog(
  items: ForegroundLogItem[],
  filter: ForegroundLogFilter
) {
  const query = filter.query.trim().toLocaleLowerCase()
  return items.filter(item => {
    if (filter.levels.length && !filter.levels.includes(item.level))
      return false
    if (filter.domains.length && !filter.domains.includes(item.domain))
      return false
    return (
      !query ||
      [
        item.title,
        item.text,
        item.safeRaw,
        ...item.details.map(detail => `${detail.label} ${detail.value}`)
      ]
        .join('\n')
        .toLocaleLowerCase()
        .includes(query)
    )
  })
}
export function collapseForegroundLog(items: ForegroundLogItem[]) {
  const collapsed: ForegroundLogItem[] = []
  for (const item of items) {
    const previous = collapsed[collapsed.length - 1]
    if (
      previous &&
      stripLogPrefix(previous.safeRaw) === stripLogPrefix(item.safeRaw)
    ) {
      previous.count += 1
      previous.occurrenceIndexes.push(item.index)
    } else
      collapsed.push({
        ...item,
        occurrenceIndexes: [...item.occurrenceIndexes]
      })
  }
  return collapsed
}
export function interpretForegroundLog(
  content: string,
  mode: ForegroundLogMode
) {
  const items = normalizeForegroundLog(content).filter(
    item =>
      mode === 'advanced' ||
      mode === 'common' ||
      item.level !== 'info' ||
      item.domain === 'login' ||
      item.domain === 'service' ||
      item.action !== null ||
      item.details.length > 0
  )
  return mode === 'advanced' ? items : collapseForegroundLog(items)
}
export function foregroundLogSummary(items: ForegroundLogItem[]) {
  return {
    errors: items
      .filter(item => item.level === 'error')
      .reduce((total, item) => total + item.count, 0),
    warnings: items
      .filter(item => item.level === 'warning')
      .reduce((total, item) => total + item.count, 0)
  }
}
export function foregroundLogDisplayText(
  item: ForegroundLogItem,
  mode: ForegroundLogMode
) {
  if (mode === 'advanced') return item.safeRaw.replace(ANSI_PATTERN, '')
  return mode === 'common' &&
    (item.level === 'error' || item.level === 'warning')
    ? stripLogPrefix(item.safeRaw)
    : item.text
}
