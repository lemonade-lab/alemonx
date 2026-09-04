import { useState, useMemo, useCallback, useRef, useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '@testone/store';
import {
  clearLogs,
  setLogEnabled,
  setLogFilter,
  setSelectedLogId,
  EventLogEntry
} from '@testone/store/slices/eventLogSlice';
import dayjs from 'dayjs';

// 事件类型的颜色标记
const DIRECTION_COLORS: Record<string, string> = {
  send: 'text-blue-400',
  receive: 'text-green-400'
};

const DIRECTION_LABELS: Record<string, string> = {
  send: '↑ 发送',
  receive: '↓ 接收'
};

// 收集所有唯一事件类型用于筛选
function getUniqueEventTypes(entries: EventLogEntry[]): string[] {
  const set = new Set<string>();
  for (const e of entries) set.add(e.eventType);
  return Array.from(set).sort();
}

function PayloadViewer({ data }: { data: any }) {
  const text = useMemo(() => {
    try {
      return JSON.stringify(data, null, 2);
    } catch {
      return String(data);
    }
  }, [data]);

  const onCopy = useCallback(() => {
    navigator.clipboard.writeText(text).catch(() => {});
  }, [text]);

  return (
    <div className="relative">
      <button
        className="absolute top-1 right-1 text-xs px-2 py-0.5 rounded bg-[var(--button-background)] text-[var(--button-foreground)] hover:opacity-80"
        onClick={onCopy}
      >
        复制
      </button>
      <pre className="text-xs leading-relaxed whitespace-pre-wrap break-all p-2 bg-[var(--editor-background)] rounded max-h-40 overflow-auto scrollbar select-text">
        {text}
      </pre>
    </div>
  );
}

function LogEntryRow({
  entry,
  selected,
  onClick
}: {
  entry: EventLogEntry;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <div
      className={
        'flex items-center gap-2 px-2 py-0.5 text-xs cursor-pointer border-b border-[var(--panel-border)] hover:bg-[var(--list-hoverBackground)] transition-colors ' +
        (selected ? 'bg-[var(--list-activeSelectionBackground)]' : '')
      }
      onClick={onClick}
    >
      <span
        className={
          'w-10 shrink-0 font-mono ' + (DIRECTION_COLORS[entry.direction] || '')
        }
      >
        {DIRECTION_LABELS[entry.direction] || entry.direction}
      </span>
      <span className="w-18 shrink-0 text-[var(--descriptionForeground)] font-mono">
        {dayjs(entry.timestamp).format('HH:mm:ss.SSS')}
      </span>
      <span className="flex-1 truncate font-mono font-semibold">
        {entry.eventType}
      </span>
      {entry.latency != null && (
        <span
          className={
            'w-14 shrink-0 text-right font-mono ' +
            (entry.latency > 1000
              ? 'text-red-400'
              : entry.latency > 300
                ? 'text-yellow-400'
                : 'text-green-400')
          }
        >
          {entry.latency}ms
        </span>
      )}
    </div>
  );
}

/**
 * 可折叠的事件日志面板，嵌入在聊天窗口输入框下方
 */
export default function EventLogPanel() {
  const dispatch = useAppDispatch();
  const { entries, enabled, filter, selectedId } = useAppSelector(
    s => s.eventLog
  );

  const [expanded, setExpanded] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

  const eventTypes = useMemo(() => getUniqueEventTypes(entries), [entries]);

  const filtered = useMemo(() => {
    if (!filter) return entries;
    return entries.filter(e => e.eventType === filter);
  }, [entries, filter]);

  const selectedEntry = useMemo(() => {
    if (!selectedId) return null;
    return entries.find(e => e.id === selectedId) || null;
  }, [entries, selectedId]);

  // 统计
  const stats = useMemo(() => {
    const sendCount = entries.filter(e => e.direction === 'send').length;
    const recvCount = entries.filter(e => e.direction === 'receive').length;
    const latencies = entries
      .filter(e => e.latency != null)
      .map(e => e.latency!);
    const avgLatency = latencies.length
      ? Math.round(latencies.reduce((a, b) => a + b, 0) / latencies.length)
      : 0;
    const maxLatency = latencies.length ? Math.max(...latencies) : 0;
    return { sendCount, recvCount, avgLatency, maxLatency };
  }, [entries]);

  // 自动滚动到底部
  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [filtered.length, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!listRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = listRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40);
  }, []);

  return (
    <div className="border-t border-[var(--panel-border)] bg-[var(--editorWidget-background)]">
      {/* 标题栏 - 始终可见，点击展开/收起 */}
      <div
        className="flex items-center gap-2 px-3 py-1 cursor-pointer hover:bg-[var(--list-hoverBackground)] transition-colors select-none"
        onClick={() => setExpanded(v => !v)}
      >
        <span className="text-xs font-mono">{expanded ? '▼' : '▶'}</span>
        <span className="text-xs font-semibold">事件日志</span>

        {/* 快速统计 */}
        <span className="text-xs text-[var(--descriptionForeground)]">
          ↑{stats.sendCount} ↓{stats.recvCount}
          {stats.avgLatency > 0 && ` ~${stats.avgLatency}ms`}
        </span>

        {/* 最新事件预览 */}
        {!expanded && entries.length > 0 && (
          <span className="ml-auto text-xs text-[var(--descriptionForeground)] truncate max-w-48 font-mono">
            {entries[entries.length - 1].eventType}
          </span>
        )}

        {/* 录制状态 */}
        <span
          className={
            'ml-auto w-2 h-2 rounded-full shrink-0 ' +
            (enabled ? 'bg-red-500 animate-pulse' : 'bg-gray-500')
          }
          title={enabled ? '记录中' : '已暂停'}
        />
      </div>

      {/* 展开内容 */}
      {expanded && (
        <div
          className="flex flex-col"
          style={{ minHeight: 'calc(100dvh / 4)', maxHeight: '240px' }}
        >
          {/* 工具栏 */}
          <div className="flex items-center gap-2 px-3 py-1 border-t border-[var(--panel-border)]">
            {/* 录制切换 */}
            <button
              className={
                'text-xs px-1.5 py-0.5 rounded border border-[var(--panel-border)] ' +
                (enabled
                  ? 'text-red-400 border-red-400/40'
                  : 'text-[var(--descriptionForeground)]')
              }
              onClick={e => {
                e.stopPropagation();
                dispatch(setLogEnabled(!enabled));
              }}
            >
              {enabled ? '⏸ 暂停' : '⏺ 录制'}
            </button>

            {/* 筛选 */}
            <select
              className="text-xs px-1 py-0.5 rounded border border-[var(--panel-border)] bg-[var(--input-background)] text-[var(--input-foreground)]"
              value={filter}
              onClick={e => e.stopPropagation()}
              onChange={e => dispatch(setLogFilter(e.target.value))}
            >
              <option value="">全部</option>
              {eventTypes.map(t => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>

            {stats.maxLatency > 0 && (
              <span className="text-xs text-[var(--descriptionForeground)]">
                max{' '}
                <b className={stats.maxLatency > 1000 ? 'text-red-400' : ''}>
                  {stats.maxLatency}ms
                </b>
              </span>
            )}

            <span className="text-xs text-[var(--descriptionForeground)] ml-auto">
              {filtered.length}条
            </span>

            {/* 清空 */}
            <button
              className="text-xs px-1.5 py-0.5 rounded bg-[var(--button-background)] text-[var(--button-foreground)] hover:opacity-80"
              onClick={e => {
                e.stopPropagation();
                dispatch(clearLogs());
              }}
            >
              清空
            </button>
          </div>

          {/* 列表 + 详情 */}
          <div className="flex-1 flex overflow-hidden">
            <div
              ref={listRef}
              className="flex-1 overflow-y-auto scrollbar"
              onScroll={handleScroll}
            >
              {filtered.length === 0 ? (
                <div className="flex items-center justify-center py-4 text-xs text-[var(--descriptionForeground)]">
                  {enabled ? '暂无事件' : '已暂停'}
                </div>
              ) : (
                filtered.map(entry => (
                  <LogEntryRow
                    key={entry.id}
                    entry={entry}
                    selected={entry.id === selectedId}
                    onClick={() =>
                      dispatch(
                        setSelectedLogId(
                          entry.id === selectedId ? null : entry.id
                        )
                      )
                    }
                  />
                ))
              )}
            </div>

            {/* 选中详情 */}
            {selectedEntry && (
              <div className="w-64 border-l border-[var(--panel-border)] overflow-y-auto scrollbar p-2">
                <div className="text-xs mb-1">
                  <span
                    className={
                      'font-semibold mr-2 ' +
                      (DIRECTION_COLORS[selectedEntry.direction] || '')
                    }
                  >
                    {DIRECTION_LABELS[selectedEntry.direction]}
                  </span>
                  <span className="text-[var(--descriptionForeground)] font-mono">
                    {dayjs(selectedEntry.timestamp).format('HH:mm:ss.SSS')}
                  </span>
                </div>
                <div className="font-mono font-semibold text-xs mb-1">
                  {selectedEntry.eventType}
                </div>
                {selectedEntry.latency != null && (
                  <div className="text-xs mb-1">
                    耗时:{' '}
                    <span
                      className={
                        selectedEntry.latency > 1000
                          ? 'text-red-400 font-bold'
                          : selectedEntry.latency > 300
                            ? 'text-yellow-400'
                            : 'text-green-400'
                      }
                    >
                      {selectedEntry.latency}ms
                    </span>
                  </div>
                )}
                <PayloadViewer data={selectedEntry.payload} />
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
