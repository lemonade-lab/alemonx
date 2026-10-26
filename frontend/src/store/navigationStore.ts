import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { persistReducer } from 'redux-persist'

// Only navigation preferences belong here. Form contents, credentials, pending
// actions and fetched service data remain in their existing transient stores.
export type NavigationArea =
  | 'backpack.package'
  | 'backpack.tab'
  | 'backpack.filter'
  | 'market.resource'
  | 'market.tab'
  | 'market.page'
  | 'settings.section'
  | 'ops.section'
  | 'git.tab'
  | 'environment.section'
  | 'system-plugins.section'
  | 'env.editor'
  | 'npmrc.editor'
  | 'robot.platform'
  | 'update.mode'
  | 'sidebar.visited'
export type NavigationValue = string | number
type NavigationState = { values: Record<string, NavigationValue> }
const areas: NavigationArea[] = [
  'backpack.package',
  'backpack.tab',
  'backpack.filter',
  'market.resource',
  'market.tab',
  'market.page',
  'settings.section',
  'ops.section',
  'git.tab',
  'environment.section',
  'system-plugins.section',
  'env.editor',
  'npmrc.editor',
  'robot.platform',
  'update.mode',
  'sidebar.visited'
]
const maxEntries = 300
export const navigationKey = (area: NavigationArea, scope: readonly string[]) =>
  JSON.stringify([
    area,
    ...scope.map(value => {
      if (/^[a-zA-Z]:[\\/]/.test(value) || /^(?:\\\\|\/\/)/.test(value)) {
        return value.replaceAll('\\', '/').replace(/\/+$/, '').toLowerCase()
      }
      return value.startsWith('/') && value.length > 1
        ? value.replace(/\/+$/, '')
        : value
    })
  ])

function validEntry(key: string, value: unknown): value is NavigationValue {
  try {
    const parts: unknown = JSON.parse(key)
    return (
      Array.isArray(parts) &&
      parts.length <= 5 &&
      areas.includes(parts[0] as NavigationArea) &&
      parts.every(part => typeof part === 'string' && part.length <= 4096) &&
      (typeof value === 'string'
        ? value.length <= 1024
        : typeof value === 'number' &&
          Number.isInteger(value) &&
          value >= 1 &&
          value <= 100000)
    )
  } catch {
    return false
  }
}
export function migrateNavigationValues(
  value: unknown
): NavigationState['values'] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return Object.fromEntries(
    Object.entries(value)
      .filter(([key, entry]) => validEntry(key, entry))
      .slice(-maxEntries)
  )
}
const slice = createSlice({
  name: 'navigation',
  initialState: { values: {} } as NavigationState,
  reducers: {
    setNavigationValue(
      state,
      action: PayloadAction<{ key: string; value: NavigationValue }>
    ) {
      const { key, value } = action.payload
      if (!validEntry(key, value) || state.values[key] === value) return
      delete state.values[key]
      state.values[key] = value
      const oldest = Object.keys(state.values)[0]
      if (Object.keys(state.values).length > maxEntries)
        delete state.values[oldest]
    }
  }
})
export const { setNavigationValue } = slice.actions
export const persistedNavigation = persistReducer(
  {
    key: 'alemonx-navigation',
    version: 1,
    storage: {
      getItem: (key: string) => Promise.resolve(localStorage.getItem(key)),
      setItem: (key: string, value: string) =>
        Promise.resolve(localStorage.setItem(key, value)),
      removeItem: (key: string) => Promise.resolve(localStorage.removeItem(key))
    },
    whitelist: ['values'],
    migrate: async state =>
      state
        ? {
            ...state,
            values: migrateNavigationValues(
              (state as typeof state & Partial<NavigationState>).values
            )
          }
        : state
  },
  slice.reducer
)
