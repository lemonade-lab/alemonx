import { useCallback, useEffect, useMemo, useState } from 'react'
import { Provider } from 'react-redux'
import { App as AntApp, ConfigProvider, theme as antdTheme } from 'antd'
import '@testone/testone.scss'
import { store as testoneStore } from '@testone/store'
import { setTestoneWsUrl } from '@testone/config'
import { wsConnectRequest, wsCancel } from '@testone/store/slices/socketSlice'
import {
  setTab,
  setPrivateMessages,
  setGroupMessages
} from '@testone/store/slices/chatSlice'
import TestoneApp from '@testone/pages/App'
import { MessageBridge } from '@testone/component/MessageBridge'
import { useLazyTestPortQuery } from '../store/workspaceApi'
import {
  clearServerRecords,
  fetchSummary,
  setTestoneRoot,
  TESTONE_RETENTION_DAYS,
  type TestoneRecordSummary
} from '@testone/core/testoneServer'
import {
  clearAllLocalChatLists,
  flushChatListServerSync
} from '@testone/core/chatlist'

// robotAppToken base64url-encodes a robot directory path without padding so it
// can sit in a URL path segment, matching the backend's robotAppToken and the
// existing application proxy mount.
function robotAppToken(root: string) {
  return btoa(
    Array.from(new TextEncoder().encode(root), byte =>
      String.fromCharCode(byte)
    ).join('')
  )
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
}

/**
 * TestCenter hosts the migrated AlemonJS sandbox (testone) as a core workbench
 * page. The sandbox never opens the robot's port from the browser: the backend
 * proxies the /testone WebSocket on the workbench origin, and TestCenter points
 * the sandbox store at that proxy URL for the current robot.
 */
