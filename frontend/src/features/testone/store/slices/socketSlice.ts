import { Connect } from '@testone/typing';
import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface SocketState {
  connected: boolean;
  isConnecting: boolean;
  allowRestart: boolean;
  lastConfig?: Connect;
  reconnectDelay: number;
  isRestarting: boolean;
  error?: string;
  reconnectAttempts: number;
  autoConnectEnabled: boolean;
  // 用户手动选择的连接，开启后应无限次重连直到显式取消
  persistentReconnect: boolean;
}

// 尝试从 localStorage 读取自动连接开关，默认 true
let _persistedAuto = true;
try {
  const v = localStorage.getItem('autoConnectEnabled');
  if (v !== null) {
    _persistedAuto = v === 'true';
  }
} catch {}

const initialState: SocketState = {
  connected: false,
  isConnecting: false,
  allowRestart: true,
  reconnectDelay: 1200, // 重连基础间隔改为 1.2s
  isRestarting: false,
  reconnectAttempts: 0,
  autoConnectEnabled: _persistedAuto,
  persistentReconnect: false
};

const socketSlice = createSlice({
  name: 'socket',
  initialState,
  reducers: {
    /**
     * 请求连接
     * @param state
     * @param action
     */
    wsConnectRequest(state, action: PayloadAction<Connect>) {
      state.isConnecting = true;
      state.lastConfig = action.payload;
      state.error = undefined;
    },
    /**
     * 连接成功
     * @param state
     */
    wsConnected(state) {
      state.connected = true;
      state.isConnecting = false;
      state.isRestarting = false;
    },
    /**
     * 连接断开
     * @param state
     */
    wsDisconnected(state) {
      state.connected = false;
      state.isConnecting = false;
      state.isRestarting = false; // 断开时清理重启状态，避免无限 loading
    },
    /**
     * 设置错误
     * @param state
     * @param action
     */
    wsSetError(state, action: PayloadAction<string | undefined>) {
      state.error = action.payload;
    },
    /**
     * 设置是否允许重启
     * @param state
     * @param action
     */
    wsSetAllowRestart(state, action: PayloadAction<boolean>) {
      state.allowRestart = action.payload;
    },
    /**
     * 计划重启
     * @param state
     */
    wsScheduleRestart(state) {
      state.isRestarting = true;
    },
    /**
     * 增加重连次数
     * @param state
     */
    wsIncReconnect(state) {
      state.reconnectAttempts += 1;
    },
    /**
     * 连接稳定（保持一段时间）后清零重连计数。仅由中间件在连接稳定后调用，
     * 避免“握手成功又立刻断开”的循环把计数反复清零导致无限重连。
     */
    wsResetReconnect(state) {
      state.reconnectAttempts = 0;
    },
    /**
     * 取消连接
     * @param state
     */
    wsCancel(state) {
      state.isConnecting = false;
      state.isRestarting = false; // 取消时同时清除重启 loading 状态
      state.allowRestart = false;
      state.persistentReconnect = false; // 取消时关闭持久重连
    },
    /**
     * 设置是否自动连接
     * @param state
     * @param action
     */
    setAutoConnectEnabled(state, action: PayloadAction<boolean>) {
      state.autoConnectEnabled = action.payload;
      try {
        localStorage.setItem(
          'autoConnectEnabled',
          action.payload ? 'true' : 'false'
        );
      } catch {}
    },
    /**
     * 设置是否持久重连（仅用户手动发起的连接）
     * @param state
     * @param action
     */
    wsSetPersistent(state, action: PayloadAction<boolean>) {
      state.persistentReconnect = action.payload;
    }
  }
});

export const {
  wsConnectRequest,
  wsConnected,
  wsDisconnected,
  wsSetError,
  wsSetAllowRestart,
  wsScheduleRestart,
  wsCancel,
  wsIncReconnect,
  wsResetReconnect,
  setAutoConnectEnabled,
  wsSetPersistent
} = socketSlice.actions;

export default socketSlice.reducer;
