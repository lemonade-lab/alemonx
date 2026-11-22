import { RefreshButton } from '../../components/RefreshButton'
import cn from 'classnames'
import {
  AlertCircle,
  Check,
  ChevronDown,
  Loader2,
  Terminal
} from 'lucide-react'
import { Button } from '../../components/Button'
import type { Check as EnvironmentCheck, Report } from './types'

const runtimeOptions = [
  {
    id: 'node',
    label: 'Node.js 22 LTS',
    detail: '使用 npm、Yarn 或 pnpm 安装依赖'
  },
  { id: 'bun', label: 'Bun', detail: '使用 Bun 运行项目和安装依赖' }
] as const

type RuntimeChoice = {
  value: 'node' | 'bun'
  onChange: (value: RuntimeChoice['value']) => void
}

type Props = {
  report: Report | null
  checking: boolean
  failed: boolean
  runtime?: RuntimeChoice
  onCheck: () => void
  onFix?: (check: EnvironmentCheck) => void
}

const statusLabels: Record<EnvironmentCheck['status'], string> = {
  ready: '已就绪',
  missing: '未检测到',
  warning: '需修复',
  outdated: '建议升级'
}

function CheckRows({
  checks,
  checking,
  onFix
}: {
  checks: EnvironmentCheck[]
  checking: boolean
  onFix?: Props['onFix']
}) {
  return (
    <ul className="m-0 divide-y divide-(--theme-border-default) rounded-xl bg-(--theme-surface-raised) px-3.5">
      {checks.map(check => {
        const ready = check.status === 'ready'
        const action =
          check.status === 'outdated'
            ? '升级'
            : check.status === 'missing'
              ? '安装'
              : '修复'
        const Icon = ready ? Check : AlertCircle
        return (
          <li
            key={check.id}
            className="flex min-w-0 flex-wrap items-start gap-x-3 gap-y-2 py-3"
          >
            <span
              className={cn(
                'mt-0.5 grid size-6 shrink-0 place-items-center rounded-full',
                ready
                  ? 'bg-(--theme-success-soft) text-(--theme-success-text)'
                  : 'bg-(--theme-warning-soft) text-(--theme-warning-text)'
              )}
            >
              <Icon className="size-3.5" aria-hidden="true" />
            </span>
            <div className="min-w-0 flex-1 basis-36">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
                <strong className="text-sm font-semibold text-(--theme-text-strong)">
                  {check.name}
                </strong>
                <span
                  className={cn(
                    'text-xs font-medium',
                    ready
                      ? 'text-(--theme-success-text)'
                      : 'text-(--theme-warning-text)'
                  )}
                >
                  {statusLabels[check.status]}
                </span>
              </div>
              {check.detail && check.detail !== statusLabels[check.status] && (
                <p className="mt-1 mb-0 break-words text-xs leading-5 text-(--theme-text-secondary)">
                  {check.detail}
                </p>
              )}
              {!ready && check.suggestion && (
                <p className="mt-1 mb-0 break-words text-xs leading-5 text-(--theme-text-secondary)">
                  {check.suggestion}
                </p>
              )}
            </div>
            {!ready && onFix && (
              <Button
                type="button"
                size="sm"
                variant="secondary"
                className="ml-auto self-center"
                disabled={checking}
                aria-label={`${action} ${check.name}`}
                onClick={() => onFix(check)}
              >
                {action}
              </Button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function EnvironmentCheckPanel({
  report,
  checking,
  failed,
  runtime,
  onCheck,
  onFix
}: Props) {
  const required = report?.checks.filter(check => !check.optional) ?? []
  const optional = report?.checks.filter(check => check.optional) ?? []
  const upgrades = required.some(check => check.status === 'outdated')
  const summary = checking
    ? '正在检查所需工具…'
    : failed
      ? '本次检查失败，请重新检查。'
      : !report
        ? '等待检查。'
        : report.ready
          ? upgrades
            ? '检测到建议升级的环境，仍可继续。'
            : '环境已就绪，可以继续。'
          : '请先处理必需工具，再继续。'

  return (
    <section className="@container mx-auto grid w-full max-w-220 gap-5 text-left">
      <header className="flex flex-wrap items-end justify-between gap-3 border-b border-(--theme-border-default) pb-4">
        <div className="grid min-w-0 gap-1">
          <h1 className="m-0! text-2xl! font-semibold! tracking-tight!">
            检查运行环境
          </h1>
          <p
            className="m-0 text-sm text-(--theme-text-secondary)"
            role="status"
          >
            {summary}
          </p>
        </div>
        <RefreshButton label="重新检查" onClick={onCheck} busy={checking} />
      </header>

      <div
        className={cn(
          'grid items-start gap-5',
          runtime && '@min-[38rem]:grid-cols-[minmax(0,3fr)_minmax(0,5fr)]'
        )}
      >
        {runtime && (
          <fieldset className="m-0 min-w-0 border-0 p-0">
            <legend className="mb-2 p-0 text-sm font-semibold text-(--theme-text-strong)">
              JavaScript 运行时
            </legend>
            <div className="grid gap-2">
              {runtimeOptions.map(option => (
                <label
                  key={option.id}
                  className={cn(
                    'flex cursor-pointer items-start gap-2.5 rounded-xl border p-3 transition-colors focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-(--theme-accent)',
                    runtime.value === option.id
                      ? 'border-(--theme-accent-soft-border) bg-(--theme-accent-soft)'
                      : 'border-transparent bg-(--theme-surface-raised) hover:bg-(--theme-surface-hover)'
                  )}
                >
                  <input
                    type="radio"
                    name="guide-project-runtime"
                    value={option.id}
                    checked={runtime.value === option.id}
                    onChange={() => runtime.onChange(option.id)}
                    className="mt-0.5 size-4 shrink-0 accent-(--theme-accent)"
                    aria-label={option.label}
                  />
                  <span className="grid min-w-0 gap-0.5">
                    <strong className="text-sm font-semibold text-(--theme-text-strong)">
                      {option.label}
                    </strong>
                    <small className="text-xs leading-5 text-(--theme-text-secondary)">
                      {option.detail}
                    </small>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
        <div className="grid min-w-0 gap-4" aria-busy={checking}>
          <section aria-label="必需工具" className="grid gap-2">
            <h2 className="m-0 text-sm font-semibold text-(--theme-text-strong)">
              必需工具
            </h2>
            {required.length > 0 ? (
              <CheckRows
                checks={required}
                checking={checking || failed}
                onFix={onFix}
              />
            ) : (
              <div className="flex min-h-20 items-center gap-2 rounded-xl bg-(--theme-surface-raised) px-3.5 py-3 text-sm text-(--theme-text-secondary)">
                {checking ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden="true" />
                ) : (
                  <Terminal className="size-4" aria-hidden="true" />
                )}
                {checking
                  ? '正在读取版本和可用状态'
                  : failed
                    ? '检查结果暂不可用'
                    : '检查结果将在此显示'}
              </div>
            )}
          </section>
          {optional.length > 0 && (
            <details className="group">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-2 rounded-lg px-1 py-1.5 text-sm marker:hidden focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) [&::-webkit-details-marker]:hidden">
                <span className="flex flex-wrap items-center gap-x-2">
                  <strong className="font-semibold text-(--theme-text-strong)">
                    可选工具
                  </strong>
                  <span className="text-xs text-(--theme-text-secondary)">
                    {optional.length} 项 · 不影响继续
                  </span>
                </span>
                <ChevronDown
                  className="size-4 shrink-0 text-(--theme-text-secondary) transition-transform group-open:rotate-180"
                  aria-hidden="true"
                />
              </summary>
              <div className="pt-1.5">
                <CheckRows
                  checks={optional}
                  checking={checking || failed}
                  onFix={onFix}
                />
              </div>
            </details>
          )}
        </div>
      </div>
    </section>
  )
}
