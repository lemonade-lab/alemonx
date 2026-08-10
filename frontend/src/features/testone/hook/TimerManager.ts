// hooks/useTimerManager.ts
import { useRef, useCallback, useState, useEffect } from 'react';

// 定时任务类型定义
export interface TimerTask {
  id: string;
  name: string;
  interval: number; // 间隔时间（毫秒）
  callback: () => void;
  autoStart?: boolean;
  maxExecutions?: number; // 最大执行次数，不设置则无限执行
  onComplete?: () => void; // 任务完成回调
  onError?: (error: Error) => void; // 错误回调
  metadata?: Record<string, any>; // 额外的元数据
}

// 任务状态
export interface TaskStatus {
  id: string;
  name: string;
  isRunning: boolean;
  executionCount: number;
  startTime?: number;
  lastExecutionTime?: number;
  nextExecutionTime?: number;
  error?: string;
  metadata?: Record<string, any>;
}

// Hook 返回类型
export interface UseTimerManagerReturn {
  // 任务操作
  createTask: (task: Omit<TimerTask, 'id'>) => string;
  startTask: (taskId: string) => boolean;
  stopTask: (taskId: string) => boolean;
  removeTask: (taskId: string) => boolean;
  updateTask: (
    taskId: string,
    updates: Partial<Omit<TimerTask, 'id'>>
  ) => boolean;

  // 批量操作
  startAllTasks: () => void;
  stopAllTasks: () => void;
  clearAllTasks: () => void;

  // 查询方法
  isTaskRunning: (taskId: string) => boolean;
  getTaskStatus: (taskId: string) => TaskStatus | null;
  getAllTasksStatus: () => TaskStatus[];
  getRunningTasks: () => TaskStatus[];
  getTasksByName: (name: string) => TaskStatus[];
  getTaskCount: () => { total: number; running: number; stopped: number };

  // 状态
  tasks: TaskStatus[];
  hasRunningTasks: boolean;
}

