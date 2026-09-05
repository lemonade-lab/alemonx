# Runtime system dependencies are maintained in a separately released image.
ARG ALX_RUNTIME_BASE=ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:latest

# 前端构建阶段 - 构建 React 工作台
FROM node:22.22.3 AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile --non-interactive
COPY frontend/ ./
RUN yarn build

# 资源准备阶段 - 安装 Yarn 依赖
FROM node:22.22.3 AS resources
WORKDIR /out
COPY resources/packages/yarn/package.json resources/packages/yarn/package-lock.json ./yarn/
RUN (cd yarn && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)

# 后端构建阶段
FROM golang:1.24 AS builder
WORKDIR /src

# BuildKit automatic platform arguments are global before the first FROM, but
# must be re-declared inside this stage before GOARCH/GOOS can consume them.
# Without these declarations an ARM Docker build silently produced an amd64
# workbench binary, causing platform-aware plugins to download x64 runtimes.
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# 配置 Go 模块代理为国内镜像源
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码和其他资源
COPY . ./
COPY --from=frontend /src/dist ./dist
COPY --from=resources /out ./resources/packages

# 打包 go 支持多架构
ARG VERSION=dev

RUN set -eu; \
  GOARM_VALUE=""; \
  if [ "${TARGETARCH}" = "arm" ] && [ -n "${TARGETVARIANT:-}" ]; then \
    GOARM_VALUE="${TARGETVARIANT#v}"; \
  fi; \
  CGO_ENABLED=0 \
  GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  GOARM=${GOARM_VALUE} \
  go build -trimpath -ldflags "-s -w -X main.Version=${VERSION} -X main.BuildTime=$(date +%s)" -o /out/alx .

# 最终运行阶段
FROM ${ALX_RUNTIME_BASE} AS runtime

RUN mkdir -p /app /app/plugins /app/workspace /data /root/.ssh

WORKDIR /app
COPY --from=builder /out/alx /app/alx

# 授权
# docker-compose 会将持久数据挂载到 /root；启动脚本会在挂载完成后写入
# GitHub、Gitee 的主机指纹，避免 known_hosts 被挂载覆盖。
COPY scripts/docker-entrypoint.sh /usr/local/bin/alx-entrypoint
RUN chmod 755 /usr/local/bin/alx-entrypoint

# 设置环境变量
ENV HOME=/root \
    XDG_CONFIG_HOME=/root/config \
    XDG_CACHE_HOME=/root/cache \
    ALX_WORKSPACE=/app/workspace \
    ALEMONJS_SETUP_ROOTS=/app/workspace \
    YARN_CACHE_FOLDER=/app/.yarn_cache

EXPOSE 17390
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/alx-entrypoint", "/app/alx"]
CMD ["--host", "0.0.0.0", "--port", "17390"]
