import { useCallback, useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, CheckCircle2, Globe, Info, X, XCircle } from 'lucide-react'
import { Button } from './Button'
import { ConfirmDialog } from './ConfirmDialog'
import { DesktopWindow } from './DesktopWindow'
import { GLOBAL_MODAL_Z_INDEX, Modal } from './Modal'

export type HostWebview = {
  id: string
  title: string
  src: string
  kind: 'url' | 'static'
  width: number
  height: number
  left: number
  top: number
  storageKey?: string
  minimized: boolean
  zIndex: number
}

export type HostUiRequest = {
  requestId: string
  kind: 'alert' | 'message' | 'modal' | 'notification'
  source?: Window
  busy?: boolean
  title?: string
  message?: string
  confirmText?: string
  cancelText?: string
  type?: 'info' | 'success' | 'warning' | 'error'
  duration?: number
}

export type HostToast = {
  id: string
  kind: 'message' | 'notification'
  type: 'info' | 'success' | 'warning' | 'error'
  title?: string
  message?: string
  duration: number
}

const toastIcon: Record<HostToast['type'], ReactNode> = {
  info: <Info className="size-3" />,
  success: <CheckCircle2 className="size-3" />,
  warning: <AlertTriangle className="size-3" />,
  error: <XCircle className="size-3" />
}

const toastIconClass: Record<HostToast['type'], string> = {
  info: 'bg-sky-100 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300',
  success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
  error: 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-300'
}

/**
 * A host-managed WebView window opened by a system plugin. The plugin never
 * re-implements window chrome: the host provides the floating-window shell
 * and simply embeds the validated src (static plugin resource or http(s) URL).
 */
export function PluginWebviewWindow({
  webview,
  onClose,
  onMinimize,
  onActivate,
  onFrame
}: {
  webview: HostWebview
  onClose: () => void
  onMinimize: () => void
  onActivate: () => void
  onFrame?: (id: string, win: Window | null) => void
}) {
  const frameRef = useRef<HTMLIFrameElement>(null)
  const registerFrame = useCallback(() => {
    onFrame?.(webview.id, frameRef.current?.contentWindow ?? null)
  }, [onFrame, webview.id])
  useEffect(() => {
    registerFrame()
    return () => onFrame?.(webview.id, null)
  }, [onFrame, registerFrame, webview.id])
  return (
    <DesktopWindow
      id={`plugin-webview-${webview.id}`}
      open
      minimized={webview.minimized}
      title={webview.title}
      subtitle={webview.src}
      icon={
        <Globe className="size-4 shrink-0 text-brand-600 dark:text-brand-200" />
      }
      onClose={onClose}
      onMinimize={onMinimize}
      zIndex={webview.zIndex}
      onActivate={onActivate}
      initialPosition={{ left: webview.left, top: webview.top }}
      storageKey={webview.storageKey}
      width={webview.width}
      height={webview.height}
    >
      <div className="min-h-0 flex-1 bg-white dark:bg-slate-900">
        <iframe
          ref={frameRef}
          className="h-full w-full border-0"
          src={webview.src}
          title={webview.title}
          referrerPolicy="no-referrer"
          onLoad={registerFrame}
        />
      </div>
    </DesktopWindow>
  )
}

/** Host-owned alert dialog: one confirm button, consistent with the app UI. */
export function HostPluginAlert({
  open,
  title,
  message,
  confirmText,
  onClose
}: {
  open: boolean
  title: string
  message: string
  confirmText?: string
  onClose: () => void
}) {
  useEscapeToClose(open, onClose)
  if (!open) return null
  return (
    <Modal
      open
      className="bg-slate-950/25 p-6"
      onClose={onClose}
      ariaLabel={title}
    >
      <section
        className="grid w-full max-w-md gap-4 rounded-xl border border-slate-200 bg-white p-4.5 shadow-[0_20px_58px_rgb(28_26_23/0.22)] dark:border-slate-700 dark:bg-slate-900"
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        onMouseDown={event => event.stopPropagation()}
      >
        <header className="flex items-center gap-2">
          <i className="inline-flex size-8.5 items-center justify-center rounded-lg bg-sky-100 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300">
            <Info className="size-4.25" />
          </i>
          <strong className="text-sm text-ink-950 dark:text-slate-100">
            {title}
          </strong>
        </header>
        <p className="m-0 whitespace-pre-line text-xs leading-5 text-slate-500 dark:text-slate-400">
          {message}
        </p>
        <footer className="flex justify-end">
          <Button autoFocus onClick={onClose}>
            {confirmText || '确定'}
          </Button>
        </footer>
      </section>
    </Modal>
  )
}

