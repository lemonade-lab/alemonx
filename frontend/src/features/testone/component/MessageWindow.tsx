import { memo, useEffect, useRef, useCallback, useState } from 'react';
import { Code2, ListChecks, Pencil, Trash2, Undo2, X } from 'lucide-react';
import { MessageItem } from '@testone/typing';
import MessageRow from './MessageRow';
import { buildMessageKey } from '@testone/core/messageKey';
import VirtualizedList from './VirtualizedList';
import logger from '@testone/core/logger';
import { useViewportPopoverPosition } from '../../../hooks/useViewportPopoverPosition';

// 定义类型
type MessageWindowProps = {
  message: MessageItem[];
  onDelete: (item: MessageItem) => void; // 左键删除或在选择模式下切换选择
  onSend: (value: string) => void;
  onInput: (value: string) => void;
  onButtonClick?: (
    item: MessageItem,
    buttonId: string,
    buttonData: string
  ) => void;
  onReact?: (item: MessageItem, emoji: string) => void;
  onRecall?: (item: MessageItem) => void;
  onEdit?: (item: MessageItem) => void;
  onInspect?: (item: MessageItem) => void;
  UserId: string;
  selectMode?: boolean;
  selectedKeys?: string[];
  onConfirmDelete?: (item: MessageItem) => void; // 非选择模式删除当前
  onExitSelectMode?: () => void; // 退出选择模式
  onBulkDeleteSelected?: () => void; // 删除所选（内部会自动关闭选择模式）
  onEnterSelectMode?: (item: MessageItem) => void; // 非选择模式进入多选并选中当前
};

