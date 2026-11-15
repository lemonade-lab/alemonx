import { RefreshCw } from 'lucide-react'
import { useStoreState } from '../store/guideStore'
import { useBunRuntimeStatusQuery } from '../store/workspaceApi'
import { EnvironmentFixDialog } from './EnvironmentFixDialog'

export function BunRuntimePanel({ onChanged }: { onChanged: () => void }) {
  const { data, isFetching, isError, refetch } = useBunRuntimeStatusQuery(
    undefined,
    { refetchOnMountOrArgChange: true }
  )
  const [installMode, setInstallMode] = useStoreState<
    'install' | 'update' | null
  >(null)
  const ready = data?.check.status === 'ready'
  const managed = ready && data?.managed
  const supportsManagedInstall =
    data &&
    !data.container &&
    /^(linux|darwin|windows)\/(arm64|amd64)$/.test(data.platform)

  return (
    <section className="grid gap-3 py-3" aria-label="Bun 版本管理">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="m-0 text-sm font-semibold text-(--theme-text-strong)">
          Bun 版本管理
        </h2>
        <button
          type="button"
          className="secondary-button inline-flex items-center gap-1.5"
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          <RefreshCw
            className={`size-3.5 ${isFetching ? 'animate-spin' : ''}`}
          />
          重新检测 Bun
        </button>
      </header>

      {isError && (
        <p role="alert" className="m-0 text-sm text-(--theme-text-danger)">
          读取 Bun 状态失败。请重新检测。
        </p>
      )}
      {!data && isFetching && (
        <p role="status" className="m-0 text-sm text-(--theme-text-secondary)">
          正在读取 Bun 状态…
        </p>
      )}
      {data && !isError && (
        <div className="grid gap-3 rounded-xl bg-(--theme-surface-raised) p-4">
          <dl className="m-0 grid gap-2 text-sm">
            <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
              <dt className="text-(--theme-text-secondary)">状态</dt>
              <dd className="m-0">{ready ? '已就绪' : data.check.detail}</dd>
            </div>
            {data.version && (
              <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
                <dt className="text-(--theme-text-secondary)">当前版本</dt>
                <dd className="m-0">
                  <code>{data.version}</code>
                </dd>
              </div>
            )}
            {data.path && (
              <>
                <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
                  <dt className="text-(--theme-text-secondary)">安装来源</dt>
                  <dd className="m-0">
                    {data.managed ? '工作台托管' : '外部安装'}
                  </dd>
                </div>
                <div className="grid grid-cols-[5rem_minmax(0,1fr)] gap-2">
                  <dt className="text-(--theme-text-secondary)">程序路径</dt>
                  <dd className="m-0 min-w-0 break-all">
                    <code>{data.path}</code>
                  </dd>
                </div>
              </>
            )}
          </dl>
          {data.container ? (
            <p className="m-0 text-xs text-(--theme-text-secondary)">
              Docker 环境的运行时由镜像提供。
            </p>
          ) : ready && !managed ? (
            <p className="m-0 text-xs text-(--theme-text-secondary)">
              请通过原安装方式更新 Bun。
            </p>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <button
                type="button"
                className="primary-button"
                disabled={isFetching}
                onClick={() => setInstallMode(managed ? 'update' : 'install')}
              >
                {managed
                  ? '检查并更新 Bun'
                  : supportsManagedInstall
                    ? '安装 Bun'
                    : '查看安装说明'}
              </button>
            </div>
          )}
        </div>
      )}
      {installMode && data && (
        <EnvironmentFixDialog
          check={{
            ...data.check,
            suggestion:
              installMode === 'update'
                ? '检查 Bun 官方稳定版，并在有新版本时更新工作台托管的副本。'
                : data.check.suggestion
          }}
          platform={data.platform}
          updateBun={installMode === 'update'}
          onClose={() => setInstallMode(null)}
          onInstalled={() => {
            void refetch()
            onChanged()
          }}
        />
      )}
    </section>
  )
}
