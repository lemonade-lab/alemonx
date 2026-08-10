import { memo, useState, useMemo, useCallback, useEffect } from 'react';
import { Command } from '@testone/typing';
import CloseCircleOutlined from '@ant-design/icons/CloseCircleOutlined';
import FieldTimeOutlined from '@ant-design/icons/FieldTimeOutlined';
import SearchOutlined from '@ant-design/icons/SearchOutlined';
import CommandItem from './CommandItem';
import PopoverBox from '../ui/PopoverBox';

interface Props {
  commands: Command[];
  onCommandSelect: (command: Command) => void;
  onTimer?: () => void;
  onClose?: () => void;
  open: boolean;
}

const PAGE_SIZE = 50;

const CommandList = ({
  commands = [],
  onCommandSelect,
  onTimer,
  onClose,
  open
}: Props) => {
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState('');

  // 过滤指令
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

  // 当 commands 变化时重置页码，避免越界
  const totalPages = useMemo(
    () => Math.ceil(filtered.length / PAGE_SIZE) || 1,
    [filtered.length]
  );
  // 修正页码（放入 effect 避免 render 期间 setState）
  useEffect(() => {
    if (page > 0 && page >= totalPages) {
      setPage(totalPages - 1);
    }
  }, [page, totalPages]);

  // 搜索变化时重置页码
  useEffect(() => {
    setPage(0);
  }, [search]);

  const pageCommands = useMemo(() => {
    if (filtered.length <= PAGE_SIZE) return filtered;
    const start = page * PAGE_SIZE;
    return filtered.slice(start, start + PAGE_SIZE);
  }, [filtered, page]);

  const handlePrev = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    e.preventDefault();
    setPage(p => Math.max(0, p - 1));
  }, []);

  const handleNext = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      e.preventDefault();
      setPage(p => Math.min(totalPages - 1, p + 1));
    },
    [totalPages]
  );

  if (!open || commands.length === 0) return null;

  const hasPagination = filtered.length > PAGE_SIZE;
  const listMaxHeightClass = hasPagination ? 'max-h-40' : 'max-h-48'; // 40=>10rem, 48=>12rem

  return (
    <PopoverBox open={open} onClose={onClose} className="w-56">
      <div className="flex justify-between text-xs text-[var(--descriptionForeground)] border-b border-[var(--dropdown-border)] p-1">
        <div className="flex items-center gap-1">
          <span>指令列表</span>
          {commands.length > PAGE_SIZE && (
            <span className="opacity-70">
              {filtered.length}/{commands.length}
            </span>
          )}
        </div>
        <div className="flex gap-2 items-center">
          <div
            className="cursor-pointer"
            onClick={e => {
              e.stopPropagation();
              e.preventDefault();
              onTimer?.();
            }}
          >
            <FieldTimeOutlined />
          </div>
          <div
            onClick={e => {
              e.stopPropagation();
              e.preventDefault();
              onClose?.();
            }}
            className="cursor-pointer"
          >
            <CloseCircleOutlined />
          </div>
        </div>
      </div>
      {/* 搜索框 */}
      <div className="flex items-center gap-1 px-1 py-0.5 border-b border-[var(--dropdown-border)]">
        <SearchOutlined className="text-[var(--descriptionForeground)] text-xs" />
        <input
          className="flex-1 bg-transparent outline-none text-xs text-[var(--foreground)]"
          placeholder="搜索指令..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          onClick={e => e.stopPropagation()}
          onKeyDown={e => e.stopPropagation()}
        />
      </div>
      <div className="shadow-inner rounded-md py-1 flex flex-col">
        <div
          className={`overflow-y-auto scrollbar ${listMaxHeightClass} flex-1`}
        >
          {pageCommands.map((command, index) => (
            <CommandItem
              key={(command as any).id ?? `${page}-${index}`}
              command={command}
              onCommandSelect={c => {
                onCommandSelect(c);
                onClose?.();
              }}
            />
          ))}
        </div>
        {hasPagination && (
          <div className="flex items-center justify-center gap-2 pt-1 border-t border-[var(--dropdown-border)] mt-1 text-xs select-none">
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
        {filtered.length === 0 && search.trim() && (
          <div className="text-xs text-center text-[var(--descriptionForeground)] py-2">
            未找到匹配指令
          </div>
        )}
      </div>
    </PopoverBox>
  );
};

export default memo(CommandList);
