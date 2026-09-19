import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Modal } from './Modal'
import { Button } from './Button'

export function FormDialog({
  open,
  title,
  description,
  busy = false,
  onClose,
  children,
  footer
}: {
  open: boolean
  title: string
  description?: string
  busy?: boolean
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
}) {
  return (
    <Modal
      open={open}
      ariaLabel={title}
      onClose={busy ? undefined : onClose}
      className="p-3 sm:p-6"
    >
      <section className="flex max-h-[calc(100dvh-3rem)] w-full max-w-2xl min-w-0 flex-col overflow-hidden rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) text-(--theme-text-primary) shadow-xl">
        <header className="flex items-start justify-between gap-3 border-b border-(--theme-border-default) p-4">
          <div>
            <h2 className="text-base font-semibold">{title}</h2>
            {description && (
              <p className="mt-1 text-xs text-(--theme-text-secondary)">
                {description}
              </p>
            )}
          </div>
          <Button
            variant="icon"
            aria-label="关闭对话框"
            disabled={busy}
            onClick={onClose}
          >
            <X className="size-4" />
          </Button>
        </header>
        <div className="grid min-w-0 gap-3 overflow-y-auto p-4">{children}</div>
        {footer && (
          <footer className="flex shrink-0 flex-wrap justify-end gap-2 border-t border-(--theme-border-default) p-4">
            {footer}
          </footer>
        )}
      </section>
    </Modal>
  )
}
