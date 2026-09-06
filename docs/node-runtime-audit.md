# NodeJS 运行环境审计

状态：进行中。本文只记录代码事实、链路和审计结论；不代表已完成重构。

## 目标不变量

`node --version` 是 ALemonX 唯一的当前版本事实来源。若该命令由
`<node-bin>/node` 执行，则全部 Node 相关子进程必须继承同一个
`PATH=<node-bin>:<其余 PATH>`。NVM 仅负责选择并应用这个 PATH，不能成为
独立的执行版本来源。

## 入口统计

本次搜索到 12 个后端执行/环境入口、3 个运行时根入口和 2 个展示入口：

| 分类 | 入口 | 结论 |
| --- | --- | --- |
| 运行时根 | `Dockerfile`、`scripts/docker-entrypoint.sh`、`internal/web/server.go` | 决定初始 PATH 与服务启动恢复。 |
| 版本选择 | `internal/system/node_runtime.go` | NVM 安装、切换、启动恢复。 |
| 状态/解析 | `internal/system/checker.go` | 环境检查、Node/npm/npx 解析、PATH 刷新。 |
| 机器人包管理 | `internal/robot/manager.go`、`internal/robot/npm.go`、`internal/robot/git_session.go` | Yarn/npm/pnpm/npx、构建、PM2。 |
| 项目创建 | `internal/project/creator.go` | 创建项目后的依赖安装。 |
| Agent 命令 | `internal/agent/command.go` | 白名单 Yarn/npm/pnpm/node 命令。 |
| 插件开发 | `internal/system/development_toolchain.go`、`internal/web/plugin_development.go`、`internal/setupplugin/registry.go` | Yarn/pnpm/Corepack/npx 与源码插件执行。 |
| 内置工具 | `internal/resources/resources.go` | 内置 Yarn 的 `node <yarn.js>` 调用。 |
| 应用页 | `internal/web/server.go` | 机器人应用页 Node IPC。 |
| 交互终端 | `internal/web/robot_terminal_session.go` | Shell 启动参数与继承环境。 |
| API/UI | `internal/web/server.go`、`frontend/src/components/NodeNVMPanel.tsx` | NVM API 与当前版本展示。 |

## 环境变更树

```text
Dockerfile / 系统服务 / 直接启动
  └─ 初始后端 PATH
      └─ server.newServerRuntimeWithAuth
          ├─ ActivateNVMDefaultForProcess()       [唯一允许的启动恢复]
          └─ RefreshCommandEnvironment(git,docker) [会改 PATH，需隔离]
              └─ NodeRuntime（应新增的唯一上下文）
                  ├─ node --version                [唯一版本事实]
                  ├─ node bin + 子进程 PATH        [唯一执行环境]
                  ├─ NVMStatus / 环境检查 / UI
                  ├─ NVM 安装与切换
                  ├─ Yarn/npm/pnpm/npx/核心包管理
                  ├─ 项目创建、Git 构建、PM2、机器人运行
                  ├─ Agent 命令
                  ├─ 插件源码构建、系统插件 Runner
                  ├─ 内置 Yarn
                  ├─ 机器人应用页 IPC
                  └─ 工作台终端
```

## 从树根开始的核验

### 1. 初始后端 PATH

- `Dockerfile` 为运行镜像设置 `/usr/local/bin` 在 `/usr/bin` 之前，且构建期
  断言 Node 是 `v22.22.3`。这只能保证镜像初始状态正确。
- `docker-entrypoint.sh` 不改 Node PATH，符合预期。
- 服务启动时 `newServerRuntimeWithAuth` 调用
  `ActivateNVMDefaultForProcess()`，可将已选择的 NVM 版本放到后端 PATH 首位。
  这是正确的恢复入口。
- 同一启动函数随后调用 `RefreshCommandEnvironment("git", "docker")`。该函数会
  重写整个 PATH；即便此处没有显式选择 Node，也不应由通用刷新器承担 Node
  环境管理。这是树根的风险点。

结论：**部分通过**。初始 PATH 与 NVM 恢复有明确入口，但通用 PATH 刷新器仍可
改变后端环境。

### 2. 当前版本读取

- `NVMStatus()` 列出版本，但 `ActiveVersion` 最终由 `systemNodeVersion()` 执行
  实际的 `node --version` 得出。
- 环境检查的 Node 行直接使用当前 PATH 的 `exec.LookPath("node")`，不再以 NVM
  默认 alias 覆盖状态。

结论：**通过**。展示层的当前版本遵守唯一事实来源。

### 3. NVM 切换

- `UseNVMNodeVersion()` 调用 NVM 的 `alias default` 与 `use`，再将目标 `bin`
  放到后端 PATH 首位。
