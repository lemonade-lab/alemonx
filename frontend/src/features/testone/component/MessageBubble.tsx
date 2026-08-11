import dayjs from 'dayjs';
import { Buffer } from 'buffer';
import { memo, useMemo, useState } from 'react';
import { Image as Zoom } from 'antd';
import { Button } from '@testone/ui/Button';
import { type DataEnums } from '@testone/typing';
import '@testone/component/MessageBubble.scss';
import { getImageObjectUrl } from '@testone/core/imageStore';
import type { Reaction } from '@testone/typing';

// 常用表情快速选择
const QUICK_EMOJIS = ['👍', '❤️', '😂', '😮', '😢', '😠'];

// ---------------- Types ----------------
type MessageBubbleProps = {
  data: DataEnums[];
  createAt: number;
  onSend?: (value: string) => void;
  onInput?: (value: string) => void;
  onButtonClick?: (buttonId: string, buttonData: string) => void;
  reactions?: Reaction[];
  onReact?: (emoji: string) => void;
  currentUserId?: string;
};

type RendererFunction = (item: any) => React.ReactNode;

interface DataItemLike {
  type?: string;
  value?: any;
  options?: any;
}

// ---------------- Utils ----------------
const MAX_BASE64_LENGTH = 2 * 1024 * 1024; // 2MB

function isPlainObject(v: any) {
  return Object.prototype.toString.call(v) === '[object Object]';
}

function safeString(v: any): string {
  if (v == null) return '';
  if (typeof v === 'string') return v;
  try {
    return JSON.stringify(v);
  } catch {
    try {
      return String(v);
    } catch {
      return '';
    }
  }
}

function isHttpUrl(url: any) {
  return typeof url === 'string' && /^https?:\/\//i.test(url);
}

function clampBase64(base64: string): string | null {
  if (!base64) return null;
  if (base64.length > MAX_BASE64_LENGTH) return null;
  return base64;
}

// ---------------- Text Styles ----------------
const TEXT_STYLES: Record<
  string,
  (content: React.ReactNode) => React.ReactNode
> = {
  bold: c => <strong>{c}</strong>,
  italic: c => <em>{c}</em>,
  boldItalic: c => (
    <strong>
      <em>{c}</em>
    </strong>
  ),
  block: c => <div className="block">{c}</div>,
  strikethrough: c => <s>{c}</s>,
  none: c => c,
  default: c => c
};

// ---------------- Mention Types ----------------
const MENTION_TYPES: Record<
  string,
  (value: string, userName?: string) => React.ReactNode
> = {
  all: () => <strong>@全体成员</strong>,
  everyone: () => <strong>@全体成员</strong>,
  全体成员: () => <strong>@全体成员</strong>,
  channel: v => <strong>#{v}</strong>,
  user: (v, userName) => <strong>@{userName || v}</strong>,
  guild: v => <strong>#{v}</strong>,
  default: () => <span />
};

