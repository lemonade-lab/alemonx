import { expect, test, type Page } from '@playwright/test'
import {
  connectionProfile,
  migrateConnections,
  newConnection,
  type SQLEngine
} from '../src/lib/connectModels'

test('profile migration preserves legacy paths and excludes secrets', () => {
  const legacy = migrateConnections(undefined, ['C:\\data\\bot.db'], {
    'C:\\data\\bot.db': '旧连接'
  })
  expect(legacy[0]).toMatchObject({
    engine: 'sqlite',
    name: '旧连接',
    path: 'C:\\data\\bot.db'
  })
  const dirty = {
    ...connectionProfile('id', '测试', newConnection()),
    password: 'secret',
    sql: 'sensitive'
  }
  expect(JSON.stringify(migrateConnections([dirty], [], {}))).not.toMatch(
    /secret|sensitive|password/
  )
})

test('connection editor uses the global modal, traps focus and cancels without saving', async ({
  page
}, info) => {
  await openConnect(page)
  await expect(page.getByLabel('数据库类型', { exact: true })).toHaveCount(0)
  const trigger = page.getByRole('button', { name: '新建连接', exact: true })
  await trigger.click()
  const dialog = page.getByRole('dialog', {
    name: '配置 SQL 连接',
    exact: true
  })
  await expect(dialog).toBeVisible()
  expect(
    await dialog.evaluate(node => node.parentElement === document.body)
  ).toBe(true)
  expect(
    await dialog.evaluate(node => Number(getComputedStyle(node).zIndex))
  ).toBeGreaterThan(2000000000)
  await dialog.getByLabel('连接名称', { exact: true }).fill('不会被保存')
  await dialog.getByRole('button', { name: '取消', exact: true }).focus()
  await page.keyboard.press('Tab')
  await expect(
    dialog.getByRole('button', { name: '关闭对话框', exact: true })
  ).toBeFocused()
  await page.screenshot({ path: info.outputPath('connect-global-dialog.png') })
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  await expect(trigger).toBeFocused()
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
    '不会被保存'
  )
  await expect(page.getByRole('region', { name: '数据管理' })).toBeVisible()
})

async function openConnect(page: Page) {
  await page.route('**/api/v1/data/catalog', route =>
    route.fulfill({ json: { sqlite: [], redisAddress: '' } })
  )
  await page.route('**/connect-regression', route =>
    route.fulfill({
      contentType: 'text/html',
      body: `<div id="root"></div><script type="module">import RefreshRuntime from '/@react-refresh'; RefreshRuntime.injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type; window.__vite_plugin_react_preamble_installed__ = true; await import('/e2e/fixtures/data-workspace.tsx');</script>`
    })
  )
  await page.goto('/connect-regression')
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
}

