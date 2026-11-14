type NativeWindow = Window & {
  chrome?: { webview?: { postMessage: (message: string) => void } }
  webkit?: {
    messageHandlers?: { external?: { postMessage: (message: string) => void } }
  }
}

const native = typeof window === 'undefined' ? undefined : window as NativeWindow
export const windows = native?.chrome?.webview
const mac = native?.webkit?.messageHandlers?.external
export const bridge = native && native.top === native ? (windows ?? mac) : undefined

export const desktopPlatform = bridge
  ? windows
    ? 'windows'
    : /Linux/i.test(navigator.platform || navigator.userAgent)
      ? 'linux'
      : 'mac'
  : ''

let runtimeReady = false
export function markNativeRuntimeReady() {
  if (!bridge || runtimeReady) return
  runtimeReady = true
  bridge.postMessage('wails:runtime:ready')
}

const auxiliaryWindowNames = [
  'settings', 'data', 'terminal', 'foreground-logs', 'pm2-logs', 'robot-app',
  'git', 'pm2-processes', 'test', 'live', 'ops', 'ops-overview', 'tasks',
  'environment', 'browser', 'plugins', 'plugin'
] as const

export type AuxiliaryWindowName = typeof auxiliaryWindowNames[number]

export function getAuxiliaryWindowName(path: string) {
  return auxiliaryWindowNames.find(name => path === `/desktop-window/${name}`)
}

export function openNativeWindow(name: AuxiliaryWindowName | 'main-project', section = '') {
  if (!bridge) return false
  bridge.postMessage(`alx:open-window:${name}:${section}`)
  return true
}
