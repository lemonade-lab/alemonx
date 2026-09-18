import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { persistReducer } from 'redux-persist'
import { migrateDataTab, type DataTab } from '../lib/dataNavigation'
import {
  migrateConnections,
  type ConnectionProfile
} from '../lib/connectModels'

type DataPreferences = {
  tab: DataTab
  sqlitePaths: string[]
  sqliteNames: Record<string, string>
  connections: ConnectionProfile[]
}
const initialState: DataPreferences = {
  tab: 'redis-browser',
  sqlitePaths: [],
  sqliteNames: {},
  connections: []
}
const slice = createSlice({
  name: 'dataPreferences',
  initialState,
  reducers: {
    saveConnection(state, action: PayloadAction<ConnectionProfile>) {
      state.connections = migrateConnections(
        [
          action.payload,
          ...state.connections.filter(c => c.id !== action.payload.id)
        ],
        [],
        {}
      ).slice(0, 50)
    },
    forgetConnection(state, action: PayloadAction<string>) {
      state.connections = state.connections.filter(c => c.id !== action.payload)
    },
    setDataTab(state, action: PayloadAction<DataPreferences['tab']>) {
      state.tab = action.payload
    },
    rememberSQLite(state, action: PayloadAction<string>) {
      state.sqlitePaths = [
        action.payload,
        ...state.sqlitePaths.filter(p => p !== action.payload)
      ].slice(0, 12)
      state.sqliteNames = Object.fromEntries(
        Object.entries(state.sqliteNames).filter(([path]) =>
          state.sqlitePaths.includes(path)
        )
      )
    },
    forgetSQLite(state, action: PayloadAction<string>) {
      state.sqlitePaths = state.sqlitePaths.filter(p => p !== action.payload)
      delete state.sqliteNames[action.payload]
    },
    nameSQLite(state, action: PayloadAction<{ path: string; name: string }>) {
      if (state.sqlitePaths.includes(action.payload.path))
        state.sqliteNames[action.payload.path] = action.payload.name
          .trim()
          .slice(0, 80)
    }
  }
})
export const {
  setDataTab,
  rememberSQLite,
  forgetSQLite,
  nameSQLite,
  saveConnection,
  forgetConnection
} = slice.actions
export const persistedDataPreferences = persistReducer(
  {
    key: 'alemonx-data',
    version: 5,
    storage: {
      getItem: (key: string) => Promise.resolve(localStorage.getItem(key)),
      setItem: (key: string, value: string) =>
        Promise.resolve(localStorage.setItem(key, value)),
      removeItem: (key: string) => Promise.resolve(localStorage.removeItem(key))
    },
    whitelist: ['tab', 'sqlitePaths', 'sqliteNames', 'connections'],
    migrate: async state => {
      if (!state) return state
      const data = state as typeof state & Partial<DataPreferences>
      const sqlitePaths = Array.isArray(data.sqlitePaths)
        ? data.sqlitePaths.filter(p => typeof p === 'string').slice(0, 12)
        : []
      return {
        ...state,
        tab: migrateDataTab(data.tab),
        connections: migrateConnections(
          data.connections,
          sqlitePaths,
          data.sqliteNames ?? {}
        ),
        sqliteNames: Object.fromEntries(
          Object.entries(data.sqliteNames ?? {})
            .filter(
              ([path, name]) =>
                sqlitePaths.includes(path) && typeof name === 'string'
            )
            .map(([path, name]) => [path, name.slice(0, 80)])
        ),
        sqlitePaths
      }
    }
  },
  slice.reducer
)
