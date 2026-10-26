import { expect, test, type Page } from '@playwright/test'
import {
  migrateNavigationValues,
  navigationKey
} from '../src/store/navigationStore'

const sample = {
  name: 'fixture-plugin',
  version: '1.2.3',
  description: '示例插件',
  path: '/test/robot/packages/fixture-plugin',
  valid: true
}
async function fixture(
  page: Page,
  options: {
    dirty?: boolean
    latest?: string
    fail?: boolean
    delayList?: boolean
  } = {}
) {
  const operations: Array<{ name: string; version: string; force?: boolean }> =
    []
  let version = sample.version
  let versionQueries = 0
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/robot/packages')) {
      if (options.delayList)
        await new Promise(resolve => setTimeout(resolve, 200))
      return route.fulfill({ json: { items: [{ ...sample, version }] } })
    }
    if (url.pathname.endsWith('/packages/status')) {
      const status = {
        name: sample.name,
        source: 'git',
        current: '1234567',
        branch: 'main',
        enabled: true,
        dirty: options.dirty,
        issues: []
      }
      return route.fulfill({
        json: url.searchParams.has('package') ? status : { items: [status] }
      })
    }
    if (url.pathname.endsWith('/package-versions')) {
      versionQueries++
      if (options.fail)
        return route.fulfill({
          status: 502,
          json: { error: '版本服务暂不可用' }
        })
      return route.fulfill({
        json: {
          source: 'git',
          current: '1234567',
          latest: options.latest ?? 'abcdef0123456789',
          versions: ['abcdef0123456789'],
          branch: 'main',
          dirty: options.dirty
        }
      })
    }
    if (url.pathname.endsWith('/robot/manifest'))
      return route.fulfill({
        json: {
          private: true,
          workspacesEnabled: true,
          workspaces: ['packages/*']
        }
      })
    if (url.pathname.endsWith('/robot/apps'))
      return route.fulfill({ json: { items: [sample.name] } })
    if (url.pathname.endsWith('/package-config'))
      return route.fulfill({ json: { fields: [], values: {} } })
    if (url.pathname.endsWith('/package-readme'))
      return route.fulfill({ json: { output: '# 示例插件文档' } })
    return route.fulfill({ json: { items: [] } })
  })
  await page.exposeFunction(
    'replaceFixturePackage',
    (input: { name: string; version: string; force?: boolean }) => {
      operations.push(input)
      version = '1.2.4'
      return true
    }
  )
  await page.route('**/backpack-regression**', route =>
    route.fulfill({
      contentType: 'text/html',
      body: `
    <div id="root"></div><script type="module">
    import '/src/styles.css';
    import RefreshRuntime from '/@react-refresh';
    RefreshRuntime.injectIntoGlobalHook(window); window.$RefreshReg$=()=>{}; window.$RefreshSig$=()=>type=>type;
    window.__vite_plugin_react_preamble_installed__=true;
    const {default:React}=await import('/node_modules/.vite/deps/react.js');
    const {default:ReactDOM}=await import('/node_modules/.vite/deps/react-dom_client.js');
    const {Provider}=await import('/node_modules/.vite/deps/react-redux.js');
    const {store,persistor}=await import('/src/store/guideStore.ts');
    const {BackpackPanel}=await import('/src/components/Dashboard.tsx');
    const {useLocalPackagesQuery}=await import('/src/store/workspaceApi.ts');
    await new Promise(resolve=>{if(persistor.getState().bootstrapped)return resolve();const stop=persistor.subscribe(()=>{if(persistor.getState().bootstrapped){stop();resolve()}})});
    const root=new URLSearchParams(location.search).get('root')||'/test/robot';
    function Fixture(){const {currentData,isFetching,error,refetch}=useLocalPackagesQuery(root);return React.createElement(BackpackPanel,{
      root,items:currentData?.items??[],loading:isFetching,failed:!!error,busy:false,onRefresh:refetch,
      onSetPrivate:async()=>true,onSaveConfig:async()=>true,onConfigChanged:async()=>{},onSetAppEnabled:async()=>{},onRemove:async()=>{},
      onReplace:(name,version,force)=>window.replaceFixturePackage({name,version,force}),onPackageAction:async()=>true,onOpenLogs:()=>{},onClone:()=>{},onOpenPlugins:()=>{}
    })}
    ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider,{store},React.createElement(Fixture)));
    </script>`
    })
  )
  await page.goto('/backpack-regression')
  return { operations, versionQueries: () => versionQueries }
}

test('list shows installed version and Git revision, then updates without opening details', async ({
  page
}) => {
  const state = await fixture(page)
  await expect(page.getByText('v1.2.3', { exact: true })).toBeVisible()
  await expect(page.getByText('Git main · 1234567')).toBeVisible()
  expect(state.versionQueries()).toBe(0)
  await page.getByRole('button', { name: '更新 fixture-plugin' }).click()
  await expect(page.getByText('v1.2.4', { exact: true })).toBeVisible()
  expect(state.operations).toEqual([
    { name: sample.name, version: 'abcdef0123456789' }
  ])
  await expect(page.getByRole('tablist', { name: '插件详情' })).toHaveCount(0)
})

