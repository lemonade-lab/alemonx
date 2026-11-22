import { RefreshButton } from './RefreshButton'
import { useEffect, useRef } from 'react'
import {
  useDshWebStatusQuery,
  useOpenDSHWebMutation
} from '../store/workspaceApi'
import { Button } from './Button'
import type { DSHWorkspaceProps } from './DSHWorkspace.d'

function getFrameURL(url?: string) {
  if (!url) return ''
  try {
    const target = new URL(url)
    if (
      target.protocol !== 'http:' ||
      target.hostname !== '127.0.0.1' ||
      !target.port ||
      target.username ||
      target.password ||
      target.pathname !== '/' ||
      !target.searchParams.has('token')
    )
      return ''
    const token = btoa(target.origin)
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/g, '')
    return `/api/v1/browser/http/${token}/${target.search}`
  } catch {
    return ''
  }
}

export function DSHWorkspace({ root, robotRoots, visible }: DSHWorkspaceProps) {
  const {
    data: status,
    isFetching,
    isError,
    refetch
  } = useDshWebStatusQuery(undefined, {
    skip: !visible || !root
  })
  const [openWeb, { data, isLoading, error }] = useOpenDSHWebMutation()
  const started = useRef(false)
  const frame = useRef<HTMLIFrameElement>(null)
  const frameURL = getFrameURL(data?.url)

  useEffect(() => {
    if (!visible || !root || !status?.ready || started.current) return
    started.current = true
    void openWeb(root)
  }, [openWeb, root, status?.ready, visible])

  useEffect(() => {
    const receive = (event: MessageEvent) => {
      if (
        !frame.current ||
        event.source !== frame.current.contentWindow ||
        event.origin !== window.location.origin ||
        event.data?.type !== 'alx:dsh-workspaces-ready'
      )
        return
      frame.current.contentWindow?.postMessage(
        { type: 'alx:dsh-robot-workspaces', roots: robotRoots },
        window.location.origin
      )
    }
    window.addEventListener('message', receive)
    return () => window.removeEventListener('message', receive)
  }, [robotRoots])

  useEffect(() => {
    frame.current?.contentWindow?.postMessage(
      { type: 'alx:dsh-robot-workspaces', roots: robotRoots },
      window.location.origin
    )
  }, [frameURL, robotRoots])

  if (frameURL)
    return (
      <iframe
        ref={frame}
        src={frameURL}
        title="DSH Web 工作区"
        className="h-full w-full border-0 bg-(--theme-surface-panel)"
      />
    )

  const failure =
    error && 'data' in error ? (error.data as { error?: string }) : undefined
  const failedToOpen = Boolean(error || (data && !frameURL))
  return (
    <section
      className="flex h-full flex-col items-center justify-center gap-3 overflow-auto p-6 text-center"
      aria-label="DSH"
    >
      <h2 className="text-lg font-semibold">DSH</h2>
      {!root ? (
        <p className="text-sm text-(--theme-text-muted)">
          请先在主页选择机器人目录。
        </p>
      ) : failedToOpen ? (
        <>
          <p role="alert" className="text-sm">
            {failure?.error || 'DSH Web 启动失败。'}
          </p>
          <Button
            variant="secondary"
            disabled={isLoading}
            onClick={() => void openWeb(root)}
          >
            重试打开 DSH
          </Button>
        </>
      ) : isLoading || isFetching || (!isError && !status) || status?.ready ? (
        <p role="status" className="text-sm text-(--theme-text-muted)">
          {status?.ready ? '正在打开 DSH…' : '正在检查 DSH…'}
        </p>
      ) : (
        <>
          <p role="alert" className="text-sm">
            {isError
              ? '无法检查 DSH 安装状态。'
              : status?.message ||
                '尚未安装 DSH。请准备好 DSH 运行环境后重试。'}
          </p>
          <RefreshButton label="重新检查" onClick={() => void refetch()} />
        </>
      )}
    </section>
  )
}
