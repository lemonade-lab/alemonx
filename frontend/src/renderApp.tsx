import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { AuthGate } from './components/AuthControl'
import { ErrorBoundary } from './components/ErrorBoundary'
import { store } from './store/guideStore'
import { WorkbenchHeader } from './components/WorkbenchHeader'
import {
  desktopPlatform,
  getAuxiliaryWindowName
} from './components/nativeWindow'

const NativeAuxiliaryWindow = lazy(() =>
  import('./components/NativeAuxiliaryWindow').then(module => ({
    default: module.NativeAuxiliaryWindow
  }))
)

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

const auxiliaryName = desktopPlatform && getAuxiliaryWindowName(window.location.pathname)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <Provider store={store}>
        {desktopPlatform && <WorkbenchHeader fallback />}
        <AuthGate>
          <BrowserRouter>
            {auxiliaryName ? (
              <Suspense
                fallback={
                  <p className="native-auxiliary-content p-4" role="status">
                    正在加载…
                  </p>
                }
              >
                <NativeAuxiliaryWindow
                  name={auxiliaryName}
                />
              </Suspense>
            ) : (
              <App />
            )}
          </BrowserRouter>
        </AuthGate>
      </Provider>
    </ErrorBoundary>
  </StrictMode>
)
