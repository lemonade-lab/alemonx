import { useStoreState } from '../store/guideStore'
import {
  CheckCircle2,
  Download,
  ExternalLink,
  FileArchive,
  Home,
  RefreshCw,
  Upload,
  X
} from 'lucide-react'
import { useCallback, useEffect, useId, useState } from 'react'
import { ConfirmDialog } from './ConfirmDialog'
import { DownloadProgress } from './DownloadProgress'
import { Tabs } from './Tabs'
import {
  useLazySetupUpdateQuery,
  useReleasesQuery
} from '../store/workspaceApi'
import { Button } from './Button'

type Release = {
  tag: string
  name: string
  url: string
  assets: Array<{ name: string; url: string }>
}

type UpdateTransaction = {
  phase: string
  targetVersion?: string
  previousVersion?: string
  error?: string
  pluginError?: string
}

const acceptedUpdateArchiveExtensions = ['.zip']
const maxUpdateArchiveSize = 200 * 1024 * 1024

function updateArchiveValidationError(candidate: File): string | null {
  const name = candidate.name.trim().toLowerCase()
  if (!acceptedUpdateArchiveExtensions.some(extension => name.endsWith(extension))) {
    return '请选择 GitHub Release 下载的 .zip 更新包。'
  }
  if (candidate.size === 0) {
    return '该更新包为空，请重新选择完整下载的安装包。'
  }
  if (candidate.size > maxUpdateArchiveSize) {
    return '更新包超过 200MB 限制，请下载正确的 Release 安装包。'
  }
  return null
}

const updatePhaseLabels: Record<string, string> = {
  checking: '正在检查更新…',
  downloading: '正在下载更新包…',
  staged: '更新包已就绪，等待确认安装。',
  applying: '正在替换应用文件…',
  restarting: '正在重启并验证新版…',
  healthy: '新版已验证并正常运行。',
  rolled_back: '新版未能启动，已恢复旧版本。',
  failed: '更新未完成。'
}

