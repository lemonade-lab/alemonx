# ALemonX · 机器人的本地工作台

> 创建、运行、管理和扩展 AlemonJS 机器人；也能让 AI Agent 在你的确认下协助维护项目。

[下载最新版](https://github.com/lemonade-lab/alemonx/releases) · [命令行](docs/cli.md) · [MCP 文档](docs/mcp.md) · [系统插件开发](docs/plugin-development.md)

![ALemonX 工作台：多机器人项目管理、运行配置与 Agent 协作](docs/images/alemonx-workbench.png)



## 开始使用

从 [GitHub Releases](https://github.com/lemonade-lab/alemonx/releases) 下载对应系统的压缩包并解压：

| 系统 | 下载文件 |
| --- | --- |
| Windows x64 | `alx-windows-amd64.zip` |
| Windows ARM64 | `alx-windows-arm64.zip` |
| Windows 32 位 | `alx-windows-386.zip` |
| macOS Apple Silicon | `alx-darwin-arm64.zip` |
| macOS Intel | `alx-darwin-amd64.zip` |
| Linux x64 | `alx-linux-amd64.zip` |
| Linux ARM64 | `alx-linux-arm64.zip` |
| Linux ARMv7 | `alx-linux-armv7.zip` |
| Linux 32 位 x86 | `alx-linux-386.zip` |
| Linux ppc64le / s390x / riscv64 | 对应的 `alx-linux-<架构>.zip` |
| FreeBSD x64 / ARM64 | 对应的 `alx-freebsd-<架构>.zip` |

Windows x64、macOS Apple Silicon 与 Linux x64 是每个版本的必备安装包。其余架构会尽力构建并在成功时随 Release 发布；若某次未生成对应压缩包，请使用该系统的必备架构包或等待后续版本。

Windows 直接运行 `alx.exe`。macOS / Linux / FreeBSD：

```bash
chmod +x alx
./alx
```

在浏览器打开终端显示的本地地址（默认 `http://127.0.0.1:17390`），即可开始创建、部署或管理机器人。

Windows、macOS 与 Linux 支持在工作台中一键注册后台服务。FreeBSD 提供前台运行与自动更新；请使用系统自身的 rc.d、daemon 或服务管理方案进行常驻托管。FreeBSD 的 Node.js 请使用系统包管理器安装，工作台不会自动下载 Node 运行时。

## 临时 Redis

工作台内置一个纯 Go 的内存版 Redis（`miniredis`），供没有独立 Redis 条件的机器人应用在本地调试时使用。在「设置 → Redis」中可以查看状态、启动/停止服务、修改端口，并配置工作台启动时自动开启。数据只保存在内存中，工作台退出后清空，不建议存放重要数据。

启动时若配置的端口已被占用：如果端口上已经是可用的 Redis，工作台会跳过启动并直接复用该服务；如果是其他程序占用，则会提示错误而不会误用。

命令行也可以控制临时 Redis：

```bash
alx --redis-port 6380     # 调整临时 Redis 端口（会保存到配置）
alx --redis-off           # 禁止启动临时 Redis（设置页可重新启用）
```

工作台默认监听 `0.0.0.0`，`http://服务器IP:17390` 可直接公网访问（历史版本默认只监听 `127.0.0.1`，所以才需要 nginx 反向代理）。请先开启身份认证：

```bash
alx auth enable --account admin --password '你的密码' --confirm-password '你的密码'
alx --port 17390
```

只想本机访问时用 `alx --host 127.0.0.1`；`alx install` 同样支持 `--host`，详见 [命令行文档](docs/cli.md)。

## 操作可控

完整能力范围与权限边界见 [MCP 控制面文档](docs/mcp.md)。

## 本地开发

需要 Go 1.23+、Node.js 22+ 与 Yarn 1.x：

```bash
go run .

cd frontend
yarn install
yarn dev
```

常用校验：

```bash
make test
make build
cd frontend && yarn lint && yarn build
```

## 文档

- [MCP 控制面](docs/mcp.md)
- [命令行](docs/cli.md)
- [系统插件开发](docs/plugin-development.md)
- [插件 WebView 规范](docs/webview.md)
