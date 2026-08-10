import { createSelector } from '@reduxjs/toolkit';
import { RootState } from '@testone/store';

const selectChat = (state: RootState) => state.chat;
const selectUsersState = (state: RootState) => state.users;
const selectChannelsState = (state: RootState) => state.channels;

export const selectCurrentChannel = createSelector(
  selectChannelsState,
  c => c.current
);
export const selectBotUser = createSelector(selectUsersState, u => u.bot);
export const selectAllUsers = createSelector(selectUsersState, u => u.users);

export const selectGroupMessages = createSelector(
  selectChat,
  chat => chat.groupMessages
);
export const selectPrivateMessages = createSelector(
  selectChat,
  chat => chat.privateMessages
);

export const makeSelectMessagesByType = (pageType: 'public' | 'private') =>
  createSelector([selectGroupMessages, selectPrivateMessages], (group, priv) =>
    pageType === 'public' ? group : priv
  );

export const makeSelectUserList = (pageType: 'public' | 'private') =>
  createSelector([selectAllUsers], users => {
    if (pageType !== 'public') {
      return [];
    }
    const everyone = {
      UserId: 'everyone',
      UserName: '全体成员',
      UserAvatar: '',
      IsMaster: false,
      IsBot: false
    };
    return [everyone, ...users];
  });

export const makeSelectHeader = (pageType: 'public' | 'private') =>
  createSelector([selectCurrentChannel, selectBotUser], (channel, bot) => {
    if (pageType === 'public') {
      return {
        Avatar: channel?.ChannelAvatar || '',
        Id: channel?.ChannelId || '',
        Name: channel?.ChannelName || ''
      };
    }
    return {
      Avatar: bot?.UserAvatar || '',
      Id: bot?.UserId || '',
      Name: bot?.UserName || ''
    };
  });
