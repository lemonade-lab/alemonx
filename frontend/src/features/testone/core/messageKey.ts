import { MessageItem } from '@testone/typing';
export function buildMessageKey(m: MessageItem): string {
  return `${m.CreateAt}-${m.UserId || 'sys'}`;
}
