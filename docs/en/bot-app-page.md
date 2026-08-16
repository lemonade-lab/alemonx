# ALemonX Bot App Page Specification

> 中文版：[../bot-app-page.md](../bot-app-page.md)。

Setup embeds the current robot's plugin pages into the backend. A bot app page is the plugin's own frontend - not a Setup management page - and never receives system commands, filesystem access, or the Setup login state.

Bot app pages and system plugin **console pages** are two different mechanisms: an app page gets `window.__alxWebview` injected and can only reach the current robot's `./api/*`; a console page gets `ALXHost` and can call host capabilities and action forwarding. For system plugins, see [System plugin development and integration](plugin-development.md).

## Registration

The plugin's `package.json` must declare both a page directory and at least one sidebar entry:

```json
{
  "name": "example-plugin",
  "alemonjs": {
    "web": { "root": "dist" },
    "desktop": { "sidebars": [{ "name": "示例插件" }] }
  }
}
```

`root` must be a relative path inside the package directory and must contain an `index.html`. Setup only scans packages in the robot's local `packages` directory plus dependencies explicitly declared in the robot's `package.json`.

## Build requirements

- Output a complete static site into `dist`.
- Prefer relative asset paths such as `assets/index.js`; do not depend on the site root.
- Setup already tolerates common Vite patterns like `/assets`, `favicon.ico`, and CSS `url(/assets/...)`, but dynamically concatenated root-absolute paths are not guaranteed to work.
- Pages may only use their own same-origin resources; external CDNs and external APIs may be blocked by browser security policy.

## Theme variables

The host injects the full `--alemonjs-*` CSS variable contract into app pages (see `docs/theme.json`): light values are defined directly on `:root`, dark values override the same variables under `[data-theme='dark']`, and `--alemonjs-dark-*` always keeps the dark values. App pages do not need their own theme color copies; use `var(--alemonjs-primary-bg)`, `var(--alemonjs-dark-primary-bg)`, and so on to follow the host theme.

## Available capabilities

```ts
window.__alxWebview.context // { package, name }
window.__alxWebview.postMessage(value)
window.__alxWebview.onMessage(listener)
window.__alxWebview.request('./api/example', options)
```

`request` only accepts paths starting with `./api/` and only proxies to the current robot app. `postMessage` starts the directory-local `alemonjs/desktop.js` desk Node.js process on first call and then communicates bidirectionally with the plugin desktop module over stdin/stdout JSON IPC; it does not start the robot's `app/dev`. `appDesktopAPI.postMessage`, `appDesktopAPI.onMessage`, and `appDesktopAPI.themeOn` have minimal compatibility implementations and must not be treated as the full Wails API of AlemonDesk.

## Isolation and lifecycle

- Each robot uses its own loopback `*.localhost` origin and does not share plugin storage, cookies, or Setup backend state.
- Switching tabs within the same robot keeps the iframe; switching to another robot unloads the page to save resources and reloads it when you return.
- Plugins must restore page data themselves using localStorage, server state, or another persistence scheme.
- Opening the page does not start the robot. The robot HTTP API is only an optional data source for the page; when the robot is not running, the page should show a connection-unavailable state on its own. Process communication between the app page and the plugin backend is handled by a separate desk runtime.
- Plugins cannot access Setup files, terminals, PM2, system commands, or other robot directories.
