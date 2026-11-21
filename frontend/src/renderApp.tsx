import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { BrowserRouter } from 'react-router-dom'
import { DeferredPanel } from './components/DeferredPanel'
import type { AuthStatus } from './lib/workbenchAuth'
import { AuthGate } from './components/AuthControl'
import { ErrorBoundary } from './components/ErrorBoundary'
import { restoreStore, store } from './store/guideStore'
import { connectDesktopTrayNetwork } from './lib/desktopTrayNetwork'
import { WorkbenchHeader } from './components/WorkbenchHeader'
import { desktopPlatform } from './components/nativeWindow'
import {
  auxiliaryName,
  loadAuxiliaryWindow,
  loadWorkbench,
  type prepareWorkbenchRoute
} from './lib/workbenchRoutes'

const LazyWorkbench = lazy(loadWorkbench)
const LazyAuxiliaryWindow = lazy(loadAuxiliaryWindow)

if (desktopPlatform) {
  document.documentElement.dataset.nativeDesktop = desktopPlatform
  window.addEventListener('storage', event => {
    if (event.key !== 'alemonjs-theme') return
    const theme = event.newValue
    if (theme !== 'light' && theme !== 'dark') return
    document.documentElement.dataset.theme = theme
    document.documentElement.style.colorScheme = theme
    window.dispatchEvent(new Event('alx:theme-changed'))
  })
}

export async function renderApp(
  initialStatus: Promise<AuthStatus>,
  ready: Awaited<ReturnType<typeof prepareWorkbenchRoute>>
) {
  await restoreStore()
  const Workbench = ready?.workbench ?? LazyWorkbench
  const AuxiliaryWindow = ready?.auxiliary ?? LazyAuxiliaryWindow
  if (desktopPlatform === 'mac') connectDesktopTrayNetwork(!auxiliaryName)
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <ErrorBoundary>
        <Provider store={store}>
          {desktopPlatform && <WorkbenchHeader fallback />}
          <AuthGate initialStatus={initialStatus}>
            {status => (
              <BrowserRouter>
                {auxiliaryName ? (
                  <Suspense
                    fallback={
                      <p className="native-auxiliary-content p-4" role="status">
                        正在加载…
                      </p>
                    }
                  >
                    <AuxiliaryWindow name={auxiliaryName} />
                  </Suspense>
                ) : (
                  <DeferredPanel label="工作台">
                    <Workbench
                      initialRobots={
                        ready?.status === status
                          ? ready.initialRobots
                          : undefined
                      }
                    />
                  </DeferredPanel>
                )}
              </BrowserRouter>
            )}
          </AuthGate>
        </Provider>
      </ErrorBoundary>
    </StrictMode>
  )
}
