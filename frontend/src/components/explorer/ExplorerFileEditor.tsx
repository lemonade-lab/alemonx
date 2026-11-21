import { RefreshCw, Save } from 'lucide-react'
import { useDispatch } from 'react-redux'
import { store, useStoreState } from '../../store/guideStore'
import {
  useSaveWorkspaceProjectFileMutation,
  useWorkspaceProjectFileQuery,
  workspaceApi
} from '../../store/workspaceApi'
import { Button } from '../Button'
import { ConfirmDialog } from '../ConfirmDialog'

export type ExplorerFileDraft = {
  content: string
  original: string
  revision: string
  saving?: boolean
  error?: string
}

function editorText(content: string) {
  return content.replace(/\r\n?/g, '\n')
}

function saveText(content: string, original: string) {
  const hasOnlyCRLF =
    original.includes('\r\n') && !original.replace(/\r\n/g, '').includes('\n')
  return hasOnlyCRLF ? content.replace(/\n/g, '\r\n') : content
}

function errorMessage(error: unknown, fallback: string) {
  const response = error as
    { status?: number; data?: { error?: string } } | undefined
  if (response?.status === 409)
    return '磁盘上的文件已更改。草稿仍保留，重新加载磁盘版本前可以复制草稿内容。'
  return response?.data?.error || fallback
}

export function ExplorerFileEditor({
  root,
  file,
  projectName,
  draft,
  onDraftChange
}: {
  root: string
  file: string
  projectName: string
  draft?: ExplorerFileDraft
  onDraftChange: (draft?: ExplorerFileDraft) => void
}) {
  const dispatch = useDispatch<typeof store.dispatch>()
  const args = { root, file }
  const { currentData, error, isFetching, refetch } =
    useWorkspaceProjectFileQuery(args)
  const [save] = useSaveWorkspaceProjectFileMutation()
  const [confirmReload, setConfirmReload] = useStoreState(false)
  const [notice, setNotice] = useStoreState('')
  const [reloadError, setReloadError] = useStoreState('')

  function edit(content: string) {
    const original = draft?.original ?? currentData?.content
    const revision = draft?.revision ?? currentData?.revision
    if (original === undefined || !revision) return
    setNotice('')
    setReloadError('')
    onDraftChange(
      content === editorText(original)
        ? undefined
        : { content, original, revision }
    )
  }

  async function saveFile() {
    if (!draft || draft.saving) return
    onDraftChange({ ...draft, saving: true, error: undefined })
    try {
      const result = await save({
        ...args,
        revision: draft.revision,
        content: saveText(draft.content, draft.original)
      }).unwrap()
      dispatch(
        workspaceApi.util.updateQueryData(
          'workspaceProjectFile',
          args,
          () => result
        )
      )
      onDraftChange()
      setNotice('已保存')
      setReloadError('')
    } catch (failure) {
      onDraftChange({
        ...draft,
        saving: false,
        error: errorMessage(failure, '无法保存文件。')
      })
    }
  }

  async function reload() {
    setConfirmReload(false)
    setReloadError('')
    try {
      await refetch().unwrap()
      onDraftChange()
      setNotice('')
    } catch (failure) {
      setReloadError(errorMessage(failure, '无法重新读取文件。'))
    }
  }

  return (
    <>
      <header className="flex min-h-10 shrink-0 items-center gap-2 border-b border-(--theme-border-subtle) px-3">
        <span className="min-w-0 flex-1 truncate text-sm" title={file}>
          {projectName} / {file}
        </span>
        {(notice || draft) && (
          <span
            className="shrink-0 text-xs text-(--theme-text-muted)"
            role="status"
          >
            {draft?.saving ? '正在保存…' : draft ? '未保存' : notice}
          </span>
        )}
        <Button
          variant="icon"
          aria-label="保存"
          title="保存"
          disabled={!draft || draft.saving || isFetching}
          onClick={() => void saveFile()}
        >
          <Save className="size-4" aria-hidden="true" />
        </Button>
        <Button
          variant="icon"
          aria-label="刷新文件内容"
          onClick={() => (draft ? setConfirmReload(true) : void reload())}
          disabled={isFetching || draft?.saving}
        >
          <RefreshCw className="size-4" aria-hidden="true" />
        </Button>
      </header>
      {(draft?.error || reloadError) && (
        <p
          className="shrink-0 px-3 py-2 text-sm text-(--theme-danger-text)"
          role="alert"
        >
          {reloadError || draft?.error}
        </p>
      )}
      {draft || (currentData && !error) ? (
        <textarea
          aria-label={`编辑 ${file}`}
          className="explorer-file-editor min-h-0 flex-1 resize-none overflow-auto bg-(--theme-surface-panel) p-4 font-mono text-sm leading-6 text-(--theme-text-primary)"
          spellCheck={false}
          wrap="off"
          readOnly={
            (!currentData?.revision && !draft) || draft?.saving || isFetching
          }
          value={draft?.content ?? editorText(currentData?.content ?? '')}
          onChange={event => edit(event.currentTarget.value)}
          onKeyDown={event => {
            if (
              !(event.metaKey || event.ctrlKey) ||
              event.key.toLowerCase() !== 's'
            )
              return
            event.preventDefault()
            if (!isFetching) void saveFile()
          }}
        />
      ) : (
        <p
          className="p-4 text-sm text-(--theme-text-muted)"
          role={error ? 'alert' : 'status'}
        >
          {error ? errorMessage(error, '无法读取文件。') : '正在读取文件…'}
        </p>
      )}
      <ConfirmDialog
        open={confirmReload}
        title="重新加载磁盘版本"
        message="重新加载会放弃这个文件的未保存修改。"
        confirmLabel="重新加载"
        onCancel={() => setConfirmReload(false)}
        onConfirm={() => void reload()}
      />
    </>
  )
}
