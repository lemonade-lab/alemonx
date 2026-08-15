// 服务端持久化：测试中心（testone）的聊天记录与图片由工作台后端写入
// SQLite/文件系统，localStorage 只作为离线缓存。所有请求失败时由调用方
// 静默降级，不阻断沙盒功能。

let currentRoot: string | null = null

// testone 是临时测试环境：记录只保留 7 天，服务端启动/写入时自动清理
// 过期会话与图片，本机缓存同样按该窗口视为过期，不重新导入。
export const TESTONE_RETENTION_DAYS = 7
export const TESTONE_RETENTION_MS = TESTONE_RETENTION_DAYS * 24 * 60 * 60 * 1000

export function setTestoneRoot(root: string | null) {
  currentRoot = root
}

export function getTestoneRoot(): string | null {
  return currentRoot
}

export type TestoneChatOptions = {
  host: string
  port: number
  type: 'public' | 'private'
  chatId: string
}

export type TestoneChatRecord = {
  chatKey: string
  payload: unknown
  updatedAt: number
}

export type TestoneRecordSummary = {
  root: string
  chats: number
  images: number
  bytes: number
}

export function testoneChatKey(opts: TestoneChatOptions): string {
  return `${opts.host}:${opts.port}:${opts.type}:${opts.chatId}`
}

async function readJSON<T>(response: Response): Promise<T> {
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  return (await response.json()) as T
}

export async function loadChatRecord(
  opts: TestoneChatOptions
): Promise<TestoneChatRecord | null> {
  const root = getTestoneRoot()
  if (!root) return null
  const query = new URLSearchParams({ root, chatKey: testoneChatKey(opts) })
  const data = await readJSON<{ chat: TestoneChatRecord | null }>(
    await fetch(`/api/v1/robot/testone/chat?${query.toString()}`)
  )
  return data.chat
}

export async function saveChatRecord(
  opts: TestoneChatOptions,
  payload: unknown
): Promise<void> {
  const root = getTestoneRoot()
  if (!root) return
  await readJSON<{ ok: boolean }>(
    await fetch('/api/v1/robot/testone/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ root, chatKey: testoneChatKey(opts), payload })
    })
  )
}

export async function deleteChatRecord(
  opts: TestoneChatOptions
): Promise<void> {
  const root = getTestoneRoot()
  if (!root) return
  const query = new URLSearchParams({ root, chatKey: testoneChatKey(opts) })
  await readJSON<{ ok: boolean }>(
    await fetch(`/api/v1/robot/testone/chat?${query.toString()}`, {
      method: 'DELETE'
    })
  )
}

export async function uploadImage(hash: string, blob: Blob): Promise<void> {
  const root = getTestoneRoot()
  if (!root) return
  const form = new FormData()
  form.append('root', root)
  form.append('hash', hash)
  form.append('file', blob, `${hash}.img`)
  await readJSON<{ ok: boolean }>(
    await fetch('/api/v1/robot/testone/image', {
      method: 'POST',
      body: form
    })
  )
}

export async function fetchImageBlob(hash: string): Promise<Blob | null> {
  const root = getTestoneRoot()
  if (!root) return null
  const query = new URLSearchParams({ root, hash })
  const response = await fetch(`/api/v1/robot/testone/image?${query.toString()}`)
  if (!response.ok) return null
  return response.blob()
}

export async function fetchSummary(): Promise<TestoneRecordSummary[]> {
  const data = await readJSON<{ items: TestoneRecordSummary[] }>(
    await fetch('/api/v1/robot/testone/summary')
  )
  return data.items
}

export async function clearServerRecords(root?: string): Promise<void> {
  const query = root
    ? new URLSearchParams({ root }).toString()
    : new URLSearchParams().toString()
  await readJSON<{ ok: boolean }>(
    await fetch(`/api/v1/robot/testone/chat?${query}`, { method: 'DELETE' })
  )
}
