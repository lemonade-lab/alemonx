# ALemonX System Plugin Development and Integration

> 中文版：[../plugin-development.md](../plugin-development.md)。

System plugins add **global local-machine management capabilities** to ALemonX (`alx`), such as network checks, system services, firewall rules, or Docker management. They are unrelated to robot projects; do not use them to implement a robot's commands, config pages, or bot app pages - those capabilities belong in robot plugins.

This document is the complete integration guide for system plugins, covering: plugin structure, the `alx.json` manifest, the **communication protocol** (console page ↔ host ↔ runner), **all available APIs**, host capabilities and the `ALXHost` SDK, local service proxying, uploads, privileged operations and audit, source development sessions, publishing and installation, and the **system features and components in the workbench**.

## Contents

1. [Positioning and boundaries](#positioning-and-boundaries)
2. [Plugin structure](#plugin-structure)
3. [Quick start](#quick-start)
4. [alx.json manifest reference](#alxjson-manifest-reference)
5. [Communication protocol](#communication-protocol)
6. [HTTP API overview](#http-api-overview)
7. [Host capabilities and the ALXHost SDK](#host-capabilities-and-the-alxhost-sdk)
8. [Local services](#local-services)
9. [Uploads](#uploads)
10. [Privileged operations and audit](#privileged-operations-and-audit)
11. [Source development sessions](#source-development-sessions)
12. [System features and components](#system-features-and-components)
13. [Publishing, installation, and version management](#publishing-installation-and-version-management)
14. [Security boundary and development guidelines](#security-boundary-and-development-guidelines)
15. [Reference implementations](#reference-implementations)

## Positioning and boundaries

- **System plugins**: provide system-level capabilities to `alx` itself; the UI is the plugin's own web frontend, which performs local operations through host-forwarded action APIs. Examples: `alemonx-network` (network/port forwarding/firewall) and `alemonx-docker` (Docker Compose management).
- **Robot plugins**: provide commands, config pages, bot app pages, etc. for a specific AlemonJS robot, managed through the robot platform lifecycle. The two are completely separate; do not mix them.
- **The UI is the plugin**: the manifest no longer declares `pages`/`actions`/`fields` - those "redundant configs" have been removed. A plugin's UI is its `web.root` static frontend, served same-origin by ALemonX; complex interactions are implemented in the plugin page itself.
- **Discovery never executes plugin code**: listing, rendering, and enabling/disabling never run the runner; an isolated process starts only when an action is invoked.

### Terminology

The repository has three easily confused "embedded page" mechanisms; docs and code distinguish them with these names:

| Name | Provider | Entry | Injected SDK | Capability boundary |
| --- | --- | --- | --- | --- |
| **Plugin Console** | alx serves the system plugin's static directory same-origin | `/api/v1/setup/plugins/web/<id>/` | `ALXHost` | action forwarding, host capabilities, privileged operations |
| **Host WebView** | opened by a system plugin via `ALXHost.webview.open` | host-managed window | none (plain iframe) | plugin static resources or external http(s) URLs only; no host privileges |
| **Bot App Page** | plugin page inside a robot package, proxied by alx | `/api/v1/robot/webview/<token>/<id>/` | `window.__alxWebview` | only the current robot's `./api/*` |

The complete bot app page spec is in [Bot app page specification](bot-app-page.md).

## Plugin structure

A system plugin is a directory like:

```text
plugins/
  my-status/
    alx.json        # manifest (required, exact filename, <= 64 KiB, not a symlink)
    web/            # console page root (required, same-origin hosted; SPA ok)
      index.html
    runner/         # runner (optional; entry must be declared to run actions)
      main.mjs
    dist/           # platform binaries for publishing (inside the Release archive)
    .alx-install.json  # written by the host after install; do not maintain by hand
```

A running ALemonX discovers plugins in this order, loading only the first location for a given `id`:

1. `<workspace>/plugins/`: the sole writable destination for user-installed plugins and the highest-priority root;
2. application-provided `plugins/`: the executable-adjacent directory for installed applications, with the current working directory's `plugins/` retained as a source-development compatibility root.

Plugins are no longer read from `alx/plugins/` in the user configuration directory. Plugin data must not be written into plugin code; runners should use the `<workspace>/store/<plugin ID>/` directory provided through `ALX_PLUGIN_STORE`.

Adding/removing plugin directories or editing `alx.json` is hot-reloaded automatically by the backend (reflected within about 1 second), with no restart or manual refresh; the frontend learns about changes via a revision number or SSE events.

## Quick start

Minimal manifest:

```json
{
  "id": "my-status",
  "name": "示例状态",
  "version": "1.0.0",
  "runtime": "node",
  "entry": { "darwin-arm64": "runner/main.mjs", "linux-amd64": "runner/main.mjs", "linux-arm64": "runner/main.mjs", "windows-amd64": "runner/main.mjs", "windows-arm64": "runner/main.mjs" },
  "web": { "root": "web" },
  "navigation": { "label": "示例", "icon": "circle", "order": 10 }
}
```

During source development, put it under `plugins/` at the repository root as a compatibility discovery root (only when it is not overridden by the same ID in `<workspace>/plugins/`); publish platform archives through GitHub Releases when shipping (see [Publishing, installation, and version management](#publishing-installation-and-version-management)).

## alx.json manifest reference

The manifest filename must be exactly `alx.json`, at most 64 KiB, and must not be a symlink. `id` must match `^[a-z][a-z0-9-]{1,63}$`.

| Field | Required | Description |
| --- | --- | --- |
| `id` | Yes | Stable plugin identifier; avoid changing it after publishing. |
| `name`, `version` | Yes | Name and version shown in the admin UI. |
| `description` | No | Plugin description. |
| `platforms` | No | Supported Go platform names (`darwin`/`linux`/`windows`); unset means all platforms. |
| `navigation` | No | Global function bar entry; `label` defaults to `name`, `icon` defaults to `◈`, and a smaller `order` comes first. |
| `runtime` | No | `binary`, `node`, `go`, or `python`; defaults to `binary`. |
| `entry` | Yes* | Runner mapping. Keys are `GOOS-GOARCH` (e.g. `linux-amd64`); a bare `GOOS` fallback is allowed. Paths must be regular files inside the plugin directory; absolute paths, `..` escapes, and symlinks are forbidden. |
| `web` | **Yes** | Console page directory (e.g. `web`); cannot be absolute or contain `..`. A plugin without `web` is unusable. |
| `services` | No | Loopback HTTP services whitelisted in the manifest; the host exposes them through an authenticated same-origin proxy (see [Local services](#local-services)). |
| `systemPickers` | No | Named workbench Finder requests (file/directory, single/multiple, title). |
| `uploads` | No | Runner actions allowed to receive browser-uploaded bytes, with a `maxBytes` limit (see [Uploads](#uploads)). |
| `statusActions` | No | Explicitly read-only runner actions; the host coalesces queries and does not allocate tasks, suitable for fast UI polling. |
| `media` | No | Typed bytes produced by runner actions (QR codes, screenshots, etc.), served to the UI through a host same-origin endpoint. |
| `privilegedOperations` | No | System operations requiring host authorization: `authorization` (`password`/`native`), platforms, `runnerAction`, optional `planAction`/`useLatestAudit` (see [Privileged operations and audit](#privileged-operations-and-audit)). |
| `development` | No | Source development declaration: development runner, frontend build/dev service, managed local services (see [Source development sessions](#source-development-sessions)). |

`runtime` and `entry` startup modes:

| runtime | entry value | Startup |
| --- | --- | --- |
| `binary` | platform binary path | execute directly |
| `node` | `.mjs`/`.js` path | `node <entry>` |
| `go` | `entry.go` path or directory | `go run <entry.go>` / `go run <dir>` |
| `python` | `entry.python` path | `python3 <entry>` |

A purely static plugin (no `entry`) can host a `web` UI for displaying information, but any action call fails because there is no runner. `runnable` is always true for local plugins (only online-recognized plugins report false).

## Communication protocol

System plugin communication has three layers:

```text
Console page (same-origin iframe)
  │  POST /api/v1/setup/plugins/<id>/actions  {action, confirm, params}
  ▼
Host (alx: validates plugin identity, task mutual exclusion, progress/audit)
  │  stdin: {"protocol":"alx/v1","method":"run","action":...,"params":...}
  │  stdout: {"output":...,"data":...,"error":...} (single JSON response)
  │  stderr: logs / @alx-progress progress frames
  ▼
Runner (isolated process; a new process per action)
```

### Request (host → runner stdin)

For each action the host starts an isolated process with the plugin directory as the working directory and writes one JSON object to stdin:

```json
{
  "protocol": "alx/v1",
  "method": "run",
  "action": "network-check",
  "params": {},
  "confirm": false
}
```

| Field | Description |
| --- | --- |
| `protocol` | Fixed `alx/v1`; the runner should validate it. |
| `method` | Fixed `run`. |
| `action` | Action name, whitelisted by the runner itself; the host does not validate semantics - the runner's internal `switch` only handles the actions it supports and validates each parameter. |
| `params` | String-to-string mapping of action parameters. |
| `confirm` | Compatibility field; second confirmation of dangerous operations is the console page's job, and the host only sets it `true` for privileged operations. |
| `stateDir` | Reserved field; the current host does not populate it; runners must not depend on it. |

### Response (runner → host stdout)

Read exactly **one JSON response** from stdout. Do not print logs or debug text to stdout; write logs to stderr.

Success:

```json
{ "output": "✓ 已检查 3 个网卡。", "data": { "interfaces": [] } }
```

Failure (still emit valid JSON with `error` set):

```json
{ "output": "已检查现有规则。", "error": "需要管理员权限" }
```

| Field | Description |
| --- | --- |
| `output` | Human-readable result summary. Lines starting with `✓ `, `! `, or `? ` (symbol plus one space) can be colored by status; indented lines are shown as plain text. When a success has no summary, "插件操作已完成。" is displayed. |
| `data` | Optional structured result for the web UI; legacy plugins returning only `output` remain compatible. |
| `error` | Non-empty means the operation failed. |

A task is marked failed under any of: non-zero process exit, no valid JSON on stdout, or a non-empty `error`.

### Progress events (stderr)

Optional. A runner may write progress frames prefixed with `@alx-progress ` to stderr; the host forwards them to task progress. Other stderr is kept as diagnostic text (shown on failure):

```text
@alx-progress {"stage": "prepare", "percent": 10, "message": "正在检查环境…"}
```

Requirements: `stage` non-empty, `percent` an integer from 0 to 100. stderr lines that are not progress frames are not parsed as progress.

### Task model and polling

- **Mutating actions** (regular `actions`, uploads, privileged operations) go through async tasks: `POST /actions` returns task info immediately (`202 Accepted`), then poll `GET /api/v1/robot/tasks` for the result (`status`: `running`/`completed`/`failed`; result in `output`, structured result in `data`, failure reason in `error`, progress in `progress`).
- **Mutual exclusion**: only one mutating operation is allowed per plugin at a time. Re-submitting the same action returns `202` with the existing task; conflicting actions return `409`.
- **Read-only status actions**: `GET /api/v1/setup/plugins/<id>/status?action=<name>`, restricted to manifest `statusActions` with no parameters. The host coalesces concurrent reads and keeps a 1-second in-memory cache; it **does not allocate tasks**, so it is suitable for fast UI polling.

### Environment variables

Runners inherit the host environment (PATH includes the managed Node toolchain directories). The host may inject controlled environment variables as needed; **the manifest cannot request environment variables** - this is a host policy layer, not a manifest feature. Currently trusted injections:

| Variable | Description |
| --- | --- |
| `ALX_PLUGIN_DOWNLOAD_BROKER` | Download broker loopback endpoint (`/api/v1/system/plugin-download`). |
| `ALX_PLUGIN_DOWNLOAD_TOKEN` | One-time download token (24 requests, 90-minute expiry). |
| `ALX_PLUGIN_PROGRESS_MODE` | `structured`: stderr progress frames are available. |
| `ALX_PLUGIN_INSTALLED_TAG` | Tag of the installed Release. |
| `ALX_PLUGIN_STORE` | Host-created persistent plugin data directory: `workspace/store/<plugin ID>`. Downloaded runtimes, sessions, databases, and recoverable configuration should default here; plugin upgrades do not remove it. |
| `ALX_PLUGIN_DEV_PORT` | The only replaceable variable in source-development commands (host-allocated loopback port). |

`ALX_PLUGIN_STORE` is injected into regular runners, host-authorized privileged runners, and source-plugin build, development-service, and development-Web processes. Static Web pages cannot read this directory directly; expose any necessary access through a controlled runner interface.

### Minimal Node.js runner

```js
import process from 'node:process'

let input = {}
try {
  input = JSON.parse(await new Response(process.stdin).text())
  if (input.protocol !== 'alx/v1' || input.method !== 'run') {
    throw new Error('不支持的 ALX Setup 插件协议')
  }
  if (input.action !== 'check') throw new Error('未知操作')
  process.stdout.write(JSON.stringify({ output: '✓ 检查完成。' }))
} catch (error) {
  process.stdout.write(JSON.stringify({ error: String(error.message || error) }))
}
```

## HTTP API overview

Except for the plugin's own web resources, all endpoints live under `/api/v1` and the host validates the login session and plugin identity. The tables below are grouped by purpose.

### Plugin lifecycle and marketplace

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins` | List of locally discovered plugins (including disabled and online-recognized-only entries). |
| GET | `/api/v1/setup/plugins/market` | Online curated catalog, independent of local installs. |
| GET | `/api/v1/setup/plugins/revision` | Plugin set revision number for cheap hot-swap detection. |
| GET | `/api/v1/setup/plugins/events` | SSE "plugins changed" event stream (`data: {}`, 25-second heartbeat). |
| GET | `/api/v1/setup/plugins/releases/<id>` | Plugin GitHub Release list and assets (with SHA-256 and platform compatibility). |
| POST | `/api/v1/setup/plugins/upload` | Upload and install a plugin archive (`.zip`/`.tar.gz`/`.tgz`, single file; host validates extraction). |
| POST | `/api/v1/setup/plugins/<id>/install` | Download and install a Release by `{version, assetName}`. A first installation is enabled by default; a previously explicitly disabled ID must be enabled again. |
| POST | `/api/v1/setup/plugins/<id>/enabled` | Enable/disable: `{enabled: bool}`. |
| POST | `/api/v1/setup/plugins/<id>/switch` | Switch version: `{version, assetName}`. |
| POST | `/api/v1/setup/plugins/<id>/uninstall` | Uninstall: `{confirm: true}`. |
| GET | `/api/v1/setup/plugins/<id>/versions` | Cached version list (tag/asset/size/fingerprint/active/cache state). |
| DELETE | `/api/v1/setup/plugins/<id>/versions/<tag>` | Delete a non-active cached version. |
| GET/POST | `/api/v1/setup/plugins/cache` | Plugin version cache summary; POST clears the cache immediately. |

### Action forwarding and data

| Method | Path | Description |
| --- | --- | --- |
| POST | `/api/v1/setup/plugins/<id>/actions` | Action forwarding: `{action, confirm, params, sudoPassword?, authorizationId?}`. |
| GET | `/api/v1/setup/plugins/<id>/status?action=<name>` | Read-only status action (manifest `statusActions` only, no parameters). |
| GET | `/api/v1/setup/plugins/<id>/media/<id>` | Manifest-declared media resource (runner returns `{available, data: <base64>}`). |
| POST | `/api/v1/setup/plugins/<id>/upload` | Browser file upload (multipart: `action`, `destination`, `files`); see [Uploads](#uploads). |

### Console page hosting

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins/web/<id>/...` | Console page static resources (same-origin). Only served for **installed, enabled, runnable** plugins; extension-less paths that do not exist fall back to `index.html` (SPA routing). HTML automatically gets `host-bridge.js` (the `ALXHost` SDK) injected, plus CSP and `X-Content-Type-Options: nosniff`; path traversal and symlink escapes are rejected. |
| GET | `/api/v1/setup/plugins/web/<id>/host-bridge.js` | Host bridge SDK (`finder-bridge.js` is an alias). |

### Host capabilities

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/system/capabilities` | Versioned capability catalog (name, version, availability, unavailability reason). |
| POST | `/api/v1/system/capabilities/webview/open` | Open a host-managed WebView window: `url` or `resource` (see [Host WebView and global UI components](#host-webview-and-global-ui-components)). |
| POST | `/api/v1/system/capabilities/finder` | Return a Finder definition by `{pluginId, pickerId}` (kind/title/multiple) for the workbench to render. Legacy `/api/v1/system/picker` stays compatible. |
| GET | `/api/v1/system/capabilities/context?pluginId=&keys=robot,network` | Redacted read-only context. Keys are limited to `robot` and `network`. |
| POST | `/api/v1/system/capabilities/desktop/open` | Open a path/URL (handed to the system). |
| GET/POST | `/api/v1/system/capabilities/clipboard` | Read/write the system clipboard. |
| POST | `/api/v1/system/capabilities/notification` | Send a system notification. |
| GET | `/api/v1/system/capabilities/info?pluginId=` | Platform, architecture, hostname. |
| POST | `/api/v1/system/capabilities/network/fetch` | Restricted network request (GET/HEAD only, <= 2 MiB, 30-second timeout). |

### Privileged authorization and audit

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/system/privileged/status` | Privileged mode and audit chain status. |
| POST | `/api/v1/system/privileged/preflight` | `{pluginId, action, planId?}`; returns availability, authorization mode, and a one-time `intentId`. |
| GET | `/api/v1/system/privileged/audit?plugin=<id>` | Audit records for the given plugin. |

### Local service proxy

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/services?plugin=<id>` | Declared loopback services with reachability and proxy addresses. |
| GET/HEAD/POST/PUT/PATCH/DELETE | `/api/v1/services/<plugin>/<service>/...` | Same-origin proxy for manifest services (WebSocket requires declaring `websocket`). |
| same | `/api/v1/services/dynamic/<plugin>/<port>/...` | Runtime loopback port proxy (e.g. Docker published ports); target forced to `127.0.0.1`. |

### Download broker (runner only)

| Method | Path | Description |
| --- | --- | --- |
| GET/HEAD | `/api/v1/system/plugin-download?url=...` | Large file downloads: host proxy, redirects, one retry, conditional requests, and a user-directory cache; **does not forward** workbench cookies, Authorization, or internal identity headers. Loopback only and token-gated. |
| GET/DELETE | `/api/v1/system/plugin-download-cache` | Download cache summary (global 1 GiB) and cleanup; completely independent of the Release version cache, so cleanup never uninstalls plugins or deletes their data. |

### Tasks and events

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/robot/tasks` | Poll plugin action task results. |
| GET | `/api/v1/events?topics=robot,ops,system,plugins` | SSE event stream, including `plugins.changed` (plugin list change signal). |

## Host capabilities and the ALXHost SDK

Plugins do not declare `hostCapabilities`. When a browser page needs workbench desktop capabilities or context, it requests them through the host-provided versioned capability catalog and corresponding APIs; the host still validates plugin identity and the current login session for every call.

### Capability catalog

`GET /api/v1/system/capabilities` returns:

```json
{ "items": [ { "name": "finder.pick", "version": "v1", "available": true, "reason": "" } ] }
```

| Capability | API | Returned scope |
| --- | --- | --- |
| Catalog | `GET /api/v1/system/capabilities` | Version, availability, and unavailability reason. |
| `webview.open` | `POST /api/v1/system/capabilities/webview/open` | Open a host-managed WebView window (static resource or http(s) URL). |
| `ui.alert` / `ui.message` / `ui.modal` / `ui.notification` | via the parent-window `ui-request` message bridge | Host-rendered global frontend components for consistent interaction. |
| `finder.pick` | `POST /api/v1/system/capabilities/finder` | Workbench Finder file/directory selection results, defined only by manifest `pickerId`. |
| `context.current-robot` / `context.network-settings` | `GET /api/v1/system/capabilities/context?pluginId=<id>&keys=robot,network` | Current verified robot and redacted network configuration; **never includes passwords**. |
| `desktop.open`, `clipboard.read/write`, `notification.send`, `system.info` | corresponding `/api/v1/system/capabilities/{...}` | Authenticated, plugin-bound desktop basics. |
| `network.fetch` | `POST /api/v1/system/capabilities/network/fetch` | GET/HEAD requests using the main app proxy, mirrors, and retry settings. |

Finder requests only send `{ "pluginId": "...", "pickerId": "..." }`. The host takes the window title, type, and multi-select policy from the manifest and opens a unified Web Finder in the parent workbench; **plugins cannot submit paths, commands, ports, or native scripts**. The legacy `/api/v1/system/picker` stays temporarily compatible with published plugins; new plugins should use the capability API.

### ALXHost SDK

Every plugin page automatically gets the dependency-free `window.ALXHost` mini SDK (injected by the host via `host-bridge.js`); it is just a convenient wrapper around the capability APIs above:

```js
const paths = await window.ALXHost.finder.pick('my-status', 'runtime-directory')
await window.ALXHost.desktop.open('my-status', paths[0])
const { robot } = await window.ALXHost.context('my-status', ['robot'])
await window.ALXHost.notification.send('my-status', '已完成', robot?.name || '未选择机器人')
const info = await window.ALXHost.info('my-status')
const resp = await window.ALXHost.network.fetch('my-status', 'https://example.com/status', 'GET')
const clip = await window.ALXHost.clipboard.read('my-status')
await window.ALXHost.clipboard.write('my-status', 'hello')
```

SDK method overview:

| Method | Equivalent request |
| --- | --- |
| `capabilities()` | `GET /api/v1/system/capabilities` |
| `webview.open(pluginId, options)` | after validating `POST /api/v1/system/capabilities/webview/open`, sends `webview-request` to the parent window; the host opens the window and replies with `webview-result` (including the window `id`). |
| `webview.close(pluginId, webviewId?)` | sends `webview-close-request` to the parent window to close the given window (all of the plugin's WebViews by default); replies with `webview-close-result`. |
| `ui.alert(pluginId, options)` / `ui.message(pluginId, options)` / `ui.modal(pluginId, options)` / `ui.notification(pluginId, options)` | sends `ui-request` to the parent window; the host renders the global component and replies with `ui-result`. |
| `ui.setBusy(pluginId, busy)` | marks the currently displayed confirmation dialog as busy (`busy: true` disables confirm/cancel and Escape and shows a loading state on the confirm button); replies with `ui-result`. |
| `finder.pick(pluginId, pickerId)` | intercepts `POST /api/v1/system/capabilities/finder` (or the legacy `/api/v1/system/picker`) and bridges via parent-window `postMessage`, waiting for the workbench Finder selection (10-minute timeout). |
| `context(pluginId, keys)` | `GET /api/v1/system/capabilities/context` |
| `desktop.open(pluginId, target)` | `POST /api/v1/system/capabilities/desktop/open` |
| `clipboard.read(pluginId)` / `clipboard.write(pluginId, text)` | `GET/POST /api/v1/system/capabilities/clipboard` |
| `notification.send(pluginId, title, message)` | `POST /api/v1/system/capabilities/notification` |
| `info(pluginId)` | `GET /api/v1/system/capabilities/info` |
| `network.fetch(pluginId, url, method)` | `POST /api/v1/system/capabilities/network/fetch` |

`ALXHost.network.fetch` only offers GET/HEAD with at most 2 MiB of response, for status or metadata; **large files must be fetched by the plugin runner through the download broker** (streaming progress, cancellation, retries, and caching).

### Finder bridge messages

When a plugin page starts a Finder selection, the SDK intercepts the corresponding `fetch` and sends to the parent window:

```js
{ source: 'alx-setup-plugin', type: 'finder-request', requestId, pluginId, pickerId }
```

After the workbench renders the Web Finder, it replies:

```js
{ source: 'alx-parent', type: 'finder-result', requestId, paths: [...] }   // success
{ source: 'alx-parent', type: 'finder-result', requestId, error: '已取消选择。' } // failure/cancel
```

## Host WebView and global UI components

Plugin pages do not need to implement their own window chrome or overlay components. The host provides two capabilities directly so that all system plugin interactions stay consistent:

### webview.open: host-managed WebView windows

Plugins do not need their own webview. `webview.open` validates the parameters, then asks the workbench to open a standard floating window (draggable, resizable, minimizable, maximizable - behaving like built-in windows) containing an iframe; multiple WebViews cascade automatically to avoid exact overlap:

```js
// Open a static page from the plugin's own web root (same-origin, ALXHost auto-injected)
const opened = await window.ALXHost.webview.open('my-status', {
  title: '详细状态',
  resource: 'pages/detail.html',
  width: 900,
  height: 640
})

// Open an external HTTP(S) address (third-party site; no host capabilities)
await window.ALXHost.webview.open('my-status', {
  title: '官方文档',
  url: 'https://docs.example.com/'
})

// Close the window just opened (no id closes all WebViews opened by the plugin)
await window.ALXHost.webview.close('my-status', opened.id)
```

Constraints and details:

- Each plugin can have at most **8 WebViews** open at once; beyond that the Promise rejects immediately with `{ ok: false, error }`.
- Static-page WebViews automatically carry the current theme parameter (`?theme=light|dark`), matching the plugin home page; the window subtitle shows the actual address.
- All plugin WebViews get `--alemonjs-*` theme variables injected by the host (the full `docs/theme.json` contract, including dark overrides under `[data-theme='dark']`), so pages can use `var(--alemonjs-primary-bg)` and similar without shipping their own theme color copies.
- Opening the same resource (by address after stripping the theme parameter) restores the window's previous position and size; position memory lives in browser local storage.

Parameters:

| Field | Description |
| --- | --- |
| `title` | Window title; defaults to the plugin name. |
| `url` | External address; only `http`/`https` allowed, must not contain userinfo (username:password); mutually exclusive with `resource`. |
| `resource` | Relative path inside the plugin `web.root` (e.g. `pages/detail.html`, query strings allowed like `pages/detail.html?tab=1`); absolute paths, backslashes, and any `..` segments are forbidden. The host validates the resource **actually exists** (extension-less paths fall back to `index.html` for SPA); static pages reuse plugin hosting (including CSP and `ALXHost` injection). |
| `width` / `height` | Initial size; the host clamps to 480-1600 x 420-1200, defaulting to 960 x 680. |

`webview.open` resolves to `{ ok: true, id }` (success) or `{ ok: false, error }` (host rejection/timeout); `webview.close` resolves to `{ ok: true, closed }` (number closed). Windows can also be closed manually by the user.

Nesting behavior: same-origin static pages inside a WebView also have `ALXHost` and can call `ui.*` and `finder.pick` (the host renders them in the parent workbench and delivers the receipt to the corresponding iframe); however, **opening/closing WebViews is limited to the plugin home page** - calls from a WebView page immediately get `{ ok: false, error: '仅插件主页可管理 WebView。' }`.

### ui.*: host global frontend components

For interaction consistency, Alert, Message, Modal, and Notification are all rendered by the host; plugins only pass data instead of implementing their own set:

```js
await window.ALXHost.ui.alert('my-status', {
  title: '环境未就绪',
  message: '请先安装 Node.js 18+ 再继续。',
  confirmText: '知道了'
})

const { confirmed } = await window.ALXHost.ui.modal('my-status', {
  title: '确认删除',
  message: '该操作会移除已保存的配置，且无法撤销。',
  confirmText: '删除',
  cancelText: '取消'
})

// When an async operation needs waiting, mark the confirmation dialog busy:
const pending = window.ALXHost.ui.modal('my-status', {
  title: '系统授权',
  message: '即将执行系统操作。',
  confirmText: '执行'
})
await window.ALXHost.ui.setBusy('my-status', true) // dialog disabled, waiting for auth preflight
// ... async preparation ...
await window.ALXHost.ui.setBusy('my-status', false) // confirmation re-enabled
const { confirmed } = await pending

await window.ALXHost.ui.message('my-status', {
  type: 'success',
  title: '已保存',
  message: '配置已写入工作区。',
  duration: 3000
})

await window.ALXHost.ui.notification('my-status', {
  type: 'warning',
  title: '端口被占用',
  message: '17117 已被其他进程使用。'
})
```

Component parameters and receipts:

| Component | Parameters | Promise result |
| --- | --- | --- |
| `ui.alert` | `title`, `message`, `confirmText` | any close resolves `{ ok: true }` |
| `ui.modal` | `title`, `message`, `confirmText`, `cancelText` | `{ ok: true, confirmed: true \| false }` |
| `ui.message` | `type` (`info`/`success`/`warning`/`error`), `title`, `message`, `duration` (ms, clamped 1500-15000, default 4000) | resolves `{ ok: true }` immediately; briefly shown top-center |
| `ui.notification` | same as `message` (default 6000) | resolves `{ ok: true }` immediately; bottom-right notification |

The host renders these components (`Modal`/`ConfirmDialog`/toast) in the parent workbench of the plugin iframe, so visuals, shortcuts, and system modal stacking match the workbench exactly; at most 5 messages/notifications of the same type are kept at once, dropping the oldest when exceeded. `ui.*` is in-app; OS-level notifications still use `notification.send`.

`ui.alert` and `ui.modal` are **queued for display** in request order (queue cap 8; beyond that, immediate `{ ok: false, error }` rejection): when a plugin pops several confirmation dialogs in a row, the next one waits until the previous closes, and each Promise resolves in order when its dialog closes; `ui.message`/`ui.notification` do not queue and display immediately.

Interaction details: both `ui.alert` and `ui.modal` support **Escape to close** (equivalent to cancel); `ui.alert` uses `alertdialog` semantics and auto-focuses the confirm button; `ui.modal` reuses the workbench `ConfirmDialog` (including click-outside-to-cancel). `ui.setBusy` only affects the **currently displayed** confirmation dialog (queue head): while busy, confirm, cancel, overlay click, and Escape are all disabled and the confirm button shows a loading state, preventing accidental dismissal during async authorization.

Timeouts: host receipts for `webview.*` and `ui.*` wait **60 seconds** (returns `{ ok: false, error }` if the parent window does not respond); `finder.pick` stays open for 10 minutes (waiting for user selection). Plugins should not rely on timeouts as a normal path.

## Local services

A plugin can declare one or more **loopback HTTP services** (e.g. an embedded WebUI); the host exposes them through an authenticated same-origin proxy, and browsers never access the host/port directly:

```json
"services": [{
  "id": "webui",
  "name": "管理界面",
  "host": "127.0.0.1",
  "port": 8080,
  "basePath": "/",
  "healthPath": "/health",
  "embed": true,
  "rewriteHtml": true,
  "rewriteApiBase": true,
  "sse": true,
  "websocket": true
}]
```

| Field | Description |
| --- | --- |
| `id`/`name` | Service identifier and display name. |
| `host`/`port` | Loopback address; browsers cannot specify it, only the manifest can declare it (or a dynamic port proxy). |
| `basePath`/`healthPath` | Mounted sub-path and health check path. |
| `embed` | Whether the service is suitable for iframe embedding. |
| `rewriteHtml` | Rewrite HTML when proxying (inject an API-compatible script). |
| `rewriteApiBase` | Rewrite root-relative `/api/...` requests in the page to the service mount point, tolerating "different listening port" redirects. |
| `sse`/`websocket` | Allow SSE / WebSocket upgrades. |

Service list and reachability come from `GET /api/v1/services?plugin=<id>`; the proxy entry is `/api/v1/services/<plugin>/<service>/...`. **Dynamic port services** (e.g. Docker published ports that cannot be statically declared in the manifest) use `/api/v1/services/dynamic/<plugin>/<port>/...` with the target forced to `127.0.0.1`, validated the same way as manifest services.

## Uploads

Browser files do not belong in JSON parameters. After a plugin declares upload actions in the manifest, the web UI sends `multipart/form-data` to `POST /api/v1/setup/plugins/<pluginId>/upload`:

- `action`: manifest `uploads[].action` (can be omitted when the manifest has only one upload action);
- `destination`: absolute target directory;
- `files` (or `file`): one or more file fields.

```json
"uploads": [{ "action": "upload", "maxBytes": 2147483648 }]
```

Host behavior: enforce the total size against `maxBytes` (request body cap 2 GiB + 1 MiB), stream to a temporary directory (basename/length/duplicate validation on filenames), run the corresponding runner action with `params: { stagingDir, destination }`, and clean up the staging content when the action finishes. **Plugins cannot treat arbitrary action parameters as local files**; uploads go through this dedicated channel.

## Privileged operations and audit

Operations that need system authorization are declared through `privilegedOperations`. The host only provides **authorization, process startup, progress, and audit**; action semantics and platform details belong to the plugin, and the host does not validate the runner's internal behavior.

```json
"privilegedOperations": [{
  "action": "firewall-set",
  "runnerAction": "firewall-set",
  "planAction": "plan",
  "title": "启用或停用系统防火墙",
  "description": "网络插件将按已确认的变更计划启用或停用系统防火墙。",
  "authorization": "native",
  "platforms": ["linux", "windows"]
}]
```

| Field | Description |
| --- | --- |
| `action` | Action name called by the console page. |
| `runnerAction` | Actual action name passed to the runner (the host calls it after authorization). |
| `authorization` | `password` (workbench sudo password) or `native` (system native authorization). |
| `platforms` | Platforms where authorization applies. |
| `commands` | `password` mode only: fixed commands the host looks up in PATH and executes (structured `program`/`args`, not through a shell). |
| `planAction` | Optional: run a "plan" action first to produce a change plan (stored by the host), then execute per the plan after confirmation. |
| `useLatestAudit` | Optional: instead of carrying a plan, use the most recent audit's changes as the undo/redo target. |

### Authorization flow

```text
Web UI ── POST /system/privileged/preflight {pluginId, action, planId?}
    ← {available, authorization, title, description, intentId, expiresAt, sourceType, sourcePath}
Web UI ── POST /setup/plugins/<id>/actions
    {action, confirm: true, authorizationId, sudoPassword?}   // sudoPassword for password mode
Host: validate the one-time intent (bound to plugin/action/plan/account/source, 5-minute expiry)
    → password: run the manifest's fixed command (sudo); native: run runnerAction after UAC/Polkit/native authorization
    → on success, append to the chained-hash audit (privilege_audit)
```

Notes:

- `native` mode platform mapping: darwin → `native`, windows → `native-uac`, linux → `polkit`.
- `password` mode only allows super admins when authentication is enabled; the password is zeroed in memory immediately, and failed attempts have lockout records.
- `planAction`/`useLatestAudit` operations accept no extra browser parameters, only the host-issued `planID` or the latest audit; at execution the host passes `{ operation, ...planParams, __alxFingerprint? }` to the runner.
- Privileged mode is controlled by `ALX_PRIVILEGED_MODE` (`enabled`/`local`); `local` is restricted to the local loopback and ignores forwarded headers.
- The audit table uses a hash chain (`previous_hash`/`chain_hash`) to prevent tampering; `GET /api/v1/system/privileged/audit?plugin=<id>` reads it, and `privileged/status` returns chain integrity and signature version.
- The **password input** in the privileged dialog and the **structured change-plan display** (risk/impact/parameters/verification) are drawn by the plugin page itself; the host `ui.modal` only provides title + body, without input fields or rich text - an intentionally preserved boundary.

## Source development sessions

Source development registers a source directory in the Plugin Center's "Develop plugin" entry (constrained to managed directories through the workbench Finder); its declared local commands only run when you click "Start development". A source session temporarily overrides the official Release with the same ID; stopping restores the Release immediately; after a workbench restart, source commands are **never run automatically**. Source development only works in the local desktop workbench.

```json
{
  "development": {
    "runtime": "go",
    "entry": { "go": "runner/main.go" },
    "web": {
      "mode": "dev-server",
      "root": "web",
      "build": { "program": "yarn", "args": ["--cwd", "frontend", "build"] },
      "dev": { "program": "yarn", "args": ["--cwd", "frontend", "dev", "--host", "127.0.0.1", "--port", "${ALX_PLUGIN_DEV_PORT}"] },
      "healthPath": "/",
      "hmr": true
    },
    "services": [
      { "id": "api", "command": { "program": "go", "args": ["run", "./runner"] }, "restart": "on-failure" }
    ]
  }
}
```

Key points:

- `runtime` can be `go`/`node`/`command`; commands use structured `program` + `args` and never go through a shell; `${ALX_PLUGIN_DEV_PORT}` is the only replaceable variable.
- `web.mode`: `static` (prefers `development.web.root`, falling back to the top-level `web.root` when undeclared) or `dev-server` (the host starts it and proxies same-origin; health is checked via `healthPath`); HMR WebSocket is only allowed with `hmr: true`, and no extra network ports are exposed.
- `development.services` are managed commands for services already declared in the manifest, with `never`/`on-failure` restart policies; service host/port stays inside the service whitelist, and development commands cannot proxy elsewhere.
- Session state machine: `registered → starting → running → stopping → stopped`, plus `building`/`failed`; only one operation is allowed per session at a time.

Source development APIs:

| Method | Path | Description |
| --- | --- | --- |
| GET | `/api/v1/setup/plugins/development` | List registered source sessions. |
| POST | `/api/v1/setup/plugins/development` | Register a source directory `{path}` (chosen via Finder). |
| POST | `/api/v1/setup/plugins/development/<id>/start` | Start (requires `{confirm:true}`). |
| POST | `/api/v1/setup/plugins/development/<id>/stop` | Stop (restores the Release only after the managed process group exits). |
| POST | `/api/v1/setup/plugins/development/<id>/restart` | Restart (requires confirmation). |
| POST | `/api/v1/setup/plugins/development/<id>/build` | Build the frontend (requires confirmation). |
| GET | `/api/v1/setup/plugins/development/<id>/logs` | Session logs (cap 256 KiB; head truncated beyond that). |
| GET | `/api/v1/setup/plugins/development/<id>/services/<sid>/logs` | Service logs. |
| POST | `/api/v1/setup/plugins/development/<id>/services/<sid>/restart` | Restart a service (requires confirmation). |
| DELETE | `/api/v1/setup/plugins/development/<id>/remove` | Remove the registration (does not run commands). |
| GET | `/api/v1/setup/plugins/development/<id>/web/...` | Development web proxy (HTML gets the host bridge injected; non-HTML is proxied as-is). |

Source plugins can also request `privilegedOperations`; the workbench clearly marks "source development authorization" with the source directory and still requires a one-time admin confirmation and the current host permission mode.

## System features and components

System plugins are not isolated scripts; they integrate with the workbench's whole "windowed" UI. Below are the plugin-related system features and components.

### System windows

The workbench hosts system features in desktop windows ([DesktopWindow.tsx](../../frontend/src/components/DesktopWindow.tsx) and [SidebarWindow.tsx](../../frontend/src/components/SidebarWindow.tsx)). System features fall into two categories:

- **Built-in features**: `plugins` (Plugin Center), `ops-overview` (Ops), `tasks` (Tasks), `environment` (Environment Check), `accounts` (Accounts). Plugins/ops/tasks/environment use the sidebar `SidebarWindow`; the rest use a plain `DesktopWindow`.
- **Plugin windows**: every installed plugin gets a `setup:<pluginId>` window whose title comes from the plugin `name`, icon from `navigation.icon`, and content is the console page (same-origin iframe).

### Plugin Center (SystemPluginCenter)

The workbench "Plugins" feature has three views:

- **Marketplace**: online curated catalog (`/setup/plugins/market`) listing Releases and assets; installable.
- **Mine**: local plugin list with enable/disable, uninstall, version switch, cached version deletion, cache cleanup, and local archive upload/install.
- **Develop**: register/start/stop/restart/build source sessions and view session and service logs.

### Console page (SetupPluginCenter)

The plugin window embeds the console page:

- Web URL: `/api/v1/setup/plugins/web/<id>/index.html?theme=light|dark` (for source dev-server sessions: `/api/v1/setup/plugins/development/<id>/web/`); the host injects scrollbar theming.
- Finder bridge: when the page sends a `finder-request` message, the host opens the unified Web Finder (`DirectoryPicker`) and sends the result back to the iframe.
- Plugins without a `web` UI or not installed show a placeholder (online-recognized plugins must be installed first).

### Global function bar and icons

Manifest `navigation` puts the plugin into the workbench's global function bar: `{ label, icon, order }`, sorted by `order` ascending then by name. The frontend maps these lucide icon names; unmatched names fall back to a single character or the default plugin icon:

`network`, `forward`/`forwarding`, `interface`, `lan`, `wifi`, `route`, `dns`/`mirror`/`proxy`, `firewall`/`shield`, `port`, `traffic`.

### Other host system components

- **Capability catalog**: `/api/v1/system/capabilities` returns availability and unavailability reasons per capability; the frontend uses it for degradation hints.
- **Host WebView windows**: windows opened by a plugin via `webview.open` reuse the `DesktopWindow` shell (drag/resize/minimize/maximize) and are rendered by `PluginHostUi`.
- **Global UI components**: `ui.alert`/`ui.message`/`ui.modal`/`ui.notification` are rendered uniformly by the parent workbench (reusing `Modal`, `ConfirmDialog`, and toast); plugin pages only pass data, keeping interaction consistent.
- **Privileged status and audit**: `/api/v1/system/privileged/status` and `/audit` show privileged mode and the audit chain; the frontend calls `preflight` before triggering privileged operations to show title, description, and source type (release/development).
- **Version and cache management**: version list, switch, delete, and cache cleanup UIs are all built on `/setup/plugins/<id>/versions`, `/switch`, `/cache`, etc.
- **Download cache management**: the broker's user-directory download cache (global 1 GiB) can be queried and cleaned.
- **Event-driven refresh**: plugin set changes are pushed via `/setup/plugins/events` (SSE) and the global event stream (`topics=plugins`); the frontend refreshes lists automatically.

### Discovery channels

Plugins are discoverable in the UI, the CLI (`alx plugin list`; lists plugins and their web entries without executing plugin code), and MCP (`alemonjs_list_setup_plugins`).

## Publishing, installation, and version management

System plugins must publish platform archives through GitHub Releases; source repositories are not used as installers. Publish one archive per supported platform:

```text
my-plugin-darwin-arm64.zip
my-plugin-darwin-amd64.zip
my-plugin-linux-amd64.zip
my-plugin-linux-arm64.zip
my-plugin-linux-armv7.zip
my-plugin-windows-amd64.zip
my-plugin-windows-arm64.zip
```

Each archive contains the plugin directory, `alx.json`, `web/`, and the `dist/` runner for the current platform. Releases should also publish `SHA256SUMS`. CI should write the version into `alx.json.version` from the Git tag, so a `v1.2.3` Release must have manifest version `1.2.3`.

ALemonX writes the Release tag, asset name, archive SHA-256, and install time into `.alx-install.json` in the plugin directory and derives an install fingerprint from it. The actual version of an installed plugin is the Release tag in that file; only when a source directory has no install fingerprint does `alx.json.version` serve as the development fallback.

ALemonX keeps verified Release archives and extracted versions under `alx/plugin-cache/<plugin-id>/` in the user config directory. By default each plugin keeps at most 3 versions and the global cache cap is 1 GiB; cleanup follows least-recently-used but never deletes the currently active version. The workbench can switch versions, delete inactive versions, or run an immediate cache cleanup from the version management UI.

Limits and timeouts: archives up to 300 MiB; installation uses the long-connection transfer path (up to 60 minutes), with 3 download attempts of at most 15 minutes each; metadata requests keep short timeouts. **Downloading only puts a verified Release on disk - it loads only after explicit enablement**, and the whole path from download to enable is visible and reversible.

## Security boundary and development guidelines

Hosting and execution:

- Console pages are only served for **installed and enabled** plugins; online-recognized or uninstalled plugins return 404. Hosting only serves regular files inside `web.root`, rejects path traversal and symlink escapes, and sets `X-Content-Type-Options` plus CSP (`frame-ancestors 'self'`, `base-uri 'none'`, etc.).
- `entry` must be a regular file inside the plugin directory; absolute paths, `..` escapes, and symlinks are forbidden.
- Runner environment variables are controlled by the host; downloaded plugins cannot obtain credentials or network policy by editing the manifest. The download broker does not forward workbench cookies, Authorization, or internal identity headers, nor does it expose proxy credentials to plugins.
- Privileged operations only accept host-issued intents/plans; browsers cannot choose executables or commands; `password` mode only executes fixed commands findable in PATH per the manifest, and passwords are zeroed after use.

Development guidelines:

- Implement every operation as a fixed action branch; never concatenate field values into shell strings or execute user-supplied commands.
- Validate inputs and the runtime environment inside the runner for dangerous operations; the console page provides second confirmation.
- Run with least privilege; clearly prompt the user when elevation is needed and handle authorization cancellation.
- Do not read, upload, or echo credentials, private keys, or tokens in output.
- Provide binaries for each release platform/architecture and test failure paths in clean environments: missing runtime, authorization cancellation, invalid input, etc.
- Make sure executables have the execute bit; Windows files should end in `.exe`.
- **Do not host plugins from untrusted sources** - installing one means trusting it to run system commands and host a same-origin page.

During repository development, at minimum run:

```bash
go test ./internal/setupplugin/...
go vet ./internal/setupplugin/...
```

## Reference implementations

This repository does not currently ship system-plugin examples in its source tree. Build plugins in separate repositories using this document's manifest, runner protocol, and Release-packaging requirements. A frontend can use React, Vite, and Tailwind with build output in `web/`; local `yarn dev` can proxy to alx.
