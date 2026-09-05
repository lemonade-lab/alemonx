# Docker 运行时修复记录

## 目标

1. 基础镜像中的 Node.js 固定为 `22.22.3`，避免 `node:22` 自动升级到其他小版本。
2. 保持既有镜像标签策略不变：基础镜像仍发布 `latest` 与人工填写的版本标签；应用镜像仍默认引用 `alemonbase:latest`。
3. Debian 官方源临时返回 5xx 时，基础镜像构建应在重试后使用可配置的备用镜像源。
4. Compose 将 `./data` 挂载到 `/root` 后，GitHub 与 Gitee 的 SSH 主机指纹仍须存在，避免克隆报 `Host key verification failed`。

## 最小改动范围

| 文件 | 改动 |
| --- | --- |
| `Dockerfile.base` | 将 `FROM node:22` 改为 `FROM node:22.22.3`；仅增强 `alx-apt-install` 的重试与备用源回退。 |
| `scripts/docker-base-build.sh` | 仅透传 `ALX_DEBIAN_FALLBACK_MIRROR`。 |
| `scripts/docker-base-buildx.sh` | 仅透传同一构建参数；不得新增或修改任何镜像标签。 |
| `Dockerfile` | 增加一个启动包装脚本，用于在 `/root` 挂载完成后初始化 SSH known_hosts；不改 `ALX_RUNTIME_BASE`，不改标签。 |
| `scripts/docker-entrypoint.sh` | 新增：仅初始化 GitHub/Gitee 的 known_hosts，再 `exec /app/alx`。 |
| `docs/docker.md`、`docs/en/docker.md` | 仅说明上述行为。 |

## 不应改动

- 所有测试文件及测试断言。
- GitHub Actions 的镜像标签列表。
- `Makefile`、`scripts/docker-buildx.sh`、应用镜像工作流中的 `latest` 默认值。
- Node 运行时管理、Redis、机器人、插件、前端及其他无关模块。

## 验证

```sh
sh -n scripts/docker-entrypoint.sh scripts/docker-base-build.sh scripts/docker-base-buildx.sh
docker build --check -f Dockerfile.base .
docker build --check -f Dockerfile .
git diff --check
```
