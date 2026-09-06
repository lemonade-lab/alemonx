#!/bin/sh
set -eu

# Docker Compose mounts /app/workspace from the host. Seed missing top-level
# entries only, so image upgrades never overwrite bot code, plugin settings or
# QQ login state that an operator has already changed.
seed_root="${ALX_YUNZAI_SEED:-/opt/alx-yunzai-seed}"
workspace_root="${ALX_WORKSPACE:-/app/workspace}"

copy_missing_children() {
  source_dir="$1"
  destination_dir="$2"
  [ -d "$source_dir" ] || return 0
  mkdir -p "$destination_dir"
  for source in "$source_dir"/* "$source_dir"/.[!.]* "$source_dir"/..?*; do
    [ -e "$source" ] || continue
    name=${source##*/}
    [ -e "$destination_dir/$name" ] || cp -a "$source" "$destination_dir/$name"
  done
}

copy_missing_children "$seed_root/plugins" "$workspace_root/plugins"
copy_missing_children "$seed_root/bots" "$workspace_root/bots"

exec /usr/local/bin/alx-entrypoint "$@"
