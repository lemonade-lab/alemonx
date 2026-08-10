import { RenderElementProps } from 'slate-react';
import type { MentionElement } from './types';

/** 段落渲染 */
export function ParagraphElement(props: RenderElementProps) {
  return (
    <p {...props.attributes} style={{ margin: 0 }}>
      {props.children}
    </p>
  );
}

/** Mention 行内元素渲染 */
export function MentionElementComponent(
  props: RenderElementProps & { element: MentionElement }
) {
  const { attributes, children, element } = props;
  const prefix = element.belong === 'channel' ? '#' : '@';
  return (
    <span
      {...attributes}
      contentEditable={false}
      data-mentionid={element.mentionId}
      data-belong={element.belong}
      className={`inline-flex items-center rounded px-1 mx-0.5 text-xs font-medium select-none align-baseline ${
        element.belong === 'channel'
          ? 'bg-[var(--textLink-foreground)] text-[var(--editor-background)]'
          : 'bg-[var(--button-background)] text-[var(--button-foreground)]'
      }`}
      style={{ userSelect: 'none' }}
    >
      {prefix}
      {element.mentionName}
      {children}
    </span>
  );
}

/** renderElement 分发器 */
export function renderElement(props: RenderElementProps) {
  switch (props.element.type) {
    case 'mention':
      return (
        <MentionElementComponent
          {...props}
          element={props.element as MentionElement}
        />
      );
    default:
      return <ParagraphElement {...props} />;
  }
}
