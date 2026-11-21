import './styles.css'
import { restoreDesktopPreferences } from './lib/desktopPreferences'
import { readAuthStatus } from './lib/workbenchAuth'
import { prepareWorkbenchRoute } from './lib/workbenchRoutes'

async function startWorkbench() {
  // Keep the full compatibility layer for WebViews older than the standard
  // library baseline targeted by the production build.
  if (!('at' in Array.prototype) || !('hasOwn' in Object))
    await import('core-js/stable')

  const initialStatus = readAuthStatus()
  const [{ renderApp }, ready] = await Promise.all([
    import('./renderApp'),
    prepareWorkbenchRoute(initialStatus),
    restoreDesktopPreferences()
  ])
  await renderApp(initialStatus, ready)
}

void startWorkbench().catch(error => {
  console.error('ALemonX 启动失败', error)
  const status = document.getElementById('startup-status')
  const retry = document.getElementById('startup-retry')
  if (status) status.textContent = '工作台加载失败，请刷新重试。'
  if (retry) {
    retry.hidden = false
    retry.addEventListener('click', () => window.location.reload())
  }
})
