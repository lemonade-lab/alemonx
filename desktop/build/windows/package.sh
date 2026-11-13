#!/bin/sh
set -eu

desktop_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
repo_dir=$(dirname "$desktop_dir")
output="$desktop_dir/bin"
mkdir -p "$output"
stage=$(mktemp -d "$output/.windows.XXXXXX")
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/payload"

version=${ALX_DESKTOP_VERSION:-dev}
export VITE_ALX_BUILD_ID="$version"
cd "$repo_dir"
yarn --cwd frontend build
(cd resources/packages/dsh && npm ci --ignore-scripts --no-audit --no-fund --os=win32 --cpu=x64)
ALX_DSH_TARGET_OS=windows ALX_DSH_TARGET_ARCH=amd64 ALX_DSH_ARCHIVE="$stage/dsh.zip" go run ./scripts/pack-dsh.go
for dir in resources/packages/yarn resources/packages/pm2; do
  (cd "$dir" && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)
done
printf '{"Replace":{"%s/resources/dsh/runtime.zip":"%s/dsh.zip"}}\n' "$repo_dir" "$stage" > "$stage/alx-overlay.json"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -overlay "$stage/alx-overlay.json" -trimpath -ldflags "-s -w -X main.Version=$version -X main.FrontendBuild=$version" -o "$stage/payload/alx.exe" .

cd "$desktop_dir"
mkdir "$stage/launcher"
cp ./*.go go.mod go.sum "$stage/launcher/"
mkdir "$stage/launcher/assets"
cp assets/tray.png "$stage/launcher/assets/"
wails3 generate syso -arch amd64 -icon build/windows/alemonx.ico -manifest build/windows/wails.exe.manifest -out "$stage/launcher/windows_amd64.syso"
cd "$stage/launcher"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags production -trimpath -ldflags "-s -w -H windowsgui" -o "$stage/payload/AlemonX Desktop.exe" .
(cd "$stage/payload" && zip -q "$stage/alemonx-desktop-windows-amd64.zip" "AlemonX Desktop.exe" alx.exe)
mv "$stage/alemonx-desktop-windows-amd64.zip" "$output/"
printf '%s\n' "$output/alemonx-desktop-windows-amd64.zip"

wails3 generate webview2bootstrapper -dir "$stage"
makensis "-DBOOTSTRAPPER=$stage/MicrosoftEdgeWebview2Setup.exe" "-DPAYLOAD=$stage/payload" "-DINSTALLER=$stage/alemonx-desktop-windows-amd64-installer.exe" "$desktop_dir/build/windows/installer.nsi"
mv "$stage/alemonx-desktop-windows-amd64-installer.exe" "$output/"
printf '%s\n' "$output/alemonx-desktop-windows-amd64-installer.exe"
