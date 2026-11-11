import { useCallback, useEffect, useRef, useState } from 'react'
import { useDispatch } from 'react-redux'
import {
  replaceAccountRobots,
  readLegacyRobots,
  clearLegacyRobots,
  type WorkspaceProject
} from '../store/workspaceStore'

type RobotList = {
  scope: string
  local: boolean
  initialized: boolean
  projects: WorkspaceProject[]
  activeProjectID: string
}
type RobotChange = {
  action: 'add' | 'remove' | 'select' | 'pin' | 'reorder' | 'import'
  projects?: WorkspaceProject[]
  id?: string
  targetId?: string
}

async function requestRobots(options?: RequestInit): Promise<RobotList> {
  const response = await fetch('/api/v1/workspace/robots', options)
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error || '无法同步账户机器人列表。')
  }
  return response.json()
}

export function useAccountRobots() {
  const dispatch = useDispatch()
  const current = useRef<RobotList | null>(null)
  const lifetime = useRef<AbortController | null>(null)
  const queue = useRef(Promise.resolve())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [canImport, setCanImport] = useState(false)
  const [reload, setReload] = useState(0)

  const apply = useCallback(
    (list: RobotList) => {
      current.current = list
      dispatch(replaceAccountRobots(list))
      setCanImport(!list.initialized && readLegacyRobots().projects.length > 0)
    },
    [dispatch]
  )

  const change = useCallback(
    (input: RobotChange): Promise<RobotList | null> => {
      const list = current.current
      const controller = lifetime.current
      if (!list || !controller) return Promise.resolve(null)
      const work = queue.current.then(async () => {
        if (controller.signal.aborted) return null
        if (
          input.action === 'select' &&
          current.current?.activeProjectID === input.id
        )
          return current.current
        try {
          const next = await requestRobots({
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ...input, scope: list.scope }),
            signal: controller.signal
          })
          if (controller.signal.aborted) return null
          if (input.action === 'import') clearLegacyRobots()
          apply(next)
          setError('')
          return next
        } catch (reason) {
          if (!controller.signal.aborted)
            setError(
              reason instanceof Error
                ? reason.message
                : '无法同步账户机器人列表。'
            )
          return null
        }
      })
      queue.current = work.then(() => undefined)
      return work
    },
    [apply]
  )

  const importLegacy = useCallback(() => {
    const legacy = readLegacyRobots()
    return change({
      action: 'import',
      projects: legacy.projects,
      id: legacy.activeProjectID
    })
  }, [change])

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    current.current = null
    dispatch(replaceAccountRobots({ projects: [], activeProjectID: '' }))
    setLoading(true)
    setError('')
    setCanImport(false)
    void (async () => {
      try {
        const list = await requestRobots({ signal: controller.signal })
        if (controller.signal.aborted) return
        apply(list)
        if (
          list.local &&
          !list.initialized &&
          readLegacyRobots().projects.length
        )
          await importLegacy()
      } catch (reason) {
        if (!controller.signal.aborted)
          setError(
            reason instanceof Error
              ? reason.message
              : '无法读取账户机器人列表。'
          )
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    })()
    return () => controller.abort()
  }, [apply, dispatch, importLegacy, reload])

  return {
    change,
    importLegacy,
    canImport,
    loading,
    error,
    ready: !loading && current.current !== null,
    retry: () => setReload(value => value + 1)
  }
}