test('dirty plugins never force-update from the list', async ({ page }) => {
  const state = await fixture(page, { dirty: true })
  await page.getByRole('button', { name: '更新 fixture-plugin' }).click()
  await expect(page.getByRole('alert')).toContainText('本地修改')
  expect(state.operations).toEqual([])
})

test('short Git hash recognizes latest, and version-service errors remain retryable', async ({
  page
}) => {
  const state = await fixture(page, { latest: '123456789abcdef' })
  await page.getByRole('button', { name: '更新 fixture-plugin' }).click()
  await expect(page.getByText('已是最新版本')).toBeVisible()
  expect(state.operations).toEqual([])
})

test('failed version lookup reports error without updating', async ({
  page
}) => {
  const state = await fixture(page, { fail: true })
  await page.getByRole('button', { name: '更新 fixture-plugin' }).click()
  await expect(page.getByRole('alert')).toContainText('版本服务暂不可用')
  await expect(
    page.getByRole('button', { name: '更新 fixture-plugin' })
  ).toBeEnabled()
  expect(state.operations).toEqual([])
})

test('selected plugin and tab survive delayed loading on reload and remain scoped per robot', async ({
  page
}) => {
  await fixture(page, { delayList: true })
  await page.getByRole('button', { name: /fixture-plugin.*v1.2.3/ }).click()
  await page.getByRole('tab', { name: '文档', exact: true }).click()
  await expect(
    page.getByRole('heading', { name: '示例插件文档' })
  ).toBeVisible()
  await expect
    .poll(() =>
      page.evaluate(
        () => localStorage.getItem('persist:alemonx-navigation') ?? ''
      )
    )
    .toContain('readme')
  await page.reload()
  await expect(
    page.getByRole('tab', { name: '文档', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  await page.goto('/backpack-regression?root=/test/another-robot')
  await expect(
    page.getByRole('button', { name: '更新 fixture-plugin' })
  ).toBeVisible()
  await expect(page.getByRole('tablist', { name: '插件详情' })).toHaveCount(0)
  await page.goto('/backpack-regression')
  await expect(
    page.getByRole('tab', { name: '文档', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
})

test('navigation migration rejects unknown areas and non-navigation values', () => {
  const key = navigationKey('backpack.tab', ['/robot', 'plugin'])
  expect(
    migrateNavigationValues({
      [key]: 'readme',
      'password': 'secret',
      '["credentials"]': 'secret',
      '["settings.section"]': { password: 'secret' }
    })
  ).toEqual({ [key]: 'readme' })
  expect(migrateNavigationValues(null)).toEqual({})
})

test('editor navigation survives reload without persisting environment values', async ({
  page
}) => {
  await page.route('**/editor-navigation-regression', route =>
    route.fulfill({
      contentType: 'text/html',
      body: `
    <div id="root"></div><script type="module">
    import '/src/styles.css';
    import RefreshRuntime from '/@react-refresh';
    RefreshRuntime.injectIntoGlobalHook(window); window.$RefreshReg$=()=>{}; window.$RefreshSig$=()=>type=>type;
    window.__vite_plugin_react_preamble_installed__=true;
    const {default:React}=await import('/node_modules/.vite/deps/react.js');
    const {default:ReactDOM}=await import('/node_modules/.vite/deps/react-dom_client.js');
    const {Provider}=await import('/node_modules/.vite/deps/react-redux.js');
    const {store,persistor}=await import('/src/store/guideStore.ts');
    const {EnvConfigForm}=await import('/src/components/EnvConfigForm.tsx');
    await new Promise(resolve=>{if(persistor.getState().bootstrapped)return resolve();const stop=persistor.subscribe(()=>{if(persistor.getState().bootstrapped){stop();resolve()}})});
    ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider,{store},React.createElement(EnvConfigForm,{content:'TOKEN=fixture-private-value',onChange:()=>{}})));
    </script>`
    })
  )
  await page.goto('/editor-navigation-regression')
  await page.getByRole('tab', { name: '文本' }).click()
  await expect
    .poll(() =>
      page.evaluate(
        () => localStorage.getItem('persist:alemonx-navigation') ?? ''
      )
    )
    .toContain('text')
  await page.reload()
  await expect(page.getByRole('tab', { name: '文本' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(
    await page.evaluate(() =>
      localStorage.getItem('persist:alemonx-navigation')
    )
  ).not.toContain('fixture-private-value')
})

test('navigation scopes recognize Windows and UNC paths without folding Unix paths', () => {
  expect(navigationKey('backpack.tab', ['C:\\Bots\\Demo', 'plugin'])).toBe(
    navigationKey('backpack.tab', ['c:/bots/demo/', 'plugin'])
  )
  expect(
    navigationKey('backpack.tab', ['\\\\HOST\\Share\\Demo', 'plugin'])
  ).toBe(navigationKey('backpack.tab', ['//host/share/demo', 'plugin']))
  expect(navigationKey('backpack.tab', ['/Bots/Demo', 'plugin'])).not.toBe(
    navigationKey('backpack.tab', ['/bots/demo', 'plugin'])
  )
})
