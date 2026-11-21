import { expect, test } from '@playwright/test'
import {
  COMPACT_COMPONENT_QUERY,
  PHONE_VIEWPORT_QUERY,
  TABLET_WINDOW_QUERY,
  WORKBENCH_NAVIGATION_QUERY
} from '../src/hooks/viewportBreakpoints'

test('responsive breakpoint contract keeps viewport and container responsibilities separate', () => {
  expect(PHONE_VIEWPORT_QUERY).toBe('(max-width: 700px)')
  expect(COMPACT_COMPONENT_QUERY).toBe('(max-width: 759px)')
  expect(WORKBENCH_NAVIGATION_QUERY).toBe('(max-width: 940px)')
  expect(TABLET_WINDOW_QUERY).toBe('(max-width: 1024px)')
})

test('narrow workbench, Git, and operations windows respond to their own width', async ({
  page
}) => {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')

  await page.evaluate(() => {
    const fixture = document.createElement('div')
    fixture.id = 'window-responsive-fixture'
    fixture.innerHTML = `
      <section class="dashboard-window" data-window-container="workbench" style="width: 700px">
        <div class="console-layout">
          <aside class="project-rail">导航</aside>
          <section class="console-page">
            <aside class="control-dock">控制</aside>
            <main class="workspace-content">内容</main>
          </section>
        </div>
      </section>
      <section class="floating-window" data-window-container="desktop" style="width: 700px">
        <div class="git-tag-form grid">
          <input /><input /><button>创建标签</button>
        </div>
      </section>
      <section class="ops-panel" style="width: 700px">
        <div class="ops-metrics grid"></div>
        <div class="ops-event-grid grid"></div>
        <div class="ops-policy-fields grid"></div>
      </section>
      <section class="floating-window" style="width: 700px">
        <div class="testone-root testone-window">
          <aside class="testone-channel-rail"></aside>
          <aside class="testone-task-rail"></aside>
          <div class="testone-mobile-user"></div>
          <div class="testone-timer-tip"></div>
          <div class="testone-special-events grid"></div>
        </div>
      </section>
      <section class="floating-window" style="width: 900px">
        <div class="testone-root testone-window testone-medium-fixture">
          <aside class="testone-channel-rail"></aside>
          <aside class="testone-task-rail"></aside>
        </div>
      </section>
    `
    document.body.append(fixture)
  })

  const result = await page
    .locator('#window-responsive-fixture')
    .evaluate(node => {
      const workbench = node.querySelector<HTMLElement>('.dashboard-window')!
      const rail = node.querySelector<HTMLElement>('.project-rail')!
      const consolePage = node.querySelector<HTMLElement>('.console-page')!
      const gitTagForm = node.querySelector<HTMLElement>('.git-tag-form')!
      const opsPanel = node.querySelector<HTMLElement>('.ops-panel')!
      const opsMetrics = node.querySelector<HTMLElement>('.ops-metrics')!
      const opsEventGrid = node.querySelector<HTMLElement>('.ops-event-grid')!
      const opsPolicyFields =
        node.querySelector<HTMLElement>('.ops-policy-fields')!
      const testone = node.querySelector<HTMLElement>('.testone-window')!
      const testoneChannelRail = testone.querySelector<HTMLElement>(
        '.testone-channel-rail'
      )!
      const testoneTaskRail =
        testone.querySelector<HTMLElement>('.testone-task-rail')!
      const testoneMobileUser = testone.querySelector<HTMLElement>(
        '.testone-mobile-user'
      )!
      const testoneTimerTip =
        testone.querySelector<HTMLElement>('.testone-timer-tip')!
      const testoneEvents = testone.querySelector<HTMLElement>(
        '.testone-special-events'
      )!
      const testoneMedium = node.querySelector<HTMLElement>(
        '.testone-medium-fixture'
      )!
      const testoneMediumChannelRail = testoneMedium.querySelector<HTMLElement>(
        '.testone-channel-rail'
      )!
      const testoneMediumTaskRail =
        testoneMedium.querySelector<HTMLElement>('.testone-task-rail')!
      return {
        workbenchContainer: getComputedStyle(workbench).containerName,
        railBorderRight: getComputedStyle(rail).borderRightWidth,
        railBorderBottom: getComputedStyle(rail).borderBottomWidth,
        consoleColumns: getComputedStyle(consolePage).gridTemplateColumns,
        gitTagColumns: getComputedStyle(gitTagForm).gridTemplateColumns,
        opsContainer: getComputedStyle(opsPanel).containerName,
        opsMetricColumns: getComputedStyle(opsMetrics).gridTemplateColumns,
        opsEventColumns: getComputedStyle(opsEventGrid).gridTemplateColumns,
        opsPolicyColumns: getComputedStyle(opsPolicyFields).gridTemplateColumns,
        testoneContainer: getComputedStyle(testone).containerName,
        testoneChannelDisplay: getComputedStyle(testoneChannelRail).display,
        testoneTaskDisplay: getComputedStyle(testoneTaskRail).display,
        testoneMobileUserDisplay: getComputedStyle(testoneMobileUser).display,
        testoneTimerTipDisplay: getComputedStyle(testoneTimerTip).display,
        testoneEventColumns:
          getComputedStyle(testoneEvents).gridTemplateColumns,
        testoneMediumChannelDisplay: getComputedStyle(testoneMediumChannelRail)
          .display,
        testoneMediumTaskDisplay: getComputedStyle(testoneMediumTaskRail)
          .display
      }
    })

  expect(result.workbenchContainer).toBe('workbench-window')
  expect(result.railBorderRight).toBe('0px')
  expect(result.railBorderBottom).toBe('1px')
  expect(result.consoleColumns.trim().split(/\s+/)).toHaveLength(1)
  expect(result.gitTagColumns.trim().split(/\s+/)).toHaveLength(1)
  expect(result.opsContainer).toBe('ops-panel')
  expect(result.opsMetricColumns.trim().split(/\s+/)).toHaveLength(2)
  expect(result.opsEventColumns.trim().split(/\s+/)).toHaveLength(1)
  expect(result.opsPolicyColumns.trim().split(/\s+/)).toHaveLength(1)
  expect(result.testoneContainer).toBe('testone-window')
  expect(result.testoneChannelDisplay).toBe('none')
  expect(result.testoneTaskDisplay).toBe('none')
  expect(result.testoneMobileUserDisplay).toBe('flex')
  expect(result.testoneTimerTipDisplay).toBe('flex')
  expect(result.testoneEventColumns.trim().split(/\s+/)).toHaveLength(2)
  expect(result.testoneMediumChannelDisplay).toBe('none')
  expect(result.testoneMediumTaskDisplay).toBe('flex')
})

