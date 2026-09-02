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

export type ForegroundLogItem = {
  id: string
  raw: string
  text: string
  title: string
  timeLabel: string | null
  level: ForegroundLogLevel
  domain: ForegroundLogDomain
  action: ForegroundLogAction
  serviceURL: string | null
  masterUserID: string | null
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

function trimContent(value: string) {
  if (value.length <= MAX_CONTENT_CHARS) return value
  const start = value.indexOf('\n', value.length - MAX_CONTENT_CHARS)
  return start < 0 ? value.slice(-MAX_CONTENT_CHARS) : value.slice(start + 1)
}

function stripLogPrefix(value: string) {
  const clean = value.replace(ANSI_PATTERN, '').trim()
  const matched = clean.match(TIMESTAMP_PATTERN)
  const body = matched ? matched[3] ?? '' : clean
  return body.replace(STREAM_PATTERN, '').replace(LEVEL_PATTERN, '').trim()
}

function timeLabel(value: string) {
  return value.replace(ANSI_PATTERN, '').trim().match(TIMESTAMP_PATTERN)?.[2] ?? null
}

function isBlockStart(value: string) {
  const clean = value.replace(ANSI_PATTERN, '').trim()
  if (!clean || NOISE_PATTERN.test(clean)) return false
  return (
    TIMESTAMP_PATTERN.test(clean) ||
    STREAM_PATTERN.test(clean) ||
    LEVEL_PATTERN.test(clean) ||
    /(?:AxiosError|Error|TypeError|SyntaxError|ReferenceError):\s+/i.test(
      clean
    )
  )
}

function buildBlocks(content: string) {
  const blocks: string[] = []
  let current = ''
  for (const rawLine of trimContent(content).split(/\r?\n/)) {
    if (!rawLine.trim()) continue
    if (!current || isBlockStart(rawLine)) {
      if (current) blocks.push(current)
      current = rawLine
    } else {
      current += `\n${rawLine}`
    }
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
    const hostname = new URL(url).hostname
    return (
      hostname === 'localhost' ||
      hostname === '127.0.0.1' ||
      hostname === '0.0.0.0' ||
      hostname === '::1' ||
      /^10\./.test(hostname) ||
      /^192\.168\./.test(hostname) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(hostname)
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

function classify(raw: string, index: number): ForegroundLogItem {
  const firstLine = raw.split(/\r?\n/, 1)[0] ?? raw
  const text = sanitizeSimpleText(firstLine)
  const url = serviceURL(raw)
  const common = stripLogPrefix(firstLine)
  const masterUserID = masterCommandUserID(raw)
  let domain: ForegroundLogDomain = 'unknown'
  let level: ForegroundLogLevel = 'info'
  let title = '运行输出'
  let action: ForegroundLogAction = null

  if (masterUserID) {
    domain = 'login'
    title = '主人权限请求'
    action = 'manage-master'
  } else if (DEPENDENCY_PATTERN.test(raw)) {
    domain = 'dependency'
    level = 'error'
    title = '缺少运行依赖'
    action = 'install-dependencies'
  } else if (PORT_PATTERN.test(raw)) {
    domain = 'port'
    level = 'error'
    title = '端口已被占用'
    action = 'open-runtime'
  } else if (NETWORK_PATTERN.test(raw)) {
    domain = 'network'
    level = 'warning'
    title = '网络连接异常'
    action = 'open-environment'
  } else if (CONFIG_PATTERN.test(raw)) {
    domain = 'config'
    level = 'error'
    title = '配置需要检查'
    action = 'open-config'
  } else if (ENVIRONMENT_PATTERN.test(raw)) {
    domain = 'environment'
    level = 'warning'
    title = '运行环境需要检查'
    action = 'open-environment'
  } else if (LOGIN_PATTERN.test(raw)) {
    domain = 'login'
    level = /成功|success|连接成功/i.test(raw) ? 'success' : 'info'
    title = /二维码|扫码|qrcode/i.test(raw) ? '等待扫码登录' : '登录连接状态'
  } else if (url) {
    domain = 'service'
    level = 'success'
    title = '服务地址已就绪'
    action = localService(url) ? 'open-service' : 'copy-service-url'
  } else if (RUNTIME_PATTERN.test(raw)) {
    domain = 'runtime'
    level = 'error'
    title = '运行进程异常'
    action = 'open-environment'
  } else if (SUCCESS_PATTERN.test(raw)) {
    level = 'success'
    title = '运行正常'
  } else if (ERROR_PATTERN.test(raw)) {
    level = 'error'
    title = '运行异常'
  } else if (WARNING_PATTERN.test(raw)) {
    level = 'warning'
    title = '需要留意'
  } else if (/插件|plugin|模块|module/i.test(raw)) {
    domain = 'plugin'
  }

  return {
    id: `${index}:${raw.slice(0, 80)}`,
    raw,
    text: text || common || '运行输出',
    title,
    timeLabel: timeLabel(firstLine),
    level,
    domain,
    action,
    serviceURL: url,
    masterUserID
  }
}

export function interpretForegroundLog(
  content: string,
  mode: ForegroundLogMode,
  attentionOnly = false
) {
  const items = buildBlocks(content).map(classify)
  const visible = items.filter(item => {
    if (attentionOnly && item.level !== 'error' && item.level !== 'warning')
      return false
    if (mode === 'advanced') return true
    if (mode === 'common') return !NOISE_PATTERN.test(item.raw)
    return (
      item.level !== 'info' ||
      item.domain === 'login' ||
      item.domain === 'service' ||
      item.action !== null
    )
  })
  return visible.slice(-MAX_ITEMS)
}

export function foregroundLogSummary(items: ForegroundLogItem[]) {
  return {
    errors: items.filter(item => item.level === 'error').length,
    warnings: items.filter(item => item.level === 'warning').length
  }
}

export function foregroundLogDisplayText(
  item: ForegroundLogItem,
  mode: ForegroundLogMode
) {
  if (mode === 'advanced') return item.raw.replace(ANSI_PATTERN, '')
  if (mode === 'common') return stripLogPrefix(item.raw)
  return item.text
}
