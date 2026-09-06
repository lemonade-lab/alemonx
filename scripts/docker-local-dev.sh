#!/bin/sh
set -eu

# Build only the host architecture. This is the fast, local counterpart to the
# multi-platform release builders: it refreshes both local images from the
# current checkout, then recreates the development container with them.
base_image="${ALX_LOCAL_BASE_IMAGE:-alemonbase:local}"
app_image="${ALX_LOCAL_IMAGE:-alemonx:local}"
app_version="${ALX_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

command -v docker >/dev/null 2>&1 || {
  echo 'Docker is not installed.' >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  echo 'Docker is not running, or the current account cannot access it.' >&2
  exit 1
}

if docker compose version >/dev/null 2>&1; then
  compose() { docker compose "$@"; }
elif command -v docker-compose >/dev/null 2>&1; then
  compose() { docker-compose "$@"; }
else
  echo 'Docker Compose is not installed.' >&2
  exit 1
fi

echo '==> Building the local runtime base for this machine'
docker build -f Dockerfile.base -t "$base_image" .

echo '==> Building the latest local ALemonX image'
./scripts/build-frontend.sh
docker build \
  --build-arg "ALX_RUNTIME_BASE=$base_image" \
  --build-arg "VERSION=$app_version" \
  -t "$app_image" \
  .

echo '==> Recreating the local development container'
export ALX_IMAGE="$app_image"
compose up -d --force-recreate
compose ps

echo 'Local development environment is ready at http://localhost:17390'
