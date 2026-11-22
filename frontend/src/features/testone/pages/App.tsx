import type { KeyboardEvent, ReactNode } from 'react';
import { desktopPlatform } from '../../../components/nativeWindow';

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

const shortcutTabs = { '1': 'group', '2': 'private', '3': 'config', '4': 'help' } as const;

export default function App({ theme }: { theme: 'light' | 'dark' }) {
  const dispatch = useAppDispatch();
  const { tab } = useAppSelector(s => s.chat);
  const { lastConfig } = useAppSelector(s => s.socket);

  const shortcutModifier = desktopPlatform
    ? desktopPlatform === 'mac' ? '⌘' : 'Ctrl'
    : undefined;

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    const matchesModifier = desktopPlatform === 'mac'
      ? event.metaKey && !event.ctrlKey
      : event.ctrlKey && !event.metaKey;
    if (
      !matchesModifier ||
      event.defaultPrevented ||
      event.nativeEvent.isComposing ||
      event.altKey ||
      event.shiftKey ||
      event.repeat
    ) return;

    const nextTab = /^[1-4]$/.test(event.key)
      ? shortcutTabs[event.key as keyof typeof shortcutTabs]
      : undefined;
    if (nextTab) {
      event.preventDefault();
      dispatch(setTab(nextTab));
      return;
    }

    if (event.key.toLowerCase() !== 'k' || (tab !== 'group' && tab !== 'private')) return;
    const target = event.target;
    if (target instanceof HTMLElement && (
      target.isContentEditable || target.closest('input, textarea, select')
    )) return;

    event.preventDefault();
    dispatch(tab === 'group' ? clearGroupMessages() : clearPrivateMessages());
  }

  // Tab 页面映射
  const tabMap: Record<typeof tab, ReactNode> = {
    group: <ChatWindow pageType="public" />,
    private: <ChatWindow pageType="private" />,
    help: <HelpPage shortcutModifier={shortcutModifier} />,
    config: <ConfigEditor />
  };

  return (
    <div
      className="overflow-hidden flex flex-1 flex-col bg-[var(--sideBar-background)]"
      data-theme={theme}
      role="region"
      aria-label="机器人测试台"
      tabIndex={shortcutModifier ? 0 : undefined}
      onKeyDown={shortcutModifier ? handleKeyDown : undefined}
    >
      <Header />
      {tabMap[tab]}
      <Footer>
        {lastConfig ? (
          <div>
            [{lastConfig.host}][{lastConfig.port}]
          </div>
        ) : null}
      </Footer>
    </div>
  );
}
