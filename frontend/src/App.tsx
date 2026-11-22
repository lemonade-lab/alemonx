import { RefreshButton } from './components/RefreshButton'
import type { RobotList } from './lib/workspaceRobots'
import { openNativeWindow } from './components/nativeWindow'
import { OfficialExtensionChoices } from './components/OfficialResources'
import { useResourceOptionsQuery } from './store/workspaceApi'
import { useStoreState } from './store/guideStore'
import {
  lazy,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState
} from 'react'
import { useDispatch, useSelector } from 'react-redux'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  store,
  setDeveloper,
  setProject,
  toggleCapability as toggleCapabilityAction,
  type RootState
} from './store/guideStore'
import { Dashboard } from './components/Dashboard'
import { DeferredPanel } from './components/DeferredPanel'
import type { SettingsSection } from './components/settingsSections'
import { navigationKey, setNavigationValue } from './store/navigationStore'
import { EnvironmentFixDialog } from './components/EnvironmentFixDialog'
import { DownloadProgress } from './components/DownloadProgress'
import { ErrorNotice } from './components/ErrorNotice'
import {
  useGoalsQuery,
  useWorkspaceQuery,
  useLazyEnvironmentReportQuery,
  useReleasesQuery
} from './store/workspaceApi'
import { GuideFlowNavigation, GuideHome, GuideProgress } from './features/guide/GuideHome'
import { EnvironmentCheckPanel } from './features/guide/EnvironmentCheckPanel'
import { guideIcons as icons } from './features/guide/icons'
import { recommendReleaseAssets } from './features/guide/releaseAssets'
import { Settings } from 'lucide-react'
import type {
  Check,
  Creation,
  Goal,
  Mirror,
  ProjectConfig,
  Release,
  Report
} from './features/guide/types'

const AppSettingsPanel = lazy(() =>
  import('./components/AppSettingsPanel').then(module => ({
    default: module.AppSettingsPanel
  }))
)

const capabilityLabels: Record<string, string> = {
  bubble: '气泡服务',
  database: '数据存储',
  discord: 'Discord',
  onebot: 'OneBot',
  qqbot: 'QQ Bot'
}

