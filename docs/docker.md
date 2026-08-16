# Docker 部署

> 本页还有 [English version](en/docker.md)。

ALemonX 的 Docker 镜像包含工作台、Node.js、Corepack、Git 与 SSH 客户端，可管理挂载到容器内 `/app/workspace` 的 AlemonJS 项目。它不挂载 Docker socket，也不会获得宿主机 sudo 或桌面权限。

## 快速启动

Linux 或 macOS：

```sh
mkdir alx-docker && cd alx-docker
curl -fsSLO https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh
sh docker-install.sh up
```

脚本会下载 `docker-compose.yml` 和 `.env.example`，复制后生成可编辑的 `.env`。首次启动后打开 `http://localhost:17390`，按引导创建管理员账户。

国内用户可通过镜像获取部署脚本：把命令中的 `raw.githubusercontent.com` 前缀替换为镜像即可，例如 `https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh`。脚本支持 `ALX_RAW_BASE` 指向镜像源，例如：

```sh
ALX_RAW_BASE=https://ghfast.top/https://raw.githubusercontent.com/lemonade-lab/alemonx/main sh docker-install.sh up
```

没有网络脚本时，也可直接使用仓库文件：

```sh
cp .env.example .env
mkdir data workspace
docker compose up -d
```

容器内程序运行目录 `/app` 是只读的，但其中的 `/app/workspace` 是独立挂载、可写；创建机器人项目时如果选择了“当前目录”，程序会自动回退到可写的 `ALEMONJS_SETUP_ROOTS`（Docker 默认指向挂载的 `/app/workspace`），因此项目默认会保存到该工作区。

## 配置与数据

`.env` 中最常用的项目如下：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `ALX_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest` | 腾讯云默认镜像；本地构建使用 `alx:local`。 |
| `ALX_PORT` | `17390` | 工作台暴露到宿主机的端口。 |
| `ALX_CONTAINER_NAME` | `alx` | 容器名（`docker ps` 显示），多实例部署时自定义。 |
| `ALX_DEPLOYMENT` | `production` | 生产模式：SQLite 与本地认证未配置时仅提示，不拦截启动；可在引导页创建管理员。 |

运行数据不放在命名卷，而是放在 compose 文件旁的宿主机目录中：

| 宿主目录 | 容器路径 | 内容 |
| --- | --- | --- |
| `./data` | `/data` | 工作台状态：账户、配置、SQLite、缓存、插件 |
| `./workspace` | `/app/workspace` | 统一工作区：模板、机器人、内置工具 |

容器内统一工作区为 `/app/workspace`：首次启动会把内嵌模板物化到其中的 `templates/`（可编辑，持久保存在宿主机 `./workspace`），新建机器人默认落在 `bots/`。内置 Yarn 物化到 `packages/yarn`；PM2 不随镜像嵌入，首次需要时用内置 Yarn 安装到 `packages/pm2`（位置固定，持久保存在宿主机目录）。

容器进程以非 root 的 `node` 用户（uid 1000）运行。Linux 上 bind 挂载保留宿主目录属主，如果 `./data` 或 `./workspace` 对 uid 1000 不可写，容器会报只读/权限错误，请执行：

```sh
chown -R 1000:1000 ./data ./workspace
```

不要为了方便把容器改成 privileged，也不要挂载 `/var/run/docker.sock`。`docker compose down` 不会删除 `./data` 与 `./workspace`；只有手动删除这两个目录才会清除数据。

## 日常运维

```sh
sh docker-install.sh status
sh docker-install.sh logs
sh docker-install.sh pull
sh docker-install.sh restart
sh docker-install.sh down
```

镜像更新使用 `pull` 后的 `restart`，不要在容器里执行 `alx update`。容器内的自更新无法安全替换只读镜像层。

MCP 的 stdio 连接可通过 `docker compose exec -T alx /app/alx mcp` 启动；如需限制它能访问的项目，设置 `MCP_ALLOWED_ROOTS=/app/workspace`。

## 从源码构建

```sh
make docker-build
ALX_IMAGE=alx:local docker compose up -d
```

构建使用多阶段镜像：Node 阶段生成嵌入式前端，Go 阶段交叉编译静态 `alx`，最终镜像只保留运行机器人所需的 Node、Git、SSH 与工作台二进制。

镜像内置 Noto CJK 与 Emoji 字体以及 **Chromium 浏览器**：机器人图片消息（jsxp 渲染）中文与表情显示正常，Puppeteer/Playwright 等浏览器自动化开箱可用（无需自行下载）。容器以非 root 运行且没有沙箱权限，容器内使用浏览器时需要 `--no-sandbox`（例如 `chromium --headless --no-sandbox ...`，Puppeteer 可传 `args: ['--no-sandbox']`）。镜像体积会因此明显增大（Chromium 约 500MB）。

多架构发布采用类似 alemongo 的人工 Buildx 流程，而不是版本标签自动推送：

```sh
# 仅验证构建，不推送镜像
./scripts/docker-buildx.sh

# 人工确认版本和推送目标后发布
docker login ccr.ccs.tencentyun.com
ALX_VERSION=v1.2.3 ALX_PUSH=1 ./scripts/docker-buildx.sh
```

GitHub Actions 中的 `Docker` 工作流同样只能从 Actions 页面手动触发；默认推送到腾讯云 `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx`。需明确填写版本、平台、仓库凭据，并勾选推送开关后才会发布。
