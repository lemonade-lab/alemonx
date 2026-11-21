import { expect, test } from '@playwright/test'
import {
  createForegroundLogNormalizer,
  normalizeForegroundLog
} from '../src/lib/foregroundLogInterpreter'

test('desktop checks auth during preference restore and defers the workbench until sign-in', async ({
  page
}) => {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: { messageHandlers: { external: { postMessage() {} } } }
    })
    localStorage.setItem('autoConnectEnabled', '1')
  })
  const authRequested = page.waitForRequest('**/api/v1/auth/status')
  const workbenchRequests: string[] = []
  let statusRequests = 0
  page.on('request', request => {
    const path = new URL(request.url()).pathname
    if (
      path === '/src/App.tsx' ||
      path === '/src/components/Dashboard.tsx' ||
      path === '/api/v1/workspace/robots'
    )
      workbenchRequests.push(path)
  })
  const migration = Promise.withResolvers<void>()
  const preferenceImport = page.waitForRequest(
    request =>
      request.url().endsWith('/api/desktop/preferences') &&
      request.method() === 'PUT' &&
      request.postDataJSON().import === true
  )
  await page.route('**/api/desktop/preferences', async route => {
    if (route.request().method() === 'PUT') {
      if (route.request().postDataJSON().import) await migration.promise
      await route.fulfill({ json: {} })
      return
    }
    // Restoring preferences and checking auth must be able to run concurrently.
    await authRequested
    await route.fulfill({ json: { 'alemonjs-theme': 'dark' } })
  })
  await page.route('**/api/v1/auth/status', route => {
    statusRequests += 1
    return route.fulfill({ json: { enabled: true, authenticated: false } })
  })
  try {
    await page.goto('/dashboard/robot')
    const importRequest = await preferenceImport
    await expect(
      page.getByRole('button', { name: '登录', exact: true })
    ).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    expect(statusRequests).toBe(1)
    expect(workbenchRequests).toEqual([])
    expect(importRequest.failure()).toBeNull()
  } finally {
    migration.resolve()
  }
})

test('an initial auth failure can be retried without reloading the page', async ({
  page
}) => {
  let statusRequests = 0
  await page.route('**/api/v1/auth/status', route => {
    statusRequests += 1
    return statusRequests === 1
      ? route.fulfill({ status: 503, json: { error: '服务暂不可用' } })
      : route.fulfill({ json: { enabled: true, authenticated: false } })
  })
  await page.goto('/dashboard/robot')
  await expect(page.getByText('服务暂不可用。', { exact: false })).toBeVisible()
  await page.getByRole('button', { name: '重试', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '登录', exact: true })
  ).toBeVisible()
  expect(statusRequests).toBe(2)
})

test('foreground log bursts retain order and switch cleanly to the new task', async ({
  page
}) => {
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    switch (path) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false, authenticated: false } })
      case '/api/v1/workspace/robots':
        return route.fulfill({
          json: {
            scope: 'fixture',
            local: true,
            initialized: true,
            projects: [{ id: 'bot', name: 'bot', path: '/fixture/bot' }],
            activeProjectID: 'bot'
          }
        })
      case '/api/v1/robot/runtime':
        return route.fulfill({
          json: { name: 'bot', platforms: [], dependenciesComplete: true }
        })
      case '/api/v1/robot/validate':
        return route.fulfill({ json: { valid: true } })
      case '/api/v1/robot/console':
        return route.fulfill({
          json: { sessionId: 'first', output: '[WARN] snapshot\n' }
        })
      case '/api/v1/events':
        return route.fulfill({ status: 204 })
      case '/api/v1/setup/plugins':
      case '/api/v1/robot/tasks':
      case '/api/v1/robot/webviews':
        return route.fulfill({ json: [] })
      default:
        return route.fulfill({ json: { items: [], checks: [], platforms: [] } })
    }
  })
  await page.goto('/dashboard/robot')
  await page.getByRole('button', { name: '日志', exact: true }).click()
  const logs = page.getByRole('dialog', { name: '日志', exact: true })
  await expect(logs.getByText(/snapshot/)).toBeVisible()
  await page.evaluate(() => {
    for (const text of ['first line', 'second line', 'third line']) {
      window.dispatchEvent(
        new CustomEvent('alx:robot-output', {
          detail: { taskId: 'first', text: `[WARN] ${text}\n` }
        })
      )
    }
  })
  await expect(logs.getByText(/(?:first|second|third) line/)).toHaveText([
    /first line/,
    /second line/,
    /third line/
  ])
  await page.evaluate(() => {
    for (const [taskId, text] of [
      ['first', 'previous task'],
      ['next', 'new task']
    ]) {
      window.dispatchEvent(
        new CustomEvent('alx:robot-output', {
          detail: { taskId, text: `[WARN] ${text}\n` }
        })
      )
    }
  })
  await expect(logs.getByText(/new task/)).toBeVisible()
  await expect(logs.getByText(/snapshot|previous task|third line/)).toHaveCount(
    0
  )
})

