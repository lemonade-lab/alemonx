import { expect, test, type Page } from '@playwright/test'
const connector = {
  id: 'official',
  type: 'connector',
  name: '官方连接',
  description: '连接简介',
  installMode: 'npm',
  packageName: '@example/connect',
  repositoryUrl: ''
}
const gitConnector = {
  ...connector,
  id: 'git',
  name: 'Git 连接',
  installMode: 'git',
  packageName: undefined,
  repositoryUrl: 'https://github.com/example/connect'
}
const missing = {
  ...connector,
  id: 'missing',
  name: '待安装连接',
  packageName: '@example/missing'
}
async function mount(page: Page, config = false, offline = false) {
  let installed = false
  await page.route('**/api/v1/**', route => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (path.endsWith('/catalog/options'))
      return route.fulfill(
        offline
          ? { status: 503, json: { error: 'offline' } }
          : {
              json: {
                data:
                  url.searchParams.get('type') === 'js-plugin'
                    ? [{ ...missing, type: 'js-plugin', name: '官方模块' }]
                    : [connector, gitConnector, missing]
              }
            }
      )
    if (path.endsWith('/catalog/resource'))
      return route.fulfill({
        json: { data: { ...connector, markdown: '# 官方文档' } }
      })
    if (path.endsWith('/robot/package-inventory'))
      return route.fulfill({
        json: {
          items: [
            { name: '@example/connect', installed: true },
            {
              name: 'actual-git-package',
              installed: true,
              repository: 'https://github.com/example/connect.git'
            },
            {
              name: 'enabled-module',
              installed: true,
              loadable: true,
              version: '1',
              enabled: true
            },
            { name: 'ordinary-library', installed: true },
            ...(installed
              ? [{ name: '@example/missing', installed: true, loadable: true }]
              : [])
          ]
        }
      })
    if (path.endsWith('/robot/runtime'))
      return route.fulfill({
        json: {
          platforms: [
            {
              id: 'local-id',
              label: '本地声明',
              package: '@example/connect',
              installed: true
            },
            {
              id: 'git-id',
              label: 'Git 声明',
              package: 'actual-git-package',
              installed: true
            },
            ...(installed
              ? [
                  {
                    id: 'new-id',
                    label: '新声明',
                    package: '@example/missing',
                    installed: true
                  }
                ]
              : [])
          ]
        }
      })
    if (path.endsWith('/test-install')) installed = true
    return route.fulfill({ json: {} })
  })
  const component = config ? 'RobotConfigForm' : 'ConnectionCatalog'
  await page.route('**/official-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
 import '/src/styles.css';import RefreshRuntime from '/@react-refresh';RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;
 const {default:React}=await import('/node_modules/.vite/deps/react.js');const {default:ReactDOM}=await import('/node_modules/.vite/deps/react-dom_client.js');const {Provider}=await import('/node_modules/.vite/deps/react-redux.js');const {store}=await import('/src/store/guideStore.ts');const {${component}}=await import('/src/components/${config ? 'RobotConfigForm' : 'OfficialResources'}.tsx');
 ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider,{store},React.createElement(${component},{root:'/test/robot',content:'login: local-id\\napps:\\n  - enabled-module\\n  - custom-offline\\n',onChange:value=>{document.body.dataset.config=value},onInstall:async(action,target)=>{document.body.dataset.install=JSON.stringify({action,target});await fetch('/api/v1/test-install');return true},onLogin:(login,pkg)=>{document.body.dataset.login=JSON.stringify({login,pkg})}})));
 </script>`
    })
  )
  await page.goto('/official-regression')
}
test('official npm and Git candidates use installed declarations and install only missing packages', async ({
  page
}) => {
  await mount(page)
  await page.getByText('从官方目录选择连接包', { exact: true }).click()
  for (const [name, login, pkg] of [
    ['官方连接', 'local-id', '@example/connect'],
    ['Git 连接', 'git-id', 'actual-git-package']
  ]) {
    await page.getByRole('button', { name: new RegExp(name) }).click()
    await page.getByRole('button', { name: '安装／使用此连接' }).click()
    await expect(page.locator('body')).toHaveAttribute(
      'data-login',
      JSON.stringify({ login, pkg })
    )
    await expect(page.locator('body')).not.toHaveAttribute('data-install')
  }
  await page.getByRole('button', { name: /待安装连接/ }).click()
  await page.getByRole('button', { name: '安装／使用此连接' }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-install',
    JSON.stringify({ action: 'install-connection', target: '@example/missing' })
  )
  await expect(page.locator('body')).toHaveAttribute(
    'data-login',
    JSON.stringify({ login: 'new-id', pkg: '@example/missing' })
  )
})
test('configuration preserves custom apps and local editing while official catalogue is offline', async ({
  page
}) => {
  await mount(page, true, true)
  await page.getByText('登录连接', { exact: true }).click()
  await page.getByText('启用模块', { exact: true }).click()
  await expect(page.getByRole('combobox', { name: '已安装连接' })).toHaveValue(
    'local-id'
  )
  await expect(
    page.getByRole('checkbox', { name: /enabled-module/ })
  ).toBeChecked()
  await expect(page.getByText('ordinary-library')).toHaveCount(0)
  await page.getByRole('checkbox', { name: /enabled-module/ }).uncheck()
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /custom-offline/
  )
  await page.getByText('从官方目录选择连接包', { exact: true }).click()
  await expect(
    page.getByRole('alert').filter({ hasText: '官方目录暂不可用' })
  ).toBeVisible()
  await expect(page.getByRole('combobox', { name: '已安装连接' })).toBeEnabled()
})

test('official module installation refreshes local facts and edits the unsaved enable list', async ({
  page
}) => {
  await mount(page, true)
  await page.getByText('启用模块', { exact: true }).click()
  await page.getByText('从官方目录添加模块', { exact: true }).click()
  await page.getByRole('button', { name: /官方模块/ }).click()
  await page.getByRole('button', { name: '安装／启用此模块' }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-install',
    JSON.stringify({ action: 'install-module', target: '@example/missing' })
  )
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /@example\/missing/
  )
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /custom-offline/
  )
  await expect(page.getByText('已加入启用列表，请保存配置。')).toBeVisible()
})
