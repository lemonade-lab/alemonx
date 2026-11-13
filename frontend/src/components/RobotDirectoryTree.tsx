import { ChevronRight, Folder } from 'lucide-react'
import { useId, useMemo, useState, type ReactNode } from 'react'

type Robot = { id: string; path: string; pinned?: boolean }

type Directory<T> = {
  key: string
  label: string
  count: number
  robots: T[]
  children: Map<string, Directory<T>>
}

function directory<T>(key = '', label = ''): Directory<T> {
  return { key, label, count: 0, robots: [], children: new Map() }
}

function buildTree<T extends Robot>(robots: T[]) {
  const root = directory<T>()
  for (const robot of robots) {
    if (robot.pinned) continue
    const windows = /^[a-z]:[\\/]|^\\\\/i.test(robot.path)
    const path = windows ? robot.path.replace(/\\/g, '/') : robot.path
    const prefix = path.startsWith('//') ? '//' : path.startsWith('/') ? '/' : ''
    const parts = path.split('/').filter(Boolean).slice(0, -1)
    let node = root
    let key = prefix
    for (const [index, part] of parts.entries()) {
      key += `${index ? '/' : ''}${part}`
      const identity = windows ? key.toLowerCase() : key
      let child = node.children.get(identity)
      if (!child) {
        child = directory<T>(identity, index === 0 ? prefix + part : part)
        node.children.set(identity, child)
      }
      child.count++
      node = child
    }
    node.robots.push(robot)
  }
  return root
}

export function RobotDirectoryTree<T extends Robot>({
  robots,
  renderRobot
}: {
  robots: T[]
  renderRobot: (robot: T) => ReactNode
}) {
  const id = useId()
  const tree = useMemo(() => buildTree(robots), [robots])
  const pinned = robots.filter(robot => robot.pinned)
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})

  const renderLeaves = (leaves: T[]) =>
    leaves.map(robot => <li key={robot.id}>{renderRobot(robot)}</li>)

  function renderContents(node: Directory<T>): ReactNode {
    return (
      <>
        {[...node.children.values()].map(renderDirectory)}
        {renderLeaves(node.robots)}
      </>
    )
  }

  function renderDirectory(start: Directory<T>) {
    let node = start
    let label = start.label
    while (node.robots.length === 0 && node.children.size === 1) {
      node = node.children.values().next().value!
      label += `/${node.label}`
    }
    const key = node.key
    const open = !collapsed[key]
    const contentsID = `${id}-${encodeURIComponent(key)}`
    return (
      <li key={start.key}>
        <button
          type="button"
          className="robot-directory-toggle"
          title={key}
          aria-label={`目录 ${label}`}
          aria-expanded={open}
          aria-controls={contentsID}
          onClick={() => setCollapsed(current => ({ ...current, [key]: open }))}
        >
          <ChevronRight className={`size-3 shrink-0 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
          <Folder className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate">{label}</span>
          <small>{node.count}</small>
        </button>
        <ul id={contentsID} hidden={!open} className="robot-directory-children">
          {renderContents(node)}
        </ul>
      </li>
    )
  }

  return (
    <nav aria-label="机器人列表" className="robot-directory-tree">
      {pinned.length > 0 && (
        <section aria-label="已置顶机器人" className="pb-1">
          <div className="px-2 py-1 text-[11px] text-(--theme-text-muted)">已置顶</div>
          <ul>{renderLeaves(pinned)}</ul>
        </section>
      )}
      <ul>{renderContents(tree)}</ul>
    </nav>
  )
}
