import { expect, test, type Locator } from '@playwright/test'

function painted(editor: Locator) {
  return editor.locator('..').locator('.source-textarea-paint')
}

test('project configuration editors highlight file syntax and save plain source', async ({ page }) => {
  const root = '/fixture/highlight-bot'
  const sourceFiles = new Map([
    ['alemon.config.yaml', 'port: 17117\n'],
    ['.env', '# Local configuration\nexport PORT=17117\n'],
    ['.npmrc', '@sample:registry=https://example.test/\n']
  ])
  await page.setViewportSize({ width: 1360, height: 850 })
  page.setDefaultTimeout(6000)
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    const target = new URL(request.url())
    switch (target.pathname) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/workspace/robots':
        return route.fulfill({
          json: {
            scope: 'source-textareas',
            initialized: true,
            local: true,
            activeProjectID: 'bot',
            projects: [{ id: 'bot', name: 'highlight-bot', path: root }]
          }
        })
      case '/api/v1/robot': {
        if (request.method() === 'PUT') {
          const { file, content } = request.postDataJSON()
          sourceFiles.set(file, content)
          return route.fulfill({ json: { output: '已保存' } })
        }
        return route.fulfill({
          json: { output: sourceFiles.get(target.searchParams.get('file') ?? '') ?? '', revision: 'fixture' }
        })
      }
      case '/api/v1/robot/manifest':
        return route.fulfill({
          json: { name: 'highlight-bot', alemonjsConfig: [{ name: 'port', type: 'number' }] }
        })
      case '/api/v1/robot/scripts':
        return route.fulfill({ json: { items: [{ name: 'example', command: 'echo "ready"', running: false }] } })
      case '/api/v1/robot/pm2-registration':
        return route.fulfill({ json: { name: 'highlight-bot', script: './index.js', env: { PORT: '17117' } } })
      case '/api/v1/robot/tasks':
      case '/api/v1/goals':
      case '/api/v1/setup/plugins':
      case '/api/v1/catalog':
        return route.fulfill({ json: [] })
      default:
        if (target.pathname.endsWith('/stream'))
          return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' })
        return route.fulfill({ status: 503, json: { error: '测试中未启用此服务' } })
    }
  })

  const dashboard = `/dashboard/robot?root=${encodeURIComponent(root)}`
  await page.goto(`${dashboard}&page=robot&section=config&editor=text`)
  const yaml = page.getByRole('textbox', { name: '编辑 alemon.config.yaml' })
  await expect(painted(yaml).locator('.hljs-attr')).toHaveText('port:')
  const nextYaml = 'port: 17118\nname: "<img src=x>"\n'
  await yaml.fill(nextYaml)
  await expect(painted(yaml)).toContainText('<img src=x>')
  await expect(painted(yaml).locator('img')).toHaveCount(0)
  await expect.poll(() => sourceFiles.get('alemon.config.yaml')).toBe(nextYaml)

  const actions = page.getByRole('button', { name: '操作', exact: true })
  await actions.click()
  await page.getByRole('region', { name: '操作菜单' }).getByRole('button', { name: '开发模式' }).click()
  await expect(page.getByRole('button', { name: '开发模式', exact: true })).toHaveAttribute('aria-pressed', 'true')

  const robotMenu = page.getByRole('navigation', { name: '机器人功能', exact: true })
  const openConfig = async (name: string) => {
    await robotMenu.getByRole('button', { name: '配置', exact: true }).click()
    await page.getByRole('group', { name: '配置子菜单' }).getByRole('button', { name, exact: true }).click()
  }
  await actions.click()
  await openConfig('npm 源')
  await page.getByRole('tablist', { name: '编辑模式', exact: true }).getByRole('tab', { name: '文本' }).click()
  const npmrc = page.getByRole('textbox', { name: '编辑 .npmrc' })
  await expect(painted(npmrc).locator('.hljs-attr')).toHaveText('@sample:registry')

  await page.goto(`${dashboard}&page=build&build=manifest`)
  await page.locator('summary').filter({ hasText: '高级 JSON 编辑' }).click()
  const json = page.getByRole('textbox', { name: '编辑 AlemonJS 配置声明 JSON' })
  await expect(painted(json).locator('.hljs-attr').first()).toHaveText('"name"')

  await page.goto(`${dashboard}&page=robot&section=runtime`)
  const scripts = page.getByRole('button', { name: /脚本控制/ })
  await scripts.click()
  await scripts.locator('..').getByRole('button', { name: '编辑', exact: true }).click()
  const shell = page.getByRole('dialog', { name: '编辑脚本' }).getByRole('textbox', { name: '命令' })
  await expect(painted(shell).locator('.hljs-string')).toHaveText('"ready"')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '修改', exact: true }).click()
  const pm2 = page.getByRole('dialog', { name: '修改后台运行配置' })
    .getByRole('textbox', { name: '环境变量（每行 KEY=value）' })
  await expect(painted(pm2).locator('.hljs-attr')).toHaveText('PORT')
  await page.keyboard.press('Escape')

  await openConfig('环境变量')
  await page.getByRole('tablist', { name: '.env 编辑模式' }).getByRole('tab', { name: '文本' }).click()
  await page.setViewportSize({ width: 390, height: 760 })
  const env = page.getByRole('textbox', { name: '编辑 .env' })
  await expect(painted(env).locator('.hljs-attr')).toHaveText('PORT')
  await expect(painted(env).locator('.hljs-comment')).toHaveText('# Local configuration')
  const longEnv = `TOKEN=${'a'.repeat(400)}\n${'PORT=17117\n'.repeat(30)}`
  await env.fill(longEnv)
  await expect.poll(() => sourceFiles.get('.env')).toBe(longEnv)
  const geometry = await env.evaluate(input => {
    const textarea = input as HTMLTextAreaElement
    const layer = textarea.parentElement!.querySelector('pre')!
    return {
      input: textarea.clientWidth,
      paint: layer.clientWidth,
      inputHeight: textarea.scrollHeight,
      paintHeight: layer.scrollHeight
    }
  })
  expect(geometry.paint).toBe(geometry.input)
  expect(Math.abs(geometry.paintHeight - geometry.inputHeight)).toBeLessThanOrEqual(2)
})
