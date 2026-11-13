import { lazy, Suspense, useEffect } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { workspaceApi } from '../store/workspaceApi'
import type { RootState } from '../store/guideStore'
import { setDataTab } from '../store/dataStore'
import { AppSettingsPanel } from './AppSettingsPanel'
import { SidebarWindow } from './SidebarWindow'
import { dataNavigation } from './data/dataNavigation'
import { NativeRobotWindow } from './Dashboard'
import type { AuxiliaryWindowName } from './nativeWindow'

const DataWorkspace = lazy(() => import('./data/DataWorkspace'))

export function NativeAuxiliaryWindow({ name }: { name: AuxiliaryWindowName }) {
  const dispatch = useDispatch()
  const dataTab = useSelector((state: RootState) => state.dataPreferences.tab)
  const title = name === 'settings' ? '设置' : '数据管理'

  useEffect(() => {
    if (name === 'settings' || name === 'data')
      document.title = title + ' · AlemonX'
  }, [name, title])

  useEffect(() => {
    const channel = new BroadcastChannel('alx-events')
    channel.onmessage = ({ data }) => {
      if (data?.type !== 'event') return
      const { topic, type } = data.envelope ?? {}
      if (topic === 'robot' && type === 'task')
        dispatch(workspaceApi.util.invalidateTags(['OperationTasks']))
      if (topic === 'plugins')
        dispatch(workspaceApi.util.invalidateTags(['SetupPlugins']))
      if (topic === 'ops' || type === 'system.store-recovered')
        window.dispatchEvent(new Event('alx:ops-changed'))
      if (type === 'system.update.changed')
        dispatch(workspaceApi.util.invalidateTags(['SetupUpdate']))
      window.dispatchEvent(new CustomEvent('alx:unified-event', { detail: data.envelope }))
    }
    return () => channel.close()
  }, [dispatch])

  if (name !== 'settings' && name !== 'data')
    return <NativeRobotWindow name={name} />

  const props = {
    native: true,
    open: true,
    minimized: false,
    title,
    icon: null,
    zIndex: 100,
    onClose: () => {},
    onMinimize: () => {},
    onActivate: () => {}
  }

  return (
    <>
      {name === 'settings' ? (
        <AppSettingsPanel id="app-settings" {...props} />
      ) : (
        <SidebarWindow
          id="system:data"
          {...props}
          sidebarAriaLabel="数据导航"
          items={dataNavigation}
          activeItem={dataTab}
          onActiveItemChange={item => dispatch(setDataTab(item))}
        >
          <Suspense
            fallback={
              <p className="p-4" role="status">
                正在加载数据管理…
              </p>
            }
          >
            <DataWorkspace />
          </Suspense>
        </SidebarWindow>
      )}
    </>
  )
}
