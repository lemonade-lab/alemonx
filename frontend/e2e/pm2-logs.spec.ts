import { expect, test } from '@playwright/test'

test('PM2 logs keep live search and historical pagination bounded', async ({
  page
}) => {
  const queries: URL[] = []
  await page.route('**/api/v1/robot/pm2-logs**', route => {
    const url = new URL(route.request().url())
    queries.push(url)
    if (url.pathname.endsWith('/days'))
      return route.fulfill({
        json: {
          days: [{ date: '2026-10-26', count: 100000, out: 100000, err: 0 }]
        }
      })
    const query = url.searchParams.get('query') || ''
    const pageNumber = Number(url.searchParams.get('page') || 1)
    const snapshot = {
      lines: Array.from({ length: query ? 1 : 120 }, (_, index) => ({
        source: 'out',
        text: query ? `matched ${query}` : `page ${pageNumber} record ${index}`
      })),
      page: pageNumber,
      perPage: 120,
      hasOlder: !query,
      hasNewer: pageNumber > 1,
      total: query ? 1 : 100000,
      truncated: false,
      sources: ['out'],
      diagnostics: []
    }
    if (url.pathname.endsWith('/stream'))
      return route.fulfill({
        contentType: 'text/event-stream',
        body: `data: ${JSON.stringify({ snapshot })}\n\n`
      })
    return route.fulfill({ json: snapshot })
  })
  await page.route('**/pm2-regression', route =>
    route.fulfill({
      contentType: 'text/html',
      body: `<div id="root"></div><script type="module">
      import '/src/styles.css';
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const {default: React} = await import('/node_modules/.vite/deps/react.js');
      const {default: ReactDOM} = await import('/node_modules/.vite/deps/react-dom_client.js');
      const {PM2LogsPanel} = await import('/src/components/PM2LogsPanel.tsx');
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(PM2LogsPanel, {
        open:true,root:'/test/robot',minimized:false,zIndex:10,onClose:()=>{},onMinimize:()=>{},onActivate:()=>{}
      }));
    </script>`
    })
  )
  await page.goto('/pm2-regression')
  const output = page.getByRole('region', { name: 'PM2 日志内容' })
  await expect(output).toContainText('page 1 record 119')
  await expect(output).not.toContainText('record 120')
  await page.getByRole('textbox', { name: '搜索 PM2 日志' }).fill('needle')
  await expect(output).toHaveText('OUTmatched needle')
  await expect
    .poll(() =>
      queries.some(
        url =>
          url.pathname.endsWith('/stream') &&
          url.searchParams.get('query') === 'needle'
      )
    )
    .toBe(true)
  await page.getByRole('textbox', { name: '搜索 PM2 日志' }).fill('')
  await page
    .getByRole('combobox', { name: '按日期查看 PM2 日志' })
    .selectOption('2026-10-26')
  await expect(output).toContainText('page 1 record 119')
  await page.getByRole('button', { name: '更早', exact: true }).click()
  await expect(output).toContainText('page 2 record 119')
  await expect(page.getByText('显示 120 行 · 共 100000 条匹配')).toBeVisible()
  await page.getByRole('button', { name: '恢复实时', exact: true }).click()
  await expect(output).toContainText('page 1 record 119')
})