// ---------------- Markdown Renderers ----------------
const MARKDOWN_RENDERERS: Record<string, (mdItem: any) => React.ReactNode> = {
  'MD.title': md => (
    <>
      <span className="text-xl font-bold">
        {safeString(md.value).slice(0, 200)}
      </span>
      <br />
    </>
  ),
  'MD.subtitle': md => (
    <>
      <span className="text-lg font-semibold">
        {safeString(md.value).slice(0, 200)}
      </span>
      <br />
    </>
  ),
  'MD.blockquote': md => (
    <blockquote className="message-bubble__blockquote">
      {safeString(md.value).slice(0, 500)}
    </blockquote>
  ),
  'MD.bold': md => (
    <strong className="px-1 py-0.5 rounded-md shadow-inner">
      {safeString(md.value).slice(0, 500)}
    </strong>
  ),
  'MD.divider': () => <hr />,
  'MD.text': md => <span>{safeString(md.value).slice(0, 2000)}</span>,
  'MD.content': md => (
    <div className="whitespace-pre-wrap break-words">
      {safeString(md.value).slice(0, 8000)}
    </div>
  ),
  'MD.link': md => {
    const url = safeString(md.value?.url).slice(0, 1000);
    const text = safeString(md.value?.text).slice(0, 200);
    if (!isHttpUrl(url)) return null;
    return (
      <a href={url} target="_blank" rel="noopener noreferrer">
        {text || url}
      </a>
    );
  },
  'MD.image': md => {
    const url = safeString(md.value);
    if (!isHttpUrl(url)) return null;
    const w = Number(md.options?.width) || 100;
    const h = Number(md.options?.height) || 100;
    return (
      <>
        <Zoom
          style={{ width: `${w}px`, height: `${h}px` }}
          className="testone-media rounded-md"
          src={url}
          alt="image"
        />
        <br />
      </>
    );
  },
  'MD.italic': md => <em>{safeString(md.value).slice(0, 500)}</em>,
  'MD.italicStar': md => (
    <em className="italic">{safeString(md.value).slice(0, 500)}</em>
  ),
  'MD.strikethrough': md => <s>{safeString(md.value).slice(0, 500)}</s>,
  'MD.newline': () => <br />,
  'MD.mention': md => {
    const value = safeString(md.value || 'everyone');
    const belong = safeString(md.options?.belong || 'user');
    if (value === 'everyone') return <strong>@全体成员</strong>;
    if (belong === 'channel') return <strong>#{value}</strong>;
    return <strong>@{value}</strong>;
  },
  'MD.button': md => {
    const text = safeString(md.value).slice(0, 120) || '按钮';
    return (
      <span className="inline-block px-2 py-0.5 rounded border">[{text}]</span>
    );
  },
  'MD.code': md => {
    const code = safeString(md.value).slice(0, 5000);
    return (
      <pre className="p-2 rounded bg-[var(--activityBar-background)] overflow-x-auto text-xs">
        <code>{code}</code>
      </pre>
    );
  },
  'MD.list': md => {
    const list = Array.isArray(md.value) ? md.value : [];
    return (
      <ul className="list-disc ml-4">
        {list.slice(0, 200).map((li: any, i: number) => {
          const text = safeString(li?.value?.text || li?.value || li).slice(
            0,
            500
          );
          return <li key={i}>{text}</li>;
        })}
      </ul>
    );
  },
  'MD.template': () => <span>暂不支持模板</span>
};

// ---------------- Unsupported Components ----------------
const UNSUPPORTED_COMPONENTS: Record<string, () => React.ReactNode> = {
  'Ark.BigCard': () => <div>暂不支持 Ark.BigCard</div>,
  'Ark.Card': () => <div>暂不支持 Ark.Card</div>,
  'Ark.list': () => <div>暂不支持 Ark.list</div>,
  'ImageFile': () => <div>暂不支持文件图片</div>,
  'MD.template': () => <div>暂不支持</div>
};

// ---------------- Audio/Video Renderers ----------------
const renderAudio = (item: any): React.ReactNode => {
  try {
    const raw = safeString(item?.value);
    if (!raw) return <span>音频数据缺失</span>;
    let src = raw;
    if (raw.startsWith('base64://')) {
      const mime = safeString(item?.options?.mime) || 'audio/mpeg';
      src = `data:${mime};base64,${raw.replace('base64://', '')}`;
    } else if (!isHttpUrl(raw)) {
      return <span>不支持的音频格式</span>;
    }
    return (
      <audio
        controls
        preload="metadata"
        className="testone-media"
        src={src}
      />
    );
  } catch {
    return <span>音频解析失败</span>;
  }
};

const renderVideo = (item: any): React.ReactNode => {
  try {
    const raw = safeString(item?.value);
    if (!raw) return <span>视频数据缺失</span>;
    let src = raw;
    if (raw.startsWith('base64://')) {
      const mime = safeString(item?.options?.mime) || 'video/mp4';
      src = `data:${mime};base64,${raw.replace('base64://', '')}`;
    } else if (!isHttpUrl(raw)) {
      return <span>不支持的视频格式</span>;
    }
    return (
      <video
        controls
        preload="metadata"
        className="testone-media rounded-md"
        src={src}
      />
    );
  } catch {
    return <span>视频解析失败</span>;
  }
};

