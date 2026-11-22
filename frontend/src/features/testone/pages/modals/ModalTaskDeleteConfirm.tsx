import { useEffect } from 'react';
import dayjs from 'dayjs';
import BaseModal from './BaseModal';

interface ModalTaskDeleteConfirmProps {
  task: any | null;
  onCancel: () => void;
  onConfirm: (taskId: string) => void;
}

export default function ModalTaskDeleteConfirm({
  task,
  onCancel,
  onConfirm
}: ModalTaskDeleteConfirmProps) {
  // 如果外部删除任务（或任务消失），自动关闭
  useEffect(() => {
    if (!task) {
      onCancel();
    }
  }, [task, onCancel]);

  if (!task) return null;

  const meta = task.metadata || {};
  const frequency = meta.frequency ?? '—';
  const startIndex =
    typeof meta.startIndex === 'number' ? meta.startIndex + 1 : '—';
  const endIndex = typeof meta.endIndex === 'number' ? meta.endIndex + 1 : '—';
  const segmentCommands = meta.segmentCommands;
  const createdAt = meta.createdAt
    ? dayjs(meta.createdAt).format('YYYY-MM-DD HH:mm:ss')
    : '—';
  const lastExec = task.lastExecutionTime
    ? dayjs(task.lastExecutionTime).format('YYYY-MM-DD HH:mm:ss')
    : '—';
  const nextExec = task.nextExecutionTime
    ? dayjs(task.nextExecutionTime).format('YYYY-MM-DD HH:mm:ss')
    : '—';

  return (
    <BaseModal
      open={!!task}
      onCancel={onCancel}
      titleIcon={
        <div className="w-11 h-11 shrink-0 rounded-full bg-(--theme-danger-soft) flex items-center justify-center text-(--theme-danger-text) text-xl">
          🗑️
        </div>
      }
      title={
        <>
          删除任务确认
          <span className="px-2 py-0.5 rounded-full bg-(--theme-danger-soft) text-[11px] text-(--theme-danger-text) border border-red-500/30">
            危险操作
          </span>
        </>
      }
      description="删除后需重新创建；此任务当前状态会实时刷新"
      okText="🗑️ 删除任务"
      onOk={() => onConfirm(task.id)}
      width={340}
    >
      <div className="space-y-1">
        {/* Header */}
        <div className="flex items-start gap-3">
          <div className="flex-1">
            <h2 className="text-lg font-semibold text-[var(--editor-foreground)] flex items-center gap-2"></h2>
            <p className="text-[11px] mt-1 leading-relaxed text-[var(--descriptionForeground)]"></p>
          </div>
        </div>

        {/* Task Info */}
        <div className="rounded-md border border-[var(--panel-border)] bg-[var(--editor-inactiveSelection)]/50 p-4 space-y-3">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <div className="text-xs font-medium text-[var(--descriptionForeground)]">
                任务名称
              </div>
              <div className="text-sm font-semibold truncate text-[var(--editor-foreground)]">
                {task.name || '（未命名任务）'}
              </div>
            </div>
            <div>
              {task.isRunning ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-(--theme-success-soft) text-[10px] text-(--theme-success-text) border border-green-500/30">
                  <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
                  运行中
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-zinc-500/15 text-[10px] text-zinc-400 border border-zinc-600/30">
                  已停止
                </span>
              )}
            </div>
          </div>

          <ul className="grid grid-cols-2 gap-x-4 gap-y-3 text-[11px] text-[var(--editor-foreground)]">
            {[
              { name: '频率', value: `${frequency} 秒` },
              { name: '起始指令', value: `#${startIndex}` },
              { name: '结束指令', value: `#${endIndex}` },
              {
                name: '区间数',
                value:
                  typeof segmentCommands === 'number'
                    ? `${segmentCommands}`
                    : '—'
              },
              { name: '模式', value: meta.loop === false ? '单轮' : '循环' },
              {
                name: '已执行次数',
                value: `${task.executionCount ?? 0} 次`
              },
              { name: '创建时间', value: createdAt },
              { name: '最近执行', value: lastExec },
              { name: '下次预计', value: nextExec }
            ].map((item, index) => (
              <li key={index} className="flex flex-col gap-0.5">
                <span className="opacity-60">{item.name}</span>
                <span className="font-medium">{item.value}</span>
              </li>
            ))}
          </ul>
        </div>

        {/* Danger Note */}
        <div className="rounded-md border border-red-500/30 bg-(--theme-danger-soft) p-3 text-[11px] leading-relaxed text-(--theme-danger-text)">
          ⚠️ 删除将立即停止该任务并移除记录，无法撤销。
        </div>
      </div>
    </BaseModal>
  );
}
