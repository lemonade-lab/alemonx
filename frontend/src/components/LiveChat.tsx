import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  Bell,
  Bot,
  CheckCircle2,
  CircleAlert,
  ContactRound,
  FileImage,
  Heart,
  History,
  Loader2,
  MessageCircle,
  Paperclip,
  Search,
  Send,
  Settings2,
  Trash2,
  Undo2,
  UsersRound,
  X
} from 'lucide-react'
import * as flattedJSON from 'flatted'
import {
  isActionAvailable,
  qqActionCatalog,
  type QQActionDefinition,
  type QQActionField,
  type QQScope
} from './qqActionCatalog'
import {
  clearQQChatStore,
  favoriteExpiry,
  resetQQChatWindowLayout,
  type QQChatPreferences,
  type QQContact,
  type QQFavorite,
  type QQSpace,
  readQQChatStore,
  writeQQChatStore
} from './qqChatStorage'
import {
  collectActionDirectory,
  mergeDirectory,
  recordText,
  resultItems
} from './qqChatDirectory'
import { ConfirmDialog } from './ConfirmDialog'

type Segment = { type: string; value?: unknown }
type LiveEvent = {
  name?: string
  Platform?: string
  BotId?: string
  UserId?: string
  OpenId?: string
  SpaceId?: string
  UserName?: string
  UserAvatar?: string
  ChannelId?: string
  GuildId?: string
  ChannelName?: string
  ChannelAvatar?: string
  MessageId?: string
  MessageText?: string
  CreateAt?: number
  value?: Segment[]
  _tag?: string
  senderType?: 'bot' | 'user'
  delivery?: 'sending' | 'sent' | 'failed'
  serverMessageID?: string
  context?: LiveEvent
}

type Conversation = {
  id: string
  label: string
  avatar?: string
  private: boolean
  event: LiveEvent
  lastEvent?: LiveEvent
  updatedAt: number
  synthetic?: boolean
}
type ActionResult = { code?: number; message?: string; data?: unknown }
type ActionState = 'success' | 'warning' | 'failed' | 'timeout' | 'cancelled'
type Upload = {
  uploadId: string
  path: string
  filename: string
  size: number
  mimeType: string
}
type PendingAction = {
  kind: 'send' | 'delete' | 'typing' | 'tool' | 'profile'
  action: string
  messageID?: string
  uploads?: Upload[]
  target?: string
  onResult?: (
    results: ActionResult[],
    state: ActionState,
    summary: string
  ) => void
}
type MessageDecoration = { pinned?: boolean; reactions?: string[] }
type ToolRecord = {
  id: string
  action: string
  target: string
  at: number
  state: ActionState
  summary: string
}
type StoredHistory = {
  savedAt: number
  events: Array<Omit<LiveEvent, 'context'>>
  tools: ToolRecord[]
  drafts: Record<string, string>
  favorites: QQFavorite[]
  contacts: QQContact[]
  spaces: QQSpace[]
  openedConversationIds: string[]
  preferences: QQChatPreferences
}
type ProfileState = {
  status: 'idle' | 'loading' | 'ready' | 'unavailable' | 'failed'
  data?: unknown
  message?: string
  updatedAt?: number
}
type UserProfileState = ProfileState & { label?: string; userID?: string }
type Confirmation = {
  title: string
  message: string
  confirmLabel: string
  destructive?: boolean
  onConfirm: () => void
}
type CBPResponse = {
  protocol?: string
  type?: string
  id?: string
  replyTo?: string
  actionId?: string
  error?: unknown
  payload?: { event?: LiveEvent; results?: ActionResult[] } | ActionResult[]
}

const historyDays = 30
const requestTimeout = 20_000
const deviceID = `alemonx-live-${crypto.randomUUID()}`
const uploadActions = new Set([
  'file.send.channel',
  'file.send.user',
  'media.send',
  'media.upload',
  'media.upload.chunked'
])

function eventText(event: LiveEvent) {
  if (event.MessageText) return event.MessageText
  if (Array.isArray(event.value))
    return event.value
      .filter(item => item.type === 'Text')
      .map(item => String(item.value ?? ''))
      .join('')
  return ''
}

function messageSegments(event: LiveEvent) {
  if (Array.isArray(event.value)) return event.value
  return event.MessageText ? [{ type: 'Text', value: event.MessageText }] : []
}

function isMessage(event: LiveEvent) {
  return (
    event.name === 'message.create' || event.name === 'private.message.create'
  )
}

function errorText(error: unknown) {
  if (typeof error === 'string') return error
  if (
    error &&
    typeof error === 'object' &&
    'message' in error &&
    typeof (error as { message?: unknown }).message === 'string'
  )
    return String((error as { message: string }).message)
  return 'QQ Bot 未能完成这项操作。'
}

function resultState(
  results: ActionResult[] | undefined,
  error?: unknown
): { state: ActionState; summary: string } {
  if (error) return { state: 'failed', summary: errorText(error) }
  if (!results?.length)
    return { state: 'failed', summary: 'QQ Bot 未返回可确认的结果。' }
  const firstNonSuccess = results.find(item => item.code !== 2000)
  if (!firstNonSuccess)
    return {
      state: 'success',
      summary: results[0]?.message || 'QQ 已确认完成。'
    }
  return {
    state: firstNonSuccess.code === 2100 ? 'warning' : 'failed',
    summary:
      firstNonSuccess.message || `QQ 返回状态 ${firstNonSuccess.code ?? '未知'}`
  }
}

function resultMessageID(results: ActionResult[] | undefined) {
  for (const result of results ?? []) {
    if (!result.data || typeof result.data !== 'object') continue
    const data = result.data as Record<string, unknown>
    for (const key of ['id', 'message_id', 'MessageId'])
      if (typeof data[key] === 'string' && data[key]) return data[key] as string
  }
  return ''
}

function qqTarget(event: LiveEvent) {
  const space = event.SpaceId || ''
  const open = event.OpenId || ''
  const bot = event.BotId ? { BotId: event.BotId } : {}
  if (space.startsWith('GROUP:'))
    return { scope: 'group' as const, targetId: space.slice(6), ...bot }
  if (space.startsWith('GUILD:'))
    return { scope: 'channel' as const, targetId: space.slice(6), ...bot }
  if (open.startsWith('C2C:'))
    return { scope: 'c2c' as const, targetId: open.slice(4), ...bot }
  if (open.startsWith('DIRECT:'))
    return { scope: 'direct' as const, targetId: open.slice(7), ...bot }
  if (event.name === 'private.message.create')
    return { scope: 'c2c' as const, targetId: event.UserId || '', ...bot }
  return {
    scope: 'channel' as const,
    targetId: event.ChannelId || event.GuildId || '',
    ...bot
  }
}

function conversationTarget(event: LiveEvent) {
  const target = qqTarget(event)
  return `${target.scope}:${target.targetId}`
}

function conversationID(event: LiveEvent) {
  const target = conversationTarget(event)
  return `${target.startsWith('c2c:') || target.startsWith('direct:') ? 'user' : 'channel'}:${target}`
}

function isBotMessage(event: LiveEvent) {
  return event.senderType === 'bot' || event.UserName === '机器人'
}

function hasRawSendContext(event?: LiveEvent) {
  return Boolean(event?.context?._tag)
}

function eventUserID(event: LiveEvent) {
  return event.UserId || event.OpenId?.replace(/^(?:C2C|DIRECT):/, '') || ''
}

function contactFromEvent(event: LiveEvent): QQContact | undefined {
  const userID = eventUserID(event)
  if (!userID || isBotMessage(event)) return undefined
  return {
    id: `user:${userID}`,
    label: event.UserName || userID,
    avatar: event.UserAvatar,
    source: event.name === 'private.message.create' ? 'private' : 'conversation',
    lastInteractionAt: event.CreateAt || Date.now()
  }
}

type QQIdentity = { name: string; avatar?: string }

function identityRecord(data: unknown) {
  const record = resultItems(data)[0]
  if (!record) return undefined
  for (const key of ['data', 'bot', 'user', 'profile', 'group', 'guild', 'channel']) {
    const nested = record[key]
    if (nested && typeof nested === 'object' && !Array.isArray(nested))
      return nested as Record<string, unknown>
  }
  return record
}

function profileIdentity(data: unknown, fallback: QQIdentity): QQIdentity {
  const record = identityRecord(data)
  if (!record) return fallback
  return {
    name:
      recordText(record, [
        'nick',
        'nickname',
        'username',
        'user_name',
        'bot_name',
        'name'
      ]) || fallback.name,
    avatar:
      recordText(record, [
        'avatar',
        'avatar_url',
        'head_url',
        'headurl',
        'user_avatar'
      ]) || fallback.avatar
  }
}

function spaceIdentity(data: unknown, fallback: QQSpace): QQSpace {
  const record = identityRecord(data)
  if (!record) return fallback
  return {
    ...fallback,
    label:
      recordText(record, [
        'name',
        'group_name',
        'guild_name',
        'channel_name',
        'title'
      ]) || fallback.label,
    avatar:
      recordText(record, [
        'avatar',
        'avatar_url',
        'icon',
        'icon_url',
        'group_avatar',
        'guild_icon',
        'channel_avatar'
      ]) || fallback.avatar,
    updatedAt: Date.now()
  }
}

function messageFormat(text: string, contacts: QQContact[]) {
  const mentionable = contacts
    .filter(contact => contact.id.startsWith('user:') && contact.label.trim())
    .sort((left, right) => right.label.length - left.label.length)
  if (!mentionable.length) return [{ type: 'Text', value: text }]
  const escaped = mentionable.map(contact =>
    contact.label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  )
  const matcher = new RegExp(`@(${escaped.join('|')})(?![^\\s])`, 'g')
  const format: Array<Record<string, unknown>> = []
  let cursor = 0
  for (const match of text.matchAll(matcher)) {
    const index = match.index ?? 0
    if (index > cursor)
      format.push({ type: 'Text', value: text.slice(cursor, index) })
    const contact = mentionable.find(item => item.label === match[1])
    format.push(
      contact
        ? {
            type: 'Mention',
            value: contact.id.slice('user:'.length),
            options: { belong: 'user' }
          }
        : { type: 'Text', value: match[0] }
    )
    cursor = index + match[0].length
  }
  if (cursor < text.length)
    format.push({ type: 'Text', value: text.slice(cursor) })
  return format.length ? format : [{ type: 'Text', value: text }]
}

function makeDirectConversation(contact: QQContact): Conversation {
  const userID = contact.id.replace(/^user:/, '')
  const event: LiveEvent = {
    name: 'private.message.create',
    Platform: 'qq-bot',
    UserId: userID,
    OpenId: `C2C:${userID}`,
    UserName: contact.label,
    UserAvatar: contact.avatar
  }
  return {
    id: conversationID(event),
    label: contact.label,
    avatar: contact.avatar,
    private: true,
    event,
    updatedAt: contact.lastInteractionAt,
    synthetic: true
  }
}

function makeSpaceConversation(space: QQSpace): Conversation {
  const targetID = space.id
    .replace(/^channel:(?:group|channel|direct):/, '')
    .replace(/^channel:/, '')
  const event: LiveEvent =
    space.scope === 'group'
      ? {
          name: 'message.create',
          Platform: 'qq-bot',
          ChannelId: targetID,
          GuildId: targetID,
          SpaceId: `GROUP:${targetID}`,
          ChannelName: space.label,
          ChannelAvatar: space.avatar
        }
      : {
          name: 'message.create',
          Platform: 'qq-bot',
          ChannelId: targetID,
          GuildId: targetID,
          SpaceId: `GUILD:${targetID}`,
          ChannelName: space.label,
          ChannelAvatar: space.avatar
        }
  return {
    id: conversationID(event),
    label: space.label,
    avatar: space.avatar,
    private: false,
    event,
    updatedAt: space.updatedAt,
    synthetic: true
  }
}

function scopeName(scope: QQScope | '') {
  return (
    (
      {
        group: '群',
        channel: '频道',
        c2c: '私聊',
        direct: '频道私信',
        global: '全局'
      } as Record<string, string>
    )[scope] || '未选择会话'
  )
}

function wsURL(root: string) {
  const token = btoa(
    Array.from(new TextEncoder().encode(root), value =>
      String.fromCharCode(value)
    ).join('')
  )
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${location.host}/api/v1/robot/live/${token}/?${new URLSearchParams({ deviceId: deviceID })}`
}

function redact(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(redact)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>).map(([key, item]) => [
      /(token|secret|password|authorization|file_data)/i.test(key)
        ? '[已脱敏]'
        : key,
      redact(item)
    ])
  )
}

function getFormValue(values: Record<string, unknown>, field: QQActionField) {
  const value = values[field.key]
  if (field.kind === 'boolean') return Boolean(value)
  return typeof value === 'string' ? value : ''
}

async function fileData(file: File) {
  const bytes = await file.arrayBuffer()
  let text = ''
  for (const value of new Uint8Array(bytes)) text += String.fromCharCode(value)
  return `base64://${btoa(text)}`
}

function initialValues(action: QQActionDefinition, event?: LiveEvent) {
  const values: Record<string, unknown> = {}
  for (const field of action.fields) {
    if (field.kind === 'boolean') values[field.key] = false
    if (field.key === 'ChannelId') values[field.key] = event?.ChannelId || ''
    if (field.key === 'UserId') values[field.key] = event?.UserId || ''
    if (field.key === 'memberOpenId') values[field.key] = event?.UserId || ''
    if (field.key === 'targetId')
      values[field.key] = qqTarget(event ?? {}).targetId
    if (field.key === 'targetScope')
      values[field.key] = qqTarget(event ?? {}).scope
    if (field.key === 'input_type') values[field.key] = '1'
    if (field.key === 'file_type') values[field.key] = '1'
    if (field.key === 'type') values[field.key] = 'image'
    if (field.key === 'code') values[field.key] = '0'
    if (field.key === 'stream_interval') values[field.key] = '300'
  }
  return values
}

