import * as _ from 'lodash-es';
import SlateTextarea from '@testone/component/SlateTextarea';
import CommandList from '@testone/component/CommandList';
import PopoverBox from '@testone/ui/PopoverBox';
import ClearOutlined from '@ant-design/icons/ClearOutlined';
import OrderedListOutlined from '@ant-design/icons/OrderedListOutlined';
import KeyOutlined from '@ant-design/icons/KeyOutlined';
import SendOutlined from '@ant-design/icons/SendOutlined';
import CloseOutlined from '@ant-design/icons/CloseOutlined';
import { useState, useCallback } from 'react';
import { Command, User, Channel } from '@testone/typing';
import {
  closeImageCompression,
  isImageCompressionOpen,
  openImageCompression
} from '@testone/core/imageStore';
import CheckCircleOutlined from '@ant-design/icons/CheckCircleOutlined';
import CloseCircleOutlined from '@ant-design/icons/CloseCircleOutlined';
import { Message } from '@testone/core/message';
import type { Descendant } from 'slate';
import type { DataEnums } from '@testone/typing';
import { useAppSelector, safeSend } from '@testone/store';
import { userHashKey, Platform } from '@testone/core/alemon';

/* ─── 特殊事件定义 ─── */
type SpecialEventDef = {
  name: string;
  label: string;
  desc: string;
  scope: 'public' | 'private' | 'both';
  needText: boolean;
};

const SPECIAL_EVENTS: SpecialEventDef[] = [
  {
    name: 'notice.create',
    label: '戳一戳',
    desc: '公共通知（戳一戳、运气王等）',
    scope: 'public',
    needText: true
  },
  {
    name: 'private.notice.create',
    label: '私聊通知',
    desc: '私有通知事件',
    scope: 'private',
    needText: true
  },
  {
    name: 'interaction.create',
    label: '交互事件',
    desc: '按钮/菜单等交互',
    scope: 'public',
    needText: true
  },
  {
    name: 'private.interaction.create',
    label: '私聊交互',
    desc: '私聊场景下的交互事件',
    scope: 'private',
    needText: true
  },
  {
    name: 'member.add',
    label: '成员加入',
    desc: '新成员加入服务器',
    scope: 'public',
    needText: false
  },
  {
    name: 'member.remove',
    label: '成员退出',
    desc: '成员离开服务器',
    scope: 'public',
    needText: false
  },
  {
    name: 'member.ban',
    label: '成员封禁',
    desc: '成员被封禁',
    scope: 'public',
    needText: false
  },
  {
    name: 'member.unban',
    label: '成员解封',
    desc: '成员被解封',
    scope: 'public',
    needText: false
  },
  {
    name: 'message.reaction.add',
    label: '添加表情',
    desc: '对消息添加表情反应',
    scope: 'public',
    needText: true
  },
  {
    name: 'message.reaction.remove',
    label: '移除表情',
    desc: '移除消息的表情反应',
    scope: 'public',
    needText: true
  },
  {
    name: 'private.friend.add',
    label: '好友申请',
    desc: '收到好友添加请求',
    scope: 'private',
    needText: false
  },
  {
    name: 'private.friend.remove',
    label: '好友删除',
    desc: '好友被删除',
    scope: 'private',
    needText: false
  },
  {
    name: 'guild.join',
    label: '加入服务器',
    desc: '机器人加入新服务器',
    scope: 'public',
    needText: false
  },
  {
    name: 'guild.exit',
    label: '退出服务器',
    desc: '机器人被移出服务器',
    scope: 'public',
    needText: false
  },
  {
    name: 'channel.create',
    label: '频道创建',
    desc: '新频道被创建',
    scope: 'public',
    needText: false
  },
  {
    name: 'channel.delete',
    label: '频道删除',
    desc: '频道被删除',
    scope: 'public',
    needText: false
  }
];

