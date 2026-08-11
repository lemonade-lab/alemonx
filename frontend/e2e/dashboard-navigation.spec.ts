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
    agentOpen: false
  })

  const search = writeDashboardNavigation(
    '?window=terminal',
    {
      root: '/tmp/bot',
      page: 'robot',
      section: 'config',
      buildMode: 'git',
      configEditor: 'text',
      agentOpen: false,
      sessionID: ''
    }
  )

  expect(search).toBe(
    '?window=terminal&page=robot&root=%2Ftmp%2Fbot&section=config&editor=text'
  )
})

test('agent links take precedence and discard incompatible dashboard modes', () => {
  const navigation = readDashboardNavigation(
    '?page=build&build=npm&agent=1&session=session-1'
  )

  expect(navigation).toMatchObject({
    page: 'robot',
    section: 'runtime',
    buildMode: 'git',
    agentOpen: true,
    sessionID: 'session-1'
  })
})
