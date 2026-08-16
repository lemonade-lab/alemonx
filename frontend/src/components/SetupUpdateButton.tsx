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
import { SettingsMessage } from './SettingsCard'

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
    <div className={embedded ? 'settings-update-panel grid gap-4' : 'relative'}>
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
      {embedded && open && (
        <header className="settings-page-head">
          <h2 className="settings-page-title">更新</h2>
          <p className="settings-page-desc">
            检查并安装 AlemonX 新版本。
          </p>
        </header>
      )}
      {open && (
        <section
          className={
            embedded
              ? 'settings-card grid gap-4'
              : 'topbar-popover absolute left-0 top-[calc(100%+8px)] z-50 grid w-80 gap-2.5 rounded-xl border border-slate-200 bg-white p-3 shadow-[0_18px_42px_rgb(28_26_23/0.13)]'
          }
          role={embedded ? undefined : 'dialog'}
          aria-label="应用更新"
        >
          <header className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2.5">
              <i className="settings-card-icon">
                <RefreshCw className="size-4" />
              </i>
              <strong className="settings-card-title">应用更新</strong>
            </div>
            <div className="settings-card-actions flex-wrap">
              <span className="settings-pill">当前 {data?.current ?? '—'}</span>
              {data?.available && (
                <span className="settings-pill is-success">
                  最新 {data.latest}
                </span>
              )}
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
                <SettingsMessage>正在检查更新…</SettingsMessage>
              ) : error ? (
                <SettingsMessage tone="error">
                  暂时无法检查更新，请稍后重试。
                </SettingsMessage>
              ) : data?.available ? (
                <>
                  <p className="m-0 text-xs leading-5 text-(--theme-text-secondary)">
                    发现新版本，可立即更新。
                  </p>
                  {data.platformMatched && data.integrityReady ? (
                    <>
                      <div className="settings-card-actions settings-card-actions-end">
                        <Button
                          variant="primary"
                          className="gap-1.5"
                          disabled={busy}
                          onClick={() => {
                            setFile(null)
                            if (data.downloadReady) setConfirmRestart(true)
                            else void download()
                          }}
                        >
                          {busy ? (
                            <RefreshCw className="size-3.5 animate-spin" />
                          ) : (
                            <Download className="size-3.5" />
                          )}
                          {busy
                            ? '正在下载…'
                            : data.downloadReady
                              ? '更新并重启'
                              : '下载更新'}
                        </Button>
                      </div>
                      {busy && !data.downloadReady && (
                        <DownloadProgress
                          label="正在下载并校验更新包"
                          detail="由 AlemonX 从发布源下载；完成校验后才会提示安装。"
                        />
                      )}
                    </>
                  ) : (
                    <SettingsMessage tone="error">
                      {data.platformMatched
                        ? data.integrityError
                          ? `暂时无法读取发布校验文件：${data.integrityError}`
                          : '该版本未提供校验文件，无法自动更新；请切换到「手动安装」。'
                        : '当前系统没有匹配的更新包，无法自动更新；请切换到「手动安装」。'}
                    </SettingsMessage>
                  )}
                </>
              ) : (
                <p className="m-0 flex items-center gap-2 text-xs text-(--theme-text-muted)">
                  <CheckCircle2 className="size-4 shrink-0 text-(--theme-success)" />
                  已是最新
                </p>
              )}
            </section>
          )}
          {mode === 'manual' && (
            <section className="grid gap-2.5">
              {releasesLoading ? (
                <SettingsMessage>
                  正在读取可用发布版本…
                </SettingsMessage>
              ) : releasesError ? (
                <SettingsMessage tone="error">
                  暂时无法读取 GitHub 发布列表；你仍可直接选择已下载的本地安装包。
                </SettingsMessage>
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
                  <SettingsMessage tone="error">
                    {uploadError}
                  </SettingsMessage>
                )}
                <Button
                  disabled={!staged || busy}
                  onClick={() => setConfirmRestart(true)}
                >
                  {busy ? '正在重启…' : '确认安装并重启'}
                </Button>
                {message && (
                  <SettingsMessage>
                    {message}
                  </SettingsMessage>
                )}
              </section>
            </section>
          )}
          {mode === 'now' && message && (
            <SettingsMessage>
              {message}
            </SettingsMessage>
          )}
          {transaction && (
            <SettingsMessage>
              {updatePhaseLabels[transaction.phase] ?? '正在处理更新…'}
              {transaction.targetVersion
                ? ` 目标 ${transaction.targetVersion}`
                : ''}
              {transaction.error ? `：${transaction.error}` : ''}
              {transaction.pluginError
                ? ` 插件同步：${transaction.pluginError}`
                : ''}
            </SettingsMessage>
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
