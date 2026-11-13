#!/bin/sh
set -eu

desktop_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
repo_dir=$(dirname "$desktop_dir")
output="$desktop_dir/bin"
version=${ALX_DESKTOP_VERSION:-dev}
export VITE_ALX_BUILD_ID="$version"
[ "$(go env GOHOSTOS)" = linux ] || { echo "请在 Linux 上构建桌面包。" >&2; exit 1; }
pkg-config --exists gtk4 webkitgtk-6.0 || {
  echo "需要 GTK4 和 WebKitGTK 6.0 开发库。" >&2
  exit 1
}

arch=$(go env GOHOSTARCH)
case "$arch" in
  amd64) npm_arch=x64 ;;
  arm64) npm_arch=arm64 ;;
  *) echo "不支持的 Linux 桌面架构：$arch" >&2; exit 1 ;;
esac
export GOOS=linux GOARCH="$arch"

mkdir -p "$output"
stage=$(mktemp -d "$output/.linux.XXXXXX")
trap 'rm -rf "$stage"' EXIT
name="alemonx-desktop-linux-$arch"
app="$stage/$name"
mkdir -p "$app"

cd "$repo_dir"
yarn --cwd frontend build
(cd resources/packages/dsh && npm ci --ignore-scripts --no-audit --no-fund --os=linux --cpu="$npm_arch")
ALX_DSH_TARGET_OS=linux ALX_DSH_TARGET_ARCH="$arch" go run ./scripts/pack-dsh.go
for dir in resources/packages/yarn resources/packages/pm2; do
  (cd "$dir" && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)
done
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$version -X main.FrontendBuild=$version" -o "$app/alx" .

cd "$desktop_dir"
CGO_ENABLED=1 go build -tags production -trimpath -ldflags "-s -w" -o "$app/alemonx-desktop" .
cp build/linux/README.txt "$app/"
tar -C "$stage" -czf "$stage/$name.tar.gz" "$name"
mv "$stage/$name.tar.gz" "$output/"
printf '%s\n' "$output/$name.tar.gz"
