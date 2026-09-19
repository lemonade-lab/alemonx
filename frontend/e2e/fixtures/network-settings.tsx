import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from '../../src/store/guideStore'
import { NetworkSettingsPanel } from '../../src/components/NetworkSettingsPanel'
import '../../src/styles.css'

createRoot(document.getElementById('root')!).render(
  <Provider store={store}><NetworkSettingsPanel /></Provider>
)
