#!/bin/sh
set -eu

# Refresh this manually for Debian security updates and system-level runtime
# dependency changes. This is intentionally separate from GitHub Actions.
image="${ALX_BASE_IMAGE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase}"
version="${ALX_BASE_VERSION:-$(date -u +%Y%m%d)}"
platforms="${ALX_PLATFORMS:-linux/amd64,linux/arm64}"
push="${ALX_BASE_PUSH:-0}"
builder="${ALX_BUILDER:-alx-builder}"

command -v docker >/dev/null 2>&1 || { echo '❌ 未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo '❌ Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  echo "🔧 创建新的 builder: $builder"
  docker buildx create --name "$builder" --config ./buildkitd.toml --use  --driver-opt network=host
else
  echo "🔧 使用已有 builder: $builder"
  docker buildx use "$builder"
fi
docker buildx inspect --bootstrap

echo '=========================================='
echo "📦 基础镜像: $image"
echo "🏷️  版本: $version"
echo "🖥️  平台: $platforms"
echo "📤 推送: $([ "$push" = '1' ] && echo '是' || echo '否')"
echo '=========================================='

if [ "$push" = '1' ]; then
  docker buildx build \
    --platform "$platforms" \
    -f Dockerfile.base \
    -t "$image:latest" \
    -t "$image:$version" \
    --push \
    .
else
  echo '🔨 仅验证构建（结果保留在 Buildx 缓存中）...'
  echo "💡 提示: 如需推送，执行: ALX_BASE_PUSH=1 $0"
  docker buildx build \
    --platform "$platforms" \
    -f Dockerfile.base \
    --output type=cacheonly \
    .
fi

echo '✅ 基础镜像构建完成。'
