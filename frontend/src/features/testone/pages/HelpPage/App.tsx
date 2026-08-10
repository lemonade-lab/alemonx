import React from 'react';

/* ────────── 纯 React 帮助页 ────────── */

const S: Record<string, React.CSSProperties> = {
  wrapper: {
    flex: 1,
    overflowY: 'auto',
    padding: '16px 20px 40px',
    color: 'var(--editor-foreground)',
    fontSize: 13,
    lineHeight: 1.7
  },
  h1: { fontSize: 20, fontWeight: 700, marginBottom: 4 },
  h2: {
    fontSize: 16,
    fontWeight: 600,
    marginTop: 24,
    marginBottom: 8,
    borderBottom: '1px solid var(--panel-border)',
    paddingBottom: 4
  },
  h3: { fontSize: 14, fontWeight: 600, marginTop: 16, marginBottom: 4 },
  p: { marginBottom: 8 },
  ul: { paddingLeft: 20, marginBottom: 8 },
  table: {
    width: '100%',
    borderCollapse: 'collapse',
    marginBottom: 12,
    fontSize: 12
  },
  th: {
    textAlign: 'left',
    padding: '4px 8px',
    borderBottom: '1px solid var(--panel-border)',
    fontWeight: 600,
    background: 'var(--activityBar-background)'
  },
  td: {
    padding: '4px 8px',
    borderBottom: '1px solid var(--panel-border)'
  },
  code: {
    background: 'var(--activityBar-background)',
    padding: '1px 5px',
    borderRadius: 3,
    fontSize: 12,
    fontFamily: 'monospace'
  },
  kbd: {
    display: 'inline-block',
    padding: '1px 6px',
    borderRadius: 3,
    border: '1px solid var(--panel-border)',
    background: 'var(--activityBar-background)',
    fontSize: 11,
    fontFamily: 'monospace',
    lineHeight: 1.6
  },
  sub: {
    color: 'var(--descriptionForeground)',
    fontSize: 12,
    marginBottom: 16,
    display: 'block'
  },
  tip: {
    background: 'var(--activityBar-background)',
    borderLeft: '3px solid var(--button-background)',
    padding: '8px 12px',
    marginBottom: 12,
    borderRadius: '0 4px 4px 0',
    fontSize: 12
  }
};

const Kbd = ({ children }: { children: React.ReactNode }) => (
  <span style={S.kbd}>{children}</span>
);

