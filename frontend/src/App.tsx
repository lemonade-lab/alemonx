import { useStoreState } from './store/guideStore'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState
} from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  setDeveloper,
  setProject,
  toggleCapability as toggleCapabilityAction,
  type RootState
} from './store/guideStore'
import { Dashboard } from './components/Dashboard'
import { AppSettingsPanel } from './components/AppSettingsPanel'
import { isWindowHeaderInteractiveTarget } from './components/desktopWindowInteraction'
import { registerDesktopWindowShortcut } from './components/desktopWindowShortcuts'
import { EnvironmentFixDialog } from './components/EnvironmentFixDialog'
import { DownloadProgress } from './components/DownloadProgress'
import { ErrorNotice } from './components/ErrorNotice'
import { clampWindowRectToViewport } from './lib/windowRect'
import {
  workspaceApi,
  useGoalsQuery,
  useWorkspaceQuery,
  useLazyEnvironmentReportQuery,
  useReleasesQuery
} from './store/workspaceApi'
import { GuideHome } from './features/guide/GuideHome'
import { EnvironmentCheckPanel } from './features/guide/EnvironmentCheckPanel'
import { guideIcons as icons } from './features/guide/icons'
import { recommendReleaseAssets } from './features/guide/releaseAssets'
import {
  Activity,
  ClipboardList,
  FlaskConical,
  GitBranch,
  Home,
  LayoutGrid,
  MessageSquare,
  Monitor,
  Settings,
  ShieldCheck,
  Terminal
} from 'lucide-react'
import type {
  Check,
  Creation,
  Goal,
  Mirror,
  ProjectConfig,
  Release,
  Report
} from './features/guide/types'

const capabilityLabels: Record<string, string> = {
  bubble: '气泡服务',
  database: '数据存储',
  discord: 'Discord',
  onebot: 'OneBot',
  qqbot: 'QQ Bot'
}

type DockWindowState = { open: boolean; minimized: boolean }
type SystemDockWindowState = DockWindowState & { label: string }
type DockWindows = {
  terminal: DockWindowState
  foregroundLogs: DockWindowState
  git: DockWindowState
  app: DockWindowState
  test: DockWindowState
  live: DockWindowState
  pm2Logs: DockWindowState
  pm2Status: DockWindowState
  ops: DockWindowState
  system: Record<string, SystemDockWindowState>
}
type ResizeCorner = 'nw' | 'ne' | 'sw' | 'se'
type WorkbenchRect = {
  left: number
  top: number
  width: number
  height: number
}

// Keep this in lockstep with the wide-screen Dock media query in styles.css.
// Below this width the workbench is the page itself, not a floating window.
const WORKBENCH_DOCK_BREAKPOINT = '(min-width: 1440px)'

function responsiveWorkbenchRect(): WorkbenchRect {
  return {
    left: 0,
    top: 0,
    width: window.innerWidth,
    height: window.innerHeight
  }
}

function initialWorkbenchRect(): WorkbenchRect {
  if (!window.matchMedia(WORKBENCH_DOCK_BREAKPOINT).matches)
    return responsiveWorkbenchRect()
  const width = Math.min(1240, Math.max(640, window.innerWidth - 48))
  const height = Math.min(760, Math.max(420, window.innerHeight - 56))
  return {
    left: Math.max(16, Math.round((window.innerWidth - width) / 2)),
    top: Math.max(16, Math.round((window.innerHeight - height) / 2)),
    width,
    height
  }
}

function clampWorkbenchRect(rect: WorkbenchRect): WorkbenchRect {
  if (!window.matchMedia(WORKBENCH_DOCK_BREAKPOINT).matches)
    return responsiveWorkbenchRect()
  return clampWindowRectToViewport(rect, {
    minWidth: 640,
    minHeight: 420,
    gutter: 32,
    minViewportWidth: 640,
    minViewportHeight: 420
  })
}

const closedDockWindow: DockWindowState = { open: false, minimized: false }
const emptyDockWindows: DockWindows = {
  terminal: closedDockWindow,
  foregroundLogs: closedDockWindow,
  git: closedDockWindow,
  app: closedDockWindow,
  test: closedDockWindow,
  live: closedDockWindow,
  pm2Logs: closedDockWindow,
  pm2Status: closedDockWindow,
  ops: closedDockWindow,
  system: {}
}

