export function isWorkbenchBrowserPageURL(value: string) {
  try {
    const target = new URL(value, window.location.origin)
    return (
      target.origin === window.location.origin &&
      (target.pathname.startsWith('/api/v1/robot/') ||
        target.pathname.startsWith('/api/v1/services/'))
    )
  } catch {
    return false
  }
}
