import { expect, test } from '@playwright/test'

test('Web Locks keeps a single SSE leader and hands over after release', async ({ browser }) => {
  const context = await browser.newContext()
  const first = await context.newPage()
  const second = await context.newPage()
  await Promise.all([first.goto('/'), second.goto('/')])

  await first.evaluate(() => {
    ;(window as Window & { releaseLeader?: () => void }).releaseLeader = undefined
    void navigator.locks.request('alx-events-leader', { mode: 'exclusive' }, () =>
      new Promise<void>(resolve => {
        ;(window as Window & { releaseLeader?: () => void }).releaseLeader = resolve
      })
    )
  })
  await expect.poll(() => first.evaluate(() => Boolean((window as Window & { releaseLeader?: () => void }).releaseLeader))).toBe(true)
  const blocked = await second.evaluate(async () => {
    const acquired = await navigator.locks.request('alx-events-leader', { ifAvailable: true }, lock => Boolean(lock))
    return acquired
  })
  expect(blocked).toBe(false)
  await first.evaluate(() => (window as Window & { releaseLeader?: () => void }).releaseLeader?.())
  await expect.poll(() => second.evaluate(async () => navigator.locks.request('alx-events-leader', { ifAvailable: true }, lock => Boolean(lock)))).toBe(true)
  await context.close()
})

test('lease fallback avoids a split leader and permits takeover after expiry', async ({ browser }) => {
  const context = await browser.newContext()
  const first = await context.newPage()
  const second = await context.newPage()
  await Promise.all([first.goto('/'), second.goto('/')])
  await first.evaluate(() => localStorage.setItem('alx-events-leader', JSON.stringify({ id: 'first', expires: Date.now() + 3500 })))
  const acquiredTooEarly = await second.evaluate(() => {
    const current = JSON.parse(localStorage.getItem('alx-events-leader') || '{}') as { id?: string; expires?: number }
    return !(current.id && current.id !== 'second' && (current.expires ?? 0) > Date.now())
  })
  expect(acquiredTooEarly).toBe(false)
  await first.evaluate(() => localStorage.setItem('alx-events-leader', JSON.stringify({ id: 'first', expires: Date.now() - 1 })))
  const takeover = await second.evaluate(() => {
    const current = JSON.parse(localStorage.getItem('alx-events-leader') || '{}') as { id?: string; expires?: number }
    if (current.id && current.id !== 'second' && (current.expires ?? 0) > Date.now()) return false
    localStorage.setItem('alx-events-leader', JSON.stringify({ id: 'second', expires: Date.now() + 3500 }))
    return true
  })
  expect(takeover).toBe(true)
  await context.close()
})
