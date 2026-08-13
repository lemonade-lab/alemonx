import { createApi, fetchBaseQuery } from '@reduxjs/toolkit/query/react'

type RobotResult = { output: string; path?: string }
// ConsolePayload splits the terminal's live process output from its static
// project context so the terminal can render them at different refresh rates.
type ConsolePayload = {
  output: string
  snapshot: string
  running: boolean
  mode: string
  path?: string
}
type RobotTask = {
  id: string
  root: string
  action: string
  status: 'running' | 'completed' | 'failed'
  output?: string
  error?: string
  createdAt?: string
  finishedAt?: string
}
type CatalogGroup = {
  title: string
  items: Array<{
    name: string
    description: string
    url: string
    install: string
  }>
}
type CatalogDocument = { source: string; markdown: string }
type CatalogVersions = { latest: string; versions: string[] }
export type PackageConfigField = {
  name: string
  type: string
  required: boolean
  description: string
  default?: unknown
  rules?: Array<{ pattern: string; message?: string }>
  config?: PackageConfigField[]
}
export type PackageConfig = {
  package: string
  namespace: string
  fields: PackageConfigField[]
  values: Record<string, unknown>
  configSource?: {
    readme?: string
    official?: string
    platform?: string
  }
  logo?: string
  commands?: Array<{ name: string; command: string }>
  webServerPort?: boolean
}
type WebViewEntry = {
  id: string
  package: string
  name: string
  description?: string
  logo?: string
  requiresServerPort?: boolean
}
type LocalPackages = {
  items: Array<{
    name: string
    version?: string
    description?: string
    path: string
    valid: boolean
  }>
}
type LocalPackageVersions = {
  source: 'git' | 'npm'
  current: string
  latest?: string
  versions: string[]
}
export type RuntimeOverview = {
  name: string
  version: string
  packageManager: string
  hasAppScript: boolean
  hasDevScript: boolean
  hasBuildScript: boolean
  hasStartScript: boolean
  pm2Configured: boolean
  dependenciesComplete: boolean
  platforms: Array<{
    id: string
    label: string
    package: string
    declared: boolean
    installed: boolean
    version?: string
    source?: 'builtin' | 'declared'
    logo?: string
  }>
}
export type PM2Status = {
  configured: boolean
  managed: boolean
  running: boolean
  status?: string
}
export type PM2Process = {
  id: number
  name: string
  namespace: string
  status: string
  pid: number
  memory: number
  cpu: number
  uptime: number
  restarts: number
  script: string
}
export type SystemNetworkMode =
  'system' | 'manual' | 'direct' | 'mirror' | 'custom-mirror'
export type SystemNetworkRoute =
  'github' | 'gitee' | 'npm' | 'node' | 'cdn' | 'official'
