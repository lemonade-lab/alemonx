import {
  configureStore,
  createListenerMiddleware,
  isAnyOf,
  Middleware
} from '@reduxjs/toolkit';
import { useDispatch, useSelector, TypedUseSelectorHook } from 'react-redux';

// slices
import socketReducer from '@testone/store/slices/socketSlice';
import userReducer, { setUserInitial } from '@testone/store/slices/userSlice';
import channelReducer from '@testone/store/slices/channelSlice';
import commandReducer from '@testone/store/slices/commandSlice';
import chatReducer, {
  appendPrivateMessage,
  appendGroupMessage,
  deleteGroupMessage,
  deletePrivateMessage,
  clearGroupMessages,
  clearPrivateMessages,
  setGroupMessages,
  setPrivateMessages
} from '@testone/store/slices/chatSlice';
import {
  deleteSelectedMessages,
  addReaction,
  updateMessage
} from '@testone/store/slices/chatSlice';

import notificationReducer, {
  addNotification
} from '@testone/store/slices/notificationSlice';
import eventLogReducer, {
  addLogEntry
} from '@testone/store/slices/eventLogSlice';

import {
  wsConnectRequest,
  wsConnected,
  wsDisconnected,
  wsSetError,
  wsScheduleRestart,
  wsIncReconnect,
  wsCancel,
  wsResetReconnect
} from '@testone/store/slices/socketSlice';

import {
  setUsers,
  setBot,
  setCurrentUser
} from '@testone/store/slices/userSlice';
import { setChannels } from '@testone/store/slices/channelSlice';
import { setCommands } from '@testone/store/slices/commandSlice';

// 外部工具
import * as flattedJSON from 'flatted';
import {
  saveChatList,
  runtimeTrimArray,
  loadChatListFromServer
} from '@testone/core/chatlist';
import { Message } from '@testone/core/message';
import { payloadToMentions } from '@testone/core/alemon';
import { normalizeFormatAsync } from '@testone/core/imageStore';
import { getTestoneWsUrl } from '@testone/config';

// ---------------- WebSocket 相关私有变量 ----------------
const RECONNECT_CODE_NORMAL = 1000;
let ws: WebSocket | null = null;
let reconnectTimer: any = null;
let connectTimeoutTimer: any = null; // 新增：连接超时计时器
let stableResetTimer: any = null; // 连接稳定后清零重连计数

// ---------------- 耗时追踪 ----------------
/** 记录发送消息的时间戳，用于计算响应耗时 */
const pendingSendTimestamps: Map<string, number> = new Map();
const MAX_PENDING = 100;

function trackSend(eventType: string) {
  // 用事件类型作 key（简化的相关性追踪）
  const key = eventType;
  pendingSendTimestamps.set(key, Date.now());
  if (pendingSendTimestamps.size > MAX_PENDING) {
    // 清理最老的
    const first = pendingSendTimestamps.keys().next().value;
    if (first) {
      pendingSendTimestamps.delete(first);
    }
  }
}

function getLatency(recvEventType: string): number | undefined {
  // 映射接收事件到发送事件
  const LATENCY_MAP: Record<string, string> = {
    'message.send': 'message.create',
    'message.reaction.add': 'message.reaction.add',
    'message.update': 'message.update'
  };
  const sendKey = LATENCY_MAP[recvEventType];
  if (!sendKey) {
    return undefined;
  }
  const sendTime = pendingSendTimestamps.get(sendKey);
  if (!sendTime) {
    return undefined;
  }
  pendingSendTimestamps.delete(sendKey);
  return Date.now() - sendTime;
}

// ---------------- 工具函数 ----------------
let _storeRef: any = null;