export default function App({ initialRobots }: { initialRobots?: Promise<RobotList> }) {
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useStoreState('')
  const [creating, setCreating] = useStoreState(false)
  const [creation, setCreation] = useStoreState<Creation | null>(null)
  const [repairCheck, setRepairCheck] = useStoreState<Check | null>(null)
  const [
    loadEnvironmentReport,
    {
      currentData: environmentData,
      isFetching: checking,
      isError: environmentCheckFailed,
      originalArgs: checkedEnvironment
    }
  ] = useLazyEnvironmentReportQuery()
  const settingsTitle = '设置'
  const routeGoal = location.pathname.match(
    /^\/guide\/([^/]+)\/step\/\d+$/
  )?.[1]
  const guideGroup = location.pathname.match(/^\/guide\/group\/([^/]+)$/)?.[1]
  const selectedID = routeGoal ?? null
  const guideOpen = !location.pathname.startsWith('/dashboard')
  const { data: goalData, isLoading: loading } = useGoalsQuery(undefined, { skip: !guideOpen })
  const goals = (goalData as Goal[] | null | undefined) ?? []
  const guideHasBack = Boolean(guideGroup || (routeGoal && !location.pathname.endsWith('/step/0')))
  const guideBack = useRef<() => void>(() => {})
  const dashboardReturn = useRef('/dashboard/robot')
  const activeID = selectedID ?? 'install'
  const report =
    checkedEnvironment?.goalId === activeID
      ? ((environmentData as Report | undefined) ?? null)
      : null
  const activeGoal = goals.find(goal => goal.id === activeID)
  const [settingsMounted, setSettingsMounted] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsMinimized, setSettingsMinimized] = useState(false)
  const [settingsLayer, setSettingsLayer] = useState(108)
  const nextWindowLayer = useRef(108)

  function openSettings(section?: SettingsSection) {
    if (openNativeWindow('settings', section)) return
    if (section) {
      store.dispatch(
        setNavigationValue({
          key: navigationKey('settings.section', []),
          value: section
        })
      )
    }
    const layer = ++nextWindowLayer.current
    setSettingsLayer(layer)
    setSettingsMounted(true)
    setSettingsOpen(true)
    setSettingsMinimized(false)
    window.dispatchEvent(
      new CustomEvent('alx:desktop-window-layer', { detail: layer })
    )
  }

  useEffect(() => {
    if (location.pathname === '/') navigate('/guide', { replace: true })
  }, [location.pathname, navigate])
  useEffect(() => {
    const syncWindowLayer = (event: Event) => {
      const layer = (event as CustomEvent<number>).detail
      if (typeof layer === 'number')
        nextWindowLayer.current = Math.max(nextWindowLayer.current, layer)
    }
    window.addEventListener('alx:desktop-window-layer', syncWindowLayer)
    return () =>
      window.removeEventListener('alx:desktop-window-layer', syncWindowLayer)
  }, [])
  function openGuide(path = '/guide') {
    if (!guideOpen) {
      dashboardReturn.current = location.pathname + location.search + location.hash
    }
    navigate(path)
    setCreation(null)
    setError('')
  }
  function selectGuideGoal(id: string | null) {
    if (id === 'manage') {
      navigate('/dashboard/robot')
      return
    }
    openGuide(id ? `/guide/${id}/step/1` : '/guide')
  }
  async function checkEnvironment(variant?: string) {
    const selectedVariant = typeof variant === 'string' ? variant : ''
    try {
      setError('')
      await loadEnvironmentReport(
        { goalId: activeID, variant: selectedVariant },
        false
      ).unwrap()
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : '环境检查未完成，请稍后重试。'
      )
    }
  }
  async function createProject(config: ProjectConfig) {
    try {
      setCreating(true)
      setError('')
      setCreation(null)
      const response = await fetch('/api/v1/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
      })
      const data = (await response.json()) as Creation & {
        error?: string
        result?: Creation
      }
      setCreation(data.result ?? data)
      if (!response.ok)
        setError(data.error ?? '项目创建未完成，请检查下方日志。')
    } catch {
      setError('项目创建请求未完成，请稍后重试。')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="app-shell">
      <div className="workbench-stage">
        <Dashboard
          initialRobots={initialRobots}
          report={report}
          checking={checking}
          error={error}
          onClearError={() => setError('')}
          defaultPage={
            location.pathname.endsWith('/robot') ? 'robot' : 'environment'
          }
          guide={guideOpen ? {
            onClose: () => navigate(dashboardReturn.current),
            renderContent: (stepsTarget, progressTarget, onStepSelect) => {
              const onBack = guideHasBack ? () => {
                onStepSelect()
                if (guideGroup) navigate('/guide')
                else guideBack.current()
              } : undefined
              return (
                <GuideHome
                  group={guideGroup}
                  stepsTarget={stepsTarget}
                  progressTarget={progressTarget}
                  onBack={onBack}
                  onStepSelect={onStepSelect}
                  error={error}
                  onClearError={() => setError('')}
                  onSelect={selectGuideGoal}
                >
                  <FlowView
                    loading={loading}
                    stepsTarget={stepsTarget}
                    progressTarget={progressTarget}
                    onBack={onBack}
                    onStepSelect={onStepSelect}
                    goal={selectedID ? activeGoal : undefined}
                    report={report}
                    reportVariant={
                      checkedEnvironment?.goalId === activeID
                        ? checkedEnvironment.variant
                        : undefined
                    }
                    checkFailed={
                      environmentCheckFailed && checkedEnvironment?.goalId === activeID
                    }
                    checking={checking}
                    creating={creating}
                    creation={creation}
                    onSelect={selectGuideGoal}
                    onCheck={checkEnvironment}
                    onCreate={createProject}
                    onFix={setRepairCheck}
                    registerBack={handler => { guideBack.current = handler }}
                  />
                </GuideHome>
              )
            }
          } : undefined}
          onSelect={selectGuideGoal}
          onCreateRobot={() => openGuide('/guide/install/step/1')}
          onOpenGuide={() => openGuide()}
          onOpenSettings={openSettings}
          onMinimizeSettings={() => {
            if (settingsOpen) setSettingsMinimized(true)
          }}
          onCheck={checkEnvironment}
          onFix={setRepairCheck}
        />
      </div>
      {settingsMounted && (
        <DeferredPanel label="设置">
          <AppSettingsPanel
            id="app-settings"
            open={settingsOpen}
            minimized={settingsMinimized}
            title={settingsTitle}
            icon={
              <Settings className="size-4 text-brand-600 dark:text-brand-200" />
            }
            onClose={() => {
              setSettingsOpen(false)
              setSettingsMinimized(false)
            }}
            onMinimize={() => setSettingsMinimized(true)}
            zIndex={settingsLayer}
            onActivate={() => openSettings()}
            initialPosition={{ left: 180, top: 120 }}
            width={820}
            height={640}
          />
        </DeferredPanel>
      )}
      {repairCheck && (
        <EnvironmentFixDialog
          check={repairCheck}
          platform={report?.platform}
          onInstalled={() =>
            void checkEnvironment(
              checkedEnvironment?.goalId === activeID
                ? checkedEnvironment.variant
                : undefined
            )
          }
          onClose={() => setRepairCheck(null)}
        />
      )}
    </div>
  )
}