export type SystemNetworkRouteSettings = {
  mode: SystemNetworkMode
  mirrorUrl?: string
  proxyUrl?: string
  hasCredentials?: boolean
}
export type SystemNetworkMirrorPreset = {
  label: string
  value: string
}
export type SystemNetworkSettings = {
  routes: Record<SystemNetworkRoute, SystemNetworkRouteSettings>
  mirrorPresets?: Partial<
    Record<SystemNetworkRoute, SystemNetworkMirrorPreset[]>
  >
}
export type SystemNetworkCheck = {
  ok: boolean
  target: string
  status?: number
  latencyMs?: number
  message: string
}
export type SystemRedisStatus = {
  running: boolean
  managed: boolean
  external: boolean
  skipped: boolean
  port: number
  address: string
  message: string
  autoStart: boolean
  disabled: boolean
}
export type SystemCurrentRobot = {
  root: string
  name: string
} | null
export type GitWorkspace = {
  root: string
  repository: boolean
  gitRoot?: string
  remote?: string
  branch?: string
  upstream?: string
  ahead: number
  behind: number
  changes: Array<{ status: string; path: string }>
  branches: Array<{ name: string; current: boolean; upstream?: string }>
  remoteBranches: Array<{ name: string; remote: string; branch: string }>
  commits: Array<{
    sha: string
    shortSha: string
    subject: string
    createdAt: string
  }>
  tags: Array<{ name: string; subject?: string; createdAt?: string }>
  remotes: Array<{ name: string; url: string }>
}
export type RuntimePreflight = {
  login: string
  package?: string
  missing: string[]
  summary: string[]
  dependenciesComplete: boolean
}
export type RuntimeRepairPlan = {
  phase: string
  profile: string
  mode: string
  automatic: string[]
  requiresConfirmation: string[]
  blocked: string[]
  diagnostics: string[]
}
export type RuntimeRepairResult = RuntimeRepairPlan & {
  backupPath?: string
  output?: string
}
export type RobotPortStatus = {
  kind: string
  label: string
  port: number
  configured: boolean
  occupied: boolean
  pid?: number
  process?: string
  owned?: boolean
  error?: string
}
type PackageManifest = {
  name: string
  version: string
  description: string
  homepage: string
  repository: string
  license: string
  private: boolean
  access: string
}
export type SetupPlugin = {
  id: string
  name: string
  version: string
  description?: string
  platforms?: string[]
  navigation: { label: string; icon?: string; order?: number }
  web?: { root: string }
  runnable: boolean
  enabled: boolean
  online?: boolean
  // Filled only for a locally discovered (therefore installed) system plugin.
  // This is the registry's actual directory, not a guessed cache path.
  source?: string
  installedTag?: string
  fingerprint?: string
  developmentSource?: boolean
  developmentWebProxy?: boolean
}
export type SetupPluginDevelopment = {
  id: string
  name: string
  source: string
  registered: boolean
  running: boolean
  state:
    | 'registered'
    | 'starting'
    | 'running'
    | 'stopping'
    | 'stopped'
    | 'building'
    | 'failed'
  busy: boolean
  runner?: string
  webMode?: string
  buildAvailable: boolean
  webUrl?: string
  webPort?: number
  sourceType: string
  privileges?: string[]
  services?: { id: string; port?: number; running: boolean; restart?: string }[]
  lastError?: string
  updatedAt: string
}
export type SetupPluginRelease = {
  tag: string
  name: string
  url: string
  publishedAt: string
  assets: Array<{
    name: string
    url: string
    size: number
    sha256?: string
    compatible: boolean
  }>
}
export type SetupPluginVersion = {
  tag: string
  asset: string
  size: number
  archiveSha256?: string
  fingerprint?: string
  active: boolean
  cached: boolean
  lastUsedAt?: string
}
export type SetupPluginCacheSummary = {
  bytes: number
  limit: number
  entries: number
  maxPerPlugin: number
}
export type PluginDownloadCacheSummary = {
  bytes: number
  limit: number
  entries: number
}

