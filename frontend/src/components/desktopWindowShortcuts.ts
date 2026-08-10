export type DesktopWindowShortcutRegistration = {
  id: string
  zIndex: number
  minimized: boolean
  onClose: () => void
  onMinimize: () => void
  onToggleMaximize?: () => void
}

const windows = new Map<string, DesktopWindowShortcutRegistration>()
let listenerInstalled = false

function hasBlockingSystemModal() {
  return [...document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]')].some(
    element =>
      !element.classList.contains('floating-window') &&
      !element.classList.contains('floating-window-backdrop') &&
      element.style.display !== 'none' &&
      getComputedStyle(element).display !== 'none'
  )
}

function topWindow() {
  return [...windows.values()]
    .filter(window => !window.minimized)
    .sort((left, right) => right.zIndex - left.zIndex)[0]
}

function installListener() {
  if (listenerInstalled) return
  listenerInstalled = true
  window.addEventListener(
    'keydown',
    event => {
      if (event.defaultPrevented || event.isComposing || (!event.metaKey && !event.ctrlKey)) return
      if (event.altKey) return
      const key = event.key.toLowerCase()
      if (hasBlockingSystemModal()) {
        if (key === 'w' || key === 'm') {
          event.preventDefault()
          event.stopImmediatePropagation()
        }
        return
      }
      const activeWindow = topWindow()
      if (!activeWindow) return
      if (key === 'w' && !event.shiftKey) {
        event.preventDefault()
        event.stopImmediatePropagation()
        activeWindow.onClose()
        return
      }
      if (key === 'm') {
        if (event.shiftKey && !activeWindow.onToggleMaximize) return
        event.preventDefault()
        event.stopImmediatePropagation()
        if (event.shiftKey) activeWindow.onToggleMaximize?.()
        else activeWindow.onMinimize()
      }
    },
    { capture: true }
  )
}

export function registerDesktopWindowShortcut(
  registration: DesktopWindowShortcutRegistration
) {
  installListener()
  windows.set(registration.id, registration)
  return () => {
    windows.delete(registration.id)
  }
}
