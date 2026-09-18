import { createRoot } from 'react-dom/client'
import { StrictMode } from 'react'
import { Provider, useDispatch, useSelector } from 'react-redux'
import { store, type RootState } from '../../src/store/guideStore'
import { setDataTab } from '../../src/store/dataStore'
import { SidebarWindow } from '../../src/components/SidebarWindow'
import { dataNavigation } from '../../src/components/data/dataNavigation'
import DataWorkspace from '../../src/components/data/DataWorkspace'
import '../../src/styles.css'

export function Fixture() {
  const tab = useSelector((state: RootState) => state.dataPreferences.tab)
  const dispatch = useDispatch()
  return (
    <SidebarWindow
      id="data-test"
      open
      title="数据"
      sidebarAriaLabel="数据导航"
      items={dataNavigation}
      activeItem={tab}
      onActiveItemChange={item => dispatch(setDataTab(item))}
      onClose={() => {}}
      width={1080}
      height={720}
    >
      <DataWorkspace />
    </SidebarWindow>
  )
}
const root = createRoot(document.getElementById('root')!)
root.render(
  <StrictMode>
    <Provider store={store}>
      <Fixture />
    </Provider>
  </StrictMode>
)
import.meta.hot?.dispose(() => root.unmount())
