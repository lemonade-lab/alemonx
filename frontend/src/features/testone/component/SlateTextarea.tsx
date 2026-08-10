import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  createEditor,
  Descendant,
  Editor,
  Element as SlateElement,
  Range,
  Text,
  Transforms
} from 'slate';
import { Slate, Editable, ReactEditor, withReact } from 'slate-react';
import { withHistory } from 'slate-history';
import { SendIcon } from '@testone/ui/Icons';
import { User, Channel } from '@testone/typing';
import type { DataEnums } from '@testone/typing';
import RobotOutlined from '@ant-design/icons/RobotOutlined';
import ProfileOutlined from '@ant-design/icons/ProfileOutlined';
import AudioOutlined from '@ant-design/icons/AudioOutlined';
import ThunderboltOutlined from '@ant-design/icons/ThunderboltOutlined';
import {
  withMentions,
  renderElement,
  serializeToText,
  deserializeFromText,
  EMPTY_VALUE
} from './slate/index';
import type { MentionElement } from './slate/types';

// 确保 slate 自定义类型声明被加载
import './slate/types';

export type Actions = 'commands' | 'clear' | 'chatlogs' | 'events';

/** 最大输入字符数 */
const MAX_LENGTH = 2000;
/** 发送最小间隔 (ms) */
const SEND_COOLDOWN = 500;
/** 单个媒体文件上限 (8MB) */
const MAX_MEDIA_FILE_SIZE = 8 * 1024 * 1024;

/** 清洗粘贴文本：移除控制字符，统一换行 */
function sanitizePlainText(text: string): string {
  return text
    .replace(/\r\n/g, '\n') // CRLF → LF
    .replace(/\r/g, '\n') // 单独 CR → LF
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, ''); // 移除控制字符（保留 \t \n）
}

/**
 * Slate 插件：在 insertText / insertFragment 级别拦截超长输入，
 * 避免在 onChange 里做 undo 导致的无限循环。
 */
function withMaxLength<T extends Editor>(editor: T, maxLength: number): T {
  const { insertText, insertFragment } = editor;

  const getCurrentLength = (): number => {
    try {
      return serializeToText(editor.children).length;
    } catch {
      return 0;
    }
  };

  editor.insertText = (text: string) => {
    const currentLen = getCurrentLength();
    const remaining = maxLength - currentLen;
    if (remaining <= 0) return; // 已满，静默忽略
    if (text.length > remaining) {
      text = text.slice(0, remaining);
    }
    insertText(text);
  };

  editor.insertFragment = (fragment: any) => {
    // 将片段序列化为文本来估算长度
    try {
      const fragmentText = serializeToText(fragment as Descendant[]);
      const currentLen = getCurrentLength();
      const remaining = maxLength - currentLen;
      if (remaining <= 0) return;
      if (fragmentText.length > remaining) {
        // 超限时降级为纯文本插入（已被 insertText 保护）
        const truncated = fragmentText.slice(0, remaining);
        editor.insertText(truncated);
        return;
      }
    } catch {
      // 无法序列化时按原逻辑处理
    }
    insertFragment(fragment);
  };

  return editor;
}

/** 当前触发类型 */
type TriggerType = 'user' | 'channel' | null;

interface SlateTextareaProps {
  value: string;
  onContentChange?: (content: string) => void;
  onSlateChange?: (nodes: Descendant[]) => void;
  onSendFormat?: (content: DataEnums[]) => void;
  onClickSend: () => void;
  getSlateValue?: React.MutableRefObject<() => Descendant[]>;
  userList?: User[];
  channelList?: Channel[];
  onAppClick?: (action: Actions) => void;
  onHistoryPrev?: (currentText?: string) => string | null;
  onHistoryNext?: () => string | null;
}