- 切换后又调用 `RefreshCommandEnvironment("node", "npm", "npx")`。它与前一步
  作用重叠，且把“选择 Node”与“通用命令修复”混合。
- Docker 已重新允许 NVM；容器重启会通过启动恢复入口重新应用已保存的默认版本。

结论：**部分通过**。切换能变更后端 PATH，但应由单一 `ApplyNodeRuntime` 完成，
并立即以 `node --version` 验收，不应再经过通用刷新器。

### 4. 解析与 PATH 刷新

- `ResolveCommand(node|npm|npx)` 当前优先读取后端 PATH，符合目标。
- 其他命令仍可经 `nvmNodeCommand`、`ManagedNodeCommand`、系统目录兜底解析。
- `RefreshCommandEnvironment` 可以重排整个后端 PATH；机器人、Agent、项目创建器
  都会调用它。

结论：**不通过**。Node 运行时不应由任何通用刷新入口改变；NVM 目录与旧的
`ManagedNode` 机制只能用于迁移/诊断，不能参与当前执行路径。

## 分支检查台账

| 分支 | 当前链路 | 风险 | 审计状态 |
| --- | --- | --- | --- |
| 机器人包管理 | `nodeToolPath` → `applyManagedNodeEnvironment` → Yarn/npm/pnpm/npx | 两个 PATH 注入函数叠加，且先调用通用刷新器。 | 待收敛 |
| 项目创建 | `projectCommandPath` → `ResolveCommand` → 手工 PATH | 同样调用通用刷新器。 | 待收敛 |
| Agent 命令 | 仅 node/npm 用 `ResolveCommand`；Yarn/PNPM 保留裸命令 | Yarn/PNPM 可不在同一 NodeRuntime 下执行。 | 不通过 |
| 插件开发 | `PrepareDevelopmentCommand` 合并用户 Yarn/Volta/asdf/fnm 目录 | 服务 PATH 虽在前，但仍存在第二套命令发现策略。 | 待收敛 |
| 系统插件 Runner | 继承 `PrepareDevelopmentCommand` 环境 | 同插件开发。 | 待收敛 |
| 内置 Yarn | `node <embedded yarn.js>` | 调用者多数注入 PATH，但资源层本身不携带 NodeRuntime。 | 待收敛 |
| 应用页 IPC | `ResolveCommand(node)` + Node bin PATH | 当前符合目标，但应改为共享 NodeRuntime API。 | 部分通过 |
| 交互终端 | 从后端 `os.Environ()` 继承；Docker 禁止 source profile | 已能继承 NVM 切换后的后端 PATH。外部终端无法被后端进程直接修改。 | 部分通过 |
| Docker | 镜像初始 PATH → 后端 PATH → NVM 可覆盖 | 初始镜像版本不是独立真相；必须仍以后端 `node --version` 为准。 | 部分通过 |

## 重构顺序

1. 新建 `NodeRuntime`：一次性解析当前 Node 路径、执行 `node --version`、生成子进程环境。
2. 新建唯一变更函数 `ApplyNodeRuntime(bin)`：仅由 NVM 切换、NVM 安装成功和服务启动恢复调用；
   应用后必须执行 `node --version` 验证。
3. 禁止 `RefreshCommandEnvironment` 接受 Node/npm/npx，移除任务执行路径中的调用。
4. 所有执行分支改为消费 `NodeRuntime`，尤其是 Agent、插件开发、内置 Yarn 与机器人包管理。
5. 终端从同一个 `NodeRuntime` 环境启动；Docker 只提供初始 PATH，不提供另一套解析规则。
6. 增加端到端矩阵：切换前后分别断言 API、Yarn、npm、npx、内置 Yarn、插件构建、Agent、
   应用页和工作台终端的 `node --version` 一致。

## 分支逐项核验结果

### 5. 机器人包管理、构建与 PM2

路径：`PackageManagerCommand` → `packageManagerCommand` → `runWithOutput`。

- npm/npx 可由 `ResolveCommand` 找到当前 PATH 中的绝对路径。
- Yarn/PNPM 会先执行裸程序名，随后以 `applyManagedNodeEnvironment` 注入 Node bin；
  内置 Yarn 也依赖这一步。
- `nodeToolPath` 在每次任务前调用 `RefreshCommandEnvironment`，这使“执行任务”意外
  成为后端 PATH 的写入者。
- `applyManagedNodeEnvironment`、`packageManagerEnvironment` 和
  `applyNodeSiblingEnvironment` 有三份近似的 PATH 拼接代码。

