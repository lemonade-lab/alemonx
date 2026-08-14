import type { QQContact, QQSpace } from './qqChatStorage'

export type QQActionResult = { code?: number; message?: string; data?: unknown }

export function resultItems(data: unknown): Record<string, unknown>[] {
  if (Array.isArray(data)) return data.filter(item => item && typeof item === 'object') as Record<string, unknown>[]
  if (!data || typeof data !== 'object') return []
  const record = data as Record<string, unknown>
  for (const key of ['data', 'items', 'list', 'members', 'guilds']) if (Array.isArray(record[key])) return record[key].filter(item => item && typeof item === 'object') as Record<string, unknown>[]
  return [record]
}

export function recordText(record: Record<string, unknown>, keys: string[]) {
  for (const key of keys) if (typeof record[key] === 'string' && record[key]) return record[key] as string
  const normalize = (key: string) => key.toLowerCase().replace(/[_-]/g, '')
  const lowercase = new Map(
    Object.entries(record).map(([key, value]) => [normalize(key), value])
  )
  for (const key of keys) {
    const value = lowercase.get(normalize(key))
    if (typeof value === 'string' && value) return value
  }
  return ''
}

export function collectActionDirectory(action: string, data: unknown, now = Date.now()): { contacts: QQContact[]; spaces: QQSpace[] } {
  const contacts: QQContact[] = []
  const spaces: QQSpace[] = []
  for (const item of resultItems(data)) {
    if (action === 'member.list') {
      const userID = recordText(item, ['user_id', 'id', 'userId', 'member_openid', 'openid'])
      if (!userID) continue
      contacts.push({ id: `user:${userID}`, label: recordText(item, ['nick', 'username', 'name', 'user_name']) || userID, avatar: recordText(item, ['avatar', 'avatar_url']), source: 'member', lastInteractionAt: now })
      continue
    }
    if (action === 'me.guilds' || action === 'guild.list' || action === 'channel.list') {
      const id = recordText(item, ['id', 'guild_id', 'channel_id', 'guildId', 'channelId'])
      if (!id) continue
      spaces.push({
        id: `channel:channel:${id}`,
        label: recordText(item, ['name', 'guild_name', 'channel_name']) || id,
        avatar: recordText(item, ['avatar', 'avatar_url', 'icon', 'icon_url', 'guild_icon', 'channel_avatar']),
        scope: 'channel',
        source: 'directory',
        updatedAt: now
      })
    }
  }
  return { contacts, spaces }
}

export function mergeDirectory<T>(current: T[], incoming: T[], id: (item: T) => string, changedAt: (item: T) => number) {
  const merged = new Map(current.map(item => [id(item), item]))
  for (const item of incoming) {
    const old = merged.get(id(item))
    if (!old || changedAt(old) <= changedAt(item)) merged.set(id(item), item)
  }
  return [...merged.values()].sort((left, right) => changedAt(right) - changedAt(left)).slice(0, 200)
}
