import { useStoreState } from '../store/guideStore'
import { GitSettingsDialog } from './GitSettingsDialog'
import { CloudDownload, Plus, Trash2 } from 'lucide-react'
import type { GitAction, GitWorkspace } from '../store/workspaceApi'

type Props = {
  data?: GitWorkspace
  busy: boolean
  remoteAction: (action: 'fetch', remote: string) => void
  save: (
    action: 'remote-add' | 'remote-set-url',
    value: string,
    message: string
  ) => Promise<string | null>
  request: (
    action: GitAction,
    value?: string,
    message?: string,
    paths?: string[]
  ) => void
}
export function GitRemotePanel({
  data,
  busy,
  remoteAction,
  request,
  save
}: Props) {
  const [addName, setAddName] = useStoreState('origin')
  const [addURL, setAddURL] = useStoreState('')
  const [addError, setAddError] = useStoreState('')
  const [editor, setEditor] = useStoreState<'edit' | null>(null)
  const [name, setName] = useStoreState('')
  const [url, setURL] = useStoreState('')
  const [formError, setFormError] = useStoreState('')
  const openEditor = (remoteName: string, remoteURL: string) => {
    setFormError('')
    setName(remoteName)
    setURL(remoteURL)
    setEditor('edit')
  }
  const submit = async () => {
    if (!editor || busy) return
    setFormError('')
    const error = await save('remote-set-url', name.trim(), url.trim())
    if (error) setFormError(error)
    else setEditor(null)
  }
  const saveAddress = async () => {
    if (busy || !addName.trim() || !addURL.trim()) return
    setAddError('')
    const error = await save('remote-add', addName.trim(), addURL.trim())
    if (error) setAddError(error)
    else setAddURL('')
  }
  return (
    <section className="grid gap-3 rounded-lg border border-slate-200 bg-white p-4">
      <div className="grid gap-3">
        <form
          className="grid gap-2"
          onSubmit={event => {
            event.preventDefault()
            void saveAddress()
          }}
        >
          <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-end">
            <label className="grid gap-1 text-xs text-slate-600 sm:w-28 sm:shrink-0">
              远程仓库名称
              <input
                className="h-9 w-full min-w-0 rounded-md border border-slate-300 bg-white px-2.5"
                value={addName}
                onChange={event => setAddName(event.target.value)}
                required
                disabled={busy}
                placeholder="例如 origin"
              />
            </label>
            <label className="grid min-w-0 gap-1 text-xs text-slate-600 sm:w-64">
              远程仓库地址
              <input
                className="h-9 w-full min-w-0 rounded-md border border-slate-300 bg-white px-2.5"
                value={addURL}
                onChange={event => setAddURL(event.target.value)}
                required
                disabled={busy}
                placeholder="https://github.com/owner/repo.git"
              />
            </label>
            <div className="flex shrink-0 gap-2">
              <button
                type="submit"
                className="primary-button whitespace-nowrap"
                disabled={busy || !addName.trim() || !addURL.trim()}
              >
                <Plus className="mr-1 size-3.5" />
                添加
              </button>
            </div>
          </div>
          {addError && (
            <p role="alert" className="break-words text-xs text-red-700">
              {addError}
            </p>
          )}
        </form>
        <section className="grid gap-2">
          <header className="grid gap-2">
            <strong className="text-sm font-semibold text-slate-700">
              已配置远程
            </strong>
          </header>
          {data?.remotes.length ? (
            <ul className="grid gap-1">
              {data.remotes.map(item => (
                <li
                  className="flex min-w-0 flex-col gap-3 rounded-md border border-slate-100 bg-slate-50 p-3 sm:flex-row sm:items-center"
                  key={item.name}
                  aria-label={`远程仓库 ${item.name}`}
                >
                  <div className="grid min-w-0 flex-1 gap-1">
                    <div className="flex min-w-0 items-center gap-3">
                      <code className="shrink-0 rounded bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-700">
                        {item.name}
                      </code>
                      <span
                        className="min-w-0 flex-1 truncate text-xs text-slate-700"
                        title={item.url}
                      >
                        {item.url}
                      </span>
                    </div>
                    {data.remoteName === item.name && data.remoteChecked && (
                      <span className="text-[11px] text-slate-500">
                        {data.remoteReachable ? '远程可访问' : '远程暂不可访问'}
                      </span>
                    )}
                    {data.remoteName === item.name && data.remoteAdvice && (
                      <p
                        role="alert"
                        className="break-words text-xs text-amber-700"
                      >
                        {data.remoteAdvice}
                      </p>
                    )}
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-2">
                    <button
                      className="secondary-button whitespace-nowrap"
                      disabled={busy}
                      onClick={() => remoteAction('fetch', item.name)}
                      title={`获取 ${item.name} 的分支和标签，不修改工作区`}
                    >
                      <CloudDownload className="mr-1 size-3.5" />
                      获取更新
                    </button>
                    <button
                      className="text-button whitespace-nowrap"
                      disabled={busy}
                      onClick={() => openEditor(item.name, item.url)}
                    >
                      编辑
                    </button>
                    <button
                      className="text-button whitespace-nowrap text-red-700"
                      disabled={busy}
                      onClick={() => request('remote-remove', item.name)}
                    >
                      <Trash2 className="mr-1 size-3.5" />
                      移除
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <p className="grid min-h-20 place-items-center text-xs text-slate-500">
              还没有配置远程仓库，在上方填写名称和地址后添加。
            </p>
          )}
        </section>
      </div>
      <GitSettingsDialog
        editor={editor}
        busy={busy}
        error={formError}
        name={name}
        email=""
        url={url}
        onNameChange={setName}
        onEmailChange={() => {}}
        onURLChange={setURL}
        onClose={() => setEditor(null)}
        onSubmit={() => void submit()}
      />
    </section>
  )
}
