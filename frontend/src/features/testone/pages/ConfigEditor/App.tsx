import { useState, useMemo, useCallback } from 'react';
import { useAppSelector } from '@testone/store';
import type { Command, User, Channel } from '@testone/typing';
import CopyOutlined from '@ant-design/icons/CopyOutlined';
import DownloadOutlined from '@ant-design/icons/DownloadOutlined';
import EditOutlined from '@ant-design/icons/EditOutlined';
import DeleteOutlined from '@ant-design/icons/DeleteOutlined';
import PlusOutlined from '@ant-design/icons/PlusOutlined';
import CheckOutlined from '@ant-design/icons/CheckOutlined';
import CloseOutlined from '@ant-design/icons/CloseOutlined';
import SearchOutlined from '@ant-design/icons/SearchOutlined';

/* ─── 样式常量 ─── */
const cls = {
  section:
    'bg-[var(--editor-background)] rounded-lg border border-[var(--panel-border)] overflow-hidden',
  sectionHead:
    'flex items-center justify-between px-4 py-2 border-b border-[var(--panel-border)] bg-[var(--activityBar-background)]',
  sectionTitle: 'text-sm font-semibold text-[var(--editor-foreground)]',
  row: 'flex items-center gap-2 px-4 py-2 text-xs border-b border-[var(--panel-border)] last:border-b-0 hover:bg-[var(--activityBar-background)] transition-colors',
  input:
    'flex-1 px-2 py-1 rounded text-xs bg-[var(--input-background)] text-[var(--input-foreground)] border border-[var(--input-border)] outline-none focus:border-[var(--button-background)]',
  inputSmall:
    'w-full px-2 py-1 rounded text-xs bg-[var(--input-background)] text-[var(--input-foreground)] border border-[var(--input-border)] outline-none focus:border-[var(--button-background)]',
  btn: 'px-2 py-1 rounded text-[11px] bg-[var(--button-background)] text-[var(--button-foreground)] hover:opacity-90 cursor-pointer',
  btnGhost:
    'px-1.5 py-0.5 rounded text-[11px] hover:bg-[var(--activityBar-background)] cursor-pointer text-[var(--editor-foreground)]',
  badge:
    'px-1.5 py-0.5 rounded text-[10px] bg-[var(--activityBar-background)] text-[var(--descriptionForeground)]',
  empty:
    'text-xs text-center py-6 text-[var(--descriptionForeground)] opacity-70',
  search:
    'px-2 py-1 rounded text-xs bg-[var(--input-background)] text-[var(--input-foreground)] border border-[var(--input-border)] outline-none w-40'
};

/* ─── 工具 ─── */
function copyJSON(data: unknown, label: string) {
  const json = JSON.stringify(data, null, 2);
  navigator.clipboard.writeText(json).then(
    () => alert(`${label} 已复制到剪贴板`),
    () => alert('复制失败')
  );
}