function valuesToInput(
  action: QQActionDefinition,
  values: Record<string, unknown>,
  event?: LiveEvent
): Record<string, unknown> {
  const params: Record<string, unknown> = {}
  const input: Record<string, unknown> = event
    ? { event: event.context ?? event, target: qqTarget(event) }
    : {}
  for (const field of action.fields) {
    const raw = values[field.key]
    if (raw === undefined || raw === '') continue
    if (field.kind === 'file') continue
    if (field.key === 'formatText') {
      params.format = [{ type: 'Text', value: String(raw) }]
      continue
    }
    if (field.key === 'targetId' || field.key === 'targetScope') continue
    if (field.key === 'members') {
      params.members = String(raw)
        .split('\n')
        .map(line => line.trim())
        .filter(Boolean)
        .map(line => {
          const [op, member_openid, mute_expire_at] = line
            .split(',')
            .map(item => item.trim())
          return {
            op,
            member_openid,
            ...(mute_expire_at ? { mute_expire_at } : {})
          }
        })
      continue
    }
    if (field.kind === 'csv') {
      params[field.key] = String(raw)
        .split(/[\n,]/)
        .map(item => item.trim())
        .filter(Boolean)
      continue
    }
    if (
      field.kind === 'number' ||
      ['code', 'input_type', 'file_type', 'stream_interval'].includes(field.key)
    ) {
      params[field.key] = Number(raw)
      continue
    }
    if (field.kind === 'boolean') {
      params[field.key] = Boolean(raw)
      continue
    }
    params[field.key] = raw
  }
  if (action.id === 'message.send.channel') {
    input.ChannelId = values.ChannelId || event?.ChannelId || ''
  } else if (action.id === 'message.send.user') {
    input.UserId = values.UserId || event?.UserId || ''
  } else if (action.id === 'message.send.target') {
    input.target = {
      scope: values.targetScope || qqTarget(event ?? {}).scope,
      targetId: values.targetId || qqTarget(event ?? {}).targetId,
      ...(event?.BotId ? { BotId: event.BotId } : {})
    }
  }
  for (const key of [
    'MessageId',
    'EmojiId',
    'RoleId',
    'StrategyId',
    'interaction_id',
    'InteractionId',
    'UserId',
    'ChannelId'
  ]) {
    if (values[key]) input[key] = values[key]
  }
  if (values.channelId) params.channelId = values.channelId
  if (values.interactionId) {
    params.interactionId = values.interactionId
    input.InteractionId = values.interactionId
  }
  if (values.memberOpenId) params.memberOpenId = values.memberOpenId
  if (values.userId) params.userId = values.userId
  if (values.file_path) params.file_path = values.file_path
  if (values.filePath) params.filePath = values.filePath
  if (values.file_data) params.file_data = values.file_data
  if (values.data) params.data = values.data
  if (values.groupActionOp)
    params.groupAction = {
      op: values.groupActionOp,
      group_openids: params.groupActionOpenIds,
      group_ids: params.groupActionIds
    }
  delete params.groupActionOp
  delete params.groupActionOpenIds
  delete params.groupActionIds
  if (action.id === 'interaction.response')
    input.code = Number(values.code || 0)
  if (values.url) params.url = values.url
  if (Object.keys(params).length) input.params = params
  return input
}

