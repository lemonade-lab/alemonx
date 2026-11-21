import { desktopPlatform } from '../components/nativeWindow'

const path = '/api/desktop/preferences'
const headers = { 'X-ALX-Desktop-Preferences': '1', 'Content-Type': 'application/json' }
const keys = new Set([
  'alemonjs-theme', 'persist:alemonx-guide', 'persist:alemonx-data',
  'persist:alemonx-workspace', 'persist:alemonx-navigation', 'persist:alemonx-browser-recent',
  'alemonx-legacy-robot-list',
  'autoConnectEnabled', 'ALemonTestOne:image:compression', 'alx:operation-log'
])

function isPreference(key: string) {
  return keys.has(key) || key.startsWith('alx:plugin-webview:') ||
    key.startsWith('alx:service-webview:') ||
    (key.startsWith('alemonx:qq-live:v2:') && key.endsWith(':window'))
}

function sharedValue(key: string, value: string) {
  if (key !== 'persist:alemonx-workspace') return value
  // Older versions could store configuration and credential-bearing drafts.
  const state = JSON.parse(value) as Record<string, unknown>
  delete state.drafts
  delete state.robotConfigs
  return JSON.stringify(state)
}

const writes = new Map<string, Promise<void>>()
let enabled = false

function queueWrite(key: string, value: string | null, importing = false) {
  const write = async () => {
    try {
      const body = JSON.stringify({
        key,
        value: value === null ? null : sharedValue(key, value),
        import: importing || undefined
      })
      const response = await fetch(path, {
        method: 'PUT', headers, body,
        signal: importing ? AbortSignal.timeout(3000) : undefined,
        keepalive: !importing && new TextEncoder().encode(body).length < 60_000
      })
      if (!response.ok) throw new Error('Unable to save desktop preference')
    } catch (error) {
      console.error('ALemonX Desktop preferences:', error)
    }
  }
  const pending = writes.get(key)
  const request = pending ? pending.then(write) : write()
  writes.set(key, request)
  void request.then(() => { if (writes.get(key) === request) writes.delete(key) })
}

function save(key: string, value: string | null) {
  if (enabled && isPreference(key)) queueWrite(key, value)
}

export const preferenceStorage = {
  getItem: (key: string) => localStorage.getItem(key),
  setItem(key: string, value: string) {
    localStorage.setItem(key, value)
    save(key, value)
  },
  removeItem(key: string) {
    localStorage.removeItem(key)
    save(key, null)
  }
}

export async function restoreDesktopPreferences() {
  if (!desktopPlatform) return
  try {
    const response = await fetch(path, { headers, signal: AbortSignal.timeout(3000) })
    if (!response.ok) return
    const saved = await response.json() as Record<string, string | null>
    enabled = true
    for (const [key, value] of Object.entries(saved)) {
      if (!isPreference(key)) continue
      if (value === null) localStorage.removeItem(key)
      else if (typeof value === 'string') {
        let restored = value
        if (key === 'persist:alemonx-workspace') {
          try {
            const { drafts } = JSON.parse(localStorage.getItem(key) ?? '{}')
            if (typeof drafts === 'string')
              restored = JSON.stringify({ ...JSON.parse(value), drafts })
          } catch {
            // Use the shared preferences when old origin data is malformed.
          }
        }
        localStorage.setItem(key, restored)
      }
    }
    const legacy = Object.keys(localStorage).filter(key =>
      isPreference(key) && !Object.prototype.hasOwnProperty.call(saved, key)
    )
    // Import in the background. Later writes to the same key wait in order.
    for (const key of legacy) {
      const value = localStorage.getItem(key)
      if (value !== null) queueWrite(key, value, true)
    }
    const theme = localStorage.getItem('alemonjs-theme')
    if (theme === 'dark' || theme === 'light') {
      document.documentElement.dataset.theme = theme
      document.documentElement.style.colorScheme = theme
    }
  } catch (error) {
    console.error('ALemonX Desktop preferences:', error)
  }
}
