import type { WorkspaceProject } from '../store/workspaceStore'

export type RobotList = {
  scope: string
  local: boolean
  initialized: boolean
  projects: WorkspaceProject[]
  activeProjectID: string
}

export async function requestRobots(options?: RequestInit): Promise<RobotList> {
  const response = await fetch('/api/v1/workspace/robots', options)
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error || '无法同步账户机器人列表。')
  }
  return response.json()
}
