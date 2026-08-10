import { useState, useMemo, useCallback } from 'react';
import { Command } from '@testone/typing';
import SearchOutlined from '@ant-design/icons/SearchOutlined';
import CommandItem from './CommandItem';

/**
 * 侧栏指令列表（带搜索和分页, 每页50）
 */
const PAGE_SIZE = 50;

export default function SidebarCommandList({
  commands = [],
  onCommandSelect
}: {
  commands: Command[];
  onCommandSelect: (c: Command) => void;
}) {
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState('');

  const filtered = useMemo(() => {
    if (!search.trim()) return commands;
    const q = search.toLowerCase();
    return commands.filter(
      c =>
        c.title.toLowerCase().includes(q) ||
        c.description.toLowerCase().includes(q) ||
        c.text.toLowerCase().includes(q)
    );
  }, [commands, search]);

  const totalPages = useMemo(
    () => Math.ceil(filtered.length / PAGE_SIZE) || 1,
    [filtered.length]
  );

  if (page > 0 && page >= totalPages) {
    setPage(totalPages - 1);
  }

  const pageCommands = useMemo(() => {
    if (filtered.length <= PAGE_SIZE) return filtered;
    const start = page * PAGE_SIZE;
    return filtered.slice(start, start + PAGE_SIZE);
  }, [filtered, page]);

  const handlePrev = useCallback(() => {
    setPage(p => Math.max(0, p - 1));
  }, []);

  const handleNext = useCallback(() => {
    setPage(p => Math.min(totalPages - 1, p + 1));
  }, [totalPages]);

  if (!commands || commands.length === 0) {
    return (
      <div className="flex-col overflow-y-auto scrollbar mt-2 rounded-md gradient-border p-2 text-xs opacity-70">
        暂无指令
      </div>
    );
  }

  const hasPagination = filtered.length > PAGE_SIZE;

  return (
    <div className="flex flex-col mt-2 rounded-md gradient-border p-1 text-xs overflow-hidden">
      {/* 搜索框 */}
      <div className="flex items-center gap-1 px-1 py-0.5 border-b border-[var(--dropdown-border)]">
        <SearchOutlined className="text-[var(--descriptionForeground)]" />
        <input
          className="flex-1 bg-transparent outline-none text-[var(--foreground)]"
          placeholder="搜索指令..."
          value={search}
          onChange={e => {
            setSearch(e.target.value);
            setPage(0);
          }}
        />
      </div>
      <div className="overflow-y-auto scrollbar flex-1">
        {pageCommands.map((command, index) => (
          <CommandItem
            key={(command as any).id ?? `${page}-${index}`}
            command={command}
            onCommandSelect={onCommandSelect}
          />
        ))}
      </div>
      {filtered.length === 0 && search.trim() && (
        <div className="text-center text-[var(--descriptionForeground)] py-2">
          未找到匹配指令
        </div>
      )}
      {hasPagination && (
        <div className="flex items-center justify-center gap-2 pt-1 border-t border-[var(--dropdown-border)] mt-1 select-none">
          <button
            className="px-1 rounded hover:bg-[var(--dropdown-border)] disabled:opacity-30"
            disabled={page === 0}
            onClick={handlePrev}
          >
            上一页
          </button>
          <span>
            {page + 1}/{totalPages}
          </span>
          <button
            className="px-1 rounded hover:bg-[var(--dropdown-border)] disabled:opacity-30"
            disabled={page + 1 >= totalPages}
            onClick={handleNext}
          >
            下一页
          </button>
        </div>
      )}
    </div>
  );
}
