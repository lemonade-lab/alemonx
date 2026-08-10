import { memo, useEffect, useRef, useCallback, useState } from 'react';
import { MessageItem } from '@testone/typing';
import MessageRow from './MessageRow';
import { buildMessageKey } from '@testone/core/messageKey';
import VirtualizedList from './VirtualizedList';
import logger from '@testone/core/logger';
import { Button } from '../ui/Button';

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
    const onGlobal = (e: MouseEvent) => {
      const target = e.target as HTMLElement;
      if (target.closest && target.closest('.msg-context-menu')) return;
      closeMenu();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeMenu();
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
        <div className="flex-1 flex items-center justify-center text-gray-500">
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
          className="msg-context-menu fixed z-50  text-sm text-[var(--foreground,#eee)] shadow-lg rounded-md  p-2 flex flex-col gap-2 min-w-20"
          style={{ top: menu.y + 4, left: menu.x + 4 }}
        >
          {selectMode ? (
            <>
              <Button
                className="text-xs"
                disabled={!selectedKeys.length}
                onClick={() => {
                  if (!selectedKeys.length) return;
                  onBulkDeleteSelected && onBulkDeleteSelected();
                  // reducer 中已关闭选择模式
                  closeMenu();
                }}
              >
                删除({selectedKeys.length})
              </Button>
              <Button
                className="text-xs"
                onClick={() => {
                  onExitSelectMode && onExitSelectMode();
                  closeMenu();
                }}
              >
                关闭
              </Button>
            </>
          ) : (
            <>
              <Button
                className="text-xs"
                onClick={() => {
                  if (onConfirmDelete) onConfirmDelete(menu.item!);
                  else onDelete(menu.item!);
                  closeMenu();
                }}
              >
                删除
              </Button>
              {onRecall && (
                <Button
                  className="text-xs"
                  onClick={() => {
                    onRecall(menu.item!);
                    closeMenu();
                  }}
                >
                  撤回
                </Button>
              )}
              {onEdit && UserId === menu.item?.UserId && (
                <Button
                  className="text-xs"
                  onClick={() => {
                    onEdit(menu.item!);
                    closeMenu();
                  }}
                >
                  编辑
                </Button>
              )}
              <Button
                className="text-xs"
                onClick={() => {
                  onEnterSelectMode && onEnterSelectMode(menu.item!);
                  closeMenu();
                }}
              >
                多选
              </Button>
              {onInspect && (
                <Button
                  className="text-xs"
                  onClick={() => {
                    onInspect(menu.item!);
                    closeMenu();
                  }}
                >
                  查看结构
                </Button>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}

export default memo(MessageWindow);