for (const phone of [
  { width: 320, height: 568, theme: 'light' },
  { width: 375, height: 667, theme: 'dark' },
  { width: 390, height: 844, theme: 'light' }
]) {
  test(`phone window chrome fills ${phone.width}px without horizontal overflow`, async ({
    page
  }) => {
    await page.setViewportSize({ width: phone.width, height: phone.height })
    await page.goto('/')
    await page.evaluate(theme => {
      document.documentElement.dataset.theme = theme
      const fixture = document.createElement('section')
      fixture.className = 'floating-window'
      fixture.style.cssText = 'left:16px; top:16px; width:320px; height:500px'
      document.body.append(fixture)
    }, phone.theme)
    const result = await page.locator('.floating-window').evaluate(node => {
      const rect = (node as HTMLElement).getBoundingClientRect()
      return {
        left: rect.left,
        right: rect.right,
        width: rect.width,
        height: rect.height,
        scrollWidth: document.documentElement.scrollWidth,
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight
      }
    })

    expect(result.left).toBe(0)
    expect(result.right).toBeLessThanOrEqual(result.viewportWidth)
    expect(result.width).toBe(phone.width)
    expect(result.height).toBe(phone.height)
    expect(result.scrollWidth).toBeLessThanOrEqual(result.viewportWidth)
  })
}

test('phone shell preserves reachable controls and localizes overflow', async ({
  page
}) => {
  await page.setViewportSize({ width: 320, height: 568 })
  await page.goto('/')
  await page.evaluate(() => {
    const fixture = document.createElement('section')
    fixture.id = 'mobile-contract-fixture'
    fixture.className = 'floating-window'
    fixture.innerHTML = `
      <header class="floating-window-header"><div>智能日志</div><div><button class="icon-button" aria-label="关闭智能日志">×</button></div></header>
      <div data-sidebar-window-shell>
        <aside data-sidebar-window-sidebar>
          <div data-sidebar-window-nav><button>运行</button><button>日志</button></div>
          <div data-sidebar-window-side-actions><button>刷新</button></div>
        </aside>
        <main data-sidebar-window-body><pre>${'long-log-line '.repeat(100)}</pre></main>
      </div>
      <aside class="operation-tasks-popover">任务日志</aside>
    `
    document.body.append(fixture)
  })

  const fixture = page.locator('#mobile-contract-fixture')
  const result = await fixture.evaluate(node => {
    const windowRect = (node as HTMLElement).getBoundingClientRect()
    const close = node.querySelector<HTMLElement>(
      '[aria-label="关闭智能日志"]'
    )!
    const nav = node.querySelector<HTMLElement>(
      '[data-sidebar-window-nav] > button'
    )!
    const popover = node.querySelector<HTMLElement>('.operation-tasks-popover')!
    const pre = node.querySelector<HTMLElement>('pre')!
    return {
      windowRect: {
        left: windowRect.left,
        right: windowRect.right,
        height: windowRect.height
      },
      closeHeight: close.getBoundingClientRect().height,
      navHeight: nav.getBoundingClientRect().height,
      popoverWidth: popover.getBoundingClientRect().width,
      preScrollWidth: pre.scrollWidth,
      preClientWidth: pre.clientWidth,
      pageScrollWidth: document.documentElement.scrollWidth,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight
    }
  })

  expect(result.windowRect.left).toBe(0)
  expect(result.windowRect.right).toBeLessThanOrEqual(result.viewportWidth)
  expect(result.windowRect.height).toBe(result.viewportHeight)
  expect(result.closeHeight).toBeGreaterThanOrEqual(44)
  expect(result.navHeight).toBeGreaterThanOrEqual(44)
  expect(result.popoverWidth).toBeLessThanOrEqual(result.viewportWidth - 24)
  expect(result.preScrollWidth).toBeGreaterThan(result.preClientWidth)
  expect(result.pageScrollWidth).toBeLessThanOrEqual(result.viewportWidth)

  // Preserve a rendered artifact in Playwright output for visual inspection
  // even when a platform-specific screenshot baseline is not available yet.
  expect((await fixture.screenshot()).byteLength).toBeGreaterThan(1_000)
})

