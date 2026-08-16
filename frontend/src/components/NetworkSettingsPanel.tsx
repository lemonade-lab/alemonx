import {
  CheckCircle2,
  Cloud,
  Download,
  GitBranch,
  Network,
  Package,
  RotateCcw,
  Save,
  Wifi
} from 'lucide-react'
import { useEffect, useState } from 'react'
import {
  useSaveSystemNetworkMutation,
  useSystemNetworkQuery,
  useTestSystemNetworkMutation,
  type SystemNetworkMode,
  type SystemNetworkMirrorPreset,
  type SystemNetworkRoute,
  type SystemNetworkRouteSettings
} from '../store/workspaceApi'
import { Button } from './Button'
import { SettingsMessage, SettingsPage } from './SettingsCard'

const defaultRoutes: Record<SystemNetworkRoute, SystemNetworkRouteSettings> = {
  github: { mode: 'mirror', mirrorUrl: 'https://ghfast.top/{url}' },
  gitee: { mode: 'direct' },
  npm: { mode: 'mirror', mirrorUrl: 'https://registry.npmmirror.com{path}' },
  node: { mode: 'mirror', mirrorUrl: 'https://npmmirror.com/mirrors/node{nodepath}' },
  cdn: { mode: 'direct' },
  official: { mode: 'direct' }
}

const routes: Array<{
  id: SystemNetworkRoute
  label: string
  description: string
  hosts: string
  icon: typeof Network
}> = [
  { id: 'github', label: 'GitHub 资源', description: '更新、Release、插件索引，以及已验证系统插件的官方内核下载；下载镜像异常时自动尝试预设，API 被镜像拒绝时会直接回源。', hosts: 'github.com · api.github.com · raw.githubusercontent.com · assets', icon: GitBranch },
  { id: 'gitee', label: 'Gitee 资源', description: '软件目录中的仓库版本与资料。', hosts: 'gitee.com · gitee.com/api/v5', icon: Network },
  { id: 'npm', label: 'NPM 软件目录', description: '仅查询软件目录元数据，不影响项目依赖安装。', hosts: 'registry.npmjs.org', icon: Package },
  { id: 'node', label: 'Node.js 环境包', description: '工作台内安装 Node.js 时使用；默认通过 npmmirror 下载、校验并缓存。', hosts: 'nodejs.org · npmmirror.com/mirrors/node', icon: Download },
  { id: 'cdn', label: '内容 CDN', description: '软件目录的 jsDelivr 缓存资源。', hosts: 'cdn.jsdelivr.net', icon: Cloud },
  { id: 'official', label: 'AlemonX 官方下载', description: '官方系统资源与引导页 Android 下载。', hosts: 'download.alemonjs.com', icon: Download }
]

function routeModes(route: SystemNetworkRoute, presets: Partial<Record<SystemNetworkRoute, SystemNetworkMirrorPreset[]>>): Array<{ id: SystemNetworkMode; label: string }> {
  const modes: Array<{ id: SystemNetworkMode; label: string }> = []
  if (presets[route]?.length) modes.push({ id: 'mirror', label: '选择镜像' })
  modes.push({ id: 'custom-mirror', label: '自定义镜像' }, { id: 'direct', label: '直连' })
  return modes
}

function messageFrom(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'data' in error) {
    const data = (error as { data?: { error?: string } }).data
    if (data?.error) return data.error
  }
  return fallback
}

