# Yunzai Docker 部署

> 普通 ALemonX 部署请参阅 [Docker 部署](docker.md)。

ALemonX Yunzai 镜像包含工作台、Node.js、Corepack、Git、SSH 客户端、Chromium、Miao-Yunzai 与 miao-plugin。工作台和机器人数据均保存在挂载的工作区中；容器不挂载 Docker socket，也不会获得宿主机 sudo 或桌面权限。

## 快速启动

Linux 或 macOS：

```sh
mkdir alemonx-yunzai && cd alemonx-yunzai
curl -fsSLO https://raw.githubusercontent.com/lemonade-lab/alemonx/main/docker-compose.yunzai.yml
mkdir data workspace data-redis
docker compose -f docker-compose.yunzai.yml up -d
```

首次启动后打开 `http://localhost:17390`，按引导创建管理员账户。镜像会在工作区中初始化预置机器人：

```text
workspace/bots/alemonb/
├── packages/alemonjs-load-yunzai/
└── Miao-Yunzai/
    └── plugins/miao-plugin/
```

预置内容不包含 `node_modules`。在工作台中先安装机器人根项目依赖，再打开 Yunzai 本地包安装 Miao-Yunzai 依赖，最后启动机器人。依赖在目标容器中安装，因此会匹配当前平台与 Node 版本。

容器内程序目录 `/app` 与持久工作区 `/app/workspace` 分开。即使在工作台选择“当前目录”，机器人也会保存到可写的 `ALEMONJS_SETUP_ROOTS`，即挂载的 `/app/workspace`。

## 配置与数据

启动前可在同目录创建 `.env` 覆盖常用配置：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `YUNZAI_IMAGE` | `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx-yunzai:latest` | Yunzai 镜像；建议生产环境固定到发布版本。 |
| `YUNZAI_PORT` | `17390` | 工作台暴露到宿主机的端口。 |
| `YUNZAI_CONTAINER_NAME` | `alemonx-yunzai` | 工作台容器名称。 |
| `YUNZAI_REDIS_CONTAINER_NAME` | `alemonx-yunzai-redis` | Redis 容器名称。 |
| `ALX_DEPLOYMENT` | `production` | 工作台生产部署模式。 |
| `TZ` | `Asia/Shanghai` | 容器时区。 |

运行数据不使用命名卷，而是保存在 compose 文件旁：

| 宿主目录 | 容器路径 | 内容 |
| --- | --- | --- |
| `./data` | `/data` | 工作台账户、SQLite、配置和下载缓存。 |
| `./workspace` | `/app/workspace` | 机器人、Miao 数据、安装后的依赖和系统插件。 |
| `./data-redis` | Redis `/data` | Redis AOF 持久化数据。 |

首次启动只把镜像 seed 中不存在的文件复制到 `workspace/bots/alemonb`。后续升级不会覆盖机器人配置、Miao 数据、插件修改或已安装依赖。通过工作台或 `alx plugin install` 添加的系统插件固定写入 `workspace/plugins`；镜像的 `/app/plugins` 仅作为兼容读取目录，且优先级更低。系统插件的默认持久数据目录为 `workspace/store/<插件 ID>/`，因此 QQ 登录态、下载组件和配置会随工作区挂载保留。

镜像内已提供 Chromium 与中文、Emoji 字体。Miao-Yunzai 会识别系统 Chromium，不会额外下载浏览器。

macOS（Docker Desktop）需要允许 Docker Desktop 访问部署目录，并在 “Settings → Resources → File sharing” 中包含该目录。若“桌面/文稿”等受保护目录无法挂载，请移动到普通目录，例如 `~/alemonx-yunzai`。

不要将容器改为 privileged，也不要挂载 `/var/run/docker.sock`。`docker compose down` 不会删除 `data`、`workspace` 或 `data-redis`；只有手动删除这些目录才会清除数据。

## 日常运维

在部署目录中执行：

```sh
docker compose -f docker-compose.yunzai.yml ps
docker compose -f docker-compose.yunzai.yml logs -f --tail=200 alx
docker compose -f docker-compose.yunzai.yml pull
docker compose -f docker-compose.yunzai.yml up -d
docker compose -f docker-compose.yunzai.yml down
```

更新镜像时执行 `pull` 再执行 `up -d`，不要在容器内自更新。MCP stdio 可通过 `docker compose -f docker-compose.yunzai.yml exec -T alx /app/alx mcp` 启动；如需限制其可访问路径，设置 `MCP_ALLOWED_ROOTS=/app/workspace`。

## 从源码构建

构建 Yunzai 镜像需要本地资源目录 `.resources/`：

| 文件 | 内容 |
| --- | --- |
| `Miao-Yunzai-master.zip` | Miao-Yunzai 源码快照。 |
| `miao-plugin-master.zip` | miao-plugin 源码快照。 |
| `alemonjs-load-yunzai.tar.gz` | 已构建 loader 的无依赖运行包。 |
| `yunzai-resources.sha256` | 上述三个构建资源的校验清单。 |

前两份压缩包由维护者预先放入 `.resources/`。下面命令从相邻的 `../alemonjs-load-yunzai` 读取已构建的 `lib/`、`dist/`、`yarn/` 和 `package.json`，生成 loader 运行包与校验清单：

```sh
make yunzai-resources
```

若 loader 目录位于其他位置：

```sh
make yunzai-resources YUNZAI_LOADER_SOURCE=/absolute/path/to/alemonjs-load-yunzai
```

本地构建：

```sh
make docker-yunzai-build
YUNZAI_IMAGE=alemonx-yunzai:local docker compose -f docker-compose.yunzai.yml up -d
```

该目标先构建 `alemonx:local`，再以它构建 `alemonx-yunzai:local`。Yunzai Dockerfile 不克隆 Git 仓库，校验本地资源哈希后才解压到镜像 seed，且不将任何依赖目录写入镜像。

当前镜像预置 `alemonjs-load-yunzai`、Miao-Yunzai 与 miao-plugin。`alemonx-finder`、`alemonx-qq` 等系统插件在准备好各自的本地构建产物后，按相同原则加入：仅打入构建产物和代码，不打入依赖，不在 Dockerfile 中 Git 克隆。

## 多架构发布

Yunzai 镜像仍由本机手动发布，不使用 GitHub Actions。先按 [Docker 部署](docker.md) 中的流程发布或确认 `alemonbase`，再发布同版本的 `alemonx` 应用镜像，最后执行：

```sh
# 仅验证 linux/amd64、linux/arm64 构建
YUNZAI_VERSION=v0.2.20 make docker-yunzai-buildx

# 构建并推送 latest 与 v0.2.20
YUNZAI_VERSION=v0.2.20 make docker-yunzai-buildx-push
```

发布目标默认为 `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx-yunzai`，基础应用镜像默认为同版本的 `ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:<版本>`。