test('robot workbench resizes the left rail and keeps the right card fixed', async ({
  page
}) => {
  await page.setViewportSize({ width: 1280, height: 820 })
  await page.route('**/robot-splitter-regression', route =>
    route.fulfill({
      contentType: 'text/html; charset=utf-8',
      body: `<div id="root" style="container:workbench-window / inline-size; display:flex; height:600px"></div><script type="module">
      import '/src/styles.css';
      import RefreshRuntime from '/@react-refresh';
      RefreshRuntime.injectIntoGlobalHook(window);
      window.$RefreshReg$ = () => {};
      window.$RefreshSig$ = () => type => type;
      window.__vite_plugin_react_preamble_installed__ = true;
      const { default: React } = await import('/node_modules/.vite/deps/react.js');
      const { default: ReactDOM } = await import('/node_modules/.vite/deps/react-dom_client.js');
      const { Provider } = await import('/node_modules/.vite/deps/react-redux.js');
      const { store } = await import('/src/store/guideStore.ts');
      const { RobotWorkspaceSplitter } = await import('/src/components/RobotWorkspaceSplitter.tsx');
      const { RobotPanel } = await import('/src/components/RobotPanel.tsx');
      ReactDOM.createRoot(document.getElementById('root')).render(React.createElement(Provider, {store},
        React.createElement(RobotWorkspaceSplitter, {list: '机器人列表', controls: React.createElement('aside', {className:'control-dock flex min-h-0 flex-col'},
          React.createElement('section', {className:'control-card control-navigation'},
            React.createElement('header', {style:{display:'block'}}, '机器人面板'),
            React.createElement('nav', {'data-project-navigation': '', 'aria-label':'机器人功能'},
              Array.from({length: 40}, (_, index) => React.createElement('button', {key:index, style:{display:'block', height:36}}, '菜单 ' + index))),
            React.createElement('footer', {className:'control-quick-actions grid-cols-4', 'aria-label':'快捷操作'},
              ['应用', '测试', '聊天', '运维'].map(label => React.createElement('button', {key:label}, label)))))}, React.createElement(RobotPanel, {title:'运行概览'}, React.createElement('div', null, '工作区')))));
    </script>`
    })
  )
  await page.goto('/robot-splitter-regression')
  const width = async (label: string) =>
    page
      .getByText(label, { exact: true })
      .evaluate(node => node.getBoundingClientRect().width)
  const left = page.getByRole('separator', { name: '调整机器人列表宽度' })
  const divider = left.locator('.ant-splitter-bar-dragger')
  await expect(divider).toBeVisible()
  expect(await divider.evaluate(node => getComputedStyle(node, '::before').content)).toBe('none')
  expect(await divider.evaluate(node => getComputedStyle(node, '::after').content)).toBe('none')
  const right = page.locator('.robot-workspace-main + .ant-splitter-bar')
  await expect(right).toHaveCount(0)
  const column = (await page.locator('.robot-workspace-content').boundingBox())!
  const content = (await page.locator('.robot-workspace-content > .console-page').boundingBox())!
  const controls = page.locator('.robot-workspace-controls')
  const dock = (await controls.boundingBox())!
  expect(Math.abs(content.x - column.x)).toBeLessThanOrEqual(1)
  expect(Math.abs(content.x + content.width - (column.x + column.width))).toBeLessThanOrEqual(1)
  const workspace = (await page.getByText('工作区', { exact: true }).boundingBox())!
  expect(workspace.x + workspace.width).toBeLessThanOrEqual(dock.x + 1)
  const card = (await controls.locator('.control-card').boundingBox())!
  const panelHeader = page.locator('[data-robot-panel-header]')
  const header = (await panelHeader.boundingBox())!
  expect(header.x).toBeCloseTo(workspace.x, 0)
  expect(header.width).toBeCloseTo(workspace.width, 0)
  expect(Math.abs(header.y - card.y)).toBeLessThanOrEqual(2)
  const leftGap = header.x - column.x
  const rightGap = card.x - (header.x + header.width)
  expect(Math.abs(leftGap - rightGap)).toBeLessThanOrEqual(2)
  expect(content.x + content.width).toBeGreaterThan(card.x + card.width)
  expect(await controls.evaluate(node => getComputedStyle(node).position)).toBe('absolute')
  const menu = page.getByRole('navigation', { name: '机器人功能' })
  const footer = controls.locator('footer[aria-label="快捷操作"]')
  const shortcutSizes = await footer.getByRole('button').evaluateAll(buttons =>
    buttons.map(button => {
      const { width, top } = button.getBoundingClientRect()
      return { width, top }
    })
  )
  expect(shortcutSizes).toHaveLength(4)
  for (const shortcut of shortcutSizes) {
    expect(shortcut.width).toBeCloseTo(shortcutSizes[0].width, 0)
    expect(shortcut.top).toBe(shortcutSizes[0].top)
  }
  const footerTop = (await footer.boundingBox())!.y
  await menu.hover()
  await page.mouse.wheel(0, 320)
  await expect
    .poll(() => menu.evaluate(node => node.scrollTop))
    .toBeGreaterThan(0)
  expect((await footer.boundingBox())!.y).toBe(footerTop)
  const beforeList = await width('机器人列表')
  const beforeControls = await width('机器人面板')
  const beforeWorkspace = await width('工作区')
  const drag = async (separator: typeof left, offset: number) => {
    const handle = await separator
      .locator('.ant-splitter-bar-dragger')
      .boundingBox()
    if (!handle) throw new Error('分隔条未渲染')
    const x = handle.x + handle.width / 2
    const y = handle.y + handle.height / 2
    await page.mouse.move(x, y)
    await page.mouse.down()
    await page.mouse.move(x + offset, y, { steps: 5 })
    await page.mouse.up()
  }
  await drag(left, 65)
  await expect.poll(() => width('机器人列表')).toBeGreaterThan(beforeList + 40)
  await expect.poll(() => width('工作区')).toBeLessThan(beforeWorkspace - 40)
  await left.press('ArrowLeft')
  await expect.poll(() => width('机器人面板')).toBe(beforeControls)
  const resizedList = await width('机器人列表')
  await expect
    .poll(() => page.evaluate(() => JSON.stringify(localStorage)))
    .toContain('workbench.splitter')
  await page.reload()
  await expect.poll(() => width('机器人列表')).toBe(resizedList)
  await expect.poll(() => width('机器人面板')).toBe(beforeControls)

  await page.setViewportSize({ width: 390, height: 820 })
  await expect(left).toBeHidden()
  await expect(right).toBeHidden()
  await expect(page.getByText('机器人列表', { exact: true })).toBeHidden()
  await expect(page.getByText('机器人面板', { exact: true })).toBeHidden()
  const phoneContent = (await page.locator('.robot-workspace-content > .console-page').boundingBox())!
  expect(phoneContent.y).toBe(0)
  expect(phoneContent.height).toBe(600)
  await page.setViewportSize({ width: 1280, height: 820 })
  await expect(left.locator('.ant-splitter-bar-dragger')).toBeVisible()
  await expect.poll(() => width('机器人列表')).toBe(resizedList)
})

