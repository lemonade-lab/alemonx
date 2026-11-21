import {
  desktopPlatform,
  getAuxiliaryWindowName
} from '../components/nativeWindow'
import type { AuthStatus } from './workbenchAuth'
import { readDashboardNavigation } from './dashboardNavigation'
import { requestRobots, type RobotList } from './workspaceRobots'

export const auxiliaryName = desktopPlatform
  ? getAuxiliaryWindowName(window.location.pathname)
  : undefined

export const loadWorkbench = () => import('../App')
export const loadAuxiliaryWindow = () =>
  import('../components/NativeAuxiliaryWindow').then(module => ({
    default: module.NativeAuxiliaryWindow
  }))

function preloadActiveRuntime(robots: Promise<RobotList>) {
  if (!window.location.pathname.startsWith('/dashboard')) return
  const navigation = readDashboardNavigation(window.location.search)
  if (
    navigation.page !== 'robot' ||
    navigation.section !== 'runtime' ||
    navigation.dshOpen
  )
    return

  void robots
    .then(async ({ projects, activeProjectID }) => {
      const root = projects.find(
        project => project.id === activeProjectID
      )?.path
      // The dashboard must resolve links to a different robot itself.
      if (!root || (navigation.root && navigation.root !== root)) return
      const [{ store }, { workspaceApi }] = await Promise.all([
        import('../store/guideStore'),
        import('../store/workspaceApi')
      ])
      void store.dispatch(
        workspaceApi.endpoints.robotRuntime.initiate(root, { subscribe: false })
      )
    })
    .catch(() => {
      // A normal dashboard query still handles errors and offers a retry.
    })
}

export function prepareWorkbenchRoute(initialStatus: Promise<AuthStatus>) {
  return initialStatus.then(
    async status => {
      if (status.enabled && !status.authenticated) return
      if (auxiliaryName)
        return { auxiliary: (await loadAuxiliaryWindow()).default }
      const initialRobots = requestRobots()
      // The workbench presents request errors and exposes a fresh retry.
      void initialRobots.catch(() => {})
      preloadActiveRuntime(initialRobots)
      return {
        status,
        initialRobots,
        workbench: (await loadWorkbench()).default
      }
    },
    () => undefined
  )
}