结论：**高风险，未通过**。该分支应改为只接受 `NodeRuntime`，由它一次生成命令路径与
`cmd.Env`；任务本身不得改写服务 PATH。

### 6. 项目创建

路径：`creator.Create` → `run` → `projectCommandPath`。

- 创建依赖安装会在启动命令前手工把 `ResolveCommand("node")` 的目录加入 PATH，方向正确。
- `projectCommandPath` 仍对 node/npm/npx 调用通用 PATH 刷新器。
- Yarn/PNPM 的可用性检测可走裸命令或内置 Yarn，未共享机器人分支的统一执行器。

结论：**中高风险，未通过**。项目创建不应维护自己的 Node PATH 规则，应复用包管理执行器。

### 7. Agent 命令

路径：`agent.commandRunner.Run`。

- node/npm 解析绝对路径后仍调用 PATH 刷新器。
- Yarn 与 PNPM 没有解析到当前 NodeRuntime，直接作为裸程序执行。
- 子进程也没有明确 `cmd.Env`，完全依赖当时服务进程环境。

结论：**高风险，未通过**。这是目前最直接能让 Agent 的 Yarn/PNPM 与页面显示版本分叉的入口。

### 8. 内置 Yarn 与按需工具安装

路径：`resources.embeddedYarnCommand` → `resources.provisionRunner`。

- 内置 Yarn 返回裸 `node` 加 Yarn JS 文件。
- `provisionRunner` 直接执行该裸 `node`，没有接收或应用 NodeRuntime 环境。
- 机器人调用内置 Yarn 时会额外注入 PATH，但 PM2 等按需资源安装并不保证经过机器人分支。

结论：**高风险，未通过**。资源包必须显式接收当前 NodeRuntime 的绝对 Node 路径和环境。

### 9. 插件开发与系统插件 Runner

路径：`PrepareDevelopmentCommand` → `DevelopmentCommandEnvironment` → 插件构建/运行。

- 开发环境把后端现有 PATH 放在最前，通常可继承当前 Node。
- 之后追加了 `.yarn`、Volta、asdf、fnm、系统目录等候选路径；这是另一套命令发现模型。
- 插件构建、前台开发进程、系统插件 Runner 都使用该模型。

结论：**中风险，待收敛**。保留开发工具查找可以，但 Node、npm、npx、Yarn、PNPM、Corepack
必须先由 NodeRuntime 固定，不能由用户目录重新选择 Node。

### 10. 机器人应用页 IPC

路径：`server.botAppPageRuntime` → `ResolveCommand("node")` → `exec.Command(node, script)`。

- 使用 Node 的绝对路径，并把其 bin 放入子进程 PATH。

结论：**低风险，部分通过**。语义正确，但应复用 NodeRuntime，移除手工拼 PATH。

### 11. 工作台终端

路径：`terminalSessionsHandler` → `terminalEnvironment(os.Environ())` → PTY shell。

- 当前终端继承后端的完整环境；Docker Bash/Zsh 不读取 profile，避免挂载的 shell 配置覆盖
  工作台已应用的 NVM PATH。
- 非 Docker 交互 shell 仍会读取用户 profile。由于切换同时设置 NVM default，通常会一致；
  但任意自定义 profile 仍可覆盖 PATH。

结论：**中风险，部分通过**。工作台终端应以 NodeRuntime 环境启动；是否读取 profile 是独立的
终端体验选择，不能承担 Node 版本切换。

### 12. API/UI 与 Docker

- API 的 GET 由 `NVMStatus` 返回下载列表和实际 `node --version`；POST 调用 NVM 安装/切换。
- Docker 已允许 NVM；Dockerfile 中的 Node 版本只是无 NVM 选择时的初始版本。
- 前端只消费 API，不应自行推导当前 Node。

结论：**低风险，部分通过**。API 的状态读取正确；需要在切换接口返回前执行实际
`node --version` 验证，并将该值作为响应的当前版本。

## 统计

| 审计结果 | 数量 | 分支 |
| --- | ---: | --- |
| 通过/低风险 | 2 | 当前版本读取、应用页 IPC |
| 部分通过 | 4 | 启动恢复、NVM 切换、终端、Docker/API |
| 中风险待收敛 | 2 | 项目创建、插件开发/系统插件 |
| 高风险未通过 | 3 | 机器人包管理、Agent 命令、内置 Yarn/按需工具 |
| 基础风险源 | 1 | 通用 `RefreshCommandEnvironment` |

下一次实现应从基础风险源开始：先阻止通用刷新器更改 Node PATH，再引入 NodeRuntime，之后按照
本节编号替换每个执行分支。这样每替换一个分支都可单独验证，不会再次制造两套当前版本。