function safeSend(obj: any) {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    console.warn('WS 未连接，发送失败');
    return false;
  }
  try {
    // ws 的 字符串用 flattedJSON
    ws.send(flattedJSON.stringify(obj));
    // 记录发送事件到日志
    const eventType = obj.type || obj.name || obj.action || 'unknown';
    trackSend(eventType);
    if (_storeRef) {
      _storeRef.dispatch(
        addLogEntry({
          timestamp: Date.now(),
          direction: 'send',
          eventType,
          payload: obj
        })
      );
    }
    return true;
  } catch (e) {
    console.error('发送失败', e);
    return false;
  }
}

async function loadHistory(
  host: string,
  port: number,
  dispatch: any,
  channelId?: string
) {
  try {
    const priv = await loadChatListFromServer({
        host,
        port,
        type: 'private',
        chatId: 'bot'
      });
    dispatch(setPrivateMessages(Array.isArray(priv) ? priv : []));
    if (channelId) {
      const group = await loadChatListFromServer({
          host,
          port,
          type: 'public',
          chatId: channelId
        });
      dispatch(setGroupMessages(Array.isArray(group) ? group : []));
    }
  } catch (e) {
    console.warn('加载历史失败', e);
  }
}

// ---------------- Listener Middleware ----------------
export const listenerMiddleware = createListenerMiddleware();

// 监听连接列表变化
listenerMiddleware.startListening({
  matcher: isAnyOf(
    appendPrivateMessage,
    appendGroupMessage,
    deleteGroupMessage,
    deletePrivateMessage,
    clearGroupMessages,
    clearPrivateMessages,
    deleteSelectedMessages
  ),
  effect: async (_action, api) => {
    // 合并多次触发：50ms 内只写一次
    schedulePersist(api.getState as any, () => Message.error('持久化聊天失败'));
  }
});

/**
 * 新增：监听当前群 ChannelId 变化，自动加载对应群历史消息
 * 说明：
 *  - 使用 predicate 以便拿到 previousState 和 currentState 进行对比
 *  - 当 channelId 变化且存在新的 channelId 时：读取本地缓存，否则清空
 */
listenerMiddleware.startListening({
  predicate: (_action, currentState, previousState) => {
    const prevId = (previousState as any)?.channels?.current?.ChannelId;
    const curId = (currentState as any)?.channels?.current?.ChannelId;
    return prevId !== curId; // 允许 curId 为空时也触发，用于切换到“无群”状态
  },
  effect: async (_action, api) => {
    const state: any = api.getState();
    const cfg = state.socket?.lastConfig;
    if (!cfg) {
      return;
    }

    const channelId = state.channels.current?.ChannelId;
    if (channelId) {
      const list = await loadChatListFromServer({
          host: cfg.host,
          port: cfg.port,
          type: 'public',
          chatId: channelId
        });
      api.dispatch(setGroupMessages(Array.isArray(list) ? list : []));
    } else {
      // 没有当前群，则清空群消息
      api.dispatch(setGroupMessages([]));
    }
  }
});

