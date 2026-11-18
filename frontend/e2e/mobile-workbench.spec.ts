import { expect, test } from '@playwright/test'

test.use({ hasTouch: true, isMobile: true })

for (const viewport of [
  { width: 320, height: 568 },
  { width: 390, height: 844 },
  { width: 667, height: 375 },
  { width: 844, height: 390 }
]) {
  test(`phone navigation leaves room for content at ${viewport.width}x${viewport.height}`, async ({
    page
  }) => {
    await page.setViewportSize(viewport)
    await page.emulateMedia({
      colorScheme: viewport.width === 390 ? 'dark' : 'light'
    })
    await page.route('**/api/v1/**', route => {
      const path = new URL(route.request().url()).pathname
      switch (path) {
        case '/api/v1/auth/status':
          return route.fulfill({
            json: { enabled: false, authenticated: false }
          })
        case '/api/v1/workspace/robots':
          return route.fulfill({
            json: {
              scope: 'test',
              local: true,
              initialized: true,
              projects: [
                {
                  id: 'phone-bot',
                  name: '手机测试机器人',
                  path: '/fixture/phone-bot'
                }
              ],
              activeProjectID: 'phone-bot'
            }
          })
        case '/api/v1/goals':
        case '/api/v1/robot/tasks':
        case '/api/v1/robot/webviews':
          return route.fulfill({ json: [] })
        case '/api/v1/robot/validate':
          return route.fulfill({ json: { valid: true } })
        case '/api/v1/robot/pm2-processes':
          return route.fulfill({
            json: {
              items: [
                {
                  id: 7,
                  name: 'phone-bot',
                  status: 'online',
                  pid: 123,
                  memory: 39000000,
                  cpu: 0,
                  restarts: 0
                }
              ]
            }
          })
        case '/api/v1/robot/pm2-processes/stream':
          return route.fulfill({
            contentType: 'text/event-stream',
            body: ': connected\n\n'
          })
        case '/api/v1/robot/runtime':
          return route.fulfill({
            json: {
              name: '手机测试机器人',
              version: '1.0.0',
              packageManager: 'npm',
              hasAppScript: false,
              hasDevScript: false,
              hasBuildScript: false,
              hasStartScript: false,
              pm2Configured: false,
              dependenciesComplete: true,
              platforms: []
            }
          })
        default:
          return route.fulfill({
            json: { items: [], keys: [], checks: [], platforms: [] }
          })
      }
    })
    await page.goto('/dashboard/robot')
    const functions = page.getByRole('button', { name: '打开机器人功能导航' })
    await expect(functions).toBeVisible()
    await expect(
      page.getByRole('navigation', { name: '机器人功能' })
    ).toBeHidden()
    const content = page.locator('.robot-workspace > .console-page')
    const box = (await content.boundingBox())!
    expect(box.y).toBeLessThanOrEqual(64)
    expect(box.height).toBeGreaterThan(viewport.height - 80)
    expect(
      await page.locator('html').evaluate(node => node.scrollWidth)
    ).toBeLessThanOrEqual(viewport.width)
    const headerButtons = await page
      .locator('.dashboard-window > .topbar button:visible')
      .evaluateAll(buttons =>
        buttons.map(button => {
          const { x, y, width, height } = button.getBoundingClientRect()
          return { x, y, width, height }
        })
      )
    for (const button of headerButtons) {
      expect(button.height).toBeGreaterThanOrEqual(44)
      expect(button.x + button.width).toBeLessThanOrEqual(viewport.width)
      expect(button.y).toBeLessThan(10)
    }

    await page.getByRole('button', { name: '操作', exact: true }).click()
    await page.getByRole('button', { name: '开发模式', exact: true }).click()
    await page
      .getByRole('button', { name: '开发模式', exact: true })
      .press('Escape')
    await functions.click()
    const drawer = page.getByRole('dialog', {
      name: '机器人功能导航',
      exact: true
    })
    await expect(drawer).toBeVisible()
    await drawer.getByRole('button', { name: '配置', exact: true }).click()
    const submenu = drawer.getByRole('group', { name: '配置子菜单' })
    await expect(submenu).toBeVisible()
    await page.screenshot({ path: `test-results/mobile-navigation-${viewport.width}.png` })
    await submenu.getByRole('button').first().press('Escape')
    await expect(submenu).toBeHidden()
    await expect(drawer).toBeVisible()
    await drawer.getByRole('button', { name: '配置', exact: true }).click()
    await submenu.getByRole('button').first().click()
    await expect(drawer).toBeHidden()
    await content.evaluate(node => { node.scrollTop = 0 })
    await expect(functions).toBeFocused()

    const projects = page.getByRole('button', { name: '打开机器人与系统导航' })
    await projects.click()
    const list = page.getByRole('dialog', { name: '机器人与系统导航' })
    await expect(list.getByText('phone-bot', { exact: true })).toBeVisible()
    await list.getByText('phone-bot', { exact: true }).click()
    await expect(list).toBeHidden()
    await expect(projects).toBeFocused()

    const restart = page.getByRole('button', {
      name: '重启 phone-bot',
      exact: true
    })
    await expect(restart).toHaveText('重启')
    await expect(
      page.getByRole('button', { name: '重载 phone-bot', exact: true })
    ).toHaveText('重载')
    await restart.click()
    const confirm = page.getByRole('dialog', {
      name: '重启 phone-bot',
      exact: true
    })
    await expect(confirm).toBeVisible()
    await confirm.getByRole('button', { name: '取消', exact: true }).click()

    await functions.click()
    await drawer.getByRole('button', { name: '关闭导航' }).click()
    await expect(drawer).toBeHidden()
    await page.screenshot({
      path: `test-results/mobile-${viewport.width}.png`,
      fullPage: true
    })
  })
}