test('browser workbench fills the viewport and integrates the tool rail', async ({
  page
}) => {
  await page.setViewportSize({ width: 1600, height: 900 })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.goto('/dashboard/robot')
  const workbench = page.locator('.dashboard-window')
  const tools = workbench.getByRole('navigation', { name: '工作台工具' })
  await expect(tools).toBeVisible()
  expect(await workbench.boundingBox()).toEqual({
    x: 0, y: 0, width: 1600, height: 900
  })
  await expect(page.getByRole('button', { name: 'Workbench Resize Handle' })).toHaveCount(0)
  await expect(page.getByRole('complementary', { name: 'Workbench Dock' })).toHaveCount(0)

  const home = tools.getByRole('button', { name: '主页', exact: true })
  const settings = tools.getByRole('button', { name: '打开设置', exact: true })
  await expect(home).toHaveCSS('border-width', '0px')
  await expect(settings).toHaveCSS('border-width', '0px')
  await expect(settings).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  const windows = tools.getByRole('group', { name: '已打开窗口' })
  await expect(windows.getByRole('button')).toHaveCount(0)
  const homePosition = (await home.boundingBox())!
  const separator = (await tools.getByRole('separator').boundingBox())!
  const settingsPosition = (await settings.boundingBox())!
  expect(homePosition.y + homePosition.height).toBeLessThan(separator.y)
  expect(separator.width).toBeLessThan(homePosition.width)
  expect(settingsPosition.y).toBeGreaterThan(850)

  await page.getByRole('region', { name: '系统功能目录' })
    .getByRole('button', { name: '浏览', exact: true }).click()
  const browser = page.getByRole('dialog', { name: '浏览器', exact: true })
  await expect(browser).toBeVisible()
  await expect(windows.getByRole('button', { name: '切换到浏览器' })).toBeVisible()
  await home.click()
  await expect(browser).toBeHidden()
  await windows.getByRole('button', { name: '恢复浏览器' }).click()
  await expect(browser).toBeVisible()
  await browser.getByRole('button', { name: '关闭浏览器' }).click()
  await expect(browser).toHaveCount(0)
  await expect(windows.getByRole('button')).toHaveCount(0)

  await settings.click()
  const settingsWindow = page.getByRole('dialog', { name: '设置', exact: true })
  await expect(settingsWindow).toBeVisible()
  await expect(windows.getByRole('button')).toHaveCount(0)
  const settingsHeader = settingsWindow.locator('.floating-window-header')
  await settingsHeader.dblclick({ position: { x: 180, y: 12 } })
  await expect.poll(async () => (await settingsWindow.boundingBox())?.x).toBe(16)
  await expect.poll(() =>
    home.evaluate(button => {
      const { x, y, width, height } = button.getBoundingClientRect()
      return document.elementFromPoint(x + width / 2, y + height / 2)
        ?.closest('[role="dialog"]')
        ?.getAttribute('aria-label')
    })
  ).toBe('设置')
  await settingsHeader.dblclick({ position: { x: 180, y: 12 } })
  await home.click()
  await expect(settingsWindow).toBeHidden()

  for (const width of [760, 390]) {
    await page.setViewportSize({ width, height: 600 })
    await expect(tools).toBeVisible()
    await expect(home).toBeInViewport()
    await expect(settings).toBeInViewport()
    expect(await workbench.boundingBox()).toEqual({
      x: 0, y: 0, width, height: 600
    })
  }
})

test('browser workbench leaves window shortcuts to the browser', async ({ page }) => {
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.goto('/dashboard/robot')
  const workbench = page.locator('.dashboard-window')
  await expect(workbench).toBeVisible()
  await page.getByRole('button', { name: '打开设置', exact: true }).click()
  const settings = page.getByRole('dialog', { name: '设置', exact: true })
  await expect(settings).toBeVisible()

  const handled = await page.evaluate(() =>
    ['metaKey', 'ctrlKey'].flatMap(modifier =>
      [{ key: 'w' }, { key: 'm' }, { key: 'M', shiftKey: true }].map(keys =>
        !document.activeElement!.dispatchEvent(
          new KeyboardEvent('keydown', {
            ...keys,
            [modifier]: true,
            bubbles: true,
            cancelable: true
          })
        )
      )
    )
  )
  expect(handled).toEqual(Array(6).fill(false))
  await expect(settings).toBeVisible()
  await expect(workbench).toBeVisible()
})

