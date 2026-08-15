#!/bin/sh
# Manage an ALemonX Docker deployment on Linux or macOS.
# Usage: curl -fsSLO https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/docker-install.sh
#        sh docker-install.sh up

set -eu

repository="${ALX_REPOSITORY:-lemonade-lab/alemonx}"
raw_base="${ALX_RAW_BASE:-https://raw.githubusercontent.com/${repository}/main}"
project_dir="${ALX_DOCKER_DIR:-$PWD}"

info() { printf '%s\n' "[INFO] $*"; }
fail() { printf '%s\n' "[ERROR] $*" >&2; exit 1; }

compose_command() {
  if docker compose version >/dev/null 2>&1; then
    printf '%s' 'docker compose'
  elif command -v docker-compose >/dev/null 2>&1; then
    printf '%s' 'docker-compose'
  else
    fail '未检测到 Docker Compose v2 或 docker-compose。'
  fi
}

check_docker() {
  command -v docker >/dev/null 2>&1 || fail '未检测到 Docker，请先安装并启动 Docker。'
  docker info >/dev/null 2>&1 || fail 'Docker 未运行，或当前账户没有访问权限。'
}

download() {
  url="$1"
  destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$destination"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$destination" "$url"
  else
    fail '需要 curl 或 wget 下载部署文件。'
  fi
}

ensure_file() {
  name="$1"
  if [ -f "$name" ] && [ "${FORCE_PULL:-0}" != '1' ]; then
    return
  fi
  info "下载 $name"
  temporary="${name}.tmp"
  if download "$raw_base/$name" "$temporary"; then
    mv "$temporary" "$name"
  else
    rm -f "$temporary"
    fail "无法下载 $raw_base/$name"
  fi
}

prepare() {
  mkdir -p "$project_dir"
  cd "$project_dir"
  ensure_file docker-compose.yml
  if [ ! -f .env ]; then
    ensure_file .env.example
    cp .env.example .env
    info '已创建 .env；请按需修改镜像、端口和机器人工作区。'
  fi
  workspace_path="${ALX_WORKSPACE:-}"
  if [ -z "$workspace_path" ] && [ -f .env ]; then
    workspace_path="$(sed -n 's/^[[:space:]]*ALX_WORKSPACE[[:space:]]*=[[:space:]]*//p' .env | sed -n '1p' | sed "s/^['\"]//;s/['\"]$//")"
  fi
  mkdir -p "${workspace_path:-./workspace}"
}

run_compose() {
  # shellcheck disable=SC2086
  $(compose_command) "$@"
}

usage() {
  cat <<'EOF'
用法: docker-install.sh <up|down|restart|pull|logs|status>

环境变量:
  ALX_DOCKER_DIR  部署目录（默认当前目录）
  ALX_RAW_BASE    docker-compose.yml 下载源
  FORCE_PULL=1    覆盖本地 docker-compose.yml/.env.example
EOF
}

action="${1:-}"
case "$action" in
  up)
    check_docker; prepare; run_compose pull; run_compose up -d; run_compose ps
    info '已启动。首次使用请访问 http://localhost:17390 并创建管理员账户。'
    ;;
  down)
    check_docker; cd "$project_dir"; [ -f docker-compose.yml ] || fail '未找到 docker-compose.yml。'; run_compose down
    ;;
  restart)
    check_docker; prepare; run_compose pull; run_compose up -d --force-recreate; run_compose ps
    ;;
  pull)
    check_docker; prepare; run_compose pull
    ;;
  logs)
    check_docker; cd "$project_dir"; run_compose logs -f --tail=200 alx
    ;;
  status|ps)
    check_docker; cd "$project_dir"; run_compose ps
    ;;
  -h|--help|help|'') usage ;;
  *) fail "未知操作：$action" ;;
esac
