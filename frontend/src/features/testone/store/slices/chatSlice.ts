import { MessageItem } from '@testone/typing';
import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface ChatState {
  privateMessages: MessageItem[];
  groupMessages: MessageItem[];
  tab: 'group' | 'private' | 'help' | 'config';
  selectMode: boolean; // 是否处于选择删除模式
  selectedKeys: string[]; // 被选中的消息 key
}

const initialState: ChatState = {
  privateMessages: [],
  groupMessages: [],
  // 打开即固定连接本机测试服务：默认直接进入群聊，不再落到连接管理页。
  tab: 'group',
  selectMode: false,
  selectedKeys: []
};

// 与 chatlist CFG.MAX_MESSAGES 对齐
const MAX_MESSAGES = 300;

const chatSlice = createSlice({
  name: 'chat',
  initialState,
  reducers: {
    /**
     * 设置当前选项卡
     * @param state
     * @param action
     */
    setTab(state, action: PayloadAction<ChatState['tab']>) {
      state.tab = action.payload;
    },
    /**
     * 设置私聊消息
     * @param state
     * @param action
     */
    setPrivateMessages(state, action: PayloadAction<MessageItem[]>) {
      state.privateMessages = action.payload;
    },
    /**
     * 设置群聊消息
     * @param state
     * @param action
     */
    setGroupMessages(state, action: PayloadAction<MessageItem[]>) {
      state.groupMessages = action.payload;
    },
    /**
     * 追加私聊消息
     * @param state
     * @param action
     */
    appendPrivateMessage(state, action: PayloadAction<MessageItem>) {
      state.privateMessages.push(action.payload);
      if (state.privateMessages.length > MAX_MESSAGES) {
        state.privateMessages.splice(
          0,
          state.privateMessages.length - MAX_MESSAGES
        );
      }
    },
    /**
     * 追加群聊消息
     * @param state
     * @param action
     */
    appendGroupMessage(state, action: PayloadAction<MessageItem>) {
      state.groupMessages.push(action.payload);
      if (state.groupMessages.length > MAX_MESSAGES) {
        state.groupMessages.splice(
          0,
          state.groupMessages.length - MAX_MESSAGES
        );
      }
    },
    /**
     * 删除私聊消息
     * @param state
     * @param action
     */
    deletePrivateMessage(
      state,
      action: PayloadAction<{ CreateAt: number; UserId: string }>
    ) {
      state.privateMessages = state.privateMessages.filter(
        m =>
          !(
            m.CreateAt === action.payload.CreateAt &&
            m.UserId === action.payload.UserId
          )
      );
    },
    /**
     * 删除群聊消息
     * @param state
     * @param action
     */
    deleteGroupMessage(
      state,
      action: PayloadAction<{ CreateAt: number; UserId: string }>
    ) {
      state.groupMessages = state.groupMessages.filter(
        m =>
          !(
            m.CreateAt === action.payload.CreateAt &&
            m.UserId === action.payload.UserId
          )
      );
    },
    /**
     * 清除私聊消息
     * @param state
     */
    clearPrivateMessages(state) {
      state.privateMessages = [];
    },
    /**
     * 清除群聊消息
     * @param state
     */
    clearGroupMessages(state) {
      state.groupMessages = [];
    },
    /**
     * 进入或退出选择模式
     * @param state
     * @param action
     */
    setSelectMode(state, action: PayloadAction<boolean>) {
      state.selectMode = action.payload;
      if (!action.payload) {
        state.selectedKeys = [];
      }
    },
    /**
     * 切换单条消息选择
     * @param state
     * @param action
     */
    toggleSelectMessage(state, action: PayloadAction<MessageItem>) {
      const key = `${action.payload.UserId}:${action.payload.CreateAt}`;
      const exists = state.selectedKeys.includes(key);
      if (exists) {
        state.selectedKeys = state.selectedKeys.filter(k => k !== key);
      } else {
        state.selectedKeys.push(key);
      }
    },
    /**
     * 批量删除已选择的消息
     * @param state
     */
    deleteSelectedMessages(state) {
      if (!state.selectedKeys.length) {
        return;
      }
      const keySet = new Set(state.selectedKeys);
      state.privateMessages = state.privateMessages.filter(
        m => !keySet.has(`${m.UserId}:${m.CreateAt}`)
      );
      state.groupMessages = state.groupMessages.filter(
        m => !keySet.has(`${m.UserId}:${m.CreateAt}`)
      );
      state.selectedKeys = [];
      state.selectMode = false;
    },
    /**
     * 添加表情回应
     */
    addReaction(
      state,
      action: PayloadAction<{
        scope: 'group' | 'private';
        CreateAt: number;
        UserId: string;
        emoji: string;
        reactUserId: string;
      }>
    ) {
      const { scope, CreateAt, UserId, emoji, reactUserId } = action.payload;
      const messages =
        scope === 'group' ? state.groupMessages : state.privateMessages;
      const msg = messages.find(
        m => m.CreateAt === CreateAt && m.UserId === UserId
      );
      if (!msg) {
        return;
      }
      if (!msg.reactions) {
        msg.reactions = [];
      }
      const existing = msg.reactions.find(r => r.emoji === emoji);
      if (existing) {
        if (!existing.users.includes(reactUserId)) {
          existing.users.push(reactUserId);
        }
      } else {
        msg.reactions.push({ emoji, users: [reactUserId] });
      }
    },
    /**
     * 移除表情回应
     */
    removeReaction(
      state,
      action: PayloadAction<{
        scope: 'group' | 'private';
        CreateAt: number;
        UserId: string;
        emoji: string;
        reactUserId: string;
      }>
    ) {
      const { scope, CreateAt, UserId, emoji, reactUserId } = action.payload;
      const messages =
        scope === 'group' ? state.groupMessages : state.privateMessages;
      const msg = messages.find(
        m => m.CreateAt === CreateAt && m.UserId === UserId
      );
      if (!msg || !msg.reactions) {
        return;
      }
      const existing = msg.reactions.find(r => r.emoji === emoji);
      if (existing) {
        existing.users = existing.users.filter(u => u !== reactUserId);
        if (existing.users.length === 0) {
          msg.reactions = msg.reactions.filter(r => r.emoji !== emoji);
        }
      }
    },
    /**
     * 编辑消息
     */
    updateMessage(
      state,
      action: PayloadAction<{
        scope: 'group' | 'private';
        CreateAt: number;
        UserId: string;
        data: any;
      }>
    ) {
      const { scope, CreateAt, UserId, data } = action.payload;
      const messages =
        scope === 'group' ? state.groupMessages : state.privateMessages;
      const msg = messages.find(
        m => m.CreateAt === CreateAt && m.UserId === UserId
      );
      if (!msg) {
        return;
      }
      msg.data = data;
      msg.UpdateAt = Date.now();
      msg.IsEdited = true;
    }
  }
});

export const {
  setTab,
  setPrivateMessages,
  setGroupMessages,
  appendPrivateMessage,
  appendGroupMessage,
  deletePrivateMessage,
  deleteGroupMessage,
  clearPrivateMessages,
  clearGroupMessages,
  setSelectMode,
  toggleSelectMessage,
  deleteSelectedMessages,
  addReaction,
  removeReaction,
  updateMessage
} = chatSlice.actions;

export default chatSlice.reducer;
