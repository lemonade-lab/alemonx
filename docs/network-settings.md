# 工作台网络

## 全局策略

版本 2 只有一个全局模式，执行顺序为“读取模式 → 固定本次请求策略 → 发起请求”。不存在运行时资源覆盖优先级。

- 自动：按后端内置用途分组选路，不继承环境代理。未匹配的地址使用原始入口。
- 直连：原始入口，忽略旧资源镜像、代理及环境代理。
- 代理：HTTP、HTTPS 或 SOCKS5；认证可保留、更换、移除，连接失败不回退直连。
- 本地 IPC、回环服务、健康检查始终使用内部通道；不改系统代理、Docker 配置、部署文件或机器人自身的任意流量。

## 页面

顶部只有自动／直连／代理。选择代理直接展示页内表单，不弹窗；保存并应用成功后才改变实际运行模式。自动／直连切换失败保留原模式。

自动模式默认展示全部分组：GitHub API、发布下载、原始文件、源码归档、Gitee API、NPM、Node.js、Python、PyPI、CDN、官方下载。检测状态和延迟在各行展示，候选编辑与草稿测试都在全局对话框内完成。官方地址不可删除，不允许自定义匹配域名。

候选支持一个用途匹配的路径模板：`{url}`、`{path}`，Node.js 可用 `{nodepath}`，Python 可用 `{pythonpath}`。带认证的候选地址会被拒绝。API、源码、发布下载不共用一次探测结果。

直连提供连接测试；代理直接显示协议、主机、端口、可选认证、测试连接及保存并应用。字段错误和操作结果就地展示，失败保留输入；测试不保存，切离代理清除未保存草稿，保存成功清除密码输入。凭据和草稿不进入浏览器持久化。候选入口与首次迁移继续使用全局 Modal，支持 Esc、焦点约束与焦点回退。

## 选路与并发

每组最多并行检测 3 个入口，单入口 5 秒；不仅检查状态码，还验证 JSON 字段、索引、许可证内容或文件魔数。可用结果缓存 10 分钟，失败入口冷却 60 秒。同组并发请求共享一次检测。

初次选择最低延迟，官方与最快入口差距在 10% 内优先官方；仍健康的已选入口保持不变。全部失败返回明确错误。只有不带认证、Cookie、自定义敏感头、查询串和请求体的 GET/HEAD 才可进入加速与重试路径。写入、发布、AI 和私有请求不发给普通镜像，不自动重放。

HTTP 重定向保持策略快照，并保留调用方的重定向安全检查。新请求读取新模式，在途下载不切换出口。TCP 客户端在建立连接时固定策略，不重建已有连接。

## 配置与接口

磁盘模型：

```json
{
  "version": 2,
  "revision": 1,
  "mode": "auto",
  "automatic": {
    "groups": [
      { "id": "npm", "candidates": ["https://registry.npmmirror.com{path}"] }
    ]
  },
  "proxy": { "url": "" }
}
```

示例省略了其余必需分组；实际提交使用 GET 返回的完整配置。健康状态、延迟和检测时间不写入文件。

| 同一接口路径 `/api/v1/system/network` | 用途 |
| --- | --- |
| GET | 脱敏配置；必要时附带迁移确认信息 |
| PUT | 带 revision 保存；过期草稿返回 409 |
| GET `?view=status` | 当前修订的分组定义和运行状态 |
| POST `?action=detect&target=组ID` | 异步重新检测；省略 target 为全部 |
| POST `?action=preview&target=组ID` | 不保存的草稿检测，返回任务 token |
| GET `?task=token` | 草稿检测状态，任务 5 分钟过期 |
| POST `?target=github-api` | 当前直连／代理草稿连接测试 |

页面仅在检测进行中每 2 秒查询，结束、切换模式或关闭页面后停止。分组配置变更使用新缓存键；未变更分组复用结果，旧任务不能覆盖新候选状态。旧式 connection/routes/overrides 提交明确要求升级，不生成隐藏规则。

