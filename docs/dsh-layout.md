# DSH 工作区目录与迁移

DSH 程序、机器人运行数据和事件记录统一存放在所选工作区：

```text
workspace/
  dsh/
    packages/<系统-架构>/<程序包 SHA-256>/
      node_modules/@deepseek-ai/dsh/lib/bin.js
      plugins/approval-bridge/
      plugins/workspace-entry/
    runtimes/<存储 ID>/
      runtime.json
      sessions.json
      alemonx.patch.yml
      alemonx.web.patch.yml
      profiles/alemonx/
      profiles/web/
      node_modules/@alemonx/dsh-approval-bridge/
      .runtime.lock
    events/
  bots/
  templates/
  packages/
  plugins/
```

工作区由 `--workspace`、`ALX_WORKSPACE`、`ALEMONJS_SETUP_ROOTS` 或默认工作区规则确定。临时目录不再作为持久 DSH 数据的回退位置。原有 API Key 保留在系统钥匙串或只读 Secret 中；官方 Web 版的新增凭据由 DSH 自身管理，详见下方说明。

## Web 版入口

机器人侧栏的 DSH 页面提供“打开 Web 版”。工作台启动随安装包交付的官方 `dsh web --host 127.0.0.1 --port 0 --no-open`，再通过内置浏览器 WebView 的 HTTP 代理访问。远程部署时连接的是工作台主机上的 DSH，而非用户浏览器本机的端口。对话、模型、设置、工作区、插件和审批由官方界面负责，ALemonX 不再维护自定义聊天界面。

打开时自动注册或复用当前机器人目录对应的官方工作区，并选中其中最近的未归档会话；没有会话时复用或创建空白会话，无需再次选择目录。每次从 ALemonX 打开都以当前机器人目录为默认入口；进入后仍可使用官方工作区切换功能，网络重连不会强制切回。目录身份由 DSH 规范化，支持 Windows 路径与符号链接。

每个机器人沿用原有受管 `DSH_HOME`，官方 Web profile 放在 `profiles/web/`。SDK 与 Web 进程不会同时写入该目录；打开 Web 版会停止该项目的 SDK 进程。旧 `profiles/alemonx/`、`runtime.json`、`sessions.json` 和事件记录保留，不删除或改写；官方页面按 DSH 自身的持久化记录读取历史，不将 ALemonX 会话索引当成 Web 协议。

端口由 DSH 自动分配，仅绑定回环地址；启动认证链接只保留在进程内存和当前浏览器窗口，不保存到配置或日志。重复打开复用已运行的服务，启动失败可以重试，退出工作台或迁移旧项目时停止受管 Web 进程。关闭浏览器标签保留后台服务和会话。

已有系统钥匙串或 Docker Secret 凭据作为启动环境提供给官方 Web 版，官方界面将这些凭据显示为只读。首次使用可在官方模型设置中配置凭据；由官方管理的凭据可能写入 `DSH_HOME/.credentials.yaml`，备份该目录时按敏感数据处理。原 SDK 审批和网络桥仅保留给兼容接口，官方 Web 使用自己的审批与网络配置。

## 程序交付

`make dev` 使用 `alxdev` 开发构建：先启动工作台，再异步执行 DSH 的 npm 安装、校验、打包与工作区释放。首次开发启动不要求 `runtime.zip` 已存在；准备完成后无需重启即可连接。日志位于 `resources/dsh/prepare.log`，失败不会退出工作台，可通过 `make dsh-runtime` 重试。刷新期间已有的完整程序继续可用。

正式 `make build`、CI 和 Docker 构建仍等待打包成功；发布程序不在用户启动时运行 npm。

源代码与锁文件位于 `resources/packages/dsh/`。`make dsh-runtime` 安装并验证 SDK 兼容接口及官方 Web 启动、认证和静态资源，然后通过 `scripts/pack-dsh.go` 生成 `resources/dsh/runtime.zip`。该归档作为资源嵌入 `alx`；没有归档的构建会失败，避免发布缺少 DSH 的程序。

