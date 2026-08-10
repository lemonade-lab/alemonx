import { useMemo, useCallback, useState } from 'react';
import type { MessageItem } from '@testone/typing';
import dayjs from 'dayjs';

interface PayloadInspectorProps {
  item: MessageItem | null;
  open: boolean;
  onClose: () => void;
}

export default function PayloadInspector({
  item,
  open,
  onClose
}: PayloadInspectorProps) {
  const [tab, setTab] = useState<'formatted' | 'raw'>('formatted');

  const rawJson = useMemo(() => {
    if (!item) return '';
    try {
      return JSON.stringify(item, null, 2);
    } catch {
      return String(item);
    }
  }, [item]);

  const dataJson = useMemo(() => {
    if (!item?.data) return '';
    try {
      return JSON.stringify(item.data, null, 2);
    } catch {
      return String(item.data);
    }
  }, [item]);

  const onCopy = useCallback((text: string) => {
    navigator.clipboard.writeText(text).catch(() => {});
  }, []);

  if (!open || !item) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="w-[520px] max-h-[80vh] flex flex-col bg-[var(--editorWidget-background)] border border-[var(--panel-border)] rounded-lg shadow-xl"
        onClick={e => e.stopPropagation()}
      >
        {/* 头部 */}
        <div className="flex items-center justify-between px-4 py-2 border-b border-[var(--panel-border)]">
          <span className="text-sm font-semibold">消息结构查看器</span>
          <button
            className="text-xs px-2 py-0.5 rounded hover:opacity-80"
            onClick={onClose}
          >
            ✕
          </button>
        </div>

        {/* 消息元信息 */}
        <div className="px-4 py-2 text-xs border-b border-[var(--panel-border)] space-y-1">
          <div className="flex gap-4">
            <span>
              用户: <b>{item.UserName}</b> ({item.UserId})
            </span>
            <span>
              时间: {dayjs(item.CreateAt).format('YYYY-MM-DD HH:mm:ss')}
            </span>
          </div>
          {item.MessageId && (
            <div>
              MessageId: <span className="font-mono">{item.MessageId}</span>
            </div>
          )}
          {item.IsEdited && (
            <div className="text-yellow-500">
              已编辑{' '}
              {item.UpdateAt ? dayjs(item.UpdateAt).format('HH:mm:ss') : ''}
            </div>
          )}
          <div>
            数据段数: <b>{item.data?.length || 0}</b> | 类型:{' '}
            {item.data?.map(d => d.type).join(', ') || '无'}
          </div>
          {item.reactions && item.reactions.length > 0 && (
            <div>
              表情:{' '}
              {item.reactions
                .map(r => `${r.emoji}(${r.users.length})`)
                .join(' ')}
            </div>
          )}
        </div>

        {/* Tab 切换 */}
        <div className="flex border-b border-[var(--panel-border)]">
          <button
            className={
              'px-4 py-1 text-xs font-semibold border-b-2 transition-colors ' +
              (tab === 'formatted'
                ? 'border-blue-500 text-blue-400'
                : 'border-transparent')
            }
            onClick={() => setTab('formatted')}
          >
            DataEnums[]
          </button>
          <button
            className={
              'px-4 py-1 text-xs font-semibold border-b-2 transition-colors ' +
              (tab === 'raw'
                ? 'border-blue-500 text-blue-400'
                : 'border-transparent')
            }
            onClick={() => setTab('raw')}
          >
            完整消息体
          </button>
          <button
            className="ml-auto text-xs px-2 py-0.5 mr-2 rounded bg-[var(--button-background)] text-[var(--button-foreground)] hover:opacity-80"
            onClick={() => onCopy(tab === 'formatted' ? dataJson : rawJson)}
          >
            复制
          </button>
        </div>

        {/* 内容 */}
        <div className="flex-1 overflow-auto scrollbar p-2">
          {tab === 'formatted' ? (
            <div className="space-y-1">
              {item.data?.map((segment, i) => (
                <div
                  key={i}
                  className="rounded border border-[var(--panel-border)] p-2"
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-xs font-mono px-1.5 py-0.5 rounded bg-blue-500/20 text-blue-400">
                      {segment.type}
                    </span>
                    <span className="text-xs text-[var(--descriptionForeground)]">
                      [{i}]
                    </span>
                  </div>
                  <pre className="text-xs leading-relaxed whitespace-pre-wrap break-all select-text max-h-32 overflow-auto scrollbar">
                    {typeof segment.value === 'string'
                      ? segment.value.length > 200
                        ? segment.value.slice(0, 200) + '...(truncated)'
                        : segment.value
                      : JSON.stringify(segment.value, null, 2)}
                  </pre>
                </div>
              )) || (
                <div className="text-xs text-[var(--descriptionForeground)]">
                  无数据
                </div>
              )}
            </div>
          ) : (
            <pre className="text-xs leading-relaxed whitespace-pre-wrap break-all select-text">
              {rawJson}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
}