export default function App() {
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useStoreState('')
  const [creating, setCreating] = useStoreState(false)
  const [creation, setCreation] = useStoreState<Creation | null>(null)
  const [repairCheck, setRepairCheck] = useStoreState<Check | null>(null)
  const { data: goalData, isLoading: loading } = useGoalsQuery()
  const [
    loadEnvironmentReport,
    { data: environmentData, isFetching: checking }
  ] = useLazyEnvironmentReportQuery()
  const report = (environmentData as Report | undefined) ?? null
  const goals = (goalData as Goal[] | null | undefined) ?? []
  const settingsTitle = '设置'
  const routeGoal = location.pathname.match(
    /^\/guide\/([^/]+)\/step\/\d+$/
  )?.[1]
  const guideGroup = location.pathname.match(/^\/guide\/group\/([^/]+)$/)?.[1]
  const selectedID = routeGoal ?? null
  const guideOpen = !location.pathname.startsWith('/dashboard')
  const activeID = selectedID ?? 'install'
  const activeGoal = goals.find(goal => goal.id === activeID)
  const [workbenchRect, setWorkbenchRect] =
    useState<WorkbenchRect>(initialWorkbenchRect)
  const workbenchRectRef = useRef(workbenchRect)
  // The workbench's preferred (user-chosen) size. `workbenchRect` stays the
  // viewport-clamped display rect, so shrinking the browser only shrinks the
  // window temporarily and growing it back restores the remembered size.
  const workbenchPreferredRef = useRef(workbenchRect)
  const previewRect = useRef<WorkbenchRect | null>(null)
  const previewFrame = useRef<number | null>(null)
  const [workbenchMaximized, setWorkbenchMaximized] = useState(false)
  const workbenchMaximizedRef = useRef(workbenchMaximized)
  const [isResponsiveWorkbench, setIsResponsiveWorkbench] = useState(
    () => !window.matchMedia(WORKBENCH_DOCK_BREAKPOINT).matches
  )
  const wasResponsiveWorkbench = useRef(isResponsiveWorkbench)
  const [workbenchLayer, setWorkbenchLayer] = useState(100)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsMinimized, setSettingsMinimized] = useState(false)
  const [settingsLayer, setSettingsLayer] = useState(108)
  const [dockWindows, setDockWindows] = useState<DockWindows>(emptyDockWindows)
  const [mainWindowHidden, setMainWindowHidden] = useState(false)
  const [launchpadOpen, setLaunchpadOpen] = useState(false)
  const nextWindowLayer = useRef(106)
  const dragState = useRef<{
    pointerId: number
    startX: number
    startY: number
    originLeft: number
    originTop: number
  } | null>(null)
  const resizeState = useRef<{
    pointerId: number
    corner: ResizeCorner
    startX: number
    startY: number
    width: number
    height: number
    left: number
    top: number
  } | null>(null)

  useEffect(
    () =>
      registerDesktopWindowShortcut({
        id: 'workbench',
        zIndex: workbenchLayer,
        minimized: mainWindowHidden,
        onClose: () => setMainWindowHidden(true),
        onMinimize: () => setMainWindowHidden(true)
      }),
    [mainWindowHidden, workbenchLayer]
  )

  function activateWorkbench() {
    const layer = ++nextWindowLayer.current
    setWorkbenchLayer(layer)
    window.dispatchEvent(
      new CustomEvent('alx:desktop-window-layer', { detail: layer })
    )
  }

  function openSettings() {
    const layer = ++nextWindowLayer.current
    setSettingsLayer(layer)
    setSettingsOpen(true)
    setSettingsMinimized(false)
    window.dispatchEvent(
      new CustomEvent('alx:desktop-window-layer', { detail: layer })
    )
  }

  function previewWorkbenchRect(rect: WorkbenchRect) {
    previewRect.current = rect
    if (previewFrame.current !== null) return
    previewFrame.current = window.requestAnimationFrame(() => {
      previewFrame.current = null
      const windowElement = document.querySelector<HTMLElement>('.guide-window')
      const next = previewRect.current
      if (!windowElement || !next) return
      windowElement.style.left = `${next.left}px`
      windowElement.style.top = `${next.top}px`
      windowElement.style.width = `${next.width}px`
      windowElement.style.height = `${next.height}px`
    })
  }

  function commitWorkbenchPreview(kind: 'drag' | 'resize', changed: boolean) {
    const rect = previewRect.current ?? workbenchRectRef.current
    if (previewFrame.current !== null) {
      window.cancelAnimationFrame(previewFrame.current)
      previewFrame.current = null
    }
    previewRect.current = null
    workbenchRectRef.current = rect
    if (changed) {
      workbenchPreferredRef.current =
        kind === 'drag'
          ? { ...workbenchPreferredRef.current, left: rect.left, top: rect.top }
          : rect
    }
    setWorkbenchRect(rect)
  }

  function beginWorkbenchDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (isResponsiveWorkbench) return
    const target = event.target as HTMLElement
    if (target.closest('.guide-window') && !hasOpenDesktopWindow)
      activateWorkbench()
    const topbar = target.closest('.topbar')
    if (
      !topbar?.closest('.guide-window') ||
      isWindowHeaderInteractiveTarget(target)
    )
      return
    dragState.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      originLeft: workbenchRectRef.current.left,
      originTop: workbenchRectRef.current.top
    }
    event.currentTarget.setPointerCapture(event.pointerId)
    event.currentTarget.classList.add('workbench-dragging')
  }

  function moveWorkbench(event: React.PointerEvent<HTMLDivElement>) {
    const resize = resizeState.current
    if (resize?.pointerId === event.pointerId) {
      const minWidth = 640
      const minHeight = 420
      const maxWidth = Math.max(minWidth, window.innerWidth - 32)
      const maxHeight = Math.max(minHeight, window.innerHeight - 32)
      const horizontal = resize.corner.endsWith('e') ? 1 : -1
      const vertical = resize.corner.startsWith('s') ? 1 : -1
      const width = Math.max(
        minWidth,
        Math.min(
          maxWidth,
          resize.width + horizontal * (event.clientX - resize.startX)
        )
      )
      const height = Math.max(
        minHeight,
        Math.min(
          maxHeight,
          resize.height + vertical * (event.clientY - resize.startY)
        )
      )
      previewWorkbenchRect({
        width,
        height,
        left: resize.corner.endsWith('w')
          ? resize.left + resize.width - width
          : resize.left,
        top: resize.corner.startsWith('n')
          ? resize.top + resize.height - height
          : resize.top
      })
      return
    }
    const drag = dragState.current
    if (!drag || drag.pointerId !== event.pointerId) return
    const current = workbenchRectRef.current
    previewWorkbenchRect({
      ...current,
      left: Math.max(
        16,
        Math.min(
          window.innerWidth - current.width - 16,
          drag.originLeft + event.clientX - drag.startX
        )
      ),
      top: Math.max(
        16,
        Math.min(
          window.innerHeight - current.height - 16,
          drag.originTop + event.clientY - drag.startY
        )
      )
    })
  }

  function endWorkbenchDrag(event: React.PointerEvent<HTMLDivElement>) {
    if (resizeState.current?.pointerId === event.pointerId) {
      const start = resizeState.current
      const rect = previewRect.current ?? workbenchRectRef.current
      const changed =
        rect.width !== start.width ||
        rect.height !== start.height ||
        rect.left !== start.left ||
        rect.top !== start.top
      resizeState.current = null
      commitWorkbenchPreview('resize', changed)
      event.currentTarget.releasePointerCapture(event.pointerId)
      event.currentTarget.classList.remove('workbench-dragging')
      return
    }
    if (dragState.current?.pointerId !== event.pointerId) return
    const start = dragState.current
    const rect = previewRect.current ?? workbenchRectRef.current
    const changed =
      rect.left !== start.originLeft || rect.top !== start.originTop
    dragState.current = null
    commitWorkbenchPreview('drag', changed)
    event.currentTarget.releasePointerCapture(event.pointerId)
    event.currentTarget.classList.remove('workbench-dragging')
  }

  function beginWorkbenchResize(
    corner: ResizeCorner,
    event: React.PointerEvent<HTMLButtonElement>
  ) {
    if (isResponsiveWorkbench) return
    const windowElement = document.querySelector<HTMLElement>('.guide-window')
    const stage =
      event.currentTarget.closest<HTMLDivElement>('.workbench-stage')
    if (!windowElement || !stage) return
    const rect = windowElement.getBoundingClientRect()
    event.preventDefault()
    event.stopPropagation()
    if (!hasOpenDesktopWindow) activateWorkbench()
    resizeState.current = {
      pointerId: event.pointerId,
      corner,
      startX: event.clientX,
      startY: event.clientY,
      width: rect.width,
      height: rect.height,
      left: rect.left,
      top: rect.top
    }
    workbenchRectRef.current = {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height
    }
    stage.setPointerCapture(event.pointerId)
    stage.classList.add('workbench-dragging')
  }

  function toggleWorkbenchMaximize(event: React.MouseEvent<HTMLDivElement>) {
    if (isResponsiveWorkbench) return
    const target = event.target as HTMLElement
    const topbar = target.closest('.topbar')
    if (
      !topbar?.closest('.guide-window') ||
      isWindowHeaderInteractiveTarget(target)
    )
      return
    if (hasOpenDesktopWindow) return
    activateWorkbench()
    if (workbenchMaximized) {
      setWorkbenchMaximized(false)
      setWorkbenchRect(clampWorkbenchRect(workbenchPreferredRef.current))
      return
    }
    setWorkbenchRect({
      left: 16,
      top: 16,
      width: Math.max(640, window.innerWidth - 48),
      height: Math.max(420, window.innerHeight - 56)
    })
    setWorkbenchMaximized(true)
  }

  useEffect(() => {
    if (location.pathname === '/') navigate('/guide', { replace: true })
  }, [location.pathname, navigate])
  useEffect(() => {
    const syncWindowLayer = (event: Event) => {
      const layer = (event as CustomEvent<number>).detail
      if (typeof layer === 'number')
        nextWindowLayer.current = Math.max(nextWindowLayer.current, layer)
    }
    window.addEventListener('alx:desktop-window-layer', syncWindowLayer)
    return () =>
      window.removeEventListener('alx:desktop-window-layer', syncWindowLayer)
  }, [])
  useLayoutEffect(() => {
    const media = window.matchMedia(WORKBENCH_DOCK_BREAKPOINT)
    const applyWorkbenchLayout = () => {
      const responsive = !media.matches
      setIsResponsiveWorkbench(responsive)
      if (responsive) {
        wasResponsiveWorkbench.current = true
        workbenchMaximizedRef.current = false
        setWorkbenchRect(responsiveWorkbenchRect())
        setWorkbenchMaximized(false)
        return
      }
      // Crossing into the desktop/Dock layout must not resurrect a smaller
      // remembered rectangle. Capture the viewport's largest available size at
      // this exact transition, then keep that captured size as the baseline.
      if (wasResponsiveWorkbench.current) {
        const maximumRect = {
          left: 16,
          top: 16,
          width: Math.max(640, window.innerWidth - 48),
          height: Math.max(420, window.innerHeight - 56)
        }
        wasResponsiveWorkbench.current = false
        workbenchPreferredRef.current = maximumRect
        workbenchMaximizedRef.current = false
        setWorkbenchRect(maximumRect)
        // This is a captured window size, not live maximization: later browser
        // resizes must not keep expanding the workbench.
        setWorkbenchMaximized(false)
        return
      }
      if (workbenchMaximizedRef.current) {
        setWorkbenchRect({
          left: 16,
          top: 16,
          width: Math.max(640, window.innerWidth - 48),
          height: Math.max(420, window.innerHeight - 56)
        })
        return
      }
      setWorkbenchRect(clampWorkbenchRect(workbenchPreferredRef.current))
    }
    applyWorkbenchLayout()
    window.addEventListener('resize', applyWorkbenchLayout)
    media.addEventListener('change', applyWorkbenchLayout)
    return () => {
      window.removeEventListener('resize', applyWorkbenchLayout)
      media.removeEventListener('change', applyWorkbenchLayout)
    }
  }, [])
  useLayoutEffect(() => {
    workbenchRectRef.current = workbenchRect
  }, [workbenchRect])
  useEffect(() => {
    workbenchMaximizedRef.current = workbenchMaximized
  }, [workbenchMaximized])
  useEffect(
    () => () => {
      if (previewFrame.current !== null)
        window.cancelAnimationFrame(previewFrame.current)
    },
    []
  )
  useEffect(() => {
    if (guideOpen) setDockWindows(emptyDockWindows)
    setMainWindowHidden(false)
  }, [guideOpen])
  function openGuide() {
    navigate('/guide')
    setCreation(null)
    setError('')
  }
  async function checkEnvironment(variant?: string) {
    const selectedVariant = typeof variant === 'string' ? variant : ''
    try {
      setError('')
      // Invalidate the cached result first so "重新检查" always performs a
      // fresh server-side environment check instead of returning the cached
      // 5-minute report with no visible change.
      workspaceApi.util.invalidateTags([
        { type: 'EnvironmentReport', id: `${activeID}:${selectedVariant}` }
      ])
      await loadEnvironmentReport(
        { goalId: activeID, variant: selectedVariant },
        false
      ).unwrap()
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : '环境检查未完成，请稍后重试。'
      )
    }
  }
  async function createProject(config: ProjectConfig) {
    try {
      setCreating(true)
      setError('')
      setCreation(null)
      const response = await fetch('/api/v1/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
      })
      const data = (await response.json()) as Creation & {
        error?: string
        result?: Creation
      }
      setCreation(data.result ?? data)
      if (!response.ok)
        setError(data.error ?? '项目创建未完成，请检查下方日志。')
    } catch {
      setError('项目创建请求未完成，请稍后重试。')
    } finally {
      setCreating(false)
    }
  }

  const handleWindowStateChange = useCallback((state: DockWindows) => {
    setDockWindows(state)
    if (
      [
        state.terminal,
        state.foregroundLogs,
        state.git,
        state.app,
        state.live,
        state.pm2Logs,
        state.pm2Status,
        state.ops,
        ...Object.values(state.system)
      ].some(item => item.open)
    )
      setMainWindowHidden(false)
  }, [])

  const windowStyle = {
    position: 'fixed' as const,
    left: workbenchRect.left,
    top: workbenchRect.top,
    width: workbenchRect.width,
    height: workbenchRect.height,
    maxWidth: 'none',
    maxHeight: 'none'
  }
  const workbenchWindowControls =
    workbenchMaximized || isResponsiveWorkbench ? null : (
      <>
        {(['nw', 'ne', 'sw', 'se'] as ResizeCorner[]).map(corner => (
          <button
            className={`desktop-window-resize desktop-window-resize-${corner}`}
            onPointerDown={event => beginWorkbenchResize(corner, event)}
            aria-label="Workbench Resize Handle"
            title="调整窗口大小"
            key={corner}
          />
        ))}
      </>
    )
  const hasOpenDesktopWindow = [
    { open: settingsOpen, minimized: settingsMinimized },
    dockWindows.terminal,
    dockWindows.foregroundLogs,
    dockWindows.git,
    dockWindows.app,
    dockWindows.live,
    dockWindows.pm2Logs,
    dockWindows.pm2Status,
    dockWindows.ops,
    ...Object.values(dockWindows.system)
  ].some(item => item.open)

  useEffect(() => {
    if (!launchpadOpen) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setLaunchpadOpen(false)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [launchpadOpen])

  return (
    <div className="app-shell">
      <div
        className="workbench-stage"
        onPointerDown={beginWorkbenchDrag}
        onPointerMove={moveWorkbench}
        onPointerUp={endWorkbenchDrag}
        onPointerCancel={endWorkbenchDrag}
        onDoubleClick={toggleWorkbenchMaximize}
      >
        <WorkbenchDock
          windowLabel={guideOpen ? 'Guide' : 'Work'}
          windowHidden={mainWindowHidden}
          hasOpenDesktopWindow={hasOpenDesktopWindow}
          launchpadOpen={launchpadOpen}
          onToggleLaunchpad={() => setLaunchpadOpen(value => !value)}
          onToggleWindow={() => {
            // Application windows stay in front of the workbench. Restoring
            // the workbench must not raise it above them and cover the stack.
            if (hasOpenDesktopWindow) {
              setMainWindowHidden(false)
              return
            }
            setMainWindowHidden(value => !value)
          }}
          windows={dockWindows}
          onTerminal={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-terminal-toggle'))
          }
          onForegroundLogs={() =>
            window.dispatchEvent(
              new CustomEvent('alx:desktop-foreground-logs-toggle')
            )
          }
          onGit={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-git-toggle'))
          }
          onApp={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-app-toggle'))
          }
          onTest={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-test-toggle'))
          }
          onLive={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-live-toggle'))
          }
          onPM2Logs={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-pm2-logs-toggle'))
          }
          onPM2Status={() =>
            window.dispatchEvent(
              new CustomEvent('alx:desktop-pm2-status-toggle')
            )
          }
          onOps={() =>
            window.dispatchEvent(new CustomEvent('alx:desktop-ops-toggle'))
          }
          onSystem={feature =>
            window.dispatchEvent(
              new CustomEvent('alx:desktop-system-toggle', { detail: feature })
            )
          }
        />
        <div
          className={`workbench-window-layer${mainWindowHidden ? ' workbench-window-hidden' : ''}`}
          style={{ zIndex: workbenchLayer }}
        >
          {guideOpen ? (
            <GuideHome
              loading={loading}
              group={guideGroup}
              goal={selectedID ? activeGoal : undefined}
              report={report}
              checking={checking}
              error={error}
              creating={creating}
              creation={creation}
              onSelect={id => {
                if (id === 'manage') {
                  navigate('/dashboard/robot')
                  return
                }
                navigate(id ? `/guide/${id}/step/1` : '/guide')
                setCreation(null)
                setError('')
              }}
              onClose={() => navigate('/dashboard')}
              onOpenSettings={openSettings}
              onClearError={() => setError('')}
              onCheck={checkEnvironment}
              onCreate={createProject}
              onFix={setRepairCheck}
              windowStyle={windowStyle}
              windowControls={workbenchWindowControls}
              renderFlow={registerBack => (
                <FlowView
                  loading={loading}
                  goal={selectedID ? activeGoal : undefined}
                  report={report}
                  checking={checking}
                  creating={creating}
                  creation={creation}
                  onSelect={id => {
                    if (id === 'manage') {
                      navigate('/dashboard/robot')
                      return
                    }
                    navigate(id ? `/guide/${id}/step/1` : '/guide')
                    setCreation(null)
                    setError('')
                  }}
                  onCheck={checkEnvironment}
                  onCreate={createProject}
                  onFix={setRepairCheck}
                  registerBack={registerBack}
                />
              )}
            />
          ) : (
            <Dashboard
              goals={goals}
              goal={activeGoal}
              report={report}
              checking={checking}
              error={error}
              onClearError={() => setError('')}
              defaultPage={
                location.pathname.endsWith('/robot') ? 'robot' : 'environment'
              }
              onSelect={id => {
                if (id === 'manage') {
                  navigate('/dashboard/robot')
                  return
                }
                navigate(`/guide/${id}/step/1`)
                setError('')
              }}
              onOpenGuide={openGuide}
              onOpenSettings={openSettings}
              onCheck={checkEnvironment}
              onFix={setRepairCheck}
              onWindowStateChange={handleWindowStateChange}
              windowStyle={windowStyle}
              windowControls={workbenchWindowControls}
            />
          )}
        </div>
      </div>
      {launchpadOpen && (
        <Launchpad
          mainWindowLabel={guideOpen ? 'Guide' : 'Workbench'}
          windows={dockWindows}
          onClose={() => setLaunchpadOpen(false)}
          onOpenMainWindow={() => {
            setMainWindowHidden(false)
            activateWorkbench()
          }}
          onOpenSettings={() => {
            openSettings()
          }}
          onOpenWindow={(name, open, minimized) => {
            if (!open || minimized)
              window.dispatchEvent(new CustomEvent(`alx:desktop-${name}-toggle`))
          }}
          onOpenSystemWindow={(feature, minimized) => {
            const state = dockWindows.system[feature]
            if (!state?.open)
              window.dispatchEvent(
                new CustomEvent('alx:desktop-system-open', { detail: feature })
              )
            else if (minimized)
            window.dispatchEvent(
              new CustomEvent('alx:desktop-system-toggle', { detail: feature })
            )
          }}
        />
      )}
      <AppSettingsPanel
        id="app-settings"
        open={settingsOpen}
        minimized={settingsMinimized}
        title={settingsTitle}
        icon={
          <Settings className="size-4 text-brand-600 dark:text-brand-200" />
        }
        onClose={() => {
          setSettingsOpen(false)
          setSettingsMinimized(false)
        }}
        onMinimize={() => setSettingsMinimized(true)}
        zIndex={settingsLayer}
        onActivate={openSettings}
        initialPosition={{ left: 180, top: 120 }}
        width={820}
        height={640}
      />
      {repairCheck && (
        <EnvironmentFixDialog
          check={repairCheck}
          platform={report?.platform}
          onInstalled={() => void checkEnvironment()}
          onClose={() => setRepairCheck(null)}
        />
      )}
    </div>
  )
}