for (const platform of ['mac', 'windows', 'linux'] as const) {
  test(`native ${platform} header fits the minimum workbench width`, async ({
    page
  }) => {
    await page.setViewportSize({ width: 640, height: 480 })
    await page.addInitScript(platform => {
      Object.defineProperty(navigator, 'platform', {
        value: platform === 'linux' ? 'Linux x86_64' : 'MacIntel'
      })
      const messages: string[] = []
      const postMessage = (message: string) => messages.push(message)
      Object.assign(window, { captionMessages: messages })
      if (platform === 'windows') {
        Object.assign(window.chrome, { webview: { postMessage } })
      } else {
        Object.assign(window, {
          webkit: { messageHandlers: { external: { postMessage } } }
        })
      }
    }, platform)
    await page.route('**/api/v1/auth/status', route =>
      route.fulfill({ json: { enabled: false, authenticated: false } })
    )
    await page.goto('/dashboard/robot')
    const nativeCaption = page.locator('.native-workbench-header:visible')
    const header = platform === 'mac'
      ? page.locator('.workbench-content-header')
      : nativeCaption
    await expect(header).toBeVisible()
    await expect(nativeCaption).toHaveCount(platform === 'mac' ? 0 : 1)
    const content = page.locator('.dashboard-window')
    const bounds = (await header.boundingBox())!
    if (platform === 'mac') {
      const rail = page.getByRole('navigation', { name: '工作台工具' })
      const railBounds = (await rail.boundingBox())!
      expect(railBounds.y).toBe(0)
      expect(bounds.x).toBe(railBounds.width)
      expect(bounds.x + bounds.width).toBe(640)
      expect((await content.boundingBox())?.y).toBe(0)
      const workbarToggle = header.getByRole('button', { name: '打开机器人与系统导航' })
      expect((await workbarToggle.boundingBox())!.x).toBeGreaterThanOrEqual(86)
      expect((await rail.getByRole('button', { name: '主页', exact: true }).boundingBox())!.y)
        .toBeGreaterThanOrEqual(bounds.height)
    } else {
      expect(bounds.width).toBe(640)
      expect((await content.boundingBox())?.y).toBe(bounds.height)
    }
    const scrollPosition = () => page.evaluate(() => ({
      x: window.scrollX,
      y: window.scrollY,
      overscroll: getComputedStyle(document.documentElement).overscrollBehavior
    }))
    await page.mouse.move(320, 440)
    await page.mouse.wheel(500, 600)
    await expect.poll(scrollPosition).toEqual({ x: 0, y: 0, overscroll: 'none' })
    expect((await header.boundingBox())?.y).toBe(0)
    await header.locator(platform === 'mac' ? '.workbench-content-drag' : '.native-workbench-drag').dblclick()
    if (platform === 'windows') {
      await header.getByRole('button', { name: '最小化窗口' }).click()
      await header.getByRole('button', { name: '关闭窗口' }).click()
    }
    const messages = await page.evaluate(() =>
      Reflect.get(window, 'captionMessages')
    )
    expect(messages.filter((message: string) => message !== 'wails:runtime:ready')).toEqual(
      platform === 'windows'
        ? [
            'alx:window:toggle-maximise',
            'alx:window:minimise',
            'alx:window:close'
          ]
        : platform === 'mac' ? ['wails:drag:doubleclick'] : []
    )
    const rail = page.getByRole('navigation', { name: '工作台工具' })
    const blank = (await rail.getByRole('group', { name: '已打开窗口' }).boundingBox())!
    const blankY = blank.y + blank.height / 2
    const dragMessages = () => page.evaluate(() =>
      Reflect.get(window, 'captionMessages').filter((message: string) => message === 'wails:drag')
    )
    await page.mouse.move(blank.x + blank.width - 2, blankY)
    await page.mouse.down()
    expect(await dragMessages()).toHaveLength(0)
    await page.mouse.move(blank.x + blank.width + 6, blankY)
    await page.mouse.up()
    await expect.poll(dragMessages).toEqual(['wails:drag'])

    const home = (await rail.getByRole('button', { name: '主页', exact: true }).boundingBox())!
    await page.mouse.move(home.x + home.width / 2, home.y + home.height / 2)
    await page.mouse.down()
    await page.mouse.move(home.x + home.width / 2 + 5, home.y + home.height / 2)
    await page.mouse.up()
    expect(await dragMessages()).toEqual(['wails:drag'])
  })
}

