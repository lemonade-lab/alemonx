import { useEffect, useState } from 'react'
import { WORKBENCH_NAVIGATION_QUERY } from './viewportBreakpoints'

/** Matches the stacked project navigation layout in styles.css. */
export const WORKBENCH_NAVIGATION_BREAKPOINT = WORKBENCH_NAVIGATION_QUERY
/** @deprecated Prefer WORKBENCH_NAVIGATION_BREAKPOINT for clarity. */
export const MOBILE_BREAKPOINT = WORKBENCH_NAVIGATION_BREAKPOINT

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