export default function SlateTextarea({
  value,
  onContentChange,
  onSlateChange,
  onSendFormat,
  onClickSend,
  getSlateValue,
  userList,
  channelList,
  onAppClick,
  onHistoryPrev,
  onHistoryNext
}: SlateTextareaProps) {
  const editor = useMemo(
    () =>
      withMaxLength(
        withMentions(withHistory(withReact(createEditor()))),
        MAX_LENGTH
      ),
    []
  );

  // ---- mention / channel 弹窗状态 ----
  const [triggerType, setTriggerType] = useState<TriggerType>(null);
  const [mentionSearch, setMentionSearch] = useState('');
  const [dragOver, setDragOver] = useState(false);
  const [recording, setRecording] = useState(false);
  const [recordingTime, setRecordingTime] = useState(0);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const recordChunksRef = useRef<Blob[]>([]);
  const recordTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  /** 最大录音时长 (秒) */
  const MAX_RECORD_SECONDS = 60;
  const [mentionAnchor, setMentionAnchor] = useState<{
    left: number;
    top: number;
  } | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const selectRef = useRef<HTMLDivElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  /** IME 输入法组合状态 */
  const composingRef = useRef(false);
  /** 上次发送时间戳 (防抖) */
  const lastSendTimeRef = useRef(0);
  /** 编辑器容器 ref (用于点击外部关闭弹窗) */
  const containerRef = useRef<HTMLElement | null>(null);

  const showPopup = triggerType !== null;

  // 外部同步 ref
  const onContentChangeRef = useRef(onContentChange);
  onContentChangeRef.current = onContentChange;
  const onSlateChangeRef = useRef(onSlateChange);
  onSlateChangeRef.current = onSlateChange;

  // 暴露获取当前 Slate 文档的方法
  useEffect(() => {
    if (getSlateValue) {
      getSlateValue.current = () => editor.children;
    }
  }, [editor, getSlateValue]);

  // 外部 value → Slate 同步
  const lastExternalValue = useRef(value);
  useEffect(() => {
    if (value !== lastExternalValue.current) {
      lastExternalValue.current = value;
      const nodes = deserializeFromText(value);
      editor.children = nodes;
      Editor.normalize(editor, { force: true });
      try {
        // 仅在编辑器已聚焦时移动光标，避免打断用户操作
        if (ReactEditor.isFocused(editor)) {
          Transforms.select(editor, Editor.end(editor, []));
        }
      } catch {
        // 编辑器尚未挂载 DOM 时忽略
      }
    }
  }, [value, editor]);

  // ---- 过滤列表 ----
  const filteredUsers = useMemo(() => {
    if (!userList || triggerType !== 'user') return [];
    if (!mentionSearch) return userList;
    const lower = mentionSearch.toLowerCase();
    return userList.filter(u => u.UserName.toLowerCase().includes(lower));
  }, [userList, mentionSearch, triggerType]);

  const filteredChannels = useMemo(() => {
    if (!channelList || triggerType !== 'channel') return [];
    if (!mentionSearch) return channelList;
    const lower = mentionSearch.toLowerCase();
    return channelList.filter(c => c.ChannelName.toLowerCase().includes(lower));
  }, [channelList, mentionSearch, triggerType]);

  /** 当前弹窗内的可选项 */
  const popupItems = triggerType === 'user' ? filteredUsers : filteredChannels;

  // ---- selectedIndex 钳位: 过滤列表变短时避免越界 ----
  useEffect(() => {
    if (popupItems.length > 0 && selectedIndex >= popupItems.length) {
      setSelectedIndex(popupItems.length - 1);
    }
  }, [popupItems.length, selectedIndex]);

  // ---- 点击编辑器外部关闭弹窗 ----
  useEffect(() => {
    if (!showPopup) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setTriggerType(null);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [showPopup]);

  // ---- 获取当前段落起始到光标的文本 ----
  const getTextBeforeCursor = useCallback((): string | null => {
    const { selection } = editor;
    if (!selection || !Range.isCollapsed(selection)) return null;

    const [start] = Range.edges(selection);

    // 找到包含光标的最近块级元素 (paragraph)
    const blockEntry = Editor.above(editor, {
      at: start,
      match: n => SlateElement.isElement(n) && Editor.isBlock(editor, n)
    });
    if (!blockEntry) return null;

    const blockStart = Editor.start(editor, blockEntry[1]);
    const range = Editor.range(editor, blockStart, start);
    return Editor.string(editor, range);
  }, [editor]);

  // ---- 检测 @ 或 # 触发 ----
  const detectMention = useCallback(() => {
    const text = getTextBeforeCursor();
    if (text === null) {
      setTriggerType(null);
      return;
    }

    // 匹配 @xxx 或 #xxx (未闭合, 光标在末尾)
    const match = text.match(/([@#])(\S*)$/);
    if (match) {
      const trigger = match[1];
      const search = match[2];
      const type: TriggerType = trigger === '#' ? 'channel' : 'user';
      setTriggerType(type);
      setMentionSearch(search);
      setSelectedIndex(0);
    } else {
      setTriggerType(null);
    }
  }, [getTextBeforeCursor]);

  // ---- 弹窗位置：延迟到 DOM 渲染后再计算 ----
  useEffect(() => {
    if (triggerType === null) {
      setMentionAnchor(null);
      return;
    }
    // requestAnimationFrame 确保 DOM 已更新
    const id = requestAnimationFrame(() => {
      try {
        const { selection } = editor;
        if (selection) {
          const domRange = ReactEditor.toDOMRange(editor, selection);
          const rect = domRange.getBoundingClientRect();
          setMentionAnchor({ left: rect.left, top: rect.top });
        }
      } catch {
        // 定位失败时不取消弹窗，保留上次位置
      }
    });
    return () => cancelAnimationFrame(id);
  }, [triggerType, mentionSearch, editor]);

  // ---- 插入 user mention ----
  const insertUserMention = useCallback(
    (user: User) => {
      const text = getTextBeforeCursor();
      if (text === null) return;

      const atMatch = text.match(/@(\S*)$/);
      if (atMatch) {
        for (let i = 0; i < atMatch[0].length; i++) {
          Editor.deleteBackward(editor, { unit: 'character' });
        }
      }

      const mention: MentionElement = {
        type: 'mention',
        belong: user.UserId === 'everyone' ? 'everyone' : 'user',
        mentionId: user.UserId,
        mentionName: user.UserName,
        children: [{ text: '' }]
      };
      Transforms.insertNodes(editor, mention);
      Transforms.insertText(editor, ' ');
      Transforms.move(editor);

      setTriggerType(null);
      ReactEditor.focus(editor);
    },
    [editor, getTextBeforeCursor]
  );

  // ---- 插入 channel mention ----
  const insertChannelMention = useCallback(
    (channel: Channel) => {
      const text = getTextBeforeCursor();
      if (text === null) return;

      const hashMatch = text.match(/#(\S*)$/);
      if (hashMatch) {
        for (let i = 0; i < hashMatch[0].length; i++) {
          Editor.deleteBackward(editor, { unit: 'character' });
        }
      }

      const mention: MentionElement = {
        type: 'mention',
        belong: 'channel',
        mentionId: channel.ChannelId,
        mentionName: channel.ChannelName,
        children: [{ text: '' }]
      };
      Transforms.insertNodes(editor, mention);
      Transforms.insertText(editor, ' ');
      Transforms.move(editor);

      setTriggerType(null);
      ReactEditor.focus(editor);
    },
    [editor, getTextBeforeCursor]
  );

  /** 选择弹窗中当前高亮项 */
  const selectCurrentItem = useCallback(
    (index: number) => {
      if (triggerType === 'user' && filteredUsers[index]) {
        insertUserMention(filteredUsers[index]);
      } else if (triggerType === 'channel' && filteredChannels[index]) {
        insertChannelMention(filteredChannels[index]);
      }
    },
    [
      triggerType,
      filteredUsers,
      filteredChannels,
      insertUserMention,
      insertChannelMention
    ]
  );

  // ---- 检测当前内容是否为空 ----
  const isEditorEmpty = useCallback((): boolean => {
    return editor.children.every(node => {
      if (SlateElement.isElement(node) && node.type === 'paragraph') {
        return node.children.every(
          child => Text.isText(child) && child.text === ''
        );
      }
      return false;
    });
  }, [editor]);

  // ---- onChange ----
  const onChange = useCallback(
    (newValue: Descendant[]) => {
      onSlateChangeRef.current?.(newValue);
      const text = serializeToText(newValue);
      lastExternalValue.current = text;
      onContentChangeRef.current?.(text);
      // IME 组合期间不做 mention 检测
      if (!composingRef.current) {
        detectMention();
      }
    },
    [detectMention]
  );

  // ---- 发送 ----
  const onSend = useCallback(async () => {
    // 空内容守卫
    if (isEditorEmpty()) return;
    // 发送频率限制
    const now = Date.now();
    if (now - lastSendTimeRef.current < SEND_COOLDOWN) return;
    lastSendTimeRef.current = now;

    try {
      await onClickSend();
      Transforms.delete(editor, {
        at: {
          anchor: Editor.start(editor, []),
          focus: Editor.end(editor, [])
        }
      });
      if (editor.children.length === 0) {
        Transforms.insertNodes(editor, {
          type: 'paragraph',
          children: [{ text: '' }]
        });
      }
      Editor.normalize(editor, { force: true });
    } catch (e) {
      console.error('发送失败:', e);
    }
  }, [onClickSend, editor, isEditorEmpty]);

  // ---- 键盘事件 ----
  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      // IME 组合期间 (如中文输入法) 不拦截任何按键
      if (e.nativeEvent.isComposing || e.key === 'Process') return;

      if (showPopup && popupItems.length > 0) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          setSelectedIndex(i => (i + 1) % popupItems.length);
          return;
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault();
          setSelectedIndex(i => (i <= 0 ? popupItems.length - 1 : i - 1));
          return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          e.preventDefault();
          selectCurrentItem(selectedIndex);
          return;
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          setTriggerType(null);
          return;
        }
      }

      if (e.key === 'Enter' && e.ctrlKey) {
        e.preventDefault();
        Editor.insertBreak(editor);
      } else if (e.key === 'Enter') {
        e.preventDefault();
        onSend();
      }

      // ↑ 历史回溯：光标在第一行或编辑器为空时触发
      if (e.key === 'ArrowUp' && onHistoryPrev) {
        const { selection } = editor;
        const empty = isEditorEmpty();
        // 仅在编辑器为空或光标在第一行开头时触发
        const atTopLine = selection
          ? selection.anchor.path[0] === 0 && selection.anchor.offset === 0
          : false;
        if (empty || atTopLine) {
          const currentText = serializeToText(editor.children);
          const prev = onHistoryPrev(currentText);
          if (prev !== null) {
            e.preventDefault();
            const nodes = deserializeFromText(prev);
            editor.children = nodes;
            Editor.normalize(editor, { force: true });
            try {
              Transforms.select(editor, Editor.end(editor, []));
            } catch {
              /* ignore */
            }
            onContentChangeRef.current?.(prev);
          }
        }
      }

      // ↓ 历史回溯：光标在最后一行末尾时触发
      if (e.key === 'ArrowDown' && onHistoryNext) {
        const { selection } = editor;
        const end = Editor.end(editor, []);
        const atBottom = selection
          ? selection.anchor.path[0] === end.path[0] &&
            selection.anchor.offset === end.offset
          : false;
        if (atBottom) {
          const next = onHistoryNext();
          if (next !== null) {
            e.preventDefault();
            const nodes = deserializeFromText(next);
            editor.children = nodes;
            Editor.normalize(editor, { force: true });
            try {
              Transforms.select(editor, Editor.end(editor, []));
            } catch {
              /* ignore */
            }
            onContentChangeRef.current?.(next);
          }
        }
      }
    },
    [showPopup, popupItems, selectedIndex, selectCurrentItem, editor, onSend]
  );

  // ---- 粘贴内容清洗: 只保留纯文本，清除控制字符 ----
  const fileToDataUrl = useCallback((file: File): Promise<string> => {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result || ''));
      reader.onerror = () => reject(new Error('文件读取失败'));
      reader.readAsDataURL(file);
    });
  }, []);

  const toMediaSegment = useCallback(
    async (file: File): Promise<DataEnums | null> => {
      if (!file || file.size <= 0) return null;
      if (file.size > MAX_MEDIA_FILE_SIZE) {
        console.warn('文件过大，已跳过:', file.name);
        return null;
      }

      const dataUrl = await fileToDataUrl(file);
      const base64 = dataUrl.split(',')[1] || '';
      if (!base64) return null;
      const value = `base64://${base64}`;

      if (file.type.startsWith('image/')) {
        return {
          type: 'ImageURL',
          value,
          options: { mime: file.type, filename: file.name }
        } as any;
      }
      if (file.type.startsWith('audio/')) {
        return {
          type: 'Audio',
          value,
          options: { mime: file.type, filename: file.name }
        } as any;
      }
      if (file.type.startsWith('video/')) {
        return {
          type: 'Video',
          value,
          options: { mime: file.type, filename: file.name }
        } as any;
      }
      return {
        type: 'Attachment',
        value,
        options: {
          mime: file.type || 'application/octet-stream',
          filename: file.name,
          size: file.size
        }
      } as any;
    },
    [fileToDataUrl]
  );

  const sendMediaFiles = useCallback(
    async (files: File[]) => {
      if (!files.length || !onSendFormat) return;
      const segments: DataEnums[] = [];
      for (const file of files.slice(0, 10)) {
        try {
          const segment = await toMediaSegment(file);
          if (segment) segments.push(segment);
        } catch (e) {
          console.warn('媒体文件处理失败', e);
        }
      }
      if (segments.length > 0) {
        onSendFormat(segments);
      }
    },
    [onSendFormat, toMediaSegment]
  );

  const onPaste = useCallback(
    (e: React.ClipboardEvent) => {
      e.preventDefault();
      const files = Array.from(e.clipboardData.files || []);
      if (files.length > 0) {
        void sendMediaFiles(files);
        return;
      }
      const raw = e.clipboardData.getData('text/plain');
      if (!raw) return;
      const plain = sanitizePlainText(raw);
      if (!plain) return;
      // 插入纯文本（长度由 withMaxLength 插件保护）
      Editor.insertText(editor, plain);
    },
    [editor, sendMediaFiles]
  );

  // ---- 录音功能 ----
  const stopRecording = useCallback(() => {
    if (
      mediaRecorderRef.current &&
      mediaRecorderRef.current.state !== 'inactive'
    ) {
      mediaRecorderRef.current.stop();
    }
    if (recordTimerRef.current) {
      clearInterval(recordTimerRef.current);
      recordTimerRef.current = null;
    }
    setRecording(false);
    setRecordingTime(0);
  }, []);

  const startRecording = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : 'audio/webm';
      const recorder = new MediaRecorder(stream, { mimeType });
      recordChunksRef.current = [];
      mediaRecorderRef.current = recorder;

      recorder.ondataavailable = e => {
        if (e.data.size > 0) recordChunksRef.current.push(e.data);
      };

      recorder.onstop = () => {
        stream.getTracks().forEach(t => t.stop());
        const blob = new Blob(recordChunksRef.current, { type: mimeType });
        if (blob.size > 0 && blob.size <= MAX_MEDIA_FILE_SIZE && onSendFormat) {
          const reader = new FileReader();
          reader.onload = () => {
            const dataUrl = String(reader.result || '');
            const base64 = dataUrl.split(',')[1] || '';
            if (base64) {
              onSendFormat([
                {
                  type: 'Audio',
                  value: `base64://${base64}`,
                  options: {
                    mime: mimeType,
                    filename: `recording_${Date.now()}.webm`
                  }
                } as any
              ]);
            }
          };
          reader.readAsDataURL(blob);
        }
        recordChunksRef.current = [];
      };

      recorder.start();
      setRecording(true);
      setRecordingTime(0);
      recordTimerRef.current = setInterval(() => {
        setRecordingTime(prev => {
          if (prev + 1 >= MAX_RECORD_SECONDS) {
            stopRecording();
            return 0;
          }
          return prev + 1;
        });
      }, 1000);
    } catch {
      console.warn('无法访问麦克风');
    }
  }, [onSendFormat, stopRecording]);

  const toggleRecording = useCallback(() => {
    if (recording) {
      stopRecording();
    } else {
      void startRecording();
    }
  }, [recording, stopRecording, startRecording]);

  // 组件卸载时停止录音
  useEffect(() => {
    return () => {
      if (
        mediaRecorderRef.current &&
        mediaRecorderRef.current.state !== 'inactive'
      ) {
        mediaRecorderRef.current.stop();
      }
      if (recordTimerRef.current) clearInterval(recordTimerRef.current);
    };
  }, []);

  // ---- 拖放文件 ----
  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      setDragOver(false);
      const files = Array.from(e.dataTransfer.files || []);
      if (files.length > 0) {
        void sendMediaFiles(files);
      }
    },
    [sendMediaFiles]
  );

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer.types.includes('Files')) {
      setDragOver(true);
    }
  }, []);

  const onDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragOver(false);
  }, []);

  // ---- render ----
  return (
    <section
      ref={containerRef}
      className="select-none w-full flex flex-row justify-center px-4 py-1"
    >
      {/* mention / channel 弹窗 */}
      {showPopup && popupItems.length > 0 && mentionAnchor && (
        <div
          ref={selectRef}
          style={{
            left: mentionAnchor.left,
            top: mentionAnchor.top - (selectRef.current?.offsetHeight || 120),
            zIndex: 1000
          }}
          className="rounded-md fixed w-[9rem] max-w-36 max-h-32 overflow-y-auto scrollbar shadow-md border border-[var(--sidebar-border)] bg-[var(--editor-background)]"
        >
          <div className="flex flex-col px-2 py-1">
            {triggerType === 'user' &&
              filteredUsers.map((user, i) => (
                <div
                  key={user.UserId}
                  onMouseDown={e => {
                    e.preventDefault();
                    insertUserMention(user);
                  }}
                  className={`rounded-md cursor-pointer p-1 text-sm ${
                    i === selectedIndex
                      ? 'bg-[var(--activityBar-background)]'
                      : 'hover:bg-[var(--activityBar-background)]'
                  }`}
                >
                  @{user.UserName}
                </div>
              ))}
            {triggerType === 'channel' &&
              filteredChannels.map((ch, i) => (
                <div
                  key={ch.ChannelId}
                  onMouseDown={e => {
                    e.preventDefault();
                    insertChannelMention(ch);
                  }}
                  className={`rounded-md cursor-pointer p-1 text-sm ${
                    i === selectedIndex
                      ? 'bg-[var(--activityBar-background)]'
                      : 'hover:bg-[var(--activityBar-background)]'
                  }`}
                >
                  #{ch.ChannelName}
                </div>
              ))}
          </div>
        </div>
      )}

      {/* 主输入区域 */}
      <div
        className={`flex gap-2 flex-col border focus-within:border-[var(--button-background)] bg-[var(--editor-background)] border-opacity-70 shadow-inner rounded-md w-full p-1 relative transition-colors ${
          dragOver
            ? 'border-[var(--button-background)] bg-[var(--activityBar-background)]'
            : 'border-[var(--editorWidget-border)]'
        }`}
        onDrop={onDrop}
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
      >
        {/* 拖拽提示 */}
        {dragOver && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-[var(--editor-background)] bg-opacity-80 rounded-md pointer-events-none">
            <span className="text-sm text-[var(--button-background)]">
              释放以发送文件
            </span>
          </div>
        )}
        {/* 工具栏 */}
        <div className="flex justify-between gap-2 shadow-inner rounded-md p-1">
          <div className="flex gap-2">
            <div>
              <div
                className="cursor-pointer flex gap-2 border rounded-md px-2 border-[var(--editorWidget-border)]"
                onClick={e => {
                  e.stopPropagation();
                  e.preventDefault();
                  onAppClick?.('commands');
                }}
              >
                <RobotOutlined />
                指令管理
              </div>
            </div>
            <div>
              <div
                className="cursor-pointer flex gap-2 border rounded-md px-2 border-[var(--editorWidget-border)]"
                onClick={e => {
                  e.stopPropagation();
                  e.preventDefault();
                  fileInputRef.current?.click();
                }}
                title="选择图片/语音/视频/文件"
              >
                文件
              </div>
              <input
                ref={fileInputRef}
                type="file"
                multiple
                className="hidden"
                accept="image/*,audio/*,video/*,*/*"
                onChange={e => {
                  const files = Array.from(e.target.files || []);
                  if (files.length > 0) {
                    void sendMediaFiles(files);
                  }
                  e.currentTarget.value = '';
                }}
              />
            </div>
            <div>
              <div
                className={`cursor-pointer flex gap-2 border rounded-md px-2 border-[var(--editorWidget-border)] ${
                  recording ? 'text-red-500 border-red-500 animate-pulse' : ''
                }`}
                onClick={e => {
                  e.stopPropagation();
                  e.preventDefault();
                  toggleRecording();
                }}
                title={
                  recording
                    ? `录音中 ${recordingTime}s（点击停止并发送）`
                    : '语音录制'
                }
              >
                <AudioOutlined />
                {recording ? `${recordingTime}s` : '录音'}
              </div>
            </div>
            <div>
              <div
                className="cursor-pointer flex gap-2 border rounded-md px-2 border-[var(--editorWidget-border)]"
                onClick={e => {
                  e.stopPropagation();
                  e.preventDefault();
                  onAppClick?.('events');
                }}
                title="触发特殊事件（戳一戳、成员变更等）"
              >
                <ThunderboltOutlined />
                事件
              </div>
            </div>
          </div>
          <div>
            <div
              onClick={e => {
                e.stopPropagation();
                e.preventDefault();
                onAppClick?.('chatlogs');
              }}
              className="cursor-pointer flex gap-2 border rounded-md px-2 border-[var(--editorWidget-border)]"
              title="清空消息"
            >
              <ProfileOutlined />
              聊天记录
            </div>
          </div>
        </div>

        {/* Slate 编辑器 */}
        <Slate editor={editor} initialValue={EMPTY_VALUE} onChange={onChange}>
          <Editable
            className="px-1 resize-none overflow-y-auto border-0 focus:border-0 bg-opacity-0 bg-[var(--editor-background)] rounded-md outline-none"
            placeholder="输入内容..."
            renderElement={renderElement}
            onKeyDown={onKeyDown}
            onPaste={onPaste}
            onDrop={e => {
              // 如果有文件则走自定义处理，否则让 Slate 默认处理
              if (e.dataTransfer.types.includes('Files')) {
                onDrop(e);
              }
            }}
            onCompositionStart={() => {
              composingRef.current = true;
            }}
            onCompositionEnd={() => {
              composingRef.current = false;
              // 组合结束后再触发一次 mention 检测
              detectMention();
            }}
            style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              minHeight: '5rem',
              maxHeight: '16rem'
            }}
          />
        </Slate>

        {/* 底部工具栏 */}
        <div className="flex flex-row justify-between">
          <div className="text-[var(--textPreformat-background)] text-sm">
            Ctrl+Enter 换行
          </div>
          <div
            className="cursor-pointer"
            onClick={e => {
              e.stopPropagation();
              e.preventDefault();
              onSend();
            }}
          >
            <SendIcon />
          </div>
        </div>
      </div>
    </section>
  );
}
