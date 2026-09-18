import {
  Database,
  Cable,
  HardDrive,
  ListTree,
  Terminal,
  Server
} from 'lucide-react'
import type { SidebarWindowItem } from '../SidebarWindow'
import type { DataTab } from '../../lib/dataNavigation'
export const dataNavigation: SidebarWindowItem<DataTab>[] = [
  {
    id: 'redis',
    label: 'Redis',
    icon: Database,
    children: [
      { id: 'redis-browser', label: '数据浏览', icon: ListTree },
      { id: 'redis-workbench', label: '命令工作台', icon: Terminal },
      { id: 'redis-service', label: '本地服务', icon: Server }
    ]
  },
  { id: 'sqlite', label: 'SQLite', icon: HardDrive },
  { id: 'connect', label: 'Connect', icon: Cable }
]
