# ALemonX MCP Control Plane

> 中文版：[../mcp.md](../mcp.md)。

ALemonX supports both standard MCP transports: **stdio** and **Streamable HTTP**. Both use JSON-RPC 2.0 and expose the same restricted set of Setup control tools to local AI clients such as Codex and Doubao.

## Client compatibility and verification scope

| Client type | Recommended mode | What this project guarantees | Still required on the client side |
| --- | --- | --- | --- |
| Codex desktop | STDIO; Streamable HTTP form also works | Coverage of `initialize`, `notifications/initialized`, tool discovery, invocation, resource discovery/read, and both transports | Make sure `alx` is on a PATH Codex can launch; fill in the Token for HTTP. |
| Doubao and similar desktop agents | Standard MCP command or HTTP form provided by the product | Same JSON-RPC, tools, and permission boundary as Codex | Select STDIO or Streamable HTTP in the MCP settings; the product's authorization UI is the client's responsibility. |
| Other MCP clients | STDIO or Streamable HTTP | No vendor-private APIs; tool schemas, annotations, text results, and structuredContent are all standard MCP data | The client must support MCP 2025-06-18 or a compatible version and show real user confirmation. |

"Support" means protocol interoperability, not that any third-party client bypasses its own account, admin policy, network policy, or human-confirmation mechanisms. Before starting, you can run `command -v alx` to confirm a stdio client can find the binary.

## Connection modes

### STDIO (recommended)

`alx mcp` is launched by the AI client as a local child process; it does not reuse the browser session and does not listen on a network port. In Codex's "Connect to custom MCP" form:

| Field | Value |
| --- | --- |
| Name | `alemonx` |
| Type | `STDIO` |
| Launch command | `alx` |
| Arguments | `mcp` |
| Environment (optional) | `MCP_ALLOWED_ROOTS=/path/to/your/robot` |

### Streamable HTTP

In **Settings → MCP**, a super administrator starts the HTTP service when workbench authentication is enabled. The workbench restores the HTTP listener on restart if it was last started manually. A manual stop persists across restarts. Each signed-in user can choose **Create Connection Key** and enter the displayed address and their own key in the client’s Streamable HTTP form.

Each account has one key for all tools within its account scope. Existing operator keys remain valid. Legacy diagnostic keys expire. Use the address shown in Settings. For remote access, the HTTPS proxy must preserve the `/mcp` path and `Authorization` header. The workbench endpoint requires account authentication.

| Field | Value |
| --- | --- |
| Name | `alemonx` |
| Type | Streamable HTTP |
| URL | HTTPS workbench URL plus `/mcp`, or `http://127.0.0.1:17391/mcp` locally |
| Auth | `Bearer <connection token>` |

The endpoint implements the Streamable HTTP `POST /mcp` request/response mode; standalone SSE push streams do not apply to this control plane and `GET /mcp` is rejected with `405` per the spec. It validates `Origin`, protocol version, and the Bearer token. The dedicated listener remains bound to loopback.

Optionally use `MCP_ALLOWED_ROOTS` to restrict the agent to specific workspaces; join multiple paths with the OS path separator (`:` on macOS/Linux, `;` on Windows):

```bash
MCP_ALLOWED_ROOTS='/Users/me/robots:/Users/me/workspaces' alx mcp
```

Once configured, project read/write, task operations, and new project creation are all validated server-side to be inside these roots.

## Account robot list

The workbench stores each account's robot folders, pinned order, and current selection on the server. An older browser list can be imported from the empty workspace screen. When account authentication is disabled, the workbench uses a local list for the current workspace.

Use `alemonjs_list_robots` to find a robot, then `alemonjs_list_robot_backpack` for its packages and `alemonjs_get_robot_run_options` for launch modes and enabled plugins. Use `alemonjs_manage_robot_plugin` to install official Git plugins or uninstall a backpack package, and `alemonjs_set_robot_plugin_enabled` to change what loads at the next startup. `alemonjs_control_robot_runtime` starts or stops the robot, and `alemonjs_get_robot_logs` returns local or PM2 logs.

