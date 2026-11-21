import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import cn from 'classnames'
import { Plus, Terminal, X } from 'lucide-react'
import { Button } from './Button'
import { directoryName } from '../lib/directoryName'

type TerminalTab = { id: string; cwd: string }

export function TerminalTabBar({ tabs, activeID, onSelect, onClose, onAdd }: {
  tabs: TerminalTab[]
  activeID?: string
  onSelect: (id: string) => void
  onClose: (id: string) => void
  onAdd?: () => void
}) {
  const available = useRef<HTMLDivElement>(null)
  const viewport = useRef<HTMLElement>(null)
  const track = useRef<HTMLDivElement>(null)
  const activeTab = useRef<HTMLDivElement>(null)
  const [overflow, setOverflow] = useState(false)

  const revealActiveTab = useCallback(() => {
    const scroller = viewport.current
    const tab = activeTab.current
    if (!scroller || !tab) return
    const bounds = scroller.getBoundingClientRect()
    const item = tab.getBoundingClientRect()
    if (item.left < bounds.left) scroller.scrollLeft += item.left - bounds.left
    else if (item.right > bounds.right) scroller.scrollLeft += item.right - bounds.right
  }, [])

  useLayoutEffect(() => {
    const space = available.current
    const list = track.current
    if (!space || !list) return
    const update = () => {
      setOverflow(list.scrollWidth > space.clientWidth)
      revealActiveTab()
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(space)
    observer.observe(list)
    return () => observer.disconnect()
  }, [revealActiveTab])

  useLayoutEffect(() => {
    revealActiveTab()
  }, [activeID, overflow, revealActiveTab])

  return (
    <div className="flex min-w-0 flex-1 items-center gap-1">
      <div ref={available} className="flex min-w-0 flex-1 items-center">
        <nav
          ref={viewport}
          className="terminal-tab-viewport min-w-0 flex-1 overflow-x-auto overflow-y-hidden"
          aria-label="终端列表"
        >
          <div ref={track} className="flex w-max items-center gap-1 px-1 py-1">
            {tabs.map(tab => {
              const label = directoryName(tab.cwd) || '终端'
              return (
                <div
                  key={tab.id}
                  ref={activeID === tab.id ? activeTab : undefined}
                  className={cn(
                    'flex shrink-0 items-center gap-1 rounded-md pr-1',
                    activeID === tab.id && 'bg-(--theme-surface-active)'
                  )}
                >
                  <button
                    className="flex min-h-8 max-w-36 items-center gap-1.5 rounded-md px-2 text-xs"
                    aria-pressed={activeID === tab.id}
                    onClick={() => onSelect(tab.id)}
                    title={tab.cwd}
                  >
                    <Terminal className="size-3.5 shrink-0" aria-hidden="true" />
                    <span className="truncate">{label}</span>
                  </button>
                  <button
                    className="grid size-6 place-items-center rounded-md hover:bg-(--theme-surface-hover)"
                    onClick={() => onClose(tab.id)}
                    aria-label={`关闭终端 ${label}`}
                  >
                    <X className="size-3" aria-hidden="true" />
                  </button>
                </div>
              )
            })}
          </div>
        </nav>
      </div>
      {onAdd && (
        <Button variant="icon" onClick={onAdd} aria-label="添加终端" title="添加终端">
          <Plus className="size-4" aria-hidden="true" />
        </Button>
      )}
    </div>
  )
}
