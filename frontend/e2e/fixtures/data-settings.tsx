import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from '../../src/store/guideStore'
import { AppSettingsPanel } from '../../src/components/AppSettingsPanel'
import '../../src/styles.css'

createRoot(document.getElementById('root')!).render(
  <Provider store={store}>
    <AppSettingsPanel id="settings-test" open title="设置" onClose={() => {}} />
  </Provider>
)