function downloadJSON(data: unknown, filename: string) {
  const json = JSON.stringify(data, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/* ─── 子区块：指令编辑器 ─── */
function CommandSection({
  commands,
  onChange
}: {
  commands: Command[];
  onChange: (c: Command[]) => void;
}) {
  const [search, setSearch] = useState('');
  const [editIdx, setEditIdx] = useState<number | null>(null);
  const [draft, setDraft] = useState<Partial<Command>>({});

  const filtered = useMemo(() => {
    if (!search) return commands.map((c, i) => ({ c, i }));
    const q = search.toLowerCase();
    return commands
      .map((c, i) => ({ c, i }))
      .filter(
        ({ c }) =>
          c.title?.toLowerCase().includes(q) ||
          c.text?.toLowerCase().includes(q) ||
          c.description?.toLowerCase().includes(q)
      );
  }, [commands, search]);

  const startEdit = (idx: number) => {
    setEditIdx(idx);
    setDraft({ ...commands[idx] });
  };
  const cancelEdit = () => {
    setEditIdx(null);
    setDraft({});
  };
  const saveEdit = () => {
    if (editIdx === null) return;
    const next = [...commands];
    next[editIdx] = { ...next[editIdx], ...draft } as Command;
    onChange(next);
    cancelEdit();
  };
  const addNew = () => {
    onChange([
      ...commands,
      { title: '新指令', description: '', text: '/new', autoEnter: false }
    ]);
    setEditIdx(commands.length);
    setDraft({
      title: '新指令',
      description: '',
      text: '/new',
      autoEnter: false
    });
  };
  const remove = (idx: number) => {
    onChange(commands.filter((_, i) => i !== idx));
    if (editIdx === idx) cancelEdit();
  };

  return (
    <div className={cls.section}>
      <div className={cls.sectionHead}>
        <span className={cls.sectionTitle}>
          📋 指令列表（commands.json）
          <span className={cls.badge + ' ml-2'}>{commands.length}</span>
        </span>
        <div className="flex items-center gap-2">
          <div className="relative">
            <SearchOutlined className="absolute left-2 top-1/2 -translate-y-1/2 text-[var(--descriptionForeground)] text-[10px]" />
            <input
              className={cls.search + ' pl-6'}
              placeholder="搜索..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          <button className={cls.btn} onClick={addNew}>
            <PlusOutlined /> 新增
          </button>
          <button
            className={cls.btnGhost}
            onClick={() => copyJSON(commands, 'commands.json')}
          >
            <CopyOutlined />
          </button>
          <button
            className={cls.btnGhost}
            onClick={() => downloadJSON(commands, 'commands.json')}
          >
            <DownloadOutlined />
          </button>
        </div>
      </div>

      <div className="max-h-72 overflow-y-auto scrollbar">
        {filtered.length === 0 && (
          <div className={cls.empty}>
            {search ? '未找到匹配指令' : '暂无指令'}
          </div>
        )}
        {filtered.map(({ c, i }) =>
          editIdx === i ? (
            <div key={i} className={cls.row + ' flex-col !items-stretch gap-2'}>
              <div className="grid grid-cols-2 gap-2">
                <input
                  className={cls.inputSmall}
                  placeholder="标题"
                  value={draft.title ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, title: e.target.value }))
                  }
                />
                <input
                  className={cls.inputSmall}
                  placeholder="指令文本"
                  value={draft.text ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, text: e.target.value }))
                  }
                />
              </div>
              <input
                className={cls.inputSmall}
                placeholder="描述"
                value={draft.description ?? ''}
                onChange={e =>
                  setDraft(d => ({ ...d, description: e.target.value }))
                }
              />
              <div className="flex items-center gap-3">
                <label className="flex items-center gap-1 text-[11px] text-[var(--descriptionForeground)] cursor-pointer">
                  <input
                    type="checkbox"
                    checked={draft.autoEnter ?? false}
                    onChange={e =>
                      setDraft(d => ({ ...d, autoEnter: e.target.checked }))
                    }
                  />
                  自动发送
                </label>
                <div className="flex-1" />
                <button className={cls.btn} onClick={saveEdit}>
                  <CheckOutlined /> 保存
                </button>
                <button className={cls.btnGhost} onClick={cancelEdit}>
                  <CloseOutlined /> 取消
                </button>
              </div>
            </div>
          ) : (
            <div key={i} className={cls.row}>
              <span className="text-[var(--descriptionForeground)] w-6 text-right">
                #{i + 1}
              </span>
              <span className="font-medium text-[var(--editor-foreground)] w-24 truncate">
                {c.title || '—'}
              </span>
              <span className="text-[var(--descriptionForeground)] flex-1 truncate">
                {c.text}
              </span>
              {c.autoEnter && <span className={cls.badge}>自发</span>}
              <button className={cls.btnGhost} onClick={() => startEdit(i)}>
                <EditOutlined />
              </button>
              <button
                className={cls.btnGhost + ' text-red-400'}
                onClick={() => remove(i)}
              >
                <DeleteOutlined />
              </button>
            </div>
          )
        )}
      </div>
    </div>
  );
}

