import { useCallback, useEffect, useImperativeHandle, useRef, useState, type Ref } from 'react'
import { createPortal } from 'react-dom'
import {
  ClipboardList,
  Download,
  Terminal
} from 'lucide-react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Button } from './Button'
import { DesktopWindow } from './DesktopWindow'
import { TerminalTabBar } from './TerminalTabBar'
import { WorkbenchTools } from './WorkbenchTools'
import { directoryName } from '../lib/directoryName'

type LocalTerminal = {
  id: string
  key: string
  cwd: string
  shell: string
  output: string
  truncated?: boolean
}

export type TerminalPanelHandle = {
  addSession: () => Promise<void>
}

const headersFor = (item: LocalTerminal) => ({
  'Content-Type': 'application/json',
  'X-ALX-Terminal-Key': item.key
})

function deleteTerminalSession(item: LocalTerminal) {
  void fetch(`/api/v1/terminal/sessions/${item.id}`, {
    method: 'DELETE',
    headers: headersFor(item),
    keepalive: true
  }).catch(() => {})
}

export function TerminalPanel({
  root,
  onClose,
  native = false,
  visible = true,
  toolbarTarget,
  onActivate,
  ref
}: {
  root: string
  onClose: () => void
  native?: boolean
  visible?: boolean
  toolbarTarget?: HTMLDivElement | null
  onActivate?: () => void
  ref?: Ref<TerminalPanelHandle>
}) {
  const storageKey = `alx-terminal-sessions:${root}`
  const hostRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<XTerm | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const writtenRef = useRef(0)
  const creatingRef = useRef(false)
  const closedRef = useRef(false)
  const restoredStorage = useRef('')
  const restoringSessions = useRef<Promise<void> | null>(null)
  const [sessions, setSessions] = useState<LocalTerminal[]>([])
  const [activeID, setActiveID] = useState('')
  const active = sessions.find(item => item.id === activeID) ?? sessions[0]
  const activeOutputRef = useRef('')
  useEffect(() => {
    activeOutputRef.current = active?.output ?? ''
  }, [active?.output])

  const updateSession = useCallback(
    (id: string, update: (current: LocalTerminal) => LocalTerminal) =>
      setSessions(current =>
        current.map(item => (item.id === id ? update(item) : item))
      ),
    []
  )
  const createTerminal = useCallback(async () => {
    if (creatingRef.current || closedRef.current) return
    creatingRef.current = true
    try {
      const response = await fetch('/api/v1/terminal/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cwd: root })
      })
      const result = await response.json()
      if (!response.ok || typeof result.id !== 'string') {
        if (!closedRef.current) window.alert(result.error || '无法创建终端。')
        return
      }
      const item: LocalTerminal = {
        id: result.id,
        key: result.key,
        cwd: result.cwd,
        shell: result.shell,
        output: ''
      }
      if (closedRef.current) {
        deleteTerminalSession(item)
        return
      }
      setSessions(current => [...current, item])
      setActiveID(item.id)
    } catch {
      if (!closedRef.current) window.alert('无法连接终端服务。')
    } finally {
      creatingRef.current = false
    }
  }, [root])

  useImperativeHandle(ref, () => ({
    async addSession() {
      await restoringSessions.current
      await createTerminal()
    }
  }), [createTerminal])

  // Restore only descriptors, never credentials in long-lived storage. The
  // daemon keeps the PTY alive across a browser refresh and validates every
  // descriptor before it becomes visible again.
  useEffect(() => {
    if (restoredStorage.current === storageKey) return
    restoredStorage.current = storageKey
    const saved = sessionStorage.getItem(storageKey)
    if (!saved) {
      void createTerminal()
      return
    }
    let descriptors: Array<Pick<LocalTerminal, 'id' | 'key'>> = []
    try {
      descriptors = JSON.parse(saved)
    } catch {
      sessionStorage.removeItem(storageKey)
    }
    restoringSessions.current = Promise.all(
      descriptors.map(async ({ id, key }): Promise<LocalTerminal | null> => {
        const response = await fetch(
          `/api/v1/terminal/sessions/${id}`,
          { headers: { 'X-ALX-Terminal-Key': key } }
        )
        if (!response.ok) return null
        const result = await response.json()
        return {
          id,
          key,
          cwd: result.cwd,
          shell: result.shell,
          output: result.output || '',
          truncated: result.truncated
        }
      })
    )
      .then(restored => {
        const items = restored.filter(item => item !== null)
        if (closedRef.current) {
          items.forEach(deleteTerminalSession)
          return
        }
        if (items.length) {
          setSessions(items)
          setActiveID(items[0].id)
        } else void createTerminal()
      })
      .catch(() => {})
  }, [createTerminal, storageKey])
  useEffect(() => {
    if (!sessions.length) return
    sessionStorage.setItem(
      storageKey,
      JSON.stringify(sessions.map(({ id, key }) => ({ id, key })))
    )
  }, [sessions, storageKey])
  useEffect(() => {
    if (!sessions.length) return
    const timer = window.setInterval(() => {
      sessions.forEach(
        item =>
          void fetch(`/api/v1/terminal/sessions/${item.id}/heartbeat`, {
            method: 'POST',
            headers: headersFor(item)
          })
      )
    }, 30_000)
    return () => window.clearInterval(timer)
  }, [sessions])
  useEffect(() => {
    const host = hostRef.current
    if (!active || !host) return
    const terminal = new XTerm({
      cursorBlink: false,
      cursorStyle: 'bar',
      cursorWidth: 1,
      convertEol: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: 13
    })
    const syncTheme = () => {
      const colors = getComputedStyle(host)
      const background = colors.getPropertyValue('--theme-terminal-background').trim()
      const foreground = colors.getPropertyValue('--theme-terminal-foreground').trim()
      terminal.options.theme = {
        background,
        foreground,
        cursor: foreground,
        cursorAccent: background,
        selectionBackground: colors.getPropertyValue('--theme-surface-active').trim()
      }
    }
    syncTheme()
    window.addEventListener('alx:theme-changed', syncTheme)
    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(host)
    fit.fit()
    terminal.write(active.output)
    writtenRef.current = active.output.length
    xtermRef.current = terminal
    fitRef.current = fit
    const send = (input: string) =>
      void fetch(`/api/v1/terminal/sessions/${active.id}/input`, {
        method: 'POST',
        headers: headersFor(active),
        body: JSON.stringify({ input })
      })
    const resize = ({ cols, rows }: { cols: number; rows: number }) =>
      void fetch(`/api/v1/terminal/sessions/${active.id}/resize`, {
        method: 'POST',
        headers: headersFor(active),
        body: JSON.stringify({ cols, rows })
      })
    const input = terminal.onData(send)
    const size = terminal.onResize(resize)
    resize({ cols: terminal.cols, rows: terminal.rows })
    const observer = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        // The panel can be hidden while its session stays active.
      }
    })
    observer.observe(host)
    return () => {
      window.removeEventListener('alx:theme-changed', syncTheme)
      observer.disconnect()
      input.dispose()
      size.dispose()
      terminal.dispose()
      xtermRef.current = null
      fitRef.current = null
    }
  }, [active?.id])
  useEffect(() => {
    if (
      !active ||
      !xtermRef.current ||
      active.output.length <= writtenRef.current
    )
      return
    xtermRef.current.write(active.output.slice(writtenRef.current))
    writtenRef.current = active.output.length
  }, [active?.output, active?.id])
  useEffect(() => {
    if (!active) return
    const source = new EventSource(
      `/api/v1/terminal/sessions/${active.id}/stream?key=${encodeURIComponent(active.key)}`
    )
    source.onmessage = event => {
      const payload = JSON.parse(event.data) as {
        text?: string
        snapshot?: boolean
        truncated?: boolean
      }
      if (payload.truncated)
        updateSession(active.id, item => ({ ...item, truncated: true }))
      if (
        payload.snapshot &&
        typeof payload.text === 'string' &&
        !payload.text.startsWith(
          activeOutputRef.current.slice(0, writtenRef.current)
        )
      ) {
        xtermRef.current?.reset()
        writtenRef.current = 0
      }
      if (payload.text)
        updateSession(active.id, item => ({
          ...item,
          output: payload.snapshot
            ? payload.text || ''
            : `${item.output}${payload.text}`
        }))
    }
    return () => source.close()
  }, [active?.id, active?.key, updateSession])
  useEffect(() => {
    if (!visible) return
    window.requestAnimationFrame(() => {
      fitRef.current?.fit()
      xtermRef.current?.focus()
    })
  }, [active?.id, visible])

  function closeAll() {
    if (closedRef.current) return
    closedRef.current = true
    sessions.forEach(deleteTerminalSession)
    setSessions([])
    setActiveID('')
    sessionStorage.removeItem(storageKey)
    onClose()
  }

  function closeTerminal(id: string) {
    const item = sessions.find(session => session.id === id)
    if (!item) return
    if (sessions.length === 1 && sessions[0].id === item.id) {
      closeAll()
      return
    }
    deleteTerminalSession(item)
    setSessions(current => current.filter(candidate => candidate.id !== item.id))
    setActiveID(current => (current === item.id ? '' : current))
  }
  const copyOutput = () =>
    active && void navigator.clipboard?.writeText(active.output)
  const downloadOutput = () => {
    if (!active) return
    const url = URL.createObjectURL(
      new Blob([active.output], { type: 'text/plain;charset=utf-8' })
    )
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${directoryName(active.cwd) || '终端'}.log`
    anchor.click()
    URL.revokeObjectURL(url)
  }
  const tabs = (
    <TerminalTabBar
      tabs={sessions}
      activeID={visible ? active?.id : undefined}
      onSelect={id => {
        setActiveID(id)
        onActivate?.()
      }}
      onClose={closeTerminal}
      onAdd={toolbarTarget ? undefined : () => void createTerminal()}
    />
  )
  const actions = (
    <WorkbenchTools
      label="终端更多操作"
      menuLabel="终端操作菜单"
      menuClassName="workbench-side-submenu"
      align="end"
      restoreFocusOnClose
    >
      {close => (
        <>
          <Button
            type="button"
            className="workbench-action-item"
            disabled={!active}
            onClick={() => {
              copyOutput()
              close()
            }}
          >
            <ClipboardList className="size-4 shrink-0" aria-hidden="true" />
            复制输出
          </Button>
          <Button
            type="button"
            className="workbench-action-item"
            disabled={!active}
            onClick={() => {
              downloadOutput()
              close()
            }}
          >
            <Download className="size-4 shrink-0" aria-hidden="true" />
            下载输出
          </Button>
        </>
      )}
    </WorkbenchTools>
  )
  const content = (
    <div className="flex min-h-0 flex-1 flex-col">
      {active?.truncated && (
        <p className="px-2 py-1 text-xs text-amber-600">
          早期输出已被循环缓冲清理；终端仍在运行。
        </p>
      )}
      <div className="terminal-panel-output min-h-0 min-w-0 flex-1 overflow-hidden bg-(--theme-terminal-background)">
        <div
          ref={hostRef}
          className="h-full"
          onPointerDown={() => xtermRef.current?.focus()}
        />
      </div>
    </div>
  )

  if (native) {
    return (
      <DesktopWindow
        native
        id="terminal"
        open
        minimized={false}
        title="终端"
        subtitle={active ? `${active.cwd} · ${active.shell}` : '本机交互式终端'}
        icon={<Terminal className="size-4" />}
        onClose={closeAll}
        onMinimize={() => {}}
        zIndex={100}
        onActivate={() => {}}
        headerLeft={tabs}
        actions={actions}
      >
        {content}
      </DesktopWindow>
    )
  }
  return (
    <aside
      aria-label="终端面板"
      hidden={!visible}
      className="workbench-terminal min-h-0 shrink-0 flex-col bg-(--theme-surface-panel)"
    >
      {toolbarTarget ? createPortal(<>{tabs}{visible && actions}</>, toolbarTarget) : (
        <header className="flex min-h-11 min-w-0 shrink-0 items-center gap-1 px-1">
          {tabs}
          {actions}
        </header>
      )}
      {content}
    </aside>
  )
}
