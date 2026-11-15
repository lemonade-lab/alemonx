import { expect, test } from '@playwright/test'

test('desktop settings checks, downloads and confirms installation through the native bridge', async ({ page }) => {
  await page.addInitScript(() => {
    let ready = false
    Object.assign(window, {
      webkit: {
        messageHandlers: {
          external: {
            postMessage: (message: string) => {
              if (message === 'wails:runtime:ready') {
                ready = true
                return
              }
              if (!ready || !message.startsWith('alx:desktop-update:')) return
              const action = message.split(':').at(-1)
              document.documentElement.dataset.updateAction = action
              window.dispatchEvent(new CustomEvent('alx:desktop-update', {
                detail: {
                  current: 'v0.3.0',
                  action,
                  latest: action === 'status' ? undefined : 'v0.3.1',
                  asset: action === 'status' ? undefined : 'alemonx-desktop-macos_arm64.zip',
                  ready: action === 'download' || action === 'install'
                }
              }))
            }
          }
        }
      }
    })
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.route('**/api/desktop/preferences', route => route.fulfill({ json: {} }))
  await page.goto('/desktop-window/settings#section=update')

  await expect(page.getByText('当前版本 v0.3.0')).toBeVisible()
  await page.getByRole('button', { name: '检查更新', exact: true }).click()
  await expect(page.getByText('发现新版本 v0.3.1')).toBeVisible()
  await page.getByRole('button', { name: '下载桌面更新包', exact: true }).click()
  await expect(page.getByText('更新包已校验，可以安装。')).toBeVisible()
  await page.getByRole('button', { name: '安装并重启', exact: true }).click()
  const confirm = page.getByRole('dialog', { name: '安装桌面更新' })
  await expect(confirm).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('data-update-action', 'download')
  await confirm.getByRole('button', { name: '安装并重启', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('data-update-action', 'install')
})


test('desktop update displays a fallback version when the native bridge is silent', async ({ page }) => {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: { messageHandlers: { external: { postMessage: () => {} } } }
    })
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.route('**/api/desktop/preferences', route => route.fulfill({ json: {} }))
  await page.goto('/desktop-window/settings#section=update')
  await expect(page.getByText('当前版本 开发版')).toBeVisible()
  const check = page.getByRole('button', { name: '检查更新', exact: true })
  await expect(check).toBeEnabled()
  await page.clock.install()
  await check.click()
  await page.clock.fastForward(26_000)
  await expect(page.getByRole('alert')).toContainText('桌面更新程序未响应')
  await expect(check).toBeEnabled()
})
