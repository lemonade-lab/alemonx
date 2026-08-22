# ALemonX 系统插件开发与接入

> 本页还有 [English version](en/plugin-development.md)。

系统插件用于为 ALemonX（`alx`）增加**全局的本机管理能力**，例如网络检查、系统服务、防火墙规则或 Docker 管理。它与机器人项目无关；不要用它实现某个机器人的命令、配置页或机器人应用页——那些能力应作为机器人插件提供。

本文档是系统插件的完整接入指南，覆盖：插件结构、`alx.json` 清单、**通讯协议**（面板页 ↔ 宿主 ↔ 执行器）、**全部可用接口**、宿主能力与 `ALXHost` SDK、本地服务代理、上传、特权操作与审计、源码开发会话、发布与安装，以及**工作台中的系统功能与系统组件**。

## 目录

1. [定位与边界](#定位与边界)
2. [插件结构](#插件结构)
3. [快速开始](#快速开始)
4. [alx.json 清单参考](#alxjson-清单参考)
5. [通讯协议](#通讯协议)
6. [HTTP 接口总览](#http-接口总览)
7. [宿主能力与 ALXHost SDK](#宿主能力与-alxhost-sdk)
8. [本地服务（services）](#本地服务services)
9. [上传（uploads）](#上传uploads)
10. [特权操作与审计](#特权操作与审计)
11. [源码开发会话](#源码开发会话)
12. [系统功能与系统组件](#系统功能与系统组件)
13. [发布、安装与版本管理](#发布安装与版本管理)
14. [安全边界与开发规范](#安全边界与开发规范)
15. [参考实现](#参考实现)

## 定位与边界

- **系统插件**：为 `alx` 本身提供系统级能力，界面是插件自己的 Web 前端，通过宿主转发的动作接口执行本机操作。示例：`alemonx-network`（网络/端口转发/防火墙）、`alemonx-docker`（Docker Compose 管理）。
- **机器人插件**：为某个 AlemonJS 机器人提供命令、配置页、机器人应用页等，通过机器人平台的生命周期管理。两者完全分离，不要混用。
- **界面即插件**：清单不再声明 `pages`/`actions`/`fields`——那些「多余配置」已移除。插件的界面就是它的 `web.root` 静态前端，由 ALemonX 同源托管；复杂交互在插件页面里自己实现。
- **发现阶段永不执行插件代码**：列出、渲染、启用/停用都不运行执行器；只有调用动作时才启动独立进程。

### 术语对照

仓库中有三种容易混淆的「嵌入页面」机制，文档与代码按以下名称区分：

| 名称 | 提供者 | 入口 | 注入 SDK | 能力边界 |
| --- | --- | --- | --- | --- |
| **系统面板页**（Plugin Console） | alx 同源托管系统插件的静态目录 | `/api/v1/setup/plugins/web/<id>/` | `ALXHost` | 动作转发、宿主能力、特权操作 |
| **宿主 WebView**（Host WebView） | 系统插件通过 `ALXHost.webview.open` 打开 | 宿主管理窗口 | 无（普通 iframe） | 仅插件静态资源或外部 http(s) URL，无宿主特权 |
| **机器人应用页**（Bot App Page） | 机器人包内的插件页面，alx 代理 | `/api/v1/robot/webview/<token>/<id>/` | `window.__alxWebview` | 仅当前机器人的 `./api/*` |

机器人应用页的完整规范见[机器人应用页规范](bot-app-page.md)。

## 插件结构

一个系统插件是如下目录：

```text
plugins/
  my-status/
    alx.json        # 清单（必填，精确文件名，≤64 KiB，非符号链接）
    web/            # 系统面板页根目录（必填，同源托管，SPA 亦可）
      index.html
    runner/         # 执行器（可选；声明了 entry 才能执行动作）
      main.mjs
    dist/           # 发布用的平台二进制（Release 包内）
    .alx-install.json  # 宿主安装后写入，勿手工维护
```

运行中的 ALemonX 按以下顺序发现插件，同一个 `id` 只加载第一个发现位置：

1. `<workspace>/plugins/`：用户安装插件的唯一写入目录，也是最高优先级；
2. 程序提供的 `plugins/`：已安装程序使用可执行文件同级目录，源码开发可使用当前工作目录的 `plugins/` 作为兼容读取目录。

不会再从用户配置目录的 `alx/plugins/` 读取插件。插件数据不应写入插件代码目录；执行器应使用 `ALX_PLUGIN_STORE` 提供的 `<workspace>/store/<插件 ID>/`。

插件目录的增删与 `alx.json` 修改由后台**自动热更新**（约 1 秒内反映），无需重启或手动刷新；前端通过修订号（revision）或 SSE 事件感知变化。

## 快速开始

最小清单：

```json
{
  "id": "my-status",
  "name": "示例状态",
  "version": "1.0.0",
  "runtime": "node",
  "entry": { "darwin-arm64": "runner/main.mjs", "linux-amd64": "runner/main.mjs", "linux-arm64": "runner/main.mjs", "windows-amd64": "runner/main.mjs", "windows-arm64": "runner/main.mjs" },
  "web": { "root": "web" },
  "navigation": { "label": "示例", "icon": "circle", "order": 10 }
}
```

源码开发时可放在仓库根目录的 `plugins/` 下作为兼容读取目录（同 ID 未被 `<workspace>/plugins/` 覆盖时生效）；发布时通过 GitHub Release 提供平台安装包（见[发布、安装与版本管理](#发布安装与版本管理)）。

## alx.json 清单参考

清单文件名必须精确为 `alx.json`，最大 64 KiB，且不能是符号链接。`id` 必须匹配 `^[a-z][a-z0-9-]{1,63}$`。

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id` | 是 | 稳定插件标识，建议发布后不再变更。 |
| `name`、`version` | 是 | 管理台显示的名称与版本。 |
| `description` | 否 | 插件说明。 |
| `platforms` | 否 | 支持的 Go 平台名（`darwin`/`linux`/`windows`）；未填写表示全部平台。 |
| `navigation` | 否 | 全局功能栏入口；`label` 默认取 `name`，`icon` 默认 `◈`，`order` 越小越靠前。 |
| `runtime` | 否 | `binary`、`node`、`go` 或 `python`；省略时为 `binary`。 |
| `entry` | 是* | 执行器映射。键为 `GOOS-GOARCH`（如 `linux-amd64`），也可仅用 `GOOS` 作回退。路径必须是插件目录内的普通文件，禁止绝对路径、`..` 越界或符号链接。 |
| `web` | **是** | 系统面板页目录（如 `web`），不能是绝对路径或含 `..`。无 `web` 的插件不可用。 |
| `services` | 否 | 清单白名单内的回环 HTTP 服务，宿主以认证同源代理暴露（见[本地服务](#本地服务services)）。 |
| `systemPickers` | 否 | 命名的工作台 Finder 请求（文件/目录、单选/多选、标题）。 |
| `uploads` | 否 | 允许接收浏览器上传字节的 runner 动作与 `maxBytes` 上限（见[上传](#上传uploads)）。 |
| `statusActions` | 否 | 显式只读的 runner 动作；宿主合并查询、不分配任务，适合 UI 快速轮询。 |
| `media` | 否 | 由 runner 动作产出的类型化字节（二维码、截图等），经宿主同源接口提供给 UI。 |
| `privilegedOperations` | 否 | 需要宿主授权的系统操作：`authorization`（`password`/`native`）、平台、`runnerAction`、可选 `planAction`/`useLatestAudit`（见[特权操作与审计](#特权操作与审计)）。 |
| `development` | 否 | 源码开发声明：开发 runner、前端构建/开发服务、受管本地服务（见[源码开发会话](#源码开发会话)）。 |

`runtime` 与 `entry` 的启动方式：

| runtime | entry 取值 | 启动方式 |
| --- | --- | --- |
| `binary` | 平台二进制路径 | 直接执行该文件 |
| `node` | `.mjs`/`.js` 路径 | `node <entry>` |
| `go` | `entry.go` 路径或目录 | `go run <entry.go>` / `go run <dir>` |
| `python` | `entry.python` 路径 | `python3 <entry>` |

纯静态插件（未声明 `entry`）可以托管 `web` 界面用于展示信息，但任何动作调用都会因缺少执行器而失败。`runnable` 对本地插件恒为真（仅在线识别插件为假）。

## 通讯协议

系统插件的通讯分为三层：

```text
面板页（同源 iframe）
  │  POST /api/v1/setup/plugins/<id>/actions  {action, confirm, params}
  ▼
宿主（alx：校验插件身份、任务互斥、进度/审计）
  │  stdin: {"protocol":"alx/v1","method":"run","action":...,"params":...}
  │  stdout: {"output":...,"data":...,"error":...}（唯一 JSON 响应）
  │  stderr: 日志 / @alx-progress 进度帧
  ▼
执行器（独立进程，每次动作一个新进程）
```

### 请求（宿主 → 执行器 stdin）

每次动作宿主启动一个独立进程，工作目录为插件目录，向标准输入写入一个 JSON 对象：

```json
{
  "protocol": "alx/v1",
  "method": "run",
  "action": "network-check",
  "params": {},
  "confirm": false
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `protocol` | 固定 `alx/v1`，执行器应校验。 |
| `method` | 固定 `run`。 |
| `action` | 动作名，由执行器自行白名单；宿主不校验语义，执行器内部 `switch` 只处理自己支持的动作并校验每个参数。 |
| `params` | 字符串到字符串的映射（动作参数）。 |
| `confirm` | 兼容字段；危险操作的二次确认由面板页负责，宿主只在特权操作时置 `true`。 |
| `stateDir` | 保留字段，当前宿主不填充；执行器勿依赖。 |

### 响应（执行器 → 宿主 stdout）

从标准输出读取**唯一的 JSON 响应**。不要在标准输出打印日志或调试文本；日志请写到标准错误。

成功：

```json
{ "output": "✓ 已检查 3 个网卡。", "data": { "interfaces": [] } }
```

失败（仍要输出合法 JSON，并设置 `error`）：

```json
{ "output": "已检查现有规则。", "error": "需要管理员权限" }
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `output` | 人类可读的结果摘要。每行以 `✓ `、`! ` 或 `? ` 开头（符号后一个空格）可按状态着色；缩进行按普通文本显示。成功但缺省时显示「插件操作已完成。」 |
| `data` | 可选结构化结果，供 Web UI 使用；旧插件只返回 `output` 仍兼容。 |
| `error` | 非空即视为操作失败。 |

以下任一情况任务都会标记为失败：进程非零退出、stdout 没有合法 JSON、`error` 非空。

### 进度事件（stderr）

可选。执行器可向标准错误输出形如 `@alx-progress ` 前缀的进度帧，宿主会转发给任务进度；其余 stderr 作为诊断文本保留（失败时展示）：

```text
@alx-progress {"stage": "prepare", "percent": 10, "message": "正在检查环境…"}
```

要求：`stage` 非空、`percent` 为 0–100 的整数。不是进度帧的 stderr 行不会被当作进度解析。

### 任务模型与轮询

- **变更类动作**（普通 `actions`、上传、特权操作）走异步任务：`POST /actions` 立即返回任务信息（`202 Accepted`），随后轮询 `GET /api/v1/robot/tasks` 获取结果（`status`：`running`/`completed`/`failed`，结果在 `output`，结构化结果在 `data`，失败原因在 `error`，进度在 `progress`）。
- **互斥**：同一插件同一时刻只允许一个变更操作。重复提交相同动作返回 `202` 与现有任务；冲突动作返回 `409`。
- **只读状态动作**：`GET /api/v1/setup/plugins/<id>/status?action=<name>`，仅限清单 `statusActions`、无参数。宿主合并并发读取并保留 1 秒内存缓存，**不分配任务**，因此适合 UI 快速轮询。

### 环境变量

执行器继承宿主环境（PATH 会并入受管 Node 工具链目录）。宿主可按需注入受控环境变量；**清单不能请求环境变量**，这是宿主策略层，不是清单特性。当前受信任注入：

| 变量 | 说明 |
| --- | --- |
| `ALX_PLUGIN_DOWNLOAD_BROKER` | 下载 Broker 的回环端点（`/api/v1/system/plugin-download`）。 |
| `ALX_PLUGIN_DOWNLOAD_TOKEN` | 一次性下载令牌（24 次请求、90 分钟过期）。 |
| `ALX_PLUGIN_PROGRESS_MODE` | `structured`：表示 stderr 进度帧可用。 |
| `ALX_PLUGIN_INSTALLED_TAG` | 已安装 Release 的 tag。 |
| `ALX_PLUGIN_STORE` | 宿主创建的插件持久数据目录：`workspace/store/<插件 ID>`。下载的运行包、登录态、数据库和可恢复配置应默认保存到这里；插件升级不会清除该目录。 |
| `ALX_PLUGIN_DEV_PORT` | 源码开发命令中唯一可替换变量（宿主分配的回环端口）。 |

`ALX_PLUGIN_STORE` 会注入普通执行器、经宿主授权的特权执行器，以及源码插件的构建、开发服务和开发 Web 进程。静态 Web 页面不能直接读取该目录，需通过 runner 提供受控接口。

### 最小 Node.js 执行器

```js
import process from 'node:process'

let input = {}
try {
  input = JSON.parse(await new Response(process.stdin).text())
  if (input.protocol !== 'alx/v1' || input.method !== 'run') {
    throw new Error('不支持的 ALX Setup 插件协议')
  }
  if (input.action !== 'check') throw new Error('未知操作')
  process.stdout.write(JSON.stringify({ output: '✓ 检查完成。' }))
} catch (error) {
  process.stdout.write(JSON.stringify({ error: String(error.message || error) }))
}
```

## HTTP 接口总览

除插件自身 Web 资源外，全部接口都在 `/api/v1` 下，由宿主校验登录会话与插件身份。下表按用途分组。

### 插件生命周期与市场

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins` | 本地发现的插件列表（含已停用、仅在线识别的条目）。 |
| GET | `/api/v1/setup/plugins/market` | 在线精选目录，独立于本地安装。 |
| GET | `/api/v1/setup/plugins/revision` | 插件集修订号，用于廉价检测热插拔。 |
| GET | `/api/v1/setup/plugins/events` | SSE「插件已变更」事件流（`data: {}`，25 秒心跳）。 |
| GET | `/api/v1/setup/plugins/releases/<id>` | 插件 GitHub Release 列表与资产（含 SHA-256、平台兼容性）。 |
| POST | `/api/v1/setup/plugins/upload` | 上传并安装插件压缩包（`.zip`/`.tar.gz`/`.tgz`，单文件，宿主校验解包）。 |
| POST | `/api/v1/setup/plugins/<id>/install` | 按 `{version, assetName}` 下载并安装 Release。首次安装默认启用；若同 ID 曾被显式停用，仍需重新启用。 |
| POST | `/api/v1/setup/plugins/<id>/enabled` | 启用/停用：`{enabled: bool}`。 |
| POST | `/api/v1/setup/plugins/<id>/switch` | 切换版本：`{version, assetName}`。 |
| POST | `/api/v1/setup/plugins/<id>/uninstall` | 卸载：`{confirm: true}`。 |
| GET | `/api/v1/setup/plugins/<id>/versions` | 已缓存版本列表（tag/asset/大小/指纹/活动/缓存状态）。 |
| DELETE | `/api/v1/setup/plugins/<id>/versions/<tag>` | 删除某个非活动缓存版本。 |
| GET/POST | `/api/v1/setup/plugins/cache` | 插件版本缓存摘要；POST 立即清理缓存。 |

### 动作转发与数据

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/setup/plugins/<id>/actions` | 动作转发：`{action, confirm, params, sudoPassword?, authorizationId?}`。 |
| GET | `/api/v1/setup/plugins/<id>/status?action=<name>` | 只读状态动作（仅清单 `statusActions`、无参数）。 |
| GET | `/api/v1/setup/plugins/<id>/media/<id>` | 清单声明的媒体资源（runner 返回 `{available, data: <base64>}`）。 |
| POST | `/api/v1/setup/plugins/<id>/upload` | 浏览器文件上传（multipart：`action`、`destination`、`files`），见[上传](#上传uploads)。 |

### 面板页托管

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins/web/<id>/...` | 系统面板页静态资源（同源）。仅对**已安装、已启用、可运行**的插件提供；无扩展名且资源不存在的路径回退到 `index.html`（SPA 路由）。HTML 自动注入 `host-bridge.js`（即 `ALXHost` SDK），设置 CSP 与 `X-Content-Type-Options: nosniff`，拒绝路径穿越与符号链接逃逸。 |
| GET | `/api/v1/setup/plugins/web/<id>/host-bridge.js` | 宿主桥接 SDK（`finder-bridge.js` 为别名）。 |

### 宿主能力

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/system/capabilities` | 版本化能力目录（名称、版本、可用性、不可用原因）。 |
| POST | `/api/v1/system/capabilities/webview/open` | 打开宿主 WebView 窗口：`url` 或 `resource` 二选一（见[宿主 WebView 与全局 UI 组件](#宿主-webview-与全局-ui-组件)）。 |
| POST | `/api/v1/system/capabilities/finder` | 按 `{pluginId, pickerId}` 返回 Finder 定义（kind/title/multiple），由工作台渲染。旧路径 `/api/v1/system/picker` 兼容。 |
| GET | `/api/v1/system/capabilities/context?pluginId=&keys=robot,network` | 脱敏只读上下文。键仅限 `robot`、`network`。 |
| POST | `/api/v1/system/capabilities/desktop/open` | 打开路径/URL（交给系统）。 |
| GET/POST | `/api/v1/system/capabilities/clipboard` | 读取/写入系统剪贴板。 |
| POST | `/api/v1/system/capabilities/notification` | 发送系统通知。 |
| GET | `/api/v1/system/capabilities/info?pluginId=` | 平台、架构、主机名。 |
| POST | `/api/v1/system/capabilities/network/fetch` | 受限网络请求（仅 GET/HEAD，≤2 MiB，30 秒超时）。 |

### 特权授权与审计

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/system/privileged/status` | 特权模式与审计链状态。 |
| POST | `/api/v1/system/privileged/preflight` | `{pluginId, action, planId?}`，返回可用性、授权方式与一次性 `intentId`。 |
| GET | `/api/v1/system/privileged/audit?plugin=<id>` | 指定插件的审计记录。 |

### 本地服务代理

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/services?plugin=<id>` | 插件声明的回环服务列表与可达性、代理地址。 |
| GET/HEAD/POST/PUT/PATCH/DELETE | `/api/v1/services/<plugin>/<service>/...` | 清单服务同源代理（WebSocket 需声明 `websocket`）。 |
| 同上 | `/api/v1/services/dynamic/<plugin>/<port>/...` | 运行时回环端口代理（如 Docker 发布端口），目标强制 `127.0.0.1`。 |

### 下载 Broker（runner 专用）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/HEAD | `/api/v1/system/plugin-download?url=...` | 大文件下载：宿主代理、重定向、一次重试、条件请求与用户目录缓存；**不转发**工作台 Cookie、Authorization 或内部身份头。仅 loopback 且带令牌可用。 |
| GET/DELETE | `/api/v1/system/plugin-download-cache` | 下载缓存摘要（全局 1 GiB）与清理；与 Release 版本缓存完全独立，清理不会卸载插件或删除其数据。 |

### 任务与事件

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/robot/tasks` | 轮询插件动作任务结果。 |
| GET | `/api/v1/events?topics=robot,ops,system,plugins` | SSE 事件流，含 `plugins.changed`（插件列表变更信号）。 |

## 宿主能力与 ALXHost SDK

插件不声明 `hostCapabilities`。浏览器页面需要工作台桌面能力或上下文时，通过宿主提供的版本化能力目录与对应 API 请求；每个调用宿主仍会校验插件身份和当前登录会话。

### 能力目录

`GET /api/v1/system/capabilities` 返回：

```json
{ "items": [ { "name": "finder.pick", "version": "v1", "available": true, "reason": "" } ] }
```

| 能力 | API | 返回范围 |
| --- | --- | --- |
| 目录 | `GET /api/v1/system/capabilities` | 版本、可用性与不可用原因。 |
| `webview.open` | `POST /api/v1/system/capabilities/webview/open` | 打开宿主管理的 WebView 窗口（静态资源或 http(s) URL）。 |
| `ui.alert` / `ui.message` / `ui.modal` / `ui.notification` | 经父窗口 `ui-request` 消息桥 | 宿主渲染的全局前端组件，保持交互一致性。 |
| `finder.pick` | `POST /api/v1/system/capabilities/finder` | 仅由清单 `pickerId` 定义的工作台 Finder 文件/目录选择结果。 |
| `context.current-robot` / `context.network-settings` | `GET /api/v1/system/capabilities/context?pluginId=<id>&keys=robot,network` | 当前已验证机器人与脱敏网络配置；**不包含密码**。 |
| `desktop.open`、`clipboard.read/write`、`notification.send`、`system.info` | 对应 `/api/v1/system/capabilities/{...}` | 受认证、绑定插件身份的桌面基础能力。 |
| `network.fetch` | `POST /api/v1/system/capabilities/network/fetch` | 使用主应用代理、镜像与重试设置的 GET/HEAD 请求。 |

Finder 请求只传 `{ "pluginId": "...", "pickerId": "..." }`。宿主从清单取得窗口标题、类型和多选策略，并在父工作台中打开统一的 Web Finder；**插件不能提交路径、命令、端口或原生脚本**。旧 `/api/v1/system/picker` 暂时兼容已发布插件，新插件应使用 capability API。

### ALXHost SDK

每个插件页面自动获得无依赖的 `window.ALXHost` 小 SDK（由宿主注入 `host-bridge.js`），它只是上述能力 API 的便捷封装：

```js
const paths = await window.ALXHost.finder.pick('my-status', 'runtime-directory')
await window.ALXHost.desktop.open('my-status', paths[0])
const { robot } = await window.ALXHost.context('my-status', ['robot'])
await window.ALXHost.notification.send('my-status', '已完成', robot?.name || '未选择机器人')
const info = await window.ALXHost.info('my-status')
const resp = await window.ALXHost.network.fetch('my-status', 'https://example.com/status', 'GET')
const clip = await window.ALXHost.clipboard.read('my-status')
await window.ALXHost.clipboard.write('my-status', 'hello')
```

SDK 方法一览：

| 方法 | 等价请求 |
| --- | --- |
| `capabilities()` | `GET /api/v1/system/capabilities` |
| `webview.open(pluginId, options)` | `POST /api/v1/system/capabilities/webview/open` 校验后，向父窗口发 `webview-request`，宿主打开窗口并回执 `webview-result`（含窗口 `id`）。 |
| `webview.close(pluginId, webviewId?)` | 向父窗口发 `webview-close-request`，关闭指定窗口（缺省关闭该插件全部 WebView），回执 `webview-close-result`。 |
| `ui.alert(pluginId, options)` / `ui.message(pluginId, options)` / `ui.modal(pluginId, options)` / `ui.notification(pluginId, options)` | 向父窗口发 `ui-request`，宿主渲染全局组件并回执 `ui-result`。 |
| `ui.setBusy(pluginId, busy)` | 设置当前展示的确认弹窗为忙碌状态（`busy: true` 禁用确认/取消与 Escape，确认按钮显示加载态），回执 `ui-result`。 |
| `finder.pick(pluginId, pickerId)` | 拦截 `POST /api/v1/system/capabilities/finder`（或旧 `/api/v1/system/picker`），与父窗口 `postMessage` 桥接，等待工作台 Finder 选择（10 分钟超时）。 |
| `context(pluginId, keys)` | `GET /api/v1/system/capabilities/context` |
| `desktop.open(pluginId, target)` | `POST /api/v1/system/capabilities/desktop/open` |
| `clipboard.read(pluginId)` / `clipboard.write(pluginId, text)` | `GET/POST /api/v1/system/capabilities/clipboard` |
| `notification.send(pluginId, title, message)` | `POST /api/v1/system/capabilities/notification` |
| `info(pluginId)` | `GET /api/v1/system/capabilities/info` |
| `network.fetch(pluginId, url, method)` | `POST /api/v1/system/capabilities/network/fetch` |

`ALXHost.network.fetch` 仅提供 GET/HEAD 和最多 2 MiB 的响应，用于状态或元数据；**大文件必须由插件 runner 通过下载 Broker 获取**（流式进度、取消、重试和缓存）。

### Finder 桥接消息

插件页面发起 Finder 选择时，SDK 拦截对应 `fetch`，并向父窗口发送：

```js
{ source: 'alx-setup-plugin', type: 'finder-request', requestId, pluginId, pickerId }
```

工作台渲染 Web Finder 后回传：

```js
{ source: 'alx-parent', type: 'finder-result', requestId, paths: [...] }   // 成功
{ source: 'alx-parent', type: 'finder-result', requestId, error: '已取消选择。' } // 失败/取消
```

## 宿主 WebView 与全局 UI 组件

插件页面无需自行实现窗口 chrome 或弹层组件。宿主直接提供两个能力，保证所有系统插件交互一致：

### webview.open：宿主管理的 WebView 窗口

插件不需要再实现自己的 webview。`webview.open` 会校验参数后让工作台打开一个标准的悬浮窗口（可拖拽、缩放、最小化、最大化，行为与内置窗口一致），窗口内嵌 iframe；多个 WebView 自动级联错开位置，避免完全重叠：

```js
// 打开插件自身 web 根目录里的静态页面（同源，自动带 ALXHost）
const opened = await window.ALXHost.webview.open('my-status', {
  title: '详细状态',
  resource: 'pages/detail.html',
  width: 900,
  height: 640
})

// 打开外部 HTTP(S) 地址（第三方站点，不获得任何宿主能力）
await window.ALXHost.webview.open('my-status', {
  title: '官方文档',
  url: 'https://docs.example.com/'
})

// 关闭刚打开的窗口（不传 id 则关闭该插件打开的全部 WebView）
await window.ALXHost.webview.close('my-status', opened.id)
```

约束与细节：

- 每个插件同时最多打开 **8 个 WebView**，超出时 Promise 立即以 `{ ok: false, error }` 拒绝。
- 静态页 WebView 自动携带当前主题参数（`?theme=light|dark`），与插件主页保持一致；窗口副标题显示实际地址。
- 所有插件 WebView 都会由宿主注入 `--alemonjs-*` 主题变量（`docs/theme.json` 的完整契约，含 `[data-theme='dark']` 下的暗色覆盖），页面可直接使用 `var(--alemonjs-primary-bg)` 这类变量，无需自带主题色拷贝。
- 同一插件打开同一资源（按去掉主题参数后的地址）时会恢复上次的窗口位置与尺寸，位置记忆存在浏览器本地。

参数：

| 字段 | 说明 |
| --- | --- |
| `title` | 窗口标题，缺省用插件名。 |
| `url` | 外部地址；仅允许 `http`/`https`，且不能包含用户名密码（userinfo），与 `resource` 二选一。 |
| `resource` | 插件 `web.root` 内的相对路径（如 `pages/detail.html`，可带查询串 `pages/detail.html?tab=1`）；禁止绝对路径、反斜杠与任何 `..` 路径段。宿主校验资源**真实存在**（无扩展名路径按 SPA 回退到 `index.html`）；静态页复用插件托管（含 CSP 与 `ALXHost` 注入）。 |
| `width` / `height` | 初始尺寸，宿主钳制在 480–1600 × 420–1200，缺省 960 × 680。 |

`webview.open` 的 Promise 解析为 `{ ok: true, id }`（成功）或 `{ ok: false, error }`（宿主拒绝/超时）；`webview.close` 解析为 `{ ok: true, closed }`（关闭数量）。窗口也可以由用户手动关闭。

嵌套行为：WebView 里的同源静态页同样拥有 `ALXHost`，可以调用 `ui.*` 与 `finder.pick`（宿主在父工作台渲染并回执到对应 iframe）；但**打开/关闭 WebView 仅限插件主页**，WebView 页调用会立即收到 `{ ok: false, error: '仅插件主页可管理 WebView。' }`。

### ui.*：宿主全局前端组件

为了交互一致性，Alert、Message、Modal、Notification 都由宿主统一渲染，插件只传数据，不再各自实现一套：

```js
await window.ALXHost.ui.alert('my-status', {
  title: '环境未就绪',
  message: '请先安装 Node.js 18+ 再继续。',
  confirmText: '知道了'
})

const { confirmed } = await window.ALXHost.ui.modal('my-status', {
  title: '确认删除',
  message: '该操作会移除已保存的配置，且无法撤销。',
  confirmText: '删除',
  cancelText: '取消'
})

// 需要等待异步操作时，可把确认弹窗置为忙碌状态：
const pending = window.ALXHost.ui.modal('my-status', {
  title: '系统授权',
  message: '即将执行系统操作。',
  confirmText: '执行'
})
await window.ALXHost.ui.setBusy('my-status', true) // 弹窗禁用，等待授权预检
// ... 异步准备 ...
await window.ALXHost.ui.setBusy('my-status', false) // 恢复可确认
const { confirmed } = await pending

await window.ALXHost.ui.message('my-status', {
  type: 'success',
  title: '已保存',
  message: '配置已写入工作区。',
  duration: 3000
})

await window.ALXHost.ui.notification('my-status', {
  type: 'warning',
  title: '端口被占用',
  message: '17117 已被其他进程使用。'
})
```

各组件参数与回执：

| 组件 | 参数 | Promise 结果 |
| --- | --- | --- |
| `ui.alert` | `title`、`message`、`confirmText` | 任意关闭即 `{ ok: true }` |
| `ui.modal` | `title`、`message`、`confirmText`、`cancelText` | `{ ok: true, confirmed: true \| false }` |
| `ui.message` | `type`（`info`/`success`/`warning`/`error`）、`title`、`message`、`duration`（毫秒，钳制 1500–15000，缺省 4000） | 立即 `{ ok: true }`，顶部居中短暂展示 |
| `ui.notification` | 同 `message`（缺省 6000） | 立即 `{ ok: true }`，右下角通知 |

宿主在插件 iframe 的父工作台中渲染这些组件（`Modal`/`ConfirmDialog`/toast），因此视觉、快捷键与系统模态层级与工作台完全一致；同类型消息/通知最多同时保留 5 条，超出时丢弃最旧一条。`ui.*` 是应用内组件；系统级操作系统通知仍用 `notification.send`。

`ui.alert` 与 `ui.modal` 按请求顺序**排队展示**（队列上限 8 个，超出立即以 `{ ok: false, error }` 拒绝）：插件连续弹多个确认框时，后一个会等前一个关闭后再出现，每个 Promise 都在对应弹窗关闭时按序回执；`ui.message`/`ui.notification` 不排队，立即展示。

交互细节：`ui.alert` 与 `ui.modal` 均支持 **Escape 关闭**（等价于取消）；`ui.alert` 使用 `alertdialog` 语义并自动聚焦确认按钮，`ui.modal` 复用工作台 `ConfirmDialog`（含遮罩点击取消）。`ui.setBusy` 只作用于**当前展示**的确认弹窗（队列头部）：忙碌期间确认、取消、遮罩点击与 Escape 全部失效，确认按钮显示加载态，避免异步授权过程中被误关。

超时：`webview.*` 与 `ui.*` 的宿主回执等待 **60 秒**（父窗口未响应即返回 `{ ok: false, error }`）；`finder.pick` 保持 10 分钟（等待用户选择）。插件不应依赖超时作为正常路径。

## 本地服务（services）

插件可以声明一个或多个**回环 HTTP 服务**（如内嵌 WebUI），宿主以认证同源代理暴露，浏览器从不直接访问其 host/port：

```json
"services": [{
  "id": "webui",
  "name": "管理界面",
  "host": "127.0.0.1",
  "port": 8080,
  "basePath": "/",
  "healthPath": "/health",
  "embed": true,
  "rewriteHtml": true,
  "rewriteApiBase": true,
  "sse": true,
  "websocket": true
}]
```

| 字段 | 说明 |
| --- | --- |
| `id`/`name` | 服务标识与显示名。 |
| `host`/`port` | 回环地址；浏览器不能指定，只能由清单声明（或经动态端口代理）。 |
| `basePath`/`healthPath` | 挂载子路径与健康检查路径。 |
| `embed` | 是否适合 iframe 内嵌。 |
| `rewriteHtml` | 代理时改写 HTML（注入 API 兼容脚本）。 |
| `rewriteApiBase` | 把页面里根相对 `/api/...` 请求重写到服务挂载点，兼容「监听端口不同」的跳转。 |
| `sse`/`websocket` | 允许 SSE / WebSocket 升级。 |

服务列表与可达性通过 `GET /api/v1/services?plugin=<id>` 获取；代理入口为 `/api/v1/services/<plugin>/<service>/...`。**动态端口服务**（如 Docker 发布端口，无法在清单静态声明）走 `/api/v1/services/dynamic/<plugin>/<port>/...`，目标强制为 `127.0.0.1`，校验与清单服务一致。

## 上传（uploads）

浏览器文件不适合塞进 JSON 参数。插件在清单中声明上传动作后，Web UI 以 `multipart/form-data` 向 `POST /api/v1/setup/plugins/<pluginId>/upload` 发送：

- `action`：清单 `uploads[].action`（若清单只有一个上传动作可省略）；
- `destination`：绝对目标目录；
- `files`（或 `file`）：一个或多个文件字段。

```json
"uploads": [{ "action": "upload", "maxBytes": 2147483648 }]
```

宿主行为：按 `maxBytes` 限制总大小（请求体上限 2 GiB + 1 MiB）、流式暂存到临时目录（文件名做 basename/长度/重复校验）、执行对应的 runner 动作并传入 `params: { stagingDir, destination }`，动作结束后清理暂存内容。**插件不能把任意动作参数当本地文件**，上传走专用通道。

## 特权操作与审计

需要系统授权的操作通过 `privilegedOperations` 声明。宿主只提供**授权、进程启动、进度与审计**；动作语义与平台细节由插件拥有，宿主不校验执行器内部行为。

```json
"privilegedOperations": [{
  "action": "firewall-set",
  "runnerAction": "firewall-set",
  "planAction": "plan",
  "title": "启用或停用系统防火墙",
  "description": "网络插件将按已确认的变更计划启用或停用系统防火墙。",
  "authorization": "native",
  "platforms": ["linux", "windows"]
}]
```

| 字段 | 说明 |
| --- | --- |
| `action` | 面板页调用的动作名。 |
| `runnerAction` | 实际传给执行器的动作名（宿主在授权后调用）。 |
| `authorization` | `password`（工作台 sudo 密码）或 `native`（系统原生授权）。 |
| `platforms` | 授权适用的平台。 |
| `commands` | 仅 `password` 模式：宿主在 PATH 中查找并执行的固定命令（结构化 `program`/`args`，不经过 shell）。 |
| `planAction` | 可选：先执行「计划」动作生成变更计划（存于宿主），确认后按计划执行。 |
| `useLatestAudit` | 可选：不携带计划，而是把最近一次审计的变更作为撤销/重做对象。 |

### 授权流程

```text
Web UI ── POST /system/privileged/preflight {pluginId, action, planId?}
    ← {available, authorization, title, description, intentId, expiresAt, sourceType, sourcePath}
Web UI ── POST /setup/plugins/<id>/actions
    {action, confirm: true, authorizationId, sudoPassword?}   // password 模式带 sudoPassword
宿主：校验一次性 intent（绑定插件/动作/计划/账户/来源，5 分钟过期）
    → password：执行清单固定命令（sudo）；native：UAC/Polkit/原生授权后运行 runnerAction
    → 成功后写入链式哈希审计（privilege_audit）
```

注意：

- `native` 模式各平台映射：darwin → `native`，windows → `native-uac`，linux → `polkit`。
- `password` 模式只在认证启用时允许超级管理员；密码在内存中立即清零，失败尝试有锁定记录。
- `planAction`/`useLatestAudit` 的操作不接受浏览器额外参数，只接受宿主签发的 `planID` 或最近审计；执行时宿主向 runner 传入 `{ operation, ...planParams, __alxFingerprint? }`。
- 特权模式由 `ALX_PRIVILEGED_MODE` 控制（`enabled` / `local`）；`local` 仅限本机回环且无转发头。
- 审计表带哈希链（`previous_hash`/`chain_hash`）防篡改，`GET /api/v1/system/privileged/audit?plugin=<id>` 可读取，`privileged/status` 返回链完整性与签名版本。
- 特权弹窗中的**密码输入**与**结构化变更计划展示**（风险/影响/参数/验证）由插件页面自绘，宿主 `ui.modal` 只提供标题+正文，不包含输入框与富文本——这是有意保留的边界。

## 源码开发会话

源码开发在插件中心「开发插件」入口登记源码目录（经工作台 Finder 约束到受管目录），点「启动开发」才执行其中声明的本地命令。源码会话临时覆盖同 ID 的正式 Release；停止后立即恢复 Release；工作台重启后**不会自动执行**源码命令。源码开发只能在本机桌面工作台中执行。

```json
{
  "development": {
    "runtime": "go",
    "entry": { "go": "runner/main.go" },
    "web": {
      "mode": "dev-server",
      "root": "web",
      "build": { "program": "yarn", "args": ["--cwd", "frontend", "build"] },
      "dev": { "program": "yarn", "args": ["--cwd", "frontend", "dev", "--host", "127.0.0.1", "--port", "${ALX_PLUGIN_DEV_PORT}"] },
      "healthPath": "/",
      "hmr": true
    },
    "services": [
      { "id": "api", "command": { "program": "go", "args": ["run", "./runner"] }, "restart": "on-failure" }
    ]
  }
}
```

要点：

- `runtime` 可为 `go`/`node`/`command`；命令使用结构化 `program` + `args`，主机不经过 shell；`${ALX_PLUGIN_DEV_PORT}` 是唯一可替换变量。
- `web.mode`：`static`（优先托管 `development.web.root`，未声明回退顶层 `web.root`）或 `dev-server`（宿主启动并同源代理，健康检查通过 `healthPath`）；只有 `hmr: true` 才允许 HMR WebSocket，不暴露额外网络端口。
- `development.services` 为清单已声明服务的受管命令，重启策略 `never`/`on-failure`；服务 host/port 仍在服务白名单内，开发命令不能代理到别处。
- 会话状态机：`registered → starting → running → stopping → stopped`，另有 `building`/`failed`；同一会话同一时刻只允许一个操作。

源码开发接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins/development` | 已登记源码会话列表。 |
| POST | `/api/v1/setup/plugins/development` | 登记源码目录 `{path}`（经 Finder 选择）。 |
| POST | `/api/v1/setup/plugins/development/<id>/start` | 启动（需 `{confirm:true}`）。 |
| POST | `/api/v1/setup/plugins/development/<id>/stop` | 停止（确认受管进程组退出后才恢复 Release）。 |
| POST | `/api/v1/setup/plugins/development/<id>/restart` | 重启（需确认）。 |
| POST | `/api/v1/setup/plugins/development/<id>/build` | 构建前端（需确认）。 |
| GET | `/api/v1/setup/plugins/development/<id>/logs` | 会话日志（上限 256 KiB，超出截断头部）。 |
| GET | `/api/v1/setup/plugins/development/<id>/services/<sid>/logs` | 服务日志。 |
| POST | `/api/v1/setup/plugins/development/<id>/services/<sid>/restart` | 重启服务（需确认）。 |
| DELETE | `/api/v1/setup/plugins/development/<id>/remove` | 移除登记（不执行命令）。 |
| GET | `/api/v1/setup/plugins/development/<id>/web/...` | 开发 Web 代理（HTML 注入宿主桥，非 HTML 原样代理）。 |

源码插件同样可以请求 `privilegedOperations`；工作台会明确标示「源码开发授权」与源码目录，并仍要求管理员单次确认与当前主机权限模式。

## 系统功能与系统组件

系统插件不是孤立脚本，它接入工作台整套「窗口化」界面。以下是插件相关的系统功能与组件。

### 系统窗口

工作台以桌面窗口承载系统功能（[DesktopWindow.tsx](../frontend/src/components/DesktopWindow.tsx) 与 [SidebarWindow.tsx](../frontend/src/components/SidebarWindow.tsx)）。系统功能分两类：

- **内置功能**：`plugins`（插件中心）、`ops-overview`（运维）、`tasks`（任务）、`environment`（环境检查）、`accounts`（账户）。其中插件/运维/任务/环境使用带侧栏的 `SidebarWindow`，其余用普通 `DesktopWindow`。
- **插件窗口**：每个已安装插件生成 `setup:<pluginId>` 窗口，标题取自插件 `name`，图标取自 `navigation.icon`，内容为系统面板页（同源 iframe）。

### 插件中心（SystemPluginCenter）

工作台「插件」功能提供三个视图：

- **市场**：在线精选目录（`/setup/plugins/market`），列出 Release 与资产，可安装。
- **我的**：本地插件列表，支持启用/停用、卸载、切换版本、删除缓存版本、缓存清理、上传本地压缩包安装。
- **开发**：登记/启动/停止/重启/构建源码会话，查看会话与服务日志。

### 系统面板页（SetupPluginCenter）

插件窗口内嵌系统面板页：

- Web 地址：`/api/v1/setup/plugins/web/<id>/index.html?theme=light|dark`（源码 dev-server 会话则为 `/api/v1/setup/plugins/development/<id>/web/`），宿主注入滚动条主题。
- Finder 桥：页面发起 `finder-request` 消息时，宿主打开统一 Web Finder（`DirectoryPicker`）并把结果回传给 iframe。
- 无 Web 或未安装的插件显示占位说明（在线识别需先安装）。

### 全局功能栏与图标

清单 `navigation` 把插件放进工作台全局功能栏：`{ label, icon, order }`，按 `order` 升序、名称排序。前端映射以下 lucide 图标名，未匹配时支持单个字符或默认插件图标：

`network`、`forward`/`forwarding`、`interface`、`lan`、`wifi`、`route`、`dns`/`mirror`/`proxy`、`firewall`/`shield`、`port`、`traffic`。

### 其他宿主系统组件

- **能力目录**：`/api/v1/system/capabilities` 返回每项能力的可用性与不可用原因，前端据此做降级提示。
- **宿主 WebView 窗口**：插件经 `webview.open` 打开的窗口复用 `DesktopWindow` 外壳（拖拽/缩放/最小化/最大化），由 `PluginHostUi` 渲染。
- **全局 UI 组件**：`ui.alert`/`ui.message`/`ui.modal`/`ui.notification` 由父工作台统一渲染（复用 `Modal`、`ConfirmDialog` 与 toast），插件页面只传数据，交互一致。
- **特权状态与审计**：`/api/v1/system/privileged/status`、`/audit` 展示特权模式与审计链；前端在触发特权操作前调用 `preflight` 展示标题、描述与来源类型（release/development）。
- **版本与缓存管理**：版本列表、切换、删除、缓存清理界面均基于 `/setup/plugins/<id>/versions`、`/switch`、`/cache` 等接口。
- **下载缓存管理**：Broker 的用户目录下载缓存（全局 1 GiB）可查询与清理。
- **事件驱动刷新**：插件集变化经 `/setup/plugins/events`（SSE）与全局事件流（`topics=plugins`）推送，前端自动刷新列表。

### 发现渠道

插件在 UI、CLI（`alx plugin list`，列出插件及其 Web 入口，不执行插件代码）与 MCP（`alemonjs_list_setup_plugins`）中均可发现。

## 发布、安装与版本管理

系统插件必须通过 GitHub Release 发布平台安装包，不使用源码仓库作为安装包。推荐每个支持的平台发布一个压缩包：

```text
my-plugin-darwin-arm64.zip
my-plugin-darwin-amd64.zip
my-plugin-linux-amd64.zip
my-plugin-linux-arm64.zip
my-plugin-linux-armv7.zip
my-plugin-windows-amd64.zip
my-plugin-windows-arm64.zip
```

压缩包内应包含插件目录、`alx.json`、`web/` 和当前平台的 `dist/` 执行器。Release 同时应发布 `SHA256SUMS`。CI 应从 Git tag 写入 `alx.json.version`，因此 `v1.2.3` Release 的清单版本必须为 `1.2.3`。

ALemonX 安装时会把 Release tag、Asset 名称、压缩包 SHA-256 和安装时间写入插件目录的 `.alx-install.json`，并据此生成安装指纹。已安装插件的实际版本以该文件中的 Release tag 为准；源码目录没有安装指纹时，才使用 `alx.json.version` 作为开发环境回退值。

ALemonX 会在用户配置目录的 `alx/plugin-cache/<plugin-id>/` 中保留已验证的 Release 压缩包和解压版本。默认每个插件最多保留 3 个版本，全局缓存上限为 1 GiB；清理按最近使用时间执行，但不会删除当前活动版本。工作台可通过版本管理界面切换、删除非活动版本或立即执行缓存清理。

限制与超时：安装包最大 300 MiB；安装走长连接传输路径（最长 60 分钟），下载尝试 3 次、每次最多 15 分钟；元数据请求保持短超时。**下载只是把已验证的 Release 放到磁盘，显式启用后才加载**，从下载到启用全程可见、可逆。

## 安全边界与开发规范

托管与执行：

- 面板页只对**已安装且启用的插件**提供；在线识别或未安装的插件返回 404。托管仅 serve `web.root` 内的普通文件，拒绝路径穿越与符号链接逃逸，并设置 `X-Content-Type-Options` 与 CSP（`frame-ancestors 'self'`、`base-uri 'none'` 等）。
- `entry` 必须是插件目录内的普通文件；禁止绝对路径、`..` 越界路径或符号链接。
- runner 环境变量由宿主控制；下载的插件不能通过修改清单索取凭据或网络策略。下载 Broker 不转发工作台 Cookie、Authorization 或内部身份头，也不向插件暴露代理凭据。
- 特权操作只接受宿主签发的意图/计划；浏览器不能选择可执行文件或命令；`password` 模式只执行清单中 PATH 可寻的固定命令，密码用后即清零。

开发规范：

- 将每个操作实现为固定的动作分支；绝不把字段值拼接为 shell 字符串或执行用户提供的命令。
- 危险操作在执行器内校验输入与运行环境；面板页提供二次确认。
- 以最小权限运行；需要提权时明确提示用户，处理取消授权的情况。
- 不读取、上传或在输出中回显凭据、私钥和令牌。
- 为每个发布平台/架构提供对应的二进制，并在干净环境中测试缺少运行时、取消提权和无效输入等失败路径。
- 确保可执行器有执行权限；Windows 文件应以 `.exe` 结尾。
- **不要托管你不信任来源的插件**——安装即信任其执行系统命令与同源面板页。

本仓库开发时至少运行：

```bash
go test ./internal/setupplugin/...
go vet ./internal/setupplugin/...
```

## 参考实现

本仓库当前不随源码分发系统插件示例。开发插件时可按本文的清单、执行器协议和 Release 打包要求建立独立仓库；前端可使用 React + Vite + Tailwind，构建产物输出到 `web/`，本地 `yarn dev` 可通过 Vite 代理指向 alx。
