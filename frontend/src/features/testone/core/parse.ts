import { DataEnums } from '@testone/typing';
import { Channel, User } from '../typing';

type ParseType = {
  Users: User[];
  Channels: Channel[];
  input: string;
};

/**
 * 解析消息
 * @param data
 * @returns
 */
export const parseMessage = (data: ParseType): DataEnums[] => {
  const bodies: DataEnums[] = [];
  const { Users, Channels, input } = data;

  // 匹配 <@id::username> 和 <#id::channelname> 格式
  const mentionPattern = /<([@#])([^:]+)::([^>]+)>/g;
  let lastIndex = 0;
  let match;

  while ((match = mentionPattern.exec(input)) !== null) {
    // 添加普通文本部分
    if (lastIndex < match.index) {
      const textValue = input.substring(lastIndex, match.index);
      if (textValue) {
        bodies.push({ type: 'Text', value: textValue });
      }
    }

    const mentionType = match[1]; // @ 或 #
    const id = match[2]; // ID
    const name = match[3]; // 名称

    if (mentionType === '@') {
      // 处理用户提及
      const user = Users.find(u => u.UserId === id);
      if (user) {
        bodies.push({
          type: 'Mention',
          value: id,
          options: {
            name: user.UserName,
            avatar: user.UserAvatar,
            belong: 'user'
          }
        });
      } else if (id === 'everyone') {
        bodies.push({
          type: 'Mention',
          value: id,
          options: {
            name: name,
            avatar: '',
            belong: 'everyone'
          }
        });
      } else {
        // 如果找不到用户，保留原始文本
        bodies.push({ type: 'Text', value: match[0] });
      }
    } else if (mentionType === '#') {
      // 处理频道提及
      const channel = Channels.find(c => c.ChannelId === id);
      if (channel) {
        bodies.push({
          type: 'Mention',
          value: id,
          options: {
            name: channel.ChannelName,
            avatar: channel.ChannelAvatar,
            belong: 'channel'
          }
        });
      } else {
        // 如果找不到频道，保留原始文本
        bodies.push({ type: 'Text', value: match[0] });
      }
    }

    lastIndex = mentionPattern.lastIndex;
  }

  // 添加剩余文本
  if (lastIndex < input.length) {
    const remainingText = input.substring(lastIndex);
    if (remainingText) {
      bodies.push({ type: 'Text', value: remainingText });
    }
  }

  console.log('Parsed message bodies:', bodies);

  return bodies;
};
