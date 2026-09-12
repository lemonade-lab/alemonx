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

容器内程序运行目录 `/app` 与持久工作区 `/app/workspace` 分开；创建机器人项目时即使选择了“当前目录”，程序也会自动保存到可写的 `ALEMONJS_SETUP_ROOTS`（Docker 默认指向挂载的 `/app/workspace`），因此项目始终位于宿主机持久化的工作区中。

## 配置与数据

`.env` 中最常用的项目如下：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `ALX_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest` | 腾讯云默认镜像；本地构建使用 `alemonx:local`。 |
| `ALX_PORT` | `17390` | 工作台暴露到宿主机的端口。 |
| `ALX_CONTAINER_NAME` | `alx` | 容器名（`docker ps` 显示），多实例部署时自定义。 |
| `ALX_DEPLOYMENT` | `production` | 生产模式：SQLite 与本地认证未配置时仅提示，不拦截启动；可在引导页创建管理员。 |

运行数据不放在命名卷，而是放在 compose 文件旁的宿主机目录中：

| 宿主目录 | 容器路径 | 内容 |
| --- | --- | --- |
| `./data` | `/root` | 工作台状态：账户、配置、SQLite、下载缓存 |
| `./workspace` | `/app/workspace` | 统一工作区：模板、机器人、内置工具、系统插件及 QQ 动态运行文件 |

容器内统一工作区为 `/app/workspace`：首次启动会把内嵌模板物化到其中的 `templates/`（可编辑，持久保存在宿主机 `./workspace`），新建机器人默认落在 `bots/`。内置 Yarn 物化到 `packages/yarn`；PM2 不随镜像嵌入，首次需要时用内置 Yarn 安装到 `packages/pm2`（位置固定，持久保存在宿主机目录）。通过工作台或 `alx plugin install` 安装的系统插件只写入 `plugins/`；程序随镜像提供的 `/app/plugins` 只用于兼容读取，且优先级低于工作区插件。每个系统插件的默认持久数据目录为 `store/<插件 ID>/`，容器重建后仍保留。QQ 插件会将动态下载的 QQ、NapCat、LLBot、SnowLuma 和登录运行态保存到 `store/alemonx-qq/runtimes/<系统-架构>/`；旧共享目录只迁移可读日志，不会复用其他系统的可执行文件。

容器进程以 root 运行，挂载目录开箱即用，无需在宿主机执行 `chown`/`chmod`。

macOS（Docker Desktop）上唯一的宿主侧要求是让 Docker Desktop 能访问部署目录：先在“系统设置 → 隐私与安全 → 文件与文件夹”（必要时“完全磁盘访问”）中允许 Docker Desktop，并在 Docker Desktop 的 “Settings → Resources → File sharing” 中包含部署目录；若目录位于“桌面/文稿”等受保护位置仍无法挂载，把目录移到普通位置（如 `~/alx-docker`）即可。

不要为了方便把容器改成 privileged，也不要挂载 `/var/run/docker.sock`。`docker compose down` 不会删除 `./data` 与 `./workspace`；只有手动删除这两个目录才会清除数据。

## 日常运维

### 可选：启用 DSH

默认不要求 DSH 密钥，工作台可独立启动。DSH 程序在镜像构建时安装、校验并嵌入，容器启动不会执行 npm。首次使用释放到 `workspace/dsh/packages/`；会话与事件位于 `workspace/dsh/runtimes/`、`workspace/dsh/events/`。备份和更新时保留 `data/` 与 `workspace/`；密钥文件单独安全备份，不放进镜像或 Git。

1. 新安装脚本会同时下载 `docker-compose.dsh.yml`。旧部署可从同一仓库版本补齐该文件。
2. 创建权限受限的 `secrets/` 目录，用编辑器将 Key 写入 `secrets/deepseek_api_key`，建议目录权限 `700`、文件权限 `600`。不要把 Key 填入命令行或 `.env`。
3. 在 `.env` 中添加 `COMPOSE_FILE=docker-compose.yml:docker-compose.dsh.yml`，然后运行 `docker compose up -d`。如还使用 Yunzai 等覆盖文件，将其一并追加到 `COMPOSE_FILE`。

可用 `ALX_DSH_SECRET_SOURCE` 指定其他宿主机密钥文件路径。界面会显示“由 Docker Secret 管理”，不能通过网页覆盖。只配置模型并连接即可。密钥缺失或为空时只禁用 DSH；更新密钥文件后使用 `docker compose up -d --force-recreate`，避免旧容器仍挂载旧文件。所有机器人默认共用此 Secret。

注意：启用覆盖文件后，若宿主机 Secret 文件不存在，Docker Compose 会拒绝创建容器。应先创建文件，或暂时移除该覆盖文件。上述“只禁用 DSH”指已启动容器中 Secret 为空或无法读取的情形。

Node 的镜像默认版本是 `22.22.3`，仍允许用户显式使用持久化 NVM 版本；Python 则固定由镜像提供，容器启动不会恢复本机 Python 选择记录。

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

