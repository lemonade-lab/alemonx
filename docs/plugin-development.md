# ALemonX 系统插件开发与接入

系统插件用于为 ALemonX（`alx`）增加**全局的本机管理能力**，例如网络检查、系统服务或防火墙规则。它与机器人项目无关；不要用它实现某个机器人的命令、配置页或 WebView。这些能力应作为机器人插件提供。

插件的**界面就是它的 Web 界面**（`web.root` 静态前端），由 ALemonX 同源托管；系统操作通过一个**通用动作转发**接口执行。清单不再声明 `pages`/`actions`/`fields`——那些「多余配置」已移除，插件需要复杂交互时直接在自己页面里实现。发现阶段永不执行插件代码。

## 快速开始

创建如下目录：

```text
plugins/
  my-status/
    alx.json
    web/
      index.html
    runner/
      main.mjs
```

在开发仓库中，把它放到仓库根目录的 `plugins/` 下。运行中的 ALemonX 也会依次从以下位置发现插件：

1. 可执行文件同级的 `plugins/`；
2. 当前工作目录的 `plugins/`；
3. 用户配置目录的 `alx/plugins/`。

同一个插件 `id` 只会加载第一个发现的位置。插件目录的增删与 `alx.json` 修改由后台**自动热更新**（约 1 秒内反映），无需重启或手动刷新。

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

## `alx.json` 清单

清单文件名必须精确为 `alx.json`，最大 64 KiB，且不能是符号链接。`id` 必须匹配 `^[a-z][a-z0-9-]{1,63}$`。

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id` | 是 | 稳定的插件标识，建议发布后不再变更。 |
| `name`、`version` | 是 | 管理台显示的名称与版本。 |
| `description` | 否 | 插件说明。 |
| `platforms` | 否 | 支持的 Go 平台名；未填写表示全部平台。 |
| `navigation` | 否 | 全局功能栏入口；`label` 默认取 `name`，`icon` 默认 `◈`，`order` 越小越靠前。 |
| `runtime` | 否 | `binary`、`node` 或 `go`；省略时为 `binary`。 |
| `entry` | 是* | 执行器映射。键为 `GOOS-GOARCH`（例如 `linux-amd64`），也可仅用 `GOOS` 作为回退。 |
| `development` | 否 | 源码开发声明：可提供开发 runner、前端构建/开发服务和受管本地服务。 |
| `web` | **是** | 插件的 Web 界面目录（如 `web`），不能是绝对路径或含 `..`。无 `web` 的插件不可用。 |
| `privilegedOperations` | 否 | 插件声明的系统权限操作：`action`、`runnerAction`、`authorization`（`password` / `native`）、平台，以及可选 `planAction` / `useLatestAudit`。 |

`entry` 路径必须是插件目录内的普通文件，不能使用绝对路径、`..` 越界路径或符号链接。`node` 会以 `node <entry>` 启动；`binary` 直接执行该文件；`go` 只读取 `entry.go`，并以 `go run <entry.go>` 启动；`python` 读取 `entry.python` 并以 `python3 <entry>` 启动。源码 `development.runtime` 还可使用 `command`，并通过结构化 `command.program` / `command.args` 启动任意本机语言工具链。纯静态插件只需声明 `web`，不能调用 actions。

## 源码开发会话

插件中心的“开发插件”会打开工作台内置 Finder，选择含 `alx.json` 的源码目录。选择后仅登记目录；点击“启动开发”才会执行其中声明的本地命令。启动中的源码会话会临时覆盖同 ID 的正式 Release，停止后立即恢复 Release；工作台重启后不会自动执行源码命令。

开发配置放在 `development` 中。命令使用 `program` 与 `args`，主机不经过 shell；`${ALX_PLUGIN_DEV_PORT}` 是唯一可替换变量，会被替换为主机分配的回环端口：

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
    }
  }
}
```

`web.mode` 可为 `static` 或 `dev-server`。静态模式优先托管 `development.web.root`（未声明时回退到顶层 `web.root`）；开发服务模式由主机启动并通过工作台同源代理访问。只有 `hmr: true` 时，代理才允许 HMR WebSocket，不会暴露额外网络端口。`development.services` 可为现有 `services` 中已声明的 loopback 服务附加受管命令与 `never` / `on-failure` 重启策略。

