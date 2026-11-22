import { RefreshButton } from '../RefreshButton'
import { useSelector } from 'react-redux'
import type { RootState } from '../../store/guideStore'
import { useDataCatalogQuery } from '../../store/dataApi'
import { RedisWorkspace } from './RedisWorkspace'
import { BuiltinSQLite } from './BuiltinSQLite'
import { ConnectWorkspace } from './ConnectWorkspace'

export default function DataWorkspace() {
  const tab = useSelector((state: RootState) => state.dataPreferences.tab)
  const catalog = useDataCatalogQuery(undefined, { skip: tab !== 'connect' })
  return (
    <section
      className="grid min-w-0 gap-4 p-4 text-(--theme-text-primary)"
      aria-label="数据管理"
    >
      {tab === 'sqlite' ? (
        <>
          <BuiltinSQLite />
        </>
      ) : tab.startsWith('redis') ? (
        <RedisWorkspace />
      ) : (
        <>
          {catalog.isError && (
            <div role="alert">
              本地数据库建议暂不可用，仍可配置其他 SQL 连接。
              <RefreshButton label="重试" onClick={() => void catalog.refetch()} />
            </div>
          )}
          <ConnectWorkspace suggestions={catalog.data?.sqlite ?? []} />
        </>
      )}
    </section>
  )
}