/* ─── 子区块：用户编辑器 ─── */
function UserSection({
  title,
  filename,
  users,
  onChange,
  isSingle
}: {
  title: string;
  filename: string;
  users: User[];
  onChange: (u: User[]) => void;
  isSingle?: boolean;
}) {
  const [editIdx, setEditIdx] = useState<number | null>(null);
  const [draft, setDraft] = useState<Partial<User>>({});

  const startEdit = (idx: number) => {
    setEditIdx(idx);
    setDraft({ ...users[idx] });
  };
  const cancelEdit = () => {
    setEditIdx(null);
    setDraft({});
  };
  const saveEdit = () => {
    if (editIdx === null) return;
    const next = [...users];
    next[editIdx] = { ...next[editIdx], ...draft } as User;
    onChange(next);
    cancelEdit();
  };
  const addNew = () => {
    const newUser: User = {
      UserId: String(Date.now()),
      UserName: '新用户',
      UserAvatar: '',
      IsMaster: false,
      IsBot: false
    };
    onChange([...users, newUser]);
    setEditIdx(users.length);
    setDraft(newUser);
  };
  const remove = (idx: number) => {
    onChange(users.filter((_, i) => i !== idx));
    if (editIdx === idx) cancelEdit();
  };

  const exportData = isSingle && users.length > 0 ? users[0] : users;

  return (
    <div className={cls.section}>
      <div className={cls.sectionHead}>
        <span className={cls.sectionTitle}>
          {title}
          <span className={cls.badge + ' ml-2'}>{users.length}</span>
        </span>
        <div className="flex items-center gap-2">
          {!isSingle && (
            <button className={cls.btn} onClick={addNew}>
              <PlusOutlined /> 新增
            </button>
          )}
          <button
            className={cls.btnGhost}
            onClick={() => copyJSON(exportData, filename)}
          >
            <CopyOutlined />
          </button>
          <button
            className={cls.btnGhost}
            onClick={() => downloadJSON(exportData, filename)}
          >
            <DownloadOutlined />
          </button>
        </div>
      </div>

      <div className="max-h-52 overflow-y-auto scrollbar">
        {users.length === 0 && <div className={cls.empty}>暂无数据</div>}
        {users.map((u, i) =>
          editIdx === i ? (
            <div key={i} className={cls.row + ' flex-col !items-stretch gap-2'}>
              <div className="grid grid-cols-2 gap-2">
                <input
                  className={cls.inputSmall}
                  placeholder="UserId"
                  value={draft.UserId ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, UserId: e.target.value }))
                  }
                />
                <input
                  className={cls.inputSmall}
                  placeholder="UserName"
                  value={draft.UserName ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, UserName: e.target.value }))
                  }
                />
              </div>
              <input
                className={cls.inputSmall}
                placeholder="UserAvatar (URL)"
                value={draft.UserAvatar ?? ''}
                onChange={e =>
                  setDraft(d => ({ ...d, UserAvatar: e.target.value }))
                }
              />
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-1 text-[11px] text-[var(--descriptionForeground)] cursor-pointer">
                  <input
                    type="checkbox"
                    checked={draft.IsMaster ?? false}
                    onChange={e =>
                      setDraft(d => ({ ...d, IsMaster: e.target.checked }))
                    }
                  />
                  管理员
                </label>
                <label className="flex items-center gap-1 text-[11px] text-[var(--descriptionForeground)] cursor-pointer">
                  <input
                    type="checkbox"
                    checked={draft.IsBot ?? false}
                    onChange={e =>
                      setDraft(d => ({ ...d, IsBot: e.target.checked }))
                    }
                  />
                  机器人
                </label>
                <div className="flex-1" />
                <button className={cls.btn} onClick={saveEdit}>
                  <CheckOutlined /> 保存
                </button>
                <button className={cls.btnGhost} onClick={cancelEdit}>
                  <CloseOutlined /> 取消
                </button>
              </div>
            </div>
          ) : (
            <div key={i} className={cls.row}>
              {u.UserAvatar ? (
                <img
                  src={u.UserAvatar}
                  className="w-5 h-5 rounded-full object-cover"
                  alt=""
                />
              ) : (
                <div className="w-5 h-5 rounded-full bg-[var(--activityBar-background)] flex items-center justify-center text-[10px]">
                  {(u.UserName || '?')[0]}
                </div>
              )}
              <span className="font-medium text-[var(--editor-foreground)] w-20 truncate">
                {u.UserName}
              </span>
              <span className="text-[var(--descriptionForeground)] flex-1 truncate text-[11px]">
                {u.UserId}
              </span>
              {u.IsMaster && <span className={cls.badge}>管理员</span>}
              {u.IsBot && <span className={cls.badge}>Bot</span>}
              <button className={cls.btnGhost} onClick={() => startEdit(i)}>
                <EditOutlined />
              </button>
              {!isSingle && (
                <button
                  className={cls.btnGhost + ' text-red-400'}
                  onClick={() => remove(i)}
                >
                  <DeleteOutlined />
                </button>
              )}
            </div>
          )
        )}
      </div>
    </div>
  );
}