const InputBox = ({
  value,
  commands,
  userList,
  channelList,
  pageType = 'public',
  onInput,
  onSend,
  onSendFormat,
  onClear,
  onSelect,
  onCommand,
  onTimer,
  onCancelSelect,
  onSlateChange,
  getSlateValue,
  onHistoryPrev,
  onHistoryNext,
  selectMode = false,
  selectedCount = 0
}: {
  value: string;
  commands: Command[];
  userList: User[];
  channelList?: Channel[];
  pageType?: 'public' | 'private';
  onInput: (value: string) => void;
  onSend: () => void;
  onSendFormat?: (content: DataEnums[]) => void;
  onClear: () => void;
  onSelect: () => void;
  onCommand: (command: Command) => void;
  onTimer: () => void;
  onCancelSelect?: () => void;
  onSlateChange?: (nodes: Descendant[]) => void;
  getSlateValue?: React.MutableRefObject<() => Descendant[]>;
  onHistoryPrev?: (currentText?: string) => string | null;
  onHistoryNext?: () => string | null;
  selectMode?: boolean;
  selectedCount?: number;
}) => {
  const [showCommands, setShowCommands] = useState(false);
  const [showClearPopover, setShowClearPopover] = useState(false);
  const [showEvents, setShowEvents] = useState(false);
  const [eventText, setEventText] = useState('');
  const [eventSent, setEventSent] = useState<string | null>(null);
  const [open, setOpen] = useState(isImageCompressionOpen());

  const currentUser = useAppSelector(s => s.users.current);
  const currentChannel = useAppSelector(s => s.channels.current);
  const connected = useAppSelector(s => s.socket.connected);

  const filteredEvents = SPECIAL_EVENTS.filter(
    ev =>
      ev.scope === 'both' ||
      ev.scope === pageType ||
      (pageType === 'public' && ev.scope === 'public') ||
      (pageType === 'private' && ev.scope === 'private')
  );

  const sendSpecialEvent = useCallback(
    (ev: SpecialEventDef) => {
      if (!connected) {
        Message.warning('请先连接到服务端');
        return;
      }
      if (!currentUser) {
        Message.warning('缺少当前用户信息');
        return;
      }
      const needChannel = ev.scope === 'public' || ev.scope === 'both';
      if (needChannel && !currentChannel) {
        Message.warning('该事件需要先选择频道');
        return;
      }

      const UserKey = userHashKey({
        Platform,
        UserId: currentUser.UserId
      });

      const payload: Record<string, unknown> = {
        name: ev.name,
        Platform,
        UserId: currentUser.UserId,
        UserKey,
        UserName: currentUser.UserName,
        UserAvatar: currentUser.UserAvatar,
        IsMaster: currentUser.IsMaster ?? false,
        IsBot: false,
        MessageId: Date.now().toString(),
        CreateAt: Date.now(),
        value: null
      };

      if (needChannel && currentChannel) {
        payload.ChannelId = currentChannel.ChannelId;
        payload.GuildId = currentChannel.GuildId || currentChannel.ChannelId;
        payload.tag = currentChannel.ChannelId;
      }

      if (ev.needText) {
        payload.MessageText = eventText;
        payload.OpenId = (currentUser as any).OpenId || '';
      }

      safeSend(payload);
      setEventSent(ev.label);
      setTimeout(() => setEventSent(null), 1500);
    },
    [connected, currentUser, currentChannel, eventText]
  );
  const updateImageCompression = () => {
    const open = isImageCompressionOpen();
    if (open) {
      Message.success('资源压缩已开启');
    } else {
      Message.success('资源压缩已关闭');
    }
    setOpen(open);
  };
  return (
    <div className="relative">
      <CommandList
        commands={commands}
        onCommandSelect={onCommand}
        open={showCommands}
        onClose={() => setShowCommands(false)}
        onTimer={onTimer}
      />
      <PopoverBox
        open={showClearPopover}
        onClose={() => setShowClearPopover(false)}
        direction="right"
        className="min-w-24"
      >
        <div className="px-2">
          {!selectMode && (
            <div
              className="flex cursor-pointer gap-1"
              onClick={() => onSelect()}
            >
              <OrderedListOutlined />
              选择删除
            </div>
          )}
          {selectMode && (
            <div className="flex flex-col gap-1">
              <div
                className="flex cursor-pointer gap-1"
                onClick={() => onSelect()}
              >
                <OrderedListOutlined /> 删除所选({selectedCount})
              </div>
              <div
                className="flex justify-end cursor-pointer gap-1 text-xs text-gray-500"
                onClick={() => onCancelSelect?.()}
              >
                取消选择
              </div>
            </div>
          )}
          <div className="flex cursor-pointer gap-1" onClick={() => onClear()}>
            <ClearOutlined />
            删除所有
          </div>
          <div
            className="flex cursor-pointer gap-1"
            onClick={() => {
              if (open) {
                closeImageCompression();
              } else {
                openImageCompression();
              }
              updateImageCompression();
            }}
          >
            <KeyOutlined />
            资源压缩
            {open ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
          </div>
        </div>
      </PopoverBox>
      {/* 特殊事件面板 */}
      {showEvents && (
        <div className="absolute bottom-full left-0 right-0 z-20 mb-1 bg-[var(--editor-background)] border border-[var(--panel-border)] rounded-lg shadow-lg overflow-hidden">
          <div className="flex items-center justify-between px-3 py-1.5 border-b border-[var(--panel-border)] bg-[var(--activityBar-background)]">
            <span className="text-xs font-semibold text-[var(--editor-foreground)]">
              ⚡ 特殊事件 — {pageType === 'public' ? '群聊' : '私聊'}
            </span>
            <CloseOutlined
              className="cursor-pointer text-[var(--descriptionForeground)] hover:text-[var(--foreground)] text-xs"
              onClick={() => setShowEvents(false)}
            />
          </div>
          <div className="px-3 py-1.5 border-b border-[var(--panel-border)]">
            <input
              className="w-full px-2 py-1 rounded text-xs bg-[var(--input-background)] text-[var(--input-foreground)] border border-[var(--input-border)] outline-none focus:border-[var(--button-background)]"
              placeholder="附加文本（戳一戳文案、交互ID、表情等）"
              value={eventText}
              onChange={e => setEventText(e.target.value)}
            />
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-1.5 p-2 max-h-48 overflow-y-auto scrollbar">
            {filteredEvents.map(ev => (
              <button
                key={ev.name}
                className={
                  'flex flex-col items-start px-2 py-1.5 rounded text-left text-xs ' +
                  'border border-[var(--panel-border)] ' +
                  'hover:bg-[var(--activityBar-background)] hover:border-[var(--button-background)] ' +
                  'transition-colors cursor-pointer ' +
                  (!connected ? 'opacity-40 cursor-not-allowed' : '')
                }
                onClick={() => sendSpecialEvent(ev)}
                disabled={!connected}
                title={ev.name}
              >
                <span className="font-medium text-[var(--editor-foreground)]">
                  <SendOutlined className="mr-1 text-[10px]" />
                  {ev.label}
                </span>
                <span className="text-[10px] text-[var(--descriptionForeground)] leading-tight">
                  {ev.desc}
                </span>
              </button>
            ))}
          </div>
          {eventSent && (
            <div className="px-3 py-1 text-xs text-green-400 border-t border-[var(--panel-border)]">
              ✓ 已发送「{eventSent}」
            </div>
          )}
        </div>
      )}
      <SlateTextarea
        value={value}
        onContentChange={onInput}
        onSlateChange={onSlateChange}
        onSendFormat={onSendFormat}
        getSlateValue={getSlateValue}
        onClickSend={onSend}
        onHistoryPrev={onHistoryPrev}
        onHistoryNext={onHistoryNext}
        onAppClick={action => {
          if (action === 'commands') {
            setShowCommands(!showCommands);
          }
          if (action === 'chatlogs') {
            setShowClearPopover(true);
          }
          if (action === 'events') {
            setShowEvents(v => !v);
          }
        }}
        userList={userList}
        channelList={channelList}
      />
    </div>
  );
};

export default InputBox;
