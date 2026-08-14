export type QQChatDensity = 'compact' | 'comfortable'
export type QQChatFontSize = 'small' | 'medium' | 'large'
export type FavoriteRetention = '7d' | '30d' | 'forever'

export type QQFavorite = {
  id: string
  conversationId: string
  messageId: string
  text: string
  createdAt: number
  expiresAt?: number
}

export type QQContact = {
  id: string
  label: string
  avatar?: string
  source: 'private' | 'member' | 'conversation'
  lastInteractionAt: number
}

export type QQSpace = {
  id: string
  label: string
  /** Platform-provided group, guild or channel portrait when available. */
  avatar?: string
  scope: 'group' | 'channel' | 'direct'
  source: 'conversation' | 'directory'
  updatedAt: number
}

export type QQChatPreferences = {
  density: QQChatDensity
  fontSize: QQChatFontSize
  autoProfile: boolean
  historyDays: 7 | 30
  favoriteRetention: FavoriteRetention
  rightPanelOpen: boolean
  activeNav: 'messages' | 'contacts' | 'spaces' | 'favorites' | 'profile' | 'tools' | 'audit' | 'settings'
}

export type QQChatStore<Event, Tool> = {
  savedAt: number
  events: Event[]
  tools: Tool[]
  drafts: Record<string, string>
  favorites: QQFavorite[]
  contacts: QQContact[]
  spaces: QQSpace[]
  /** Private threads the user explicitly chose to open. */
  openedConversationIds: string[]
  preferences: QQChatPreferences
}

const defaultPreferences: QQChatPreferences = {
  density: 'comfortable',
  fontSize: 'medium',
  autoProfile: true,
  historyDays: 30,
  favoriteRetention: 'forever',
  rightPanelOpen: true,
  activeNav: 'messages'
}

export function qqChatStorageKey(root: string) {
  return `alemonx:qq-live:v2:${btoa(new TextEncoder().encode(root).reduce((text, value) => text + String.fromCharCode(value), ''))}`
}

function legacyStorageKey(root: string) {
  return `alemonx:qq-live:v1:${btoa(new TextEncoder().encode(root).reduce((text, value) => text + String.fromCharCode(value), ''))}`
}

export function qqChatWindowStorageKey(root: string) {
  return `${qqChatStorageKey(root)}:window`
}

export function readQQChatStore<Event, Tool>(root: string): QQChatStore<Event, Tool> {
  const empty: QQChatStore<Event, Tool> = { savedAt: Date.now(), events: [], tools: [], drafts: {}, favorites: [], contacts: [], spaces: [], openedConversationIds: [], preferences: defaultPreferences }
  try {
    const saved = JSON.parse(localStorage.getItem(qqChatStorageKey(root)) || 'null') as Partial<QQChatStore<Event, Tool>> | null
    if (!saved) {
      const legacy = JSON.parse(localStorage.getItem(legacyStorageKey(root)) || 'null') as Partial<QQChatStore<Event, Tool>> | null
      if (!legacy) return empty
      const migrated = { ...empty, ...legacy, drafts: legacy.drafts || {}, events: Array.isArray(legacy.events) ? legacy.events : [], tools: Array.isArray(legacy.tools) ? legacy.tools : [] }
      writeQQChatStore(root, migrated)
      localStorage.removeItem(legacyStorageKey(root))
      return migrated
    }
    return {
      ...empty,
      ...saved,
      events: Array.isArray(saved.events) ? saved.events : [],
      tools: Array.isArray(saved.tools) ? saved.tools : [],
      drafts: saved.drafts || {},
      favorites: (saved.favorites || []).filter(item => !item.expiresAt || item.expiresAt > Date.now()),
      contacts: Array.isArray(saved.contacts) ? saved.contacts : [],
      spaces: Array.isArray(saved.spaces) ? saved.spaces : [],
      openedConversationIds: Array.isArray(saved.openedConversationIds) ? saved.openedConversationIds : [],
      preferences: { ...defaultPreferences, ...saved.preferences }
    }
  } catch {
    return empty
  }
}

export function writeQQChatStore<Event, Tool>(root: string, store: QQChatStore<Event, Tool>) {
  localStorage.setItem(qqChatStorageKey(root), JSON.stringify({ ...store, savedAt: Date.now() }))
}

export function favoriteExpiry(retention: FavoriteRetention, from = Date.now()) {
  if (retention === 'forever') return undefined
  return from + (retention === '7d' ? 7 : 30) * 86_400_000
}

export function clearQQChatStore(root: string) {
  localStorage.removeItem(qqChatStorageKey(root))
}

export function resetQQChatWindowLayout(root: string) {
  localStorage.removeItem(qqChatWindowStorageKey(root))
}