首次构建或需要刷新 Debian 安全更新、Chromium、QQ/NapCat 系统依赖时，先由本机 Builder 手动发布 `alemonbase`。`Dockerfile.base` 当前以 `node:22.22.3` 为基础；应用构建会校验最终基础镜像的 Node 版本。必须执行带 `push` 的命令，普通的 `make docker-base-buildx` 只验证，不会更新腾讯云镜像：

```sh
docker login ccr.ccs.tencentyun.com
ALX_BASE_VERSION=20260822 make docker-base-buildx-push
```

发布完成后，应用镜像继续复用腾讯云的 `alemonbase:latest`，不会再次执行 `apt-get update` 或 `apt-get upgrade`：

```sh
make docker-build
ALX_IMAGE=alemonx:local docker compose up -d
```

前端先在构建设备上生成 `dist/`，再复制进 Go 构建阶段，不在跨架构模拟环境中运行 Vite。`make docker-build` 与 `make docker-buildx` 都会自动完成该步骤。最终镜像继承 `alemonbase:latest`，基础镜像负责 Node、Git、SSH、Chromium 和系统库；应用镜像不重复安装系统包。发布前会在每个目标架构的最终运行环境执行 DSH SDK 初始化及持久会话恢复检查，不发送模型请求。检查工具和临时数据不进入最终镜像。

镜像内置 Noto CJK 与 Emoji 字体以及 **Chromium 浏览器**：机器人图片消息（jsxp 渲染）中文与表情显示正常，Puppeteer/Playwright 等浏览器自动化开箱可用（无需自行下载）。容器以 root 运行，浏览器或 QQ/NapCat 的 Electron 运行时必须使用 `--no-sandbox`；QQ 插件会自动添加该参数。镜像体积会因此明显增大（Chromium 约 500MB）。

镜像也预装 QQ/NapCat Linux 所需的 Xvfb、XKB、GTK/NSS/GBM、音频、CUPS 和 X11 动态库，并把 `/dev/shm` 设为 1 GiB。安装 QQ 插件时无需再执行“准备 QQ 登录运行环境”的系统授权；插件仍会负责下载 QQ/NapCat、启动独立 Xvfb 显示与登录流程。

NapCat 启动后会在容器回环地址创建 QQ 桌面入口。该入口不映射宿主机端口，只能从已登录的 ALemonX 工作台内打开。SnowLuma 默认关闭，因为它需要注入 QQ 进程；需要时使用显式高权限覆盖文件启动：

```sh
docker compose -f docker-compose.yml -f docker-compose.snowluma.yml up -d
```

## Yunzai 预置版

`alemonx-yunzai` 在基础 ALemonX 镜像上预置 QQ、Finder（当前上游仅提供 Linux amd64 安装包）、LoadYunzai、TRSS-Yunzai、Miao、Genshin 与锅巴。QQ/Finder 必须通过 ALX 的正式 Release 安装流程下载和校验；Yunzai 相关仓库则保留 `.git`，可在工作区内自行更新。首次启动会把这些文件复制到持久化的 `workspace/`，而不会覆盖已有机器人、插件配置或 QQ 登录状态。

本地构建：

```sh
make docker-yunzai-build
ALX_YUNZAI_IMAGE=alemonx-yunzai:local docker compose -f docker-compose.yml -f docker-compose.yunzai.yml up -d
```

多架构验证或发布：

```sh
make docker-yunzai-buildx
ALX_YUNZAI_VERSION=v1.2.3 make docker-yunzai-buildx-push
```

该模式会添加 `SYS_PTRACE` 与受控的 seccomp 放宽项，仅应在确认需要 SnowLuma 的主机上启用。

## 发布与构建边界

应用镜像可通过 GitHub Actions 的 **发布腾讯云 Docker 镜像** 工作流手动发布。进入仓库的 **Actions** 页面，选择该工作流并点击 **Run workflow**，然后填写腾讯云 TCR/CCR 仓库域名、命名空间、镜像仓库、登录账号及密码或访问令牌。运行时基础镜像默认使用 `alemonbase:latest`。表单默认使用当前的腾讯云镜像地址，也允许填写自定义的腾讯云仓库域名。工作流会在日志中遮罩密码；但 GitHub Actions 的手动表单不提供密码字段类型，密码可能保留在工作流运行元数据中，请只在受信任的仓库中使用。每次构建会自动读取当前提交可追溯到的最新 Git 标签，并同时推送该版本和 `latest`，同时构建 `linux/amd64` 与 `linux/arm64` 两个架构。

基础镜像可通过 **发布腾讯云 alemonbase 镜像** 工作流手动发布。填写版本标签（例如 `20260901`）后，工作流会同时发布该版本和 `latest` 标签；应用镜像工作流的运行时基础镜像应使用已发布的版本标签或 `latest`。

基础镜像仍由本机 Builder 手动构建；`alemonbase` 发布到腾讯云后，GitHub Actions 或隔离的 Buildx builder 均可读取它并构建多架构应用镜像：

```sh
# 仅验证应用镜像
make docker-buildx

# 手动推送应用镜像
ALX_VERSION=v1.2.3 make docker-buildx-push
```

仅需本机单架构开发时，可运行 `make docker-base-build`，它生成 `alemonbase:local`；再以 `ALX_RUNTIME_BASE=alemonbase:local make docker-build` 使用它。