function WorkbenchDock({
  windowLabel,
  windowHidden,
  hasOpenDesktopWindow,
  launchpadOpen,
  onToggleLaunchpad,
  onToggleWindow,
  windows,
  onTerminal,
  onForegroundLogs,
  onGit,
  onApp,
  onTest,
  onLive,
  onPM2Logs,
  onPM2Status,
  onOps,
  onSystem
}: {
  windowLabel: string
  windowHidden: boolean
  hasOpenDesktopWindow: boolean
  launchpadOpen: boolean
  onToggleLaunchpad: () => void
  onToggleWindow: () => void
  windows: DockWindows
  onTerminal: () => void
  onForegroundLogs: () => void
  onGit: () => void
  onApp: () => void
  onTest: () => void
  onLive: () => void
  onPM2Logs: () => void
  onPM2Status: () => void
  onOps: () => void
  onSystem: (feature: string) => void
}) {
  const visibleApps = [
    windows.terminal,
    windows.git,
    windows.app,
    windows.test,
    windows.live,
    windows.pm2Logs,
    windows.pm2Status,
    windows.ops,
    ...Object.values(windows.system)
  ].filter(item => item.open).length
  return (
    <aside
      className={`workbench-dock${visibleApps >= 0 ? ' workbench-dock-visible' : ''}`}
      aria-label="Workbench Dock"
    >
      <div className="workbench-dock-items">
        <button
          className={windowHidden ? '' : 'active'}
          onClick={onToggleWindow}
          title={
            hasOpenDesktopWindow
              ? `置顶${windowLabel}`
              : windowHidden
                ? `打开${windowLabel}`
                : `隐藏${windowLabel}`
          }
        >
          <Home className="size-5" />
          <span>{windowLabel}</span>
        </button>
        <button
          className={launchpadOpen ? 'active' : ''}
          onClick={onToggleLaunchpad}
          aria-pressed={launchpadOpen}
          title="打开 Launchpad"
        >
          <LayoutGrid className="size-5" />
          <span>Launchpad</span>
        </button>
        {windows.terminal.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.terminal.minimized ? '' : 'active'}
              onClick={onTerminal}
              title={windows.terminal.minimized ? '恢复终端' : '最小化终端'}
            >
              <Terminal className="size-5" />
              <span>终端</span>
            </button>
          </div>
        )}
        {windows.foregroundLogs.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.foregroundLogs.minimized ? '' : 'active'}
              onClick={onForegroundLogs}
              title={
                windows.foregroundLogs.minimized
                  ? '恢复前台日志'
                  : '最小化前台日志'
              }
            >
              <ClipboardList className="size-5" />
              <span>日志</span>
            </button>
          </div>
        )}
        {windows.git.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.git.minimized ? '' : 'active'}
              onClick={onGit}
              title={
                windows.git.minimized
                  ? '恢复 Git 仓库管理'
                  : '最小化 Git 仓库管理'
              }
            >
              <GitBranch className="size-5" />
              <span>Git</span>
            </button>
          </div>
        )}
        {windows.app.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.app.minimized ? '' : 'active'}
              onClick={onApp}
              title={windows.app.minimized ? '恢复应用' : '最小化应用'}
            >
              <Monitor className="size-5" />
              <span>应用</span>
            </button>
          </div>
        )}
        {windows.test.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.test.minimized ? '' : 'active'}
              onClick={onTest}
              title={windows.test.minimized ? '恢复测试' : '最小化测试'}
            >
              <FlaskConical className="size-5" />
              <span>测试</span>
            </button>
          </div>
        )}
        {windows.live.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.live.minimized ? '' : 'active'}
              onClick={onLive}
              title={windows.live.minimized ? '恢复在线聊天' : '最小化在线聊天'}
            >
              <MessageSquare className="size-5" />
              <span>聊天</span>
            </button>
          </div>
        )}
        {windows.pm2Logs.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.pm2Logs.minimized ? '' : 'active'}
              onClick={onPM2Logs}
              title={
                windows.pm2Logs.minimized ? '恢复 PM2 日志' : '最小化 PM2 日志'
              }
            >
              <Terminal className="size-5" />
              <span>日志</span>
            </button>
          </div>
        )}
        {windows.pm2Status.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.pm2Status.minimized ? '' : 'active'}
              onClick={onPM2Status}
              title={
                windows.pm2Status.minimized
                  ? '恢复 PM2 状态'
                  : '最小化 PM2 状态'
              }
            >
              <Activity className="size-5" />
              <span>PM2</span>
            </button>
          </div>
        )}
        {windows.ops.open && (
          <div className="workbench-dock-apps">
            <button
              className={windows.ops.minimized ? '' : 'active'}
              onClick={onOps}
              title={windows.ops.minimized ? '恢复运维' : '最小化运维'}
            >
              <ShieldCheck className="size-5" />
              <span>运维</span>
            </button>
          </div>
        )}
        {Object.entries(windows.system).filter(([, item]) => item.open).map(([feature, item]) => (
          <div className="workbench-dock-apps" key={feature}>
            <button
              className={item.minimized ? '' : 'active'}
              onClick={() => onSystem(feature)}
              title={
                item.minimized ? `恢复${item.label}` : `最小化${item.label}`
              }
            >
              <Settings className="size-5" />
              <span>{item.label}</span>
            </button>
          </div>
        ))}
      </div>
    </aside>
  )
}