/* ─── 子区块：频道编辑器 ─── */
function ChannelSection({
  channels,
  onChange
}: {
  channels: Channel[];
  onChange: (c: Channel[]) => void;
}) {
  const [editIdx, setEditIdx] = useState<number | null>(null);
  const [draft, setDraft] = useState<Partial<Channel>>({});

  const startEdit = (idx: number) => {
    setEditIdx(idx);
    setDraft({ ...channels[idx] });
  };
  const cancelEdit = () => {
    setEditIdx(null);
    setDraft({});
  };
  const saveEdit = () => {
    if (editIdx === null) return;
    const next = [...channels];
    next[editIdx] = { ...next[editIdx], ...draft } as Channel;
    onChange(next);
    cancelEdit();
  };
  const addNew = () => {
    const c: Channel = {
      GuildId: 'guild-' + Date.now(),
      ChannelId: 'ch-' + Date.now(),
      ChannelAvatar: '',
      ChannelName: '新频道'
    };
    onChange([...channels, c]);
    setEditIdx(channels.length);
    setDraft(c);
  };
  const remove = (idx: number) => {
    onChange(channels.filter((_, i) => i !== idx));
    if (editIdx === idx) cancelEdit();
  };

  return (
    <div className={cls.section}>
      <div className={cls.sectionHead}>
        <span className={cls.sectionTitle}>
          📺 频道列表（channels.json）
          <span className={cls.badge + ' ml-2'}>{channels.length}</span>
        </span>
        <div className="flex items-center gap-2">
          <button className={cls.btn} onClick={addNew}>
            <PlusOutlined /> 新增
          </button>
          <button
            className={cls.btnGhost}
            onClick={() => copyJSON(channels, 'channels.json')}
          >
            <CopyOutlined />
          </button>
          <button
            className={cls.btnGhost}
            onClick={() => downloadJSON(channels, 'channels.json')}
          >
            <DownloadOutlined />
          </button>
        </div>
      </div>

      <div className="max-h-52 overflow-y-auto scrollbar">
        {channels.length === 0 && <div className={cls.empty}>暂无频道</div>}
        {channels.map((ch, i) =>
          editIdx === i ? (
            <div key={i} className={cls.row + ' flex-col !items-stretch gap-2'}>
              <div className="grid grid-cols-2 gap-2">
                <input
                  className={cls.inputSmall}
                  placeholder="ChannelName"
                  value={draft.ChannelName ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, ChannelName: e.target.value }))
                  }
                />
                <input
                  className={cls.inputSmall}
                  placeholder="ChannelId"
                  value={draft.ChannelId ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, ChannelId: e.target.value }))
                  }
                />
              </div>
              <div className="grid grid-cols-2 gap-2">
                <input
                  className={cls.inputSmall}
                  placeholder="GuildId"
                  value={draft.GuildId ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, GuildId: e.target.value }))
                  }
                />
                <input
                  className={cls.inputSmall}
                  placeholder="ChannelAvatar (URL)"
                  value={draft.ChannelAvatar ?? ''}
                  onChange={e =>
                    setDraft(d => ({ ...d, ChannelAvatar: e.target.value }))
                  }
                />
              </div>
              <div className="flex justify-end gap-2">
                <button className={cls.btn} onClick={saveEdit}>
                  <CheckOutlined /> 保存
                </button>
                <button className={cls.btnGhost} onClick={cancelEdit}>
                  <CloseOutlined /> 取消
                </button>
              </div>
            </div>
          ) : (
            <div key={i} className={cls.row}>
              <span className="font-medium text-[var(--editor-foreground)] w-24 truncate">
                {ch.ChannelName}
              </span>
              <span className="text-[var(--descriptionForeground)] flex-1 truncate text-[11px]">
                {ch.ChannelId}
              </span>
              <span className="text-[var(--descriptionForeground)] truncate text-[11px]">
                {ch.GuildId}
              </span>
              <button className={cls.btnGhost} onClick={() => startEdit(i)}>
                <EditOutlined />
              </button>
              <button
                className={cls.btnGhost + ' text-red-400'}
                onClick={() => remove(i)}
              >
                <DeleteOutlined />
              </button>
            </div>
          )
        )}
      </div>
    </div>
  );
}

