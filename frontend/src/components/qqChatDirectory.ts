import type { QQContact, QQSpace } from './qqChatStorage'

export type QQActionResult = { code?: number; message?: string; data?: unknown }

const identityContainerKeys = [
  'data',
  'result',
  'results',
  'payload',
  'body',
  'response',
  'group_info',
  'groupInfo',
  'group',
  'bot',
  'user',
  'profile',
  'guild',
  'channel',
  'items',
  'item',
  'list'
]

export function resultItems(data: unknown): Record<string, unknown>[] {
  if (Array.isArray(data))
    return data.filter(item => item && typeof item === 'object') as Record<
      string,
      unknown
    >[]
  if (!data || typeof data !== 'object') return []
  const record = data as Record<string, unknown>
  for (const key of ['data', 'items', 'list', 'members', 'guilds'])
    if (Array.isArray(record[key]))
      return record[key].filter(
        item => item && typeof item === 'object'
      ) as Record<string, unknown>[]
  return [record]
}

export function recordText(record: Record<string, unknown>, keys: string[]) {
  const text = (value: unknown) => {
    if (typeof value === 'string' && value.trim()) return value
    if (typeof value === 'number' && Number.isFinite(value))
      return String(value)
    return ''
  }
  for (const key of keys) {
    const value = text(record[key])
    if (value) return value
  }
  const normalize = (key: string) => key.toLowerCase().replace(/[_-]/g, '')
  const lowercase = new Map(
    Object.entries(record).map(([key, value]) => [normalize(key), value])
  )
  for (const key of keys) {
    const value = text(lowercase.get(normalize(key)))
    if (value) return value
  }
  return ''
}

function parsedJSON(value: unknown) {
  if (typeof value !== 'string') return value
  const text = value.trim()
  if (!text || !/^[{[]/.test(text)) return value
  try {
    return JSON.parse(text) as unknown
  } catch {
    return value
  }
}

export function findIdentityRecord(
  data: unknown,
  preferredKeys: string[],
  maxDepth = 8,
  maxNodes = 240
) {
  const queue: Array<{ value: unknown; depth: number }> = [
    { value: data, depth: 0 }
  ]
  const seen = new Set<object>()
  let fallback: Record<string, unknown> | undefined
  let visited = 0
  while (queue.length && visited < maxNodes) {
    const next = queue.shift()
    if (!next || next.depth > maxDepth) continue
    const value = parsedJSON(next.value)
    if (!value || typeof value !== 'object') continue
    if (seen.has(value)) continue
    seen.add(value)
    visited += 1
    if (Array.isArray(value)) {
      for (const item of value)
        queue.push({ value: item, depth: next.depth + 1 })
      continue
    }
    const record = value as Record<string, unknown>
    fallback ||= record
    if (preferredKeys.some(key => recordText(record, [key]))) return record
    const prioritized = new Set(identityContainerKeys)
    for (const key of identityContainerKeys)
      if (key in record)
        queue.push({ value: record[key], depth: next.depth + 1 })
    for (const [key, child] of Object.entries(record))
      if (!prioritized.has(key))
        queue.push({ value: child, depth: next.depth + 1 })
  }
  return fallback
}

const groupNameKeys = ['group_name', 'groupName', 'name', 'title']
const groupNumberKeys = [
  'group_num',
  'group_number',
  'group_code',
  'group_uin',
  'raw_group_id',
  'group_id'
]
const groupAvatarKeys = [
  'group_avatar',
  'group_avatar_url',
  'group_icon',
  'group_icon_url'
]

export function extractQQGroupIdentity(data: unknown) {
  const record = findIdentityRecord(data, [
    ...groupNumberKeys,
    'group_name',
    'groupName',
    ...groupAvatarKeys
  ])
  if (!record) return {}
  const groupNumber = recordText(record, groupNumberKeys)
  return {
    name: recordText(record, groupNameKeys),
    groupNumber,
    avatar:
      (/^\d{5,}$/.test(groupNumber)
        ? `https://p.qlogo.cn/gh/${groupNumber}/${groupNumber}/100`
        : '') || qqGroupAvatarSource(recordText(record, groupAvatarKeys))
  }
}

export function qqGroupAvatarSource(value: unknown) {
  if (typeof value !== 'string') return ''
  const source = value.trim()
  if (!source) return ''
  return /^https?:\/\/p\.qlogo\.cn\/gh\/\d+\/\d+\/\d+(?:[/?#]|$)/i.test(source)
    ? source
    : ''
}

export function qqConversationAvatarSource(
  scope: string,
  conversationAvatar?: unknown,
  channelAvatar?: unknown,
  userAvatar?: unknown
) {
  const candidates =
    scope === 'group'
      ? [conversationAvatar, channelAvatar]
      : [conversationAvatar, channelAvatar, userAvatar]
  for (const candidate of candidates) {
    const source =
      scope === 'group'
        ? qqGroupAvatarSource(candidate)
        : typeof candidate === 'string'
          ? candidate.trim()
          : ''
    if (source) return source
  }
  return ''
}

export function collectActionDirectory(
  action: string,
  data: unknown,
  now = Date.now()
): { contacts: QQContact[]; spaces: QQSpace[] } {
  const contacts: QQContact[] = []
  const spaces: QQSpace[] = []
  for (const item of resultItems(data)) {
    if (action === 'member.list') {
      const userID = recordText(item, [
        'user_id',
        'id',
        'userId',
        'member_openid',
        'openid'
      ])
      if (!userID) continue
      contacts.push({
        id: `user:${userID}`,
        label:
          recordText(item, ['nick', 'username', 'name', 'user_name']) || userID,
        avatar: recordText(item, ['avatar', 'avatar_url']),
        source: 'member',
        lastInteractionAt: now
      })
      continue
    }
    if (
      action === 'me.guilds' ||
      action === 'guild.list' ||
      action === 'channel.list'
    ) {
      const id = recordText(item, [
        'id',
        'guild_id',
        'channel_id',
        'guildId',
        'channelId'
      ])
      if (!id) continue
      spaces.push({
        id: `channel:channel:${id}`,
        label: recordText(item, ['name', 'guild_name', 'channel_name']) || id,
        avatar: recordText(item, [
          'avatar',
          'avatar_url',
          'icon',
          'icon_url',
          'guild_icon',
          'channel_avatar'
        ]),
        scope: 'channel',
        source: 'directory',
        updatedAt: now
      })
    }
  }
  return { contacts, spaces }
}

export function mergeDirectory<T>(
  current: T[],
  incoming: T[],
  id: (item: T) => string,
  changedAt: (item: T) => number
) {
  const merged = new Map(current.map(item => [id(item), item]))
  for (const item of incoming) {
    const old = merged.get(id(item))
    if (!old || changedAt(old) <= changedAt(item)) merged.set(id(item), item)
  }
  return [...merged.values()]
    .sort((left, right) => changedAt(right) - changedAt(left))
    .slice(0, 200)
}
