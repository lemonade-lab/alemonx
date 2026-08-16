import { AlertTriangle as AlertTriangleIcon, Check as CheckIcon } from 'lucide-react'
import type { Check, Report } from './types'

function statusLabel(status: string) {
  if (status === 'missing') return '未检测到'
  if (status === 'warning') return '需处理'
  if (status === 'outdated') return '建议升级'
  return '待处理'
}

type Props = {
  title: string
  report: Report | null
  checking: boolean
  onCheck: () => void
  onFix?: (check: Check) => void
}

export function EnvironmentCheckPanel({
  title,
  report,
  checking,
  onCheck,
  onFix
}: Props) {
  const hasIssues = Boolean(
    report?.checks.some(check => check.status !== 'ready' && !check.optional)
  )
  const ready = Boolean(report?.ready) && !hasIssues

  return (
    <section className="mx-auto grid w-full max-w-160 gap-5 pt-8">
      <header className="flex items-start justify-between gap-5">
        <div>
          <h1 className="m-0 text-xl font-bold tracking-tight text-slate-800">
            {title}
          </h1>
          <p className="mt-1.5 text-sm text-slate-500">
            {checking || !report
              ? '正在检查所需工具…'
              : ready
                ? '环境已就绪，可以继续。'
                : report?.ready
                  ? '检测到建议升级的环境，仍可继续。'
                  : '有项目需要先处理。'}
          </p>
        </div>
        <button
          className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-bold text-slate-600 transition hover:border-brand-200 hover:text-brand-600 disabled:cursor-wait disabled:opacity-50"
          onClick={onCheck}
          disabled={checking}
        >
          {checking ? '检查中' : '重新检查'}
        </button>
      </header>
      {checking || !report ? (
        <div className="flex min-h-32 items-center justify-center rounded-2xl border border-dashed border-slate-200 bg-slate-50">
          <span className="checking-indicator" />
        </div>
      ) : (
        <div className="choice-list">
          {report.checks.map(check => {
            const isReady = check.status === 'ready'
            return (
              <div className="check-row" key={check.id}>
                <span
                  className={`inline-flex size-7 shrink-0 items-center justify-center rounded-full text-white ${
                    isReady ? 'bg-emerald-500' : 'bg-amber-500'
                  }`}
                >
                  {isReady ? (
                    <CheckIcon className="size-4" strokeWidth={3} />
                  ) : (
                    <AlertTriangleIcon className="size-4" strokeWidth={2.5} />
                  )}
                </span>
                <div className="check-main">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <strong className="text-sm font-semibold text-slate-800">
                      {check.name}
                    </strong>
                    {check.optional && (
                      <em className="not-italic rounded-full bg-slate-200/70 px-2 py-0.5 text-[10px] font-medium text-slate-500">
                        可选
                      </em>
                    )}
                    <span
                      className={`ml-auto rounded-full px-2 py-0.5 text-[10px] font-medium ${
                        isReady
                          ? 'bg-emerald-100 text-emerald-700'
                          : 'bg-amber-100 text-amber-700'
                      }`}
                    >
                      {isReady ? '已就绪' : statusLabel(check.status)}
                    </span>
                  </div>
                  <p className="mt-1 text-xs leading-5 text-slate-500">
                    {check.detail}
                  </p>
                  {!isReady && check.suggestion && (
                    <p className="mt-1 text-xs leading-5 text-amber-700">
                      {check.suggestion}
                    </p>
                  )}
                </div>
                {!isReady && onFix && (
                  <button
                    className="shrink-0 rounded-lg border border-brand-200 bg-white px-3 py-1.5 text-xs font-semibold text-brand-700 shadow-sm transition hover:bg-brand-50"
                    onClick={() => onFix(check)}
                  >
                    {check.id === 'node' && check.status === 'outdated'
                      ? '升级'
                      : check.id === 'browser'
                        ? '安装'
                      : '修复'}
                  </button>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
