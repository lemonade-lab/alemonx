import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from '../../src/store/guideStore'
import { RobotGitControl } from '../../src/components/RobotGitControl'
import '../../src/styles.css'

export function Fixture() {
  const root =
    new URLSearchParams(window.location.search).get('root') ??
    '/tmp/git-workspace-fixture'
  return (
    <main className="flex h-dvh">
      <RobotGitControl project={{ name: 'Git 验证项目', path: root }} />
    </main>
  )
}

createRoot(document.getElementById('root')!).render(
  <Provider store={store}>
    <Fixture />
  </Provider>
)
