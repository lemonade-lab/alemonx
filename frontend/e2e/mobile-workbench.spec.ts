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
        case '/api/v1/setup/plugins':
          return route.fulfill({
            json: viewport.width === 390 ? [{
              id: 'fixture', name: '测试插件', enabled: true,
              navigation: { label: '测试插件', icon: 'plug' }
            }] : []
          })
        case '/api/v1/robot/validate':
          return route.fulfill({ json: { valid: true } })
        case '/api/v1/directories':
          return route.fulfill({ json: {
            path: '/fixture', parent: '/', roots: ['/'], locations: [],
            directories: [{ name: '机器人目录', path: '/fixture/robot-directory' }]
          } })
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
    const content = page.locator('.robot-workspace-content > .console-page')
    const box = (await content.boundingBox())!
    expect(box.y).toBeLessThanOrEqual(64)
    expect(box.height).toBeGreaterThan(viewport.height - 80)
    expect(
      await page.locator('html').evaluate(node => node.scrollWidth)
    ).toBeLessThanOrEqual(viewport.width)
    const headerButtons = await page
      .locator('.workbench-content-header button:visible')
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
    const operations = page.getByRole('region', { name: '操作菜单' })
    const tools = page.getByRole('navigation', { name: '工作台工具' })
    await expect(operations).toBeVisible()
    await expect(operations.getByRole('button', { name: '打开终端', exact: true })).toHaveCount(0)
    await expect(operations.getByRole('button', { name: '打开仓库', exact: true })).toHaveCount(0)
    for (const name of ['主页', 'DSH', '资源管理器', '打开设置']) {
      await expect(operations.getByRole('button', { name, exact: true })).toHaveCount(0)
      await expect(tools.getByRole('button', { name, exact: true })).toBeInViewport()
    }
    await page.getByRole('button', { name: '开发模式', exact: true }).click()
    await page
      .getByRole('button', { name: '开发模式', exact: true })
      .press('Escape')
    await functions.click()
    const menu = page.getByRole('region', {
      name: '机器人功能菜单',
      exact: true
    })
    await expect(menu).toBeVisible()
    const menuRadius = await menu.evaluate(node => getComputedStyle(node).borderRadius)
    const rowHeight = (await menu.getByRole('button', { name: '运行', exact: true }).boundingBox())!.height
    const mainBounds = (await menu.boundingBox())!
    await menu.getByRole('button', { name: '配置', exact: true }).click()
    const submenu = menu.getByRole('group', { name: '配置子菜单' })
    await expect(submenu).toBeVisible()
    const anchor = (await functions.boundingBox())!
    const popup = (await menu.boundingBox())!
    expect(popup.height).toBe(mainBounds.height)
    const submenuBounds = (await submenu.boundingBox())!
    expect(submenuBounds.x).toBeGreaterThanOrEqual(0)
    expect(submenuBounds.x + submenuBounds.width).toBeLessThanOrEqual(viewport.width)
    expect(submenuBounds.y + submenuBounds.height).toBeLessThanOrEqual(viewport.height)
    expect(popup.y).toBeGreaterThanOrEqual(anchor.y + anchor.height)
    expect(popup.y + popup.height).toBeLessThanOrEqual(viewport.height)
    await page.screenshot({ path: `test-results/mobile-navigation-${viewport.width}.png` })
    await submenu.getByRole('button').first().press('Escape')
    await expect(submenu).toBeHidden()
    await expect(menu).toBeVisible()
    await menu.getByRole('button', { name: '配置', exact: true }).click()
    await submenu.getByRole('button').first().click()
    await expect(menu).toBeHidden()
    await content.evaluate(node => { node.scrollTop = 0 })
    await expect(functions).toBeFocused()

    const projects = page.getByRole('button', { name: '打开机器人导航' })
    await projects.click()
    const list = page.getByRole('dialog', { name: '机器人导航' })
    const robotPanel = list.getByRole('complementary')
    const robot = robotPanel.getByText('phone-bot', { exact: true })
    await expect(list.getByRole('tablist')).toHaveCount(0)
    await expect(robot).toBeVisible()
    await expect(robotPanel.getByRole('button', { name: '添加本地机器人目录' })).toBeVisible()
    await expect(robotPanel.getByRole('button', { name: '测试插件' })).toHaveCount(0)
    await expect(robotPanel).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    const more = robotPanel.getByRole('button', { name: 'phone-bot 的更多操作' })
    await expect(more).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(more).toHaveCSS('border-width', '0px')
    await more.click()
    await expect(more).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    const projectMenu = robotPanel.getByRole('menu', { name: 'phone-bot 的更多操作' })
    await expect(projectMenu).toHaveCSS('border-radius', menuRadius)
    const pin = projectMenu.getByRole('button', { name: '置顶', exact: true })
    await pin.click({ trial: true })
    expect((await pin.boundingBox())!.height).toBe(rowHeight)
    const remove = projectMenu.getByRole('button', { name: '移除' })
    await remove.click({ trial: true })
    await remove.press('Escape')
    await expect(projectMenu).toBeHidden()
    await expect(more).toBeFocused()
    if (viewport.width === 390) {
      await robotPanel.getByRole('button', { name: '添加本地机器人目录' }).click()
      const picker = page.locator('.directory-picker')
      await picker.getByText('机器人目录', { exact: true }).click({ button: 'right' })
      const directoryMenu = page.getByRole('menu', { name: '目录操作' })
      await expect(directoryMenu).toHaveCSS('border-radius', menuRadius)
      await directoryMenu.getByRole('button', { name: '删除文件夹' }).click({ trial: true })
      await page.keyboard.press('Escape')
      await expect(directoryMenu).toBeHidden()
      await expect(picker).toBeVisible()
      await picker.getByRole('button', { name: '取消', exact: true }).click()
      await projects.click()
    }
    await robotPanel.getByText('phone-bot', { exact: true }).click()
    await expect(list).toBeHidden()
    await expect(projects).toBeFocused()
    const plugins = tools.getByRole('group', { name: '已启动插件' })
    if (viewport.width === 390) {
      await expect(plugins.getByRole('button', { name: '测试插件' })).toBeInViewport()
      await expect(plugins.getByRole('button', { name: '更多插件' })).toHaveCount(0)
    } else {
      await expect(plugins).toHaveCount(0)
    }
    const systemTools = tools.getByRole('group', { name: '系统功能' })
    await expect(systemTools.getByRole('button')).toHaveCount(6)
    const environment = systemTools.getByRole('button', { name: '全局环境', exact: true })
    await environment.scrollIntoViewIfNeeded()
    await expect(environment).toBeInViewport()
    await expect(tools.getByRole('button', { name: '打开设置', exact: true })).toBeInViewport()

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
    await functions.press('Escape')
    await expect(menu).toBeHidden()
    await expect(functions).toBeFocused()
    await page.screenshot({
      path: `test-results/mobile-${viewport.width}.png`,
      fullPage: true
    })
  })
}
