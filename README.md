# ALemonX · 机器人的本地工作台

> 在一个本地工作台中创建、运行、管理和扩展 AlemonJS 机器人；也可以让 AI Agent 在你的确认下协助维护项目。

[一行安装](#一行安装) · [Docker 部署](docs/docker.md) · [命令行](docs/cli.md) · [MCP 文档](docs/mcp.md) · [系统插件开发](docs/plugin-development.md)

![ALemonX 工作台：多机器人项目管理、运行配置与 Agent 协作](docs/images/alemonx-workbench.png)

## 目录布局

ALemonX 使用单一运行期工作区收敛所有用户数据，前端构建产物与运行期资源只作为内嵌文件存在：

```text
dist/                     前端构建产物（构建时嵌入 alx 二进制，不参与运行期布局）
resources/                运行期资源（构建时嵌入 alx 二进制）
├── templates/            项目模板源（bot/、dev/）
└── packages/             内置工具包（Yarn 嵌入；PM2 等按需用 Yarn 安装）
workspace/                统一工作区（默认 <运行目录>/workspace，可用 --workspace 或 ALX_WORKSPACE 指定）
├── templates/            项目模板（首次启动从内嵌模板物化，可编辑）
├── packages/             工具目录（Yarn 物化副本；PM2 首次使用安装到这里，位置固定）
└── bots/                 新建机器人的默认落点
```

模板与工具包文件只会在缺失时复制；已存在文件不会被覆盖，因此可以安全地自定义模板。Yarn 由构建流程安装并嵌入二进制，创建项目与安装依赖从不依赖 npm 拉取；PM2 不随包嵌入，首次需要时用内置 Yarn 安装到 `<workspace>/packages/pm2`（位置固定、可复用），不会依赖 npx 或临时缓存。项目本地已安装 PM2 时，优先使用项目内的版本。

## 一行安装

macOS、Linux 或 FreeBSD 在终端执行：

```sh
curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh
```

Windows 在 PowerShell 执行：

```powershell
irm https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

### 手动安装

如需手动下载，可从 [GitHub Releases](https://github.com/lemonade-lab/alemonx/releases) 选择对应系统的压缩包：

| 系统                            | 下载文件                        |
| ------------------------------- | ------------------------------- |
| Windows x64                     | `alx-windows-amd64.zip`         |
| Windows ARM64                   | `alx-windows-arm64.zip`         |
| Windows 32 位                   | `alx-windows-386.zip`           |
| macOS Apple Silicon             | `alx-darwin-arm64.zip`          |
| macOS Intel                     | `alx-darwin-amd64.zip`          |
| Linux x64                       | `alx-linux-amd64.zip`           |
| Linux ARM64                     | `alx-linux-arm64.zip`           |
| Linux ARMv7                     | `alx-linux-armv7.zip`           |
| Linux 32 位 x86                 | `alx-linux-386.zip`             |
| Linux ppc64le / s390x / riscv64 | 对应的 `alx-linux-<架构>.zip`   |
| FreeBSD x64 / ARM64             | 对应的 `alx-freebsd-<架构>.zip` |

Windows 直接运行 `alx.exe`。macOS / Linux / FreeBSD：

```bash
chmod +x alx
./alx
```
## Docker 部署

详细的持久化、认证、更新与安全边界见 [Docker 部署文档](docs/docker.md)。

## 操作可控

完整能力范围与权限边界见 [MCP 控制面文档](docs/mcp.md)。

## 文档

- [MCP 控制面](docs/mcp.md)
- [Docker 部署](docs/docker.md)
- [命令行](docs/cli.md)
- [系统插件开发](docs/plugin-development.md)
- [机器人应用页规范](docs/bot-app-page.md)
