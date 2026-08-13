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

Windows 直接运行 `alx.exe`。macOS / Linux / FreeBSD：

```bash
chmod +x alx
./alx
```

在浏览器打开终端显示的本地地址（默认 `http://127.0.0.1:17390`），即可开始创建、部署或管理机器人。

Windows、macOS 与 Linux 支持在工作台中一键注册后台服务。FreeBSD 提供前台运行与自动更新；请使用系统自身的 rc.d、daemon 或服务管理方案进行常驻托管。FreeBSD 的 Node.js 请使用系统包管理器安装，工作台不会自动下载 Node 运行时。

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
