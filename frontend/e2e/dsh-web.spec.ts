import { expect, test, type Page } from '@playwright/test'

const root = 'C:\\机器人\\demo'
const secondRoot = '/mock/second-project'
const token = Buffer.from(root).toString('base64url')
const originToken = Buffer.from('http://127.0.0.1:4242').toString('base64url')

async function mount(page: Page, selectedRoot = root) {
  let activeRoot = selectedRoot
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.route('**/api/v1/**', route => {
    const { pathname } = new URL(route.request().url())
    if (pathname === '/api/v1/goals' || pathname === '/api/v1/robot/tasks')
      return route.fulfill({ json: [] })
    if (pathname === '/api/v1/checks')
      return route.fulfill({ json: { checks: [] } })
    if (pathname === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false } })
    if (pathname === '/api/v1/workspace/robots') {
      if (route.request().method() === 'POST') {
        const { action, id } = route.request().postDataJSON()
        if (action === 'select') activeRoot = id
      }
      return route.fulfill({
        json: {
          scope: 'test',
          local: true,
          initialized: true,
          activeProjectID: activeRoot,
          projects: selectedRoot
            ? [
                { id: root, path: root, name: 'demo' },
                { id: secondRoot, path: secondRoot, name: 'second-project' }
              ]
            : []
        }
      })
    }
    if (pathname === '/api/v1/robot/validate')
      return route.fulfill({ json: { valid: true } })
    if (pathname === '/api/v1/dsh/status')
      return route.fulfill({ json: { ready: true } })
    return route.fulfill({ json: {} })
  })
}

function launchButton(page: Page) {
  return page
    .getByRole('navigation', { name: '工作台工具' })
    .getByRole('button', { name: 'DSH', exact: true })
}

test('DSH opens the official Web UI in the workbench and offers robot folders additively', async ({
  page
}) => {
  await mount(page)
  let launches = 0
  await page.route(`**/api/v1/dsh/runtimes/${token}/web`, route => {
    expect(route.request().method()).toBe('POST')
    launches++
    return route.fulfill({
      json: { url: 'http://127.0.0.1:4242/?token=fixture' }
    })
  })
  await page.route('**/api/v1/terminal/sessions', route =>
    route.fulfill({
      json: { id: 'dsh-terminal', key: 'fixture', cwd: root, shell: '/bin/sh' }
    })
  )
  await page.route('**/api/v1/terminal/sessions/dsh-terminal/stream**', route =>
    route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' })
  )
  await page.route(
    `**/api/v1/browser/http/${originToken}/?token=fixture`,
    route =>
      route.fulfill({
        contentType: 'text/html',
        body: `<p>官方 DSH 工作区</p><script>
      window.addEventListener('message', event => {
        if (event.source === parent && event.data?.type === 'alx:dsh-robot-workspaces')
          document.body.dataset.robotRoots = JSON.stringify(event.data.roots)
      });
      parent.postMessage({type: 'alx:dsh-workspaces-ready'}, location.origin);
    </script>`
      })
  )
  await page.goto('/dashboard/robot')
  await expect(
    page.getByRole('button', { name: '使用 Agent 协助当前机器人' })
  ).toHaveCount(0)
  await launchButton(page).click()
  await expect(page.locator('.robot-workspace-list')).toBeHidden()
  const header = page.locator('.workbench-content-header')
  await expect(header).toHaveCSS('position', 'absolute')
  await expect(header.getByRole('button', { name: '打开终端' })).toBeVisible()
  const frame = page.frameLocator('iframe[title="DSH Web 工作区"]')
  await expect(frame.getByText('官方 DSH 工作区')).toBeVisible()
  const workspace = page.locator('.workbench-content-column')
  const iframe = page.getByTitle('DSH Web 工作区', { exact: true })
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 })
    const workspaceBounds = (await workspace.boundingBox())!
    expect(await iframe.boundingBox()).toEqual(workspaceBounds)
    expect((await header.boundingBox())!.width).toBeLessThan(
      workspaceBounds.width / 2
    )
  }
  const operations = header.getByRole('button', { name: '操作', exact: true })
  const menu = header.getByRole('region', { name: '操作菜单' })
  await expect(header.getByRole('button', { name: '打开终端' })).toBeHidden()
  await operations.click()
  await menu.getByRole('button', { name: '打开终端' }).click()
  await expect(menu).toBeHidden()
  const terminal = page.getByRole('complementary', { name: '终端面板' })
  await expect(
    terminal.getByRole('button', { name: 'demo', exact: true })
  ).toBeVisible()
  const floatingBounds = (await header.boundingBox())!
  expect((await terminal.boundingBox())!.y).toBe(
    floatingBounds.y + floatingBounds.height
  )
  expect(await iframe.boundingBox()).toEqual(await workspace.boundingBox())
  await operations.click()
  await menu.getByRole('button', { name: '收起终端面板' }).click()
  await expect(terminal).toBeHidden()
  await expect(frame.locator('body')).toHaveAttribute(
    'data-robot-roots',
    JSON.stringify([root, secondRoot])
  )
  await expect(page.getByRole('dialog', { name: '浏览器' })).toHaveCount(0)
  await page.getByRole('button', { name: '主页', exact: true }).click()
  await expect(frame.getByText('官方 DSH 工作区')).toBeHidden()
  await expect(header).toHaveCSS('position', 'relative')
  await launchButton(page).click()
  await expect(frame.getByText('官方 DSH 工作区')).toBeVisible()
  expect(launches).toBe(1)
})

