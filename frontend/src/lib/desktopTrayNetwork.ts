import { bridge } from '../components/nativeWindow'
import { networkError } from '../components/network/networkSettings'
import { store } from '../store/guideStore'
import {
  desktopNetworkSaved,
  workspaceApi,
  type SystemNetworkMode,
  type SystemNetworkSettings
} from '../store/workspaceApi'

export function connectDesktopTrayNetwork(isWorkbench: boolean) {
  const channel = new BroadcastChannel(desktopNetworkSaved)
  channel.onmessage = () => {
    store.dispatch(workspaceApi.util.invalidateTags(['SystemNetwork']))
    if (isWorkbench) void refresh()
  }
  if (!isWorkbench) return

  let saving = false
  let refreshSequence = 0
  const read = () =>
    store.dispatch(workspaceApi.endpoints.systemNetwork.initiate(undefined, {
      forceRefetch: true,
      subscribe: false
    })).unwrap()
  const publish = (settings?: SystemNetworkSettings, error = '') => {
    const available = settings?.version === 2 && !settings.migration?.pending
    bridge?.postMessage(`alx:tray-network:${JSON.stringify({
      mode: available ? settings.mode : '',
      proxyConfigured: available && !!settings.proxy.url,
      error
    })}`)
  }

  async function refresh() {
    if (saving) return
    const sequence = ++refreshSequence
    const settings = await read().catch(() => undefined)
    if (sequence === refreshSequence) publish(settings)
  }

  async function selectMode(mode: SystemNetworkMode) {
    if (saving) return
    saving = true
    ++refreshSequence
    let settings: SystemNetworkSettings | undefined
    let error = ''
    try {
      settings = await read()
      if (settings.version !== 2 || settings.migration?.pending)
        throw new Error('请先在网络设置中确认配置迁移。')
      if (mode === 'proxy' && !settings.proxy.url)
        throw new Error('请先在网络设置中配置代理地址。')
      if (mode !== settings.mode) {
        settings = await store.dispatch(workspaceApi.endpoints.saveSystemNetwork.initiate(
          { ...settings, mode }, { track: false }
        )).unwrap()
      }
    } catch (reason) {
      error = networkError(reason)
      settings = await read().catch(() => undefined)
    } finally {
      saving = false
    }
    publish(settings, error)
  }

  window.addEventListener('alx:tray-network-select', event => {
    const mode: unknown = (event as CustomEvent<unknown>).detail
    if (mode === 'auto' || mode === 'direct' || mode === 'proxy') void selectMode(mode)
  })
  window.addEventListener(desktopNetworkSaved, refresh)
  window.addEventListener('alx:auth-changed', refresh)
  window.addEventListener('focus', refresh)
  void refresh()
}
