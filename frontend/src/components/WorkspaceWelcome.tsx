import { ArrowRight, FolderOpen, GitBranch } from 'lucide-react'

type Props = {
  onAdd: () => void
  onClone: () => void
}

export function WorkspaceWelcome({ onAdd, onClone }: Props) {
  return (
    <section
      className="col-span-full flex min-h-full flex-col items-center justify-center gap-6 px-5 py-8"
      aria-labelledby="workspace-welcome-title"
    >
      <div className="max-w-xl text-center">
        <h1
          id="workspace-welcome-title"
          className="m-0 text-2xl font-semibold text-(--theme-text-strong)"
        >
          开始管理你的机器人
        </h1>
        <p className="mb-0 mt-3 text-sm leading-6 text-(--theme-text-secondary)">
          添加一个机器人项目，集中管理运行状态、配置与插件。
        </p>
      </div>
      <div className="grid w-full max-w-2xl gap-3 sm:grid-cols-2">
        {[
          {
            title: '已有本地项目',
            description:
              '选择电脑上已有的机器人目录，加入管理列表。原有项目文件会保留。',
            action: '添加本地目录',
            icon: FolderOpen,
            onClick: onAdd
          },
          {
            title: '从远程仓库开始',
            description:
              '填写 Git 仓库地址并选择保存位置，将项目下载到本机后管理。',
            action: '从 Git 克隆',
            icon: GitBranch,
            onClick: onClone
          }
        ].map(({ title, description, action, icon: Icon, onClick }) => (
          <button
            key={title}
            type="button"
            onClick={onClick}
            className="group flex min-w-0 flex-col items-start gap-3 rounded-xl border border-(--theme-border-default) bg-(--theme-surface-panel) p-5 text-left transition hover:border-(--theme-accent) hover:bg-(--theme-accent-soft)"
            aria-label={action}
          >
            <Icon
              className="size-6 text-(--theme-accent-text)"
              aria-hidden="true"
            />
            <span className="text-base font-semibold text-(--theme-text-strong)">
              {title}
            </span>
            <span className="text-sm leading-6 text-(--theme-text-secondary)">
              {description}
            </span>
            <span className="mt-auto flex items-center gap-2 pt-2 text-sm font-semibold text-(--theme-accent-text)">
              {action}
              <ArrowRight className="size-4" aria-hidden="true" />
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}
