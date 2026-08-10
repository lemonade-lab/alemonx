import React, { useEffect, useCallback } from 'react';

import ChatWindow from '@testone/pages/ChatWindow/App';
import HelpPage from '@testone/pages/HelpPage/App';
import ConfigEditor from '@testone/pages/ConfigEditor/App';
import Header from '@testone/pages/common/Header';
import { useAppDispatch, useAppSelector } from '@testone/store';
import { setTab } from '@testone/store/slices/chatSlice';
import {
  clearGroupMessages,
  clearPrivateMessages
} from '@testone/store/slices/chatSlice';
import Footer from './common/Footer';

export default function App({ theme }: { theme: 'light' | 'dark' }) {
  const dispatch = useAppDispatch();
  const { tab } = useAppSelector(s => s.chat);
  const { lastConfig } = useAppSelector(s => s.socket);

  // 快捷键
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      const isMod = e.metaKey || e.ctrlKey;
      if (!isMod) return;
      if (e.isComposing) return;
      // 仅在“正在工作台的其他输入框里打字”时让出快捷键，避免劫持终端/编辑
      // 器的按键；焦点在按钮、空白处或测试台内部时快捷键照常生效。
      const target = e.target as HTMLElement | null;
      const typingElsewhere =
        target &&
        (target instanceof HTMLInputElement ||
          target instanceof HTMLTextAreaElement ||
          target instanceof HTMLSelectElement ||
          target.isContentEditable) &&
        typeof target.closest === 'function' &&
        !target.closest('.testone-root');
      if (typingElsewhere) return;

      switch (e.key) {
        case '1':
          e.preventDefault();
          dispatch(setTab('group'));
          break;
        case '2':
          e.preventDefault();
          dispatch(setTab('private'));
          break;
        case '3':
          e.preventDefault();
          dispatch(setTab('config'));
          break;
        case '4':
          e.preventDefault();
          dispatch(setTab('help'));
          break;
        case 'k':
          // Cmd/Ctrl+K 清空当前聊天
          e.preventDefault();
          if (tab === 'group') dispatch(clearGroupMessages());
          else if (tab === 'private') dispatch(clearPrivateMessages());
          break;
      }
    },
    [dispatch, tab]
  );

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  // Tab 页面映射
  const tabMap: Record<typeof tab, React.ReactNode> = {
    group: <ChatWindow pageType="public" />,
    private: <ChatWindow pageType="private" />,
    help: <HelpPage />,
    config: <ConfigEditor />
  };

  return (
    <div
      className="overflow-hidden flex flex-1 flex-col bg-[var(--sideBar-background)]"
      data-theme={theme}
    >
      <Header />
      {tabMap[tab]}
      <Footer>
        {lastConfig ? (
          <div>
            [{lastConfig.host}][{lastConfig.port}]
          </div>
        ) : (
          <div> </div>
        )}
      </Footer>
    </div>
  );
}
