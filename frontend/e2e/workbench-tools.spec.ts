import { expect, test } from '@playwright/test'

test('plugin center opens as a movable window with sidebar navigation', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/setup/plugins' || path === '/api/v1/setup/plugins/market')
      return route.fulfill({ json: [] })
    return route.fulfill({ status: 503, json: {} })
  })
  await page.goto('/dashboard/robot')
  const tools = page.getByRole('navigation', { name: '工作台工具' })
  const trigger = tools.getByRole('button', { name: '插件', exact: true })
  await trigger.click()
  const plugins = page.getByRole('dialog', { name: '插件', exact: true })
  const sidebar = plugins.getByRole('complementary', { name: '系统功能' })
  const mine = sidebar.getByRole('button', { name: '我的', exact: true })
  const market = sidebar.getByRole('button', { name: '市场', exact: true })
  const close = plugins.getByRole('button', { name: '关闭插件', exact: true })
  const minimize = plugins.getByRole('button', { name: '最小化插件', exact: true })
  const empty = plugins.getByText('还没有已下载的插件')
  await expect(mine).toHaveAttribute('aria-current', 'page')
  await expect(empty).toBeVisible()
  expect((await mine.boundingBox())!.x).toBeLessThan((await empty.boundingBox())!.x)

  const header = plugins.locator('header').first()
  for (const [theme, raised] of [['light', 'rgb(242, 241, 238)'], ['dark', 'rgb(35, 33, 31)']]) {
    await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
    await expect(header).toHaveCSS('background-color', raised)
  }
  await page.evaluate(() => { document.documentElement.dataset.theme = 'light' })

  const before = (await plugins.boundingBox())!
  await page.mouse.move(before.x + before.width / 2, before.y + 20)
  await page.mouse.down()
  await page.mouse.move(before.x + before.width / 2 + 30, before.y, { steps: 4 })
  await page.mouse.up()
  await expect.poll(async () => (await plugins.boundingBox())!.x).toBe(before.x + 30)

  const marketRequest = page.waitForRequest('**/api/v1/setup/plugins/market')
  await market.click()
  await marketRequest
  await expect(market).toHaveAttribute('aria-current', 'page')

  await minimize.click()
  await expect(plugins).toBeHidden()
  await trigger.click()
  await expect(plugins).toBeVisible()
  await expect(market).toHaveAttribute('aria-current', 'page')

  await page.setViewportSize({ width: 390, height: 610 })
  await expect(close).toBeInViewport({ ratio: 1 })
  await expect(market).toBeInViewport({ ratio: 1 })
  await close.click()
  await expect(plugins).toHaveCount(0)
})

test('desktop tool rail opens independent native windows', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'platform', { value: 'MacIntel' })
    Object.assign(window, {
      webkit: {
        messageHandlers: {
          external: {
            postMessage: (message: string) => {
              document.documentElement.dataset.lastNativeMessage = message
            }
          }
        }
      }
    })
  })
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/setup/plugins')
      return route.fulfill({ json: [] })
    return route.fulfill({ status: 503, json: {} })
  })
  await page.goto('/dashboard/robot')
  const tools = page.getByRole('navigation', { name: '工作台工具' })
  for (const [label, destination] of [
    ['插件', 'plugins:'],
    ['浏览', 'browser:'],
    ['数据', 'data:'],
    ['运维', 'ops-overview:'],
    ['任务', 'tasks:root='],
    ['全局环境', 'environment:root='],
    ['打开设置', 'settings:']
  ]) {
    await tools.getByRole('button', { name: label, exact: true }).click()
    await expect(page.locator('html')).toHaveAttribute('data-last-native-message', `alx:open-window:${destination}`)
    await expect(page.locator('[data-window-container="desktop"]')).toHaveCount(0)
  }

  await page.goto('/desktop-window/plugins')
  await expect(page.locator('.native-workbench-header:visible')).toHaveText('插件')
  await expect(page.getByRole('button', { name: '我的', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByText('还没有已下载的插件')).toBeVisible()
})

