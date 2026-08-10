import { useEffect, useState } from 'react'

/** Matches the tablet/iPad shell breakpoint in styles.css. */
export const PAD_BREAKPOINT = '(max-width: 1024px)'

export function isPadViewport(): boolean {
  return window.matchMedia(PAD_BREAKPOINT).matches
}

export function useIsPadViewport(): boolean {
  const [isPad, setIsPad] = useState(isPadViewport)
  useEffect(() => {
    const media = window.matchMedia(PAD_BREAKPOINT)
    const update = () => setIsPad(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  return isPad
}
