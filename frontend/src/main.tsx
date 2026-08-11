// Baseline compatibility layer for older embedded browsers/WebViews. Import it
// before React and application modules so standard APIs are available during
// initial render. Web Crypto remains intentionally outside core-js; callers
// that need it keep their own secure fallback.
import 'core-js/stable'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { AuthGate } from './components/AuthControl'
import { ErrorBoundary } from './components/ErrorBoundary'
import { store } from './store/guideStore'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <Provider store={store}>
        <AuthGate>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </AuthGate>
      </Provider>
    </ErrorBoundary>
  </StrictMode>
)