function Launchpad({
  mainWindowLabel,
  windows,
  onClose,
  onOpenMainWindow,
  onOpenSettings,
  onOpenWindow,
  onOpenSystemWindow
}: {
  mainWindowLabel: string
  windows: DockWindows
  onClose: () => void
  onOpenMainWindow: () => void
  onOpenSettings: () => void
  onOpenWindow: (name: string, open: boolean, minimized: boolean) => void
  onOpenSystemWindow: (feature: string, minimized: boolean) => void
}) {
  const apps = [
    {
      id: 'main',
      label: mainWindowLabel,
      icon: <Home className="size-8" />,
      onOpen: onOpenMainWindow
    },
    {
      id: 'settings',
      label: '设置',
      icon: <Settings className="size-8" />,
      onOpen: onOpenSettings
    },
    ...([
      ['terminal', '终端', Terminal],
      ['foregroundLogs', '前台日志', ClipboardList],
      ['git', 'Git 仓库管理', GitBranch],
      ['app', '应用', Monitor],
      ['test', '测试', FlaskConical],
      ['live', '在线聊天', MessageSquare],
      ['pm2Logs', 'PM2 日志', Terminal],
      ['pm2Status', 'PM2 状态', Activity],
      ['ops', '运维', ShieldCheck]
    ] as const)
      .map(([id, label, Icon]) => ({
        id,
        label,
        icon: <Icon className="size-8" />,
        onOpen: () =>
          onOpenWindow(
            id === 'foregroundLogs'
              ? 'foreground-logs'
              : id === 'pm2Logs'
                ? 'pm2-logs'
                : id === 'pm2Status'
                  ? 'pm2-status'
                  : id,
            windows[id].open,
            windows[id].minimized
          )
      })),
    ...Object.entries(windows.system).map(([feature, item]) => ({
      id: `system-${feature}`,
      label: item.label,
      icon: <Settings className="size-8" />,
      onOpen: () => onOpenSystemWindow(feature, item.minimized)
    }))
  ]

  return (
    <div
      className="launchpad"
      role="presentation"
      onPointerDown={event => {
        if (!(event.target as HTMLElement).closest('button')) onClose()
      }}
    >
      <section className="launchpad-panel" aria-label="应用启动台">
        <div className="launchpad-apps">
          {apps.map(app => (
            <button
              className="launchpad-app"
              key={app.id}
              onClick={() => {
                app.onOpen()
                onClose()
              }}
            >
              <span className="launchpad-app-icon">{app.icon}</span>
              <strong>{app.label}</strong>
            </button>
          ))}
        </div>
      </section>
    </div>
  )
}

