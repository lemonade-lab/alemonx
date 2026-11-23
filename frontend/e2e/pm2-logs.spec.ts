import { expect, test, type Page } from '@playwright/test'

async function mountPM2Logs(page: Page) {
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
}

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
    const stderrOnly = url.searchParams.get('source') === 'err'
    const lineCount = stderrOnly ? 0 : query ? 1 : 120
    const pageNumber = Number(url.searchParams.get('page') || 1)
    const snapshot = {
      lines: Array.from({ length: lineCount }, (_, index) => ({
        source: 'out',
        text: query ? `[ERROR] matched ${query}` : `page ${pageNumber} record ${index}`
      })),
      page: pageNumber,
      perPage: 120,
      hasOlder: !query,
      hasNewer: pageNumber > 1,
      total: stderrOnly ? 0 : query ? 1 : 100000,
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
  await mountPM2Logs(page)
  const output = page.getByRole('region', { name: 'PM2 日志内容' })
  await expect(output).toContainText('page 1 record 119')
  await expect(output).not.toContainText('record 120')
  await page.getByRole('textbox', { name: '搜索 PM2 日志' }).fill('needle')
  await expect(output).toHaveText('OUT[ERROR] matched needle')
  await page.getByRole('button', { name: '标准错误', exact: true }).click()
  await expect(output).toContainText('标准输出也可能包含错误日志')
  await page.getByRole('button', { name: '全部', exact: true }).click()
  await expect(output).toHaveText('OUT[ERROR] matched needle')
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

test('PM2 history searches undated logs and clears live results while switching dates', async ({
  page
}) => {
  let releaseDatedHistory: (() => void) | undefined
  const waitForRelease = new Promise<void>(resolve => {
    releaseDatedHistory = resolve
  })
  await page.route('**/api/v1/robot/pm2-logs**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/days')) {
      return route.fulfill({
        json: { days: [{ date: '2026-11-22', count: 1, out: 0, err: 1 }] }
      })
    }
    const dated = url.searchParams.has('date')
    const snapshot = {
      lines: [{ source: 'err', text: dated ? 'archived error' : 'undated error' }],
      page: 1,
      total: 1,
      undatedExcluded: dated ? 1 : 0
    }
    if (url.pathname.endsWith('/stream')) {
      return route.fulfill({
        contentType: 'text/event-stream',
        body: `data: ${JSON.stringify({ snapshot })}\n\n`
      })
    }
    if (dated) await waitForRelease
    return route.fulfill({ json: snapshot })
  })
  await mountPM2Logs(page)
  const output = page.getByRole('region', { name: 'PM2 日志内容' })
  await expect(output).toContainText('undated error')
  const allHistory = page.getByRole('button', { name: '历史 · 全部日期' })
  const historyRequest = page.waitForRequest(request => {
    const url = new URL(request.url())
    return url.pathname.endsWith('/pm2-logs') && !url.searchParams.has('date')
  })
  await allHistory.click()
  await historyRequest
  await expect(output).toContainText('undated error')
  await page.getByRole('combobox', { name: '按日期查看 PM2 日志' }).selectOption('2026-11-22')
  try {
    await expect(output).toContainText('正在读取 PM2 日志')
    await expect(output).not.toContainText('undated error')
  } finally {
    releaseDatedHistory?.()
  }
  await expect(output).toContainText('archived error')
  await expect(page.getByText('1 行没有可识别的时间戳', { exact: false })).toBeVisible()
  await allHistory.click()
  await expect(output).toContainText('undated error')
  await expect(output).not.toContainText('archived error')
})

test('PM2 keeps the selection on a retained log line when old records rotate out', async ({
  page
}) => {
  await page.addInitScript(() => {
    class FixtureEventSource {
      onmessage?: (message: MessageEvent) => void

      constructor() {
        window.addEventListener('fixture:pm2-event', this.receive)
      }

      private receive = (event: Event) => {
        this.onmessage?.(
          new MessageEvent('message', {
            data: JSON.stringify((event as CustomEvent).detail)
          })
        )
      }

      close() {
        window.removeEventListener('fixture:pm2-event', this.receive)
      }
    }
    Object.defineProperty(window, 'EventSource', { value: FixtureEventSource })
  })
  await page.route('**/api/v1/robot/pm2-logs/days?**', route =>
    route.fulfill({ json: { days: [] } })
  )
  await mountPM2Logs(page)
  const output = page.getByRole('region', { name: 'PM2 日志内容' })
  await expect(output).toHaveAttribute('aria-busy', 'true')
  await page.evaluate(() => {
    window.dispatchEvent(
      new CustomEvent('fixture:pm2-event', {
        detail: {
          snapshot: {
            lines: Array.from({ length: 2000 }, (_, index) => ({
              source: 'out',
              text: 'record ' + index
            })),
            page: 1,
            total: 2000
          }
        }
      })
    )
  })
  await expect(output.getByText('record 1999', { exact: true })).toBeVisible()
  const retainedLine = output.getByText('record 1990', { exact: true })
  await retainedLine.evaluate(node => {
    window.getSelection()?.selectAllChildren(node)
  })
  await page.evaluate(() => {
    window.dispatchEvent(
      new CustomEvent('fixture:pm2-event', {
        detail: { source: 'err', text: 'next live record' }
      })
    )
  })
  await expect(
    output.getByText('next live record', { exact: true })
  ).toBeVisible()
  await expect(output.getByText('record 0', { exact: true })).toHaveCount(0)
  await expect(page.getByText('显示 2000 行', { exact: true })).toBeVisible()
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(
    'record 1990'
  )
})
