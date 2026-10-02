import { ExternalLink, Loader2, Sparkles } from 'lucide-react'
import { useOpenDSHWebMutation } from '../store/workspaceApi'
import type { DSHWorkspaceProps } from './DSHWorkspace.d'

export function DSHWorkspace({ root, onOpenWeb }: DSHWorkspaceProps) {
  const [openWeb, { isLoading, error }] = useOpenDSHWebMutation()
  const failure =
    error && 'data' in error ? (error.data as { error?: string }) : undefined

  async function open() {
    try {
      const result = await openWeb(root).unwrap()
      onOpenWeb(result.url)
    } catch {
      // The mutation exposes the recoverable failure below.
    }
  }

  return (
    <section className="flex h-full min-h-0 flex-col items-center justify-center gap-4 p-6 text-center">
      <Sparkles
        className="size-8 text-(--theme-accent-text)"
        aria-hidden="true"
      />
      <h2 className="text-lg font-semibold text-(--theme-text-primary)">
        DSH Web 版
      </h2>
      <p className="max-w-lg text-sm text-(--theme-text-secondary)">
        自动进入当前机器人工作目录，在官方界面中管理对话、模型和设置。
      </p>
      {!root && (
        <p className="text-sm text-(--theme-text-muted)">
          请先选择机器人项目。
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-(--theme-text-secondary)">
          {failure?.error || '无法打开 DSH Web 版，请重试。'}
        </p>
      )}
      <button
        type="button"
        disabled={!root || isLoading}
        onClick={() => void open()}
        className="inline-flex items-center gap-2 rounded-lg bg-(--theme-accent) px-4 py-2 text-sm text-white transition hover:opacity-90 focus-visible:ring-2 focus-visible:ring-(--theme-accent-soft-border) disabled:cursor-not-allowed disabled:opacity-50"
      >
        {isLoading ? (
          <Loader2 className="size-4 animate-spin" aria-hidden="true" />
        ) : (
          <ExternalLink className="size-4" aria-hidden="true" />
        )}
        {isLoading ? '正在启动…' : error ? '重试打开 Web 版' : '打开 Web 版'}
      </button>
    </section>
  )
}
