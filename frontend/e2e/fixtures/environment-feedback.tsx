import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { useStoreState, store } from '../../src/store/guideStore'
import { EnvironmentFixDialog } from '../../src/components/EnvironmentFixDialog'
import { DownloadProgress } from '../../src/components/DownloadProgress'
import '../../src/styles.css'

export function Fixture() {
  const [open, setOpen] = useStoreState(true)
  return (
    <>
      {open ? (
        <EnvironmentFixDialog
          check={{
            id: 'node',
            name: 'Node.js',
            suggestion: '安装机器人所需的运行环境。'
          }}
          platform="linux/amd64"
          onClose={() => setOpen(false)}
        />
      ) : (
        <p>窗口已关闭</p>
      )}
      <DownloadProgress label="未知进度" progress={Number.NaN} />
    </>
  )
}

createRoot(document.getElementById('root')!).render(
  <Provider store={store}>
    <Fixture />
  </Provider>
)
