import { expect, test } from '@playwright/test'

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
