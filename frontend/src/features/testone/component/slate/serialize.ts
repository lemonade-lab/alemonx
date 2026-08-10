import { Descendant, Text, Element as SlateElement } from 'slate';
import type { DataEnums } from '@testone/typing';
import type { MentionElement } from './types';
import type { User, Channel } from '@testone/typing';

/* ------------------------------------------------------------------ */
/*  Slate → DataEnums[]  (发送时)                                       */
/* ------------------------------------------------------------------ */

/**
 * 将 Slate 文档序列化为 DataEnums[]
 * 段落之间用 \n 分隔
 */
export function serializeToDataEnums(
  nodes: Descendant[],
  users: User[],
  channels: Channel[]
): DataEnums[] {
  const result: DataEnums[] = [];

  for (let i = 0; i < nodes.length; i++) {
    const node = nodes[i];
    // 段落之间插入换行
    if (i > 0) {
      result.push({ type: 'Text', value: '\n' });
    }
    if (SlateElement.isElement(node) && node.type === 'paragraph') {
      for (const child of node.children) {
        if (Text.isText(child)) {
          if (child.text) {
            result.push({ type: 'Text', value: child.text });
          }
        } else if (SlateElement.isElement(child) && child.type === 'mention') {
          const m = child as MentionElement;
          if (m.belong === 'channel') {
            const ch = channels.find(c => c.ChannelId === m.mentionId);
            if (ch) {
              result.push({
                type: 'Mention',
                value: m.mentionId,
                options: {
                  belong: 'channel'
                }
              });
            } else {
              result.push({ type: 'Text', value: `#${m.mentionName}` });
            }
          } else if (m.mentionId === 'everyone') {
            result.push({
              type: 'Mention',
              value: 'everyone',
              options: { belong: 'everyone' }
            });
          } else {
            const user = users.find(u => u.UserId === m.mentionId);
            if (user) {
              result.push({
                type: 'Mention',
                value: m.mentionId,
                options: {
                  belong: 'user'
                }
              });
            } else {
              result.push({ type: 'Text', value: `@${m.mentionName}` });
            }
          }
        }
      }
    }
  }

  return result;
}

/* ------------------------------------------------------------------ */
/*  纯文本提取 (供外部需要 string 的场景)                                   */
/* ------------------------------------------------------------------ */

/**
 * 将 Slate 文档序列化为纯文本字符串
 * mention 节点以 <@id::name> 格式输出, 保持向后兼容
 */
export function serializeToText(nodes: Descendant[]): string {
  return nodes
    .map(node => {
      if (SlateElement.isElement(node) && node.type === 'paragraph') {
        return node.children
          .map(child => {
            if (Text.isText(child)) {
              return child.text;
            }
            if (SlateElement.isElement(child) && child.type === 'mention') {
              const m = child as MentionElement;
              const prefix = m.belong === 'channel' ? '#' : '@';
              return `<${prefix}${m.mentionId}::${m.mentionName}>`;
            }
            return '';
          })
          .join('');
      }
      return '';
    })
    .join('\n');
}

/* ------------------------------------------------------------------ */
/*  纯文本 → Slate (兼容旧数据 / 外部设置 value)                          */
/* ------------------------------------------------------------------ */

/**
 * 将包含 <@id::name> 标记的纯文本转换为 Slate 文档
 */
export function deserializeFromText(text: string): Descendant[] {
  if (!text) {
    return [{ type: 'paragraph', children: [{ text: '' }] }];
  }

  const lines = text.split('\n');
  return lines.map(line => {
    const children: Descendant[] = [];
    // 同时匹配 <@id::name> 和 <#id::name>
    const mentionPattern = /<([@#])([^:]+)::([^>]+)>/g;
    let lastIndex = 0;
    let match: RegExpExecArray | null;

    while ((match = mentionPattern.exec(line)) !== null) {
      if (lastIndex < match.index) {
        children.push({ text: line.substring(lastIndex, match.index) });
      }
      const prefix = match[1]; // '@' or '#'
      const id = match[2];
      const name = match[3];
      const belong: 'user' | 'channel' | 'everyone' =
        prefix === '#' ? 'channel' : id === 'everyone' ? 'everyone' : 'user';
      children.push({
        type: 'mention',
        belong,
        mentionId: id,
        mentionName: name,
        children: [{ text: '' }]
      });
      lastIndex = mentionPattern.lastIndex;
    }

    if (lastIndex < line.length) {
      children.push({ text: line.substring(lastIndex) });
    }

    if (children.length === 0) {
      children.push({ text: '' });
    }

    return { type: 'paragraph', children } as Descendant;
  });
}
