import { useCallback, useRef } from 'react';

const STORAGE_KEY = 'testone_input_history';
const MAX_HISTORY = 50;

function loadHistory(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : [];
  } catch {
    return [];
  }
}

function saveHistory(history: string[]) {
  try {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify(history.slice(-MAX_HISTORY))
    );
  } catch {
    // ignore
  }
}

/**
 * 输入历史回溯 hook
 * push(): 发送成功后记录文本
 * prev() / next(): 返回上一条/下一条历史文本，null 表示已到尽头
 * reset(): 重置游标（开始新一轮输入后调用）
 */
export function useInputHistory() {
  const historyRef = useRef<string[]>(loadHistory());
  /** -1 表示不在历史中（当前为空/新输入） */
  const indexRef = useRef(-1);
  /** 记住进入历史前的草稿 */
  const draftRef = useRef('');

  const push = useCallback((text: string) => {
    const trimmed = text.trim();
    if (!trimmed) {
      return;
    }
    const history = historyRef.current;
    // 避免连续重复
    if (history.length > 0 && history[history.length - 1] === trimmed) {
      return;
    }
    history.push(trimmed);
    if (history.length > MAX_HISTORY) {
      history.splice(0, history.length - MAX_HISTORY);
    }
    saveHistory(history);
    indexRef.current = -1;
  }, []);

  const prev = useCallback((currentText?: string): string | null => {
    const history = historyRef.current;
    if (history.length === 0) {
      return null;
    }
    if (indexRef.current === -1) {
      // 进入历史浏览，保存草稿
      draftRef.current = currentText ?? '';
      indexRef.current = history.length - 1;
    } else if (indexRef.current > 0) {
      indexRef.current -= 1;
    } else {
      return null; // 已到最早
    }
    return history[indexRef.current];
  }, []);

  const next = useCallback((): string | null => {
    const history = historyRef.current;
    if (indexRef.current === -1) {
      return null;
    }
    if (indexRef.current < history.length - 1) {
      indexRef.current += 1;
      return history[indexRef.current];
    }
    // 回到草稿
    indexRef.current = -1;
    return draftRef.current;
  }, []);

  const reset = useCallback(() => {
    indexRef.current = -1;
    draftRef.current = '';
  }, []);

  return { push, prev, next, reset };
}