export function LiveChat({ root }: { root: string }) {
  const socket = useRef<WebSocket | null>(null)
  const pendingRef = useRef<Record<string, PendingAction>>({})
  const pendingTimers = useRef<Record<string, number>>({})
  const attachmentInput = useRef<HTMLInputElement | null>(null)
  const [state, setState] = useState<'connecting' | 'connected' | 'failed'>(
    'connecting'
  )
  const [error, setError] = useState('')
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [selected, setSelected] = useState('')
  const [text, setText] = useState('')
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [attachment, setAttachment] = useState<File | null>(null)
  const [pending, setPending] = useState<Record<string, PendingAction>>({})
  const [tools, setTools] = useState<ToolRecord[]>([])
  const [activeAction, setActiveAction] = useState('')
  const [formValues, setFormValues] = useState<Record<string, unknown>>({})
  const [showUnavailable, setShowUnavailable] = useState(false)
  const [historyReadyRoot, setHistoryReadyRoot] = useState('')
  const [activeNav, setActiveNav] = useState<
    | 'messages'
    | 'contacts'
    | 'spaces'
    | 'favorites'
    | 'profile'
    | 'tools'
    | 'audit'
    | 'settings'
  >('messages')
  const [rightOpen, setRightOpen] = useState(true)
  const [search, setSearch] = useState('')
  const [favorites, setFavorites] = useState<QQFavorite[]>([])
  const [contacts, setContacts] = useState<QQContact[]>([])
  const [spaces, setSpaces] = useState<QQSpace[]>([])
  const [openedConversationIDs, setOpenedConversationIDs] = useState<string[]>([])
  const [preferences, setPreferences] = useState<QQChatPreferences>({
    density: 'comfortable',
    fontSize: 'medium',
    autoProfile: true,
    historyDays: 30,
    favoriteRetention: 'forever',
    rightPanelOpen: true,
    activeNav: 'messages'
  })
  const [profile, setProfile] = useState<ProfileState>({ status: 'idle' })
  const [members, setMembers] = useState<ProfileState>({ status: 'idle' })
  const [roles, setRoles] = useState<ProfileState>({ status: 'idle' })
  const [groupBotState, setGroupBotState] = useState<ProfileState>({ status: 'idle' })
  const [groupMuteSetting, setGroupMuteSetting] = useState<ProfileState>({ status: 'idle' })
  const [joinRequests, setJoinRequests] = useState<ProfileState>({ status: 'idle' })
  const [botProfile, setBotProfile] = useState<UserProfileState>({ status: 'idle' })
  const [userProfile, setUserProfile] = useState<UserProfileState>({ status: 'idle' })
  const [decorations, setDecorations] = useState<Record<string, MessageDecoration>>({})
  const [typing, setTyping] = useState(false)
  const typingTimer = useRef<number | null>(null)
  const [connectionAttempt, setConnectionAttempt] = useState(0)
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null)
  const botIdentity = useMemo(
    () => profileIdentity(botProfile.data, { name: '机器人' }),
    [botProfile.data]
  )

  const cleanupUpload = useCallback(
    async (upload: Upload) => {
      try {
        await fetch('/api/v1/robot/live/upload', {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            root,
            deviceId: deviceID,
            uploadId: upload.uploadId
          })
        })
      } catch {
        // The server also expires abandoned files. A network failure must not
        // change a QQ result that has already been confirmed.
      }
    },
    [root]
  )

  const addToolRecord = useCallback(
    (action: string, target: string, state: ActionState, summary: string) => {
      setTools(current =>
        [
          {
            id: crypto.randomUUID(),
            action,
            target,
            state,
            summary: String(redact(summary)),
            at: Date.now()
          },
          ...current
        ].slice(0, 100)
      )
    },
    []
  )

  useEffect(() => {
    if (!root) return
    setHistoryReadyRoot('')
    setEvents([])
    setTools([])
    setDrafts({})
    setFavorites([])
    setContacts([])
    setSpaces([])
    setOpenedConversationIDs([])
    setOpenedConversations([])
    setBotProfile({ status: 'idle' })
    setUserProfile({ status: 'idle' })
    try {
      const parsed = readQQChatStore<
        Array<Omit<LiveEvent, 'context'>>,
        ToolRecord
      >(root) as StoredHistory
      if (parsed && Date.now() - parsed.savedAt < historyDays * 86_400_000) {
        setEvents(parsed.events.map(event => ({ ...event })))
        setTools(
          parsed.tools.filter(
            item => Date.now() - item.at < historyDays * 86_400_000
          )
        )
        setDrafts(parsed.drafts || {})
        setFavorites(parsed.favorites || [])
        setContacts(parsed.contacts || [])
        setSpaces(parsed.spaces || [])
        setOpenedConversationIDs(parsed.openedConversationIds || [])
        setPreferences(parsed.preferences)
        // “会话资料” used to be a persistent left-side navigation state.
        // Keep that legacy preference from reopening the chat into a detail
        // pane; on desktop details now belong in the explicit right drawer.
        setActiveNav(
          parsed.preferences.activeNav === 'profile'
            ? 'messages'
            : parsed.preferences.activeNav
        )
        setRightOpen(parsed.preferences.rightPanelOpen)
      }
    } catch {
      /* a corrupt local record is ignored */
    }
    setHistoryReadyRoot(root)
  }, [root])

  useEffect(() => {
    if (!root || historyReadyRoot !== root) return
    const retention = preferences.historyDays * 86_400_000
    const retainedEvents = events
      .filter(
        event => !event.CreateAt || Date.now() - event.CreateAt < retention
      )
      .map(({ context, ...event }) => {
        void context
        return event
      })
    const retainedTools = tools.filter(item => Date.now() - item.at < retention)
    writeQQChatStore(root, {
      savedAt: Date.now(),
      events: retainedEvents.slice(-500),
      tools: retainedTools.slice(0, 100),
      drafts,
      favorites: favorites.filter(
        item => !item.expiresAt || item.expiresAt > Date.now()
      ),
      contacts,
      spaces,
      openedConversationIds: openedConversationIDs,
      preferences: { ...preferences, activeNav, rightPanelOpen: rightOpen }
    })
  }, [
    activeNav,
    contacts,
    drafts,
    events,
    favorites,
    historyReadyRoot,
    openedConversationIDs,
    preferences,
    rightOpen,
    root,
    spaces,
    tools
  ])

  const resolvePending = useCallback(
    (
      requestID: string,
      result: { state: ActionState; summary: string },
      results?: ActionResult[]
    ) => {
      const request = pendingRef.current[requestID]
      if (!request) return
      if (pendingTimers.current[requestID])
        window.clearTimeout(pendingTimers.current[requestID])
      delete pendingTimers.current[requestID]
      const next = { ...pendingRef.current }
      delete next[requestID]
      pendingRef.current = next
      setPending(next)
      request.uploads?.forEach(upload => void cleanupUpload(upload))
      if (request.kind === 'send') {
        setEvents(current =>
          current.map(event =>
            event.MessageId === request.messageID
              ? {
                  ...event,
                  delivery: result.state === 'success' ? 'sent' : 'failed',
                  serverMessageID:
                    result.state === 'success'
                      ? resultMessageID(results)
                      : undefined
                }
              : event
          )
        )
      }
      if (request.kind === 'delete' && result.state === 'success')
        setEvents(current =>
          current.filter(event => event.MessageId !== request.messageID)
        )
      request.onResult?.(results || [], result.state, result.summary)
      if (
        result.state === 'success' &&
        ['member.list', 'me.guilds', 'guild.list', 'channel.list'].includes(
          request.action
        )
      ) {
        const directory = collectActionDirectory(
          request.action,
          results?.[0]?.data
        )
        if (directory.contacts.length)
          setContacts(current =>
            mergeDirectory(
              current,
              directory.contacts,
              item => item.id,
              item => item.lastInteractionAt
            )
          )
        if (directory.spaces.length)
          setSpaces(current =>
            mergeDirectory(
              current,
              directory.spaces,
              item => item.id,
              item => item.updatedAt
            )
          )
      }
      if (request.kind !== 'send')
        addToolRecord(
          request.action,
          request.target || scopeName('global'),
          result.state,
          result.summary
        )
      if (result.state !== 'success') setError(result.summary)
    },
    [addToolRecord, cleanupUpload]
  )

  useEffect(() => {
    if (!root) return
    let closed = false
    setState('connecting')
    setError('')
    const connection = new WebSocket(wsURL(root))
    socket.current = connection
    connection.onopen = () => !closed && setState('connected')
    connection.onmessage = message => {
      try {
        const payload = flattedJSON.parse(String(message.data)) as CBPResponse
        const isResponse =
          payload.protocol === 'cbp' && payload.type === 'action.res'
        if (isResponse) {
          const requestID = payload.replyTo || payload.actionId
          if (!requestID || !pendingRef.current[requestID]) return
          const results = Array.isArray(payload.payload)
            ? payload.payload
            : payload.payload?.results
          resolvePending(
            requestID,
            resultState(results, payload.error),
            results
          )
          return
        }
        const body = Array.isArray(payload.payload)
          ? undefined
          : payload.payload
        const event =
          payload.protocol === 'cbp' && payload.type === 'event'
            ? body?.event
            : undefined
        if (event && isMessage(event))
          setEvents(current => [
            ...current.slice(-499),
            { ...event, context: event }
          ])
      } catch {
        setError('收到无法识别的 QQ Bot 响应，操作未被标记为成功。')
      }
    }
    connection.onerror = () =>
      !closed && setError('无法连接到已登录机器人的 CBP 服务。')
    connection.onclose = event => {
      if (closed) return
      setState('failed')
      setError(event.reason || '在线连接已断开。请确认机器人仍在运行。')
    }
    return () => {
      closed = true
      Object.values(pendingTimers.current).forEach(timer =>
        window.clearTimeout(timer)
      )
      pendingTimers.current = {}
      connection.close(1000, 'window closed')
      if (socket.current === connection) socket.current = null
    }
  }, [connectionAttempt, resolvePending, root])

  const reconnect = useCallback(() => {
    setError('')
    setState('connecting')
    setConnectionAttempt(current => current + 1)
  }, [])

  const conversations = useMemo(() => {
    const all = new Map<string, Conversation>()
    for (const event of events) {
      const target = conversationTarget(event)
      if (target.endsWith(':')) continue
      const id = conversationID(event)
      const privateChat = target.startsWith('c2c:') || target.startsWith('direct:')
      // New people are collected as contacts, but a private thread is only
      // created after the user explicitly opens that person from their avatar
      // or the contacts list.
      if (privateChat && !openedConversationIDs.includes(id)) continue
      const previous = all.get(id)
      // Delivery records are authored by the robot, but their target still
      // identifies the person / group / channel. Resolve that identity from
      // the local directory so an outgoing message cannot turn a chat into
      // a conversation named “机器人”.
      const knownContact =
        privateChat
          ? contacts.find(
              item =>
                item.id ===
                `user:${event.UserId || event.OpenId?.replace(/^C2C:/, '') || ''}`
            )
          : undefined
      const knownSpace =
        !privateChat
          ? spaces.find(
              item =>
                item.id ===
                `channel:${qqTarget(event).scope}:${qqTarget(event).targetId}`
            )
          : undefined
      const identityEvent = knownContact
        ? {
            ...event,
            UserName: knownContact.label,
            UserAvatar: knownContact.avatar
          }
        : knownSpace
          ? {
              ...event,
              ChannelName: knownSpace.label,
              ChannelAvatar: knownSpace.avatar
            }
          : event
      const metadata = previous?.event || identityEvent
      const latest =
        !previous ||
        (event.CreateAt || 0) >=
          (previous.lastEvent?.CreateAt || previous.updatedAt)
          ? event
          : previous.lastEvent
      all.set(id, {
        id,
        private: previous?.private ?? privateChat,
        // A sent robot message must never replace the conversation identity.
        event: isBotMessage(event) ? metadata : identityEvent,
        label:
          knownContact?.label ||
          knownSpace?.label ||
          previous?.label ||
          (isBotMessage(event)
            ? `${scopeName(qqTarget(event).scope)} ${qqTarget(event).targetId}`
            : privateChat
              ? event.UserName || event.UserId || '私聊'
              : event.ChannelName ||
                event.ChannelId ||
                event.GuildId ||
                '群 / 频道'),
        avatar:
          knownContact?.avatar ||
          knownSpace?.avatar ||
          previous?.avatar ||
          (privateChat ? event.UserAvatar : event.ChannelAvatar),
        lastEvent: latest,
        updatedAt:
          latest?.CreateAt || previous?.updatedAt || event.CreateAt || 0
      })
    }
    return [...all.values()].sort(
      (left, right) => right.updatedAt - left.updatedAt
    )
  }, [contacts, events, openedConversationIDs, spaces])

  useEffect(() => {
    const learned = events.flatMap(event => {
      const target = qqTarget(event)
      if (
        !target.targetId ||
        !['group', 'channel', 'direct'].includes(target.scope)
      )
        return []
      return [
        {
          id: `channel:${target.scope}:${target.targetId}`,
          label:
            event.ChannelName ||
            event.GuildId ||
            event.ChannelId ||
            target.targetId,
          avatar: event.ChannelAvatar,
          scope: target.scope as QQSpace['scope'],
          source: 'conversation' as const,
          updatedAt: event.CreateAt || Date.now()
        }
      ]
    })
    if (learned.length)
      setSpaces(current =>
        mergeDirectory(
          current,
          learned,
          item => item.id,
          item => item.updatedAt
        )
      )
  }, [events])

  const [openedConversations, setOpenedConversations] = useState<
    Conversation[]
  >([])
  const allConversations = useMemo(() => {
    const merged = new Map(openedConversations.map(item => [item.id, item]))
    for (const conversation of conversations) {
      const opened = merged.get(conversation.id)
      // A first outgoing message may be the newest event. Keep the locally
      // chosen contact/space identity instead of renaming the chat to bot.
      if (opened && isBotMessage(conversation.event)) {
        const unresolved = /^(群|频道|私聊|频道私信) /.test(conversation.label)
        merged.set(conversation.id, {
          ...conversation,
          event: opened.event,
          label: unresolved ? opened.label : conversation.label,
          avatar: conversation.avatar || opened.avatar,
          private: conversation.private
        })
      } else {
        merged.set(conversation.id, conversation)
      }
    }
    return [...merged.values()].sort(
      (left, right) => right.updatedAt - left.updatedAt
    )
  }, [conversations, openedConversations])

  // A social inbox should open on the most recent thread.  The message column
  // itself is bottom-anchored, so this does not rely on an imperative scroll.
  useEffect(() => {
    if (!selected && allConversations.length) setSelected(allConversations[0].id)
  }, [allConversations, selected])

  const currentConversation = allConversations.find(
    item => item.id === selected
  )
  const currentEvent = currentConversation?.event
  const isQQ = currentEvent?.Platform === 'qq-bot'
  const currentScope: QQScope | '' =
    currentEvent && isQQ ? qqTarget(currentEvent).scope : ''
  const messages = useMemo(
    () =>
      currentConversation
        ? events.filter(event => conversationID(event) === selected)
        : [],
    [currentConversation, events, selected]
  )
  const activeDefinition = qqActionCatalog.find(
    item => item.id === activeAction
  )

  useEffect(() => {
    const learned = new Map<string, QQContact>()
    for (const event of events) {
      const contact = contactFromEvent(event)
      if (contact) learned.set(contact.id, contact)
    }
    if (!learned.size) return
    setContacts(current => {
      const merged = new Map(current.map(item => [item.id, item]))
      learned.forEach((item, id) => {
        const old = merged.get(id)
        if (!old || old.lastInteractionAt < item.lastInteractionAt)
          merged.set(id, item)
      })
      return [...merged.values()]
        .sort((a, b) => b.lastInteractionAt - a.lastInteractionAt)
        .slice(0, 200)
    })
  }, [events])

  const callAction = useCallback(
    (
      action: string,
      input: Record<string, unknown>,
      pendingAction: PendingAction
    ) => {
      if (socket.current?.readyState !== WebSocket.OPEN) {
        setError('在线连接已断开，无法执行操作。')
        return ''
      }
      const createdAt = Date.now()
      const requestID = `${deviceID}:${createdAt}:${crypto.randomUUID().slice(0, 8)}`
      const next = { ...pendingRef.current, [requestID]: pendingAction }
      pendingRef.current = next
      setPending(next)
      try {
        socket.current.send(
          flattedJSON.stringify({
            protocol: 'cbp',
            version: 1,
            type: 'action.req',
            id: requestID,
            timestamp: createdAt,
            source: { role: 'app-client', deviceId: deviceID },
            payload: { action, input }
          })
        )
        pendingTimers.current[requestID] = window.setTimeout(
          () =>
            resolvePending(requestID, {
              state: 'timeout',
              summary: 'QQ Bot 在 20 秒内未返回结果，无法确认是否执行成功。'
            }),
          requestTimeout
        )
        return requestID
      } catch {
        delete next[requestID]
        pendingRef.current = next
        setPending(next)
        pendingAction.uploads?.forEach(upload => void cleanupUpload(upload))
        setError('操作未发出，在线连接已断开。')
        return ''
      }
    },
    [cleanupUpload, resolvePending]
  )

  const requestAutoRead = useCallback(() => {
    if (socket.current?.readyState !== WebSocket.OPEN)
      return
    const request = (action: 'me.info' | 'me.guilds' | 'guild.list') => {
      const definition = qqActionCatalog.find(item => item.id === action)
      if (!definition) return
      callAction(action, valuesToInput(definition, initialValues(definition)), {
        kind: 'profile',
        action,
        target: action === 'me.info' ? '当前机器人' : '全局目录',
        onResult: (results, nextState, summary) => {
          if (action !== 'me.info') return
          setBotProfile(
            nextState === 'success'
              ? {
                  status: 'ready',
                  data: results[0]?.data,
                  updatedAt: Date.now()
                }
              : {
                  status: nextState === 'timeout' ? 'failed' : 'unavailable',
                  message: summary,
                  updatedAt: Date.now()
                }
          )
        }
      })
    }
    request('me.info')
    request('me.guilds')
    request('guild.list')
  }, [callAction])

  useEffect(() => {
    if (state === 'connected') requestAutoRead()
  }, [requestAutoRead, state])

  const uploadFile = useCallback(
    async (file: File) => {
      const body = new FormData()
      body.append('root', root)
      body.append('deviceId', deviceID)
      body.append('file', file)
      const response = await fetch('/api/v1/robot/live/upload', {
        method: 'POST',
        body
      })
      const data = (await response.json()) as Upload & { error?: string }
      if (!response.ok || !data.uploadId || !data.path)
        throw new Error(data.error || '文件暂存失败。')
      return data
    },
    [root]
  )

  const send = useCallback(async () => {
    if (
      !currentConversation ||
      !isQQ ||
      socket.current?.readyState !== WebSocket.OPEN
    )
      return
    const value = text.trim()
    if (!value && !attachment) return
    let upload: Upload | undefined
    try {
      if (attachment) upload = await uploadFile(attachment)
      const sourceEvent = currentEvent?.context ?? currentEvent
      if (!sourceEvent) return
      const target = qqTarget(sourceEvent)
      const fileName = attachment?.name || ''
      const action =
        upload && (target.scope === 'group' || target.scope === 'c2c')
          ? 'media.send'
          : hasRawSendContext(currentEvent)
            ? 'message.send'
            : target.scope === 'c2c' || target.scope === 'direct'
              ? 'message.send.user'
              : target.scope === 'channel'
                ? 'message.send.channel'
                : 'message.send.target'
      const format = [
        ...(value ? messageFormat(value, contacts) : []),
        ...(upload
          ? [
              {
                type: attachment?.type.startsWith('image/') ? 'Image' : 'File',
                value: upload.path,
                name: fileName
              }
            ]
          : [])
      ]
      const input =
        action === 'media.send'
          ? {
              event: sourceEvent,
              target,
              params: {
                type: attachment?.type.startsWith('image/') ? 'image' : 'file',
                filePath: upload?.path,
                content: value
              }
            }
          : action === 'message.send.user'
            ? { UserId: target.targetId, params: { format } }
            : action === 'message.send.channel'
              ? { ChannelId: target.targetId, params: { format } }
              : action === 'message.send.target'
                ? { target, params: { format } }
                : { event: sourceEvent, target, params: { format } }
      const requestID = callAction(action, input, {
        kind: 'send',
        action,
        messageID: '',
        uploads: upload ? [upload] : [],
        target: `${scopeName(target.scope)} ${target.targetId}`
      })
      if (!requestID) return
      setEvents(current => [
        ...current.slice(-499),
        {
          name: currentConversation.private
            ? 'private.message.create'
            : 'message.create',
          MessageId: requestID,
          CreateAt: Date.now(),
          MessageText: value || `📎 ${fileName}`,
          UserName: '机器人',
          senderType: 'bot',
          Platform: 'qq-bot',
          ...(currentConversation.private
            ? { UserId: currentEvent?.UserId, OpenId: currentEvent?.OpenId }
            : {
                ChannelId: currentEvent?.ChannelId,
                GuildId: currentEvent?.GuildId,
                SpaceId: currentEvent?.SpaceId
              }),
          context: sourceEvent,
          delivery: 'sending'
        }
      ])
      pendingRef.current[requestID].messageID = requestID
      setText('')
      setDrafts(current => {
        const next = { ...current }
        delete next[selected]
        return next
      })
      setAttachment(null)
      setError('')
    } catch (reason) {
      if (upload) void cleanupUpload(upload)
      setError(reason instanceof Error ? reason.message : '文件发送准备失败。')
    }
  }, [
    attachment,
    callAction,
    cleanupUpload,
    contacts,
    currentConversation,
    currentEvent,
    isQQ,
    selected,
    text,
    uploadFile
  ])

  const deleteMessage = useCallback(
    (event: LiveEvent) => {
      if (!isQQ || !event.serverMessageID || !event.context) return
      const target = qqTarget(event.context)
      setConfirmation({
        title: '确认撤回消息',
        message: `将撤回 QQ 消息 ${event.serverMessageID}。撤回后对方将无法继续看到该消息。`,
        confirmLabel: '撤回消息',
        destructive: true,
        onConfirm: () =>
          callAction(
            'message.delete',
            { event: event.context, target, MessageId: event.serverMessageID },
            {
              kind: 'delete',
              action: 'message.delete',
              messageID: event.MessageId,
              target: `${scopeName(target.scope)} ${target.targetId}`
            }
          )
      })
    },
    [callAction, isQQ]
  )

  const decorateMessage = useCallback((event: LiveEvent, action: 'message.pin' | 'message.unpin' | 'reaction.add' | 'reaction.remove', emoji = '👍') => {
    if (!isQQ || !event.serverMessageID || !event.context || qqTarget(event.context).scope !== 'channel') return
    const target = qqTarget(event.context)
    const input = action.startsWith('reaction.')
      ? { event: event.context, target, MessageId: event.serverMessageID, EmojiId: emoji }
      : { event: event.context, target, MessageId: event.serverMessageID }
    callAction(action, input, {
      kind: 'tool', action, target: `${scopeName(target.scope)} ${target.targetId}`,
      onResult: (_results, state) => {
        if (state !== 'success') return
        setDecorations(current => {
          const previous = current[event.MessageId || event.serverMessageID || ''] || {}
          const reactions = new Set(previous.reactions || [])
          if (action === 'reaction.add') reactions.add(emoji)
          if (action === 'reaction.remove') reactions.delete(emoji)
          return { ...current, [event.MessageId || event.serverMessageID || '']: { pinned: action === 'message.pin' ? true : action === 'message.unpin' ? false : previous.pinned, reactions: [...reactions] } }
        })
      }
    })
  }, [callAction, isQQ])

  const notifyTyping = useCallback((value: string) => {
    if (!value || !currentEvent || !isQQ || qqTarget(currentEvent).scope !== 'c2c' || typing || socket.current?.readyState !== WebSocket.OPEN) return
    setTyping(true)
    const target = qqTarget(currentEvent)
    callAction('message.input.notify', { event: currentEvent.context ?? currentEvent, target, params: { input_type: 1, input_second: 5 } }, { kind: 'typing', action: 'message.input.notify', target: `${scopeName(target.scope)} ${target.targetId}`, onResult: (_results, state, summary) => { if (state !== 'success') setError(`输入状态未能同步：${summary}`) } })
    if (typingTimer.current) window.clearTimeout(typingTimer.current)
    typingTimer.current = window.setTimeout(() => setTyping(false), 4_000)
  }, [callAction, currentEvent, isQQ, typing])

  const executeToolConfirmed = useCallback(async () => {
    if (!activeDefinition) return
    const uploads: Upload[] = []
    const values = { ...formValues }
    try {
      for (const field of activeDefinition.fields.filter(
        field => field.kind === 'file'
      )) {
        const localFile = values[field.key]
        if (!(localFile instanceof File)) continue
        const upload = await uploadFile(localFile)
        uploads.push(upload)
        if (activeDefinition.id.startsWith('file.send.')) {
          values.file_path = upload.path
          values.file_data = await fileData(localFile)
        } else if (activeDefinition.id === 'media.send.user')
          values.data = await fileData(localFile)
        else if (activeDefinition.id === 'media.upload.chunked')
          values.file_path = upload.path
        else values.filePath = upload.path
      }
      if (
        uploadActions.has(activeDefinition.id) &&
        !uploads.length &&
        !values.url
      ) {
        setError('请提供本地文件或 URL。')
        return
      }
      const input = valuesToInput(activeDefinition, values, currentEvent)
      const target = currentEvent ? qqTarget(currentEvent) : undefined
      const requestID = callAction(activeDefinition.id, input, {
        kind: 'tool',
        action: activeDefinition.id,
        uploads,
        target: target
          ? `${scopeName(target.scope)} ${target.targetId}`
          : '全局'
      })
      if (!requestID) return
      setError('')
    } catch (reason) {
      uploads.forEach(upload => void cleanupUpload(upload))
      setError(reason instanceof Error ? reason.message : '操作准备失败。')
    }
  }, [
    activeDefinition,
    callAction,
    cleanupUpload,
    currentEvent,
    formValues,
    uploadFile
  ])

  const executeTool = useCallback(async () => {
    if (!activeDefinition) return
    const hasContext =
      activeDefinition.scopes.includes('global') || (isQQ && currentEvent)
    if (!hasContext) {
      setError('请先选择 QQ 会话；该工具需要当前会话上下文。')
      return
    }
    for (const field of activeDefinition.fields)
      if (field.required && !formValues[field.key]) {
        setError(`请填写「${field.label}」。`)
        return
      }
    if (activeDefinition.risk !== 'high') {
      await executeToolConfirmed()
      return
    }
    const target = currentEvent
      ? `${scopeName(qqTarget(currentEvent).scope)} ${qqTarget(currentEvent).targetId}`
      : '机器人全局设置'
    setConfirmation({
      title: `确认执行「${activeDefinition.title}」`,
      message: `目标：${target}\n此操作会直接影响 QQ 平台，必须逐次确认。`,
      confirmLabel: '确认执行',
      destructive: true,
      onConfirm: () => void executeToolConfirmed()
    })
  }, [activeDefinition, currentEvent, executeToolConfirmed, formValues, isQQ])

  const chooseAction = useCallback(
    (action: QQActionDefinition) => {
      setActiveAction(action.id)
      setFormValues(initialValues(action, currentEvent))
      setError('')
    },
    [currentEvent]
  )

  const groups = useMemo(
    () => [...new Set(qqActionCatalog.map(item => item.group))],
    []
  )
  const busy = Object.values(pending).some(item => item.kind === 'tool')

  const refreshProfile = useCallback(() => {
    if (!currentEvent || !isQQ) {
      setProfile({
        status: 'unavailable',
        message: '请先选择一个 QQ 会话后再读取资料。'
      })
      return
    }
    const scope = qqTarget(currentEvent).scope
    if (scope === 'c2c') {
      setProfile({
        status: 'unavailable',
        message: '当前 QQ Bot Action 未提供可读取私聊对方资料的接口。'
      })
      return
    }
    const action = scope === 'group' ? 'group.info' : 'channel.info'
    const definition = qqActionCatalog.find(item => item.id === action)
    if (!definition) {
      setProfile({
        status: 'unavailable',
        message: '当前适配器未注册此会话的资料读取能力。'
      })
      return
    }
    setProfile({ status: 'loading' })
    const requestID = callAction(
      action,
      valuesToInput(
        definition,
        initialValues(definition, currentEvent),
        currentEvent
      ),
      {
        kind: 'profile',
        action,
        target: `${scopeName(scope)} ${qqTarget(currentEvent).targetId}`,
        onResult: (results, nextState, summary) => {
          if (nextState === 'success') {
            const target = qqTarget(currentEvent)
            const hydrated = spaceIdentity(results[0]?.data, {
              id: `channel:${target.scope}:${target.targetId}`,
              label:
                currentEvent.ChannelName ||
                `${scopeName(target.scope)} ${target.targetId}`,
              avatar: currentEvent.ChannelAvatar,
              scope: target.scope as QQSpace['scope'],
              source: 'conversation',
              updatedAt: Date.now()
            })
            setSpaces(current =>
              mergeDirectory(
                current,
                [hydrated],
                item => item.id,
                item => item.updatedAt
              )
            )
            setProfile({
              status: 'ready',
              data: results[0]?.data,
              updatedAt: Date.now()
            })
            return
          }
          setProfile({
            status: nextState === 'timeout' ? 'failed' : 'unavailable',
            message: summary,
            updatedAt: Date.now()
          })
        }
      }
    )
    if (!requestID)
      setProfile({ status: 'failed', message: '资料请求未能发出。' })
  }, [callAction, currentEvent, isQQ])

  const refreshMembers = useCallback(() => {
    if (!currentEvent || !isQQ) {
      setMembers({
        status: 'unavailable',
        message: '请先选择 QQ 群或频道会话。'
      })
      return
    }
    const scope = qqTarget(currentEvent).scope
    const fallbackToKnownSpeakers = (message: string) => {
      const known = new Map<string, Record<string, unknown>>()
      for (const event of messages) {
        if (isBotMessage(event)) continue
        const userID = eventUserID(event)
        if (!userID) continue
        known.set(userID, {
          user_id: userID,
          nick: event.UserName || userID,
          avatar: event.UserAvatar
        })
      }
      const listed = [...known.values()]
      setMembers({
        status: 'ready',
        data: listed,
        message,
        updatedAt: Date.now()
      })
      if (listed.length)
        setContacts(current =>
          mergeDirectory(
            current,
            listed.map(item => ({
              id: `user:${String(item.user_id)}`,
              label: String(item.nick),
              avatar: typeof item.avatar === 'string' ? item.avatar : undefined,
              source: 'conversation' as const,
              lastInteractionAt: Date.now()
            })),
            item => item.id,
            item => item.lastInteractionAt
          )
        )
    }
    const definition = qqActionCatalog.find(item => item.id === 'member.list')
    if (!definition || !isActionAvailable(definition, scope)) {
      fallbackToKnownSpeakers('已根据当前会话的已知发言者生成成员列表。')
      return
    }
    setMembers({ status: 'loading' })
    const requestID = callAction(
      definition.id,
      valuesToInput(
        definition,
        initialValues(definition, currentEvent),
        currentEvent
      ),
      {
        kind: 'profile',
        action: definition.id,
        target: `${scopeName(scope)} ${qqTarget(currentEvent).targetId}`,
        onResult: (results, nextState, summary) => {
          if (nextState !== 'success') {
            setMembers({
              status: nextState === 'timeout' ? 'failed' : 'unavailable',
              message: summary,
              updatedAt: Date.now()
            })
            return
          }
          const listed = resultItems(results[0]?.data)
          setMembers({ status: 'ready', data: listed, updatedAt: Date.now() })
          const directory = collectActionDirectory(
            'member.list',
            results[0]?.data
          )
          if (directory.contacts.length)
            setContacts(current =>
              mergeDirectory(
                current,
                directory.contacts,
                item => item.id,
                item => item.lastInteractionAt
              )
            )
        }
      }
    )
    if (!requestID)
      setMembers({ status: 'failed', message: '成员列表请求未能发出。' })
  }, [callAction, currentEvent, isQQ, messages])

  const refreshUserProfile = useCallback(
    (event: LiveEvent) => {
      const userID = eventUserID(event)
      if (!userID || isBotMessage(event)) return
      const scope = currentEvent && isQQ ? qqTarget(currentEvent).scope : ''
      if (scope === 'group') {
        const definition = qqActionCatalog.find(
          item => item.id === 'group.member.info'
        )
        if (!definition || !currentEvent) return
        setUserProfile({
          status: 'loading',
          label: event.UserName || userID,
          userID
        })
        const requestID = callAction(
          definition.id,
          valuesToInput(definition, { memberOpenId: userID }, currentEvent),
          {
            kind: 'profile',
            action: definition.id,
            target: `群成员 ${event.UserName || userID}`,
            onResult: (results, nextState, summary) => {
              if (nextState === 'success') {
                const identity = profileIdentity(results[0]?.data, {
                  name: event.UserName || userID,
                  avatar: event.UserAvatar
                })
                setContacts(current =>
                  mergeDirectory(
                    current,
                    [{ id: `user:${userID}`, label: identity.name, avatar: identity.avatar, source: 'member', lastInteractionAt: Date.now() }],
                    item => item.id,
                    item => item.lastInteractionAt
                  )
                )
                setUserProfile({ status: 'ready', data: results[0]?.data, label: identity.name, userID, updatedAt: Date.now() })
                return
              }
              setUserProfile({ status: nextState === 'timeout' ? 'failed' : 'unavailable', message: summary, label: event.UserName || userID, userID, updatedAt: Date.now() })
            }
          }
        )
        if (!requestID)
          setUserProfile({
            status: 'failed',
            message: '成员资料请求未能发出。',
            label: event.UserName || userID,
            userID
          })
        return
      }
      if (scope === 'channel' || scope === 'direct') {
        const definition = qqActionCatalog.find(
          item => item.id === 'member.info'
        )
        if (!definition || !currentEvent) return
        setUserProfile({
          status: 'loading',
          label: event.UserName || userID,
          userID
        })
        const requestID = callAction(
          definition.id,
          valuesToInput(definition, { userId: userID }, currentEvent),
          {
            kind: 'profile',
            action: definition.id,
            target: `频道成员 ${event.UserName || userID}`,
            onResult: (results, nextState, summary) => {
              if (nextState === 'success') {
                const identity = profileIdentity(results[0]?.data, {
                  name: event.UserName || userID,
                  avatar: event.UserAvatar
                })
                setContacts(current =>
                  mergeDirectory(
                    current,
                    [{ id: `user:${userID}`, label: identity.name, avatar: identity.avatar, source: 'member', lastInteractionAt: Date.now() }],
                    item => item.id,
                    item => item.lastInteractionAt
                  )
                )
                setUserProfile({ status: 'ready', data: results[0]?.data, label: identity.name, userID, updatedAt: Date.now() })
                return
              }
              setUserProfile({ status: nextState === 'timeout' ? 'failed' : 'unavailable', message: summary, label: event.UserName || userID, userID, updatedAt: Date.now() })
            }
          }
        )
        if (!requestID)
          setUserProfile({
            status: 'failed',
            message: '成员资料请求未能发出。',
            label: event.UserName || userID,
            userID
          })
        return
      }
      setUserProfile({
        status: 'unavailable',
        message: '当前 QQ Bot Action 未提供私聊对方的完整资料读取接口。',
        label: event.UserName || userID,
        userID
      })
    },
    [callAction, currentEvent, isQQ]
  )

  const refreshChannelList = useCallback(() => {
    if (!currentEvent || !isQQ || !['channel', 'direct'].includes(qqTarget(currentEvent).scope)) return
    const definition = qqActionCatalog.find(item => item.id === 'channel.list')
    if (!definition) return
    callAction(definition.id, valuesToInput(definition, initialValues(definition, currentEvent), currentEvent), { kind: 'profile', action: definition.id, target: `${scopeName(qqTarget(currentEvent).scope)} ${qqTarget(currentEvent).targetId}` })
  }, [callAction, currentEvent, isQQ])

  const manageMember = useCallback((userID: string, actionID: 'member.info' | 'member.mute' | 'member.ban' | 'member.kick' | 'permission.get' | 'permission.set' | 'role.assign' | 'role.remove') => {
    const definition = qqActionCatalog.find(item => item.id === actionID)
    if (!definition) return
    setActiveAction(actionID)
    setFormValues({ ...initialValues(definition, currentEvent), UserId: userID, userId: userID })
    setActiveNav('tools')
    setRightOpen(true)
  }, [currentEvent])

  const manageRole = useCallback((roleID: string, actionID: 'role.create' | 'role.update' | 'role.delete') => {
    const definition = qqActionCatalog.find(item => item.id === actionID)
    if (!definition) return
    setActiveAction(actionID)
    setFormValues({ ...initialValues(definition, currentEvent), ...(roleID ? { RoleId: roleID } : {}) })
    setActiveNav('tools')
    setRightOpen(true)
  }, [currentEvent])

  const refreshRoles = useCallback(() => {
    if (!currentEvent || !isQQ || !['channel', 'direct'].includes(qqTarget(currentEvent).scope)) return
    const definition = qqActionCatalog.find(item => item.id === 'role.list')
    if (!definition) return
    setRoles({ status: 'loading' })
    const requestID = callAction(definition.id, valuesToInput(definition, initialValues(definition, currentEvent), currentEvent), { kind: 'profile', action: definition.id, target: `${scopeName(qqTarget(currentEvent).scope)} ${qqTarget(currentEvent).targetId}`, onResult: (results, nextState, summary) => setRoles(nextState === 'success' ? { status: 'ready', data: results[0]?.data, updatedAt: Date.now() } : { status: nextState === 'timeout' ? 'failed' : 'unavailable', message: summary, updatedAt: Date.now() }) })
    if (!requestID) setRoles({ status: 'failed', message: '身份组列表请求未能发出。' })
  }, [callAction, currentEvent, isQQ])

  const refreshGroupManagement = useCallback(() => {
    if (!currentEvent || !isQQ || qqTarget(currentEvent).scope !== 'group') return
    const request = (actionID: 'group.botState' | 'group.mute.setting' | 'group.joinRequest.list', setState: React.Dispatch<React.SetStateAction<ProfileState>>) => {
      const definition = qqActionCatalog.find(item => item.id === actionID)
      if (!definition) return
      setState({ status: 'loading' })
      callAction(actionID, valuesToInput(definition, initialValues(definition, currentEvent), currentEvent), { kind: 'profile', action: actionID, target: `群 ${qqTarget(currentEvent).targetId}`, onResult: (results, nextState, summary) => setState(nextState === 'success' ? { status: 'ready', data: results[0]?.data, updatedAt: Date.now() } : { status: nextState === 'timeout' ? 'failed' : 'unavailable', message: summary, updatedAt: Date.now() }) })
    }
    request('group.botState', setGroupBotState)
    request('group.mute.setting', setGroupMuteSetting)
    request('group.joinRequest.list', setJoinRequests)
  }, [callAction, currentEvent, isQQ])

  const manageJoinRequest = useCallback((item: Record<string, unknown>, approve: boolean) => {
    const definition = qqActionCatalog.find(action => action.id === 'group.joinRequest.approve')
    if (!definition) return
    setActiveAction(definition.id)
    setFormValues({ ...initialValues(definition, currentEvent), memberOpenId: recordText(item, ['member_openid', 'user_id', 'userId', 'openid']), joinRequestId: recordText(item, ['join_request_id', 'joinRequestId', 'id']), op: approve ? 'approve' : 'decline' })
    setActiveNav('tools')
    setRightOpen(true)
  }, [currentEvent])

  const manageChannel = useCallback((channelID: string, actionID: 'channel.create' | 'channel.update' | 'channel.delete' | 'channel.announce') => {
    const definition = qqActionCatalog.find(item => item.id === actionID)
    if (!definition) return
    setActiveAction(actionID)
    setFormValues({ ...initialValues(definition, currentEvent), ChannelId: channelID, channelId: channelID })
    setActiveNav('tools')
    setRightOpen(true)
  }, [currentEvent])

  useEffect(() => {
    setProfile({ status: 'idle' })
    setMembers({ status: 'idle' })
    setRoles({ status: 'idle' })
    setGroupBotState({ status: 'idle' })
    setGroupMuteSetting({ status: 'idle' })
    setJoinRequests({ status: 'idle' })
    setUserProfile({ status: 'idle' })
    if (currentEvent && isQQ) refreshMembers()
    if (preferences.autoProfile && currentEvent && isQQ) {
      refreshProfile()
      refreshRoles()
      refreshGroupManagement()
      refreshChannelList()
      const user = messages.find(
        event => !isBotMessage(event) && eventUserID(event)
      )
      if (user) refreshUserProfile(user)
    }
    // Selecting a conversation is the deliberate refresh boundary. Incoming
    // messages must not repeatedly issue profile reads.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, preferences.autoProfile])

  const toggleFavorite = useCallback(
    (event: LiveEvent) => {
      if (!currentConversation) return
      const messageId =
        event.MessageId || `${event.CreateAt || Date.now()}:${eventText(event)}`
      setFavorites(current => {
        const existing = current.find(
          item =>
            item.conversationId === currentConversation.id &&
            item.messageId === messageId
        )
        if (existing) return current.filter(item => item !== existing)
        return [
          {
            id: crypto.randomUUID(),
            conversationId: currentConversation.id,
            messageId,
            text: eventText(event).slice(0, 160) || '非文本消息',
            createdAt: Date.now(),
            expiresAt: favoriteExpiry(preferences.favoriteRetention)
          },
          ...current
        ]
      })
    },
    [currentConversation, preferences.favoriteRetention]
  )
  const openContactConversation = useCallback(
    (contact: QQContact) => {
      const conversation = makeDirectConversation(contact)
      setOpenedConversationIDs(current =>
        current.includes(conversation.id) ? current : [...current, conversation.id]
      )
      setOpenedConversations(current =>
        mergeDirectory(
          current,
          [conversation],
          item => item.id,
          item => item.updatedAt
        )
      )
      setSelected(conversation.id)
      setText(drafts[conversation.id] || '')
      setActiveNav('messages')
      if (window.matchMedia('(max-width: 759px)').matches) setRightOpen(false)
    },
    [drafts]
  )
  const openSpaceConversation = useCallback(
    (space: QQSpace) => {
      const conversation = makeSpaceConversation(space)
      setOpenedConversations(current =>
        mergeDirectory(
          current,
          [conversation],
          item => item.id,
          item => item.updatedAt
        )
      )
      setSelected(conversation.id)
      setText(drafts[conversation.id] || '')
      setActiveNav('messages')
      if (window.matchMedia('(max-width: 759px)').matches) setRightOpen(false)
    },
    [drafts]
  )

  const clearAudit = useCallback(() => setTools([]), [])
  const clearHistory = useCallback(() => {
    if (
      !window.confirm(
        '确认清除当前机器人在本机保存的聊天记录、草稿、联系人、群频道目录和收藏吗？不会影响 QQ 平台消息。'
      )
    )
      return
    clearQQChatStore(root)
    setEvents([])
    setDrafts({})
    setFavorites([])
    setContacts([])
    setSpaces([])
    setOpenedConversationIDs([])
    setOpenedConversations([])
    setSelected('')
  }, [root])
  const resetLayout = useCallback(() => {
    resetQQChatWindowLayout(root)
    setRightOpen(true)
    setActiveNav('messages')
    setPreferences(current => ({
      ...current,
      density: 'comfortable',
      fontSize: 'medium',
      autoProfile: true,
      rightPanelOpen: true,
      activeNav: 'messages'
    }))
  }, [root])
  const runDirectoryRead = useCallback(
    (actionID: 'me.guilds' | 'guild.list') => {
      const definition = qqActionCatalog.find(item => item.id === actionID)
      if (!definition) return
      const requestID = callAction(
        actionID,
        valuesToInput(definition, initialValues(definition), undefined),
        { kind: 'tool', action: actionID, target: '全局目录' }
      )
      if (requestID) setError('')
    },
    [callAction]
  )

  useEffect(() => {
    if (state !== 'connected') return
    if (activeNav === 'spaces') {
      runDirectoryRead('me.guilds')
      runDirectoryRead('guild.list')
      return
    }
    if (
      activeNav === 'contacts' &&
      (currentScope === 'channel' || currentScope === 'direct')
    )
      refreshMembers()
  }, [activeNav, currentScope, refreshMembers, runDirectoryRead, state])

  return (
    <>
      <QQ9ChatShell
        activeNav={activeNav}
        setActiveNav={setActiveNav}
        rightOpen={rightOpen}
        setRightOpen={setRightOpen}
        search={search}
        setSearch={setSearch}
        state={state}
        error={error}
        reconnect={reconnect}
        conversations={allConversations}
        selected={selected}
        selectConversation={id => {
          setSelected(id)
          setText(drafts[id] || '')
          setActiveNav('messages')
        }}
        currentConversation={currentConversation}
        currentEvent={currentEvent}
        currentScope={currentScope}
        messages={messages}
        isQQ={isQQ}
        text={text}
      setText={value => {
        setText(value)
        setDrafts(current => ({ ...current, [selected]: value }))
        notifyTyping(value)
      }}
        attachment={attachment}
        setAttachment={setAttachment}
        attachmentInput={attachmentInput}
        send={send}
      deleteMessage={deleteMessage}
      decorateMessage={decorateMessage}
      decorations={decorations}
        toggleFavorite={toggleFavorite}
        favorites={favorites}
        contacts={contacts}
        spaces={spaces}
        preferences={preferences}
        setPreferences={setPreferences}
        profile={profile}
        refreshProfile={refreshProfile}
      members={members}
      refreshMembers={refreshMembers}
      botProfile={botProfile}
      botIdentity={botIdentity}
      userProfile={userProfile}
      refreshUserProfile={refreshUserProfile}
      manageMember={manageMember}
      manageChannel={manageChannel}
      roles={roles}
      refreshRoles={refreshRoles}
      manageRole={manageRole}
      groupBotState={groupBotState}
      groupMuteSetting={groupMuteSetting}
      joinRequests={joinRequests}
      manageJoinRequest={manageJoinRequest}
      groups={groups}
        showUnavailable={showUnavailable}
        setShowUnavailable={setShowUnavailable}
        activeDefinition={activeDefinition}
        activeAction={activeAction}
        chooseAction={chooseAction}
        currentScopeForActions={currentScope}
        formValues={formValues}
        setFormValues={setFormValues}
        busy={busy}
        executeTool={executeTool}
        tools={tools}
        clearAudit={clearAudit}
        clearHistory={clearHistory}
        resetLayout={resetLayout}
        runDirectoryRead={runDirectoryRead}
        openContactConversation={openContactConversation}
        openSpaceConversation={openSpaceConversation}
      />
      <ConfirmDialog
        open={Boolean(confirmation)}
        title={confirmation?.title || ''}
        message={confirmation?.message || ''}
        confirmLabel={confirmation?.confirmLabel}
        destructive={confirmation?.destructive}
        onCancel={() => setConfirmation(null)}
        onConfirm={() => {
          const current = confirmation
          setConfirmation(null)
          current?.onConfirm()
        }}
      />
    </>
  )
}

type QQ9ShellProps = {
  activeNav:
    | 'messages'
    | 'contacts'
    | 'spaces'
    | 'favorites'
    | 'profile'
    | 'tools'
    | 'audit'
    | 'settings'
  setActiveNav: (
    value:
      | 'messages'
      | 'contacts'
      | 'spaces'
      | 'favorites'
      | 'profile'
      | 'tools'
      | 'audit'
      | 'settings'
  ) => void
  rightOpen: boolean
  setRightOpen: (value: boolean) => void
  search: string
  setSearch: (value: string) => void
  state: 'connecting' | 'connected' | 'failed'
  error: string
  reconnect: () => void
  conversations: Conversation[]
  selected: string
  selectConversation: (id: string) => void
  currentConversation?: Conversation
  currentEvent?: LiveEvent
  currentScope: QQScope | ''
  messages: LiveEvent[]
  isQQ: boolean
  text: string
  setText: (value: string) => void
  attachment: File | null
  setAttachment: (value: File | null) => void
  attachmentInput: React.RefObject<HTMLInputElement | null>
  send: () => Promise<void>
  deleteMessage: (event: LiveEvent) => void
  decorateMessage: (event: LiveEvent, action: 'message.pin' | 'message.unpin' | 'reaction.add' | 'reaction.remove', emoji?: string) => void
  decorations: Record<string, MessageDecoration>
  toggleFavorite: (event: LiveEvent) => void
  favorites: QQFavorite[]
  contacts: QQContact[]
  spaces: QQSpace[]
  preferences: QQChatPreferences
  setPreferences: React.Dispatch<React.SetStateAction<QQChatPreferences>>
  profile: ProfileState
  refreshProfile: () => void
  members: ProfileState
  refreshMembers: () => void
  botProfile: UserProfileState
  botIdentity: QQIdentity
  userProfile: UserProfileState
  refreshUserProfile: (event: LiveEvent) => void
  manageMember: (userID: string, actionID: 'member.info' | 'member.mute' | 'member.ban' | 'member.kick' | 'permission.get' | 'permission.set' | 'role.assign' | 'role.remove') => void
  manageChannel: (channelID: string, actionID: 'channel.create' | 'channel.update' | 'channel.delete' | 'channel.announce') => void
  roles: ProfileState
  refreshRoles: () => void
  manageRole: (roleID: string, actionID: 'role.create' | 'role.update' | 'role.delete') => void
  groupBotState: ProfileState
  groupMuteSetting: ProfileState
  joinRequests: ProfileState
  manageJoinRequest: (item: Record<string, unknown>, approve: boolean) => void
  groups: string[]
  showUnavailable: boolean
  setShowUnavailable: (value: boolean) => void
  activeDefinition?: QQActionDefinition
  activeAction: string
  chooseAction: (action: QQActionDefinition) => void
  currentScopeForActions: QQScope | ''
  formValues: Record<string, unknown>
  setFormValues: React.Dispatch<React.SetStateAction<Record<string, unknown>>>
  busy: boolean
  executeTool: () => Promise<void>
  tools: ToolRecord[]
  clearAudit: () => void
  clearHistory: () => void
  resetLayout: () => void
  runDirectoryRead: (action: 'me.guilds' | 'guild.list') => void
  openContactConversation: (contact: QQContact) => void
  openSpaceConversation: (space: QQSpace) => void
}

function QQ9ChatShell(props: QQ9ShellProps) {
  const { rightOpen, setRightOpen } = props
  const [profileOpen, setProfileOpen] = useState(false)
  const [botCardOpen, setBotCardOpen] = useState(false)
  const [conversationInfoOpen, setConversationInfoOpen] = useState(false)
  const messageViewport = useRef<HTMLElement | null>(null)
  const followNewest = useRef(true)
  const conversationID = props.currentConversation?.id

  // Follow incoming messages only while the reader is still close to the
  // bottom.  Once they scroll into history, preserve that reading position.
  useLayoutEffect(() => {
    const viewport = messageViewport.current
    if (!viewport || !conversationID) return
    viewport.scrollTop = viewport.scrollHeight
    followNewest.current = true
  }, [conversationID])

  useLayoutEffect(() => {
    const viewport = messageViewport.current
    if (!viewport || !conversationID || !followNewest.current) return
    viewport.scrollTop = viewport.scrollHeight
  }, [conversationID, props.messages.length])

  const filteredConversations = props.conversations.filter(item =>
    `${item.label} ${eventText(item.event)}`
      .toLowerCase()
      .includes(props.search.toLowerCase())
  )
  const visibleFavorites = props.favorites.filter(
    item => !item.expiresAt || item.expiresAt > Date.now()
  )
  const primaryNav = [
    ['messages', '消息', MessageCircle],
    ['contacts', '联系人', ContactRound],
    ['spaces', '群/频道', UsersRound],
    ['favorites', '收藏', Heart]
  ] as const
  const workNav = [
    ['tools', '机器人', Bot]
  ] as const
  const density = props.preferences.density === 'compact' ? 'py-2' : 'py-3'
  const font =
    props.preferences.fontSize === 'small'
      ? 'text-xs'
      : props.preferences.fontSize === 'large'
        ? 'text-base'
        : 'text-sm'
  const favoriteFor = (event: LiveEvent) =>
    props.favorites.some(
      item =>
        item.conversationId === props.currentConversation?.id &&
        item.messageId ===
          (event.MessageId || `${event.CreateAt || 0}:${eventText(event)}`)
    )
  const title = (
    {
      messages: '消息',
      contacts: '联系人',
      spaces: '群 / 频道',
      favorites: '收藏',
      profile: '会话资料',
      tools: '机器人能力',
      audit: '操作记录',
      settings: '聊天设置'
    } as Record<string, string>
  )[props.activeNav]
  const chooseConversation = (id: string) => {
    props.selectConversation(id)
    setProfileOpen(false)
    if (window.matchMedia('(max-width: 759px)').matches)
      props.setRightOpen(false)
  }
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || !rightOpen) return
      setRightOpen(false)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [rightOpen, setRightOpen])
  const profilePanel = (
    <QQ9Profile
      event={props.currentEvent}
      members={props.members}
      openContact={props.openContactConversation}
      mentionContact={contact => props.setText(`${props.text}${props.text && !props.text.endsWith(' ') ? ' ' : ''}@${contact.label} `)}
    />
  )
  return (
    <div
      className={`qq-live-shell qq-live-sidebar-${props.rightOpen ? 'open' : 'closed'} relative grid h-full min-h-0 flex-1 overflow-hidden bg-(--theme-surface-panel) text-(--theme-text-primary) [grid-template-columns:56px_272px_minmax(360px,1fr)] ${props.rightOpen ? '' : '[grid-template-columns:56px_minmax(360px,1fr)]'}`}
      style={
        profileOpen
          ? {
              gridTemplateColumns: props.rightOpen
                ? '56px 272px minmax(0, 1fr) 320px'
                : '56px minmax(0, 1fr) 320px'
            }
          : undefined
      }
    >
      <nav
        className="qq-live-nav relative z-40 flex min-h-0 flex-col items-center gap-3.5 border-r border-(--theme-border-default) bg-(--theme-surface-raised) px-1.5 py-3"
        aria-label="在线聊天导航"
      >
        <div className="grid w-full gap-1.5">
          {primaryNav.map(([id, label, Icon]) => (
            <button
              key={id}
              className={`flex h-11 w-full flex-col items-center justify-center gap-0.5 rounded-[10px] border-0 bg-transparent text-[9px] text-(--theme-text-muted) transition hover:bg-(--theme-accent-soft) hover:text-(--theme-accent-text) ${props.activeNav === id ? 'bg-(--theme-accent-soft) text-(--theme-accent-text)' : ''}`}
              aria-label={label}
              aria-current={props.activeNav === id ? 'page' : undefined}
              onClick={() => {
                props.setActiveNav(id)
                props.setRightOpen(true)
              }}
              title={label}
            >
              <Icon className="size-[19px]" />
              <span>{label}</span>
            </button>
          ))}
        </div>
        <div className="mt-0.5 grid w-full gap-1.5 border-t border-(--theme-border-default) pt-2.5">
          {workNav.map(([id, label, Icon]) => (
            <button
              key={id}
              className={`flex h-10 w-full flex-col items-center justify-center gap-0.5 rounded-[9px] border-0 bg-transparent text-[9px] text-(--theme-text-muted) transition hover:bg-(--theme-accent-soft) hover:text-(--theme-accent-text) ${props.activeNav === id ? 'bg-(--theme-accent-soft) text-(--theme-accent-text)' : ''}`}
              aria-label={label}
              aria-current={props.activeNav === id ? 'page' : undefined}
              onClick={() => {
                props.setActiveNav(id)
                props.setRightOpen(true)
              }}
              title={label}
            >
              <Icon className="size-[18px]" />
              <span>{label}</span>
            </button>
          ))}
        </div>
        <div className="mt-auto grid w-full gap-1.5">
          <div className="relative justify-self-center">
            <button
              className="inline-flex size-10 items-center justify-center overflow-hidden rounded-full border border-(--theme-border-default) bg-(--theme-surface-active) text-(--theme-text-secondary) transition hover:border-(--theme-accent-soft-border) hover:bg-(--theme-accent-soft) [&_img]:size-full [&_img]:object-cover"
              aria-label="查看当前机器人资料"
              aria-expanded={botCardOpen}
              title={`当前机器人：${props.botIdentity.name}`}
              onClick={() => setBotCardOpen(open => !open)}
            >
              {props.botIdentity.avatar ? (
                <img src={props.botIdentity.avatar} alt="" />
              ) : (
                <Bot className="size-5" aria-hidden="true" />
              )}
            </button>
            <span
              className={`absolute -top-0.5 -right-0.5 size-3 rounded-full border-2 border-(--theme-surface-raised) ${props.state === 'connected' ? 'bg-emerald-500' : props.state === 'connecting' ? 'animate-pulse bg-amber-500' : 'bg-rose-500'}`}
              role="status"
              aria-label={props.state === 'connected' ? 'QQ Bot 已连接' : props.state === 'connecting' ? '正在连接 QQ Bot' : 'QQ Bot 已断开'}
              title={props.state === 'connected' ? 'QQ Bot 已连接' : props.state === 'connecting' ? '正在连接 QQ Bot' : 'QQ Bot 已断开'}
            />
          </div>
          {botCardOpen && (
            <section
              className="absolute bottom-3 left-[calc(100%+8px)] w-60 rounded-xl border border-(--theme-border-strong) bg-(--theme-surface-panel) p-3 shadow-(--theme-shadow-pop)"
              aria-label="当前机器人资料"
            >
              <header className="flex items-center gap-2">
                <span className="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-(--theme-border-default) bg-(--theme-surface-active) [&_img]:size-full [&_img]:object-cover">
                  {props.botIdentity.avatar ? <img src={props.botIdentity.avatar} alt="" /> : <Bot className="size-4" />}
                </span>
                <span className="min-w-0 flex-1">
                  <strong className="block truncate text-xs">{props.botIdentity.name}</strong>
                  <small className="block text-[10px] text-(--theme-text-muted)">当前 QQ Bot</small>
                </span>
                <button className="inline-flex size-6 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover)" onClick={() => setBotCardOpen(false)} aria-label="关闭机器人资料"><X className="size-3.5" /></button>
              </header>
              <div className="mt-2 border-t border-(--theme-border-subtle) pt-2 text-[11px] text-(--theme-text-muted)">
                {props.botProfile.status === 'ready' ? <ProfileRows data={props.botProfile.data} /> : props.botProfile.status === 'loading' ? '正在读取机器人资料…' : props.botProfile.message || '机器人资料将在连接后自动读取。'}
              </div>
            </section>
          )}
          <button
            className={`flex h-10 w-full flex-col items-center justify-center gap-0.5 rounded-[9px] border-0 bg-transparent text-[9px] text-(--theme-text-muted) transition hover:bg-(--theme-accent-soft) hover:text-(--theme-accent-text) ${props.activeNav === 'settings' ? 'bg-(--theme-accent-soft) text-(--theme-accent-text)' : ''}`}
            aria-label="聊天设置"
            aria-current={props.activeNav === 'settings' ? 'page' : undefined}
            onClick={() => {
              props.setActiveNav('settings')
              props.setRightOpen(true)
            }}
            title="设置"
          >
            <Settings2 className="size-[18px]" />
            <span>设置</span>
          </button>
        </div>
      </nav>
      {props.rightOpen && (
        <button
          className="qq-live-backdrop"
          aria-label="关闭侧边栏"
          onClick={() => props.setRightOpen(false)}
        />
      )}
      <aside
        className="qq-live-sidebar flex gap-4 min-w-0 flex-col overflow-hidden border-r border-(--theme-border-default) bg-(--theme-surface-panel) px-2 py-3"
        aria-label={title}
      >
        {['messages', 'contacts', 'spaces', 'favorites'].includes(
          props.activeNav
        ) && (
          <div className="flex items-center gap-1.5 rounded-lg border border-(--theme-border-subtle) bg-(--theme-surface-raised) px-2 text-(--theme-text-muted) focus-within:border-(--theme-accent-soft-border) focus-within:bg-(--theme-surface-input)">
            <Search className="size-4 shrink-0" />
            <input
              className="min-w-0 flex-1 border-0 bg-transparent py-2 text-xs text-(--theme-text-primary) outline-none"
              value={props.search}
              onChange={event => props.setSearch(event.target.value)}
              placeholder={
                props.activeNav === 'messages' ? '搜索会话' : '搜索本机记录'
              }
            />
          </div>
        )}
        {props.activeNav === 'messages' ? (
          <>
            <div className="min-h-0 overflow-auto">
              {filteredConversations.map(item => (
                <button
                  key={item.id}
                  className={`flex min-h-13 w-full items-center gap-2 rounded-lg border-0 bg-transparent px-2 text-left transition-colors hover:bg-(--theme-surface-hover) ${density} ${props.selected === item.id ? 'bg-(--theme-accent-soft)' : ''}`}
                  onClick={() => chooseConversation(item.id)}
                >
                  <span className="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-(--theme-border-default) bg-(--theme-surface-active) text-[13px] font-bold text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover">
                    {item.avatar ? (
                      <img src={item.avatar} alt="" />
                    ) : (
                      item.label.slice(0, 1)
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 items-center gap-2">
                      <b
                        className="min-w-0 flex-1 truncate text-[13px] font-semibold"
                        title={item.label}
                      >
                        {item.label}
                      </b>
                      <time className="ml-auto shrink-0 text-[10px] text-(--theme-text-muted)">
                        {formatTime(item.updatedAt)}
                      </time>
                    </span>
                    <small
                      className="mt-0.5 block truncate text-[11px] text-(--theme-text-muted)"
                      title={
                        eventText(item.lastEvent || item.event) ||
                        scopeName(qqTarget(item.event).scope)
                      }
                    >
                      {eventText(item.lastEvent || item.event) ||
                        scopeName(qqTarget(item.event).scope)}
                    </small>
                  </span>
                </button>
              ))}
              {!filteredConversations.length ? (
                <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
                  QQ 消息到达后，会在本机建立会话。
                </p>
              ) : null}
            </div>
          </>
        ) : props.activeNav === 'contacts' ? (
          <QQ9Contacts
            contacts={props.contacts}
            search={props.search}
            openConversation={props.openContactConversation}
          />
        ) : props.activeNav === 'spaces' ? (
          <QQ9Spaces
            conversations={props.conversations}
            spaces={props.spaces}
            search={props.search}
            selectConversation={chooseConversation}
            openConversation={props.openSpaceConversation}
          />
        ) : props.activeNav === 'favorites' ? (
          <QQ9Favorites
            favorites={visibleFavorites}
            selectConversation={chooseConversation}
          />
        ) : props.activeNav === 'profile' ? (
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">
            {profilePanel}
          </div>
        ) : props.activeNav === 'tools' ? (
          <QQ9ToolCenter {...props} />
        ) : props.activeNav === 'audit' ? (
          <QQ9Audit tools={props.tools} clear={props.clearAudit} />
        ) : (
          <QQ9Settings
            preferences={props.preferences}
            setPreferences={props.setPreferences}
            clearHistory={props.clearHistory}
            resetLayout={props.resetLayout}
          />
        )}
      </aside>
      <main className="grid h-full min-h-0 min-w-0 overflow-hidden bg-(--theme-surface-panel) [grid-template-rows:auto_minmax(0,1fr)_auto]">
        <div className="border-b border-(--theme-border-default)">
          <header className="relative flex min-h-14.5 min-w-0 items-center justify-between px-5">
            <button
              className="flex min-w-0 items-center gap-2.5 rounded-md border-0 bg-transparent p-0 text-left text-inherit hover:bg-(--theme-surface-hover)"
              disabled={!props.currentEvent}
              aria-label="查看群资料"
              aria-expanded={conversationInfoOpen}
              title="查看群资料"
              onClick={() => setConversationInfoOpen(open => !open)}
            >
              <span className="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-(--theme-border-default) bg-(--theme-surface-active) text-xs font-bold text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover">
                {props.currentConversation?.avatar || props.currentEvent?.ChannelAvatar || props.currentEvent?.UserAvatar ? (
                  <img src={props.currentConversation?.avatar || props.currentEvent?.ChannelAvatar || props.currentEvent?.UserAvatar} alt="" />
                ) : (
                  (props.currentConversation?.label || 'Q').slice(0, 1)
                )}
              </span>
              <div className="min-w-0">
              <strong className="block truncate text-sm">
                {props.currentConversation?.label || '在线聊天'}
              </strong>
              <small className="mt-0.5 block text-[11px] text-(--theme-text-muted)">
                {props.currentEvent
                  ? scopeName(props.currentScope)
                  : '选择会话后可发送消息'}
              </small>
              </div>
            </button>
            {conversationInfoOpen && props.currentEvent ? (
              <section
                className="absolute top-[calc(100%+6px)] left-4 z-30 w-72 rounded-xl border border-(--theme-border-strong) bg-(--theme-surface-panel) p-3 shadow-(--theme-shadow-pop)"
                aria-label="群资料"
              >
                <div className="flex items-center gap-2">
                  <strong className="min-w-0 flex-1 truncate text-xs">
                    {props.currentConversation?.label || '当前会话'}
                  </strong>
                  <button
                    className="inline-flex size-6 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover)"
                    onClick={() => setConversationInfoOpen(false)}
                    aria-label="关闭群资料"
                  >
                    <X className="size-3.5" />
                  </button>
                </div>
                <div className="mt-2 border-t border-(--theme-border-subtle) pt-2 text-[11px] text-(--theme-text-muted)">
                  {props.profile.status === 'ready' ? (
                    <ProfileRows data={props.profile.data} />
                  ) : props.profile.status === 'loading' ? (
                    '正在读取群资料…'
                  ) : (
                    <div>
                      {props.profile.message || '群资料尚未读取。'}
                      <button
                        className="ml-1 rounded border border-(--theme-border-default) bg-(--theme-surface-raised) px-1 py-0.5 text-[10px] text-(--theme-accent-text)"
                        onClick={props.refreshProfile}
                      >
                        读取
                      </button>
                    </div>
                  )}
                </div>
              </section>
            ) : null}
            <div className="flex gap-1">
              {props.currentEvent && (
                <button
                  className="inline-flex size-7.5 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-text-secondary)"
                  title="会话资料"
                  aria-label="打开会话资料"
                  onClick={() => {
                    if (window.matchMedia('(max-width: 759px)').matches) {
                      props.setActiveNav('profile')
                      props.setRightOpen(true)
                    } else {
                      setProfileOpen(true)
                    }
                  }}
                >
                  <Bell className="size-4" />
                </button>
              )}
              <button
                className="inline-flex size-7.5 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-text-secondary)"
                title={props.rightOpen ? '收起侧边栏' : '展开当前能力'}
                aria-label={props.rightOpen ? '收起侧边栏' : '展开当前能力'}
                onClick={() => props.setRightOpen(!props.rightOpen)}
              >
                {props.rightOpen ? (
                  <X className="size-4" />
                ) : (
                  <MessageCircle className="size-4" />
                )}
              </button>
            </div>
          </header>
          {props.state !== 'connected' && (
            <div
              className={`mx-4 mt-3 flex items-center justify-between gap-3 rounded-lg border px-3 py-2 text-xs ${
                props.state === 'failed'
                  ? 'border-(--theme-danger) bg-(--theme-danger-soft) text-(--theme-danger-text)'
                  : 'border-(--theme-warning) bg-(--theme-warning-soft) text-(--theme-warning-text)'
              }`}
              role={props.state === 'failed' ? 'alert' : 'status'}
            >
              <div className="min-w-0">
                <b className="block">
                  {props.state === 'connecting'
                    ? '正在连接 QQ Bot…'
                    : 'QQ Bot 当前未连接'}
                </b>
                <span className="block pt-0.5 text-[11px] leading-relaxed">
                  {props.state === 'connecting'
                    ? '连接成功后即可发送消息和执行机器人操作。'
                    : props.error || '请确认机器人已登录并正在运行。'}
                </span>
              </div>
              {props.state === 'failed' && (
                <button
                  className="shrink-0 rounded-md border border-current bg-transparent px-2 py-1 text-[11px] font-medium"
                  onClick={props.reconnect}
                >
                  重新连接
                </button>
              )}
            </div>
          )}
        </div>
        <section
          ref={messageViewport}
          className={`min-h-0 overflow-auto overscroll-contain bg-(--theme-surface-page) px-[clamp(22px,5%,64px)] py-6.5 ${font}`}
          onScroll={event => {
            const viewport = event.currentTarget
            const distanceToBottom =
              viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight
            followNewest.current = distanceToBottom <= 96
          }}
        >
          {props.error && props.state === 'connected' ? (
            <p
              className="mb-3.5 rounded-lg border border-[color-mix(in_srgb,var(--theme-danger)_28%,transparent)] bg-(--theme-danger-soft) px-2.5 py-2 text-xs text-(--theme-danger-text)"
              role="alert"
            >
              {props.error}
            </p>
          ) : null}
          {props.currentConversation ? (
            <div className="flex min-h-full flex-col justify-end">
              {props.messages.map((event, index) => (
                <QQ9Message
                  key={`${event.MessageId ?? index}:${event.CreateAt ?? index}`}
                  event={event}
                  mine={isBotMessage(event)}
                  botIdentity={props.botIdentity}
                  isQQ={props.isQQ}
                  favorite={favoriteFor(event)}
                  onFavorite={() => props.toggleFavorite(event)}
                  onDelete={() => props.deleteMessage(event)}
                  decoration={props.decorations[event.MessageId || event.serverMessageID || '']}
                  onDecorate={(action, emoji) => props.decorateMessage(event, action, emoji)}
                  onOpenDirect={contact => props.openContactConversation(contact)}
                  onMention={contact => props.setText(`${props.text}${props.text && !props.text.endsWith(' ') ? ' ' : ''}@${contact.label} `)}
                  onReadProfile={() => props.refreshUserProfile(event)}
                />
              ))}
            </div>
          ) : (
            <div className="grid min-h-full place-content-center justify-items-center text-center text-(--theme-text-muted)">
              <Bot className="size-8" />
              <p className="my-2.5 text-sm text-(--theme-text-secondary)">
                选择一个 QQ 会话开始聊天
              </p>
              <small className="text-[11px]">
                机器人能力和操作记录可从最左侧能力栏打开。
              </small>
            </div>
          )}
          {props.currentConversation && !props.messages.length ? (
            <p className="grid min-h-full place-content-center justify-items-center text-center text-[11px] text-(--theme-text-muted)">
              此会话暂无本机历史消息。
            </p>
          ) : null}
        </section>
        <QQ9Composer
          currentConversation={props.currentConversation}
          isQQ={props.isQQ}
          state={props.state}
          text={props.text}
          setText={props.setText}
          contacts={props.contacts}
          attachment={props.attachment}
          setAttachment={props.setAttachment}
          attachmentInput={props.attachmentInput}
          send={props.send}
          openHistory={() => {
            props.setActiveNav('audit')
            props.setRightOpen(true)
          }}
        />
      </main>
      {profileOpen && (
        <aside
          className="qq-live-profile-drawer relative flex min-h-0 flex-col overflow-hidden border-l border-(--theme-border-default) bg-(--theme-surface-panel)"
          aria-label="会话资料"
        >
          <button
            className="absolute top-2 right-2 z-10 inline-flex size-8 items-center justify-center rounded-md border-0 bg-(--theme-surface-panel) text-(--theme-text-muted) hover:bg-(--theme-surface-hover)"
            onClick={() => setProfileOpen(false)}
            aria-label="关闭会话资料"
          >
            <X className="size-4" />
          </button>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pt-8">
            {profilePanel}
          </div>
        </aside>
      )}
    </div>
  )
}

function QQ9Message({
  event,
  mine,
  botIdentity,
  isQQ,
  favorite,
  onFavorite,
  onDelete,
  decoration,
  onDecorate,
  onOpenDirect,
  onMention,
  onReadProfile
}: {
  event: LiveEvent
  mine: boolean
  botIdentity: QQIdentity
  isQQ: boolean
  favorite: boolean
  onFavorite: () => void
  onDelete: () => void
  decoration?: MessageDecoration
  onDecorate: (action: 'message.pin' | 'message.unpin' | 'reaction.add' | 'reaction.remove', emoji?: string) => void
  onOpenDirect: (contact: QQContact) => void
  onMention: (contact: QQContact) => void
  onReadProfile: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [avatarMenuOpen, setAvatarMenuOpen] = useState(false)
  const contact = contactFromEvent(event)
  const canDelete = isQQ && mine && event.delivery === 'sent' && event.serverMessageID
  const canDecorate = isQQ && Boolean(event.serverMessageID) && qqTarget(event.context ?? event).scope === 'channel'
  const sender = mine
    ? { name: botIdentity.name, avatar: botIdentity.avatar }
    : { name: event.UserName || event.UserId || '未知用户', avatar: event.UserAvatar }
  const closeMenus = () => { setMenuOpen(false); setAvatarMenuOpen(false) }
  return (
    <article
      className={`mb-4.5 flex max-w-[82%] items-start gap-2 ${mine ? 'ml-auto flex-row-reverse' : ''}`}
      onContextMenu={contextEvent => {
        contextEvent.preventDefault()
        setMenuOpen(true)
      }}
    >
      <button
        className="relative inline-flex size-7.5 shrink-0 items-center justify-center overflow-hidden rounded-full border border-(--theme-border-default) bg-(--theme-surface-active) text-[13px] font-bold text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover"
        disabled={!contact}
        onContextMenu={contextEvent => { contextEvent.preventDefault(); setAvatarMenuOpen(true) }}
        onClick={() => contact && onOpenDirect(contact)}
        aria-label={contact ? `打开与 ${contact.label} 的私聊` : '头像'}
        title={contact ? `打开与 ${contact.label} 的私聊` : undefined}
      >
        {sender.avatar ? (
          <img src={sender.avatar} alt="" />
        ) : (
          mine ? <Bot className="size-4" aria-hidden="true" /> : sender.name.slice(0, 1)
        )}
        {avatarMenuOpen && contact ? <span className="qq9-popover-menu qq9-avatar-menu" role="menu" onClick={menuEvent => menuEvent.stopPropagation()}><button onClick={() => { onMention(contact); closeMenus() }}>提及 @{contact.label}</button><button onClick={() => { onOpenDirect(contact); closeMenus() }}>打开私聊</button><button onClick={() => { onReadProfile(); closeMenus() }}>读取完整资料</button></span> : null}
      </button>
      <div className="min-w-0">
        <header
          className={`mb-1 flex items-baseline gap-1.5 ${mine ? 'flex-row-reverse' : ''}`}
        >
          <b className="text-[11px] font-medium">
            {sender.name}
          </b>
          <time className="text-[10px] text-(--theme-text-muted)">
            {formatTime(event.CreateAt)}
          </time>
        </header>
        <div
          onContextMenu={contextEvent => { contextEvent.preventDefault(); setMenuOpen(true) }}
          className={`border border-(--theme-border-default) px-3 py-2 text-[length:inherit] leading-relaxed shadow-[0_1px_2px_var(--theme-shadow-soft)] ${mine ? 'rounded-[11px_5px_11px_11px] border-(--theme-accent-soft-border) bg-(--theme-accent-soft) text-(--theme-accent-soft-text)' : 'rounded-[5px_11px_11px_11px] bg-(--theme-surface-panel)'}`}
        >
          <p className="m-0 wrap-anywhere whitespace-pre-wrap">{messageSegments(event).length ? messageSegments(event).map((segment, index) => segment.type === 'Mention' ? <mark className="qq9-mention-chip" key={index}>@{String((segment as Segment & { options?: { name?: string } }).options?.name || segment.value || '成员')}</mark> : <span key={index}>{String(segment.value ?? '')}</span>) : '（非文本消息）'}</p>
        </div>
        {decoration?.pinned ? <span className="qq9-message-badge">已设为精华</span> : null}{decoration?.reactions?.map(emoji => <button className="qq9-reaction-chip" key={emoji} onClick={() => onDecorate('reaction.remove', emoji)} title="移除表态">{emoji}</button>)}
        {menuOpen ? <div className="qq9-popover-menu qq9-message-menu" role="menu"><button onClick={() => { onFavorite(); closeMenus() }}>{favorite ? '取消收藏' : '收藏消息'}</button>{canDecorate ? <><button onClick={() => { onDecorate('reaction.add', '👍'); closeMenus() }}>添加 👍 表态</button><button onClick={() => { onDecorate(decoration?.pinned ? 'message.unpin' : 'message.pin'); closeMenus() }}>{decoration?.pinned ? '取消精华' : '设为精华'}</button></> : null}{canDelete ? <button className="danger" onClick={() => { onDelete(); closeMenus() }}>撤回消息</button> : null}</div> : null}
        <footer
          className={`mt-0.5 flex min-h-4 items-center gap-1 text-[10px] text-(--theme-text-muted) ${mine ? 'justify-end' : ''}`}
        >
          {mine ? (
            <span>
              {event.delivery === 'failed'
                ? '发送失败'
                : event.delivery === 'sending'
                  ? '发送中…'
                  : '已发送'}
            </span>
          ) : null}
          <button
            className="inline-flex size-8 items-center justify-center rounded border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) focus-visible:ring-2 focus-visible:ring-(--theme-accent)"
            onClick={onFavorite}
            title={favorite ? '取消收藏' : '收藏消息'}
            aria-label={favorite ? '取消收藏消息' : '收藏消息'}
          >
            <Heart
              className={`size-3.5 ${favorite ? 'fill-current text-rose-500' : ''}`}
            />
          </button>
          {isQQ &&
          mine &&
          event.delivery === 'sent' &&
          event.serverMessageID ? (
            <button
              className="inline-flex size-8 items-center justify-center rounded border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) focus-visible:ring-2 focus-visible:ring-(--theme-accent)"
              onClick={onDelete}
              title="撤回消息"
              aria-label="撤回消息"
            >
              <Undo2 className="size-3.5" />
            </button>
          ) : null}
        </footer>
      </div>
    </article>
  )
}

function QQ9Composer({
  currentConversation,
  isQQ,
  state,
  text,
  setText,
  contacts,
  attachment,
  setAttachment,
  attachmentInput,
  send,
  openHistory
}: Pick<
  QQ9ShellProps,
  | 'currentConversation'
  | 'isQQ'
  | 'state'
  | 'text'
  | 'setText'
  | 'contacts'
  | 'attachment'
  | 'setAttachment'
  | 'attachmentInput'
  | 'send'
> & { openHistory: () => void }) {
  const [mentionOpen, setMentionOpen] = useState(false)
  const mentionQuery = text.match(/(?:^|\s)@([^\s@]*)$/)?.[1] || ''
  const mentionCandidates = contacts.filter(contact => contact.label.toLowerCase().includes(mentionQuery.toLowerCase())).slice(0, 8)
  if (!currentConversation)
    return (
      <footer className="border-t border-(--theme-border-default) p-3.5 text-[11px] text-(--theme-text-muted)">
        全局能力不会伪造 QQ 会话上下文。
      </footer>
    )
  return (
    <footer className="relative z-10 shrink-0 overflow-visible border-t border-(--theme-border-default) bg-(--theme-surface-panel) px-4 pt-2.5 pb-[max(10px,env(safe-area-inset-bottom))] shadow-[0_-8px_24px_rgb(28_26_23/0.04)]">
      {attachment && (
        <div className="mb-1.5 flex items-center gap-1.5 rounded-md border border-(--theme-border-subtle) bg-(--theme-surface-raised) px-2 py-1 text-[11px]">
          <Paperclip className="size-3.5" />
          <span className="flex-1 truncate">{attachment.name}</span>
          <button
            className="border-0 bg-transparent"
            onClick={() => setAttachment(null)}
            aria-label="移除附件"
          >
            <X className="size-3.5" />
          </button>
        </div>
      )}
      <div className="relative rounded-xl border border-(--theme-border-default) bg-(--theme-surface-input) p-2 shadow-[0_1px_2px_var(--theme-shadow-soft)] transition focus-within:border-(--theme-accent-soft-border) focus-within:ring-2 focus-within:ring-(--theme-accent-soft)">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1">
            <button className="inline-flex size-7 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-accent-text) disabled:cursor-not-allowed disabled:opacity-40" onClick={() => setMentionOpen(value => !value)} title="提及成员" aria-label="提及成员"><span className="text-base leading-none">@</span></button>
            <input ref={attachmentInput} className="hidden" type="file" onChange={event => setAttachment(event.target.files?.[0] || null)} />
            <button className="inline-flex size-7 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-accent-text)" title="图片或文件" aria-label="添加图片或文件" onClick={() => attachmentInput.current?.click()}><FileImage className="size-4" /></button>
            <button className="inline-flex size-7 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-accent-text)" title="添加附件" aria-label="添加附件" onClick={() => attachmentInput.current?.click()}><Paperclip className="size-4" /></button>
          </div>
          <button className="inline-flex size-7 items-center justify-center rounded-md border-0 bg-transparent text-(--theme-text-muted) hover:bg-(--theme-surface-hover) hover:text-(--theme-accent-text)" onClick={openHistory} title="聊天记录" aria-label="查看聊天记录"><History className="size-4" /></button>
        </div>
        <textarea
          className="mt-1.5 block h-18 min-h-12 max-h-30 box-border w-full max-w-full resize-none rounded-lg border-0 bg-transparent px-2 py-2 text-(--theme-text-primary) outline-none disabled:cursor-not-allowed disabled:opacity-55"
          value={text}
          disabled={!isQQ || state !== 'connected'}
          aria-label="消息内容"
          onChange={event => { setText(event.target.value); setMentionOpen(/(?:^|\s)@[^\s@]*$/.test(event.target.value)) }}
          onKeyDown={event => {
            if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault()
              void send()
            }
          }}
          placeholder="以机器人身份发送消息"
        />
        {mentionOpen && <div className="qq9-mention-menu" role="listbox">{mentionCandidates.map(contact => <button key={contact.id} onClick={() => { setText(text.replace(/@[^\s@]*$/, `@${contact.label} `)); setMentionOpen(false) }}><span className="qq9-avatar">{contact.avatar ? <img src={contact.avatar} alt="" /> : contact.label.slice(0, 1)}</span><span>{contact.label}</span></button>)}{!mentionCandidates.length && <p>暂无可提及成员；频道会话会自动读取成员。</p>}</div>}
        <div className="flex items-center justify-between border-t border-(--theme-border-subtle) pt-2">
          <small className="text-[10px] text-(--theme-text-muted)">Enter 发送 · Shift + Enter 换行</small>
          <button className="inline-flex items-center gap-1 rounded-md border-0 bg-(--theme-accent) px-2.5 py-1.5 text-xs text-(--theme-accent-contrast) disabled:cursor-not-allowed disabled:opacity-45" disabled={!isQQ || (!text.trim() && !attachment) || state !== 'connected'} onClick={() => void send()} aria-label="发送消息"><Send className="size-4" />发送</button>
        </div>
      </div>
    </footer>
  )
}

function QQ9Profile({
  event,
  members,
  openContact,
  mentionContact
}: {
  event?: LiveEvent
  members: ProfileState
  openContact: (contact: QQContact) => void
  mentionContact: (contact: QQContact) => void
}) {
  if (!event)
    return (
      <div className="min-h-0 px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
        <p>选择会话后可读取群、频道或私聊资料。</p>
      </div>
    )
  return (
    <div className="min-h-0 px-2 py-3">
      <section className="border-t border-(--theme-border-default) pt-3">
        <b className="block text-xs">公告</b>
        <p className="mt-1 mb-0 text-[11px] leading-relaxed text-(--theme-text-muted)">
          当前 QQ Bot 未提供可安全读取的群公告内容。
        </p>
      </section>
      <section className="mt-3 border-t border-(--theme-border-default) pt-3">
        <header className="flex items-center justify-between">
          <div>
            <b className="block text-xs">成员</b>
            <small className="mt-0.5 block text-[10px] text-(--theme-text-muted)">
              {members.message || '打开会话时会自动读取；无成员列表能力时使用已知发言者。'}
            </small>
          </div>
          {members.status === 'loading' ? <Loader2 className="size-4 animate-spin text-(--theme-text-muted)" /> : null}
        </header>
        {members.status === 'ready' ? (
          <MemberRows data={members.data} openContact={openContact} mentionContact={mentionContact} />
        ) : members.status === 'failed' || members.status === 'unavailable' ? (
          <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
            {members.message}
          </p>
        ) : null}
      </section>
    </div>
  )
}

function MemberRows({ data, openContact, mentionContact }: { data: unknown; openContact: (contact: QQContact) => void; mentionContact: (contact: QQContact) => void }) {
  const rows = resultItems(data)
  return (
    <div className="mt-2 grid gap-1">
      {rows.slice(0, 50).map((item, index) => {
        const name =
          recordText(item, ['nick', 'username', 'name', 'user_name']) ||
          recordText(item, ['user_id', 'id', 'userId']) ||
          '未知成员'
        const userID = recordText(item, ['user_id', 'id', 'userId', 'member_openid', 'openid'])
        const avatar = recordText(item, [
          'avatar',
          'avatar_url',
          'avatarUrl',
          'head_url',
          'headUrl',
          'headurl',
          'user_avatar',
          'userAvatar'
        ])
        const contact: QQContact = { id: `user:${userID || name}`, label: name, avatar, source: 'member', lastInteractionAt: Date.now() }
        return (
          <div
            className="group flex items-center gap-1.5 rounded-md px-1 py-1 text-[11px] hover:bg-(--theme-surface-hover)"
            key={`${name}:${index}`}
          >
            <button className="inline-flex size-6 shrink-0 items-center justify-center overflow-hidden rounded-full border border-(--theme-border-default) bg-(--theme-surface-active) text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover" onClick={() => openContact(contact)} title={`打开与 ${name} 的私聊`} aria-label={`打开与 ${name} 的私聊`}>
              {avatar ? <img src={avatar} alt="" /> : name.slice(0, 1)}
            </button>
            <button className="min-w-0 flex-1 truncate border-0 bg-transparent p-0 text-left text-inherit" onClick={() => mentionContact(contact)} title={`提及 ${name}`}>{name}</button>
          </div>
        )
      })}
      {!rows.length && (
        <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
          QQ 返回了空成员列表。
        </p>
      )}
    </div>
  )
}

function ProfileRows({ data }: { data: unknown }) {
  if (!data || typeof data !== 'object')
    return (
      <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
        QQ 返回了空资料。
      </p>
    )
  return (
    <dl className="m-0 grid gap-1">
      {Object.entries(data as Record<string, unknown>)
        .filter(([key]) => !/(token|secret|password)/i.test(key))
        .slice(0, 8)
        .map(([key, value]) => (
          <div
            className="grid grid-cols-[82px_minmax(0,1fr)] gap-2 text-[11px]"
            key={key}
          >
            <dt className="text-(--theme-text-muted)">{key}</dt>
            <dd className="m-0 truncate">
              {typeof value === 'string' || typeof value === 'number'
                ? String(value)
                : '已返回结构化数据'}
            </dd>
          </div>
        ))}
    </dl>
  )
}

function QQ9ToolCenter(props: QQ9ShellProps) {
  const [toolSearch, setToolSearch] = useState('')
  const [showHighRisk, setShowHighRisk] = useState(false)
  const query = toolSearch.trim().toLowerCase()
  const matchesTool = (action: QQActionDefinition) =>
    (!query ||
      `${action.title} ${action.id} ${action.group} ${action.description || ''}`
        .toLowerCase()
        .includes(query)) &&
    (!showHighRisk || action.risk === 'high')
  const visibleActionCount = qqActionCatalog.filter(
    action =>
      matchesTool(action) &&
      (props.showUnavailable ||
        isActionAvailable(action, props.currentScopeForActions))
  ).length
  return (
    <div className="min-h-0 overflow-auto px-2 py-3">
      <input
        className="mb-2 w-full rounded-md border border-(--theme-border-subtle) bg-(--theme-surface-input) px-2 py-1.5 text-xs outline-none focus:border-(--theme-accent-soft-border)"
        value={toolSearch}
        onChange={event => setToolSearch(event.target.value)}
        placeholder="搜索能力、动作 ID 或分组"
        aria-label="搜索机器人能力"
      />
      <div className="mb-2.5 flex flex-wrap gap-x-3 gap-y-1.5 text-[11px] text-(--theme-text-muted)">
        <label className="flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={props.showUnavailable}
            onChange={event => props.setShowUnavailable(event.target.checked)}
          />
          显示不支持的工具
        </label>
        <label className="flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={showHighRisk}
            onChange={event => setShowHighRisk(event.target.checked)}
          />
          仅高风险操作
        </label>
      </div>
      <div className="order-2">
        {props.groups.map(group => (
          <details
            className="border-t border-(--theme-border-default) py-2"
            key={group}
            open={
              group === '机器人状态' ||
              props.activeDefinition?.group === group
            }
          >
            <summary className="cursor-pointer list-none text-[10px] text-(--theme-text-muted) marker:content-none">
              {group}
            </summary>
            <div className="mt-1.5 flex flex-wrap gap-1">
              {qqActionCatalog
                .filter(
                  action =>
                    action.group === group &&
                    matchesTool(action) &&
                    (props.showUnavailable ||
                      isActionAvailable(action, props.currentScopeForActions))
                )
                .map(action => (
                  <button
                    key={action.id}
                    disabled={
                      !isActionAvailable(action, props.currentScopeForActions)
                    }
                    className={`rounded border border-(--theme-border-default) bg-(--theme-surface-raised) px-1.5 py-1 text-[10px] text-(--theme-text-secondary) ${props.activeAction === action.id ? 'border-(--theme-accent-soft-border) text-(--theme-accent-text)' : ''} disabled:cursor-not-allowed disabled:opacity-40`}
                    onClick={() => props.chooseAction(action)}
                    title={action.description || action.id}
                  >
                    {action.title}
                  </button>
                ))}
            </div>
          </details>
        ))}
        {!visibleActionCount && (
          <p className="rounded-md border border-dashed border-(--theme-border-default) px-2 py-4 text-center text-[11px] text-(--theme-text-muted)">
            没有匹配的能力。可清除筛选或先选择适用的 QQ 会话。
          </p>
        )}
      </div>
      {props.activeDefinition && (
        <div className="order-1 sticky top-0 z-10 mt-1.5 border-y border-(--theme-border-default) bg-(--theme-surface-panel) py-3 shadow-[0_8px_14px_rgb(28_26_23/0.06)]">
          <header className="flex items-start justify-between">
            <div>
              <strong className="block text-xs">
                {props.activeDefinition.title}
              </strong>
              <small className="mt-0.5 block text-[10px] text-(--theme-text-muted)">
                {props.activeDefinition.id}
              </small>
            </div>
            {props.activeDefinition.risk === 'high' && (
              <span className="rounded bg-(--theme-warning-soft) px-1 py-0.5 text-[9px] text-(--theme-warning-text)">
                每次确认
              </span>
            )}
          </header>
          {props.activeDefinition.description && (
            <p className="text-[11px] leading-relaxed text-(--theme-text-muted)">
              {props.activeDefinition.description}
            </p>
          )}
          {props.activeDefinition.fields.map(field => (
            <ActionField
              key={field.key}
              field={field}
              value={getFormValue(props.formValues, field)}
              onChange={value =>
                props.setFormValues(current => ({
                  ...current,
                  [field.key]: value
                }))
              }
            />
          ))}
          <button
            className="mt-3 inline-flex w-full items-center justify-center gap-1 rounded-md border-0 bg-(--theme-accent) px-2.5 py-1.5 text-xs text-(--theme-accent-contrast) disabled:cursor-not-allowed disabled:opacity-45"
            disabled={props.busy || props.state !== 'connected'}
            onClick={() => void props.executeTool()}
          >
            {props.busy ? (
              <Loader2 className="animate-spin" />
            ) : props.activeDefinition.risk === 'high' ? (
              <CircleAlert />
            ) : (
              <CheckCircle2 />
            )}
            {props.busy ? '等待 QQ 响应…' : '执行操作'}
          </button>
        </div>
      )}
    </div>
  )
}

function QQ9Audit({
  tools,
  clear
}: {
  tools: ToolRecord[]
  clear: () => void
}) {
  return (
    <div className="min-h-0 overflow-auto px-2 py-3">
      <header className="mb-2.5 flex items-center justify-between text-[11px] text-(--theme-text-muted)">
        <span>本机记录 · 30 天</span>
        <button
          className="inline-flex items-center gap-1 border-0 bg-transparent text-[11px] text-(--theme-danger-text)"
          onClick={clear}
        >
          <Trash2 />
          清除
        </button>
      </header>
      {tools.length ? (
        tools.map(item => (
          <article
            className="mb-2 rounded-md border border-(--theme-border-default) bg-(--theme-surface-raised) p-2 text-[10px]"
            key={item.id}
          >
            <header className="flex justify-between gap-1.5">
              <b className="truncate">{item.action}</b>
              <span
                className={
                  item.state === 'success'
                    ? 'text-(--theme-success-text)'
                    : item.state === 'warning'
                      ? 'text-(--theme-warning-text)'
                      : 'text-(--theme-danger-text)'
                }
              >
                {item.state === 'success'
                  ? '成功'
                  : item.state === 'warning'
                    ? '警告'
                    : item.state === 'timeout'
                      ? '超时'
                      : '失败'}
              </span>
            </header>
            <p className="my-1 text-(--theme-text-muted)">{item.target}</p>
            <small className="block truncate text-(--theme-text-muted)">
              {item.summary} · {new Date(item.at).toLocaleString()}
            </small>
          </article>
        ))
      ) : (
        <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
          操作结果会在这里以脱敏摘要保留。
        </p>
      )}
    </div>
  )
}

function QQ9Contacts({
  contacts,
  search,
  openConversation
}: {
  contacts: QQContact[]
  search: string
  openConversation: (contact: QQContact) => void
}) {
  return (
    <div className="h-full min-h-0 overflow-auto px-1 py-3">
      {contacts
        .filter(item => item.label.toLowerCase().includes(search.toLowerCase()))
        .map(item => (
          <button
            className="flex w-full items-center gap-2 rounded-lg border-0 bg-transparent px-1 py-2 text-left hover:bg-(--theme-surface-hover)"
            key={item.id}
            onClick={() => openConversation(item)}
            title={`打开与 ${item.label} 的私聊`}
          >
            <span className="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-(--theme-border-default) bg-(--theme-surface-active) text-[13px] font-bold text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover">
              {item.avatar ? (
                <img src={item.avatar} alt="" />
              ) : (
                item.label.slice(0, 1)
              )}
            </span>
            <div className="min-w-0 flex-1">
              <b className="block truncate text-xs" title={item.label}>
                {item.label}
              </b>
              <small className="mt-0.5 block truncate text-[10px] text-(--theme-text-muted)">
                {item.source === 'private'
                  ? '历史私聊'
                  : item.source === 'member'
                    ? '已读取群成员'
                    : '当前会话用户'}
              </small>
            </div>
          </button>
        ))}
      {!contacts.length && (
        <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
          不会伪造平台联系人。消息或已读取成员资料会逐步建立本机列表。
        </p>
      )}
    </div>
  )
}

function QQ9Spaces({
  conversations,
  spaces,
  search,
  selectConversation,
  openConversation
}: {
  conversations: Conversation[]
  spaces: QQSpace[]
  search: string
  selectConversation: (id: string) => void
  openConversation: (space: QQSpace) => void
}) {
  const known = new Map(
    conversations.filter(item => !item.private).map(item => [item.id, item])
  )
  const rows = spaces.filter(item =>
    item.label.toLowerCase().includes(search.toLowerCase())
  )
  return (
    <div className="h-full min-h-0 overflow-auto px-1 py-3">
      {rows.map(item => {
        const conversation = known.get(makeSpaceConversation(item).id)
        return (
          <button
            className="flex w-full items-center gap-2 rounded-lg border-0 bg-transparent px-1 py-2 text-left hover:bg-(--theme-surface-hover)"
            key={item.id}
            onClick={() =>
              conversation
                ? selectConversation(conversation.id)
                : openConversation(item)
            }
            title={
              conversation ? '进入已加载会话' : '打开会话并创建本机发送上下文'
            }
          >
            <span className="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-(--theme-border-default) bg-(--theme-surface-active) text-[13px] font-bold text-(--theme-text-secondary) [&_img]:size-full [&_img]:object-cover">
              {item.avatar ? <img src={item.avatar} alt="" /> : item.label.slice(0, 1)}
            </span>
            <div className="min-w-0 flex-1">
              <b className="block truncate text-xs" title={item.label}>
                {item.label}
              </b>
              <small
                className="mt-0.5 block truncate text-[10px] text-(--theme-text-muted)"
                title={`${scopeName(item.scope)} · ${conversation ? '进入会话' : '打开会话'}`}
              >
                {scopeName(item.scope)} ·{' '}
                {conversation ? '进入会话' : '打开会话'}
              </small>
            </div>
          </button>
        )
      })}
      {!rows.length && (
        <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
          正在自动读取群与频道；若 QQ Bot 未返回目录，收到消息后也会自动建立会话。
        </p>
      )}
    </div>
  )
}

function QQ9Favorites({
  favorites,
  selectConversation
}: {
  favorites: QQFavorite[]
  selectConversation: (id: string) => void
}) {
  return (
    <div className="h-full min-h-0 overflow-auto px-1 py-3">
      {favorites.map(item => (
        <button
          className="flex w-full items-center gap-2 rounded-lg border-0 bg-transparent px-1 py-2 text-left hover:bg-(--theme-surface-hover)"
          key={item.id}
          onClick={() => selectConversation(item.conversationId)}
        >
          <Heart className="size-4 text-rose-500" />
          <div className="min-w-0 flex-1">
            <b className="block truncate text-xs" title={item.text}>
              {item.text}
            </b>
            <small className="mt-0.5 block text-[10px] text-(--theme-text-muted)">
              {new Date(item.createdAt).toLocaleDateString()}
            </small>
          </div>
        </button>
      ))}
      {!favorites.length && (
        <p className="px-2 py-3 text-center text-[11px] leading-relaxed text-(--theme-text-muted)">
          在消息气泡下点击心形图标即可收藏。
        </p>
      )}
    </div>
  )
}

function QQ9Settings({
  preferences,
  setPreferences,
  clearHistory,
  resetLayout
}: {
  preferences: QQChatPreferences
  setPreferences: React.Dispatch<React.SetStateAction<QQChatPreferences>>
  clearHistory: () => void
  resetLayout: () => void
}) {
  return (
    <div className="grid h-full min-h-0 content-start gap-3 overflow-auto px-1 py-3">
      <header className="px-1">
        <strong className="block text-[13px]">聊天设置</strong>
        <small className="mt-0.5 block text-[10px] text-(--theme-text-muted)">
          仅保存在当前机器人目录
        </small>
      </header>
      <SettingSelect
        label="聊天密度"
        value={preferences.density}
        options={[
          ['comfortable', '舒适'],
          ['compact', '紧凑']
        ]}
        onChange={density =>
          setPreferences(current => ({
            ...current,
            density: density as QQChatPreferences['density']
          }))
        }
      />
      <SettingSelect
        label="文字大小"
        value={preferences.fontSize}
        options={[
          ['small', '小'],
          ['medium', '中'],
          ['large', '大']
        ]}
        onChange={fontSize =>
          setPreferences(current => ({
            ...current,
            fontSize: fontSize as QQChatPreferences['fontSize']
          }))
        }
      />
      <SettingSelect
        label="历史保留"
        value={String(preferences.historyDays)}
        options={[
          ['7', '7 天'],
          ['30', '30 天']
        ]}
        onChange={value =>
          setPreferences(current => ({
            ...current,
            historyDays: Number(value) as 7 | 30
          }))
        }
      />
      <SettingSelect
        label="收藏保留"
        value={preferences.favoriteRetention}
        options={[
          ['7d', '7 天'],
          ['30d', '30 天'],
          ['forever', '长期']
        ]}
        onChange={favoriteRetention =>
          setPreferences(current => ({
            ...current,
            favoriteRetention:
              favoriteRetention as QQChatPreferences['favoriteRetention']
          }))
        }
      />
      <label className="flex items-center gap-1.5 text-[11px] text-(--theme-text-muted)">
        <input
          type="checkbox"
          checked={preferences.autoProfile}
          onChange={event =>
            setPreferences(current => ({
              ...current,
              autoProfile: event.target.checked
            }))
          }
        />
        自动读取会话资料
      </label>
      <div className="mt-1 grid gap-1.5">
        <button
          className="rounded-md border border-(--theme-border-default) bg-(--theme-surface-raised) p-2 text-left text-[11px] text-(--theme-text-secondary) hover:border-(--theme-accent-soft-border) hover:text-(--theme-accent-text)"
          onClick={resetLayout}
        >
          重置聊天布局与偏好
        </button>
        <button
          className="rounded-md border border-(--theme-border-default) bg-(--theme-surface-raised) p-2 text-left text-[11px] text-(--theme-danger-text) hover:border-(--theme-accent-soft-border)"
          onClick={clearHistory}
        >
          清除本机聊天历史
        </button>
      </div>
    </div>
  )
}

function SettingSelect({
  label,
  value,
  options,
  onChange
}: {
  label: string
  value: string
  options: Array<[string, string]>
  onChange: (value: string) => void
}) {
  return (
    <label className="flex items-center justify-between text-xs">
      <span>{label}</span>
      <select
        className="rounded border border-(--theme-border-default) bg-(--theme-surface-raised) p-1 text-[11px]"
        value={value}
        onChange={event => onChange(event.target.value)}
      >
        {options.map(([id, title]) => (
          <option key={id} value={id}>
            {title}
          </option>
        ))}
      </select>
    </label>
  )
}

function formatTime(value?: number) {
  return value
    ? new Date(value).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit'
      })
    : ''
}

