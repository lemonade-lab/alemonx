#!/bin/sh
set -eu

image="${ALX_YUNZAI_IMAGE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonx-yunzai}"
version="${ALX_YUNZAI_VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)}"
platforms="${ALX_PLATFORMS:-linux/amd64,linux/arm64}"
push="${ALX_YUNZAI_PUSH:-0}"
builder="${ALX_BUILDER:-alx-builder}"
base="${ALX_YUNZAI_BASE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonx:latest}"

command -v docker >/dev/null 2>&1 || { echo '❌ 未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo '❌ Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  docker buildx create --name "$builder" --config ./buildkitd.toml --use --driver-opt network=host
else
  docker buildx use "$builder"
fi
docker buildx inspect --bootstrap

if [ "$push" = '1' ]; then
  docker buildx build --platform "$platforms" -f Dockerfile.yunzai \
    --build-arg "ALX_YUNZAI_BASE=$base" \
    -t "$image:latest" -t "$image:$version" --push .
else
  echo "🔨 验证 alemonx-yunzai（${platforms}）；使用 ALX_YUNZAI_PUSH=1 发布。"
  docker buildx build --platform "$platforms" -f Dockerfile.yunzai \
    --build-arg "ALX_YUNZAI_BASE=$base" --output type=cacheonly .
fi
