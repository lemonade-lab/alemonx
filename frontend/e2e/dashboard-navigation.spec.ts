import { expect, test } from '@playwright/test'
import {
  readDashboardNavigation,
  writeDashboardNavigation
} from '../src/lib/dashboardNavigation'

test('dashboard navigation query restores only durable navigation state', () => {
  const navigation = readDashboardNavigation(
    '?root=%2Ftmp%2Fbot&page=build&build=npm&editor=text&window=terminal'
  )

  expect(navigation).toMatchObject({
    root: '/tmp/bot',
    page: 'build',
    buildMode: 'npm',
    configEditor: 'visual',
    dshOpen: false
  })

  const search = writeDashboardNavigation('?window=terminal', {
    root: '/tmp/bot',
    page: 'robot',
    section: 'config',
    buildMode: 'git',
    configEditor: 'text',
    dshOpen: false
  })

  expect(search).toBe(
    '?window=terminal&page=robot&root=%2Ftmp%2Fbot&section=config&editor=text'
  )
})

test('dsh links take precedence and discard incompatible dashboard modes', () => {
  const navigation = readDashboardNavigation('?page=build&build=npm&dsh=1')

  expect(navigation).toMatchObject({
    page: 'robot',
    section: 'runtime',
    buildMode: 'git',
    dshOpen: true
  })
})

test('writing navigation removes retired agent and session parameters', () => {
  const search = writeDashboardNavigation('?agent=1&session=old', {
    root: '/tmp/bot',
    page: 'robot',
    section: 'runtime',
    buildMode: 'git',
    configEditor: 'visual',
    dshOpen: true
  })
  const params = new URLSearchParams(search)
  expect(params.get('dsh')).toBe('1')
  expect(params.has('agent')).toBe(false)
  expect(params.has('session')).toBe(false)
})
