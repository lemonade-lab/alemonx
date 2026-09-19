import { useEffect } from 'react'
import { useStoreState } from '../store/guideStore'
import {
  useSaveSystemNetworkMutation,
  useSystemNetworkQuery,
  useTestSystemNetworkMutation,
  useSystemNetworkStatusQuery,
  useDetectSystemNetworkMutation,
  type SystemNetworkSettings,
  type SystemNetworkDefinition
} from '../store/workspaceApi'
import { Button } from './Button'
import { SettingsMessage, SettingsPage } from './SettingsCard'
import { NetworkConnectionForm } from './network/NetworkConnectionForm'
import { NetworkCandidatesDialog } from './network/NetworkCandidatesDialog'
import {
  networkError,
  networkModes,
  groupStatus,
  entryLabel
} from './network/networkSettings'

export function NetworkSettingsPanel() {
  const { data, isLoading, isError, refetch } = useSystemNetworkQuery()
  const [save, { isLoading: saving }] = useSaveSystemNetworkMutation()
  const [test, { isLoading: testing }] = useTestSystemNetworkMutation()
  const [detect] = useDetectSystemNetworkMutation()
  const [polling, setPolling] = useStoreState(false)
  const [editor, setEditor] = useStoreState<{
    settings: SystemNetworkSettings
    group: SystemNetworkDefinition
  } | null>(null)
  const [feedback, setFeedback] = useStoreState<Record<string, string>>({})
  const [migrationClosed, setMigrationClosed] = useStoreState(false)
  const [proxySelected, setProxySelected] = useStoreState(false)
  const visibleMode = proxySelected ? 'proxy' : data?.mode
  const status = useSystemNetworkStatusQuery(undefined, {
    skip: !data || visibleMode !== 'auto' || !!data.migration?.pending,
    pollingInterval: polling ? 2000 : 0
  })
  const states =
    status.currentData?.revision === data?.revision
      ? status.currentData?.groups
      : undefined
  useEffect(() => {
    setPolling(!!states?.some(s => s.state === 'checking'))
  }, [states, setPolling])
  const report = (scope: string, message: string) =>
    setFeedback(current => ({ ...current, [scope]: message }))
  const note = (scope: string) =>
    feedback[scope] && (
      <p role="status" className="mt-2 text-xs text-(--theme-text-secondary)">
        {feedback[scope]}
      </p>
    )
  const persist = async (next: SystemNetworkSettings) => {
    await save(next).unwrap()
    report('test', '')
  }
  const redetect = async (id = '') => {
    try {
      await detect(id).unwrap()
      setPolling(true)
      void status.refetch()
    } catch (error) {
      report(id || 'mode', networkError(error))
    }
  }
  if (isLoading)
    return (
      <SettingsPage title="网络">
        <span role="status">加载中…</span>
      </SettingsPage>
    )
  if (isError || !data)
    return (
      <SettingsPage title="网络">
        <SettingsMessage tone="error">无法读取网络设置</SettingsMessage>
        <Button onClick={() => void refetch()}>重试</Button>
      </SettingsPage>
    )
  const migration = data.migration?.pending
  const testConnection = async () => {
    report('test', '测试中…')
    try {
      const result = await test({
        target: 'github-api',
        settings: data
      }).unwrap()
      report('test', `${result.message} · ${result.latencyMs ?? 0} ms`)
    } catch (error) {
      report('test', networkError(error))
    }
  }
  return (
    <SettingsPage className="min-w-0">
      <fieldset
        disabled={saving || testing || !!migration}
        className="m-0 min-w-0 border-0 p-0"
      >
        <div
          role="radiogroup"
          aria-label="全局网络模式"
          className="sticky top-0 z-10 grid grid-cols-3 gap-1 rounded-lg bg-(--theme-surface-raised) p-1"
        >
          {(['auto', 'direct', 'proxy'] as const).map(mode => (
            <label
              key={mode}
              className={`relative cursor-pointer rounded-md px-3 py-2.5 text-center text-sm has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-(--theme-accent) ${visibleMode === mode ? 'bg-(--theme-surface-panel) text-(--theme-accent-text) shadow-sm' : 'text-(--theme-text-muted)'}`}
            >
              <input
                className="absolute inset-0 size-full cursor-pointer opacity-0"
                type="radio"
                name="network-mode"
                checked={visibleMode === mode}
                onChange={() => {
                  report('mode', '')
                  if (mode === 'proxy') {
                    setProxySelected(true)
                    return
                  }
                  if (mode === data.mode) {
                    setProxySelected(false)
                    return
                  }
                  void persist({ ...data, mode })
                    .then(() => {
                      setProxySelected(false)
                      report('test', '')
                    })
                    .catch(error => report('mode', networkError(error)))
                }}
              />
              {networkModes[mode]}
            </label>
          ))}
        </div>
        {note('mode')}
        {visibleMode === 'auto' && (
          <section className="mt-4" aria-label="自动选路">
            <header className="flex items-center justify-between">
              <h3 className="text-sm font-medium">自动选路</h3>
              <Button disabled={polling} onClick={() => void redetect()}>
                全部检测
              </Button>
            </header>
            {status.isError && (
              <div role="alert" className="py-3 text-sm">
                检测状态读取失败{' '}
                <Button onClick={() => void status.refetch()}>重试</Button>
              </div>
            )}
            {status.isLoading && <p role="status">加载分组…</p>}
            <div className="mt-2 divide-y divide-(--theme-border-default)">
              {status.currentData?.definitions.map(group => {
                const state = states?.find(item => item.id === group.id)
                return (
                  <div
                    key={group.id}
                    role="group"
                    aria-label={group.label}
                    className="flex min-w-0 flex-wrap items-center justify-between gap-3 py-3"
                  >
                    <div className="min-w-0 flex-1">
                      <h4 className="text-sm font-medium">{group.label}</h4>
                      <p
                        className="mt-1 truncate text-xs text-(--theme-text-muted)"
                        title={state?.current}
                      >
                        {entryLabel(state?.current)}
                      </p>
                      <p role="status" className="mt-1 text-xs">
                        {groupStatus(state)}
                      </p>
                      {note(group.id)}
                    </div>
                    <div className="flex gap-2">
                      <Button
                        disabled={state?.state === 'checking'}
                        aria-label={`重新检测 ${group.label}`}
                        onClick={() => void redetect(group.id)}
                      >
                        重新检测
                      </Button>
                      <Button
                        aria-label={`配置 ${group.label}`}
                        onClick={() => setEditor({ settings: data, group })}
                      >
                        配置
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>
          </section>
        )}
        {visibleMode === 'proxy' && (
          <NetworkConnectionForm settings={data} onSave={persist} />
        )}
        {visibleMode === 'direct' && (
          <section className="mt-4 space-y-3" aria-label="直连状态">
            <div className="flex gap-2">
              <Button loading={testing} onClick={() => void testConnection()}>
                测试连接
              </Button>
            </div>
            {note('test')}
          </section>
        )}
      </fieldset>
      {migration && (
        <div className="mt-4">
          <Button onClick={() => setMigrationClosed(false)}>
            确认网络迁移
          </Button>
        </div>
      )}
      {migration && !migrationClosed && (
        <NetworkConnectionForm
          settings={data}
          migration
          onSave={persist}
          onClose={() => setMigrationClosed(true)}
        />
      )}
      {editor && (
        <NetworkCandidatesDialog
          settings={editor.settings}
          group={editor.group}
          onSave={persist}
          onClose={() => setEditor(null)}
        />
      )}
    </SettingsPage>
  )
}
