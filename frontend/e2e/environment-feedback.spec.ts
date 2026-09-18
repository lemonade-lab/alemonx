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
