import { useEffect, useState } from 'react'

export function useCanManageAccounts(active = true) {
  const [canManage, setCanManage] = useState(false)

  useEffect(() => {
    if (!active) return
    let pending: AbortController | undefined
    const refresh = () => {
      pending?.abort()
      const controller = new AbortController()
      pending = controller
      setCanManage(false)
      void fetch('/api/v1/auth/status', {
        credentials: 'same-origin',
        signal: controller.signal
      })
        .then(async response => {
          if (!response.ok) return null
          return (await response.json()) as {
            enabled?: boolean
            authenticated?: boolean
            superAdmin?: boolean
          }
        })
        .then(status => {
          if (!controller.signal.aborted) {
            setCanManage(
              Boolean(
                status?.enabled && status.authenticated && status.superAdmin
              )
            )
          }
        })
        .catch(() => {
          if (!controller.signal.aborted) setCanManage(false)
        })
    }
    refresh()
    window.addEventListener('alx:auth-changed', refresh)
    return () => {
      pending?.abort()
      window.removeEventListener('alx:auth-changed', refresh)
    }
  }, [active])

  return active && canManage
}