// ---------------- WebSocket Middleware ----------------
export const wsMiddleware: Middleware<{}, any> = store => next => action => {
  _storeRef = store;
  const result = next(action);

  if (wsConnectRequest.match(action)) {
    const { host, port } = action.payload;
    const proxiedUrl = getTestoneWsUrl();
    if (!proxiedUrl && (!host || !port)) {
      return result;
    }

    // 关闭旧连接
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.close();
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (stableResetTimer) {
      clearTimeout(stableResetTimer);
      stableResetTimer = null;
    }

    let attemptWs: WebSocket;
    try {
      attemptWs = new WebSocket(proxiedUrl || `ws://${host}:${port}/testone`);
      ws = attemptWs;
    } catch (err: any) {
      store.dispatch(wsSetError(err.message));
      store.dispatch(wsDisconnected());
      return result;
    }

    // 启动连接超时（6 秒）
    if (connectTimeoutTimer) {
      clearTimeout(connectTimeoutTimer);
      connectTimeoutTimer = null;
    }
    connectTimeoutTimer = setTimeout(() => {
      // 仍处于连接中则判定超时
      if (attemptWs.readyState === WebSocket.CONNECTING) {
        try {
          attemptWs.close();
        } catch {}
        store.dispatch(wsSetError('连接超时'));
        // 交给 onclose 逻辑决定是否重连
        // 兜底：个别环境 close() 不触发 onclose 时，2 秒后仍卡在 CONNECTING
        // 就强制收敛状态，避免底部永久显示“连接中”。
        setTimeout(() => {
          if (attemptWs.readyState === WebSocket.CONNECTING) {
            store.dispatch(wsDisconnected());
          }
        }, 2000);
      }
    }, 1000 * 6);

    ws.onopen = () => {
      if (connectTimeoutTimer) {
        clearTimeout(connectTimeoutTimer);
        connectTimeoutTimer = null;
      }
      store.dispatch(wsConnected());
      // 连接能保持 5 秒以上才算稳定，届时才清零重连计数。这样服务端
      // “握手成功后又立刻断开”（如非沙盒模式拒绝 /testone）不会导致
      // 计数被反复清零、无限重连。
      if (stableResetTimer) {
        clearTimeout(stableResetTimer);
      }
      stableResetTimer = setTimeout(() => {
        stableResetTimer = null;
        const current: any = store.getState();
        if (current.socket?.connected) {
          store.dispatch(wsResetReconnect());
        }
      }, 5000);
      const state: any = store.getState();
      // 请求 init
      safeSend({ type: 'init.data' });
      // 加载历史
      const channelId = state.channels.current?.ChannelId;
      loadHistory(host, port, store.dispatch, channelId);
    };

    ws.onmessage = (e: MessageEvent) => {
      try {
        // ws的解析用 flattedJSON
        const data = flattedJSON.parse(e.data.toString());
        // 记录接收事件到日志
        const eventType = data.type || data.action || 'unknown';
        const latency = getLatency(eventType);
        store.dispatch(
          addLogEntry({
            timestamp: Date.now(),
            direction: 'receive',
            eventType,
            payload: data,
            latency
          })
        );
        handleServerPayload(store, data);
      } catch (err) {
        console.error('WS 消息解析失败', err);
      }
    };

    ws.onerror = err => {
      if (connectTimeoutTimer) {
        clearTimeout(connectTimeoutTimer);
        connectTimeoutTimer = null;
      }
      console.error('WS 错误', err);
      store.dispatch(wsSetError('WebSocket 错误'));
    };

    ws.onclose = ev => {
      if (connectTimeoutTimer) {
        clearTimeout(connectTimeoutTimer);
        connectTimeoutTimer = null;
      }
      if (stableResetTimer) {
        clearTimeout(stableResetTimer);
        stableResetTimer = null;
      }
      store.dispatch(wsDisconnected());
      const state: any = store.getState();
      const socket = state.socket;
      if (!socket.allowRestart) {
        return;
      }
      if (ev.code !== RECONNECT_CODE_NORMAL && socket.lastConfig) {
        // 每次异常断开都计数（而不是在重连定时器里），次数上限才能真正
        // 限制“连接后又断开”的循环。
        store.dispatch(wsIncReconnect());
        const afterInc: any = store.getState();
        if (
          !afterInc.socket.persistentReconnect &&
          afterInc.socket.reconnectAttempts >= 3
        ) {
          // 放弃（非持久模式达到上限），并暴露最后一次断开原因
          if (ev.reason) {
            store.dispatch(wsSetError(ev.reason));
          }
          return;
        }
        const backoff = socket.reconnectDelay || 1200; // 固定 1.2s
        // 立即标记正在等待重连，以便 UI 显示 loading
        store.dispatch(wsScheduleRestart());
        reconnectTimer = setTimeout(() => {
          // 再次检查次数，避免 race
          const againState: any = store.getState();
          if (
            !againState.socket.persistentReconnect &&
            againState.socket.reconnectAttempts >= 3
          ) {
            // 达到上限：确保关闭 loading
            store.dispatch(wsDisconnected());
            // 关闭后续自动重启
            // 直接修改 allowRestart 需要 action，这里复用 wsSetError 不合适，可忽略或增加一个 action；简单起见保持 allowRestart 状态
            return;
          }
          console.debug(
            '[WS] attempt reconnect',
            againState.socket.reconnectAttempts + 1,
            'persistent=',
            againState.socket.persistentReconnect
          );
          store.dispatch(wsConnectRequest(againState.socket.lastConfig!));
        }, backoff);
      }
    };
  }
  // 处理取消连接
  if (wsCancel.match(action)) {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (connectTimeoutTimer) {
      clearTimeout(connectTimeoutTimer);
      connectTimeoutTimer = null;
    }
    if (stableResetTimer) {
      clearTimeout(stableResetTimer);
      stableResetTimer = null;
    }
    try {
      if (
        ws &&
        (ws.readyState === WebSocket.OPEN ||
          ws.readyState === WebSocket.CONNECTING)
      ) {
        // 使用正常关闭码，避免触发重连
        ws.close(RECONNECT_CODE_NORMAL, 'manual cancel');
      }
    } catch {}
  }

  return result;
};

