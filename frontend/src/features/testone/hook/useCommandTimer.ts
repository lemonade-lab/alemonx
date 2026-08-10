import { useCallback, useEffect, useRef, useState } from 'react';
import { Channel, Command, User } from '@testone/typing';
import { Message } from '@testone/core/message';
import useTimerManager from '@testone/hook/TimerManager';

export interface TimerConfig {
  time: number;
  startIndex: number;
  endIndex?: number; // 结束指令索引（包含）。缺省表示最后一条
  selectedIndices?: number[]; // 选中的指令索引列表（优先于 startIndex/endIndex）
  user?: User;
  channel?: Channel;
  loop?: boolean; // 是否循环执行（默认 true）
}

interface UseCommandTimerParams {
  commands: Command[];
  status: boolean;
  pageType: 'public' | 'private';
  onCommand: (
    command: Command,
    ctx?: {
      curchannel?: Channel;
      curuser?: User;
    }
  ) => void;
  requireTarget?: boolean;
  requireBothUserAndChannel?: boolean;
  onClose: () => void;
  onStart: () => void;
}

interface UseCommandTimerResult {
  taskToDelete: any | null;
  setTaskToDelete: (t: any | null) => void;

  timerConfig: TimerConfig;
  setTimerConfig: React.Dispatch<React.SetStateAction<TimerConfig>>;

  onSubmitTimer: (
    e: React.FormEvent,
    payload: { user?: User; channel?: Channel }
  ) => void;

  stopAllCommandTasks: () => void;
  deleteSingleTask: (taskId: string) => void;

  timerManager: ReturnType<typeof useTimerManager>;
  commandTasksInfo: any[];
  hasCommandTaskRunning: () => boolean;
  getCurrentCommandIndex: () => number;
}