源码会话具有 `registered`、`starting`、`running`、`building`、`stopping`、`stopped`、`failed` 状态。同一插件同一时间只允许一个启动、停止、重启或构建操作；停止必须确认受管进程组退出后才会恢复正式 Release。

源码开发会话只能在本机桌面工作台中启动。源码插件也可以请求其 `privilegedOperations`；工作台会明确标示“源码开发授权”和源码目录，并仍要求管理员单次确认与当前主机权限模式。

## 宿主内置能力

系统插件拥有自己的业务、进程、文件和下载逻辑。浏览器页面需要工作台桌面能力或上下文时，通过版本化能力目录和对应 API 请求；不再声明 `hostCapabilities`：

```json
{
  "systemPickers": [
    { "id": "runtime-directory", "kind": "directory", "title": "选择运行目录" }
  ]
}
```

| 能力 | API | 返回范围 |
| --- | --- | --- |
| 目录 | `GET /api/v1/system/capabilities` | 返回版本、可用性与不可用原因。 |
| `finder.pick` | `POST /api/v1/system/capabilities/finder` | 仅由清单中 `pickerId` 定义的工作台 Finder 文件或目录选择结果。 |
| `context.current-robot` / `context.network-settings` | `GET /api/v1/system/capabilities/context?pluginId=<id>&keys=robot,network` | 当前已验证机器人与脱敏网络配置。 |
| `desktop.open`、`clipboard.read/write`、`notification.send`、`system.info` | `/api/v1/system/capabilities/{desktop/open|clipboard|notification|info}` | 受认证、绑定插件身份的桌面基础能力。 |
| `network.fetch` | `POST /api/v1/system/capabilities/network/fetch` | 使用主应用代理、镜像和重试设置的 GET/HEAD 请求。 |

Finder 请求只传 `{ "pluginId": "...", "pickerId": "..." }`。宿主从清单取得窗口标题、类型和多选策略，并在父工作台中打开统一的 Web Finder；插件不能提交路径、命令、端口或原生脚本。旧 `/api/v1/system/picker` 暂时兼容已发布插件，新插件应使用 capability API。

`network-context` 仅供界面展示脱敏状态，不能读取密码。runner 的下载 Broker 支持任意 HTTP(S) URL、重定向、一次重试和用户目录缓存；它不转发工作台 Cookie、Authorization 或内部身份头，也不会向插件暴露代理凭据。

每个插件页面会自动获得无依赖的 `window.ALXHost` 小型 SDK。它只是对上述能力 API 的便捷封装，宿主仍会校验插件身份和当前登录会话：

```js
const paths = await window.ALXHost.finder.pick('my-status', 'runtime-directory')
await window.ALXHost.desktop.open('my-status', paths[0])
const { robot } = await window.ALXHost.context('my-status', ['robot'])
await window.ALXHost.notification.send('my-status', '已完成', robot?.name || '未选择机器人')
```

`ALXHost.network.fetch` 仅提供 GET/HEAD 和最多 2 MiB 的响应，用于状态或元数据；大文件必须由插件 runner 通过下载 Broker 获取，以获得流式进度、取消、重试和缓存。

需要接收浏览器文件的插件应在清单中声明上传动作，再由 Web UI 以 `multipart/form-data` 向 `POST /api/v1/setup/plugins/<pluginId>/upload` 发送 `action`、`destination` 与一个或多个 `files` 字段。宿主按清单 `maxBytes` 限制总大小、临时流式暂存文件，并只把暂存目录与目标目录传给对应 runner 动作；暂存内容会在动作结束后清理。示例：

```json
"uploads": [{ "action": "upload", "maxBytes": 2147483648 }]
```

## 静态 Web 界面（必选）

插件详情页会直接嵌入 `web.root/index.html`（同源 iframe）。界面用 `fetch` 调用本插件的**动作转发接口**：

```http
POST /api/v1/setup/plugins/<插件id>/actions
Content-Type: application/json

{ "action": "network-check", "confirm": false, "params": {} }
```