const renderAttachment = (item: any): React.ReactNode => {
  try {
    const url = safeString(item?.value);
    const filename = safeString(item?.options?.filename) || '文件';
    if (!url) return <span>附件缺失</span>;
    if (url.startsWith('base64://')) {
      const mime =
        safeString(item?.options?.mime) || 'application/octet-stream';
      const dataUrl = `data:${mime};base64,${url.replace('base64://', '')}`;
      return (
        <a
          href={dataUrl}
          download={filename}
          className="inline-flex items-center gap-1 px-2 py-1 rounded bg-[var(--activityBar-background)] text-sm"
        >
          📎 {filename}
        </a>
      );
    }
    if (isHttpUrl(url)) {
      return (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 px-2 py-1 rounded bg-[var(--activityBar-background)] text-sm"
        >
          📎 {filename}
        </a>
      );
    }
    return <span>📎 {filename}</span>;
  } catch {
    return <span>附件解析失败</span>;
  }
};

// ---------------- Render Helpers ----------------
const renderImage = (item: any): React.ReactNode => {
  try {
    const raw = item?.value;
    if (!raw) return null;
    let buffer: Buffer;
    if (Array.isArray(raw)) {
      buffer = Buffer.from(raw as any);
    } else if (typeof raw === 'string') {
      const limited = clampBase64(raw);
      if (!limited) return <span>图片太大或无效</span>;
      buffer = Buffer.from(limited, 'base64');
    } else {
      return null;
    }
    const base64String = buffer.toString('base64');
    const url = `data:image/png;base64,${base64String}`;
    return (
      <Zoom
        className="testone-media rounded-md"
        src={url}
        alt="Image"
      />
    );
  } catch (e) {
    console.warn('renderImage error', e);
    return <span>图片解析失败</span>;
  }
};

const renderImageURL = (item: any): React.ReactNode => {
  try {
    const raw = safeString(item?.value);
    if (!raw) return null;
    if (raw.startsWith('base64://')) {
      const mime = safeString(item?.options?.mime) || 'image/png';
      const src = `data:${mime};base64,${raw.replace('base64://', '')}`;
      return (
        <Zoom
          className="testone-media rounded-md"
          src={src}
          alt="ImageURL"
        />
      );
    }
    if (!isHttpUrl(raw)) return null;
    return (
      <Zoom
        className="testone-media rounded-md"
        src={raw}
        alt="ImageURL"
      />
    );
  } catch (e) {
    console.warn('renderImageURL error', e);
    return <span>图片地址无效</span>;
  }
};

const renderText = (item: any): React.ReactNode => {
  try {
    let value = safeString(item?.value).slice(0, 5000);
    if (!value) return null;
    const styleKey = safeString(item?.options?.style) || 'default';
    const styleRenderer = TEXT_STYLES[styleKey] || TEXT_STYLES.default;
    const parts = value.split('\n');
    const content =
      parts.length > 1 ? (
        <span>
          {parts.map((line, i) => (
            <span key={i}>
              {line}
              <br />
            </span>
          ))}
        </span>
      ) : (
        value
      );
    return <span>{styleRenderer(content)}</span>;
  } catch (e) {
    console.warn('renderText error', e);
    return <span>文本错误</span>;
  }
};

const renderMention = (item: any): React.ReactNode => {
  try {
    const value = safeString(item?.value);
    if (!value) return null;
    if (['all', 'everyone', '全体成员'].includes(value)) {
      return (
        <span>{MENTION_TYPES[value]?.(value) || MENTION_TYPES.all(value)}</span>
      );
    }
    const belong = safeString(item?.options?.belong) || 'default';
    const userName = item?.options?.payload?.name;
    const renderer = MENTION_TYPES[belong] || MENTION_TYPES.default;
    return <span>{renderer(value, userName)}</span>;
  } catch (e) {
    console.warn('renderMention error', e);
    return null;
  }
};

