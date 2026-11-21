import { useEffect, useRef, useState } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { ArrowLeft, ArrowRight, Globe2, Plus, RefreshCw, Search, X } from 'lucide-react'
import { DesktopWindow } from './DesktopWindow'
import meLogo from '../assets/images/me.png'
import { recordBrowserSite } from '../store/browserRecentStore'
import type { RootState } from '../store/guideStore'
import { isWorkbenchBrowserPageURL } from '../lib/workbenchBrowserAddress'

type BrowserTab = {
  id: string
  query: string
  title?: string
  pageURL: string
  frameKey: number
  history: string[]
  historyIndex: number
}

type BrowserAddress = {
  url: URL
  displayURL: string
}

// Keep navigation sources explicit. A workbench document is never interpreted
// as text from the address bar, and an address-bar URL can never acquire a
// robot application route by accident.
export type BrowserCommand = {
  id: string
  kind: 'open' | 'close'
  source?: 'address' | 'workbench'
  url?: string
  tabID?: string
  newTab?: boolean
}

function createBrowserHomeTab(): BrowserTab {
  return {
    id: `browser-home-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    query: '',
    pageURL: '',
    frameKey: 0,
    history: [],
    historyIndex: -1
  }
}

function isBrowserIPAddress(host: string) {
  const value = host.replace(/^\[|\]$/g, '')
  return (
    /^\d{1,3}(?:\.\d{1,3}){3}$/.test(value) ||
    value.includes(':') ||
    value === 'localhost'
  )
}

// URL.href removes default ports. The browser keeps a separate display URL so
// address completion remains visible and deterministic to users.
function completeBrowserAddress(input: string): BrowserAddress | null {
  const raw = input.trim()
  if (!raw) return null
  const portOnly = /^(\d{1,5})([/?#].*)?$/.exec(raw)
  let scheme = ''
  let authority = ''
  let suffix = ''
  if (portOnly) {
    scheme = 'http:'
    authority = '127.0.0.1:' + portOnly[1]
    suffix = portOnly[2] || ''
  } else {
    const prefixed = /^(https?):\/\/?(.*)$/i.exec(raw)
    if (prefixed) {
      scheme = prefixed[1].toLowerCase() + ':'
      authority = prefixed[2]
      if (/^\d{1,5}$/.test(authority)) authority = '127.0.0.1:' + authority
    } else {
      authority = raw
    }
    const boundary = authority.search(/[/?#]/)
    if (boundary >= 0) {
      suffix = authority.slice(boundary)
      authority = authority.slice(0, boundary)
    }
    if (!authority) return null
    let provisional: URL
    try {
      provisional = new URL('http://' + authority)
    } catch {
      return null
    }
    const explicitPort =
      /(?:^\[[^\]]+\]|^[^:]+):(\d{1,5})$/.exec(authority)?.[1] || ''
    const hasPort = explicitPort !== ''
    const hostIsIP = isBrowserIPAddress(provisional.hostname)
    if (!scheme) {
      if (hasPort && explicitPort === '80') scheme = 'http:'
      else if (hasPort && explicitPort === '443') scheme = 'https:'
      else scheme = hostIsIP ? 'http:' : 'https:'
    }
    if (!hasPort) authority += ':' + (scheme === 'https:' ? '443' : '80')
  }
  try {
    const url = new URL(scheme + '//' + authority + suffix)
    // A Hash route is handled entirely by the document; it is never included
    // in the HTTP request. Directory-hosted SPAs commonly require their entry
    // document to end with `/`, e.g. `/console/#/config` rather than
    // `/console#/config`. Canonicalize only extensionless Hash entry paths so
    // a direct deep link does not become an upstream 404 before the SPA runs.
    const finalSegment = url.pathname.split('/').at(-1) ?? ''
    if (
      url.hash &&
      finalSegment !== '' &&
      !finalSegment.includes('.') &&
      !url.pathname.endsWith('/')
    ) {
      url.pathname += '/'
    }
    const port = url.port || (scheme === 'https:' ? '443' : '80')
    const host =
      url.hostname.includes(':') && !url.hostname.startsWith('[')
        ? `[${url.hostname}]`
        : url.hostname
    return {
      url,
      displayURL: `${scheme}//${host}:${port}${url.pathname}${url.search}${url.hash}`
    }
  } catch {
    return null
  }
}

