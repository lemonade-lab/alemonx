import { useEffect, useMemo, useState } from 'react'
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
  const [projectTheme, setProjectTheme] = useState<'light' | 'dark'>(() =>
    document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
  )
  const wsUrl = useMemo(() => {
    if (!root) return null
    return `/api/v1/robot/test/${robotAppToken(root)}/testone`
  }, [root])

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
    setTestoneWsUrl(wsUrl)
    // 打开即进入群聊：testone 的 tab 路由在单例 store 中跨会话保留，这里
    // 每次打开都重置到初始页，避免重新打开时停留在上一次的页面。
    testoneStore.dispatch(setTab('group'))
    // testone store 是单例：切换到另一个机器人时清掉上一个机器人的内存
    // 消息，避免打开瞬间看到旧的聊天缓存（连接后会按 host:port 重新加载）。
    testoneStore.dispatch(setPrivateMessages([]))
    testoneStore.dispatch(setGroupMessages([]))
    void loadTestPort(root, false).then(result => {
      const port = result.data?.port ?? 17117
      testoneStore.dispatch(
        wsConnectRequest({
          name: '本机测试服务',
          // 打开即固定连接本机地址；实际 socket 经后端同源代理转发。
          host: '127.0.0.1',
          port
        })
      )
    })
    return () => {
      setTestoneWsUrl(null)
      testoneStore.dispatch(wsCancel())
    }
  }, [loadTestPort, root, wsUrl])

  return (
    <div className="testone-root testone-window flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
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
