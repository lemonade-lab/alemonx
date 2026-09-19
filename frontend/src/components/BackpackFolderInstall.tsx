import { useEffect, useRef } from 'react'
import { Check, FolderOpen, LoaderCircle, Upload, X } from 'lucide-react'
import { Modal } from './Modal'
import { Button } from './Button'
import { useStoreState } from '../store/guideStore'
import { useInstallRobotPackageFolderMutation } from '../store/workspaceApi'
import {
  filesFromFolderDrop,
  filesFromFolderPicker,
  validatePackageFolder,
  type PackageFolderFile
} from '../lib/packageFolder'

export function BackpackFolderInstall({
  root,
  disabled,
  onInstalled
}: {
  root: string
  disabled?: boolean
  onInstalled: () => void
}) {
  const [open, setOpen] = useStoreState(false)
  const [reading, setReading] = useStoreState(false)
  const [dragging, setDragging] = useStoreState(false)
  const [summary, setSummary] = useStoreState<{
    name: string
    count: number
  } | null>(null)
  const [error, setError] = useStoreState('')
  const [success, setSuccess] = useStoreState(false)
  const files = useRef<PackageFolderFile[]>([])
  const input = useRef<HTMLInputElement>(null)
  const generation = useRef(0)
  const [install, { isLoading }] = useInstallRobotPackageFolderMutation()
  useEffect(
    () => () => {
      generation.current++
    },
    []
  )
  const close = () => {
    if (isLoading) return
    generation.current++
    files.current = []
    setSummary(null)
    setError('')
    setSuccess(false)
    setReading(false)
    setDragging(false)
    setOpen(false)
  }
  const choose = async (load: () => Promise<PackageFolderFile[]>) => {
    if (reading || isLoading) return
    const request = ++generation.current
    setReading(true)
    setError('')
    setSummary(null)
    files.current = []
    try {
      const next = await load()
      const name = await validatePackageFolder(next)
      if (request !== generation.current) return
      files.current = next
      setSummary({ name, count: next.length })
    } catch (reason) {
      if (request === generation.current)
        setError(
          reason instanceof Error
            ? reason.message
            : '无法读取文件夹，请重新选择。'
        )
    } finally {
      if (request === generation.current) setReading(false)
    }
  }
  const submit = async () => {
    if (!summary || isLoading) return
    const request = generation.current
    setError('')
    try {
      await install({ root, files: files.current }).unwrap()
      if (request !== generation.current) return
      files.current = []
      setSuccess(true)
      onInstalled()
    } catch (reason) {
      if (request !== generation.current) return
      const payload =
        reason && typeof reason === 'object' && 'data' in reason
          ? reason.data
          : null
      setError(
        payload &&
          typeof payload === 'object' &&
          'error' in payload &&
          typeof payload.error === 'string'
          ? payload.error
          : '安装失败，请检查连接后重试。'
      )
    }
  }
  return (
    <>
      <Button
        disabled={disabled}
        className="gap-1.5"
        onClick={() => setOpen(true)}
      >
        <FolderOpen className="size-4" />
        本地安装
      </Button>
      <Modal open={open} onClose={close} ariaLabel="安装插件" className="p-4">
        <section className="grid max-h-[90dvh] w-full max-w-md gap-4 overflow-auto rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-5 text-(--theme-text-primary)">
          <header className="flex items-center justify-between gap-3">
            <h2 className="m-0 text-lg font-semibold">安装插件</h2>
            <Button
              variant="icon"
              disabled={isLoading}
              onClick={close}
              aria-label="关闭安装对话框"
            >
              <X className="size-4" />
            </Button>
          </header>
          {success ? (
            <p
              role="status"
              className="m-0 flex items-center gap-3 py-4 text-sm"
            >
              <Check
                aria-hidden="true"
                className="size-6 shrink-0 text-(--theme-accent-text)"
              />
              <span className="min-w-0 break-words">
                {summary?.name} · 已安装
              </span>
            </p>
          ) : (
            <>
              <input
                type="file"
                multiple
                aria-label="选择插件文件夹"
                className="hidden"
                ref={node => {
                  input.current = node
                  node?.setAttribute('webkitdirectory', '')
                }}
                onChange={event => {
                  const selected = event.currentTarget.files
                  if (selected?.length) {
                    void choose(async () => filesFromFolderPicker(selected))
                  }
                  event.currentTarget.value = ''
                }}
              />
              <button
                type="button"
                aria-label={summary ? '重新选择文件夹' : '选择文件夹'}
                disabled={reading || isLoading}
                className={`flex min-w-0 min-h-44 flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed p-5 text-center transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--theme-accent) disabled:cursor-wait ${dragging ? 'border-(--theme-accent) bg-(--theme-accent-soft)' : 'border-(--theme-border-strong) hover:border-(--theme-accent) hover:bg-(--theme-surface-hover)'}`}
                onClick={() => input.current?.click()}
                onDragOver={event => {
                  event.preventDefault()
                  if (!reading && !isLoading) setDragging(true)
                }}
                onDragLeave={() => setDragging(false)}
                onDrop={event => {
                  event.preventDefault()
                  setDragging(false)
                  const selected = event.dataTransfer.items
                  void choose(() => filesFromFolderDrop(selected))
                }}
              >
                {reading || isLoading ? (
                  <LoaderCircle
                    aria-hidden="true"
                    className="size-7 animate-spin text-(--theme-accent-text)"
                  />
                ) : summary ? (
                  <FolderOpen
                    aria-hidden="true"
                    className="size-7 text-(--theme-accent-text)"
                  />
                ) : (
                  <Upload
                    aria-hidden="true"
                    className="size-7 text-(--theme-accent-text)"
                  />
                )}
                <span
                  role="status"
                  className="max-w-full break-words text-sm font-semibold"
                >
                  {reading
                    ? '正在读取…'
                    : summary
                      ? `${summary.name} · ${summary.count} 个文件`
                      : dragging
                        ? '松开以选择'
                        : '拖入文件夹'}
                </span>
                <span className="text-sm text-(--theme-accent-text)">
                  {isLoading
                    ? '正在安装…'
                    : reading
                      ? ' '
                      : summary
                        ? '重新选择'
                        : '选择文件夹'}
                </span>
              </button>
              {error && (
                <p
                  role="alert"
                  className="m-0 break-words text-sm text-(--theme-danger-text)"
                >
                  {error}
                </p>
              )}
            </>
          )}
          <footer className="flex justify-end gap-2">
            <Button onClick={close} disabled={isLoading}>
              {success ? '完成' : '取消'}
            </Button>
            {!success && (
              <Button
                variant="primary"
                disabled={!summary || reading}
                loading={isLoading}
                loadingLabel="正在安装…"
                onClick={() => void submit()}
              >
                安装
              </Button>
            )}
          </footer>
        </section>
      </Modal>
    </>
  )
}
