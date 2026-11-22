import { expect, test } from '@playwright/test'
import type { GitActionInput } from '../src/store/workspaceApi'

test('Git management separates changes, history and repository actions in a narrow pane', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 600 })
  const actions: GitActionInput[] = []
  let staged = false
  let branch = 'main'
  const longBranch = 'feature/' + 'long-branch-component-'.repeat(4) + 'release'
  await page.route('**/api/v1/robot/git**', async route => {
    const url = new URL(route.request().url())
    if (route.request().method() === 'POST') {
      const action = route.request().postDataJSON() as GitActionInput
      actions.push(action)
      if (action.action === 'stage') staged = true
      if (action.action === 'branch-switch' && action.value) branch = action.value
      return route.fulfill({ json: { output: '已完成' } })
    }
    if (url.pathname.endsWith('/diff')) {
      return route.fulfill({ json: {
        path: url.searchParams.get('path'),
        status: 'M',
        diff: `@@ -1 +1 @@\n-旧值\n+${url.searchParams.get('view') === 'staged' ? '暂存版本' : '工作区版本'}`
      } })
    }
    return route.fulfill({ json: {
      root: '/tmp/git-workspace-fixture',
      gitRoot: '/tmp/git-workspace-fixture',
      repository: true,
      atRepositoryRoot: true,
      branch,
      upstream: 'origin/main',
      remoteName: 'origin',
      ahead: 2,
      behind: 1,
      authorName: '测试用户',
      authorEmail: 'git@example.test',
      indexRevision: 'fixture-index',
      changes: [
        { path: 'src/runtime.ts', status: 'MM', staged: true, unstaged: true },
        { path: 'README.md', status: 'M', staged, unstaged: !staged },
        ...Array.from({ length: 24 }, (_, index) => ({
          path: `src/config-${index + 1}.ts`,
          status: 'M',
          staged: false,
          unstaged: true
        }))
      ],
      branches: url.searchParams.get('view') === 'branch'
        ? ['main', 'release', longBranch, ...Array.from({ length: 24 }, (_, i) => `feature/qa-${i}`)]
          .map(name => ({ name, current: name === branch }))
        : [],
      remoteBranches: [],
      commits: [
        { sha: 'aabbccdd', shortSha: 'aabbccd', subject: '运行日志', createdAt: '2026-11-22' },
        ...Array.from({ length: 19 }, (_, i) => ({
          sha: String(i).padStart(8, '0'),
          shortSha: String(i).padStart(7, '0'),
          subject: `更新配置 ${i + 1}`,
          createdAt: '2026-11-22'
        }))
      ],
      tags: [{ name: 'v1.0.0', subject: '初始版本' }],
      remotes: [{ name: 'origin', url: 'https://example.test/repo.git' }],
      stashes: []
    } })
  })

  await page.goto('/e2e/fixtures/git-workspace.html')
  const git = page.getByRole('region', { name: 'Git 管理' })
  const tabs = git.getByRole('tablist', { name: 'Git 功能' })
  await expect(tabs.getByRole('tab')).toHaveText(['变更', '历史'])
  const stagedFiles = git.getByRole('region', { name: '已暂存', exact: true })
  const unstagedFiles = git.getByRole('region', { name: '未暂存', exact: true })
  const stagedDiff = stagedFiles.getByRole('button', { name: '查看 src/runtime.ts 的已暂存差异' })
  await stagedDiff.click()
  const diff = git.getByRole('region', { name: '文件差异' })
  await expect(diff.getByText('+暂存版本', { exact: true })).toBeVisible()
  await diff.getByRole('button', { name: '返回' }).click()
  await expect(stagedDiff).toBeFocused()
  await unstagedFiles.getByRole('button', { name: '查看 src/runtime.ts 的未暂存差异' }).click()
  await expect(diff.getByText('+工作区版本', { exact: true })).toBeVisible()
  await diff.getByRole('button', { name: '返回' }).click()
  const body = git.getByRole('tabpanel', { name: '变更' })
  const tabY = (await tabs.boundingBox())!.y
  const lastFile = unstagedFiles.getByRole('button', { name: '查看 src/config-24.ts 的未暂存差异' })
  await lastFile.scrollIntoViewIfNeeded()
  const offset = await body.evaluate(element => element.scrollTop)
  expect(offset).toBeGreaterThan(0)
  expect((await tabs.boundingBox())!.y).toBe(tabY)
  await lastFile.click()
  await diff.getByRole('button', { name: '返回' }).click()
  await expect(lastFile).toBeFocused()
  expect(await body.evaluate(element => element.scrollTop)).toBe(offset)
  await body.evaluate(element => { element.scrollTop = 0 })
  await unstagedFiles.getByRole('button', { name: '暂存 README.md' }).click()
  await expect(stagedFiles.getByRole('button', { name: '取消暂存 README.md' })).toBeVisible()
  expect(actions).toContainEqual(expect.objectContaining({ action: 'stage', paths: ['README.md'], expectedBranch: 'main' }))
  await git.getByRole('textbox', { name: '提交说明' }).fill('更新运行文档')
  await git.getByRole('button', { name: '提交 2 个文件' }).click()
  await page.getByRole('dialog', { name: '提交已暂存内容' }).getByRole('button', { name: '确认提交' }).click()
  await expect.poll(() => actions.find(action => action.action === 'commit')).toEqual(expect.objectContaining({
    message: '更新运行文档',
    paths: ['src/runtime.ts', 'README.md'],
    indexRevision: 'fixture-index'
  }))
  await expect(git.getByRole('textbox', { name: '提交说明' })).toHaveValue('')

  await tabs.getByRole('tab', { name: '历史' }).click()
  await expect(git.getByRole('button', { name: /查看提交 aabbccd 运行日志/ })).toBeVisible()
  const historyTabs = git.getByRole('tablist', { name: '历史内容' })
  const historyTabsTop = (await historyTabs.boundingBox())!.y
  const historyBody = git.getByRole('tabpanel', { name: '历史' })
  await historyBody.getByRole('button', { name: /更新配置 19$/ }).scrollIntoViewIfNeeded()
  expect(await historyBody.evaluate(node => node.scrollTop)).toBeGreaterThan(0)
  expect((await historyTabs.boundingBox())!.y).toBe(historyTabsTop)
  expect((await tabs.boundingBox())!.y).toBe(tabY)
  await git.getByRole('tab', { name: '版本标签' }).click()
  await expect(git.getByText('v1.0.0', { exact: true })).toBeVisible()
  const tagBody = git.getByRole('tabpanel', { name: '历史' })
  expect(await tagBody.evaluate(node => node.scrollWidth)).toBe(await tagBody.evaluate(node => node.clientWidth))
  await expect(tagBody.getByRole('button', { name: '创建标签' })).toBeInViewport()
  const branchTrigger = git.getByRole('button', { name: '切换 Git 分支' })
  await branchTrigger.click()
  const branchPicker = git.getByRole('region', { name: '选择 Git 分支' })
  await expect(branchPicker.getByRole('button', { name: /main 当前分支/ })).toBeDisabled()
  const triggerBounds = (await branchTrigger.boundingBox())!
  const menuBounds = (await branchPicker.boundingBox())!
  const verticalGap = menuBounds.y - (triggerBounds.y + triggerBounds.height)
  expect(verticalGap).toBeGreaterThanOrEqual(0)
  expect(verticalGap).toBeLessThanOrEqual(12)
  expect(menuBounds.x + menuBounds.width).toBeLessThanOrEqual(390)
  const manage = branchPicker.getByRole('button', { name: '管理分支' })
  const manageBounds = (await manage.boundingBox())!
  expect(manageBounds.y + manageBounds.height).toBeLessThanOrEqual(menuBounds.y + menuBounds.height)
  const longName = branchPicker.getByText(longBranch, { exact: true })
  expect(await longName.evaluate(node => node.scrollWidth)).toBe(await longName.evaluate(node => node.clientWidth))
  await branchPicker.getByRole('button', { name: 'release', exact: true }).click()
  await expect(branchPicker).toBeHidden()
  await expect(branchTrigger).toHaveText('release')
  expect(actions).toContainEqual(expect.objectContaining({
    action: 'branch-switch',
    value: 'release',
    expectedBranch: 'main'
  }))
  await expect(page.getByRole('dialog', { name: '切换分支', exact: true })).toHaveCount(0)
  await branchTrigger.click()
  await manage.click()
  await expect(branchPicker).toBeHidden()
  const branches = page.getByRole('dialog', { name: '管理分支' })
  await expect(branches.getByRole('button', { name: '创建并切换' })).toBeVisible()
  await expect(branches.getByRole('button', { name: '更新', exact: true })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await git.getByRole('button', { name: /同步状态/ }).click()
  const sync = page.getByRole('dialog', { name: '远程同步' })
  await expect(sync.getByText('待推送 2 个 · 待拉取 1 个')).toBeVisible()
  await sync.getByText('管理远程', { exact: true }).click()
  await expect(sync.getByText('origin', { exact: true })).toBeVisible()
})
