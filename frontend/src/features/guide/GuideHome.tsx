import { useStoreState } from '../../store/guideStore'
import { useEffect, useRef, type CSSProperties, type ReactNode } from 'react'
import { useDispatch } from 'react-redux'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, Code2 } from 'lucide-react'
import { setProject } from '../../store/guideStore'
import { DirectoryPicker } from '../../components/Dashboard'
import { ErrorNotice } from '../../components/ErrorNotice'
import { GuideHeader } from '../../components/GuideHeader'
import type { Goal } from './types'
import { guideIcons } from './icons'

type Props = {
  group?: string
  goal?: Goal
  error: string
  onSelect: (id: string | null) => void
  onClose: () => void
  onOpenSettings: () => void
  onClearError: () => void
  windowStyle?: CSSProperties
  windowControls?: ReactNode
  renderFlow: (registerBack: (handler: () => void) => void) => ReactNode
}

export function GuideHome({
  group,
  goal,
  error,
  onSelect,
  onClose,
  onOpenSettings,
  onClearError,
  renderFlow,
  windowStyle,
  windowControls
}: Props) {
  const backAction = useRef<() => void>(() => {})
  const dispatch = useDispatch()
  const [directoryPickerOpen, setDirectoryPickerOpen] = useStoreState(false)
  const navigate = useNavigate()
  useEffect(() => {
    const open = () => setDirectoryPickerOpen(true)
    window.addEventListener('alemon:choose-directory', open)
    return () => window.removeEventListener('alemon:choose-directory', open)
  }, [setDirectoryPickerOpen])

  return (
    <main className="guide-shell">
      <section className="guide-window" style={windowStyle}>
        <GuideHeader
          onBack={() => (group ? navigate('/guide') : backAction.current())}
          onClose={onClose}
          onOpenSettings={onOpenSettings}
          showBack={Boolean(goal || group)}
        />
        {error && <ErrorNotice message={error} onClose={onClearError} />}
        {group ? (
          <PurposeGroup group={group} onSelect={onSelect} />
        ) : (
          renderFlow(handler => {
            backAction.current = handler
          })
        )}
        <DirectoryPicker
          open={directoryPickerOpen}
          multiple={false}
          onClose={() => setDirectoryPickerOpen(false)}
          onSelect={paths => {
            dispatch(
              setProject({ destinationMode: 'custom', destination: paths[0] })
            )
            setDirectoryPickerOpen(false)
          }}
        />
        {windowControls}
      </section>
    </main>
  )
}

function PurposeGroup({
  group,
  onSelect
}: {
  group: string
  onSelect: (id: string) => void
}) {
  const options =
    group === 'deploy'
      ? [
          [
            'install',
            '源码版(推荐)',
            '创建一个可用于生产环境的机器人源码项目。'
          ],
          ['mobile', '手机版(简单)', '下载 Android 端的 通用 APK 安装包。'],
          ['web', 'Web版(困难)', '下载 alx，或使用 Docker 部署 alx。']
        ]
      : [
          [
            'develop',
            '开发机器人',
            '创建一个可自由选择语言、依赖和工具的开发项目。'
          ]
        ]

  return (
    <section className="wizard purpose-group">
      <section className="wizard-page">
        <div className="wizard-content">
          <div className="guide-question guide-choice-screen mx-auto max-w-140 text-center">
            <p className="guide-choice-eyebrow">
              部署向导
            </p>
            <h1>
              {group === 'deploy' ? '你要部署哪一种版本？' : '开始开发机器人'}
            </h1>
            <p className="text-(--theme-text-muted) text-[0.95rem] leading-[1.65] mt-3.5">
              选择一种方式，接下来只会展示与它有关的步骤。
            </p>
            <div className="question-options" role="list">
              {options.map(([id, title, note]) => (
                <button
                  type="button"
                  className="guide-choice-card"
                  key={id}
                  onClick={() => onSelect(id)}
                >
                  <i>{guideIcons[id] ?? <Code2 />}</i>
                  <span>
                    <strong>{title}</strong>
                    <small>{note}</small>
                  </span>
                  <b aria-hidden="true">
                    <ArrowRight />
                  </b>
                </button>
              ))}
            </div>
          </div>
        </div>
      </section>
    </section>
  )
}