export function SetupUpdateButton({ embedded = false }: { embedded?: boolean }) {
  const [check, { data, isFetching, error }] = useLazySetupUpdateQuery()
  const [open, setOpen] = useStoreState(embedded)
  const [mode, setMode] = useStoreState<'now' | 'manual'>('now')
  const [releaseURL, setReleaseURL] = useStoreState('')
  const [file, setFile] = useStoreState<File | null>(null)
  const [uploadProgress, setUploadProgress] = useStoreState<number | null>(null)
  const [uploadError, setUploadError] = useStoreState('')
  const [browserDownloadNotice, setBrowserDownloadNotice] = useStoreState('')
  const [staged, setStaged] = useStoreState(false)
  const [busy, setBusy] = useStoreState(false)
  const [message, setMessage] = useStoreState('')
  const [confirmRestart, setConfirmRestart] = useStoreState(false)
  const [transaction, setTransaction] = useState<UpdateTransaction | null>(null)
  const updateAvailable = Boolean(data?.available)
  const checkUpdate = useCallback(
    async (force = false) => {
      try {
        // The response that performs a force refresh must be the one rendered
        // by this panel. A separate fetch followed by the cache-keyed query
        // could display the previous RTK Query result instead.
        await check(force ? { refresh: true } : undefined).unwrap()
      } catch {
        // The server retains the last known status; a temporary failure should
        // not turn into a client polling loop.
      }
    },
    [check]
  )
  const loadUpdateStatus = useCallback(async () => {
    try {
      const response = await fetch('/api/v1/update/status', {
        cache: 'no-store'
      })
      if (!response.ok) throw new Error('更新状态暂不可用。')
      const next = (await response.json()) as UpdateTransaction
      setTransaction(next.phase === 'idle' ? null : next)
      return next
    } catch {
      return null
    }
  }, [])
  useEffect(() => {
    if (embedded) return
    const closeWhenAnotherToolOpens = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== 'update') setOpen(false)
    }
    window.addEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
    return () =>
      window.removeEventListener('alx:top-tool-open', closeWhenAnotherToolOpens)
  }, [embedded, setOpen])
  const uploadInputID = useId()
  const {
    data: releaseData = [],
    error: releasesError,
    isFetching: releasesLoading
  } = useReleasesQuery(
    { app: 'alemonx', refresh: true, currentPlatform: true },
    { skip: !open || mode !== 'manual' }
  )
  const releases = releaseData as Release[]
  const selected = releases.find(item => item.url === releaseURL) ?? releases[0]

  // Read the server's cached status initially so the top-bar indicator can
  // render promptly. Opening either update panel then performs a forced check.
  useEffect(() => {
    void checkUpdate()
  }, [checkUpdate])
  useEffect(() => {
    const onUpdateChanged = (event: Event) => {
      const detail = (event as CustomEvent<{ type?: string }>).detail
      if (detail?.type === 'system.update.changed') void checkUpdate()
    }
    window.addEventListener('alx:unified-event', onUpdateChanged)
    return () =>
      window.removeEventListener('alx:unified-event', onUpdateChanged)
  }, [checkUpdate])
  useEffect(() => {
    if (open) {
      // Both the popover and the embedded settings panel must perform the
      // same explicit latest-version lookup when opened.
      void checkUpdate(true)
      void loadUpdateStatus()
    }
  }, [checkUpdate, loadUpdateStatus, open])

  const api = async (path: string, options: RequestInit) => {
    const response = await fetch(path, options)
    const result = (await response.json()) as {
      output?: string
      error?: string
    }
    if (!response.ok) throw new Error(result.error || '操作未完成。')
    return result
  }

  const reconnectAfterRestart = () => {
    const rollbackDeadline = Date.now() + 65_000
    const healthDeadline = Date.now() + 40_000
    const retry = () => {
      void loadUpdateStatus()
        .then(status => {
          if (status?.phase === 'healthy') {
            window.location.reload()
            return
          }
          if (status?.phase === 'rolled_back' || status?.phase === 'failed') {
            setBusy(false)
            setMessage(status.error || updatePhaseLabels[status.phase])
            return
          }
          if (Date.now() > healthDeadline) {
            setMessage('新版验证超时，正在恢复旧版本…')
          }
          if (Date.now() < rollbackDeadline) window.setTimeout(retry, 500)
          else {
            setBusy(false)
            setMessage(
              '未收到最终更新结果，请重新打开更新面板查看事务状态。'
            )
          }
        })
        .catch(() => {
          if (Date.now() < rollbackDeadline) window.setTimeout(retry, 500)
          else {
            setBusy(false)
            setMessage('无法确认新版是否已启动，请稍后重新检查。')
          }
        })
    }
    // Give the old server enough time to send the update response and exit;
    // Windows may then need several retries before its executable is unlocked.
    window.setTimeout(retry, 900)
  }

  const download = async () => {
    setBusy(true)
    setMessage('')
    try {
      const result = await api('/api/v1/update/download', { method: 'POST' })
      setMessage(result.output || '更新包已下载完成。')
      await checkUpdate(true)
      setConfirmRestart(true)
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '下载更新失败。')
    } finally {
      setBusy(false)
    }
  }

  const applyAndRestart = async () => {
    setBusy(true)
    setMessage('')
    try {
      await api('/api/v1/update/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ confirm: true })
      })
      setMessage('正在重启应用…')
      reconnectAfterRestart()
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : '更新重启失败。')
      setBusy(false)
    }
  }

  const uploadUpdateArchive = (
    candidate: File,
    onProgress: (percent: number) => void
  ): Promise<{ staged: boolean; version?: string }> =>
    new Promise((resolve, reject) => {
      const request = new XMLHttpRequest()
      request.open('POST', '/api/v1/update/load')
      request.upload.onprogress = event => {
        if (event.lengthComputable)
          onProgress(Math.round((event.loaded / event.total) * 100))
      }
      request.onload = () => {
        if (request.status >= 200 && request.status < 300) {
          try {
            resolve(JSON.parse(request.responseText) as { staged: boolean; version?: string })
          } catch {
            resolve({ staged: true })
          }
          return
        }
        try {
          reject(
            new Error(JSON.parse(request.responseText).error || '上传失败。')
          )
        } catch {
          reject(new Error('上传失败。'))
        }
      }
      request.onerror = () => reject(new Error('上传失败，请检查网络。'))
      const form = new FormData()
      form.append('package', candidate)
      if (selected?.tag) form.append('version', selected.tag)
      form.append('confirm', 'true')
      request.send(form)
    })

  const selectUpdateArchive = async (candidate: File | null) => {
    if (!candidate) return
    const validationError = updateArchiveValidationError(candidate)
    if (validationError) {
      setFile(null)
      setUploadProgress(null)
      setStaged(false)
      setUploadError(validationError)
      return
    }
    setFile(candidate)
    setUploadError('')
    setStaged(false)
    setUploadProgress(0)
    setMessage('')
    try {
      const result = await uploadUpdateArchive(candidate, setUploadProgress)
      setStaged(true)
      setUploadProgress(100)
      setMessage(
        result.version
          ? `已暂存 ${result.version}，点击「确认安装并重启」应用。`
          : '更新包已上传，点击「确认安装并重启」应用。'
      )
    } catch (reason) {
      setUploadProgress(null)
      setUploadError(reason instanceof Error ? reason.message : '上传失败。')
    }
  }

  const openPanel = () => {
    window.dispatchEvent(
      new CustomEvent('alx:top-tool-open', { detail: 'update' })
    )
    setOpen(true)
    setMode('now')
    setMessage('')
    setConfirmRestart(false)
    void checkUpdate(true)
    void loadUpdateStatus()
  }

  return (
    <div className={embedded ? 'settings-update-panel' : 'relative'}>
      {!embedded && (
        <button
        className={`inline-flex size-8 items-center justify-center rounded-md border transition disabled:cursor-wait disabled:opacity-70 ${
          updateAvailable
            ? 'border-brand-100 bg-brand-50 text-brand-600 hover:bg-brand-100'
            : 'border-transparent text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800'
        }`}
        onClick={openPanel}
        disabled={isFetching}
        aria-label="检查应用更新"
        title={
          isFetching
            ? '正在检查更新'
            : updateAvailable
              ? '发现可用更新'
              : '检查更新'
        }
      >
        {isFetching || updateAvailable ? (
          <RefreshCw className="size-4" />
        ) : (
          <Home className="size-4" />
        )}
        </button>
      )}
      {open && (
        <section
          className={
            embedded
              ? 'settings-panel-content grid gap-4'
              : 'topbar-popover absolute left-0 top-[calc(100%+8px)] z-50 grid w-80 gap-2.5 rounded-xl border border-slate-200 bg-white p-3 shadow-[0_18px_42px_rgb(28_26_23/0.13)]'
          }
          role={embedded ? undefined : 'dialog'}
          aria-label="应用更新"
        >
          <header className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2.5">
              <i className="inline-flex size-8 items-center justify-center rounded-lg bg-brand-50 text-brand-600">
                <RefreshCw className="size-4" />
              </i>
              <span className="grid gap-0.5">
                <strong className="text-sm text-ink-950">应用更新</strong>
                <small className="text-[11px] text-slate-400">
                  当前版本 {data?.current ?? '—'}
                </small>
              </span>
            </div>
            {!embedded && (
              <button
                className="topbar-popover-close"
                onClick={() => setOpen(false)}
                aria-label="关闭更新提示"
              >
                <X className="size-4" />
              </button>
            )}
          </header>
          <Tabs
            ariaLabel="更新方式"
            value={mode}
            onChange={nextMode => {
              setMode(nextMode)
              if (nextMode === 'manual') setReleaseURL('')
              if (nextMode === 'now') {
                setFile(null)
                setUploadProgress(null)
                setUploadError('')
                setStaged(false)
              }
            }}
            variant="segmented"
            className="grid grid-cols-2"
            items={[
              { id: 'now', label: '自动更新' },
              { id: 'manual', label: '手动安装' }
            ]}
          />
          {mode === 'now' && (
            <section className="grid gap-2.5">
              {isFetching ? (
                <p className="rounded-lg border border-slate-200 bg-slate-50 p-2.5 text-xs leading-5 text-slate-500">
                  正在检查更新…
                </p>
              ) : error ? (
                <p className="rounded-lg border border-slate-200 bg-slate-50 p-2.5 text-xs leading-5 text-slate-500">
                  暂时无法检查更新，请稍后重试。
                </p>
              ) : data?.available ? (
                <>
                  <div className="flex items-center gap-2.5 rounded-lg border border-brand-100 bg-brand-50 p-3">
                    <i className="inline-flex size-8 items-center justify-center rounded-md bg-white text-brand-600">
                      <Download className="size-4" />
                    </i>
                    <span className="grid gap-0.5">
                      <small className="text-[11px] text-brand-700/70">
                        发现新版本
                      </small>
                      <strong className="text-sm text-brand-700">
                        {data.latest}
                      </strong>
                    </span>
                  </div>
                  {data.platformMatched && data.integrityReady ? (
                    <>
                      <Button
                        className="inline-flex min-h-9 justify-self-end rounded-md bg-brand-600 px-3 text-xs font-semibold text-white transition hover:bg-brand-700 disabled:opacity-60"
                        disabled={busy}
                        onClick={() => {
                          setFile(null)
                          if (data.downloadReady) setConfirmRestart(true)
                          else void download()
                        }}
                      >
                        {busy
                          ? '正在下载…'
                          : data.downloadReady
                            ? '更新并重启'
                            : '下载更新'}
                      </Button>
                      {busy && !data.downloadReady && (
                        <DownloadProgress
                          label="正在下载并校验更新包"
                          detail="由 AlemonX 从发布源下载；完成校验后才会提示安装。"
                        />
                      )}
                    </>
                  ) : (
                    <p className="rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-xs leading-5 text-amber-800">
                      {data.platformMatched
                        ? data.integrityError
                          ? `暂时无法读取发布校验文件：${data.integrityError}`
                          : '该版本未提供校验文件，无法自动更新；请切换到「手动安装」。'
                        : '当前系统没有匹配的更新包，无法自动更新；请切换到「手动安装」。'}
                    </p>
                  )}
                </>
              ) : (
                <div className="flex items-center gap-2.5 rounded-lg border border-slate-200 bg-slate-50 p-3">
                  <i className="inline-flex size-8 items-center justify-center rounded-md bg-white text-slate-500">
                    <CheckCircle2 className="size-4" />
                  </i>
                  <span className="grid gap-0.5 leading-tight">
                    <small className="text-[11px] text-slate-500">
                      已是最新 · 最新版本 {data?.latest ?? data?.current ?? '—'}
                    </small>
                    <strong className="text-sm text-slate-700">
                      当前 {data?.current ?? '—'}
                    </strong>
                  </span>
                </div>
              )}
            </section>
          )}
          {mode === 'manual' && (
            <section className="grid gap-2.5">
              {releasesLoading ? (
                <small className="rounded-md bg-slate-50 p-2 text-[11px] leading-4 text-slate-500">
                  正在读取可用发布版本…
                </small>
              ) : releasesError ? (
                <small className="rounded-md border border-amber-200 bg-amber-50 p-2 text-[11px] leading-4 text-amber-800">
                  暂时无法读取 GitHub 发布列表；你仍可直接选择已下载的本地安装包。
                </small>
              ) : (
                <>
                  <label className="grid gap-1 text-[11px] font-semibold text-slate-500">
                    版本
                    <select
                      className="min-h-9 rounded-md border border-slate-200 bg-white px-2 text-xs font-normal text-slate-700"
                      value={releaseURL || selected?.url || ''}
                      onChange={event => setReleaseURL(event.target.value)}
                    >
                      {releases.map(item => (
                        <option key={item.url} value={item.url}>
                          {item.tag} · {item.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <div className="overflow-hidden rounded-lg border border-slate-200">
                    {selected?.assets.map(asset => (
                      <a
                        className="flex min-h-8 items-center gap-2 border-b border-slate-100 px-2.5 text-xs text-brand-600 last:border-0 hover:bg-brand-50"
                        key={asset.url}
                        href={asset.url}
                        target="_blank"
                        rel="noreferrer"
                        onClick={() =>
                          setBrowserDownloadNotice(
                            '下载已交给浏览器，请在浏览器下载栏查看实际进度；完成后回到这里选择 .zip 安装包。'
                          )
                        }
                      >
                        <Download className="size-3.5 shrink-0" />
                        <span className="min-w-0 truncate">{asset.name}</span>
                        <ExternalLink className="ml-auto size-3.5 shrink-0 text-slate-400" />
                      </a>
                    ))}
                  </div>
                  {browserDownloadNotice && (
                    <DownloadProgress
                      handoff
                      label="已开始浏览器下载"
                      detail={browserDownloadNotice}
                    />
                  )}
                </>
              )}
              <section
                className="grid gap-2.5"
                onDragOver={event => event.preventDefault()}
                onDrop={event => {
                  event.preventDefault()
                  if (uploadProgress !== null) return
                  void selectUpdateArchive(event.dataTransfer.files[0] ?? null)
                }}
              >
                <input
                  className="sr-only"
                  id={uploadInputID}
                  type="file"
                  accept=".zip,application/zip"
                  onChange={event => {
                    void selectUpdateArchive(event.target.files?.[0] ?? null)
                    event.currentTarget.value = ''
                  }}
                />
                <label
                  htmlFor={uploadInputID}
                  className={`flex items-center gap-2.5 rounded-lg border border-dashed p-3 text-left transition ${
                    uploadProgress !== null
                      ? 'cursor-default border-brand-300 bg-brand-50/60'
                      : 'cursor-pointer border-brand-200 bg-brand-50/40 hover:border-brand-600 hover:bg-brand-50'
                  }`}
                >
                  <i className="inline-flex size-8 shrink-0 items-center justify-center rounded-md bg-brand-50 text-brand-600">
                    {uploadProgress !== null ? (
                      <RefreshCw className="size-4 animate-spin" />
                    ) : file ? (
                      <FileArchive className="size-4" />
                    ) : (
                      <Upload className="size-4" />
                    )}
                  </i>
                  <span className="grid min-w-0 flex-1 gap-1">
                    <strong className="truncate text-xs text-slate-700">
                      {file ? file.name : '选择更新包'}
                    </strong>
                    <small className="text-[11px] leading-4 text-slate-500">
                      {uploadProgress !== null
                        ? staged
                          ? '已上传，等待确认安装。'
                          : `正在上传 ${uploadProgress}%…`
                        : '仅支持 GitHub Release 下载的 .zip，也可拖到这里。'}
                    </small>
                  </span>
                </label>
                {uploadProgress !== null && (
                  <div className="h-1.5 overflow-hidden rounded-full bg-slate-200">
                    <div
                      className="h-full rounded-full bg-brand-600 transition-[width] duration-200"
                      style={{ width: `${uploadProgress}%` }}
                    />
                  </div>
                )}
                {uploadError && (
                  <small className="rounded-md border border-amber-200 bg-amber-50 p-2 text-[11px] leading-4 text-amber-800">
                    {uploadError}
                  </small>
                )}
                <Button
                  disabled={!staged || busy}
                  onClick={() => setConfirmRestart(true)}
                >
                  {busy ? '正在重启…' : '确认安装并重启'}
                </Button>
                {message && (
                  <small className="rounded-md bg-slate-50 p-2 text-[11px] leading-4 text-slate-500">
                    {message}
                  </small>
                )}
              </section>
            </section>
          )}
          {mode === 'now' && message && (
            <small className="rounded-md bg-slate-50 p-2 text-[11px] leading-4 text-slate-500">
              {message}
            </small>
          )}
          {transaction && (
            <small className="rounded-md bg-slate-50 p-2 text-[11px] leading-4 text-slate-500">
              {updatePhaseLabels[transaction.phase] ?? '正在处理更新…'}
              {transaction.targetVersion
                ? ` 目标 ${transaction.targetVersion}`
                : ''}
              {transaction.error ? `：${transaction.error}` : ''}
              {transaction.pluginError
                ? ` 插件同步：${transaction.pluginError}`
                : ''}
            </small>
          )}
          {data?.releaseUrl && (
            <a
              className="inline-flex items-center gap-1 text-[11px] text-slate-500 hover:text-brand-600"
              href={data.releaseUrl}
              target="_blank"
              rel="noreferrer"
            >
              查看发布说明 <ExternalLink className="size-3" />
            </a>
          )}
        </section>
      )}
      <ConfirmDialog
        open={confirmRestart}
        title={file && staged ? '确认安装本地更新包' : '立即更新并重启'}
        subtitle={
          file && staged
            ? `${file.name} 已上传并通过校验，确认后将替换当前应用。`
            : '已下载的更新包保存在本机应用存储目录中。'
        }
        message="应用会替换为新版本并自动重启；浏览器会在重启后重新连接。"
        confirmLabel={file && staged ? '确认安装并重启' : '立即更新并重启'}
        busy={busy}
        onCancel={() => setConfirmRestart(false)}
        onConfirm={() => {
          setConfirmRestart(false)
          void applyAndRestart()
        }}
      />
    </div>
  )
}
