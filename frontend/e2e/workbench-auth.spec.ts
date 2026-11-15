import { expect, test, type Page } from '@playwright/test'

async function mockNativeCredentials(page: Page) {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: {
        messageHandlers: {
          external: {
            postMessage(message: string) {
              const prefix = 'alx:workbench-credential:'
              if (!message.startsWith(prefix)) return
              const { id, action, account, password } = JSON.parse(
                message.slice(prefix.length)
              )
              const key = `fixture-keychain:${account}`
              if (action === 'set') sessionStorage.setItem(key, password)
              if (action === 'delete') sessionStorage.removeItem(key)
              window.dispatchEvent(
                new CustomEvent('alx:workbench-credential', {
                  detail: {
                    id,
                    password:
                      action === 'get' ? sessionStorage.getItem(key) : undefined
                  }
                })
              )
            }
          }
        }
      }
    })
  })
}

test('desktop login remembers the account and only restores a password with consent', async ({
  page
}) => {
  await mockNativeCredentials(page)
  let authenticated = false
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({
      json: {
        enabled: true,
        authenticated,
        account: authenticated ? 'fixture-user' : undefined
      }
    })
  )
  await page.route('**/api/v1/auth/login', route => {
    expect(route.request().postDataJSON()).toEqual({
      account: 'fixture-user',
      password: 'fixture-password'
    })
    authenticated = true
    return route.fulfill({ json: { account: 'fixture-user' } })
  })
  await page.goto('/desktop-window/settings#section=auth')
  const account = page.getByLabel('账户', { exact: true })
  const password = page.getByLabel('密码', { exact: true })
  const remember = page.getByRole('checkbox', { name: '记住密码' })
  const submit = page.getByRole('button', { name: '登录', exact: true })
  await expect(remember).not.toBeChecked()
  await account.fill('fixture-user')
  await password.fill('fixture-password')
  await remember.check()
  await submit.click()
  await expect(submit).toBeHidden()

  authenticated = false
  await page.reload()
  await expect(account).toHaveValue('fixture-user')
  await expect(password).toHaveValue('fixture-password')
  await expect(remember).toBeChecked()
  await remember.uncheck()
  await submit.click()
  await expect(submit).toBeHidden()

  authenticated = false
  await page.reload()
  await expect(account).toHaveValue('fixture-user')
  await expect(password).toHaveValue('')
  await expect(remember).not.toBeChecked()
})

test('binding and signing in with GitHub use the workbench account', async ({
  page
}) => {
  await mockNativeCredentials(page)
  await page.clock.install()
  let authenticated = true
  let githubLogin: string | undefined
  const device = {
    flowId: 'fixture-flow',
    userCode: 'TEST-CODE',
    verificationUri: 'https://github.com/login/device',
    expiresIn: 900,
    interval: 5
  }
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({
      json: {
        enabled: true,
        authenticated,
        account: authenticated ? 'fixture-user' : undefined,
        githubLogin: authenticated ? githubLogin : undefined
      }
    })
  )
  await page.route('**/api/v1/auth/github/binding', route => {
    const body = route.request().postDataJSON()
    if (body.flowId) {
      expect(body.flowId).toBe(device.flowId)
      githubLogin = 'octocat'
      return route.fulfill({ json: { status: 'ok', login: githubLogin } })
    }
    expect(body).toEqual({ password: 'fixture-password' })
    return route.fulfill({ json: device })
  })
  await page.route('**/api/v1/auth/logout', route => {
    authenticated = false
    return route.fulfill({ json: { authenticated } })
  })
  await page.route('**/api/v1/auth/github/login', route => {
    if (!route.request().postDataJSON().flowId)
      return route.fulfill({ json: device })
    authenticated = true
    return route.fulfill({ json: { status: 'ok', account: 'fixture-user' } })
  })

  await page.goto('/desktop-window/settings#section=auth')
  await page.getByRole('button', { name: '绑定 GitHub', exact: true }).click()
  await page.getByLabel('当前工作台密码').fill('fixture-password')
  await page.getByRole('button', { name: '确认并授权' }).click()
  await expect(page.getByText('TEST-CODE')).toBeVisible()
  await page.clock.fastForward(5100)
  await expect(page.getByText('已绑定 GitHub @octocat')).toBeVisible()

  await page.getByRole('button', { name: '退出登录' }).click()
  await page.getByRole('button', { name: '使用 GitHub 登录' }).click()
  await expect(page.getByText('TEST-CODE')).toBeVisible()
  await page.clock.fastForward(5100)
  await expect(page.getByText('已绑定 GitHub @octocat')).toBeVisible()
  expect(
    await page.evaluate(() => localStorage.getItem('alx:auth:last-account'))
  ).toBe('fixture-user')
})
