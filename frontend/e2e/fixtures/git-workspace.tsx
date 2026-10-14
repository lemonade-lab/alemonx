import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store, useStoreState } from '../../src/store/guideStore'
import { RobotGitControl } from '../../src/components/RobotGitControl'
import '../../src/styles.css'

export function Fixture() {
  const [open, setOpen] = useStoreState(true)
  const root =
    new URLSearchParams(window.location.search).get('root') ??
    '/tmp/git-workspace-fixture'
  return open ? (
    <RobotGitControl
      project={{ name: 'Git 验证项目', path: root }}
      minimized={false}
      zIndex={10}
      onActivate={() => {}}
      onMinimize={() => {}}
      onClose={() => setOpen(false)}
    />
  ) : (
    <p>窗口已关闭</p>
  )
}

createRoot(document.getElementById('root')!).render(
  <Provider store={store}>
    <Fixture />
  </Provider>
)
