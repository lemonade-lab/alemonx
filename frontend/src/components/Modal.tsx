import { createPortal } from 'react-dom'
import type { ReactNode } from 'react'

// Desktop windows and the dock use the lower application layer. Any modal
// rendered through this component is a system-level interaction barrier and
// therefore always sits above every application window.
export const GLOBAL_MODAL_Z_INDEX = 2_147_483_000

type Props = {
  open: boolean
  children: ReactNode
  className?: string
  zIndex?: number
  onClose?: () => void
  onBackdropClick?: () => void
  ariaLabel?: string
}

// Global modal surface. Rendering through createPortal into document.body
// detaches the overlay from any transform/filter/overflow ancestor, so a
// "fixed inset-0" backdrop always tracks the viewport instead of being trapped
// inside a composited container. All app dialogs should go through this.
export function Modal({
  open,
  children,
  className = '',
  zIndex = GLOBAL_MODAL_Z_INDEX,
  onClose,
  onBackdropClick,
  ariaLabel
}: Props) {
  if (!open) return null
  return createPortal(
    <div
      className={`fixed inset-0 flex items-center justify-center bg-slate-950/30 p-4 ${className}`}
      style={{ zIndex }}
      role={ariaLabel ? 'dialog' : 'presentation'}
      aria-modal={ariaLabel ? 'true' : undefined}
      aria-label={ariaLabel}
      // Only close when the backdrop itself is clicked; a mousedown on the
      // dialog content must not bubble up and dismiss the modal.
      onMouseDown={
        onClose
          ? event => {
              if (event.target === event.currentTarget) onClose()
            }
          : undefined
      }
      onClick={
        onBackdropClick
          ? event => {
              if (event.target === event.currentTarget) onBackdropClick()
            }
          : undefined
      }
    >
      {children}
    </div>,
    document.body
  )
}
