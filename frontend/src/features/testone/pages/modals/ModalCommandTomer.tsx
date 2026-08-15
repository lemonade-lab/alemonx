import { useMemo, useState } from 'react';
import { Input } from '@testone/ui/Input';
import { Switch } from '@testone/ui/Switch';
import dayjs from 'dayjs';
import BaseModal from './BaseModal';

interface CommandLike {
  title?: string;
  text?: string;
}

type SelectMode = 'range' | 'pick';

const ModalCommandTimer = ({
  open,
  onCancel,
  values,
  onChange,
  commands,
  onConfirm
}: {
  open: boolean;
  onCancel: () => void;
  values: any;
  onChange: (config: any) => void;
  commands: CommandLike[];
  onConfirm: (e: React.MouseEvent<HTMLButtonElement>) => void;
}) => {
  const {
    time,
    startIndex,
    endIndex,
    loop = false,
    selectedIndices = [] as number[],
    selectMode: savedMode
  } = values || {};
  const [cmdSearch, setCmdSearch] = useState('');
  const [mode, setMode] = useState<SelectMode>(savedMode || 'range');

  // 频率合法性判断
  const timeInvalid = !time || Number.isNaN(time) || time < 1 || time > 12;

  // 搜索过滤（仅影响显示，不影响已选中的）
  const filteredCommands = useMemo(() => {
    if (!Array.isArray(commands)) return [];
    if (!cmdSearch.trim()) return commands.map((c, i) => ({ cmd: c, idx: i }));
    const q = cmdSearch.toLowerCase();
    return commands
      .map((c, i) => ({ cmd: c, idx: i }))
      .filter(
        ({ cmd }) =>
          (cmd.title || '').toLowerCase().includes(q) ||
          (cmd.text || '').toLowerCase().includes(q)
      );
  }, [commands, cmdSearch]);

  const toggleIndex = (idx: number) => {
    onChange((prev: any) => {
      const current: number[] = Array.isArray(prev?.selectedIndices)
        ? prev.selectedIndices
        : [];
      const next = current.includes(idx)
        ? current.filter((i: number) => i !== idx)
        : [...current, idx].sort((a, b) => a - b);
      return { ...prev, selectedIndices: next };
    });
  };

  const selectAll = () => {
    const allIndices = filteredCommands.map(c => c.idx);
    onChange((prev: any) => {
      const current: number[] = Array.isArray(prev?.selectedIndices)
        ? prev.selectedIndices
        : [];
      const merged = Array.from(new Set([...current, ...allIndices])).sort(
        (a, b) => a - b
      );
      return { ...prev, selectedIndices: merged };
    });
  };

  const deselectAll = () => {
    const visibleIndices = new Set(filteredCommands.map(c => c.idx));
    onChange((prev: any) => {
      const current: number[] = Array.isArray(prev?.selectedIndices)
        ? prev.selectedIndices
        : [];
      return {
        ...prev,
        selectedIndices: current.filter(i => !visibleIndices.has(i))
      };
    });
  };

  // 执行顺序预览 (根据模式)
  const nextSequence = useMemo(() => {
    if (!Array.isArray(commands) || commands.length === 0) return [];
    if (mode === 'pick') {
      if (selectedIndices.length === 0) return [];
      const sorted = [...selectedIndices]
        .filter((i: number) => i >= 0 && i < commands.length)
        .sort((a: number, b: number) => a - b);
      return sorted.slice(0, 5).map((idx: number) => ({
        idx,
        name: commands[idx]?.title || commands[idx]?.text || `指令#${idx + 1}`
      }));
    } else {
      const realStart = Math.max(
        0,
        Math.min(startIndex ?? 0, commands.length - 1)
      );
      const realEnd =
        typeof endIndex === 'number'
          ? Math.max(realStart, Math.min(endIndex, commands.length - 1))
          : commands.length - 1;
      const arr: { idx: number; name: string }[] = [];
      for (let i = 0; i < Math.min(5, realEnd - realStart + 1); i++) {
        const realIndex = realStart + i;
        const cmd = commands[realIndex];
        arr.push({
          idx: realIndex,
          name: cmd?.title || cmd?.text || `指令#${realIndex + 1}`
        });
      }
      return arr;
    }
  }, [commands, mode, selectedIndices, startIndex, endIndex]);

  /** 当前模式下选中的指令数 */
  const selectedCount = useMemo(() => {
    if (mode === 'pick') return selectedIndices.length;
    if (!commands.length) return 0;
    const realStart = Math.max(
      0,
      Math.min(startIndex ?? 0, commands.length - 1)
    );
    const realEnd =
      typeof endIndex === 'number'
        ? Math.max(realStart, Math.min(endIndex, commands.length - 1))
        : commands.length - 1;
    return realEnd - realStart + 1;
  }, [mode, selectedIndices, startIndex, endIndex, commands.length]);

  // 预计下一次执行时间
  const nextExecPreview = useMemo(() => {
    if (timeInvalid) return '—';
    const date = dayjs().add(time, 'second');
    return date.format('HH:mm:ss');
  }, [time, timeInvalid]);

  return (
    <BaseModal
      open={open}
      onCancel={onCancel}
      titleIcon={
        <div className="w-10 h-10 rounded-full bg-gradient-to-br from-blue-500 to-indigo-500 flex items-center justify-center text-white text-lg shadow">
          ⏰
        </div>
      }
      title={
        <>
          新建循环任务<span className="text-base">🌀</span>{' '}
        </>
      }
      description={'自动按顺序循环发送指令'}
      okText="🚀 启动任务"
      onOk={onConfirm}
      width={380}
    >
      <form className="flex flex-col animate-fadeIn">
        {/* 配置区 */}
        <div className="space-y-3">
          {/* 执行频率 */}
          <div className="space-y-2">
            <label className="flex items-center justify-between gap-2 text-sm font-medium text-[var(--editor-foreground)]">
              <span>速度频率（秒）⚡</span>
              <span
                className={`truncate text-xs font-normal ${
                  timeInvalid && time
                    ? 'text-red-500'
                    : 'text-[var(--descriptionForeground)]'
                }`}
                title={
                  timeInvalid && time
                    ? '请输入 1 - 12 之间的数值'
                    : '所有任务总计不低于 1 秒，低于该频率将被限流'
                }
              >
                {timeInvalid && time ? '请输入 1 - 12 秒' : '最小 1 秒'}
              </span>
            </label>
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min="1"
                max="12"
                step="0.1"
                value={time}
                onChange={e => {
                  const value = parseFloat(e.target.value);
                  if (!isNaN(value)) {
                    onChange((prev: any) => ({
                      ...prev,
                      time: value
                    }));
                  } else {
                    onChange((prev: any) => ({
                      ...prev,
                      time: ''
                    }));
                  }
                }}
                placeholder="1 - 12"
                className={`w-full ${
                  timeInvalid && time
                    ? 'border-red-500'
                    : 'border-[var(--input-border)]'
                }`}
              />
            </div>
          </div>

          {/* 是否循环 */}
          <div className="space-y-2">
            <label className="text-sm font-medium text-[var(--editor-foreground)] flex items-center justify-between">
              <span>循环执行 🔁</span>
              <Switch
                value={loop !== false}
                onChange={checked =>
                  onChange((prev: any) => ({ ...prev, loop: checked }))
                }
              />
            </label>
            <p className="text-xs text-[var(--descriptionForeground)] leading-relaxed">
              关闭后仅执行一轮所有指令，自动停止
            </p>
          </div>

          {/* 选择指令 - 模式切换 */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="block text-sm font-medium text-[var(--editor-foreground)]">
                选择指令 🎯
              </label>
              <div className="flex gap-1 text-[11px]">
                <button
                  type="button"
                  className={`px-2 py-0.5 rounded ${mode === 'range' ? 'bg-[var(--button-background)] text-[var(--button-foreground)]' : 'bg-[var(--activityBar-background)] hover:opacity-80'}`}
                  onClick={() => {
                    setMode('range');
                    onChange((prev: any) => ({ ...prev, selectMode: 'range' }));
                  }}
                >
                  区间模式
                </button>
                <button
                  type="button"
                  className={`px-2 py-0.5 rounded ${mode === 'pick' ? 'bg-[var(--button-background)] text-[var(--button-foreground)]' : 'bg-[var(--activityBar-background)] hover:opacity-80'}`}
                  onClick={() => {
                    setMode('pick');
                    onChange((prev: any) => ({ ...prev, selectMode: 'pick' }));
                  }}
                >
                  手选模式
                </button>
              </div>
            </div>

            {mode === 'range' ? (
              <>
                {/* 区间模式：起始/结束指令 */}
                <div className="space-y-2">
                  <label className="block text-xs text-[var(--descriptionForeground)]">
                    起始指令
                  </label>
                  <select
                    value={startIndex ?? 0}
                    onChange={e => {
                      const idx = Number(e.target.value);
                      onChange((prev: any) => ({ ...prev, startIndex: idx }));
                    }}
                    className="w-full px-3 py-1.5 rounded-md text-xs bg-[var(--input-background)] hover:bg-[var(--activityBar-background)] text-[var(--input-foreground)] border border-[var(--input-border)]"
                  >
                    {Array.isArray(commands) &&
                      commands.map((item, index) => (
                        <option key={index} value={index}>
                          {index + 1}. {item.title || item.text || '未命名'}
                        </option>
                      ))}
                  </select>
                </div>
                <div className="space-y-2">
                  <label className="block text-xs text-[var(--descriptionForeground)]">
                    结束指令（含）
                  </label>
                  <select
                    value={
                      typeof endIndex === 'number'
                        ? Math.min(endIndex, commands.length - 1)
                        : commands.length - 1
                    }
                    onChange={e => {
                      const idx = Number(e.target.value);
                      onChange((prev: any) => ({ ...prev, endIndex: idx }));
                    }}
                    className="w-full px-3 py-1.5 rounded-md text-xs bg-[var(--input-background)] hover:bg-[var(--activityBar-background)] text-[var(--input-foreground)] border border-[var(--input-border)]"
                  >
                    {Array.isArray(commands) &&
                      commands.map((item, index) => (
                        <option key={index} value={index}>
                          {index + 1}. {item.title || item.text || '未命名'}
                        </option>
                      ))}
                  </select>
                  <p className="text-[11px] text-[var(--descriptionForeground)]">
                    区间：#{(startIndex ?? 0) + 1} - #
                    {(typeof endIndex === 'number'
                      ? Math.max(
                          startIndex ?? 0,
                          Math.min(endIndex, commands.length - 1)
                        )
                      : commands.length - 1) + 1}
                    （共 {selectedCount} 条）
                  </p>
                </div>
              </>
            ) : (
              <>
                {/* 手选模式：复选框列表 */}
                <div className="flex items-center justify-between text-xs text-[var(--descriptionForeground)]">
                  <span>
                    已选 {selectedIndices.length}/{commands.length}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <input
                    className="flex-1 px-2 py-1 rounded-md text-xs bg-[var(--input-background)] text-[var(--input-foreground)] border border-[var(--input-border)] outline-none"
                    placeholder="搜索指令..."
                    value={cmdSearch}
                    onChange={e => setCmdSearch(e.target.value)}
                  />
                  <button
                    type="button"
                    className="text-[11px] px-2 py-1 rounded bg-[var(--activityBar-background)] hover:opacity-80"
                    onClick={selectAll}
                  >
                    全选
                  </button>
                  <button
                    type="button"
                    className="text-[11px] px-2 py-1 rounded bg-[var(--activityBar-background)] hover:opacity-80"
                    onClick={deselectAll}
                  >
                    清除
                  </button>
                </div>
                <div className="max-h-36 overflow-y-auto scrollbar rounded-md border border-[var(--input-border)] bg-[var(--input-background)]">
                  {filteredCommands.length > 0 ? (
                    filteredCommands.map(({ cmd, idx }) => {
                      const checked = selectedIndices.includes(idx);
                      return (
                        <label
                          key={idx}
                          className={`flex items-center gap-2 px-2 py-1.5 cursor-pointer text-xs hover:bg-[var(--activityBar-background)] border-b border-[var(--panel-border)] last:border-b-0 ${
                            checked ? 'bg-blue-500/10' : ''
                          }`}
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleIndex(idx)}
                            className="accent-[var(--button-background)]"
                          />
                          <span className="text-[var(--descriptionForeground)] w-5 text-right">
                            #{idx + 1}
                          </span>
                          <span className="flex-1 truncate text-[var(--editor-foreground)]">
                            {cmd.title || cmd.text || '未命名'}
                          </span>
                        </label>
                      );
                    })
                  ) : (
                    <div className="text-xs text-center text-[var(--descriptionForeground)] py-3">
                      {cmdSearch ? '未找到匹配指令' : '暂无指令'}
                    </div>
                  )}
                </div>
                {selectedIndices.length === 0 && (
                  <p className="text-xs text-yellow-500">请至少选择一条指令</p>
                )}
              </>
            )}
          </div>

          {/* 预览区 */}
          <div className="bg-[var(--editor-inactiveSelection)]/60 backdrop-blur rounded-md p-2 border border-[var(--panel-border)] space-y-2">
            <h4 className="text-sm font-medium text-[var(--editor-foreground)] flex items-center gap-2">
              任务预览 📋
            </h4>

            <ul className="text-xs space-y-1 text-[var(--editor-foreground)]">
              <li>
                <span className="mr-1">🏷️</span>
                类型：
                <span className="font-medium">循环发送任务</span>
              </li>
              <li>
                <span className="mr-1">⚡</span>
                频率：
                <span className="font-medium">
                  {timeInvalid ? '未设置' : `${time}s / 次`}
                </span>
              </li>
              <li>
                <span className="mr-1">🧮</span>
                已选指令数：
                <span className="font-medium">
                  {selectedCount}/{commands.length}
                </span>
              </li>
              <li>
                <span className="mr-1">🔁</span>
                模式：
                <span className="font-medium">
                  {loop === false ? '单轮执行' : '循环执行'}（
                  {mode === 'range' ? '区间' : '手选'}）
                </span>
              </li>
              <li>
                <span className="mr-1">⏱️</span>
                预计首轮下一次执行时间：
                <span className="font-medium">{nextExecPreview}</span>
              </li>
            </ul>

            {/* 顺序预览 */}
            <div className="mt-2">
              <div className="text-[11px] font-medium mb-1 text-[var(--descriptionForeground)] flex items-center gap-1">
                📌 即将执行的指令顺序（预览）
              </div>
              {nextSequence.length > 0 ? (
                <div className="flex flex-col gap-1">
                  {nextSequence.map((c, i) => (
                    <div
                      key={c.idx}
                      className={`text-[11px] px-2 py-1 rounded border border-transparent ${
                        i === 0
                          ? 'bg-blue-500/15 border-blue-500/40 text-blue-400'
                          : 'bg-[var(--editor-inactiveSelection)]'
                      }`}
                    >
                      {i === 0 ? '➡️ 首条' : `→ 第 ${i + 1} 步`} ：#
                      {c.idx + 1} {c.name}
                    </div>
                  ))}
                  {selectedCount > 5 && (
                    <div className="text-[11px] opacity-70">
                      ...还有 {selectedCount - 5} 条
                    </div>
                  )}
                </div>
              ) : (
                <div className="text-[11px] opacity-70">请先选择指令</div>
              )}
            </div>
          </div>
        </div>
      </form>
    </BaseModal>
  );
};

export default ModalCommandTimer;
