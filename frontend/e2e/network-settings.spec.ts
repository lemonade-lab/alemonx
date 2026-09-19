import { expect, test, type Page } from '@playwright/test'
import type {
  SystemNetworkSettings,
  SystemNetworkStatus
} from '../src/store/workspaceApi'

const definitions = [
  {
    id: 'github-api',
    label: 'GitHub API',
    probe: 'https://api.github.com/',
    mirrors: true
  },
  {
    id: 'npm',
    label: 'NPM',
    probe: 'https://registry.npmjs.org/npm/latest',
    mirrors: true
  },
  {
    id: 'official',
    label: '官方下载',
    probe: 'https://download.alemonjs.com/app.apk',
    mirrors: false
  }
]
const defaults = (): SystemNetworkSettings => ({
  version: 2,
  revision: 0,
  mode: 'auto',
  automatic: { groups: definitions.map(d => ({ id: d.id, candidates: [] })) },
  proxy: { url: '' }
})

async function openNetwork(page: Page, initial = defaults()) {
  let state = initial
  let fail = false
  let checks = 0
  let pending = false
  let preview = false
  let draft = initial
  const writes: SystemNetworkSettings[] = []
  const status = (config = state): SystemNetworkStatus => ({
    revision: config.revision,
    definitions,
    groups: definitions.map(d => ({
      id: d.id,
      state: pending ? 'checking' : 'ready',
      current: 'official',
      latencyMs: 12,
      checkedAt: new Date().toISOString(),
      candidates: [
        { url: 'official', ok: true, latencyMs: 12, message: '可用' },
        ...(
          config.automatic.groups.find(g => g.id === d.id)?.candidates || []
        ).map(url => ({ url, ok: true, latencyMs: 25, message: '可用' }))
      ]
    }))
  })
  await page.route('**/api/v1/system/network*', async route => {
    const req = route.request()
    const url = new URL(req.url())
    if (req.method() === 'PUT') {
      const input = req.postDataJSON() as SystemNetworkSettings
      writes.push(input)
      if (fail)
        return route.fulfill({
          status: 409,
          json: { error: '网络配置已更新，请重新打开编辑' }
        })
      state = {
        ...input,
        revision: state.revision + 1,
        migration: undefined,
        proxy: {
          url: input.proxy.url,
          hasCredentials:
            input.proxy.credentials === 'replace' || input.proxy.hasCredentials
        }
      }
      return route.fulfill({ json: state })
    }
    if (req.method() === 'POST') {
      if (url.searchParams.get('action') === 'detect') {
        pending = true
        return route.fulfill({ status: 202, json: status() })
      }
      if (url.searchParams.get('action') === 'preview') {
        preview = true
        pending = true
        draft = req.postDataJSON()
        return route.fulfill({ status: 202, json: { task: 'draft' } })
      }
      return route.fulfill({
        json: { ok: true, message: '连接正常', latencyMs: 12 }
      })
    }
    if (url.searchParams.has('view') || url.searchParams.has('task')) {
      checks++
      const value = status(url.searchParams.has('task') ? draft : state)
      pending = false
      return route.fulfill({ json: value })
    }
    return route.fulfill({ json: state })
  })
  await page.route('**/network-regression', route =>
    route.fulfill({
      contentType: 'text/html',
      body: `<div id="root"></div><script type="module">import R from '/@react-refresh'; R.injectIntoGlobalHook(window);window.$RefreshReg$=()=>{}; window.$RefreshSig$=()=>t=>t;window.__vite_plugin_react_preamble_installed__=true;await import('/e2e/fixtures/network-settings.tsx');</script>`
    })
  )
  await page.goto('/network-regression')
  await expect(
    page.getByRole('radio', { name: '自动', exact: true })
  ).toBeVisible()
  return {
    writes,
    fail: () => {
      fail = true
    },
    checks: () => checks,
    preview: () => preview
  }
}

