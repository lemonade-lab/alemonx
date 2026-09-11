# Runtime system dependencies are maintained in a separately released image.
ARG ALX_RUNTIME_BASE=ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:latest

# Keep the application image independent from an accidentally stale runtime
# base tag. The exact Node runtime is copied into the final stage below.
FROM node:22.22.3 AS node-runtime

# 资源准备阶段 - 安装 Yarn 依赖
FROM node-runtime AS resources
WORKDIR /out
COPY resources/packages/yarn/package.json resources/packages/yarn/package-lock.json ./yarn/
RUN (cd yarn && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)

# DSH is an application-owned sidecar, never a user-global installation. The
# lockfile is copied separately so changes to Go or the frontend do not bust
# the expensive Harness dependency layer.
FROM node-runtime AS dsh-runtime
WORKDIR /out/dsh
COPY dsh/package.json dsh/package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund && npm run check

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
# The workbench is compiled by the machine that invokes Docker (or the CI
# runner) before this build starts. Keeping Vite out of BuildKit avoids the
# virtualized/emulated CPU bottleneck, especially during multi-platform builds.
COPY dist ./dist
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

# 备份旧版本 Node.js
RUN mv /usr/bin/node /usr/bin/node.v18.bak || true && \
    mv /usr/bin/nodejs /usr/bin/nodejs.v18.bak || true

# 创建软链接指向新版本（假设新版本在 /usr/local/bin/node）
RUN ln -sf /usr/local/bin/node /usr/bin/node && \
    ln -sf /usr/local/bin/node /usr/bin/nodejs

RUN mkdir -p /app /app/plugins /app/workspace /data /root/.ssh

WORKDIR /app
COPY --from=builder /out/alx /app/alx
COPY --from=dsh-runtime /out/dsh /app/dsh

# 授权
# docker-compose 会将持久数据挂载到 /root；启动脚本会在挂载完成后写入
# GitHub、Gitee 的主机指纹，避免 known_hosts 被挂载覆盖。
COPY scripts/docker-entrypoint.sh /usr/local/bin/alx-entrypoint
RUN chmod 755 /usr/local/bin/alx-entrypoint

# 设置环境变量
ENV HOME=/root \
	PATH="/opt/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
	XDG_CONFIG_HOME=/root/config \
    XDG_CACHE_HOME=/root/cache \
    ALX_WORKSPACE=/app/workspace \
    ALEMONJS_SETUP_ROOTS=/app/workspace \
    YARN_CACHE_FOLDER=/app/.yarn_cache \
    ALX_DSH_BIN=/app/dsh/node_modules/.bin/dsh \
    ALX_DSH_SECRET_FILE=/run/secrets/deepseek_api_key

EXPOSE 17390
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/alx-entrypoint", "/app/alx"]
CMD ["--host", "0.0.0.0", "--port", "17390"]
