#!/bin/sh
set -eu

desktop_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
repo_dir=$(dirname "$desktop_dir")
output="$desktop_dir/bin"
mkdir -p "$output"
stage=$(mktemp -d "$output/.package.XXXXXX")
trap 'rm -rf "$stage"' EXIT

app="$stage/AlemonX Desktop.app"
mkdir -p "$app/Contents/MacOS"
mkdir -p "$app/Contents/Resources"
cp "$desktop_dir/build/darwin/AlemonX.icns" "$app/Contents/Resources/"
cp "$desktop_dir/build/darwin/Info.plist" "$app/Contents/Info.plist"

version=${ALX_DESKTOP_VERSION:-dev}
export VITE_ALX_BUILD_ID="$version"
case "$version" in
  desktop-v*)
    bundle_version=${version#desktop-v}
    /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $bundle_version" "$app/Contents/Info.plist"
    /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $bundle_version" "$app/Contents/Info.plist"
    ;;
esac
cd "$repo_dir"
yarn --cwd frontend build
(cd resources/packages/dsh && npm ci --ignore-scripts --no-audit --no-fund)
go run ./scripts/pack-dsh.go
for dir in resources/packages/yarn resources/packages/pm2; do
  (cd "$dir" && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund)
done
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$version -X main.FrontendBuild=$version" -o "$app/Contents/MacOS/alx" .

cd "$desktop_dir"
CGO_ENABLED=1 go build -tags production -trimpath -o "$app/Contents/MacOS/AlemonX Desktop" .
codesign --force --sign - "$app/Contents/MacOS/alx"
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
previous="$stage/previous.app"
if [ -e "$output/AlemonX Desktop.app" ]; then
  mv "$output/AlemonX Desktop.app" "$previous"
fi
if ! mv "$app" "$output/"; then
  if [ -e "$previous" ]; then
    mv "$previous" "$output/AlemonX Desktop.app"
  fi
  exit 1
fi
printf '%s\n' "$output/AlemonX Desktop.app"
arch=$(go env GOARCH)
archive="alemonx-desktop-macos-$arch.zip"
ditto -c -k --keepParent "$output/AlemonX Desktop.app" "$stage/$archive"
mv "$stage/$archive" "$output/"
printf '%s\n' "$output/$archive"
