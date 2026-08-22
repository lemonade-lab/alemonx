#!/bin/sh
set -eu

# A native local base is useful for rapid single-platform development builds.
image="${ALX_BASE_IMAGE:-alemonbase}"
version="${ALX_BASE_VERSION:-$(date -u +%Y%m%d)}"

command -v docker >/dev/null 2>&1 || { echo '❌ 未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo '❌ Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

echo '=========================================='
echo "📦 本地基础镜像: $image"
echo "🏷️  标签: local, $version"
echo '=========================================='

docker build \
  -f Dockerfile.base \
  -t "$image:local" \
  -t "$image:$version" \
  .

echo "✅ 已在本机生成 $image:local。"
