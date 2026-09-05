#!/bin/sh
set -eu

# docker-compose mounts persistent data at /root after the image is built.
# Initialise known_hosts at container start so the mount cannot hide it.
ssh_dir="${HOME:-/root}/.ssh"
known_hosts="$ssh_dir/known_hosts"

mkdir -p "$ssh_dir"
chmod 700 "$ssh_dir"
touch "$known_hosts"
chmod 600 "$known_hosts"

for host in github.com gitee.com; do
  if ! ssh-keygen -F "$host" -f "$known_hosts" >/dev/null 2>&1; then
    # A temporary DNS or network issue must not prevent ALemonX from starting.
    # Git continues to enforce host-key verification for every recorded key.
    ssh-keyscan -T 5 "$host" >> "$known_hosts" 2>/dev/null || true
  fi
done

exec "$@"
