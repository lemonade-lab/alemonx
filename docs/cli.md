# AlemonX 命令行

工作台提供同名的 `alx` 命令，用于在浏览器不可用或需要远程排障时完成常见操作。后台安装后命令会位于用户目录的本地命令目录；若终端尚未找到它，请按安装结果将该目录加入 `PATH`。

```bash
# 打开与状态
alx open
alx status
alx health
alx doctor

# 后台服务（Windows、macOS、Linux）
alx install
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

`alx health --port 17390` 只检查本机 `127.0.0.1` 的 `/healthz`，可用于确认服务是否已恢复。`alx doctor` 额外汇总后台服务、HTTP 健康、Node.js 与 Git 的环境状态。

`alx logs` 读取托管服务日志：macOS 读取 `~/Library/Logs/alx.log`，Linux 读取 `journalctl --user -u alx.service`，Windows 读取 `%LOCALAPPDATA%\\alx\\alx.log`。前台直接运行时日志只在启动它的终端内；FreeBSD 请使用其系统服务管理器的日志工具。
