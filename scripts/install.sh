#!/bin/sh
# Install the latest ALemonX release for macOS, Linux, or FreeBSD.
# Usage: curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonjs-setup/main/scripts/install.sh | sh

set -eu

repository="${ALX_REPOSITORY:-lemonade-lab/alemonx}"
install_dir="${ALX_INSTALL_DIR:-$HOME/.local/bin}"
download_base="https://github.com/${repository}/releases/latest/download"

fail() {
  printf '%s\n' "安装失败：$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "需要命令 $1，请安装后重试。"
}

download() {
  url="$1"
  destination="$2"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --retry 2 --connect-timeout 10 --max-time 300 --silent --show-error "$url" --output "$destination"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget --quiet --timeout=10 --tries=3 --output-document="$destination" "$url"
    return
  fi
  fail "需要 curl 或 wget 才能下载安装包。"
}

sha256_file() {
  file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  if command -v sha256 >/dev/null 2>&1; then
    sha256 -q "$file"
    return
  fi
  fail "需要 sha256sum、shasum 或 sha256 才能校验安装包。"
}

platform() {
  case "$(uname -s)" in
    Darwin) printf '%s' darwin ;;
    Linux) printf '%s' linux ;;
    FreeBSD) printf '%s' freebsd ;;
    *) fail "暂不支持 $(uname -s)，请使用 Windows、macOS、Linux 或 FreeBSD。" ;;
  esac
}

architecture() {
  case "$(uname -m)" in
    x86_64|amd64) printf '%s' amd64 ;;
    arm64|aarch64) printf '%s' arm64 ;;
    i386|i486|i586|i686) printf '%s' 386 ;;
    armv7l|armv7|armhf) printf '%s' armv7 ;;
    ppc64le|s390x|riscv64) uname -m ;;
    *) fail "暂不支持 $(uname -m) 架构。" ;;
  esac
}

require_command unzip

os="$(platform)"
arch="$(architecture)"
asset="alx-${os}-${arch}.zip"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/alx-install.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM

printf '%s\n' "正在下载 ALemonX 最新版（${os}/${arch}）…"
download "${download_base}/${asset}" "$work_dir/$asset" || fail "没有找到适用于 ${os}/${arch} 的安装包。"
download "${download_base}/SHA256SUMS" "$work_dir/SHA256SUMS" || fail "无法下载安装包校验文件。"

expected_checksum="$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$work_dir/SHA256SUMS")"
[ -n "$expected_checksum" ] || fail "校验文件中未找到 $asset。"
actual_checksum="$(sha256_file "$work_dir/$asset")"
[ "$expected_checksum" = "$actual_checksum" ] || fail "安装包校验失败，已取消安装。"

unzip -q "$work_dir/$asset" -d "$work_dir/package"
[ -f "$work_dir/package/alx" ] || fail "安装包中缺少 alx 可执行文件。"

mkdir -p "$install_dir"
cp "$work_dir/package/alx" "$install_dir/alx"
chmod 0755 "$install_dir/alx"

printf '%s\n' "ALemonX 已安装到 $install_dir/alx"
case ":$PATH:" in
  *":$install_dir:"*) printf '%s\n' '现在可直接运行：alx' ;;
  *)
    printf '%s\n' "请重新打开终端后运行：$install_dir/alx"
    printf '%s\n' "如需直接使用 alx，请将 $install_dir 加入 PATH。"
    ;;
esac