export function MiniBrowserWindow({
  minimized,
  browserCommand,
  zIndex,
  onClose,
  onMinimize,
  onActivate,
  onCommandConsumed,
  native = false
}: {
  native?: boolean
  minimized: boolean
  browserCommand: BrowserCommand | null
  zIndex: number
  onClose: () => void
  onMinimize: () => void
  onActivate: () => void
  onCommandConsumed: (id: string) => void
}) {
  const dispatch = useDispatch()
  const recentSites = useSelector((state: RootState) => state.browserRecent.sites)
  const [homeQuery, setHomeQuery] = useState('')
  // Directly opening the browser gets one blank tab. A concrete navigation is
  // itself the first tab, so service and plugin launches never leave a spare
  // home tab behind.
  const startsWithNavigation = Boolean(
    browserCommand?.kind === 'open' && browserCommand.url
  )
  const [tabs, setTabs] = useState<BrowserTab[]>(() =>
    startsWithNavigation ? [] : [createBrowserHomeTab()]
  )
  const [activeTabID, setActiveTabID] = useState(() => tabs[0]?.id ?? '')
  const [error, setError] = useState('')
  const lastBrowserCommandID = useRef('')
  const frameRefs = useRef(new Map<string, HTMLIFrameElement>())
  const activeTab = tabs.find(tab => tab.id === activeTabID)
  const query = activeTab?.query ?? homeQuery
  const pageURL = activeTab?.pageURL ?? ''
  const history = activeTab?.history ?? []
  const historyIndex = activeTab?.historyIndex ?? -1
  const updateActiveTab = (update: (tab: BrowserTab) => BrowserTab) => {
    setTabs(current =>
      current.map(tab => (tab.id === activeTabID ? update(tab) : tab))
    )
  }
  const setQuery = (value: string) => {
    if (activeTab) updateActiveTab(tab => ({ ...tab, query: value }))
    else setHomeQuery(value)
  }
  const addressTextFor = (target: URL) =>
    target.origin === window.location.origin
      ? `${target.pathname}${target.search}${target.hash}` || '/'
      : target.href
  const frameURLFor = (target: URL) => {
    if (target.origin === window.location.origin)
      return `${target.pathname}${target.search}${target.hash}`
    if (target.protocol !== 'http:') return target.href
    const token = window
      .btoa(target.origin)
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/g, '')
    return `/api/v1/browser/http/${token}${target.pathname}${target.search}${target.hash}`
  }
  const openInSystemBrowser = async (target: BrowserAddress) => {
    const response = await fetch('/api/v1/browser/external/open', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: target.displayURL })
    })
    if (!response.ok) throw new Error('无法交给系统浏览器打开。')
  }
  const removeBrowserTab = (id: string) => {
    setTabs(current => {
      const remaining = current.filter(tab => tab.id !== id)
      if (remaining.length === 0) {
        // The only tab was a provisional HTTP tab that has been classified for
        // the system browser. Do not leave an empty embedded browser behind.
        window.setTimeout(onClose, 0)
        if (native) {
          const home = createBrowserHomeTab()
          setActiveTabID(home.id)
          return [home]
        }
        return remaining
      }
      if (activeTabID === id) setActiveTabID(remaining.at(-1)?.id ?? '')
      return remaining
    })
  }
  const openWorkbenchPage = (
    path: string,
    openInNewTab: boolean,
    tabID?: string
  ) => {
    let target: URL
    try {
      target = new URL(path, window.location.origin)
    } catch {
      setError('工作台页面地址无效。')
      return
    }
    if (
      target.origin !== window.location.origin ||
      !isWorkbenchBrowserPageURL(target.href)
    ) {
      setError('不允许将该地址作为工作台内部页面打开。')
      return
    }
    const pageURL = `${target.pathname}${target.search}${target.hash}`
    const nextTab: BrowserTab = {
      id:
        tabID ??
        `browser-workbench-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      query: pageURL,
      pageURL,
      frameKey: 1,
      history: [target.href],
      historyIndex: 0
    }
    setError('')
    if (openInNewTab || !activeTab) {
      setTabs(current => [...current, nextTab])
      setActiveTabID(nextTab.id)
      return
    }
    setTabs(current =>
      current.map(tab =>
        tab.id === activeTab.id ? { ...nextTab, id: tab.id } : tab
      )
    )
  }
  const navigate = async (
    value = query,
    openInNewTab = false,
    tabID?: string
  ) => {
    const input = value.trim()
    if (!input) return
    try {
      const portOnly = /^(\d{1,5})(?:[/?#].*)?$/.exec(input)
      const port = portOnly ? Number(portOnly[1]) : 0
      if (portOnly && (port < 1 || port > 65535)) {
        setError('端口号必须在 1 到 65535 之间。')
        return
      }
      const currentEntry = activeTab?.history[activeTab.historyIndex]
      const isPathReference =
        input.startsWith('/') ||
        input.startsWith('./') ||
        input.startsWith('../') ||
        input.startsWith('?') ||
        input.startsWith('#') ||
        /^[^/?#\s]+\/[^/?#\s].*$/.test(input)
      const looksLikeAddress =
        input.includes('.') ||
        input.startsWith('localhost') ||
        input.includes(':')
      const searchURL = `https://html.duckduckgo.com/html/?q=${encodeURIComponent(input)}`
      if (isPathReference && !currentEntry) {
        setError('请先打开一个网站，再输入相对路径、查询或锚点。')
        return
      }
      const complete = isPathReference
        ? completeBrowserAddress(new URL(input, currentEntry).href)
        : completeBrowserAddress(
            looksLikeAddress || portOnly ? input : searchURL
          )
      if (!complete) throw new Error('地址无效')
      const next = complete.url
      // A newly-created home tab is active but deliberately has no history
      // entry yet. Its first navigation must not try to parse `undefined`.
      const current = currentEntry ? new URL(currentEntry) : null
      const sameSite = Boolean(
        current &&
        current.protocol === next.protocol &&
        current.hostname === next.hostname &&
        (current.port || (current.protocol === 'https:' ? '443' : '80')) ===
          (next.port || (next.protocol === 'https:' ? '443' : '80'))
      )
      const shouldOpenNewTab = openInNewTab || !activeTab
      // HTTPS is completely decided in the renderer: cross-site HTTPS always
      // belongs to the system browser, so it must not wait for a backend host
      // classification request or briefly create an embedded tab.
      if (!sameSite && next.protocol === 'https:') {
        await openInSystemBrowser(complete)
        if (tabs.length === 0) window.setTimeout(onClose, 0)
        return
      }
      // HTTP targets get a visible tab before asynchronous classification. In
      // particular, this keeps a plugin Web UI open request reliable while its
      // host panel and the browser window are mounting concurrently.
      const pendingTabID = shouldOpenNewTab
        ? (tabID ??
          `browser-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`)
        : ''
      if (shouldOpenNewTab && next.protocol === 'http:') {
        setTabs(current => [
          ...current,
          {
            id: pendingTabID,
            query: complete.displayURL,
            pageURL: '',
            frameKey: 0,
            history: [],
            historyIndex: -1
          }
        ])
        setActiveTabID(pendingTabID)
      }
      if (!sameSite) {
        let scope = 'public'
        try {
          const classified = await fetch('/api/v1/browser/classify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: complete.displayURL })
          })
          const result = (await classified.json().catch(() => ({}))) as {
            scope?: string
          }
          if (!classified.ok) throw new Error('地址分类失败')
          scope = result.scope ?? scope
        } catch (reason) {
          // HTTP is served through our compatibility proxy. When host
          // classification itself is unavailable, still create the requested
          // tab and let the proxy show the real connection result instead of
          // silently discarding the user's new-tab action.
          if (next.protocol !== 'http:') throw reason
          scope = 'private'
        }
        if (scope !== 'private') {
          await openInSystemBrowser(complete)
          if (pendingTabID) removeBrowserTab(pendingTabID)
          return
        }
      }
      setError('')
      const recentSite = recordBrowserSite(next)
      if (recentSite) dispatch(recentSite)
      const nextHistory = shouldOpenNewTab
        ? [next.href]
        : activeTab.history.slice(0, activeTab.historyIndex + 1)
      if (nextHistory.at(-1) !== next.href) nextHistory.push(next.href)
      const nextTab: BrowserTab = {
        id:
          pendingTabID ||
          tabID ||
          `browser-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        query: sameSite ? addressTextFor(next) : complete.displayURL,
        pageURL: frameURLFor(next),
        frameKey: 1,
        history: nextHistory,
        historyIndex: nextHistory.length - 1
      }
      if (shouldOpenNewTab) {
        setTabs(current =>
          pendingTabID
            ? current.map(tab => (tab.id === pendingTabID ? nextTab : tab))
            : [...current, nextTab]
        )
        setActiveTabID(nextTab.id)
      } else {
        setTabs(current =>
          current.map(tab =>
            tab.id === activeTab.id ? { ...nextTab, id: tab.id } : tab
          )
        )
      }
    } catch (reason) {
      setError(
        reason instanceof Error && reason.message
          ? reason.message
          : '请输入有效的网址或搜索内容。'
      )
    }
  }
  const navigateHistory = (delta: -1 | 1) => {
    const nextIndex = historyIndex + delta
    const nextURL = history[nextIndex]
    if (!nextURL) return
    const target = new URL(nextURL)
    updateActiveTab(tab => ({
      ...tab,
      historyIndex: nextIndex,
      query: addressTextFor(target),
      pageURL: frameURLFor(target),
      frameKey: tab.frameKey + 1
    }))
    setError('')
  }
  const syncTabTitle = (tabID: string) => {
    const frame = frameRefs.current.get(tabID)
    if (!frame) return
    try {
      const title = frame.contentDocument?.title.trim() ?? ''
      if (!title) return
      setTabs(current =>
        current.map(tab =>
          tab.id === tabID && tab.title !== title ? { ...tab, title } : tab
        )
      )
    } catch {
      // A direct document may be cross-origin. Its existing URL label remains
      // the safe fallback, while proxied and workbench documents are readable.
    }
  }
  const syncFrameNavigation = (tabID: string) => {
    const frame = frameRefs.current.get(tabID)
    if (!frame) return
    syncTabTitle(tabID)
    try {
      const frameURL = new URL(frame.contentWindow?.location.href ?? '')
      const match = frameURL.pathname.match(
        /^\/api\/v1\/browser\/http\/([^/]+)\/(.*)$/
      )
      if (!match) return
      const targetOrigin = window.atob(
        match[1].replace(/-/g, '+').replace(/_/g, '/')
      )
      const logical = new URL(
        `/${match[2]}${frameURL.search}${frameURL.hash}`,
        targetOrigin
      )
      setTabs(current =>
        current.map(tab => {
          if (
            tab.id !== tabID ||
            tab.history[tab.historyIndex] === logical.href
          )
            return tab
          const nextHistory = tab.history.slice(0, tab.historyIndex + 1)
          nextHistory.push(logical.href)
          return {
            ...tab,
            query: addressTextFor(logical),
            pageURL: `${frameURL.pathname}${frameURL.search}${frameURL.hash}`,
            history: nextHistory,
            historyIndex: nextHistory.length - 1
          }
        })
      )
    } catch {
      // HTTPS pages are direct and intentionally remain inaccessible to the
      // workbench document; their address stays as the user entered it.
    }
  }
  useEffect(() => {
    if (!browserCommand || browserCommand.id === lastBrowserCommandID.current)
      return
    lastBrowserCommandID.current = browserCommand.id
    onCommandConsumed(browserCommand.id)
    if (browserCommand.kind === 'close' && browserCommand.tabID) {
      closeTab(browserCommand.tabID)
      return
    }
    if (browserCommand.kind === 'open' && browserCommand.url) {
      if (browserCommand.source === 'workbench') {
        openWorkbenchPage(
          browserCommand.url,
          browserCommand.newTab === true,
          browserCommand.tabID
        )
        return
      }
      navigate(
        browserCommand.url,
        browserCommand.newTab === true,
        browserCommand.tabID
      )
    }
  }, [browserCommand])
  useEffect(() => {
    const receive = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return
      const data = event.data as {
        source?: string
        type?: string
        url?: string
      } | null
      if (data?.source !== 'alx-browser' || !data.url) return
      const sourceTab = tabs.find(
        tab => frameRefs.current.get(tab.id)?.contentWindow === event.source
      )
      if (!sourceTab) return
      if (data.type === 'form-blocked') {
        setError(
          '已阻止跨站 POST 提交：系统浏览器无法安全复用当前页面的表单数据与会话。'
        )
        return
      }
      if (data.type === 'open') {
        navigate(data.url, true)
        return
      }
      if (data.type !== 'navigate') return
      try {
        const logical = new URL(data.url)
        setTabs(current =>
          current.map(tab => {
            if (
              tab.id !== sourceTab.id ||
              tab.history[tab.historyIndex] === logical.href
            )
              return tab
            const nextHistory = tab.history.slice(0, tab.historyIndex + 1)
            nextHistory.push(logical.href)
            return {
              ...tab,
              query: addressTextFor(logical),
              pageURL: frameURLFor(logical),
              history: nextHistory,
              historyIndex: nextHistory.length - 1
            }
          })
        )
      } catch {
        // Ignore malformed messages from an embedded document.
      }
    }
    window.addEventListener('message', receive)
    return () => window.removeEventListener('message', receive)
  })
  useEffect(() => {
    // SPAs often replace document.title after their initial load. Every browser
    // tab has its own persistent frame, so update titles independently without
    // disturbing DOM or JavaScript state.
    const timer = window.setInterval(() => {
      tabs.forEach(tab => {
        if (tab.pageURL) syncTabTitle(tab.id)
      })
    }, 800)
    return () => window.clearInterval(timer)
  }, [tabs])
  const closeTab = (id: string) => {
    const index = tabs.findIndex(tab => tab.id === id)
    if (index < 0) return
    const remaining = tabs.filter(tab => tab.id !== id)
    window.dispatchEvent(
      new CustomEvent('alx-browser-tab-closed', { detail: { id } })
    )
    if (remaining.length === 0) {
      const home = createBrowserHomeTab()
      setTabs([home])
      setActiveTabID(home.id)
      setHomeQuery('')
      setError('')
      return
    }
    setTabs(remaining)
    if (id === activeTabID)
      setActiveTabID(remaining[Math.max(0, index - 1)]?.id ?? '')
  }
  const openHomeTab = () => {
    const homeTab = createBrowserHomeTab()
    setTabs(current => [...current, homeTab])
    setActiveTabID(homeTab.id)
    setHomeQuery('')
    setError('')
  }
  return (
    <DesktopWindow
      native={native}
      id="mini-browser"
      open
      minimized={minimized}
      title="浏览器"
      subtitle={pageURL || '搜索与访问网页'}
      icon={
        <Globe2 className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      headerLeft={
        tabs.length > 0 ? (
          <div className="flex w-full items-center gap-1 overflow-x-auto py-1">
            {tabs.map(tab => (
              <div
                key={tab.id}
                className={`flex min-w-28 max-w-52 items-center gap-1 rounded-t-md border border-b-0 px-2 py-1 text-xs ${tab.id === activeTabID ? 'bg-white text-slate-800 dark:bg-slate-700 dark:text-slate-100' : 'border-transparent text-slate-500 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-700/70'}`}
              >
                <button
                  className="min-w-0 flex-1 truncate text-left"
                  type="button"
                  onClick={() => setActiveTabID(tab.id)}
                  title={tab.title || tab.query || '新标签页'}
                >
                  {tab.title || tab.query || '新标签页'}
                </button>
                <button
                  className="shrink-0 rounded p-0.5 hover:bg-slate-200 dark:hover:bg-slate-600"
                  type="button"
                  onClick={() => closeTab(tab.id)}
                  aria-label="关闭标签页"
                >
                  <X className="size-3" />
                </button>
              </div>
            ))}
            <button
              className="icon-button size-8 shrink-0 p-0"
              type="button"
              onClick={openHomeTab}
              aria-label="新建标签页"
              title="新建标签页"
            >
              <Plus className="size-4" />
            </button>
          </div>
        ) : undefined
      }
      onClose={() => {
        tabs.forEach(tab =>
          window.dispatchEvent(
            new CustomEvent('alx-browser-tab-closed', {
              detail: { id: tab.id }
            })
          )
        )
        onClose()
      }}
      onMinimize={onMinimize}
      zIndex={zIndex}
      onActivate={onActivate}
      initialPosition={{ left: 144, top: 96 }}
      width={1020}
      height={700}
    >
      <section className="mini-browser grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] bg-slate-50 dark:bg-slate-900">
        <div className="min-w-0" hidden={!pageURL}>
          <form
            className="flex shrink-0 items-center gap-2 border-b border-slate-200 bg-slate-50 px-3 py-2 dark:border-slate-700 dark:bg-slate-800"
            onSubmit={event => {
              event.preventDefault()
              void navigate()
            }}
          >
            <button
              className="icon-button size-8 shrink-0 p-0"
              type="button"
              onClick={() => navigateHistory(-1)}
              disabled={historyIndex <= 0}
              aria-label="后退"
              title="后退"
            >
              <ArrowLeft className="size-4" />
            </button>
            <button
              className="icon-button size-8 shrink-0 p-0"
              type="button"
              onClick={() => navigateHistory(1)}
              disabled={historyIndex >= history.length - 1}
              aria-label="前进"
              title="前进"
            >
              <ArrowRight className="size-4" />
            </button>
            <button
              className="icon-button size-8 shrink-0 p-0"
              type="button"
              onClick={() =>
                updateActiveTab(tab => ({
                  ...tab,
                  frameKey: tab.frameKey + 1
                }))
              }
              aria-label="刷新"
              title="刷新"
            >
              <RefreshCw className="size-4" />
            </button>
            <input
              className="min-w-0 flex-1 rounded-full border border-slate-300 bg-white px-4 py-1.5 text-sm text-slate-800 outline-none placeholder:text-slate-400 focus:border-brand-600 focus:ring-2 focus:ring-brand-100 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder="输入网址或搜索内容"
              aria-label="网址或搜索内容"
              autoComplete="off"
              spellCheck={false}
            />
            <button
              className="icon-button size-8 shrink-0 p-0"
              type="submit"
              aria-label="前往"
              title="前往"
            >
              <Search className="size-4" />
            </button>
          </form>
          {error && (
            <p className="m-0 border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/30 dark:text-amber-300">
              {error}
            </p>
          )}
        </div>
        <div className="relative row-start-2 min-h-0 overflow-hidden">
          {tabs
            .filter(tab => tab.pageURL)
            .map(tab => (
              <iframe
                key={`${tab.id}:${tab.frameKey}`}
                ref={node => {
                  if (node) frameRefs.current.set(tab.id, node)
                  else frameRefs.current.delete(tab.id)
                }}
                className={`absolute inset-0 h-full w-full border-0 bg-white ${tab.id === activeTabID ? '' : 'invisible pointer-events-none'}`}
                name={`alx-browser-tab:${tab.id}`}
                src={tab.pageURL}
                title={tab.query || '迷你浏览器页面'}
                referrerPolicy={
                  tab.pageURL.startsWith('/api/v1/browser/http/')
                    ? 'strict-origin-when-cross-origin'
                    : 'no-referrer'
                }
                onLoad={() => syncFrameNavigation(tab.id)}
              />
            ))}
          {!pageURL && (
            <div className="grid h-full place-items-center overflow-y-auto bg-white p-4 sm:p-8 dark:bg-slate-900">
              <form
                className="grid w-full max-w-2xl gap-7"
                onSubmit={event => {
                  event.preventDefault()
                  navigate()
                }}
              >
                <img
                  className="mx-auto h-auto w-52 max-w-[62%] dark:brightness-125"
                  src={meLogo}
                  alt="Alemon"
                />
                <div className="flex items-center rounded-full border border-slate-300 bg-white px-4 shadow-sm transition-shadow focus-within:border-brand-500 focus-within:ring-4 focus-within:ring-brand-100 dark:border-slate-600 dark:bg-slate-800 dark:focus-within:ring-brand-900/40">
                  <Plus className="mr-3 size-5 shrink-0 text-slate-700 dark:text-slate-300" />
                  <input
                    className="h-12 min-w-0 flex-1 bg-transparent text-base text-slate-800 outline-none placeholder:text-slate-400 dark:text-slate-100"
                    value={query}
                    onChange={event => setQuery(event.target.value)}
                    placeholder="搜索、输入网址或端口号"
                    aria-label="搜索或输入网址"
                    autoComplete="off"
                    autoFocus
                    spellCheck={false}
                  />
                  <button
                    className="inline-flex size-8 shrink-0 items-center justify-center rounded-full text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-slate-400 dark:hover:bg-slate-700 dark:hover:text-slate-100"
                    type="submit"
                    aria-label="搜索或前往网址"
                    title="搜索或前往网址"
                  >
                    <Search className="size-4" />
                  </button>
                </div>
                {error && (
                  <p className="m-0 text-center text-xs text-amber-700 dark:text-amber-300">
                    {error}
                  </p>
                )}
                {recentSites.length > 0 && (
                  <section aria-label="近期打开" className="grid min-w-0 gap-2">
                    <h2 className="m-0 px-2 text-xs font-medium text-slate-500 dark:text-slate-400">
                      近期打开
                    </h2>
                    <ul className="m-0 grid list-none grid-cols-2 gap-2 p-0 sm:grid-cols-3">
                      {recentSites.map(site => {
                        const hostname = new URL(site).host
                        return (
                          <li key={site} className="min-w-0">
                            <button
                              type="button"
                              className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm text-slate-700 hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-brand-500 dark:text-slate-200 dark:hover:bg-slate-800"
                              title={site}
                              onClick={() => void navigate(site)}
                            >
                              <Globe2 aria-hidden="true" className="size-4 shrink-0 text-slate-500 dark:text-slate-400" />
                              <span className="truncate">{hostname}</span>
                            </button>
                          </li>
                        )
                      })}
                    </ul>
                  </section>
                )}
              </form>
            </div>
          )}
        </div>
      </section>
    </DesktopWindow>
  )
}