test('the fixed tool rail follows the active plugin registry in resource mode', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: {
        messageHandlers: {
          external: {
            postMessage: (message: string) => {
              document.documentElement.dataset.lastNativeMessage = message
            }
          }
        }
      }
    })
  })
  let running = false
  let additionalPlugins = 0
  const plugin = {
    id: 'fixture', name: '源码插件', enabled: true, developmentSource: true,
    navigation: { label: '测试插件入口', icon: 'plug' }
  }
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/workspace/robots')
      return route.fulfill({ json: { scope: 'test', local: true, initialized: true, projects: [], activeProjectID: '' } })
    if (path === '/api/v1/setup/plugins')
      return route.fulfill({ json: [
        { ...plugin, id: 'disabled', name: '停用插件', navigation: {}, enabled: false },
        { ...plugin, id: 'online', name: '在线目录插件', navigation: {}, online: true },
        ...(running ? [plugin, ...Array.from({ length: additionalPlugins }, (_, index) => ({
          ...plugin,
          id: `extra-${index + 1}`,
          navigation: { ...plugin.navigation, label: `扩展 ${index + 1}` }
        }))] : [])
      ] })
    return route.fulfill({ status: 503, json: {} })
  })
  await page.goto('/dashboard/robot')
  const tools = page.getByRole('navigation', { name: '工作台工具' })
  await tools.getByRole('button', { name: '资源管理器' }).click()
  await expect(page.getByRole('region', { name: '资源管理器' })).toBeVisible()
  const pluginTools = tools.getByRole('group', { name: '已启动插件' })
  await expect(pluginTools).toHaveCount(0)
  const systemTools = tools.getByRole('group', { name: '系统功能' })
  const systemPositions = () => systemTools.getByRole('button').evaluateAll(buttons =>
    buttons.map(button => button.getBoundingClientRect().top)
  )
  const initialSystemPositions = await systemPositions()

  const notifyPluginChange = () => page.evaluate(() => {
    const channel = new BroadcastChannel('alx-events')
    channel.postMessage({ type: 'event', envelope: { topic: 'plugins', type: 'plugins.changed' } })
    channel.close()
  })
  running = true
  await notifyPluginChange()
  const entry = pluginTools.getByRole('button', { name: '测试插件入口' })
  await expect(pluginTools.getByRole('button')).toHaveCount(1)
  await expect(entry).toBeInViewport()
  expect(await systemPositions()).toEqual(initialSystemPositions)
  const systemBounds = (await systemTools.boundingBox())!
  expect((await entry.boundingBox())!.y).toBeGreaterThanOrEqual(systemBounds.y + systemBounds.height)
  await entry.click()
  await expect(page.locator('html')).toHaveAttribute('data-last-native-message', 'alx:open-window:plugin:id=fixture')

  additionalPlugins = 8
  await notifyPluginChange()
  const morePlugins = pluginTools.getByRole('button', { name: '更多插件', exact: true })
  await expect(pluginTools.getByRole('button')).toHaveCount(9)
  await expect(morePlugins).toHaveCount(0)

  await page.setViewportSize({ width: 900, height: 570 })
  await expect(morePlugins).toBeInViewport()
  const compactSystemBounds = (await systemTools.boundingBox())!
  expect((await morePlugins.boundingBox())!.y).toBeGreaterThanOrEqual(compactSystemBounds.y + compactSystemBounds.height)
  await expect(systemTools.getByRole('button', { name: '全局环境' })).toBeInViewport({ ratio: 1 })
  await expect(tools.getByRole('button', { name: '打开设置' })).toBeInViewport({ ratio: 1 })
  await expect(entry).toBeVisible()
  const lastPlugin = pluginTools.getByRole('button', { name: '扩展 8', exact: true })
  await expect(lastPlugin).toBeHidden()
  await morePlugins.click()
  const menu = pluginTools.getByRole('region', { name: '更多插件列表' })
  await expect(menu.getByRole('button', { name: '扩展 8', exact: true })).toBeVisible()
  await morePlugins.press('Tab')
  await expect(menu.getByRole('button').first()).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await expect(morePlugins).toBeFocused()
  await morePlugins.click()
  await lastPlugin.click()
  await expect(menu).toBeHidden()
  await expect(page.locator('html')).toHaveAttribute('data-last-native-message', 'alx:open-window:plugin:id=extra-8')

  additionalPlugins = 24
  await notifyPluginChange()
  await morePlugins.click()
  const finalEntry = menu.getByRole('button', { name: '扩展 24', exact: true })
  await expect(finalEntry).toBeAttached()
  await expect(finalEntry).not.toBeInViewport()
  await finalEntry.click()
  await expect(page.locator('html')).toHaveAttribute('data-last-native-message', 'alx:open-window:plugin:id=extra-24')

  additionalPlugins = 8
  await notifyPluginChange()
  await page.setViewportSize({ width: 1440, height: 900 })
  await expect(morePlugins).toHaveCount(0)
  await expect(lastPlugin).toBeInViewport()

  running = false
  await notifyPluginChange()
  await expect(pluginTools).toHaveCount(0)
  expect(await systemPositions()).toEqual(initialSystemPositions)
})

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
  let hasToken = false
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
          expect(request.postDataJSON()).toEqual({ action })
          if (action === 'token') {
            hasToken = true
            return route.fulfill({ json: { token: 'test-token' } })
          }
          if (action === 'rotate')
            return route.fulfill({ json: { token: 'rotated-token' } })
          if (action === 'revoke') {
            hasToken = false
            return route.fulfill({ status: 204 })
          }
          managed = action === 'start'
        }
        return route.fulfill({
          json: { canControl: true, canIssueToken: true, hasToken, autoStart: managed, running: managed, managed, url: 'http://127.0.0.1:17391/mcp' }
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
  await expect(page.getByText('密钥类型', { exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '创建连接密钥' }).click()
  await expect(
    page.getByRole('textbox', { name: '连接密钥', exact: true })
  ).toHaveValue('test-token')
  await page.getByRole('button', { name: '停止 HTTP 服务' }).click()
  await expect(
    page.getByText('MCP HTTP 服务已停止', { exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '重新生成密钥', exact: true }).click()
  await page.getByRole('button', { name: '确认重新生成', exact: true }).click()
  await expect(
    page.getByRole('textbox', { name: '连接密钥', exact: true })
  ).toHaveValue('rotated-token')
  await page.getByRole('button', { name: '撤销密钥', exact: true }).click()
  await page.getByRole('button', { name: '确认撤销', exact: true }).click()
  await expect(page.getByRole('button', { name: '创建连接密钥' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: '连接密钥' })).toBeHidden()
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
  const header = page.locator('.workbench-content-header')
  const rail = page.getByRole('navigation', { name: '工作台工具' })
  await header.getByRole('button', { name: '折叠侧边栏' }).click()
  await page.getByRole('button', { name: '立即全新创建' }).click()
  await expect(page).toHaveURL(/\/guide\/install\/step\/1$/)
  await expect(rail).toBeVisible()
  const guide = page.locator('.workbench-content-body').getByRole('region', { name: '引导' })
  await expect(guide.getByRole('heading', { name: '检查运行环境' })).toBeVisible()
  await expect(guide.getByRole('button', { name: '返回工作台' })).toHaveCount(0)
  await expect(guide.getByRole('navigation', { name: '引导流程' })).toHaveCount(0)
  const workbar = page.locator('.robot-workspace-list')
  await expect(workbar.locator('.project-rail')).toHaveCount(0)
  const steps = workbar.getByRole('navigation', { name: '引导流程' })
  await expect(steps.getByText('引导', { exact: true })).toHaveCount(0)
  await expect(steps.getByRole('button', { name: '返回', exact: true })).toBeVisible()
  await expect(header.getByText('第 2 / 4 步 · 环境检查', { exact: true })).toBeVisible()
  await expect(guide.getByText('第 2 / 4 步 · 环境检查', { exact: true })).toHaveCount(0)
  await expect(steps.getByRole('button', { name: '环境检查' })).toHaveAttribute('aria-current', 'step')
  await expect(steps.getByRole('button', { name: '机器人名称与位置' })).toBeDisabled()
  await page.setViewportSize({ width: 390, height: 760 })
  await header.getByRole('button', { name: '打开引导流程' }).click()
  const drawer = page.getByRole('dialog', { name: '引导流程导航' })
  await expect(drawer.getByRole('button', { name: '返回', exact: true })).toBeVisible()
  await drawer.getByRole('button', { name: '环境检查' }).click()
  await expect(drawer).toBeHidden()
  await expect(page).toHaveURL(/\/guide\/install\/step\/1$/)
  await page.setViewportSize({ width: 1280, height: 720 })
  await steps.getByRole('button', { name: '返回', exact: true }).click()
  await expect(page).toHaveURL(/\/guide$/)
  await expect(guide.getByRole('heading', { name: '你现在想做什么？' })).toBeVisible()
  await expect(header.getByText('选择目的', { exact: true })).toBeVisible()
  await header.getByRole('button', { name: '返回工作台' }).click()
  await expect(page).toHaveURL(/\/dashboard\/robot(?:\?|$)/)
  await expect(guide).toHaveCount(0)
  await expect(header.getByRole('button', { name: '展开侧边栏' })).toBeVisible()
  await header.getByRole('button', { name: '打开引导' }).click()
  await expect(guide).toBeVisible()
  await rail.getByRole('button', { name: '资源管理器' }).click()
  await expect(page).toHaveURL(/\/dashboard\/robot(?:\?|$)/)
  await expect(page.getByRole('region', { name: '资源管理器' })).toBeVisible()
  await header.getByRole('button', { name: '打开引导' }).click()
  await rail.getByRole('button', { name: '主页', exact: true }).click()
  await expect(page.getByRole('heading', { name: '开始管理你的机器人' })).toBeVisible()
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
    if (path === '/api/v1/terminal/sessions')
      return route.fulfill({
        json: { id: 'navigation-terminal', key: 'fixture-key', cwd: saved, shell: '/bin/sh' }
      })
    if (path === '/api/v1/terminal/sessions/navigation-terminal/stream')
      return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' })
    if (path === '/api/v1/terminal/sessions/navigation-terminal')
      return route.fulfill({ json: {} })
    if (path === '/api/v1/workspace/project-files')
      return route.fulfill({ json: { entries: [{ name: 'package.json', path: 'package.json', directory: false }] } })
    if (path === '/api/v1/catalog')
      return route.fulfill({ json: ['全部', '数据管理', '框架桥接'].map(title => ({ id: title, title, items: [] })) })
    return route.fulfill({ status: 503, json: { error: '测试中未启用此服务' } })
  })
  await page.goto(`/dashboard/robot?root=${encodeURIComponent(stale)}`)
  const savedRobot = page
    .locator('.project-rail')
    .getByText('saved-robot', { exact: true })
  await expect(savedRobot).toBeVisible()
  const menu = page.getByRole('complementary', { name: '目录操作' })
  await expect(menu).toBeVisible()
  await expect(menu.locator('footer').getByRole('button')).toHaveCount(4)
  await expect(menu.getByRole('button', { name: '查看运行终端' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: '辅助标签页', exact: true })).toBeVisible()
  await page.getByRole('button', { name: '隐藏机器人导航' }).click()
  await expect(menu).toHaveCount(0)
  await page.getByRole('button', { name: '显示机器人导航' }).click()
  await expect(menu).toBeVisible()

  await page.setViewportSize({ width: 1180, height: 720 })
  const header = page.locator('.workbench-content-header')
  const auxiliary = page.locator('.workbench-split-terminal')
  const auxiliaryToggle = header.getByRole('button', { name: '辅助标签页' })
  const launcher = auxiliary.getByRole('region', { name: '新标签页' })
  await auxiliaryToggle.click()
  await expect(launcher.getByRole('button')).toHaveText(['终端', '仓库', '目录'])
  await launcher.getByRole('button', { name: '目录', exact: true }).click()
  const directory = auxiliary.getByRole('complementary', { name: '机器人项目文件' })
  await expect(directory.getByRole('button', { name: 'package.json' })).toBeVisible()
  await expect(directory.getByText('alpha', { exact: true })).toHaveCount(0)
  await auxiliary.getByRole('button', { name: '新标签页' }).click()
  await launcher.getByRole('button', { name: '仓库', exact: true }).click()
  await expect(auxiliary.getByRole('complementary', { name: '仓库', exact: true })).toBeVisible()
  await auxiliary.getByRole('button', { name: '新标签页' }).click()
  await launcher.getByRole('button', { name: '终端', exact: true }).click()
  const terminal = page.getByRole('complementary', { name: '终端面板' })
  await expect(terminal).toBeVisible()
  await expect(menu).toHaveCount(0)
  const functions = header.getByRole('button', { name: '打开机器人功能导航' })
  const dropdown = header.getByRole('region', { name: '机器人功能菜单' })
  await functions.click()
  await expect(dropdown).toBeVisible()
  await expect(dropdown.getByRole('heading')).toHaveCount(0)
  await expect(dropdown.locator('.workbench-action-item')).toHaveText([
    '运行', '配置', '背包', '插件', '连接', 'Git 管理', '应用', '测试', '聊天', '运维'
  ])
  const plugins = dropdown.getByRole('button', { name: '插件', exact: true })
  await plugins.click()
  await expect(dropdown).toBeHidden()
  await functions.click()
  await expect(plugins).toHaveAttribute('aria-expanded', 'false')
  const mainBounds = (await dropdown.boundingBox())!
  await plugins.click()
  const categories = dropdown.getByRole('group', { name: '插件子菜单' })
  await expect(categories).toBeVisible()
  const categoryBounds = (await categories.boundingBox())!
  expect(categoryBounds.x + categoryBounds.width).toBeLessThanOrEqual(mainBounds.x)
  expect((await dropdown.boundingBox())!.height).toBe(mainBounds.height)
  const data = categories.getByRole('button', { name: '数据管理' })
  await data.press('Escape')
  await expect(categories).toBeHidden()
  await expect(plugins).toBeFocused()
  await expect(dropdown).toBeVisible()
  await plugins.click()
  await data.click()
  await expect(dropdown).toBeHidden()
  await expect(functions).toBeFocused()
  await functions.click()
  await plugins.click()
  await expect(data).toHaveAttribute('aria-current', 'page')
  await page.mouse.click(5, 690)
  await expect(dropdown).toBeHidden()
  await functions.click()
  await dropdown.getByRole('button', { name: '运行', exact: true }).click()
  await expect(dropdown).toBeHidden()
  await expect(functions).toBeFocused()
  await auxiliary.getByRole('button', { name: '关闭终端 saved-robot', exact: true }).click()
  await expect(terminal).toBeHidden()
  await auxiliaryToggle.click()
  await expect(menu).toBeVisible()

  const list = page.getByRole('navigation', { name: '机器人列表' })
  await expect(list.getByRole('region', { name: '已置顶机器人' })).toContainText('saved-robot')
  const folder = list.getByRole('button', { name: '目录 /test/robots/team' })
  const alpha = list.getByRole('button', { name: 'alpha', exact: true })
  await expect(alpha).toBeVisible()
  await alpha.click({ button: 'right' })
  const contextMenu = list.getByRole('menu', { name: 'alpha 的操作' })
  await expect(contextMenu.getByRole('button')).toHaveText(['打开', '置顶', '移除'])
  await expect(contextMenu.getByRole('button', { name: '打开', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(contextMenu).toBeHidden()
  await expect(alpha).toBeFocused()
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
  await git.click()
  const gitManagement = auxiliary.getByRole('region', { name: 'Git 管理' })
  await expect(page.getByRole('region', { name: '资源管理器' })).toBeVisible()
  await expect(gitManagement.getByRole('heading', { name: 'saved-robot · Git' })).toBeVisible()
  await expect(page.getByRole('dialog', { name: 'saved-robot · Git' })).toHaveCount(0)
  await auxiliary.getByRole('button', { name: '关闭辅助面板' }).click()
  await expect(gitManagement).toBeHidden()
})

test('robot runtime switches only after a fresh compatible preview is confirmed', async ({ page }) => {
  const root = '/test/robots/runtime-fixture'
  let runtime = 'node'
  let stopped = false
  const mutations: unknown[] = []
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    const url = new URL(request.url())
    switch (url.pathname) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/goals':
        return route.fulfill({ json: [] })
      case '/api/v1/workspace/robots':
        return route.fulfill({
          json: {
            scope: 'local',
            initialized: true,
            projects: [{ id: root, name: 'runtime-fixture', path: root }],
            activeProjectID: root
          }
        })
      case '/api/v1/robot/validate':
        return route.fulfill({ json: { valid: true } })
      case '/api/v1/robot/runtime':
        return route.fulfill({
          json: { name: 'runtime-fixture', version: '1.0.0', runtime, packageManager: runtime === 'node' ? 'npm' : 'bun', platforms: [] }
        })
      case '/api/v1/robot/pm2-processes':
        return route.fulfill({
          json: {
            items: [
              { id: 1, name: 'bun-worker', status: 'online', pid: 101, runtime: 'bun', runtimeVersion: '1.4.4' },
              { id: 2, name: 'node-worker', status: 'online', pid: 102, runtime: 'node', runtimeVersion: '22.23.5' },
              { id: 3, name: 'stopped-worker', status: 'stopped', pid: 103, runtime: 'bun', runtimeVersion: '9.9.9' }
            ]
          }
        })
      case '/api/v1/robot/runtime/switch':
        if (request.method() === 'POST') {
          mutations.push(request.postDataJSON())
          runtime = 'bun'
          return route.fulfill({ json: { runtime } })
        }
        expect(url.searchParams.get('target')).toBe('bun')
        return route.fulfill({
          json: {
            current: 'node',
            target: 'bun',
            changes: ['重新安装依赖'],
            blocked: stopped ? [] : ['请先手动停止该项目的 PM2 进程，再切换运行时。'],
            packageManager: 'bun',
            revision: 'fixture-preview',
            platform: 'darwin/arm64',
            check: { id: 'bun', name: 'Bun', status: 'ready', suggestion: '' }
          }
        })
      default:
        return route.fulfill({ status: 503, json: { error: '测试中未启用此服务' } })
    }
  })
  await page.goto(`/dashboard/robot?root=${encodeURIComponent(root)}`)
  await expect(page.getByText('Bun v1.4.4', { exact: true })).toBeVisible()
  await expect(page.getByText('Node.js v22.23.5', { exact: true })).toBeVisible()
  await expect(page.getByText('Bun v9.9.9', { exact: true })).toHaveCount(0)
  const switchRuntime = page.getByRole('button', {
    name: '切换运行时',
    exact: true
  })
  await expect(switchRuntime).toBeVisible()
  await expect(switchRuntime).not.toHaveCSS('font-size', '0px')
  await switchRuntime.click()
  const dialog = page.getByRole('dialog', { name: '切换项目运行时' })
  await expect(dialog.getByText(/请先手动停止/)).toBeVisible()
  const confirm = dialog.getByRole('button', { name: '确认切换' })
  await expect(confirm).toBeDisabled()
  expect(mutations).toEqual([])
  stopped = true
  await dialog.getByRole('button', { name: '重新检查' }).click()
  await expect(confirm).toBeEnabled()
  await confirm.click()
  await expect(dialog).toHaveCount(0)
  await expect(page.getByRole('status').filter({ hasText: /^Bun$/ })).toBeVisible()
  await expect(page.getByText(/1\.0\.0 · Bun · bun/)).toBeVisible()
  expect(mutations).toEqual([{ root, target: 'bun', revision: 'fixture-preview' }])
})
