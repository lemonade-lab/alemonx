import { expect, test } from '@playwright/test'
import { clampWindowRectToViewport } from '../src/lib/windowRect'

test('shrinking the viewport clamps the display without forgetting the preferred rect', () => {
  const preferred = { left: 120, top: 80, width: 1200, height: 700 }
  const options = { minWidth: 440, minHeight: 320 }

  const smallViewport = clampWindowRectToViewport(preferred, options, {
    width: 800,
    height: 600
  })
  expect(smallViewport.width).toBe(752)
  expect(smallViewport.height).toBe(552)
  expect(smallViewport.left).toBe(32)
  expect(smallViewport.top).toBe(32)

  // Growing the viewport back restores the remembered size automatically.
  expect(
    clampWindowRectToViewport(preferred, options, { width: 1440, height: 900 })
  ).toEqual(preferred)
})

test('workbench clamp keeps its minimum size and restores on a wider viewport', () => {
  const preferred = { left: 200, top: 100, width: 1240, height: 760 }
  const options = {
    minWidth: 640,
    minHeight: 420,
    gutter: 32,
    minViewportWidth: 640,
    minViewportHeight: 420
  }

  const smallViewport = clampWindowRectToViewport(preferred, options, {
    width: 900,
    height: 700
  })
  expect(smallViewport.width).toBe(868)
  expect(smallViewport.width).toBeGreaterThanOrEqual(640)

  const restored = clampWindowRectToViewport(preferred, options, {
    width: 1400,
    height: 900
  })
  expect(restored.width).toBe(1240)
  expect(restored.height).toBe(760)
  // The preferred position is re-anchored so the window still fits.
  expect(restored.left).toBe(144)
  expect(restored.top).toBe(100)
})

test('clamping re-anchors the position without shrinking the preferred size', () => {
  const preferred = { left: 1000, top: 900, width: 800, height: 500 }
  const clamped = clampWindowRectToViewport(
    preferred,
    { minWidth: 440, minHeight: 320 },
    { width: 1000, height: 700 }
  )

  expect(clamped.width).toBe(800)
  expect(clamped.height).toBe(500)
  expect(clamped.left).toBe(184)
  expect(clamped.top).toBe(184)
})

test('phone-sized viewports never place a clamped window outside the viewport', () => {
  const clamped = clampWindowRectToViewport(
    { left: 64, top: 56, width: 860, height: 620 },
    { minWidth: 440, minHeight: 320 },
    { width: 320, height: 568 }
  )

  expect(clamped.left).toBe(0)
  expect(clamped.width).toBe(320)
  expect(clamped.left + clamped.width).toBeLessThanOrEqual(320)
})
