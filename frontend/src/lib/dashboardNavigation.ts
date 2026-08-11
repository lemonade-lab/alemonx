export type DashboardPage = 'robot' | 'build' | 'plugins' | 'connections'
export type DashboardSection =
  | 'backpack'
  | 'config'
  | 'npmrc'
  | 'env'
  | 'runtime'
export type DashboardBuildMode = 'manifest' | 'npm' | 'git'
export type DashboardConfigEditor = 'visual' | 'text'

export type DashboardNavigation = {
  root: string
  page: DashboardPage
  section: DashboardSection
  buildMode: DashboardBuildMode
  configEditor: DashboardConfigEditor
  agentOpen: boolean
  sessionID: string
  hasPage: boolean
}

const dashboardPages = new Set<DashboardPage>([
  'robot',
  'build',
  'plugins',
  'connections'
])
const dashboardSections = new Set<DashboardSection>([
  'backpack',
  'config',
  'npmrc',
  'env',
  'runtime'
])
const dashboardBuildModes = new Set<DashboardBuildMode>([
  'manifest',
  'npm',
  'git'
])
const dashboardConfigEditors = new Set<DashboardConfigEditor>([
  'visual',
  'text'
])

/**
 * The query string is the durable contract for dashboard navigation. Keep it
 * intentionally small: content selection belongs here, window geometry and
 * sensitive form data do not.
 */
export function readDashboardNavigation(search: string): DashboardNavigation {
  const parameters = new URLSearchParams(search)
  const requestedPage = parameters.get('page')
  const requestedSection = parameters.get('section')
  const requestedBuildMode = parameters.get('build')
  const requestedConfigEditor = parameters.get('editor')
  const agentOpen = parameters.get('agent') === '1'
  const page = agentOpen
    ? 'robot'
    : dashboardPages.has(requestedPage as DashboardPage)
      ? (requestedPage as DashboardPage)
      : 'robot'
  const section =
    page === 'robot' && dashboardSections.has(requestedSection as DashboardSection)
      ? (requestedSection as DashboardSection)
      : 'runtime'

  return {
    root: parameters.get('root') ?? '',
    page,
    section,
    buildMode:
      page === 'build' && dashboardBuildModes.has(requestedBuildMode as DashboardBuildMode)
        ? (requestedBuildMode as DashboardBuildMode)
        : 'git',
    configEditor:
      page === 'robot' &&
      section === 'config' &&
      dashboardConfigEditors.has(requestedConfigEditor as DashboardConfigEditor)
        ? (requestedConfigEditor as DashboardConfigEditor)
        : 'visual',
    agentOpen,
    sessionID: agentOpen ? parameters.get('session') ?? '' : '',
    hasPage: requestedPage !== null
  }
}

export function writeDashboardNavigation(
  search: string,
  navigation: Omit<DashboardNavigation, 'hasPage'>
) {
  const parameters = new URLSearchParams(search)
  const page = navigation.agentOpen ? 'robot' : navigation.page

  parameters.set('page', page)
  if (navigation.root) parameters.set('root', navigation.root)
  else parameters.delete('root')

  if (page === 'robot') parameters.set('section', navigation.section)
  else parameters.delete('section')

  if (page === 'build') parameters.set('build', navigation.buildMode)
  else parameters.delete('build')

  if (page === 'robot' && navigation.section === 'config')
    parameters.set('editor', navigation.configEditor)
  else parameters.delete('editor')

  if (page === 'robot' && navigation.agentOpen) {
    parameters.set('agent', '1')
    if (navigation.sessionID) parameters.set('session', navigation.sessionID)
    else parameters.delete('session')
  } else {
    parameters.delete('agent')
    parameters.delete('session')
  }

  const value = parameters.toString()
  return value ? `?${value}` : ''
}
