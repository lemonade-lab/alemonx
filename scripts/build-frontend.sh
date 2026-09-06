#!/bin/sh
set -eu

# Build the architecture-independent workbench before Docker receives its
# context. This keeps Vite on the invoking machine instead of inside BuildKit
# (or QEMU during multi-platform releases).
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repository_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)

cd "$repository_dir/frontend"
corepack enable
yarn install --frozen-lockfile --non-interactive
yarn build
