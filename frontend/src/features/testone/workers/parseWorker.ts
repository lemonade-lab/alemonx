import { DataEnums } from '../typing';

interface User {
  UserId: string;
  UserName: string;
  UserAvatar: string;
}
interface Channel {
  ChannelId: string;
  ChannelName: string;
  ChannelAvatar: string;
}

(self as any).onmessage = (e: MessageEvent) => {
  const msg = e.data;
  try {
    const { id, input, users, channels } = msg as {
      id: number;
      input: string;
      users: User[];
      channels: Channel[];
    };
    const result: DataEnums[] = parseRaw({ input, users, channels });
    (self as any).postMessage({ id, ok: true, data: result });
  } catch (err: any) {
    (self as any).postMessage({
      id: msg.id,
      ok: false,
      error: err?.message || 'parse error'
    });
  }
};

function parseRaw({
  input,
  users,
  channels
}: {
  input: string;
  users: User[];
  channels: Channel[];
}): DataEnums[] {
  const bodies: DataEnums[] = [];
  const mentionPattern = /<([@#])([^:]+)::([^>]+)>/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = mentionPattern.exec(input)) !== null) {
    if (lastIndex < match.index) {
      const textValue = input.substring(lastIndex, match.index);
      if (textValue) {
        bodies.push({ type: 'Text', value: textValue });
      }
    }
    const mentionType = match[1];
    const id = match[2];
    if (mentionType === '@') {
      const user = users.find(u => u.UserId === id);
      if (user) {
        bodies.push({
          type: 'Mention',
          value: id,
          options: {
            belong: 'user',
            payload: { name: user.UserName, avatar: user.UserAvatar } as any
          }
        });
      } else if (id === 'everyone') {
        bodies.push({
          type: 'Mention',
          value: id,
          options: { belong: 'everyone', payload: 'everyone' as any }
        });
      } else {
        bodies.push({ type: 'Text', value: match[0] });
      }
    } else {
      const channel = channels.find(c => c.ChannelId === id);
      if (channel) {
        bodies.push({
          type: 'Mention',
          value: id,
          options: {
            belong: 'channel',
            payload: {
              name: channel.ChannelName,
              avatar: channel.ChannelAvatar
            } as any
          }
        });
      } else {
        bodies.push({ type: 'Text', value: match[0] });
      }
    }
    lastIndex = mentionPattern.lastIndex;
  }
  if (lastIndex < input.length) {
    const remaining = input.substring(lastIndex);
    if (remaining) {
      bodies.push({ type: 'Text', value: remaining });
    }
  }
  return bodies;
}