test('terminal opens at minimum width and remembers later adjustments', async ({
  page
}) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.setViewportSize({ width: 1600, height: 900 })
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: (text: string) => {
        document.documentElement.dataset.copiedTerminalOutput = text
        return Promise.resolve()
      } }
    })
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  let created = 0
  let deleted = 0
  await page.route('**/api/v1/terminal/sessions', route => {
    created++
    return route.fulfill({
      json: { id: `fixture-${created}`, key: 'fixture-key', cwd: '/fixture', shell: '/bin/sh' }
    })
  })
  await page.route('**/api/v1/terminal/sessions/fixture**', route => {
    if (route.request().method() === 'DELETE') deleted++
    return route.fulfill({ json: { cwd: '/fixture', shell: '/bin/sh', output: '' } })
  })
  await page.route('**/api/v1/terminal/sessions/fixture-*/stream**', route =>
    route.fulfill({
      contentType: 'text/event-stream',
      body: 'data: {"snapshot":true,"text":"terminal fixture"}\n\n'
    })
  )

  await page.goto('/dashboard/robot')
  const header = page.locator('.workbench-content-header')
  const projectRail = page.locator('.robot-workspace-list')
  const leftBounds = (await projectRail.boundingBox())!
  const headerBounds = (await header.boundingBox())!
  expect(headerBounds.x).toBeCloseTo(leftBounds.x + leftBounds.width, 1)
  expect(headerBounds.y).toBe(leftBounds.y)
  await expect(header.getByRole('button', { name: '打开设置' })).toHaveCount(0)
  await header.getByRole('button', { name: '打开终端' }).click()
  const panel = page.getByRole('complementary', { name: '终端面板' })
  const directoryTabs = panel.getByRole('button', { name: 'fixture', exact: true })
  const tab = directoryTabs.first()
  await expect(tab).toBeVisible()
  await expect(tab).toHaveAttribute('title', '/fixture')
  const addTerminal = panel.getByRole('button', { name: '添加终端' })
  await expect(addTerminal).toHaveCSS('border-width', '0px')
  await addTerminal.hover()
  await expect(addTerminal).toHaveCSS('border-width', '0px')
  await expect(panel).toHaveCSS('border-left-width', '0px')
  await expect(panel.getByRole('button', { name: '收起终端面板' })).toHaveCount(0)
  const terminalHeader = panel.locator('header')
  await expect(terminalHeader).toHaveCSS('border-bottom-width', '0px')
  const terminalMore = terminalHeader.getByRole('button', { name: '终端更多操作' })
  const terminalActions = terminalHeader.getByRole('region', { name: '终端操作菜单' })
  await expect(terminalMore).toHaveCSS('border-width', '0px')
  await expect(terminalHeader.getByRole('button', { name: '复制输出' })).toBeHidden()
  await expect(terminalHeader.getByRole('button', { name: '下载输出' })).toBeHidden()
  const terminalOutput = panel.locator('.terminal-panel-output')
  const terminal = terminalOutput.locator('.xterm')
  const outputRows = terminal.locator('.xterm-rows')
  await expect(terminal).toBeVisible()
  await expect(outputRows).toContainText('terminal fixture')
  await terminalMore.click()
  await expect(terminalActions.getByRole('button')).toHaveText(['复制输出', '下载输出'])
  await terminalActions.getByRole('button', { name: '复制输出' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-copied-terminal-output', 'terminal fixture')
  await expect(terminalActions).toBeHidden()
  await expect(terminalMore).toBeFocused()
  await terminalMore.click()
  const download = page.waitForEvent('download')
  await terminalActions.getByRole('button', { name: '下载输出' }).click()
  expect((await download).suggestedFilename()).toBe('fixture.log')
  await expect(terminalActions).toBeHidden()
  const originalTerminal = await terminal.elementHandle()
  await expect(terminalOutput).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await expect(outputRows).toHaveCSS('color', 'rgb(23, 23, 23)')
  const themeMenu = header.getByRole('region', { name: '操作菜单' })
  const themeMenuTrigger = header.getByRole('button', { name: '操作', exact: true })
  await themeMenuTrigger.click()
  await themeMenu.getByRole('button', { name: '切换到暗色' }).click()
  await expect(terminalOutput).toHaveCSS('background-color', 'rgb(11, 11, 11)')
  await expect(outputRows).toHaveCSS('color', 'rgb(245, 245, 245)')
  await themeMenu.getByRole('button', { name: '切换到亮色' }).click()
  await expect(outputRows).toHaveCSS('color', 'rgb(23, 23, 23)')
  await themeMenuTrigger.click()
  expect(await originalTerminal?.evaluate(node => node.isConnected)).toBe(true)
  expect(created).toBe(1)
  const titleBounds = (await terminalHeader.boundingBox())!
  const outputBounds = (await terminalOutput.boundingBox())!
  expect(outputBounds.x).toBe(titleBounds.x)
  expect(outputBounds.width).toBe(titleBounds.width)
  expect(outputBounds.y).toBe(titleBounds.y + titleBounds.height)
  const bounds = (await panel.boundingBox())!
  expect(bounds.x + bounds.width).toBeCloseTo(1600, 1)
  const resizedHeaderBounds = (await header.boundingBox())!
  expect(await projectRail.boundingBox()).toEqual(leftBounds)
  expect(resizedHeaderBounds.x + resizedHeaderBounds.width).toBeCloseTo(bounds.x, 1)
  expect(resizedHeaderBounds.y).toBe(bounds.y)
  const splitter = page.locator('.workbench-terminal-splitter')
  const availableWidth = (await splitter.boundingBox())!.width
  expect(bounds.width).toBeCloseTo(availableWidth * 0.25, 0)
  await expect(page.getByRole('dialog', { name: '终端', exact: true })).toHaveCount(0)

  const separator = page.getByRole('separator', { name: '调整终端宽度' })
  const divider = separator.locator('.ant-splitter-bar-dragger')
  await expect(divider).toBeVisible()
  expect(await divider.evaluate(node => getComputedStyle(node, '::before').content)).toBe('none')
  expect(await divider.evaluate(node => getComputedStyle(node, '::after').content)).toBe('none')
  const handle = (await divider.boundingBox())!
  const x = handle.x + handle.width / 2
  const y = handle.y + handle.height / 2
  await page.mouse.move(x, y)
  await page.mouse.down()
  await page.mouse.move(x - 70, y, { steps: 6 })
  await page.mouse.up()
  const width = async () => (await panel.boundingBox())?.width ?? 0
  await expect.poll(width).toBeGreaterThan(bounds.width + 40)
  const draggedWidth = await width()
  await separator.press('ArrowRight')
  await expect.poll(width).toBeLessThan(draggedWidth - 15)
  const resizedWidth = await width()
  await expect.poll(() => page.evaluate(() => JSON.stringify(localStorage)))
    .toContain('workbench.splitter')

  await header.getByRole('button', { name: '收起终端面板' }).click()
  await expect(panel).toBeHidden()
  await expect(separator).toBeHidden()
  await header.getByRole('button', { name: '打开终端' }).click()
  await expect(tab).toBeVisible()
  expect(created).toBe(1)
  expect(deleted).toBe(0)
  await expect.poll(width).toBe(resizedWidth)

  await page.reload()
  await header.getByRole('button', { name: '打开终端' }).click()
  await expect(tab).toBeVisible()
  await expect.poll(width).toBe(resizedWidth)
  expect(created).toBe(1)

  await page.setViewportSize({ width: 390, height: 760 })
  await expect(panel).toBeVisible()
  await expect(separator).toBeHidden()
  const rail = page.getByRole('navigation', { name: '工作台工具' })
  await expect(rail).toBeVisible()
  const railBounds = (await rail.boundingBox())!
  await expect.poll(width).toBe(390 - railBounds.width)
  expect((await panel.boundingBox())!.y).toBe(
    (await header.boundingBox())!.y + (await header.boundingBox())!.height
  )
  const settings = rail.getByRole('button', { name: '打开设置', exact: true })
  await settings.click()
  const settingsDialog = page.getByRole('dialog', { name: '设置', exact: true })
  await expect(settingsDialog).toBeVisible()
  await settingsDialog.getByRole('button', { name: '关闭设置', exact: true }).click()
  await addTerminal.click()
  await expect(directoryTabs).toHaveCount(2)
  await expect(directoryTabs.last()).toHaveAttribute('aria-pressed', 'true')
  const closeTabs = panel.getByRole('button', { name: '关闭终端 fixture', exact: true })
  await closeTabs.first().click()
  await expect(directoryTabs).toHaveCount(1)
  await expect.poll(() => deleted).toBe(1)
  await expect(tab).toHaveAttribute('aria-pressed', 'true')
  await closeTabs.click()
  await expect(panel).toHaveCount(0)
  await expect.poll(() => deleted).toBe(2)
  const more = header.getByRole('button', { name: '操作', exact: true })
  const actions = header.getByRole('region', { name: '操作菜单' })
  await expect(header.getByRole('button', { name: '打开终端' })).toBeHidden()
  await more.click()
  await actions.getByRole('button', { name: '打开终端' }).click()
  await expect(actions).toBeHidden()
  await expect(tab).toBeVisible()
  expect(created).toBe(3)
  for (const count of [2, 3, 4]) {
    await addTerminal.click()
    await expect(directoryTabs).toHaveCount(count)
    await expect(directoryTabs.last()).toHaveAttribute('aria-pressed', 'true')
  }
  const tabList = panel.getByRole('navigation', { name: '终端列表' })
  await expect(tabList).toHaveCSS('scrollbar-width', 'none')
  await expect.poll(() => tabList.evaluate(node => node.scrollLeft)).toBeGreaterThan(0)
  await expect(addTerminal).toBeInViewport()
  await expect(terminalMore).toBeInViewport()
  await terminalMore.click()
  await expect(terminalActions.getByRole('button', { name: '复制输出' })).toBeInViewport()
  await terminalActions.getByRole('button', { name: '下载输出' }).press('Escape')
  await expect(terminalActions).toBeHidden()
  await expect(terminalMore).toBeFocused()
  await tab.click()
  await expect(tab).toBeInViewport({ ratio: 1 })
  await expect(tab).toHaveAttribute('aria-pressed', 'true')
  await more.click()
  await actions.getByRole('button', { name: '收起终端面板' }).click()
  await expect(panel).toBeHidden()
  await settings.click()
  await expect(settingsDialog).toBeVisible()
})