test('three exclusive modes, direct hides automatic configuration, failed change retains mode', async ({
  page
}) => {
  const calls = await openNetwork(page)
  await expect(
    page.getByRole('group', { name: 'NPM', exact: true })
  ).toBeVisible()
  await expect(page.getByText('资源例外')).toHaveCount(0)
  await page.getByRole('radio', { name: '直连', exact: true }).click()
  await expect(
    page.getByRole('radio', { name: '直连', exact: true })
  ).toBeChecked()
  await expect(
    page.getByRole('group', { name: 'NPM', exact: true })
  ).toHaveCount(0)
  await expect(page.getByRole('region', { name: '直连状态' })).toBeVisible()
  calls.fail()
  await page.getByRole('radio', { name: '自动', exact: true }).click()
  await expect(
    page.getByRole('radio', { name: '直连', exact: true })
  ).toBeChecked()
  await expect(page.getByRole('status')).toContainText('网络配置已更新')
})

test('proxy selection shows an inline form and validates without applying', async ({
  page
}) => {
  const calls = await openNetwork(page)
  const trigger = page.getByRole('radio', { name: '代理', exact: true })
  await trigger.click()
  const dialog = page.getByRole('region', { name: '代理配置' })
  await expect(trigger).toBeChecked()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await dialog.getByRole('button', { name: '保存并应用' }).click()
  await expect(dialog.getByRole('alert')).toHaveCount(2)
  await page.getByRole('radio', { name: '自动', exact: true }).click()
  await expect(dialog).toHaveCount(0)
  await expect(
    page.getByRole('radio', { name: '自动', exact: true })
  ).toBeChecked()
  expect(calls.writes).toHaveLength(0)
})

test('proxy credentials are transient and save failures retain inline inputs', async ({
  page
}) => {
  const calls = await openNetwork(page)
  await page.getByRole('radio', { name: '代理', exact: true }).click()
  const dialog = page.getByRole('region', { name: '代理配置' })
  await dialog.getByLabel('协议').selectOption('socks5')
  await dialog.getByLabel('主机', { exact: true }).fill('127.0.0.1')
  await dialog.getByLabel('端口', { exact: true }).fill('1080')
  await dialog.getByLabel('认证', { exact: true }).selectOption('replace')
  await dialog.getByLabel('用户名', { exact: true }).fill('alice')
  await dialog.getByLabel('密码', { exact: true }).fill('proxy-secret')
  await dialog.getByRole('button', { name: '测试连接' }).click()
  await expect(dialog.getByRole('status')).toContainText('连接正常')
  expect(calls.writes).toHaveLength(0)
  calls.fail()
  await dialog.getByRole('button', { name: '保存并应用' }).click()
  await expect(dialog.getByRole('alert')).toContainText('网络配置已更新')
  await expect(dialog.getByLabel('密码', { exact: true })).toHaveValue(
    'proxy-secret'
  )
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
    'proxy-secret'
  )
  await page.getByRole('radio', { name: '自动', exact: true }).click()
  await page.getByRole('radio', { name: '代理', exact: true }).click()
  await dialog.getByLabel('认证', { exact: true }).selectOption('replace')
  await expect(dialog.getByLabel('密码', { exact: true })).toHaveValue('')
})

test('candidate preview is asynchronous, does not save, and updates inside its dialog', async ({
  page
}) => {
  const calls = await openNetwork(page)
  await page.getByRole('button', { name: '配置 NPM', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '配置 NPM' })
  await expect(dialog.getByText('官方地址 · 固定')).toBeVisible()
  await dialog.getByRole('button', { name: '添加入口' }).click()
  await dialog
    .getByRole('textbox', { name: '候选入口 1', exact: true })
    .fill('https://mirror.example/{path}')
  await dialog.getByRole('button', { name: '测试候选' }).click()
  await expect.poll(calls.preview).toBeTruthy()
  await expect(dialog.getByRole('status').last()).toContainText('可用', {
    timeout: 6000
  })
  expect(calls.writes).toHaveLength(0)
  await dialog.getByRole('button', { name: '保存并应用' }).click()
  await expect(dialog).toHaveCount(0)
  expect(
    calls.writes[0].automatic.groups.find(g => g.id === 'npm')?.candidates
  ).toEqual(['https://mirror.example/{path}'])
})

