#!/bin/sh
set -eu

# ========== 配置 ==========
image="${ALX_IMAGE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonx}"
version="${ALX_VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)}"
platforms="${ALX_PLATFORMS:-linux/amd64,linux/arm64}"
push="${ALX_PUSH:-0}"
builder="${ALX_BUILDER:-alx-builder}"
runtime_base="${ALX_RUNTIME_BASE:-ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:latest}"

# ========== 检查环境 ==========
command -v docker >/dev/null 2>&1 || { echo '❌ 未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo '❌ Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

# ========== 配置 Builder ==========
if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  echo "🔧 创建新的 builder: $builder"
  docker buildx create --name "$builder" --config ./buildkitd.toml --use  --driver-opt network=host
else
  echo "🔧 使用已有 builder: $builder"
  docker buildx use "$builder"
fi
docker buildx inspect --bootstrap

# ========== 显示构建信息 ==========
echo "=========================================="
echo "📦 镜像: $image"
echo "🏷️  版本: $version"
echo "🖥️  平台: $platforms"
echo "🧱  运行基础: $runtime_base"
echo "📤 推送: $([ "$push" = '1' ] && echo '是' || echo '否')"
echo "=========================================="

# ========== 构建 ==========
if [ "$push" = '1' ]; then
  echo "🚀 构建并推送..."
  docker buildx build \
    --platform "$platforms" \
    --build-arg "ALX_RUNTIME_BASE=$runtime_base" \
    --build-arg "VERSION=$version" \
    -t "$image:latest" \
    -t "$image:$version" \
    --push \
    .
else
  echo "🔨 仅验证构建（结果仅在缓存中，不会生成或更新应用镜像）..."
  echo "💡 如需推送，执行: ALX_PUSH=1 $0"
  docker buildx build \
    --platform "$platforms" \
    --build-arg "ALX_RUNTIME_BASE=$runtime_base" \
    --build-arg "VERSION=$version" \
    -t "$image:latest" \
    -t "$image:$version" \
    --output type=cacheonly \
    .
fi

echo "=========================================="
echo "✅ 完成！"
echo "📦 镜像: $image"
echo "🏷️  标签: latest, $version"
if [ "$push" = '1' ]; then
  echo "📤 已推送到远程仓库"
else
  echo "💡 仅完成验证；远端和本地镜像均未更新"
fi
echo "=========================================="