// ---------------- 即时裁剪中间件（追加消息后立刻同步 UI） ----------------
const trimMiddleware: Middleware = storeAPI => next => action => {
  const result = next(action);
  try {
    if (
      appendGroupMessage.match(action) ||
      appendPrivateMessage.match(action)
    ) {
      const state: any = storeAPI.getState();
      const cfg = state.socket?.lastConfig;
      if (!cfg) {
        return result;
      }
      if (appendPrivateMessage.match(action)) {
        const current = state.chat.privateMessages;
        const trimmed = runtimeTrimArray(current);
        if (trimmed !== current) {
          storeAPI.dispatch(setPrivateMessages(trimmed));
        }
      }
      if (appendGroupMessage.match(action)) {
        const current = state.chat.groupMessages;
        const trimmed = runtimeTrimArray(current);
        if (trimmed !== current) {
          storeAPI.dispatch(setGroupMessages(trimmed));
        }
      }
    }
  } catch (e) {
    console.warn('即时裁剪中间件错误', e);
  }
  return result;
};

// ---------------- 服务器数据处理 ----------------
function handleServerPayload(store: any, receivedData: any) {
  switch (receivedData.type) {
    case 'init.data': {
      const { commands, users, channels, user, bot } =
        receivedData.payload || {};

      if (user || bot || users) {
        store.dispatch(
          setUserInitial({ current: user, bot, users: users || [] })
        );
      }
      if (channels) {
        store.dispatch(setChannels(channels));
      }
      if (commands) {
        store.dispatch(setCommands(commands));
      }
      return;
    }
    case 'users':
      if (Array.isArray(receivedData.payload)) {
        store.dispatch(setUsers(receivedData.payload));
      }
      return;
    case 'channels':
      if (Array.isArray(receivedData.payload)) {
        store.dispatch(setChannels(receivedData.payload));
      }
      return;
    case 'commands':
      store.dispatch(setCommands(receivedData.payload || []));
      return;
    case 'bot':
      store.dispatch(setBot(receivedData.payload));
      return;
    case 'user':
      store.dispatch(setCurrentUser(receivedData.payload));
      return;
    default:
      if (receivedData.actionId) {
        handleAction(store, receivedData);
      }
  }
}