Workbench HTTP keys are scoped to the account that creates them and its robot list. The service log records the account, tool and result in `mcp.tool.called`. Keys survive workbench logout and service restarts. Users can revoke or regenerate their own key at any time. Revocation or regeneration immediately invalidates the old key. Only a key digest is stored, and the plaintext is shown when a key is created or regenerated. Keys issued before this change must be recreated. Workbench HTTP shares runtime state with the UI and supports development, foreground, and PM2. CLI connections support development and PM2. When account authentication is enabled, only a super administrator can start or stop Workbench HTTP or launch CLI MCP with `ALX_AUTH_TOKEN`.

### Backpack maintenance

| Operation | MCP tool |
| --- | --- |
| Inspect status, README, local changes, a diff, or stashes | `alemonjs_get_backpack_plugin_details` |
| List versions and check the local Git state | `alemonjs_list_backpack_plugin_versions` |
| Safely update one or more plugins | `alemonjs_update_backpack_plugins` |
| Refresh versions or history, configure Git, switch versions, stash before switching, restore or drop stashes | `alemonjs_manage_backpack_plugin_version` |
| Import a local folder or ZIP/TGZ/TAR.GZ archive | `alemonjs_import_backpack_plugin` |
| Clone a Git repository with a branch, directory name, mirror, and depth | `alemonjs_clone_backpack_plugin` |
| Set plugin states in bulk, list and change load order | `alemonjs_set_backpack_plugins_enabled`, `alemonjs_get_backpack_plugin_order`, `alemonjs_move_backpack_plugin` |
| Read and set the `packages/*` workspace switch | `alemonjs_get_backpack_workspace`, `alemonjs_set_backpack_workspace` |

Git version queries use previously fetched refs. Use `refresh-versions` after confirmation to fetch the latest refs, or `refresh-history` to expand a shallow checkout. Safe update skips dirty or ahead Git worktrees and non-Git plugins whose local edits cannot be verified. After confirmation, `switch` can select a specific npm version. For Git, `force-switch` discards edits or ahead commits, while `stash-switch` preserves edits in a stash and ahead commits in a backup branch.

Import paths are relative to the robot directory on the server, for example `imports/my-plugin.zip`. Clients can also send an archive of up to 700 KiB as `archiveBase64`. Imports refuse to overwrite existing plugins. Writes require `confirm: true`. Version maintenance, safe updates, imports, clones, and workspace changes return task IDs for `alemonjs_get_project_task`. Send native JSON values when saving plugin configuration.

## Capability model

| Layer | MCP capability | Purpose |
| --- | --- | --- |
| Context | `resources/list`, `resources/read` | Fetch `alemonjs://mcp/capabilities` to learn the available scope and boundaries first. |
| System checks | Environment, updates, version, official ecosystem catalog | Give the agent current facts before creating, installing, or publishing. |
| Read-only | Project status, file listing, reading source, local package list | Let the agent inspect before deciding whether to modify. |
| Modify | Writing project files, creating projects | Every call must include `confirm: true`. |
| Async operations | Start project operations, query tasks, list tasks | Install, build, and Git operations do not block the MCP connection. |

All tools return both a text result and `structuredContent`, so clients can show results to the model and read fields reliably.

The `alemonjs://mcp/capabilities` resource returns the workspace paths (`workspace.root`, `workspace.templates`, `workspace.bots`). When `alemonjs_create_project` is called without a custom save location, new projects are created in `<workspace>/bots/<project-name>` by default.

## Mapping to Setup robot management

| Setup capability | MCP tool |
| --- | --- |
| Dependency check, install, build | `alemonjs_project_status`, `alemonjs_start_project_action` (`install`, `build`) |
| Development and background robot lifecycle | `alemonjs_start_project_action` (`dev`, `pm2`, `pm2-status`, `pm2-stop`) and `alemonjs_stop_development` |
| Runtime configuration | `alemonjs_get_package_runtime_config`, `alemonjs_save_package_runtime_config` |
| Local packages and installers | `alemonjs_list_local_packages`, `alemonjs_start_project_action` (`install-package`, `uninstall-package`) |
| Package publishing info | `alemonjs_get_package_manifest`, `alemonjs_save_package_manifest` |
| NPM packaging and publishing | `alemonjs_get_npm_publish_status`, `alemonjs_get_npm_pack_preview`, `alemonjs_start_project_action` (`npm-version`, `npm-publish`) |
| Git init and packaging | `alemonjs_initialize_git`, `alemonjs_get_git_release_status`, `alemonjs_start_project_action` (`commit`, `git-release`) |
| Setup system extensions | `alemonjs_list_setup_plugins` (lists plugins and their Web entries without executing plugin code) |
| Setup system checks | `alemonjs_check_environment`, `alemonjs_check_setup_update`, `alemonjs_list_releases`, `alemonjs_list_catalog` |

