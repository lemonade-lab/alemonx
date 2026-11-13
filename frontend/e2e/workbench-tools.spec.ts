import { expect, test } from '@playwright/test'

test('operation menu stays collapsed at all widths and opens nested tools', async ({
  page
}) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.route('**/workbench-tools-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div style="container: workbench-window / inline-size; width:100%"><header id="root" style="display:flex;flex-wrap:wrap"></header></div><script type="module">
      import '/src/styles.css';
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const { default: React } = await import('/node_modules/.vite/deps/react.js');
      const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
      const { Provider } = await import('/node_modules/.vite/deps/react-redux.js');
      const { store } = await import('/src/store/guideStore.ts');
      const { WorkbenchTools } = await import('/src/components/WorkbenchTools.tsx');
      const { SSHControl } = await import('/src/components/SSHControl.tsx');
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store}, React.createElement(WorkbenchTools, null, React.createElement('input', {'aria-label':'工具输入'}), React.createElement(SSHControl, {menuItem:true}))));
    </script>`
    })
  )
  await page.route('**/api/v1/system/ssh', route =>
    route.fulfill({ json: { keys: [] } })
  )
  await page.goto('/workbench-tools-regression')
  const trigger = page.getByRole('button', { name: '操作', exact: true })
  const input = page.getByRole('textbox', { name: '工具输入' })
  await expect(trigger).toBeVisible()
  await expect(input).toBeHidden()
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(trigger).toBeVisible()
  await expect(input).toBeHidden()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await trigger.click()
  await input.fill('保留未完成输入')
  await input.press('Escape')
  await expect(trigger).toBeFocused()
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await expect(input).toBeHidden()
  await trigger.click()
  await expect(input).toHaveValue('保留未完成输入')
  const ssh = page.getByRole('button', { name: 'SSH 管理', exact: true })
  await expect(ssh.locator('svg.lucide-chevron-right')).toBeVisible()
  await ssh.click()
  const panel = page.getByRole('dialog', { name: 'SSH 管理' })
  await expect(panel).toBeVisible()
  const rect = await panel.boundingBox()
  expect(rect!.x).toBeGreaterThanOrEqual(0)
  expect(rect!.x + rect!.width).toBeLessThanOrEqual(390)
  expect(rect!.y + rect!.height).toBeLessThanOrEqual(844)
  await ssh.press('Escape')
  await expect(panel).toBeHidden()
  await expect(ssh).toBeFocused()
  await expect(input).toBeVisible()
  await ssh.press('Escape')
  await expect(input).toBeHidden()
  await expect(trigger).toBeFocused()
  await page.setViewportSize({ width: 1280, height: 800 })
  await expect(trigger).toBeVisible()
  await expect(input).toBeHidden()
  await trigger.click()
  await expect(input).toHaveValue('保留未完成输入')
  await ssh.click()
  await expect(panel).toBeVisible()
  const desktopRect = await panel.boundingBox()
  expect(desktopRect!.x).toBeGreaterThanOrEqual(0)
  expect(desktopRect!.x + desktopRect!.width).toBeLessThanOrEqual(1280)
  await page.mouse.click(10, 750)
  await expect(input).toBeHidden()
  await trigger.click()
  await expect(panel).toBeHidden()
})

test('settings navigation opens SSH and manages HTTP MCP with developer mode off', async ({
  page
}) => {
  let managed = false
  let networkMode = 'auto'
  let githubLogin = ''
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    switch (path) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false, authenticated: false } })
      case '/api/v1/goals':
        return route.fulfill({ json: [] })
      case '/api/v1/system/ssh':
        return route.fulfill({
          json: {
            keys: [{ name: 'id_ed25519.pub', value: 'ssh-ed25519 example' }]
          }
        })
      case '/api/v1/system/mcp':
        if (request.method() === 'POST') {
          const { action } = request.postDataJSON() as { action: string }
          if (action === 'token')
            return route.fulfill({ json: { token: 'test-token' } })
          managed = action === 'start'
        }
        return route.fulfill({
          json: { running: managed, managed, url: 'http://127.0.0.1:17391/mcp' }
        })
      case '/api/v1/github/auth/device':
        return route.fulfill({ json: {
          flowId: 'mock', userCode: 'ABCD-1234', verificationUri: 'https://github.com/login/device',
          expiresIn: 900, interval: 5
        } })
      case '/api/v1/github/auth/poll':
        githubLogin = 'test-user'
        return route.fulfill({ json: { status: 'ok', login: 'test-user' } })
      case '/api/v1/github/auth/status':
        return route.fulfill({ json: { loggedIn: Boolean(githubLogin), login: githubLogin } })
      case '/api/v1/system/network':
        if (request.method() === 'PUT') networkMode = request.postDataJSON().mode
        return route.fulfill({
          json: {
            version: 2, revision: 1, mode: networkMode,
            automatic: { groups: [] }, proxy: { url: 'http://127.0.0.1:7890' }
          }
        })
      default:
        return route.fulfill({
          status: 503,
          json: { error: '测试中未启用其他服务' }
        })
    }
  })
  await page.goto('/dashboard/robot')
  const trigger = page.getByRole('button', { name: '操作', exact: true })
  const quickActions = page.getByRole('region', { name: '操作菜单' })
  await trigger.click()
  const initialTheme = await page.locator('html').getAttribute('data-theme')
  const theme = quickActions.getByRole('button', { name: /切换到(亮色|暗色)/ })
  await theme.click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', initialTheme === 'dark' ? 'light' : 'dark')
  await theme.click()
  const developer = quickActions.getByRole('button', { name: '开发模式' })
  await developer.click()
  await expect(developer).toHaveAttribute('aria-pressed', 'true')
  await developer.click()
  await expect(developer).toHaveAttribute('aria-pressed', 'false')
  await quickActions.getByRole('button', { name: '启动 MCP HTTP' }).click()
  await expect(quickActions.getByRole('button', { name: '停止 MCP HTTP' })).toBeEnabled()
  await quickActions.getByRole('button', { name: '停止 MCP HTTP' }).click()
  await page.context().route('https://github.com/login/device', route =>
    route.fulfill({ body: 'GitHub device fixture' })
  )
  const deviceURL = '**/api/v1/github/auth/device'
  await page.route(deviceURL, route =>
    route.fulfill({ status: 503, json: { error: 'GitHub 暂时不可用' } })
  )
  const menuHeight = (await quickActions.boundingBox())!.height
  await quickActions.getByRole('button', { name: '登录 GitHub' }).click()
  const errorIcon = quickActions.getByRole('button', { name: '查看 GitHub 登录错误' })
  await expect(errorIcon).toBeVisible()
  const errorDetails = quickActions.getByText('GitHub 暂时不可用', { exact: true })
  await expect(errorDetails).toBeHidden()
  await errorIcon.click()
  await expect(errorDetails).toBeVisible()
  expect((await quickActions.boundingBox())!.height).toBe(menuHeight)
  await errorIcon.press('Escape')
  await expect(errorDetails).toBeHidden()
  await page.unroute(deviceURL)
  const popup = page.waitForEvent('popup')
  await quickActions.getByRole('button', { name: '登录 GitHub' }).click()
  const authorization = await popup
  await expect(authorization).toHaveURL('https://github.com/login/device')
  await expect(quickActions.getByText('ABCD-1234')).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'GitHub 授权' })).toHaveCount(0)
  await authorization.close()
  await quickActions.getByRole('button', { name: /^网络/ }).click()
  const modes = quickActions.getByRole('group', { name: '网络模式' })
  for (const mode of ['直连', '代理', '自动']) {
    const option = modes.getByRole('button', { name: mode, exact: true })
    await option.click()
    await expect(option).toHaveAttribute('aria-pressed', 'true')
  }
  await expect(page.getByRole('tab', { name: '通用', exact: true })).toHaveCount(0)
  await trigger.click()
  await expect.poll(() => githubLogin, { timeout: 7000 }).toBe('test-user')
  await trigger.click()
  await expect(quickActions.getByRole('button', { name: 'test-user' })).toBeDisabled()
  await trigger.click()
  await page.getByRole('button', { name: '打开设置', exact: true }).click()
  const selectSetting = (name: string) =>
    page.getByRole('tab', { name, exact: true }).click()

  await selectSetting('通用')
  await expect(
    page.getByRole('checkbox', { name: /开发模式/ })
  ).not.toBeChecked()
  await selectSetting('SSH 管理')
  await expect(page.getByRole('heading', { name: 'SSH 管理' })).toBeVisible()
  await expect(page.getByRole('button', { name: '复制公钥' })).toBeVisible()
  await selectSetting('MCP')
  await expect(
    page.getByRole('heading', { name: 'MCP', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '启动 HTTP 服务' }).click()
  await expect(
    page.getByText('MCP HTTP 服务正在运行', { exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '显示连接令牌' }).click()
  await expect(
    page.getByRole('textbox', { name: '连接令牌', exact: true })
  ).toHaveValue('test-token')
  await page.getByRole('button', { name: '停止 HTTP 服务' }).click()
  await expect(
    page.getByText('MCP HTTP 服务已停止', { exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('textbox', { name: '连接令牌', exact: true })
  ).toBeHidden()
})

test('empty robot workspace opens the new robot wizard', async ({ page }) => {
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/checks')
      return route.fulfill({ json: { checks: [] } })
    if (path === '/api/v1/workspace/robots')
      return route.fulfill({
        json: {
          scope: 'local',
          local: true,
          initialized: false,
          projects: [],
          activeProjectID: ''
        }
      })
    if (path === '/api/v1/goals')
      return route.fulfill({
        json: [
          {
            id: 'install',
            title: '安装机器人',
            description: '用推荐默认配置创建机器人',
            steps: ['环境检查', '机器人名称与位置', '确认安装']
          }
        ]
      })
    return route.fulfill({ status: 503, json: { error: '测试中未启用此服务' } })
  })
  await page.goto('/dashboard/robot')
  await expect(
    page.getByRole('heading', { name: '开始管理你的机器人' })
  ).toBeVisible()
  await expect(page.getByRole('button', { name: /^(显示|隐藏)机器人导航$/ })).toHaveCount(0)
  await page.getByRole('button', { name: '立即全新创建' }).click()
  await expect(page).toHaveURL(/\/guide\/install\/step\/1$/)
  await expect(
    page.getByRole('heading', { name: '你的环境都准备好了吗' })
  ).toBeVisible()
})

test('saved robots reopen from the account and stale URLs cannot register robots', async ({
  page
}) => {
  const saved = '/test/robots/saved-robot'
  const stale = '/test/robots/previous-account'
  const writes: string[] = []
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({
        json: { enabled: true, authenticated: true, account: 'robot-owner' }
      })
    if (path === '/api/v1/goals') return route.fulfill({ json: [] })
    if (path === '/api/v1/checks')
      return route.fulfill({ json: { checks: [] } })
    if (path === '/api/v1/workspace/robots') {
      if (request.method() === 'POST')
        writes.push(request.postDataJSON().action)
      return route.fulfill({
        json: {
          scope: 'robot-owner',
          local: false,
          initialized: true,
          projects: [
            { id: saved, name: 'saved-robot', path: saved, pinned: true },
            { id: 'alpha', name: 'alpha', path: '/test/robots/team/alpha' },
            { id: 'beta', name: 'beta', path: '/test/robots/team/beta' }
          ],
          activeProjectID: saved
        }
      })
    }
    if (path === '/api/v1/robot/validate')
      return route.fulfill({ json: { valid: true } })
    return route.fulfill({ status: 503, json: { error: '测试中未启用此服务' } })
  })
  await page.goto(`/dashboard/robot?root=${encodeURIComponent(stale)}`)
  const savedRobot = page
    .locator('.project-rail')
    .getByText('saved-robot', { exact: true })
  await expect(savedRobot).toBeVisible()
  const menu = page.getByRole('complementary', { name: '目录操作' })
  await expect(menu).toBeVisible()
  await page.getByRole('button', { name: '隐藏机器人导航' }).click()
  await expect(menu).toHaveCount(0)
  await page.getByRole('button', { name: '显示机器人导航' }).click()
  await expect(menu).toBeVisible()
  const list = page.getByRole('navigation', { name: '机器人列表' })
  await expect(list.getByRole('region', { name: '已置顶机器人' })).toContainText('saved-robot')
  const folder = list.getByRole('button', { name: '目录 /test/robots/team' })
  const alpha = list.getByRole('button', { name: 'alpha', exact: true })
  await expect(alpha).toBeVisible()
  await folder.click()
  await expect(folder).toHaveAttribute('aria-expanded', 'false')
  await expect(alpha).toBeHidden()
  await expect(savedRobot).toBeVisible()
  await folder.click()
  await expect(alpha).toBeVisible()
  await expect
    .poll(() => new URL(page.url()).searchParams.get('root'))
    .toBe(saved)
  await page.reload()
  await expect(savedRobot).toBeVisible()
  expect(writes).toEqual([])

  const git = page.getByRole('button', { name: '管理 saved-robot 的 Git' })
  const gitWindow = page.getByRole('dialog', { name: 'saved-robot · Git' })
  await git.evaluate(button => {
    for (let index = 0; index < 10; index++) (button as HTMLButtonElement).click()
  })
  await expect(gitWindow).toHaveCount(1)
  await gitWindow.getByRole('button', { name: '最小化saved-robot · Git' }).click()
  await expect(gitWindow).toBeHidden()
  await git.click()
  await expect(gitWindow).toBeVisible()
  await expect(gitWindow).toHaveCount(1)
  await gitWindow.getByRole('button', { name: '关闭saved-robot · Git' }).click()
  await expect(gitWindow).toHaveCount(0)
})