function FlowView({
  loading,
  goal,
  report,
  reportVariant,
  checkFailed,
  checking,
  creating,
  creation,
  onSelect,
  onCheck,
  onCreate,
  onFix,
  registerBack,
  stepsTarget,
  progressTarget,
  onBack,
  onStepSelect
}: {
  loading: boolean
  goal?: Goal
  report: Report | null
  reportVariant?: string
  checkFailed: boolean
  checking: boolean
  creating: boolean
  creation: Creation | null
  onSelect: (id: string | null) => void
  onCheck: (variant?: string) => void
  onCreate: (config: ProjectConfig) => void
  onFix: (check: Check) => void
  registerBack: (handler: () => void) => void
  stepsTarget: HTMLDivElement | null
  progressTarget: HTMLDivElement | null
  onBack?: () => void
  onStepSelect: () => void
}) {
  const navigate = useNavigate()
  const location = useLocation()
  const dispatch = useDispatch()
  const { data: workspace } = useWorkspaceQuery()
  // A previously chosen "custom folder" is persisted by redux-persist and
  // would otherwise silently divert every new robot away from the workspace
  // bots directory. Reset to the workspace option each time the wizard opens.
  useEffect(() => {
    dispatch(setProject({ destinationMode: 'current', destination: '' }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const config = useSelector((state: RootState) => state.guide.developer)
  const project = useSelector((state: RootState) => state.guide.project)
  const routedStep = Number(location.pathname.match(/\/step\/(\d+)/)?.[1] ?? 0)
  const [step, setStep] = useStoreState(routedStep)
  const stepViewport = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (stepViewport.current) stepViewport.current.scrollTop = 0
  }, [goal?.id, step])
  const [webEdition, setWebEdition] = useStoreState<'clean' | 'docker' | null>(
    null
  )
  const [projectRuntime, setProjectRuntime] = useStoreState<'node' | 'bun'>(
    'node'
  )
  const [buildMode, setBuildMode] = useStoreState<'npm' | 'git' | null>(null)
  const [selectedMirror, setSelectedMirror] = useStoreState<Mirror | null>(null)
  const [releaseURL, setReleaseURL] = useStoreState<string | null>(null)
  const [selectedAssetURL, setSelectedAssetURL] = useStoreState<string | null>(
    null
  )
  const [browserDownloadNotice, setBrowserDownloadNotice] = useStoreState('')
  const [folderError, setFolderError] = useStoreState('')
  const automaticCheck = useRef<string | null>(null)
  const capabilities = config.capabilities ?? []
  const { data: officialExtensions } = useResourceOptionsQuery(
    'connector,js-plugin',
    { skip: goal?.id !== 'develop' && goal?.id !== 'install' }
  )
  const isDeveloper = goal?.id === 'develop'
  const isInstaller = goal?.id === 'install'
  const useBun = projectRuntime === 'bun'
  const releaseApp = goal?.id === 'web' && webEdition === 'clean' ? 'alx' : null
  const {
    data: releaseData,
    isError: releaseError,
    isFetching: releaseFetching,
    refetch: refetchReleases
  } = useReleasesQuery(releaseApp ?? '', {
    skip: !releaseApp
  })
  const releases = useMemo(
    () => (releaseData as Release[] | null | undefined) ?? [],
    [releaseData]
  )
  const webSteps =
    webEdition === 'clean'
      ? [
          '选择部署方式',
          '检查 Node.js 与 Git',
          '选择下载镜像',
          '选择版本',
          '选择安装包'
        ]
      : webEdition === 'docker'
        ? ['选择部署方式', '检查 Docker', 'Docker Compose 快速启动']
        : ['选择部署方式']
  const buildSteps =
    buildMode === 'npm'
      ? ['选择构建方式', '检查 npm 环境', 'NPM 构建']
      : buildMode === 'git'
        ? ['选择构建方式', '检查 Git 环境', 'Git 化构建']
        : ['选择构建方式']
  const downloadSteps =
    goal?.id === 'mobile'
      ? ['下载 Android 安装包']
      : selectedMirror
        ? ['选择镜像', '选择版本', '选择安装包']
        : ['选择镜像']
  const totalSteps = goal
    ? [
        '选择目的',
        ...(goal.id === 'web'
          ? webSteps
          : goal.id === 'build'
            ? buildSteps
            : goal.id === 'mobile'
              ? downloadSteps
              : goal.steps)
      ]
    : ['选择目的']
  const flowStep = step - 1
  const environmentVariant =
    goal?.id === 'web'
      ? webEdition
      : goal?.id === 'build'
        ? buildMode
        : useBun
          ? 'bun'
          : ''
  const isEnvironmentStep =
    ((isInstaller || isDeveloper) && flowStep === 0) ||
    ((goal?.id === 'web' || goal?.id === 'build') && flowStep === 1)
  const environmentReport =
    reportVariant === environmentVariant ? report : null
  const failedEnvironment = reportVariant === environmentVariant && checkFailed

  const setFlowStep = (value: number) => {
    const nextStep = Math.max(0, Math.min(value, totalSteps.length - 1))
    setStep(nextStep)
    if (goal) navigate(`/guide/${goal.id}/step/${nextStep}`)
  }
  const next = () => setFlowStep(step + 1)
  const back = () => setFlowStep(step - 1)
  const choose = (key: keyof typeof config, value: string) =>
    dispatch(setDeveloper({ [key]: value }))
  const toggleCapability = (value: string) =>
    dispatch(toggleCapabilityAction(value))
  const chooseDestination = () => {
    setFolderError('')
    window.dispatchEvent(new Event('alemon:choose-directory'))
  }
  useEffect(() => {
    if (routedStep !== step) setStep(routedStep)
  }, [routedStep, step, setStep])
  useEffect(() => {
    const key = `${goal?.id}:${environmentVariant ?? ''}`
    if (
      isEnvironmentStep &&
      environmentVariant !== null &&
      automaticCheck.current !== key
    ) {
      automaticCheck.current = key
      onCheck(environmentVariant)
    }
  }, [environmentVariant, goal?.id, isEnvironmentStep, onCheck])
  useEffect(() => {
    if (!releaseApp) return
    setReleaseURL(releases[0]?.url ?? null)
    setSelectedAssetURL(null)
  }, [releaseApp, releases, setReleaseURL, setSelectedAssetURL])
  const mirrorURL = (mirror: Mirror | null, url: string) => {
    if (!mirror) return url
    const index = mirror.url.indexOf('https://github.com')
    return index === -1 ? url : mirror.url.slice(0, index) + url
  }
  const releasePicker = () => (
    <div className="release-picker">
      <label className="release-picker-label">
        <span>
          选择版本
          {!releases.length && !releaseError && releaseFetching && (
            <small>读取中…</small>
          )}
          {!releases.length && releaseError && (
            <small
              className="text-(--theme-danger-text)"
              title="正式版本列表获取失败，可能是网络或代理问题。"
            >
              读取失败
              <RefreshButton label="重试" onClick={() => void refetchReleases()} />
            </small>
          )}
        </span>
        <select
          value={releaseURL ?? ''}
          onChange={event => {
            setReleaseURL(event.target.value)
            setSelectedAssetURL(null)
          }}
        >
          {releases.map(item => (
            <option value={item.url} key={item.tag}>
              {item.tag} · {item.name}
            </option>
          ))}
        </select>
      </label>
    </div>
  )
  const selectedRelease = releases.find(item => item.url === releaseURL)
  const releaseAssets = recommendReleaseAssets(
    selectedRelease?.assets ?? [],
    navigator.userAgent.toLowerCase()
  )
  const assetPicker = () => (
    <>
      <h1>选择安装包</h1>
      <div className="grid gap-2.5 my-5 asset-list">
        {releaseAssets.assets.map(asset => (
          <button
            className={
              selectedAssetURL === asset.url ? 'choice selected' : 'choice'
            }
            key={asset.url}
            onClick={() => setSelectedAssetURL(asset.url)}
          >
            <strong>
              {asset.name}
              {releaseAssets.isRecommended(asset) && <em>推荐</em>}
            </strong>
            <small>
              {asset.size
                ? `${(asset.size / 1024 / 1024).toFixed(1)} MB`
                : 'GitHub 安装包'}
            </small>
          </button>
        ))}
      </div>
      {selectedRelease && releaseAssets.assets.length === 0 && (
        <p>该版本没有可直接下载的安装包，请返回选择其他版本。</p>
      )}
      {browserDownloadNotice && (
        <DownloadProgress
          handoff
          label="已开始浏览器下载"
          detail={browserDownloadNotice}
        />
      )}
    </>
  )
  const environmentPage = (chooseRuntime = false) => (
    <EnvironmentCheckPanel
      report={environmentReport}
      checking={checking}
      failed={failedEnvironment}
      runtime={
        chooseRuntime
          ? {
              value: projectRuntime,
              onChange: value => {
                automaticCheck.current = null
                setProjectRuntime(value)
              }
            }
          : undefined
      }
      onCheck={() => onCheck(environmentVariant ?? undefined)}
      onFix={onFix}
    />
  )
  const developerPage = () => {
    const choices = (
      title: string,
      items: Array<[string, string, string]>,
      key: keyof typeof config
    ) => (
      <>
        <h1>{title}</h1>
        <div className="grid gap-2.5 my-5">
          {items.map(([value, label, note]) => (
            <button
              className={
                String(config[key]) === value ? 'choice selected' : 'choice'
              }
              key={value}
              onClick={() => choose(key, value)}
            >
              <strong>{label}</strong>
              <small>{note}</small>
            </button>
          ))}
        </div>
      </>
    )
    switch (flowStep) {
      case 0:
        return environmentPage(true)
      case 1:
        return (
          <>
            <h1>给项目起个名字</h1>
            <p>会在你选择的保存位置中新建一个同名文件夹。</p>
            <div className="project-fields">
              <label>
                项目名称
                <input
                  value={project.name}
                  onChange={event =>
                    dispatch(setProject({ name: event.target.value }))
                  }
                  placeholder="my-alemonjs-bot"
                />
              </label>
              <div className="location-options">
                <button
                  className={
                    project.destinationMode === 'current'
                      ? 'choice selected'
                      : 'choice'
                  }
                  onClick={() =>
                    dispatch(setProject({ destinationMode: 'current' }))
                  }
                >
                  <strong>工作区（推荐）</strong>
                  <small>
                    {workspace
                      ? `保存到 ${workspace.bots}`
                      : '保存到工作区的 bots 目录。'}
                  </small>
                </button>
                <button
                  className={
                    project.destinationMode === 'custom'
                      ? 'choice selected'
                      : 'choice'
                  }
                  onClick={chooseDestination}
                >
                  <strong>选择指定文件夹</strong>
                  <small
                    className="truncate"
                    title={
                      project.destinationMode === 'custom' &&
                      project.destination
                        ? project.destination
                        : '在目录选择器中选择保存位置。'
                    }
                  >
                    {project.destinationMode === 'custom' && project.destination
                      ? `已选择：${project.destination}`
                      : '在目录选择器中选择保存位置。'}
                  </small>
                </button>
              </div>
              {folderError && (
                <ErrorNotice
                  message={folderError}
                  onClose={() => setFolderError('')}
                />
              )}
            </div>
          </>
        )
      case 2:
        return choices(
          '你想用哪种语言？',
          [
            ['js', 'JavaScript（推荐新手）', '写法更简单，先把机器人跑起来。'],
            ['ts', 'TypeScript', '会在写代码时提前提醒常见错误。']
          ],
          'language'
        )
      case 3:
        return choices(
          '需要代码小助手吗？',
          [
            ['yes', '需要', 'ESLint 像拼写检查，会提醒容易写错的地方。'],
            ['no', '暂时不要（默认）', '项目更简单，以后随时可以加。']
          ],
          'eslint'
        )
      case 4:
        return choices(
          '要给项目留存档吗？',
          [
            ['yes', '要（推荐）', 'Git 会记录每次修改，写错了也方便回退。'],
            ['no', '暂时不要', '不会创建版本记录。']
          ],
          'git'
        )
      case 5:
        return choices(
          '要让机器人在后台运行吗？',
          [
            [
              'yes',
              '要，使用 PM2（默认）',
              'PM2 是帮你守着机器人的小管家：关掉终端后，它仍会继续运行。'
            ],
            ['no', '暂时不要', '开发时在终端里直接运行，更容易看懂。']
          ],
          'pm2'
        )
      case 6:
        return useBun
          ? (
            <>
              <h1>用什么安装项目依赖？</h1>
              <p>Bun 负责运行项目和管理依赖。</p>
            </>
          )
          : choices(
              '用什么安装项目依赖？',
              [
                [
                  'yarn',
                  'Yarn（推荐）',
                  '没有 Yarn 时会临时使用，不会修改电脑的全局安装。'
                ],
                ['npm', 'npm', 'Node.js 自带，不需要额外安装。'],
                ['pnpm', 'pnpm', '更省磁盘空间，需要电脑已经安装。']
              ],
              'manager'
            )
      case 7:
        return (
          <>
            <h1>选择扩展包</h1>
            <div className="my-5 grid gap-3">
              <OfficialExtensionChoices
                label="项目扩展"
                selected={capabilities}
                onSelect={resource => toggleCapability(resource.id)}
              />
            </div>
          </>
        )
      case 8:
        return (
          <>
            <h1>需要做图片功能吗？</h1>
            <div className="grid gap-2.5 my-5">
              {[
                ['none', '不需要', '生成文字、按钮和普通消息模板。'],
                [
                  'html',
                  '纯 HTML',
                  '保留文本帮助模板，后续可按需加入 HTML 渲染方案。'
                ],
                [
                  'react',
                  'React / JSX',
                  '安装 jsxp，并生成可拆分组件的图片模板。'
                ]
              ].map(([value, label, note]) => (
                <button
                  className={
                    config.image === value ? 'choice selected' : 'choice'
                  }
                  key={value}
                  onClick={() => choose('image', value)}
                >
                  <strong>{label}</strong>
                  <small>{note}</small>
                </button>
              ))}
            </div>
          </>
        )
      case 9:
        return config.image === 'react' ? (
          choices(
            '图片用什么方式做样式？',
            [
              ['css', '原生 CSS（推荐）', '最容易理解，不需要再学额外工具。'],
              ['tailwind', 'Tailwind CSS', '安装 Tailwind 与 CSS 压缩工具。'],
              ['sass', 'Sass / SCSS', '安装 Sass。'],
              ['less', 'Less', '安装 Less。']
            ],
            'style'
          )
        ) : (
          <>
            <h1>不需要样式工具</h1>
            <p>未选择 React / JSX 图片模板，不会安装额外样式依赖。</p>
          </>
        )
      case 10:
        return (
          <>
            <h1>下载开发技能吗？</h1>
            <p>
              开发技能像一本 AlemonJS 的使用说明。安装后，Codex
              等工具更容易按推荐方式帮你写代码。
            </p>
            <div className="grid gap-2.5 my-5">
              <button
                className={
                  config.skills === 'yes' ? 'choice selected' : 'choice'
                }
                onClick={() => choose('skills', 'yes')}
              >
                <strong>下载（推荐）</strong>
                <small>下载 alemonjs-dev-skill，后续可随时更新。</small>
              </button>
              <button
                className={
                  config.skills === 'no' ? 'choice selected' : 'choice'
                }
                onClick={() => choose('skills', 'no')}
              >
                <strong>暂时不下载</strong>
                <small>不会影响机器人运行，以后也可以安装。</small>
              </button>
            </div>
            <a
              className="download-link"
              href="https://github.com/lemonade-lab/alemonjs-dev-skill"
              target="_blank"
              rel="noreferrer"
            >
              查看开发技能说明
            </a>
          </>
        )
      case 11:
        return (
          <>
            <h1>
              {creation?.status === 'ready' ? '项目已创建' : '确认创建项目'}
            </h1>
            {creation?.status === 'ready' ? (
              <>
                <p>{creation.path}</p>
                {creation.path && (
                  <a
                    className="primary-button"
                    href={`/dashboard/robot?root=${encodeURIComponent(creation.path)}`}
                  >
                    前往管理机器人
                  </a>
                )}
              </>
            ) : (
              <div className="config-summary">
                <span>
                  位置：
                  {project.destinationMode === 'current'
                    ? workspace
                      ? `${workspace.bots}/${project.name}`
                      : `工作区/${project.name}`
                    : project.destination
                      ? `${project.destination}/${project.name}`
                      : '请返回填写保存位置'}
                </span>
                <span>
                  语言：{config.language === 'ts' ? 'TypeScript' : 'JavaScript'}
                </span>
                <span>包管理器：{useBun ? 'Bun' : config.manager === 'bun' ? 'Yarn' : config.manager}</span>
                <span>
                  开发能力包：
                  {capabilities.length
                    ? capabilities
                        .map(
                          item =>
                            officialExtensions?.data.find(
                              resource => resource.id === item
                            )?.name ??
                            capabilityLabels[item] ??
                            item
                        )
                        .join('、')
                    : '仅基础框架'}
                </span>
                <span>
                  图片模板：
                  {config.image === 'react'
                    ? `React / JSX · ${config.style}`
                    : config.image === 'html'
                      ? '纯 HTML'
                      : '不生成'}
                </span>
                <span>
                  代码小助手：{config.eslint === 'yes' ? '启用' : '不启用'}
                </span>
                <span>
                  项目存档：{config.git === 'yes' ? '初始化 Git' : '跳过'}
                </span>
                <span>
                  后台运行：{config.pm2 === 'yes' ? '使用 PM2' : '不使用'}
                </span>
                <span>
                  开发技能：{config.skills === 'yes' ? '下载' : '不下载'}
                </span>
              </div>
            )}
          </>
        )
      default:
        return null
    }
  }
  const installerPage = () => {
    if (flowStep === 0) return environmentPage(true)
    if (flowStep === 1)
      return (
        <>
          <h1>机器人放在哪里？</h1>
          <p>
            给机器人起名并选一个保存位置。
          </p>
          <div className="project-fields">
            <label>
              机器人名称
              <input
                value={project.name}
                onChange={event =>
                  dispatch(setProject({ name: event.target.value }))
                }
                placeholder="my-alemonjs-bot"
              />
            </label>
            <div className="location-options">
              <button
                className={
                  project.destinationMode === 'current'
                    ? 'choice selected'
                    : 'choice'
                }
                onClick={() =>
                  dispatch(setProject({ destinationMode: 'current' }))
                }
              >
                <strong>工作区（推荐）</strong>
                <small>
                  {workspace
                    ? `保存到 ${workspace.bots}`
                    : '保存到工作区的 bots 目录。'}
                </small>
              </button>
              <button
                className={
                  project.destinationMode === 'custom'
                    ? 'choice selected'
                    : 'choice'
                }
                onClick={chooseDestination}
              >
                <strong>选择指定文件夹</strong>
                <small>
                  {project.destinationMode === 'custom' && project.destination
                    ? `已选择：${project.destination}`
                    : '在目录选择器中选择保存位置。'}
                </small>
              </button>
            </div>
            <details>
              <summary className="cursor-pointer text-sm">
                选择扩展包（可选）
              </summary>
              <div className="mt-3">
                <OfficialExtensionChoices
                  label="机器人扩展"
                  selected={capabilities}
                  onSelect={resource => toggleCapability(resource.id)}
                />
              </div>
            </details>
            {folderError && (
              <ErrorNotice
                message={folderError}
                onClose={() => setFolderError('')}
              />
            )}
          </div>
        </>
      )
    return (
      <>
        <h1>
          {creation?.status === 'ready' ? '机器人已安装' : '确认安装机器人'}
        </h1>
        {creation?.status === 'ready' ? (
          <>
            <p>机器人已安装至：{creation.path}</p>
            {creation.path && (
              <a
                className="primary-button"
                href={`/dashboard/robot?root=${encodeURIComponent(creation.path)}`}
              >
                前往管理机器人
              </a>
            )}
          </>
        ) : (
          <>
            <p>
              将使用默认 JavaScript 模板、Git
              存档，不附加开发技能、图片工具或 PM2。
            </p>
            <div className="config-summary">
              <span>
                位置：
                {project.destinationMode === 'current'
                  ? workspace
                    ? `${workspace.bots}/${project.name}`
                    : `工作区/${project.name}`
                  : project.destination
                    ? `${project.destination}/${project.name}`
                    : '请返回填写保存位置'}
              </span>
              <span>运行时：{useBun ? 'Bun' : 'Node.js'}</span>
              <span>依赖安装：{useBun ? 'Bun' : 'Yarn'}</span>
              <span>项目存档：初始化 Git</span>
            </div>
          </>
        )}
        {creation?.logs && (
          <div className="creation-logs">
            {creation.logs.map((log, index) => (
              <p key={index}>{log}</p>
            ))}
          </div>
        )}
      </>
    )
  }
  const webPage = () => {
    if (flowStep === 0)
      return (
        <>
          <h1>选择部署方式</h1>
          <div className="grid gap-2.5 my-5">
            <button
              className={webEdition === 'clean' ? 'choice selected' : 'choice'}
              onClick={() => {
                setWebEdition('clean')
                automaticCheck.current = null
              }}
            >
              <strong>纯净版</strong>
              <small>检查 Node.js 与 Git 后启动 alx</small>
            </button>
            <button
              className={webEdition === 'docker' ? 'choice selected' : 'choice'}
              onClick={() => {
                setWebEdition('docker')
                automaticCheck.current = null
              }}
            >
              <strong>Docker 版</strong>
              <small>检查 Docker 后使用 Docker Compose 快速启动</small>
            </button>
          </div>
        </>
      )
    if (flowStep === 1)
      return environmentPage()
    if (webEdition === 'clean' && flowStep === 2)
      return (
        <>
          <h1>选择下载镜像</h1>
          <p>选择下载来源后继续；随后选择版本与安装包。</p>
          <div className="grid gap-2.5 my-5">
            {goal?.mirrors?.map(mirror => (
              <button
                className={
                  selectedMirror?.url === mirror.url
                    ? 'choice selected'
                    : 'choice'
                }
                key={mirror.url}
                onClick={() => {
                  setSelectedMirror(mirror)
                  setSelectedAssetURL(null)
                }}
              >
                <strong>{mirror.name}</strong>
                <small>
                  {mirror.name === 'GitHub 官方'
                    ? '从 GitHub 官方下载'
                    : mirror.name.includes('推荐')
                      ? '推荐的快捷下载入口'
                      : '下载速度可能更快'}
                </small>
              </button>
            ))}
          </div>
        </>
      )
    if (webEdition === 'clean' && flowStep === 3)
      return (
        <>
          <h1>选择版本</h1>
          <p>默认已选择最新正式版本。确认后继续选择适合电脑的安装包。</p>
          {releasePicker()}
        </>
      )
    if (webEdition === 'clean' && flowStep === 4) return assetPicker()
    return (
      <>
        <h1>Docker Compose 快速启动</h1>
        <p>下一步会生成 docker-compose.yml 并启动服务。</p>
      </>
    )
  }
  const buildPage = () => {
    if (flowStep === 0)
      return (
        <>
          <h1>选择构建方式</h1>
          <div className="grid gap-2.5 my-5">
            <button
              className={buildMode === 'npm' ? 'choice selected' : 'choice'}
              onClick={() => {
                setBuildMode('npm')
                automaticCheck.current = null
              }}
            >
              <strong>NPM 构建</strong>
              <small>使用 npm 生成应用构建产物</small>
            </button>
            <button
              className={buildMode === 'git' ? 'choice selected' : 'choice'}
              onClick={() => {
                setBuildMode('git')
                automaticCheck.current = null
              }}
            >
              <strong>Git 化构建</strong>
              <small>遵循 main、release 分支与版本标签标准</small>
            </button>
          </div>
        </>
      )
    if (flowStep === 1)
      return environmentPage()
    return (
      <>
        <h1>{buildMode === 'git' ? 'Git 化构建' : 'NPM 构建'}</h1>
        <p>
          {buildMode === 'git'
            ? '构建产物将按标准整理至 release 分支，并创建对应版本标签。'
            : '将构建应用并生成可分发产物。'}
        </p>
      </>
    )
  }
  const downloadPage = () => {
    if (goal?.id === 'mobile')
      return (
        <>
          <h1>下载 Android 安装包</h1>
          <p>手机版目前仅提供 Android 通用 APK，不经过 GitHub。</p>
          {browserDownloadNotice && (
            <DownloadProgress
              handoff
              className="mt-4"
              label="已开始浏览器下载"
              detail={browserDownloadNotice}
            />
          )}
        </>
      )
    if (flowStep === 0)
      return (
        <>
          <h1>选择下载镜像</h1>
          <p>选择一个下载来源后继续；下一页再选要下载的版本。</p>
          <div className="grid gap-2.5 my-5">
            {goal?.mirrors?.map(mirror => (
              <button
                className={
                  selectedMirror?.url === mirror.url
                    ? 'choice selected'
                    : 'choice'
                }
                key={mirror.url}
                onClick={() => {
                  setSelectedMirror(mirror)
                  setSelectedAssetURL(null)
                }}
              >
                <strong>{mirror.name}</strong>
                <small>
                  {mirror.name === 'GitHub 官方'
                    ? '从 GitHub 官方下载'
                    : mirror.name.includes('推荐')
                      ? '推荐的快捷下载入口'
                      : '下载速度可能更快'}
                </small>
              </button>
            ))}
          </div>
        </>
      )
    if (flowStep === 1)
      return (
        <>
          <h1>选择下载版本</h1>
          <p>默认已选择最新正式版本。确认后继续选择安装包。</p>
          {releasePicker()}
          <p className="selected-download">
            下载镜像：{selectedMirror?.name ?? '尚未选择'}
          </p>
        </>
      )
    return assetPicker()
  }
  const selectPurpose = (id: string) => {
    automaticCheck.current = null
    setSelectedMirror(null)
    setSelectedAssetURL(null)
    onSelect(id)
  }
  const goBack = () => {
    if (step === 1) {
      onSelect(null)
      setStep(0)
      return
    }
    back()
  }
  const resetPurpose = () => {
    onSelect(null)
    setStep(0)
  }
  registerBack(step > 0 ? goBack : () => {})
  const officialIDs = new Set(
    officialExtensions?.data.map(resource => resource.id) ?? []
  )
  const isLegacyCapability = (id: string) =>
    !officialIDs.has(id) &&
    Object.prototype.hasOwnProperty.call(capabilityLabels, id)
  const createConfig = (): ProjectConfig =>
    isInstaller
      ? {
          template: 'bot',
          name: project.name,
          destinationMode: project.destinationMode,
          destination: project.destination,
          language: 'js',
          runtime: projectRuntime,
          packageManager: useBun ? 'bun' : 'yarn',
          eslint: false,
          initializeGit: true,
          usePM2: false,
          imageMode: 'none',
          styleMode: 'css',
          downloadSkills: false,
          developmentPackages: [],
          resourceIDs: capabilities.filter(item => !isLegacyCapability(item))
        }
      : {
          template: 'dev',
          name: project.name,
          destinationMode: project.destinationMode,
          destination: project.destination,
          language: config.language,
          runtime: projectRuntime,
          packageManager: useBun ? 'bun' : config.manager === 'bun' ? 'yarn' : config.manager,
          eslint: config.eslint === 'yes',
          initializeGit: config.git === 'yes',
          usePM2: config.pm2 === 'yes',
          imageMode: config.image,
          styleMode: config.image === 'react' ? config.style : 'css',
          downloadSkills: config.skills === 'yes',
          developmentPackages: capabilities.filter(item =>
            isLegacyCapability(item)
          ),
          resourceIDs: capabilities.filter(item => !isLegacyCapability(item))
        }
  const isDownloadFlow = goal?.id === 'mobile'
  const isWeb = goal?.id === 'web'
  const isBuild = goal?.id === 'build'
  function stepAction(): { label: string; disabled?: boolean; onClick?: () => void; href?: string } | null {
    if (!goal || step === 0) return null
    const continueAction = (disabled = false) => ({ label: '继续', disabled, onClick: next })
    if (isEnvironmentStep) {
      return continueAction(checking || failedEnvironment || !environmentReport?.ready)
    }
    if (isInstaller || isDeveloper) {
      const confirmationStep = isInstaller ? 2 : 11
      if (flowStep < confirmationStep) return continueAction()
      if (flowStep !== confirmationStep || creation?.status === 'ready') return null
      return {
        label: isInstaller
          ? creating ? '正在安装…' : '确认安装'
          : creating ? '正在创建…' : '确认创建',
        disabled: creating,
        onClick: () => onCreate(createConfig())
      }
    }
    if (isWeb) {
      if (flowStep === 0) return continueAction(!webEdition)
      if (flowStep === 2 && webEdition === 'docker') return { label: '生成并启动', disabled: true }
      if (webEdition !== 'clean') return null
      if (flowStep === 2) return continueAction(!selectedMirror)
      if (flowStep === 3) return continueAction(!releaseURL)
      if (flowStep === 4) {
        const href = selectedAssetURL ? mirrorURL(selectedMirror, selectedAssetURL) : ''
        return { label: '开始下载', href, disabled: !href }
      }
      return null
    }
    if (isBuild) {
      if (flowStep === 0) return continueAction(!buildMode)
      return flowStep === 2 ? { label: '开始构建', disabled: true } : null
    }
    if (isDownloadFlow) {
      return { label: '下载 Android APK', href: goal.downloadUrl, disabled: !goal.downloadUrl }
    }
    return step < totalSteps.length - 1 ? continueAction() : null
  }

  const action = stepAction()
  const purposeOptions = [
    ['deploy', '部署', '部署源码版、手机版或 Web 版。'],
    ['develop', '开发', '按需选择并创建 AlemonJS 开发模板。'],
    ['manage', '管理', '如果已有可用项目, 点击进入管理面板。']
  ]
  return (
    <section className="wizard">
      <GuideFlowNavigation
        target={stepsTarget}
        title={goal?.title ?? '引导流程'}
        steps={totalSteps}
        currentStep={step}
        onBack={onBack}
        onSelect={index => {
          onStepSelect()
          if (index === 0) resetPurpose()
          else setFlowStep(index)
        }}
      />
      <GuideProgress
        target={progressTarget}
        text={goal ? `第 ${step + 1} / ${totalSteps.length} 步 · ${totalSteps[step]}` : '选择目的'}
      />
      <section
        className={isEnvironmentStep ? 'wizard-page wizard-flow-page guide-environment-page' : 'wizard-page wizard-flow-page'}
      >
        <div ref={stepViewport} className="wizard-scroll">
          <div className="wizard-content">
            {!goal || step === 0 ? (
              <div className="guide-question guide-choice-screen mx-auto max-w-140 text-center">
                <p className="guide-choice-eyebrow">ALemonX</p>
                <h1>你现在想做什么？</h1>
                <p className="guide-choice-description">
                  选择一个目标；引导会只展示必要步骤，并在需要时检查你的环境。
                </p>
                <div className="question-options" role="list">
                  {loading ? (
                    <p>正在准备选项…</p>
                  ) : (
                    purposeOptions.map(([id, title, note]) => (
                      <button
                        type="button"
                        className="guide-choice-card"
                        key={id}
                        onClick={() =>
                          id === 'deploy'
                            ? navigate('/guide/group/deploy')
                            : selectPurpose(id)
                        }
                      >
                        <i>{icons[id] ?? '·'}</i>
                        <span>
                          <strong>{title}</strong>
                          <small>{note}</small>
                        </span>
                        <b aria-hidden="true">→</b>
                      </button>
                    ))
                  )}
                </div>
              </div>
            ) : isInstaller ? (
              installerPage()
            ) : isDeveloper ? (
              developerPage()
            ) : isWeb ? (
              webPage()
            ) : isBuild ? (
              buildPage()
            ) : isDownloadFlow ? (
              downloadPage()
            ) : (
              <>
                <h1>{totalSteps[step]}</h1>
                <p>{goal.description}</p>
              </>
            )}{' '}
          </div>
        </div>
        {action && (
          <footer className="wizard-actions">
            {action.href ? (
              <a
                className="next-button"
                href={action.href}
                target="_blank"
                rel="noreferrer"
                onClick={() =>
                  setBrowserDownloadNotice(
                    '下载已交给浏览器，请在浏览器下载栏查看实际进度。'
                  )
                }
              >
                {action.label}
              </a>
            ) : (
              <button type="button" className="next-button" disabled={action.disabled} onClick={action.onClick}>
                {action.label}
              </button>
            )}
          </footer>
        )}
      </section>
    </section>
  )
}
