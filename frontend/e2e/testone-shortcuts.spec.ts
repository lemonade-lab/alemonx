import { expect, test, type Page } from '@playwright/test'

async function openTestone(page: Page, mode: 'browser' | 'desktop' | 'composer' | 'timer') {
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
        <div id="testone" class="testone-root testone-window"></div>
        <div id="composer"></div>
        <output aria-label="发送次数">0</output>
        <script type="module">
          import '/src/styles.css';
          import '/src/features/testone/testone.scss';
          import RefreshRuntime from '/@react-refresh';
          RefreshRuntime.injectIntoGlobalHook(window);
          window.$RefreshReg$ = () => {};
          window.$RefreshSig$ = () => type => type;
          window.__vite_plugin_react_preamble_installed__ = true;
          if (${mode === 'timer'}) await import('/e2e/fixtures/testone-timer.tsx');
          else {
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

test('TestOne special events remain reachable on a short phone screen', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 568 })
  await openTestone(page, 'desktop')
  await page.getByText('事件', { exact: true }).click()
  const events = page.getByRole('dialog', { name: '特殊事件', exact: true })
  await expect(events.getByRole('textbox', { name: '事件附加文本' })).toBeInViewport()
  await expect(events.getByRole('button', { name: '关闭特殊事件' })).toBeInViewport()
  await page.keyboard.press('Escape')
  await expect(events).toBeHidden()
})

test('TestOne timer keeps its controls visible without borders in both themes', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openTestone(page, 'timer')
  const dialog = page.getByRole('dialog')
  const toggle = dialog.getByRole('checkbox', { name: /循环执行/ })
  const track = toggle.locator('..').locator(':scope > [aria-hidden="true"]')
  const frequency = dialog.getByPlaceholder('1 - 12')
  await expect(frequency).toHaveAttribute('aria-invalid', 'true')
  await expect(track).toBeVisible()
  await expect.poll(async () => (await track.boundingBox())?.width).toBe(48)
  const bounds = await track.boundingBox()
  for (const { theme, on, off, error } of [
    { theme: 'light', on: 'rgb(142, 75, 19)', off: 'rgb(201, 190, 176)', error: 'rgb(253, 239, 239)' },
    { theme: 'dark', on: 'rgb(237, 183, 126)', off: 'rgb(104, 93, 78)', error: 'rgb(51, 27, 24)' }
  ]) {
    await page.evaluate(theme => { document.documentElement.dataset.theme = theme }, theme)
    await expect(frequency).toHaveCSS('background-color', error)
    await expect(track).toHaveCSS('background-color', on)
    await toggle.press('Space')
    await expect(toggle).not.toBeChecked()
    await expect(track).toHaveCSS('background-color', off)
    await expect(track).toHaveCSS('outline-width', '2px')
    expect(await track.boundingBox()).toEqual(bounds)
    await toggle.press('Space')
    await expect(toggle).toBeChecked()
  }
})
