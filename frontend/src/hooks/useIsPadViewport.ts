import { useEffect, useState } from 'react'
import {
  PHONE_VIEWPORT_QUERY,
  TABLET_WINDOW_QUERY
} from './viewportBreakpoints'

/** Matches the tablet/iPad shell breakpoint in styles.css. */
export const PAD_BREAKPOINT = TABLET_WINDOW_QUERY
export const PHONE_BREAKPOINT = PHONE_VIEWPORT_QUERY

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

export function useIsPhoneViewport(): boolean {
  const [isPhone, setIsPhone] = useState(() =>
    window.matchMedia(PHONE_BREAKPOINT).matches
  )
  useEffect(() => {
    const media = window.matchMedia(PHONE_BREAKPOINT)
    const update = () => setIsPhone(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])
  return isPhone
}