/* ═════════════════ 主页面 ═════════════════ */

export default function ConfigEditor() {
  /* 从 Redux 获取当前数据作为初始值 */
  const storeCommands = useAppSelector(s => s.commands.commands);
  const storeUsers = useAppSelector(s => s.users.users);
  const storeBot = useAppSelector(s => s.users.bot);
  const storeCurrentUser = useAppSelector(s => s.users.current);
  const storeChannels = useAppSelector(s => s.channels.channels);

  /* 本地编辑状态（不直接修改 store，由用户导出后放入 testone/ 目录） */
  const [commands, setCommands] = useState<Command[]>(storeCommands);
  const [users, setUsers] = useState<User[]>(storeUsers);
  const [bot, setBot] = useState<User[]>(storeBot ? [storeBot] : []);
  const [currentUser, setCurrentUser] = useState<User[]>(
    storeCurrentUser ? [storeCurrentUser] : []
  );
  const [channels, setChannels] = useState<Channel[]>(storeChannels);

  /* 重置为 store 数据 */
  const resetAll = useCallback(() => {
    setCommands(storeCommands);
    setUsers(storeUsers);
    setBot(storeBot ? [storeBot] : []);
    setCurrentUser(storeCurrentUser ? [storeCurrentUser] : []);
    setChannels(storeChannels);
  }, [storeCommands, storeUsers, storeBot, storeCurrentUser, storeChannels]);

  /* 一键导出全部 */
  const exportAll = () => {
    const bundle = {
      'commands.json': commands,
      'user.json': currentUser[0] || null,
      'bot.json': bot[0] || null,
      'users.json': users,
      'channels.json': channels
    };
    downloadJSON(bundle, 'testone-config.json');
  };

  return (
    <div
      className="flex-1 overflow-y-auto scrollbar px-3 py-4 space-y-4"
      style={{ color: 'var(--editor-foreground)' }}
    >
      {/* 顶部标题栏 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-base font-bold">⚙️ 配置编辑器</h1>
          <p className="text-[11px] text-[var(--descriptionForeground)] mt-0.5">
            编辑后通过 复制/下载 将 JSON 放入项目的{' '}
            <code className="px-1 py-0.5 rounded bg-[var(--activityBar-background)] text-[11px]">
              testone/
            </code>{' '}
            目录，服务端会自动加载
          </p>
        </div>
        <div className="flex gap-2">
          <button className={cls.btnGhost} onClick={resetAll}>
            重置
          </button>
          <button className={cls.btn} onClick={exportAll}>
            <DownloadOutlined /> 导出全部
          </button>
        </div>
      </div>

      {/* 指令 */}
      <CommandSection commands={commands} onChange={setCommands} />

      {/* 当前用户 */}
      <UserSection
        title="👤 当前用户（user.json）"
        filename="user.json"
        users={currentUser}
        onChange={setCurrentUser}
        isSingle
      />

      {/* 机器人 */}
      <UserSection
        title="🤖 机器人（bot.json）"
        filename="bot.json"
        users={bot}
        onChange={setBot}
        isSingle
      />

      {/* 用户列表 */}
      <UserSection
        title="👥 用户列表（users.json）"
        filename="users.json"
        users={users}
        onChange={setUsers}
      />

      {/* 频道 */}
      <ChannelSection channels={channels} onChange={setChannels} />

      <div className="pb-4" />
    </div>
  );
}