// 主消息窗口组件
function MessageWindow({
  message,
  onDelete,
  onSend = () => {},
  onInput = () => {},
  onButtonClick,
  onReact,
  onRecall,
  onEdit,
  onInspect,
  UserId,
  selectMode = false,
  selectedKeys = [],
  onConfirmDelete,
  onExitSelectMode,
  onBulkDeleteSelected,
  onEnterSelectMode
}: MessageWindowProps) {
  const MessageWindowRef = useRef<HTMLElement>(null);
  const [stickBottom, setStickBottom] = useState(true);
  const NEAR_BOTTOM_THRESHOLD = 80; // px 内视为粘底
  const [menu, setMenu] = useState<{
    x: number;
    y: number;
    item: MessageItem | null;
  } | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuStyle = useViewportPopoverPosition({
    anchor: menu ? new DOMRect(menu.x, menu.y, 0, 0) : null,
    open: Boolean(menu),
    popoverRef: menuRef
  });
  // 监听消息数量变化，自动滚动到底部
  const messageLength = message.length;
  const prevLenRef = useRef(messageLength);

  useEffect(() => {
    logger.info('message.len', prevLenRef.current, '->', messageLength);
    const decreased = prevLenRef.current > messageLength;
    prevLenRef.current = messageLength;
    if (decreased) return; // 数据减少不滚动
    if (!stickBottom) return; // 非粘底不滚动
    const el = MessageWindowRef.current;
    if (!el) return;
    requestAnimationFrame(() => {
      el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
    });
  }, [messageLength, stickBottom]);

  const handleScroll = useCallback(() => {
    const el = MessageWindowRef.current;
    if (!el) return;
    const distance = el.scrollHeight - (el.scrollTop + el.clientHeight);
    const nearBottom = distance <= NEAR_BOTTOM_THRESHOLD;
    setStickBottom(prev => (prev !== nearBottom ? nearBottom : prev));
  }, []);

  useEffect(() => {
    const el = MessageWindowRef.current;
    if (!el) return;
    el.addEventListener('scroll', handleScroll, { passive: true });
    handleScroll();
    return () => el.removeEventListener('scroll', handleScroll);
  }, [handleScroll]);

  const handleDelete = useCallback(
    (item: MessageItem) => onDelete(item),
    [onDelete]
  );
  const handleSend = useCallback((value: string) => onSend(value), [onSend]);
  const handleInput = useCallback((value: string) => onInput(value), [onInput]);

  const closeMenu = useCallback(() => setMenu(null), []);

  // 全局点击 / ESC 关闭菜单
  useEffect(() => {
    if (!menu) return;
    const previousFocus = document.activeElement;
    menuRef.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus({ preventScroll: true });
    const onGlobal = (e: MouseEvent) => {
      if (e.target instanceof Node && menuRef.current?.contains(e.target)) return;
      closeMenu();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      e.preventDefault();
      e.stopPropagation();
      closeMenu();
      if (previousFocus instanceof HTMLElement) previousFocus.focus({ preventScroll: true });
    };
    window.addEventListener('click', onGlobal, true);
    window.addEventListener('contextmenu', onGlobal, true);
    window.addEventListener('keydown', onKey, true);
    return () => {
      window.removeEventListener('click', onGlobal, true);
      window.removeEventListener('contextmenu', onGlobal, true);
      window.removeEventListener('keydown', onKey, true);
    };
  }, [menu, closeMenu]);

  // 如果没有消息，显示提示
  if (message.length === 0) {
    return (
      <section className="flex-1 flex flex-col overflow-y-auto scrollbar">
        <div className="flex-1 flex items-center justify-center text-(--theme-text-muted)">
          <p>暂无消息，开始聊天吧！</p>
        </div>
      </section>
    );
  }

  const useVirtual = message.length > 300;
  if (useVirtual) {
    return (
      <section
        ref={MessageWindowRef as any}
        className="flex-1 flex flex-col overflow-y-hidden"
      >
        <VirtualizedList
          className="flex-1 overflow-y-auto relative px-3 py-2"
          items={message}
          estimateHeight={80}
          itemKey={item => item.CreateAt}
          render={item => (
            <MessageRow
              key={item.CreateAt}
              item={item}
              isOwnMessage={UserId === item.UserId}
              onDelete={handleDelete}
              onSend={handleSend}
              onInput={handleInput}
              onButtonClick={(bid, bdata) => onButtonClick?.(item, bid, bdata)}
              onReact={emoji => onReact?.(item, emoji)}
              currentUserId={UserId}
            />
          )}
        />
      </section>
    );
  }

  return (
    <section
      ref={MessageWindowRef}
      className="flex-1 flex flex-col overflow-y-auto scrollbar"
    >
      <section className="flex-1 px-3 py-2 flex gap-4 flex-col">
        {message.map(item => {
          const key = `${item.UserId}:${item.CreateAt}`;
          const selected = selectedKeys.includes(key);
          return (
            <div
              key={buildMessageKey(item)}
              className={selected ? 'ring-2 ' : ''}
              onClick={e => {
                if (!selectMode) return;
                e.stopPropagation();
                e.preventDefault();
                handleDelete(item);
              }}
              onContextMenu={e => {
                e.preventDefault();
                e.stopPropagation();
                setMenu({ x: e.clientX, y: e.clientY, item });
              }}
            >
              <MessageRow
                item={item}
                isOwnMessage={UserId === item.UserId}
                onDelete={handleDelete}
                onSend={handleSend}
                onInput={handleInput}
                onButtonClick={(bid, bdata) =>
                  onButtonClick?.(item, bid, bdata)
                }
                onReact={emoji => onReact?.(item, emoji)}
                currentUserId={UserId}
                selectMode={selectMode}
                selected={selected}
              />
            </div>
          );
        })}
      </section>
      {menu && menu.item && (
        <div
          ref={menuRef}
          className="workbench-tools-menu workbench-side-submenu fixed"
          style={menuStyle}
          role="menu"
          aria-label="消息操作"
        >
          {selectMode ? (
            <>
              <button
                type="button"
                className="workbench-action-item"
                data-destructive
                disabled={!selectedKeys.length}
                onClick={() => {
                  if (!selectedKeys.length) return;
                  onBulkDeleteSelected && onBulkDeleteSelected();
                  // reducer 中已关闭选择模式
                  closeMenu();
                }}
              >
                <Trash2 className="size-4 shrink-0" aria-hidden="true" />
                删除({selectedKeys.length})
              </button>
              <button
                type="button"
                className="workbench-action-item"
                onClick={() => {
                  onExitSelectMode && onExitSelectMode();
                  closeMenu();
                }}
              >
                <X className="size-4 shrink-0" aria-hidden="true" />
                关闭
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                className="workbench-action-item"
                data-destructive
                onClick={() => {
                  if (onConfirmDelete) onConfirmDelete(menu.item!);
                  else onDelete(menu.item!);
                  closeMenu();
                }}
              >
                <Trash2 className="size-4 shrink-0" aria-hidden="true" />
                删除
              </button>
              {onRecall && (
                <button
                  type="button"
                  className="workbench-action-item"
                  data-destructive
                  onClick={() => {
                    onRecall(menu.item!);
                    closeMenu();
                  }}
                >
                  <Undo2 className="size-4 shrink-0" aria-hidden="true" />
                  撤回
                </button>
              )}
              {onEdit && UserId === menu.item?.UserId && (
                <button
                  type="button"
                  className="workbench-action-item"
                  onClick={() => {
                    onEdit(menu.item!);
                    closeMenu();
                  }}
                >
                  <Pencil className="size-4 shrink-0" aria-hidden="true" />
                  编辑
                </button>
              )}
              <button
                type="button"
                className="workbench-action-item"
                onClick={() => {
                  onEnterSelectMode && onEnterSelectMode(menu.item!);
                  closeMenu();
                }}
              >
                <ListChecks className="size-4 shrink-0" aria-hidden="true" />
                多选
              </button>
              {onInspect && (
                <button
                  type="button"
                  className="workbench-action-item"
                  onClick={() => {
                    onInspect(menu.item!);
                    closeMenu();
                  }}
                >
                  <Code2 className="size-4 shrink-0" aria-hidden="true" />
                  查看结构
                </button>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}

export default memo(MessageWindow);
