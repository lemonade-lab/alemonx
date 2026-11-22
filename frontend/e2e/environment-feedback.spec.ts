import { expect, test } from '@playwright/test'

test('installation stays open while pending and supports retry after long errors', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 568 })
  let finishRequest!: () => void
  const pending = new Promise<void>(resolve => {
    finishRequest = resolve
  })
  let attempts = 0
  await page.route('**/api/v1/system/environment/install', async route => {
    attempts++
    if (attempts === 1) {
      await pending
      return route.fulfill({
        status: 500,
        json: { error: '模拟安装失败\n'.repeat(100) }
      })
    }
    return route.fulfill({ json: { output: '安装成功' } })
  })
  await page.route('**/environment-feedback-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      await import('/e2e/fixtures/environment-feedback.tsx');
    </script>`
    })
  )
  await page.goto('/environment-feedback-regression')
  const dialog = page.getByRole('dialog', { name: '环境修复' })
  await expect(page.getByRole('dialog')).toHaveCount(1)
  await page.getByRole('button', { name: '安装', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '关闭', exact: true })
  ).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeVisible()
  finishRequest()
  await expect(dialog.getByRole('alert')).toContainText('安装未完成')
  await page.getByText('查看错误详情', { exact: true }).click()
  const retry = page.getByRole('button', { name: '重试安装', exact: true })
  await expect(retry).toBeInViewport()
  await retry.click()
  await expect(dialog.getByRole('status')).toHaveText('安装完成')
  await page.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByText('窗口已关闭')).toBeVisible()
  await expect(
    page.getByRole('progressbar', { name: '未知进度' })
  ).not.toHaveAttribute('aria-valuenow')
  expect(attempts).toBe(2)
})

test('long extension steps scroll inside the guide with the continue button fixed', async ({ page }) => {
  const capabilities = Array.from({ length: 24 }, (_, index) => ({
    id: `extension-${index + 1}`,
    name: `扩展 ${index + 1}`,
    installMode: 'npm',
    packageName: `fixture-extension-${index + 1}`
  }))
  await page.route('**/api/v1/**', route => {
    switch (new URL(route.request().url()).pathname) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/goals':
        return route.fulfill({ json: [
          {
            id: 'develop', title: '开发机器人', description: '',
            steps: ['环境检查', '项目名称', '开发语言', '代码规范', '版本管理', '本地运行', '包管理器', '开发能力包', '图片开发', '样式方案', '开发技能', '确认创建']
          },
          { id: 'install', title: '安装机器人', description: '', steps: ['环境检查', '机器人名称与位置', '确认安装'] }
        ] })
      case '/api/v1/workspace/robots':
        return route.fulfill({ json: { scope: 'test', initialized: true, local: true, projects: [], activeProjectID: '' } })
      case '/api/v1/catalog/options':
        return route.fulfill({ json: { data: capabilities } })
      default:
        return route.fulfill({ status: 503, json: { error: '测试未启用此接口' } })
    }
  })

  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 600 })
    for (const path of ['/guide/develop/step/8', '/guide/install/step/2']) {
      await page.goto(path)
      if (path.includes('/install/')) await page.getByText('选择扩展包（可选）', { exact: true }).click()
      const scroll = page.locator('.wizard-scroll')
      const first = scroll.getByRole('button', { name: /^扩展 1 点击添加$/ })
      const last = scroll.getByRole('button', { name: /^扩展 24 / })
      const actions = page.locator('.wizard-actions')
      const next = actions.getByRole('button', { name: '继续', exact: true })
      await expect(actions).toHaveCSS('position', 'absolute')
      await expect(first).toBeInViewport()
      await expect(last).not.toBeInViewport()
      const footerBounds = await next.boundingBox()
      await expect(next).toBeInViewport()

      const bounds = (await scroll.boundingBox())!
      await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
      await page.mouse.wheel(0, 4000)
      await expect(last).toBeInViewport()
      await expect.poll(() => scroll.evaluate(node => node.scrollTop)).toBeGreaterThan(0)
      expect(await next.boundingBox()).toEqual(footerBounds)
      const lastBounds = (await last.boundingBox())!
      expect(lastBounds.y + lastBounds.height).toBeLessThanOrEqual(footerBounds!.y)
      expect(await page.evaluate(() => window.scrollY)).toBe(0)
      const selected = await last.getAttribute('aria-pressed')
      await last.click()
      await expect(last).toHaveAttribute('aria-pressed', selected === 'true' ? 'false' : 'true')
      await next.click()
      await expect(page).toHaveURL(path.replace(/\d+$/, step => String(Number(step) + 1)))
      await expect.poll(() => scroll.evaluate(node => node.scrollTop)).toBe(0)
    }
  }
})

test('clean web guide rechecks Node.js and Git after an upgrade', async ({
  page
}) => {
  let upgraded = false
  const checks: unknown[] = []
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    switch (new URL(request.url()).pathname) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/goals':
        return route.fulfill({
          json: [{ id: 'web', title: 'Web 工作台', description: '', steps: [] }]
        })
      case '/api/v1/releases':
        return route.fulfill({ json: [] })
      case '/api/v1/checks':
        checks.push(request.postDataJSON())
        return route.fulfill({
          json: {
            ready: true,
            platform: 'darwin/arm64',
            checks: [
              {
                id: 'node',
                name: 'Node.js',
                status: upgraded ? 'ready' : 'outdated',
                detail: upgraded ? 'v22.23.5' : 'v22.22.2'
              },
              {
                id: 'git',
                name: 'Git',
                status: 'ready',
                detail: 'git version 2.50.1'
              }
            ]
          }
        })
      case '/api/v1/system/environment/install':
        expect(request.postDataJSON()).toEqual({
          checkId: 'node',
          confirm: true
        })
        upgraded = true
        return route.fulfill({ json: { output: '安装成功' } })
      default:
        return route.fulfill({ json: {} })
    }
  })

  await page.goto('/guide/web/step/1')
  const actions = page.locator('.wizard-actions')
  const next = actions.getByRole('button', { name: '继续', exact: true })
  await expect(next).toBeDisabled()
  const initialBounds = await next.boundingBox()
  await page.getByRole('button', { name: /纯净版/ }).click()
  await expect(next).toBeEnabled()
  await next.click()
  await expect(page.getByText('git version 2.50.1')).toBeVisible()
  await expect(page.getByText('建议升级', { exact: true })).toBeVisible()
  await expect(next).toBeEnabled()
  expect(await next.boundingBox()).toEqual(initialBounds)
  await page.getByRole('button', { name: '升级 Node.js', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '升级', exact: true }).click()
  await dialog.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByText('v22.23.5', { exact: true })).toBeVisible()
  await expect(page.getByText('环境已就绪，可以继续。')).toBeVisible()
  expect(checks).toEqual([
    { goalId: 'web', variant: 'clean' },
    { goalId: 'web', variant: 'clean' }
  ])
})

test('Bun runtime installs independently of Node.js', async ({ page }) => {
  let installed = false
  let bunCheckFailed = false
  await page.setViewportSize({ width: 600, height: 700 })
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    switch (new URL(request.url()).pathname) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/goals':
        return route.fulfill({
          json: [
            {
              id: 'install',
              title: '安装机器人',
              description: '',
              steps: ['环境', '位置', '确认']
            }
          ]
        })
      case '/api/v1/catalog/options':
        return route.fulfill({ json: { data: [] } })
      case '/api/v1/checks': {
        const { goalId, variant } = request.postDataJSON()
        expect(goalId).toBe('install')
        if (variant === 'bun' && !bunCheckFailed) {
          bunCheckFailed = true
          return route.fulfill({
            status: 503,
            json: { error: '暂时无法检查环境' }
          })
        }
        const runtime =
          variant === 'bun'
            ? {
                id: 'bun',
                name: 'Bun',
                status: installed ? 'ready' : 'missing',
                detail: installed ? '1.4.4' : ''
              }
            : { id: 'node', name: 'Node.js', status: 'missing', detail: '' }
        return route.fulfill({
          json: {
            ready: runtime.status === 'ready',
            platform: 'darwin/arm64',
            checks: [
              runtime,
              {
                id: 'git',
                name: 'Git',
                status: 'ready',
                detail: 'git version 2.50.1'
              },
              {
                id: 'browser',
                name: '浏览器',
                status: 'missing',
                detail: '没有已安装的浏览器',
                optional: true
              }
            ]
          }
        })
      }
      case '/api/v1/system/environment/install':
        expect(request.postDataJSON()).toEqual({
          checkId: 'bun',
          confirm: true
        })
        installed = true
        return route.fulfill({ json: { output: '已安装 Bun 1.4.4' } })
      case '/api/v1/projects':
        expect(request.postDataJSON()).toMatchObject({
          runtime: 'bun',
          packageManager: 'bun'
        })
        return route.fulfill({
          json: { status: 'ready', path: '/bots/alemonb' }
        })
      default:
        return route.fulfill({ json: {} })
    }
  })

  await page.goto('/guide/install/step/1')
  const next = page.getByRole('button', { name: '继续', exact: true })
  const required = page.getByRole('region', { name: '必需工具' })
  await expect(required.getByText('Node.js', { exact: true })).toBeVisible()
  await expect(next).toBeDisabled()
  await page.getByRole('radio', { name: 'Bun', exact: true }).check()
  await expect(page.getByText('本次检查失败，请重新检查。')).toBeVisible()
  await expect(next).toBeDisabled()
  const recheck = page.getByRole('button', { name: '重新检查', exact: true })
  await expect(recheck).toHaveText('')
  await expect(recheck).toHaveAttribute('title', '重新检查')
  await expect(recheck.locator('svg')).toBeVisible()
  await recheck.click()
  await expect(required.getByText('未检测到', { exact: true })).toBeVisible()
  await expect(required.getByText('Node.js', { exact: true })).toHaveCount(0)
  await expect(next).toBeDisabled()
  await expect(page.getByText('没有已安装的浏览器')).toBeHidden()
  await page.getByText('可选工具', { exact: true }).click()
  await expect(page.getByText('没有已安装的浏览器')).toBeVisible()
  await required.getByRole('button', { name: '安装 Bun', exact: true }).click()
  const dialog = page.getByRole('dialog')
  await expect(
    dialog.getByRole('link', { name: 'Bun 官方安装说明' })
  ).toHaveAttribute('href', 'https://bun.com/docs/installation')
  await dialog.getByRole('button', { name: '安装', exact: true }).click()
  await dialog.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByText('1.4.4', { exact: true })).toBeVisible()
  await next.click()
  await next.click()
  await expect(page.getByText('运行时：Bun')).toBeVisible()
  await page.getByRole('button', { name: '确认安装', exact: true }).click()
  await expect(page.getByText('机器人已安装', { exact: true })).toBeVisible()
})

test('Windows Git install offers the official installer if automatic installation fails', async ({
  page
}) => {
  let attempts = 0
  let installed = false
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    switch (path) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/goals':
        return route.fulfill({
          json: [
            {
              id: 'install',
              title: '安装机器人',
              description: '',
              steps: ['环境', '位置', '确认']
            }
          ]
        })
      case '/api/v1/catalog/options':
        return route.fulfill({ json: { data: [] } })
      case '/api/v1/checks':
        return route.fulfill({
          json: {
            ready: installed,
            platform: 'windows/amd64',
            checks: [
              {
                id: 'node',
                name: 'Node.js',
                status: 'ready',
                detail: 'v22.23.5'
              },
              {
                id: 'git',
                name: 'Git',
                status: installed ? 'ready' : 'missing',
                detail: installed ? 'git version 2.56.1.windows.1' : '未检测到'
              }
            ]
          }
        })
      case '/api/v1/system/environment/install':
        expect(route.request().postDataJSON()).toEqual({
          checkId: 'git',
          confirm: true
        })
        attempts++
        if (attempts === 1)
          return route.fulfill({
            status: 400,
            json: { error: '下载 Git for Windows 失败' }
          })
        installed = true
        return route.fulfill({ json: { output: '已安装工作台托管的 Git' } })
      default:
        return route.fulfill({ json: {} })
    }
  })
  await page.goto('/guide/install/step/1')
  await page.getByRole('button', { name: '安装 Git', exact: true }).click()
  const dialog = page.getByRole('dialog')
  const download = dialog.getByRole('link', {
    name: '下载完整的 Git for Windows'
  })
  await expect(download).toHaveAttribute(
    'href',
    'https://git-scm.com/download/win'
  )
  await dialog.getByRole('button', { name: '安装', exact: true }).click()
  await expect(dialog.getByRole('alert')).toBeVisible()
  await expect(download).toBeVisible()
  await dialog.getByRole('button', { name: '重试安装', exact: true }).click()
  await dialog.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByText('git version 2.56.1.windows.1')).toBeVisible()
  await expect(
    page.getByRole('button', { name: '继续', exact: true })
  ).toBeEnabled()
})