async function handleAction(store: any, data: any) {
  const state: any = store.getState();
  const bot = state.users.bot;
  // 当前选择的频道
  const channel = state.channels.current;

  if (data.action === 'message.send') {
    let format = data.payload?.params?.format;
    if (Array.isArray(format)) {
      try {
        format = await normalizeFormatAsync(format);
      } catch (e) {
        console.warn('服务器消息格式图片转换失败', e);
      }
    }
    const message: any = {
      UserId: bot?.UserId,
      UserName: bot?.UserName,
      UserAvatar: bot?.UserAvatar,
      CreateAt: Date.now(),
      data: format
    };
    if (/private/.test(data.payload?.event?.name || '')) {
      store.dispatch(appendPrivateMessage(message));
      persistPrivate(store);
    } else {
      if (channel.ChannelId === data.payload?.event?.ChannelId) {
        store.dispatch(appendGroupMessage(message));
      }
      persistGroup(store);
    }
  } else if (data.action === 'mention.get') {
    try {
      const mentions = payloadToMentions(data.payload, state.users.users);
      safeSend({
        ...data,
        payload: [{ code: 2000, message: '', data: mentions }]
      });
    } catch (e) {
      console.error('处理 mentions 失败', e);
    }
  } else if (data.action === 'message.reaction.add') {
    // 服务端发来的表情回应事件
    try {
      const event = data.payload?.event;
      const emoji = data.payload?.params?.emoji || event?.MessageText || '';
      const targetCreateAt = event?.CreateAt;
      const targetUserId = event?.UserId;
      if (emoji && targetCreateAt && targetUserId) {
        const scope = /private/.test(event?.name || '') ? 'private' : 'group';
        store.dispatch(
          addReaction({
            scope,
            CreateAt: targetCreateAt,
            UserId: targetUserId,
            emoji,
            reactUserId: bot?.UserId || 'bot'
          })
        );
      }
    } catch (e) {
      console.error('处理 reaction 失败', e);
    }
  } else if (data.action === 'message.update') {
    // 处理消息编辑事件
    try {
      const targetCreateAt = data.payload?.event?.CreateAt;
      const targetUserId = data.payload?.event?.UserId;
      const updatedFormat =
        data.payload?.params?.format || data.payload?.event?.value || [];

      if (targetCreateAt && targetUserId) {
        const scope = /private/.test(data.payload?.event?.name || '')
          ? 'private'
          : 'group';
        store.dispatch(
          updateMessage({
            scope,
            CreateAt: targetCreateAt,
            UserId: targetUserId,
            data: updatedFormat
          })
        );
      }
    } catch (e) {
      console.error('处理 message.update 失败', e);
    }
  } else if (data.action === 'notice.create') {
    // 系统通知事件
    try {
      const title = data.payload?.event?.title || '系统通知';
      const content =
        data.payload?.event?.content ||
        data.payload?.params?.content ||
        JSON.stringify(data.payload);
      store.dispatch(
        addNotification({
          type: 'notice',
          title,
          content
        })
      );
    } catch (e) {
      console.error('处理 notice 失败', e);
    }
  } else if (/^(member\.|channel\.|guild\.)/.test(data.action || '')) {
    // 成员/频道/公会事件通知
    try {
      let title = '系统更新';
      let type: 'member_change' | 'channel_change' | 'guild_change' =
        'member_change';
      let content = '收到系统event';

      const eventName = data.payload?.event?.name || '';

      if (eventName.includes('member')) {
        type = 'member_change';
        const memberAction = eventName.split('.')[1];
        const userName = data.payload?.event?.UserName || '用户';
        const titleMap: Record<string, string> = {
          add: '新成员加入',
          remove: '成员移除',
          ban: '成员被禁言',
          unban: '成员解除禁言',
          update: '成员信息更新'
        };
        title = titleMap[memberAction] || '成员变化';
        content = `${userName} (${data.payload?.event?.UserId})`;
      } else if (eventName.includes('channel')) {
        type = 'channel_change';
        const channelAction = eventName.split('.')[1];
        const channelName = data.payload?.event?.ChannelName || '频道';
        const titleMap: Record<string, string> = {
          create: '频道已创建',
          delete: '频道已删除',
          update: '频道已更新'
        };
        title = titleMap[channelAction] || '频道变化';
        content = channelName;
      } else if (eventName.includes('guild')) {
        type = 'guild_change';
        const guildAction = eventName.split('.')[1];
        const guildName = data.payload?.event?.GuildName || '公会';
        const titleMap: Record<string, string> = {
          join: '已加入公会',
          exit: '已离开公会',
          update: '公会已更新'
        };
        title = titleMap[guildAction] || '公会变化';
        content = guildName;
      }

      store.dispatch(
        addNotification({
          type,
          title,
          content
        })
      );
    } catch (e) {
      console.error('处理 event 通知失败', e);
    }
  }
}

