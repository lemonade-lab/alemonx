import { ArrowUpRight, Download, Loader2, X } from 'lucide-react'
import { useStoreState } from '../store/guideStore'
import { Button } from './Button'
import { Modal } from './Modal'
import { DownloadProgress } from './DownloadProgress'

type Check = { id: string; name: string; status?: string; suggestion: string }

type Props = {
  check: Check
  platform?: string
  onClose: () => void
  onInstalled?: () => void
}

const links: Record<string, Array<{ label: string; href: string }>> = {
  node: [
    {
      label: 'Node.js 官方下载',
      href: 'https://nodejs.org/en/download'
    }
  ],
  git: [
    {
      label: 'Git 官方下载',
      href: 'https://git-scm.com/downloads'
    }
  ],
  docker: [
    {
      label: 'Docker Desktop',
      href: 'https://www.docker.com/products/docker-desktop/'
    }
  ],
  browser: [
    {
      label: 'Google Chrome',
      href: 'https://www.google.com/chrome/'
    },
    {
      label: 'Chromium',
      href: 'https://www.chromium.org/getting-involved/download-chromium/'
    }
  ],
  fonts: [
    {
      label: 'Google Noto 字体',
      href: 'https://fonts.google.com/noto'
    }
  ]
}

export function EnvironmentFixDialog({
  check,
  platform = '',
  onClose,
  onInstalled
}: Props) {
  const options = links[check.id] ?? []
  const [installing, setInstalling] = useStoreState(false)
  const [installed, setInstalled] = useStoreState(false)
  const [message, setMessage] = useStoreState('')
  const [browserDownloadNotice, setBrowserDownloadNotice] = useStoreState('')
  const isLinux = platform.startsWith('linux/')
  const canInstallOnServer =
    (isLinux ||
      platform.startsWith('darwin/') ||
      platform.startsWith('windows/')) &&
    (['browser-dependencies', 'common-dependencies'].includes(check.id)
      ? isLinux
      : ['node', 'git', 'docker', 'browser', 'fonts'].includes(check.id))
  const isManagedNode = check.id === 'node'
  const isNodeUpgrade = isManagedNode && check.status === 'outdated'
  const installOnServer = async () => {
    setInstalling(true)
    setInstalled(false)
    setMessage('')
    try {
      const response = await fetch('/api/v1/system/environment/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ checkId: check.id, confirm: true })
      })
      const text = await response.text()
      let body: { output?: string; error?: string } = {}
      try {
        body = JSON.parse(text) as { output?: string; error?: string }
      } catch {
        // Some desktop or reverse-proxy failure pages are plain text. Keep the
        // original diagnostic instead of replacing it with a JSON parse error.
      }
      if (!response.ok) {
        throw new Error(body.error || text.trim() || '服务器安装未完成。')
      }
      setMessage(
        body.output || text.trim() || '服务器安装已完成，请重新检查环境。'
      )
      setInstalled(true)
      onInstalled?.()
    } catch (reason) {
      setMessage(
        reason instanceof Error ? reason.message : '服务器安装未完成。'
      )
    } finally {
      setInstalling(false)
    }
  }
  return (
    <Modal
      open
      onClose={installing ? undefined : onClose}
      ariaLabel="环境修复"
      className="p-4"
    >
      <section
        className="relative flex max-h-[calc(100dvh-32px)] w-full max-w-110 flex-col rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-5 text-(--theme-text-primary) shadow-xl"
        onMouseDown={event => event.stopPropagation()}
      >
        <Button
          variant="icon"
          className="absolute right-3 top-3"
          disabled={installing}
          onClick={onClose}
          aria-label="关闭"
        >
          <X className="size-4" />
        </Button>
        <h2
          id="environment-fix-title"
          className="mr-12 shrink-0 text-base font-semibold text-(--theme-text-strong)"
        >
          {isNodeUpgrade ? '升级' : '安装'} {check.name}
        </h2>
        <div className="min-h-0 overflow-y-auto overscroll-contain">
          <p className="mt-2 text-sm leading-6 text-(--theme-text-secondary)">
            {check.suggestion ||
              '请选择官方安装包，完成后返回环境面板重新检查。'}
          </p>
          {canInstallOnServer ? (
            installing && (
              <DownloadProgress
                className="mt-5"
                label={`正在安装 ${check.name}`}
                detail="安装期间请保持此窗口打开，完成后将自动更新环境状态。"
              />
            )
          ) : (
            <div className="mt-5 grid gap-2">
              {options.map(option => (
                <a
                  className="flex items-center justify-between gap-3 rounded-lg border border-(--theme-border-default) p-3 text-(--theme-accent-text) transition hover:border-(--theme-accent) hover:bg-(--theme-accent-soft)"
                  href={option.href}
                  target="_blank"
                  rel="noreferrer"
                  key={option.href}
                  onClick={() =>
                    setBrowserDownloadNotice(
                      '已打开官方下载页面。请在新页面选择安装包，安装完成后返回检查环境。'
                    )
                  }
                >
                  <span className="min-w-0">
                    <strong className="text-sm font-semibold">
                      {option.label}
                    </strong>
                  </span>
                  <ArrowUpRight className="size-4 shrink-0 text-slate-400" />
                </a>
              ))}
            </div>
          )}
          {browserDownloadNotice && (
            <p
              role="status"
              className="mt-4 text-sm leading-6 text-(--theme-text-secondary)"
            >
              {browserDownloadNotice}
            </p>
          )}
          {message && (
            <div className="mt-5 rounded-lg border border-(--theme-border-default) bg-(--theme-surface-raised) p-3">
              <p
                role={installed ? 'status' : 'alert'}
                className="m-0 text-sm font-medium text-(--theme-text-strong)"
              >
                {installed ? '安装完成' : '安装未完成，请查看详情后重试。'}
              </p>
              <details className="mt-2 text-xs text-(--theme-text-secondary)">
                <summary className="cursor-pointer py-1">
                  {installed ? '查看安装日志' : '查看错误详情'}
                </summary>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono leading-5">
                  {message}
                </pre>
              </details>
            </div>
          )}
        </div>
        <footer className="mt-5 flex shrink-0 flex-wrap justify-end gap-2">
          {canInstallOnServer && !installed ? (
            <>
              <button
                className="secondary-button"
                disabled={installing}
                onClick={onClose}
              >
                取消
              </button>
              <button
                className="primary-button inline-flex items-center gap-2"
                disabled={installing}
                onClick={() => void installOnServer()}
              >
                {installing ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Download className="size-4" />
                )}
                {installing
                  ? '安装中…'
                  : message
                    ? '重试安装'
                    : isNodeUpgrade
                      ? '升级'
                      : '安装'}
              </button>
            </>
          ) : (
            <button className="primary-button" onClick={onClose}>
              {installed ? '完成' : '关闭'}
            </button>
          )}
        </footer>
      </section>
    </Modal>
  )
}
