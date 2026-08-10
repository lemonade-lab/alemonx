import { message } from 'antd';

// 全局配置消息
message.config({
  prefixCls: 'testone-message'
});

// ALemonX 集成：静态 message 不走 ConfigProvider 主题。TestoneMessageBridge
// 在主题上下文中用 App.useApp() 拿到实例后注入这里，弹窗/提示即跟随项目主题；
// 桥接就绪前（极少数中间件场景）回退到静态 message + CSS 主题覆盖。
let themedMessage: {
  info: (content: string, duration?: number) => void;
  error: (content: string, duration?: number) => void;
  success: (content: string, duration?: number) => void;
  warning: (content: string, duration?: number) => void;
} | null = null;

export const setThemedMessage = (
  instance: typeof themedMessage
): void => {
  themedMessage = instance;
};

const notify = (
  method: 'info' | 'error' | 'success' | 'warning',
  text: string
) => {
  if (themedMessage) {
    themedMessage[method](text, 8);
    return;
  }
  message[method](text, 8);
};

// 消息提示
export const Message = {
  info: (text: string) => {
    notify('info', text);
  },
  error: (text: string) => {
    notify('error', text);
  },
  success: (text: string) => {
    notify('success', text);
  },
  warning: (text: string) => {
    notify('warning', text);
  }
};
