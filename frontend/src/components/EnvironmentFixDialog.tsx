import { ArrowUpRight, Download, Loader2, X } from 'lucide-react'
import { useState } from 'react'
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
  const [installing, setInstalling] = useState(false)
  const [installed, setInstalled] = useState(false)
  const [message, setMessage] = useState('')
  const [browserDownloadNotice, setBrowserDownloadNotice] = useState('')
  const isLinux = platform.startsWith('linux/')
  const canInstallOnServer =
    (isLinux || platform.startsWith('darwin/') || platform.startsWith('windows/')) &&
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
      const body = (await response.json()) as {
        output?: string
        error?: string
      }
      if (!response.ok) throw new Error(body.error || '服务器安装未完成。')
      setMessage(body.output || '服务器安装已完成，请重新检查环境。')
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
    <Modal open onClose={onClose} ariaLabel="环境修复">
      <section
        className="relative w-full max-w-110 rounded-xl border border-slate-200 bg-white p-6 shadow-[0_24px_60px_rgb(28_26_23/0.24)]"
        role="dialog"
        aria-modal="true"
        aria-labelledby="environment-fix-title"
        onMouseDown={event => event.stopPropagation()}
      >
        <button
          className="absolute right-3 top-3 inline-flex size-8 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-100 hover:text-slate-700 focus:outline-none focus:ring-2 focus:ring-brand-200"
          onClick={onClose}
          aria-label="关闭"
        >
          <X className="size-4" />
        </button>
        <h2
          id="environment-fix-title"
          className="mr-9 text-base font-semibold text-ink-950"
        >
          {isNodeUpgrade ? '升级' : '安装'} {check.name}
        </h2>
        <p className="mt-2 text-sm leading-6 text-slate-500">
          {check.suggestion || '请选择官方安装包，完成后返回环境面板重新检查。'}
        </p>
        {canInstallOnServer ? (
          installing && (
            <DownloadProgress
              className="mt-5"
              label={`正在安装 ${check.name}`}
              detail="正在安装，请稍候。"
            />
          )
        ) : (
          <div className="mt-5 grid gap-2">
            {options.map(option => (
              <a
                className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 p-3 text-brand-700 transition hover:border-brand-200 hover:bg-brand-50"
                href={option.href}
                target="_blank"
                rel="noreferrer"
                key={option.href}
                onClick={() =>
                  setBrowserDownloadNotice(
                    '已交给浏览器下载，完成后回到这里继续。'
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
          <DownloadProgress
            className="mt-4"
            handoff
            label="已开始浏览器下载"
            detail={browserDownloadNotice}
          />
        )}
        {message && (
          <p className="mt-5 text-xs leading-5 text-slate-500">
            {message}
          </p>
        )}
        <footer className="mt-6 flex justify-end gap-2">
          {canInstallOnServer && !installed ? (
            <>
              <button className="secondary-button" disabled={installing} onClick={onClose}>
                取消
              </button>
              <button
                className="primary-button inline-flex items-center gap-2"
                disabled={installing}
                onClick={() => void installOnServer()}
              >
                {installing ? <Loader2 className="size-4 animate-spin" /> : <Download className="size-4" />}
                {installing ? '安装中…' : isNodeUpgrade ? '升级' : '安装'}
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
