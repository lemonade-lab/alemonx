# Docker 部署

ALemonX 的 Docker 镜像包含工作台、Node.js、Corepack、Git 与 SSH 客户端，可管理挂载到容器内 `/workspace` 的 AlemonJS 项目。它不挂载 Docker socket，也不会获得宿主机 sudo 或桌面权限。

## 快速启动

Linux 或 macOS：

```sh
mkdir alx-docker && cd alx-docker
curl -fsSLO https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh
sh docker-install.sh up
```

脚本会下载 `docker-compose.yml` 和 `.env.example`，复制后生成可编辑的 `.env`。首次启动后打开 `http://localhost:17390`，按引导创建管理员账户。

没有网络脚本时，也可直接使用仓库文件：

```sh
cp .env.example .env
mkdir workspace
docker compose up -d
```

## 配置与数据

`.env` 中最常用的项目如下：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `ALX_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest` | 腾讯云默认镜像；本地构建使用 `alx:local`。 |
| `ALX_PORT` | `17390` | 工作台暴露到宿主机的端口。 |
| `ALX_WORKSPACE` | `./workspace` | 宿主机机器人项目目录；容器只允许管理该目录。 |
| `ALX_DEPLOYMENT` | `production` | 保持生产门禁：SQLite 和本地认证必须启用。 |

工作台状态、账户、Agent 任务、插件和缓存保存在 Docker 命名卷 `alx-data` 中；机器人源码在 `ALX_WORKSPACE` 指向的宿主机目录中。执行 `docker compose down` 不会删除它们；只有 `docker compose down -v` 才会删除工作台状态卷。

容器进程以非 root 的 `node` 用户运行。若绑定目录在 Linux 上不可写，请让目录归当前 Docker 用户或调整其权限；不要为了方便把容器改成 privileged，也不要挂载 `/var/run/docker.sock`。

## 日常运维

```sh
sh docker-install.sh status
sh docker-install.sh logs
sh docker-install.sh pull
sh docker-install.sh restart
sh docker-install.sh down
```

镜像更新使用 `pull` 后的 `restart`，不要在容器里执行 `alx update`。容器内的自更新无法安全替换只读镜像层。

MCP 的 stdio 连接可通过 `docker compose exec -T alx /app/alx mcp` 启动；如需限制它能访问的项目，设置 `MCP_ALLOWED_ROOTS=/workspace`。

## 从源码构建

```sh
make docker-build
ALX_IMAGE=alx:local docker compose up -d
```

构建使用多阶段镜像：Node 阶段生成嵌入式前端，Go 阶段交叉编译静态 `alx`，最终镜像只保留运行机器人所需的 Node、Git、SSH 与工作台二进制。

多架构发布采用类似 alemongo 的人工 Buildx 流程，而不是版本标签自动推送：

```sh
# 仅验证构建，不推送镜像
./scripts/docker-buildx.sh

# 人工确认版本和推送目标后发布
docker login ccr.ccs.tencentyun.com
ALX_VERSION=v1.2.3 ALX_PUSH=1 ./scripts/docker-buildx.sh
```

GitHub Actions 中的 `Docker` 工作流同样只能从 Actions 页面手动触发；默认推送到腾讯云 `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx`。需明确填写版本、平台、仓库凭据，并勾选推送开关后才会发布。