返回任务信息后，轮询 `/api/v1/robot/tasks` 获取结果（`status`：`running`/`completed`/`failed`，结果在 `output`，失败原因在 `error`）。

- **`action` 名称由执行器自行白名单**：alx 不校验清单，直接把请求转发给执行器，执行器内部 `switch` 只处理自己支持的动作并校验每个参数。
- **危险操作的二次确认由 Web 界面负责**（例如自绘确认弹窗）。安装插件即等于授予其执行系统命令的能力（「安装即信任」），因此不再有清单级 confirm 声明。
- 结果文本若每行以 `✓ `、`! ` 或 `? ` 开头（符号后一个空格），可按状态着色；缩进行按普通文本显示。
- 完整可参考的界面见 [alemonx-network/frontend](../plugins/alemonx-network/frontend/)（React + Vite + Tailwind，复用 alx 的设计 token，构建产物为 `web/`）。

安全边界：Web 界面**只对已安装且启用的插件**提供；在线识别或未安装的插件返回 404。托管仅 serve `web.root` 内的普通文件，拒绝路径穿越与符号链接逃逸，并设置 `X-Content-Type-Options` 与 CSP。

插件前端建议用 `frontend/` 工程（React + Vite + Tailwind，视觉对齐 alx），构建产物输出到 `web.root` 目录。参考 `alemonx-network/frontend`：`yarn install && yarn build` 生成 `web/`；开发时 `yarn dev`（Vite 代理可指向本地 alx）。`web/` 是构建产物，不提交仓库，发布 zip 由 CI 构建。

## 执行器协议

每次操作均会启动一个独立进程。工作目录为插件目录；ALemonX 会向标准输入写入一个 JSON 对象，并从标准输出读取**唯一的 JSON 响应**。不要在标准输出打印日志或调试文本；请输出到标准错误。

请求：

```json
{
  "protocol": "alx/v1",
  "method": "run",
  "action": "network-check",
  "params": {}
}
```

响应（`data` 可选，供 Web UI 使用结构化结果；旧插件只返回 `output` 仍兼容）：

```json
{ "output": "✓ 已检查 3 个网卡。", "data": { "interfaces": [] } }
```

操作失败时仍应正常输出 JSON，并设置 `error`：

```json
{ "output": "已检查现有规则。", "error": "需要管理员权限" }
```

进程非零退出、没有有效 JSON 响应或 `error` 非空都会将该任务标记为失败。成功但未提供 `output` 时，显示“插件操作已完成。”

Node.js 最小执行器：

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

## 安全与发布清单

## Release 安装与实际版本

系统插件必须通过 GitHub Release 发布平台安装包，不使用源码仓库作为安装包。推荐每个支持的平台发布一个压缩包，例如：

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

版本管理接口为 `GET /api/v1/setup/plugins/<id>/versions`、`POST /api/v1/setup/plugins/<id>/switch`、`DELETE /api/v1/setup/plugins/<id>/versions/<tag>`，全局缓存接口为 `GET/POST /api/v1/setup/plugins/cache`。

- 将每个操作实现为固定的动作分支；绝不把字段值拼接为 shell 字符串或执行用户提供的命令。
- 危险操作在执行器内校验输入与运行环境；Web 界面提供二次确认。
- 以最小权限运行；需要提权时明确提示用户，处理取消授权的情况。
- 不读取、上传或在输出中回显凭据、私钥和令牌。
- 为每个发布平台/架构提供对应的二进制，并在干净环境中测试缺少运行时、取消提权和无效输入等失败路径。
- 确保可执行器有执行权限；Windows 文件应以 `.exe` 结尾。
- **不要托管你不信任来源的插件**——安装即信任其执行系统命令与同源 Web 界面。

在本仓库开发时，至少运行：

```bash
go test ./internal/setupplugin/...
go vet ./internal/setupplugin/...
```

插件在 UI、CLI（`alx plugin list`）与 MCP（`alemonjs_list_setup_plugins`，列出插件及其 Web 入口）中均可发现。详见 [MCP 控制面文档](mcp.md)。
