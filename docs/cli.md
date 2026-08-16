# AlemonX 命令行

工作台提供同名的 `alx` 命令，用于在浏览器不可用或需要远程排障时完成常见操作。后台安装后命令会位于用户目录的本地命令目录；若终端尚未找到它，请按安装结果将该目录加入 `PATH`。

尚未安装时，不必前往 GitHub Releases。macOS、Linux、FreeBSD 可执行：

```sh
curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh
```

Windows PowerShell 可执行：

```powershell
irm https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

脚本会下载当前系统对应的最新正式包并校验 SHA-256；完成后重新打开终端即可运行 `alx`。

服务器无法直连 GitHub 时，脚本会自动依次尝试 `ghfast.top`、`ghproxy.net` 和 `gh-proxy.com`。若 GitHub Raw 也无法访问，可通过镜像取得脚本本身：

```sh
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh
```

也可设置 HTTPS 的 `ALX_DOWNLOAD_BASE` 来优先使用自建镜像；内置下载源会在它失败后继续尝试。

```bash
# 打开与状态
alx open
alx status
alx health
alx doctor

# 启动（前台）与监听设置
alx --port 17390                        # 默认监听 0.0.0.0，局域网/公网可直接访问
alx --host 127.0.0.1                    # 仅本机可访问
alx --workspace /path/to/workspace      # 指定统一工作区（模板与新建机器人）
alx --redis-port 6380                   # 调整内置 Redis 端口
alx --redis-off                         # 禁止启动内置 Redis

# 后台服务（Windows、macOS、Linux）
alx install --port 17390 [--host 0.0.0.0]
alx start
alx restart
alx stop
alx uninstall --yes

# 日志：默认最近 200 行；--follow 使用 Ctrl+C 结束
alx logs
alx logs --lines 500
alx logs --follow

# 更新与版本
alx version
alx update
```

工作区默认取 `ALX_WORKSPACE` 环境变量，其次取 `ALEMONJS_SETUP_ROOTS` 中第一个可写目录，最后回退到 `<运行目录>/workspace`；`--workspace` 优先级最高。模板存放在 `<workspace>/templates`，新建机器人默认落在 `<workspace>/bots`，内置 Yarn 物化到 `<workspace>/packages/yarn`；PM2 不随包嵌入，首次需要时用内置 Yarn 安装到 `<workspace>/packages/pm2`（位置固定）。

`alx health --port 17390` 只检查本机 `127.0.0.1` 的 `/healthz`，可用于确认服务是否已恢复。`alx doctor` 额外汇总后台服务、HTTP 健康、Node.js 与 Git 的环境状态。

## 公网访问（为什么需要 nginx）

历史版本默认只监听 `127.0.0.1`（仅本机可访问），因此服务器上的公网请求无法直接到达，只能靠 nginx 做反向代理：让 nginx 监听公网地址、再把请求转发到本机的 `127.0.0.1:17390`。当前版本默认监听 `0.0.0.0`，`http://服务器IP:17390` 即可直接访问，不再需要 nginx。

需要重新限制为本机访问时，显式指定 loopback：

```bash
alx --host 127.0.0.1 --port 17390
```

由于默认就监听所有网卡，请务必先开启身份认证（`alx auth enable`），并用防火墙限制可访问的 IP；公网直接暴露且未认证的工作台等同于任意控制你的机器人。生产部署还建议设置 `ALX_DEPLOYMENT=production`：SQLite 与本地认证未配置时都仅提示，不会阻止启动（可在引导页创建管理员账户）。

`alx install` 也支持 `--host`，安装的后台服务会按同样的地址监听；`--redis-port` 与 `--redis-off` 会持久化到 Redis 配置（`alx-redis.json`），设置页可随时重新调整。

`alx logs` 读取托管服务日志：macOS 读取 `~/Library/Logs/alx.log`，Linux 读取 `journalctl --user -u alx.service`，Windows 读取 `%LOCALAPPDATA%\\alx\\alx.log`。前台直接运行时日志只在启动它的终端内；FreeBSD 请使用其系统服务管理器的日志工具。

## 保活与开机恢复

`alx install` 注册的后台服务会在登录后启动，并在 macOS/Linux 上于异常退出后自动拉起。Linux 服务器若需要在用户**未登录**时、系统重启后也持续运行，应在设置 → 服务 → 保活与开机恢复中确认“启用无登录运行”；等效的管理员命令为：

```sh
loginctl enable-linger "$(id -un)"
```

机器人生产运行应使用 PM2。ALemonX 在启动、重启或 reload 成功后会执行 `pm2 save` 保存恢复清单；首次部署仍需由服务器管理员按 PM2 输出完成一次 `pm2 startup` 注册，避免在主机重启后丢失 PM2 守护进程。
