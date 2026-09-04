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

test('narrow workbench, Git, and operations windows respond to their own width', async ({ page }) => {
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

  const result = await page.locator('#window-responsive-fixture').evaluate(node => {
    const workbench = node.querySelector<HTMLElement>('.dashboard-window')!
    const rail = node.querySelector<HTMLElement>('.project-rail')!
    const consolePage = node.querySelector<HTMLElement>('.console-page')!
    const gitTagForm = node.querySelector<HTMLElement>('.git-tag-form')!
    const opsPanel = node.querySelector<HTMLElement>('.ops-panel')!
    const opsMetrics = node.querySelector<HTMLElement>('.ops-metrics')!
    const opsEventGrid = node.querySelector<HTMLElement>('.ops-event-grid')!
    const opsPolicyFields = node.querySelector<HTMLElement>('.ops-policy-fields')!
    const testone = node.querySelector<HTMLElement>('.testone-window')!
    const testoneChannelRail = testone.querySelector<HTMLElement>('.testone-channel-rail')!
    const testoneTaskRail = testone.querySelector<HTMLElement>('.testone-task-rail')!
    const testoneMobileUser = testone.querySelector<HTMLElement>('.testone-mobile-user')!
    const testoneTimerTip = testone.querySelector<HTMLElement>('.testone-timer-tip')!
    const testoneEvents = testone.querySelector<HTMLElement>('.testone-special-events')!
    const testoneMedium = node.querySelector<HTMLElement>('.testone-medium-fixture')!
    const testoneMediumChannelRail = testoneMedium.querySelector<HTMLElement>('.testone-channel-rail')!
    const testoneMediumTaskRail = testoneMedium.querySelector<HTMLElement>('.testone-task-rail')!
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
      testoneEventColumns: getComputedStyle(testoneEvents).gridTemplateColumns,
      testoneMediumChannelDisplay: getComputedStyle(testoneMediumChannelRail).display,
      testoneMediumTaskDisplay: getComputedStyle(testoneMediumTaskRail).display
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
  test(`phone window chrome fills ${phone.width}px without horizontal overflow`, async ({ page }) => {
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

test('phone shell preserves reachable controls and localizes overflow', async ({ page }) => {
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
    const close = node.querySelector<HTMLElement>('[aria-label="关闭智能日志"]')!
    const nav = node.querySelector<HTMLElement>('[data-sidebar-window-nav] > button')!
    const popover = node.querySelector<HTMLElement>('.operation-tasks-popover')!
    const pre = node.querySelector<HTMLElement>('pre')!
    return {
      windowRect: { left: windowRect.left, right: windowRect.right, height: windowRect.height },
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
