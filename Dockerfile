# syntax=docker/dockerfile:1

# Build the embedded React workbench before compiling the Go binary. Keeping
# this stage separate makes dependency downloads cacheable between releases.
FROM node:22-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/yarn.lock ./
RUN corepack enable && yarn install --frozen-lockfile --non-interactive
COPY frontend/ ./
RUN yarn build

# Install the embedded Yarn with its locked dependency tree so the Go binary
# always ships a package manager that never relies on npm. Optional tools such
# as PM2 are provisioned on demand with this Yarn at runtime instead of being
# embedded.
FROM node:22-bookworm-slim AS resources
WORKDIR /out
COPY resources/packages/yarn/package.json resources/packages/yarn/package-lock.json ./yarn/
RUN (cd yarn && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)

FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/dist ./dist
COPY --from=resources /out ./resources/packages

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
  && mkdir -p /app /app/workspace /data \
  && chown -R node:node /app /data
WORKDIR /app
COPY --from=builder /out/alx /app/alx
USER node
ENV HOME=/data \
    XDG_CONFIG_HOME=/data/config \
    XDG_CACHE_HOME=/data/cache \
    ALX_WORKSPACE=/app/workspace \
    ALEMONJS_SETUP_ROOTS=/app/workspace \
    NODE_ENV=production
EXPOSE 17390
ENTRYPOINT ["/usr/bin/tini", "--", "/app/alx"]
CMD ["--host", "0.0.0.0", "--port", "17390", "--redis-off"]
