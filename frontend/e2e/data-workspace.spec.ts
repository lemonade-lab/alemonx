import { expect, test, type Page } from '@playwright/test'
import { migrateDataTab } from '../src/lib/dataNavigation'

test('old SQL preference moves to Connect without changing the fixed SQLite entry', () => {
  expect(migrateDataTab('sql')).toBe('connect')
  expect(migrateDataTab('sqlite')).toBe('sqlite')
  expect(migrateDataTab('unknown')).toBe('redis-browser')
  expect(migrateDataTab('redis')).toBe('redis-browser')
  expect(migrateDataTab('redis-service')).toBe('redis-service')
})

test('fixed SQLite auto-opens separately from Connect and shares the standard sidebar', async ({
  page
}) => {
  const paths: string[] = []
  await page.route('**/api/v1/data/sqlite/default', route =>
    route.fulfill({ json: { path: '/workspace/data/default.sqlite' } })
  )
  await page.route('**/api/v1/data/sql', route => {
    paths.push(route.request().postDataJSON().path as string)
    return route.fulfill({
      json: { columns: ['name'], rows: [], affected: 0, truncated: false }
    })
  })
  await openData(page)
  const tabs = page.getByRole('tablist', { name: '数据导航' })
  await expect(tabs.getByRole('tab')).toHaveCount(3)
  await tabs.getByRole('tab', { name: 'SQLite', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '刷新表', exact: true })
  ).toBeEnabled()
  await expect(
    page.getByText('/workspace/data/default.sqlite', { exact: true })
  ).toBeVisible()
  await expect(page.getByLabel('SQLite 文件（服务器绝对路径）')).toHaveCount(0)
  expect(paths).toEqual(['/workspace/data/default.sqlite'])
  await tabs.getByRole('tab', { name: 'SQLite', exact: true }).press('End')
  await expect(tabs.getByRole('tab', { name: 'Connect' })).toBeFocused()
  await page.getByRole('button', { name: '新建连接', exact: true }).click()
  await expect(page.getByLabel('数据库类型', { exact: true })).toHaveValue(
    'mysql'
  )
  await expect(page.getByLabel('SQLite 文件（服务器绝对路径）')).toHaveCount(0)
  await page.getByRole('button', { name: '取消', exact: true }).click()
  await tabs.getByRole('tab', { name: 'SQLite', exact: true }).click()
  await expect.poll(() => paths.at(-1)).toBe('/workspace/data/default.sqlite')
})

