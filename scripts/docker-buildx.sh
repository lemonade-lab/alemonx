#!/bin/sh
# Manually build and optionally publish ALemonX's multi-architecture image.
# Examples:
#   ./scripts/docker-buildx.sh
#   ALX_VERSION=v1.2.3 ALX_PUSH=1 ./scripts/docker-buildx.sh

set -eu

image="${ALX_IMAGE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonx}"
version="${ALX_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
platforms="${ALX_PLATFORMS:-linux/amd64,linux/arm64}"
push="${ALX_PUSH:-0}"
builder="${ALX_BUILDER:-alx-builder}"

command -v docker >/dev/null 2>&1 || { printf '%s\n' '未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { printf '%s\n' 'Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  docker buildx create --name "$builder" --use
else
  docker buildx use "$builder"
fi
docker buildx inspect --bootstrap

set -- \
  docker buildx build \
  --platform "$platforms" \
  --build-arg "VERSION=$version" \
  -t "$image:latest" \
  -t "$image:$version"

if [ "$push" = '1' ]; then
  set -- "$@" --push
  printf '%s\n' "构建并推送 $image:$version ($platforms)"
else
  # Multi-platform results are retained in BuildKit's cache only. This makes
  # the default safe for validation; use `docker build` for a local image.
  printf '%s\n' "仅验证构建 $image:$version ($platforms)，不会推送"
fi

"$@" .