test('native workbench docks its terminal and opens settings in a standalone page', async ({
  page
}) => {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: {
        messageHandlers: {
          external: {
            postMessage: (message: string) => {
              document.documentElement.dataset.lastNativeMessage = message
            }
          }
        }
      }
    })
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.route('**/api/v1/terminal/sessions', route =>
    route.fulfill({
      json: { id: 'fixture', key: 'fixture-key', cwd: '/fixture', shell: '/bin/sh' }
    })
  )
  await page.route('**/api/v1/terminal/sessions/fixture/**', route =>
    route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' })
  )
  await page.goto('/dashboard/robot')
  const tools = page.getByRole('button', { name: '操作', exact: true })
  const terminal = page.getByRole('button', { name: '打开终端', exact: true })
  await expect(terminal).toBeVisible()
  expect((await terminal.boundingBox())!.x).toBeGreaterThan(
    (await tools.boundingBox())!.x
  )
  await terminal.click()
  await expect(page.getByRole('complementary', { name: '终端面板' })
    .getByRole('button', { name: 'fixture', exact: true })).toBeVisible()
  await expect(page.locator('html')).not.toHaveAttribute('data-last-native-message', /^alx:open-window:terminal:/)
  await page.getByRole('button', { name: '打开设置', exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute(
    'data-last-native-message',
    'alx:open-window:settings:'
  )
  await page.goto('/desktop-window/settings#section=network')
  await expect(
    page.getByRole('tab', { name: '网络', exact: true })
  ).toHaveAttribute('aria-selected', 'true')
  await expect(page.locator('.native-auxiliary-content')).toBeVisible()
  await expect(page.locator('.workbench-stage')).toHaveCount(0)
})

test('native terminal restores its session and robot application embeds the existing route', async ({
  page
}) => {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: { messageHandlers: { external: { postMessage: (message: string) => {
        document.documentElement.dataset.lastNativeMessage = message
      } } } }
    })
    if (!sessionStorage.getItem('alx-terminal-sessions:/fixture')) {
      sessionStorage.setItem(
        'alx-terminal-sessions:/fixture',
        JSON.stringify([
          { id: 'existing', key: 'fixture-key', label: '终端 1' }
        ])
      )
    }
  })
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  let created = 0
  await page.route('**/api/v1/terminal/sessions', route => {
    created++
    return route.fulfill({ status: 500 })
  })
  await page.route('**/api/v1/terminal/sessions/existing', route =>
    route.fulfill({
      json: { cwd: '\\\\server\\share\\测试机器人\\', shell: 'powershell.exe', output: 'hello' }
    })
  )
  await page.route('**/api/v1/terminal/sessions/existing/**', route =>
    route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' })
  )
  await page.goto('/desktop-window/terminal?root=%2Ffixture')
  await expect(page.getByRole('button', { name: '测试机器人', exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('button', { name: '测试机器人', exact: true })).toBeVisible()
  expect(created).toBe(0)
  const deleted = page.waitForRequest(request =>
    request.method() === 'DELETE' && new URL(request.url()).pathname.endsWith('/terminal/sessions/existing')
  )
  await page.getByRole('button', { name: '关闭终端 测试机器人', exact: true }).click()
  await deleted
  await expect(page.locator('html')).toHaveAttribute('data-last-native-message', 'alx:window:close')

  const appPath = '/api/v1/robot/app/L2ZpeHR1cmU/'
  await page.route('**' + appPath, route =>
    route.fulfill({
      contentType: 'text/html',
      body: '<main>机器人应用内容</main>'
    })
  )
  await page.goto(
    '/desktop-window/robot-app?' + new URLSearchParams({ path: appPath })
  )
  await expect(
    page.frameLocator('iframe').getByText('机器人应用内容')
  ).toBeVisible()
})

test('Git clone keeps its themed footer visible on a narrow window', async ({
  page
}) => {
  await page.setViewportSize({ width: 1180, height: 760 })
  await page.route('**/api/v1/**', route =>
    route.fulfill({ json: { items: [], keys: [] } })
  )
  await page.route('**/api/v1/workspace/robots', route =>
    route.fulfill({
      json: { scope: 'test', local: true, initialized: true, projects: [], activeProjectID: '' }
    })
  )
  await page.route('**/api/v1/auth/status', route =>
    route.fulfill({ json: { enabled: false, authenticated: false } })
  )
  await page.route('**/api/v1/goals', route => route.fulfill({ json: [] }))
  await page.route('**/api/v1/checks', route => route.fulfill({ json: { checks: [] } }))
  await page.route('**/api/v1/robot/tasks', route => route.fulfill({ json: [] }))
  await page.goto('/dashboard/robot')
  await page.getByRole('button', { name: '从 Git 克隆', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 540 })
  const dialog = page.getByRole('dialog', { name: '从 Git 克隆机器人' }).last()
  const submit = dialog.getByRole('button', { name: '克隆并添加' })
  await expect(submit).toBeInViewport()
  await expect(submit).toBeDisabled()
  await dialog.getByRole('tab', { name: 'HTTPS' }).click()
  await expect(dialog.getByLabel('仓库地址')).toHaveAttribute(
    'placeholder',
    /^https:/
  )
  await expect(dialog.getByRole('group', { name: '克隆设置' })).toBeVisible()
  await expect(dialog.getByRole('group', { name: '存放位置' })).toBeVisible()
  expect(await dialog.evaluate(element => element.scrollWidth)).toBeLessThanOrEqual(390)
})

