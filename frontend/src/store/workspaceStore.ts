import { createSlice, type PayloadAction } from '@reduxjs/toolkit'
import { createTransform, persistReducer } from 'redux-persist'

const storage = {
  getItem: (key: string) => Promise.resolve(window.localStorage.getItem(key)),
  setItem: (key: string, value: string) =>
    Promise.resolve(window.localStorage.setItem(key, value)),
  removeItem: (key: string) =>
    Promise.resolve(window.localStorage.removeItem(key))
}

const workspacePersistKey = 'persist:alemonx-workspace'
const isPrivateDraft = (key: string) =>
  key.endsWith(':.env') ||
  key.endsWith(':.npmrc') ||
  key.endsWith(':alemon.config.yaml') ||
  key.endsWith(':alemon.config.yml')

// Remove values stored by older releases as soon as this module is loaded.
// A redux-persist transform protects future writes, but would otherwise leave
// an old raw value in localStorage until the next state change.
function removeLegacyPrivateDrafts() {
  try {
    const stored = window.localStorage.getItem(workspacePersistKey)
    if (!stored) return
    const state = JSON.parse(stored) as Record<string, string>
    const drafts = JSON.parse(state.drafts ?? '{}') as Record<string, string>
    const cleanDrafts = Object.fromEntries(
      Object.entries(drafts).filter(([key]) => !isPrivateDraft(key))
    )
    if (Object.keys(cleanDrafts).length === Object.keys(drafts).length) return
    state.drafts = JSON.stringify(cleanDrafts)
    window.localStorage.setItem(workspacePersistKey, JSON.stringify(state))
  } catch {
    // Persisted workspace data is optional; ignore malformed legacy entries.
  }
}

removeLegacyPrivateDrafts()

export type WorkspaceProject = {
  id: string
  path: string
  name: string
  pinned?: boolean
}
const legacyRobotListKey = 'alemonx-legacy-robot-list'
type LegacyRobots = { projects: WorkspaceProject[]; activeProjectID: string }

export function readLegacyRobots(): LegacyRobots {
  try {
    const saved = JSON.parse(
      window.localStorage.getItem(legacyRobotListKey) ?? '{}'
    ) as Partial<LegacyRobots>
    return {
      projects: Array.isArray(saved.projects)
        ? saved.projects.filter(item => typeof item?.path === 'string')
        : [],
      activeProjectID:
        typeof saved.activeProjectID === 'string' ? saved.activeProjectID : ''
    }
  } catch {
    return { projects: [], activeProjectID: '' }
  }
}

export function clearLegacyRobots() {
  window.localStorage.removeItem(legacyRobotListKey)
}

try {
  const raw = window.localStorage.getItem(workspacePersistKey)
  if (raw) {
    const saved = JSON.parse(raw) as Record<string, string>
    if (saved.projects || saved.activeProjectID) {
      const projects = JSON.parse(saved.projects ?? '[]')
      if (
        Array.isArray(projects) &&
        projects.length &&
        !window.localStorage.getItem(legacyRobotListKey)
      ) {
        window.localStorage.setItem(
          legacyRobotListKey,
          JSON.stringify({
            projects,
            activeProjectID: JSON.parse(saved.activeProjectID ?? '""')
          })
        )
      }
      delete saved.projects
      delete saved.activeProjectID
      window.localStorage.setItem(workspacePersistKey, JSON.stringify(saved))
    }
  }
} catch {
  // An unreadable legacy browser list must not block account data.
}

type WorkspaceState = {
  projects: WorkspaceProject[]
  activeProjectID: string
  drafts: Record<string, string>
  // One in-memory configuration document per robot root. It is deliberately
  // separate from generic file drafts, because several dashboard features
  // edit alemon.config.yaml without going through the text editor.
  robotConfigs: Record<string, string>
  developerMode: boolean
}

const initialState: WorkspaceState = {
  projects: [],
  activeProjectID: '',
  drafts: {},
  robotConfigs: {},
  developerMode: false
}
const workspaceSlice = createSlice({
  name: 'workspace',
  initialState,
  reducers: {
    replaceAccountRobots(state, action: PayloadAction<LegacyRobots>) {
      state.projects = action.payload.projects
      state.activeProjectID = action.payload.activeProjectID
    },
    setDraft(state, action: PayloadAction<{ key: string; content: string }>) {
      if (state.drafts[action.payload.key] === action.payload.content) return
      state.drafts[action.payload.key] = action.payload.content
    },
    setRobotConfig(
      state,
      action: PayloadAction<{ root: string; content: string }>
    ) {
      if (
        action.payload.root &&
        state.robotConfigs[action.payload.root] !== action.payload.content
      )
        state.robotConfigs[action.payload.root] = action.payload.content
    },
    setDeveloperMode(state, action: PayloadAction<boolean>) {
      state.developerMode = action.payload
    },
    clearDraft(state, action: PayloadAction<string>) {
      delete state.drafts[action.payload]
    }
  }
})

// Environment, npm registry and robot configuration files can contain tokens,
// passwords and connection credentials. Keep an unsaved edit while the page
// is open, but never write those values to browser localStorage.
const privateDraftsTransform = createTransform<
  Record<string, string>,
  Record<string, string>
>(
  drafts =>
    Object.fromEntries(
      Object.entries(drafts).filter(([key]) => !isPrivateDraft(key))
    ),
  drafts =>
    Object.fromEntries(
      Object.entries(drafts).filter(([key]) => !isPrivateDraft(key))
    ),
  { whitelist: ['drafts'] }
)

export const persistedWorkspace = persistReducer(
  {
    key: 'alemonx-workspace',
    storage,
    whitelist: ['drafts', 'developerMode'],
    transforms: [privateDraftsTransform],
    // A stored state written by an older release may hold null/undefined for
    // fields this build expects to be arrays. Never let that leak into the
    // live state, or components calling .find on them crash on first render.
    stateReconciler: (inbound, _original, reduced) => {
      const restored = { ...reduced, ...(inbound ?? {}) } as WorkspaceState
      if (!Array.isArray(restored.projects)) restored.projects = []
      if (!restored.robotConfigs || typeof restored.robotConfigs !== 'object')
        restored.robotConfigs = {}
      if (!restored.activeProjectID) restored.activeProjectID = ''
      return restored
    }
  },
  workspaceSlice.reducer
)
export const {
  replaceAccountRobots,
  setDraft,
  setRobotConfig,
  clearDraft,
  setDeveloperMode
} = workspaceSlice.actions
