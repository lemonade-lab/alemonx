import { expect, test, type Page } from '@playwright/test'
import path from 'node:path'

async function openInstaller(page: Page) {
  await page.route('**/backpack-folder-regression', route => route.fulfill({
    contentType: 'text/html; charset=utf-8',
    body: `<div id="root"></div><script type="module">
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      await import('/e2e/fixtures/backpack-folder.tsx');
    </script>`
  }))
  await page.goto('/backpack-folder-regression')
  await page.getByRole('button', { name: '本地安装', exact: true }).click()
}

test('folder picker opens from drop zone, preserves paths and retries a failed install', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  let requests = 0
  await page.route('**/api/v1/robot/packages/folder?**', route => {
    requests++
    const body = route.request().postDataBuffer()?.toString() ?? ''
    expect(body).toContain('files:package.json')
    expect(body).toContain('files:lib/index.js')
    return requests === 1
      ? route.fulfill({ status: 400, json: { error: '背包中已存在同名插件包' } })
      : route.fulfill({ json: { name: 'folder-example', valid: true } })
  })
  await openInstaller(page)
  await expect(page.getByRole('button', { name: '安装', exact: true })).toBeDisabled()
  const chooser = page.waitForEvent('filechooser')
  await page.getByRole('button', { name: '选择文件夹', exact: true }).click()
  await (await chooser).setFiles(path.resolve('e2e/fixtures/folder-plugin'))
  await expect(page.getByRole('status')).toContainText('folder-example · 2 个文件')
  await page.getByRole('button', { name: '安装', exact: true }).click()
  await expect(page.getByRole('alert')).toContainText('已存在')
  const replacement = page.waitForEvent('filechooser')
  await page.getByRole('button', { name: '重新选择文件夹', exact: true }).click()
  await (await replacement).setFiles(path.resolve('e2e/fixtures/folder-plugin'))
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('status')).toContainText('folder-example · 2 个文件')
  await page.getByRole('button', { name: '安装', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('已安装')
  await expect(page).toHaveTitle('安装后已刷新')
  await page.getByRole('button', { name: '完成', exact: true }).click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByRole('button', { name: '本地安装', exact: true })).toBeFocused()
})

test('folder drop reads all directory batches and excludes dependencies', async ({ page }) => {
  await openInstaller(page)
  await page.getByRole('button', { name: '选择文件夹', exact: true }).evaluate(node => {
    const file = (name: string, content: string) => ({ name, isFile: true, isDirectory: false, file: (resolve: (value: File) => void) => resolve(new File([content], name)) })
    const folder = (name: string, batches: object[][]) => ({ name, isDirectory: true, isFile: false, createReader: () => { let index = 0; return { readEntries: (resolve: (value: object[]) => void) => resolve(batches[index++] ?? []) } } })
    const entry = folder('plugin', [
      [file('package.json', '{"name":"dropped-plugin"}')],
      [folder('lib', [[file('index.js', 'ok')]]), folder('node_modules', [[file('ignore.js', 'ignore')]]), file('.env', 'ignored')]
    ])
    const event = new Event('drop', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'dataTransfer', { value: { items: [{ kind: 'file', webkitGetAsEntry: () => entry }] } })
    node.dispatchEvent(event)
  })
  await expect(page.getByRole('status')).toContainText('dropped-plugin · 2 个文件')
  await page.getByRole('button', { name: '取消', exact: true }).press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
})
