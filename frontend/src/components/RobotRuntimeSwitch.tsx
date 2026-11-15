import { useStoreState } from '../store/guideStore'
import {
  useApplyRobotRuntimeSwitchMutation,
  useLazyRobotRuntimeSwitchQuery,
  type ProjectRuntime
} from '../store/workspaceApi'
import { EnvironmentFixDialog } from './EnvironmentFixDialog'
import { Modal } from './Modal'

const labels = { node: 'Node.js', bun: 'Bun' }

function requestError(reason: unknown) {
  const error = reason as { data?: { error?: string }; message?: string }
  return error?.data?.error || error?.message || '操作失败，请重新检查后重试。'
}

export function RobotRuntimeSwitch({
  root,
  current,
  onChanged,
  disabled
}: {
  root: string
  current: ProjectRuntime
  onChanged: () => void
  disabled: boolean
}) {
  const [open, setOpen] = useStoreState(false)
  const [repair, setRepair] = useStoreState(false)
  const [message, setMessage] = useStoreState('')
  const [failure, setFailure] = useStoreState('')
  const [load, { data: plan, isFetching }] = useLazyRobotRuntimeSwitchQuery()
  const [apply, { isLoading: applying }] = useApplyRobotRuntimeSwitchMutation()
  const target = current === 'bun' ? 'node' : 'bun'
  const preview = plan?.target === target ? plan : undefined
  const missing =
    preview?.check.status === 'missing' || preview?.check.status === 'warning'

  async function check() {
    setFailure('')
    try {
      await load({ root, target }, false).unwrap()
    } catch (reason) {
      setFailure(requestError(reason))
    }
  }

  async function confirm() {
    if (!preview || preview.blocked.length || applying) return
    setFailure('')
    try {
      await apply({ root, target, revision: preview.revision }).unwrap()
      setOpen(false)
      setMessage(`${labels[target]}`)
      onChanged()
    } catch (reason) {
      setFailure(requestError(reason))
    }
  }

  return (
    <>
      <button
        className="secondary-button whitespace-nowrap"
        disabled={disabled || applying}
        onClick={() => {
          setMessage('')
          setOpen(true)
          void check()
        }}
      >
        切换运行时
      </button>
      {message && (
        <span role="status" className="text-xs">
          {message}
        </span>
      )}
      <Modal
        open={open && !repair}
        ariaLabel="切换项目运行时"
        onClose={() => !applying && setOpen(false)}
      >
        <section className="modal-surface grid max-h-[calc(100dvh-48px)] w-full max-w-md gap-4 overflow-auto">
          <header>
            <h2 className="m-0 text-base font-semibold">切换项目运行时</h2>
            <p className="mt-1 text-sm">
              {labels[current]} → {labels[target]}
            </p>
          </header>
          {isFetching ? (
            <p role="status">正在检查环境和项目…</p>
          ) : (
            preview && (
              <div className="grid gap-2 text-sm">
                <p>
                  目标包管理器 {preview.packageManager}
                  {preview.version && ` · ${preview.version}`}
                </p>
                {preview.check.status === 'outdated' && (
                  <p className="text-sm text-(--theme-text-secondary)">
                    {preview.check.suggestion}
                  </p>
                )}
                <ul className="list-disc pl-5">
                  {preview.changes.map(change => (
                    <li key={change}>{change}</li>
                  ))}
                </ul>
                {preview.blocked.length > 0 && (
                  <ul
                    role="alert"
                    className="list-disc pl-5 text-(--theme-text-danger)"
                  >
                    {preview.blocked.map(item => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                )}
              </div>
            )
          )}
          {failure && (
            <p role="alert" className="text-sm text-(--theme-text-danger)">
              {failure}
            </p>
          )}
          <footer className="flex flex-wrap justify-end gap-2">
            <button
              className="secondary-button"
              disabled={applying}
              onClick={() => setOpen(false)}
            >
              取消
            </button>
            <button
              className="secondary-button"
              disabled={isFetching || applying}
              onClick={() => void check()}
            >
              重新检查
            </button>
            {missing && preview && (
              <button
                className="secondary-button"
                disabled={isFetching || applying}
                onClick={() => setRepair(true)}
              >
                安装 {labels[target]}
              </button>
            )}
            <button
              className="primary-button"
              disabled={
                isFetching ||
                applying ||
                !preview ||
                Boolean(failure) ||
                preview.blocked.length > 0
              }
              onClick={() => void confirm()}
            >
              {applying ? '正在切换…' : '确认切换'}
            </button>
          </footer>
        </section>
      </Modal>
      {repair && preview && (
        <EnvironmentFixDialog
          check={preview.check}
          platform={preview.platform}
          onClose={() => setRepair(false)}
          onInstalled={() => void check()}
        />
      )}
    </>
  )
}
