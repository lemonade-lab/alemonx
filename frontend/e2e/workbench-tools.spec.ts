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

test('settings shortcuts open SSH and manage HTTP MCP with developer mode off', async ({
  page
}) => {
  let managed = false
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
      default:
        return route.fulfill({
          status: 503,
          json: { error: '测试中未启用其他服务' }
        })
    }
  })
  await page.goto('/dashboard/robot')
  const menuTrigger = page.getByRole('button', { name: '操作', exact: true })
  const menu = page.getByRole('region', { name: '操作菜单' })
  const shortcut = async (name: string) => {
    await menuTrigger.click()
    await menu.getByRole('button', { name, exact: true }).click()
    await expect(menu).toBeHidden()
  }

  await shortcut('通用')
  await expect(
    page.getByRole('checkbox', { name: /开发模式/ })
  ).not.toBeChecked()
  await page.getByRole('button', { name: '关闭设置', exact: true }).click()
  await shortcut('SSH 管理')
  await expect(page.getByRole('heading', { name: 'SSH 管理' })).toBeVisible()
  await expect(page.getByRole('button', { name: '复制公钥' })).toBeVisible()
  await page.getByRole('button', { name: '关闭设置', exact: true }).click()
  await shortcut('MCP')
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
  await page.getByRole('button', { name: '立即全新创建' }).click()
  await expect(page).toHaveURL(/\/guide\/install\/step\/1$/)
  await expect(
    page.getByRole('heading', { name: '你的环境都准备好了吗' })
  ).toBeVisible()
})