test('detection feedback stays on the group and polling stops when complete', async ({
  page
}) => {
  const calls = await openNetwork(page)
  const row = page.getByRole('group', { name: 'NPM', exact: true })
  await row.getByRole('button', { name: '重新检测 NPM' }).click()
  await expect(row.getByRole('status')).toContainText('检测中')
  await expect(row.getByRole('status')).toContainText('可用', { timeout: 6000 })
  const checks = calls.checks()
  await page.waitForTimeout(2300)
  expect(calls.checks()).toBe(checks)
  await page.getByRole('radio', { name: '直连', exact: true }).click()
  await expect(
    page.getByRole('radio', { name: '直连', exact: true })
  ).toBeChecked()
  const after = calls.checks()
  await page.waitForTimeout(2300)
  expect(calls.checks()).toBe(after)
})

test('migration requires explicit confirmation and can be cancelled without writes', async ({
  page
}) => {
  const initial = defaults()
  initial.migration = {
    pending: true,
    proxies: ['http://localhost:7890', 'http://localhost:1080']
  }
  const calls = await openNetwork(page, initial)
  const dialog = page.getByRole('dialog', { name: '确认网络迁移' })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '取消' }).click()
  expect(calls.writes).toHaveLength(0)
  await page.getByRole('button', { name: '确认网络迁移' }).click()
  await dialog.getByLabel('全局模式').selectOption('direct')
  await dialog.getByRole('button', { name: '保存并应用' }).click()
  await expect(dialog).toHaveCount(0)
  expect(calls.writes[0].confirmMigration).toBe(true)
  expect(calls.writes[0].mode).toBe('direct')
})

test('compact layout keeps mode controls and dialogs usable', async ({
  page
}) => {
  await page.setViewportSize({ width: 375, height: 720 })
  await openNetwork(page)
  await page.getByRole('button', { name: '配置 NPM', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '保存并应用' })
  ).toBeInViewport()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})

test('saving a proxy switches the global mode and editing clears obsolete test feedback', async ({
  page
}) => {
  const calls = await openNetwork(page)
  await page.getByRole('radio', { name: '代理', exact: true }).click()
  const dialog = page.getByRole('region', { name: '代理配置' })
  await dialog.getByLabel('主机', { exact: true }).fill('localhost')
  await dialog.getByLabel('端口', { exact: true }).fill('7890')
  await dialog.getByRole('button', { name: '测试连接' }).click()
  await expect(dialog.getByRole('status')).toContainText('连接正常')
  await dialog.getByLabel('端口', { exact: true }).fill('7891')
  await expect(dialog.getByRole('status')).toHaveCount(0)
  await dialog.getByRole('button', { name: '保存并应用' }).click()
  await expect(dialog.getByRole('status')).toHaveText('已应用')
  await expect(
    page.getByRole('radio', { name: '代理', exact: true })
  ).toBeChecked()
  await expect(dialog.getByLabel('主机', { exact: true })).toHaveValue(
    'localhost'
  )
  await expect(dialog.getByLabel('端口', { exact: true })).toHaveValue('7891')
  await expect(page.getByRole('region', { name: '自动选路' })).toHaveCount(0)
  expect(calls.writes[0].mode).toBe('proxy')
})

test('saved proxy opens inline on a compact screen and clears credentials after saving', async ({
  page
}) => {
  await page.setViewportSize({ width: 375, height: 720 })
  const initial = defaults()
  initial.mode = 'proxy'
  initial.proxy = { url: 'https://localhost:7890', hasCredentials: true }
  const calls = await openNetwork(page, initial)
  const form = page.getByRole('region', { name: '代理配置' })
  await expect(form.getByLabel('主机', { exact: true })).toHaveValue(
    'localhost'
  )
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await form.getByLabel('认证', { exact: true }).selectOption('replace')
  await form.getByLabel('用户名', { exact: true }).fill('alice')
  await form.getByLabel('密码', { exact: true }).fill('new-secret')
  await form.getByRole('button', { name: '保存并应用' }).click()
  await expect(form.getByRole('status')).toHaveText('已应用')
  await expect(form.getByLabel('认证', { exact: true })).toHaveValue('preserve')
  await form.getByLabel('认证', { exact: true }).selectOption('replace')
  await expect(form.getByLabel('密码', { exact: true })).toHaveValue('')
  expect(calls.writes[0].proxy.password).toBe('new-secret')
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})