test('startup restores missing baseline APIs in older WebViews', async ({
  page
}) => {
  await page.addInitScript(() => Reflect.deleteProperty(Array.prototype, 'at'))
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: true, authenticated: false } })
  )
  await page.goto('/dashboard/robot')
  await expect(
    page.getByRole('button', { name: '登录', exact: true })
  ).toBeVisible()
  expect(await page.evaluate(() => 'at' in Array.prototype)).toBe(true)
})

test('authorized startup loads the active robot runtime while the workbench module is pending', async ({
  page
}) => {
  const runtimeRequest = page.waitForRequest('**/api/v1/robot/runtime?**')
  let reads = 0
  let runtimeReads = 0
  let moduleHeld = false
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({
        json: { enabled: true, authenticated: true, account: 'fixture' }
      })
    if (path === '/api/v1/workspace/robots') {
      reads += 1
      return route.fulfill({
        json: {
          scope: 'fixture',
          local: false,
          initialized: true,
          activeProjectID: 'boot',
          projects: [
            { id: 'boot', name: 'boot-bot', path: '/fixture/boot-bot' }
          ]
        }
      })
    }
    if (path === '/api/v1/robot/runtime') {
      runtimeReads += 1
      expect(new URL(route.request().url()).searchParams.get('root')).toBe(
        '/fixture/boot-bot'
      )
      return route.fulfill({
        json: {
          name: 'boot-bot',
          version: '1.2.3',
          packageManager: 'npm',
          platforms: []
        }
      })
    }
    if (path === '/api/v1/robot/validate')
      return route.fulfill({ json: { valid: true } })
    if (path === '/api/v1/events') return route.fulfill({ status: 204 })
    if (path === '/api/v1/setup/plugins' || path === '/api/v1/robot/tasks')
      return route.fulfill({ json: [] })
    return route.fulfill({ json: { items: [], checks: [], platforms: [] } })
  })
  await page.route('**/src/App.tsx', async route => {
    moduleHeld = true
    await runtimeRequest
    await route.continue()
  })

  await page.goto('/dashboard/robot')
  await expect(
    page
      .getByRole('navigation', { name: '机器人列表' })
      .getByRole('button', { name: 'boot-bot', exact: true })
  ).toBeVisible()
  await expect(page.getByText(/1\.2\.3/).first()).toBeVisible()
  expect(moduleHeld).toBe(true)
  expect(runtimeReads).toBe(1)
  expect(reads).toBe(1)
})

test('foreground normalization reuses blocks and reparses continuations and truncated history', () => {
  const normalize = createForegroundLogNormalizer()
  const before = '[WARN] first entry\n[WARN] partial'
  const first = normalize(before)
  const continued = before + ' entry\n stack detail\n[WARN] next'
  const second = normalize(continued)
  expect(second).toEqual(normalizeForegroundLog(continued))
  expect(second[0]).toBe(first[0])
  expect(second[1]).not.toBe(first[1])

  const repeated =
    continued +
    '\n' +
    Array.from({ length: 600 }, (_, i) => '[WARN] message ' + (i % 20)).join(
      '\n'
    )
  expect(normalize(repeated)).toEqual(normalizeForegroundLog(repeated))
})

test('closing a large robot directory removes its rows and keeps pinned entries available', async ({
  page
}) => {
  const projects = Array.from({ length: 240 }, (_, index) => ({
    id: String(index),
    name: `bulk-robot-${index}`,
    path: `/fixture/group/bulk-robot-${index}`
  }))
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/workspace/robots')
      return route.fulfill({
        json: {
          scope: 'fixture',
          local: true,
          initialized: true,
          activeProjectID: '',
          projects: [
            ...projects,
            {
              id: 'pinned',
              name: 'pinned-robot',
              path: '/fixture/pinned-robot',
              pinned: true
            }
          ]
        }
      })
    if (path === '/api/v1/events') return route.fulfill({ status: 204 })
    if (path === '/api/v1/setup/plugins' || path === '/api/v1/robot/tasks')
      return route.fulfill({ json: [] })
    return route.fulfill({ json: { items: [], checks: [], platforms: [] } })
  })
  await page.goto('/dashboard/robot')
  const list = page.getByRole('navigation', { name: '机器人列表' })
  const folder = list.getByRole('button', { name: '目录 /fixture/group' })
  const rows = list.getByText(/^bulk-robot-\d+$/, { exact: true })
  await expect(rows).toHaveCount(240)
  await folder.press('Space')
  await expect(folder).toHaveAttribute('aria-expanded', 'false')
  await expect(rows).toHaveCount(0)
  await expect(list.getByText('pinned-robot', { exact: true })).toBeVisible()
  await folder.press('Space')
  await expect(rows).toHaveCount(240)
})
