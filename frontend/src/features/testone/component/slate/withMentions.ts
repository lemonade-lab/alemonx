import { Editor, Element as SlateElement } from 'slate';

/**
 * Slate 插件: 让 mention 成为 inline void 元素
 */
export function withMentions<T extends Editor>(editor: T): T {
  const { isInline, isVoid, markableVoid } = editor;

  editor.isInline = (element: SlateElement) => {
    return element.type === 'mention' ? true : isInline(element);
  };

  editor.isVoid = (element: SlateElement) => {
    return element.type === 'mention' ? true : isVoid(element);
  };

  editor.markableVoid = (element: SlateElement) => {
    return element.type === 'mention' || markableVoid(element);
  };

  return editor;
}