export default function useCommandTimer(
  params: UseCommandTimerParams
): UseCommandTimerResult {
  const {
    commands,
    status,
    pageType,
    onCommand,
    requireTarget = true,
    requireBothUserAndChannel = false,
    onStart,
    onClose
  } = params;

  const timerManager = useTimerManager();

  const [taskToDelete, setTaskToDelete] = useState<any | null>(null);

  const [timerConfig, setTimerConfig] = useState<TimerConfig>({
    time: 1.2,
    startIndex: 0,
    endIndex: undefined,
    loop: false
  });

  // 每个任务独立的当前指令索引
  const commandIndexRef = useRef<Record<string, number>>({});

  // 断线时停止所有任务
  useEffect(() => {
    if (!status && timerManager.hasRunningTasks) {
      timerManager.stopAllTasks();
      Message.info('连接断开，已停止所有定时任务');
    }
  }, [status, timerManager]);

  // 获取当前定义的循环任务（通过 metadata.loopTask 标记）
  const getCommandTasks = useCallback(
    () =>
      timerManager
        .getAllTasksStatus()
        .filter(t => t.metadata && t.metadata.loopTask === true),
    [timerManager]
  );

  const hasCommandTaskRunning = useCallback(
    () => getCommandTasks().some(t => t.isRunning),
    [getCommandTasks]
  );

  const getCurrentCommandIndex = useCallback(() => {
    const first = getCommandTasks()[0];
    if (!first) {
      return 0;
    }
    return commandIndexRef.current[first.id] || 0;
  }, [getCommandTasks]);

  // 为任务生成执行回调
  const createCommandCallback = useCallback(
    (
      taskId: string,
      indices: number[],
      loop: boolean,
      snapshotUser?: User,
      snapshotChannel?: Channel
    ) => {
      const segmentLength = indices.length;
      commandIndexRef.current[taskId] = 0; // 在 indices 数组中的位移
      const execCountRef: { current: number } = { current: 0 };

      return () => {
        if (!status) {
          Message.info('连接已断开，停止任务');
          timerManager.stopTask(taskId);
          return;
        }
        if (segmentLength <= 0) {
          Message.warning('没有可用的指令，任务终止');
          timerManager.stopTask(taskId);
          return;
        }

        if (!loop && execCountRef.current >= segmentLength) {
          Message.success('单轮执行完成，任务自动停止');
          timerManager.stopTask(taskId);
          return;
        }

        const pointer = commandIndexRef.current[taskId] % segmentLength;
        const currentIndex = indices[pointer];
        const command = commands[currentIndex];

        if (command) {
          try {
            onCommand(command, {
              curchannel: snapshotChannel,
              curuser: snapshotUser
            });
          } catch (err) {
            console.error('执行指令出错:', err);
            Message.error('执行指令时发生错误');
          }
          commandIndexRef.current[taskId] = pointer + 1;
          execCountRef.current++;
          if (!loop && execCountRef.current >= segmentLength) {
            Message.info('已完成全部指令，即将自动停止');
          }
        }
      };
    },
    [status, commands, onCommand, timerManager]
  );

  // 创建 & 启动一个新的循环任务
  const startTimerTask = useCallback(
    (override?: { user?: User; channel?: Channel }) => {
      const mergedUser = override?.user ?? timerConfig.user;
      const mergedChannel = override?.channel ?? timerConfig.channel;

      if (
        timerConfig.time === undefined ||
        timerConfig.time === null ||
        Number.isNaN(timerConfig.time)
      ) {
        Message.error('请输入定时频率（秒）');
        return;
      }
      if (timerConfig.time < 1 || timerConfig.time > 12) {
        Message.error('请输入 1 - 12 秒 之间的定时频率');
        return;
      }
      if (commands.length === 0) {
        Message.error('没有可用的指令，无法启动任务');
        return;
      }
      if (!status) {
        Message.error('连接已断开，无法启动任务');
        return;
      }

      if (requireBothUserAndChannel) {
        if (!mergedUser || !mergedChannel) {
          Message.error('请同时选择用户与频道');
          return;
        }
      } else if (requireTarget) {
        if (!mergedUser && !mergedChannel) {
          Message.error('请至少选择 用户 或 频道');
          return;
        }
      }

      // 构建实际执行的指令索引列表
      let indices: number[];
      if (
        timerConfig.selectedIndices &&
        timerConfig.selectedIndices.length > 0
      ) {
        // 使用手动选择的指令
        indices = timerConfig.selectedIndices
          .filter(i => i >= 0 && i < commands.length)
          .sort((a, b) => a - b);
      } else {
        // 回退到 startIndex/endIndex 区间
        let startIndex = Math.floor(timerConfig.startIndex ?? 0);
        if (startIndex < 0) {
          startIndex = 0;
        }
        if (startIndex >= commands.length) {
          startIndex = 0;
        }
        let endIndex =
          typeof timerConfig.endIndex === 'number'
            ? Math.floor(timerConfig.endIndex)
            : commands.length - 1;
        if (endIndex < startIndex) {
          endIndex = startIndex;
        }
        if (endIndex >= commands.length) {
          endIndex = commands.length - 1;
        }
        indices = [];
        for (let i = startIndex; i <= endIndex; i++) {
          indices.push(i);
        }
      }

      if (indices.length === 0) {
        Message.error('请至少选择一条指令');
        return;
      }

      try {
        const snapshotUser = mergedUser;
        const snapshotChannel = mergedChannel;

        const uniqueName = `指令循环发送#${Date.now()}`;
        const taskId = timerManager.createTask({
          name: uniqueName,
          interval: timerConfig.time * 1000,
          callback: () => {},
          metadata: {
            loopTask: true,
            loop: timerConfig.loop !== false,
            pageType,
            channelId: snapshotChannel?.ChannelId,
            channelName: snapshotChannel?.ChannelName,
            userId: snapshotUser?.UserId,
            userName: snapshotUser?.UserName,
            selectedIndices: indices,
            frequency: timerConfig.time,
            totalCommands: commands.length,
            segmentCommands: indices.length,
            createdAt: new Date().toISOString()
          }
        });

        timerManager.updateTask(taskId, {
          callback: createCommandCallback(
            taskId,
            indices,
            timerConfig.loop !== false,
            snapshotUser,
            snapshotChannel
          )
        });

        const started = timerManager.startTask(taskId);
        if (started) {
          setTimerConfig(prev => ({
            ...prev,
            user: snapshotUser,
            channel: snapshotChannel,
            selectedIndices: indices,
            loop: timerConfig.loop !== false
          }));
          onStart();
          const firstCmd = commands[indices[0]];
          Message.success(
            [
              '新的循环任务已启动 ✅',
              `任务: ${uniqueName}`,
              `频率: ${timerConfig.time}s`,
              `已选指令: ${indices.length} 条`,
              `模式: ${timerConfig.loop === false ? '单轮执行' : '循环执行'}`,
              `首条指令: ${firstCmd?.title || firstCmd?.text || '未知'}`
            ].join('\n')
          );
        } else {
          Message.error('定时任务启动失败');
        }
      } catch (err) {
        console.error('创建循环任务失败:', err);
        Message.error('创建循环任务失败');
      }
    },
    [
      timerConfig,
      commands,
      status,
      timerManager,
      createCommandCallback,
      pageType,
      requireTarget,
      requireBothUserAndChannel
    ]
  );

  // 停止所有循环任务
  const stopAllCommandTasks = useCallback(() => {
    console.log('Stopping all command tasks');
    const tasks = getCommandTasks();
    tasks.forEach(task => timerManager.stopTask(task.id));

    onClose();

    if (tasks.length > 0) {
      Message.success(`已停止 ${tasks.length} 个循环任务`);
    } else {
      Message.info('当前没有运行的循环任务');
    }
  }, [getCommandTasks, timerManager]);

  // 删除单个循环任务
  const deleteSingleTask = useCallback(
    (taskId: string) => {
      timerManager.removeTask(taskId);
      setTaskToDelete(null);
      Message.success('任务已删除');
    },
    [timerManager]
  );

  // 表单提交创建任务
  const onSubmitTimer = useCallback(
    (e: React.FormEvent, payload: { user?: User; channel?: Channel }) => {
      e.preventDefault();
      startTimerTask(payload);
    },
    [startTimerTask]
  );

  const commandTasksInfo = getCommandTasks();

  return {
    taskToDelete,
    setTaskToDelete,
    timerConfig,
    setTimerConfig,
    onSubmitTimer,
    stopAllCommandTasks,
    deleteSingleTask,
    timerManager,
    commandTasksInfo,
    hasCommandTaskRunning,
    getCurrentCommandIndex
  };
}
