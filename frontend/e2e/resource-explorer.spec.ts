import { expect, test } from '@playwright/test'

test('resource explorer opens project files and manages staged Git changes', async ({
  page
}) => {
  const root = '/fixture/bot'
  const otherRoot = '/fixture/other'
  const otherSource = 'export const other = true\n'
  const extraPaths = Array.from(
    { length: 32 },
    (_, index) => `extra-${index + 1}.ts`
  )
  let savedText = 'export const fixture = 1\r\n'
  let revision = 'original'
  const readmeText = [
    '# 使用指南',
    '- 第一项',
    '- 第二项',
    '',
    '```sh',
    'npm test',
    '```',
    '',
    '[文档](https://example.com/docs)',
    '[无效链接](javascript:alert%281%29)',
    '<button>仅作原文显示</button>'
  ].join('\n')
  let staged = false
  let committed = false
  let terminalCount = 0
  const actions: string[] = []
  const createdEntries: { name: string; path: string; directory: boolean }[] = []
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.setViewportSize({ width: 1792, height: 900 })
  await page.route('**/api/v1/**', route => {
    const request = route.request()
    const target = new URL(request.url())
    switch (target.pathname) {
      case '/api/v1/goals':
      case '/api/v1/robot/tasks':
        return route.fulfill({ json: [] })
      case '/api/v1/terminal/sessions':
        return route.fulfill({
          json: {
            id: `explorer-terminal-${++terminalCount}`,
            key: 'fixture',
            cwd: root,
            shell: '/bin/sh'
          }
        })
      case '/api/v1/terminal/sessions/explorer-terminal-1/stream':
      case '/api/v1/terminal/sessions/explorer-terminal-2/stream':
      case '/api/v1/terminal/sessions/explorer-terminal-3/stream':
        return route.fulfill({
          contentType: 'text/event-stream',
          body: ': fixture\n\n'
        })
      case '/api/v1/checks':
        return route.fulfill({ json: { checks: [] } })
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/workspace/robots':
        return route.fulfill({
          json: {
            scope: 'test',
            initialized: true,
            local: true,
            projects: [
              { id: 'bot', name: 'bot', path: root },
              { id: 'other', name: 'other', path: otherRoot }
            ],
            activeProjectID: 'bot'
          }
        })
      case '/api/v1/robot/validate':
        return route.fulfill({ json: { valid: true } })
      case '/api/v1/workspace/project-files':
        if (target.searchParams.get('root') === otherRoot) {
          return route.fulfill({ json: target.searchParams.get('file') === 'src/index.ts'
            ? { content: otherSource, revision: 'other' }
            : { entries: [] }
          })
        }
        expect(target.searchParams.get('root')).toBe(root)
        if (request.method() === 'POST') {
          const { name, directory } = request.postDataJSON()
          if (name === 'README.md') return route.fulfill({
            status: 409,
            json: { error: '项目中已有同名文件或目录。' }
          })
          const entry = { name, path: name, directory }
          createdEntries.push(entry)
          return route.fulfill({ status: 201, json: entry })
        }
        if (request.method() === 'PUT') {
          const update = request.postDataJSON()
          if (update.revision !== revision)
            return route.fulfill({
              status: 409,
              json: { error: '文件已被修改' }
            })
          savedText = update.content
          revision = 'saved'
          return route.fulfill({ json: { content: savedText, revision } })
        }
        return route.fulfill({
          json: target.searchParams.has('file')
            ? target.searchParams.get('file') === 'README.md'
              ? { content: readmeText, revision: 'readme' }
              : target.searchParams.get('file') === 'new.ts'
                ? { content: '', revision: 'new-file' }
                : { content: savedText, revision }
            : target.searchParams.get('directory') === 'src'
              ? {
                  entries: [
                    {
                      name: 'index.ts',
                      path: 'src/index.ts',
                      directory: false
                    },
                    { name: 'nested', path: 'src/nested', directory: true }
                  ]
                }
              : {
                  entries: [
                    { name: 'README.md', path: 'README.md', directory: false },
                    { name: 'src', path: 'src', directory: true },
                    ...createdEntries,
                    ...extraPaths.map(path => ({
                      name: path,
                      path,
                      directory: false
                    }))
                  ]
                }
        })
      case '/api/v1/robot/git':
        if (request.method() === 'POST') {
          const input = request.postDataJSON()
          expect(input.root).toBe(root)
          actions.push(input.action)
          if (input.action === 'commit') {
            expect(input.indexRevision).toBe('staged-revision')
            expect(input.message).toBe('验证资源管理器')
            committed = true
          }
          if (input.action === 'stage') staged = true
          return route.fulfill({ json: { output: '已完成' } })
        }
        return route.fulfill({
          json: {
            root: target.searchParams.get('root'),
            repository: true,
            branch: target.searchParams.get('root') === otherRoot ? 'release' : 'main',
            atRepositoryRoot: true,
            authorName: 'Git Fixture',
            authorEmail: 'fixture@example.test',
            changes: [
              ...(committed
                ? []
                : [
                    {
                      path: 'src/index.ts',
                      status: 'M',
                      staged,
                      unstaged: !staged
                    }
                  ]),
              ...extraPaths.map(path => ({
                path,
                status: 'M',
                staged: false,
                unstaged: true
              }))
            ],
            indexRevision: 'staged-revision',
            branches: [],
            remoteBranches: [],
            commits: [],
            tags: [],
            stashes: [],
            remotes: []
          }
        })
      case '/api/v1/robot/git/diff':
        return route.fulfill({
          json: {
            path: 'src/index.ts',
            status: 'M',
            diff: '@@ -1 +1 @@\n-export const initial = 0\n+export const changed = 1'
          }
        })
      default:
        return route.fulfill({ json: {} })
    }
  })
  await page.goto('/dashboard/robot')
  const rail = page.getByRole('navigation', { name: '工作台工具' })
  await rail.getByRole('button', { name: '资源管理器' }).click()
  const files = page.getByRole('complementary', { name: '机器人项目文件' })
  const preview = page.getByRole('region', { name: '文件内容' })
  const header = page.locator('.workbench-content-header')
  const auxiliaryToggle = header.getByRole('button', { name: '辅助标签页' })
  const auxiliary = page.locator('.workbench-split-terminal')
  const git = auxiliary.getByRole('region', { name: 'Git 管理' })
  const launcher = auxiliary.getByRole('region', { name: '新标签页' })
  await expect(auxiliaryToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(header.getByRole('button', { name: '打开终端' })).toHaveCount(0)
  await expect(header.getByRole('button', { name: '打开仓库' })).toHaveCount(0)
  await expect(auxiliary).toBeHidden()
  await auxiliaryToggle.click()
  await expect(launcher.getByRole('button')).toHaveText(['终端', 'Git 管理'])
  await launcher.getByRole('button', { name: 'Git 管理', exact: true }).click()
  await expect(git.getByRole('tab', { name: '变更' })).toBeVisible()

  const moreGit = git.getByRole('button', { name: '仓库更多操作' })
  const gitMenu = git.getByRole('region', { name: '仓库操作菜单' })
  await moreGit.click()
  await expect(gitMenu.getByRole('button')).toHaveText(['刷新 Git 状态', '提交身份', '临时保存'])
  const triggerBounds = (await moreGit.boundingBox())!
  const menuBounds = (await gitMenu.boundingBox())!
  expect(menuBounds.y - (triggerBounds.y + triggerBounds.height)).toBeLessThanOrEqual(12)
  expect(menuBounds.y).toBeGreaterThanOrEqual(triggerBounds.y + triggerBounds.height)
  await gitMenu.getByRole('button', { name: '提交身份' }).focus()
  await page.keyboard.press('Escape')
  await expect(gitMenu).toBeHidden()
  await expect(moreGit).toBeFocused()

  const chooseRepository = git.getByRole('button', { name: '选择 Git 仓库，当前 bot' })
  const chooseOtherRepository = git.getByRole('button', { name: '选择 Git 仓库，当前 other' })
  const repoPicker = git.getByRole('region', { name: '选择 Git 仓库' })
  await chooseRepository.click()
  await expect(repoPicker.getByRole('button', { name: /bot/ })).toHaveAttribute('aria-current', 'true')
  await repoPicker.getByRole('button', { name: /other/ }).click()
  await expect(git.getByRole('button', { name: '切换 Git 分支' })).toHaveText('release')
  const diff = page.getByRole('region', { name: '文件差异' })
  const viewChange = git.getByRole('button', { name: '查看 src/index.ts 的未暂存差异' })
  await viewChange.click()
  await expect(diff.getByText('+export const changed = 1')).toBeVisible()
  await expect(git.getByRole('region', { name: '文件差异' })).toHaveCount(0)
  await diff.getByRole('button', { name: '打开文件' }).click()
  await expect(preview.getByRole('textbox', { name: '编辑 src/index.ts' })).toHaveValue(otherSource)
  await expect(diff).toHaveCount(0)
  await header.getByRole('button', { name: '关闭文件 src/index.ts' }).click()
  await chooseOtherRepository.click()
  await repoPicker.getByRole('button', { name: /bot/ }).click()
  await expect(git.getByRole('button', { name: '切换 Git 分支' })).toHaveText('main')
  await viewChange.click()
  await expect(diff.getByText('+export const changed = 1')).toBeVisible()
  await expect(diff.getByRole('button', { name: '关闭差异', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(diff).toHaveCount(0)
  await expect(viewChange).toBeFocused()

  await expect(launcher).toBeHidden()
  await expect(page.locator('.robot-workspace-list').getByRole('complementary', {
    name: '机器人项目文件'
  })).toBeVisible()
  await expect(page.locator('.robot-workspace-list .project-rail')).toHaveCount(0)
  await expect(files).toBeVisible()
  expect((await files.boundingBox())!.x).toBeLessThan(
    (await preview.boundingBox())!.x
  )
  expect((await preview.boundingBox())!.x).toBeLessThan(
    (await git.boundingBox())!.x
  )
  const fileTitle = files.getByRole('heading', { name: '目录', exact: true })
  const gitTitle = auxiliary.getByRole('button', {
    name: 'Git 管理',
    exact: true
  })
  const fileTitleTop = (await fileTitle.boundingBox())!.y
  const gitTitleTop = (await gitTitle.boundingBox())!.y
  const lastFile = files.getByRole('button', {
    name: 'extra-32.ts',
    exact: true
  })
  const changes = git.getByRole('region', { name: '未暂存', exact: true })
  const gitBody = git.getByRole('tabpanel', { name: '变更' })
  const lastChange = changes.getByRole('button', {
    name: '暂存 extra-32.ts',
    exact: true
  })
  const fixedGitControls = [
    git.getByRole('button', { name: '切换 Git 分支' }),
    git.getByRole('tablist', { name: 'Git 功能' })
  ]
  for (const icon of [
    files.getByRole('button', { name: '新建文件', exact: true }),
    files.getByRole('button', { name: '新建目录', exact: true })
  ]) {
    await expect(icon).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
    await expect(icon).toHaveCSS('border-width', '0px')
  }
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  for (const item of [fileTitle, gitTitle]) {
    await expect(item).toHaveCSS('color', 'rgb(230, 221, 208)')
  }
  for (const control of fixedGitControls) await expect(control).toBeInViewport()
  const controlTops = await Promise.all(
    fixedGitControls.map(async control => (await control.boundingBox())!.y)
  )
  await expect(lastFile).not.toBeInViewport()
  await expect(lastChange).not.toBeInViewport()
  await lastFile.scrollIntoViewIfNeeded()
  await lastChange.scrollIntoViewIfNeeded()
  await expect(lastFile).toBeInViewport()
  await expect(lastChange).toBeInViewport()
  expect((await fileTitle.boundingBox())!.y).toBe(fileTitleTop)
  expect((await gitTitle.boundingBox())!.y).toBe(gitTitleTop)
  expect(await gitBody.evaluate(body => body.scrollTop)).toBeGreaterThan(0)
  for (const [index, control] of fixedGitControls.entries()) {
    expect((await control.boundingBox())!.y).toBe(controlTops[index])
  }
  const folder = files.getByText('src', { exact: true })
  const readme = files.getByRole('button', { name: 'README.md', exact: true })
  expect((await folder.boundingBox())!.y).toBeLessThan(
    (await readme.boundingBox())!.y
  )
  await folder.click()
  const indexFile = files.getByRole('button', { name: 'index.ts', exact: true })
  const nestedFolder = files.getByText('nested', { exact: true })
  await expect(nestedFolder).toBeVisible()
  expect((await nestedFolder.boundingBox())!.y).toBeLessThan(
    (await indexFile.boundingBox())!.y
  )
  await indexFile.click()
  const editor = preview.getByRole('textbox', { name: '编辑 src/index.ts' })
  const saveFile = preview.getByRole('button', { name: '保存', exact: true })
  const tabs = header.getByRole('navigation', { name: '已打开文件' })
  await expect(
    tabs.getByRole('button', { name: 'index.ts', exact: true })
  ).toHaveAttribute('aria-current', 'page')
  const tabBounds = (await tabs.boundingBox())!
  const directoryButton = (await header
    .getByRole('button', { name: '折叠侧边栏', exact: true })
    .boundingBox())!
  const actionsButton = (await header
    .getByRole('button', { name: '操作', exact: true })
    .boundingBox())!
  expect(tabBounds.x).toBeGreaterThanOrEqual(
    directoryButton.x + directoryButton.width
  )
  expect(tabBounds.x + tabBounds.width).toBeLessThanOrEqual(actionsButton.x)
  await expect(
    preview.getByRole('navigation', { name: '已打开文件' })
  ).toHaveCount(0)
  await expect(editor).toHaveValue('export const fixture = 1\n')
  await expect(editor).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  const markdownToggle = preview.getByRole('button', { name: '预览 Markdown' })
  await expect(markdownToggle).toHaveCount(0)
  await expect(saveFile).toBeDisabled()
  await expect(saveFile).toHaveText('')
  const paintedSource = preview.locator('.source-textarea-paint')
  await expect(paintedSource.locator('.hljs-keyword').first()).toHaveText('export')
  const longSource = `const html = '${'<img src=x>'.repeat(120)}'\n${'// next line\n'.repeat(80)}`
  await editor.fill(longSource)
  await expect(paintedSource).toContainText('<img src=x>')
  await expect(paintedSource.locator('img')).toHaveCount(0)
  await editor.evaluate(node => node.scrollTo({ left: 180, top: 240 }))
  await expect(paintedSource).toHaveCSS('translate', '-180px -240px')
  await editor.fill('x'.repeat(64 * 1024 + 1))
  await expect(paintedSource).toHaveCount(0)
  await editor.fill('export const fixture = 2\n')
  await expect(saveFile).toBeEnabled()
  await expect(editor).toHaveCSS('outline-offset', '-2px')
  await expect(editor).toHaveCSS('box-shadow', 'none')
  await readme.click()
  const markdownSource = preview.getByRole('textbox', { name: '编辑 README.md' })
  const renderedMarkdown = preview.getByRole('article', { name: 'Markdown 预览' })
  await expect(markdownSource).toHaveValue(readmeText)
  await expect(markdownToggle).toHaveCSS('border-width', '0px')
  await markdownToggle.click()
  await expect(markdownToggle).toHaveAttribute('aria-pressed', 'true')
  await expect(markdownSource).toBeHidden()
  await expect(renderedMarkdown.getByRole('heading', { name: '使用指南', level: 1 })).toBeVisible()
  await expect(renderedMarkdown.getByRole('listitem')).toHaveText(['第一项', '第二项'])
  await expect(renderedMarkdown.locator('pre')).toHaveText('npm test')
  await expect(renderedMarkdown.getByRole('link', { name: '文档' })).toHaveAttribute('href', 'https://example.com/docs')
  await expect(renderedMarkdown.getByRole('link', { name: '无效链接' })).toHaveCount(0)
  await expect(renderedMarkdown.getByRole('button', { name: '仅作原文显示' })).toHaveCount(0)
  await markdownToggle.click()
  await markdownSource.fill('# 未保存的草稿')
  await markdownToggle.click()
  await expect(renderedMarkdown.getByRole('heading', { name: '未保存的草稿' })).toBeVisible()
  await expect(preview.getByRole('status')).toHaveText('未保存')
  await markdownToggle.click()
  await expect(markdownSource).toHaveValue('# 未保存的草稿')
  await markdownSource.fill(readmeText)
  await tabs.getByRole('button', { name: /^index\.ts/ }).click()
  await expect(editor).toHaveValue('export const fixture = 2\n')
  await tabs.getByRole('button', { name: '关闭文件 src/index.ts' }).click()
  await page
    .getByRole('dialog', { name: '关闭未保存的文件' })
    .getByRole('button', { name: '取消', exact: true })
    .click()
  await saveFile.click()
  await expect(preview.getByRole('status')).toHaveText('已保存')
  expect(savedText).toBe('export const fixture = 2\r\n')

  await editor.fill('export const draft = 3\n')
  savedText = 'export const external = 4\r\n'
  revision = 'external'
  await editor.press('Control+s')
  await expect(preview.getByRole('alert')).toContainText('磁盘上的文件已更改')
  await expect(editor).toHaveValue('export const draft = 3\n')
  expect(savedText).toBe('export const external = 4\r\n')
  await preview.getByRole('button', { name: '刷新文件内容' }).click()
  const confirmReload = page
    .getByRole('dialog', { name: '重新加载磁盘版本' })
    .getByRole('button', { name: '重新加载', exact: true })
  await expect(confirmReload).toHaveText('')
  await expect(confirmReload).toHaveAttribute('title', '重新加载')
  await expect(confirmReload.locator('svg')).toBeVisible()
  await confirmReload.click()
  await expect(editor).toHaveValue('export const external = 4\n')

  await expect(git.getByText('main')).toBeVisible()
  await git.getByRole('button', { name: '暂存 src/index.ts', exact: true }).click()
  await expect(
    git.getByRole('button', { name: '取消暂存 src/index.ts' })
  ).toBeVisible()
  await git
    .getByRole('textbox', { name: '提交说明' })
    .fill('验证资源管理器')
  await git.getByRole('button', { name: '提交 1 个文件' }).click()
  await expect.poll(() => actions).toEqual(['stage'])
  await page
    .getByRole('dialog', { name: '提交已暂存内容' })
    .getByRole('button', { name: '确认提交', exact: true })
    .click()
  await expect(changes.getByRole('heading', { name: /未暂存\s*32/ })).toBeVisible()
  expect(actions).toEqual(['stage', 'commit'])

  await auxiliary.getByRole('button', { name: '新标签页' }).click()
  await launcher.getByRole('button', { name: '终端', exact: true }).click()
  const session = auxiliary.getByRole('button', { name: 'bot', exact: true })
  await expect(session).toHaveAttribute('aria-pressed', 'true')
  const terminalMore = auxiliary.getByRole('button', { name: '终端更多操作' })
  await expect(auxiliary.getByRole('button', { name: '添加终端' })).toHaveCount(0)
  await terminalMore.click()
  await expect(auxiliary.getByRole('region', { name: '终端操作菜单' }).getByRole('button')).toHaveText(['复制输出', '下载输出'])
  await expect(git).toBeHidden()
  await gitTitle.click()
  await expect(git).toBeVisible()
  await expect(terminalMore).toHaveCount(0)
  await expect(session).toHaveAttribute('aria-pressed', 'false')
  await session.click()
  await expect(session).toHaveAttribute('aria-pressed', 'true')
  await auxiliary.getByRole('button', { name: '新标签页' }).click()
  await expect(session).toHaveAttribute('aria-pressed', 'false')
  await launcher.getByRole('button', { name: '终端', exact: true }).click()
  await expect(session).toHaveCount(2)
  await expect(session.last()).toHaveAttribute('aria-pressed', 'true')
  await auxiliary.getByRole('button', { name: '关闭终端 bot', exact: true }).last().click()
  await expect(session).toHaveCount(1)
  await expect(session).toHaveAttribute('aria-pressed', 'true')
  await auxiliary.getByRole('button', { name: '关闭终端 bot', exact: true }).click()
  await expect(git).toBeVisible()
  await expect(session).toHaveCount(0)

  await rail.getByRole('button', { name: '主页', exact: true }).click()
  await expect(tabs).toHaveCount(0)
  await page.setViewportSize({ width: 390, height: 760 })
  const actionsMenu = header.getByRole('button', { name: '操作', exact: true })
  await actionsMenu.click()
  await header.getByRole('region', { name: '操作菜单', exact: true })
    .getByRole('button', { name: '切换到亮色' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await actionsMenu.click()
  await expect(auxiliaryToggle).toBeInViewport()
  await expect(git).toBeHidden()
  await auxiliaryToggle.click()
  await expect(launcher.getByRole('button')).toHaveText(['终端', 'Git 管理', '目录'])
  await launcher.getByRole('button', { name: 'Git 管理', exact: true }).click()
  await expect(changes.getByRole('heading', { name: /未暂存\s*32/ })).toBeVisible()
  await moreGit.click()
  const narrowMenu = (await gitMenu.boundingBox())!
  expect(narrowMenu.x).toBeGreaterThanOrEqual(0)
  expect(narrowMenu.x + narrowMenu.width).toBeLessThanOrEqual(390)
  await gitMenu.getByRole('button', { name: '临时保存' }).click()
  await expect(gitMenu).toBeHidden()
  const stash = page.getByRole('dialog', { name: '临时保存' })
  await expect(stash).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(stash).toBeHidden()
  await chooseRepository.click()
  await expect(repoPicker).toBeInViewport()
  const repoTrigger = (await chooseRepository.boundingBox())!
  const repoMenu = (await repoPicker.boundingBox())!
  expect(repoMenu.y).toBeGreaterThanOrEqual(repoTrigger.y + repoTrigger.height)
  expect(repoMenu.y - (repoTrigger.y + repoTrigger.height)).toBeLessThanOrEqual(12)
  expect(repoMenu.x + repoMenu.width).toBeLessThanOrEqual(390)
  await repoPicker.getByRole('button', { name: /other/ }).click()
  await viewChange.click()
  await expect(git.getByRole('region', { name: '文件差异' })).toBeVisible()
  await diff.getByRole('button', { name: '打开文件' }).click()
  await expect(preview.getByRole('textbox', { name: '编辑 src/index.ts' })).toHaveValue(otherSource)
  await expect(auxiliaryToggle).toHaveAttribute('aria-expanded', 'false')
  await expect(git).toBeHidden()
  await header.getByRole('button', { name: '打开文件目录' }).click()
  const drawer = page.getByRole('dialog', { name: '文件目录导航' })
  const drawerTree = drawer.getByRole('complementary', { name: '机器人项目文件' })
  const bottomFile = drawerTree.getByRole('button', { name: 'extra-32.ts' })
  const firstFile = drawerTree.getByRole('button', { name: 'README.md', exact: true })
  await expect(bottomFile).not.toBeInViewport()
  await firstFile.hover()
  await page.mouse.wheel(0, 1800)
  await expect(bottomFile).toBeInViewport()
  await expect(drawer.getByRole('button', { name: '关闭导航' })).toBeInViewport()
  await drawer.getByRole('button', { name: 'README.md', exact: true }).click()
  await expect(drawer).toBeHidden()
  await expect(
    preview.getByRole('textbox', { name: '编辑 README.md' })
  ).toBeVisible()
  await markdownToggle.click()
  await expect(renderedMarkdown.getByRole('heading', { name: '使用指南' })).toBeVisible()
  await markdownToggle.click()
  await expect(
    tabs.getByRole('button', { name: 'README.md', exact: true })
  ).toBeInViewport()
  await expect(
    tabs.getByRole('button', { name: '关闭文件 README.md' })
  ).toBeInViewport()
  await header.getByRole('button', { name: '打开文件目录' }).click()
  await drawer.getByRole('button', { name: '新建目录' }).click()
  const folderDialog = page.getByRole('dialog', { name: '新建目录' })
  await folderDialog.getByLabel('目录名称').fill('new-dir')
  await folderDialog.getByRole('button', { name: '创建', exact: true }).click()
  await expect(folderDialog).toBeHidden()
  await expect(drawer.getByText('new-dir', { exact: true })).toBeVisible()

  await drawer.getByRole('button', { name: '新建文件' }).click()
  const fileDialog = page.getByRole('dialog', { name: '新建文件' })
  const fileName = fileDialog.getByLabel('文件名称')
  await fileName.fill('README.md')
  await fileName.press('Enter')
  await expect(fileDialog.getByRole('alert')).toHaveText('项目中已有同名文件或目录。')
  await fileName.fill('new.ts')
  await fileName.press('Enter')
  await expect(fileDialog).toBeHidden()
  await expect(drawer).toBeHidden()
  await expect(preview.getByRole('textbox', { name: '编辑 new.ts' })).toHaveValue('')

  await rail.getByRole('button', { name: '主页', exact: true }).click()
  await rail.getByRole('button', { name: '资源管理器' }).click()
  await expect(git).toBeHidden()
  await auxiliaryToggle.click()
  await launcher.getByRole('button', { name: '终端', exact: true }).click()
  await expect(session).toHaveAttribute('aria-pressed', 'true')
  await expect(git).toBeHidden()
})
