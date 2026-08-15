# syntax=docker/dockerfile:1

# Build the embedded React workbench before compiling the Go binary. Keeping
# this stage separate makes dependency downloads cacheable between releases.
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile --non-interactive
COPY frontend/ ./
RUN yarn build

FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/dist ./dist

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TARGETVARIANT
ARG VERSION=dev
RUN set -eu; \
  goarm=""; \
  if [ "$TARGETARCH" = "arm" ] && [ -n "${TARGETVARIANT:-}" ]; then goarm="${TARGETVARIANT#v}"; fi; \
  CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="$goarm" \
  go build -trimpath -ldflags "-s -w -X main.Version=$VERSION" -o /out/alx .

# AlemonJS projects need Node.js, npm/corepack, Git and SSH at runtime. The
# workbench intentionally runs as the unprivileged node user; Compose mounts
# only the explicit workspace and persistent data directories.
FROM node:22-bookworm-slim AS runtime
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates curl git lsof openssh-client tini \
  && rm -rf /var/lib/apt/lists/* \
  && corepack enable \
  && mkdir -p /app /data /workspace \
  && chown -R node:node /app /data /workspace
WORKDIR /app
COPY --from=builder /out/alx /app/alx
USER node
ENV HOME=/data \
    XDG_CONFIG_HOME=/data/config \
    XDG_CACHE_HOME=/data/cache \
    NODE_ENV=production
EXPOSE 17390
ENTRYPOINT ["/usr/bin/tini", "--", "/app/alx"]
CMD ["--host", "0.0.0.0", "--port", "17390", "--redis-off"]
