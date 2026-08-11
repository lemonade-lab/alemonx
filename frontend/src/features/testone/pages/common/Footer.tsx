import { useAppDispatch, useAppSelector } from '@testone/store';
import { NavActions } from './NavActions';
import Select from '@testone/ui/Select';
import { setCurrentUser } from '@testone/store/slices/userSlice';

const Footer = ({ children }: { children: React.ReactNode }) => {
  const { current: user, users } = useAppSelector(s => s.users);
  const { connected, isConnecting, isRestarting, error, reconnectAttempts } =
    useAppSelector(s => s.socket);
  const dispatch = useAppDispatch();
  const connecting = isConnecting || isRestarting;
  const statusText = connected
    ? '已连接'
    : connecting
      ? `连接中…${reconnectAttempts > 0 ? `（重试 ${reconnectAttempts} 次）` : ''}`
      : error
        ? `连接失败：${error}`
        : '未连接';
  const statusTone = connecting
    ? 'bg-amber-400 animate-pulse'
    : connected
      ? 'bg-emerald-500'
      : error
        ? 'bg-red-400'
        : 'bg-slate-400';
  return (
    <footer className="flex justify-between  px-2 py-1">
      <div className="flex gap-2 items-center justify-center">
        <span
          className="inline-flex items-center gap-1 text-[11px] text-[var(--descriptionForeground)]"
          title={statusText}
        >
          <i className={`inline-block size-2 rounded-full ${statusTone}`} />
          {statusText}
        </span>
        {children}
        <div className="testone-mobile-user flex gap-2">
          <Select
            className="py-0 px-0"
            value={user?.UserId || ''}
            options={users.map(u => ({
              ...u,
              value: u.UserId,
              label: u.UserName
            }))}
            onSelect={value => {
              const user = users.find(u => u.UserId === value);
              if (!user) {
                return;
              }
              dispatch(setCurrentUser(user));
            }}
          />
          {user?.UserId}
        </div>
      </div>
      <NavActions />
    </footer>
  );
};

export default Footer;
