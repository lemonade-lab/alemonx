# 包管理器命令与参数检查

检查日期：2026-10-22。

## 检查范围

本次检查应用生成的 Node.js 包管理命令，包含参数生成、网络策略追加、内置工具、临时启动和源码插件入口。用户在终端自行输入的命令、插件自行声明的任意参数以及未来 CLI 版本，不能由这一组测试保证兼容。

| 入口 | 检查位置 | 行为 |
| --- | --- | --- |
| 创建项目 | `internal/project/creator.go` | 按所选包管理器安装；临时启动与项目声明使用相同版本。 |
| 连接包、模块、依赖操作 | `internal/robot/local_packages.go`、`manager.go` | add/remove、开发依赖、workspace 根标志。 |
| 依赖同步、框架升级 | `internal/robot/manager.go` | 安装、指定框架包升级；区分 Yarn Classic/Berry。 |
| 构建和版本修改 | `internal/robot/manager.go`、`git_session.go` | run 与业务脚本参数隔离；区分 Yarn 版本命令。 |
| npm 打包、发布、身份检查 | `internal/robot/npm.go`、`local_packages.go` | npm 专用命令；发布/认证不切换到自动镜像。 |
| 内置 Yarn、PM2 | `internal/resources/resources.go`、`robot/manager.go` | `node yarn.js` 与 npx 的参数边界。 |
| Agent 与源码插件 | `internal/agent/command.go`、`system/development_toolchain.go`、`web/plugin_development.go`、`setupplugin/registry.go` | 保留项目根路径、声明版本和可用的内置工具。 |
| 网络策略 | `internal/systemnetwork/package_commands.go`、`commands.go` | 按实际子命令处理 npm/Yarn/pnpm/npx/Corepack，包含 Node 包装入口及 Windows 命令后缀。 |
| 构建脚本 | `Makefile`、`scripts/build-frontend.sh`、`.github/workflows`、`Dockerfile*` | 静态核对固定的 npm ci、Yarn Classic 构建参数；不将这些参数混用于其他包管理器。 |

## 本次修复

- 无明确包管理器声明或已识别锁文件时默认 Yarn；保留现有声明优先、锁文件次之的识别顺序。
- pnpm 的代理、registry 统一通过 `--config.*` 传入。真实 pnpm 9/10 验证发现 `remove --registry=...` 也会报未知参数，不能只修复 `--proxy`。
- npx/npm exec 的 npm 配置放在启动器参数区，子工具参数保持原位，避免把 registry/proxy 传给 lvy、PM2 或业务脚本。
- 不再扫描所有参数中的 `install` 等单词；`run install`、`run build -- install` 不会被当成依赖安装。
- Yarn 2+ 使用其环境配置键，避免 Classic 网络参数、`YARN_REGISTRY` 和 workspace `-W` 导致错误；版本修改使用现代 Yarn 的语法。
- npm 直连使用 `--proxy=false --https-proxy=false`，避免空代理参数产生警告并污染 `pack --json` 输出。
- npm JSON 命令单独读取 stdout，成功时不把诊断 stderr 混入 JSON；失败时仍保留 stderr 错误信息。
- 临时 pnpm 不再使用 `latest`；npx/Corepack 沿用项目声明版本。无版本声明时使用集中定义的固定版本。Yarn 2+ 的 npm 分发包使用 `@yarnpkg/cli-dist`，不误用 `yarn@4`。
- pnpm workspace 根标志同时识别 `pnpm-workspace.yaml`，不只读取 `package.json.workspaces`。
- 私有 registry/认证检查补充现代 Yarn 和 pnpm 配置文件，避免自动镜像选择忽略这些配置。

## 可重复验证

```sh
go test ./internal/...
go vet ./internal/...
node scripts/test-package-managers.mjs
```

真实 CLI 检查使用临时项目和本地依赖压缩包，不安装全局工具，不修改实际机器人。工具从 npm 官方 registry 下载并校验发布元数据中的 integrity，结束后清理临时目录。该检查已接入发布 CI 的 verify 阶段。

| CLI | 真实命令验证 |
| --- | --- |
| Yarn 1.22.22 | 安装、普通/开发依赖添加与卸载、workspace 根、脚本运行、版本修改。 |
| Yarn 4.9.2 | 同上，并验证不传 Classic 参数。 |
| pnpm 9.15.0、10.17.1 | 同上，并验证卸载接受配置形式的 registry/proxy 参数。 |
| npm 10.9.8、11.6.2 | 同上，加 npm pack JSON、npm exec/npx 子进程参数。 |

单元测试另覆盖直连/代理、默认包管理器、声明版本优先、Node/Corepack/npx 包装、Windows 后缀、脚本参数边界和 pnpm workspace YAML。

真实 CLI 检查不发布包、不连接真实账号，也不等价于 Windows 上的实际运行或所有网络条件的端到端验证。Windows 另执行测试二进制交叉编译。

## 参数依据

- [pnpm CLI：未知参数与 --config.](https://pnpm.io/pnpm-cli)
- [npx：启动器与子命令的参数边界](https://docs.npmjs.com/cli/v11/commands/npx/)
- [Yarn 现代配置与环境变量](https://yarnpkg.com/configuration/yarnrc)
- [Yarn add](https://yarnpkg.com/cli/add)、[Yarn version](https://yarnpkg.com/cli/version)