test('macOS tray panel shows service status and sends desktop actions', async ({ page }) => {
  await page.setViewportSize({ width: 316, height: 186 })
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'platform', { value: 'MacIntel' })
    Object.assign(window, {
      webkit: { messageHandlers: { external: {
        postMessage: (message: string) => {
          document.documentElement.dataset.lastNativeMessage = message
        }
      } } }
    })
  })
  await page.route('**/healthz', route =>
    route.fulfill({ json: { status: 'ok', version: 'v1.0.0' } })
  )
  let mode = 'auto'
  await page.route('**/api/v1/system/network', route => {
    if (route.request().method() === 'PUT') mode = route.request().postDataJSON().mode
    return route.fulfill({ json: {
      version: 2, revision: 1, mode, automatic: { groups: [] }, proxy: { url: '' }
    } })
  })
  await page.goto('/desktop-window/tray-panel')
  const panel = page.getByRole('main', { name: 'ALemonX 菜单' })
  await expect(panel.getByRole('status')).toHaveText('工作台在线')
  await expect(panel.getByText('v1.0.0')).toBeVisible()
  const bounds = { x: 0, y: 0, width: 316, height: 186 }
  expect(await panel.boundingBox()).toEqual(bounds)
  await page.evaluate(() => {
    window.dispatchEvent(new Event('blur'))
    window.dispatchEvent(new Event('focus'))
  })
  expect(await panel.boundingBox()).toEqual(bounds)
  await expect(panel).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(panel).toHaveCSS('color', 'rgb(241, 241, 244)')
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(panel).toHaveCSS('color', 'rgb(32, 32, 36)')
  await expect(page.locator('body')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(panel.getByRole('button', { name: '打开终端' })).toHaveCount(0)
  expect(await panel.evaluate(node => getComputedStyle(node, '::before').content)).toBe('none')
  const modes = panel.getByRole('group', { name: '网络模式' })
  await expect(modes.getByRole('button', { name: '自动' })).toHaveAttribute('aria-pressed', 'true')
  await modes.getByRole('button', { name: '直连' }).click()
  await expect(modes.getByRole('button', { name: '直连' })).toHaveAttribute('aria-pressed', 'true')
  await modes.getByRole('button', { name: '代理' }).click()
  await expect(panel.getByRole('alert')).toContainText('配置代理地址')
  await panel.getByRole('button', { name: /^切换到/ }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(panel).toHaveCSS('color', 'rgb(32, 32, 36)')
  await panel.getByRole('button', { name: '更多' }).click()
  for (const [label, action] of [['切换开发模式', 'developer-mode'], ['打开', 'open'], ['重启应用', 'restart'], ['退出', 'quit']]) {
    const button = panel.getByRole('button', { name: label, exact: true })
    await expect(button).toBeInViewport()
    await button.click()
    await expect(page.locator('html')).toHaveAttribute(
      'data-last-native-message',
      `alx:tray:${action}`
    )
  }
})

test('native environment window installs and updates managed Bun', async ({ page }) => {
  await page.addInitScript(() => {
    Object.assign(window, {
      webkit: { messageHandlers: { external: { postMessage: () => {} } } }
    })
  })
  const actions: string[] = []
  let version = ''
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    switch (path) {
      case '/api/v1/auth/status':
        return route.fulfill({ json: { enabled: false } })
      case '/api/v1/checks':
        return route.fulfill({ json: { checks: [], platform: 'darwin/arm64' } })
      case '/api/v1/system/bun':
        return route.fulfill({
          json: {
            check: {
              id: 'bun',
              name: 'Bun',
              status: version ? 'ready' : 'missing',
              detail: version || '未检测到',
              suggestion: '请安装 Bun 后重新检测。'
            },
            version,
            path: version ? '/cache/alemonx/environments/bun/bin/bun' : '',
            managed: Boolean(version),
            platform: 'darwin/arm64',
            container: false
          }
        })
      case '/api/v1/system/environment/install': {
        const { checkId, confirm } = route.request().postDataJSON()
        expect(confirm).toBe(true)
        actions.push(checkId)
        version = checkId === 'bun' ? '1.4.4' : '1.4.5'
        return route.fulfill({ json: { output: '操作完成' } })
      }
      default:
        return route.fulfill({ json: {} })
    }
  })

  await page.goto('/desktop-window/environment')
  await page.getByRole('button', { name: 'Bun', exact: true }).click()
  const panel = page.getByRole('region', { name: 'Bun 版本管理' })
  await expect(panel.getByText('未检测到', { exact: true })).toBeVisible()
  await panel.getByRole('button', { name: '安装 Bun', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '环境修复' })
  await dialog.getByRole('button', { name: '安装', exact: true }).click()
  await dialog.getByRole('button', { name: '完成', exact: true }).click()
  await expect(panel.getByText('1.4.4', { exact: true })).toBeVisible()
  await expect(panel.getByText('工作台托管', { exact: true })).toBeVisible()
  await panel.getByRole('button', { name: '检查并更新 Bun' }).click()
  await dialog.getByRole('button', { name: '升级', exact: true }).click()
  await dialog.getByRole('button', { name: '完成', exact: true }).click()
  await expect(panel.getByText('1.4.5', { exact: true })).toBeVisible()
  expect(actions).toEqual(['bun', 'bun-update'])
})