代理 URL 的用户名和密码不会回传。请求认证操作为 `credentials: preserve | replace | remove`；replace 搭配独立 username/password。服务端文件及迁移备份使用 0600，网络接口请求体不记入日志。Windows 还应使用受保护的运行账户目录及 ACL。

## 迁移

读取旧配置只生成建议，不写文件、不改变旧运行策略。已有镜像转为候选并去重；不同代理单独列出，包括同地址不同认证。用户通过一次对话框确认全局模式和代理，再提交 confirmMigration。

确认后先创建 `network.json.v1.bak`，再通过同目录临时文件、Sync、Rename 保存版本 2。已有备份内容不符时拒绝覆盖。迁移后旧规则不参与执行。

## 联网入口清单

| 入口 | 统一方式 |
| --- | --- |
| 更新、资源目录、在线文档、环境下载、Redis 运行时 | `systemnetwork.DefaultClient` |
| GitHub 登录、AI、Agent Webhook、机器人 NPM 元数据、媒体代理 | 动态全局 HTTP 客户端 |
| 系统插件宿主下载与能力请求 | 宿主网络客户端；能力上下文只返回当前版本配置 |
| DSH DeepSeek 请求与流式响应 | 受令牌保护的回环 bridge → 宿主网络客户端；不把 AI 凭据交给镜像 |
| 手机 APK 浏览器下载 | 固定目标的认证宿主下载接口，不交给外部浏览器直接联网 |
| 项目创建、依赖安装、受管工具安装、Agent 包命令 | 命令级环境／参数适配，退出时关闭临时网络通道 |
| Git HTTPS 克隆、拉取、推送 | 覆盖命令级代理和 URL 专属代理，保留认证；停用独立镜像和 insteadOf 选路 |
| npm／Yarn／pnpm／npx、pip、pyenv、NVM | 原始源或已检测候选；清理继承代理，不永久修改项目配置 |
| apt／dnf／yum、受控 sudo | 命令级代理参数与受限网络环境继承；不修改系统源文件 |
| 数据窗口远程 Redis、MySQL、PostgreSQL | 统一 TCP Dialer；数据库驱动负责自己的 TLS |
| 本地服务、健康检查、CLI 回环控制 | 明确禁用外部代理和外部重定向 |

项目锁文件或用户明确指定的第三方依赖 URL 仍属于依赖内容，不会永久改写。包管理器执行第三方安装脚本不能等同于 OS 级流量拦截；任意机器人进程、用户终端命令和外部浏览器站点不接管。

### 命令兼容边界

- 代理模式的 Git SSH 远端明确要求改用 HTTPS，不会悄悄绕过代理。
- WinGet 自动／直连使用单次 `--no-proxy`。当前安全通道要求认证，WinGet 代理模式明确阻止并提示使用受管下载／离线安装；不修改 WinGet 全局设置。命令参数与认证限制参考 [Microsoft 文档](https://learn.microsoft.com/en-us/windows/package-manager/winget/install) 和 [WinGet 代理设计](https://github.com/microsoft/winget-cli/blob/master/doc/specs/%23190%20-%20Proxy%20Support.md)。
- Chocolatey 使用命令级本地通道及临时认证，不传递外部代理密码，参数参考 [Chocolatey install](https://docs.chocolatey.org/en-us/choco/commands/install/)。
- 不支持这些命令参数的旧版外部工具应报错升级，不静默重试直连。

## 回归

覆盖模式互斥、迁移与备份、同地址不同认证、修订冲突、脱敏、不记录请求体、异步草稿、缓存与并发、失败冷却、私有请求保护、HTTP/HTTPS/SOCKS5、代理失败与超时、重定向快照、回环隔离、数据库 TCP 及 DSH bridge。

执行 Go 全量测试、网络及相关模块竞态测试、Windows 测试交叉编译、Linux/Windows 构建、前端 lint/build 和独立端口完整 Playwright。真实 Windows 安装器和需要外部账户的服务仍需目标平台集成验证；交叉编译不等于实机验收。
