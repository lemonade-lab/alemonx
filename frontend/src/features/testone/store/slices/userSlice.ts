import { initBot, initUser } from '@testone/config';
import { User } from '@testone/typing';
import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface UserState {
  users: User[];
  bot?: User;
  current?: User;
}

const initialState: UserState = {
  users: [initUser, initBot],
  bot: initBot,
  current: initUser
};

const userSlice = createSlice({
  name: 'users',
  initialState,
  reducers: {
    setUserInitial(state, action: PayloadAction<UserState>) {
      state.bot = action.payload?.bot || initBot;
      state.current = action.payload?.current || initUser;
      const curUsers = [state.bot, state.current].filter(Boolean) as User[];
      if (Array.isArray(action.payload?.users)) {
        const incomingUsers = action.payload.users;
        const newUsers = [...curUsers, ...incomingUsers];
        state.users = newUsers;
      } else {
        state.users = curUsers;
      }
    },
    /**
     * 设置用户列表
     * @param state
     * @param action
     */
    setUsers(state, action: PayloadAction<User[]>) {
      const curBot = state.bot || initBot;
      const curUser = state.current || initUser;
      const curUsers = [curBot, curUser].filter(Boolean) as User[];
      if (Array.isArray(action.payload) && action.payload.length) {
        const incomingUsers = action.payload;
        const newUsers = [...curUsers, ...incomingUsers];
        state.users = newUsers;
      } else {
        state.users = curUsers;
      }
    },
    /**
     * 设置机器人
     * @param state
     * @param action
     */
    setBot(state, action: PayloadAction<User | undefined>) {
      if (action.payload) {
        state.bot = action.payload;
      }
    },
    /**
     * 设置当前用户
     * @param state
     * @param action
     */
    setCurrentUser(state, action: PayloadAction<User | undefined>) {
      if (action.payload) {
        state.current = action.payload;
      }
    }
  }
});

export const { setUsers, setBot, setCurrentUser, setUserInitial } =
  userSlice.actions;
export default userSlice.reducer;
