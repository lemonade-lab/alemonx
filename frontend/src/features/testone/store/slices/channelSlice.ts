import { initChannel } from '@testone/config';
import { Channel } from '@testone/typing';
import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface ChannelState {
  channels: Channel[];
  current?: Channel;
}

const initialState: ChannelState = {
  channels: [initChannel],
  current: initChannel
};

const channelSlice = createSlice({
  name: 'channels',
  initialState,
  reducers: {
    /**
     * 设置频道列表
     * @param state
     * @param action
     */
    setChannels(state, action: PayloadAction<Channel[]>) {
      if (Array.isArray(action.payload)) {
        if (action.payload.length) {
          state.channels = action.payload;
        } else {
          state.channels = [initChannel];
        }
      } else {
        state.channels = [initChannel];
      }
    },
    /**
     * 设置当前频道
     * @param state
     * @param action
     */
    setCurrentChannel(state, action: PayloadAction<Channel | undefined>) {
      state.current = action.payload;
    }
  }
});

export const { setChannels, setCurrentChannel } = channelSlice.actions;
export default channelSlice.reducer;
