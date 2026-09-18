import { expect, test } from '@playwright/test'

test('desktop tools stay visible and only compact windows collapse tools', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.route('**/workbench-tools-regression', route => route.fulfill({
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
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store}, React.createElement(WorkbenchTools, null, React.createElement('input', {'aria-label':'工具输入'}))));
    </script>`
  }))
  await page.goto('/workbench-tools-regression')
  const trigger = page.getByRole('button', { name: '工具', exact: true })
  const input = page.getByRole('textbox', { name: '工具输入' })
  await expect(trigger).toBeHidden()
  await expect(input).toBeVisible()
  await expect(page.getByRole('button', { name: '收起工具' })).toBeHidden()
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
  await page.setViewportSize({ width: 1280, height: 800 })
  await expect(trigger).toBeHidden()
  await expect(input).toHaveValue('保留未完成输入')
  await expect(input).toBeVisible()
  await expect(page.getByRole('button', { name: '收起工具' })).toBeHidden()
  // A resized desktop workbench must respond to its own width, too.
  await page.locator('#root').evaluate(header => {
    header.parentElement!.style.width = '390px'
  })
  await expect(trigger).toBeVisible()
})
