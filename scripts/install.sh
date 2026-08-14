#!/bin/sh
# Install the latest ALemonX release for macOS, Linux, or FreeBSD.
# Usage: curl -fsSL https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.sh | sh

set -eu

repository="${ALX_REPOSITORY:-lemonade-lab/alemonx}"
install_dir="${ALX_INSTALL_DIR:-$HOME/.local/bin}"
official_download_base="https://github.com/${repository}/releases/latest/download"
download_sources="
${ALX_DOWNLOAD_BASE:-}
${official_download_base}
https://ghfast.top/https://github.com/${repository}/releases/latest/download
https://ghproxy.net/https://github.com/${repository}/releases/latest/download
https://gh-proxy.com/https://github.com/${repository}/releases/latest/download
"

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
    curl --fail --location --connect-timeout 5 --max-time 300 --silent --show-error "$url" --output "$destination"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget --quiet --timeout=5 --tries=1 --output-document="$destination" "$url"
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
selected_download_source=""
for download_base in $download_sources; do
  download_base="${download_base%/}"
  case "$download_base" in
    https://*) ;;
    *)
      [ -n "$download_base" ] && printf '%s\n' "跳过非 HTTPS 下载源：$download_base" >&2
      continue
      ;;
  esac

  printf '%s\n' "尝试下载源：$download_base"
  rm -f "$work_dir/$asset" "$work_dir/SHA256SUMS"
  download "${download_base}/${asset}" "$work_dir/$asset" || continue
  download "${download_base}/SHA256SUMS" "$work_dir/SHA256SUMS" || continue

  expected_checksum="$(awk -v asset="$asset" '$2 == asset { print $1; exit }' "$work_dir/SHA256SUMS")"
  [ -n "$expected_checksum" ] || continue
  actual_checksum="$(sha256_file "$work_dir/$asset")"
  if [ "$expected_checksum" = "$actual_checksum" ]; then
    selected_download_source="$download_base"
    break
  fi
  printf '%s\n' "下载源校验失败，尝试下一个下载源。" >&2
done

[ -n "$selected_download_source" ] || fail "官方下载源和备用镜像均不可用，或未找到适用于 ${os}/${arch} 的校验安装包。"

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