for (const engine of [
  'mysql',
  'mariadb',
  'postgres',
  'sqlite'
] as SQLEngine[]) {
  test(`${engine} generic object navigation, query tabs and confirmed write`, async ({
    page
  }, info) => {
    const requests: Array<{
      action: string
      connection: { engine: string; password: string }
      write?: boolean
      confirmed?: boolean
      sql?: string
    }> = []
    await page.route('**/api/v1/data/connect', route => {
      const body = route.request().postDataJSON()
      requests.push(body)
      const rows =
        body.action === 'databases'
          ? [['app'], ['other']]
          : body.action === 'schemas'
            ? [
                [
                  engine === 'postgres'
                    ? 'public'
                    : engine === 'sqlite'
                      ? 'main'
                      : 'app'
                ]
              ]
            : body.action === 'tables'
              ? [['users', 'BASE TABLE']]
              : body.action === 'browse'
                ? [['9007199254740993', 'Alice']]
                : body.action === 'structure'
                  ? [['id', 'bigint']]
                  : body.action === 'indexes'
                    ? [['PRIMARY']]
                    : []
      return route.fulfill({
        json: {
          columns: body.write ? [] : ['name', 'type'],
          rows,
          affected: body.write ? 1 : 0,
          truncated: false
        }
      })
    })
    await openConnect(page)
    await page.getByRole('button', { name: '新建连接', exact: true }).click()
    await page.getByLabel('数据库类型', { exact: true }).selectOption(engine)
    if (engine === 'sqlite')
      await page.getByLabel('SQLite 文件（服务器绝对路径）').fill('/tmp/app.db')
    else {
      await page
        .getByLabel('密码（仅本次窗口）')
        .fill('never-persist-connect-password')
      await page.getByLabel('初始数据库（可选）').fill('app')
    }
    await page.getByLabel('连接名称', { exact: true }).fill('业务连接')
    await page.getByRole('button', { name: '连接', exact: true }).click()
    await page
      .getByRole('button', { name: 'users · BASE TABLE', exact: true })
      .click()
    await expect(
      page.getByRole('cell', { name: 'Alice', exact: true })
    ).toBeVisible()
    await page.getByRole('button', { name: '表结构', exact: true }).click()
    await expect(
      page.getByRole('cell', { name: 'bigint', exact: true })
    ).toBeVisible()
    await page.getByRole('button', { name: '索引', exact: true }).click()
    await expect(
      page.getByRole('cell', { name: 'PRIMARY', exact: true })
    ).toBeVisible()
    await page.getByRole('button', { name: '配置连接', exact: true }).click()
    await page.getByRole('button', { name: '保存连接', exact: true }).click()
    await expect
      .poll(() => page.evaluate(() => JSON.stringify(localStorage)))
      .toContain('业务连接')
    expect(
      await page.evaluate(() => JSON.stringify(localStorage))
    ).not.toContain('never-persist-connect-password')
    await page.getByRole('button', { name: '新建查询', exact: true }).click()
    await page
      .getByLabel('SQL 编辑器', { exact: true })
      .fill('DELETE FROM users WHERE id=1')
    await page.getByLabel('允许写入（执行时需确认）').check()
    await page.getByRole('button', { name: '执行写入', exact: true }).click()
    expect(requests.filter(r => r.write)).toHaveLength(0)
    await page.getByRole('button', { name: '取消', exact: true }).click()
    await page.getByRole('button', { name: '执行写入', exact: true }).click()
    await page.getByRole('button', { name: '确认继续', exact: true }).click()
    await expect.poll(() => requests.filter(r => r.write).length).toBe(1)
    expect(requests.at(-1)).toMatchObject({
      action: 'query',
      connection: { engine },
      confirmed: true
    })
    await page.getByRole('button', { name: '查询 1', exact: true }).click()
    await expect(page.getByLabel('SQL 编辑器')).toHaveValue('SELECT 1;')
    await page.getByRole('button', { name: '查询 2', exact: true }).click()
    await expect(page.getByLabel('SQL 编辑器')).toHaveValue(
      'DELETE FROM users WHERE id=1'
    )
    await page.getByRole('button', { name: '断开连接', exact: true }).click()
    await expect(
      page.getByText('切换查询上下文', { exact: true })
    ).toBeVisible()
    await page.getByRole('button', { name: '取消', exact: true }).click()
    await expect(page.getByLabel('SQL 编辑器')).toHaveValue(
      'DELETE FROM users WHERE id=1'
    )
    await page.getByRole('button', { name: '断开连接', exact: true }).click()
    await page.getByRole('button', { name: '确认继续', exact: true }).click()
    await expect(
      page.getByRole('button', { name: '执行查询', exact: true })
    ).toBeDisabled()
    await page.screenshot({ path: info.outputPath(`connect-${engine}.png`) })
  })
}

test('Connect stays available without local SQLite catalog and works at narrow width', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openConnect(page)
  await page.route('**/api/v1/data/catalog', route =>
    route.fulfill({ status: 503, json: {} })
  )
  await page.reload()
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await page.getByRole('button', { name: '新建连接', exact: true }).click()
  await expect(page.getByLabel('数据库类型', { exact: true })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('仍可配置其他 SQL 连接')
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})
