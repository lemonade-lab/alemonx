import { expect, test, type Page } from '@playwright/test'

async function openTestone(page: Page, mode: 'browser' | 'desktop' | 'composer') {
  if (mode === 'desktop') {
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'platform', { value: 'MacIntel' })
      Object.assign(window, {
        webkit: { messageHandlers: { external: { postMessage: () => {} } } }
      })
    })
  }
  await page.route('**/testone-shortcuts-fixture', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `
        <button type="button">测试台外的按钮</button>
        <div id="testone"></div>
        <div id="composer"></div>
        <output aria-label="发送次数">0</output>
        <script type="module">
          import RefreshRuntime from '/@react-refresh';
          RefreshRuntime.injectIntoGlobalHook(window);
          window.$RefreshReg$ = () => {};
          window.$RefreshSig$ = () => type => type;
          window.__vite_plugin_react_preamble_installed__ = true;
          const { default: React } = await import('/node_modules/.vite/deps/react.js');
          const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
          const { Provider } = await import('/node_modules/.vite/deps/react-redux.js');
          const { store } = await import('/src/features/testone/store/index.ts');
          const { default: Testone } = await import('/src/features/testone/pages/App.tsx');
          const { default: SlateTextarea } = await import('/src/features/testone/component/SlateTextarea.tsx');
          if (${mode === 'composer'}) {
            const output = document.querySelector('output');
            ReactDOM.createRoot(document.getElementById('composer')).render(
              React.createElement(Provider, { store }, React.createElement(SlateTextarea, {
                value: '消息',
                onClickSend: () => { output.textContent = String(Number(output.textContent) + 1); }
              }))
            );
          } else {
            ReactDOM.createRoot(document.getElementById('testone')).render(
              React.createElement(Provider, { store }, React.createElement(Testone, { theme: 'dark' }))
            );
          }
        </script>
      `
    })
  )
  await page.goto('/testone-shortcuts-fixture')
}

test('browser TestOne leaves navigation and clearing shortcuts to the browser', async ({ page }) => {
  await openTestone(page, 'browser')
  const content = page.getByRole('region', { name: '机器人测试台' })
  const handled = await content.evaluate(node =>
    ['metaKey', 'ctrlKey'].flatMap(modifier =>
      ['1', '2', '3', '4', 'k'].map(key =>
        !node.dispatchEvent(new KeyboardEvent('keydown', {
          key, [modifier]: true, bubbles: true, cancelable: true
        }))
      )
    )
  )
  expect(handled).toEqual(Array(10).fill(false))
  await expect(page.getByRole('heading', { name: /ALemonTestOne 使用说明/ })).toHaveCount(0)
})

test('desktop TestOne shortcuts follow focus and use the platform modifier', async ({ page }) => {
  await openTestone(page, 'desktop')
  const help = page.getByRole('heading', { name: /ALemonTestOne 使用说明/ })
  await page.getByRole('button', { name: '测试台外的按钮' }).focus()
  await page.keyboard.press('Meta+4')
  await expect(help).toHaveCount(0)

  await page.getByRole('region', { name: '机器人测试台' }).focus()
  await page.keyboard.press('Control+4')
  await expect(help).toHaveCount(0)
  await page.keyboard.press('Meta+4')
  await expect(help).toBeVisible()
  await expect(page.getByText('⌘+4', { exact: true })).toBeVisible()
})

test('TestOne composer inserts line breaks without sending on modified Enter', async ({ page }) => {
  await openTestone(page, 'composer')
  const input = page.locator('#composer [contenteditable="true"]')
  const sent = page.getByLabel('发送次数')
  await input.fill('消息')
  await input.press('Shift+Enter')
  await input.press('Control+Enter')
  await expect(sent).toHaveText('0')
  await expect(input.locator('[data-slate-node="element"]')).toHaveCount(3)
  await input.press('Enter')
  await expect(sent).toHaveText('1')
})
