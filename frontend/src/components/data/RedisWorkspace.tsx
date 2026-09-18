import { useSelector } from 'react-redux'
import type { RootState } from '../../store/guideStore'
import { useDataCatalogQuery } from '../../store/dataApi'
import { Button } from '../Button'
import { RedisBrowser } from './RedisBrowser'
import { RedisServicePanel } from './RedisServicePanel'

export function RedisWorkspace() {
  const tab = useSelector((state: RootState) => state.dataPreferences.tab)
  const service = tab === 'redis-service'
  const catalog = useDataCatalogQuery()
  return (
    <div className="grid min-w-0 gap-4">
      {/* 保持浏览器挂载，切换服务面板时不丢失尚未保存的编辑内容。 */}
      <div hidden={service}>
        {catalog.isLoading ? (
          <p role="status">正在读取本地连接…</p>
        ) : catalog.isError ? (
          <div role="alert">
            无法读取连接，可能缺少管理权限。
            <Button onClick={() => void catalog.refetch()}>重试</Button>
          </div>
        ) : (
          <RedisBrowser
            defaultAddress={catalog.data?.redisAddress ?? ''}
            workbench={tab === 'redis-workbench'}
          />
        )}
      </div>
      {service && <RedisServicePanel />}
    </div>
  )
}
