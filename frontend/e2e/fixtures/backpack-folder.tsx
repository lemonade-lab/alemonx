import { createRoot } from 'react-dom/client'
import { Provider } from 'react-redux'
import { store } from '../../src/store/guideStore'
import { BackpackFolderInstall } from '../../src/components/BackpackFolderInstall'
import '../../src/styles.css'

const root = createRoot(document.getElementById('root')!)
root.render(<Provider store={store}><BackpackFolderInstall root="/mock/robot" onInstalled={() => { document.title = '安装后已刷新' }} /></Provider>)
import.meta.hot?.dispose(() => root.unmount())
