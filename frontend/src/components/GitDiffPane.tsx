import { useEffect, useRef, type ReactNode } from 'react'
import { ArrowLeft, X } from 'lucide-react'
import { Button } from './Button'

export function GitDiffPane({
  title,
  path,
  inline,
  onClose,
  actions,
  children
}: {
  title: string
  path: string
  inline: boolean
  onClose: () => void
  actions?: ReactNode
  children: ReactNode
}) {
  const dismiss = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    dismiss.current?.focus({ preventScroll: true })
  }, [inline])

  return (
    <section
      aria-label={title}
      className="absolute inset-0 z-10 flex min-h-0 min-w-0 flex-col bg-(--theme-surface-panel)"
      onKeyDown={event => {
        if (event.key !== 'Escape') return
        event.stopPropagation()
        onClose()
      }}
    >
      <header className="grid shrink-0 gap-2 bg-(--theme-surface-raised) px-3 py-2">
        <div className="flex min-w-0 items-center justify-between gap-2">
          {inline && (
            <Button ref={dismiss} variant="ghost" className="shrink-0 gap-1.5" onClick={onClose}>
              <ArrowLeft className="size-4" aria-hidden="true" /> 返回
            </Button>
          )}
          <strong className="min-w-0 flex-1 truncate text-sm" title={path}>{path}</strong>
          {!inline && (
            <Button ref={dismiss} variant="icon" className="shrink-0" onClick={onClose} aria-label="关闭差异">
              <X className="size-4" aria-hidden="true" />
            </Button>
          )}
        </div>
        {actions}
      </header>
      <div className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain">{children}</div>
    </section>
  )
}