function FlowView({
  loading,
  goal,
  report,
  checking,
  creating,
  creation,
  onSelect,
  onCheck,
  onCreate,
  onFix,
  registerBack
}: {
  loading: boolean
  goal?: Goal
  report: Report | null
  checking: boolean
  creating: boolean
  creation: Creation | null
  onSelect: (id: string | null) => void
  onCheck: (variant?: string) => void
  onCreate: (config: ProjectConfig) => void
  onFix: (check: Check) => void
  registerBack: (handler: () => void) => void
}) {
  const navigate = useNavigate()
  const location = useLocation()
  const dispatch = useDispatch()
  const { data: workspace } = useWorkspaceQuery()
  // A previously chosen "custom folder" is persisted by redux-persist and
  // would otherwise silently divert every new robot away from the workspace
  // bots directory. Reset to the workspace option each time the wizard opens.
  useEffect(() => {
    dispatch(setProject({ destinationMode: 'current', destination: '' }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const config = useSelector((state: RootState) => state.guide.developer)
  const project = useSelector((state: RootState) => state.guide.project)
  const routedStep = Number(location.pathname.match(/\/step\/(\d+)/)?.[1] ?? 0)
  const [step, setStep] = useStoreState(routedStep)
  const [webEdition, setWebEdition] = useStoreState<'clean' | 'docker' | null>(
    null
  )
  const [buildMode, setBuildMode] = useStoreState<'npm' | 'git' | null>(null)
  const [selectedMirror, setSelectedMirror] = useStoreState<Mirror | null>(null)
  const [releaseURL, setReleaseURL] = useStoreState<string | null>(null)
  const [selectedAssetURL, setSelectedAssetURL] = useStoreState<string | null>(
    null
  )
  const [browserDownloadNotice, setBrowserDownloadNotice] = useStoreState('')
  const [folderError, setFolderError] = useStoreState('')
  const automaticCheck = useRef<string | null>(null)
  const currentStepElement = useRef<HTMLButtonElement | null>(null)
  const capabilities = config.capabilities ?? []
  const isDeveloper = goal?.id === 'develop'
  const isInstaller = goal?.id === 'install'
  const releaseApp = goal?.id === 'web' && webEdition === 'clean' ? 'alx' : null
  const {
    data: releaseData,
    isError: releaseError,
    isFetching: releaseFetching,
    refetch: refetchReleases
  } = useReleasesQuery(releaseApp ?? '', {
    skip: !releaseApp
  })
  const releases = useMemo(
    () => (releaseData as Release[] | null | undefined) ?? [],
    [releaseData]
  )
  const webSteps =
    webEdition === 'clean'
      ? [
          '选择部署方式',
          '检查 Node.js 与 Git',
          '选择下载镜像',
          '选择版本',
          '选择安装包'
        ]
      : webEdition === 'docker'
        ? ['选择部署方式', '检查 Docker', 'Docker Compose 快速启动']
        : ['选择部署方式']
  const buildSteps =
    buildMode === 'npm'
      ? ['选择构建方式', '检查 npm 环境', 'NPM 构建']
      : buildMode === 'git'
        ? ['选择构建方式', '检查 Git 环境', 'Git 化构建']
        : ['选择构建方式']
  const downloadSteps =
    goal?.id === 'mobile'
      ? ['下载 Android 安装包']
      : selectedMirror
        ? ['选择镜像', '选择版本', '选择安装包']
        : ['选择镜像']
  const totalSteps = goal
    ? [
        '选择目的',
        ...(goal.id === 'web'
          ? webSteps
          : goal.id === 'build'
            ? buildSteps
            : goal.id === 'mobile'
              ? downloadSteps
              : goal.steps)
      ]
    : ['选择目的']
  const flowStep = step - 1
  const setFlowStep = (value: number) => {
    const nextStep = Math.max(0, Math.min(value, totalSteps.length - 1))
    setStep(nextStep)
    if (goal) navigate(`/guide/${goal.id}/step/${nextStep}`)
  }
  const next = () => setFlowStep(step + 1)
  const back = () => setFlowStep(step - 1)
  const choose = (key: keyof typeof config, value: string) =>
    dispatch(setDeveloper({ [key]: value }))
  const toggleCapability = (value: string) =>
    dispatch(toggleCapabilityAction(value))
  const chooseDestination = () => {
    setFolderError('')
    window.dispatchEvent(new Event('alemon:choose-directory'))
  }
  useEffect(() => {
    if (routedStep !== step) setStep(routedStep)
  }, [routedStep, step, setStep])
  useEffect(() => {
    const variant =
      goal?.id === 'web'
        ? webEdition
        : goal?.id === 'build'
          ? buildMode
          : undefined
    const isCheckStep =
      goal?.id === 'develop' || goal?.id === 'install'
        ? flowStep === 0
        : (goal?.id === 'web' || goal?.id === 'build') &&
          Boolean(variant) &&
          flowStep === 1
    if (
      (goal?.id === 'develop' || goal?.id === 'install' || variant) &&
      isCheckStep &&
      automaticCheck.current !== `${goal?.id}:${variant ?? ''}`
    ) {
      automaticCheck.current = `${goal?.id}:${variant ?? ''}`
      onCheck(variant ?? undefined)
    }
  }, [buildMode, flowStep, goal?.id, onCheck, webEdition])
  useEffect(() => {
    currentStepElement.current?.scrollIntoView({
      behavior: 'smooth',
      block: 'center'
    })
  }, [step, setStep])
  useEffect(() => {
    if (!releaseApp) return
    setReleaseURL(releases[0]?.url ?? null)
    setSelectedAssetURL(null)
  }, [releaseApp, releases, setReleaseURL, setSelectedAssetURL])
  const mirrorURL = (mirror: Mirror | null, url: string) => {
    if (!mirror) return url
    const index = mirror.url.indexOf('https://github.com')
    return index === -1 ? url : mirror.url.slice(0, index) + url
  }
  const releasePicker = () => (
    <div className="release-picker">
      <label className="release-picker-label">
        <span>
          选择版本
          {!releases.length && !releaseError && releaseFetching && (
            <small>读取中…</small>
          )}
          {!releases.length && releaseError && (
            <small
              className="text-(--theme-danger-text)"
              title="正式版本列表获取失败，可能是网络或代理问题。"
            >
              读取失败
              <button type="button" onClick={() => void refetchReleases()}>
                重试
              </button>
            </small>
          )}
        </span>
        <select
          value={releaseURL ?? ''}
          onChange={event => {
            setReleaseURL(event.target.value)
            setSelectedAssetURL(null)
          }}
        >
          {releases.map(item => (
            <option value={item.url} key={item.tag}>
              {item.tag} · {item.name}
            </option>
          ))}
        </select>
      </label>
    </div>
  )
  const selectedRelease = releases.find(item => item.url === releaseURL)
  const releaseAssets = recommendReleaseAssets(
    selectedRelease?.assets ?? [],
    navigator.userAgent.toLowerCase()
  )
  const assetPicker = () => (
    <>
      <h1>选择安装包</h1>
      <div className="grid gap-2.5 my-5 asset-list">
        {releaseAssets.assets.map(asset => (
          <button
            className={
              selectedAssetURL === asset.url ? 'choice selected' : 'choice'
            }
            key={asset.url}
            onClick={() => setSelectedAssetURL(asset.url)}
          >
            <strong>
              {asset.name}
              {releaseAssets.isRecommended(asset) && <em>推荐</em>}
            </strong>
            <small>
              {asset.size
                ? `${(asset.size / 1024 / 1024).toFixed(1)} MB`
                : 'GitHub 安装包'}
            </small>
          </button>
        ))}
      </div>
      {selectedRelease && releaseAssets.assets.length === 0 && (
        <p>该版本没有可直接下载的安装包，请返回选择其他版本。</p>
      )}
      {browserDownloadNotice && (
        <DownloadProgress
          handoff
          label="已开始浏览器下载"
          detail={browserDownloadNotice}
        />
      )}
    </>
  )
  const developerPage = () => {
    const choices = (
      title: string,
      items: Array<[string, string, string]>,
      key: keyof typeof config
    ) => (
      <>
        <h1>{title}</h1>
        <div className="grid gap-2.5 my-5">
          {items.map(([value, label, note]) => (
            <button
              className={
                String(config[key]) === value ? 'choice selected' : 'choice'
              }
              key={value}
              onClick={() => choose(key, value)}
            >
              <strong>{label}</strong>
              <small>{note}</small>
            </button>
          ))}
        </div>
      </>
    )
    switch (flowStep) {
      case 0:
        return (
          <EnvironmentCheckPanel
            title="你的环境都准备好了吗"
            report={report}
            checking={checking}
            onCheck={() => onCheck()}
            onFix={onFix}
          />
        )
      case 1:
        return (
          <>
            <h1>给项目起个名字</h1>
            <p>会在你选择的保存位置中新建一个同名文件夹。</p>
            <div className="project-fields">
              <label>
                项目名称
                <input
                  value={project.name}
                  onChange={event =>
                    dispatch(setProject({ name: event.target.value }))
                  }
                  placeholder="my-alemonjs-bot"
                />
              </label>
              <div className="location-options">
                <button
                  className={
                    project.destinationMode === 'current'
                      ? 'choice selected'
                      : 'choice'
                  }
                  onClick={() =>
                    dispatch(setProject({ destinationMode: 'current' }))
                  }
                >
                  <strong>工作区（推荐）</strong>
                  <small>
                    {workspace
                      ? `保存到 ${workspace.bots}`
                      : '保存到工作区的 bots 目录。'}
                  </small>
                </button>
                <button
                  className={
                    project.destinationMode === 'custom'
                      ? 'choice selected'
                      : 'choice'
                  }
                  onClick={chooseDestination}
                >
                  <strong>选择指定文件夹</strong>
                  <small
                    className="truncate"
                    title={
                      project.destinationMode === 'custom' &&
                      project.destination
                        ? project.destination
                        : '在目录选择器中选择保存位置。'
                    }
                  >
                    {project.destinationMode === 'custom' && project.destination
                      ? `已选择：${project.destination}`
                      : '在目录选择器中选择保存位置。'}
                  </small>
                </button>
              </div>
              {folderError && (
                <ErrorNotice
                  message={folderError}
                  onClose={() => setFolderError('')}
                />
              )}
            </div>
          </>
        )
      case 2:
        return choices(
          '你想用哪种语言？',
          [
            ['js', 'JavaScript（推荐新手）', '写法更简单，先把机器人跑起来。'],
            ['ts', 'TypeScript', '会在写代码时提前提醒常见错误。']
          ],
          'language'
        )
      case 3:
        return choices(
          '需要代码小助手吗？',
          [
            ['yes', '需要', 'ESLint 像拼写检查，会提醒容易写错的地方。'],
            ['no', '暂时不要（默认）', '项目更简单，以后随时可以加。']
          ],
          'eslint'
        )
      case 4:
        return choices(
          '要给项目留存档吗？',
          [
            ['yes', '要（推荐）', 'Git 会记录每次修改，写错了也方便回退。'],
            ['no', '暂时不要', '不会创建版本记录。']
          ],
          'git'
        )
      case 5:
        return choices(
          '要让机器人在后台运行吗？',
          [
            [
              'yes',
              '要，使用 PM2（默认）',
              'PM2 是帮你守着机器人的小管家：关掉终端后，它仍会继续运行。'
            ],
            ['no', '暂时不要', '开发时在终端里直接运行，更容易看懂。']
          ],
          'pm2'
        )
      case 6:
        return choices(
          '用什么安装项目依赖？',
          [
            [
              'yarn',
              'Yarn（推荐）',
              '没有 Yarn 时会临时使用，不会修改电脑的全局安装。'
            ],
            ['npm', 'npm', 'Node.js 自带，不需要额外安装。'],
            ['pnpm', 'pnpm', '更省磁盘空间，需要电脑已经安装。']
          ],
          'manager'
        )
      case 7:
        return (
          <>
            <h1>选择扩展包</h1>
            <div className="grid gap-2.5 my-5">
              {[
                ['database', '数据存储', '@alemonjs/db'],
                ['qqbot', 'QQ Bot 连接', '@alemonjs/qq-bot'],
                ['onebot', 'OneBot 连接', '@alemonjs/onebot'],
                ['bubble', 'bubble服务', '@alemonjs/bubble'],
                ['discord', 'Discord 连接', '@alemonjs/discord']
              ].map(([value, label, note]) => (
                <button
                  className={
                    capabilities.includes(value) ? 'choice selected' : 'choice'
                  }
                  key={value}
                  onClick={() => toggleCapability(value)}
                >
                  <strong>{label}</strong>
                  <small
                    className="truncate"
                    title={
                      project.destinationMode === 'custom' &&
                      project.destination
                        ? project.destination
                        : '在目录选择器中选择保存位置。'
                    }
                  >
                    {note} ·{' '}
                    {capabilities.includes(value) ? '已选择' : '点击添加'}
                  </small>
                </button>
              ))}
            </div>
          </>
        )
      case 8:
        return (
          <>
            <h1>需要做图片功能吗？</h1>
            <div className="grid gap-2.5 my-5">
              {[
                ['none', '不需要', '生成文字、按钮和普通消息模板。'],
                [
                  'html',
                  '纯 HTML',
                  '保留文本帮助模板，后续可按需加入 HTML 渲染方案。'
                ],
                [
                  'react',
                  'React / JSX',
                  '安装 jsxp，并生成可拆分组件的图片模板。'
                ]
              ].map(([value, label, note]) => (
                <button
                  className={
                    config.image === value ? 'choice selected' : 'choice'
                  }
                  key={value}
                  onClick={() => choose('image', value)}
                >
                  <strong>{label}</strong>
                  <small>{note}</small>
                </button>
              ))}
            </div>
          </>
        )
      case 9:
        return config.image === 'react' ? (
          choices(
            '图片用什么方式做样式？',
            [
              ['css', '原生 CSS（推荐）', '最容易理解，不需要再学额外工具。'],
              ['tailwind', 'Tailwind CSS', '安装 Tailwind 与 CSS 压缩工具。'],
              ['sass', 'Sass / SCSS', '安装 Sass。'],
              ['less', 'Less', '安装 Less。']
            ],
            'style'
          )
        ) : (
          <>
            <h1>不需要样式工具</h1>
            <p>未选择 React / JSX 图片模板，不会安装额外样式依赖。</p>
          </>
        )
      case 10:
        return (
          <>
            <h1>下载开发技能吗？</h1>
            <p>
              开发技能像一本 AlemonJS 的使用说明。安装后，Codex
              等工具更容易按推荐方式帮你写代码。
            </p>
            <div className="grid gap-2.5 my-5">
              <button
                className={
                  config.skills === 'yes' ? 'choice selected' : 'choice'
                }
                onClick={() => choose('skills', 'yes')}
              >
                <strong>下载（推荐）</strong>
                <small>下载 alemonjs-dev-skill，后续可随时更新。</small>
              </button>
              <button
                className={
                  config.skills === 'no' ? 'choice selected' : 'choice'
                }
                onClick={() => choose('skills', 'no')}
              >
                <strong>暂时不下载</strong>
                <small>不会影响机器人运行，以后也可以安装。</small>
              </button>
            </div>
            <a
              className="download-link"
              href="https://github.com/lemonade-lab/alemonjs-dev-skill"
              target="_blank"
              rel="noreferrer"
            >
              查看开发技能说明
            </a>
          </>
        )
      case 11:
        return (
          <>
            <h1>
              {creation?.status === 'ready' ? '项目已创建' : '确认创建项目'}
            </h1>
            {creation?.status === 'ready' ? (
              <>
                <p>{creation.path}</p>
                {creation.path && (
                  <a
                    className="primary-button"
                    href={`/dashboard/robot?root=${encodeURIComponent(creation.path)}`}
                  >
                    前往管理机器人
                  </a>
                )}
              </>
            ) : (
              <div className="config-summary">
                <span>
                  位置：
                  {project.destinationMode === 'current'
                    ? workspace
                      ? `${workspace.bots}/${project.name}`
                      : `工作区/${project.name}`
                    : project.destination
                      ? `${project.destination}/${project.name}`
                      : '请返回填写保存位置'}
                </span>
                <span>
                  语言：{config.language === 'ts' ? 'TypeScript' : 'JavaScript'}
                </span>
                <span>包管理器：{config.manager}</span>
                <span>
                  开发能力包：
                  {capabilities.length
                    ? capabilities
                        .map(item => capabilityLabels[item] ?? item)
                        .join('、')
                    : '仅基础框架'}
                </span>
                <span>
                  图片模板：
                  {config.image === 'react'
                    ? `React / JSX · ${config.style}`
                    : config.image === 'html'
                      ? '纯 HTML'
                      : '不生成'}
                </span>
                <span>
                  代码小助手：{config.eslint === 'yes' ? '启用' : '不启用'}
                </span>
                <span>
                  项目存档：{config.git === 'yes' ? '初始化 Git' : '跳过'}
                </span>
                <span>
                  后台运行：{config.pm2 === 'yes' ? '使用 PM2' : '不使用'}
                </span>
                <span>
                  开发技能：{config.skills === 'yes' ? '下载' : '不下载'}
                </span>
              </div>
            )}
          </>
        )
      default:
        return null
    }
  }
  const installerPage = () => {
    if (flowStep === 0)
      return (
        <EnvironmentCheckPanel
          title="你的环境都准备好了吗"
          report={report}
          checking={checking}
          onCheck={() => onCheck()}
          onFix={onFix}
        />
      )
    if (flowStep === 1)
      return (
        <>
          <h1>机器人放在哪里？</h1>
          <p>
            只需给机器人起名并选一个保存位置；其余均使用适合新手的默认设置。
          </p>
          <div className="project-fields">
            <label>
              机器人名称
              <input
                value={project.name}
                onChange={event =>
                  dispatch(setProject({ name: event.target.value }))
                }
                placeholder="my-alemonjs-bot"
              />
            </label>
            <div className="location-options">
              <button
                className={
                  project.destinationMode === 'current'
                    ? 'choice selected'
                    : 'choice'
                }
                onClick={() =>
                  dispatch(setProject({ destinationMode: 'current' }))
                }
              >
                <strong>工作区（推荐）</strong>
                <small>
                  {workspace
                    ? `保存到 ${workspace.bots}`
                    : '保存到工作区的 bots 目录。'}
                </small>
              </button>
              <button
                className={
                  project.destinationMode === 'custom'
                    ? 'choice selected'
                    : 'choice'
                }
                onClick={chooseDestination}
              >
                <strong>选择指定文件夹</strong>
                <small>
                  {project.destinationMode === 'custom' && project.destination
                    ? `已选择：${project.destination}`
                    : '在目录选择器中选择保存位置。'}
                </small>
              </button>
            </div>
            {folderError && (
              <ErrorNotice
                message={folderError}
                onClose={() => setFolderError('')}
              />
            )}
          </div>
        </>
      )
    return (
      <>
        <h1>
          {creation?.status === 'ready' ? '机器人已安装' : '确认安装机器人'}
        </h1>
        {creation?.status === 'ready' ? (
          <>
            <p>机器人已安装至：{creation.path}</p>
            {creation.path && (
              <a
                className="primary-button"
                href={`/dashboard/robot?root=${encodeURIComponent(creation.path)}`}
              >
                前往管理机器人
              </a>
            )}
          </>
        ) : (
          <>
            <p>
              将使用默认 JavaScript 模板、Yarn、Git
              存档，不附加开发技能、图片工具或 PM2。
            </p>
            <div className="config-summary">
              <span>
                位置：
                {project.destinationMode === 'current'
                  ? workspace
                    ? `${workspace.bots}/${project.name}`
                    : `工作区/${project.name}`
                  : project.destination
                    ? `${project.destination}/${project.name}`
                    : '请返回填写保存位置'}
              </span>
              <span>默认环境：JavaScript + Yarn</span>
              <span>项目存档：初始化 Git</span>
            </div>
          </>
        )}
        {creation?.logs && (
          <div className="creation-logs">
            {creation.logs.map((log, index) => (
              <p key={index}>{log}</p>
            ))}
          </div>
        )}
      </>
    )
  }
  const webPage = () => {
    if (flowStep === 0)
      return (
        <>
          <h1>选择部署方式</h1>
          <div className="grid gap-2.5 my-5">
            <button
              className={webEdition === 'clean' ? 'choice selected' : 'choice'}
              onClick={() => {
                setWebEdition('clean')
                automaticCheck.current = null
              }}
            >
              <strong>纯净版</strong>
              <small>检查 Node.js 与 Git 后启动 alx</small>
            </button>
            <button
              className={webEdition === 'docker' ? 'choice selected' : 'choice'}
              onClick={() => {
                setWebEdition('docker')
                automaticCheck.current = null
              }}
            >
              <strong>Docker 版</strong>
              <small>检查 Docker 后使用 Docker Compose 快速启动</small>
            </button>
          </div>
        </>
      )
    if (flowStep === 1)
      return (
        <EnvironmentCheckPanel
          title="你的环境都准备好了吗"
          report={report}
          checking={checking}
          onCheck={() => onCheck(webEdition ?? undefined)}
          onFix={onFix}
        />
      )
    if (webEdition === 'clean' && flowStep === 2)
      return (
        <>
          <h1>选择下载镜像</h1>
          <p>选择下载来源后继续；随后选择版本与安装包。</p>
          <div className="grid gap-2.5 my-5">
            {goal?.mirrors?.map(mirror => (
              <button
                className={
                  selectedMirror?.url === mirror.url
                    ? 'choice selected'
                    : 'choice'
                }
                key={mirror.url}
                onClick={() => {
                  setSelectedMirror(mirror)
                  setSelectedAssetURL(null)
                }}
              >
                <strong>{mirror.name}</strong>
                <small>
                  {mirror.name === 'GitHub 官方'
                    ? '从 GitHub 官方下载'
                    : mirror.name.includes('推荐')
                      ? '推荐的快捷下载入口'
                      : '下载速度可能更快'}
                </small>
              </button>
            ))}
          </div>
        </>
      )
    if (webEdition === 'clean' && flowStep === 3)
      return (
        <>
          <h1>选择版本</h1>
          <p>默认已选择最新正式版本。确认后继续选择适合电脑的安装包。</p>
          {releasePicker()}
        </>
      )
    if (webEdition === 'clean' && flowStep === 4) return assetPicker()
    return (
      <>
        <h1>Docker Compose 快速启动</h1>
        <p>下一步会生成 docker-compose.yml 并启动服务。</p>
      </>
    )
  }
  const buildPage = () => {
    if (flowStep === 0)
      return (
        <>
          <h1>选择构建方式</h1>
          <div className="grid gap-2.5 my-5">
            <button
              className={buildMode === 'npm' ? 'choice selected' : 'choice'}
              onClick={() => {
                setBuildMode('npm')
                automaticCheck.current = null
              }}
            >
              <strong>NPM 构建</strong>
              <small>使用 npm 生成应用构建产物</small>
            </button>
            <button
              className={buildMode === 'git' ? 'choice selected' : 'choice'}
              onClick={() => {
                setBuildMode('git')
                automaticCheck.current = null
              }}
            >
              <strong>Git 化构建</strong>
              <small>遵循 main、release 分支与版本标签标准</small>
            </button>
          </div>
        </>
      )
    if (flowStep === 1)
      return (
        <EnvironmentCheckPanel
          title="你的环境都准备好了吗"
          report={report}
          checking={checking}
          onCheck={() => onCheck(buildMode ?? undefined)}
          onFix={onFix}
        />
      )
    return (
      <>
        <h1>{buildMode === 'git' ? 'Git 化构建' : 'NPM 构建'}</h1>
        <p>
          {buildMode === 'git'
            ? '构建产物将按标准整理至 release 分支，并创建对应版本标签。'
            : '将构建应用并生成可分发产物。'}
        </p>
      </>
    )
  }
  const downloadPage = () => {
    if (goal?.id === 'mobile')
      return (
        <>
          <h1>下载 Android 安装包</h1>
          <p>手机版目前仅提供 Android 通用 APK，不经过 GitHub。</p>
          {goal.downloadUrl && (
            <a
              className="primary-button"
              href={goal.downloadUrl}
              target="_blank"
              rel="noreferrer"
              onClick={() =>
                setBrowserDownloadNotice(
                  '下载已交给浏览器，请在浏览器下载栏查看实际进度。'
                )
              }
            >
              下载 Android APK
            </a>
          )}
          {browserDownloadNotice && (
            <DownloadProgress
              handoff
              className="mt-4"
              label="已开始浏览器下载"
              detail={browserDownloadNotice}
            />
          )}
        </>
      )
    if (flowStep === 0)
      return (
        <>
          <h1>选择下载镜像</h1>
          <p>选择一个下载来源后继续；下一页再选要下载的版本。</p>
          <div className="grid gap-2.5 my-5">
            {goal?.mirrors?.map(mirror => (
              <button
                className={
                  selectedMirror?.url === mirror.url
                    ? 'choice selected'
                    : 'choice'
                }
                key={mirror.url}
                onClick={() => {
                  setSelectedMirror(mirror)
                  setSelectedAssetURL(null)
                }}
              >
                <strong>{mirror.name}</strong>
                <small>
                  {mirror.name === 'GitHub 官方'
                    ? '从 GitHub 官方下载'
                    : mirror.name.includes('推荐')
                      ? '推荐的快捷下载入口'
                      : '下载速度可能更快'}
                </small>
              </button>
            ))}
          </div>
        </>
      )
    if (flowStep === 1)
      return (
        <>
          <h1>选择下载版本</h1>
          <p>默认已选择最新正式版本。确认后继续选择安装包。</p>
          {releasePicker()}
          <p className="selected-download">
            下载镜像：{selectedMirror?.name ?? '尚未选择'}
          </p>
        </>
      )
    return assetPicker()
  }
  const selectPurpose = (id: string) => {
    automaticCheck.current = null
    setSelectedMirror(null)
    setSelectedAssetURL(null)
    onSelect(id)
  }
  const goBack = () => {
    if (step === 1) {
      onSelect(null)
      setStep(0)
      return
    }
    back()
  }
  const resetPurpose = () => {
    onSelect(null)
    setStep(0)
  }
  registerBack(step > 0 ? goBack : () => {})
  const createConfig = (): ProjectConfig =>
    isInstaller
      ? {
          template: 'bot',
          name: project.name,
          destinationMode: project.destinationMode,
          destination: project.destination,
          language: 'js',
          packageManager: 'yarn',
          eslint: false,
          initializeGit: true,
          usePM2: false,
          imageMode: 'none',
          styleMode: 'css',
          downloadSkills: false,
          developmentPackages: []
        }
      : {
          template: 'dev',
          name: project.name,
          destinationMode: project.destinationMode,
          destination: project.destination,
          language: config.language,
          packageManager: config.manager,
          eslint: config.eslint === 'yes',
          initializeGit: config.git === 'yes',
          usePM2: config.pm2 === 'yes',
          imageMode: config.image,
          styleMode: config.image === 'react' ? config.style : 'css',
          downloadSkills: config.skills === 'yes',
          developmentPackages: capabilities
        }
  const isDownloadFlow = goal?.id === 'mobile'
  const isWeb = goal?.id === 'web'
  const isBuild = goal?.id === 'build'
  const purposeOptions = [
    ['deploy', '部署', '部署源码版、手机版或 Web 版。'],
    ['develop', '开发', '按需选择并创建 AlemonJS 开发模板。'],
    ['manage', '管理', '如果已有可用项目, 点击进入管理面板。']
  ]
  return (
    <section className="wizard">
      <aside className="wizard-steps">
        <p>{goal?.title ?? '开始'}</p>
        {totalSteps.map((label, index) => (
          <button
            type="button"
            ref={index === step ? currentStepElement : null}
            key={label}
            className={index < step ? 'done' : index === step ? 'current' : ''}
            disabled={index > step}
            aria-current={index === step ? 'step' : undefined}
            onClick={
              index <= step
                ? () => (index === 0 ? resetPurpose() : setFlowStep(index))
                : undefined
            }
          >
            <span>{index < step ? '✓' : index + 1}</span>
            {label}
          </button>
        ))}
      </aside>
      <section className="wizard-page">
        {goal && (
          <p className="wizard-progress" aria-live="polite">
            第 {step + 1} / {totalSteps.length} 步 · {totalSteps[step]}
          </p>
        )}
        <div className="wizard-content">
          {!goal || step === 0 ? (
            <div className="guide-question guide-choice-screen mx-auto max-w-140 text-center">
              <p className="guide-choice-eyebrow">ALemonX</p>
              <h1>你现在想做什么？</h1>
              <p className="guide-choice-description">
                选择一个目标；引导会只展示必要步骤，并在需要时检查你的环境。
              </p>
              <div className="question-options" role="list">
                {loading ? (
                  <p>正在准备选项…</p>
                ) : (
                  purposeOptions.map(([id, title, note]) => (
                    <button
                      type="button"
                      className="guide-choice-card"
                      key={id}
                      onClick={() =>
                        id === 'deploy'
                          ? navigate('/guide/group/deploy')
                          : selectPurpose(id)
                      }
                    >
                      <i>{icons[id] ?? '·'}</i>
                      <span>
                        <strong>{title}</strong>
                        <small>{note}</small>
                      </span>
                      <b aria-hidden="true">→</b>
                    </button>
                  ))
                )}
              </div>
            </div>
          ) : isInstaller ? (
            installerPage()
          ) : isDeveloper ? (
            developerPage()
          ) : isWeb ? (
            webPage()
          ) : isBuild ? (
            buildPage()
          ) : isDownloadFlow ? (
            downloadPage()
          ) : (
            <>
              <h1>{totalSteps[step]}</h1>
              <p>{goal.description}</p>
            </>
          )}{' '}
        </div>
        <footer className="wizard-actions">
          {isInstaller && flowStep === 0 && report?.ready && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isInstaller && flowStep === 1 && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isInstaller && flowStep === 2 && creation?.status !== 'ready' && (
            <button
              className="next-button"
              onClick={() => onCreate(createConfig())}
              disabled={creating}
            >
              {creating ? '正在安装…' : '确认安装'}
            </button>
          )}
          {isDeveloper && flowStep === 0 && report?.ready && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isDeveloper && flowStep > 0 && flowStep < 11 && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isDeveloper && flowStep === 11 && creation?.status !== 'ready' && (
            <button
              className="next-button"
              onClick={() => onCreate(createConfig())}
              disabled={creating}
            >
              {creating ? '正在创建…' : '确认创建'}
            </button>
          )}
          {isWeb && flowStep === 0 && webEdition && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isWeb && flowStep === 1 && report?.ready && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isWeb &&
            flowStep === 2 &&
            webEdition === 'clean' &&
            selectedMirror && (
              <button className="next-button" onClick={next}>
                继续
              </button>
            )}
          {isWeb && flowStep === 3 && webEdition === 'clean' && releaseURL && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isWeb &&
            flowStep === 4 &&
            webEdition === 'clean' &&
            selectedAssetURL && (
              <a
                className="next-button"
                href={mirrorURL(selectedMirror, selectedAssetURL)}
                target="_blank"
                rel="noreferrer"
                onClick={() =>
                  setBrowserDownloadNotice(
                    '下载已交给浏览器，请在浏览器下载栏查看实际进度。'
                  )
                }
              >
                开始下载
              </a>
            )}
          {isWeb && flowStep === 2 && webEdition === 'docker' && (
            <button className="next-button" disabled>
              生成并启动
            </button>
          )}
          {isBuild && flowStep === 0 && buildMode && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isBuild && flowStep === 1 && report?.ready && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isBuild && flowStep === 2 && (
            <button className="next-button" disabled>
              开始构建
            </button>
          )}
          {!isInstaller &&
            !isDeveloper &&
            !isWeb &&
            !isBuild &&
            !isDownloadFlow &&
            goal && (
              <button className="next-button" onClick={next}>
                继续
              </button>
            )}
          {isDownloadFlow && flowStep === 0 && selectedMirror && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isDownloadFlow && flowStep === 1 && releaseURL && (
            <button className="next-button" onClick={next}>
              继续
            </button>
          )}
          {isDownloadFlow && flowStep === 2 && selectedAssetURL && (
            <a
              className="next-button"
              href={mirrorURL(selectedMirror, selectedAssetURL)}
              target="_blank"
              rel="noreferrer"
              onClick={() =>
                setBrowserDownloadNotice(
                  '下载已交给浏览器，请在浏览器下载栏查看实际进度。'
                )
              }
            >
              开始下载
            </a>
          )}
        </footer>
      </section>
    </section>
  )
}
