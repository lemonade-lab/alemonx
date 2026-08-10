import { createAsyncThunk } from '@reduxjs/toolkit';
import { userHashKey, Platform } from '../core/alemon';
import type { DataEnums } from '@testone/typing';
import { appendGroupMessage, appendPrivateMessage } from './slices/chatSlice';
import { normalizeFormatAsync } from '../core/imageStore';
import { RootState, safeSend } from './index';
import { Channel, User } from '../typing';

export const sendGroupFormat = createAsyncThunk<
  void,
  {
    currentChannel: Channel;
    content: DataEnums[];
  },
  { state: RootState }
>(
  'chat/sendGroupFormat',
  async ({ currentChannel, content }, { getState, dispatch }) => {
    // 发送前格式归一（图片引用 + 压缩）
    const normalized = (await normalizeFormatAsync(
      content as any
    )) as DataEnums[];
    const state = getState();
    const { current: currentUser } = {
      current: state.users.current
    };
    const { channel } = {
      channel: state.channels.current
    };
    if (!currentUser || !channel) {
      return;
    }
    const UserKey = userHashKey({
      Platform,
      UserId: currentUser.UserId
    });
    const MessageText = normalized.find(i => i.type === 'Text')?.value || '';
    const payload = {
      name: 'message.create',
      ChannelId: currentChannel.ChannelId,
      GuildId: currentChannel.ChannelId,
      UserId: currentUser.UserId,
      OpenId: currentUser.OpenId || '',
      UserName: currentUser.UserName,
      UserKey,
      IsBot: false,
      IsMaster: false,
      UserAvatar: currentUser.UserAvatar,
      Platform,
      MessageText,
      CreateAt: Date.now(),
      MessageId: Date.now().toString(),
      tag: currentChannel.ChannelId,
      value: normalized
    };

    // 如果目标频道 和 当前频道不同。则不进行状态更新
    if (currentChannel.ChannelId === channel.ChannelId) {
      dispatch(
        appendGroupMessage({
          UserId: currentUser.UserId,
          UserName: currentUser.UserName,
          UserAvatar: currentUser.UserAvatar,
          CreateAt: payload.CreateAt,
          data: normalized
        })
      );
    }

    safeSend(payload);
  }
);

export const sendPrivateFormat = createAsyncThunk<
  void,
  {
    currentUser: User;
    content: DataEnums[];
  },
  { state: RootState }
>('chat/sendPrivateFormat', async ({ currentUser, content }, { dispatch }) => {
  if (!currentUser) {
    return;
  }
  const normalized = (await normalizeFormatAsync(
    content as any
  )) as DataEnums[];
  const UserKey = userHashKey({
    Platform,
    UserId: currentUser.UserId
  });
  const MessageText = normalized.find(i => i.type === 'Text')?.value || '';
  const payload = {
    name: 'private.message.create',
    UserId: currentUser.UserId,
    UserKey,
    OpenId: currentUser.OpenId || '',
    IsBot: false,
    IsMaster: false,
    UserAvatar: currentUser.UserAvatar,
    Platform,
    MessageText,
    CreateAt: Date.now(),
    MessageId: Date.now().toString(),
    UserName: currentUser.UserName,
    tag: 'bot',
    value: normalized
  };
  dispatch(
    appendPrivateMessage({
      UserId: currentUser.UserId,
      UserName: currentUser.UserName,
      UserAvatar: currentUser.UserAvatar,
      CreateAt: payload.CreateAt,
      data: normalized
    })
  );
  safeSend(payload);
});

/**
 * 发送 interaction.create 事件（按钮交互）
 */
export const sendInteraction = createAsyncThunk<
  void,
  {
    scope: 'public' | 'private';
    buttonId: string;
    buttonData: string;
    messageId?: string;
  },
  { state: RootState }
>(
  'chat/sendInteraction',
  async ({ scope, buttonId, buttonData, messageId }, { getState }) => {
    const state = getState();
    const currentUser = state.users.current;
    const channel = state.channels.current;
    if (!currentUser) {
      return;
    }

    const UserKey = userHashKey({
      Platform,
      UserId: currentUser.UserId
    });

    const name =
      scope === 'private' ? 'private.interaction.create' : 'interaction.create';

    const payload: Record<string, any> = {
      name,
      UserId: currentUser.UserId,
      UserKey,
      OpenId: currentUser.OpenId || '',
      UserName: currentUser.UserName,
      UserAvatar: currentUser.UserAvatar,
      IsBot: false,
      IsMaster: false,
      Platform,
      CreateAt: Date.now(),
      MessageId: messageId || Date.now().toString(),
      value: [{ type: 'Text', value: buttonData }],
      MessageText: buttonData,
      interaction: {
        type: 'button',
        buttonId,
        data: buttonData
      }
    };

    if (scope === 'public' && channel) {
      payload.ChannelId = channel.ChannelId;
      payload.GuildId = channel.ChannelId;
      payload.tag = channel.ChannelId;
    } else {
      payload.tag = 'bot';
    }

    safeSend(payload);
  }
);

/**
 * 发送 message.delete 事件（通知 bot 消息被撤回）
 */
export const sendMessageDelete = createAsyncThunk<
  void,
  {
    scope: 'public' | 'private';
    messageId: string;
    messageCreateAt: number;
  },
  { state: RootState }
