// The release workflow sets VITE_ALX_BUILD_ID to the same value embedded in
// the Go binary. A mismatch means a browser has stale assets or reached a
// different server after an update.
export const frontendBuildID = import.meta.env.VITE_ALX_BUILD_ID || 'dev'
