# ALemonX - Local Workbench for Robots

> Create, run, manage, and extend AlemonJS robots from a local workbench, and let an AI Agent help maintain projects with your confirmation.

English · [中文](README.md)

[User manual](docs/en/user-manual.md) · [One-line install](#one-line-install) · [Docker deployment](docs/en/docker.md) · [Command line](docs/en/cli.md) · [MCP](docs/en/mcp.md) · [System plugin development](docs/en/plugin-development.md)

![ALemonX workbench: multi-robot project management, run configuration, and Agent collaboration](docs/images/alemonx-workbench.png)

## Directory layout

```text
dist/                     Frontend build output (embedded into the alx binary at build time, not part of the runtime layout)
resources/                Runtime resources (embedded into the alx binary)
├── templates/            Project template sources (bot/, dev/)
└── packages/             Bundled tooling (Yarn embedded; tools such as PM2 are installed on demand via Yarn)
workspace/                Unified workspace (default <run-dir>/workspace; override with --workspace or ALX_WORKSPACE)
├── templates/            Project templates (materialized from embedded templates on first run; editable)
├── packages/             Tooling directory (Yarn materialized copies; PM2 is installed here on first use and stays stable)
└── bots/                 Default destination for newly created robots
```

## One-line install

On macOS, Linux, or FreeBSD, run in a terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh
```

On Windows, run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

### Users in mainland China

`raw.githubusercontent.com` and GitHub Releases can be slow or unreliable there. Fetch the script through a mirror and ask the installer to prefer mirror download sources:

macOS, Linux, or FreeBSD:

```sh
curl -fsSL https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | ALX_PREFER_MIRROR=1 sh
```

Windows PowerShell:

```powershell
$env:ALX_PREFER_MIRROR='1'; irm https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
```

If `ghfast.top` is unavailable, replace it with `ghproxy.net` or `gh-proxy.com`. The installer also honors the `ALX_DOWNLOAD_BASE` environment variable for self-hosted mirrors, and automatically falls back to the next source when the current one fails.

### Manual install

To install manually, pick the archive for your system from [GitHub Releases](https://github.com/lemonade-lab/alemonx/releases):

| System                          | Archive                          |
| ------------------------------- | -------------------------------- |
| Windows x64                     | `alx-windows-amd64.zip`          |
| Windows ARM64                   | `alx-windows-arm64.zip`          |
| Windows 32-bit                  | `alx-windows-386.zip`            |
| macOS Apple Silicon             | `alx-darwin-arm64.zip`           |
| macOS Intel                     | `alx-darwin-amd64.zip`           |
| Linux x64                       | `alx-linux-amd64.zip`            |
| Linux ARM64                     | `alx-linux-arm64.zip`            |
| Linux ARMv7                     | `alx-linux-armv7.zip`            |
| Linux 32-bit x86                | `alx-linux-386.zip`              |
| Linux ppc64le / s390x / riscv64 | matching `alx-linux-<arch>.zip`  |
| FreeBSD x64 / ARM64             | matching `alx-freebsd-<arch>.zip`|

Run `alx.exe` directly on Windows. On macOS / Linux / FreeBSD:

```bash
chmod +x alx
./alx
```

If downloads are slow in mainland China, prefix the GitHub URL with a mirror, e.g. `https://ghfast.top/https://github.com/lemonade-lab/alemonx/releases/latest`.

## Docker deployment

See [Docker deployment docs](docs/en/docker.md) for persistence, authentication, updates, and the security boundary.

## Human-in-the-loop operations

The full capability model and permission boundaries are described in the [MCP docs](docs/en/mcp.md).

## Documentation

- [User manual](docs/en/user-manual.md)
- [MCP control plane](docs/en/mcp.md)
- [Docker deployment](docs/en/docker.md)
- [Command line](docs/en/cli.md)
- [System plugin development](docs/en/plugin-development.md)
- [Bot app page specification](docs/en/bot-app-page.md)
