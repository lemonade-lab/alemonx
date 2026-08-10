import { initCommand } from '@testone/config';
import { Command } from '@testone/typing';
import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface CommandState {
  commands: Command[];
}

const initialState: CommandState = {
  commands: [initCommand]
};

const commandSlice = createSlice({
  name: 'commands',
  initialState,
  reducers: {
    /**
     * 设置命令列表
     * @param state
     * @param action
     */
    setCommands(state, action: PayloadAction<Command[]>) {
      if (Array.isArray(action.payload)) {
        if (action.payload.length) {
          state.commands = action.payload;
        } else {
          state.commands = [initCommand];
        }
      } else {
        state.commands = [initCommand];
      }
    }
  }
});

export const { setCommands } = commandSlice.actions;
export default commandSlice.reducer;
