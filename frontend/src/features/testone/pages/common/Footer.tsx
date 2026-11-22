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
    <footer className="flex min-w-0 items-center justify-between gap-2 px-2 py-1">
      <div className="flex min-w-0 items-center gap-2">
        <span
          className="inline-flex min-w-0 max-w-24 shrink-0 items-center gap-1 text-[11px] text-[var(--descriptionForeground)] sm:max-w-64"
          title={statusText}
        >
          <i className={`inline-block size-2 shrink-0 rounded-full ${statusTone}`} />
          <span className="truncate">{statusText}</span>
        </span>
        {children && <div className="sr-only sm:not-sr-only sm:min-w-0 sm:truncate">{children}</div>}
        <div className="testone-mobile-user flex min-w-0 items-center gap-1 sm:gap-2">
          <Select
            className="max-w-full truncate px-0 py-0"
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
          {user && <span className="min-w-0 truncate max-[360px]:sr-only" title={user.UserId}>{user.UserId}</span>}
        </div>
      </div>
      <NavActions className="shrink-0" />
    </footer>
  );
};

export default Footer;
