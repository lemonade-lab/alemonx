import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { MemoryRouter } from 'react-router-dom'
import { store } from '../../src/store/guideStore'
import { Dashboard } from '../../src/components/Dashboard'
import '../../src/styles.css'

const noop = () => {}
createRoot(document.getElementById('root')!).render(
  <Provider store={store}>
    <MemoryRouter>
      <Dashboard
        report={{ checks: [] }}
        checking={false}
        error=""
        defaultPage="robot"
        onOpenGuide={noop}
        onOpenSettings={noop}
        onClearError={noop}
        onCheck={noop}
        onFix={noop}
      />
    </MemoryRouter>
  </Provider>
)
