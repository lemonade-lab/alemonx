export function ConnectionContext({
  engine,
  target,
  state,
  access
}: {
  engine: string
  target: string
  state: string
  access: string
}) {
  return (
    <div
      aria-label="当前连接"
      className="flex min-w-0 flex-wrap items-center gap-2 rounded-md border border-(--theme-border-default) bg-(--theme-surface-panel) p-3 text-xs"
    >
      <strong>{engine}</strong>
      <span className="min-w-0 flex-1 break-all">{target}</span>
      <span>{state}</span>
      <span>{access}</span>
    </div>
  )
}
