import { createSlice, PayloadAction } from '@reduxjs/toolkit';

export interface EventLogEntry {
  id: string;
  timestamp: number;
  direction: 'send' | 'receive';
  eventType: string;
  payload: any;
  /** 关联的发送 id，用于计算响应耗时 */
  correlationId?: string;
  /** 响应耗时 (ms)，仅 receive 时有 */
  latency?: number;
}

interface EventLogState {
  entries: EventLogEntry[];
  /** 是否开启日志记录 */
  enabled: boolean;
  /** 当前筛选的事件类型，空则显示全部 */
  filter: string;
  /** 当前选中查看详情的日志 id */
  selectedId: string | null;
  /** 最大保留条数 */
  maxEntries: number;
}

const initialState: EventLogState = {
  entries: [],
  enabled: true,
  filter: '',
  selectedId: null,
  maxEntries: 500
};

let _logIdCounter = 0;
export function genLogId() {
  return `log_${Date.now()}_${++_logIdCounter}`;
}

const eventLogSlice = createSlice({
  name: 'eventLog',
  initialState,
  reducers: {
    addLogEntry(state, action: PayloadAction<Omit<EventLogEntry, 'id'>>) {
      if (!state.enabled) {
        return;
      }
      const entry: EventLogEntry = {
        ...action.payload,
        id: genLogId()
      };
      state.entries.push(entry);
      // 裁剪
      if (state.entries.length > state.maxEntries) {
        state.entries = state.entries.slice(-state.maxEntries);
      }
    },
    clearLogs(state) {
      state.entries = [];
    },
    setLogEnabled(state, action: PayloadAction<boolean>) {
      state.enabled = action.payload;
    },
    setLogFilter(state, action: PayloadAction<string>) {
      state.filter = action.payload;
    },
    setSelectedLogId(state, action: PayloadAction<string | null>) {
      state.selectedId = action.payload;
    }
  }
});

export const {
  addLogEntry,
  clearLogs,
  setLogEnabled,
  setLogFilter,
  setSelectedLogId
} = eventLogSlice.actions;

export default eventLogSlice.reducer;