const renderButtonGroup = (
  item: any,
  onSend: (v: string) => void,
  onInput: (v: string) => void
): React.ReactNode => {
  try {
    if (item?.options?.template_id) return <div>暂不支持按钮模板</div>;
    const groups = item?.value;
    if (!Array.isArray(groups)) return <div>按钮数据错误</div>;
    return (
      <div className="flex w-full flex-col items-start gap-3">
        {groups.slice(0, 20).map((group: any, gi: number) => {
          const bts = Array.isArray(group?.value) ? group.value : [];
          return (
            <div key={gi} className="flex w-full flex-wrap items-start gap-2">
              {bts.slice(0, 30).map((bt: any, bi: number) => {
                const meta =
                  typeof bt?.value === 'string'
                    ? { label: bt.value, title: bt.value }
                    : bt?.value || { label: 'Button', title: '' };
                const autoEnter = !!bt?.options?.autoEnter;
                const data =
                  typeof bt?.options?.data === 'string'
                    ? { click: bt.options.data }
                    : bt?.options?.data || {};
                const label = safeString(meta.label).slice(0, 40) || 'Button';
                return (
                  <Button
                    key={bi}
                    className="max-w-full whitespace-normal break-words text-left"
                    onClick={e => {
                      e.stopPropagation();
                      e.preventDefault();
                      if (autoEnter) {
                        onSend(safeString(data.click).slice(0, 2000));
                      } else {
                        onInput(safeString(data.click).slice(0, 2000));
                      }
                    }}
                  >
                    {label}
                  </Button>
                );
              })}
            </div>
          );
        })}
      </div>
    );
  } catch (e) {
    console.warn('renderButtonGroup error', e);
    return <div>按钮渲染失败</div>;
  }
};

const renderMarkdown = (
  item: any,
  onSend: (v: string) => void = () => {},
  onInput: (v: string) => void = () => {}
): React.ReactNode => {
  try {
    const markdown = Array.isArray(item?.value) ? item.value : [];
    return (
      <div className="mb-2 w-full">
        {markdown.slice(0, 200).map((md: any, i: number) => {
          if (!md || typeof md !== 'object') return null;
          const type = safeString(md.type);

          if (type === 'MD.button') {
            const label = safeString(md.value).slice(0, 120) || '按钮';
            const data = safeString(md?.options?.data || md.value).slice(
              0,
              2000
            );
            const autoEnter = !!md?.options?.autoEnter;
            return (
              <button
                type="button"
                key={`mdbtn-${i}`}
                className="inline whitespace-normal break-words rounded-none border-0 bg-transparent px-0 py-0 align-baseline font-medium leading-inherit text-[var(--button-background)] underline-offset-2 hover:underline"
                onClick={e => {
                  e.preventDefault();
                  e.stopPropagation();
                  if (autoEnter) onSend(data);
                  else onInput(data);
                }}
              >
                {label}
              </button>
            );
          }

          const renderer = MARKDOWN_RENDERERS[type];
          if (!renderer) return null;
          try {
            return <span key={`md-${i}`}>{renderer(md)}</span>;
          } catch (err) {
            console.warn('markdown item render error', err);
            return null;
          }
        })}
      </div>
    );
  } catch (e) {
    console.warn('renderMarkdown error', e);
    return <div>Markdown 数据错误</div>;
  }
};

const renderMarkdownOriginal = (item: any): React.ReactNode => {
  try {
    const content = safeString(item?.value);
    if (!content) return null;
    return (
      <pre className="whitespace-pre-wrap break-words text-sm">
        {content.slice(0, 12000)}
      </pre>
    );
  } catch (e) {
    console.warn('renderMarkdownOriginal error', e);
    return <div>Markdown 原文解析失败</div>;
  }
};

/**
 * 带 interaction 回调的按钮渲染（按钮点击同时发送 interaction.create 事件）
 */
