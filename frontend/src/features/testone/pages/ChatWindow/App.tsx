import { useState, useMemo, useCallback, lazy, Suspense, useRef } from 'react';
import { DataEnums } from '@testone/typing';
import type { Descendant } from 'slate';
import * as _ from 'lodash-es';
import { Channel, Command, MessageItem } from '@testone/typing';
import SearchOutlined from '@ant-design/icons/SearchOutlined';
import CloseOutlined from '@ant-design/icons/CloseOutlined';
import {
  makeSelectMessagesByType,
  makeSelectUserList,
  makeSelectHeader
} from '@testone/store/selectors';
import MessageWindow from '@testone/component/MessageWindow';
import MessageHeader from '@testone/component/MessageHeader';
import ChannelSelect from '@testone/pages/common/ChannelSelect';
import ChannelItem from '@testone/pages/common/ChannelItem';
import { Message } from '@testone/core/message';
const ModalCommandTimer = lazy(
  () => import('@testone/pages/modals/ModalCommandTomer')
);
const ModalStopAllConfirm = lazy(
  () => import('@testone/pages/modals/ModalStopAllConfirm')
);
const ModalTaskDeleteConfirm = lazy(
  () => import('@testone/pages/modals/ModalTaskDeleteConfirm')
);
const ModalEditMessage = lazy(
  () => import('@testone/pages/modals/ModalEditMessage')
);
const ModalReactionList = lazy(
  () => import('@testone/pages/modals/ModalReactionList')
);
const PayloadInspector = lazy(
  () => import('@testone/component/PayloadInspector')
);
const EventLogPanel = lazy(() => import('@testone/pages/EventLog/App'));
import TimerManager from '@testone/pages/common/TimerManager';
import { useAppDispatch, useAppSelector } from '@testone/store';
import {
  sendGroupFormat,
  sendPrivateFormat,
  sendInteraction,
  sendMessageDelete,
  sendMessageUpdate,
  sendReactionAdd,
  sendReactionRemove
} from '@testone/store/sendMessage';
import {
  clearGroupMessages,
  clearPrivateMessages,
  deleteGroupMessage,
  deletePrivateMessage,
  addReaction,
  removeReaction
} from '@testone/store/slices/chatSlice';
import { setCurrentChannel } from '@testone/store/slices/channelSlice';
import useCommandTimer from '@testone/hook/useCommandTimer';
import { parseMessageSmart } from '../../core/asyncParse';
import { serializeToDataEnums } from '@testone/component/slate/serialize';
import { useInputHistory } from '@testone/hook/useInputHistory';
import UserInfo from '../common/UserInfo';
import SidebarCommandList from '@testone/component/SidebarCommandList';
import InputBox from '../common/InputBox';
import TimerManagerTip from '../common/TimerManagerTip';
import {
  setSelectMode,
  toggleSelectMessage,
  deleteSelectedMessages,
  updateMessage
} from '@testone/store/slices/chatSlice';
import { setCurrentUser } from '@testone/store/slices/userSlice';