test('the robot row opens its own directory in DSH', async ({ page }) => {
  await mount(page, secondRoot)
  const launches: string[] = []
  await page.route('**/api/v1/dsh/runtimes/*/web', route => {
    launches.push(new URL(route.request().url()).pathname)
    return route.fulfill({
      json: { url: 'http://127.0.0.1:4242/?token=fixture' }
    })
  })
  await page.route(
    `**/api/v1/browser/http/${originToken}/?token=fixture`,
    route =>
      route.fulfill({
        contentType: 'text/html',
        body: '<p>机器人 DSH 工作区</p>'
      })
  )

  await page.goto('/dashboard/robot')
  const robots = page.getByRole('navigation', { name: '机器人列表' })
  const open = robots.getByRole('button', {
    name: '在 DSH 中打开 demo',
    exact: true
  })
  const more = robots.getByRole('button', {
    name: 'demo 的更多操作',
    exact: true
  })
  await expect(
    robots.getByRole('button', { name: 'second-project', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  await expect(open).toBeVisible()
  const openBounds = (await open.boundingBox())!
  const moreBounds = (await more.boundingBox())!
  expect(openBounds.x + openBounds.width).toBeLessThanOrEqual(moreBounds.x)
  expect(openBounds.y + openBounds.height / 2).toBeCloseTo(
    moreBounds.y + moreBounds.height / 2,
    0
  )

  await open.click()
  await expect(launchButton(page)).toHaveAttribute('aria-current', 'page')
  await expect(
    page
      .frameLocator('iframe[title="DSH Web 工作区"]')
      .getByText('机器人 DSH 工作区')
  ).toBeVisible()
  await expect
    .poll(() => new URL(page.url()).searchParams.get('root'))
    .toBe(root)
  expect(launches).toEqual([`/api/v1/dsh/runtimes/${token}/web`])
})

test('DSH shows an installation prompt without launching when the runtime is unavailable', async ({
  page
}) => {
  await mount(page)
  await page.route('**/api/v1/dsh/status', route =>
    route.fulfill({ json: { ready: false, message: '尚未安装 DSH' } })
  )
  let launches = 0
  await page.route('**/api/v1/dsh/runtimes/**', route => {
    launches++
    return route.fulfill({ status: 503 })
  })
  await page.goto('/dashboard/robot')
  await launchButton(page).click()
  await expect(page.getByRole('alert')).toContainText('尚未安装 DSH')
  await expect(page.getByRole('button', { name: '重新检查' })).toBeVisible()
  expect(launches).toBe(0)
})

test('DSH startup failure can be retried in place', async ({ page }) => {
  await mount(page)
  let attempts = 0
  await page.route(`**/api/v1/dsh/runtimes/${token}/web`, route => {
    attempts++
    return route.fulfill(
      attempts === 1
        ? { status: 503, json: { error: 'DSH Web 版启动失败，请重试' } }
        : { json: { url: 'http://127.0.0.1:4242/?token=fixture' } }
    )
  })
  await page.route(
    `**/api/v1/browser/http/${originToken}/?token=fixture`,
    route =>
      route.fulfill({ contentType: 'text/html', body: '<p>DSH 已打开</p>' })
  )
  await page.goto('/dashboard/robot')
  await launchButton(page).click()
  await expect(page.getByRole('alert')).toContainText('DSH Web 版启动失败')
  await page.getByRole('button', { name: '重试打开 DSH' }).click()
  await expect(
    page.frameLocator('iframe[title="DSH Web 工作区"]').getByText('DSH 已打开')
  ).toBeVisible()
  expect(attempts).toBe(2)
})

test('DSH asks for a robot directory when none is selected', async ({
  page
}) => {
  await mount(page, '')
  await page.goto('/dashboard/robot')
  await launchButton(page).click()
  await expect(page.getByText('请先在主页选择机器人目录。')).toBeVisible()
})
