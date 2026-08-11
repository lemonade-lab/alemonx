// Older embedded WebViews may expose crypto without randomUUID. This ID is
// used only to distinguish a browser tab while coordinating SSE leadership;
// it is never an authentication token or persistent identifier.
export function createRandomID(): string {
  const cryptoAPI = globalThis.crypto
  if (typeof cryptoAPI?.randomUUID === 'function') return cryptoAPI.randomUUID()
  if (typeof cryptoAPI?.getRandomValues === 'function') {
    const bytes = cryptoAPI.getRandomValues(new Uint8Array(16))
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80
    const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
  }
  return `tab-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`
}
