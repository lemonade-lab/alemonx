import { useId, useRef, type ReactNode } from 'react'
import { SlidersHorizontal } from 'lucide-react'
import { useStoreState } from '../store/guideStore'
import { Button } from './Button'

export function WorkbenchTools({ children }: { children: ReactNode }) {
  const [open, setOpen] = useStoreState(false)
  const id = useId()
  const trigger = useRef<HTMLButtonElement>(null)
  return (
    <div className="contents">
      <button
        ref={trigger}
        type="button"
        className="workbench-tools-trigger secondary-button ml-auto shrink-0 gap-2"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen(value => !value)}
      >
        <SlidersHorizontal className="size-4" aria-hidden="true" />
        工具
      </button>
      <div
        id={id}
        role="region"
        aria-label="工作台工具"
        data-open={open}
        className="workbench-tools-actions"
        onKeyDown={event => {
          if (event.key === 'Escape' && !event.defaultPrevented && trigger.current?.getClientRects().length) {
            event.stopPropagation()
            setOpen(false)
            trigger.current?.focus()
          }
        }}
      >
        {children}
        <Button
          variant="ghost"
          className="workbench-tools-close ml-auto"
          onClick={() => {
            setOpen(false)
            trigger.current?.focus()
          }}
        >
          收起工具
        </Button>
      </div>
    </div>
  )
}