>(
  'chat/sendMessageDelete',
  async ({ scope, messageId, messageCreateAt }, { getState }) => {
    const state = getState();
    const currentUser = state.users.current;
    const channel = state.channels.current;
    if (!currentUser) {
      return;
    }

    const UserKey = userHashKey({
      Platform,
      UserId: currentUser.UserId
    });

    const name =
      scope === 'private' ? 'private.message.delete' : 'message.delete';

    const payload: Record<string, any> = {
      name,
      UserId: currentUser.UserId,
      UserKey,
      OpenId: currentUser.OpenId || '',
      UserName: currentUser.UserName,
      UserAvatar: currentUser.UserAvatar,
      IsBot: false,
      IsMaster: false,
      Platform,
      CreateAt: messageCreateAt,
      MessageId: messageId || messageCreateAt.toString(),
      MessageText: ''
    };

    if (scope === 'public' && channel) {
      payload.ChannelId = channel.ChannelId;
      payload.GuildId = channel.ChannelId;
      payload.tag = channel.ChannelId;
    } else {
      payload.tag = 'bot';
    }

    safeSend(payload);
  }
);

/**
 * 发送 message.reaction.add 事件
 */
export const sendReactionAdd = createAsyncThunk<
  void,
  {
    scope: 'public' | 'private';
    messageId: string;
    emoji: string;
  },
  { state: RootState }
>('chat/sendReactionAdd', async ({ scope, messageId, emoji }, { getState }) => {
  const state = getState();
  const currentUser = state.users.current;
  const channel = state.channels.current;
  if (!currentUser) {
    return;
  }

  const UserKey = userHashKey({
    Platform,
    UserId: currentUser.UserId
  });

  const payload: Record<string, any> = {
    name: 'message.reaction.add',
    UserId: currentUser.UserId,
    UserKey,
    OpenId: currentUser.OpenId || '',
    UserName: currentUser.UserName,
    UserAvatar: currentUser.UserAvatar,
    IsBot: false,
    IsMaster: false,
    Platform,
    CreateAt: Date.now(),
    MessageId: messageId,
    MessageText: emoji,
    value: [{ type: 'Text', value: emoji }]
  };

  if (scope === 'public' && channel) {
    payload.ChannelId = channel.ChannelId;
    payload.GuildId = channel.ChannelId;
    payload.tag = channel.ChannelId;
  } else {
    payload.tag = 'bot';
  }

  safeSend(payload);
});

/**
 * 发送 message.reaction.remove 事件
 */
export const sendReactionRemove = createAsyncThunk<
  void,
  {
    scope: 'public' | 'private';
    messageId: string;
    emoji: string;
  },
  { state: RootState }
>(
  'chat/sendReactionRemove',
  async ({ scope, messageId, emoji }, { getState }) => {
    const state = getState();
    const currentUser = state.users.current;
    const channel = state.channels.current;
    if (!currentUser) {
      return;
    }

    const UserKey = userHashKey({
      Platform,
      UserId: currentUser.UserId
    });

    const payload: Record<string, any> = {
      name: 'message.reaction.remove',
      UserId: currentUser.UserId,
      UserKey,
      OpenId: currentUser.OpenId || '',
      UserName: currentUser.UserName,
      UserAvatar: currentUser.UserAvatar,
      IsBot: false,
      IsMaster: false,
      Platform,
      CreateAt: Date.now(),
      MessageId: messageId,
      MessageText: emoji,
      value: [{ type: 'Text', value: emoji }]
    };

    if (scope === 'public' && channel) {
      payload.ChannelId = channel.ChannelId;
      payload.GuildId = channel.ChannelId;
      payload.tag = channel.ChannelId;
    } else {
      payload.tag = 'bot';
    }

    safeSend(payload);
  }
);

/**
 * 发送 message.update 事件（编辑消息）
 */
export const sendMessageUpdate = createAsyncThunk<
  void,
  {
    scope: 'public' | 'private';
    messageId: string;
    messageCreateAt: number;
    content: DataEnums[];
  },
  { state: RootState }
>(
  'chat/sendMessageUpdate',
  async ({ scope, messageId, messageCreateAt, content }, { getState }) => {
    const state = getState();
    const currentUser = state.users.current;
    const channel = state.channels.current;
    if (!currentUser) {
      return;
    }

    const UserKey = userHashKey({
      Platform,
      UserId: currentUser.UserId
    });

    const MessageText = content.find(i => i.type === 'Text')?.value || '';
    const normalized = (await normalizeFormatAsync(
      content as any
    )) as DataEnums[];

    const name =
      scope === 'private' ? 'private.message.update' : 'message.update';

    const payload: Record<string, any> = {
      name,
      UserId: currentUser.UserId,
      UserKey,
      OpenId: currentUser.OpenId || '',
      UserName: currentUser.UserName,
      UserAvatar: currentUser.UserAvatar,
      IsBot: false,
      IsMaster: false,
      Platform,
      CreateAt: messageCreateAt,
      MessageId: messageId,
      MessageText,
      value: normalized,
      UpdateAt: Date.now()
    };

    if (scope === 'public' && channel) {
      payload.ChannelId = channel.ChannelId;
      payload.GuildId = channel.ChannelId;
      payload.tag = channel.ChannelId;
    } else {
      payload.tag = 'bot';
    }

    safeSend(payload);
  }
);
