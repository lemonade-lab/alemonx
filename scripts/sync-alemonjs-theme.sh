#!/bin/sh
set -eu

# 将 docs/theme.json（主题契约源文件）同步到 internal/web/alemonjs_theme.json
# （go:embed 注入到所有 webview 的资源）。修改契约后运行本脚本即可生效。

project_dir="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
source_file="$project_dir/docs/theme.json"
target_file="$project_dir/internal/web/alemonjs_theme.json"

command -v python3 >/dev/null 2>&1 || { echo '需要 python3 来校验 JSON。' >&2; exit 1; }
python3 - "$source_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    data = json.load(handle)
if not all(str(key).startswith("alemonjs-") for key in data):
    raise SystemExit("theme.json 中存在不以 alemonjs- 开头的变量名")
PY

cp "$source_file" "$target_file"
echo "已同步: $target_file"
