import { useCallback } from 'react'
import { useSelector } from 'react-redux'
import { store, type RootState } from '../store/guideStore'
import {
  navigationKey,
  setNavigationValue,
  type NavigationArea,
  type NavigationValue
} from '../store/navigationStore'

export function useNavigationState<T extends NavigationValue>(
  area: NavigationArea,
  scope: readonly string[],
  initial: T,
  allowed?: readonly T[]
): [T, (next: T | ((previous: T) => T)) => void] {
  const key = navigationKey(area, scope)
  const allowedKey = JSON.stringify(allowed ?? null)
  const valid = useCallback(
    (value: NavigationValue | undefined): T =>
      typeof value === typeof initial &&
      (allowedKey === 'null' ||
        (JSON.parse(allowedKey) as T[]).includes(value as T))
        ? (value as T)
        : initial,
    [allowedKey, initial]
  )
  const value = useSelector((state: RootState) =>
    valid(state.navigation.values[key])
  )
  const setValue = useCallback(
    (next: T | ((previous: T) => T)) => {
      const previous = valid(store.getState().navigation.values[key])
      store.dispatch(
        setNavigationValue({
          key,
          value: typeof next === 'function' ? next(previous) : next
        })
      )
    },
    [key, valid]
  )
  return [value, setValue]
}