首次启动 DSH 时，程序包先在工作区临时目录解压并校验必要文件，再发布到内容指纹目录。程序与数据分开：更新 `alx` 后释放新的程序目录，回滚后使用旧二进制内的程序包；不会覆盖 SDK profile、会话或机器人代码。旧程序版本暂不自动清理，以支持回滚。

完成标记不等于程序始终完整：若入口、官方 Web 首页或审批桥接文件缺失，会从程序包自动重建。替换前先完成解压和校验，旧程序目录保留在同平台目录的 `.damaged-*/` 下；修复不操作 `runtimes/` 或 `events/`，多实例发布通过文件锁串行执行。

一行安装器和 `alx update` 继续交付单个可执行文件，其中已包含 DSH，因此不会遗漏外部 sidecar。发布流水线按目标系统和 CPU 安装 DSH 的原生依赖，运行时拒绝平台不匹配的程序包。

DSH 使用工作台当前 Node 的绝对路径启动 JavaScript 入口，不依赖用户全局 `dsh` 或 npm 的平台相关 `.bin` 启动脚本。`ALX_DSH_BIN` 仅保留给开发和测试中的显式启动覆盖。

## 旧数据迁移

升级前停止旧工作台，尤其避免新旧版本同时写入相同机器人会话。

旧用户配置目录中的 `alemonjs/dsh/runtimes/` 会复制到工作区 `dsh/runtimes/`。复制通过临时目录完成，目标已有数据时不覆盖；旧目录完整保留。旧审批桥接链接不作为用户数据复制，由新版本重建为托管模块，不再需要 Windows 符号链接权限。

旧 Agent 目录中的 `dsh-events` 在读取对应机器人事件时迁入工作区，原文件保留。备份应包含整个 `workspace/dsh/`；跨主机恢复还需要重新配置系统钥匙串凭据。仅复制 `bots/` 不包含 DSH 会话。

迁移跳过 SDK 自动生成的 `profiles/node_modules`、各 profile 的 `.dsh-module-fallback` 及指向它的托管链接，由 SDK 按当前安装位置重建。profile 配置、会话和用户插件仍保留；其他未知越界链接继续报错，不跟随复制。旧目录不作删除或修改。

工作区程序目录可重新生成，`runtimes/` 和 `events/` 是持久数据，不能作为缓存清理。运行中的数据目录具有 OS 文件锁，第二个工作台实例会明确报告占用；进程退出后锁自动释放。

## 机器人目录搬迁

搬迁后使用管理接口显式更新关联，不通过猜测路径自动接管其他项目。要求当前管理员权限、有效的新机器人目录及明确确认：

```text
POST /api/v1/dsh/relocate
Content-Type: application/json

{"oldRoot":"/原机器人绝对路径","newRoot":"/新机器人绝对路径","confirm":true}
```

此操作停止原 DSH 进程，保留存储 ID 和会话数据，备份原配置并更新项目路径，复制凭据关联和事件记录。目标已有 DSH 配置或不同凭据时拒绝覆盖。完成后在新机器人目录重新连接。

迁移不会移动机器人文件。更换工作区根目录时，应先停止工作台并完整复制 `dsh/` 数据；机器人绝对路径改变后，再调用此接口更新关联。旧配置备份和原凭据保留以便恢复。

## 验证官方 Web 访问

`make dsh-runtime` 包含官方 Web 启动、认证跳转和静态资源检查，不调用模型。可额外运行真实的受管启动与浏览器代理测试；浏览器检查需要先安装前端依赖，并能启动 Chrome：

```sh
ALX_DSH_WEB_SMOKE=1 ALX_DSH_WEB_BROWSER_SMOKE=1 go test ./internal/web -run TestDSHOfficialWebThroughBrowserProxy -count=1
```

该测试使用临时工作区，覆盖官方页面插件加载、实时连接、首次打开的默认目录、并发打开、刷新复用、最近未归档会话恢复以及切换项目后的目录和会话隔离，不读取用户已有 DSH 数据或发送模型请求。
