#!/bin/sh
set -eu

registry="ccr.ccs.tencentyun.com/ningmengchongshui"
image="${YUNZAI_IMAGE:-${registry}/alemonx-yunzai}"
version="${YUNZAI_VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev)}"
base_image="${YUNZAI_BASE_IMAGE:-${registry}/alemonx:${version}}"
platforms="${YUNZAI_PLATFORMS:-linux/amd64,linux/arm64}"
push="${YUNZAI_PUSH:-0}"
builder="${YUNZAI_BUILDER:-alemonx-yunzai-builder}"

for resource in \
  .resources/Miao-Yunzai-master.zip \
  .resources/miao-plugin-master.zip \
  .resources/alemonjs-load-yunzai.tar.gz \
  .resources/yunzai-resources.sha256; do
  [ -f "$resource" ] || { echo "缺少 Yunzai 本地资源：$resource。请先执行 make yunzai-resources。" >&2; exit 1; }
done

command -v docker >/dev/null 2>&1 || { echo '未检测到 Docker。' >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo 'Docker 未运行，或当前账户没有访问权限。' >&2; exit 1; }

if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  echo "创建 Buildx builder: $builder"
  docker buildx create --name "$builder" --use
else
  echo "使用 Buildx builder: $builder"
  docker buildx use "$builder"
fi
docker buildx inspect --bootstrap

echo "镜像: $image"
echo "版本: $version"
echo "基础镜像: $base_image"
echo "平台: $platforms"
echo "推送: $push"

build_args="--platform $platforms --build-arg BASE_IMAGE=$base_image -f Dockerfile.yunzai -t $image:latest -t $image:$version"

if [ "$push" = '1' ]; then
  echo '构建并推送 Yunzai 镜像...'
  # shellcheck disable=SC2086
  docker buildx build $build_args --push .
else
  echo '仅验证 Yunzai 镜像构建（结果保留在 Buildx 缓存）。'
  echo "如需推送，执行：YUNZAI_PUSH=1 $0"
  # shellcheck disable=SC2086
  docker buildx build $build_args .
fi
