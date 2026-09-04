import { createPortal } from 'react-dom'
import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'

// Desktop windows and the dock use the lower application layer. Any modal
// rendered through this component is a system-level interaction barrier and
// therefore always sits above every application window.
export const GLOBAL_MODAL_Z_INDEX = 2_147_483_000
let openModalCount = 0
let previousBodyOverflow = ''
let previousDocumentOverflow = ''

type Props = {
  open: boolean
  children: ReactNode
  className?: string
  zIndex?: number
  onClose?: () => void
  onBackdropClick?: () => void
  ariaLabel?: string
  /** Floating desktop windows use this portal but are not blocking dialogs. */
  trapFocus?: boolean
  lockScroll?: boolean
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
  ariaLabel,
  trapFocus = true,
  lockScroll = true
}: Props) {
  const overlayRef = useRef<HTMLDivElement>(null)
  const returnFocusRef = useRef<HTMLElement | null>(null)
  useEffect(() => {
    if (!open) return
    returnFocusRef.current = document.activeElement as HTMLElement | null
    if (lockScroll && openModalCount++ === 0) {
      previousBodyOverflow = document.body.style.overflow
      previousDocumentOverflow = document.documentElement.style.overflow
      document.body.style.overflow = 'hidden'
      document.documentElement.style.overflow = 'hidden'
    }
    const focusFirst = () => {
      const focusable = overlayRef.current?.querySelector<HTMLElement>(
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      ;(focusable ?? overlayRef.current)?.focus()
    }
    const frame = trapFocus ? window.requestAnimationFrame(focusFirst) : 0
    const onKeyDown = (event: KeyboardEvent) => {
      const dismiss = onClose ?? onBackdropClick
      if (event.key === 'Escape' && dismiss) {
        event.preventDefault()
        dismiss()
        return
      }
      if (!trapFocus || event.key !== 'Tab' || !overlayRef.current) return
      const focusable = Array.from(
        overlayRef.current.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
        )
      )
      if (!focusable.length) {
        event.preventDefault()
        overlayRef.current.focus()
        return
      }
      const first = focusable[0]
      const last = focusable.at(-1)!
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      if (trapFocus) window.cancelAnimationFrame(frame)
      document.removeEventListener('keydown', onKeyDown)
      if (lockScroll && --openModalCount === 0) {
        document.body.style.overflow = previousBodyOverflow
        document.documentElement.style.overflow = previousDocumentOverflow
      }
      if (trapFocus) returnFocusRef.current?.focus()
    }
  }, [lockScroll, onBackdropClick, onClose, open, trapFocus])
  if (!open) return null
  return createPortal(
    <div
      ref={overlayRef}
      className={`fixed inset-0 flex items-center justify-center bg-slate-950/30 p-4 ${className}`}
      style={{ zIndex }}
      role={ariaLabel ? 'dialog' : 'presentation'}
      aria-modal={ariaLabel ? 'true' : undefined}
      aria-label={ariaLabel}
      tabIndex={-1}
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
