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
