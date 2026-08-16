# ALemonX Command Line

> 中文版：[../cli.md](../cli.md)。

The workbench ships a matching `alx` command for common operations when the browser is unavailable or you need to troubleshoot remotely. Once installed as a background service, the command lives in a local command directory under your user directory; if your terminal cannot find it, add that directory to `PATH` as the install result suggests.

## Installation

You do not need to visit GitHub Releases first. On macOS, Linux, or FreeBSD:

```sh
curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh
```

On Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

The script downloads the latest release for your system and verifies its SHA-256 checksum; after it finishes, reopen your terminal and run `alx`.

When GitHub is not directly reachable, the script automatically tries `ghfast.top`, `ghproxy.net`, and `gh-proxy.com` in order. Users in mainland China can set `ALX_PREFER_MIRROR=1` so mirror sources are tried before the official source, and can also fetch the script itself through a mirror:

```sh
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | ALX_PREFER_MIRROR=1 sh
```

Windows PowerShell:

```powershell
$env:ALX_PREFER_MIRROR='1'; irm https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

You can also point the HTTPS `ALX_DOWNLOAD_BASE` environment variable at a self-hosted mirror; the built-in download sources are tried after it fails.

## Open, status, and diagnostics

```bash
# Open and status
alx open
alx status
alx health
alx doctor

# Foreground start and listen settings
alx --port 17390                        # default listens on 0.0.0.0; reachable from LAN/public
alx --host 127.0.0.1                    # localhost only
alx --workspace /path/to/workspace      # set the unified workspace (templates and new bots)
alx --redis-port 6380                   # change the built-in Redis port
alx --redis-off                         # do not start the built-in Redis

# Background service (Windows, macOS, Linux)
alx install --port 17390 [--host 0.0.0.0]
alx start
alx restart
alx stop
alx uninstall --yes

# Logs: 200 lines by default; --follow ends with Ctrl+C
alx logs
alx logs --lines 500
alx logs --follow

# Updates and version
alx version
alx update
```

The workspace root is resolved from the `ALX_WORKSPACE` environment variable, then from the first writable entry in `ALEMONJS_SETUP_ROOTS`, and finally falls back to `<run-dir>/workspace`; `--workspace` takes the highest precedence. Templates live in `<workspace>/templates`, new robots land in `<workspace>/bots` by default, the built-in Yarn is materialized into `<workspace>/packages/yarn`, and PM2 is not embedded - when first needed it is installed with the built-in Yarn into `<workspace>/packages/pm2` (a stable location).

`alx health --port 17390` only checks `127.0.0.1` `/healthz` locally and can confirm whether the service has recovered. `alx doctor` additionally summarizes the background service, HTTP health, and Node.js/Git environment state.

## Public access (why nginx)

Older versions listened only on `127.0.0.1` (localhost only), so public requests could not reach the server directly; the only option was an nginx reverse proxy: nginx listens on the public address and forwards requests to the local `127.0.0.1:17390`. The current version listens on `0.0.0.0` by default, so `http://<server-ip>:17390` works directly without nginx.

To restrict access to the local machine again, bind to loopback explicitly:

```bash
alx --host 127.0.0.1 --port 17390
```

Because it listens on all interfaces by default, enable local authentication first (`alx auth enable`) and restrict reachable IPs with a firewall; an unauthenticated workbench exposed to the public internet is equivalent to handing out control of your robots. For production deployments, also set `ALX_DEPLOYMENT=production`: missing SQLite or local authentication only prints reminders and does not block startup (an administrator account can be created from the setup guide).

`alx install` also accepts `--host`; the installed background service listens on the same address. `--redis-port` and `--redis-off` are persisted to the Redis configuration (`alx-redis.json`) and can be changed anytime from the settings page.

`alx logs` reads the managed service logs: `~/Library/Logs/alx.log` on macOS, `journalctl --user -u alx.service` on Linux, and `%LOCALAPPDATA%\alx\alx.log` on Windows. When running in the foreground, logs only appear in the terminal that started it; on FreeBSD use its system service manager log tool.

## Keep-alive and startup recovery

The background service registered by `alx install` starts after login and is automatically restarted after abnormal exits on macOS/Linux. If a Linux server must keep running when the user is **not** logged in and after a system reboot, confirm "Run without login" in Settings → Service → Keep-alive & Startup Recovery; the equivalent admin command is:

```sh
loginctl enable-linger "$(id -un)"
```

Robots should run with PM2 in production. ALemonX runs `pm2 save` after a successful start, restart, or reload to persist the recovery list; on first deployment a server administrator still needs to run `pm2 startup` once per the PM2 output, so the PM2 daemon survives host reboots.