export function TestCenter({ root }: { root: string }) {
  const [loadTestPort] = useLazyTestPortQuery()
  const [connectionError, setConnectionError] = useState('')
  const [recordsOpen, setRecordsOpen] = useState(false)
  const [recordSummary, setRecordSummary] = useState<
    TestoneRecordSummary | undefined
  >()
  const [recordsBusy, setRecordsBusy] = useState(false)
  const [projectTheme, setProjectTheme] = useState<'light' | 'dark'>(() =>
    document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
  )
  const wsUrl = useMemo(() => {
    if (!root) return null
    return `/api/v1/robot/test/${robotAppToken(root)}/testone`
  }, [root])

  const refreshRecordSummary = useCallback(async () => {
    if (!root) return
    try {
      const items = await fetchSummary()
      setRecordSummary(
        items.find(item => item.root === root) ?? {
          root,
          chats: 0,
          images: 0,
          bytes: 0
        }
      )
    } catch {
      setRecordSummary(undefined)
    }
  }, [root])

  const clearRecords = useCallback(async () => {
    if (
      !window.confirm(
        '确认清空该机器人在服务端保存的测试中心聊天记录与图片吗？本机缓存也会一并清除。不会影响沙盒进程与 QQ 平台消息。'
      )
    )
      return
    setRecordsBusy(true)
    try {
      await clearServerRecords(root)
      clearAllLocalChatLists()
      testoneStore.dispatch(setPrivateMessages([]))
      testoneStore.dispatch(setGroupMessages([]))
      await refreshRecordSummary()
    } catch (reason) {
      setConnectionError(
        reason instanceof Error ? reason.message : '测试中心记录清理未完成。'
      )
    } finally {
      setRecordsBusy(false)
    }
  }, [refreshRecordSummary, root])

  // 跟随当前项目主题：文档根 [data-theme] 由工作台主题切换维护，这里用
  // MutationObserver 即时同步，避免在 testone 内再维护一份主题状态。
  useEffect(() => {
    const rootEl = document.documentElement
    const sync = () =>
      setProjectTheme(rootEl.dataset.theme === 'dark' ? 'dark' : 'light')
    sync()
    const observer = new MutationObserver(sync)
    observer.observe(rootEl, {
      attributes: true,
      attributeFilter: ['data-theme']
    })
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!wsUrl) return
    setConnectionError('')
    void flushChatListServerSync().then(() => setTestoneRoot(root))
    setTestoneWsUrl(wsUrl)
    // 打开即进入群聊：testone 的 tab 路由在单例 store 中跨会话保留，这里
    // 每次打开都重置到初始页，避免重新打开时停留在上一次的页面。
    testoneStore.dispatch(setTab('group'))
    // testone store 是单例：切换到另一个机器人时清掉上一个机器人的内存
    // 消息，避免打开瞬间看到旧的聊天缓存（连接后会按 host:port 重新加载）。
    testoneStore.dispatch(setPrivateMessages([]))
    testoneStore.dispatch(setGroupMessages([]))
    void loadTestPort(root, false).then(result => {
      const info = result.data
      if (!info?.configured || !Number.isInteger(info.port)) {
        testoneStore.dispatch(wsCancel())
        setConnectionError('服务端口未配置或配置已失效，请关闭后重新打开测试并确认端口。')
        return
      }
      testoneStore.dispatch(
        wsConnectRequest({
          name: '本机测试服务',
          // 打开即固定连接本机地址；实际 socket 经后端同源代理转发。
          host: '127.0.0.1',
          port: info.port
        })
      )
    })
    return () => {
      void flushChatListServerSync().then(() => setTestoneRoot(null))
      setTestoneWsUrl(null)
      testoneStore.dispatch(wsCancel())
    }
  }, [loadTestPort, root, wsUrl])

  return (
    <div className="testone-root testone-window relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
      <div className="absolute right-3 top-3 z-50 flex gap-1.5">
        <button
          type="button"
          className="rounded-md border border-slate-200 bg-white/90 px-2.5 py-1 text-xs text-slate-600 shadow-sm backdrop-blur hover:border-brand-300 hover:text-brand-600"
          onClick={() => {
            setRecordsOpen(current => !current)
            if (!recordsOpen) void refreshRecordSummary()
          }}
        >
          记录
        </button>
        {recordsOpen && (
          <div className="grid w-56 gap-2 rounded-lg border border-slate-200 bg-white p-3 text-xs text-slate-700 shadow-md">
            <strong className="text-slate-800">服务端记录</strong>
            <span className="text-slate-400">
              临时测试数据，自动保留 {TESTONE_RETENTION_DAYS} 天
            </span>
            <span className="text-slate-500">
              已保存 {recordSummary?.chats ?? 0} 个会话 ·{' '}
              {recordSummary?.images ?? 0} 张图片
              {recordSummary && recordSummary.bytes > 0
                ? ` · 约 ${(recordSummary.bytes / 1024).toFixed(1)} KB`
                : ''}
            </span>
            <button
              type="button"
              className="rounded-md border border-red-200 bg-red-50 px-2 py-1 text-red-600 hover:border-red-300"
              disabled={recordsBusy}
              onClick={() => void clearRecords()}
            >
              {recordsBusy ? '正在清理…' : '清空服务端记录'}
            </button>
            <button
              type="button"
              className="text-slate-400 hover:text-slate-600"
              onClick={() => setRecordsOpen(false)}
            >
              关闭
            </button>
          </div>
        )}
      </div>
      {connectionError ? (
        <div className="m-4 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          {connectionError}
        </div>
      ) : null}
      <Provider store={testoneStore}>
        <ConfigProvider
          theme={{
            algorithm:
              projectTheme === 'dark'
                ? antdTheme.darkAlgorithm
                : antdTheme.defaultAlgorithm
          }}
        >
          <AntApp className="flex min-h-0 min-w-0 flex-1 flex-col">
            <MessageBridge />
            <TestoneApp theme={projectTheme} />
          </AntApp>
        </ConfigProvider>
      </Provider>
    </div>
  )
}
