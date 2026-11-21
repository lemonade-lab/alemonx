import { useStoreState } from '../../store/guideStore'
import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useDispatch } from 'react-redux'
import { ArrowLeft, ArrowRight, Check, Code2 } from 'lucide-react'
import cn from 'classnames'
import { setProject } from '../../store/guideStore'
import { DirectoryPicker } from '../../components/Dashboard'
import { ErrorNotice } from '../../components/ErrorNotice'
import { Button } from '../../components/Button'
import { guideIcons } from './icons'

type Props = {
  group?: string
  stepsTarget: HTMLDivElement | null
  progressTarget: HTMLDivElement | null
  onBack?: () => void
  onStepSelect: () => void
  error: string
  onSelect: (id: string | null) => void
  onClearError: () => void
  children: ReactNode
}

export function GuideHome({
  group,
  stepsTarget,
  progressTarget,
  onBack,
  onStepSelect,
  error,
  onSelect,
  onClearError,
  children
}: Props) {
  const dispatch = useDispatch()
  const [directoryPickerOpen, setDirectoryPickerOpen] = useStoreState(false)
  useEffect(() => {
    const open = () => setDirectoryPickerOpen(true)
    window.addEventListener('alemon:choose-directory', open)
    return () => window.removeEventListener('alemon:choose-directory', open)
  }, [setDirectoryPickerOpen])

  return (
    <section className="workbench-guide flex min-h-0 min-w-0 flex-1 flex-col" aria-label="引导">
      {error && <ErrorNotice message={error} onClose={onClearError} />}
      {group ? (
        <>
          <GuideFlowNavigation
            target={stepsTarget}
            title={group === 'deploy' ? '部署引导' : '开发引导'}
            steps={['选择目的']}
            currentStep={0}
            onBack={onBack}
            onSelect={() => {
              onStepSelect()
              onSelect(null)
            }}
          />
          <GuideProgress target={progressTarget} text="选择目的" />
          <PurposeGroup group={group} onSelect={onSelect} />
        </>
      ) : (
        children
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
    </section>
  )
}

export function GuideProgress({ target, text }: {
  target: HTMLDivElement | null
  text: string
}) {
  return target && createPortal(
    <p className="m-0 truncate text-sm font-semibold text-(--theme-text-secondary)" aria-live="polite" title={text}>
      {text}
    </p>,
    target
  )
}

export function GuideFlowNavigation({ target, title, steps, currentStep, onBack, onSelect }: {
  target: HTMLDivElement | null
  title: string
  steps: string[]
  currentStep: number
  onBack?: () => void
  onSelect: (step: number) => void
}) {
  const active = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    active.current?.scrollIntoView({ block: 'nearest' })
  }, [currentStep, target])

  if (!target) return null

  return createPortal(
    <nav className="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto p-2.5" aria-label="引导流程">
      <div className="mb-1 flex min-h-8 min-w-0 items-center justify-between gap-2 px-2">
        <h2 className="min-w-0 truncate text-xs font-semibold text-(--theme-text-muted)" title={title}>{title}</h2>
        {onBack && (
          <Button variant="icon" className="bg-transparent! hover:bg-(--theme-surface-hover)!" onClick={onBack} aria-label="返回" title="返回">
            <ArrowLeft className="size-4" aria-hidden="true" />
          </Button>
        )}
      </div>
      <ol className="m-0 grid list-none gap-1 p-0">
        {steps.map((label, index) => (
          <li key={label} className="min-w-0">
            <button
              type="button"
              ref={index === currentStep ? active : undefined}
              aria-current={index === currentStep ? 'step' : undefined}
              disabled={index > currentStep}
              className={cn(
                'flex min-h-10 w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm enabled:hover:bg-(--theme-surface-hover) disabled:cursor-default',
                index === currentStep
                  ? 'bg-(--theme-accent-soft) font-semibold text-(--theme-accent-text)'
                  : 'text-(--theme-text-secondary) disabled:text-(--theme-text-muted)'
              )}
              onClick={() => onSelect(index)}
            >
              <span
                aria-hidden="true"
                className={cn(
                  'grid size-6 shrink-0 place-items-center rounded-full text-xs font-semibold',
                  index === currentStep
                    ? 'bg-(--theme-accent) text-(--theme-accent-contrast)'
                    : index < currentStep
                      ? 'bg-(--theme-success-soft) text-(--theme-success-text)'
                      : 'bg-(--theme-surface-active)'
                )}
              >
                {index < currentStep ? <Check className="size-3.5" /> : index + 1}
              </span>
              <span>{label}</span>
            </button>
          </li>
        ))}
      </ol>
    </nav>,
    target
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