function persistPrivate(store: any) {
  const state: any = store.getState();
  const cfg = state.socket.lastConfig;
  if (!cfg) {
    return;
  }
  schedulePersist(store.getState, () => Message.error('保存私聊失败'));
}

function persistGroup(store: any) {
  const state: any = store.getState();
  const cfg = state.socket.lastConfig;
  const channelId = state.channels.current?.ChannelId;
  if (!cfg || !channelId) {
    return;
  }
  schedulePersist(store.getState, () => Message.error('保存群聊失败'));
}

// ---------------- 批量持久化调度 ----------------
let _persistTimer: any = null;
let _pendingPersist = false;
function schedulePersist(getState: () => any, onError: () => void) {
  if (_persistTimer) {
    _pendingPersist = true;
    return;
  }
  _pendingPersist = true;
  _persistTimer = setTimeout(() => {
    _persistTimer = null;
    if (!_pendingPersist) {
      return;
    }
    _pendingPersist = false;
    const state = getState();
    const cfg = state.socket?.lastConfig;
    if (!cfg) {
      return;
    }
    const persistFn = () => {
      try {
        const { data: trimmedPriv, changed: privChanged } = saveChatList(
          { host: cfg.host, port: cfg.port, type: 'private', chatId: 'bot' },
          state.chat.privateMessages
        );
        if (privChanged) {
          // 同步 Redux，避免 UI 保留被裁剪的旧消息
          try {
            store.dispatch(setPrivateMessages(trimmedPriv));
          } catch (e) {
            console.warn('同步裁剪私聊消息失败', e);
          }
        }
        const channelId = state.channels.current?.ChannelId;
        if (channelId) {
          const { data: trimmedGroup, changed: groupChanged } = saveChatList(
            {
              host: cfg.host,
              port: cfg.port,
              type: 'public',
              chatId: channelId
            },
            state.chat.groupMessages
          );
          if (groupChanged) {
            try {
              store.dispatch(setGroupMessages(trimmedGroup));
            } catch (e) {
              console.warn('同步裁剪群聊消息失败', e);
            }
          }
        }
      } catch {
        onError();
      }
    };
    if (typeof (window as any)?.requestIdleCallback === 'function') {
      (window as any).requestIdleCallback(persistFn, { timeout: 1000 });
    } else {
      persistFn();
    }
  }, 50); // 50ms 节流窗口
}

// ---------------- Store ----------------
export const store = configureStore({
  reducer: {
    socket: socketReducer,
    users: userReducer,
    channels: channelReducer,
    commands: commandReducer,
    chat: chatReducer,
    notification: notificationReducer,
    eventLog: eventLogReducer
  },
  middleware: getDefault =>
    getDefault({
      serializableCheck: false
    }).concat(listenerMiddleware.middleware, trimMiddleware, wsMiddleware),
  devTools: import.meta.env.DEV
});

// ---------------- 类型导出（供组件使用） ----------------
export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

// ---------------- 自定义 hooks ----------------
export const useAppDispatch = () => useDispatch<AppDispatch>();
export const useAppSelector: TypedUseSelectorHook<RootState> = useSelector;

// ---------------- 可选导出：safeSend (若外部还需要) ----------------
export { safeSend };
