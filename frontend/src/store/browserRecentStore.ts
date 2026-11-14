import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { persistReducer } from 'redux-persist'
import { preferenceStorage } from '../lib/desktopPreferences'

const recentSiteLimit = 6

function recentSiteOrigin(address: string) {
  try {
    const url = new URL(address)
    if (
      !['http:', 'https:'].includes(url.protocol) ||
      url.username ||
      url.password ||
      [...url.searchParams.keys()].some(key => /token/i.test(key))
    ) {
      return null
    }
    // Store only the origin. Paths, queries and hashes may contain credentials.
    return url.origin
  } catch {
    return null
  }
}

type BrowserRecentState = { sites: string[] }

const slice = createSlice({
  name: 'browserRecent',
  initialState: { sites: [] } as BrowserRecentState,
  reducers: {
    recordBrowserSite(state, action: PayloadAction<string>) {
      const origin = recentSiteOrigin(action.payload)
      if (!origin || state.sites[0] === origin) return
      state.sites = [origin, ...state.sites.filter(site => site !== origin)].slice(
        0,
        recentSiteLimit
      )
    }
  }
})

export function recordBrowserSite(address: URL) {
  const origin = recentSiteOrigin(address.href)
  return origin ? slice.actions.recordBrowserSite(origin) : null
}

export const persistedBrowserRecent = persistReducer(
  {
    key: 'alemonx-browser-recent',
    version: 1,
    storage: {
      getItem: (key: string) => Promise.resolve(preferenceStorage.getItem(key)),
      setItem: (key: string, value: string) =>
        Promise.resolve(preferenceStorage.setItem(key, value)),
      removeItem: (key: string) =>
        Promise.resolve(preferenceStorage.removeItem(key))
    },
    whitelist: ['sites'],
    migrate: async state => {
      if (!state) return state
      const { sites } = state as typeof state & Partial<BrowserRecentState>
      return {
        ...state,
        sites: Array.isArray(sites)
          ? [...new Set(sites.filter(site =>
              typeof site === 'string' && recentSiteOrigin(site) === site
            ))].slice(0, recentSiteLimit)
          : []
      }
    }
  },
  slice.reducer
)