export const workspaceApi = createApi({
  reducerPath: 'workspaceApi',
  baseQuery: fetchBaseQuery({ baseUrl: '/api/v1/' }),
  keepUnusedDataFor: 60 * 60,
  tagTypes: [
    'RobotFile',
    'Catalog',
    'GitStatus',
    'NpmStatus',
    'PackageConfig',
    'LocalPackages',
    'PackageManifest',
    'Runtime',
    'OperationTasks',
    'SetupUpdate',
    'SetupPlugins',
    'EnvironmentReport',
    'SystemNetwork',
    'SystemRedis'
  ],
  endpoints: build => ({
    goals: build.query<unknown[], void>({ query: () => 'goals' }),
    environmentReport: build.query<
      Record<string, unknown>,
      { goalId: string; variant: string }
    >({
      query: body => ({ url: 'checks', method: 'POST', body }),
      providesTags: (_result, _error, arg) => [
        { type: 'EnvironmentReport', id: `${arg.goalId}:${arg.variant}` }
      ]
    }),
    releases: build.query<
      unknown[],
      string | { app: string; refresh?: boolean }
    >({
      query: input => {
        const { app, refresh } =
          typeof input === 'string' ? { app: input } : input
        const query = new URLSearchParams({ app })
        if (refresh) query.set('refresh', '1')
        return `releases?${query}`
      }
    }),
    setupUpdate: build.query<
      {
        current: string
        latest?: string
        available: boolean
        releaseUrl?: string
        downloadUrl?: string
        assetName?: string
        integrityError?: string
        platformMatched: boolean
        integrityReady: boolean
        downloadReady: boolean
      },
      { refresh?: boolean } | void
    >({
      query: input => (input?.refresh ? 'update?refresh=1' : 'update'),
      providesTags: ['SetupUpdate']
    }),
    setupPlugins: build.query<SetupPlugin[], void>({
      query: () => 'setup/plugins',
      providesTags: ['SetupPlugins']
    }),
    setupPluginReleases: build.query<SetupPluginRelease[], string>({
      query: pluginID =>
        `setup/plugins/releases/${encodeURIComponent(pluginID)}`
    }),
    setupPluginVersions: build.query<SetupPluginVersion[], string>({
      query: pluginID =>
        `setup/plugins/${encodeURIComponent(pluginID)}/versions`,
      providesTags: ['SetupPlugins']
    }),
    setupPluginCache: build.query<SetupPluginCacheSummary, void>({
      query: () => 'setup/plugins/cache',
      providesTags: ['SetupPlugins']
    }),
    pluginDownloadCache: build.query<PluginDownloadCacheSummary, void>({
      query: () => 'system/plugin-download-cache',
      providesTags: ['SetupPlugins']
    }),
    setSetupPluginEnabled: build.mutation<
      { id: string; enabled: boolean },
      { pluginID: string; enabled: boolean }
    >({
      query: ({ pluginID, ...body }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/enabled`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    installSetupPlugin: build.mutation<
      { id: string; downloaded: boolean; enabled: boolean },
      { pluginID: string; version: string; assetName: string }
    >({
      query: ({ pluginID, ...body }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/install`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    uninstallSetupPlugin: build.mutation<
      { id: string; uninstalled: boolean },
      { pluginID: string }
    >({
      query: ({ pluginID }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/uninstall`,
        method: 'POST',
        body: { confirm: true }
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    switchSetupPluginVersion: build.mutation<
      { id: string; tag?: string; switched: boolean },
      { pluginID: string; version: string; assetName: string }
    >({
      query: ({ pluginID, ...body }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/switch`,
        method: 'POST',
        body
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    deleteSetupPluginVersion: build.mutation<
      { id: string; tag: string; deleted: boolean },
      { pluginID: string; tag: string }
    >({
      query: ({ pluginID, tag }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/versions/${encodeURIComponent(tag)}`,
        method: 'DELETE'
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    cleanupSetupPluginCache: build.mutation<SetupPluginCacheSummary, void>({
      query: () => ({ url: 'setup/plugins/cache', method: 'POST' }),
      invalidatesTags: ['SetupPlugins']
    }),
    clearPluginDownloadCache: build.mutation<PluginDownloadCacheSummary, void>({
      query: () => ({ url: 'system/plugin-download-cache', method: 'DELETE' }),
      invalidatesTags: ['SetupPlugins']
    }),
    setupPluginDevelopment: build.query<
      { items: SetupPluginDevelopment[] },
      void
    >({
      query: () => 'setup/plugins/development',
      providesTags: ['SetupPlugins']
    }),
    registerSetupPluginDevelopment: build.mutation<
      SetupPluginDevelopment,
      { path: string }
    >({
      query: body => ({
        url: 'setup/plugins/development',
        method: 'POST',
        body
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    runSetupPluginDevelopment: build.mutation<
      SetupPluginDevelopment,
      {
        pluginID: string
        action: 'build' | 'start' | 'stop' | 'restart'
        confirm?: boolean
      }
    >({
      query: ({ pluginID, action, confirm = false }) => ({
        url: `setup/plugins/development/${encodeURIComponent(pluginID)}/${action}`,
        method: 'POST',
        body: { confirm }
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    removeSetupPluginDevelopment: build.mutation<
      { id: string; removed: boolean },
      string
    >({
      query: pluginID => ({
        url: `setup/plugins/development/${encodeURIComponent(pluginID)}/remove`,
        method: 'DELETE'
      }),
      invalidatesTags: ['SetupPlugins']
    }),
    setupPluginDevelopmentLogs: build.query<{ output: string }, string>({
      query: pluginID =>
        `setup/plugins/development/${encodeURIComponent(pluginID)}/logs`
    }),
    systemMcp: build.query<{ running: boolean }, void>({
      query: () => 'system/mcp'
    }),
    systemNetwork: build.query<SystemNetworkSettings, void>({
      query: () => 'system/network',
      providesTags: ['SystemNetwork']
    }),
    systemRedis: build.query<SystemRedisStatus, void>({
      query: () => 'system/redis',
      providesTags: ['SystemRedis']
    }),
    controlSystemRedis: build.mutation<
      SystemRedisStatus,
      'start' | 'stop' | 'restart'
    >({
      query: action => ({
        url: 'system/redis',
        method: 'POST',
        body: { action }
      }),
      invalidatesTags: ['SystemRedis']
    }),
    saveSystemRedisConfig: build.mutation<
      SystemRedisStatus,
      { port: number; autoStart: boolean; disabled: boolean }
    >({
      query: body => ({ url: 'system/redis', method: 'PUT', body }),
      invalidatesTags: ['SystemRedis']
    }),
    saveSystemNetwork: build.mutation<
      SystemNetworkSettings,
      Pick<SystemNetworkSettings, 'routes'>
    >({
      query: body => ({ url: 'system/network', method: 'PUT', body }),
      invalidatesTags: ['SystemNetwork']
    }),
    testSystemNetwork: build.mutation<SystemNetworkCheck, SystemNetworkRoute>({
      query: target => ({
        url: `system/network?${new URLSearchParams({ target })}`,
        method: 'POST'
      })
    }),
    setSystemCurrentRobot: build.mutation<SystemCurrentRobot, { root: string }>(
      {
        query: body => ({ url: 'system/context/robot', method: 'POST', body })
      }
    ),
    startSetupPluginTask: build.mutation<
      RobotTask,
      {
        pluginID: string
        action: string
        confirm: boolean
        params?: Record<string, string>
      }
    >({
      query: ({ pluginID, ...body }) => ({
        url: `setup/plugins/${encodeURIComponent(pluginID)}/actions`,
        method: 'POST',
        body
      })
    }),
    catalog: build.query<CatalogGroup[], 'apps' | 'environment'>({
      query: kind => `catalog?kind=${kind}`,
      providesTags: (_result, _error, kind) => [{ type: 'Catalog', id: kind }]
    }),
    catalogVersions: build.query<CatalogVersions, string>({
      query: packageName =>
        `catalog/versions?${new URLSearchParams({ package: packageName })}`,
      keepUnusedDataFor: 5 * 60
    }),
    catalogDocument: build.query<CatalogDocument, string>({
      query: url => `catalog/document?${new URLSearchParams({ url })}`
    }),
    catalogPackageConfig: build.query<PackageConfig, string>({
      query: url => `catalog/package-config?${new URLSearchParams({ url })}`
    }),
    packageConfig: build.query<
      PackageConfig,
      { root: string; package: string }
    >({
      query: ({ root, package: packageName }) =>
        `robot/package-config?${new URLSearchParams({ root, package: packageName })}`,
      providesTags: (_result, _error, arg) => [
        { type: 'PackageConfig', id: `${arg.root}:${arg.package}` }
      ]
    }),
    localPackages: build.query<LocalPackages, string>({
      query: root => `robot/packages?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [
        { type: 'LocalPackages', id: root }
      ]
    }),
    localPackageVersions: build.query<
      LocalPackageVersions,
      { root: string; package: string }
    >({
      query: ({ root, package: packageName }) =>
        `robot/package-versions?${new URLSearchParams({ root, package: packageName })}`
    }),
    localPackageReadme: build.query<
      RobotResult,
      { root: string; package: string }
    >({
      query: ({ root, package: packageName }) =>
        `robot/package-readme?${new URLSearchParams({ root, package: packageName })}`
    }),
    packageManifest: build.query<PackageManifest, string>({
      query: root => `robot/manifest?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [
        { type: 'PackageManifest', id: root }
      ]
    }),
    robotTasks: build.query<RobotTask[], void>({
      query: () => 'robot/tasks',
      providesTags: ['OperationTasks']
    }),
    robotTask: build.query<RobotTask, string>({
      query: id => `robot/tasks?${new URLSearchParams({ id })}`
    }),
    robotConsole: build.query<
      ConsolePayload,
      { root: string; refresh?: boolean }
    >({
      query: ({ root, refresh }) =>
        `robot/console?${new URLSearchParams(refresh ? { root, refresh: '1' } : { root })}`
    }),
    robotRuntime: build.query<RuntimeOverview, string>({
      query: root => `robot/runtime?${new URLSearchParams({ root })}`,
      keepUnusedDataFor: 20,
      providesTags: (_result, _error, root) => [{ type: 'Runtime', id: root }]
    }),
    robotPM2Status: build.query<PM2Status, string>({
      query: root => `robot/pm2-status?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [{ type: 'Runtime', id: root }]
    }),
    robotPM2Processes: build.query<{ items: PM2Process[] }, string>({
      query: root => `robot/pm2-processes?${new URLSearchParams({ root })}`
    }),
    appPort: build.query<{ port: number; configured: boolean }, string>({
      query: root => `robot/app-port?${new URLSearchParams({ root })}`
    }),
    robotApps: build.query<{ items: string[] }, string>({
      query: root => `robot/apps?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [
        { type: 'RobotFile', id: `${root}:alemon.config.yaml` }
      ]
    }),
    robotWebViews: build.query<WebViewEntry[], string>({
      query: root => `robot/webviews?${new URLSearchParams({ root })}`
    }),
    robotAppPortProbe: build.query<
      { reachable: boolean; port: number },
      string
    >({
      query: root =>
        `robot/app-port?${new URLSearchParams({ root, probe: '1' })}`
    }),
    testPort: build.query<
      { port: number; configured: boolean; sandbox?: boolean },
      string
    >({
      query: root => `robot/test-port?${new URLSearchParams({ root })}`
    }),
    robotTestPortProbe: build.query<
      { reachable: boolean; port: number; sandbox?: boolean },
      string
    >({
      query: root =>
        `robot/test-port?${new URLSearchParams({ root, probe: '1' })}`
    }),
    robotPorts: build.query<{ items: RobotPortStatus[] }, string>({
      query: root => `robot/ports?${new URLSearchParams({ root })}`
    }),
    setAppEnabled: build.mutation<
      RobotResult,
      { root: string; package: string; enabled: boolean }
    >({
      query: ({ root, package: packageName, enabled }) => ({
        url: `robot/apps?${new URLSearchParams({ root })}`,
        method: 'POST',
        body: { package: packageName, enabled }
      }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'RobotFile', id: `${body.root}:alemon.config.yaml` }
      ]
    }),
    saveAppPort: build.mutation<RobotResult, { root: string; port: number }>({
      query: ({ root, port }) => ({
        url: `robot/app-port?${new URLSearchParams({ root })}`,
        method: 'POST',
        body: { port }
      }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'RobotFile', id: `${body.root}:alemon.config.yaml` },
        { type: 'Runtime', id: body.root }
      ]
    }),
    saveTestPort: build.mutation<RobotResult, { root: string; port: number }>({
      query: ({ root, port }) => ({
        url: `robot/test-port?${new URLSearchParams({ root })}`,
        method: 'POST',
        body: { port }
      }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'RobotFile', id: `${body.root}:alemon.config.yaml` },
        { type: 'Runtime', id: body.root }
      ]
    }),
    robotRuntimePreflight: build.query<RuntimePreflight, string>({
      query: root => `robot/runtime/preflight?${new URLSearchParams({ root })}`
    }),
    robotRuntimeRepair: build.query<
      RuntimeRepairPlan,
      { root: string; mode: string }
    >({
      query: ({ root, mode }) =>
        `robot/runtime/repair?${new URLSearchParams({ root, mode })}`
    }),
    applyRuntimeRepair: build.mutation<
      RuntimeRepairResult,
      { root: string; mode: string; confirmOverrides: boolean }
    >({
      query: body => ({ url: 'robot/runtime/repair', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'Runtime', id: body.root }
      ]
    }),
    robotProject: build.query<
      { valid: boolean; path?: string; error?: string },
      string
    >({
      query: root => `robot/validate?${new URLSearchParams({ root })}`,
      keepUnusedDataFor: 60
    }),
    robotFile: build.query<RobotResult, { root: string; file: string }>({
      query: ({ root, file }) => `robot?${new URLSearchParams({ root, file })}`,
      providesTags: (_result, _error, arg) => [
        { type: 'RobotFile', id: `${arg.root}:${arg.file}` }
      ]
    }),
    gitStatus: build.query<Record<string, unknown>, string>({
      query: root => `publish/git/status?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [{ type: 'GitStatus', id: root }]
    }),
    gitWorkspace: build.query<
      GitWorkspace,
      { root: string; view: 'commit' | 'history' | 'tag' | 'branch' | 'remote' }
    >({
      query: ({ root, view }) =>
        `robot/git?${new URLSearchParams({ root, view })}`,
      providesTags: (_result, _error, arg) => [
        { type: 'GitStatus', id: arg.root }
      ]
    }),
    npmStatus: build.query<Record<string, unknown>, string>({
      query: root => `publish/npm/status?${new URLSearchParams({ root })}`,
      providesTags: (_result, _error, root) => [{ type: 'NpmStatus', id: root }]
    }),
    npmPack: build.query<
      Record<string, unknown>,
      { root: string; commit?: string }
    >({
      query: ({ root, commit }) =>
        `publish/npm/pack?${new URLSearchParams(commit ? { root, commit } : { root })}`
    }),
    robotOperation: build.mutation<RobotResult, Record<string, string>>({
      query: body => ({ url: 'robot', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'GitStatus', id: body.root },
        { type: 'NpmStatus', id: body.root },
        { type: 'LocalPackages', id: body.root }
      ]
    }),
    gitWorkspaceAction: build.mutation<
      RobotResult,
      {
        root: string
        action:
          | 'fetch'
          | 'pull'
          | 'push'
          | 'commit'
          | 'branch-create'
          | 'branch-switch'
          | 'branch-track'
          | 'branch-delete'
          | 'tag-create'
          | 'tag-push'
          | 'tag-delete'
          | 'remote-add'
          | 'remote-set-url'
          | 'remote-remove'
        value?: string
        message?: string
      }
    >({
      query: body => ({ url: 'robot/git', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'GitStatus', id: body.root }
      ]
    }),
    startRobotTask: build.mutation<RobotTask, Record<string, string>>({
      query: body => ({ url: 'robot/tasks', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'GitStatus', id: body.root },
        { type: 'NpmStatus', id: body.root },
        { type: 'Runtime', id: body.root },
        'OperationTasks'
      ]
    }),
    writeRobotFile: build.mutation<
      RobotResult,
      { root: string; file: string; content: string }
    >({
      query: body => ({ url: 'robot', method: 'PUT', body }),
      invalidatesTags: (_result, _error, body) => {
        const tags: (
          | { type: 'RobotFile'; id: string }
          | { type: 'PackageConfig'; id?: string }
        )[] = [{ type: 'RobotFile', id: `${body.root}:${body.file}` }]
        // Saving alemon.config.yaml changes every structured config view
        // (current project config, connection package config, start dialog),
        // so drop all cached PackageConfig entries along with the file tag.
        if (body.file === 'alemon.config.yaml')
          tags.push({ type: 'PackageConfig' })
        return tags
      }
    }),
    writePackageManifest: build.mutation<
      RobotResult,
      { root: string } & PackageManifest
    >({
      query: body => ({ url: 'robot/manifest', method: 'PUT', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'PackageManifest', id: body.root },
        { type: 'GitStatus', id: body.root },
        { type: 'NpmStatus', id: body.root }
      ]
    }),
    writePackageConfig: build.mutation<
      RobotResult,
      { root: string; package: string; values: Record<string, unknown> }
    >({
      query: body => ({ url: 'robot/package-config', method: 'PUT', body }),
      invalidatesTags: (_result, _error, body) => [
        // The whole file changed, so every structured view (current project,
        // connection package, start dialog) must re-parse it.
        { type: 'PackageConfig' },
        { type: 'RobotFile', id: `${body.root}:alemon.config.yaml` }
      ]
    }),
    saveRobotLogin: build.mutation<
      RobotResult,
      { root: string; login: string; package?: string }
    >({
      query: body => ({ url: 'robot/login', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'RobotFile', id: `${body.root}:alemon.config.yaml` },
        { type: 'PackageConfig' }
      ]
    }),
    initializeGit: build.mutation<
      RobotResult,
      {
        root: string
        authorName: string
        authorEmail: string
        repository: string
        message: string
      }
    >({
      query: body => ({ url: 'robot/git-init', method: 'POST', body }),
      invalidatesTags: (_result, _error, body) => [
        { type: 'GitStatus', id: body.root }
      ]
    })
  })
})

export const {
  useGoalsQuery,
  useLazyEnvironmentReportQuery,
  useReleasesQuery,
  useLazySetupUpdateQuery,
  useSetupPluginsQuery,
  useSetupPluginReleasesQuery,
  useLazySetupPluginReleasesQuery,
  useSetupPluginVersionsQuery,
  useLazySetupPluginVersionsQuery,
  useSetupPluginCacheQuery,
  usePluginDownloadCacheQuery,
  useSetSetupPluginEnabledMutation,
  useInstallSetupPluginMutation,
  useUninstallSetupPluginMutation,
  useSwitchSetupPluginVersionMutation,
  useDeleteSetupPluginVersionMutation,
  useCleanupSetupPluginCacheMutation,
  useClearPluginDownloadCacheMutation,
  useSetupPluginDevelopmentQuery,
  useRegisterSetupPluginDevelopmentMutation,
  useRunSetupPluginDevelopmentMutation,
  useRemoveSetupPluginDevelopmentMutation,
  useLazySetupPluginDevelopmentLogsQuery,
  useSystemMcpQuery,
  useSystemNetworkQuery,
  useSystemRedisQuery,
  useSaveSystemNetworkMutation,
  useTestSystemNetworkMutation,
  useControlSystemRedisMutation,
  useSaveSystemRedisConfigMutation,
  useSetSystemCurrentRobotMutation,
  useStartSetupPluginTaskMutation,
  useCatalogQuery,
  useCatalogVersionsQuery,
  useCatalogDocumentQuery,
  useCatalogPackageConfigQuery,
  usePackageConfigQuery,
  useLazyPackageConfigQuery,
  useLocalPackagesQuery,
  useLocalPackageVersionsQuery,
  useLocalPackageReadmeQuery,
  usePackageManifestQuery,
  useRobotTasksQuery,
  useLazyRobotTaskQuery,
  useLazyRobotConsoleQuery,
  useRobotRuntimeQuery,
  useRobotPM2StatusQuery,
  useRobotPM2ProcessesQuery,
  useAppPortQuery,
  useLazyAppPortQuery,
  useSaveAppPortMutation,
  useRobotAppsQuery,
  useRobotWebViewsQuery,
  useRobotAppPortProbeQuery,
  useLazyRobotPortsQuery,
  useTestPortQuery,
  useLazyTestPortQuery,
  useRobotTestPortProbeQuery,
  useSaveTestPortMutation,
  useSetAppEnabledMutation,
  useLazyRobotRuntimePreflightQuery,
  useLazyRobotRuntimeRepairQuery,
  useApplyRuntimeRepairMutation,
  useLazyRobotProjectQuery,
  useLazyRobotFileQuery,
  useGitStatusQuery,
  useGitWorkspaceQuery,
  useGitWorkspaceActionMutation,
  useNpmStatusQuery,
  useLazyNpmPackQuery,
  useRobotOperationMutation,
  useStartRobotTaskMutation,
  useWriteRobotFileMutation,
  useWritePackageManifestMutation,
  useWritePackageConfigMutation,
  useSaveRobotLoginMutation,
  useInitializeGitMutation
} = workspaceApi
