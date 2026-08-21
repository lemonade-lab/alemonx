#!/bin/sh
set -eu

seed=/opt/alx-yunzai-seed/bots/alemonb
workspace="${ALX_WORKSPACE:-/app/workspace}"
target="${workspace}/bots/alemonb"

# /app/workspace is normally a volume. Copy only missing paths so an image
# upgrade never overwrites robot code, configuration, data, or node_modules
# installed by the user. Keep this POSIX-shell implementation portable so the
# same first-run behaviour can be checked outside the Linux image as well.
copy_missing_tree() {
  source_root=$1
  destination_root=$2

  (
    cd "$source_root"
    find . -type d -print
  ) | while IFS= read -r path; do
    mkdir -p "$destination_root/$path"
  done

  (
    cd "$source_root"
    find . -type f -print
  ) | while IFS= read -r path; do
    destination="$destination_root/$path"
    if [ ! -e "$destination" ]; then
      cp -p "$source_root/$path" "$destination"
    fi
  done
}

if [ -d "$seed" ]; then
  mkdir -p "$target"
  copy_missing_tree "$seed" "$target"
fi

exec /app/alx "$@"
