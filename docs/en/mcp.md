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

For clients with a graphical "Streamable HTTP" form, first set a random `MCP_TOKEN` and start the protected loopback service:

```bash
MCP_TOKEN='<strong-random-value>' alx --mcp-port 17391 mcp-http
```

Fill in the form:

| Field | Value |
| --- | --- |
| Name | `alemonx` |
| Type | Streamable HTTP |
| URL | `http://127.0.0.1:17391/mcp` |
| Auth | `Bearer <MCP_TOKEN>` |

The endpoint implements the Streamable HTTP `POST /mcp` request/response mode; standalone SSE push streams do not apply to this control plane and `GET /mcp` is rejected with `405` per the spec. It validates `Origin`, protocol version, and the Bearer token, and never listens on LAN or public addresses.

Optionally use `MCP_ALLOWED_ROOTS` to restrict the agent to specific workspaces; join multiple paths with the OS path separator (`:` on macOS/Linux, `;` on Windows):

```bash
MCP_ALLOWED_ROOTS='/Users/me/robots:/Users/me/workspaces' alx mcp
```

Once configured, project read/write, task operations, and new project creation are all validated server-side to be inside these roots.

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
