import type { LocalPackageStatus } from '../store/workspaceApi'
export type BackpackPackageUpdateProps = {
  root: string
  name: string
  busy: boolean
  status?: LocalPackageStatus
  onReplace: (
    name: string,
    version: string,
    force?: boolean
  ) => Promise<boolean>
  onRefresh: () => void
}
