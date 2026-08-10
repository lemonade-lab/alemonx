import { useEffect, useState } from 'react'

/** Matches the narrow stacked-sidebar layout breakpoint in styles.css. */
export const MOBILE_BREAKPOINT = '(max-width: 940px)'

export function useIsMobileViewport(): boolean {
  const [isMobile, setIsMobile] = useState(
    () => window.matchMedia(MOBILE_BREAKPOINT).matches
  )
  useEffect(() => {
    const media = window.matchMedia(MOBILE_BREAKPOINT)
    const update = () => setIsMobile(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  return isMobile
}
