export type DataTab =
  | 'redis'
  | 'redis-browser'
  | 'redis-workbench'
  | 'redis-service'
  | 'sqlite'
  | 'connect'
export function migrateDataTab(tab: unknown): DataTab {
  if (tab === 'sql' || tab === 'connect') return 'connect'
  if (tab === 'redis-workbench' || tab === 'redis-service') return tab
  return tab === 'sqlite' ? 'sqlite' : 'redis-browser'
}
