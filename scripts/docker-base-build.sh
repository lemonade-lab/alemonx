#!/bin/sh
set -eu

# The runtime base is intentionally local-only. Refresh it manually when Debian
# security updates or system-level runtime dependencies need to change.
image="${ALX_BASE_IMAGE:-alemonx-base}"
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