const renderButtonGroupWithInteraction = (
  item: any,
  onSend: (v: string) => void,
  onInput: (v: string) => void,
  onButtonClick: (buttonId: string, buttonData: string) => void
): React.ReactNode => {
  try {
    if (item?.options?.template_id) return <div>暂不支持按钮模板</div>;
    const groups = item?.value;
    if (!Array.isArray(groups)) return <div>按钮数据错误</div>;
    return (
      <div className="flex w-full flex-col items-start gap-3">
        {groups.slice(0, 20).map((group: any, gi: number) => {
          const bts = Array.isArray(group?.value) ? group.value : [];
          return (
            <div key={gi} className="flex w-full flex-wrap items-start gap-2">
              {bts.slice(0, 30).map((bt: any, bi: number) => {
                const meta =
                  typeof bt?.value === 'string'
                    ? { label: bt.value, title: bt.value }
                    : bt?.value || { label: 'Button', title: '' };
                const autoEnter = !!bt?.options?.autoEnter;
                const data =
                  typeof bt?.options?.data === 'string'
                    ? { click: bt.options.data }
                    : bt?.options?.data || {};
                const label = safeString(meta.label).slice(0, 40) || 'Button';
                const buttonId = safeString(
                  bt?.options?.id || `btn_${gi}_${bi}`
                );
                const clickData = safeString(data.click).slice(0, 2000);
                return (
                  <Button
                    key={bi}
                    className="max-w-full whitespace-normal break-words text-left"
                    onClick={e => {
                      e.stopPropagation();
                      e.preventDefault();
                      // 发送 interaction.create 事件给 bot
                      onButtonClick(buttonId, clickData);
                      // 同时执行本地操作
                      if (autoEnter) {
                        onSend(clickData);
                      } else {
                        onInput(clickData);
                      }
                    }}
                  >
                    {label}
                  </Button>
                );
              })}
            </div>
          );
        })}
      </div>
    );
  } catch (e) {
    console.warn('renderButtonGroup error', e);
    return <div>按钮渲染失败</div>;
  }
};

// ---------------- Component Renderer Map ----------------
const COMPONENT_RENDERERS: Record<string, RendererFunction> = {
  'Text': renderText,
  'Mention': renderMention,
  'Image': renderImage,
  'ImageURL': renderImageURL,
  'ImageRef': (item: any) => {
    try {
      const hash = item?.value?.hash;
      if (!hash) return <span>图片引用缺失</span>;
      const url = getImageObjectUrl(hash);
      if (!url) return <span>图片已释放</span>;
      return (
        <Zoom
        className="testone-media rounded-md"
          src={url}
          alt="ImageRef"
        />
      );
    } catch (e) {
      console.warn('render ImageRef error', e);
      return <span>图片引用错误</span>;
    }
  },
  'BT.group': (item: any) => (
    <>
      {renderButtonGroup(
        item,
        () => {},
        () => {}
      )}
    </>
  ), // 占位，实际调用时替换
  'Markdown': renderMarkdown,
  'MarkdownOriginal': renderMarkdownOriginal,
  'Audio': renderAudio,
  'Video': renderVideo,
  'Attachment': renderAttachment,
  // 直接映射 markdown 子类型（如果后端直接下发）
  'MD.title': md => MARKDOWN_RENDERERS['MD.title'](md),
  'MD.subtitle': md => MARKDOWN_RENDERERS['MD.subtitle'](md),
  'MD.blockquote': md => MARKDOWN_RENDERERS['MD.blockquote'](md),
  'MD.bold': md => MARKDOWN_RENDERERS['MD.bold'](md),
  'MD.divider': md => MARKDOWN_RENDERERS['MD.divider'](md),
  'MD.text': md => MARKDOWN_RENDERERS['MD.text'](md),
  'MD.link': md => MARKDOWN_RENDERERS['MD.link'](md),
  'MD.image': md => MARKDOWN_RENDERERS['MD.image'](md),
  'MD.italic': md => MARKDOWN_RENDERERS['MD.italic'](md),
  'MD.italicStar': md => MARKDOWN_RENDERERS['MD.italicStar'](md),
  'MD.strikethrough': md => MARKDOWN_RENDERERS['MD.strikethrough'](md),
  'MD.newline': md => MARKDOWN_RENDERERS['MD.newline'](md),
  'MD.list': md => MARKDOWN_RENDERERS['MD.list'](md)
};