export default function ChatWindow({
  pageType = 'public'
}: {
  pageType: 'public' | 'private';
}) {
  const dispatch = useAppDispatch();
  const selectMessages = useMemo(
    () => makeSelectMessagesByType(pageType),
    [pageType]
  );
  const { connected: status, isConnecting, isRestarting } = useAppSelector(
    s => s.socket
  );
  const { channels, current: channel } = useAppSelector(s => s.channels);
  const { users, current: user } = useAppSelector(s => s.users);
  const { commands } = useAppSelector(s => s.commands);
  const { selectMode, selectedKeys } = useAppSelector(s => s.chat);

  const [open, setOpen] = useState(false);
  const [openStopAllConfirm, setOpenStopAllConfirm] = useState(false);

  const selectUserList = useMemo(
    () => makeSelectUserList(pageType),
    [pageType]
  );
  const selectHeader = useMemo(() => makeSelectHeader(pageType), [pageType]);
  const message = useAppSelector(selectMessages);
  const userList = useAppSelector(selectUserList);
  const headerMessage = useAppSelector(selectHeader);

  const onSendFormat = useCallback(
    (format: DataEnums[], { curchannel = channel, curuser = user } = {}) => {
      if (!status) {
        // 与底部状态同一变量：连接阶段与失败/未连接阶段分别提示，
        // 避免“连接中”却提示“未连接”的歧义。
        if (isConnecting || isRestarting) {
          Message.warning('正在连接本机测试服务，请稍候…');
        } else {
          Message.warning('未连接本机测试服务，无法发送消息');
        }
        return;
      }
      if (pageType === 'public') {
        if (!curchannel) return;
        dispatch(
          // 群里发送
          sendGroupFormat({
            currentChannel: curchannel,
            content: format
          })
        );
      } else {
        if (!curuser) return;
        //
        dispatch(
          sendPrivateFormat({
            currentUser: curuser,
            content: format
          })
        );
      }
    },
    [pageType, channel, user, dispatch, status, isConnecting, isRestarting]
  );

  const onSend = useCallback(
    (text: string, { curchannel = channel, curuser = user } = {}) => {
      parseMessageSmart({ input: text, users, channels }).then(
        (content: any) => {
          onSendFormat(content, { curchannel, curuser });
        }
      );
    },
    [users, channels, onSendFormat, channel, user]
  );

  const onDelete = useCallback(
    (item: MessageItem) => {
      if (selectMode) {
        // 选择模式下改为选择/取消
        dispatch(toggleSelectMessage(item));
        return;
      }
      if (pageType === 'public') dispatch(deleteGroupMessage(item));
      else dispatch(deletePrivateMessage(item));
    },
    [pageType, dispatch, selectMode]
  );

  const onClear = useCallback(() => {
    if (pageType === 'public') dispatch(clearGroupMessages());
    else dispatch(clearPrivateMessages());
  }, [pageType, dispatch]);

  const onSelect = useCallback(
    (channel: Channel) => {
      dispatch(setCurrentChannel(channel));
    },
    [dispatch]
  );

  const [value, onInput] = useState('');
  const inputHistory = useInputHistory();
  const getSlateValueRef = useRef<() => Descendant[]>(() => []);
  const [editingItem, setEditingItem] = useState<MessageItem | null>(null);
  const [reactionMessage, setReactionMessage] = useState<MessageItem | null>(
    null
  );
  const [reactionEmoji, setReactionEmoji] = useState<string | null>(null);
  const [inspectingItem, setInspectingItem] = useState<MessageItem | null>(
    null
  );
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');

  /** 按关键词过滤消息 */
  const filteredMessage = useMemo(() => {
    if (!searchOpen || !searchQuery.trim()) return message;
    const q = searchQuery.toLowerCase();
    return message.filter(m =>
      m.data.some(d => {
        if (
          d.type === 'Text' ||
          d.type === 'Markdown' ||
          d.type === 'MarkdownOriginal'
        ) {
          return String(d.value).toLowerCase().includes(q);
        }
        if (d.type === 'Mention') {
          return String((d as any).options?.UserName || d.value)
            .toLowerCase()
            .includes(q);
        }
        return false;
      })
    );
  }, [message, searchOpen, searchQuery]);

  const onEdit = useCallback((item: MessageItem) => {
    setEditingItem(item);
  }, []);

  const onEditConfirm = useCallback(
    (newContent: DataEnums[]) => {
      if (!editingItem) return;
      const scope = pageType === 'public' ? 'public' : 'private';
      const msgId = editingItem.MessageId || editingItem.CreateAt.toString();

      dispatch(
        updateMessage({
          scope: scope === 'public' ? 'group' : 'private',
          CreateAt: editingItem.CreateAt,
          UserId: editingItem.UserId,
          data: newContent
        })
      );

      dispatch(
        sendMessageUpdate({
          scope,
          messageId: msgId,
          messageCreateAt: editingItem.CreateAt,
          content: newContent
        })
      );

      setEditingItem(null);
    },
    [editingItem, pageType, dispatch]
  );

  const handleCommand = useMemo(
    () =>
      _.throttle((command: Command) => {
        if (!status) {
          Message.warning(
            isConnecting || isRestarting
              ? '正在连接本机测试服务，请稍候…'
              : '未连接本机测试服务，无法执行指令'
          );
          return;
        }
        if (typeof command.autoEnter === 'boolean' && !command.autoEnter) {
          if (!command.data) {
            onInput(command.text);
            return;
          }
          onSendFormat(command.data);
          return;
        }
        if (!command.data) {
          onSend(command.text);
          return;
        }
        onSendFormat(command.data);
      }, 400),
    [status, onSendFormat, onSend, isConnecting, isRestarting]
  );

  const onCommand = useCallback(
    (command: Command, { curchannel = channel, curuser = user } = {}) => {
      if (!status) {
        Message.warning(
          isConnecting || isRestarting
            ? '正在连接本机测试服务，请稍候…'
            : '未连接本机测试服务，无法执行指令'
        );
        return;
      }
      if (!command.data) {
        onSend(command.text, { curchannel, curuser });
        return;
      }
      onSendFormat(command.data, { curchannel, curuser });
    },
    [status, onSend, onSendFormat, channel, user, isConnecting, isRestarting]
  );

  const handleChannelSelect = useMemo(
    () =>
      _.throttle((c: Channel) => {
        onSelect(c);
      }, 400),
    [onSelect]
  );

  const {
    // 选择删除的任务
    taskToDelete,
    // 删除单个任务
    setTaskToDelete,
    // 定时器配置
    timerConfig,
    // 更新定时器
    setTimerConfig,
    // 提交定时器
    onSubmitTimer,
    // 停止所有任务
    stopAllCommandTasks,
    // 删除单个任务
    deleteSingleTask,
    // 定时器管理
    timerManager,
    // 所有任务
    commandTasksInfo
  } = useCommandTimer({
    commands,
    status,
    pageType,
    onCommand: onCommand,
    // 启动任务
    onStart: () => {
      setOpen(false);
    },
    // 关闭任务
    onClose: () => setOpenStopAllConfirm(false)
  });

  return (
    <div className="flex-1 flex flex-row overflow-y-auto scrollbar bg-[var(--sideBar-background)]">
      {
        // 消息管理
      }
      {pageType === 'public' && (
        <section className="testone-channel-rail w-56 flex flex-col bg-[var(--editorWidget-background)] border-r border-[--panel-border] p-2 gradient-border">
          <div className="flex-1 flex-col overflow-y-auto scrollbar gap-2">
            {Array.isArray(channels) &&
              channels.map((c, index) => (
                <ChannelItem
                  key={index}
                  channel={c}
                  onSelect={handleChannelSelect}
                />
              ))}
          </div>
          {
            // 个人信息
          }
          <div className="border-t border-[--panel-border] pt-2">
            <UserInfo
              users={users}
              onSelect={user => {
                dispatch(setCurrentUser(user));
              }}
              user={
                user || {
                  UserId: '',
                  UserName: '',
                  UserAvatar: '',
                  IsMaster: false,
                  IsBot: false
                }
              }
            />
          </div>
        </section>
      )}
      <section className="flex-1 flex flex-col overflow-y-auto scrollbar">
        <MessageHeader value={headerMessage}>
          <div className="flex items-center px-4 gap-2">
            {pageType === 'public' && (
              <ChannelSelect
                value={channel?.ChannelId || ''}
                channels={channels}
                onSelect={handleChannelSelect}
              />
            )}
          </div>
          <TimerManagerTip
            open={timerManager.hasRunningTasks}
            count={timerManager.getTaskCount().running}
            addTask={() => {
              setOpen(true);
            }}
            onClose={() => {
              if (timerManager.hasRunningTasks) {
                setOpenStopAllConfirm(true);
              } else {
                Message.warning('没有正在运行的任务');
              }
            }}
          />
          <div
            className="cursor-pointer px-2 flex items-center"
            onClick={() => {
              setSearchOpen(v => {
                if (v) setSearchQuery('');
                return !v;
              });
            }}
            title="搜索消息"
          >
            <SearchOutlined />
          </div>
        </MessageHeader>
        {/* 搜索栏 */}
        {searchOpen && (
          <div className="flex items-center gap-2 px-3 py-1 border-b border-[var(--panel-border)] bg-[var(--editorWidget-background)]">
            <SearchOutlined className="text-[var(--descriptionForeground)]" />
            <input
              autoFocus
              className="flex-1 bg-transparent outline-none text-sm text-[var(--foreground)]"
              placeholder="搜索消息..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Escape') {
                  setSearchOpen(false);
                  setSearchQuery('');
                }
              }}
            />
            {searchQuery && (
              <span className="text-xs text-[var(--descriptionForeground)]">
                {filteredMessage.length}/{message.length}
              </span>
            )}
            <CloseOutlined
              className="cursor-pointer text-[var(--descriptionForeground)] hover:text-[var(--foreground)]"
              onClick={() => {
                setSearchOpen(false);
                setSearchQuery('');
              }}
            />
          </div>
        )}
        {
          // 消息窗口
        }
        <MessageWindow
          message={filteredMessage}
          onDelete={onDelete}
          onSend={onSend}
          onInput={onInput}
          UserId={user?.UserId || ''}
          selectMode={selectMode}
          selectedKeys={selectedKeys}
          onButtonClick={(item, buttonId, buttonData) => {
            dispatch(
              sendInteraction({
                scope: pageType === 'public' ? 'public' : 'private',
                buttonId,
                buttonData,
                messageId: item.MessageId || item.CreateAt.toString()
              })
            );
          }}
          onReact={(item, emoji) => {
            const userId = user?.UserId || '';
            const msgId = item.MessageId || item.CreateAt.toString();
            const scope = pageType === 'public' ? 'group' : 'private';
            const existing = item.reactions?.find(r => r.emoji === emoji);
            if (existing && existing.users.includes(userId)) {
              dispatch(
                removeReaction({
                  scope,
                  CreateAt: item.CreateAt,
                  UserId: item.UserId,
                  emoji,
                  reactUserId: userId
                })
              );
              dispatch(
                sendReactionRemove({
                  scope: pageType === 'public' ? 'public' : 'private',
                  emoji,
                  messageId: msgId
                })
              );
            } else {
              dispatch(
                addReaction({
                  scope,
                  CreateAt: item.CreateAt,
                  UserId: item.UserId,
                  emoji,
                  reactUserId: userId
                })
              );
              dispatch(
                sendReactionAdd({
                  scope: pageType === 'public' ? 'public' : 'private',
                  emoji,
                  messageId: msgId
                })
              );
            }
          }}
          onRecall={item => {
            if (pageType === 'public') dispatch(deleteGroupMessage(item));
            else dispatch(deletePrivateMessage(item));
            dispatch(
              sendMessageDelete({
                scope: pageType === 'public' ? 'public' : 'private',
                messageId: item.MessageId || item.CreateAt.toString(),
                messageCreateAt: item.CreateAt
              })
            );
          }}
          onEdit={onEdit}
          onInspect={item => setInspectingItem(item)}
          onConfirmDelete={item => {
            // 右键确认删除，不进入选择模式
            if (pageType === 'public') dispatch(deleteGroupMessage(item));
            else dispatch(deletePrivateMessage(item));
          }}
          onBulkDeleteSelected={() => dispatch(deleteSelectedMessages())}
          onEnterSelectMode={item => {
            dispatch(setSelectMode(true));
            dispatch(toggleSelectMessage(item));
          }}
          onExitSelectMode={() => dispatch(setSelectMode(false))}
        />
        {
          // 输入框
        }
        <InputBox
          value={value}
          commands={commands}
          userList={userList}
          channelList={channels}
          pageType={pageType}
          onInput={onInput}
          onSendFormat={onSendFormat}
          getSlateValue={getSlateValueRef}
          onHistoryPrev={inputHistory.prev}
          onHistoryNext={inputHistory.next}
          onSend={() => {
            // 优先使用 Slate 文档直接序列化 (无需正则解析)
            const nodes = getSlateValueRef.current();
            if (nodes && nodes.length > 0) {
              const content = serializeToDataEnums(nodes, users, channels);
              if (
                content.length > 0 &&
                content.some(c => c.type !== 'Text' || c.value.trim())
              ) {
                // 记录发送历史
                const textParts = content
                  .filter(c => c.type === 'Text')
                  .map(c => c.value);
                if (textParts.length > 0) inputHistory.push(textParts.join(''));
                onSendFormat(content);
                onInput('');
                inputHistory.reset();
              }
            } else if (value.trim()) {
              // 回退: 纯文本走旧路径
              inputHistory.push(value);
              onSend(value);
              onInput('');
              inputHistory.reset();
            }
          }}
          onClear={onClear}
          onSelect={() => {
            if (!selectMode) {
              dispatch(setSelectMode(true));
            } else {
              dispatch(deleteSelectedMessages());
            }
          }}
          onCommand={handleCommand}
          onTimer={() => setOpen(true)}
          onCancelSelect={() => dispatch(setSelectMode(false))}
          selectMode={selectMode}
          selectedCount={selectedKeys.length}
        />
        {/* 事件日志面板 - 输入框下方可折叠 */}
        <Suspense fallback={null}>
          <EventLogPanel />
        </Suspense>
      </section>
      {/* 桌面端任务管理（移动端隐藏） */}
      <section className="testone-task-rail w-56 flex flex-col bg-[var(--editorWidget-background)] border-l border-[--panel-border] p-2 gradient-border">
        <TimerManager
          commandTasksInfo={commandTasksInfo}
          onDel={t => setTaskToDelete(t)}
          running={timerManager.getTaskCount().running}
          stopAllCommandTasks={() => {
            if (timerManager.hasRunningTasks) {
              setOpenStopAllConfirm(true);
            } else {
              Message.warning('没有正在运行的任务');
            }
          }}
          addTask={() => {
            setOpen(true);
          }}
        />
        <SidebarCommandList
          commands={commands}
          onCommandSelect={handleCommand}
        />
      </section>
      {/* 全部任务停止确认 */}
      <Suspense fallback={null}>
        <ModalStopAllConfirm
          tasks={commandTasksInfo.filter(t => t.isRunning)}
          open={openStopAllConfirm}
          onCancel={() => setOpenStopAllConfirm(false)}
          onConfirm={stopAllCommandTasks}
        />
        <ModalTaskDeleteConfirm
          task={commandTasksInfo.find(t => t.id === taskToDelete?.id)}
          onCancel={() => setTaskToDelete(null)}
          onConfirm={deleteSingleTask}
        />
        <ModalEditMessage
          open={!!editingItem}
          item={editingItem}
          onCancel={() => setEditingItem(null)}
          onConfirm={onEditConfirm}
        />
        <ModalReactionList
          open={!!reactionMessage && !!reactionEmoji}
          message={reactionMessage}
          reaction={
            reactionMessage?.reactions?.find(r => r.emoji === reactionEmoji) ||
            null
          }
          onClose={() => {
            setReactionMessage(null);
            setReactionEmoji(null);
          }}
        />
        <ModalCommandTimer
          commands={commands}
          values={timerConfig}
          onChange={setTimerConfig}
          open={open}
          onCancel={() => setOpen(false)}
          onConfirm={e => onSubmitTimer(e, { channel: channel, user: user })}
        />
        <PayloadInspector
          item={inspectingItem}
          open={!!inspectingItem}
          onClose={() => setInspectingItem(null)}
        />
      </Suspense>
    </div>
  );
}
