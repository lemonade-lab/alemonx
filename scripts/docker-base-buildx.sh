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
  echo '✅ 基础镜像已构建并推送。'
  echo "📦 已更新: $image:latest, $image:$version"
else
  echo '🔨 使用 Builder 缓存进行多架构验证（不会生成或更新任何镜像）...'
  echo "💡 如需推送并更新远端镜像，执行: ALX_BASE_PUSH=1 $0"
  docker buildx build \
    --platform "$platforms" \
    -f Dockerfile.base \
    --output type=cacheonly \
    .
  echo '✅ 基础镜像多架构验证通过；远端和本地镜像均未更新。'
fi
