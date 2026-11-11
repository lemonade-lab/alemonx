import {
  Boxes,
  Cable,
  KeyRound,
  Network,
  RefreshCw,
  Server,
  Settings2,
  Terminal,
  UsersRound
} from 'lucide-react'
import { GithubMark } from './GithubMark'
import type { SidebarWindowItem } from './SidebarWindow'

export type SettingsSection =
  | 'general'
  | 'auth'
  | 'accounts'
  | 'github'
  | 'ssh'
  | 'mcp'
  | 'network'
  | 'update'
  | 'service'
  | 'dependency-sources'

export const settingsSections: readonly SidebarWindowItem<SettingsSection>[] = [
  { id: 'general', label: '通用', icon: Settings2 },
  { id: 'auth', label: '认证', icon: KeyRound },
  { id: 'accounts', label: '账户', icon: UsersRound },
  { id: 'github', label: 'GitHub', icon: GithubMark },
  { id: 'ssh', label: 'SSH 管理', icon: Terminal },
  { id: 'mcp', label: 'MCP', icon: Cable },
  { id: 'network', label: '网络', icon: Network },
  { id: 'update', label: '更新', icon: RefreshCw },
  { id: 'service', label: '服务', icon: Server },
  { id: 'dependency-sources', label: '依赖源', icon: Boxes }
]