function ActionField({
  field,
  value,
  onChange
}: {
  field: QQActionField
  value: unknown
  onChange: (value: unknown) => void
}) {
  const base =
    'w-full rounded-md border border-slate-300 bg-white px-2 py-1.5 text-xs text-slate-800 outline-none focus:border-brand-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
  return (
    <label className="grid gap-1 text-xs text-slate-600 dark:text-slate-300">
      <span>
        {field.label}
        {field.required ? <b className="ml-0.5 text-rose-500">*</b> : null}
      </span>
      {field.kind === 'boolean' ? (
        <span className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={Boolean(value)}
            onChange={event => onChange(event.target.checked)}
          />
          <span>{field.help || '启用'}</span>
        </span>
      ) : field.kind === 'file' ? (
        <input
          className={base}
          type="file"
          onChange={event => onChange(event.target.files?.[0])}
        />
      ) : field.kind === 'select' ? (
        <select
          className={base}
          value={String(value ?? '')}
          onChange={event => onChange(event.target.value)}
        >
          <option value="">请选择</option>
          {field.options?.map(([id, title]) => (
            <option key={id} value={id}>
              {title}
            </option>
          ))}
        </select>
      ) : field.kind === 'textarea' ? (
        <textarea
          className={base}
          value={String(value ?? '')}
          placeholder={field.placeholder}
          onChange={event => onChange(event.target.value)}
        />
      ) : (
        <input
          className={base}
          type={
            field.kind === 'number'
              ? 'number'
              : field.kind === 'url'
                ? 'url'
                : 'text'
          }
          value={String(value ?? '')}
          placeholder={field.placeholder}
          onChange={event => onChange(event.target.value)}
        />
      )}
    </label>
  )
}
