import { expect, test } from '@playwright/test'
import { clampPopoverAxis } from '../src/hooks/useViewportPopoverPosition'

test('viewport popover keeps a 12px edge gutter', () => {
  expect(clampPopoverAxis(310, 100, 12, 320 - 12)).toBe(208)
  expect(clampPopoverAxis(-20, 100, 12, 308)).toBe(12)
})

test('viewport popover remains usable above a shortened keyboard viewport', () => {
  expect(clampPopoverAxis(490, 120, 12, 400 - 12)).toBe(268)
})
