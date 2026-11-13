# AlemonX 桌面版

桌面模块从 Wails v3.0.0 的 vanilla 模板改造。Wails 窗口加载随应用分发的 `alx` 工作台，复用仓库的 React 前端、HTTP API 和认证。浏览器部署仍使用仓库根目录的 `alx serve`。

## 在 macOS 构建

准备 Go 1.26 或更新版本、Xcode 命令行工具、Node.js 和 Yarn。在仓库根目录安装前端依赖后进入桌面模块：

```sh
yarn --cwd frontend install --frozen-lockfile
cd desktop
sh build/darwin/package.sh
open "bin/AlemonX Desktop.app"
```

如果已经安装 Wails 3 CLI，也可以在 `desktop/` 下运行 `wails3 task package`。构建脚本会打包当前 Mac 架构，并用临时签名生成供本机验证的 `bin/AlemonX Desktop.app`。

## 运行

工作台监听 `127.0.0.1:17392`，默认工作区位于 `~/Library/Application Support/AlemonX Desktop/workspace`。可通过 `ALX_WORKSPACE` 或启动参数选用已有工作区：

```sh
open "bin/AlemonX Desktop.app" --args --workspace "/path/to/workspace" --port 17392
```

固定端口能保留同一来源下的 WebView 本地设置。关闭窗口会收起工作台，从菜单栏打开可恢复窗口。选择「退出 AlemonX」会关闭工作台服务。macOS 和 Windows 可通过托盘恢复收起的工作台。通过替换完整应用包更新桌面版，工作区位于应用包外。

## 构建 Windows x64

在 macOS 或 Linux 上准备 Go 1.26+、Node.js、Yarn、zip、NSIS（`makensis`）以及 Wails v3.0.0 CLI。安装前端依赖后执行：

```sh
cd desktop
wails3 task package:windows
```

产物位于 `desktop/bin/`。便携 ZIP 解压后双击 `AlemonX Desktop.exe`，并保留同目录的 `alx.exe`。NSIS 安装包按当前用户安装，缺少 WebView2 时会通过 Microsoft 引导程序下载安装。两个产物均为 Windows x64，默认工作区为 `%APPDATA%\AlemonX Desktop\workspace`。

## 构建 Linux

在 Ubuntu 24.04 或具有 GTK4 和 WebKitGTK 6.0 的 Linux 上构建。安装 Go 1.26+、Node.js、Yarn 和构建依赖：

```sh
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
yarn --cwd frontend install --frozen-lockfile
sh desktop/build/linux/package.sh
```

解压 `desktop/bin/alemonx-desktop-linux-<arch>.tar.gz` 后运行 `./alemonx-desktop`。支持 x64 和 ARM64，运行时依赖可见包内的 README。Linux 使用系统标题栏，可直接通过窗口关闭按钮关闭工作台。

## 桌面发布

在 Actions 中手动运行 **AlemonX Desktop** 可以生成构建产物。推送 `desktop-vX.Y.Z` 标签会发布独立的桌面预发行版本，资源名称以 `alemonx-desktop-` 开头。Web 版继续使用 `vX.Y.Z` 和原有的更新索引。
