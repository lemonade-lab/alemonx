import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from '../../src/store/guideStore'
import { DataWorkspaceFixture } from './data-workspace'

const root = createRoot(document.getElementById('root')!)
root.render(
  <StrictMode>
    <Provider store={store}>
      <DataWorkspaceFixture />
    </Provider>
  </StrictMode>
)
import.meta.hot?.dispose(() => root.unmount())
