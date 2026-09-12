import { expect, test } from '@playwright/test'

test('restores latest session and ignores initial idle until the answer ends', async ({ page }) => {
  page.on('pageerror', error => console.error(error.message))
  let created = 0
  await page.route('**/api/v1/dsh/**', async route => {
    const url = route.request().url()
    if (url.endsWith('/status')) return route.fulfill({ json: { ready: true } })
    if (url.endsWith('/config')) return route.fulfill({ json: { configured: true, credentialConfigured: true, model: 'deepseek-chat' } })
    if (url.endsWith('/history')) return route.fulfill({ json: { messages: [{ role: 'user', content: '之前的问题' }, { role: 'assistant', content: '之前的回复' }] } })
    if (url.endsWith('/sessions')) {
      if (route.request().method() === 'POST') created++
      return route.fulfill({ json: [{ id: 'recent', updatedAt: new Date().toISOString(), access: 'ask' }] })
    }
    if (url.endsWith('/prompt')) {
      await page.evaluate(() => {
        const stream = (window as unknown as { dshStream: EventTarget }).dshStream
        const emit = (event: object) => stream.dispatchEvent(new MessageEvent('dsh', { data: JSON.stringify(event) }))
        emit({ id: 1, sessionId: 'recent', type: 'session.status', status: 'idle' })
        emit({ id: 2, sessionId: 'recent', type: 'turn/start' })
        emit({ id: 3, sessionId: 'recent', type: 'assistant/message', text: '回复已经到达' })
        emit({ id: 4, sessionId: 'recent', type: 'turn/end' })
      })
      return route.fulfill({ json: { messageId: 'mock' } })
    }
    return route.fulfill({ json: {} })
  })
  await page.addInitScript(() => {
    class Stream extends EventTarget {
      constructor() {
        super()
        Object.assign(window, { dshStream: this })
        setTimeout(() => this.dispatchEvent(new Event('ready')), 0)
      }
      close() {}
    }
    Object.assign(window, { EventSource: Stream })
  })
  await page.route('**/dsh-regression', route => route.fulfill({
    contentType: 'text/html',
    body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const { default: React } = await import('/node_modules/.vite/deps/react.js');
      const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
      const { DSHWorkspace } = await import('/src/components/DSHWorkspace.tsx');
      const { Provider } = await import('/node_modules/.vite/deps/react-redux.js');
      const { store } = await import('/src/store/guideStore.ts');
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store}, React.createElement(React.StrictMode, null, React.createElement(DSHWorkspace, {root:'/mock/project'}))));
    </script>`
  }))
  await page.goto('/dsh-regression')
  const input = page.getByRole('textbox', { name: '描述要交给 Agent 的任务' })
  await expect(input).toBeEnabled()
  await expect(page.getByText('之前的问题', { exact: true })).toBeVisible()
  await expect(page.getByText('之前的回复', { exact: true })).toBeVisible()
  expect(created).toBe(0)
  await input.fill('你是什么模型')
  await input.press('Enter')
  await expect(page.getByText('回复已经到达', { exact: true })).toBeVisible()
  await expect(input).toBeEnabled()
  await expect(page.getByText('正在使用受限工具：event')).toHaveCount(0)
})
