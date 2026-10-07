import { expect, test, type Page } from '@playwright/test'

const npmResource = {
  id: 'npm-plugin',
  type: 'js-plugin',
  subtype: 'js-module-data',
  name: '展示名称',
  description: '平台插件',
  installMode: 'npm',
  packageName: '@example/data',
  repositoryUrl: '',
  markdown: '# 平台文档\n\n<script>window.marketInjected=true</script>'
}
const gitResource = {
  id: 'git-plugin',
  type: 'js-plugin',
  subtype: '',
  name: 'Git 插件',
  description: 'Git 分发',
  installMode: 'git',
  repositoryUrl: 'https://github.com/example/plugin',
  markdown: '# Git 文档'
}

async function mount(page: Page) {
  const requests: string[] = []
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url())
    requests.push(url.pathname + url.search)
    if (url.pathname.endsWith('/catalog/resources')) {
      const second = url.searchParams.get('page') === '2'
      return route.fulfill({
        json: {
          data: [second ? gitResource : npmResource],
          total: 21,
          page: second ? 2 : 1,
          pageSize: 20
        }
      })
    }
    if (url.pathname.endsWith('/catalog/resource'))
      return route.fulfill({
        json: {
          data:
            url.searchParams.get('id') === 'git-plugin'
              ? gitResource
              : npmResource
        }
      })
    if (url.pathname.endsWith('/catalog/versions'))
      return route.fulfill({
        json: { latest: '2.0.0', versions: ['2.0.0', '1.0.0'] }
      })
    if (url.pathname.endsWith('/robot/packages'))
      return route.fulfill({ json: { items: [] } })
    if (url.pathname.endsWith('/robot/package-config'))
      return route.fulfill({ status: 404, json: { error: '未安装' } })
    if (url.pathname.endsWith('/robot'))
      return route.fulfill({
        json: { output: '{"dependencies":{"@example/data":"1.0.0"}}' }
      })
    return route.fulfill({ json: { items: [] } })
  })
  await page.route('**/market-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
    import '/src/styles.css';
    import RefreshRuntime from '/@react-refresh';
    RefreshRuntime.injectIntoGlobalHook(window);
    window.$RefreshReg$ = () => {};
    window.$RefreshSig$ = () => type => type;
    window.__vite_plugin_react_preamble_installed__ = true;
    const {default: React} = await import('/node_modules/.vite/deps/react.js');
    const {default: ReactDOM} = await import('/node_modules/.vite/deps/react-dom_client.js');
    const {Provider} = await import('/node_modules/.vite/deps/react-redux.js');
    const {store} = await import('/src/store/guideStore.ts');
    const {EcosystemMarket} = await import('/src/components/EcosystemMarket.tsx');
    ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store}, React.createElement(EcosystemMarket, {root:'/test/robot',type:'js-plugin',subtype:'js-module-data',category:'数据管理',busy:false,onRun: async (action, pkg) => {document.body.dataset.operation = JSON.stringify({action,pkg});return true},onSaveConfig:async()=>true})));
  </script>`
    })
  )
  await page.goto('/market-regression')
  return requests
}

test('platform search, pagination, npm target and safe detail use no legacy market endpoint', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const requests = await mount(page)
  await expect(page.getByRole('button', { name: /展示名称/ })).toBeVisible()
  await page.screenshot({ path: '/tmp/alemonx-market-mobile.png' })
  await expect(page.getByText('已安装', { exact: true })).toBeVisible()
  await page.getByRole('textbox', { name: '搜索资源' }).fill('数据 & 中文')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await expect
    .poll(() =>
      requests.some(
        request =>
          new URL(request, 'http://test').searchParams.get('q') ===
          '数据 & 中文'
      )
    )
    .toBe(true)
  await page.getByRole('button', { name: /展示名称/ }).click()
  await page.getByRole('combobox', { name: '安装版本' }).selectOption('1.0.0')
  await page.getByRole('button', { name: '安装', exact: true }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-operation',
    JSON.stringify({ action: 'install-module', pkg: '@example/data@1.0.0' })
  )
  await page.getByRole('tab', { name: '文档', exact: true }).click()
  await expect(page.getByRole('heading', { name: '平台文档' })).toBeVisible()
  await expect(page.locator('.markdown-page script')).toHaveCount(0)
  await page.getByRole('button', { name: '返回目录' }).click()
  await page.getByRole('button', { name: '下一页' }).click()
  await expect(page.getByRole('button', { name: /Git 插件/ })).toBeVisible()
  expect(
    requests.some(request => /catalog\/(document|package-config)/.test(request))
  ).toBe(false)
})

test('Git mode uses its repository and selected branch without GitHub versions', async ({
  page
}) => {
  const requests = await mount(page)
  await page.getByRole('button', { name: '下一页' }).click()
  await page.getByRole('button', { name: /Git 插件/ }).click()
  await page.getByRole('textbox', { name: 'Git 分支' }).fill('release')
  await page.getByRole('button', { name: '安装', exact: true }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-operation',
    JSON.stringify({
      action: 'install-package',
      pkg: 'git+https://github.com/example/plugin.git#release'
    })
  )
  expect(requests.some(request => request.includes('/catalog/versions'))).toBe(
    false
  )
})

test('a removed resource blocks installation and reports its detail error', async ({
  page
}) => {
  await mount(page)
  await page.route('**/api/v1/catalog/resource?*', route =>
    route.fulfill({ status: 404, json: { error: '资源已下架' } })
  )
  await page.getByRole('button', { name: /展示名称/ }).click()
  await expect(page.getByRole('alert')).toContainText('资源已下架')
  await expect(
    page.getByRole('button', { name: '安装', exact: true })
  ).toHaveCount(0)
})
