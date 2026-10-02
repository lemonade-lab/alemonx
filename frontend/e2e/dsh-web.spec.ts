import { expect, test } from '@playwright/test'

async function mount(
  page: import('@playwright/test').Page,
  root = '/mock/project'
) {
  await page.route('**/dsh-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const { default: React } = await import('/node_modules/.vite/deps/react.js');
      const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
      const { DSHWorkspace } = await import('/src/components/DSHWorkspace.tsx');
      const { Provider } = await import('/node_modules/.vite/deps/react-redux.js');
      const { store } = await import('/src/store/guideStore.ts');
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store}, React.createElement(DSHWorkspace, {root:${JSON.stringify(root)},onOpenWeb:url=>{document.body.dataset.openedUrl=url}})));
    </script>`
    })
  )
  await page.goto('/dsh-regression')
}

test('opens official Web UI through the browser without custom chat or configuration', async ({
  page
}) => {
  const requests: string[] = []
  await page.route('**/api/v1/dsh/**', async route => {
    requests.push(
      route.request().method() + ' ' + new URL(route.request().url()).pathname
    )
    await route.fulfill({
      json: { url: 'http://127.0.0.1:4242/?token=launch' }
    })
  })
  const root = 'C:\\机器人\\demo'
  await mount(page, root)
  await page.getByRole('button', { name: '打开 Web 版' }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-opened-url',
    'http://127.0.0.1:4242/?token=launch'
  )
  const token = Buffer.from(root).toString('base64url')
  expect(requests).toEqual([`POST /api/v1/dsh/runtimes/${token}/web`])
  await expect(page.getByRole('textbox')).toHaveCount(0)
})

test('startup failure is retryable and does not open a browser tab', async ({
  page
}) => {
  let attempts = 0
  await page.route('**/api/v1/dsh/**', async route => {
    attempts++
    await route.fulfill(
      attempts === 1
        ? { status: 503, json: { error: 'DSH Web 版启动失败，请重试' } }
        : { json: { url: 'http://127.0.0.1:4242/?token=retry' } }
    )
  })
  await mount(page)
  await page.getByRole('button', { name: '打开 Web 版' }).click()
  await expect(page.getByRole('alert')).toContainText('DSH Web 版启动失败')
  await expect(page.locator('body')).not.toHaveAttribute('data-opened-url')
  await page.getByRole('button', { name: '重试打开 Web 版' }).click()
  await expect(page.locator('body')).toHaveAttribute(
    'data-opened-url',
    'http://127.0.0.1:4242/?token=retry'
  )
})

test('requires a selected robot project', async ({ page }) => {
  await mount(page, '')
  await expect(page.getByRole('button', { name: '打开 Web 版' })).toBeDisabled()
  await expect(page.getByText('请先选择机器人项目。')).toBeVisible()
})
