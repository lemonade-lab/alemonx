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

The container's `/app` directory is read-only, but `/app/workspace` is a separate writable mount. When creating a robot project with "current directory" selected, the program automatically falls back to the writable `ALEMONJS_SETUP_ROOTS` (Docker defaults it to the mounted `/app/workspace`), so projects are saved there by default.

## Configuration and data

The most common `.env` variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `ALX_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest` | Default Tencent Cloud image; use `alx:local` for local builds. |
| `ALX_PORT` | `17390` | Port exposed to the host. |
| `ALX_CONTAINER_NAME` | `alx` | Container name shown by `docker ps`; customize for multi-instance deployments. |
| `ALX_DEPLOYMENT` | `production` | Production mode: missing SQLite or local auth only prints reminders and does not block startup; an admin can be created from the setup guide. |

Runtime data is not kept in a named volume but in host directories next to the compose file:

| Host directory | Container path | Contents |
| --- | --- | --- |
| `./data` | `/data` | Workbench state: accounts, config, SQLite, caches, plugins |
| `./workspace` | `/app/workspace` | Unified workspace: templates, robots, bundled tools |

The unified workspace inside the container is `/app/workspace`: on first start the embedded templates are materialized into its `templates/` directory (editable and persisted on the host under `./workspace`), and new robots land in `bots/` by default. The built-in Yarn is materialized into `packages/yarn`; PM2 is not embedded in the image and is installed with the built-in Yarn into `packages/pm2` on first use (a stable location persisted on the host).

The container runs as the non-root `node` user (uid 1000). On Linux, bind mounts keep the host directory ownership; if `./data` or `./workspace` is not writable by uid 1000, the container reports read-only/permission errors. Fix it with:

```sh
chown -R 1000:1000 ./data ./workspace
```

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

```sh
make docker-build
ALX_IMAGE=alx:local docker compose up -d
```

The build uses a multi-stage image: the Node stage produces the embedded frontend, the Go stage cross-compiles the static `alx`, and the final image keeps only the Node, Git, SSH, and workbench binaries needed to run robots.

The image ships Noto CJK and Emoji fonts so Chinese text and emoji render correctly in robot image messages (jsxp). **A browser (Chrome/Chromium) is intentionally not bundled**: it is an optional capability, Puppeteer downloads one on demand, or you can install a browser on the host and map it into the container.

Multi-architecture releases follow a manual Buildx flow (similar to alemongo) rather than automatic pushes on version tags:

```sh
# Validate the build only; do not push
./scripts/docker-buildx.sh

# After manually confirming the version and target, publish
docker login ccr.ccs.tencentyun.com
ALX_VERSION=v1.2.3 ALX_PUSH=1 ./scripts/docker-buildx.sh
```

The `Docker` workflow in GitHub Actions can only be triggered manually from the Actions page; by default it pushes to the Tencent Cloud `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx`. It only publishes after the version, platforms, repository credentials, and the push switch are explicitly filled in.