export default function HelpPage() {
  return (
    <div className="scrollbar" style={S.wrapper}>
      {/* ===== 标题 ===== */}
      <h1 style={S.h1}>📖 ALemonTestOne 使用说明</h1>
      <span style={S.sub}>
        阿柠檬框架沙盒测试平台 · VS Code 扩展 · alemonjs &gt;= 2.1
      </span>

      {/* ===== 简介 ===== */}
      <h2 style={S.h2}>这是什么？</h2>
      <p style={S.p}>
        ALemonTestOne 是一个运行在 VS Code 中的沙盒聊天测试平台，用来调试和测试
        AlemonJS 机器人的指令与消息处理逻辑。
      </p>
      <div style={S.tip}>
        <b>它能做什么：</b>
        <ul style={{ ...S.ul, marginBottom: 0 }}>
          <li>模拟群聊 / 私聊场景，向本地 AlemonJS 机器人发送消息</li>
          <li>接收并展示机器人回复（文本、图片、按钮、Markdown …）</li>
          <li>管理预设指令并批量定时执行（压力测试 / 自动化）</li>
          <li>切换多连接、多频道、多用户</li>
          <li>事件日志、载荷检查等调试工具</li>
        </ul>
      </div>

      {/* ===== 界面总览 ===== */}
      <h2 style={S.h2}>界面总览</h2>
      <p style={S.p}>底部导航栏可切换四个标签页：</p>
      <table style={S.table}>
        <thead>
          <tr>
            <th style={S.th}>标签</th>
            <th style={S.th}>说明</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td style={S.td}>群聊</td>
            <td style={S.td}>公共频道消息收发</td>
          </tr>
          <tr>
            <td style={S.td}>私聊</td>
            <td style={S.td}>一对一消息收发</td>
          </tr>
          <tr>
            <td style={S.td}>连接</td>
            <td style={S.td}>管理 WebSocket 后端连接</td>
          </tr>
          <tr>
            <td style={S.td}>帮助</td>
            <td style={S.td}>就是你正在看的这个页面</td>
          </tr>
          <tr>
            <td style={S.td}>配置</td>
            <td style={S.td}>可视化编辑 commands / users / channels 等配置</td>
          </tr>
        </tbody>
      </table>

      {/* ===== 连接管理 ===== */}
      <h2 style={S.h2}>连接管理</h2>
      <h3 style={S.h3}>添加连接</h3>
      <ol style={S.ul}>
        <li>进入「连接」标签页，点击「新增」</li>
        <li>
          填写名称（中文/字母/数字/连字符）、主机地址（IP
          或域名）、端口（1-65535）
        </li>
        <li>保存即可</li>
      </ol>
      <h3 style={S.h3}>连接与断开</h3>
      <p style={S.p}>
        点击连接卡片上的按钮建立 WebSocket
        连接，连接成功后状态灯变绿。再次点击断开。
      </p>
      <h3 style={S.h3}>自动重连</h3>
      <p style={S.p}>
        底部导航栏有一个小圆点开关。绿色 =
        自动重连已开启，断线后会自动尝试恢复。
      </p>

      {/* ===== 聊天窗口 ===== */}
      <h2 style={S.h2}>聊天窗口</h2>

      <h3 style={S.h3}>发送消息</h3>
      <p style={S.p}>
        在底部输入框输入文字，按 <Kbd>Enter</Kbd> 发送，
        <Kbd>Shift + Enter</Kbd> 换行。最多 2000 字符。
      </p>

      <h3 style={S.h3}>语音录制 🎤</h3>
      <ul style={S.ul}>
        <li>点击工具栏麦克风按钮开始录音</li>
        <li>录音中按钮变红闪烁并显示秒数</li>
        <li>再次点击停止，语音自动发送</li>
        <li>最长 60 秒，超时自动停止并发送</li>
      </ul>

      <h3 style={S.h3}>文件 / 图片上传</h3>
      <ul style={S.ul}>
        <li>
          <b>拖拽</b>：将文件拖入输入框区域（单文件 ≤ 8MB）
        </li>
        <li>
          <b>点击</b>：工具栏上传按钮选择文件
        </li>
        <li>
          <b>粘贴</b>：直接粘贴剪贴板中的图片
        </li>
        <li>可通过菜单开关图片压缩（最大 800×800，目标 220KB）</li>
      </ul>

      <h3 style={S.h3}>@提及 / #频道</h3>
      <ul style={S.ul}>
        <li>
          输入 <span style={S.code}>@</span> 弹出用户列表，选中即插入
        </li>
        <li>
          输入 <span style={S.code}>#</span> 弹出频道列表
        </li>
        <li>
          <span style={S.code}>@everyone</span> 全体提及
        </li>
      </ul>

      <h3 style={S.h3}>输入历史</h3>
      <p style={S.p}>
        按 <Kbd>↑</Kbd> / <Kbd>↓</Kbd> 方向键回溯 / 前进已发送过的消息，保留最近
        50 条，跨会话持久化。
      </p>

      <h3 style={S.h3}>消息搜索</h3>
      <p style={S.p}>
        点击聊天标题栏 🔍 图标打开搜索栏，输入关键词实时过滤消息。
      </p>

      {/* ===== 消息操作 ===== */}
      <h2 style={S.h2}>消息操作</h2>
      <p style={S.p}>右键点击消息气泡，弹出操作菜单：</p>
      <table style={S.table}>
        <thead>
          <tr>
            <th style={S.th}>操作</th>
            <th style={S.th}>说明</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td style={S.td}>编辑</td>
            <td style={S.td}>修改已发送消息内容，编辑后显示「已编辑」标记</td>
          </tr>
          <tr>
            <td style={S.td}>回应</td>
            <td style={S.td}>
              添加 emoji（👍 ❤️ 😂 😮 😢 😠），可查看回应人列表
            </td>
          </tr>
          <tr>
            <td style={S.td}>删除</td>
            <td style={S.td}>移除这条消息</td>
          </tr>
          <tr>
            <td style={S.td}>检查</td>
            <td style={S.td}>打开载荷检查器查看原始数据结构</td>
          </tr>
        </tbody>
      </table>
      <p style={S.p}>还可以进入「批量选择」模式，勾选多条消息后一键删除。</p>

      {/* ===== 指令系统 ===== */}
      <h2 style={S.h2}>指令系统</h2>

      <h3 style={S.h3}>指令列表</h3>
      <p style={S.p}>
        底部工具栏的「指令」按钮打开指令面板。显示来自{' '}
        <span style={S.code}>testone/commands.json</span>{' '}
        的所有预设指令。支持搜索过滤、分页浏览，单击即发送。
      </p>

      <h3 style={S.h3}>定时任务 ⏱️</h3>
      <p style={S.p}>用于自动批量执行指令（压力测试 / 自动化流程）。</p>
      <table style={S.table}>
        <thead>
          <tr>
            <th style={S.th}>配置项</th>
            <th style={S.th}>说明</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td style={S.td}>频率</td>
            <td style={S.td}>每条指令间隔 1~12 秒</td>
          </tr>
          <tr>
            <td style={S.td}>循环</td>
            <td style={S.td}>关 = 单轮执行完停止；开 = 反复循环</td>
          </tr>
          <tr>
            <td style={S.td}>区间模式</td>
            <td style={S.td}>选择起始 ~ 结束指令编号，执行范围内所有指令</td>
          </tr>
          <tr>
            <td style={S.td}>手选模式</td>
            <td style={S.td}>通过复选框逐条勾选，支持搜索、全选/清除</td>
          </tr>
        </tbody>
      </table>
      <p style={S.p}>
        任务运行后底部出现「任务管理器」，可查看执行次数、删除单个任务或停止全部。
      </p>

      {/* ===== 开发者工具 ===== */}
      <h2 style={S.h2}>开发者工具</h2>

      <h3 style={S.h3}>事件日志</h3>
      <p style={S.p}>
        输入框下方可折叠的面板，记录所有 WebSocket
        收发事件。每条日志显示事件类型、时间戳、延迟（绿 &lt; 100ms · 黄 &lt;
        500ms · 红 ≥ 500ms），可按类型过滤。
      </p>

      <h3 style={S.h3}>载荷检查器</h3>
      <p style={S.p}>
        右键消息 → 检查。查看 DataEnums 分段视图、原始 JSON、元数据（用户/消息
        ID/时间戳/回应），支持一键复制。
      </p>

      {/* ===== 快捷键 ===== */}
      <h2 style={S.h2}>键盘快捷键</h2>
      <table style={S.table}>
        <thead>
          <tr>
            <th style={S.th}>快捷键</th>
            <th style={S.th}>操作</th>
          </tr>
        </thead>
        <tbody>
          {(
            [
              ['Cmd/Ctrl + 1', '切换群聊'],
              ['Cmd/Ctrl + 2', '切换私聊'],
              ['Cmd/Ctrl + 3', '切换连接'],
              ['Cmd/Ctrl + K', '清空当前消息'],
              ['Enter', '发送消息'],
              ['Shift + Enter', '换行'],
              ['↑ / ↓', '输入历史回溯 / 前进'],
              ['Escape', '关闭弹窗'],
              ['@', '触发用户提及补全'],
              ['#', '触发频道提及补全']
            ] as const
          ).map(([key, desc]) => (
            <tr key={key}>
              <td style={S.td}>
                <Kbd>{key}</Kbd>
              </td>
              <td style={S.td}>{desc}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* ===== 配置 ===== */}
      <h2 style={S.h2}>自定义配置</h2>
      <div style={S.tip}>
        <b>💡 推荐：</b>点击底部导航栏的 ⚙️
        按钮打开「配置编辑器」，可视化编辑后导出 JSON 文件，放入{' '}
        <span style={S.code}>testone/</span> 目录即可。
      </div>
      <p style={S.p}>
        在项目根目录新建 <span style={S.code}>testone/</span> 文件夹，放入以下
        JSON 文件即可自定义数据：
      </p>
      <table style={S.table}>
        <thead>
          <tr>
            <th style={S.th}>文件</th>
            <th style={S.th}>说明</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td style={S.td}>
              <span style={S.code}>commands.json</span>
            </td>
            <td style={S.td}>
              预设指令列表（title / description / text / autoEnter / data）
            </td>
          </tr>
          <tr>
            <td style={S.td}>
              <span style={S.code}>user.json</span>
            </td>
            <td style={S.td}>当前用户配置</td>
          </tr>
          <tr>
            <td style={S.td}>
              <span style={S.code}>bot.json</span>
            </td>
            <td style={S.td}>机器人用户配置</td>
          </tr>
          <tr>
            <td style={S.td}>
              <span style={S.code}>users.json</span>
            </td>
            <td style={S.td}>用户列表（@提及用）</td>
          </tr>
          <tr>
            <td style={S.td}>
              <span style={S.code}>channels.json</span>
            </td>
            <td style={S.td}>频道列表（GuildId / ChannelId / ChannelName）</td>
          </tr>
        </tbody>
      </table>

      {/* ===== FAQ ===== */}
      <h2 style={S.h2}>常见问题</h2>
      <div style={S.tip}>
        <b>连接不上？</b>
        <br />
        确保 AlemonJS 后端已启动，检查主机和端口是否正确。事件日志可看 WebSocket
        状态。
      </div>
      <div style={S.tip}>
        <b>指令列表为空？</b>
        <br />
        检查项目中是否有 <span style={S.code}>testone/commands.json</span>{' '}
        且格式正确。
      </div>
      <div style={S.tip}>
        <b>消息发不出去？</b>
        <br />
        检查底部状态灯是否为绿色（已连接）。未连接时消息无法送达后端。
      </div>
      <div style={S.tip}>
        <b>图片显示不出来？</b>
        <br />
        可能图片 URL 无法访问，或图片压缩开关影响了解析。尝试关闭压缩。
      </div>

      <div
        style={{
          textAlign: 'center',
          marginTop: 32,
          fontSize: 11,
          color: 'var(--descriptionForeground)'
        }}
      >
        ALemonTestOne · Made with ❤️
      </div>
    </div>
  );
}
