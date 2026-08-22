# Runtime system dependencies are maintained in a separately released image.
ARG ALX_RUNTIME_BASE=ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:latest

# 前端构建阶段 - 构建 React 工作台
FROM node:22 AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile --non-interactive
COPY frontend/ ./
RUN yarn build

# 资源准备阶段 - 安装 Yarn 依赖
FROM node:22 AS resources
WORKDIR /out
COPY resources/packages/yarn/package.json resources/packages/yarn/package-lock.json ./yarn/
RUN (cd yarn && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)

# 后端构建阶段
FROM golang:1.24 AS builder
WORKDIR /src

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
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
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

RUN mkdir -p /app /app/plugins /app/workspace /data

WORKDIR /app
COPY --from=builder /out/alx /app/alx

# 设置环境变量
ENV HOME=/data \
    XDG_CONFIG_HOME=/data/config \
    XDG_CACHE_HOME=/data/cache \
    ALX_WORKSPACE=/app/workspace \
    ALEMONJS_SETUP_ROOTS=/app/workspace \
    YARN_CACHE_FOLDER=/app/.yarn_cache

EXPOSE 17390
ENTRYPOINT ["/usr/bin/tini", "--", "/app/alx"]
CMD ["--host", "0.0.0.0", "--port", "17390", "--redis-off"]
