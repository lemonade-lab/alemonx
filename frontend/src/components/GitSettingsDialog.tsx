import { X } from 'lucide-react'
import { Button } from './Button'
import { Modal } from './Modal'
import type { GitSettingsDialogProps } from './GitSettingsDialog.d'

export function GitSettingsDialog({
  editor,
  busy,
  error,
  name,
  email,
  url,
  onNameChange,
  onEmailChange,
  onURLChange,
  onClose,
  onSubmit
}: GitSettingsDialogProps) {
  if (!editor) return null
  const identity = editor === 'identity'
  const title = identity
    ? '提交身份'
    : editor === 'add'
      ? '添加远程仓库'
      : '编辑远程仓库'
  const valid = name.trim() && (identity ? email.trim() : url.trim())
  const inputClass =
    'h-9 min-w-0 rounded-md border border-slate-300 bg-white px-2.5 text-sm outline-none focus:border-brand-600 focus:ring-2 focus:ring-brand-100 disabled:opacity-60'
  return (
    <Modal
      open
      ariaLabel={title}
      className="p-4"
      onClose={busy ? undefined : onClose}
    >
      <form
        className="flex max-h-[calc(100dvh-32px)] w-full max-w-md min-w-0 flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-xl"
        onSubmit={event => {
          event.preventDefault()
          if (!busy && valid) onSubmit()
        }}
      >
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-slate-200 p-4">
          <strong className="text-sm text-slate-800">{title}</strong>
          <Button
            variant="icon"
            disabled={busy}
            aria-label="关闭设置"
            onClick={onClose}
            type="button"
          >
            <X className="size-4" />
          </Button>
        </header>
        <div className="grid min-h-0 gap-3 overflow-auto p-4">
          {identity && (
            <p className="text-xs text-slate-500">
              仅修改当前仓库的提交姓名和邮箱。
            </p>
          )}
          <label className="grid gap-1 text-xs text-slate-600">
            {identity ? '提交姓名' : '远程仓库名称'}
            <input
              className={inputClass}
              value={name}
              required
              disabled={busy || editor === 'edit'}
              onChange={event => onNameChange(event.target.value)}
              placeholder={identity ? '你的姓名' : '例如 origin'}
            />
          </label>
          {identity ? (
            <label className="grid gap-1 text-xs text-slate-600">
              提交邮箱
              <input
                className={inputClass}
                type="email"
                value={email}
                required
                disabled={busy}
                onChange={event => onEmailChange(event.target.value)}
                placeholder="name@example.com"
              />
            </label>
          ) : (
            <label className="grid gap-1 text-xs text-slate-600">
              远程仓库地址
              <input
                className={inputClass}
                value={url}
                required
                disabled={busy}
                onChange={event => onURLChange(event.target.value)}
                placeholder="https://github.com/owner/repo.git"
              />
            </label>
          )}
          {error && (
            <p role="alert" className="break-words text-xs text-red-700">
              {error}
            </p>
          )}
        </div>
        <footer className="flex shrink-0 justify-end gap-2 border-t border-slate-200 p-4">
          <Button type="button" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={!valid}
            loading={busy}
            loadingLabel="正在保存…"
          >
            {editor === 'add' ? '添加' : '保存'}
          </Button>
        </footer>
      </form>
    </Modal>
  )
}