// ---------------- Main Component ----------------
const MessageBubble = ({
  data,
  createAt,
  onSend,
  onInput,
  onButtonClick,
  reactions,
  onReact,
  currentUserId
}: MessageBubbleProps) => {
  const _onSend = onSend || (() => {});
  const _onInput = onInput || (() => {});
  const _onButtonClick = onButtonClick || (() => {});
  const [showEmojiPicker, setShowEmojiPicker] = useState(false);

  const formattedTime = useMemo(() => {
    const ts =
      typeof createAt === 'number' && isFinite(createAt)
        ? createAt
        : Date.now();
    return dayjs(ts).format('YYYY-MM-DD HH:mm:ss');
  }, [createAt]);

  const renderedItems = useMemo(() => {
    if (!Array.isArray(data)) return null;
    return data.map((raw: any, index: number) => {
      const item: DataItemLike = isPlainObject(raw)
        ? raw
        : { type: 'Text', value: safeString(raw) };
      const type = safeString(item.type).trim();
      if (!type) return null;
      try {
        if (type === 'BT.group') {
          return (
            <div key={index}>
              {renderButtonGroupWithInteraction(
                item,
                _onSend,
                _onInput,
                _onButtonClick
              )}
            </div>
          );
        }
        if (type === 'Markdown') {
          return (
            <div key={index}>{renderMarkdown(item, _onSend, _onInput)}</div>
          );
        }
        const renderer = COMPONENT_RENDERERS[type];
        if (!renderer) {
          const unsupported = UNSUPPORTED_COMPONENTS[type];
          return unsupported ? <div key={index}>{unsupported()}</div> : null;
        }
        return <div key={index}>{renderer(item)}</div>;
      } catch (e) {
        console.warn('render item error', type, e);
        return null;
      }
    });
  }, [data, _onSend, _onInput, _onButtonClick]);

  return (
    <div className="flex flex-col items-start">
      <div className="flex items-end">
        <div className="message-bubble rounded-md py-1 flex flex-col relative px-2 shadow-md bg-[var(--panel-background)]">
          {renderedItems}
          <span className="absolute -bottom-3 whitespace-nowrap right-0 text-[0.5rem]">
            {formattedTime}
          </span>
        </div>
      </div>
      {/* 表情回应区域 */}
      {((reactions && reactions.length > 0) || showEmojiPicker) && (
        <div className="flex flex-wrap gap-1 mt-4 ml-1">
          {reactions?.map((r, i) => {
            const isActive = currentUserId
              ? r.users.includes(currentUserId)
              : false;
            return (
              <button
                key={i}
                onClick={() => onReact?.(r.emoji)}
                className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full text-xs border cursor-pointer select-none transition-colors ${
                  isActive
                    ? 'border-[var(--button-background)] bg-[var(--button-background)] bg-opacity-20'
                    : 'border-[var(--editorWidget-border)] hover:bg-[var(--activityBar-background)]'
                }`}
              >
                <span>{r.emoji}</span>
                <span>{r.users.length}</span>
              </button>
            );
          })}
        </div>
      )}
      {/* 添加表情按钮 */}
      <div className="relative ml-1 mt-1">
        <button
          onClick={() => setShowEmojiPicker(prev => !prev)}
          className="text-xs px-1 py-0.5 rounded border border-[var(--editorWidget-border)] hover:bg-[var(--activityBar-background)] cursor-pointer opacity-0 hover:opacity-100 transition-opacity select-none"
          title="添加表情"
        >
          😀+
        </button>
        {showEmojiPicker && (
          <div className="absolute bottom-full left-0 mb-1 flex gap-1 p-1 rounded shadow-md border border-[var(--sidebar-border)] bg-[var(--editor-background)] z-10">
            {QUICK_EMOJIS.map(emoji => (
              <button
                key={emoji}
                onClick={() => {
                  onReact?.(emoji);
                  setShowEmojiPicker(false);
                }}
                className="text-base cursor-pointer hover:scale-125 transition-transform p-0.5"
              >
                {emoji}
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default memo(MessageBubble);
