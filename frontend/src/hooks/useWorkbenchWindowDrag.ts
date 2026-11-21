import { useRef, type MouseEvent } from 'react'
import { bridge } from '../components/nativeWindow'

export function useWorkbenchWindowDrag(enabled: boolean) {
  const dragReady = useRef(false)

  if (!bridge || !enabled) return undefined

  const resetDrag = () => {
    dragReady.current = false
  }

  const startDrag = (event: MouseEvent<HTMLElement>) => {
    if (!dragReady.current) return
    resetDrag()
    if (event.buttons === 1) bridge?.postMessage('wails:drag')
  }

  return {
    onMouseDown(event: MouseEvent<HTMLElement>) {
      const interactive = event.target instanceof Element && event.target.closest(
        'button, a, input, select, textarea, [contenteditable], [role="button"]'
      )
      dragReady.current = event.button === 0 && event.detail === 1 && !interactive
    },
    onMouseUp: resetDrag,
    onMouseLeave: startDrag,
    onMouseMove: startDrag
  }
}
