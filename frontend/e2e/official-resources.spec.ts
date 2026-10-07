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
async function mount(
  page: Page,
  config = false,
  offline = false,
  componentName = ''
) {
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
  const component =
    componentName || (config ? 'RobotConfigForm' : 'ConnectionCatalog')
  await page.route('**/official-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
 import '/src/styles.css';import RefreshRuntime from '/@react-refresh';RefreshRuntime.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{};window.$RefreshSig$=()=>type=>type;window.__vite_plugin_react_preamble_installed__=true;
 const {default:React}=await import('/node_modules/.vite/deps/react.js');const {default:ReactDOM}=await import('/node_modules/.vite/deps/react-dom_client.js');const {Provider}=await import('/node_modules/.vite/deps/react-redux.js');const {store}=await import('/src/store/guideStore.ts');const {${component}}=await import('/src/components/${config ? 'RobotConfigForm' : 'OfficialResources'}.tsx');
 ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider,{store},React.createElement(${component},{root:'/test/robot',label:'${component === 'OfficialExtensionChoices' ? '项目扩展' : '登录连接'}',selected:[],id:'dependency-options',onSelect:resource=>{document.body.dataset.selected=resource.id},content:'login: local-id\\napps:\\n  - enabled-module\\n  - custom-offline\\n',onChange:value=>{document.body.dataset.config=value},onInstall:async(action,target)=>{document.body.dataset.install=JSON.stringify({action,target});await fetch('/api/v1/test-install');return true},onLogin:(login,pkg)=>{document.body.dataset.login=JSON.stringify({login,pkg})}})));
 </script>`
    })
  )
  await page.goto('/official-regression')
}
test('official npm and Git candidates use installed declarations and install only missing packages', async ({
  page
}) => {
  await mount(page)
  const selector = page.getByRole('combobox', { name: '登录连接' })
  await expect(
    selector.getByRole('option', { name: /待安装连接/ })
  ).toHaveCount(1)
  await expect(page.getByRole('textbox')).toHaveCount(0)
  for (const [login, pkg] of [
    ['local-id', '@example/connect'],
    ['git-id', 'actual-git-package']
  ]) {
    await selector.selectOption(login)
    await expect(page.locator('body')).toHaveAttribute(
      'data-login',
      JSON.stringify({ login, pkg })
    )
    await expect(page.locator('body')).not.toHaveAttribute('data-install')
  }
  await selector.selectOption('resource:missing')
  await expect(page.locator('body')).not.toHaveAttribute('data-install')
  await page.getByRole('button', { name: '安装连接包' }).click()
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
  await page.locator('summary').filter({ hasText: '常规运行' }).click()
  await expect(page.getByRole('combobox', { name: '登录连接' })).toHaveValue(
    'local-id'
  )
  await expect(
    page.getByText('官方选项暂时无法读取，仍可选择本地连接。')
  ).toBeVisible()
  await page.getByRole('button', { name: '高级', exact: true }).click()
  await page.locator('summary').filter({ hasText: '运行与模块' }).click()
  const firstRow = page.getByRole('combobox', { name: '模块 1' }).locator('..')
  await expect(firstRow.getByRole('checkbox')).toBeChecked()
  await firstRow.getByRole('checkbox').uncheck()
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /custom-offline/
  )
  await expect(page.getByRole('combobox', { name: '登录连接' })).toBeEnabled()
  await expect(page.getByText('从官方目录添加模块')).toHaveCount(0)
})

test('module suggestions enhance the existing row without changing its enable state', async ({
  page
}) => {
  await mount(page, true)
  await page.getByRole('button', { name: '高级', exact: true }).click()
  await page.locator('summary').filter({ hasText: '运行与模块' }).click()
  const group = page
    .locator('details')
    .filter({ has: page.locator('summary').filter({ hasText: '运行与模块' }) })
  await group.getByRole('button', { name: '+ 新增项' }).click()
  const input = page.getByRole('combobox', { name: '模块 3' })
  await input.fill('@example/missing')
  const row = input.locator('..')
  await row.getByRole('checkbox').uncheck()
  await row.getByRole('button', { name: '安装', exact: true }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-install',
    JSON.stringify({ action: 'install-module', target: '@example/missing' })
  )
  await expect(row.getByRole('checkbox')).not.toBeChecked()
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /custom-offline/
  )
  await expect(page.locator('body')).toHaveAttribute(
    'data-config',
    /'@example\/missing': false/
  )
})

test('creation preserves direct choice buttons without a search or installation step', async ({
  page
}) => {
  await mount(page, false, false, 'OfficialExtensionChoices')
  await expect(page.getByRole('button', { name: /待安装连接/ })).toBeVisible()
  await expect(page.getByRole('textbox')).toHaveCount(0)
  await expect(page.getByRole('combobox')).toHaveCount(0)
  await page.getByRole('button', { name: /待安装连接/ }).click()
  await expect(page.locator('body')).toHaveAttribute('data-selected', 'missing')
  await expect(page.locator('body')).not.toHaveAttribute('data-install')
})

test('dependency suggestions remain native options and accept arbitrary package names', async ({
  page
}) => {
  await mount(page, false, false, 'OfficialDependencyOptions')
  await page.evaluate(() => {
    const input = document.createElement('input')
    input.setAttribute('list', 'dependency-options')
    input.setAttribute('aria-label', '包名')
    document.body.append(input)
  })
  await expect(
    page.locator('datalist option[value="@example/missing"]')
  ).toHaveCount(1)
  await expect(page.getByRole('combobox', { name: '包名' })).toHaveCount(1)
  await page.getByRole('combobox', { name: '包名' }).fill('@custom/package')
  await expect(page.getByRole('combobox', { name: '包名' })).toHaveValue(
    '@custom/package'
  )
  await expect(page.getByRole('button')).toHaveCount(0)
})
