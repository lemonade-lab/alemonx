# Docker Deployment

> 中文版：[../docker.md](../docker.md)。

The ALemonX Docker image bundles the workbench plus Node.js, Corepack, Git, and SSH clients, and can manage AlemonJS projects mounted at `/app/workspace` inside the container. It does not mount the Docker socket and never receives host sudo or desktop permissions.

## Quick start

On Linux or macOS:

```sh
mkdir alx-docker && cd alx-docker
curl -fsSLO https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh
sh docker-install.sh up
```

The script downloads `docker-compose.yml` and `.env.example`, copies them, and generates an editable `.env`. After the first start, open `http://localhost:17390` and create the administrator account from the setup guide.

Users in mainland China can fetch the deployment script through a mirror by prefixing `raw.githubusercontent.com` accordingly, e.g. `https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh`. The script supports `ALX_RAW_BASE` for mirror sources:

```sh
ALX_RAW_BASE=https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main sh docker-install.sh up
```

Without the network script, use the repository files directly:

```sh
cp .env.example .env
mkdir data workspace
docker compose up -d
```

The container's `/app` directory is separate from the persistent `/app/workspace` mount. When creating a robot project with "current directory" selected, the program automatically saves into the writable `ALEMONJS_SETUP_ROOTS` (Docker defaults it to the mounted `/app/workspace`), so projects always live in the host-persisted workspace.

## Configuration and data

The most common `.env` variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `ALX_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest` | Default Tencent Cloud image; use `alemonx:local` for local builds. |
| `ALX_PORT` | `17390` | Port exposed to the host. |
| `ALX_CONTAINER_NAME` | `alx` | Container name shown by `docker ps`; customize for multi-instance deployments. |
| `ALX_DEPLOYMENT` | `production` | Production mode: missing SQLite or local auth only prints reminders and does not block startup; an admin can be created from the setup guide. |

Runtime data is not kept in a named volume but in host directories next to the compose file:

| Host directory | Container path | Contents |
| --- | --- | --- |
| `./data` | `/data` | Workbench state: accounts, config, SQLite, download caches |
| `./workspace` | `/app/workspace` | Unified workspace: templates, robots, bundled tools, system plugins |

The unified workspace inside the container is `/app/workspace`: on first start the embedded templates are materialized into its `templates/` directory (editable and persisted on the host under `./workspace`), and new robots land in `bots/` by default. The built-in Yarn is materialized into `packages/yarn`; PM2 is not embedded in the image and is installed with the built-in Yarn into `packages/pm2` on first use (a stable location persisted on the host). System plugins installed from the workbench or `alx plugin install` are written only to `plugins/`; the image-level `/app/plugins` directory is consulted only for compatibility and has lower priority than workspace plugins. Every system plugin receives a persistent default data directory at `store/<plugin ID>/`, which survives container replacement; for example, the QQ plugin should save downloaded components, sessions, and configuration in `store/alemonx-qq/`.

The container runs as root, so the mounted directories work out of the box without any `chown`/`chmod` on the host. On macOS, the only host-side requirement is letting Docker Desktop access the deployment directory (System Settings → Privacy & Security → Files and Folders, and Docker Desktop → Settings → Resources → File sharing).

Do not switch the container to privileged for convenience, and do not mount `/var/run/docker.sock`. `docker compose down` does not delete `./data` or `./workspace`; only deleting these directories manually removes the data.

## Daily operations

```sh
sh docker-install.sh status
sh docker-install.sh logs
sh docker-install.sh pull
sh docker-install.sh restart
sh docker-install.sh down
```

Update the image with `pull` followed by `restart`; do not run `alx update` inside the container. Self-update inside a container cannot safely replace the read-only image layer.

For an MCP stdio connection, run `docker compose exec -T alx /app/alx mcp`; to restrict which projects it can manage, set `MCP_ALLOWED_ROOTS=/app/workspace`.

## Building from source

For the first build, or whenever Debian security updates, Chromium, or the QQ/NapCat system dependencies should be refreshed, manually publish `alemonbase` from the local Builder first:

```sh
docker login ccr.ccs.tencentyun.com
ALX_BASE_VERSION=20260822 make docker-base-buildx-push
```

Routine ALemonX builds reuse Tencent Cloud's `alemonbase` layer and do not run `apt-get update` or `apt-get upgrade` again:

```sh
make docker-build
ALX_IMAGE=alemonx:local docker compose up -d
```

The frontend is compiled into the repository-root `dist/` directory on the machine that starts the Docker build, then copied into the Go build stage as static assets. Vite therefore does not run under Docker/Buildx virtualization or cross-architecture emulation. Both `make docker-build` and `make docker-buildx` perform that step automatically. The Go stage cross-compiles the static `alx`, and the final image inherits Tencent Cloud's `ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase`. The base image owns Node, Git, SSH, Chromium, and the system libraries; the application image does not reinstall system packages. To reproduce a verified base, pass its versioned tag, for example `ALX_RUNTIME_BASE=ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:20260822 make docker-build`.

The image ships Noto CJK and Emoji fonts plus **Chromium**, so Chinese text and emoji render correctly in robot image messages (jsxp) and browser automation with Puppeteer/Playwright works out of the box (no manual download). The container runs as root, so browser and QQ/NapCat Electron launches need `--no-sandbox`; the QQ plugin adds it automatically. The image size grows noticeably (Chromium adds roughly 500 MB).

It also preinstalls the Linux runtime required by QQ/NapCat: Xvfb, XKB, GTK/NSS/GBM, audio, CUPS, and X11 libraries. Compose reserves 1 GiB for `/dev/shm`, avoiding the small Docker default that can crash Electron renderers. Installing the QQ plugin therefore does not need the privileged “prepare QQ login runtime” action; the plugin still downloads QQ/NapCat and starts its isolated Xvfb display and login flow.

## Yunzai pre-seeded image

`alemonx-yunzai` layers QQ, Finder (its current upstream release only ships a Linux amd64 asset), LoadYunzai, TRSS-Yunzai, Miao, Genshin, and Guoba over the base ALemonX image. QQ and Finder use ALX's formal Release installation and verification flow; the Yunzai-related repositories retain their `.git` directories for in-workspace updates. On first start it copies the seed into the persistent `workspace/` directory without overwriting an existing bot, plugin configuration, or QQ login state.

Build and run it locally:

```sh
make docker-yunzai-build
ALX_YUNZAI_IMAGE=alemonx-yunzai:local docker compose -f docker-compose.yml -f docker-compose.yunzai.yml up -d
```

For multi-platform validation or publishing:

```sh
make docker-yunzai-buildx
ALX_YUNZAI_VERSION=v1.2.3 make docker-yunzai-buildx-push
```

## Publishing and build boundary

The application image can be released manually through the GitHub Actions **发布腾讯云 Docker 镜像** workflow. On the repository's **Actions** page, choose the workflow and select **Run workflow**, then enter the Tencent Cloud TCR/CCR registry domain, namespace, image repository, login username, and password or access token. The form defaults to the current Tencent Cloud registry and also accepts a custom Tencent Cloud registry domain. The workflow masks the password in logs; however, GitHub Actions manual forms have no password field type, so it may be retained in workflow-run metadata. Use it only in trusted repositories. Each build automatically reads the nearest Git tag reachable from the selected commit and pushes that version plus `latest` for `linux/amd64` and `linux/arm64`.

The base image can be released manually through the **发布腾讯云 alemonbase 镜像** workflow. Provide a version tag (for example, `20260901`) and the workflow publishes both that version and `latest`; the application-image workflow should use that published version or `latest` as its runtime base image.

The base image is still built manually by the local Builder. Once `alemonbase` is published to Tencent Cloud, either GitHub Actions or the isolated Buildx builder can read it and build the multi-platform application image:

```sh
# Validate the application image only
make docker-buildx

# Manually publish the application image
ALX_VERSION=v1.2.3 make docker-buildx-push
```

For native single-platform development, use `make docker-base-build` to create `alemonbase:local`, then run `ALX_RUNTIME_BASE=alemonbase:local make docker-build`.
