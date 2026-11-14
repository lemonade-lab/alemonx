import { expect, test } from '@playwright/test'

test('recent sites survive reopening the browser and omit token launches', async ({
  page
}) => {
  test.setTimeout(30_000)
  await page.addInitScript(() => {
    Object.defineProperty(window, 'webkit', {
      value: {
        messageHandlers: {
          external: {
            postMessage(message: string) {
              if (message === 'alx:window:close') {
                document.body.dataset.windowClosed = 'true'
              }
            }
          }
        }
      }
    })
  })
  const recentKey = 'persist:alemonx-browser-recent'
  let savedSites = ''
  await page.route('**/api/desktop/preferences', route => {
    if (route.request().method() === 'PUT') {
      const { key, value } = route.request().postDataJSON() as {
        key: string
        value: string
      }
      if (key === recentKey) savedSites = value
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ json: savedSites ? { [recentKey]: savedSites } : {} })
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.route('**/api/v1/browser/classify', route =>
    route.fulfill({ json: { scope: 'private' } })
  )
  await page.route('**/api/v1/browser/http/**', route =>
    route.fulfill({ contentType: 'text/html', body: '<title>测试服务</title>' })
  )

  const browserPath = '/desktop-window/browser'
  await page.goto(browserPath)
  const homeInput = page.getByRole('textbox', { name: '搜索或输入网址' })
  const recent = page.getByRole('region', { name: '近期打开' })
  const recentButtons = recent.getByRole('button')
  const closeTab = page.getByRole('button', { name: '关闭标签页', exact: true })

  const visit = async (address: string) => {
    await homeInput.fill(address)
    await homeInput.press('Enter')
    await expect(page.getByRole('textbox', { name: '网址或搜索内容' })).toBeVisible()
    await closeTab.click()
    await expect(homeInput).toBeVisible()
  }

  await expect(recent).toBeHidden()
  for (let port = 4101; port <= 4107; port++) {
    await visit(`http://127.0.0.1:${port}/admin?view=active`)
  }
  await visit('http://127.0.0.1:4103/other')
  await visit('http://127.0.0.1:4199/start?token=mock-launch-credential')
  const expected = [4103, 4107, 4106, 4105, 4104, 4102].map(
    port => `127.0.0.1:${port}`
  )
  await expect(recentButtons).toHaveText(expected)
  await expect(page.locator('body')).not.toHaveAttribute('data-window-closed', 'true')
  await expect.poll(() => {
    const { sites } = JSON.parse(savedSites || '{}') as { sites?: string }
    return sites && JSON.parse(sites)
  }).toEqual(expected.map(site => `http://${site}`))

  await page.route('**/browser-closed', route => route.fulfill({ body: '' }))
  await page.goto('/browser-closed')
  await page.evaluate(key => localStorage.removeItem(key), recentKey)
  await page.goto(browserPath)
  await expect(recentButtons).toHaveText(expected)
  await recent.getByRole('button', { name: '127.0.0.1:4102' }).click()
  const addressBar = page.getByRole('textbox', { name: '网址或搜索内容' })
  await expect(addressBar).toHaveValue('http://127.0.0.1:4102/')
  await addressBar.click()
  await expect(addressBar).toBeFocused()
  const frameBounds = await page.locator('iframe').evaluate(frame => {
    const { top, bottom } = frame.getBoundingClientRect()
    return { top, bottom }
  })
  const addressBottom = await addressBar.evaluate(input =>
    input.getBoundingClientRect().bottom
  )
  expect(frameBounds.top).toBeGreaterThanOrEqual(addressBottom)
  expect(frameBounds.bottom).toBe(await page.evaluate(() => window.innerHeight))
  await closeTab.click()
  await expect(recentButtons.first()).toHaveText('127.0.0.1:4102')
})