async function openData(page: Page, fixture = 'data-workspace') {
  await page.route('**/api/v1/data/catalog', route =>
    route.fulfill({
      json: { sqlite: ['/tmp/demo.sqlite'], redisAddress: '127.0.0.1:6379' }
    })
  )
  await page.route('**/data-workspace-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      await import('/e2e/fixtures/${fixture}.tsx');
    </script>`
    })
  )
  await page.goto('/data-workspace-regression')
}

test('secondary sidebar restores Redis page and supports narrow-window keyboard navigation', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await openData(page)
  const subnav = page.getByRole('tablist', { name: 'Redis页面' })
  await expect(subnav.getByRole('tab')).toHaveCount(3)
  await subnav
    .getByRole('tab', { name: '数据浏览', exact: true })
    .press('ArrowDown')
  await expect(
    subnav.getByRole('tab', { name: '命令工作台', exact: true })
  ).toBeFocused()
  await expect(
    page.getByRole('textbox', { name: 'Redis 命令', exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('button', { name: '扫描键', exact: true })
  ).toBeHidden()
  await page
    .getByRole('textbox', { name: 'Redis 命令', exact: true })
    .fill('["GET","draft"]')
  await subnav.getByRole('tab', { name: '数据浏览', exact: true }).click()
  await subnav.getByRole('tab', { name: '命令工作台', exact: true }).click()
  await expect(
    page.getByRole('textbox', { name: 'Redis 命令', exact: true })
  ).toHaveValue('["GET","draft"]')
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await expect(subnav).toHaveCount(0)
  await page.getByRole('tab', { name: 'Redis', exact: true }).click()
  await expect(
    subnav.getByRole('tab', { name: '命令工作台', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('persist:alemonx-data'))
    )
    .toContain('redis-workbench')
  await page.reload()
  await expect(
    page.getByRole('textbox', { name: 'Redis 命令', exact: true })
  ).toBeVisible()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})

test('Redis global dialogs cancel drafts and only the top modal handles Escape', async ({
  page
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 })
  let nativeDialogs = 0
  let writes = 0
  page.on('dialog', async dialog => {
    nativeDialogs++
    await dialog.dismiss()
  })
  await page.route('**/api/v1/data/redis', route => {
    writes++
    return route.fulfill({ json: { value: 'OK' } })
  })
  await openData(page)
  await page.getByRole('button', { name: '连接配置', exact: true }).click()
  await page.getByLabel('Redis 地址', { exact: true }).fill('other:9999')
  await page.keyboard.press('Escape')
  await expect(page.getByLabel('当前连接')).not.toContainText('other:9999')
  await page.getByRole('button', { name: '新建键', exact: true }).click()
  const form = page.getByRole('dialog', { name: '新建 Redis 键', exact: true })
  await form.getByLabel('键名', { exact: true }).fill('cancelled')
  await form.getByLabel('初始值', { exact: true }).fill('retain me')
  await form.getByRole('button', { name: '创建并打开', exact: true }).click()
  await expect(
    page.getByRole('dialog', { name: '确认 Redis 数据变更', exact: true })
  ).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(
    page.getByRole('dialog', { name: '确认 Redis 数据变更', exact: true })
  ).toHaveCount(0)
  await expect(form).toBeVisible()
  await expect(form.getByLabel('初始值', { exact: true })).toHaveValue(
    'retain me'
  )
  await expect(
    form.getByRole('button', { name: '创建并打开', exact: true })
  ).toBeFocused()
  await page.screenshot({
    path: info.outputPath('redis-global-dialog-narrow.png')
  })
  await page.keyboard.press('Escape')
  await expect(form).toHaveCount(0)
  expect(nativeDialogs).toBe(0)
  expect(writes).toBe(0)
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})

test('settings no longer contains Redis service management', async ({
  page
}) => {
  await page.route('**/api/v1/**', route => route.fulfill({ json: {} }))
  await openData(page, 'data-settings')
  const settings = page.getByRole('tablist', { name: '设置页面' })
  await expect(settings).toBeVisible()
  await expect(settings.getByRole('tab', { name: 'Redis' })).toHaveCount(0)
  await expect(
    settings.getByRole('tab', { name: '服务', exact: true })
  ).toBeVisible()
})

test('Redis service management remains available when catalog fails and can save config', async ({
  page
}) => {
  const writes: unknown[] = []
  const status = {
    running: true,
    managed: true,
    external: false,
    port: 6379,
    autoStart: true,
    disabled: false,
    address: '127.0.0.1:6379',
    message: '本地服务运行中'
  }
  await page.route('**/api/v1/system/redis', route => {
    if (route.request().method() === 'PUT')
      writes.push(route.request().postDataJSON())
    return route.fulfill({ json: status })
  })
  await openData(page)
  await page.route('**/api/v1/data/catalog', route =>
    route.fulfill({ status: 503, json: { error: '暂不可用' } })
  )
  await page.reload()
  await expect(page.getByRole('alert')).toContainText('无法读取连接')
  await page.getByRole('tab', { name: '本地服务', exact: true }).click()
  await expect(page.getByText('Redis 已准备好', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '高级设置', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '停止', exact: true })
  ).toBeVisible()
  await page.getByLabel('端口', { exact: true }).fill('6380')
  await page.getByRole('button', { name: '保存配置', exact: true }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect(page.getByText('配置已保存。', { exact: true })).toBeVisible()
  expect(writes).toEqual([{ port: 6380, autoStart: true, disabled: false }])
})

test('external Redis service does not expose destructive local controls', async ({
  page
}) => {
  await page.route('**/api/v1/system/redis', route =>
    route.fulfill({
      json: {
        running: true,
        external: true,
        port: 6379,
        address: '127.0.0.1:6379'
      }
    })
  )
  await openData(page)
  await page.getByRole('tab', { name: '本地服务', exact: true }).click()
  await expect(
    page.getByText('正在使用已有 Redis', { exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '高级设置', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '停止', exact: true })
  ).toHaveCount(0)
  await expect(page.getByLabel('端口', { exact: true })).toHaveCount(0)
})

test('Connect tests, names, restores and forgets connections without writing data', async ({
  page
}) => {
  const requests: Array<{ sql: string; write?: boolean }> = []
  await page.route('**/api/v1/data/connect', route => {
    requests.push(route.request().postDataJSON())
    return route.fulfill({
      json: {
        columns: ['version'],
        rows: [['3']],
        affected: 0,
        truncated: false
      }
    })
  })
  await openData(page)
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await page.getByRole('button', { name: '新建连接', exact: true }).click()
  await page.getByLabel('连接名称', { exact: true }).fill('机器人业务库')
  await expect(
    page.getByRole('button', { name: '保存连接', exact: true })
  ).toBeDisabled()
  await page.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(
    page.getByText('连接测试成功，未修改数据库。', { exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '保存连接', exact: true }).click()
  await expect(
    page
      .getByRole('complementary', { name: '数据库连接' })
      .getByRole('button', { name: '机器人业务库 · mysql', exact: true })
  ).toBeVisible()
  await expect
    .poll(() =>
      page.evaluate(() => localStorage.getItem('persist:alemonx-data'))
    )
    .toContain('机器人业务库')
  await page.reload()
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await page
    .getByRole('complementary', { name: '数据库连接' })
    .getByRole('button', { name: '机器人业务库 · mysql', exact: true })
    .click()
  await expect(page.getByLabel('连接名称', { exact: true })).toHaveValue(
    '机器人业务库'
  )
  await page.getByRole('button', { name: '连接', exact: true }).click()
  await expect(page.getByLabel('当前连接')).toContainText('已连接')
  await page.getByRole('button', { name: '断开连接', exact: true }).click()
  await expect(page.getByLabel('当前连接')).toContainText('未连接')
  await expect(
    page.getByRole('button', { name: '执行查询', exact: true })
  ).toBeDisabled()
  await page.getByRole('button', { name: '忘记连接', exact: true }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect(
    page
      .getByRole('complementary', { name: '数据库连接' })
      .getByRole('button', { name: '机器人业务库 · mysql', exact: true })
  ).toHaveCount(0)
  expect(requests.every(request => !request.write)).toBe(true)
})

test('version 2 connection paths migrate without losing the saved database', async ({
  page
}) => {
  await page.addInitScript(() =>
    localStorage.setItem(
      'persist:alemonx-data',
      JSON.stringify({
        tab: JSON.stringify('connect'),
        sqlitePaths: JSON.stringify(['/tmp/legacy.sqlite']),
        _persist: JSON.stringify({ version: 2, rehydrated: true })
      })
    )
  )
  await openData(page)
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await expect(
    page
      .getByRole('complementary', { name: '数据库连接' })
      .getByRole('button', { name: '/tmp/legacy.sqlite · sqlite', exact: true })
  ).toBeVisible()
  await page
    .getByRole('button', { name: '/tmp/legacy.sqlite · sqlite', exact: true })
    .click()
  await expect(page.getByLabel('SQLite 文件（服务器绝对路径）')).toHaveValue(
    '/tmp/legacy.sqlite'
  )
})

test('failed connection test cannot save a new connection', async ({
  page
}) => {
  await page.route('**/api/v1/data/connect', route =>
    route.fulfill({ status: 400, json: { error: '数据库文件不存在' } })
  )
  await openData(page)
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await page.getByRole('button', { name: '新建连接', exact: true }).click()
  await page.getByLabel('连接名称', { exact: true }).fill('无效连接')
  await page.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('数据库文件不存在')
  await expect(
    page.getByRole('button', { name: '保存连接', exact: true })
  ).toBeDisabled()
})

test('Redis connection context and progressive configuration', async ({
  page
}) => {
  await page.route('**/api/v1/data/redis', route =>
    route.fulfill({ json: { value: 'PONG' } })
  )
  await openData(page)
  await expect(page.getByLabel('Redis 地址', { exact: true })).toBeHidden()
  await expect(page.getByLabel('Redis 命令', { exact: true })).toBeHidden()
  await page.getByRole('button', { name: '测试连接', exact: true }).click()
  await expect(page.getByLabel('当前连接')).toContainText('最近请求成功')
  await page.getByRole('button', { name: '连接配置', exact: true }).click()
  await page.getByLabel('Redis 地址', { exact: true }).fill('localhost:6380')
  await page.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByLabel('当前连接')).toContainText('未验证连接')
  await expect(page.getByLabel('当前连接')).toContainText('localhost:6380')
})

test('builtin SQLite tables, row SQL and write confirmation', async ({
  page
}) => {
  const writes: unknown[] = []
  await page.route('**/api/v1/data/sqlite/default', route =>
    route.fulfill({ json: { path: '/tmp/demo.sqlite' } })
  )
  await page.route('**/api/v1/data/sql', route => {
    const body = route.request().postDataJSON() as {
      sql: string
      write?: boolean
    }
    if (body.write) {
      writes.push(body)
      return route.fulfill({
        json: { columns: [], rows: [], affected: 1, truncated: false }
      })
    }
    const result = body.sql.includes('sqlite_schema')
      ? {
          columns: ['name', 'type', 'sql'],
          rows: [
            [
              'demo',
              'table',
              'CREATE TABLE demo(id INTEGER PRIMARY KEY, value TEXT)'
            ]
          ]
        }
      : body.sql.includes('pragma_table_xinfo')
        ? {
            columns: ['name', 'pk', 'hidden'],
            rows: [
              ['id', 1, 0],
              ['value', 0, 0]
            ]
          }
        : { columns: ['id', 'value'], rows: [[1, 'before']] }
    return route.fulfill({ json: { ...result, affected: 0, truncated: false } })
  })
  await openData(page)
  await page.getByRole('tab', { name: 'SQLite', exact: true }).click()
  await page.getByRole('button', { name: 'demo', exact: true }).click()
  await expect(
    page.getByRole('cell', { name: 'before', exact: true })
  ).toBeVisible()
  await page.getByRole('button', { name: '编辑行', exact: true }).click()
  await page
    .getByRole('dialog', { name: '编辑 SQLite 行' })
    .getByRole('textbox', { name: 'value', exact: true })
    .fill('"after"')
  await page.getByRole('button', { name: '生成变更 SQL', exact: true }).click()
  await expect(page.getByLabel('SQL 编辑器')).toHaveValue(
    /WHERE "id" IS 1 AND "value" IS 'before'/
  )
  await page.getByRole('button', { name: '执行写入', exact: true }).click()
  await page.getByRole('button', { name: '取消', exact: true }).click()
  expect(writes).toHaveLength(0)
  await page
    .getByLabel('SQL 编辑器')
    .fill("UPDATE demo SET value='after' WHERE id=1")
  await page.getByRole('button', { name: '执行写入', exact: true }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('影响 1 行')
  expect(writes).toHaveLength(1)
  expect(writes[0]).toMatchObject({
    path: '/tmp/demo.sqlite',
    write: true,
    confirmed: true
  })
  const persisted = await page.evaluate(() => JSON.stringify(localStorage))
  expect(persisted).not.toContain('/tmp/demo.sqlite')
  expect(persisted).not.toContain('UPDATE demo')
  expect(persisted).not.toContain('after')
})

test('Redis scan, inspect, preserve TTL and do not persist credentials', async ({
  page
}) => {
  const writes: Array<{ args: string[]; confirmed?: boolean }> = []
  await page.route('**/api/v1/data/redis', route => {
    const body = route.request().postDataJSON() as {
      args: string[]
      confirmed?: boolean
    }
    const command = body.args[0]
    if (command === 'SET') writes.push(body)
    const value =
      command === 'SCAN'
        ? ['0', ['hello']]
        : command === 'TYPE'
          ? 'string'
          : command === 'TTL'
            ? 120
            : command === 'GET'
              ? 'old value'
              : 'OK'
    return route.fulfill({ json: { value } })
  })
  await openData(page)
  await page.getByRole('button', { name: '连接配置', exact: true }).click()
  await page
    .getByLabel('密码（仅本次窗口）')
    .fill('never-persist-this-password')
  await page.getByRole('button', { name: '完成', exact: true }).click()
  await page.getByRole('button', { name: '扫描键', exact: true }).click()
  await expect(
    page.getByRole('button', { name: '下一批', exact: true })
  ).toBeDisabled()
  await page.getByRole('button', { name: 'hello', exact: true }).click()
  await expect(
    page.getByRole('textbox', { name: '字符串值', exact: true })
  ).toHaveValue('old value')
  await page
    .getByRole('textbox', { name: '字符串值', exact: true })
    .fill('new value')
  await page.route('**/api/v1/system/redis', route =>
    route.fulfill({ status: 503, json: {} })
  )
  await page.getByRole('tab', { name: '本地服务', exact: true }).click()
  await page.getByRole('tab', { name: '数据浏览', exact: true }).click()
  await expect(
    page.getByRole('textbox', { name: '字符串值', exact: true })
  ).toHaveValue('new value')
  await page.getByRole('button', { name: '保存字符串（保留 TTL）' }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect(page.getByLabel('Redis 执行结果')).toContainText('OK')
  expect(writes).toEqual([
    expect.objectContaining({
      args: ['SET', 'hello', 'new value', 'KEEPTTL'],
      confirmed: true,
      expected: 'old value'
    })
  ])
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain(
    'never-persist-this-password'
  )
  await page.getByRole('tab', { name: 'Connect' }).click()
  await page.getByRole('tab', { name: 'Redis', exact: true }).click()
  await page.getByRole('button', { name: '连接配置', exact: true }).click()
  await expect(page.getByLabel('密码（仅本次窗口）')).toHaveValue('')
})

test('Redis uses structure-specific editors and confirms their writes', async ({
  page
}) => {
  const writes: string[][] = []
  await page.route('**/api/v1/data/redis', route => {
    const { args } = route.request().postDataJSON() as { args: string[] }
    if (['HSET', 'HDEL'].includes(args[0])) writes.push(args)
    const value =
      args[0] === 'SCAN'
        ? ['0', ['prefs']]
        : args[0] === 'TYPE'
          ? 'hash'
          : args[0] === 'TTL'
            ? -1
            : args[0] === 'HSCAN'
              ? ['0', ['theme', 'dark']]
              : 'OK'
    return route.fulfill({ json: { value } })
  })
  await openData(page)
  await page.getByRole('button', { name: '扫描键', exact: true }).click()
  await page.getByRole('button', { name: 'prefs', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Hash 编辑器' })).toContainText(
    'theme'
  )
  await page
    .getByRole('region', { name: 'Hash 编辑器' })
    .getByLabel('字段', { exact: true })
    .fill('locale')
  await page
    .getByRole('region', { name: 'Hash 编辑器' })
    .getByLabel('值', { exact: true })
    .fill('zh-CN')
  await page.getByRole('button', { name: '保存字段', exact: true }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect
    .poll(() => writes)
    .toContainEqual(['HSET', 'prefs', 'locale', 'zh-CN'])
})

test('Redis creates a typed key through the guarded data API', async ({
  page
}) => {
  const writes: string[][] = []
  await page.route('**/api/v1/data/redis', route => {
    const { args } = route.request().postDataJSON() as { args: string[] }
    if (args[0] === 'SET') writes.push(args)
    const value =
      args[0] === 'TYPE'
        ? 'string'
        : args[0] === 'TTL'
          ? -1
          : args[0] === 'GET'
            ? 'hello'
            : args[0] === 'SCAN'
              ? ['0', []]
              : 'OK'
    return route.fulfill({ json: { value } })
  })
  await openData(page)
  await page.getByRole('button', { name: '新建键', exact: true }).click()
  await page
    .getByRole('region', { name: '新建 Redis 键' })
    .getByLabel('键名', { exact: true })
    .fill('welcome')
  await page
    .getByRole('region', { name: '新建 Redis 键' })
    .getByLabel('初始值', { exact: true })
    .fill('hello')
  await page.getByRole('button', { name: '创建并打开', exact: true }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect.poll(() => writes).toContainEqual(['SET', 'welcome', 'hello'])
  await expect(
    page.getByRole('heading', { name: 'welcome · string', exact: true })
  ).toBeVisible()
})

test('failed Redis value load cannot be saved; narrow viewport remains usable', async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/v1/data/redis', route => {
    const { args } = route.request().postDataJSON() as { args: string[] }
    if (args[0] === 'GET')
      return route.fulfill({
        status: 400,
        json: { error: '该值包含二进制数据，不能作为 UTF-8 文本编辑' }
      })
    return route.fulfill({
      json: {
        value:
          args[0] === 'SCAN'
            ? ['0', ['binary']]
            : args[0] === 'TYPE'
              ? 'string'
              : -1
      }
    })
  })
  await openData(page)
  await page.getByRole('button', { name: '扫描键', exact: true }).click()
  await page.getByRole('button', { name: 'binary', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('二进制')
  await expect(
    page.getByRole('button', { name: '保存字符串（保留 TTL）' })
  ).toBeDisabled()
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth
    )
  ).toBe(true)
})

test('connection failure offers retry without showing database contents', async ({
  page
}) => {
  await openData(page)
  await page.route('**/api/v1/data/catalog', route =>
    route.fulfill({ status: 403, json: { error: '权限不足' } })
  )
  await page.reload()
  await expect(page.getByRole('alert')).toContainText('无法读取连接')
  await expect(
    page.getByRole('button', { name: '重试', exact: true })
  ).toBeVisible()
})

test('system SQLite connections disable all write entry points', async ({
  page
}) => {
  await page.route('**/api/v1/data/connect', route =>
    route.fulfill({
      json: {
        columns: ['name'],
        rows: [],
        affected: 0,
        truncated: false,
        systemDatabase: true
      }
    })
  )
  await openData(page)
  await page.getByRole('tab', { name: 'Connect', exact: true }).click()
  await page.getByRole('button', { name: '新建连接', exact: true }).click()
  await page.getByLabel('数据库类型', { exact: true }).selectOption('sqlite')
  await page.getByLabel('SQLite 文件（服务器绝对路径）').fill('/tmp/system.db')
  await page.getByRole('button', { name: '连接', exact: true }).click()
  await expect(
    page.getByText('系统库 · 强制只读', { exact: true })
  ).toBeVisible()
  await expect(
    page.getByRole('checkbox', { name: '允许写入（执行时需确认）' })
  ).toBeDisabled()
  await expect(
    page.getByRole('button', { name: '执行查询', exact: true })
  ).toBeEnabled()
})

test('Redis conflicts preserve the unsaved editor and request a reload', async ({
  page
}) => {
  await page.route('**/api/v1/data/redis', route => {
    const { args, expected } = route.request().postDataJSON() as {
      args: string[]
      expected?: string
    }
    if (args[0] === 'SET') {
      expect(expected).toBe('original')
      return route.fulfill({
        status: 409,
        json: {
          error: 'Redis 值已被其他客户端修改或已过期，请重新读取后再保存'
        }
      })
    }
    return route.fulfill({
      json: {
        value:
          args[0] === 'SCAN'
            ? ['0', ['hello']]
            : args[0] === 'TYPE'
              ? 'string'
              : args[0] === 'TTL'
                ? -1
                : 'original'
      }
    })
  })
  await openData(page)
  await page.getByRole('button', { name: '扫描键', exact: true }).click()
  await page.getByRole('button', { name: 'hello', exact: true }).click()
  await expect(
    page.getByRole('textbox', { name: '字符串值', exact: true })
  ).toHaveValue('original')
  await page
    .getByRole('textbox', { name: '字符串值', exact: true })
    .fill('unsaved')
  await page.getByRole('button', { name: '保存字符串（保留 TTL）' }).click()
  await page.getByRole('button', { name: '确认继续', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('重新读取')
  await expect(
    page.getByRole('textbox', { name: '字符串值', exact: true })
  ).toHaveValue('unsaved')
})

test('workbench lower-left data entry opens a standalone window without a robot', async ({
  page
}, testInfo) => {
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v1/auth/status')
      return route.fulfill({ json: { enabled: false, authenticated: false } })
    if (path === '/api/v1/data/catalog')
      return route.fulfill({
        json: { sqlite: [], redisAddress: '127.0.0.1:6379' }
      })
    return route.fulfill({
      status: 503,
      json: { error: '测试中未启用其他服务' }
    })
  })
  await page.route('**/data-entry-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      await import('/e2e/fixtures/data-entry.tsx');
    </script>`
    })
  )
  await page.goto('/data-entry-regression')
  await page
    .getByLabel('系统功能目录')
    .getByRole('button', { name: '数据', exact: true })
    .click()
  await expect(page.getByRole('region', { name: '数据管理' })).toBeVisible()
  await expect(
    page.getByRole('button', { name: '扫描键', exact: true })
  ).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath('data-window.png') })
})