export function NetworkSettingsPanel() {
  const { data, isLoading, isError } = useSystemNetworkQuery()
  const [saveNetwork, { isLoading: saving }] = useSaveSystemNetworkMutation()
  const [testNetwork, { isLoading: testing }] = useTestSystemNetworkMutation()
  const [selectedRoutes, setSelectedRoutes] = useState(defaultRoutes)
  const [changed, setChanged] = useState(false)
  const [message, setMessage] = useState('')
  const [success, setSuccess] = useState(false)
  const [testingRoute, setTestingRoute] = useState<SystemNetworkRoute | null>(null)

  useEffect(() => {
    if (!data || changed) return
    setSelectedRoutes(Object.fromEntries(
      Object.entries({ ...defaultRoutes, ...data.routes }).map(([route, setting]) => [
        route,
        { ...setting, mode: setting.mode === 'mirror' || setting.mode === 'custom-mirror' ? setting.mode : 'direct' }
      ])
    ) as Record<SystemNetworkRoute, SystemNetworkRouteSettings>)
  }, [changed, data])

  const updateRoute = (route: SystemNetworkRoute, patch: Partial<SystemNetworkRouteSettings>) => {
    setSelectedRoutes(current => ({ ...current, [route]: { ...current[route], ...patch } }))
    setChanged(true)
    setMessage('')
  }

  const restoreRecommended = () => {
    setSelectedRoutes(defaultRoutes)
    setChanged(true)
    setSuccess(false)
    setMessage('已恢复推荐配置，保存后生效。')
  }

  const save = async () => {
    setMessage('')
    setSuccess(false)
    try {
      const next = await saveNetwork({ routes: selectedRoutes }).unwrap()
      setSelectedRoutes({ ...defaultRoutes, ...next.routes })
      setChanged(false)
      setSuccess(true)
      setMessage('系统联网路由已保存。')
      return true
    } catch (error) {
      setMessage(messageFrom(error, '系统联网路由未保存。'))
      return false
    }
  }

  const test = async (route: SystemNetworkRoute, label: string) => {
    if (changed && !(await save())) return
    setTestingRoute(route)
    setMessage('')
    setSuccess(false)
    try {
      const result = await testNetwork(route).unwrap()
      setSuccess(result.ok)
      setMessage(`${label}：${result.message}${result.latencyMs ? ` · ${result.latencyMs} ms` : ''}`)
    } catch (error) {
      setMessage(messageFrom(error, `${label} 无法连接，请检查这一项的镜像地址。`))
    } finally {
      setTestingRoute(null)
    }
  }

  if (isLoading) return <div className="settings-network-panel">正在读取系统联网设置…</div>
  if (isError || !data) return <div className="settings-network-panel">无法读取系统联网设置。</div>

  const mirrorPresets = data.mirrorPresets ?? {}

  return (
    <SettingsPage
      title="网络"
      description="控制系统联网资源（GitHub、NPM、Node、CDN 等）的访问方式与镜像。"
    >
      <header className="settings-network-toolbar">
        <span>系统内容网络</span>
        <div className="settings-network-toolbar-actions">
          <Button
            className="settings-network-restore gap-1.5"
            disabled={!changed}
            onClick={restoreRecommended}
            title="恢复 GitHub 与 NPM 的推荐镜像，其余资源直连"
          >
            <RotateCcw className="size-3.5" />
            恢复推荐
          </Button>
          <Button
            className="settings-network-save gap-1.5"
            disabled={!changed}
            loading={saving}
            loadingLabel="正在保存…"
            onClick={() => void save()}
            variant="primary"
          >
            <Save className="size-3.5" />
            保存
          </Button>
        </div>
      </header>
      <fieldset aria-label="系统内容网络" className="settings-network-routes">
        {routes.map(item => {
          const Icon = item.icon
          const setting = selectedRoutes[item.id]
          return (
            <article className="settings-network-route" key={item.id}>
              <span className="settings-network-route-icon"><Icon className="size-4" /></span>
              <div className="settings-network-route-copy">
                <strong>{item.label}</strong>
                <small>{item.description}</small>
                <code>{item.hosts}</code>
              </div>
              <div className="settings-network-route-actions">
                <select
                  aria-label={`${item.label}连接方式`}
                  onChange={event => {
                    const mode = event.target.value as SystemNetworkMode
                    updateRoute(item.id, {
                      mode,
                      ...(mode === 'mirror' && !setting.mirrorUrl
                        ? { mirrorUrl: mirrorPresets[item.id]?.[0]?.value }
                        : {})
                    })
                  }}
                  value={setting.mode}
                >
                  {routeModes(item.id, mirrorPresets).map(mode => <option key={mode.id} value={mode.id}>{mode.label}</option>)}
                </select>
                <Button
                  className="settings-network-test"
                  loading={testing && testingRoute === item.id}
                  loadingLabel="测试中…"
                  onClick={() => void test(item.id, item.label)}
                  title={`测试 ${item.label}`}
                >
                  <Wifi className="size-3.5" />
                  测试
                </Button>
              </div>
              {setting.mode === 'mirror' && mirrorPresets[item.id]?.length && (
                <label className="settings-network-route-input">
                  <span>
                    镜像地址
                    <small title="默认已选择适合该类别的镜像地址。">已选推荐值</small>
                  </span>
                  <select
                    aria-label={`${item.label}镜像地址`}
                    onChange={event => updateRoute(item.id, { mirrorUrl: event.target.value })}
                    value={setting.mirrorUrl ?? mirrorPresets[item.id]?.[0]?.value}
                  >
                    {setting.mirrorUrl && !mirrorPresets[item.id]?.some(preset => preset.value === setting.mirrorUrl) && (
                      <option value={setting.mirrorUrl}>当前配置（不在预设中）</option>
                    )}
                    {mirrorPresets[item.id]?.map(preset => <option key={preset.value} value={preset.value}>{preset.label}</option>)}
                  </select>
                </label>
              )}
              {setting.mode === 'custom-mirror' && (
                <label className="settings-network-route-input">
                  <span>
                    镜像模板
                    <small
                      title={
                        item.id === 'npm'
                          ? '使用 {path} 保留软件包路径，例如 https://registry.example{path}。'
                          : '使用 {url} 代表原始官方地址，例如 https://mirror.example/{url}。'
                      }
                    >
                      {item.id === 'npm' ? '使用 {path}' : '使用 {url}'}
                    </small>
                  </span>
                  <input
                    autoCapitalize="none"
                    autoComplete="url"
                    inputMode="url"
                    onChange={event => updateRoute(item.id, { mirrorUrl: event.target.value })}
                    placeholder="https://mirror.example/{url}"
                    spellCheck={false}
                    value={setting.mirrorUrl ?? ''}
                  />
                </label>
              )}
            </article>
          )
        })}
      </fieldset>

      {message && (
        <SettingsMessage tone={success ? 'success' : 'info'}>
          {success && <CheckCircle2 className="size-3.5" />}
          {message}
        </SettingsMessage>
      )}
    </SettingsPage>
  )
}
