import { BaseEditor, Descendant } from 'slate';
import { ReactEditor } from 'slate-react';
import { HistoryEditor } from 'slate-history';

/** 提及元素 — inline void */
export type MentionElement = {
  type: 'mention';
  /** 'user' | 'channel' | 'everyone' */
  belong: 'user' | 'channel' | 'everyone';
  mentionId: string;
  mentionName: string;
  children: [{ text: '' }];
};

/** 段落元素 */
export type ParagraphElement = {
  type: 'paragraph';
  children: Descendant[];
};

export type CustomElement = MentionElement | ParagraphElement;
export type CustomText = { text: string };

declare module 'slate' {
  interface CustomTypes {
    Editor: BaseEditor & ReactEditor & HistoryEditor;
    Element: CustomElement;
    Text: CustomText;
  }
}

/** 空文档 */
export const EMPTY_VALUE: Descendant[] = [
  { type: 'paragraph', children: [{ text: '' }] }
];
