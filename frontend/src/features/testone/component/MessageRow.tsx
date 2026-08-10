import { memo } from 'react';
import MessageBubble from '@testone/component/MessageBubble';
import { MessageItem } from '@testone/typing';
import classNames from 'classnames';

type MessageRowProps = {
  item: MessageItem;
  isOwnMessage: boolean;
  onDelete: (item: MessageItem) => void;
  onSend: (value: string) => void;
  onInput: (value: string) => void;
  onButtonClick?: (buttonId: string, buttonData: string) => void;
  onReact?: (emoji: string) => void;
  currentUserId?: string;
  selectMode?: boolean;
  selected?: boolean;
};

// 单个消息行组件
const MessageRow = memo(
  ({
    item,
    isOwnMessage,
    onDelete,
    onSend,
    onInput,
    onButtonClick,
    onReact,
    currentUserId,
    selectMode = false,
    selected = false
  }: MessageRowProps) => {
    console.log('MessageRow 渲染，消息创建时间:', item.CreateAt);
    return (
      <div
        className={classNames('flex gap-1 ', {
          'ml-auto flex-row-reverse': isOwnMessage,
          'mr-auto': !isOwnMessage,
          'opacity-80': selectMode,
          'pb-3 pt-2 px-2': selectMode && selected
        })}
      >
        {selectMode && (
          <div
            onClick={e => {
              e.stopPropagation();
              e.preventDefault();
              onDelete(item);
            }}
            className={classNames(
              'w-5 h-5 mt-3 rounded border flex items-center justify-center cursor-pointer select-none',
              selected
                ? 'bg-blue-500 border-blue-500 text-white'
                : 'border-gray-400 text-transparent'
            )}
          >
            ✓
          </div>
        )}
        {/* 用户头像 */}
        {item.UserAvatar ? (
          <img
            className="size-12 rounded-full"
            src={item.UserAvatar}
            alt="用户头像"
          />
        ) : (
          <div className="size-12 rounded-full bg-gray-300 flex items-center justify-center">
            <span className="text-gray-600 text-sm">用户</span>
          </div>
        )}

        {/* 消息气泡 */}
        <MessageBubble
          data={item.data}
          onSend={onSend}
          onInput={onInput}
          onButtonClick={onButtonClick}
          reactions={item.reactions}
          onReact={onReact}
          currentUserId={currentUserId}
          createAt={item.CreateAt}
        />
      </div>
    );
  },
  (prevProps, nextProps) => {
    // 仅当消息创建时间不同才重新渲染
    const isSame =
      prevProps.item.CreateAt === nextProps.item.CreateAt &&
      prevProps.item.reactions === nextProps.item.reactions &&
      prevProps.selected === nextProps.selected &&
      prevProps.selectMode === nextProps.selectMode;
    // isOwnMessage 变化了也要重新渲染
    if (prevProps.isOwnMessage !== nextProps.isOwnMessage) {
      return false;
    }
    return isSame;
  }
);

export default MessageRow;
