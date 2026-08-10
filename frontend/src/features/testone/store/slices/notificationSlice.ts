import { createSlice, PayloadAction } from '@reduxjs/toolkit';

export type SystemNotification = {
  id: string;
  type: 'notice' | 'member_change' | 'channel_change' | 'guild_change';
  title: string;
  content: string;
  CreateAt: number;
  duration?: number; // 毫秒，0 表示不自动关闭
};

interface NotificationState {
  notifications: SystemNotification[];
}

const initialState: NotificationState = {
  notifications: []
};

const notificationSlice = createSlice({
  name: 'notification',
  initialState,
  reducers: {
    addNotification(
      state,
      action: PayloadAction<Omit<SystemNotification, 'id' | 'CreateAt'>>
    ) {
      const notification: SystemNotification = {
        ...action.payload,
        id: Date.now().toString(36) + Math.random().toString(36).substring(2),
        CreateAt: Date.now(),
        duration: action.payload.duration ?? 5000
      };
      state.notifications.push(notification);
    },
    removeNotification(state, action: PayloadAction<string>) {
      state.notifications = state.notifications.filter(
        n => n.id !== action.payload
      );
    },
    clearNotifications(state) {
      state.notifications = [];
    }
  }
});

export const { addNotification, removeNotification, clearNotifications } =
  notificationSlice.actions;

export default notificationSlice.reducer;