`npm-publish` and `git-release` have external side effects and require the user's explicit confirmation for this run after the pre-publish checks pass; MCP never reads or forwards the npm token.

## Production diagnostics

The workbench **Runtime → Production Diagnostics** view and MCP use the same project-scoped diagnostic backend.

1. Call `alemonjs_get_robot_diagnostics` for the project version, startup checks, supervised local run, PM2 process state, configured loopback TCP ports, and recent operations. The workbench probes the observed runtime port when available, including autoPort changes, and reports the configured port separately. TCP reachability, listener ownership, application HTTP readiness, and platform connection are separate signals. Platform connection stays unknown without session evidence.
2. Filter `alemonjs_list_robot_operations` with `status: "failed"`, then call `alemonjs_get_project_task` by task ID for its timeline and redacted output. Workbench HTTP merges live tasks, durable MCP tasks, and retained journal entries and returns at most 40 recent operations per project. The journal retains up to seven days within its size limit. For asynchronous operations, Workbench HTTP records `mcp.task.accepted` before execution and `mcp.task.result` after execution. Save the task ID to look up the initiating account, robot, and outcome after a restart, even after journal expiry. If no outcome was recorded before restart, the task is marked `interrupted` with an unknown result. The workbench does not replay it.
3. For `alemonjs_get_robot_logs` in `background` mode, the default reads both stdout and stderr. Use `since`, `until`, `query`, or `page` to narrow the results. AlemonJS sends the console output from `logger.error` to stdout by default. To inspect a single stream, select `source: "out"` or `source: "err"`. The response reports the applied stream filter in `sourceFilter`. Use RFC3339 timestamps for time ranges. PM2 reads use the existing daemon or its saved process list and include attributable archives using the default `pm2-logrotate` naming convention or numbered system `logrotate` files (plain or gzip). `readScope` records the active and archived file counts and limits, as well as the scanned line count and timestamp span for the selected streams. Each query reads up to 16 files, with up to 4 MiB and 20,000 lines per file. Reads use the tail of plain files and a bounded decompressed prefix for gzip archives. `undatedExcluded` counts matching lines omitted by date or time-range filters. A true `truncated` flag indicates skipped files or content.

## Recommended agent workflow

1. Read `alemonjs://mcp/capabilities`.
2. Use `alemonjs_project_status` and `alemonjs_list_project_files` to understand the target project.
3. Read the necessary source files and propose changes with their impact.
4. After user confirmation, write files with `confirm: true` or call `alemonjs_start_project_action`.
5. Poll long operations with `alemonjs_get_project_task` until `completed` or `failed`.

## Permission boundary

The project root must contain `package.json`. MCP may manage project source and ordinary configuration, but permanently rejects:

- any host shell command;
- `.env`, `.npmrc`, private key/certificate files;
- `.git`, `node_modules`, symlinks;
- reads or writes of files larger than 1 MiB.

Development mode, NPM publishing, Git publishing, and Setup plugin operations are all controlled by explicit tools and `confirm: true`; NPM and Git publishing must also pass their preflight tools before execution. The confirmation field is emitted by the MCP client, so the client must provide a real user-confirmation UI; it must not be treated as an independent authorization system.

These constraints live in the Go service layer, not in client hints. With `MCP_ALLOWED_ROOTS` set, the server resolves real paths before comparing boundaries to avoid symlink bypasses. If remote MCP access is ever needed in the future, the HTTP adapter must sit behind a gateway with OAuth, project-scope policy, audit logging, and rate limiting; the current local controller must not be exposed directly to the public internet.

The protocol implementation follows the [MCP 2025-06-18 tool specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools): it declares tool capabilities, tool metadata, and structured results, and keeps user confirmation authority over modifying operations.