/** Host-owned confirm modal (rendered through the shared ConfirmDialog). */
export function HostPluginModal({
  request,
  busy = false,
  onConfirm,
  onCancel
}: {
  request: HostUiRequest | null
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  useEscapeToClose(request !== null && !busy, onCancel)
  useFocusDialog(request !== null)
  const handleCancel = busy ? () => undefined : onCancel
  const handleConfirm = busy ? () => undefined : onConfirm
  return (
    <ConfirmDialog
      open={request !== null}
      title={request?.title || '确认'}
      message={request?.message || ''}
      confirmLabel={request?.confirmText}
      cancelLabel={request?.cancelText}
      busy={busy}
      onConfirm={handleConfirm}
      onCancel={handleCancel}
    />
  )
}

function useEscapeToClose(active: boolean, onClose: () => void) {
  useEffect(() => {
    if (!active) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      event.preventDefault()
      onClose()
    }
    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [active, onClose])
}

// Moves focus into the topmost host dialog so keyboard interaction (including
// Escape) targets the dialog instead of the plugin iframe behind it.
function useFocusDialog(active: boolean) {
  useEffect(() => {
    if (!active) return
    const frame = window.requestAnimationFrame(() => {
      const dialogs = document.querySelectorAll<HTMLElement>(
        '[role="dialog"][aria-modal="true"]'
      )
      const dialog = dialogs[dialogs.length - 1]
      dialog?.querySelector<HTMLButtonElement>('button')?.focus()
    })
    return () => window.cancelAnimationFrame(frame)
  }, [active])
}

/**
 * Host-owned transient messages and notifications. Messages stack at the top
 * centre; notifications stack bottom-right, matching typical desktop shells.
 */
export function HostUiToasts({
  toasts,
  onDismiss
}: {
  toasts: HostToast[]
  onDismiss: (id: string) => void
}) {
  const messages = toasts.filter(toast => toast.kind === 'message')
  const notifications = toasts.filter(toast => toast.kind === 'notification')
  return createPortal(
    <>
      <div
        className="pointer-events-none fixed inset-x-0 top-4 grid justify-items-center gap-2 px-4"
        style={{ zIndex: GLOBAL_MODAL_Z_INDEX - 1 }}
        role="status"
        aria-live="polite"
      >
        {messages.map(toast => (
          <HostToastCard key={toast.id} toast={toast} onDismiss={onDismiss} />
        ))}
      </div>
      <div
        className="pointer-events-none fixed bottom-4 right-4 grid w-80 max-w-[calc(100vw-2rem)] gap-2"
        style={{ zIndex: GLOBAL_MODAL_Z_INDEX - 1 }}
        role="status"
        aria-live="polite"
      >
        {notifications.map(toast => (
          <HostToastCard key={toast.id} toast={toast} onDismiss={onDismiss} />
        ))}
      </div>
    </>,
    document.body
  )
}

function HostToastCard({
  toast,
  onDismiss
}: {
  toast: HostToast
  onDismiss: (id: string) => void
}) {
  useEffect(() => {
    const timer = window.setTimeout(() => onDismiss(toast.id), toast.duration)
    return () => window.clearTimeout(timer)
  }, [onDismiss, toast.duration, toast.id])
  return (
    <div className="pointer-events-auto flex w-full max-w-md items-start gap-2 rounded-xl border border-slate-200 bg-white/95 px-3 py-2.5 shadow-lg dark:border-slate-700 dark:bg-slate-900/95">
      <i
        className={`mt-0.5 inline-flex size-5 shrink-0 items-center justify-center rounded-full ${toastIconClass[toast.type]}`}
      >
        {toastIcon[toast.type]}
      </i>
      <div className="grid min-w-0 flex-1 gap-0.5">
        {toast.title && (
          <strong className="truncate text-xs font-semibold text-slate-900 dark:text-slate-100">
            {toast.title}
          </strong>
        )}
        {toast.message && (
          <p className="m-0 whitespace-pre-line text-xs leading-5 text-slate-500 dark:text-slate-400">
            {toast.message}
          </p>
        )}
      </div>
      <button
        type="button"
        className="rounded-md p-1 text-slate-400 transition hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
        onClick={() => onDismiss(toast.id)}
        aria-label="关闭提示"
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
