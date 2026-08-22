# ALemonX User Manual

> For users new to ALemonX / AlemonJS. This manual starts from "what is this", explains the core concepts and related technologies you will meet in the workbench, and ends with frequently asked questions. 中文版见 [../user-manual.md](../user-manual.md)。

## Contents

1. [Getting to know the project](#1-getting-to-know-the-project)
2. [Related technologies](#2-related-technologies)
3. [Frequently asked questions](#3-frequently-asked-questions)

## 1. Getting to know the project

### What is ALemonX

ALemonX (the command is `alx`) is a **local workbench for AlemonJS robots**. AlemonJS is a Node.js-based robot development framework that can connect to QQ, Discord, OneBot, and other platforms; ALemonX brings creating, running, debugging, testing, and publishing those robots into one graphical workbench so you do not have to type commands all the time.

In one sentence: **ALemonX manages "robot projects", AlemonJS is the "robot framework", and your robot project is an AlemonJS application running on Node.js.**

### What you can do with ALemonX

- **Create robot projects**: JavaScript or TypeScript, with optional style, image component, and capability templates (bubble service, data storage, QQ Bot, Discord, OneBot, etc.).
- **Check the development environment**: automatically detects whether Node.js, Git, package managers, and browsers are ready, with one-click fix entries for what is missing.
- **Run robots**: development mode (hot reload), foreground mode, and PM2 persistent mode.
- **Manage multiple robots**: add, pin, and switch robot directories; view run state and logs in one place.
- **Test and debug**: built-in Test Center, LiveChat, and bot app pages; PM2 logs support paging, streaming, and export.
- **Publish**: package and publish to npm, or create Git Release tags.
- **AI collaboration**: let the AI Agent read and modify your robot code in the workbench, with confirmation required for every write; there is also AI ops (observe/canary/auto-fix) and an MCP control plane for local AI clients such as Codex.
- **Extend capabilities**: system plugins (network, Docker, QQ management, etc.) add local machine management capabilities to the workbench itself.

### Core concepts

#### Workbench and windows

The frontend is a "desktop-style" UI: windows can be dragged, resized, and minimized; system features (environment, tasks, ops, accounts, plugins) appear in sidebar windows, and every installed system plugin gets its own window. The UI supports light/dark themes and adapts to small windows and narrow screens.

#### Robot project / robot directory

A robot project is a directory whose root must contain `package.json`. You can create a new project from the setup guide or add an existing robot directory under "Manage"; the workbench uses the robot directory as the identity key for run state, ports, PM2, and bot app pages.

#### Three run modes

| Mode | Purpose | Characteristics |
| --- | --- | --- |
| Development (dev) | Daily development and debugging | Starts with `lvy app.ts`; hot reload, logs directly visible |
| Foreground | Temporary runs | Runs in the current terminal; Ctrl+C stops |
| PM2 persistent | Production runs | Managed by a daemon with auto-restart; supports `pm2 save` recovery lists; recommended for production |

#### Unified workspace

ALemonX keeps templates, tools, and new robots in one workspace (default `<run-dir>/workspace`; override with `--workspace` or `ALX_WORKSPACE`):

```text
workspace/
├── templates/    project templates (materialized from embedded templates on first run; editable)
├── packages/     tooling directory (built-in Yarn materialized copy; PM2 is installed here on first use)
└── bots/         default destination for newly created robots
```

#### Three entry points

- **Browser UI**: `alx` (default `http://127.0.0.1:17390`) for day-to-day use.
- **CLI `alx`**: when the browser is unavailable or for remote troubleshooting; see [Command line](cli.md).
- **MCP control plane**: lets local AI clients like Codex and Doubao manage robots through the standard MCP protocol; see [MCP](mcp.md).

#### Three easily confused "embedded pages"

| Name | Who uses it | What it can do |
| --- | --- | --- |
| Plugin Console | system plugins | The plugin's own management UI; can call host capabilities and privileged operations |
| Host WebView | windows opened by system plugins | A plain iframe showing plugin pages or external URLs; no host privileges |
| Bot App Page | robot plugins | Can only reach the current robot's `./api/*`; the robot's own frontend |

#### System plugins vs robot plugins

- **System plugins**: provide system-level capabilities to ALemonX itself (network, firewall, Docker, etc.) and are unrelated to specific robots.
- **Robot plugins**: provide commands, config pages, and bot app pages for a specific robot; they are part of the robot project.

### Quick start

1. **Install**: install `alx` with the "One-line install" command in the README (use the mirror command in mainland China).
2. **Open**: run `alx` and open `http://127.0.0.1:17390` in a browser; or run `alx open`.
3. **Create**: choose "Develop" in the setup guide to create a new robot, or "Manage" to add an existing robot directory.
4. **Check the environment**: in "Environment", confirm Node.js (v22.22.3+) and Git are ready and fix them with one click as prompted.
5. **Run**: choose development mode in "Run" for debugging, or use PM2 for persistent runs.
6. **Test and publish**: verify features with the Test Center, then publish to npm or create a Git Release.

## 2. Related technologies

You will meet the technologies below in the workbench. As a beginner you do **not need to learn them all first** - just understand "what it is and what it does"; the workbench guides you through the actual operations.

| Technology | One-sentence explanation | Role in ALemonX |
| --- | --- | --- |
| Node.js | The "engine" that runs JavaScript on a computer | The runtime base of robots; ALemonX also uses it to manage robot processes |
| npm / Yarn | JavaScript "package managers" that download and install dependencies | Install robot dependencies, run scripts, package and publish |
| JSON | A general text format for data/configuration | `package.json` and other config files |
| YAML | A more human-readable configuration format | `alemon.config.yaml` robot configuration |
| JavaScript / TypeScript | The language for robot logic | Robot templates support JS or TS |
| Git | Code version control | Version management and Git Releases |
| PM2 | Node.js process daemon | Keeps robots running and auto-restarts on crash |
| Redis | In-memory database | Optional built-in cache; can be disabled |
| SSE | Server-to-browser one-way event push | Real-time log, task progress, and event refresh in the UI |
| MCP | Protocol for AI clients to connect local tools | Codex and others connect to the ALemonX control plane |
| AlemonJS ecosystem | Robot framework plus companion tools | Run, build, and check robots |

### Node.js

- **What it is**: a runtime that lets JavaScript run directly on a computer. Just as a browser runs JS in web pages, Node.js lets JS run on servers/local machines.
- **What it does in ALemonX**: your robot is a Node.js application; ALemonX starts, stops, and monitors it with Node.js, and checks that the Node.js environment is usable.
- **What you need to do**: **v22.22.3 or newer** is recommended. A lower version shows an upgrade prompt but does not block you; only "completely missing or broken" is a blocker.

### npm and Yarn

- **What they are**: package managers. npm ships with Node.js; Yarn is another popular one. They download code packages (dependencies) written by others and record versions.
- **What they do in ALemonX**: install robot dependencies (`install`), build, run scripts, and publish to npm. ALemonX bundles a Yarn copy, so creating projects and installing dependencies does not depend on you installing npm packages manually; tools like PM2 are installed into the workspace automatically.
- **What you need to do**: usually nothing manual; pick a package manager (npm / yarn / pnpm) in the workbench when needed.

### JSON

- **What it is**: JavaScript Object Notation - a plain-text format using `{}`, `[]`, and `"key": value` that both humans and programs can read.
- **What it does in ALemonX**: `package.json` (project metadata, dependencies, scripts), plugin manifests `alx.json`, and some local data files are JSON.
- **What you need to do**: just know that config files are JSON and be careful with commas and quotes; the workbench config editors validate for you.

### YAML

- **What it is**: another configuration format that uses indentation for nesting and reads more "human-friendly" than JSON. AlemonJS robots use `alemon.config.yaml` for platform accounts, listen ports, etc.
- **What it does in ALemonX**: the workbench provides a visual editor for this configuration and injects theme variables in settings.
- **What you need to do**: YAML is **indentation-sensitive**; editing with the workbench editor is safer than handwriting.

### JavaScript / TypeScript

- **What they are**: JavaScript is the language for robot logic; TypeScript is a typed superset that catches many low-level mistakes early.
- **What they do in ALemonX**: choose a JS or TS template when creating a project; robot commands and event handlers are written in source files.
- **What you need to do**: basic syntax is enough to write robot features; the AI Agent can also help edit code with your confirmation.

### JSX / JSXP

- **What they are**: JSX is a syntax for writing UI structure inside JavaScript; JSXP (jsxp) is AlemonJS's rendering scheme for generating images/UI components in robots.
- **What they do in ALemonX**: image messages, help pages, and similar components; templates can optionally include image component capability.

### Git

- **What it is**: code version control that records every change and supports rollback, branches, and collaboration.
- **What it does in ALemonX**: initialize repositories, review changes, commit, and create Git Release tags for publishing.
- **What you need to do**: make sure Git is installed; you do not need to know the commands because the workbench has a graphical Git panel.

### PM2

- **What it is**: a Node.js process daemon that keeps programs running, restarts them after crashes, and manages logs.
- **What it does in ALemonX**: the robot "persistent run" mode is based on PM2; ALemonX runs `pm2 save` after a successful start/restart/reload to persist the recovery list.
- **Identity and moves**: the PM2 app name is derived from the `.alemonx-id` file in the robot root plus the `package.json` name, so moving the directory does not change it; older projects without the file keep their legacy name until the config is rewritten.
- **What you need to do**: on first server deployment, an admin runs `pm2 startup` once per the PM2 output so the daemon survives host reboots.

### Redis

- **What it is**: a high-performance in-memory database, often used for caching and queues.
- **What it does in ALemonX**: a built-in optional Redis; adjust the port with `--redis-port` or disable it with `--redis-off`; the settings page lets you change it anytime.
- **What you need to do**: usually nothing; if the port is occupied, change it or disable Redis.

### SSE

- **What it is**: Server-Sent Events - a mechanism for the server to push one-way events to the browser, like subscribing to a real-time message stream.
- **What it does in ALemonX**: logs, task progress, plugin changes, and ops events refresh in real time through it.

### MCP

- **What it is**: Model Context Protocol - a standard protocol for AI clients (Codex, Doubao, etc.) to connect local tools safely, using JSON-RPC 2.0.
- **What it does in ALemonX**: exposes robot management as restricted tools to local AI clients; every modifying operation requires `confirm: true`.
- **What you need to do**: to let an AI client control robots, configure stdio or Streamable HTTP per the [MCP docs](mcp.md); optionally set `MCP_ALLOWED_ROOTS` to limit manageable directories.

### AlemonJS ecosystem tools

| Tool | Role |
| --- | --- |
| `alemonjs` | Core robot framework package |
| `alemonc` | Robot check/start helper (`npx alemonc start`) |
| `lvy` | Development/build tool (`lvy app.ts`, `lvy build`) |
| `jsxp` | Image/UI component rendering |

## 3. Frequently asked questions

### Installation and startup

**Q1: Installation in mainland China is slow or fails. What now?**

Use the mirror install command from the README (e.g. `curl ... | ALX_PREFER_MIRROR=1 sh`); if `ghfast.top` is unavailable, switch to `ghproxy.net` or `gh-proxy.com`. You can also point `ALX_DOWNLOAD_BASE` at a self-hosted mirror. The installer verifies SHA-256 automatically.

**Q2: `alx: command not found`?**

The installer puts `alx` in a user command directory (usually `~/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\ALemonX` on Windows). Reopen your terminal, or add that directory to `PATH`.

**Q3: Port 17390 is occupied?**

Start with `alx --port <other-port>`, or stop the process holding the port; if the built-in Redis port conflicts, adjust it with `--redis-port` or disable Redis with `--redis-off`.

**Q4: The browser cannot open the workbench?**

Confirm the service is running with `alx health`, `alx status`, or `alx doctor`. It listens on `0.0.0.0` by default, so LAN clients can visit `http://<server-ip>:17390`; for local-only access use `alx --host 127.0.0.1`.

**Q5: It asks for administrator privileges / sudo?**

Some system-level operations (installing environment dependencies, system services, privileged plugin operations) need authorization. Enter a password in the privileged dialog (password mode) or use native system authorization (native mode); operations are written to the audit log.

**Q6: Where is the background service registered? What if I move the alx program?**

`alx install` registers the alx program you are currently running (it is not copied to another directory) and pins the workspace resolved at install time into the service command line, so the background service and the foreground share the same workspace. Every `alx install` overwrites the existing registration with the current program and workspace; it never skips because a service is already installed. Installation enables **startup recovery** by default; on Linux it also attempts to enable **run-without-login** (a permission failure is reported and can be fixed later under Settings → Service). After moving the program, run `alx install` again from the new location, or simply run `alx start` (it detects the change and re-registers with the current program); `alx status` shows the registered program and workspace paths.

To uninstall the background service, run `alx uninstall --yes` or click "Uninstall service" under Settings → Service. Uninstalling only removes the service registration and startup recovery; it never deletes workbench data, accounts, or robot projects. You can reopen the workbench later by running `alx` manually.

### Environment and dependencies

**Q7: Node.js version is too low?**

Upgrade to v22.22.3+ if possible. A low version is marked `outdated` with an upgrade entry but does not block usage; only a missing or broken environment is a blocker.

**Q8: Git or a package manager is missing?**

Check the "Environment" page and install with one click as prompted. Git is used for version control and publishing; a package manager installs robot dependencies.

### Running robots

**Q9: The robot failed to start. Where are the logs?**

Development/foreground logs are in the terminal or run panel; for PM2 mode use "PM2 logs" (paging, streaming, export supported). From the CLI, `alx logs` shows the background service logs (macOS: `~/Library/Logs/alx.log`; Linux: `journalctl --user -u alx.service`; Windows: `%LOCALAPPDATA%\alx\alx.log`).

**Q10: The port is already in use by another process?**

The workbench identifies port ownership automatically; if another program owns it, use a different robot port or stop the occupying program. On Windows, ownership is resolved through the managed process tree (npm/yarn-launched Node processes).

**Q11: What is the difference between dev mode and PM2 mode?**

Dev mode is for editing code: hot reload and direct logs. PM2 mode is for stable running: daemon management, auto-restart, and startup recovery lists - recommended for production.

### Configuration and data

**Q12: Where do I change robot configuration?**

`alemon.config.yaml` in the robot root (platform accounts, ports, etc.) and `package.json` (dependencies, scripts, publishing info). The workbench provides visual editors; sensitive fields (`.env`, `.npmrc`, alemon config drafts) are never persisted in the browser.

**Q13: Where does workbench data live, and how do I back it up?**

The workspace directory stores templates, tools, new robots, and system plugins. System plugins always live in `workspace/plugins`; installation never writes to the application directory. Default recoverable plugin data lives in `workspace/store/<plugin ID>`—for example, QQ sessions and downloaded components—and is retained across plugin upgrades. Workbench state (accounts, config, SQLite, and download caches) lives in the user config directory (host `./data` in Docker). AI ops data defaults to `ops.db` (SQLite). Back up both the workspace and data directories.

The workspace paths also appear in the robot creation destination step (default `workspace/bots`). When templates or built-in tools have a newer version, they are never overwritten automatically; to refresh, delete the corresponding copy directory and use it again.

**Q14: I forgot the workbench admin password?**

Reconfigure the account with `alx auth enable --account ... --password ... --confirm-password ...`; `alx auth status` shows the current state. In production, also restrict access with a firewall.

### AI and MCP

**Q15: Why does the AI ask for confirmation again and again?**

This is a deliberate security boundary: every modifying operation requires your confirmation (`confirm: true`), and MCP constraints are enforced in the Go service layer, not by client hints. You can reject or stop tasks anytime.

**Q16: Codex cannot connect to MCP?**

First run `command -v alx` to confirm `alx` is on PATH. For stdio, use launch command `alx` with arguments `mcp`; for Streamable HTTP, set `MCP_TOKEN` and start `alx mcp-http`, then use `http://127.0.0.1:17391/mcp`. Optionally set `MCP_ALLOWED_ROOTS` to limit manageable directories.

**Q17: What can the AI do to my robots, and what not?**

Can: read source, edit code, install dependencies, build, Git operations, manage runs (with your confirmation). Cannot: arbitrary host shell commands; reading `.env`/`.npmrc`/private keys; accessing `.git`/`node_modules`/symlinks; reading or writing files larger than 1 MiB.

### Updates and upgrades

**Q18: How do I update ALemonX?**

`alx update` checks and auto-updates (with SHA-256 verification). For Docker, run `sh docker-install.sh pull` followed by `restart` on the host; **do not run `alx update` inside the container**.

**Q19: Will my configuration be lost after an update?**

No. Templates and tools are only copied when missing, and existing files are never overwritten; accounts, config, SQLite, and plugin data are kept. If you hit incompatibilities, back up `workspace`, `./data` (Docker), and `ops.db` before upgrading.

---

More details: [Command line](cli.md) · [Docker deployment](docker.md) · [MCP control plane](mcp.md) · [System plugin development](plugin-development.md) · [Bot app page specification](bot-app-page.md).
