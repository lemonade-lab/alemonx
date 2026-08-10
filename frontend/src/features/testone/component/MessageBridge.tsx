import { useEffect } from 'react';
import { App as AntApp } from 'antd';
import { setThemedMessage } from '../core/message';

/**
 * 把 antd App.useApp() 的消息实例注入 core/message，使 Message.info 等提示
 * 在 ConfigProvider 主题上下文内渲染，跟随项目亮/暗主题。
 */
export function MessageBridge() {
  const { message } = AntApp.useApp();
  useEffect(() => {
    setThemedMessage(message);
    return () => setThemedMessage(null);
  }, [message]);
  return null;
}
