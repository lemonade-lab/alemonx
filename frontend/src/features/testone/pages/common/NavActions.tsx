import { useCallback } from 'react';
import { useAppDispatch, useAppSelector } from '@testone/store';
import { setTab } from '@testone/store/slices/chatSlice';

import TagOutlined from '@ant-design/icons/TagOutlined';
import TagsOutlined from '@ant-design/icons/TagsOutlined';
import QuestionCircleOutlined from '@ant-design/icons/QuestionCircleOutlined';
import SettingOutlined from '@ant-design/icons/SettingOutlined';
import { Tooltip } from '@testone/ui/Tooltip';

interface NavActionsProps {
  autoTooltipPlacement?:
    | 'top'
    | 'bottom'
    | 'left'
    | 'right'
    | 'topright'
    | 'topleft'
    | 'bottomright'
    | 'bottomleft';
  className?: string;
}

export const NavActions = ({
  autoTooltipPlacement = 'top',
  className
}: NavActionsProps) => {
  const dispatch = useAppDispatch();
  const { tab } = useAppSelector(s => s.chat);

  const onGotab = useCallback(
    (t: typeof tab) => {
      // 页面切换不依赖连接状态：未连接时也能在群聊/私聊/帮助/设置之间浏览，
      // 发送消息时才需要连接（safeSend 会静默忽略）。
      dispatch(setTab(t));
    },
    [dispatch, tab]
  );

  const vt = (txt: string) => (
    <span
      style={{
        writingMode: 'vertical-rl',
        textOrientation: 'upright',
        lineHeight: 1.2,
        display: 'inline-block'
      }}
    >
      {txt}
    </span>
  );

  return (
    <div className={'flex gap-3 justify-end items-center ' + (className || '')}>
      <Tooltip placement={autoTooltipPlacement} content={vt('群聊')} portal>
        <div className="cursor-pointer" onClick={() => onGotab('group')}>
          <TagsOutlined />
        </div>
      </Tooltip>
      <Tooltip placement={autoTooltipPlacement} content={vt('私聊')} portal>
        <div className="cursor-pointer" onClick={() => onGotab('private')}>
          <TagOutlined />
        </div>
      </Tooltip>
      <Tooltip placement={autoTooltipPlacement} content={vt('配置')} portal>
        <div
          className="cursor-pointer"
          onClick={() => dispatch(setTab('config'))}
        >
          <SettingOutlined />
        </div>
      </Tooltip>
      <Tooltip placement={autoTooltipPlacement} content={vt('帮助')} portal>
        <div
          className="cursor-pointer"
          onClick={() => dispatch(setTab('help'))}
        >
          <QuestionCircleOutlined />
        </div>
      </Tooltip>
    </div>
  );
};

export default NavActions;