// 生成唯一 ID
const generateId = (): string => {
  return `timer_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
};

export const useTimerManager = (): UseTimerManagerReturn => {
  // 存储任务信息
  const tasksRef = useRef<Map<string, TimerTask>>(new Map());
  // 存储定时器 ID
  const timersRef = useRef<Map<string, NodeJS.Timeout>>(new Map());
  // 存储任务状态
  const statusRef = useRef<Map<string, TaskStatus>>(new Map());

  // 用于触发重新渲染的状态
  const [, forceUpdate] = useState({});
  const triggerUpdate = useCallback(() => {
    forceUpdate({});
  }, []);

  // 创建任务状态
  const createTaskStatus = useCallback((task: TimerTask): TaskStatus => {
    return {
      id: task.id,
      name: task.name,
      isRunning: false,
      executionCount: 0,
      error: undefined,
      metadata: task.metadata
    };
  }, []);

  // 更新任务状态
  const updateTaskStatus = useCallback(
    (taskId: string, updates: Partial<TaskStatus>) => {
      const currentStatus = statusRef.current.get(taskId);
      if (currentStatus) {
        const newStatus = { ...currentStatus, ...updates };
        statusRef.current.set(taskId, newStatus);
        triggerUpdate();
      }
    },
    [triggerUpdate]
  );

  // 创建任务
  const createTask = useCallback(
    (taskConfig: Omit<TimerTask, 'id'>): string => {
      const id = generateId();
      const task: TimerTask = {
        ...taskConfig,
        id
      };

      tasksRef.current.set(id, task);
      statusRef.current.set(id, createTaskStatus(task));

      console.log(`📝 任务已创建: ${task.name} (ID: ${id})`);

      // 如果设置了自动启动，则立即启动
      if (task.autoStart) {
        startTask(id);
      }

      triggerUpdate();
      return id;
    },
    [createTaskStatus, triggerUpdate]
  );

  // 启动任务
  const startTask = useCallback(
    (taskId: string): boolean => {
      const task = tasksRef.current.get(taskId);
      const status = statusRef.current.get(taskId);

      if (!task || !status) {
        console.warn(`⚠️ 任务不存在: ${taskId}`);
        return false;
      }

      if (status.isRunning) {
        console.warn(`⚠️ 任务已在运行: ${task.name}`);
        return false;
      }

      try {
        const startTime = Date.now();

        // 创建定时器
        const timerId = setInterval(() => {
          try {
            const currentStatus = statusRef.current.get(taskId);
            if (!currentStatus) {
              return;
            }

            // 检查最大执行次数
            if (
              task.maxExecutions &&
              currentStatus.executionCount >= task.maxExecutions
            ) {
              console.log(
                `✅ 任务完成: ${task.name} (已执行 ${currentStatus.executionCount} 次)`
              );
              stopTask(taskId);
              task.onComplete?.();
              return;
            }

            // 执行任务回调
            task.callback();

            // 更新状态
            updateTaskStatus(taskId, {
              executionCount: currentStatus.executionCount + 1,
              lastExecutionTime: Date.now(),
              nextExecutionTime: Date.now() + task.interval,
              error: undefined
            });
          } catch (error) {
            console.error(`❌ 任务执行错误: ${task.name}`, error);
            const errorMessage =
              error instanceof Error ? error.message : '未知错误';

            updateTaskStatus(taskId, {
              error: errorMessage
            });

            task.onError?.(
              error instanceof Error ? error : new Error(errorMessage)
            );
          }
        }, task.interval);

        // 保存定时器 ID
        timersRef.current.set(taskId, timerId);

        // 更新状态
        updateTaskStatus(taskId, {
          isRunning: true,
          startTime,
          nextExecutionTime: startTime + task.interval,
          error: undefined
        });

        console.log(`▶️ 任务已启动: ${task.name} (间隔: ${task.interval}ms)`);
        return true;
      } catch (error) {
        console.error(`❌ 启动任务失败: ${task.name}`, error);
        const errorMessage =
          error instanceof Error ? error.message : '启动失败';
        updateTaskStatus(taskId, { error: errorMessage });
        return false;
      }
    },
    [updateTaskStatus]
  );

  // 停止任务
  const stopTask = useCallback(
    (taskId: string): boolean => {
      const task = tasksRef.current.get(taskId);
      const timerId = timersRef.current.get(taskId);

      if (!task) {
        console.warn(`⚠️ 任务不存在: ${taskId}`);
        return false;
      }

      if (timerId) {
        clearInterval(timerId);
        timersRef.current.delete(taskId);
      }

      updateTaskStatus(taskId, {
        isRunning: false,
        nextExecutionTime: undefined
      });

      console.log(`⏹️ 任务已停止: ${task.name}`);
      return true;
    },
    [updateTaskStatus]
  );

  // 删除任务
  const removeTask = useCallback(
    (taskId: string): boolean => {
      const task = tasksRef.current.get(taskId);

      if (!task) {
        console.warn(`⚠️ 任务不存在: ${taskId}`);
        return false;
      }

      // 先停止任务
      stopTask(taskId);

      // 删除任务和状态
      tasksRef.current.delete(taskId);
      statusRef.current.delete(taskId);

      console.log(`🗑️ 任务已删除: ${task.name}`);
      triggerUpdate();
      return true;
    },
    [stopTask, triggerUpdate]
  );

  // 更新任务
  const updateTask = useCallback(
    (taskId: string, updates: Partial<Omit<TimerTask, 'id'>>): boolean => {
      const task = tasksRef.current.get(taskId);

      if (!task) {
        console.warn(`⚠️ 任务不存在: ${taskId}`);
        return false;
      }

      const wasRunning = statusRef.current.get(taskId)?.isRunning || false;

      // 如果任务正在运行，先停止
      if (wasRunning) {
        stopTask(taskId);
      }

      // 更新任务配置
      const updatedTask = { ...task, ...updates };
      tasksRef.current.set(taskId, updatedTask);

      // 如果之前在运行，重新启动
      if (wasRunning) {
        startTask(taskId);
      }

      console.log(`🔄 任务已更新: ${updatedTask.name}`);
      triggerUpdate();
      return true;
    },
    [stopTask, startTask, triggerUpdate]
  );

  // 启动所有任务
  const startAllTasks = useCallback(() => {
    let successCount = 0;
    tasksRef.current.forEach((_, taskId) => {
      if (startTask(taskId)) {
        successCount++;
      }
    });
    console.log(
      `▶️ 批量启动完成: ${successCount}/${tasksRef.current.size} 个任务`
    );
  }, [startTask]);

  // 停止所有任务
  const stopAllTasks = useCallback(() => {
    let successCount = 0;
    tasksRef.current.forEach((_, taskId) => {
      if (stopTask(taskId)) {
        successCount++;
      }
    });
    console.log(
      `⏹️ 批量停止完成: ${successCount}/${tasksRef.current.size} 个任务`
    );
  }, [stopTask]);

  // 清空所有任务
  const clearAllTasks = useCallback(() => {
    const taskCount = tasksRef.current.size;

    // 停止所有定时器
    timersRef.current.forEach(timerId => {
      clearInterval(timerId);
    });

    // 清空所有数据
    tasksRef.current.clear();
    timersRef.current.clear();
    statusRef.current.clear();

    console.log(`🗑️ 已清空所有任务: ${taskCount} 个`);
    triggerUpdate();
  }, [triggerUpdate]);

  // 查询任务是否运行
  const isTaskRunning = useCallback((taskId: string): boolean => {
    return statusRef.current.get(taskId)?.isRunning || false;
  }, []);

  // 获取任务状态
  const getTaskStatus = useCallback((taskId: string): TaskStatus | null => {
    return statusRef.current.get(taskId) || null;
  }, []);

  // 获取所有任务状态
  const getAllTasksStatus = useCallback((): TaskStatus[] => {
    return Array.from(statusRef.current.values());
  }, []);

  // 获取运行中的任务
  const getRunningTasks = useCallback((): TaskStatus[] => {
    return Array.from(statusRef.current.values()).filter(
      status => status.isRunning
    );
  }, []);

  // 根据名称获取任务
  const getTasksByName = useCallback((name: string): TaskStatus[] => {
    return Array.from(statusRef.current.values()).filter(
      status => status.name === name
    );
  }, []);

  // 获取任务统计
  const getTaskCount = useCallback(() => {
    const allTasks = Array.from(statusRef.current.values());
    const running = allTasks.filter(task => task.isRunning).length;
    return {
      total: allTasks.length,
      running,
      stopped: allTasks.length - running
    };
  }, []);

  // 组件卸载时清理所有定时器
  useEffect(() => {
    return () => {
      console.log('🧹 清理所有定时器...');
      timersRef.current.forEach(timerId => {
        clearInterval(timerId);
      });
      timersRef.current.clear();
    };
  }, []);

  // 计算衍生状态
  const tasks = getAllTasksStatus();
  const hasRunningTasks = getRunningTasks().length > 0;

  return {
    // 任务操作
    createTask,
    startTask,
    stopTask,
    removeTask,
    updateTask,

    // 批量操作
    startAllTasks,
    stopAllTasks,
    clearAllTasks,

    // 查询方法
    isTaskRunning,
    getTaskStatus,
    getAllTasksStatus,
    getRunningTasks,
    getTasksByName,
    getTaskCount,

    // 状态
    tasks,
    hasRunningTasks
  };
};

export default useTimerManager;
