import { expect, test } from '@playwright/test'

const root = '/test/robot'
const task = {
  id: 'upgrade-under-test',
  root,
  action: 'upgrade-alemon',
  status: 'running'
}
const newerTask = {
  ...task,
  id: 'unrelated-task',
  action: 'install-package',
  status: 'completed',
  output: '另一项任务的日志'
}

for (const status of ['completed', 'failed'] as const) {
  test(`operation feedback opens the matching ${status} task`, async ({
    page
  }) => {
    let finish!: () => void
    const pending = new Promise<void>(resolve => {
      finish = resolve
    })
    await page.route('**/api/v1/**', async route => {
      const url = new URL(route.request().url())
      switch (url.pathname) {
        case '/api/v1/workspace/robots':
          return route.fulfill({
            json: {
              scope: 'test',
              local: true,
              initialized: true,
              projects: [{ id: 'robot', path: root, name: 'robot' }],
              activeProjectID: 'robot'
            }
          })
        case '/api/v1/robot/validate':
          return route.fulfill({ json: { valid: true, path: root } })
        case '/api/v1/robot/runtime':
          return route.fulfill({ json: { dependenciesComplete: true, platforms: [] } })
        case '/api/v1/robot/tasks':
          if (route.request().method() === 'POST')
            return route.fulfill({ status: 202, json: task })
          if (url.searchParams.get('id') === task.id) {
            await pending
            return route.fulfill({
              json: {
                ...task,
                status,
                output: '本次升级的完整输出',
                ...(status === 'failed' ? { error: '升级命令失败' } : {})
              }
            })
          }
          return route.fulfill({ json: [newerTask] })
        default:
          return route.fulfill({ json: { items: [] } })
      }
    })
    await page.route('**/task-feedback-regression', route =>
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
    await page.goto('/task-feedback-regression')
    await page.getByRole('button', { name: '一键升级' }).click()
    await page.getByRole('button', { name: '确认继续' }).click()
    await expect(page.getByText('正在执行', { exact: true })).toBeVisible()
    await expect(page.getByRole('dialog', { name: '操作日志' })).toHaveCount(0)
    finish()
    await expect(
      page.getByText(status === 'failed' ? '操作未完成' : '操作完成', {
        exact: true
      })
    ).toBeVisible()
    const viewTask = page.getByRole('button', { name: '查看任务', exact: true })
    await expect(viewTask).toHaveCount(1)
    const taskWindow = page.getByRole('dialog', { name: '任务', exact: true })
    await expect(taskWindow).toHaveCount(0)
    await viewTask.click()
    await expect(
      taskWindow.getByText('本次升级的完整输出', { exact: status !== 'failed' })
    ).toBeVisible()
    if (status === 'failed')
      await expect(taskWindow.getByText('升级命令失败')).toBeVisible()
    await expect(taskWindow.getByText('另一项任务的日志')).toHaveCount(0)
  })
}
